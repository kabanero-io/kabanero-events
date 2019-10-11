# kabanero-webhook

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
- Clone this repository into $GOPATH/src/github.com/kabanero-webhook
- Run `dep ensure --vendor-only` to generate the prerequisite vendor files.

#### Local development and unit test

Run `go test` to run unit test

Run `go build` to build the executable `kabanero-webhook`. To test outside of of a pod:
- Ensure you have kubectl configured and it is running correctly.
- `kabanero-webhook -master <path to openshift API server> -v <n>`,  where the -v option is the client-go logging verbosity. 

If you import new prerequisites in your source code:
- run `dep ensure` to regenerate the vendor directory, and `Gopkg.lock`, `Gopkg.toml`.  
- Re-run both the unit test and buld.
- Push the updated `Gopkg.lock` and `Gopkg.toml`. 


<a name="Functional_Spec"></a>
## Functional Specifications

**TBD**