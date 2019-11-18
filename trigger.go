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
//	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
	"gopkg.in/yaml.v2"
	"sync"
	"encoding/json"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/interpreter/functions"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog"
)

/* Trigger file syntax

keyword: block, default, if, switch

Settings section:
settrings :
  dryrun: bool

eventTrigger section:
eventTriggers:
  - eventSource: <ident>
   input: <variable>
   body:
    ...
  - eventSources: 
    ...

function section:
functions:
  - name: <ident>
    input: <ident>
    output: <ident>
	body:
  - name: <ident>
    ...


And a body:
body:
  - <ident> : <value>
  - <ident>: <expr>
  - if: <condition>
	<ident>: <expr>
  - if: <condition>
	body:
	    - <ident>: <expr>
	    - <ident>: <expr>
  - switch:
	- if: <condition>
	  <ident>: <expr>
	- if: <condition>
	  body:
        - <ident>: <expr>
        - <ident>: <expr>
	- default:
	   - <ident>: <expr>
    - <ident>: <expr>

body:
  - <ident> : <value>
	<ident>: <value>
  - if: <condition>
    <ident>: <value>
  - if: <condition>
    <ident>: <value>
    <ident>: <value>
  - switch:
	- if: <condition>
	  <ident>: <value>
	- if: <condition>
	  body:
        - <ident>: <value>
        - <ident>: <value>
	- <ident>: <value>
*/
  
/* constants for parsing */
const (
	METADATA   = "metadata"
	LABELS     = "labels"
	APIVERSION = "apiVersion"
	KIND       = "kind"
	NAME       = "name"
	NAMESPACE  = "namespace"
	EVENT      = "event" // TODO: remove
	MESSAGE    = "message"
	HEADER     = "header"
    JOBID      = "jobid"
	TYPEINT    = "int"
	TYPEDOUBLE = "double"
	TYPEBOOL   = "bool"
	TYPESTRING = "string"
	TYPELIST   = "list"
	TYPEMAP    = "map"
	WEBHOOK    = "webhook"
	BODY       = "body"
	IF         = "if"
	SWITCH     = "switch"
	DEFAULT    = "default"
	EVENTSOURCE = "eventSource"
	INPUT       = "input"
	OUTPUT       = "output"
	SETTINGS    = "settings"
	EVENTTRIGGERS = "eventTriggers"
	SYSTEMERROR = "systemError"
	FUNCTIONS   = "functions"
)

const (
	// IfFlag is flag for If statement
	IfFlag uint = 1<< iota  
	// SwitchFlag is flag for switch statement
	SwitchFlag
	// DefaultFlag is flag for default statement
	DefaultFlag
	// BodyFlag is flag for body statement
	BodyFlag
)

var keywords map[string] uint = map[string] uint {
	IF: IfFlag,
	SWITCH: SwitchFlag,
	DEFAULT: DefaultFlag, 
	BODY: BodyFlag,
}

func  isKeyword(variableName string) bool {
	_, ok := keywords[variableName]
	return ok
}

/*
Return 
- number of the keywords in the map
- flag for all the keywords found
*/
func countKeywords(mymap map[interface{}]interface{}) (int, uint) {
	count := 0
	flag := uint(0)
	for keyObj:= range mymap {
		key, ok := keyObj.(string)
		if ok {
			mask, ok := keywords[key]
			if ok {
				count++
				flag |= mask
			}
		}
	}
	return count, flag
}

type eventTriggerDefinition struct {
  setting []map[interface{}]interface{} // all settings 
  eventTriggers map[string] []map[interface{}]interface{} // event source name to triggers 
  functions map[string]map[interface{}]interface{} // funtion name to function body
}

type triggerProcessor struct {
	triggerDef *eventTriggerDefinition
	triggerDir string // directory where trigger file is stored
}

func newTriggerProcessor() *triggerProcessor {
	return &triggerProcessor{}
}

/* Initialize trigger directory */
func (tp *triggerProcessor) initialize(dir string) error {
	if klog.V(6) {
		klog.Infof("triggerProcessor.initialize %v", dir)
		defer klog.Infof("Leaving trigggerProcessor.initialize %v", dir)
	}
	var err error
	tp.triggerDef = &eventTriggerDefinition {
		setting: make([]map[interface{}]interface{}, 0),
		eventTriggers: make(map[string] []map[interface{}]interface{}, 0),
		functions: make(map[string]map[interface{}]interface{}, 0),
	}
	files, err := findFiles(dir, []string { ".yaml", ".yml"})
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("Unable to locate trigger files at directory %v", dir)
	}
	for _, fileName := range files {
		err = readTriggerDefinition(fileName, tp.triggerDef)
		if err != nil {
			return err
		}
	}
	tp.triggerDir = dir
	return nil
}

/* Helper to fetch parameters of trigger object 
  input 
	 tringger: the object containing trigger definition
	 output:
	   eventSource: []string
	   input: string
	   body: []interface{}
*/
func parseTrigger(trigger map[interface{}]interface{}) ( []string, string, []interface{}, error) {
	eventSourceArray := make([]string, 0)
	eventSourceObj, ok  := trigger[EVENTSOURCE]
	if !ok {
		return eventSourceArray, "",  nil, fmt.Errorf("trigger object %v does not containt eventSource", trigger)
	}
	eventSource, ok := eventSourceObj.(string)
	if !ok {
		return eventSourceArray, "",  nil, fmt.Errorf("trigger object %v eventSoruce if not a string but a %T", eventSource, eventSourceObj)

	}
	eventSourceArray = append(eventSourceArray,  eventSource)

	inputObj, ok := trigger[INPUT]
	if !ok {
		return eventSourceArray, "",  nil, fmt.Errorf("trigger object %v does not containt input", trigger)
	}
	input, ok := inputObj.(string)
	if !ok {
		return eventSourceArray, "",  nil, fmt.Errorf("trigger object %v input not string but %T", eventSource, inputObj)

	}

	bodyObj, ok := trigger[BODY]
	if !ok {
		return eventSourceArray, "",  nil, fmt.Errorf("trigger object %v does not containt body", trigger)
	}
	body, ok := bodyObj.([]interface{})
	if !ok {
		return eventSourceArray, "",  nil, fmt.Errorf("trigger object %v body not []interface{} but %T", eventSource, bodyObj)

	}
	return eventSourceArray, input, body, nil
}

func (tp *triggerProcessor) processMessage(message map[string]interface{}, eventSource string ) ([]map[string]interface{}, error) {
	if klog.V(5) {
		klog.Infof("Entering triggerProcessor.processMessage. message: %v, eventSource: %v", message, eventSource);
		defer klog.Infof("Leaving triggerProcessor.processMessage")
	}

	if klog.V(5) {
		klog.Infof("before getting triggerArray")
	}
	triggerArray, ok := tp.triggerDef.eventTriggers[eventSource]
	if !ok {
		err := fmt.Errorf("No trigger found for event source %v", eventSource)
		klog.Error(err)
		return nil, err
	}
	if klog.V(5) {
		klog.Infof("Found triggerArray")
	}

	savedVariables := make([]map[string]interface{}, 0)
	for _, trigger := range(triggerArray) {
		/* evaluate all trigger definitions for the event source*/
		eventSources, inputVariable, bodyArray, err := parseTrigger(trigger)
		if err != nil {
			klog.Error(err)
			return nil, err
		}
		if klog.V(5) {
			klog.Infof("processMessage after parseTrigger: eventsources: %v", eventSources)
		}

		env, variables, err := initializeCELEnv( message, inputVariable)
		if err != nil {
			return nil, err
		}
		if klog.V(5) {
			klog.Infof("processMessage after initializeCELEnv");
		}


		depth := 1
		_,  err = evalArrayObject(env, variables, bodyArray, depth)
		if err != nil {
			klog.Errorf("Error evaluating trigger %v: ERROR MESSAGE: %v", trigger, err)
			return nil, err
		}
		if klog.V(5) {
			klog.Infof("processMessage after evalArrayObject");
		}
		savedVariables = append(savedVariables, variables)
	}
	return savedVariables, nil
}

