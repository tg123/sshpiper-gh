package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tg123/sshpiper/libplugin"
	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
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

			return &libplugin.SshPiperPluginConfig{
				KeyboardInteractiveCallback: func(conn libplugin.ConnMetadata, client libplugin.KeyboardInteractiveChallenge) (u *libplugin.Upstream, err error) {
					session := conn.UniqueID()

					defer func() {
						if err != nil {
							store.SetSshError(session, err.Error())
						} else {
							store.SetSshError(session, errMsgPipeApprove)
						}
					}()

					{
						// check if retry
						lasterr := store.GetSshError(session)
						if lasterr == errMsgPipeApprove {
							_, _ = client("", "your password/private key in sshpiper.yaml auth failed with upstream", "", false)
							return nil, fmt.Errorf(errMsgBadUpstreamCred)
						}

						if lasterr == errMsgBadUpstreamCred {
							return nil, fmt.Errorf(errMsgBadUpstreamCred)
						}
					}

					_, _ = client("", fmt.Sprintf("please open %v/pipe/%v with your browser to verify (timeout 1m)", baseurl, session), "", false)

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

						u = &libplugin.Upstream{
							UserName:      upstream.Username,
							Host:          host,
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

							_, _ = client("", fmt.Sprintf("piping to %v@%v with private key", remoteuser, upstream.Host), "", false)

							return u, nil
						}

						if password != "" {
							u.Auth = libplugin.CreatePasswordAuth([]byte(password))

							_, _ = client("", fmt.Sprintf("piping to %v@%v with password", remoteuser, upstream.Host), "", false)

							return u, nil
						}

						_, _ = client("", fmt.Sprintf("piping to %v@%v with none auth", remoteuser, upstream.Host), "", false)

						u.Auth = libplugin.CreateNoneAuth()
						return u, nil
					}
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

					hostKeyCallback, err := knownhosts.NewFromReader(bytes.NewBuffer(data))
					if err != nil {
						return err
					}

					pub, err := ssh.ParsePublicKey(key)
					if err != nil {
						return err
					}

					addr, err := net.ResolveTCPAddr("tcp", netaddr)
					if err != nil {
						return err
					}

					return hostKeyCallback(hostname, addr, pub)
				},
			}, nil
		},
	})
}
