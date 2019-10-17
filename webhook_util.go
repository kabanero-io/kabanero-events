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
	"io/ioutil"
	"fmt"
	"io"
	"net/http"
	"k8s.io/klog"
	"os"
	"path/filepath"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"gopkg.in/yaml.v2"
	"strings"
	"archive/tar"
	"compress/gzip"
)

/* constants*/
const (
	TRIGGERS = "triggers"
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

func getHTTPURLReaderCloser(url string) (io.ReadCloser, error) {

	client := http.Client{}
	response, err := client.Get(url)
	if err != nil {
		return nil, err
	}

	if response.StatusCode == http.StatusOK {
	    return response.Body, nil
	} 
    return nil, fmt.Errorf("Unable to read from url %s, http status: %s", url, response.Status)
}

/* Read remote file from URL and return bytes */
func readHTTPURL(url string) ([]byte, error) {
	readCloser, err := getHTTPURLReaderCloser(url) 
	if err != nil {
		return nil, err
	}
	defer readCloser.Close()
	bytes, err := ioutil.ReadAll(readCloser)
	return bytes, err
}

func unmarshallKabaneroIndex(bytes []byte) (map[string]interface{}, error) {
	var myMap map[string]interface{}
	err := yaml.Unmarshal(bytes, &myMap)
	if err != nil {
		return nil, err
	} 
	return myMap, nil
}

/* Get the URL of where the trigger is stored*/
func getTriggerURL(collection map[string]interface{}) (string, error) {
	triggersObj, ok := collection[TRIGGERS]
    if !ok{
		return "", fmt.Errorf("collection does not contain triggers: section")
	}
	triggersArray, ok := triggersObj.([]interface{})
	if !ok {
		return "", fmt.Errorf("collection does not contain triggers section is not an Arry")
	}
	var retURL = ""
	for index, arrayElement := range triggersArray {
		mapObj, ok := arrayElement.(map[interface{}]interface{})
		if !ok {
			return "", fmt.Errorf("triggers section at index %d is not properly formed", index)
		}
		urlObj, ok := mapObj[URL]
		if !ok {
			return "", fmt.Errorf("triggers section at index %d is not contain url", index)
		}
		url, ok := urlObj.(string)
		if !ok {
			return "", fmt.Errorf("triggers section at index %d url is not a string: %s",index, url)
		}
		retURL = url
	}

	if retURL == "" {
        return "", fmt.Errorf("Unable to find url from triggers section")
	}
	return retURL, nil
}


/* Merage a directory path with a relative path. Return error if the rectory not a prefix of the merged path after the merge  */
func mergePathWithErrorCheck(dir string, toMerge string) (string, error) {
	dest := filepath.Join(dir, toMerge)
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	dest, err = filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(dest, dir) {
		return dest, nil
	}
	return dest, fmt.Errorf("Unable to merge directory %s with %s, The merged directory %s is not in a subdirectory", dir, toMerge, dest)
}

/* gunzip and then untar into a directory */
func gUnzipUnTar(readCloser io.ReadCloser, dir string)  error {
	defer readCloser.Close()

	gzReader, err := gzip.NewReader(readCloser)
	if err != nil {
		return err
	}
	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()

		if err == io.EOF {
            break
		}

		if err != nil {
			return err
		}

		if header == nil {
			continue
		}
		dest, err := mergePathWithErrorCheck(dir, header.Name)
		if err != nil {
			return err
		}
		fileInfo := header.FileInfo()
		mode := fileInfo.Mode();
        if mode.IsRegular() {
			fileToCreate, err := os.OpenFile(dest, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("Unable to create file %s, error: %s", dest, err)
			}
			_, err = io.Copy(fileToCreate, tarReader)
			closeErr := fileToCreate.Close()
			if err != nil {
				return fmt.Errorf("Unable to read file %s, error: %s", dest, err)
			}
			if closeErr != nil {
				return fmt.Errorf("Unable to close file %s, error: %s", dest, closeErr)
			}
		} else if mode.IsDir() {
			err = os.MkdirAll(dest, 0755)
			if  err != nil {
				return fmt.Errorf("Unable to make directory %s, error:  %s", dest, err)
			}
			klog.Infof("Created subdirectory %s\n", dest)
		} else {
			return fmt.Errorf("unsupported file type within tar archive: file within tar: %s, fiele type: %v",header.Name, mode)
		}	

	}
	return nil
}


/* Download the trigger.tar.gz and unpack into the directory
 kabaneroIndexUrl: URL that serves kabanero-index.yaml
 dir: directory to unpack the trigger.tar.gz
*/
func downloadTrigger(kabaneroIndexURL string, dir string ) error {
   kabaneroIndexBytes, err :=  readHTTPURL(kabaneroIndexURL) 
   if err != nil {
	   return err
   }
   kabaneroIndexMap, err := unmarshallKabaneroIndex(kabaneroIndexBytes)
   if err != nil {
	   return err
   }
   triggerURL, err := getTriggerURL(kabaneroIndexMap) 
   if err != nil {
	   return err
   }
   triggerReadCloser , err := getHTTPURLReaderCloser(triggerURL )
   if err != nil {
	   return err
   }

   err = gUnzipUnTar(triggerReadCloser, dir)
   return err
}