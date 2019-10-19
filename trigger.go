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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
	"gopkg.in/yaml.v2"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types/ref"
	"k8s.io/apimachinery/pkg/runtime/schema"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog"
)

/* constants for parsing */
const (
	METADATA   = "metadata"
	APIVERSION = "apiVersion"
	KIND       = "kind"
	NAME       = "name"
	NAMESPACE  = "namespace"
	EVENT      = "event"
        JOBID      = "jobid"
	TYPEINT    = "int"
	TYPEDOUBLE = "double"
	TYPEBOOL   = "bool"
	TYPESTRING = "string"
	TYPELIST   = "list"
	TYPEMAP    = "map"
)

/*
Variable as used in trigger
*/
type Variable struct {
	When  string `yaml:"when"`
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
	Variables []*Variable `yaml:"variables,omitempty"`
}

/*
ApplyResource as used in the trigger file
*/
type ApplyResource struct {
	Directory string `yaml:"directory"`
}

/*
Action as used in the trigger file
*/
type Action struct {
	ApplyResources *ApplyResource `yaml:"applyResources"`
}

/*
Trigger as used in the trigger file
*/
type Trigger struct {
	When     string     `yaml:"when"`
	Action   *Action    `yaml:"action,omitempty"`
	Triggers []*Trigger `yaml:"triggers,omitempty"`
}

/*
TriggerDefinition as used in the trigger file
*/
type TriggerDefinition struct {
	Variables []*Variable `yaml:"variables,omitempty"`
	Triggers  []*Trigger  `yaml:"triggers,omitempty"`
}

type triggerProcessor struct {
	triggerDef *TriggerDefinition
	triggerDir string // directory where trigger file is stored
}

func newTriggerProcessor() *triggerProcessor {
	return &triggerProcessor{}
}

func (tp *triggerProcessor) initialize(fileName string) error {
	var err error
	tp.triggerDef, err = readTriggerDefinition(fileName)
	if err != nil {
		return err
	}
	tp.triggerDir = filepath.Dir(fileName)
	return nil
}

func (tp *triggerProcessor) processMessage(message map[string]interface{}, initialVariables map[string]interface{}) error {
	if klog.V(5) {
		klog.Infof("Entering triggerProcessor.processMessage. message: %v, initialVariables %v", message, initialVariables);
		defer klog.Infof("Leaving triggerProcessor.processMessage")
	}
	env, variables, err := initializeCELEnv(tp.triggerDef, message, initialVariables)
	if err != nil {
		return err
	}

	/* Go through the trigger section to find the rule to fire*/
	action, err := findTrigger(env, tp.triggerDef, variables)
	if err != nil {
		return err
	}

	/* Fire the rule */
	if action == nil {
		/* no action found */
		klog.Infof("No action found for message %s", message)
		return nil
	}
	directory := action.ApplyResources.Directory
	if directory == "" {
		klog.Errorf("action drectory in trigger file is empty")
		return fmt.Errorf("action directory in trigger file is empty")
	}
	resourceDir := filepath.Join(tp.triggerDir, directory)

	err = filepath.Walk(resourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// problem with accessing a path
			klog.Errorf("problem processing trigger %s, error %s", path, err)
			return err
		}
		if info.IsDir() {
			// don't process directory
			klog.Infof("Skipping processing rerectory %s", path)
			return nil
		}
		if strings.HasSuffix(info.Name(), ".yaml") || strings.HasSuffix(info.Name(), ".yml") {
			substituted, err := substituteTemplateFile(path, variables)
			if err != nil {
				return err
			}
			/* Apply the file */
			klog.Infof("applying resource: %s", substituted)
			/*
			err = createResource(substituted, dynamicClient)
			if err != nil {
				return err
			}
			*/
		} else {
			klog.Infof("Skipping processing file %s", path)
		}
		return nil
	})

	return nil
}

/* Shallow copy a map */
func shallowCopy(originalMap map[string]interface{}) map[string]interface{} {
	newMap := make(map[string]interface{})
	for key, val := range originalMap {
		newMap[key] = val
	}
	return newMap
}

