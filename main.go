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
	"flag"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery" 
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog"
	ghw "gopkg.in/go-playground/webhooks.v3/github"
)

const (
	kubeAPIURL = "http://localhost:9080"
	DEFAULT_NAMESPACE = "kabanero"
    KUBE_NAMESPACE = "KUBE_NAMESPACE"
)


var (
	masterURL  string        // URL of Kube master
	kubeconfig string        // path to kube config file. default <home>/.kube/config
	klogFlags  *flag.FlagSet // flagset for logging
	gitHubListener *GitHubListener // Listens for and handles GH events
	kubeClient *kubernetes.Clientset
	discClient  *discovery.DiscoveryClient
	dynamicClient dynamic.Interface
	webhook_namespace string
)

func init() {
	// Print stacks and exit on SIGINT
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT)
		buf := make([]byte, 1<<20)
		<-sigChan
		stacklen := runtime.Stack(buf, true)
		klog.Infof("=== received SIGQUIT ===\n*** goroutine dump...\n%s\n*** end\n", buf[:stacklen])
		os.Exit(1)
	}()
}

func main() {

	flag.Parse()

	var cfg *rest.Config
	var err error
	if strings.Compare(masterURL, "") != 0 {
		// running outside of Kube cluster
		klog.Infof("starting Kabanero webhook outside cluster\n")
		klog.Infof("masterURL: %s\n", masterURL)
		klog.Infof("kubeconfig: %s\n", kubeconfig)
		cfg, err = clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
		if err != nil {
			klog.Fatal(err)
		}
	} else {
		// running inside the Kube cluster
		klog.Infof("starting Kabanero webhook status controller inside cluster\n")
		cfg, err = rest.InClusterConfig()
		if err != nil {
			klog.Fatal(err)
		}
	}

	kubeClient, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatal(err)
	}

	discClient = kubeClient.DiscoveryClient
	dynamicClient, err = dynamic.NewForConfig(cfg)
	if err != nil {
		klog.Fatal(err)
	}
	klog.Infof("Received discClient %T, dynamicClient  %T\n", discClient, dynamicClient)

	/* Get namespace of where we are installed */
	webhook_namespace = os.Getenv(KUBE_NAMESPACE)
    if webhook_namespace == "" {
            webhook_namespace = DEFAULT_NAMESPACE
	}

	// gvr := schema.GroupVersionResource { Group: "app.k8s.io", Version: "v1beta1", Resource: "applications" }
	// deleteOrphanedAutoCreatedApplications(dynamicClient, gvr )

	// plugin := &ControllerPlugin{dynamicClient, discClient, DefaultBatchDuration, calculateComponentStatus}
	// resController, err := NewClusterWatcher(plugin)
	// _, err = NewClusterWatcher(plugin)
	// if err != nil {
	//	klog.Fatal(err)
	//}

	// Handle GitHub events
	if gitHubListener, err = NewGitHubEventListener(); err != nil {
		klog.Fatal(err)
	}

	http.HandleFunc("/", func (w http.ResponseWriter, r *http.Request) {
		payload, err := gitHubListener.ParseEvent(r)

		if err == nil {
			switch payload.(type) {
			case ghw.PushPayload:
				klog.Infof("Received Push event:\n%v\n", payload)
			}
		} else {
			klog.Error(err)
		}
	})

	klog.Fatal(http.ListenAndServe(":8080", nil))

	select {}
}

func printEvent(event watch.Event) {
	klog.Infof("event type %s, object type is %T\n", event.Type, event.Object)
	printEventObject(event.Object, "    ")
}

func printEventObject(obj interface{}, indent string) {
	switch obj.(type) {
	case *unstructured.Unstructured:
		var unstructuredObj = obj.(*unstructured.Unstructured)
		// printObject(unstructuredObj.Object, indent)
		printUnstructuredJSON(unstructuredObj.Object, indent)
		klog.Infof("\n")
	default:
		klog.Infof("%snot Unstructured: type: %T val: %s\n", indent, obj, obj)
	}
}

func printUnstructuredJSON(obj interface{}, indent string) {
	data, err := json.MarshalIndent(obj, "", indent)
	if err != nil {
		klog.Fatalf("JSON Marshaling failed %s", err)
	}
	klog.Infof("%s\n", data)
}

func printObject(obj interface{}, indent string) {
	nextIndent := indent + "    "
	switch obj.(type) {
	case int:
		klog.Infof("%d", obj.(int))
	case bool:
		klog.Infof("%t", obj.(bool))
	case float64:
		klog.Infof("%f", obj.(float64))
	case string:
		klog.Infof("%s", obj.(string))
	case []interface{}:
		var arr = obj.([]interface{})
		for index, elem := range arr {
			klog.Infof("\n%sindex:%d, type %T, ", indent, index, elem)
			printObject(elem, nextIndent)
		}
	case map[string]interface{}:
		var objMap = obj.(map[string]interface{})
		for label, val := range objMap {
			klog.Infof("\n%skey: %s type: %T| ", indent, label, val)
			printObject(val, nextIndent)
		}
	default:
		klog.Infof("\n%stype: %T val: %s", indent, obj, obj)
	}
}

func printPods(pods *v1.PodList) {
	for _, pod := range pods.Items {
		klog.Infof("%s", pod.ObjectMeta.Name)
	}
}

func init() {
	// flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	if home := homedir.HomeDir(); home != "" {
		flag.StringVar(&kubeconfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")

	// init falgs for klog
	klog.InitFlags(nil)

}
