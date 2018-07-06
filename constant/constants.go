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

package constant

import (
	"os"

	"github.com/ian-kent/go-log/levels"
)

const (
	DEFAULT_LOG_LEVEL = levels.WARN

	PATH_SEPARATOR    = string(os.PathSeparator)
	PLUGINS_DIRECTORY = "repository" + PATH_SEPARATOR + "components" + PATH_SEPARATOR + "plugins" + PATH_SEPARATOR

	//constants to store resource file names
	README_FILE               = "README.txt"
	LICENSE_FILE              = "LICENSE.txt"
	NOT_A_CONTRIBUTION_FILE   = "NOT_A_CONTRIBUTION.txt"
	INSTRUCTIONS_FILE         = "instructions.txt"
	UPDATE_DESCRIPTOR_V2_FILE = "update-descriptor.yaml"
	UPDATE_DESCRIPTOR_V3_FILE = "update-descriptor3.yaml"
	WUMUC_CONFIG_FILE         = "config.yaml"

	//Temporary directory to copy files before creating the new zip
	TEMP_DIR = "temp"
	//This is used to store carbon.home string
	CARBON_HOME = "carbon.home"
	//Prefix of the update file and the root directory of the update zip
	UPDATE_NAME_PREFIX = "WSO2-CARBON-UPDATE"

	//Constants to store configs in viper
	DISTRIBUTION_ROOT = "DISTRIBUTION_ROOT"
	UPDATE_ROOT       = "UPDATE_ROOT"
	UPDATE_NAME       = "_UPDATE_NAME"
	PRODUCT_NAME      = "_PRODUCT_NAME"

	UPDATE_NUMBER_REGEX  = "^\\d{4}$"
	KERNEL_VERSION_REGEX = "^\\d+\\.\\d+\\.\\d+$"
	FILENAME_REGEX       = "^WSO2-CARBON-UPDATE-\\d+\\.\\d+\\.\\d+-\\d{4}.zip$"
	EMAIL_ADDRESS_REGEX  = "^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$"

	OTHER   = 0
	YES     = 1
	NO      = 2
	REENTER = 3

	SAMPLE             = "SAMPLE"
	CHECK_MD5_DISABLED = "CHECK_MD5_DISABLED"
	//resource_files
	RESOURCE_FILES           = "RESOURCE_FILES"
	MANDATORY                = "MANDATORY"
	OPTIONAL                 = "OPTIONAL"
	SKIP                     = "SKIP"
	RESOURCE_FILES_MANDATORY = RESOURCE_FILES + "." + MANDATORY
	RESOURCE_FILES_OPTIONAL  = RESOURCE_FILES + "." + OPTIONAL
	RESOURCE_FILES_SKIP      = RESOURCE_FILES + "." + SKIP

	PLATFORM_VERSIONS = "PLATFORM_VERSIONS"

	PATCH_ID_REGEX         = "WSO2-CARBON-PATCH-(\\d+\\.\\d+\\.\\d+)-(\\d{4})"
	APPLIES_TO_REGEX       = "(?s)Applies To.*?:(.*)Associated JIRA|Applies To.*?:(.*)DESCRIPTION"
	ASSOCIATED_JIRAS_REGEX = "https:\\/\\/wso2\\.org\\/jira\\/browse\\/([A-Z]*?-\\d+)"
	DESCRIPTION_REGEX      = "(?s)DESCRIPTION\n-*\n(.*)INSTALLATION INSTRUCTIONS"

	PATCH_REGEX = "(?m).*patch.*"

	JIRA_API_URL = "https://wso2.org/jira/rest/api/latest/issue/"

	UPDATE_NO_DEFAULT        = "ADD_UPDATE_NUMBER_HERE"
	PLATFORM_NAME_DEFAULT    = "ADD_PLATFORM_NAME_HERE"
	PLATFORM_VERSION_DEFAULT = "ADD_PLATFORM_VERSION_HERE"
	APPLIES_TO_DEFAULT       = "ADD_APPLIES_TO_HERE"
	DESCRIPTION_DEFAULT      = "ADD_DESCRIPTION_HERE\n"

	JIRA_KEY_DEFAULT     = "ADD_JIRA_KEY_HERE/GITHUB_ISSUE_URL"
	JIRA_NA              = "N/A"
	JIRA_SUMMARY_DEFAULT = "ADD_JIRA_SUMMARY_HERE/GITHUB_ISSUE_SUMMARY"
	DISTRIBUTION         = "Distribution"
	UPDATE               = "Update"

	LICENSE_URL          = "LICENSE_URL"
	LICENSE_DOWNLOAD_URL = "https://wso2.com/license/wso2-update/LICENSE.txt"
	LICENSE_MD5          = "LICENSE_MD5"
	LICENSE_MD5_URL      = "https://wso2.com/license/wso2-update/LICENSE.txt.md5"

	NOT_A_CONTRIBUTION_URL          = "NOT_A_CONTRIBUTION_URL"
	NOT_A_CONTRIBUTION_DOWNLOAD_URL = "https://wso2.com/license/wso2-update/NOT_A_CONTRIBUTION.txt"
	NOT_A_CONTRIBUTION_MD5          = "NOT_A_CONTRIBUTION_MD5"
	NOT_A_CONTRIBUTION_MD5_URL      = "https://wso2.com/license/wso2-update/NOT_A_CONTRIBUTION.txt.md5"

	WUMUC_HOME_DIR_NAME = ".wum-uc"
	WUM_UC_HOME         = "WUM_UC_HOME"

	WUMUC_AUTHENTICATION_URL               = "https://api.updates.wso2.com"
	TOKEN_API_CONTEXT                      = "token"
	BASE64_ENCODED_CONSUMER_KEY_AND_SECRET = "cmNMWXQwcjd2azZQTTE3SVA4U3VYRDR0MjRNYTpHTlBhV2JmYVpveVhrUkdZT1FwdkIyN3EyOUlh"
	RENEW_REFRESH_TOKEN                    = "renewRefreshToken"
	RETRIEVE_ACCESS_TOKEN                  = "getAccessToken"
	INVALID_GRANT                          = "invalid_grant"
	WUMUC_UPDATE_TOKEN_TIMEOUT             = 2
	ERROR_READING_RESPONSE_MSG             = "there is an error reading the response from WSO2 Update"
	INVALID_CREDENTIALS                    = "Invalid Credentials."
	ENTER_YOUR_CREDENTIALS_MSG             = "Please enter your WSO2 credentials to continue"
	UNABLE_TO_READ_YOUR_INPUT_MSG          = "unable to read your input"
	USERNAME_PASSWORD_EMPTY_MSG            = "username or password cannot be empty"
	DONE_MSG                               = "Done!\n"
	INVALID_EMAIL_ADDRESS                  = "Invalid email address"

	PRODUCT_API_CONTEXT = "products"
	DEFAULT_DESCRIPTION = `Description goes here
`

	DEFAULT_INSTRUCTIONS = `Instruction goes here
`
	DEFAULT_JIRA_KEY     = "Enter JIRA_KEY/GITHUB ISSUE URL"
	DEFAULT_JIRA_SUMMARY = "Enter JIRA_KEY SUMMARY/GITHUB_ISSUE_SUMMARY"

	PRODUCT_API_VERSION                  = "3.0.0"
	APPLICABLE_PRODUCTS                  = "applicable-products"
	FILE_LIST_ONLY                       = "fileListOnly=true"
	UNABLE_TO_CONNECT_WSO2_UPDATE        = "there is a problem connecting to WSO2 Update please try again"
	WUMUC_API_CALL_TIMEOUT               = 5
	TOO_MANY_REQUESTS_ERROR_MSG          = "servers are busy at the moment. Please try again later."
	CONTINUED_ERROR_REPORT_MSG           = "if you continue to have this problem, please contact WUM team"
	INVALID_EXPIRED_REFRESH_TOKEN_MSG    = "your session has timed out"
	YOU_HAVENT_INITIALIZED_WUMUC_YET_MSG = "you haven't initialized wum-uc with your WSO2 credentials"
	RUN_WUMUC_INIT_TO_CONTINUE_MSG       = "run 'wum-uc init' to continue"

	HEADER_AUTHORIZATION               = "Authorization"
	HEADER_CONTENT_TYPE                = "Content-Type"
	HEADER_ACCEPT                      = "Accept"
	HEADER_VALUE_APPLICATION_JSON      = "application/json"
	HEADER_VALUE_X_WWW_FORM_URLENCODED = "application/x-www-form-urlencoded"
)
