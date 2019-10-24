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
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog"
)

/* Kubernetes and Kabanero yaml constants*/
const (
	V1                         = "v1"
	V1ALPHA1                   = "v1alpha1"
	KABANEROIO                 = "kabanero.io"
	KABANERO                   = "kabanero"
	KABANEROS                  = "kabaneros"
	DATA                       = "data"
	URL                        = "url"
	USERNAME                   = "username"
	TOKEN                      = "token"
	SECRETS                    = "secrets"
	SPEC                       = "spec"
	COLLECTIONS                = "collections"
	REPOSITORIES               = "repositories"
	ACTIVATEDEFAULTCOLLECTIONS = "activateDefaultCollections"

    maxNameLength  = 253 // max length of a name in Kubernetes
)

/*
 Find the user/token for a Github APi KEy
 The format of the secret:
apiVersion: v1
kind: Secret
metadata:
  name:  kabanero-org-test-secret
  namespace: kabanero
  annotations:
   url: https://github.ibm.com/kabanero-org-test
type: Opaque
stringData:
  url: <url to org or repo>
data:
  username:  <base64 encoded user name>
  token: <base64 encoded token>

 If the url in the secret is a prefix of repoURL, and username and token are defined, then return the user and token.
 Return user, token, error.
 TODO: Change to controller pattern and cache the secrets.

Return: username, token, secret name, error
*/
func getURLAPIToken(dynInterf dynamic.Interface, namespace string, repoURL string) (string, string, string, error) {
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
		return "", "", "", err
	}

	for _, unstructuredObj := range unstructuredList.Items {
		var objMap = unstructuredObj.Object

		metadataObj, ok := objMap[METADATA]
		if !ok {
			continue
		}

		metadata, ok := metadataObj.(map[string]interface{})
		if !ok {
			continue
		}

        nameObj, ok := metadata["name"]
        if !ok {
            continue
        }
        name, ok := nameObj.(string)
        if !ok {
            continue
        } 

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
		decodedURLBytes, err := base64.StdEncoding.DecodeString(url)
		if err != nil {
			return "", "", "", err
		}

		decodedURL := string(decodedURLBytes)
		if !strings.HasPrefix(repoURL, decodedURL) {
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
			return "", "", "", err
		}

		decodedToken, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			return "", "", "", err
		}
		return string(decodedUserName), string(decodedToken), name,  nil
	}
	return "", "", "", fmt.Errorf("Unable to find API token for url: %s", repoURL)
}

/* Get the URL to kabanero-index.yaml
 */
