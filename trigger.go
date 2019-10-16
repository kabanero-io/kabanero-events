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

	"gopkg.in/yaml.v2"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types/ref"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/klog"
	"k8s.io/client-go/dynamic"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
 	 metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	METADATA   = "metadata"
	APIVERSION = "apiVersion"
	KIND       = "kind"
	NAME       = "name"
	NAMESPACE  = "namespace"

	EVENT = "event"

	/* type names */
	TYPE_INT    = "int"
	TYPE_DOUBLE = "double"
	TYPE_BOOL   = "bool"
	TYPE_STRING = "string"
	TYPE_LIST   = "list"
	TYPE_MAP    = "map"
)

type Variable struct {
	Name      string       `yaml:"name"`
	Value     string       `yaml:"value"`
}

type ApplyResource struct {
	Directory string `yaml:"directory"`
}

type Action struct {
	ApplyResources *ApplyResource `yaml:"applyResources"`
}

type Trigger struct {
	When     string     `yaml:"when"`
	Action   *Action     `yaml:"action,omitempty"`
	Triggers []*Trigger `yaml:"triggers,omitempty"`
}


type TriggerDefinition struct {
	Variables  []*Variable `yaml:"variables,omitempty"`
	Triggers   []*Trigger   `yaml:"triggers,omitempty"`
}

