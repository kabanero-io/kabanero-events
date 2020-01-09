package main

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"text/template"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"gopkg.in/yaml.v2"
)

const (
	TRIGGER0 = "test_data/trigger0"
	TRIGGER1 = "test_data/trigger1"
	TRIGGER2 = "test_data/trigger2"
	TRIGGER3 = "test_data/trigger3"
	TRIGGER4 = "test_data/trigger4"
	TRIGGER5 = "test_data/trigger5"
	TRIGGER6 = "test_data/trigger6"
	TRIGGER7 = "test_data/trigger7"
	TRIGGER8 = "test_data/trigger8"
	TRIGGER9 = "test_data/trigger9"
)

/* Simaple test to read data structure*/
func TestTriggerDefinition(t *testing.T) {
	var trigger = `
initialize:
  - name: name1
    value: value1
  - name: name2
    value: value2
`
	mapObj := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(trigger), mapObj)
	if err != nil {
		t.Fatal(err)
	}
}

/* Test reading trigger yaml files withou errors*/
func TestReadYaml(t *testing.T) {
	var files = []string{
		TRIGGER0 + "/trigger0.yaml",
	}

	for _, fileName := range files {
		td := &eventTriggerDefinition{
			setting:       []map[interface{}]interface{}{},
			eventTriggers: make(map[string][]map[interface{}]interface{}),
			functions:     make(map[string]map[interface{}]interface{}),
		}
		err := readTriggerDefinition(fileName, td)
		if err != nil {
			t.Fatal(err)
			break
		}
	}
}

/* This is not so much a unit test as an experiment to get CEL to use map[string]interface{] as variable definition*/
func TestCEL(t *testing.T) {

	ident1 := decls.NewIdent("name", decls.String, nil)
	ident2 := decls.NewIdent("group", decls.String, nil)
	ident3 := decls.NewIdent("message", decls.NewMapType(decls.String, decls.Any), nil)
	ident4 := decls.NewIdent("orgs", decls.NewListType(decls.Any), nil)
	env, err := cel.NewEnv(cel.Declarations(ident1))
	env, err = env.Extend(cel.Declarations(ident2))
	env, err = env.Extend(cel.Declarations(ident3))
	env, err = env.Extend(cel.Declarations(ident4))
	/*
	   			decls.NewIdent("name", decls.String, nil),
	   			decls.NewIdent("group", decls.String, nil),
	               decls.NewIdent("message", decls.NewMapType(decls.String, decls.Any), nil)))
	*/

	parsed, issues := env.Parse(`message.attr1 in orgs`)
	if issues != nil && issues.Err() != nil {
		t.Fatal(fmt.Errorf("parse error: %s", issues.Err()))
	}
	checked, issues := env.Check(parsed)
	if issues != nil && issues.Err() != nil {
		t.Fatal(fmt.Errorf("type-check error: %s", issues.Err()))
	}
	prg, err := env.Program(checked)
	if err != nil {
		t.Fatal(fmt.Errorf("program construction error: %s", err))
	}

	map0 := make(map[string]interface{})
	map0["attr1"] = "val2"
	map0["attr2"] = []string{"str1", "str2"}

	map1 := make(map[string]interface{})
	map1["attr3"] = "val3"

	map0["map1"] = map1

	var orgs = []string{"org1", "org2", "val2"}
	out, details, err := prg.Eval(map[string]interface{}{
		"name":    "/groups/acme.co/documents/secret-stuff",
		"group":   "acme.co",
		"message": map0,
		"orgs":    orgs})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf(" out: %s, details: %s\n", out, details) // 'true'
}

/* This is an experiment to unmarshal JSON and check the types of objects */
func TestJSON(t *testing.T) {

	srcJSON := []byte(`{"float": 1.2, "int": 1, "bool": true,  "array":["apple", 2]}`)
	var m map[string]interface{}
	err := json.Unmarshal(srcJSON, &m)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%T, %T, %T, %T, %T %T\n", m["float"], m["int"], m["bool"], m["array"], m["array"].([]interface{})[0], m["array"].([]interface{})[1])
}

/* Test applying template using map[string]interface{}*/
func TestGoTemplate(t *testing.T) {
	srcJSON := []byte(`{"floatAttr": 1.2, "intAttr": 1, "boolAttr": true,  "arrayAttr":["apple", "orange"] }`)
	var variables map[string]interface{}
	err := json.Unmarshal(srcJSON, &variables)
	if err != nil {
		t.Fatal(err)
	}

	srcTemplate := `attr1: {{.floatAttr}}; attr2: {{.intAttr}}; attr3: {{.boolAttr}};  attr4: {{.arrayAttr}}  attr4[0]: {{index .arrayAttr 0}}`
	var temp *template.Template
	temp, err = template.New("TestTemplate").Parse(srcTemplate)
	if err != nil {
		t.Fatal(err)
	}

	err = temp.Execute(os.Stdout, variables)
	if err != nil {
		t.Fatal(err)
	}
}

