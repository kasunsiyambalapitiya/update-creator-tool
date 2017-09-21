// Copyright (c) 2017, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
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
	"github.com/spf13/cobra"
	"github.com/renstrom/dedent"
	"github.com/wso2/update-creator-tool/util"
	"errors"
	"strings"
	"fmt"
	"github.com/spf13/viper"
	"github.com/wso2/update-creator-tool/constant"
)

// This struct used to store directory structure of the distribution.
type node struct {
	name             string
	isDir            bool
	relativeLocation string
	parent           *node
	childNodes       map[string]*node
	md5Hash          string
}
// This is used to create a new node which will initialize the childNodes map.
func createNewNode() node {
	return node{
		childNodes: make(map[string]*node),
	}
}

// Values used to print help command.
var (
	generateCmdUse = "generate <update_zip_loc> <dist_zip_loc>"
	generateCmdShortDesc = "generate a new update"
	generateCmdLongDesc = dedent.Dedent(`
		This command will generate a new update zip by comparing the diff between the updated pack and the
		previous released distribution.`)
)

// generateCmd represents the generate command.
var generateCmd = &cobra.Command{
	Use:   generateCmdUse,
	Short: generateCmdShortDesc,
	Long:  generateCmdLongDesc,
	Run:   initializeGenerateCommand,
}

// This function will be called first and this will add flags to the command.
func init() {
	RootCmd.AddCommand(generateCmd)

	generateCmd.Flags().BoolVarP(&isDebugLogsEnabled, "debug", "d", util.EnableDebugLogs, "Enable debug logs")
	generateCmd.Flags().BoolVarP(&isTraceLogsEnabled, "trace", "t", util.EnableTraceLogs, "Enable trace logs")

	generateCmd.Flags().BoolP("md5", "m", util.CheckMd5Disabled, "Disable checking MD5 sum")
	//viper.BindPFlag(constant.CHECK_MD5_DISABLED, generateCmd.Flags().Lookup("md5"))
}

// This function will be called when the generate command is called.
func initializeGenerateCommand(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		util.HandleErrorAndExit(errors.New("Invalid number of argumants. Run 'wum-uc generate --help' to " +
			"view help."))
	}
	generateUpdate(args[0], args[1])
}

func generateUpdate(updatedPack, previosPack string) {
	// set debug level
	setLogLevel()
	logger.Debug("[generate] command called")

	//to a seperate method and reuse
	if !strings.HasSuffix(updatedPack, ".zip") {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("Entered distribution path(%s) does not point to a " +
			"zip file.", updatedPack)))
	}

	distributionName := getDistributionName(updatedPack)
	// Read the distribution zip file
	logger.Debug("Reading zip")
	util.PrintInfo(fmt.Sprintf("Reading %s. Please wait...", distributionName))
	rootNode, err = ReadZip(distributionPath)
	util.HandleErrorAndExit(err)
	logger.Debug("Reading zip finished")

}

func getDistributionName(distributionZipName string) string {

	//make this a common method
	// Get the product name from the distribution path and set it as a viper config
	paths := strings.Split(distributionZipName, constant.PATH_SEPARATOR)
	distributionName := strings.TrimSuffix(paths[len(paths) - 1], ".zip")
	viper.Set(constant.PRODUCT_NAME, distributionName)
	return distributionName
}