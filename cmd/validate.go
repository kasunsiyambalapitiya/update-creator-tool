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
	"crypto/md5"
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
		the structure of the update-descriptor.yaml and update-descrjptor3.yaml files as well.
		Please set LICENSE_MD5 environment variable to the expected
		md5 value of the LICENSE.txt file.`)
)

// ValidateCmd represents the validate command
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

// This function will be called when the validate command is called.
func initializeValidateCommand(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		util.HandleErrorAndExit(errors.New("invalid number of arguments. Run 'wum-uc validate --help' to " +
			"view help"))
	}
	startValidation(args[0], args[1])
}

// This function will start the validation process.
func startValidation(updateFilePath, distributionLocation string) {

	// Sets the log level
	setLogLevel()
	logger.Debug("validate command called")
	fmt.Println("Validating update ...")

	updateFileMap := make(map[string]bool)
	distributionFileMap := make(map[string]bool)

	// Checks whether the update has the zip extension
	util.IsZipFile(constant.UPDATE, updateFilePath)

	// Checks whether the update file exists
	exists, err := util.IsFileExists(updateFilePath)
	util.HandleErrorAndExit(err, "")
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("Entered update file does not exist at '%s'.",
			updateFilePath)))
	}

	// Checks whether the given distribution is a zip file
	util.IsZipFile(constant.DISTRIBUTION, distributionLocation)

	// Sets the product name in viper configs
	lastIndex := strings.LastIndex(distributionLocation, constant.PATH_SEPARATOR)
	productName := strings.TrimSuffix(distributionLocation[lastIndex+1:], ".zip")
	logger.Debug(fmt.Sprintf("Setting ProductName: %s", productName))
	viper.Set(constant.PRODUCT_NAME, productName)

	// Checks whether the distribution file exists
	exists, err = util.IsFileExists(distributionLocation)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while checking '%s'", distributionLocation))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("Entered distribution file does not exist at '%s'.",
			distributionLocation)))
	}

	// Checks update filename
	locationInfo, err := os.Stat(updateFilePath)
	util.HandleErrorAndExit(err, "Error occurred while getting the information of update file")
	match, err := regexp.MatchString(constant.FILENAME_REGEX, locationInfo.Name())
	if !match {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("Update filename '%s' does not match '%s' regular "+
			"expression.", locationInfo.Name(), constant.FILENAME_REGEX)))
	}

	// Sets the update name in viper configs
	updateName := strings.TrimSuffix(locationInfo.Name(), ".zip")
	viper.Set(constant.UPDATE_NAME, updateName)

	// Reads the update zip file
	updateFileMap, updateDescriptorV2, err := readUpdateZip(updateFilePath)
	util.HandleErrorAndExit(err)
	logger.Trace(fmt.Sprintf("updateFileMap: %v\n", updateFileMap))

	// Reads the distribution zip file
	distributionFileMap, err = readDistributionZip(distributionLocation)
	util.HandleErrorAndExit(err)
	logger.Trace(fmt.Sprintf("distributionFileMap: %v\n", distributionFileMap))

	// Compares the update with the provided distribution only if update-descriptor.yaml exists
	if updateDescriptorV2.UpdateNumber != "" {
		err = compare(updateFileMap, distributionFileMap, updateDescriptorV2)
		util.HandleErrorAndExit(err)
	}
	fmt.Println("'" + updateName + "' validation successfully finished.")
}

// This function compares the files in the update and the provided distribution.
func compare(updateFileMap, distributionFileMap map[string]bool, updateDescriptorV2 *util.UpdateDescriptorV2) error {
	updateName := viper.GetString(constant.UPDATE_NAME)
	for filePath := range updateFileMap {
		logger.Debug(fmt.Sprintf("Searching: %s", filePath))
		_, found := distributionFileMap[filePath]
		if !found {
			logger.Debug("Added files: ", updateDescriptorV2.FileChanges.AddedFiles)
			isInAddedFiles := util.IsStringIsInSlice(filePath, updateDescriptorV2.FileChanges.AddedFiles)
			logger.Debug(fmt.Sprintf("isInAddedFiles: %v", isInAddedFiles))
			resourceFiles := getResourceFiles()
			logger.Debug(fmt.Sprintf("resourceFiles: %v", resourceFiles))
			fileName := strings.TrimPrefix(filePath, updateName+"/")
			logger.Debug(fmt.Sprintf("fileName: %s", fileName))
			_, foundInResources := resourceFiles[fileName]
			logger.Debug(fmt.Sprintf("found in resources: %v", foundInResources))
			//check
			if !isInAddedFiles && !foundInResources {
				return errors.New(fmt.Sprintf("File not found in the distribution: '%v'. If this is "+
					"a new file, add an entry to the 'added_files' sections in the '%v' file",
					filePath, constant.UPDATE_DESCRIPTOR_V2_FILE))
			} else {
				logger.Debug("'" + filePath + "' found in added files.")
			}
		}
	}
	return nil
}

// This function will read the update zip at the the given location.
func readUpdateZip(filename string) (map[string]bool, *util.UpdateDescriptorV2, error) {
	fileMap := make(map[string]bool)
	updateDescriptorV2 := util.UpdateDescriptorV2{}
	updateDescriptorV3 := util.UpdateDescriptorV3{}

	isNotAContributionFileFound := false
	isASecPatch := false

	// Create a reader out of the zip archive
	zipReader, err := zip.OpenReader(filename)
	if err != nil {
		return nil, nil, err
	}
	defer zipReader.Close()

	updateName := viper.GetString(constant.UPDATE_NAME)
	logger.Debug("UpdateName:", updateName)
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
			case constant.UPDATE_DESCRIPTOR_V2_FILE:
				data, err := validateFile(file, constant.UPDATE_DESCRIPTOR_V2_FILE, fullPath, updateName)
				if err != nil {
					return nil, nil, err
				}
				err = yaml.Unmarshal(data, &updateDescriptorV2)
				if err != nil {
					return nil, nil, err
				}
				//check
				err = util.ValidateUpdateDescriptorV2(&updateDescriptorV2)
				if err != nil {
					return nil, nil, errors.New("'" + constant.UPDATE_DESCRIPTOR_V2_FILE +
						"' is invalid. " + err.Error())
				}
			case constant.UPDATE_DESCRIPTOR_V3_FILE:
				data, err := validateFile(file, constant.UPDATE_DESCRIPTOR_V3_FILE, fullPath, updateName)
				if err != nil {
					return nil, nil, err
				}
				err = yaml.Unmarshal(data, &updateDescriptorV3)
				if err != nil {
					return nil, nil, err
				}
				//check
				err = util.ValidateUpdateDescriptorV3(&updateDescriptorV3)
				if err != nil {
					return nil, nil, errors.New("'" + constant.UPDATE_DESCRIPTOR_V3_FILE +
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
				resourceFiles := getResourceFiles()
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
	return fileMap, &updateDescriptorV2, nil
}

// This function will validate the provided file. If the word 'patch' is found, a warning message is printed.
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
	// Validate checksum of the LICENSE.txt file.
	if fileName == constant.LICENSE_FILE {
		err := validateMD5(fileName, parent, constant.LICENSE_MD5_URL, constant.LICENSE_MD5, data)
		if err != nil {
			return nil, err
		}
	}
	// Validate checksum of the NOT_A_CONTRIBUTION.txt file.
	if fileName == constant.NOT_A_CONTRIBUTION_FILE {
		err := validateMD5(fileName, parent, constant.NOT_A_CONTRIBUTION_MD5_URL, constant.NOT_A_CONTRIBUTION_MD5, data)
		if err != nil {
			return nil, err
		}
	}
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

	logger.Debug(fmt.Sprintf("Validating '%s' finished.", fileName))
	return data, nil
}

// This function reads the product distribution at the given location.
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

		relativePath := util.GetRelativePath(file)

		if !file.FileInfo().IsDir() {
			fileMap[relativePath] = false
		}
	}
	return fileMap, nil
}

// When reading zip files in windows, file.FileInfo().Name() does not return the filename correctly
// (where file *zip.File) To fix this issue, this function was added.
func getFileName(filename string) string {
	filename = filepath.ToSlash(filename)
	if lastIndex := strings.LastIndex(filename, "/"); lastIndex > -1 {
		filename = filename[lastIndex+1:]
	}
	return filename
}

func validateMD5(fileName, parent, md5DownloadUrl, md5hashName string, data []byte) error {
	logger.Debug(fmt.Sprintf("Checking MD5 of the '%s'", fileName))
	actualMD5Sum := fmt.Sprintf("%x", md5.Sum(data))
	expectedMD5Sum, exists := os.LookupEnv(md5hashName)
	if !exists {
		expectedMD5SumByte, err := util.GetContentFromUrl(md5DownloadUrl)
		if err != nil {
			util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while getting md5 from: %s.",
				md5DownloadUrl))
		}
		expectedMD5Sum = strings.ToLower(string(expectedMD5SumByte))
	}
	if actualMD5Sum != expectedMD5Sum {
		logger.Debug(fmt.Sprintf("MD5 checksum failed for the file '%s': "+
			"Expected-'%s', Actual-'%s'", fileName, expectedMD5Sum, actualMD5Sum))
		return errors.New(fmt.Sprintf("'%s' in '%s' is invalid.", fileName, parent))
	}
	return nil
}
