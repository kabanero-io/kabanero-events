# kabanero-webhook
[![Build Status](https://travis-ci.org/kabanero-io/kabanero-webhook.svg?branch=master)](https://travis-ci.org/kabanero-io/kabanero-webhook)

## Table of Contents
* [Introduction](#Introduction)   
* [Building](#Building)   
* [Functional Specification](#Functional_Spec)   

<a name="Introduction"></a>
## Introduction 

This repository contains the webhook component of Kabanero


<a name="Building"></a>
## Building

There are two ways to build the code:
- Building in a docker container
- locally on your laptop or desktop

### Docker build

To build in a docker container:
- Clone this repository
- Install version of docker that supports multi-stage build.
- Run `./build.sh` to produces an image called `kabanero-webhook`.  This image is to be run in an openshift environment. An official build pushes the image as kabanero/kabanero-webhook and it is installed as part of Kabanero operator.

### Local build

#### Set up the build environment
To set up a local build environment:
- Install `go`
- Install `dep` tool
- Install `golint` tool
- Clone this repository into $GOPATH/src/github.com/kabanero-webhook
- Run `dep ensure --vendor-only` to generate the prerequisite vendor files.

#### Local development and unit test

##### Building Locally

Run `go test` to run unit test

Run `go build` to build the executable `kabanero-webhook`. 

If you import new prerequisites in your source code:
- run `dep ensure` to regenerate the vendor directory, and `Gopkg.lock`, `Gopkg.toml`.  
- Re-run both the unit test and buld.
- Run `golint` to ensure it's lint free.
- Push the updated `Gopkg.lock` and `Gopkg.toml` if any. 

##### Testing with an Existing Kabanero Collection

To test locally outside of of a pod with existing event triggers in a collection:
- Install and configure Kabanero foundation: `https://kabanero.io/docs/ref/general/installing-kabanero-foundation.html`. Also go through the optional section to make sure you can trigger a Tekton pipeline .
- Ensure you have kubectl configured and you are able to connect to an Openshift API Server.
- `kabanero-webhook -master <path to openshift API server> -v <n>`,  where the -v option is the client-go logging verbosity. 
- To test webhook, create a new webhook to point to your local machine's host and port. For example, `https://my-host:9080/webhook`

##### Testing with Event Triggers in a sandbox

The subdirectories under the directory `test_data/sandbox` contains sandboxes. For example, `test_data/sandbox/sample0` is a sandbox. You may create additional sandboxes that conform the same directory structure. 

To set up your sandbox: 
- Create a branch or clone of this repository.
- Make a copy of `sample0` directory into a different directory. For example, `sample1`.
- Modify or create one or more subdirectories under `sample1`, each containing Kubernetes resources to be applied when an event trigger fires.
- Create your `sample1.tar.gz` file: change directory to `sample1/triggers` and run the command `tar -cvzf ../sample1.tar.gz *`.  Push the changes.
- Edit kabanero-index.yaml and modify the url under the triggers section to point to your URL of your sample1.tar.gz. Push the changes to your branch. For example:
```
triggers:
 - description: triggers for this collection
   url: https://raw.githubusercontent.com/<owner>/kabanero-webhook/<barnch>/test_data/sandbox/sample1/sample1.tar.gz
```

To set up the kabanero-webhook to use the sandbox:
- From the browser, browse to kabanero-index.yaml file **in your branch**.
- Click on `raw` button and copy the URL in the browser. 
- Export a new environment variable: `export KABANERO_INDEX_URL=<url>`. For example, `export KABANERO_INDEX_URL=https://raw.githubusercontent.com/<owner>/kabanero-webhook/<branch>/test_data/sandbox/sample1/kabanero-index.yaml`

To run the kabanero-webhook:
- Ensure you have set up the secret that contains the personal access token. See functional specification below.
- Ensure you can run `kubectl` against your Kubernetes API server.
- Run `kabanero-webhook -master <API server URL> -v <n>`, where n is the Kubernetes log level.

To update your sandbox:
- Make changes to the files under the `sample1/triggers` subdirectory
- Re-create `sample1.tar.gz`
- Push the changes to your branch
- Restart kabanero-webhook 


#### Running in OpenShift

Running a temporary copy of Kabanero Webhook in OpenShift can be done using `oc new-app` like so:
```bash
oc new-app kabanero/webhook -e KABANERO_INDEX_URL=<url> 
```

<a name="Functional_Spec"></a>
## Functional Specifications
**Note:** The event trigger portion of this specification shall be moved to the kabanero-event repository when the implementation is separated into distinct web hook and event components.
