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

package util

import (
	"testing"

	"github.com/wso2/update-creator-tool/constant"
)

func TestProcessUserPreferenceScenario01(t *testing.T) {
	preference := ProcessUserPreference("yes")
	if preference != constant.YES {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.YES, preference)
	}
	preference = ProcessUserPreference("Yes")
	if preference != constant.YES {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.YES, preference)
	}
	preference = ProcessUserPreference("YES")
	if preference != constant.YES {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.YES, preference)
	}
	preference = ProcessUserPreference("y")
	if preference != constant.YES {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.YES, preference)
	}
	preference = ProcessUserPreference("Y")
	if preference != constant.YES {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.YES, preference)
	}
}

func TestProcessUserPreferenceScenario02(t *testing.T) {
	preference := ProcessUserPreference("no")
	if preference != constant.NO {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.NO, preference)
	}
	preference = ProcessUserPreference("No")
	if preference != constant.NO {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.NO, preference)
	}
	preference = ProcessUserPreference("NO")
	if preference != constant.NO {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.NO, preference)
	}
	preference = ProcessUserPreference("n")
	if preference != constant.NO {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.NO, preference)
	}
	preference = ProcessUserPreference("N")
	if preference != constant.NO {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.NO, preference)
	}
}

func TestProcessUserPreferenceScenario03(t *testing.T) {
	preference := ProcessUserPreference("reenter")
	if preference != constant.REENTER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.REENTER, preference)
	}
	preference = ProcessUserPreference("re-enter")
	if preference != constant.REENTER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.REENTER, preference)
	}
	preference = ProcessUserPreference("Reenter")
	if preference != constant.REENTER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.REENTER, preference)
	}
	preference = ProcessUserPreference("Re-enter")
	if preference != constant.REENTER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.REENTER, preference)
	}
	preference = ProcessUserPreference("REENTER")
	if preference != constant.REENTER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.REENTER, preference)
	}
	preference = ProcessUserPreference("RE-ENTER")
	if preference != constant.REENTER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.REENTER, preference)
	}
	preference = ProcessUserPreference("r")
	if preference != constant.REENTER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.REENTER, preference)
	}
	preference = ProcessUserPreference("R")
	if preference != constant.REENTER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.REENTER, preference)
	}
}

func TestProcessUserPreferenceScenario04(t *testing.T) {
	preference := ProcessUserPreference("ya")
	if preference != constant.OTHER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.OTHER, preference)
	}
	preference = ProcessUserPreference("nope")
	if preference != constant.OTHER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.OTHER, preference)
	}
	preference = ProcessUserPreference("re")
	if preference != constant.OTHER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.OTHER, preference)
	}
	preference = ProcessUserPreference("random")
	if preference != constant.OTHER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.OTHER, preference)
	}
	preference = ProcessUserPreference("1234")
	if preference != constant.OTHER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.OTHER, preference)
	}
	preference = ProcessUserPreference("abc")
	if preference != constant.OTHER {
		t.Errorf("Test failed, expected: %d, actual: %d", constant.OTHER, preference)
	}
}

func TestIsUserPreferencesValid(t *testing.T) {
	preferences := []string{"3", "1", "2"}
	isValid, err := IsUserPreferencesValid(preferences, len(preferences))
	if err != nil {
		t.Errorf("Test failed. Unexpected error: %v", err)
	}
	if !isValid {
		t.Errorf("Test failed, expected: %v, actual: %v", true, isValid)
	}

	preferences = []string{"0"}
	isValid, err = IsUserPreferencesValid(preferences, len(preferences))
	if err != nil {
		t.Errorf("Test failed. Unexpected error: %v", err)
	}
	if !isValid {
		t.Errorf("Test failed, expected: %v, actual: %v", true, isValid)
	}

	preferences = []string{"-1"}
	isValid, err = IsUserPreferencesValid(preferences, len(preferences))
	if err == nil {
		t.Error("Test failed. Error expected")
	}
	if isValid {
		t.Errorf("Test failed, expected: %v, actual: %v", false, isValid)
	}

	preferences = []string{"10"}
	isValid, err = IsUserPreferencesValid(preferences, len(preferences))
	if err == nil {
		t.Error("Test failed. Error expected")
	}
	if isValid {
		t.Errorf("Test failed, expected: %v, actual: %v", false, isValid)
	}
}

