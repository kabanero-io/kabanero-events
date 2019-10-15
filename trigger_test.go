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
	TRIGGER0 = "test_data/trigger0.yaml"
	TRIGGER1 = "test_data/trigger1.yaml"
	TRIGGER2 = "test_data/trigger2.yaml"
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
	var triggerDef TriggerDefinition
	err := yaml.Unmarshal([]byte(trigger), &triggerDef)
	if err != nil {
		t.Fatal(err)
	}
}

/* Test reading trigger yaml files withou errors*/
func TestReadYaml(t *testing.T) {
	var files = []string{
		TRIGGER0,
	}

	for _, fileName := range files {
		triggerDef, err := readTriggerDefinition(fileName)
		if err != nil {
			t.Fatal(err)
			break
		}
		fmt.Print(triggerDef)
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

	src_json := []byte(`{"float": 1.2, "int": 1, "bool": true,  "array":["apple", 2]}`)
	var m map[string]interface{}
	err := json.Unmarshal(src_json, &m)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%T, %T, %T, %T, %T %T\n", m["float"], m["int"], m["bool"], m["array"], m["array"].([]interface{})[0], m["array"].([]interface{})[1])
}

/* Test applying template using map[string]interface{}*/
func TestGoTemplate(t *testing.T) {
	src_json := []byte(`{"floatAttr": 1.2, "intAttr": 1, "boolAttr": true,  "arrayAttr":["apple", "orange"] }`)
	var variables map[string]interface{}
	err := json.Unmarshal(src_json, &variables)
	if err != nil {
		t.Fatal(err)
	}

	src_template := `attr1: {{.floatAttr}}; attr2: {{.intAttr}}; attr3: {{.boolAttr}};  attr4: {{.arrayAttr}}  attr4[0]: {{index .arrayAttr 0}}`
	var temp *template.Template
	temp, err = template.New("TestTemplate").Parse(src_template)
	if err != nil {
		t.Fatal(err)
	}

	err = temp.Execute(os.Stdout, variables)
	if err != nil {
		t.Fatal(err)
	}
}

var testTemplate string =
`
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

var result string =
`
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
	src_event := []byte(`{"stringAttr": "string1", "floatAttr": 1.2, "intAttr": 100, "boolAttr": true,  "arrayAttr":["apple", "orange"], "objectAttr": { "innerFloatAttr": 1.2, "innerStringAttr": "inner string"} } `)
	var event map[string]interface{}
	err := json.Unmarshal(src_event, &event)
	if err != nil {
		t.Fatal(err)
	}

	triggerDef, err := readTriggerDefinition(TRIGGER1)
	if err != nil {
		t.Fatal(err)
	}
	_, variables, err1 := initializeCelEnv(triggerDef, event)
	if err1 != nil {
		t.Fatal(err1)
	}

	afterSubstitution, err2 := substituteTemplate(testTemplate, variables)
    if err2 != nil {
         t.Fatal(err2)
	}
	fmt.Printf("after substitution: %s\n", afterSubstitution)

	if result != afterSubstitution {
        t.Fatal("template substitution is not as expected.")
	}
}

func TestFindTrigger(t *testing.T) {
	src_events := [][]byte {
	    []byte(`{"attr1": "string1", "attr2": "string2"}`),
	    []byte(`{"attr1": "string1a", "attr2": "string2"}`),
	    []byte(`{"attr1": "string1", "attr2": "string2a"}`),
		[]byte(`{"attr1": "string1a", "attr2": "string2a"}`),
	}

	triggerDef, err := readTriggerDefinition(TRIGGER2)
	if err != nil {
		t.Fatal(err)
	}

	expectedDirs := []string {
		    "string1string2", "notstring1string2", "string1notstring2", "notstring1notstring2",
	}

	for index, src_bytes := range src_events{
		if err = testEvent(triggerDef, src_bytes, expectedDirs[index]); err != nil {
			t.Fatal(err)
		}
	}
}

func testEvent( triggerDef *TriggerDefinition, src_event []byte, expectedDirectory string) error {
	var event map[string]interface{}
	err := json.Unmarshal(src_event, &event)
	if err != nil {
		return err
	}
	env, variables, err := initializeCelEnv(triggerDef, event)
	if err != nil {
		return err
	}
    action, err := findTrigger(env, triggerDef, variables);
	if err != nil {
        return err
	}
    if action.ApplyResources.Directory != expectedDirectory {
        return fmt.Errorf("Expecting directory %s but got %s", expectedDirectory, action.ApplyResources.Directory)
	} 
	return nil
}