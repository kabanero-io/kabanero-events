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
	"encoding/json"
	"net/http"
	"io"
	"io/ioutil"
	"k8s.io/klog"
	"github.com/google/go-github/github"
	// "golang.org/x/oauth2"
	"context"
	"fmt"
	"strings"
)



/* HTTP listsnert */
func listenerHandler(writer http.ResponseWriter, req *http.Request) {

    header := req.Header
	klog.Infof("Recevied request. Header: %v", header)

    initialVariables := make(map[string]interface{})
	initialVariables["eventType"] = "Repository"
	reposiotryEventHeader, ok := header[http.CanonicalHeaderKey("x-github-event")]
	if !ok {
		klog.Errorf("header does not contain x-github-event. Skipping")
		return
	}
	initialVariables["repositoryEvent"] = reposiotryEventHeader[0]
	initialVariables["repositoryType"] = "github"
	initialVariables[CONTROLNAMESPACE] = webhookNamespace

	hostHeader, isEnterprise := header[http.CanonicalHeaderKey("x-github-enterprise-host")]
	if !isEnterprise {
		klog.Errorf("header does not contain x-github-enterprise-host. Skipping")
		return
	} 
	host := hostHeader[0]

	var body io.ReadCloser = req.Body

	defer body.Close()
	bytes, err := ioutil.ReadAll(body)
	if err != nil {
		klog.Errorf("Webhook listener can not read body. Error: %v", err);
	} else {
	 	klog.Infof("Webhook listener received body: %v", string(bytes))
    }

	var bodyMap map[string]interface{}
	err = json.Unmarshal(bytes, &bodyMap)
	if err != nil {
		klog.Errorf("Unable to unarmshal json body: %v", err)
		return
	}

	owner, name, htmlURL, err := getRepositoryInfo(bodyMap)
	if err != nil {
		klog.Errorf("Unable to get repository owner, name, or html_url from webhook message: %v", err);
		return
	}

    user, token , err := getURLAPIToken(dynamicClient, webhookNamespace, htmlURL )
	if err != nil {
		klog.Errorf("Unable to get user/token secrets for URL %v", htmlURL);
		return
	}

	githubURL := "https://" + host
	collectionPrefix, collectionID, collectionVersion, found, err := downloadAppsodyConfig(owner, name, githubURL, user, token, isEnterprise)
	if err != nil {
		klog.Errorf("Error checking whether repository is appsody: %v", err);
		return
	}
	if found {
		initialVariables["collectionPrefix"] = collectionPrefix
		initialVariables["collectionID"] = collectionID
		initialVariables["collectionVersion"] = collectionVersion
	}

	// get the refs if they xist
	refsObj, ok := bodyMap["ref"]
	if ok {
		refs, ok := refsObj.(string)
		if !ok {
			klog.Errorf("Found refs, but it is not a string: %v", refs);
			return
		}
		refsArray := strings.Split(refs, "/")
		initialVariables["ref"] = refsArray
	}

	message := make(map[string]interface{})
	message[KABANERO] = initialVariables
	message[EVENT] = bodyMap
	err = triggerProc.processMessage(message)
	if err != nil {
		klog.Errorf("Error processing webhook message: %v", err)
	}

}


func newListener() error{

    http.HandleFunc("/webhook", listenerHandler)
	klog.Infof("Starting listener on port 9080");
    err := http.ListenAndServe(":9080", nil)
	return err
}

/* Get the repository's information from from github message: name, owner, and html_url */
func getRepositoryInfo(body map[string]interface{}) (string, string, string, error){
	repositoryObj, ok := body["repository"]
	if !ok {
		return "", "", "", fmt.Errorf("Unable to find repository in webhook message")
	}
	repository, ok := repositoryObj.(map[string]interface{})
	if !ok {
		return "", "", "", fmt.Errorf("webhook message repository object not map[string]interface{}: %v", repositoryObj)
	}

	nameObj, ok := repository["name"]
	if !ok {
		return "", "", "", fmt.Errorf("webhook message repository name not found")
	}
	name, ok := nameObj.(string)
	if !ok {
		return "", "", "", fmt.Errorf("webhook message repository name not a string: %v", nameObj)
	}

	ownerMapObj, ok := repository["owner"]
	if !ok {
		return "", "", "", fmt.Errorf("webhook message repository owner not found")
	}
	ownerMap, ok := ownerMapObj.(map[string]interface{})
	if !ok {
		return "", "", "", fmt.Errorf("webhook message repository owner object not map[string]interface{}: %v", ownerMapObj)
	}
	ownerObj, ok := ownerMap["login"]
	if !ok {
		return "", "", "", fmt.Errorf("webhook message repository owner login not found")
	}
	owner, ok := ownerObj.(string)
	if !ok {
		return "", "", "", fmt.Errorf("webhook message repository owner login not string : %v", ownerObj)
	}


	htmlURLObj, ok := repository["html_url"]
	if !ok {
		return "", "", "", fmt.Errorf("webhook message repository html_url not found")
	}
	htmlURL, ok := htmlURLObj.(string)
	if !ok {
		return "", "", "", fmt.Errorf("webhook message html_url not string: %v", htmlURL)
	}

	return owner, name,  htmlURL, nil
}


/*
func testGithubEnterprise() error {
    prefix, collection, version, err := downloadAppsodyConfig("kabanero-org-test", "test1", "https://github.ibm.com", "w3id", "token", true)
	if err != nil {
		return err
	}
	fmt.Printf("prefix: %s, collection: %s, version: %s\n", prefix, collection, version)
	return nil
}
*/

/* Download .appsody-cofig.yaml and return: prefix, collection, version, true if file exists, and error 
 for example:  stack: kabanero/nodejs-express:0.2 
	prefix: kabanero
	collection: nodejs-express
	version: 0.2
*/
func downloadAppsodyConfig(owner, repository, githubURL, user, token string, isEnterprise bool) (string, string, string, bool, error) {

	context := context.Background()

	/* TODO: add SSL */
    tp := github.BasicAuthTransport{
       Username: user,
       Password: token,
    }
/*
	tokenService := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tokenClient := oauth2.NewClient(context, tokenService)
*/

	var err error
	var client *github.Client
	if isEnterprise {
		githubURL = githubURL + "/api/v3"
		client, err = github.NewEnterpriseClient(githubURL, githubURL, tp.Client())
		if err != nil {
			return "", "", "", false, err
		}
	} else {
		client = github.NewClient(tp.Client())
	}

    rc, err := client.Repositories.DownloadContents(context, owner, repository, ".appsody-config.yaml", nil)
    if err != nil {
		fmt.Printf("Error type: %T, value: %v\n", err, err)
        return "", "", "", false, err
    }
    defer rc.Close()

	buf, err := ioutil.ReadAll(rc)
	if  err != nil {
		return "", "", "", false, err
	}
	
	/* stack: kabanero/nodejs-express:0.2 */
	bufStr := string(buf)
	components := strings.Split(bufStr, ":")
	if len(components) == 3 {
		prefixName := strings.Trim(components[1], " ")
		prefixNameArray := strings.Split(prefixName, "/")
		if len(prefixNameArray) == 2 {
			return prefixNameArray[0], prefixNameArray[1], components[2], true, nil
		} 
	} 
	return "", "", "", false, fmt.Errorf(".appsody-config.yaml contains %s.  It is not of the format stacK: prefix/name:version", bufStr)

}

