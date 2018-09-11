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
	"fmt"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"github.com/wso2/update-creator-tool/constant"
	"github.com/wso2/update-creator-tool/util"
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
	// Todo change according to version check
	//isCurrentVersionSupported()
	fmt.Fprintln(os.Stderr, constant.DONE_MSG)
}