type triggerProcessor struct {
	triggerDef *TriggerDefinition
	triggerDir string  // directory where trigger file is stored
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


func (tp *triggerProcessor) processMessage(message map[string]interface{}) error {
	env, variables, err := initializeCelEnv(tp.triggerDef, message)
	if err != nil {
		return err
	}

	/* Go through the trigger section to find the rule to fire*/
	action, err := findTrigger(env, tp.triggerDef, variables)
	if  err != nil {
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
        klog.Errorf("action drectory in trigger file is empty" )
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
			 err = createResource(substituted, dynamicClient)
             if err != nil {
                 return err
             }
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
func initializeCelEnv(td *TriggerDefinition, message map[string]interface{}) (cel.Env, map[string]interface{}, error) {

	variables := make(map[string]interface{})

	/* initilize empty CEL environment */
	env, err := cel.NewEnv()
	if err != nil {
		return nil, nil, err
	}

	/* Create a new environment with event as a built-in variable*/
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

	/* evaluate value of each variable */
	for _, nameVal := range td.Variables {
		//name := nameVal.Name
		val := nameVal.Value
		val = strings.Trim(val, " ")

		parsed, issues := env.Parse(val)
		if issues != nil && issues.Err() != nil {
			return nil, nil, issues.Err()
		}
		checked, issues := env.Check(parsed)
		if issues != nil && issues.Err() != nil {
			return nil, nil, issues.Err()
		}
		prg, err := env.Program(checked)
		if err != nil {
			return nil, nil, err
		}
		// out, details, err := prg.Eval(variables)
		out, _, err := prg.Eval(variables)
		if err != nil {
			return nil, nil, err
		}
		klog.Infof("Eval of %s results in typename: %s, value type: %T, value: %s\n", val, out.Type().TypeName(), out.Value(), out.Value())

		var ok bool
		switch out.Type().TypeName() {
		case TYPE_INT:
			ident := decls.NewIdent(nameVal.Name, decls.Double, nil)
			env, err = env.Extend(cel.Declarations(ident))
			if err != nil {
				return nil, nil, err
			}
			var intval int64
			if intval, ok = out.Value().(int64); !ok {
				return nil, nil, fmt.Errorf("Unable to cast variable %s, value %s into int64", nameVal.Name, nameVal.Value)
			}
			variables[nameVal.Name] = float64(intval)
		case TYPE_BOOL:
			ident := decls.NewIdent(nameVal.Name, decls.Bool, nil)
			env, err = env.Extend(cel.Declarations(ident))
			if err != nil {
				return nil, nil, err
			}
			if variables[nameVal.Name], ok = out.Value().(bool); !ok {
				return nil, nil, fmt.Errorf("Unable to cast variable %s, value %s into bool", nameVal.Name, nameVal.Value)
			}
		case TYPE_DOUBLE:
			ident := decls.NewIdent(nameVal.Name, decls.Double, nil)
			env, err = env.Extend(cel.Declarations(ident))
			if err != nil {
				return nil, nil, err
			}
			if variables[nameVal.Name], ok = out.Value().(float64); !ok {
				return nil, nil, fmt.Errorf("Unable to cast variable %s, value %s into float64", nameVal.Name, nameVal.Value)
			}
		case TYPE_STRING:
			ident := decls.NewIdent(nameVal.Name, decls.String, nil)
			env, err = env.Extend(cel.Declarations(ident))
			if err != nil {
				return nil, nil, err
			}
			if variables[nameVal.Name], ok = out.Value().(string); !ok {
				return nil, nil, fmt.Errorf("Unable to cast variable %s, value %s into string", nameVal.Name, nameVal.Value)
			}
		case TYPE_LIST:
			ident := decls.NewIdent(nameVal.Name, decls.NewListType(decls.Any), nil)
			env, err = env.Extend(cel.Declarations(ident))
			if err != nil {
				return nil, nil, err
			}
			switch out.Value().(type) {
			case []ref.Val:
				variables[nameVal.Name], ok = out.Value().([]ref.Val)
			case []interface{}:
				variables[nameVal.Name], ok = out.Value().([]interface{})
			default:
				return nil, nil, fmt.Errorf("Unable to cast variable %s, value %s into %T", nameVal.Name, nameVal.Value, out.Value())
			}
		case TYPE_MAP:
			ident := decls.NewIdent(nameVal.Name, decls.NewMapType(decls.String, decls.Any), nil)
			env, err = env.Extend(cel.Declarations(ident))
			if err != nil {
				return nil, nil, err
			}
			if variables[nameVal.Name], ok = out.Value().(map[string]interface{}); !ok {
				return nil, nil, fmt.Errorf("Unable to cast variable %s, value %s into ma[pstring]interface{}", nameVal.Name, nameVal.Value)
			}
		default:
			return nil, nil, fmt.Errorf("Unable to process variable name: %s, value: %s, type %s", nameVal.Name, nameVal.Value, out.Type().TypeName())
		}
	}
	return env, variables, nil
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
	for _, trigger := range(td.Triggers) {
		action, err := evalTrigger(env, trigger, variables)
		if action != nil || err != nil {
			// either found a match or error
			return action, err
		}
	}
	return nil, nil
}

func evalTrigger(env cel.Env, trigger *Trigger, variables map[string]interface{}) (*Action, error) {
	    if trigger == nil {
			return nil, nil
		}

		parsed, issues := env.Parse(trigger.When)
		if issues != nil && issues.Err() != nil {
			return nil, issues.Err()
		}
		checked, issues := env.Check(parsed)
		if issues != nil && issues.Err() != nil {
			return nil, issues.Err()
		}
		prg, err := env.Program(checked)
		if err != nil {
			return nil,  err
		}
		// out, details, err := prg.Eval(variables)
		out, _, err := prg.Eval(variables)
		if err != nil {
			return nil, err
		}

		var boolVal bool
		var ok bool
	    if boolVal, ok = out.Value().(bool); !ok {
            return nil, fmt.Errorf("The When expression %s does not evaluate to a bool. Instead it is of type %T", trigger.When, out.Value())
		}
		if boolVal {
			/* trigger matched */
			klog.Infof("Trigger %s matched", trigger.When)
			if  trigger.Action != nil {
			    return trigger.Action, nil
			}

            /* no action. Check children to see if they match */
  		    if trigger.Triggers == nil {
			    return nil, nil
		    }

  	        for _, trigger := range(trigger.Triggers) {
		        action, err := evalTrigger(env, trigger, variables)
		        if action != nil || err != nil {
			        // either found a match or error
			        return action,     err
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
	if (err != nil) {
        klog.Errorf("Error in template substitution for %s: %s", fileName, err)
	} else {
	    klog.Infof("After template substitution for %s: %s", fileName,substituted) 
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
	gvr := schema.GroupVersionResource  { group, version, resource}
	if err == nil {
		var intfNoNS = dynamicClient.Resource(gvr)
		var intf dynamic.ResourceInterface
        intf = intfNoNS.Namespace(namespace)

		_, err = intf.Create(unstructuredObj, metav1.CreateOptions{})
		if err != nil {
			klog.Errorf("Unable to create resource %s/%s error: %s", namespace, name , err)
			return err
		}
	} else {
		klog.Errorf("Unable to create resource /%s.  Error: %s",  resourceStr, err)
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
	if len(components) == 1{
		group = ""
		version = components[0]
	} else if len(components) == 2{
		group = components[0]
		version = components[1]
	} else {
		return "", "", "", "", "", fmt.Errorf("Resource has invalid group/version: %s, resource: %s", apiVersion, unstructuredObj)
	}

	kindObj, ok := objMap[KIND]
	if ! ok {
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
