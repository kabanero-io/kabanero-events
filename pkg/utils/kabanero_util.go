package utils

import (
	"encoding/base64"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog"
	"os"
	"strings"
)

const (
	KUBENAMESPACE    = "KUBE_NAMESPACE"
	DEFAULTNAMESPACE = "kabanero"
)

var (
	kabaneroNamespace string
)

/* Get namespace of where kabanero is installed */
func GetKabaneroNamespace() string {
	kabaneroNamespace = os.Getenv(KUBENAMESPACE)
	if kabaneroNamespace == "" {
		kabaneroNamespace = DEFAULTNAMESPACE
	}

	return kabaneroNamespace
}

/* Get the URL to kabanero-index.yaml */
func GetKabaneroIndexURL(namespace string) (string, error) {
	if klog.V(5) {
		klog.Infof("Entering getKabaneroIndexURL")
		defer klog.Infof("Leaving getKabaneroIndexURL")
	}

	dynInterf := GetDynamicClient()
	if dynInterf == nil {
		return "", fmt.Errorf("unable to get dynamic client")
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

/*
 Find the user/token for a GitHub API key
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
func GetURLAPIToken(namespace string, repoURL string) (string, string, string, error) {
	if klog.V(5) {
		klog.Infof("GetURLAPIToken namespace: %s, repoURL: %s", namespace, repoURL)
	}

	dynInterf := GetDynamicClient()
	if dynInterf == nil {
		return "", "", "", fmt.Errorf("unable to get dynamic client")
	}

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

		annotationsObj, ok := metadata[ANNOTATIONS]
		if !ok {
			continue
		}

		annotations, ok := annotationsObj.(map[string]interface{})
		if !ok {
			continue
		}

		tektonList := make([]string, 0)
		kabaneroList := make([]string, 0)
		for key, val := range annotations {
			if strings.HasPrefix(key, "kabanero.io/git-") {
				url, ok := val.(string)
				if ok {
					kabaneroList = append(kabaneroList, url)
				}
			} else if strings.HasPrefix(key, "tekton.dev/git-") {
				url, ok := val.(string)
				if ok {
					tektonList = append(tektonList, url)
				}
			}
		}

		/* find that annotation that is a match */
		urlMatched, matchedURL := matchPrefix(repoURL, kabaneroList)
		if !urlMatched {
			urlMatched, matchedURL = matchPrefix(repoURL, tektonList)
		}
		if !urlMatched {
			/* no match */
			continue
		}
		if klog.V(5) {
			klog.Infof("getURLAPIToken found match %v", matchedURL)
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

		usernameObj, ok := dataMap[USERNAME]
		if !ok {
			continue
		}
		username, ok := usernameObj.(string)
		if !ok {
			continue
		}

		tokenObj, ok := dataMap[PASSWORD]
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
		return string(decodedUserName), string(decodedToken), name, nil
	}
	return "", "", "", fmt.Errorf("unable to find API token for url: %s", repoURL)
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