/* Eval body  Array
   env: the CEL execution environment
   variables: variables gathered so far
   bodyParam: body to evaluate
   depth: depth of recursion
   Return:
	 cel.Env: updated execution environment
	 error: any error
*/
func evalArrayObject(env cel.Env, variables map[string]interface{}, bodyArray []interface{}, depth int) (cel.Env, error ) {

	var err error
	for _, objectObj := range(bodyArray) {
		object, ok := objectObj.(map[interface{}] interface{})
		if !ok {
			return env, fmt.Errorf("body object %v not map[interface{}]interface{}, but of type %T", objectObj, objectObj)
		}
		numKeywords, flags := countKeywords(object)
		switch {
			case (flags & IfFlag) != 0 :
				/* If statement, only allow If or If and BODY */
				env, _, err := evalIfWithSyntaxCheck(env, variables, object, numKeywords, flags, depth)
				if err != nil {
					return env, err
				}
				continue
			case (flags & SwitchFlag) != 0 :
				if numKeywords > 1 {
					err = fmt.Errorf("switch contains more than one keyword: %v", object)
					return env, err
				}
				if len(object) > 1 {
					err = fmt.Errorf("switch also contains assignment: %v", object)
					return env, err
				}
				env, err := evalSwitch(env, variables, object, numKeywords, flags, depth)
				if err != nil {
					return env, err
				}
				continue
			case (flags & BodyFlag) != 0 :
				/* evaluate body */
				if numKeywords > 1 {
					err = fmt.Errorf("body contains more than one keyword: %v", object)
					return env, err
				}
				if len(object) > 1 {
					err = fmt.Errorf("body also contains assignment: %v", object)
					return env, err
				}
				env, err := evalBody(env, variables, object, numKeywords, flags, depth)
				if err != nil {
					return env, err
				}
				continue
			case (flags & DefaultFlag) != 0 :
				return env, fmt.Errorf("unexpected keyword default outside of a swtich: %v", objectObj)
			default:
				/* just plain assignment */
				if len(object) > 1 {
					err = fmt.Errorf("Multiple assignments in one object: %v", object)
					return env, err
				}
				env, err = evalAssignment(env, variables, object, numKeywords, flags, depth )
				if err != nil {
					return env, err
				}
		}
	}
	return env, nil
}

func evalAssignment(env cel.Env, variables map[string]interface{}, object map[interface{}]interface{}, numKeywords int, flags uint, depth int) (cel.Env, error) {
	if klog.V(6) {
		klog.Infof("Entering evalAssignment object: %v", object)
		defer klog.Infof("Leaving evalAssignment object")
	}
	var err error
	for variableNameObj, valObj := range object {
		if klog.V(6){
			klog.Infof("processing name: %v, type %T, object: %v, type %T", variableNameObj, variableNameObj, valObj, valObj)
		}
		variableName, ok := variableNameObj.(string)
		if !ok {
			return env, fmt.Errorf("varialbe %s is not  string, but of type %T", variableNameObj, variableNameObj)
		}
		if isKeyword(variableName) {
			continue
		}

		/* Format value as string for CEL parsing */
		var val string
		switch valObj.(type) {
			case int:
				val = fmt.Sprintf("%v", valObj)
			case int64:
				val = fmt.Sprintf("%v", valObj)
			case int32:
				val = fmt.Sprintf("%v", valObj)
			case float32:
				val = fmt.Sprintf("%v", valObj)
			case float64:
				val = fmt.Sprintf("%v", valObj)
			case bool:
				val = fmt.Sprintf("%v", valObj)
			case string:
				val = valObj.(string)
			default:
				return env, fmt.Errorf("Value of variables not stored as  YAML primitive types or string when assgining %v to %v. Type of value is %T", variableName, valObj, valObj)
		}
        env, err = setOneVariable(env, variableName, val, variables ) 
		if err != nil {
			return env, err
		}
	}
	return env, nil
}

/*
 * Evaluate body 
 */
func evalBody(env cel.Env, variables map[string]interface{}, object map[interface{}]interface{}, numKeyword int, flags uint, depth int) (cel.Env, error) {
	/* check if recursive body exists */
	nestedBodyObj := object[BODY]
	nestedBody, ok := nestedBodyObj.([]interface{})
	if ok {
		return evalArrayObject(env, variables, nestedBody, depth );
	} 

	err := fmt.Errorf("body %v contains nested body that is not []interface, but of type %T", nestedBodyObj, nestedBody)
	return env, err
}

func evalIfWithSyntaxCheck(env cel.Env, variables map[string]interface{}, object map[interface{}]interface{}, numKeywords int, flags uint, depth int) (cel.Env, bool, error) {
	if klog.V(6) {
		klog.Infof("evalIfWithSyntaxCheck : %v", object)
	}
	if numKeywords > 2 {
		err := fmt.Errorf("body of if %v contains more than two keyword", object)
		return env, false, err
	}
	if numKeywords == 2 && (flags & BodyFlag) == 0  && (flags & SwitchFlag) == 0  {
		/* second keyword is not body or switch */
		err := fmt.Errorf("if object also contains keywords other than body or switch: %v", object)
		return env, false, err
	}
	if numKeywords == 2 && len(object) > 2 {
		err := fmt.Errorf("can not mix assignment with body object in if: %v", object)
		return env, false, err
	}

	conditionObj := object[IF]
	condition, ok := conditionObj.(string)
	if ( !ok ) {
		return env, false, fmt.Errorf("condition of if object not a string: %v", object)
	}
	boolVal, err := evalCondition(env, condition, variables)
	if err != nil {
		return env, false, err
	}

	if !boolVal {
		/* condition not met */
		if klog.V(6) {
			klog.Infof("evalIfWithSyntaxCheck condition not met: %v", condition)
		}
		return env, false, nil
	}

	if klog.V(6) {
		klog.Infof("evalIfWithSyntaxCheck condition met: %v", condition)
	}
	_, ok = object[BODY]
	if ok {
		/* if statement also contains body */
		env, err = evalBody(env, variables, object, numKeywords, flags, depth)
		return env,  true, err
	} 

	_, ok = object[SWITCH]
	if ok {
		/* if statement also contains switch */
		env, err = evalSwitch(env, variables, object, numKeywords, flags, depth)
		return env,  true, err
	} 

	/* perform assignments */
	env, err = evalAssignment(env, variables, object,  numKeywords, flags, depth)
	return env, true, err
}

