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
	"fmt"
	"encoding/base64"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	V1 = "v1"
	DATA = "data"
    URL = "url"
	USERNAME = "username"
	TOKEN = "token"
	SECRETS = "secrets"
)

/*
 Find the user/token for a Github APi KEy
 The format of the secret:
    apiVersion: v1
    kind: Secret
    metadata:
      name: <name of secret>
    type: Opaque
	data:
      url: https://github.com/org  or https://github.com/org/repo
      username: base64 encoded user
	  token: base64 encoded token

 If the url in the secret is a prefix of repoURL, and username and token are defined, then return the user and token.
 Return user, token, error. 
 TODO: Change to controller pattern and cache the secrets.
 */
func getURLAPIToken(dynInterf dynamic.Interface, namespace string, repoURL string) (string, string, error){
	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  V1,
		Resource: SECRETS,
	}
	var intfNoNS = dynInterf.Resource(gvr)
	var intf dynamic.ResourceInterface
	intf = intfNoNS.Namespace(namespace)

	// fetch the current resource
	var unstructuredList *unstructured.UnstructuredList
	var err error
	unstructuredList, err = intf.List(metav1.ListOptions{})
	if err != nil {
		return "", "", err
	}

	for _, unstructuredObj := range unstructuredList.Items {
	    var objMap = unstructuredObj.Object
		dataMapObj, ok := objMap[DATA]
		if !ok {
			continue
		}

    	dataMap, ok := dataMapObj.(map[string]interface{})
    	if !ok {
    		continue
		}

		urlObj, ok := dataMap[URL]
		if !ok {
			continue
		}
		url, ok := urlObj.(string)
		if !ok {
			continue
		}

		if !strings.HasPrefix(repoURL, url) {
            /* not for this uRL */
            continue
		}

		usernameObj, ok := dataMap[USERNAME]
		if !ok {
			continue
		}
		username, ok := usernameObj.(string)
		if !ok {
			continue
		}

		tokenObj, ok := dataMap[TOKEN]
		if !ok {
			continue
		}
		token, ok := tokenObj.(string)
		if !ok {
			continue
		}

		decodedUserName, err := base64.StdEncoding.DecodeString(username)
		if err != nil {
			return "", "", err
		}

		decodedToken, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			return "", "", err
		}
		return string(decodedUserName), string(decodedToken), nil
	}
	return "", "", fmt.Errorf("Unable to find API token for url: %s", repoURL)
}