func getKabaneroIndexURL(dynInterf dynamic.Interface, namespace string) (string, error) {
	if klog.V(5) {
		klog.Infof("Entering getKabaneroIndexURL")
		defer klog.Infof("Leaving getKabaneroIndexURL")
	}

	gvr := schema.GroupVersionResource{
		Group:    KABANEROIO,
		Version:  V1ALPHA1,
		Resource: KABANEROS,
	}
	var intfNoNS = dynInterf.Resource(gvr)
	var intf dynamic.ResourceInterface
	intf = intfNoNS.Namespace(namespace)

	// fetch the current resource
	var unstructuredList *unstructured.UnstructuredList
	var err error
	unstructuredList, err = intf.List(metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Unable to list resource of kind kabanero in the namespace %s", namespace)
		return "", err
	}

	for _, unstructuredObj := range unstructuredList.Items {
		if klog.V(5) {
			klog.Infof("Processing kabanero CRD instance: %v", unstructuredObj)
		}
		var objMap = unstructuredObj.Object
		specMapObj, ok := objMap[SPEC]
		if !ok {
			if klog.V(5) {
				klog.Infof("    kabanero CRD instance: has no spec section. Skipping")
			}
			continue
		}

		specMap, ok := specMapObj.(map[string]interface{})
		if !ok {
			if klog.V(5) {
				klog.Infof("    kabanero CRD instance: spec section is type %T. Skipping", specMapObj)
			}
			continue
		}

		collectionsMapObj, ok := specMap[COLLECTIONS]
		if !ok {
			if klog.V(5) {
				klog.Infof("    kabanero CRD instance: spec section has no collections section. Skipping")
			}
			continue
		}
		collectionMap, ok := collectionsMapObj.(map[string]interface{})
		if !ok {
			if klog.V(5) {
				klog.Infof("    kabanero CRD instance: collections type is %T. Skipping", collectionsMapObj)
			}
			continue
		}

		repositoriesInterface, ok := collectionMap[REPOSITORIES]
		if !ok {
			if klog.V(5) {
				klog.Infof("    kabanero CRD instance: collections section has no repositories section. Skipping")
			}
			continue
		}
		repositoriesArray, ok := repositoriesInterface.([]interface{})
		if !ok {
			if klog.V(5) {
				klog.Infof("    kabanero CRD instance: repositories  type is %T. Skipping", repositoriesInterface)
			}
			continue
		}
		for index, elementObj := range repositoriesArray {
			elementMap, ok := elementObj.(map[string]interface{})
			if !ok {
				if klog.V(5) {
					klog.Infof("    kabanero CRD instance repositories index %d, types is %T. Skipping", index, elementObj)
				}
				continue
			}
			activeDefaultCollectionsObj, ok := elementMap[ACTIVATEDEFAULTCOLLECTIONS]
			if !ok {
				if klog.V(5) {
					klog.Infof("    kabanero CRD instance: index %d, activeDefaultCollection not set. Skipping", index)
				}
				continue
			}
			active, ok := activeDefaultCollectionsObj.(bool)
			if !ok {
				if klog.V(5) {
					klog.Infof("    kabanero CRD instance index %d, activeDefaultCollection, types is %T. Skipping", index, activeDefaultCollectionsObj)
				}
				continue
			}
			if active {
				urlObj, ok := elementMap[URL]
				if !ok {
					if klog.V(5) {
						klog.Infof("    kabanero CRD instance: index %d, url set. Skipping", index)
					}
					continue
				}
				url, ok := urlObj.(string)
				if !ok {
					if klog.V(5) {
						klog.Infof("    kabanero CRD instance index %d, url type is %T. Skipping", index, url)
					}
					continue
				}
				return url, nil
			}
		}
	}
	return "", fmt.Errorf("Unable to find collection url in kabanero custom resource for namespace %s", namespace)
}

/* @Return true if character is valid for a domain name */
func isValidDomainNameChar(ch byte) bool {
    return (ch == '.' || ch == '-' ||
        (ch >= 'a' && ch <= 'z') ||
        (ch >= '0' && ch <= '9'))
}

/* Convert a name to domain name format.
 The name must
 - Start with [a-z0-9]. If not, "0" is prepended.
 - lower case. If not, lower case is used.
 - contain only '.', '-', and [a-z0-9]. If not, "." is used insteaad.
 - end with alpha numeric characters. Otherwise, '0' is appended
 - can't have consecutive '.'.  Consecutivie ".." is substituted with ".".
Return emtpy string if the name is empty after conversion
*/
func toDomainName(name string) string {
    maxLength := maxNameLength
    name = strings.ToLower(name)
    ret := bytes.Buffer{}
    chars := []byte(name)
    for i, ch := range chars {
        if i == 0 {
            // first character must be [a-z0-9]
            if (ch >= 'a' && ch <= 'z') ||
                (ch >= '0' && ch <= '9') {
                ret.WriteByte(ch)
            } else {
                ret.WriteByte('0')
                if isValidDomainNameChar(ch) {
                    ret.WriteByte(ch)
                } else {
                    ret.WriteByte('.')
                }
            }
        } else {
            if isValidDomainNameChar(ch) {
                ret.WriteByte(ch)
            } else {
                ret.WriteByte('.')
            }
        }
    }

    // change all ".." to ".
    retStr := ret.String()
    for strings.Index(retStr, "..") > 0 {
        retStr = strings.ReplaceAll(retStr, "..", ".")
    }

    strLen := len(retStr)
    if strLen == 0 {
        return retStr
    }
    if strLen > maxLength {
        strLen = maxLength
        retStr = retStr[0:strLen]
    }
    ch := retStr[strLen-1]
    if (ch >= 'a' && ch <= 'z') ||
        (ch >= '0' && ch <= '9') {
        // last char is alphanumeric
        return retStr
    }
    if strLen < maxLength-1 {
        //  append alphanumeric
        return retStr + "0"
    }
    // replace last char to be alphanumeric
    return retStr[0:strLen-2] + "0"
}