func evalSwitch(env cel.Env, variables map[string]interface{}, object map[interface{}]interface{}, numKeywords int, flags uint, depth int) (cel.Env, error) {
	var err error
	switchObj, ok :=  object[SWITCH]
	if !ok {
		return env, fmt.Errorf("Expecting switch statement: %v ", switchObj)
	}
	switchArray, ok := switchObj.([]interface{})
	if !ok {
		return env, fmt.Errorf("body of switch not array of objects: %v ", switchObj)

	}
	var defaultArray []interface{} = nil
	for _ , arrayElementObj := range switchArray {
		arrayElement, ok := arrayElementObj.(map[interface{}]interface{})
		if !ok {
			return env, fmt.Errorf("body component of switch not object: %v ", arrayElementObj)
		}
		switchCaseNumKeywords, switchCaseFlags := countKeywords(arrayElement)
		_, ifOK := arrayElement[IF]
		if ifOK {
			/* evaluate the if statement */
			env, conditionTrue, err := evalIfWithSyntaxCheck(env, variables, arrayElement, switchCaseNumKeywords, switchCaseFlags, depth)
			if err != nil || conditionTrue {
				return env, err
			}
			continue
		} 
		defaultObj, defaultOK := arrayElement[DEFAULT]
		if defaultOK {
			if len(arrayElement) > 1 {
				return env, fmt.Errorf("default object must be stand alone: %v", object)
			}
			if defaultArray != nil {
				return env, fmt.Errorf("Only one default statement supported.  Extra default statement: %v", arrayElement)
			}
			defaultArray, ok  = defaultObj.([]interface{})
			if !ok {
				return env, fmt.Errorf("content of default not []interface{}: %v, type: %T", defaultObj, defaultObj)

			}
			continue
		}
		/* Unsupported keyword, or assignment */
		return env, fmt.Errorf("switch statement must contain if or default statements, but found: %v, type %T", arrayElement, arrayElement)
		
	}
	/* evaluate defaults */

	if defaultArray != nil  {
		env, err = evalArrayObject(env, variables, defaultArray, depth)
		if err != nil {
			return env, err
		}
	}
	return env, nil
}


/* Shallow copy a map */
func shallowCopy(originalMap map[string]interface{}) map[string]interface{} {
	newMap := make(map[string]interface{})
	for key, val := range originalMap {
		newMap[key] = val
	}
	return newMap
}


/* Get initial CEL environment 
*/
func initializeEmptyCELEnv() (cel.Env, error) {
	/* initilize empty CEL environment with additional functions */
	additionalFuncs := getAdditionalCELFuncDecls()
//	klog.Infof("Additional Func Decls: %v", additionalFuncs)
	return cel.NewEnv(additionalFuncs )
}

/* Get initial CEL environment
	Return: cel.Env: the CEL environment
		map[string]interface{}: variables used during substitution
		error: any error encountered
 */
func initializeCELEnv(message map[string]interface{}, inputVariableName string) (cel.Env, map[string]interface{},  error) {
	if klog.V(5) {
		klog.Infof("entering initializeCELEnv")
		defer klog.Infof("Leaving initializeCELEnv")
	}

	/* initilize empty CEL environment with additional functions */
	env, err := initializeEmptyCELEnv()
	if err != nil {
		return nil, nil,  err
	}

	variables := make(map[string]interface{})
	ident := decls.NewIdent(inputVariableName, decls.NewMapType(decls.String, decls.Any), nil)
	env, err = env.Extend(cel.Declarations(ident))
	if err != nil {
		return nil, nil,  err
	}
	/* Add message as a new variable */
	variables[inputVariableName] = message

	return env, variables,  nil
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
		return env, fmt.Errorf("CEL check error when setting variable %s to %s, error: %v, existing variables: %v", name, val, issues.Err(), variables)
	}
	prg, err := env.Program(checked, getAdditionalCELFuncs())
	if err != nil {
		return env, fmt.Errorf("CEL program error when setting variable %s to %s, error: %v", name, val, err)
	}
	// out, details, err := prg.Eval(variables)
	out, _, err := prg.Eval(variables)
	if err != nil {
		return env, fmt.Errorf("CEL Eval error when setting variable %s to %s, error: %v", name, val, err)
	}

	if klog.V(3) {
		klog.Infof("When setting variable %s to %s, eval of value results in typename: %s, value type: %T, value: %s\n", name, val, out.Type().TypeName(), out.Value(), out.Value())
	}

	env, err = createOneVariable(env, name, val, out, variables)
	return env, err
}


func createOneVariable(env cel.Env, entireName string, val string, out ref.Val, variables map[string]interface{}) (cel.Env, error) {
	if klog.V(6){
		klog.Infof("Entering createOneVariables: setting %v to %v", entireName, val)
		defer klog.Infof("Levaning createOneVariables: setting %v to %v", entireName, val)
	}
	nameArray := strings.Split(entireName, ".")
	arrayLen := len(nameArray)
	tempMap := variables
	var err error
	for index:= 0;  index < arrayLen-1; index++ {
		componentName := nameArray[index]
		entry, ok := tempMap[componentName]
		if !ok {
			/* multi-level name, and entry does not exist. Create it */
			if (index == 0) && (arrayLen > 1) {
				/* create top level identifier */
				ident := decls.NewIdent(componentName, decls.NewMapType(decls.String, decls.Any), nil)
				env, err = env.Extend(cel.Declarations(ident))
				if err != nil {
					return  env, err
				}
			}
			newMap := make(map[string]interface{})
			tempMap[componentName] = newMap
			tempMap = newMap
		} else {
			/* entry already exists */
			entryMap, ok := entry.(map[string]interface{})
			if ! ok {
				return env, fmt.Errorf("unable to set %s to %s: the component name %s already exists but is not a map", entireName, val, componentName) 
			}
			tempMap = entryMap
		}
	}
	lastComponent := nameArray[arrayLen-1]
	/* If we get here, tempMap is a map that we can directly insert the value */
    env, err = createOneVariableHelper(env, entireName, lastComponent,  val, out, tempMap, arrayLen == 1)
	return env, err
}