/* Get initial CEL environment by processing the initialize section */
func initializeCELEnv(td *TriggerDefinition, message map[string]interface{}, initialVariables map[string]interface{}) (cel.Env, map[string]interface{}, error) {
	if klog.V(5) {
		klog.Infof("entering initializeCELEnv")
		defer klog.Infof("Leaving initializeCELEnv")
	}
	variables := make(map[string]interface{})

	/* initilize empty CEL environment */
	env, err := cel.NewEnv()
	if err != nil {
		return nil, nil, err
	}

	/* set initial variables */
	if initialVariables != nil {
		for key, val := range initialVariables {
			if klog.V(5) {
				klog.Infof("Before creating variable %s with value %v", key, val)
			}
			switch val.(type) {
			case string:
				newIdent := decls.NewIdent(key, decls.String, nil)
				env, err = env.Extend(cel.Declarations(newIdent))
				if err != nil {
					return nil, nil, err
				}
				variables[key] = val
			case [] interface{}:	
				newIdent := decls.NewIdent(key, decls.NewListType(decls.Any), nil)
				env, err = env.Extend(cel.Declarations(newIdent))
				if err != nil {
					return nil, nil, err
				}
				variables[key] = val			
			default:
				return nil, nil, fmt.Errorf("In initializeCEL variable %s with value %v has unsupported type %T", key, val, val)
			}
		}
	}

	/* Create a new environment with event and jobid as a built-in variable*/
	if klog.V(5) {
		klog.Infof("Setting variable jobid")
	}
	jobIDIdent := decls.NewIdent(JOBID, decls.String, nil)
	env, err = env.Extend(cel.Declarations(jobIDIdent))
	if err != nil {
		return nil, nil, err
	}
	variables[JOBID] = getTimestamp()

	eventIdent := decls.NewIdent(EVENT, decls.NewMapType(decls.String, decls.Any), nil)
	env, err = env.Extend(cel.Declarations(eventIdent))
	if err != nil {
		return nil, nil, err
	}
	/* Add event as a new variable */
	variables[EVENT] = message

	if td.Variables == nil {
		/* no initialize section */
		return env, variables, nil
	}
	for _, triggerVar := range td.Variables {
		env, err = evaluateVariable(env, triggerVar, variables)
		if err != nil {
			return nil, nil, err
		}
	}
	return env, variables, nil
}

/* Evaluate one variable and update variables map with any new variables 
	env: the CEL environment
	triggerVar: the variable declaration in the trigger.yaml
	variables: variables mappings collection so far
	Return:
		env: new environment after variables are updated. Even if there are errors, env can be set to contain all valid variables collected up to the point of error
*/
func evaluateVariable(env cel.Env, triggerVar *Variable,  variables map[string]interface{} ) (cel.Env, error) {
	/* evaluate value of each variable */
	if triggerVar == nil {
		return env, nil
	}
	boolVal, err := evalCondition(env, triggerVar.When, variables) 
	if err != nil {
		return env, err
	}
	/* Condition did not meet */
	if !boolVal {
		return env, nil
	}
	env, err = setOneVariable(env, triggerVar.Name, triggerVar.Value, variables)
	if err != nil {
		return env, err
	}

	/* recursive evaluate */
	for _, childVar := range triggerVar.Variables {
		env, err = evaluateVariable(env, childVar, variables)
		if err != nil {
			return env, err
		}
	}
	if klog.V(5) {
		klog.Infof("After evaluating variable name: %s, value: %s, variables contains: %#v", triggerVar.Name, triggerVar.Value, variables)

	}
	return env, nil
}

