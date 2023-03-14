package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"net"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sethvargo/go-limiter/memorystore"
	"github.com/tg123/sshpiper/libplugin"
	"github.com/urfave/cli/v2"
	"golang.org/x/oauth2"
	githubendpoint "golang.org/x/oauth2/github"
)

const errMsgPipeApprove = "ok"
const errMsgBadUpstreamCred = "bad upstream credential"

func main() {

	gin.DefaultWriter = os.Stderr

	libplugin.CreateAndRunPluginTemplate(&libplugin.PluginTemplate{
		Name:  "githubapp",
		Usage: "sshpiperd githubapp plugin",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "webaddr",
				Value:   ":3000",
				EnvVars: []string{"SSHPIPERD_GITHUBAPP_WEBADDR"},
			},
			&cli.StringFlag{
				Name:     "baseurl",
				EnvVars:  []string{"SSHPIPERD_GITHUBAPP_BASEURL"},
				Required: true,
			},
			&cli.StringFlag{
				Name:     "clientid",
				EnvVars:  []string{"SSHPIPERD_GITHUBAPP_CLIENTID"},
				Required: true,
			},
			&cli.StringFlag{
				Name:     "clientsecret",
				EnvVars:  []string{"SSHPIPERD_GITHUBAPP_CLIENTSECRET"},
				Required: true,
			},
		},
		CreateConfig: func(c *cli.Context) (*libplugin.SshPiperPluginConfig, error) {

			store, err := newSessionstoreMemory()
			if err != nil {
				return nil, err
			}

			baseurl := c.String("baseurl")

			w, err := newWeb(&oauth2.Config{
				ClientID:     c.String("clientid"),
				ClientSecret: c.String("clientsecret"),
				Endpoint:     githubendpoint.Endpoint,
				RedirectURL:  fmt.Sprintf("%s/oauth2callback", baseurl),
			}, store)

			if err != nil {
				return nil, err
			}

			go func() {
				panic(w.Run(c.String("webaddr")))
			}()

			limiter, err := memorystore.New(&memorystore.Config{
				Tokens:      3,
				Interval:    time.Minute,
				SweepMinTTL: time.Minute * 5,
			})

			if err != nil {
				return nil, err
			}

			return &libplugin.SshPiperPluginConfig{
				KeyboardInteractiveCallback: func(conn libplugin.ConnMetadata, client libplugin.KeyboardInteractiveChallenge) (u *libplugin.Upstream, err error) {
					session := conn.UniqueID()

					defer func() {
						if err != nil {
							store.SetSshError(session, err.Error())
						} else {
							store.SetSshError(session, errMsgPipeApprove) // this happens before pipestart, but it's ok because pipestart may timeout due to network issues
						}
					}()

					lasterr := store.GetSshError(session)

					if lasterr == nil {
						// new session
						_, _ = client("", fmt.Sprintf("please open %v/pipe/%v with your browser to verify (timeout 1m)", baseurl, session), "", false)
						store.SetSshError(session, "") // set waiting for approval

					} else if *lasterr != "" {

						// check if retry
						if *lasterr != errMsgBadUpstreamCred {
							_, _ = client("", fmt.Sprintf("your password/private key in sshpiper.yaml auth failed with upstream %v", *lasterr), "", false)
							store.SetSshError(session, errMsgBadUpstreamCred) // set already notified
						}

						return nil, fmt.Errorf(errMsgBadUpstreamCred)
					}

					st := time.Now()

					for {

						if time.Now().After(st.Add(time.Second * 60)) {
							return nil, fmt.Errorf("timeout waiting for approval")
						}

						upstream, _ := store.GetUpstream(session)
						if upstream == nil {
							time.Sleep(time.Millisecond * 100)
							continue
						}

						key, _ := store.GetSecret(session)
						if key == nil {
							return nil, fmt.Errorf("secret expired")
						}

						host, port, err := libplugin.SplitHostPortForSSH(upstream.Host)
						if err != nil {
							return nil, err
						}

						var resolvedips []string
						ips, err := net.LookupIP(host)
						if err != nil {
							return nil, err
						}

						for _, ip := range ips {
							if !ip.IsPrivate() {
								resolvedips = append(resolvedips, ip.String())
							}
						}

						if len(resolvedips) == 0 {
							return nil, fmt.Errorf("no public ip found for %v", host)
						}

						// choose random ip from resolveips
						selectedip := resolvedips[rand.Intn(len(resolvedips))]

						hosttoshow := upstream.Host

						if host != selectedip {
							hosttoshow = fmt.Sprintf("%v (%v)", upstream.Host, selectedip)
						}

						u = &libplugin.Upstream{
							UserName:      upstream.Username,
							Host:          selectedip,
							Port:          int32(port),
							IgnoreHostKey: upstream.KnownHostsData == "",
						}

						password, _ := decrypt(upstream.Password, key)
						privateKeyData, _ := decrypt(upstream.PrivateKeyData, key)

						remoteuser := upstream.Username
						if remoteuser == "" {
							remoteuser = conn.User()
						}

						if privateKeyData != "" {
							priv, err := base64.StdEncoding.DecodeString(privateKeyData)
							if err != nil {
								return nil, err
							}

							u.Auth = libplugin.CreatePrivateKeyAuth(priv)

							_, _ = client("", fmt.Sprintf("piping to %v@%v with private key", remoteuser, hosttoshow), "", false)

							return u, nil
						}

						if password != "" {
							u.Auth = libplugin.CreatePasswordAuth([]byte(password))

							_, _ = client("", fmt.Sprintf("piping to %v@%v with password", remoteuser, hosttoshow), "", false)

							return u, nil
						}

						_, _ = client("", fmt.Sprintf("piping to %v@%v with none auth", remoteuser, hosttoshow), "", false)

						u.Auth = libplugin.CreateNoneAuth()
						return u, nil
					}
				},
				NewConnectionCallback: func(conn libplugin.ConnMetadata) error {
					ip, _, _ := net.SplitHostPort(conn.RemoteAddr())
					_, _, _, ok, err := limiter.Take(context.Background(), ip)
					if err != nil {
						return err
					}

					if !ok {
						return fmt.Errorf("too many connections")
					}

					return nil
				},
				UpstreamAuthFailureCallback: func(conn libplugin.ConnMetadata, method string, err error, allowmethods []string) {
					session := conn.UniqueID()
					store.SetSshError(session, err.Error())
					store.DeleteSession(session, true)
				},
				PipeStartCallback: func(conn libplugin.ConnMetadata) {
					session := conn.UniqueID()
					store.SetSshError(session, errMsgPipeApprove)
					store.DeleteSession(session, true)
				},
				PipeErrorCallback: func(conn libplugin.ConnMetadata, err error) {
					session := conn.UniqueID()
					store.DeleteSession(session, false)

					ip, _, _ := net.SplitHostPort(conn.RemoteAddr())
					limiter.Burst(context.Background(), ip, 1)
				},
				VerifyHostKeyCallback: func(conn libplugin.ConnMetadata, hostname, netaddr string, key []byte) error {
					session := conn.UniqueID()

					upstream, _ := store.GetUpstream(session)

					if upstream == nil {
						return fmt.Errorf("connection expired")
					}

					if upstream.KnownHostsData == "" {
						return nil
					}

					data, err := base64.StdEncoding.DecodeString(upstream.KnownHostsData)
					if err != nil {
						return err
					}

					return libplugin.VerifyHostKeyFromKnownHosts(bytes.NewBuffer(data), hostname, netaddr, key)
				},
			}, nil
		},
	})
}