func createOneVariableHelper(env cel.Env, entireName string, name string, val string, out ref.Val, variables map[string]interface{}, createNewIdent bool) (cel.Env, error) {
	if klog.V(6) {
		klog.Infof("Entering createOneVariableHelper entireName: %v, name: %v, val: %v, type: %v, createNewIdent %v", entireName, name, val, out.Type().TypeName(), createNewIdent)
		defer klog.Infof("Leaving createOneVariableHelper entireName: %v, name: %v, val: %v, type: %v", entireName, name, val, out.Type().TypeName())
	}
	var ok bool
	var err error
	switch out.Type().TypeName() {
	case TYPEINT:
		var intval int64
		if intval, ok = out.Value().(int64); !ok {
			return env, fmt.Errorf("Unable to cast variable %s, value %s into int64", entireName, val)
		}
		if createNewIdent {
			ident := decls.NewIdent(name, decls.Int, nil)
			env, err = env.Extend(cel.Declarations(ident))
			if err != nil {
				return env, err
			}
		}
		variables[name] = intval
		if klog.V(4) {
			klog.Infof("variable %s set to %v", entireName, intval)
		}
		/*
		floatval := float64(intval)

		if createNewIdent {
			ident := decls.NewIdent(name, decls.Double, nil)
			env, err = env.Extend(cel.Declarations(ident))
			if err != nil {
				return env, err
			}
		}
		variables[name] = floatval
		if klog.V(4) {
			klog.Infof("variable %s set to %v", entireName, floatval)
		}
		*/
	case TYPEBOOL:
		boolval, ok := out.Value().(bool);
		if ! ok {
			return env, fmt.Errorf("Unable to cast variable %s, value %s into bool", entireName, val)
		}

		if createNewIdent {
			ident := decls.NewIdent(name, decls.Bool, nil)
			env, err = env.Extend(cel.Declarations(ident))
			if err != nil {
				return env, err
			}
		}
		variables[name] = boolval
		if klog.V(4) {
			klog.Infof("variable %s set to %v", entireName, boolval)
		}
	case TYPEDOUBLE:
		floatvar, ok := out.Value().(float64)
		if !ok {
			return env, fmt.Errorf("Unable to cast variable %s, value %s into float64", entireName, val)
		}

		if createNewIdent {
			ident := decls.NewIdent(name, decls.Double, nil)
			env, err = env.Extend(cel.Declarations(ident))
			if err != nil {
				return env, err
			}
		}
		variables[name]  = floatvar
		if klog.V(4) {
			klog.Infof("variable %s set to %v", entireName, floatvar)
		}
	case TYPESTRING:
		stringval, ok := out.Value().(string)
		if !ok {
			return env, fmt.Errorf("Unable to cast variable %s, value %s into string", entireName, val)
		}

		if createNewIdent {
			ident := decls.NewIdent(name, decls.String, nil)
			env, err = env.Extend(cel.Declarations(ident))
			if err != nil {
				return env, err
			}
		}
		variables[name] = stringval
		if klog.V(4) {
			klog.Infof("variable %s set to %v", entireName, stringval)
		}
	case TYPELIST:
		/*
		switch out.Value().(type) {
		case []ref.Val:
		case []interface{}:
		case []string:
		case []int64:
		case []float64:
		default:
			return  env, fmt.Errorf("Unable to cast variable %s, value %s into %T", entireName, val, out.Value())
		}
		*/
		if createNewIdent {
			ident := decls.NewIdent(name, decls.NewListType(decls.Any), nil)
			env, err = env.Extend(cel.Declarations(ident))
			if err != nil {
				return env, err
			}
		}
		var listval interface{} = out.Value()
		variables[name] = listval
		if klog.V(4) {
			klog.Infof("variable %s set to %v", entireName, listval)
		}
	case TYPEMAP:
		_, ok := out.Value().(map[string]interface{})
		if !ok {
			_, ok := out.Value().( map[ref.Val]ref.Val)
			if !ok {
				return env, fmt.Errorf("Unable to cast variable %s, value %s, evaluaed value %v, type %T into map[ref.Val]ref.Val", entireName, val, out.Value(), out.Value())
			}
		}
		if createNewIdent {
			ident := decls.NewIdent(name, decls.NewMapType(decls.String, decls.Any), nil)
			env, err = env.Extend(cel.Declarations(ident))
			if err != nil {
				return  env, err
			}
		}
		variables[name] = out.Value()
		if klog.V(4) {
			klog.Infof("variable %s set to %v", entireName, out)
		}
	default:
		if createNewIdent {
			/* don't have a usable type to create new top level variable  */
			return env, fmt.Errorf("In function setOneVariable: Unable to process variable name: %s, value: %s, type %s", entireName, val, out.Type().TypeName())
		}
		/* If not creating a new identifier, try to use the value as is. TODO: Should we just use out? */
		variables[name] = out.Value()
		if klog.V(4) {
			klog.Infof("variable %s set to %v", entireName, out)
		}
	}
	return env, nil
}

/*
func normalizeMapString(mapObj map[string] interface{}) (map[string]interface{}, error) {
	ret := make(map[string]interface{}, 0)
	for key, val := range yaml {
		newVal, err := normalizeValue(val)
		if err != nil {
			return nil, err
		}
		ret[key] =  newVal
	}
}

func normalizeValue(val interface{}) (interface{}, error)  {
		var newVal interface{}
		var err error
		switch val.(type) {
		case map[interface{}] interface{}:
			newVal, err = normalizeMapInterface(val.(map[interface{}]interface{}))
			if err != nil {
				return nil, err
			}
		case map[string]interface{}:
			newVal, err = normalizeMapString(val.map[string]interface{})
			if err != nil {
				return nil, err
			}
		case [] interface{} :
			newVal, err = normalizeArrayInterface(val.([]interface{}))
			if err != nil {
				return nil, err
			}
		default:
			newVal = val
		}
		return newVal, nil
}

func normalizeMapInterface( mapObj map[interface{}] interface{}) (map[string]interface{}, error) {
	ret := make(map[string]interface{}, 0)
	for key, val := range mapObj {
		keyStr, ok := key.(string)
		if !ok {
			return nil, fmt.Errorf("key %v is not string, but %T", key, key)
		}
		ret[keyStr] = val
	}
	return ret, nil
}
func normalizeArrayInterface(val []interface{}) ([]interface{}, error) {
	ret := make([]interface{}, 0)
	for _, valObj := range val {
		newVal, err := normalizeValue(valObj)
		if err != nil {
			return nil, err
		}
		ret = append(ret, newVal)
	}
	return ret, nil
}
*/

func readTriggerDefinition(fileName string, td *eventTriggerDefinition) error {
	if klog.V(5) {
		klog.Infof("enter readTriggerDefinitions %v", fileName)
		defer klog.Infof("Leaving readTriggerDefinitions %v", fileName)
	}
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}

	yamlMap := make(map[string]interface{})
	err = yaml.Unmarshal(bytes, yamlMap)
	if err != nil  {
		return fmt.Errorf("Unable to marshal %v. Error: %v", fileName, err)
	}

	/* gather args in the yaml */
	settingsObj, ok := yamlMap[SETTINGS]
	if ok {
		if klog.V(5) {
			klog.Infof("found settings %v", settingsObj)
		}
		settings, ok := settingsObj.(map[interface{}]interface{})
		if ok {
			td.setting = append(td.setting, settings)
		}
	}

	eventTriggersObj, ok := yamlMap[EVENTTRIGGERS]
	if ok {
		if klog.V(5) {
			klog.Infof("found eventTriggers %v %T", eventTriggersObj, eventTriggersObj)
		}
		eventTriggersArray, ok := eventTriggersObj.([]interface{})
		if ok {
			for _, triggerMapObj := range(eventTriggersArray) {
				triggerMap, ok := triggerMapObj.(map[interface{}]interface{})
				if !ok {
					return fmt.Errorf("triggerMapObj %v not type map[interface{}]interface{}, but type %T", triggerMapObj, triggerMapObj)
				}
				eventSourceObj, ok := triggerMap[EVENTSOURCE]
				if ok {
					if klog.V(5) {
						klog.Infof("Found eventSource %v", eventSourceObj)
					}
					eventSource, ok := eventSourceObj.(string)
					if ok {
						existingArray, ok := td.eventTriggers[eventSource]
						if !ok {
							existingArray = make([]map[interface{}]interface{}, 0)
						}
						td.eventTriggers[eventSource] = append(existingArray, triggerMap)
					}
				}
			}
		} else {
			return fmt.Errorf("event trigger %v not type map[interface{}]interface{] but type %F", eventTriggersObj, eventTriggersObj)
		}
	}

	/* read functions */
	functionsObj, ok := yamlMap[FUNCTIONS]
	if ok {
		functionsArray, ok := functionsObj.([]interface{})
		if ok {
			for _, functionMapObj := range(functionsArray) {
				functionMap, ok := functionMapObj.(map[interface{}] interface{})
				if !ok {
					if klog.V(5) {
						klog.Infof("functionMap %v not of type map[interface{}]interface{}, but is type %T", functionMapObj, functionMapObj)
					}
					continue
				}
				nameObj, ok := functionMap[NAME]
				if ok {
					name, ok := nameObj.(string)
					if ok {

						inputObj, ok := functionMap[INPUT]
						if !ok {
							return fmt.Errorf("function %v does not contain input variable name: %v", name, functionMap)
						}
						_, ok = inputObj.(string)
						if !ok {
							return fmt.Errorf("function input %v not a string: %v", name, inputObj)
						} 

						outputObj, ok := functionMap[OUTPUT]
						if !ok {
							return fmt.Errorf("function %v does not contain output variable name: %v", name, functionMap)
						}
						_, ok = outputObj.(string)
						if !ok {
							return fmt.Errorf("function output %v not a string %v", name, outputObj)
						}

						_, existing := td.functions[name]
						if existing {
							return fmt.Errorf("error: event trigger function redcelared: %v", name)
						}
						td.functions[name] = functionMap
					}
				}
			}
		} else {
			return fmt.Errorf("functionsArray %v not of type []interface{}, but type %T", functionsArray, functionsArray)
		}
	} 

	return nil
}