func setOneVariable(env cel.Env, name string, val string, variables map[string]interface{}) (cel.Env, error) {
	if name == "" {
		/* name not set */
		return env, nil
	}
	
	val = strings.Trim(val, " ")

	parsed, issues := env.Parse(val)
	if issues != nil && issues.Err() != nil {
		return env, fmt.Errorf("Parsing error setting variable %s to %s, error: %v", name, val, issues.Err())
	}
	checked, issues := env.Check(parsed)
	if issues != nil && issues.Err() != nil {
		return env, fmt.Errorf("CEL check error when setting variable %s to %s, error: %v", name, val, issues.Err())
	}
	prg, err := env.Program(checked)
	if err != nil {
		return env, fmt.Errorf("CEL program error when setting variable %s to %s, error: %v", name, val, err)
	}
	// out, details, err := prg.Eval(variables)
	out, _, err := prg.Eval(variables)
	if err != nil {
		return env, fmt.Errorf("CEL Eval error when setting variable %s to %s, error: %v", name, val, err)
	}

	klog.Infof("When setting variable %s to %s, eval of value results in typename: %s, value type: %T, value: %s\n", name, val, out.Type().TypeName(), out.Value(), out.Value())

	var ok bool
	switch out.Type().TypeName() {
	case TYPEINT:
		var intval int64
		if intval, ok = out.Value().(int64); !ok {
			return env, fmt.Errorf("Unable to cast variable %s, value %s into int64", name, val)
		}
		floatval := float64(intval)

		ident := decls.NewIdent(name, decls.Double, nil)
		env, err = env.Extend(cel.Declarations(ident))
		if err != nil {
			return env, err
		}
		variables[name] = floatval
		klog.Infof("variable %s set to %v", name, floatval)
	case TYPEBOOL:
		boolval, ok := out.Value().(bool);
		if ! ok {
			return env, fmt.Errorf("Unable to cast variable %s, value %s into bool", name, val)
		}

		ident := decls.NewIdent(name, decls.Bool, nil)
		env, err = env.Extend(cel.Declarations(ident))
		if err != nil {
			return env, err
		}
		variables[name] = boolval
		klog.Infof("variable %s set to %v", name, boolval)
	case TYPEDOUBLE:
		floatvar, ok := out.Value().(float64)
		if !ok {
			return env, fmt.Errorf("Unable to cast variable %s, value %s into float64", name, val)
		}

		ident := decls.NewIdent(name, decls.Double, nil)
		env, err = env.Extend(cel.Declarations(ident))
		if err != nil {
			return env, err
		}
		variables[name]  = floatvar
		klog.Infof("variable %s set to %v", name, floatvar)
	case TYPESTRING:
		stringval, ok := out.Value().(string)
		if !ok {
			return env, fmt.Errorf("Unable to cast variable %s, value %s into string", name, val)
		}

		ident := decls.NewIdent(name, decls.String, nil)
		env, err = env.Extend(cel.Declarations(ident))
		if err != nil {
			return env, err
		}
		variables[name] = stringval
		klog.Infof("variable %s set to %v", name, stringval)
	case TYPELIST:
		var listval interface{} = out.Value()
		switch out.Value().(type) {
		case []ref.Val:
		case []interface{}:
		default:
			return  env, fmt.Errorf("Unable to cast variable %s, value %s into %T", name, val, out.Value())
		}
		ident := decls.NewIdent(name, decls.NewListType(decls.Any), nil)
		env, err = env.Extend(cel.Declarations(ident))
		if err != nil {
			return env, err
		}
		variables[name] = listval
		klog.Infof("variable %s set to %v", name, listval)
	case TYPEMAP:
		mapval, ok := out.Value().(map[string]interface{})
		if !ok {
			return env, fmt.Errorf("Unable to cast variable %s, value %s into ma[pstring]interface{}", name, val)
		}
		ident := decls.NewIdent(name, decls.NewMapType(decls.String, decls.Any), nil)
		env, err = env.Extend(cel.Declarations(ident))
		if err != nil {
			return  env, err
		}
		variables[name] = mapval
		klog.Infof("variable %s set to %v", name, mapval)
	default:
		return env, fmt.Errorf("In function setOneVariable: Unable to process variable name: %s, value: %s, type %s", name, val, out.Type().TypeName())
	}
	return env, nil
}

func readTriggerDefinition(fileName string) (*TriggerDefinition, error) {
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}

	var t TriggerDefinition
	err = yaml.Unmarshal(bytes, &t)
	return &t, err
}

