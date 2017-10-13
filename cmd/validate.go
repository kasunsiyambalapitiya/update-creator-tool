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
	"archive/zip"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wso2/update-creator-tool/constant"
	"github.com/wso2/update-creator-tool/util"
	"gopkg.in/yaml.v2"
)

var (
	validateCmdUse       = "validate <update_loc> <dist_loc>"
	validateCmdShortDesc = "Validate update zip"
	validateCmdLongDesc  = dedent.Dedent(`
		This command will validate the given update zip. Files will be
		matched against the given distribution. This will also validate
		the structure of the update-descriptor.yaml file as well.`)
)

// validateCmd represents the validate command
var validateCmd = &cobra.Command{
	Use:   validateCmdUse,
	Short: validateCmdShortDesc,
	Long:  validateCmdLongDesc,
	Run:   initializeValidateCommand,
}

//This function will be called first and this will add flags to the command.
func init() {
	RootCmd.AddCommand(validateCmd)

	validateCmd.Flags().BoolVarP(&isDebugLogsEnabled, "debug", "d", util.EnableDebugLogs, "Enable debug logs")
	validateCmd.Flags().BoolVarP(&isTraceLogsEnabled, "trace", "t", util.EnableTraceLogs, "Enable trace logs")
}

//This function will be called when the validate command is called.
func initializeValidateCommand(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		util.HandleErrorAndExit(errors.New("Invalid number of argumants. Run 'wum-uc validate --help' to " +
			"view help."))
	}
	startValidation(args[0], args[1])
}

//This function will start the validation process.
func startValidation(updateFilePath, distributionLocation string) {

	//Set the log level
	setLogLevel()
	logger.Debug("validate command called")

	updateFileMap := make(map[string]bool)
	distributionFileMap := make(map[string]bool)

	//Check whether the update has the zip extension
	if !strings.HasSuffix(updateFilePath, ".zip") {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("Update must be a zip file. Entered file '%s' does "+
			"not have a zip extension.", updateFilePath)))
	}

	//Check whether the update file exists
	exists, err := util.IsFileExists(updateFilePath)
	util.HandleErrorAndExit(err, "")
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("Entered update file does not exist at '%s'.",
			updateFilePath)))
	}

	//Check whether the distribution has the zip extension
	if !strings.HasSuffix(distributionLocation, ".zip") {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("Distribution must be a zip file. Entered file "+
			"'%s' does not have a zip extension.", distributionLocation)))
	}

	//Set the product name in viper configs
	lastIndex := strings.LastIndex(distributionLocation, constant.PATH_SEPARATOR)
	productName := strings.TrimSuffix(distributionLocation[lastIndex+1:], ".zip")
	logger.Debug(fmt.Sprintf("Setting ProductName: %s", productName))
	viper.Set(constant.PRODUCT_NAME, productName)

	//Check whether the distribution file exists
	exists, err = util.IsFileExists(distributionLocation)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while checking '%s'", distributionLocation))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("Entered distribution file does not exist at '%s'.",
			distributionLocation)))
	}

	//Check update filename
	locationInfo, err := os.Stat(updateFilePath)
	util.HandleErrorAndExit(err, "Error occurred while getting the information of update file")
	match, err := regexp.MatchString(constant.FILENAME_REGEX, locationInfo.Name())
	if !match {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("Update filename '%s' does not match '%s' regular "+
			"expression.", locationInfo.Name(), constant.FILENAME_REGEX)))
	}

	//Set the update name in viper configs
	updateName := strings.TrimSuffix(locationInfo.Name(), ".zip")
	viper.Set(constant.UPDATE_NAME, updateName)

	//Read the update zip file
	updateFileMap, updateDescriptor, err := readUpdateZip(updateFilePath)
	util.HandleErrorAndExit(err)
	logger.Trace(fmt.Sprintf("updateFileMap: %v\n", updateFileMap))

	//Read the distribution zip file
	distributionFileMap, err = readDistributionZip(distributionLocation)
	util.HandleErrorAndExit(err)
	logger.Trace(fmt.Sprintf("distributionFileMap: %v\n", distributionFileMap))

	//Compare the update with the distribution
	err = compare(updateFileMap, distributionFileMap, updateDescriptor)
	util.HandleErrorAndExit(err)
	util.PrintInfo("'" + updateName + "' validation successfully finished.")
}