/* Find Action for trigger. Only return the first one found*/
//func findTrigger(env cel.Env, td *EventTriggerDefinition, variables map[string]interface{}) (*Action, error) {
//	if td.EventTriggers == nil {
//		return nil, nil
//	}
//	for _, trigger := range td.EventTriggers {
//		action, err := evalTrigger(env, trigger, variables)
//		if action != nil || err != nil {
//			// either found a match or error
//			return action, err
//		}
//	}
//	return nil, nil
//}

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
	prg, err := env.Program(checked, getAdditionalCELFuncs())
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

//func evalTrigger(env cel.Env, trigger *EventTrigger, variables map[string]interface{}) (*Action, error) {
//	if trigger == nil {
//		return nil, nil
//	}
//
//	boolVal, err := evalCondition(env, trigger.When, variables)
//	if err != nil {
//		return nil, err
//	}
//
//	if boolVal {
//		/* trigger matched */
//		klog.Infof("Trigger %s matched", trigger.When)
//		if trigger.Action != nil {
//			return trigger.Action, nil
//		}
//
//		/* no action. Check children to see if they match */
//		if trigger.EventTriggers == nil {
//			return nil, nil
//		}
//
//		for _, trigger := range trigger.EventTriggers {
//			action, err := evalTrigger(env, trigger, variables)
//			if action != nil || err != nil {
//				// either found a match or error
//				return action, err
//			}
//		}
//	}
//	return nil, nil
//}

func substituteTemplateFile(fileName string, variables interface{}) (string, error) {
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return "", err
	}
	str := string(bytes)
	klog.Infof("Before template substitution for %s: %s, variables type: %T", fileName, str, variables)
	substituted, err := substituteTemplate(str, variables)
	if err != nil {
		klog.Errorf("Error in template substitution for %s: %s", fileName, err)
	} else {
		klog.Infof("After template substitution for %s: %s", fileName, substituted)
	}
	return substituted, err
}

