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

/*
eventType: Repository
type: github
repositoryEventType: Push, PullRequest, etc.
branch: name of the branch
collectionId: appsody collection id, if any
collectionVersion: appsody collection version, if any
forkedFrom: information about the original repository if a fork.
data: actual JSON data from github
*/

// RepositoryEvent A repository event
type RepositoryEvent struct {
	eventType           string `json:"eventType"`
	repoType            string `json:"type"`
	repositoryEventType string `json:"repositoryEventType"`
	branch              string `json:"branch"`
	collectionID        string `json:"collectionId"`
	collectionVersion   string `json:"collectionVersion"`
	forkedFrom          string `json:"forkedFrom"`
	data                string `json:"data"`
}

// NewRepositoryEvent Creates a new RepositoryEvent
func NewRepositoryEvent(repoType, repoEventType, branch, collectionID, collectionVersion, forkedFrom, data string) (*RepositoryEvent, error) {
	event := new(RepositoryEvent)
	event.eventType = "Repository"
	event.repoType = repoType
	event.repositoryEventType = repoEventType
	event.branch = branch
	event.collectionID = collectionID
	event.collectionVersion = collectionVersion
	event.forkedFrom = forkedFrom
	event.data = data
	return event, nil
}

// NewGitHubRepositoryEvent Creates a new GitHub RepositoryEvent
func NewGitHubRepositoryEvent(repoEventType, branch, collectionID, collectionVersion, forkedFrom, data string) (*RepositoryEvent, error) {
	return NewRepositoryEvent("github", repoEventType, branch, collectionID, collectionVersion, forkedFrom, data)
}