//This function compares the files in the update and the distribution.
func compare(updateFileMap, distributionFileMap map[string]bool, updateDescriptor *util.UpdateDescriptor) error {
	updateName := viper.GetString(constant.UPDATE_NAME)
	for filePath := range updateFileMap {
		logger.Debug(fmt.Sprintf("Searching: %s", filePath))
		_, found := distributionFileMap[filePath]
		if !found {
			logger.Debug("Added files: ", updateDescriptor.File_changes.Added_files)
			isInAddedFiles := util.IsStringIsInSlice(filePath, updateDescriptor.File_changes.Added_files)
			logger.Debug(fmt.Sprintf("isInAddedFiles: %v", isInAddedFiles))
			resourceFiles := GetResourceFiles()
			logger.Debug(fmt.Sprintf("resourceFiles: %v", resourceFiles))
			fileName := strings.TrimPrefix(filePath, updateName+"/")
			logger.Debug(fmt.Sprintf("fileName: %s", fileName))
			_, foundInResources := resourceFiles[fileName]
			logger.Debug(fmt.Sprintf("found in resources: %v", foundInResources))
			//check
			if !isInAddedFiles && !foundInResources {
				return errors.New(fmt.Sprintf("File not found in the distribution: '%v'. If this is "+
					"a new file, add an entry to the 'added_files' sections in the '%v' file",
					filePath, constant.UPDATE_DESCRIPTOR_FILE))
			} else {
				logger.Debug("'" + filePath + "' found in added files.")
			}
		}
	}
	return nil
}