func substituteTemplate(templateStr string, variables interface{}) (string, error) {
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

/* Create resource. Assume it does not already exist */
func createResource(resourceStr string, dynamicClient dynamic.Interface) error {
	if klog.V(4) {
		klog.Infof("Creating resource %s", resourceStr)
	}

	/* Convert yaml to unstructured*/
	resourceBytes, err := k8syaml.ToJSON([]byte(resourceStr))
	if err != nil {
		return fmt.Errorf("Unable to convert yaml resource to JSON: %v", resourceStr)
	}
	var unstructuredObj = &unstructured.Unstructured{}
	err = unstructuredObj.UnmarshalJSON(resourceBytes)
	if err != nil {
		klog.Errorf("Unable to convert JSON %s to unstructured", resourceStr)
		return err
	}

	group, version, resource, namespace, name, err := getGroupVersionResourceNamespaceName(unstructuredObj)
	if namespace == "" {
		return fmt.Errorf("resource %s does not contain namepsace", resourceStr)
	}

	/* add label kabanero.io/jobld = <jobid> */
	/*
	err = setJobID(unstructuredObj, jobid)
	if err != nil {
		return err
	}
	*/

	if klog.V(5) {
		klog.Infof("Resources before creating : %v", unstructuredObj)
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

func setJobID(unstructuredObj *unstructured.Unstructured, jobid string) error {
	var objMap = unstructuredObj.Object
	metadataObj, ok := objMap[METADATA]
	if !ok {
		return fmt.Errorf("Resource has no metadata: %v", unstructuredObj)
	}
	var metadata map[string]interface{}
	metadata, ok = metadataObj.(map[string]interface{})
	if !ok {
		return fmt.Errorf("Resource metadata is not a map: %v", unstructuredObj)
	}

	labelsObj, ok := metadata[LABELS]
	var labels map[string]interface{}
	if !ok {
		/* create new label */
		labels = make(map[string]interface{})
		metadata[LABELS] = labels
	} else {
		labels, ok = labelsObj.(map[string]interface{})
		if !ok {
			return fmt.Errorf("Resource labels is not a map: %v", labels)
		}
	}
	labels["kabanero.io/jobid"] = jobid
	
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

/* implementation of toDomainName for CEL */
func toDomainNameCEL(param ref.Val) ref.Val {
	str, ok := param.(types.String)
	if !ok {
		return types.ValOrErr(param, "unexpected type '%v' passed to toDomainName", param.Type())
	}
	return types.String(toDomainName(string(str)))
}

/* implementation of toLabel for CEL */
func toLabelCEL(param ref.Val) ref.Val {
	str, ok := param.(types.String)
	if !ok {
		return types.ValOrErr(param, "unexpected type '%v' passed to toLabel", param.Type())
	}
	return types.String(toLabel(string(str)))
}

/* implementation of split for CEL */
func splitCEL(strVal ref.Val, sepVal ref.Val ) ref.Val {
	str, ok := strVal.(types.String)
	if !ok {
		return types.ValOrErr(strVal, "unexpected type '%v' passed as first parameter to function split", strVal.Type())
	}
	sep, ok := sepVal.(types.String)
	if !ok {
		return types.ValOrErr(sepVal, "unexpected type '%v' passed as second parameter to function split", sepVal.Type())
	}
	arrayStr := strings.Split(string(str), string(sep))

	return types.NewStringList(types.DefaultTypeAdapter, arrayStr)
}

/* Return next job ID */
func jobIDCEL(values ...ref.Val) ref.Val {
	return types.String(getTimestamp())
}


/* Return next job ID */
func kabaneroConfigCEL(values ...ref.Val) ref.Val {
    ret := make(map[string]interface{})
	ret[NAMESPACE] = webhookNamespace
	return types.NewDynamicMap(types.DefaultTypeAdapter, ret)
}

/* implementation of downlodYAML for CEL. 
   webhookMessage: map[string]interface{} contains the original webhook message
   fileNameVal: name of file to download
   Return: map[string] interface{} where
	   map["error"], if set, is the error message enccountered when reading the file.
       map["exists"] is true if the file exists, or false if it doesn't exist
	   map["content"], if set, is the actual file content, of type map[string]interface{}
*/
func downloadYAMLCEL(webhookMessage ref.Val, fileNameVal ref.Val) ref.Val {
	klog.Infof("downloadYAMLCEL first param: %v, second param: %v", webhookMessage, fileNameVal)

	if webhookMessage.Value() == nil {
		return types.ValOrErr(webhookMessage, "unexpected null first parameter passed to function downloadYAML.") 
	}
	mapInst, ok := webhookMessage.Value().(map[string]interface{})
	if !ok {
		return types.ValOrErr(webhookMessage, "unexpected type '%v' passed as first parameter to function downloadYAML. It should be map[string]interface{}", webhookMessage.Type())
	}

	bodyMapObj, ok := mapInst[BODY]
	if !ok {
		return types.ValOrErr(webhookMessage, "Missing event parameter %v passed to downloadYAML.", webhookMessage)
	}
	var bodyMap map[string]interface{}
	bodyMap, ok  = bodyMapObj.(map[string]interface{})
	if !ok {
		return types.ValOrErr(webhookMessage, "Event parameter %v passed to downloadYAML not map[string]interface{}. Instead, it is %T.", webhookMessage, bodyMapObj)
	}

	headerMapObj, ok := mapInst[HEADER]
	if !ok {
		return types.ValOrErr(webhookMessage, "Missing header parameter %v passed to downloadYAML.", webhookMessage)
	}
	var headerMap map[string][]string
	headerMap, ok = headerMapObj.(map[string][]string)
	if !ok {
		return types.ValOrErr(webhookMessage, "Parameter %v passed to downloadYAML not map[string][]string. Instead, it is %T.", headerMapObj, headerMapObj)
	}
	
	if fileNameVal.Value() == nil {
		return types.ValOrErr(fileNameVal, "unexpected null second parameter passed to function downloadYAML.") 
	}
	fileName, ok := fileNameVal.Value().(string)
	if !ok {
		return types.ValOrErr(fileNameVal, "unexpected type '%v' passed as first parameter to function downloadYAML. It should be string", fileNameVal.Type())
	}

	var ret map[string]interface{} = make(map[string]interface{})
	fileContent, exists , err := downloadYAML(headerMap, bodyMap, fileName) 
	ret["exists"] = exists
	if err != nil {
		ret["error"] = fmt.Sprintf("%v", err)
		if klog.V(5) {
			klog.Infof("downloadYAMLCEI error: %v", err)
		}
	} else {
		ret["content"] = fileContent
		if klog.V(5) {
			klog.Infof("downloadYAMLCEI content: %v", fileContent)
		}
	}
	return types.NewDynamicMap(types.DefaultTypeAdapter, ret)
}


/* implementation of call for CEL. 
   function string: name of function to call
   param map[string]interface{}: param to pass to function
   Return interface{} : result
*/
func callCEL(functionVal ref.Val, param ref.Val) ref.Val {
	if klog.V(6) {
		klog.Infof("callCEL first param: %v, second param: %v", functionVal, param)
	}

	if param.Value() == nil {
		klog.Infof("callCEL param is nil")
		return types.ValOrErr(param, "unexpected null second parameter passed to function call.") 
	}
	/*
	paramMap, ok := param.Value().(map[string]interface{})
	if !ok {
		klog.Infof("callCEL param is not map[string]interface{}")
		return types.ValOrErr(param, "unexpected type '%v' passed as second parameter to function call. It should be map[string]interface{}", param.Type())
	}
	*/

	if functionVal.Value() == nil {
		klog.Infof("callCEL function is nil")
		return types.ValOrErr(functionVal, "unexpected null first parameter passed to function call.") 
	}
	if klog.V(6) {
		klog.Infof("callCEL first param type: %v, second param type: %v", functionVal.Type(), param.Type())
	}

	function, ok := functionVal.Value().(string)
	if !ok {
		klog.Infof("callCEL function is not string")
		return types.ValOrErr(functionVal, "unexpected type '%v' passed as first parameter to function call. It should be string", functionVal.Type())
	}

	if klog.V(6) {
		klog.Infof("callCEL: getting functions:  %v ", triggerProc.triggerDef.functions)
	}

	functionDecl, ok := triggerProc.triggerDef.functions[function]
	if !ok {
		klog.Errorf("callCEL function %v not found", function)
		return types.ValOrErr(functionVal, "function %v not found", function)
	}

	inputObj, ok := functionDecl[INPUT]
	if !ok {
		klog.Infof("callCEL function %v input variable name not found", function)
		return types.ValOrErr(functionVal, "function %v does not contain input variable", functionDecl)
	}
	input, ok := inputObj.(string)
	if !ok {
		klog.Infof("callCEL function %v input variable name not string", function)
		return types.ValOrErr(functionVal, "input variable of function %v not a string: %v", function, inputObj)
	}
	outputObj, ok := functionDecl[OUTPUT]
	if !ok {
		klog.Infof("callCEL function %v output variable name not found", function)
		return types.ValOrErr(functionVal, "function %v does not contain output variable", functionDecl)
	}
	output, ok := outputObj.(string)
	if !ok {
		klog.Infof("callCEL function %v output variable name not string", function)
		return types.ValOrErr(functionVal, "output variable of function %v not a string: %v", functionDecl, outputObj)
	}

	bodyArrayObj , ok := functionDecl[BODY]
	if !ok {
		klog.Infof("callCEL function %v function body not found", function)
		return types.ValOrErr(functionVal, "function %v does not a have a body", functionDecl)
	}
	bodyArray, ok := bodyArrayObj.([]interface{})
	if !ok {
		klog.Infof("callCEL function %v function body not []interface{}", function)
		return types.ValOrErr(functionVal, "body of function %v not not []interface{}", functionDecl)
	}


	variables := make(map[string]interface{})
	env, err := initializeEmptyCELEnv() 
	if err != nil {
		klog.Infof("callCEL function %v Unable to initialize CEL environment", function)
		return types.ValOrErr(functionVal, "callCEL Unable to initialize CEL environment. Error: %v ", err)
	}

	env, err = createOneVariable(env, input, "", param , variables)
	if err != nil {
		klog.Infof("callCEL function %v unable to create input variable %v for %v", function, input, param)
		return types.ValOrErr(param, "callCEL Unable to initialize CEL environment. Error: %v ", err)
	}

	depth := 1
	_,  err = evalArrayObject(env, variables, bodyArray, depth)
	if err != nil {
		klog.Infof("callCEL error: %v", err)
		return types.ValOrErr(param, "callCEL error evaluating function body. Error: %v ", err)
	} 
	outValueObj, ok := variables[output]
	if !ok {
		klog.Errorf("callCEL error calling function %v: output variable %v not set", function, output)
		return types.ValOrErr(types.NewDynamicMap(types.DefaultTypeAdapter, variables), "error calling function %v: output variable %v not set by function", function, output)
	} 

	ret, err := convertToRefVal(outValueObj)
	if err != nil {
		return types.ValOrErr(types.NewDynamicMap(types.DefaultTypeAdapter, variables), "while calling function %v: return value  %v has unsupproted type %T", function, outValueObj, outValueObj)
	}
	return ret
}

/* Convert a value to ref.Val
*/
func convertToRefVal(outValueObj interface{}) (ref.Val, error) {
	var ret ref.Val // return value
	var err error = nil
	switch outValueObj.(type) {
	case bool:
		ret = types.Bool(outValueObj.(bool))
	case int64:
		ret = types.Int(outValueObj.(int64))
	case float64:
		ret = types.Double(outValueObj.(float64))
	case string:
		ret = types.String(outValueObj.(string))
	case ref.Val:
		ret = outValueObj.(ref.Val)
	case map[string] interface{}:
		ret = types.NewDynamicMap(types.DefaultTypeAdapter,outValueObj)
	case map[interface{}] interface{}:
		ret = types.NewDynamicMap(types.DefaultTypeAdapter,outValueObj)
	case map[ref.Val] ref.Val:
		ret = types.NewDynamicMap(types.DefaultTypeAdapter,outValueObj)
	case [] interface{}:
		ret = types.NewDynamicList(types.DefaultTypeAdapter, outValueObj)
	case [] ref.Val:
		ret = types.NewDynamicList(types.DefaultTypeAdapter, outValueObj)
	default:
		ret = nil
		err = fmt.Errorf("Unable to convert %v of type %T to rev.Val",  outValueObj, outValueObj)
	}
	return ret, err
}

/* implementation of call for applyResources. 
   dir string: directory
   variable Any: variable to pass to go template
   Return string : empty if OK, otherwise, error message
*/
func applyResourcesCEL(dir ref.Val, variables ref.Val) ref.Val {
	klog.Infof("applyResourcesCEL first param: %v, second param: %v", dir, variables)

	if variables.Value() == nil {
		klog.Infof("applyResourcesCEL variables is nil")
		return types.ValOrErr(variables, "unexpected null second parameter passed to function applyResources.") 
	}

	if dir.Value() == nil {
		klog.Infof("applyResourcesCEL directory is nil")
		return types.ValOrErr(dir, "unexpected null first parameter passed to function applyResources.") 
	}
	klog.Infof("applyResources first param type: %v, second param type: %v", dir.Type(), variables.Type())

	dirStr, ok := dir.Value().(string)
	if !ok {
		klog.Infof("applyResources dir is not string")
		return types.ValOrErr(dir, "unexpected type '%v' passed as first parameter to function applyResources. It should be string", dir.Type())
	}

	err := applyResourcesHelper(triggerProc.triggerDir, dirStr, variables.Value(), true)
	var ret ref.Val
	if err != nil {
		ret = types.String(fmt.Sprintf("applyResources error  applying template %v", err) )
	} else {
		ret = types.String("")
	}
	return ret
}


/* Find files with given suffixes */
func findFiles(resourceDir string, suffixes []string) ([]string, error) {

	ret := make([]string, 0)
	for _, suffix := range suffixes {
		fileNames, err := filepath.Glob(resourceDir + "/" + "*" + suffix)
		if err != nil {
			return nil, err
		}
		for _, fileName := range fileNames {
			if klog.V(6) {
				klog.Infof("findFiles adding: %s", fileName)
			}
			ret = append(ret, fileName)
		}
	}
	return ret, nil
}

func applyResourcesHelper(triggerDirectory string, directory string, variables interface{}, dryrun bool) error {

	resourceDir, err := mergePathWithErrorCheck(triggerDirectory , directory)
	if err != nil {
		return err
	}
	files, err := findFiles(resourceDir , []string{"yaml", "yml"})
	if err != nil {
		return err
	}

	/* ensure all files are substituted OK*/
	substituted := make([] string, 0)
	for _, path := range files {
		after, err := substituteTemplateFile(path, variables)
		if err != nil {
			return err
		}
		substituted = append(substituted, after)
	}

    if dryrun {
		klog.Infof("applyResources: dryrun is set. Resources not created")
    } else {
		/* Apply the files */
		for _, resource:= range substituted {
			if klog.V(5) {
				klog.Infof("applying resource: %s", resource)
			}
			err = createResource(resource, dynamicClient)
			if err != nil {
				return err
			}
		}
	}
	return nil
}


/* implementation of sendEvent
   destination string: where to send the event
   message Any: JSON message
   Return string : empty if OK, otherwise, error message
*/
func sendEventCEL(destination ref.Val, message ref.Val) ref.Val {
	if klog.V(6) {
		klog.Infof("sendEventCEL first param: %v, second param: %v", destination, message)
	}

	if message.Value() == nil {
		klog.Infof("sendEventCEL  message is nil")
		return types.ValOrErr(message, "unexpected null message parameter passed to function sendEvent.") 
	}

	if destination.Value() == nil {
		klog.Infof("sendEventCEL destination is nil")
		return types.ValOrErr(destination, "unexpected null destination parameter passed to function sendEvent.") 
	}
	klog.Infof("sendEventCEL first param type: %v, second param type: %v", destination.Type(), message.Type())

	dest, ok := destination.Value().(string)
	if !ok {
		klog.Infof("sendEvent destination is not string")
		return types.ValOrErr(destination, "unexpected type '%v' passed as destination parameter to function sendEventCEL. It should be string", destination.Type())
	}

	var ret ref.Val
	value := message.Value()
	bytes, err := json.Marshal(value)
	if err != nil {
		klog.Errorf("Unable to marshall as JSON: %v, type %T", value, value)
		ret = types.String(fmt.Sprintf("sendEventCEL error applying sending message: %v", err) )
	} else {
		klog.Infof("in sendEvent message %v marshaled as: %s", value, string(bytes))
		ret = types.String("")
	}

	destNode := eventProviders.GetEventDestination(dest)
	if destNode == nil {
		klog.Errorf("Unable to find an eventDestination with the name '%s'. Verify that it has been defined.", dest)
	}
	provider := eventProviders.GetMessageProvider(destNode.ProviderRef)
	if provider == nil {
		klog.Errorf("Unable to find a messageProvider with the name '%s'. Verify that is has been defined.", destNode.ProviderRef)
	}

	err = provider.Send(destNode, bytes)
	if err != nil {
		klog.Error(err)
	}
	if klog.V(6) {
		klog.Infof("sendEvent successfully sent message to destination '%s'", dest)
	}
	return ret
}

/* implementation of filter
   message: map or array to be filtered
   expression string: expression used to filter each element of the map or array, must return a bool
       For a map, the variables key and value are bound as variables to be used in the epxress
       For an array, the variable value is bound to be used.
   Return map or array with elements filtered
   Example:
        newHader : ' filter(header, " key.startsWith(\"X-Github\") || key.startsWith(\"github\")) '
		newArray: ' filter(oldArray, " value < 10 " )
*/
func filterCEL(message ref.Val, expression ref.Val) ref.Val {
	if klog.V(6) {
		klog.Infof("filterCEL first param: %v, second param: %v", message, expression)
	}

	messageVal := message.Value()
	if messageVal == nil {
		klog.Infof("filterCEL  message is nil")
		return types.ValOrErr(message, "unexpected null message parameter passed to function filter.") 
	}

	expressionVal := expression.Value()
	if expressionVal == nil {
		klog.Infof("filterCEL expression is nil")
		return types.ValOrErr(expression, "unexpected null expression parameter passed to function filter.") 
	}
	if klog.V(6) {
		klog.Infof("filterCEL first param type: %v, %T, second param type: %v, %T",  message.Type(), messageVal, expression.Type(), expressionVal)
	}

	expressionStr, ok := expressionVal.(string)
	if !ok {
		klog.Infof("filter expression %v is not string", expressionVal)
		return types.ValOrErr(expression, "unexpected type '%v' passed as destination parameter to function filter. It should be string", expression.Type())
	}
	
	var err error
	messageValue := reflect.ValueOf(messageVal)
	messageType := messageValue.Type()
	messageKind := messageType.Kind()
	if messageKind != reflect.Map && messageKind != reflect.Array && messageKind != reflect.Slice {
		klog.Errorf("filterCEL value is neither map nor array, but : %v", messageKind)
		return types.ValOrErr(message, "for function filter, parameter message is neither a map nor an array, but %v", messageKind) 
	}
	var retValue interface{}
	if messageKind == reflect.Map {
		retMap := reflect.MakeMap(messageType)
		for iter := messageValue.MapRange(); iter.Next(); {
			key := iter.Key()
			value := iter.Value()
			err = filterMapEntry(retMap, key, value , expressionStr ) 
			if err != nil {
				klog.Errorf("In built-in function filter evaluation of condition %v resulted in error  %v", expressionStr, err) 
				return types.ValOrErr(message, "In built-in function filter evaluation of condition %v resulted in error  %v", expressionStr, err) 
			}
		}
		retValue = retMap.Interface()
	} else {
		/* array or array slice */
		retArray := reflect.MakeSlice(reflect.SliceOf(messageType.Elem()), 0, 0)
		for i := 0; i < messageValue.Len(); i++ {
			value := messageValue.Index(i)
			retArray, err = filterArraySlice(retArray, value , expressionStr ) 
			if err != nil {
				klog.Errorf("In built-in function filter evaluation of condition %v resulted in error  %v", expressionStr, err) 
				return types.ValOrErr(message, "In built-in function filter evaluation of condition %v resulted in error  %v", expressionStr, err) 
			}
		}
		retValue = retArray.Interface()
	}
	ret, err := convertToRefVal(retValue)
	if err != nil {
		klog.Errorf("In built-in function filter, error converting return value of type %T value %v to CEL type. Error: %v", retValue, retValue, err)
		return types.ValOrErr(message, "In built-in function filter,  error converting return value of type %T, value %v to CEL type. Error: %v", retValue, retValue, err)
	}
	return ret
}
	
/* Add key/value into a map if the expression evaluates to true.
  map: map to insert the key/value
  key: key of the entry
  value: value of the entry
  expression: expression to evaluate

  This function first creates 2 variables:
	key: value of the key
	value: value of the "value" variable
 Then then evaluates the expression in the context of the key/value variables.
 If the expression is true, it inserts the ke/value into the map
*/
func filterMapEntry(mapVal reflect.Value, key, value reflect.Value, expression string ) error {
	env, err := initializeEmptyCELEnv() 
	if err != nil {
		return err
	}

	variables := make(map[string]interface{})
	keyName := "key"
	variables[keyName] = key.Interface()
	ident := decls.NewIdent(keyName, decls.String, nil)
	env, err = env.Extend(cel.Declarations(ident))
	if err != nil {
		return  err
	}

	valueName := "value"
	variables[valueName] = value.Interface()
	ident = decls.NewIdent(valueName, decls.Any, nil)
	env, err = env.Extend(cel.Declarations(ident))
	if err != nil {
		return err
	}
	condition, err := evalCondition(env, expression, variables) 
	if err != nil {
		return err
	}

	if condition {
		mapVal.SetMapIndex(key, value)
	}
	return nil
}

var nilValue = reflect.ValueOf(nil)

/* Add value into an array slice if the expression evaluates to true.
  slice: array slice to append the value
  value: value of the entry
  expression: expression to evaluate

  This function first creates 1 variables
	value: value of the array entry
 Then then evaluates the expression in the context of the value variables.
 If the expression is true, it inserts the value into the slice
*/
func filterArraySlice(slice reflect.Value, value reflect.Value, expression string ) (reflect.Value, error) {
	env, err := initializeEmptyCELEnv() 
	if err != nil {
		return nilValue, err
	}

	variables := make(map[string]interface{})

	valueName := "value"
	variables[valueName] = value.Interface()
	ident := decls.NewIdent(valueName, decls.Any, nil)
	env, err = env.Extend(cel.Declarations(ident))
	if err != nil {
		return nilValue, err
	}
	condition, err := evalCondition(env, expression, variables) 
	if err != nil {
		return nilValue, err
	}

	if condition {
		slice = reflect.Append(slice, value)
	}
	return slice, nil
}

/* Get declaration of additional overloaded CEL functions */
func getAdditionalCELFuncDecls() cel.EnvOption{
	return triggerFuncDecls
}


/* Get implemenations of additional overloaded CEL functions */
func getAdditionalCELFuncs() cel.ProgramOption {
	return triggerFuncs
}

var triggerFuncDecls cel.EnvOption
var triggerFuncs cel.ProgramOption

func init() {
	triggerFuncDecls = cel.Declarations (
		decls.NewFunction("filter", 
			decls.NewOverload("filter_any_string", []*exprpb.Type{ decls.Any, decls.String}, decls.Any)),
		decls.NewFunction("call", 
			decls.NewOverload("call_string_any_string", []*exprpb.Type{decls.String, decls.Any}, decls.Any)),
		decls.NewFunction("sendEvent", 
			decls.NewOverload("applyResources_string_any", []*exprpb.Type{decls.String, decls.Any}, decls.String)),
		decls.NewFunction("applyResources", 
			decls.NewOverload("applyResources_string_any", []*exprpb.Type{decls.String, decls.Any}, decls.String)),
		decls.NewFunction("kabaneroConfig", 
			decls.NewOverload("kabaneroConfig", []*exprpb.Type{}, decls.NewMapType(decls.String, decls.Any))),
		decls.NewFunction("jobID", 
			decls.NewOverload("jobID", []*exprpb.Type{}, decls.String)),
		decls.NewFunction("downloadYAML", 
			decls.NewOverload("downloadYAML_map_string", []*exprpb.Type{decls.NewMapType(decls.String, decls.Any), decls.String}, decls.NewMapType(decls.String, decls.Any))),
		decls.NewFunction("toDomainName", 
			decls.NewOverload("toDomainName_string", []*exprpb.Type{decls.String}, decls.String)),
		decls.NewFunction("toLabel", 
			decls.NewOverload("toLabel_string", []*exprpb.Type{decls.String}, decls.String)),
		decls.NewFunction("split",
			decls.NewOverload("split_string", []*exprpb.Type{decls.String, decls.String}, decls.NewListType(decls.String))))

	triggerFuncs = cel.Functions(
		&functions.Overload{
	        Operator: "filter",
	        Binary: filterCEL} ,
		&functions.Overload{
	        Operator: "call",
	        Binary: callCEL} ,
		&functions.Overload{
	        Operator: "sendEvent",
	        Binary: sendEventCEL} ,
		&functions.Overload{
	        Operator: "applyResources",
	        Binary: applyResourcesCEL} ,
		&functions.Overload{
	        Operator: "kabaneroConfig",
	        Function: kabaneroConfigCEL} ,
		&functions.Overload{
	        Operator: "jobID",
	        Function: jobIDCEL} ,
		&functions.Overload{
	        Operator: "downloadYAML",
	        Binary: downloadYAMLCEL} ,
		&functions.Overload{
	        Operator: "toDomainName",
	        Unary: toDomainNameCEL} ,
		&functions.Overload{
	        Operator: "toLabel",
	        Unary: toLabelCEL} ,
		&functions.Overload{
	        Operator: "split",
	        Binary: splitCEL})
}