var testTemplate = `
----- Event Variables ----
{{.event.stringAttr}}
{{.event.floatAttr}}
{{.event.intAttr}}
{{.event.boolAttr}}
{{.event.arrayAttr}}
{{index .event.arrayAttr 0}}
{{index .event.arrayAttr 1}}
{{.event.objectAttr}}
{{.event.objectAttr.innerFloatAttr}}
{{.event.objectAttr.innerStringAttr}}
----- Other Variables ----
{{.int64Attr}}
{{.float64Attr}}
{{.boolAttr}}
{{.arrayStringAttr}}
{{.arrayIntAttr}}
{{.eventIntAttr}}
{{.eventFloatAttr}}
{{.eventBoolAttr}}
{{.eventArrayElementAttr}}
{{.eventArrayAttr}}
{{.eventObjectAttr}}
{{.eventObjectInnerFloatAttr}}
{{.mathAttr1}}
{{.mathAttr2}}
{{.listAttrWithVariables}}
{{.ifThenElseStringAttr}}
----- Reuse Variables ----
{{.reuseInt64Attr}}
{{.reuseFloat64Attr}}
{{.reuseboolAttr}}
{{.reusearrayStringAttr}}
{{.reuseArrayIntAttr}}
{{.reuseEventIntAttr}}
{{.reuseEventFloatAttr}}
{{.reuseEventBoolAttr}}
{{.reuseEventArrayElementAttr}}
{{.reuseEventArrayAttr}}
{{.reuseEventObjectAttr}}
{{.reuseEventObjectInnerFloatAttr}}
{{.reuseMathAttr1}}
{{.reuseMathAttr2}}
{{.reuseListAttrWithVariables}}
{{.reuseIfThenElseStringAttr}}
`

var result = `
----- Event Variables ----
string1
1.2
100
true
[apple orange]
apple
orange
map[innerFloatAttr:1.2 innerStringAttr:inner string]
1.2
inner string
----- Other Variables ----
1
1.2
true
[abc def ghi]
[1 2 3]
100
1.2
true
apple
[apple orange]
map[innerFloatAttr:1.2 innerStringAttr:inner string]
1.2
102
3.2
[abc string1]
got-string-1
----- Reuse Variables ----
1
1.2
true
[abc def ghi]
[1 2 3]
100
1.2
true
apple
[apple orange]
map[innerFloatAttr:1.2 innerStringAttr:inner string]
1.2
102
3.2
[abc string1]
got-string-1
`

/* Test applying template using CEL variables */
func TestApplyTemplateWithCELVariables(t *testing.T) {
	srcEvent := []byte(`{"stringAttr": "string1", "floatAttr": 1.2, "intAttr": 100, "boolAttr": true,  "arrayAttr":["apple", "orange"], "objectAttr": { "innerFloatAttr": 1.2, "innerStringAttr": "inner string"} } `)
	var event map[string]interface{}
	err := json.Unmarshal(srcEvent, &event)
	if err != nil {
		t.Fatal(err)
	}

	tp := newTriggerProcessor()
	err = tp.initialize(TRIGGER1)
	if err != nil {
		t.Fatal(err)
	}

	variables, err := tp.processMessage(event, "default")
	if err != nil {
		t.Fatal(err)
	}

	afterSubstitution, err2 := substituteTemplate(testTemplate, variables[0])
	if err2 != nil {
		t.Fatal(err2)
	}
	fmt.Printf("after substitution: %s\n", afterSubstitution)

	if result != afterSubstitution {
		t.Fatal("template substitution is not as expected.")
	}
}

func TestConditionalVariable(t *testing.T) {
	srcEvents := [][]byte{
		[]byte(`{"attr1": "string1", "attr2": "string2"}`),
		[]byte(`{"attr1": "string1a", "attr2": "string2"}`),
		[]byte(`{"attr1": "string1", "attr2": "string2a"}`),
		[]byte(`{"attr1": "string1a", "attr2": "string2a"}`),
	}

	/*
		triggerDef, err := readTriggerDefinition(TRIGGER3)
		if err != nil {
			t.Fatal(err)
		}
	*/
	tp := newTriggerProcessor()
	err := tp.initialize(TRIGGER3)
	if err != nil {
		t.Fatal(err)
	}

	expectedDirs := []string{
		"string1string2", "notstring1string2", "string1notstring2", "notstring1notstring2",
	}

	for index, srcBytes := range srcEvents {
		if err = testOneConditional(tp, srcBytes, expectedDirs[index]); err != nil {
			t.Fatal(err)
		}
	}
}

