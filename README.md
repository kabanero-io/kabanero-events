# kabanero-events
[![Build Status](https://travis-ci.org/kabanero-io/kabanero-events.svg?branch=master)](https://travis-ci.org/kabanero-io/kabanero-events)

## Table of Contents
* [Introduction](#Introduction)   
* [Building](#Building)   
* [Functional Specification](#Functional_Spec)   
* [Sample Event Trigger](#Sample_Trigger)   

<a name="Introduction"></a>
## Introduction 

This repository contains the experimental webhook and events components of Kabanero.


<a name="Building"></a>
## Building

There are two ways to build the code:
- Building in a docker container
- locally on your laptop or desktop

### Docker build

To build in a docker container:
- Clone this repository
- Install version of docker that supports multi-stage build.
- Run `./build.sh` to produces an image called `kabanero-events`.  This image is to be run in an openshift environment. An official build pushes the image as kabanero/kabanero-events and it is installed as part of Kabanero operator.

### Local build

#### Set up the build environment
To set up a local build environment:
- Install `go`
- Install `dep` tool
- Install `golint` tool
- Clone this repository into $GOPATH/src/github.com/kabanero-events
- Run `dep ensure --vendor-only` to generate the prerequisite vendor files.

#### Local development and unit test

##### Building Locally

Run `go test` to run unit test

Run `go build` to build the executable `kabanero-events`.

If you import new prerequisites in your source code:
- run `dep ensure` to regenerate the vendor directory, and `Gopkg.lock`, `Gopkg.toml`.  
- Re-run both the unit test and buld.
- Run `golint` to ensure it's lint free.
- Push the updated `Gopkg.lock` and `Gopkg.toml` if any. 

##### Testing with an Existing Kabanero Collection

To test locally outside of of a pod with existing event triggers in a collection:
- Install and configure Kabanero foundation: `https://kabanero.io/docs/ref/general/installing-kabanero-foundation.html`. Also go through the optional section to make sure you can trigger a Tekton pipeline .
- Ensure you have kubectl configured and you are able to connect to an Openshift API Server.
- `kabanero-events -master <path to openshift API server> -v <n>`,  where the -v option is the client-go logging verbosity.
- To test webhook, create a new webhook to point to your local machine's host and port. For example, `https://my-host:9080/webhook`

##### Testing with Event Triggers in a sandbox

The subdirectories under the directory `test_data/sandbox` contains sandboxes. For example, `test_data/sandbox/sample1` is a sandbox. 

To set up your sandbox: 
- Create your own sandbox repository.
-  Copy one of the sample sandboxes into your repository, say `sample1`.
- Modify or create one or more subdirectories under `sample1`, each containing Kubernetes resources to be applied when an event trigger fires.
- Create your `sample1.tar.gz` file: change directory to `sample1/triggers` and run the command `tar -cvzf ../sample1.tar.gz *`.  Push the changes.
- Edit kabanero-index.yaml and modify the url under the triggers section to point to your URL of your sample1.tar.gz. Push the changes to your reposiotyr. For example:
```
triggers:
 - description: triggers for this collection
   url: https://raw.githubusercontent.com/<owner>/kabanero-events/<barnch>/master/sample1/sample1.tar.gz
```

To set up the kabanero webhook to use the sandbox:
- From the browser, browse to kabanero-index.yaml file.
- Click on `raw` button and copy the URL in the browser. 
- Export a new environment variable: `export KABANERO_INDEX_URL=<url>`. For example, `export KABANERO_INDEX_URL=https://raw.githubusercontent.com/<owner>/<repo>/master/sample1/kabanero-index.yaml`

To run the kabanero-events in a sandbox:
- Ensure the non-sandbox version is working.
- Ensure you can run `kubectl` against your Kubernetes API server.
- Run `kabanero-events -disableTLS -master <API server URL> -v <n>`, where n is the Kubernetes log level.
- create a new webhook that points to the URL of your sandbox build.

To update your sandbox event triggers:
- Make changes to the files under the  sandbox `triggers` subdirectory
- Re-create `sample1.tar.gz`
- Push the changes
- Restart kabanero-events


#### Running in OpenShift

Running a temporary copy of Kabanero Webhook in OpenShift can be done using `oc new-app` like so:
```bash
oc new-app kabanero/webhook -disableTLS -e KABANERO_INDEX_URL=<url> 
```



<a name="Functional_Spec"></a>
## Functional Specifications

**NOTE: The webhook and events components are experimental, and the specification may be updated with incompatible changes in the next Kabanero releases.**

**Note: The event trigger portion of this specification shall be moved to the kabanero-event repository when the implementation is separated into distinct web hook and event components.**

### Definitions

A Kabanero installations is an installation of a specific version of the Kabanero operator via the operator installer.

A Kabanero instance is an instance of the Kabanero runtime created by applying custom resource of kind `kabanero` to a Kabanero installation.

The webhook component of Kabanero receives webhook POST events through its listener. The webhook listener is created when a Kabanero instance is created. Its primary purpose is to publish webhook events.

The events component of Kabanero is designed to mediate and choreograph the operations of other components within Kabanero. It may be used to filter events, transform events, and initiate additional actions based on events.


### Webhook Component

#### Wbhook Configuration

A webhook listener is created by default whenever a new Kabanero instance is created. Below is an example of of the kabanero CRD after it was created:
 
```
apiVersion: kabanero.io/v1alpha1
kind: Kabanero
metadata:
  name: kabanero
  namespace: kabanero
spec:
  appsodyOperator: {}
  che:
    cheOperator: {}
    cheOperatorInstance: {}
    enable: false
    kabaneroChe: {}
  cliServices: {}
  collections:
    repositories:
    - activateDefaultCollections: true
      name: incubator
      url: https://github.com/kabanero-io/collections/releases/download/0.3.0/kabanero-index.yaml
  github: {}
  landing: {}
  tekton: {}
  version: 0.3.0
  webhook:
    enable: true
status:
  appsody:
    ready: "True"
  cli:
    hostnames:
    - kabanero-cli-kabanero.sires1.fyre.ibm.com
    ready: "True"
  kabaneroInstance:
    ready: "True"
    version: 0.3.0
  knativeEventing:
    ready: "True"
    version: 0.6.0
  knativeServing:
    ready: "True"
    version: 0.6.0
  landing:
    ready: "True"
    version: 0.2.0
  tekton:
    ready: "True"
    version: v0.5.2
  webhook:
    hostnames:
    - kabanero-events-kabanero.myhost.com
    ready: "True"
```

Note that:
- The webhook listener may be disabled via the CRD.
- If enabled, the status shows the host(s) where the listener is available. 

You can get more information about the route via `oc get route kabanero-events -o yaml`.

The webhook component expects to receive POST requests in JSON format, and creates a new event that contains:
```
"webhook": { <JSON body that was received>},
"header": { <header that was received>}
```

#### Github Webhook

Github webhooks may be configured at either a per-repository level, or at an organization level. To configure at a per-repository level, follow these instructions: https://developer.github.com/webhooks/creating/#setting-up-a-webhook

To configure an organizational webhook, follow these instructions: https://help.github.com/en/github/setting-up-and-managing-your-enterprise-account/configuring-webhooks-for-organization-events-in-your-enterprise-account

Note these configurations:
- Use the hostname of the exported route for kabanero-events as the hostname for the URL of the webhook. For example, `https://kabaner-webhook-kabanero.myhost.com`
- Use "aplication/JSON" as the content type.
- The Kabanero webhook listener does not currently verify the secret you configure in Github.
- The default configuration uses Openshift auto-generated service serving self-signed certificate. Unless you had replaced the route with a certificate signed by a public certificate authority, when configuring webhook you need to choose the `disable SSL` option to skip certificate verification.

### The Events Component

The Events component provides message mediation, transformation, and actions based on incoming events.  It uses `Common Express Language` (https://opensource.google/projects/cel) as the underlying framework to
- Filter events
- Initial actions based on events
- Define new events as JSON data structure.


#### Configuring Events Framework

The events component is configured via additional files in the Kabanero collection. The file `kabanero-index.yaml` in the collection contains a pointer to the location of those files. For example,

```
stacks:
...
triggers:
 - description: triggers for this collection
   url: https://raw.githubusercontent.com/kabanero-io/release/<release>/triggers.tar.gz
```


The `triggers.tar.gz` file must include a file named `eventTriggers.yaml that contains the definition of the event triggers.

#### Event Trigger Definitions

The file `eventTriggers.yaml` contains three sections:
- settings: for events framework settings.
- variables: for declaring variables and their values
- eventTriggers: for declaring what actions to take based on events.

##### Settings Section

The setting section supports the following options:
- dryrun: if true, will not execute actions.

For example:
```
settings:
  dryrun: false
```

##### Variables Section

When processing an event, the input event is placed in a default variable named `message`.  Additional variables may be defined via the `variables` section. The name of the variable must conform to the CEL identifier rule.  The value of the variable is a CEL expression.  For example,

```
variables:
  - name: build.pr.allowedBranches
    value: ' [ "master" ] '
  - name: build.push.allowedBranches
    value: ' [ "master" ] '
  - name: build.tag.pattern
    value: '"\\d\\.\\d\\.\\d"'
```

The above example shows that:
- Variables may be nested, and the result is a JSON data structure. For example, `build.pr.allowedBranches` creates a data structure where the name of the variable is `build`, and the content is a JSON data structure:
```
{
  "pr": {
      "allowedBranches": [ "master"]
  }
}
```
- The value of the variable is a CEL expression.  
- Escape characters are required for a string when parsing via CEL. For example, "\\d\\.\\d\\.\\d", after parsing, becomes "\d.\d.\d", a RegEx2 pattern meaning digits separated by ".".

Variables may be conditionally defined via the `when` clause. The value of the `when` clause is a CEL expression that must evaluate to a boolean. The following example shows the variable `build` with component JSON attribute `repositoryEvent` being defined only if the input message is a webhook message:
```
- when: ' has(message.webhook)' # webhook event
  variables:
    - name: build.repositoryEvent
      value: ' message.webhook.header["X-Github-Event"][0] ' # push, pull_request, tag, etc
```

Variable declarations may be nested. For example:
```
variables:
   - name : <name>
     value: <value>
   - when: <condition>
     variables:
       - name: <name>
         value: <value>
       - when: <condition>
         variables:
           - name: <name>
             value: <value>
```

##### Event Triggers Section

The event triggers section defines what actions to apply based on conditions. The basic format looks like:
```
eventTriggers:
 - when: <condition>
   action:
      <action name>:
        <action parameters>
```

Kabanero events compoent current supports applyResources action:

```
eventTriggers:
  - when: <condition>
    action:
      applyResources:
        directory: <dir>
```

When the `condition` is true, the `applyResources` action goes through all `.yaml` files in the specified directory in alphabetical order, applies go template substitution to all the variables in the files, then applies the resources into the Openshift environment where the events infrastructure is running. The variables in the `.yaml` files must already be defined through event triggers, or the value of the template substitution is `<no value>`.

<a name="Sample_Trigger"></a>
## Sample Event Trigger

The incubator collection shipped with Kabanero 0.3 contains a sample event trigger that demonstrates:
- Organizational webhook
- Triggering of 3 different pipelines based on github events:
  - A build only pipeline for a pull request to master branch
  - A build and push pipeline when pushing to master. 
  - A retag pipeline for tagging an image based on a tag to Github.

### Configurations

#### Tekton Configuration

Configure Github **basic authentication** for Tekton builds by following the instructions here: https://github.com/tektoncd/pipeline/blob/master/docs/auth.md.

Note: Basic authentication is the minimum required configuration to authenticate with Github. It is used to retrieve individual configuration files from Github to determine the type of repository being built, without first cloning the repository. It can also be used to clone the repository.  

In addition to basic authentication, you may also optionally configure SSH to clone the repository prior to builds. The first step is to use the SSH URL for a GitHub repository in your PipelineResource for your git source. To use ssh URL, update the `build.repoURL` in `eventTriggers.yaml` to use `message.webhook.body.repository.clone_url`.

Another necessary step is to base64 encode the SSH private key associated with your GitHub account and add it to a
secret. An example Secret has been provided below.

#### ssh-key-secret.yaml
```yaml
 apiVersion: v1
 kind: Secret
 metadata:
   name: ssh-key
   annotations:
     tekton.dev/git-0: github.ibm.com # URL to GH or your GHE
 type: kubernetes.io/ssh-auth
 data:
   ssh-privatekey: <base64 encoded private key> # This can be generated using `cat ~/.ssh/id_rsa | base64`

   # This is non-standard, but its use is encouraged to make this more secure.
   #known_hosts: <base64 encoded>
```

After applying the Secret with `kubectl apply -f ssh-key-secret.yaml`, associate it with the ServiceAccount you created to run the Appsody Tekton builds. For example, this can be done with `kabanero-operator` service account, which is used by the samples by running
```bash
$ kubectl edit sa kabanero-operator yaml
```

and then appending `ssh-key` to the `secrets` section like so:
```
 apiVersion: v1
 imagePullSecrets:
 - name: kabanero-operator-dockercfg-4vzbk
 kind: ServiceAccount
 metadata:
   creationTimestamp: "2019-10-22T14:05:09Z"
   name: kabanero-operator
   namespace: kabanero
 secrets:
 - name: kabanero-operator-token-r7kdg
 - name: kabanero-operator-dockercfg-4vzbk
 - name: ssh-key
```

#### Github configuration

Create an organization on Github.

Configure an organizational webhook following the instruction here: https://help.github.com/en/github/setting-up-and-managing-your-enterprise-account/configuring-webhooks-for-organization-events-in-your-enterprise-account.
- For `payload URl`, enter the route to your webhook, such as `kabanero-events-kabaner.<host>.com`. The actual URL is installation dependent.
- For `Content type` select `application/json`
- For `Secret`, leave blank for now, as the current implementation does not yet support checking secrets.
- For the list of events, select `send me everything`.

### Running the Sample

#### Initial Push to master

- Create a new empty repository for the org, say `project1`.  Note the url for the new repository.
- On the developer machine, initialize the appsody project:
```
mkdir  project1
cd project1
appsody init nodejs-express
git init
git remote add origin <url>
git remote add origin git@<host>:<owner>/project1.git
git add * .appsody-config.yaml .gitignore
git commit -m "initial drop"
git push -u origin master
```
- Check your Tekton dashboard for a new build that extracts from master, and pushes the result to the internal openshift registry as: kabanero/project1:sha.

#### Build from a pull request

- Create a branch on the repository
- Push some code to the branch
- Create a pull request
- Check your Tekton dashboard for a new build that just performs a test build.

#### Creating a new version via Github Tagging

- Create a new namespace project1-test
- Switch back go the master repository
- Use `git log` to locate the SHA of the commits. Pick a commit to be tag.
- Tag the commit as version 0.0.1:
```
git tag 0.0.1 SHA
git push orig 0.0.1
```
- Check the Tekton dashboard for a new build that tags the original docker image at kabanero:project1:SHA into a new image project1-test:project1:0.0.1 and pushed into the new namespace.
