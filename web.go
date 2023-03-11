package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v50/github"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
)

const appurl = "https://github.com/apps/sshpiper"

const templatefile = "web.tmpl"

type web struct {
	sessionstore sessionstore
	oauth        *oauth2.Config
	r            *gin.Engine
}

func newWeb(oauth *oauth2.Config, sessionstore sessionstore) (*web, error) {
	r := gin.Default()
	r.LoadHTMLFiles(templatefile)

	w := &web{
		r:            r,
		oauth:        oauth,
		sessionstore: sessionstore,
	}

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusTemporaryRedirect, appurl)
	})

	r.GET("/pipe/:session", w.pipe)
	r.GET("/oauth2callback", w.oauth2callback)
	r.POST("/approve/:session", w.approve)

	return w, nil
}

func (w *web) Run(addr string) error {
	return w.r.Run(addr)
}

func (w *web) pipe(c *gin.Context) {
	session := c.Param("session")

	if session == "" {
		c.Redirect(http.StatusTemporaryRedirect, appurl)
		return
	}

	// todo is valid session
	c.Redirect(http.StatusTemporaryRedirect, w.oauth.AuthCodeURL(session))
}

func (w *web) approve(c *gin.Context) {
	session := c.Param("session")
	if session == "" {
		c.Redirect(http.StatusTemporaryRedirect, appurl)
		return
	}

	upstreamConfig := &upstreamConfig{
		Host:           c.PostForm("host"),
		Username:       c.PostForm("username"),
		Password:       c.PostForm("password"),
		PrivateKeyData: c.PostForm("privatekey"),
		KnownHostsData: c.PostForm("knownhosts"),
	}

	w.sessionstore.SetUpstream(session, upstreamConfig)

	var errors []string
	var infos []string

	for {

		errmsg := w.sessionstore.GetSshError(session)
		if errmsg == nil {
			errors = append(errors, "session expired")
			break
		}

		if *errmsg == "" {
			time.Sleep(time.Millisecond * 300)
			continue
		}

		if *errmsg == errMsgPipeApprove {
			infos = append(infos, "ssh pipe approved")
		} else {
			errors = append(errors, *errmsg)
		}

		break
	}

	c.HTML(http.StatusOK, templatefile, gin.H{
		"errors": errors,
		"infos":  infos,
	})
}

func (w *web) oauth2callback(c *gin.Context) {
	code := c.Query("code")
	session := c.Query("state")

	if code == "" || session == "" {
		c.Redirect(http.StatusTemporaryRedirect, appurl)
		return
	}

	token, err := w.oauth.Exchange(context.Background(), code)

	if err != nil {
		c.HTML(http.StatusOK, templatefile, gin.H{
			"errors": []string{err.Error()},
		})
		return
	}

	tc := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(token))
	client := github.NewClient(tc)

	repos, _, err := client.Repositories.List(context.Background(), "", &github.RepositoryListOptions{
		Visibility: "private",
	})

	if err != nil {
		c.HTML(http.StatusOK, templatefile, gin.H{
			"errors": []string{err.Error()},
		})
		return
	}

	key, err := randomkey()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	var upstreams []upstreamConfig

	contentFound := false
	var errors []string

	for _, repo := range repos {
		if repo.FullName == nil {
			continue
		}

		fullname := strings.Split(*repo.FullName, "/")
		owner := fullname[0]
		reponame := fullname[1]
		conf, _, _, err := client.Repositories.GetContents(context.Background(), owner, reponame, "sshpiper.yaml", nil)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to get sshpiper.yaml from %s/%s: %v", owner, reponame, err))
			continue
		}

		content, err := conf.GetContent()
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to decode sshpiper.yaml from %s/%s: %v", owner, reponame, err))
			continue
		}

		contentFound = true

		var config pipeConfig
		if err := yaml.Unmarshal([]byte(content), &config); err != nil {
			errors = append(errors, fmt.Sprintf("failed to parse sshpiper.yaml from %s/%s: %v", owner, reponame, err))
		}

		for _, upstream := range config.Upstreams {
			upstream.Password, _ = encrypt(upstream.Password, key)
			upstream.PrivateKeyData, _ = encrypt(upstream.PrivateKeyData, key)
			upstream.Repo = *repo.FullName
			upstreams = append(upstreams, upstream)
		}
	}

	if len(upstreams) > 0 {
		w.sessionstore.SetSecret(session, key)
	}

	if len(repos) == 0 {
		errors = append(errors, "no private repositories found, please install github app to any of your private repositories")
	} else if !contentFound {
		errors = append(errors, "no sshpiper.yaml found in any private repositories, please add sshpiper.yaml")
	} else if len(upstreams) == 0 {
		errors = append(errors, "no valid upstreams found in sshpiper.yaml, please check sshpiper.yaml")
	}

	c.HTML(http.StatusOK, templatefile, gin.H{
		"upstreams": upstreams,
		"session":   session,
		"errors":    errors,
	})
}
