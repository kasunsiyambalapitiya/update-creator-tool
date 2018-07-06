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

// Default values used in the application
var (
	EnableDebugLogs = false
	EnableTraceLogs = false
	// We only check md5 if -m flag is not found. If -m is set, it's value by default is true. That means we don't
	// want to check md5 if this value is true. By default we want to check. So that's why we have set
	// CheckMd5Disabled to false here.
	CheckMd5Disabled        = false
	ResourceFiles_Mandatory = []string{"LICENSE.txt"}
	ResourceFiles_Optional  = []string{"update-descriptor.yaml", "update-descriptor3.yaml", "instructions.txt",
		"NOT_A_CONTRIBUTION.txt"}
	ResourceFiles_Skip = []string{"README.txt"}
	PlatformVersions   = map[string]string{
		"4.2.0": "turing",
		"4.3.0": "perlis",
		"4.4.0": "wilkes",
		"5.0.0": "hamming",
	}
)
