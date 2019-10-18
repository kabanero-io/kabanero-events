/*
Copyright 2019 IBM Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/go-github/github"
	ghw "gopkg.in/go-playground/webhooks.v3/github"
	"io/ioutil"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog"
	"net/http"
	"os"
	"strings"
)

// Event A GitHub Event
type Event string

// EventHandler Handler for an event
type EventHandler func(*GitHubListener, *http.Request) (interface{}, error)

// GitHubListener Event listener for GitHub events
type GitHubListener struct {
	DynamicClient dynamic.Interface
	Client        *github.Client
	Events        map[Event]EventHandler
}

var (
	// ErrInvalidHTTPMethod Only support the POST HTTP method for webhooks
	ErrInvalidHTTPMethod = errors.New("invalid HTTP method; must use HTTP POST")

	// ErrMissingGitHubEventHeader X-GitHub-Event header is required for GH payloads
	ErrMissingGitHubEventHeader = errors.New("missing X-GitHub-Event header")

	// ErrInvalidPayload Payload was invalid
	ErrInvalidPayload = errors.New("unable to parse event payload")

	// ErrUnknownEvent Event is either unknown or unsupported
	ErrUnknownEvent = errors.New("unknown or unsupported event")

	// ErrConnection Unable to connect to GH/GHE
	ErrConnection = errors.New("connection to GitHub failed")

	// ErrUserNameNotFound The GH_USER environment variable is required for the GitHub listener
	ErrUserNameNotFound = errors.New("GH_USER environment variable is required")

	// ErrTokenNotFound The GH_TOKEN environment variable is required for the GitHub listener
	ErrTokenNotFound = errors.New("GH_TOKEN environment variable is required")
)

const (
	// PushEvent GH Push event
	PushEvent Event = "push"
)

// NewGitHubEventListener Create a new GitHubListener
func NewGitHubEventListener(dynamicClient dynamic.Interface) (*GitHubListener, error) {
	// TODO: Read GH URL for each event.
	githubURL := strings.TrimSpace(os.Getenv("GH_URL"))

	// TODO: Switch to Secret and clean this up later.
	username := strings.TrimSpace(os.Getenv("GH_USER"))
	token := strings.TrimSpace(os.Getenv("GH_TOKEN"))

	var err error
	//username, token, err := getURLAPIToken(dynamicClient, webhookNamespace, githubURL)

	if err != nil {
		return nil, err
	}

	if username == "" {
		return nil, ErrUserNameNotFound
	}

	if token == "" {
		return nil, ErrTokenNotFound
	}

	tp := github.BasicAuthTransport{
		Username: username,
		Password: token,
	}

	var client *github.Client
	if githubURL == "" {
		client = github.NewClient(tp.Client())
	} else {
		client, err = github.NewEnterpriseClient(githubURL, githubURL, tp.Client())
	}

	if err != nil {
		return nil, err
	}

	listener := new(GitHubListener)
	listener.DynamicClient = dynamicClient
	listener.Client = client
	listener.Events = make(map[Event]EventHandler)
	listener.Events[PushEvent] = (*GitHubListener).handlePushEvent
	return listener, nil
}

// ParseEvent Parse GH and GHE events.
func (listener *GitHubListener) ParseEvent(r *http.Request) (interface{}, error) {
	if r.Method != http.MethodPost {
		return nil, ErrInvalidHTTPMethod
	}

	event := Event(r.Header.Get("X-GitHub-Event"))

	if event == "" {
		return nil, ErrMissingGitHubEventHeader
	}

	if eventHandler := listener.Events[event]; eventHandler != nil {
		return eventHandler(listener, r)
	}

	return nil, ErrUnknownEvent
}

// GetFile Retrieve file contents from a GitHub repository
func (listener *GitHubListener) GetFile(ghURL, owner, repo, path string) (string, error) {
	// TODO: Clean this up.
	username := strings.TrimSpace(os.Getenv("GH_USER"))
	token := strings.TrimSpace(os.Getenv("GH_TOKEN"))

	if username == "" {
		return "", ErrUserNameNotFound
	}

	if token == "" {
		return "", ErrTokenNotFound
	}

	tp := github.BasicAuthTransport{
		Username: username,
		Password: token,
	}

	var client *github.Client
	var err error
	if ghURL == "" {
		client = github.NewClient(tp.Client())
	} else {
		client, err = github.NewEnterpriseClient("https://api."+ghURL, ghURL, tp.Client())
	}

	rc, err := client.Repositories.DownloadContents(context.Background(), owner, repo, path, nil)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	if buf, err := ioutil.ReadAll(rc); err == nil {
		return string(buf), nil
	}

	return "", err
}

func readPayload(r *http.Request) (string, error) {
	payload, err := ioutil.ReadAll(r.Body)

	if err != nil || len(payload) == 0 {
		return "", ErrInvalidPayload
	}

	return string(payload), nil
}

func getPayload(data string, eventPayload interface{}) error {
	return json.Unmarshal([]byte(data), eventPayload)
}

func (listener *GitHubListener) handlePushEvent(r *http.Request) (interface{}, error) {
	data, err := readPayload(r)
	if err != nil {
		return nil, err
	}

	var payload ghw.PushPayload
	err = getPayload(data, &payload)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	ghURL := getGitHubURL(payload.Repository.HTMLURL)
	owner, repo := getOwnerAndRepo(payload.Repository.SSHURL)
	appsodyConfig, err := gitHubListener.GetFile(ghURL, owner, repo, ".appsody-config.yaml")
	if err == nil {
		klog.Infof("Appsody information from repo: %s", appsodyConfig)
	} else {
		klog.Error(err)
	}

	branch := payload.Ref
	branch = branch[strings.LastIndex(branch, "/"):]

	// Get appsody information
	appsodyConfig = appsodyConfig[len("stack:")+1:]
	sep := strings.Index(appsodyConfig, ":")
	collectionID := appsodyConfig[:sep]
	collectionVersion := appsodyConfig[sep+1:]

	event, err := NewGitHubRepositoryEvent("Push", branch, collectionID, collectionVersion, "", data)
	if err != nil {
		return nil, err
	}
	return event, nil
}

func getGitHubURL(url string) string {
	// Strip off `https://`
	url = url[8:]
	i := strings.Index(url, "/")
	return url[:i+1]
}

func getOwnerAndRepo(gitURL string) (string, string) {
	// Strip off the Git URL and .git suffix, leaving owner/repo
	gitURL = gitURL[strings.Index(gitURL, ":")+1:]
	gitURL = gitURL[:len(gitURL)-4]
	ownerAndRepo := strings.Split(gitURL, "/")
	return ownerAndRepo[0], ownerAndRepo[1]
}