/* Find Action for trigger. Only return the first one found*/
func findTrigger(env cel.Env, td *TriggerDefinition, variables map[string]interface{}) (*Action, error) {
	if td.Triggers == nil {
		return nil, nil
	}
	for _, trigger := range td.Triggers {
		action, err := evalTrigger(env, trigger, variables)
		if action != nil || err != nil {
			// either found a match or error
			return action, err
		}
	}
	return nil, nil
}

func evalCondition(env cel.Env, when string, variables map[string]interface{}) (bool, error) {
	if when == "" {
		/* unconditional */
		return true, nil
	}
	parsed, issues := env.Parse(when)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("Error evaluating condition %s, error: %v", when, issues.Err())
	}
	checked, issues := env.Check(parsed)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("Error parsing condition %s, error: %v", when, issues.Err())
	}
	prg, err := env.Program(checked)
	if err != nil {
		return false, fmt.Errorf("Error creating CEL program for condition %s, error: %v", when, err)
	}
	// out, details, err := prg.Eval(variables)
	out, _, err := prg.Eval(variables)
	if err != nil {
		return false, fmt.Errorf("Error evaluating condition %s, error: %v", when, err)
	}

	var boolVal bool
	var ok bool
	if boolVal, ok = out.Value().(bool); !ok {
		return false, fmt.Errorf("The when expression %s does not evaluate to a bool. Instead it is of type %T", when, out.Value())
	}
	return boolVal, nil
}

func evalTrigger(env cel.Env, trigger *Trigger, variables map[string]interface{}) (*Action, error) {
	if trigger == nil {
		return nil, nil
	}

	boolVal, err := evalCondition(env, trigger.When, variables)
	if err != nil {
		return nil, err
	}

	if boolVal {
		/* trigger matched */
		klog.Infof("Trigger %s matched", trigger.When)
		if trigger.Action != nil {
			return trigger.Action, nil
		}

		/* no action. Check children to see if they match */
		if trigger.Triggers == nil {
			return nil, nil
		}

		for _, trigger := range trigger.Triggers {
			action, err := evalTrigger(env, trigger, variables)
			if action != nil || err != nil {
				// either found a match or error
				return action, err
			}
		}
	}
	return nil, nil
}

func substituteTemplateFile(fileName string, variables map[string]interface{}) (string, error) {
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return "", err
	}
	str := string(bytes)
	klog.Infof("Before template substitution for %s: %s", fileName, str)
	substituted, err := substituteTemplate(str, variables)
	if err != nil {
		klog.Errorf("Error in template substitution for %s: %s", fileName, err)
	} else {
		klog.Infof("After template substitution for %s: %s", fileName, substituted)
	}
	return substituted, err
}

func substituteTemplate(templateStr string, variables map[string]interface{}) (string, error) {
	t, err := template.New("kabanero").Parse(templateStr)
	if err != nil {
		return "", err
	}
	buffer := new(bytes.Buffer)
	err = t.Execute(buffer, variables)
	if err != nil {
		return "", err
	}
	return buffer.String(), nil
}

/* Create application. Assume it does not already exist */
func createResource(resourceStr string, dynamicClient dynamic.Interface) error {
	if klog.V(4) {
		klog.Infof("Creating resource %s", resourceStr)
	}
	var unstructuredObj = &unstructured.Unstructured{}
	err := unstructuredObj.UnmarshalJSON([]byte(resourceStr))
	if err != nil {
		klog.Errorf("Unable to convert JSON %s to unstructured", resourceStr)
		return err
	}

	group, version, resource, namespace, name, err := getGroupVersionResourceNamespaceName(unstructuredObj)
	if namespace == "" {
		return fmt.Errorf("resource %s does not contain namepsace", resourceStr)
	}
	gvr := schema.GroupVersionResource{group, version, resource}
	if err == nil {
		var intfNoNS = dynamicClient.Resource(gvr)
		var intf dynamic.ResourceInterface
		intf = intfNoNS.Namespace(namespace)

		_, err = intf.Create(unstructuredObj, metav1.CreateOptions{})
		if err != nil {
			klog.Errorf("Unable to create resource %s/%s error: %s", namespace, name, err)
			return err
		}
	} else {
		klog.Errorf("Unable to create resource /%s.  Error: %s", resourceStr, err)
		return fmt.Errorf("Unable to get GVR for resource %s, error: %s", resourceStr, err)
	}
	if klog.V(2) {
		klog.Infof("Created resource %s/%s", namespace, name)
	}
	return nil
}

