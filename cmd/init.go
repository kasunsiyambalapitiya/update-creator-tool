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
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ian-kent/go-log/log"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"github.com/wso2/update-creator-tool/constant"
	"github.com/wso2/update-creator-tool/util"
	"net/http"
	"os"
)

// Values used to print help command.
var (
	initCmdUse       = "init"
	initCmdShortDesc = "Initialize wum-uc with your WSO2 credentials"
	initCmdLongDesc  = dedent.Dedent(`Initialize WUM-UC with your WSO2 credentials`)
	InitCmdExamples  = dedent.Dedent(`
		# You will be prompted to enter WSO2 credentials.
		  wum-uc init
		  Username: user@wso2.com
		  Password for 'user@wso2.com': my_Password

		# You will be prompted to enter your password.
		  wum-uc init -u user@wso2.com
		  Password for 'user@wso2.com': my_Password

		# Enter your WSO2 credentials as arguments.
		  wum-uc init -u user@wso2.com -p my_Password`)
)

var username string
var password string

// initCmd represents the init command.
var initCmd = &cobra.Command{
	Use:     initCmdUse,
	Short:   initCmdShortDesc,
	Long:    initCmdLongDesc,
	Example: InitCmdExamples,
	Run:     initializeInitCommand,
}

// struct which is used for checking if newer versions of 'wum-uc' are available
type WUMUCVersionCheckRequest struct {
	WUMUCVersion string `json:"wum-uc-version"`
}

//This function will be called first and this will add flags to the command.
func init() {
	RootCmd.AddCommand(initCmd)

	initCmd.Flags().BoolVarP(&isDebugLogsEnabled, "debug", "d", util.EnableDebugLogs, "Enable debug logs")
	initCmd.Flags().BoolVarP(&isTraceLogsEnabled, "trace", "t", util.EnableTraceLogs, "Enable trace logs")
	initCmd.Flags().StringVarP(&username, "username", "u", "", "Specify your email")
	initCmd.Flags().StringVarP(&password, "password", "p", "", "Specify your password")

}

// Initialize WUM-UC with WSO2 credentials and check if the current version of 'wum-uc' is supported.
func initializeInitCommand(cmd *cobra.Command, args []string) {
	logger.Debug("[Init] called")
	util.Init(username, []byte(password))
	isCurrentVersionSupported()
	fmt.Fprintln(os.Stderr, constant.DONE_MSG)
}

// This function checks if the current version of 'wum-uc' still supported for creating wum updates.
func isCurrentVersionSupported() {
	WUMUCVersionCheckRequest := WUMUCVersionCheckRequest{}
	WUMUCVersionCheckRequest.WUMUCVersion = Version
	requestBody := new(bytes.Buffer)
	err := json.NewEncoder(requestBody).Encode(WUMUCVersionCheckRequest)
	if err != nil {
		util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred when performing the 'wum-uc' version check"))
	}
	log.Debug(fmt.Sprintf("Request sent %v", requestBody))
	apiURL := util.GetWUMUCConfigs().URL + "/" + constant.PRODUCT_API_CONTEXT + "/" + constant.
		PRODUCT_API_VERSION + "/" + constant.APPLICABLE_PRODUCTS + "?" + constant.FILE_LIST_ONLY
	response := util.InvokePOSTRequest(apiURL, requestBody)
	if response.StatusCode != http.StatusOK {
		util.HandleUnableToConnectErrorAndExit(nil)
	}
	log.Debug(fmt.Sprintf("'wum-uc' %s version is supported", Version))
}