func testOneConditional(tp *triggerProcessor, srcEvent []byte, expectedDirectory string) error {
	var event map[string]interface{}
	err := json.Unmarshal(srcEvent, &event)
	if err != nil {
		return err
	}

	variablesArray, err := tp.processMessage(event, "default")
	if err != nil {
		return err
	}

	variables := variablesArray[0]
	dir, ok := variables["directory"]
	if !ok {
		return fmt.Errorf("unable to locate variable directory")
	}
	if dir != expectedDirectory {
		return fmt.Errorf("directory value %v is not expected value %s", dir, expectedDirectory)
	}

	return nil
}

//func TestFindTrigger(t *testing.T) {
//	srcEvents := [][]byte {
//		[]byte(`{ "event" : {"attr1": "string1", "attr2": "string2"}}`),
//		[]byte(`{ "event" : {"attr1": "string1a", "attr2": "string2"}}`),
//		[]byte(`{ "event" : {"attr1": "string1", "attr2": "string2a"}}`),
//		[]byte(`{ "event" : {"attr1": "string1a", "attr2": "string2a"}}`),
//	}
//
//
//	triggerDef, err := readTriggerDefinition(TRIGGER2)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	expectedDirs := []string{
//		"string1string2", "notstring1string2", "string1notstring2", "notstring1notstring2",
//	}
//
//	for index, srcBytes := range srcEvents {
//		if err = testEvent(triggerDef, srcBytes, expectedDirs[index]); err != nil {
//			t.Fatal(err)
//		}
//	}
//}
//
//func testEvent(triggerDef *EventTriggerDefinition, srcEvent []byte, expectedDirectory string) error {
//	var event map[string]interface{}
//	err := json.Unmarshal(srcEvent, &event)
//	if err != nil {
//		return err
//	}
//	env, variables, _, err := initializeCELEnv(triggerDef, event)
//	if err != nil {
//		return err
//	}
//	action, err := findTrigger(env, triggerDef, variables)
//	if err != nil {
//		return err
//	}
//
//	if action == nil {
//		return fmt.Errorf("in testEvent, no action found")
//	}
//	if action.ApplyResources.Directory != expectedDirectory {
//		return fmt.Errorf("Expecting directory %s but got %s", expectedDirectory, action.ApplyResources.Directory)
//	}
//	return nil
//}

func TestTimestamp(t *testing.T) {
	next := ""
	last := getTimestamp()
	for i := 0; i < 20; i++ {
		next = getTimestamp()
		if next == last {
			t.Errorf("Some consecutive timestamps have the same value: %s %s", next, last)
		}
		last = next
	}
	fmt.Printf("Last timestamp is : %s\n", last)
}

func TestAdditionalfunctions(t *testing.T) {

	/*
		triggerDef, err := readTriggerDefinition(TRIGGER4)
		if err != nil {
			t.Error(err)
		}
	*/

	tp := newTriggerProcessor()
	err := tp.initialize(TRIGGER4)
	if err != nil {
		t.Fatal(err)
	}

	event := make(map[string]interface{})
	variablesArray, err := tp.processMessage(event, "default")
	if err != nil {
		t.Fatal(err)
	}

	variables := variablesArray[0]
	str1, ok := variables["str1"]
	if !ok {
		t.Errorf("Unable to locate variable str1")
	}
	expectedStr1 := "abc.def"
	if str1 != expectedStr1 {
		t.Errorf("str1 value %v is not expected value %v", str1, expectedStr1)
	}

	str2Obj, ok := variables["str2"]
	if !ok {
		t.Errorf("Unable to locate variable str2")
	}
	fmt.Printf("after split, type: %t,  value: %v", str2Obj, str2Obj)
	str2, ok := str2Obj.([]string)
	if !ok {
		t.Errorf("str2 %v is not []string", str2)
	}

	str3, ok := variables["str3"]
	if !ok {
		t.Errorf("Unable to locate variable str3")
	}
	expectedStr3 := "b"
	if str3 != expectedStr3 {
		t.Errorf("str3 value %v is not expected value %v", str3, expectedStr3)
	}

}

