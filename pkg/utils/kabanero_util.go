/*
Copyright 2020 IBM Corporation

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

package utils

import (
	"fmt"
	"github.com/kabanero-io/kabanero-operator/pkg/apis/kabanero/v1alpha2"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog"
	"os"
	"strings"
)

const (
	// KUBENAMESPACE the namespace that kabanero is running in
	KUBENAMESPACE = "KUBE_NAMESPACE"
	// DEFAULTNAMESPACE the default namespace name
	DEFAULTNAMESPACE = "kabanero"
)

var (
	kabaneroNamespace string
)

// GetKabaneroNamespace Get namespace of where kabanero is installed
func GetKabaneroNamespace() string {
	if kabaneroNamespace == "" {
		kabaneroNamespace = os.Getenv(KUBENAMESPACE)
		if kabaneroNamespace == "" {
			kabaneroNamespace = DEFAULTNAMESPACE
		}
	}

	return kabaneroNamespace
}

// GetKabaneroIndexURL Get the URL to kabanero-index.yaml
func GetKabaneroIndexURL(dynInterf dynamic.Interface, namespace string) (string, error) {
	if klog.V(5) {
		klog.Infof("Entering GetKabaneroIndexURL")
		defer klog.Infof("Leaving GetKabaneroIndexURL")
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
	return "", fmt.Errorf("unable to find collection url in kabanero custom resource for namespace %s", namespace)
}

// GetKabaneroIndexURL Get the URL to kabanero-index.yaml
func GetKabaneroIndexURLNew(client rest.Interface, namespace string) (string, error) {
	kabaneroList := v1alpha2.KabaneroList{}
	err := client.Get().Resource(KABANEROS).Namespace(namespace).Do().Into(&kabaneroList)
	if err != nil {
		return "", err
	}

	for _, kabanero := range kabaneroList.Items {
		for _, triggerSpec := range kabanero.Spec.Triggers {
			klog.Infof("trigger id, sha256, url -> %s, %s, %s", triggerSpec.Id, triggerSpec.Sha256, triggerSpec.Url)
		}
	}

	return "https://localhost", nil
}

/*
GetGitHubSecret Find the user/token for a GitHub API key. The format of the secret:
apiVersion: v1
kind: Secret
metadata:
  name: gh-https-secret
  annotations:
    tekton.dev/git-0: https://github.com
type: kubernetes.io/basic-auth
stringData:
  username: <username>
  password: <token>

This will scan for a secret with either of the following annotations:
 * tekton.dev/git-*
 * kabanero.io/git-*

GetGitHubSecret will return the username and token of a secret whose annotation's value is a prefix match for repoURL.
Note that a secret with the `kabanero.io/git-*` annotation is preferred over one with `tekton.dev/git-*`.
Return: username, token, error
*/
func GetGitHubSecret(client *kubernetes.Clientset, namespace string, repoURL string) (string, string, error) {
	// TODO: Change to controller pattern and cache the secrets.
	if klog.V(8) {
		klog.Infof("GetGitHubSecret namespace: %s, repoURL: %s", namespace, repoURL)
	}

	secrets, err := client.CoreV1().Secrets(namespace).List(metav1.ListOptions{})
	if err != nil {
		return "", "", err
	}

	secret := getGitHubSecretForRepo(secrets, repoURL)
	if secret == nil {
		return "", "", fmt.Errorf("unable to find GitHub token for url: %s", repoURL)
	}

	username, ok := secret.Data["username"]
	if !ok {
		return "", "", fmt.Errorf("unable to find username field of secret: %s", secret.Name)
	}

	token, ok := secret.Data["password"]
	if !ok {
		return "", "", fmt.Errorf("unable to find password field of secret: %s", secret.Namespace)
	}

	return string(username), string(token), nil
}

func getGitHubSecretForRepo(secrets *v1.SecretList, repoURL string) *v1.Secret {
	var tknSecret *v1.Secret
	for i, secret := range secrets.Items {
		for key, val := range secret.Annotations {
			if strings.HasPrefix(key, "tekton.dev/git-") && strings.HasPrefix(repoURL, val) {
				tknSecret = &secrets.Items[i]
			} else if strings.HasPrefix(key, "kabanero.io/git-") && strings.HasPrefix(repoURL, val) {
				// Since we prefer the kabanero.io annotation, we can terminate early if we find one that matches.
				return &secrets.Items[i]
			}
		}
	}

	return tknSecret
}

/*
 Input:
	str: input string
	arrStr: input array of string
 Return:
	true if any element of arrStr is a prefix of str
	the first element of arrStr that is a prefix of str
*/
func matchPrefix(str string, arrStr []string) (bool, string) {
	for _, val := range arrStr {
		if strings.HasPrefix(str, val) {
			return true, val
		}
	}
	return false, ""
}