//This function will read the update zip at the the given location.
func readUpdateZip(filename string) (map[string]bool, *util.UpdateDescriptor, error) {
	fileMap := make(map[string]bool)
	updateDescriptor := util.UpdateDescriptor{}

	isNotAContributionFileFound := false
	isASecPatch := false

	// Create a reader out of the zip archive
	zipReader, err := zip.OpenReader(filename)
	if err != nil {
		return nil, nil, err
	}
	defer zipReader.Close()

	updateName := viper.GetString(constant.UPDATE_NAME)
	logger.Debug("updateName:", updateName)
	// Iterate through each file/dir found in
	for _, file := range zipReader.Reader.File {
		name := getFileName(file.FileInfo().Name())
		if file.FileInfo().IsDir() {
			logger.Debug(fmt.Sprintf("filepath: %s", file.Name))

			logger.Debug(fmt.Sprintf("filename: %s", name))
			if name != updateName {
				logger.Debug("Checking:", name)
				//Check
				prefix := filepath.Join(updateName, constant.CARBON_HOME)
				hasPrefix := strings.HasPrefix(file.Name, prefix)
				if !hasPrefix {
					return nil, nil, errors.New("Unknown directory found: '" + file.Name + "'")
				}
			}
		} else {
			//todo: check for ignored files .gitignore
			logger.Debug(fmt.Sprintf("file.Name: %s", file.Name))
			logger.Debug(fmt.Sprintf("file.FileInfo().Name(): %s", name))
			fullPath := filepath.Join(updateName, name)
			logger.Debug(fmt.Sprintf("fullPath: %s", fullPath))
			switch name {
			case constant.UPDATE_DESCRIPTOR_FILE:
				//todo: check for any remaining placeholders
				data, err := validateFile(file, constant.UPDATE_DESCRIPTOR_FILE, fullPath, updateName)
				if err != nil {
					return nil, nil, err
				}
				err = yaml.Unmarshal(data, &updateDescriptor)
				if err != nil {
					return nil, nil, err
				}
				//check
				err = util.ValidateUpdateDescriptor(&updateDescriptor)
				if err != nil {
					return nil, nil, errors.New("'" + constant.UPDATE_DESCRIPTOR_FILE +
						"' is invalid. " + err.Error())
				}
			case constant.LICENSE_FILE:
				data, err := validateFile(file, constant.LICENSE_FILE, fullPath, updateName)
				if err != nil {
					return nil, nil, err
				}
				dataString := string(data)
				if strings.Contains(dataString, "under Apache License 2.0") {
					isASecPatch = true
				}
			case constant.INSTRUCTIONS_FILE:
				_, err := validateFile(file, constant.INSTRUCTIONS_FILE, fullPath, updateName)
				if err != nil {
					return nil, nil, err
				}
			case constant.NOT_A_CONTRIBUTION_FILE:
				isNotAContributionFileFound = true
				_, err := validateFile(file, constant.NOT_A_CONTRIBUTION_FILE, fullPath, updateName)
				if err != nil {
					return nil, nil, err
				}
			default:
				resourceFiles := GetResourceFiles()
				logger.Debug(fmt.Sprintf("resourceFiles: %v", resourceFiles))
				prefix := filepath.Join(updateName, constant.CARBON_HOME)
				logger.Debug(fmt.Sprintf("Checking prefix %s in %s", prefix, file.Name))
				hasPrefix := strings.HasPrefix(file.Name, prefix)
				_, foundInResources := resourceFiles[name]
				logger.Debug(fmt.Sprintf("foundInResources: %v", foundInResources))
				if !hasPrefix && !foundInResources {
					return nil, nil, errors.New(fmt.Sprintf("Unknown file found: '%s'.", file.Name))
				}
				logger.Debug(fmt.Sprintf("Trimming: %s using %s", file.Name,
					prefix+constant.PATH_SEPARATOR))
				relativePath := strings.TrimPrefix(file.Name, prefix+constant.PATH_SEPARATOR)
				fileMap[relativePath] = false
			}
		}
	}
	if !isASecPatch && !isNotAContributionFileFound {
		util.PrintWarning(fmt.Sprintf("This update is not a security update. But '%v' was not found. Please "+
			"review and add '%v' file if necessary.", constant.NOT_A_CONTRIBUTION_FILE,
			constant.NOT_A_CONTRIBUTION_FILE))
	} else if isASecPatch && isNotAContributionFileFound {
		util.PrintWarning(fmt.Sprintf("This update is a security update. But '%v' was found. Please review "+
			"and remove '%v' file if necessary.", constant.NOT_A_CONTRIBUTION_FILE,
			constant.NOT_A_CONTRIBUTION_FILE))
	}
	return fileMap, &updateDescriptor, nil
}