/* Return GVR, namespace, name, ok */
func getGroupVersionResourceNamespaceName(unstructuredObj *unstructured.Unstructured) (string, string, string, string, string, error) {
	var objMap = unstructuredObj.Object
	apiVersionObj, ok := objMap[APIVERSION]
	if !ok {
		return "", "", "", "", "", fmt.Errorf("Resource has does not contain apiVersion: %s", unstructuredObj)
	}
	apiVersion, ok := apiVersionObj.(string)
	if !ok {
		return "", "", "", "", "", fmt.Errorf("Resource apiVersion not a string: %s", unstructuredObj)
	}

	components := strings.Split(apiVersion, "/")
	var group, version string
	if len(components) == 1 {
		group = ""
		version = components[0]
	} else if len(components) == 2 {
		group = components[0]
		version = components[1]
	} else {
		return "", "", "", "", "", fmt.Errorf("Resource has invalid group/version: %s, resource: %s", apiVersion, unstructuredObj)
	}

	kindObj, ok := objMap[KIND]
	if !ok {
		return "", "", "", "", "", fmt.Errorf("Resource has invalid kind: %s", unstructuredObj)
	}
	kind, ok := kindObj.(string)
	if !ok {
		return "", "", "", "", "", fmt.Errorf("Resource kind not a string: %s", unstructuredObj)
	}
	resource := kindToPlural(kind)

	metadataObj, ok := objMap[METADATA]
	var metadata map[string]interface{}
	if !ok {
		return "", "", "", "", "", fmt.Errorf("Resource has no metadata: %s", unstructuredObj)
	}
	metadata, ok = metadataObj.(map[string]interface{})
	if !ok {
		return "", "", "", "", "", fmt.Errorf("Resource metadata is not a map: %s", unstructuredObj)
	}

	nameObj, ok := metadata[NAME]
	if !ok {
		return "", "", "", "", "", fmt.Errorf("Resource has no name: %s", unstructuredObj)
	}
	name, ok := nameObj.(string)
	if !ok {
		return "", "", "", "", "", fmt.Errorf("Resource name not a string: %s", unstructuredObj)
	}

	nsObj, ok := metadata[NAMESPACE]
	if !ok {
		return "", "", "", "", "", fmt.Errorf("Resource has no namespace: %s", unstructuredObj)
	}
	namespace, ok := nsObj.(string)
	if !ok {
		return "", "", "", "", "", fmt.Errorf("Resource namespace not a string: %s", unstructuredObj)
	}
	return group, version, resource, namespace, name, nil
}

func kindToPlural(kind string) string {
	lowerKind := strings.ToLower(kind)
	if index := strings.LastIndex(lowerKind, "ss"); index == len(lowerKind)-2 {
		return lowerKind + "es"
	}
	if index := strings.LastIndex(lowerKind, "cy"); index == len(lowerKind)-2 {
		return lowerKind[0:index] + "cies"
	}
	return lowerKind + "s"
}

/* tracking time for timestamp */
var lastTime time.Time = time.Now().UTC()
var mutex = &sync.Mutex{}
/* 
Get timestamp. Timestamp format is UTC time expressed as:
      YYYYMMDDHHMMSSL, where L is last digits in multiples of 1/10 second.
WARNING: This function may sleep up to 0.1 second per request if there are too many concurent requests
*/
func getTimestamp() string {
	mutex.Lock()
	defer mutex.Unlock()

	now := time.Now().UTC()
	for now.Sub(lastTime).Nanoseconds() < 100000000 {
            // < 0.1 since last time. Sleep for at least 0.1 second
            time.Sleep(time.Millisecond*time.Duration(100))
            now = time.Now().UTC()
	}
	lastTime = now

	return fmt.Sprintf("%04d%02d%02d%02d%02d%02d%01d", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(),now.Second(), now.Nanosecond()/100000000)
}
