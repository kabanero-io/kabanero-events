package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/go-github/github"
	"io/ioutil"
	"k8s.io/klog"
	"net/http"
	"os"
	"strings"
	ghw "gopkg.in/go-playground/webhooks.v3/github"
)

// Event A GitHub Event
type Event string

// EventHandler Handler for an event
type EventHandler func(*http.Request) (interface{}, error)

// GitHubListener Event listener for GitHub events
type GitHubListener struct {
	Client *github.Client
	Events  map[Event]EventHandler
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
func NewGitHubEventListener() (*GitHubListener, error) {
	// TODO: Read GH URL from CRD?
	githubURL := strings.TrimSpace(os.Getenv("GH_URL"))

	// TODO: Switch to Secret
	username := strings.TrimSpace(os.Getenv("GH_USER"))
	password := strings.TrimSpace(os.Getenv("GH_TOKEN"))

	if username == "" {
		return nil, ErrUserNameNotFound
	}

	if password == "" {
		return nil, ErrTokenNotFound
	}

	tp := github.BasicAuthTransport{
		Username: username,
		Password: password,
	}

	var client *github.Client
	var err error

	if githubURL == "" {
		client = github.NewClient(tp.Client())
	} else {
		client, err = github.NewEnterpriseClient(githubURL, githubURL, tp.Client())
	}

	if err != nil {
		return nil, err
	}

	listener := new(GitHubListener)
	listener.Client = client
	listener.Events = make(map[Event]EventHandler)
	listener.Events[PushEvent] = handlePushEvent
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
		return eventHandler(r)
	}

	return nil, ErrUnknownEvent
}

// GetFile Retrieve file contents from a GitHub repository
func (listener *GitHubListener) GetFile(owner, repo, path string) (string, error) {
	rc, err := listener.Client.Repositories.DownloadContents(context.Background(), owner, repo, path, nil)
	//rc.Close()

	if err != nil {
		return "", err
	}

	if buf, err := ioutil.ReadAll(rc); err == nil {
		return string(buf), nil
	}

	return "", err
}

func getPayload(r *http.Request, eventPayload interface{}) error {
	payload, err := ioutil.ReadAll(r.Body)

	if err != nil || len(payload) == 0 {
		return ErrInvalidPayload
	}

	return json.Unmarshal([]byte(payload), eventPayload)
}

func handlePushEvent(r *http.Request) (interface{}, error) {
	var payload ghw.PushPayload
	err := getPayload(r, &payload)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	return payload, nil
}

// GetOwnerAndRepo Get the owner and repo of a Git repository
func GetOwnerAndRepo(gitURL string) (string, string) {
	// Strip off the Git URL and .git suffix, leaving owner/repo
	gitURL = gitURL[strings.Index(gitURL, ":") + 1:]
	gitURL = gitURL[:len(gitURL) - 4]
	ownerAndRepo := strings.Split(gitURL, "/")
	return ownerAndRepo[0], ownerAndRepo[1]
}
