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

func init() {
	// Print stacks and exit on SIGINT
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT)
		buf := make([]byte, 1<<20)
		<-sigChan
		stackLen := runtime.Stack(buf, true)
		klog.Infof("=== received SIGQUIT ===\n*** goroutine dump...\n%s\n*** end\n", buf[:stackLen])
		os.Exit(1)
	}()
}

func main() {
	// Flags
	var masterURL string
	var providerCfg string
	var kubeConfig string
	var disableTLS bool
	var skipChkSumVerify bool

	flag.StringVar(&masterURL, "master", "", "overrides the address of the Kubernetes API server in the kubeconfig file (only required if out-of-cluster)")
	flag.StringVar(&providerCfg, "providerCfg", "", "path to the provider config")
	flag.BoolVar(&disableTLS, "disableTLS", false, "set to use non-TLS listener and listen on port 9080")
	flag.BoolVar(&skipChkSumVerify, "skipChecksumVerify", false, "set to skip the verification of the trigger collection checksum")

	var kubeConfigPath string
	if home := homedir.HomeDir(); home != "" {
		kubeConfigPath = filepath.Join(home, ".kube", "config")
	} else {
		kubeConfigPath = ""
	}
	flag.StringVar(&kubeConfig, "kubeconfig", kubeConfigPath, "absolute path to the kubeconfig file (optional)")

	// init flags for klog
	klog.InitFlags(nil)

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
		kabaneroIndexURL, err = utils.GetKabaneroIndexURL(dynamicClient, webhookNamespace)
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

	if providerCfg == "" {
		providerCfg = filepath.Join(dir, "eventDefinitions.yaml")
	}

	if _, err := os.Stat(providerCfg); os.IsNotExist(err) {
		// Tolerate this for now.
		klog.Errorf("eventDefinitions.yaml was not found: %s", providerCfg)
	}

	messageService, err := messages.NewService(providerCfg)
	if err != nil {
		klog.Fatal(fmt.Errorf("unable to initialize message service: %s", err))
	}

	// Create a new environment that holds our clients etc.
	env := &endpoints.Environment{
		MessageService: messageService,
		KubeClient:     kubeClient,
		DynamicClient:  dynamicClient,
	}

	triggerProc := trigger.NewProcessor(env)
	err = triggerProc.Initialize(dir)
	if err != nil {
		klog.Fatal(fmt.Errorf("unable to initialize trigger definition: %s", err))
	}

	/* Start listeners to listen on events */
	err = triggerProc.StartListeners()
	if err != nil {
		klog.Fatal(fmt.Errorf("unable to start listeners for event triggers: %s", err))
	}

	// Listen for events
	if disableTLS {
		err = endpoints.NewListener(messageService)
	} else {
		err = endpoints.NewListenerTLS(messageService, tlsCertPath, tlsKeyPath)
	}

	if err != nil {
		klog.Fatal(err)
	}

	select {}
}