//This function will validate the provided file. If the word 'patch' is found, a warning message is printed.
func validateFile(file *zip.File, fileName, fullPath, updateName string) ([]byte, error) {
	logger.Debug(fmt.Sprintf("Validating '%s' at '%s' started.", fileName, fullPath))
	parent := strings.TrimSuffix(file.Name, getFileName(file.FileInfo().Name()))
	if file.Name != fullPath {
		return nil, errors.New(fmt.Sprintf("'%s' found at '%s'. It should be in the '%s' directory.", fileName,
			parent, updateName))
	} else {
		logger.Debug(fmt.Sprintf("'%s' found at '%s'.", fileName, parent))
	}
	zippedFile, err := file.Open()
	if err != nil {
		logger.Debug(fmt.Sprintf("Error occurred while opening the zip file: %v", err))
		return nil, err
	}
	data, err := ioutil.ReadAll(zippedFile)
	if err != nil {
		logger.Debug(fmt.Sprintf("Error occurred while reading the zip file: %v", err))
		return nil, err
	}
	zippedFile.Close()

	dataString := string(data)
	dataString = util.ProcessString(dataString, "\n", true)

	//check
	regex, err := regexp.Compile(constant.PATCH_REGEX)
	allMatches := regex.FindAllStringSubmatch(dataString, -1)
	logger.Debug(fmt.Sprintf("All matches: %v", allMatches))
	isPatchWordFound := false
	if len(allMatches) > 0 {
		isPatchWordFound = true
	}
	if isPatchWordFound {
		util.PrintWarning(fmt.Sprintf("'%v' file contains the word 'patch' in following lines. Please "+
			"review and change it to 'update' if possible.", fileName))
		for i, line := range allMatches {
			util.PrintInfo(fmt.Sprintf("Matching Line #%d - %v", i+1, line[0]))
		}
		fmt.Println()
	}

	// Check whether the all placeholders are removed
	contains := strings.Contains(dataString, constant.UPDATE_NO_DEFAULT)
	if contains {
		util.PrintWarning(fmt.Sprintf("Please add the correct value for '%v' in the '%v' file.",
			constant.UPDATE_NO_DEFAULT, constant.UPDATE_DESCRIPTOR_FILE))
	}
	contains = strings.Contains(dataString, constant.PLATFORM_NAME_DEFAULT)
	if contains {
		util.PrintWarning(fmt.Sprintf("Please add the correct value for '%v' in the '%v' file.",
			constant.PLATFORM_NAME_DEFAULT, constant.UPDATE_DESCRIPTOR_FILE))
	}
	contains = strings.Contains(dataString, constant.PLATFORM_VERSION_DEFAULT)
	if contains {
		util.PrintWarning(fmt.Sprintf("Please add the correct value for '%v' in the '%v' file.",
			constant.PLATFORM_VERSION_DEFAULT, constant.UPDATE_DESCRIPTOR_FILE))
	}
	contains = strings.Contains(dataString, constant.APPLIES_TO_DEFAULT)
	if contains {
		util.PrintWarning(fmt.Sprintf("Please add the correct value for '%v' in the '%v' file.",
			constant.APPLIES_TO_DEFAULT, constant.UPDATE_DESCRIPTOR_FILE))
	}
	contains = strings.Contains(dataString, constant.DESCRIPTION_DEFAULT)
	if contains {
		util.PrintWarning(fmt.Sprintf("Please add the correct value for '%v' in the '%v' file.",
			constant.DESCRIPTION_DEFAULT, constant.UPDATE_DESCRIPTOR_FILE))
	}
	contains = strings.Contains(dataString, constant.JIRA_KEY_DEFAULT)
	if contains {
		util.PrintWarning(fmt.Sprintf("Please add the correct value for '%v' in the '%v' file.",
			constant.JIRA_KEY_DEFAULT, constant.UPDATE_DESCRIPTOR_FILE))
	}
	contains = strings.Contains(dataString, constant.JIRA_SUMMARY_DEFAULT)
	if contains {
		util.PrintWarning(fmt.Sprintf("Please add the correct value for '%v' in the '%v' file.",
			constant.JIRA_SUMMARY_DEFAULT, constant.UPDATE_DESCRIPTOR_FILE))
	}

	logger.Debug(fmt.Sprintf("Validating '%s' finished.", fileName))
	return data, nil
}

//This function reads the product distribution at the given location.
func readDistributionZip(filename string) (map[string]bool, error) {
	fileMap := make(map[string]bool)
	// Create a reader out of the zip archive
	zipReader, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	defer zipReader.Close()

	productName := viper.GetString(constant.PRODUCT_NAME)
	logger.Debug(fmt.Sprintf("productName: %s", productName))
	// Iterate through each file/dir found in
	for _, file := range zipReader.Reader.File {
		logger.Trace(file.Name)

		var relativePath string
		if (strings.Contains(file.Name, "/")) {
			relativePath = strings.SplitN(file.Name, "/", 2)[1]
		} else {
			relativePath = file.Name
		}

		if !file.FileInfo().IsDir() {
			fileMap[relativePath] = false
		}
	}
	return fileMap, nil
}

//When reading zip files in windows, file.FileInfo().Name() does not return the filename correctly
// (where file *zip.File) To fix this issue, this function was added.
func getFileName(filename string) string {
	filename = filepath.ToSlash(filename)
	if lastIndex := strings.LastIndex(filename, "/"); lastIndex > -1 {
		filename = filename[lastIndex+1:]
	}
	return filename
}