var testNestedVariableTemplate = `
----- Event Variables ----
{{.event.stringAttr}}
{{.event.floatAttr}}
{{.event.intAttr}}
{{.event.boolAttr}}
{{.event.arrayAttr}}
{{index .event.arrayAttr 0}}
{{index .event.arrayAttr 1}}
{{.event.objectAttr}}
{{.event.objectAttr.innerFloatAttr}}
{{.event.objectAttr.innerStringAttr}}
----- Other Variables ----
{{.nested.int64Attr}}
{{.nested.float64Attr}}
{{.nested.boolAttr}}
{{.nested.arrayStringAttr}}
{{.nested.arrayIntAttr}}
{{.nested.eventIntAttr}}
{{.nested.eventFloatAttr}}
{{.nested.eventBoolAttr}}
{{.nested.eventArrayElementAttr}}
{{.nested.eventArrayAttr}}
{{.nested.eventObjectAttr}}
{{.nested.eventObjectInnerFloatAttr}}
{{.nested.mathAttr1}}
{{.nested.mathAttr2}}
{{.nested.listAttrWithVariables}}
{{.nested.ifThenElseStringAttr}}
----- Reuse Variables ----
{{.nested.reuseInt64Attr}}
{{.nested.reuseFloat64Attr}}
{{.nested.reuseboolAttr}}
{{.nested.reusearrayStringAttr}}
{{.nested.reuseArrayIntAttr}}
{{.nested.reuseEventIntAttr}}
{{.nested.reuseEventFloatAttr}}
{{.nested.reuseEventBoolAttr}}
{{.nested.reuseEventArrayElementAttr}}
{{.nested.reuseEventArrayAttr}}
{{.nested.reuseEventObjectAttr}}
{{.nested.reuseEventObjectInnerFloatAttr}}
{{.nested.reuseMathAttr1}}
{{.nested.reuseMathAttr2}}
{{.nested.reuseListAttrWithVariables}}
{{.nested.reuseIfThenElseStringAttr}}
----- Double Nested Variables ----
{{.nested.nested1.int64Attr}}
{{.nested.nested1.reuseInt64Attr}}
`

var nestedVariableResult = `
----- Event Variables ----
string1
1.2
100
true
[apple orange]
apple
orange
map[innerFloatAttr:1.2 innerStringAttr:inner string]
1.2
inner string
----- Other Variables ----
1
1.2
true
[abc def ghi]
[1 2 3]
100
1.2
true
apple
[apple orange]
map[innerFloatAttr:1.2 innerStringAttr:inner string]
1.2
102
3.2
[abc string1]
got-string-1
----- Reuse Variables ----
1
1.2
true
[abc def ghi]
[1 2 3]
100
1.2
true
apple
[apple orange]
map[innerFloatAttr:1.2 innerStringAttr:inner string]
1.2
102
3.2
[abc string1]
got-string-1
----- Double Nested Variables ----
10
10
`

/* Test double nested variables */
func TestDoubleNestedCELVariables(t *testing.T) {
	srcEvent := []byte(` {"stringAttr": "string1", "floatAttr": 1.2, "intAttr": 100, "boolAttr": true,  "arrayAttr":["apple", "orange"], "objectAttr": { "innerFloatAttr": 1.2, "innerStringAttr": "inner string"} }`)
	var event map[string]interface{}
	err := json.Unmarshal(srcEvent, &event)
	if err != nil {
		t.Fatal(err)
	}

	/*
		triggerDef, err := readTriggerDefinition(TRIGGER5)
		if err != nil {
			t.Fatal(err)
		}
		_, variables, _, err1 := initializeCELEnv(triggerDef, event)
		if err1 != nil {
			t.Fatal(err1)
		}

	*/
	tp := newTriggerProcessor()
	err = tp.initialize(TRIGGER5)
	if err != nil {
		t.Fatal(err)
	}
	variablesArray, err := tp.processMessage(event, "default")
	if err != nil {
		t.Fatal(err)
	}

	variables := variablesArray[0]
	afterSubstitution, err2 := substituteTemplate(testNestedVariableTemplate, variables)
	if err2 != nil {
		t.Fatal(err2)
	}
	fmt.Printf("after substitution: %s\n", afterSubstitution)

	if nestedVariableResult != afterSubstitution {
		t.Fatalf("template substitution is not as expected: Expecting: '%s', but received: '%s'", nestedVariableResult, afterSubstitution)
	}
}

func TestSwitch(t *testing.T) {
	srcEvents := [][]byte{
		[]byte(`{"attr1": "string1", "attr2": "string2"}`),
		[]byte(`{"attr1": "string1a", "attr2": "string2"}`),
		[]byte(`{"attr1": "string1", "attr2": "string2a"}`),
		[]byte(`{"attr1": "string1a", "attr2": "string2a"}`),
	}

	tp := newTriggerProcessor()
	err := tp.initialize(TRIGGER6)
	if err != nil {
		t.Fatal(err)
	}

	expectedDirs := []string{
		"string1string2", "notstring1string2", "string1notstring2", "notstring1notstring2",
	}

	for index, srcBytes := range srcEvents {
		if err = testOneSwitch(tp, srcBytes, expectedDirs[index]); err != nil {
			t.Fatal(err)
		}
	}
}

