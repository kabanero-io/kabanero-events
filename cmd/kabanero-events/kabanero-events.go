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
	"flag"
	"fmt"
	"github.com/kabanero-io/kabanero-events/pkg/endpoints"
	"github.com/kabanero-io/kabanero-events/pkg/messages"
	"github.com/kabanero-io/kabanero-events/pkg/trigger"
	"github.com/kabanero-io/kabanero-events/pkg/utils"
	"io/ioutil"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
)

/* useful constants */
const (
	DEFAULTNAMESPACE   = "kabanero"
	KABANEROINDEXURL   = "KABANERO_INDEX_URL" // use the given URL to fetch kabaneroindex.yaml
	WEBHOOKDESTINATION = "github"             // name of the destination to send github webhook events
	tlsCertPath        = "/etc/tls/tls.crt"
	tlsKeyPath         = "/etc/tls/tls.key"
)

var (
	masterURL        string // URL of Kube master
	kubeConfig       string // path to kube config file. default <home>/.kube/config
	triggerProc      *trigger.Processor
	eventProviders   *messages.EventDefinition
	providerCfg      string // Path of provider config to use
	disableTLS       bool   // Option to disable TLS listener
	skipChkSumVerify bool   // Option to skip verification of SHA256 checksum of trigger collection
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

	klog.Infof("disableTLS: %v", disableTLS)
	klog.Infof("skipChecksumVerify: %v", skipChkSumVerify)

	cfg, err := utils.NewKubeConfig(masterURL, kubeConfig)
	if err != nil {
		klog.Fatal(err)
	}

	kubeClient, err := utils.NewKubeClient(cfg)
	if err != nil {
		klog.Fatal(err)
	}

	dynamicClient, err := utils.NewDynamicClient(cfg)
	if err != nil {
		klog.Fatal(err)
	}

	klog.Infof("Received kubeClient %T, dynamicClient  %T\n", kubeClient, dynamicClient)

	/* Get namespace of where kabanero is installed */
	webhookNamespace := utils.GetKabaneroNamespace()

	kabaneroIndexURL := os.Getenv(KABANEROINDEXURL)
	if kabaneroIndexURL == "" {
		// not overridden, use the one in the kabanero CRD
		kabaneroIndexURL, err = utils.GetKabaneroIndexURL(webhookNamespace)
		if err != nil {
			klog.Fatal(fmt.Errorf("unable to get kabanero index URL from kabanero CRD. Error: %s", err))
		}
	} else {
		klog.Infof("Using value of KABANERO_INDEX_URL environment variable to fetch kabanero index from: %s", kabaneroIndexURL)
	}

	/* Download the trigger into temp directory */
	dir, err := ioutil.TempDir("", "webhook")
	if err != nil {
		klog.Fatal(fmt.Errorf("unable to create temproary directory. Error: %s", err))
	}
	defer os.RemoveAll(dir)

	err = utils.DownloadTrigger(kabaneroIndexURL, dir, !skipChkSumVerify)
	if err != nil {
		klog.Fatal(fmt.Errorf("unable to download trigger pointed by kabanero_index_url at: %s, error: %s", kabaneroIndexURL, err))
	}

	triggerProc = trigger.NewProcessor()
	err = triggerProc.Initialize(dir)
	if err != nil {
		klog.Fatal(fmt.Errorf("unable to initialize trigger definition: %s", err))
	}

	if providerCfg == "" {
		providerCfg = filepath.Join(dir, "eventDefinitions.yaml")
	}

	if _, err := os.Stat(providerCfg); os.IsNotExist(err) {
		// Tolerate this for now.
		klog.Errorf("eventDefinitions.yaml was not found: %s", providerCfg)
	}

	eventProviders, err = messages.InitializeEventProviders(providerCfg)
	if err != nil {
		klog.Fatal(fmt.Errorf("unable to initialize event providers: %s", err))
	}

	/* Start listeners to listen on events */
	err = triggerProc.StartListeners(eventProviders)
	if err != nil {
		klog.Fatal(fmt.Errorf("unable to start listeners for event triggers: %s", err))
	}

	// Listen for events
	if disableTLS {
		err = endpoints.NewListener()
	} else {
		err = endpoints.NewListenerTLS(tlsCertPath, tlsKeyPath)
	}

	if err != nil {
		klog.Fatal(err)
	}

	select {}
}

func init() {
	if home := homedir.HomeDir(); home != "" {
		flag.StringVar(&kubeConfig, "kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		flag.StringVar(&kubeConfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&providerCfg, "providerCfg", "", "path to the provider config")
	flag.BoolVar(&disableTLS, "disableTLS", false, "set to use non-TLS listener")
	flag.BoolVar(&skipChkSumVerify, "skipChecksumVerify", false, "set to skip the verification of trigger collection checksum")

	// init flags for klog
	klog.InitFlags(nil)

}
