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
)



/* HTTP listsnert */
func listenerHandler(writer http.ResponseWriter, req *http.Request) {
    header := req.Header
	klog.Infof("Recevied request. Header: %v", header)

    finalMessage := make(map[string]interface{})
	finalMessage["type"] = "Repository"
	eventType, ok := header["x-github-event"]
	if !ok {
		klog.Errorf("header does not contain x-github-event. Skipping")
		return
	}
	finalMessage["repositoryEvent"] = eventType
	finalMessage["repository"] = "github"




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

	/* Create a new message */
	finalMessage["data"] = bodyMap

	htmlURL, err := "https://github.ibm.com", nil //getHTMLURL(bodyMap)
	if err != nil {
		fmt.Errorf("Unable to get html URL from message");
		return
	}

    /*user, token*/ _, _ , err = getURLAPIToken(dynamicClient, webhookNamespace, htmlURL )
	if err != nil {
		klog.Errorf("Unable to get user/token secrets for URL %v", htmlURL);
		return
	}

	// collectionID, collectionVersion, bool, err := getAppsodyInfo(user, token, htmlURL)
	
	finalMessage["collectionID"] = "nodejs-express-appsody"
	finalMessage["collectionVersion"] = "0.2"
	finalMessage["refs"] = [] string { "refs", "heads", "master" }
}


func newListener() error{

    http.HandleFunc("/webhook", listenerHandler)
	klog.Infof("Starting listener on port 9080");
    err := http.ListenAndServe(":9080", nil)
	return err
}

/* Get the repository's htmlURL from github message */
func getHTMLURL(body map[string]interface{}) (string, error){
	repositoryObj, ok := body["repository"]
	if !ok {
		return "", fmt.Errorf("Unable to find repository in map")
	}
	repository, ok := repositoryObj.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("repository object not map[string]interface{}: %v", repositoryObj)
	}

	htmlURLObj, ok := repository["html_url"]
	if !ok {
		return "", fmt.Errorf("html_URL not found")
	}
	htmlURL, ok := htmlURLObj.(string)
	return htmlURL, nil
}

// Model
type Package struct {
	FullName      string
	Description   string
	StarsCount    int
	ForksCount    int
	LastUpdatedBy string
}

func testGithubEnterprise() error {

	context := context.Background()
    tp := github.BasicAuthTransport{
       Username: "mcheng",
       Password: "fa9737e13f9c7688381bae20430d2148ed5b171f",
    }
/*
	tokenService := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: "fa9737e13f9c7688381bae20430d2148ed5b171f"},
	)
	tokenClient := oauth2.NewClient(context, tokenService)
*/

	client, err := github.NewEnterpriseClient("https://github.ibm.com/api/v3", "https://github.ibm.com/api/v3", tp.Client())
	if err != nil {
		return err
	}

	repo, _, err := client.Repositories.Get(context, "kabanero-org-test", "test1")

	if err != nil {
		return fmt.Errorf("Problem in getting repository information %v\n", err)
	}

	pack := &Package{
		FullName: *repo.FullName,
		Description: *repo.Description,
		ForksCount: *repo.ForksCount,
		StarsCount: *repo.StargazersCount,
	}

	fmt.Printf("%+v\n", pack)


     rc, err := client.Repositories.DownloadContents(context, "kabanero-org-test", "test1", ".appsody-config.yaml", nil)
     if err != nil {
         return err
     }
     defer rc.Close()

	 buf, err := ioutil.ReadAll(rc)
	 if  err != nil {
		return err
	 }

	 fmt.Printf(".appsody-config.yaml: %s", string(buf))
	return nil
}