func testOneSwitch(tp *triggerProcessor, srcEvent []byte, expectedDirectory string) error {
	var event map[string]interface{}
	err := json.Unmarshal(srcEvent, &event)
	if err != nil {
		return err
	}

	variablesArray, err := tp.processMessage(event, "default")
	if err != nil {
		return err
	}

	variables := variablesArray[0]
	dir, ok := variables["directory"]
	if !ok {
		return fmt.Errorf("unable to locate variable directory")
	}
	if dir != expectedDirectory {
		return fmt.Errorf("directory value %v is not expected value %s", dir, expectedDirectory)
	}

	return nil
}

func TestCall(t *testing.T) {
	srcEvent := []byte(`{"stringAttr": "string1", "floatAttr": 1.2, "intAttr": 100, "boolAttr": true,  "arrayAttr":["apple", "orange"], "objectAttr": { "innerFloatAttr": 1.2, "innerStringAttr": "inner string"} } `)
	var event map[string]interface{}
	err := json.Unmarshal(srcEvent, &event)
	if err != nil {
		t.Fatal(err)
	}

	// Need to set global triggerProc variable
	triggerProc = newTriggerProcessor()
	err = triggerProc.initialize(TRIGGER7)
	if err != nil {
		t.Fatal(err)
	}

	variables, err := triggerProc.processMessage(event, "default")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Variables: %v", variables)

	/* compare nested with temp.result */
	temp := variables[0]["temp"].(map[string]interface{})
	err = checkFinal(temp)
	if err != nil {
		t.Fatal(err)
	}
}

func checkFinal(variables map[string]interface{}) error {
	finalObj, ok := variables["final"]
	if !ok {
		return fmt.Errorf("variable final not found. Variables: %v", variables)
	}
	finalBool, ok := finalObj.(bool)
	if !ok {
		return fmt.Errorf("variable final %v not type bool, but type %T", finalObj, finalObj)
	}

	if !finalBool {
		return fmt.Errorf("variable final not set to true. Variables: %v", variables)
	}
	return nil
}

func TestRecursiveCall(t *testing.T) {
	srcEvent := []byte(`{"stringAttr": "string1", "floatAttr": 1.2, "intAttr": 100, "boolAttr": true,  "arrayAttr":["apple", "orange"], "objectAttr": { "innerFloatAttr": 1.2, "innerStringAttr": "inner string"} } `)
	var event map[string]interface{}
	err := json.Unmarshal(srcEvent, &event)
	if err != nil {
		t.Fatal(err)
	}

	// Need to set global triggerProc variable
	triggerProc = newTriggerProcessor()
	err = triggerProc.initialize(TRIGGER8)
	if err != nil {
		t.Fatal(err)
	}

	variables, err := triggerProc.processMessage(event, "default")
	if err != nil {
		t.Fatal(err)
	}
	err = checkFinal(variables[0])
	if err != nil {
		t.Fatal(err)
	}
}

func TestFilter(t *testing.T) {
	srcEvent := []byte(`{ "Connection": ["close"], "X-Forwarded-For": ["169.60.70.162"], "Content-Length": [23808], ` +
		` "Content-Type": [ "application/json" ], "X-Github-Delivery" : [ "14571b40-f72e-11e9-9252-a0ce3bc96ef7" ],` +
		` "X-Github-Event" : [ "pull_request" ], "X-Github-Enterprise-host" : [ "github.ibm.com" ],` +
		` "X-Github-Enterprise-version" : ["2.16.16" ], "User-Agent" : [ "GitHub-Hookshot/632ecda" ],` +
		` "Accept": [ "*/*" ], "Host" : [ "webhook.site" ]} `)
	var event map[string]interface{}
	err := json.Unmarshal(srcEvent, &event)
	if err != nil {
		t.Fatalf("Unable to parse srrc: %v", err)
	}

	// Need to set global triggerProc variable
	triggerProc = newTriggerProcessor()
	err = triggerProc.initialize(TRIGGER9)
	if err != nil {
		t.Fatal(err)
	}

	variables, err := triggerProc.processMessage(event, "default")
	if err != nil {
		t.Fatal(err)
	}

	err = checkFinal(variables[0])
	if err != nil {
		t.Fatal(err)
	}
}
