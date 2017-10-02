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
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"strings"
	"fmt"
	"github.com/spf13/viper"
	"github.com/wso2/update-creator-tool/util"
	"github.com/wso2/update-creator-tool/constant"
	"errors"
	"archive/zip"
	"io/ioutil"
	"crypto/md5"
	"encoding/hex"
	"path/filepath"
	"reflect"
)

// Values used to print help command.
var (
	generateCmdUse       = "generate <update_dist_loc> <dist_dist_loc>"
	generateCmdShortDesc = "generate a new update"
	generateCmdLongDesc  = dedent.Dedent(`
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
	viper.BindPFlag(constant.CHECK_MD5_DISABLED, generateCmd.Flags().Lookup("md5"))
}

// This function will be called when the generate command is called.
func initializeGenerateCommand(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		util.HandleErrorAndExit(errors.New("Invalid number of argumants. Run 'wum-uc generate --help' to " +
			"view help."))
	}
	generateUpdate(args[0], args[1])
}

func generateUpdate(updatedDistPath, previousDistPath string) {
	// set debug level
	setLogLevel()
	logger.Debug("[generate] command called")

	checkDistribution(updatedDistPath)
	checkDistribution(previousDistPath)

	distributionName := getDistributionName(updatedDistPath)
	// Read the distribution zip file
	logger.Debug("Reading updated distribution zip")
	//use viper for getrting distributionName
	util.PrintInfo(fmt.Sprintf("Reading the updated %s. Please wait...", distributionName))
	// rootNode is what we use as the root of the updated distribution when we populate tree like structure.
	rootNodeOfUpdatedDistribution := CreateNewNode()
	var err error
	rootNodeOfUpdatedDistribution, err = ReadZip(updatedDistPath)
	logger.Debug("root node of the updated distribution received")
	//util.PrintInfo(fmt.Sprintf("Recieved ", len(rootNodeOfUpdatedDistribution.childNodes)))
	util.HandleErrorAndExit(err)
	logger.Debug("Reading updated distribution zip finished")
	logger.Debug("Reading previuosly released distribution zip")
	util.PrintInfo(fmt.Sprintf("Reading the previous %s. to get diff Please wait...", distributionName))

	//rootNodeOfPreviousDistribution := CreateNewNode()
	zipReader, err := zip.OpenReader(previousDistPath)
	if err != nil {
		//chck this
		util.HandleErrorAndExit(err)
	}
	defer zipReader.Close()

	//slices for modified, changed and deleted files from the update
	modifiedFiles := make(map[string]struct{})
	deletedFiles := make(map[string]struct{})
	addedFiles := make(map[string]struct{})

	//iterate through each file to identify modified and deleted files
	for _, file := range zipReader.Reader.File {
		//open the file for calculating MD5
		zippedFile, err := file.Open()
		if err != nil {
			util.HandleErrorAndExit(err)
		}
		data, err := ioutil.ReadAll(zippedFile)
		// Don't use defer here because otherwise there will be too many open files and it will cause a panic
		zippedFile.Close()
		// Calculate the md5 of the file
		hash := md5.New()
		hash.Write(data)
		md5Hash := hex.EncodeToString(hash.Sum(nil))

		//name of the file
		fileName := file.Name
		fmt.Println("name of the file", fileName)
		//check this log
		logger.Trace(fmt.Sprintf("file.Name: %s and md5", fileName, md5Hash))
		//delete this print
		//fmt.Println("file.Name: and md5", fileName, md5Hash)

		if strings.HasSuffix(fileName, "/") {
			fileName = strings.TrimSuffix(fileName, "/")
		}
		// Get the relative path of the file
		var relativePath string

		if (strings.Contains(fileName, "/")) {
			relativePath = strings.SplitN(fileName, "/", 2)[1]
		} else {
			relativePath = ""
		}

		// Replace all \ with /. Otherwise it will cause issues in Windows OS.
		relativePath = filepath.ToSlash(relativePath)
		logger.Trace(fmt.Sprintf("relativePath: %s", relativePath))
		if strings.HasSuffix(relativePath, "/") {
			relativePath = strings.TrimSuffix(relativePath, "/")
		}
		//delete
		fmt.Println("relativePath:", relativePath)

		fileNameStrings := strings.Split(fileName, "/")
		fmt.Println("length", len(fileNameStrings))
		fileName = fileNameStrings[len(fileNameStrings)-1]
		fmt.Println(fileName)
		if relativePath != "" {
			//Finding modified files
			findModifiedFiles(&rootNodeOfUpdatedDistribution, fileName, md5Hash, relativePath, modifiedFiles)
			//Finding deleted files
			findDeletedOrNewlyAddedFiles(&rootNodeOfUpdatedDistribution, fileName, relativePath, rootNodeOfUpdatedDistribution.childNodes, deletedFiles)
		}
	}

	//finding newly added files to the previous distribution
	distributionName = getDistributionName(previousDistPath)
	// Read the distribution zip file
	logger.Debug("Reading previous distribution zip")
	//use viper for getrting distributionName
	util.PrintInfo(fmt.Sprintf("Reading the previous %s. Please wait...", distributionName))
	// rootNode is what we use as the root of the updated distribution when we populate tree like structure.
	rootNodeOfPreviousDistribution := CreateNewNode()
	rootNodeOfPreviousDistribution, err = ReadZip(previousDistPath)
	logger.Debug("root node of the previous distribution received")
	//util.PrintInfo(fmt.Sprintf("Recieved ", len(rootNodeOfPreviousDistribution.childNodes)))
	util.HandleErrorAndExit(err)
	logger.Debug("Reading previous distribution zip finished")
	logger.Debug("Reading updated distribution zip for finding newly added files")
	//check text content of the log
	util.PrintInfo(fmt.Sprintf("Reading the updated %s. to get diff Please wait...", distributionName))

	zipReader, err = zip.OpenReader(updatedDistPath)
	if err != nil {
		//chck this
		util.HandleErrorAndExit(err)
	}
	defer zipReader.Close()
	// iterate throug updated pack to identify the newly added files
	for _, file := range zipReader.Reader.File {
		// we donot need to calculate the md5 of the file as we are filtering only the added files
		// name of the file
		fileName := file.Name
		fmt.Println("name of the file", fileName)
		//check this log
		logger.Trace(fmt.Sprintf("file.Name: %s and md5", fileName))
		//delete this print
		//fmt.Println("file.Name: and md5", fileName, md5Hash)

		if strings.HasSuffix(fileName, "/") {
			fileName = strings.TrimSuffix(fileName, "/")
		}
		// Get the relative path of the file
		var relativePath string
		if (strings.Contains(fileName, "/")) {
			relativePath = strings.SplitN(fileName, "/", 2)[1]
		} else {
			relativePath = ""
		}

		// Replace all \ with /. Otherwise it will cause issues in Windows OS.
		relativePath = filepath.ToSlash(relativePath)
		logger.Trace(fmt.Sprintf("relativePath: %s", relativePath))
		//delete
		//fmt.Println("relativePath:", relativePath)

		fileNameStrings := strings.Split(fileName, "/")
		fmt.Println("length", len(fileNameStrings))
		fileName = fileNameStrings[len(fileNameStrings)-1]
		fmt.Println(fileName)
		//Finding newly added files
		if relativePath != "" {
			findDeletedOrNewlyAddedFiles(&rootNodeOfPreviousDistribution, fileName, relativePath, rootNodeOfPreviousDistribution.childNodes, addedFiles)
		}
	}

	//fmt.Println("Modified files",modifiedFiles)
	util.PrintInfo("Modified Files", modifiedFiles)
	util.PrintInfo("length", len(modifiedFiles))
	//fmt.Println("Deleted Files",deletedFiles)
	util.PrintInfo("Deleted Files", deletedFiles)
	util.PrintInfo("length", len(deletedFiles))
	//fmt.Println("Added Files",addedFiles)
	util.PrintInfo("Added Files", addedFiles)
	util.PrintInfo("length", len(addedFiles))

}

func checkDistribution(distributionName string) {
	//to a seperate method and reuse in create.go
	if !strings.HasSuffix(distributionName, ".zip") {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("Entered distribution path(%s) does not point to a "+
			"zip file.", distributionName)))
	}
}

func getDistributionName(distributionZipName string) string {

	//make this a common method
	// Get the product name from the distribution path and set it as a viper config
	paths := strings.Split(distributionZipName, constant.PATH_SEPARATOR)
	distributionName := strings.TrimSuffix(paths[len(paths)-1], ".zip")
	viper.Set(constant.PRODUCT_NAME, distributionName)
	return distributionName
}

func findModifiedFiles(root *Node, name string, md5Hash string, relativePath string, modifiedFiles map[string]struct{}) {
	// Check whether the given name is in the child Nodes
	childNode, found := root.childNodes[name]
	//fmt.Println("entered to findModified")
	if found && childNode.isDir == false && childNode.relativeLocation == relativePath && childNode.md5Hash !=
		md5Hash {
		//fmt.Println("found")
		_, found := modifiedFiles[childNode.relativeLocation]
		if (!found) {
			modifiedFiles[childNode.relativeLocation] = struct{}{}
		}

	}
	// Regardless of whether the file is found or not, iterate through all sub directories to find all matches
	for _, childNode := range root.childNodes {
		if childNode.isDir {
			findModifiedFiles(childNode, name, md5Hash, relativePath, modifiedFiles)
		}
	}
}

func findDeletedOrNewlyAddedFiles(root *Node, fileName string, relativeLocation string, childNodesOfRootOfParentDistribution map[string]*Node, matches map[string]struct{}) bool {
	// need to remove if there is a slash at the end of the relativeLocation path
	//fmt.Println("relative loc before: ", relativeLocation)
	//relativeLocation = strings.TrimSuffix(relativeLocation, "/")
	//fmt.Println(relativeLocation)
	// Check whether a file exists in the given relative path in any child Node
	_, found := root.childNodes[fileName]
	//_, recorded := matches[relativeLocation]

	//checking whether the file is in the correct relative location
	if found {
		if root.childNodes[fileName].relativeLocation != relativeLocation {
			found = false
		} else {
			return true
		}
	}
	if !found {
		for _, childNode := range root.childNodes {
			if childNode.isDir {
				found = findDeletedOrNewlyAddedFiles(childNode, fileName, relativeLocation, childNodesOfRootOfParentDistribution, matches)
				if found {
					break
				}
			}
		}

	}
	//after going through all the childnodes if it is still false means, that relative location is not present
	parentRootNode := reflect.DeepEqual(childNodesOfRootOfParentDistribution, root.childNodes)
	if !found && parentRootNode {
		matches[relativeLocation] = struct{}{}
	}
	return found
}
