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
	"bufio"
/*
	"fmt"
*/
	"os"
/*
	"strings"
	"testing"
*/

/*
	openapi_v2 "github.com/googleapis/gnostic/OpenAPIv2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
*/
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
/*
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	restClient "k8s.io/client-go/rest"
	"k8s.io/klog"
*/
)

func readFile(fileName string) ([]byte, error) {
	ret := make([]byte, 0)
	file, err := os.Open(fileName)
	if err != nil {
		return ret, err
	}
	defer file.Close()
	input := bufio.NewScanner(file)
	for input.Scan() {
		for _, b := range input.Bytes() {
			ret = append(ret, b)
		}
	}
	return ret, nil
}

func readJSON(fileName string) (*unstructured.Unstructured, error) {
	bytes, err := readFile(fileName)
	if err != nil {
		return nil, err
	}
	var unstructuredObj = &unstructured.Unstructured{}
	err = unstructuredObj.UnmarshalJSON(bytes)
	if err != nil {
		return nil, err
	}
	return unstructuredObj, nil
}

