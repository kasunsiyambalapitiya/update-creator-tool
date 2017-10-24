/*
 * Copyright (c) 2017, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package cmd

import (
	"errors"
	"fmt"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"github.com/wso2/update-creator-tool/constant"
	"github.com/wso2/update-creator-tool/util"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var (
	validateCmdUse       = "validate <update_zip_loc> <prev_dist_loc>"
	validateCmdShortDesc = "Validate the formed update zip"
	validateCmdLongDesc  = dedent.Dedent(`
	This command will validate the given update zip by checking whether all the files listed in update-descriptor
	.yaml  under 'added_files' and 'modified_files' are contained within the update zip and all the files listed under
	'removed_files' exists in the previous distribution so that wum-client can perform the update successfully.
	<update_zip_loc>	path to the formed update zip
	<prev_dist_loc>		path to the previous distribution`)
)

var validateCmd = &cobra.Command{
	Use:   validateCmdUse,
	Short: validateCmdShortDesc,
	Long:  validateCmdLongDesc,
	Run:   initializeValidateCommand,
}

// This function will be called first and this will add flags to the command.
func init() {
	RootCmd.AddCommand(validateCmd)

	validateCmd.Flags().BoolVarP(&isDebugLogsEnabled, "debug", "d", util.EnableDebugLogs, "Enable debug logs")
	validateCmd.Flags().BoolVarP(&isTraceLogsEnabled, "trace", "t", util.EnableTraceLogs, "Enable trace logs")
}

// This function will be called when validate command is called.
func initializeValidateCommand(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		util.HandleErrorAndExit(errors.New("invalid number of arguments. Run 'wum-uc validate --help' to " +
			"view help"))
	}
	validateUpdateZip(args[0], args[1])
}

// This function validate the given update zip to check whether it can be applied via the wum-client.
func validateUpdateZip(updateZipPath, previousDistPath string) {
	// Set log level
	setLogLevel()
	logger.Debug("[validate] command called")

	// Check whether the given archives exists
	checkArchiveExists(updateZipPath)
	checkArchiveExists(previousDistPath)

	// Check whether the given archives are zip files
	util.IsZipFile("update zip", updateZipPath)
	logger.Debug(fmt.Sprintf("Provided update archive is a zip file"))
	util.IsZipFile("previous distribution", previousDistPath)
	logger.Debug(fmt.Sprintf("Provided previous distribution is a zip file"))

	// Get the name of the update
	updateZipPathString := strings.Split(updateZipPath, "/")
	updateName := updateZipPathString[len(updateZipPathString)-1]
	updateName = strings.TrimSuffix(updateName, ".zip")

	// Get zipReaders for both archives
	updateZipReader := getZipReader(updateZipPath)
	logger.Debug(fmt.Sprintf("Zip reader used for reading update zip created successfully"))
	previousDistributionReader := getZipReader(previousDistPath)
	logger.Debug(fmt.Sprintf("Zip reader used for reading previous distribution created successfully"))

	defer updateZipReader.Close()
	defer previousDistributionReader.Close()

	// Extract out update-descriptor.yaml to a temp location
	logger.Info(fmt.Sprintf("Extracting out update-descriptor.yaml to a temp location"))
	destination := path.Join(constant.TEMP_DIR, constant.UPDATE_DESCRIPTOR_FILE)
	// Replace all / with OS specific path separators to handle OSs like Windows
	destination = strings.Replace(destination, "/", constant.PATH_SEPARATOR, -1)

	for _, file := range updateZipReader.Reader.File {
		// Name of the file
		fileName := file.Name
		// Filter out only the update-descriptor.yaml for opening its content
		if fileName == updateName+"/"+constant.UPDATE_DESCRIPTOR_FILE {
			zippedFile, err := file.Open()
			if err != nil {
				util.HandleErrorAndExit(err)
			}
			data, err := ioutil.ReadAll(zippedFile)
			if err != nil {
				util.HandleErrorAndExit(err)
			}
			// Close the zippedFile after reading its data
			zippedFile.Close()

			// Need to create relevant parent directory in the destination before witting to update-descriptor.yaml file
			parentDirectory := filepath.Dir(destination)
			err = util.CreateDirectory(parentDirectory)
			util.HandleErrorAndExit(err, fmt.Sprintf("Error occured when creating the %s directory", parentDirectory))

			// Create update-descriptor.yaml file in the destination
			file, err := os.OpenFile(
				destination,
				os.O_WRONLY|os.O_TRUNC|os.O_CREATE,
				0600,
			)
			if err != nil {
				util.HandleErrorAndExit(err)
			}

			// Write bytes to the created file
			_, err = file.Write(data)
			if err != nil {
				util.HandleErrorAndExit(err)
			}
			// Close the update-descriptor.yaml file opened for writing
			file.Close()
			// Break the for loop when the update-descriptor.yaml is located
			break
		}
	}
	logger.Info(fmt.Sprintf("Extracting out update-descriptor.yaml to a temp location completed successfully"))
	// Read update-descriptor.yaml and parse it to UpdateDescriptor struct
	// Need to reset destination to 'temp' directory for using the util.LoadUpdateDescriptor
	destination = path.Join(constant.TEMP_DIR)
	updateDescriptor, err := util.LoadUpdateDescriptor(constant.UPDATE_DESCRIPTOR_FILE, destination)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred when reading '%s' file.",
		constant.UPDATE_DESCRIPTOR_FILE))

	// Get added, modified and removed files from the UpdateDescriptor struct
	logger.Info(fmt.Sprintf("Identifying file being added, removed and modified from the update"))
	addedFiles := updateDescriptor.File_changes.Added_files
	modifiedFiles := updateDescriptor.File_changes.Modified_files
	removedFiles := updateDescriptor.File_changes.Removed_files

	// Need to add carbon.home/ to the beginning of added and modified file paths due to structure of the update zip
	logger.Debug(fmt.Sprintf("Adding %s/ to the beginning of added and modified file paths", constant.CARBON_HOME))
	prefixedAddedFiles := addPathPrefix(&addedFiles)
	logger.Debug("Adding %s/ to the beginning of added files completed successfully", constant.CARBON_HOME)
	prefixedModifiedFiles := addPathPrefix(&modifiedFiles)
	logger.Debug("Adding %s/ to the beginning of modified files completed successfully", constant.CARBON_HOME)

	logger.Info(fmt.Sprintf("Identifying file being added, removed and modified from the update completed " +
		"successfully"))

	// RootNode is what we use as the root of the update zip when populating tree like structure
	rootNodeOfUpdatezip := createNewNode()
	rootNodeOfUpdatezip, err = readZip(updateZipReader, &rootNodeOfUpdatezip)
	util.HandleErrorAndExit(err)
	logger.Debug(fmt.Sprintf("Node tree for update zip created successfully"))
	logger.Debug(fmt.Sprintf("Reading update zip completed successfully"))

	// Check whether the added files exists in the update zip
	logger.Info(fmt.Sprintf("Checking for existance of added files in the update zip"))
	checkFileExistsInNodeTree(&rootNodeOfUpdatezip, prefixedAddedFiles, "update zip")
	logger.Info(fmt.Sprintf("Checking for existance of added files in the update zip completed successfully"))

	// Check whether the modified files exists in the update zip
	logger.Debug(fmt.Sprintf("Checking for existance of modified files in the update zip"))
	checkFileExistsInNodeTree(&rootNodeOfUpdatezip, prefixedModifiedFiles, "update zip")
	logger.Debug(fmt.Sprintf("Checking for existance of modified files in the update zip completed successfully"))

	// Delete temp directory
	util.CleanUpDirectory(path.Join(constant.TEMP_DIR))

	// RootNode is what we use as the root of the previous distribution when populating tree like structure
	rootNodeOfPreviousDistribution := createNewNode()
	rootNodeOfPreviousDistribution, err = readZip(previousDistributionReader, &rootNodeOfPreviousDistribution)
	util.HandleErrorAndExit(err)
	logger.Debug(fmt.Sprintf("Node tree for previous distribution created successfully"))
	logger.Debug(fmt.Sprintf("Reading previous distribution completed successfully"))

	// Check whether the removed files exists in the previous distribution
	logger.Info(fmt.Sprintf("Checking for existance of removed files in the previous distribution"))
	checkFileExistsInNodeTree(&rootNodeOfPreviousDistribution, &removedFiles, "previous distribution")
	logger.Info(fmt.Sprintf("Checking for existance of removed files in the previous distribution completed " +
		"successfully"))
	logger.Info(fmt.Sprintf("Validating the update zip completed successfully"))
}

// This function checks whether the given zip file exists.
func checkArchiveExists(archivePath string) {
	exists, err := util.IsFileExists(archivePath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while reading '%s' file", archivePath))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("'%s' file does not exists.", archivePath)))
	}
	logger.Debug(fmt.Sprintf("The '%s' file exists", archivePath))
}

// This function checks whether the given file exists in the given node tree.
func checkFileExistsInNodeTree(rootNode *node, files *[]string, archiveType string) {
	for _, relativePath := range *files {
		found := false
		// Check whether the relative path points to a directory
		if strings.HasSuffix(relativePath, "/") {
			found, _ = pathExists(rootNode, relativePath, true)

		} else {
			found, _ = pathExists(rootNode, relativePath, false)
		}
		if found {
			logger.Trace(fmt.Sprintf("Relative path %s exists in %s", relativePath, archiveType))
		} else {
			util.HandleErrorAndExit(errors.New(fmt.Sprintf("%s does not exists in %s", relativePath,
				archiveType)))
		}
	}
}

// This function adds the given prefix to file path
func addPathPrefix(files *[]string) *[]string {
	tempFiles := make([]string, 0, len(*files))
	// Iterating through the files slice
	for _, file := range *files {
		logger.Trace(fmt.Sprintf("File path before adding the prefix : %s", file))
		file = constant.CARBON_HOME + "/" + file
		logger.Trace(fmt.Sprintf("File path after adding the prefix : %s", file))
		tempFiles = append(tempFiles, file)
	}
	return &tempFiles
}