func TestValidateUpdateDescriptor(t *testing.T) {
	updateDescriptor := UpdateDescriptorV2{}
	err := ValidateUpdateDescriptor(&updateDescriptor)
	if err == nil {
		t.Error("Test failed. Error expected")
	}

	updateDescriptor.Update_number = "0001"
	err = ValidateUpdateDescriptor(&updateDescriptor)
	if err == nil {
		t.Error("Test failed. Error expected")
	}

	updateDescriptor.Platform_name = "wilkes"
	err = ValidateUpdateDescriptor(&updateDescriptor)
	if err == nil {
		t.Error("Test failed. Error expected")
	}

	updateDescriptor.Platform_version = "4.4.0"
	err = ValidateUpdateDescriptor(&updateDescriptor)
	if err == nil {
		t.Error("Test failed. Error expected")
	}

	updateDescriptor.Applies_to = "wso2esb-4.9.0"
	err = ValidateUpdateDescriptor(&updateDescriptor)
	if err == nil {
		t.Error("Test failed. Error expected")
	}

	updateDescriptor.Bug_fixes = map[string]string{
		"N/A": "N/A",
	}
	err = ValidateUpdateDescriptor(&updateDescriptor)
	if err == nil {
		t.Error("Test failed. Error expected")
	}

	updateDescriptor.Description = "sample description"
	err = ValidateUpdateDescriptor(&updateDescriptor)
	if err != nil {
		t.Errorf("Test failed. Unexpected error %v", err)
	}
}

func TestIsStringIsInSlice(t *testing.T) {
	data := []string{"a", "b", "c"}
	str := "a"
	foundInSlice := IsStringIsInSlice(str, data)
	if !foundInSlice {
		t.Errorf("Test failed. String '%v' not found in slice %v", str, data)
	}

	str = "b"
	foundInSlice = IsStringIsInSlice(str, data)
	if !foundInSlice {
		t.Errorf("Test failed. String '%v' not found in slice %v", str, data)
	}

	str = "c"
	foundInSlice = IsStringIsInSlice(str, data)
	if !foundInSlice {
		t.Errorf("Test failed. String '%v' not found in slice %v", str, data)
	}

	str = "d"
	foundInSlice = IsStringIsInSlice(str, data)
	if foundInSlice {
		t.Errorf("Test failed. String '%v' found in slice %v", str, data)
	}
}

func TestProcessString01(t *testing.T) {
	data := "esb, am"
	delimiter := ","
	result := ProcessString(data, delimiter, false)
	expectedResult := "esb, am"
	if result != expectedResult {
		t.Errorf("Test failed, expected: '%v', actual: '%v'", expectedResult, result)
	}

	data = "esb\n am"
	delimiter = ","
	result = ProcessString(data, delimiter, false)
	expectedResult = "esb, am"
	if result != expectedResult {
		t.Errorf("Test failed, expected: '%v', actual: '%v'", expectedResult, result)
	}

	data = "esb, es\n am"
	delimiter = ","
	result = ProcessString(data, delimiter, false)
	expectedResult = "esb, es, am"
	if result != expectedResult {
		t.Errorf("Test failed, expected: '%v', actual: '%v'", expectedResult, result)
	}

	data = "esb\n am, es"
	delimiter = ","
	result = ProcessString(data, delimiter, false)
	expectedResult = "esb, am, es"
	if result != expectedResult {
		t.Errorf("Test failed, expected: '%v', actual: '%v'", expectedResult, result)
	}
}

func TestProcessString02(t *testing.T) {
	data := "sample"
	delimiter := "\n"
	result := ProcessString(data, delimiter, false)
	expectedResult := "sample"
	if result != expectedResult {
		t.Errorf("Test failed, expected: '%v', actual: '%v'", expectedResult, result)
	}

	data = "  sample  "
	delimiter = "\n"
	result = ProcessString(data, delimiter, false)
	expectedResult = "sample"
	if result != expectedResult {
		t.Errorf("Test failed, expected: '%v', actual: '%v'", expectedResult, result)
	}

	data = "sample\nsample"
	delimiter = "\n"
	result = ProcessString(data, delimiter, false)
	expectedResult = "sample\nsample"
	if result != expectedResult {
		t.Errorf("Test failed, expected: '%v', actual: '%v'", expectedResult, result)
	}

	data = "sample\rsample"
	delimiter = "\n"
	result = ProcessString(data, delimiter, false)
	expectedResult = "sample\nsample"
	if result != expectedResult {
		t.Errorf("Test failed, expected: '%v', actual: '%v'", expectedResult, result)
	}

	data = "sample\n\tsample"
	delimiter = "\n"
	result = ProcessString(data, delimiter, false)
	expectedResult = "sample\n    sample"
	if result != expectedResult {
		t.Errorf("Test failed, expected: '%v', actual: '%v'", expectedResult, result)
	}

	data = "sample\n\tsample"
	delimiter = "\n"
	result = ProcessString(data, delimiter, true)
	expectedResult = "sample\nsample"
	if result != expectedResult {
		t.Errorf("Test failed, expected: '%v', actual: '%v'", expectedResult, result)
	}

	data = "    sample\n\tsample    "
	delimiter = "\n"
	result = ProcessString(data, delimiter, false)
	expectedResult = "sample\n    sample"
	if result != expectedResult {
		t.Errorf("Test failed, expected: '%v', actual: '%v'", expectedResult, result)
	}

	data = "    sample\n\tsample    "
	delimiter = "\n"
	result = ProcessString(data, delimiter, true)
	expectedResult = "sample\nsample"
	if result != expectedResult {
		t.Errorf("Test failed, expected: '%v', actual: '%v'", expectedResult, result)
	}
}
