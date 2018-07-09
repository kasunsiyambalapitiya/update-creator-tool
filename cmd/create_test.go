// Copyright (c) 2016, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"strings"
	"testing"

	"github.com/wso2/update-creator-tool/constant"
	"github.com/wso2/update-creator-tool/util"
)

func TestGetUpdateName(t *testing.T) {
	updateNumber := "0001"
	kernelVersion := "4.4.0"
	updateDescriptor := util.UpdateDescriptorV2{
		UpdateNumber:    updateNumber,
		PlatformVersion: kernelVersion,
	}
	updateName := getUpdateName(&updateDescriptor, constant.UPDATE_NAME_PREFIX)
	expected := constant.UPDATE_NAME_PREFIX + "-" + kernelVersion + "-" + updateNumber
	if updateName != expected {
		t.Errorf("Test failed, expected: %s, actual: %s", expected, updateName)
	}
}

func TestAddToRootNode(t *testing.T) {
	//Add new file
	isDir := false
	hash := "hash1"
	root := createNewNode()
	AddToRootNode(&root, strings.Split("a/b/c.jar", "/"), isDir, hash)

	nodeName := "a"
	nodeA, exists := root.childNodes[nodeName]
	if !exists {
		t.Errorf("Test failed, node '%v' not found.", nodeName)
	}
	if nodeA.isDir == false {
		t.Errorf("Test failed, expected: %v, actual: %v", false, nodeA.isDir)
	}

	nodeName = "b"
	nodeB, exists := nodeA.childNodes[nodeName]
	if !exists {
		t.Errorf("Test failed, node '%v' not found.", nodeName)
	}
	if nodeB.isDir == false {
		t.Errorf("Test failed, expected: %v, actual: %v", false, nodeB.isDir)
	}

	nodeName = "c.jar"
	nodeC, exists := nodeB.childNodes[nodeName]
	if !exists {
		t.Errorf("Test failed, node '%v' not found.", nodeName)
	}

	if nodeC.md5Hash != hash {
		t.Errorf("Test failed, expected: %v, actual: %v", hash, nodeC.md5Hash)
	}

	if nodeC.isDir != isDir {
		t.Errorf("Test failed, expected: %v, actual: %v", hash, nodeC.md5Hash)
	}

	//Add new file
	isDir = false
	hash = "hash2"
	AddToRootNode(&root, strings.Split("a/b/d.jar", "/"), isDir, hash)
	nodeName = "a"
	nodeA, exists = root.childNodes[nodeName]
	if !exists {
		t.Errorf("Test failed, node '%v' not found.", nodeName)
	}
	if nodeA.isDir == false {
		t.Errorf("Test failed, expected: %v, actual: %v", false, nodeA.isDir)
	}

	nodeName = "b"
	nodeB, exists = nodeA.childNodes[nodeName]
	if !exists {
		t.Errorf("Test failed, node '%v' not found.", nodeName)
	}
	if nodeB.isDir == false {
		t.Errorf("Test failed, expected: %v, actual: %v", false, nodeB.isDir)
	}

	nodeName = "d.jar"
	nodeD, exists := nodeB.childNodes[nodeName]
	if !exists {
		t.Errorf("Test failed, node '%v' not found.", nodeName)
	}

	if nodeD.md5Hash != hash {
		t.Errorf("Test failed, expected: %v, actual: %v", hash, nodeD.md5Hash)
	}

	if nodeD.isDir != isDir {
		t.Errorf("Test failed, expected: %v, actual: %v", hash, nodeD.md5Hash)
	}

}

func TestPathExists(t *testing.T) {
	root := createNewNode()
	AddToRootNode(&root, strings.Split("a/b/c.jar", "/"), false, "hash1")

	exists := PathExists(&root, "a/b/c.jar", false)
	expected := true
	if expected != exists {
		t.Errorf("Test failed, expected: %v, actual: %v", expected, exists)
	}

	exists = PathExists(&root, "a/b", true)
	expected = true
	if expected != exists {
		t.Errorf("Test failed, expected: %v, actual: %v", expected, exists)
	}

	exists = PathExists(&root, "a", true)
	expected = true
	if expected != exists {
		t.Errorf("Test failed, expected: %v, actual: %v", expected, exists)
	}

	exists = PathExists(&root, "a/b/d.jar", false)
	expected = false
	if expected != exists {
		t.Errorf("Test failed, expected: %v, actual: %v", expected, exists)
	}

	AddToRootNode(&root, strings.Split("a/b/d.jar", "/"), false, "hash2")

	exists = PathExists(&root, "a/b/d.jar", false)
	expected = true
	if expected != exists {
		t.Errorf("Test failed, expected: %v, actual: %v", expected, exists)
	}

	exists = PathExists(&root, "a/d.jar", false)
	expected = false
	if expected != exists {
		t.Errorf("Test failed, expected: %v, actual: %v", expected, exists)
	}

	AddToRootNode(&root, strings.Split("a/d.jar", "/"), false, "hash3")

	exists = PathExists(&root, "a/d.jar", false)
	expected = true
	if expected != exists {
		t.Errorf("Test failed, expected: %v, actual: %v", expected, exists)
	}

	exists = PathExists(&root, "d.jar", false)
	expected = false
	if expected != exists {
		t.Errorf("Test failed, expected: %v, actual: %v", expected, exists)
	}

	AddToRootNode(&root, strings.Split("d.jar", "/"), false, "hash3")

	exists = PathExists(&root, "d.jar", false)
	expected = true
	if expected != exists {
		t.Errorf("Test failed, expected: %v, actual: %v", expected, exists)
	}
}
