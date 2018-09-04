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
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"bytes"
	"github.com/olekukonko/tablewriter"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wso2/update-creator-tool/constant"
	"github.com/wso2/update-creator-tool/util"
	"gopkg.in/yaml.v2"
	"os/exec"
	"regexp"
	"syscall"
	time2 "time"
)

// This struct is used to store file/directory information.
type data struct {
	name         string
	isDir        bool
	relativePath string
	md5          string
}

// This struct used to store directory structure of the distribution.
type node struct {
	name             string
	isDir            bool
	relativeLocation string
	parent           *node
	childNodes       map[string]*node
	md5Hash          string
}

// This struct is used for resuming the update creation using `wum-uc create -- continue`
type resumeFile struct {
	explodedUpdateDirectoryPath string
	developer                   string
	updateName                  string
	resourceDirectoryPath       string
	distributionPath            string
	platformName                string
	timestamp                   int
	updateNumber                string
	isUpdateZipCreated          bool
}

// This is used to create a new node which will initialize the childNodes map.
func createNewNode() node {
	return node{
		childNodes: make(map[string]*node),
	}
}

// Values used to print help command.
var (
	createCmdUse       = "create <update_dir> <dist_loc>"
	createCmdShortDesc = "Create a new update"
	createCmdLongDesc  = dedent.Dedent(`
		This command will create a new update zip file from the files in the
		given directory. To generate the directory structure, it requires the
		product distribution zip file path as input.`)
)

// createCmd represents the create command.
var createCmd = &cobra.Command{
	Use:   createCmdUse,
	Short: createCmdShortDesc,
	Long:  createCmdLongDesc,
	Run:   initializeCreateCommand,
}

var isContinueEnabled = false
var wumucResumeFile = filepath.Join(constant.WUM_UC_HOME, constant.WUMUC_RESUME_FILE)

// This function will be called first and this will add flags to the command.
func init() {
	RootCmd.AddCommand(createCmd)

	createCmd.Flags().BoolVarP(&isDebugLogsEnabled, "debug", "d", util.EnableDebugLogs, "Enable debug logs")
	createCmd.Flags().BoolVarP(&isTraceLogsEnabled, "trace", "t", util.EnableTraceLogs, "Enable trace logs")
	createCmd.Flags().BoolVar(&isContinueEnabled, "continue", false, "Continue resumed update creation")

	createCmd.Flags().BoolP("md5", "m", util.CheckMd5Disabled, "Disable checking MD5 sum")
	viper.BindPFlag(constant.CHECK_MD5_DISABLED, createCmd.Flags().Lookup("md5"))
}

// This function will be called when the create command is called.
func initializeCreateCommand(cmd *cobra.Command, args []string) {

	// Check for resuming the update creation or creating the update from scratch
	if !isContinueEnabled {
		if len(args) != 2 {
			util.HandleErrorAndExit(errors.New("invalid number of arguments. Run 'wum-uc create --help' to " +
				"view help"))
		}
		createUpdate(args[0], args[1])
	} else {
		//continueResumedUpdateCreation()
		//---------- Testing -----------
		commitUpdateToSVN()
		//---------- Testing End -----------

	}
}

// This function will start the update creation process.
func createUpdate(updateDirectoryPath, distributionPath string) {

	// set debug level
	setLogLevel()
	logger.Debug("[create] command called")

	// Flow - First check whether the given locations exist and required files exist,
	// create them if they are not available. Then start processing.
	// If one step fails, print the error message and exit.

	//1) Check whether the given update directory exists
	exists, err := util.IsDirectoryExists(updateDirectoryPath)
	util.HandleErrorAndExit(err, "Error occurred while reading the update directory")
	logger.Debug(fmt.Sprintf("Directory %s exists: %v", updateDirectoryPath, exists))
	if !exists {
		// If the directory does not exists, prompt the user
	userInputLoop:
		for {
			util.PrintInBold(fmt.Sprintf("'%s'does not exists. Do you want to create '%s' directory?"+
				"[Y/n]: ", updateDirectoryPath, updateDirectoryPath))
			preference, err := util.GetUserInput()
			util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
			if len(preference) == 0 {
				preference = "y"
			}

			// Get the user preference
			userPreference := util.ProcessUserPreference(preference)
			switch userPreference {
			case constant.YES:
				util.PrintInfo(fmt.Sprintf("'%s' directory does not exist. Creating '%s' directory.",
					updateDirectoryPath, updateDirectoryPath))
				err := util.CreateDirectory(updateDirectoryPath)
				util.HandleErrorAndExit(err)
				logger.Debug(fmt.Sprintf("'%s' directory created.", updateDirectoryPath))
				break userInputLoop
			case constant.NO:
				util.HandleErrorAndExit(errors.New("directory creation skipped. Please enter a valid directory"))
			default:
				util.PrintError("Invalid preference. Enter Y for Yes or N for No.")
			}
		}
		util.PrintInBold(fmt.Sprintf("Directory created. Please copy updated files to '%s' and rerun 'wum-uc create'", updateDirectoryPath))
		os.Exit(1)
	}
	updateRoot := strings.TrimSuffix(updateDirectoryPath, constant.PATH_SEPARATOR)
	logger.Debug(fmt.Sprintf("updateRoot: %s\n", updateRoot))
	viper.Set(constant.UPDATE_ROOT, updateRoot)

	// Create new update descriptor structs
	updateDescriptorV2 := util.UpdateDescriptorV2{}
	updateDescriptorV3 := util.UpdateDescriptorV3{}

	//2) Process the README.txt file if it exists
	readMeDataString := processReadMe(updateDirectoryPath, &updateDescriptorV2)

	//3) Check whether the given distribution exists
	exists, err = util.IsFileExists(distributionPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while checking '%s'", distributionPath))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("File does not exist at '%s'. Distribution must "+
			"be a zip file.", distributionPath)))
	}
	// Checks whether the given distribution is a zip file
	util.IsZipFile(constant.DISTRIBUTION, distributionPath)

	//4) Set the update name
	updateName := getUpdateName(&updateDescriptorV2, constant.UPDATE_NAME_PREFIX)
	viper.Set(constant.UPDATE_NAME, updateName)

	//5) Validate UpdateDescriptorV2 for basic details of update-descriptor.yaml
	err = util.ValidateBasicDetailsOfUpdateDescriptorV2(&updateDescriptorV2)
	util.HandleErrorAndExit(err, fmt.Sprintf("'%s' format is incorrect.", constant.UPDATE_DESCRIPTOR_V2_FILE))

	//6) Download mandatory files
	// Download the LICENSE.txt
	downloadFile(updateDirectoryPath, constant.LICENSE_URL, constant.LICENSE_DOWNLOAD_URL, constant.LICENSE_FILE)
	// Download the NOT_A_CONTRIBUTION.txt
	downloadFile(updateDirectoryPath, constant.NOT_A_CONTRIBUTION_URL, constant.NOT_A_CONTRIBUTION_DOWNLOAD_URL,
		constant.NOT_A_CONTRIBUTION_FILE)

	// Get ignored files. These files wont be stored in the data structure. So matches will not be searched for
	// these files
	ignoredFiles := getIgnoredFilesInUpdate()
	logger.Debug(fmt.Sprintf("Ignored files: %v", ignoredFiles))

	//7) Traverse and read the update

	// allFilesMap - Map which contains details of all files in the directory. Key will be relativePath of the file.
	// rootLevelDirectoriesMap - Map which have all directories in the root of the given directory. Key will be the
	// 		    	     directory path.
	// rootLevelFilesMap - Map which have all files in the root of the given directory. Key will be the file path.
	allFilesMap, rootLevelDirectoriesMap, rootLevelFilesMap, err := readDirectory(updateDirectoryPath, ignoredFiles)
	util.HandleErrorAndExit(err, "Error occurred while reading update directory.")

	logger.Debug(fmt.Sprintf("allFilesMap: %v\n", allFilesMap))
	logger.Debug(fmt.Sprintf("rootLevelDirectoriesMap: %v\n", rootLevelDirectoriesMap))
	logger.Debug(fmt.Sprintf("rootLevelFilesMap: %v\n", rootLevelFilesMap))

	// rootNode is what we use as the root of the distribution when we populate tree like structure.
	rootNode := createNewNode()

	// Get the product name from the distribution path and set it as a viper config
	paths := strings.Split(distributionPath, constant.PATH_SEPARATOR)
	distributionName := strings.TrimSuffix(paths[len(paths)-1], ".zip")
	viper.Set(constant.PRODUCT_NAME, distributionName)

	// Read the distribution zip file
	logger.Debug("Reading zip")
	fmt.Println(fmt.Sprintf("\nReading %s. Please wait...\n", distributionName))
	rootNode, err = readZip(distributionPath)
	util.HandleErrorAndExit(err)
	logger.Debug("Reading zip finished")

	logger.Trace("Top level nodes ---------------------")
	for name, node := range rootNode.childNodes {
		logger.Trace(fmt.Sprintf("%s: %v", name, node))
	}
	logger.Trace("-------------------------------------")

	// Create an interrupt handler
	cleanupChannel := util.HandleInterrupts(func() {
		util.CleanUpDirectory(constant.TEMP_DIR)
		util.CleanUpFile(wumucResumeFile)
	})

	//todo: save the selected location to generate the final summary map
	//8) Find matches
	// This will be used to store all the matches (matching locations in for the given directory)
	matches := make(map[string]*node)
	// Find matches in the distribution for all directories in the root level of the update directory
	logger.Debug("Checking Directories:")
	for directoryName := range rootLevelDirectoriesMap {
		matches = make(map[string]*node)
		// Find all matching locations for the directory
		logger.Debug(fmt.Sprintf("DirectoryName: %s", directoryName))
		FindMatches(&rootNode, directoryName, true, matches)
		logger.Debug(fmt.Sprintf("matches: %v", matches))

		// Now we can act according to the number of matches we found
		switch len(matches) {
		// No match found in the distribution for the given directory
		case 0:
			// Handle the no match situation
			logger.Debug("\nNo match found\n")
			err := handleNoMatch(directoryName, true, allFilesMap, &rootNode, &updateDescriptorV2)
			util.HandleErrorAndExit(err)
			// Single match found in the distribution for the given directory
		case 1:
			// Handle the single match situation
			logger.Debug("\nSingle match found\n")
			// Get the matching node from the map. For this, we need to iterate through the map. Map size
			// will always be 1 because we check the size above.
			var match *node
			for _, node := range matches {
				match = node
			}
			err := handleSingleMatch(directoryName, match, true, allFilesMap, &rootNode, &updateDescriptorV2)
			util.HandleErrorAndExit(err)
			// Multiple matches found in the distribution for the given directory
		default:
			// Handle the multiple matches situation
			logger.Debug("\nMultiple matches found\n")
			err := handleMultipleMatches(directoryName, true, matches, allFilesMap, &rootNode,
				&updateDescriptorV2)
			util.HandleErrorAndExit(err)
		}
	}

	// Find matches in the distribution for all files in the root level of the update directory
	logger.Debug("Checking Files:")
	for fileName := range rootLevelFilesMap {
		matches = make(map[string]*node)
		// Find all matching locations for the file
		logger.Debug(fmt.Sprintf("FileName: %s", fileName))
		FindMatches(&rootNode, fileName, false, matches)
		logger.Debug(fmt.Sprintf("matches: %v", matches))

		// Now we can act according to the number of matches we found
		switch len(matches) {
		// No match found in the distribution for the given file
		case 0:
			// Handle the no match situation
			logger.Debug("No match found\n")
			err := handleNoMatch(fileName, false, allFilesMap, &rootNode, &updateDescriptorV2)
			util.HandleErrorAndExit(err)
			// Single match found in the distribution for the given file
		case 1:
			// Handle the single match situation
			logger.Debug("Single match found\n")
			// Get the matching node from the map. For this, we need to iterate through the map. Map size
			// will always be 1 because we check the size above.
			var match *node
			for _, node := range matches {
				match = node
			}
			err := handleSingleMatch(fileName, match, false, allFilesMap, &rootNode, &updateDescriptorV2)
			util.HandleErrorAndExit(err)
			// Multiple matches found in the distribution for the given file
		default:
			// Handle the multiple matches situation
			logger.Debug("Multiple matches found\n")
			err := handleMultipleMatches(fileName, false, matches, allFilesMap, &rootNode, &updateDescriptorV2)
			util.HandleErrorAndExit(err)
		}
	}

	//9) Request the user to add removed files as they can't be identified by comparing.
removedFilesInputLoop:
	for {
		util.PrintInBold(fmt.Sprintf("\nAre the existing files in %s removed from this update? [y"+
			"/n]: ",
			distributionName))
		preference, err := util.GetUserInput()
		util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
		userPreference := util.ProcessUserPreference(preference)
		switch userPreference {
		case constant.YES:
			appendRemovedFilesToUpdateDescriptor(&updateDescriptorV2)
			break removedFilesInputLoop
		case constant.NO:
			break removedFilesInputLoop
		default:
			util.PrintError("Invalid preference. Enter y for Yes or n for No.")
		}
	}

	// Get partial updated file changes
	partialUpdatedFileResponse := util.GetPartialUpdatedFiles(&updateDescriptorV2)
	if partialUpdatedFileResponse.BackwardCompatible {
		// Create update-descriptor.yaml
		if len(readMeDataString) != 0 {
			processReadMeData(&readMeDataString, &updateDescriptorV2)
		} else {
			setRemainingValuesInUpdateDescriptorsV2(&updateDescriptorV2)
		}
		createUpdateDescriptorV2(updateDirectoryPath, &updateDescriptorV2)
		data, err := marshalUpdateDescriptor(&updateDescriptorV2)
		util.HandleErrorAndExit(err, "Error occurred while marshalling the update-descriptorV2.")
		// Save the updated update-descriptor.yaml with newly added, modified and removed files to the temp directory
		err = saveUpdateDescriptor(constant.UPDATE_DESCRIPTOR_V2_FILE, data)
		util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while saving the '%v'.",
			constant.UPDATE_DESCRIPTOR_V2_FILE))
	}

	// Set values for UpdateDescriptorV3
	updateDescriptorV3.UpdateNumber = partialUpdatedFileResponse.UpdateNumber
	updateDescriptorV3.PlatformName = partialUpdatedFileResponse.PlatformName
	updateDescriptorV3.PlatformVersion = partialUpdatedFileResponse.PlatformVersion
	updateDescriptorV3.Description = constant.DEFAULT_DESCRIPTION
	updateDescriptorV3.Instructions = constant.DEFAULT_INSTRUCTIONS
	defaultBugFixes := map[string]string{
		constant.DEFAULT_JIRA_KEY: constant.DEFAULT_JIRA_SUMMARY,
	}
	updateDescriptorV3.BugFixes = defaultBugFixes

	for _, partialUpdatedProducts := range partialUpdatedFileResponse.CompatibleProducts {
		productChanges := setProductChangesInUpdateDescriptorV3(&partialUpdatedProducts)
		updateDescriptorV3.CompatibleProducts = append(updateDescriptorV3.CompatibleProducts, *productChanges)
	}
	for _, partialUpdatedProducts := range partialUpdatedFileResponse.PartiallyApplicableProducts {
		productChanges := setProductChangesInUpdateDescriptorV3(&partialUpdatedProducts)
		updateDescriptorV3.PartiallyApplicableProducts = append(updateDescriptorV3.PartiallyApplicableProducts, *productChanges)
	}

	// Generate md5sum for product changes
	updateDescriptorV3.Md5sum = util.GenerateMd5sumForFileChanges(&updateDescriptorV3)

	// Set values to compatible products slice for displaying purpose
	var compatibleProducts []string
	for _, productChange := range updateDescriptorV3.CompatibleProducts {
		productId := productChange.ProductName + "-" + productChange.ProductVersion
		compatibleProducts = append(compatibleProducts, productId)
	}
	// Set values to partially applicable products slice for displaying purpose
	var partiallyApplicableProducts []string
	for _, productChange := range updateDescriptorV3.PartiallyApplicableProducts {
		productId := productChange.ProductName + "-" + productChange.ProductVersion
		partiallyApplicableProducts = append(partiallyApplicableProducts, productId)
	}
	// Set values to notify products slice for displaying purpose
	var notifyProducts []string
	for _, partialUpdatedProducts := range partialUpdatedFileResponse.NotifyProducts {
		productId := partialUpdatedProducts.ProductName + "-" + partialUpdatedProducts.BaseVersion
		notifyProducts = append(notifyProducts, productId)
	}

	//10) Copy resource files (LICENSE.txt, etc) to temp directory
	resourceFiles := getResourceFiles()
	err = copyResourceFilesToTempDir(resourceFiles)
	util.HandleErrorAndExit(err, errors.New("error occurred while copying resource files"))
	// Create update-descriptor3.yaml in user given update directory
	createUpdateDescriptorV3(updateDirectoryPath, &updateDescriptorV3)

	explodedUpdateDirectory := path.Join(constant.TEMP_DIR, updateName)
	explodedUpdateDirectory = strings.Replace(explodedUpdateDirectory, "/", constant.PATH_SEPARATOR, -1)

	logger.Debug(fmt.Sprintf("Exploded update directory: %s", explodedUpdateDirectory))
	WUMUCConfig := util.GetWUMUCConfigs()
	absUpdateDirectoryPath, err := filepath.Abs(updateDirectoryPath)
	if err != nil {
		absUpdateDirectoryPath = updateDirectoryPath
	}

	// Set details of the update to resumeFile struct for resuming update creation
	logger.Debug(fmt.Sprintf("Setting values to %s for resuming update creation", constant.WUMUC_RESUME_FILE))
	resumeFile := resumeFile{}
	resumeFile.explodedUpdateDirectoryPath = explodedUpdateDirectory
	resumeFile.updateName = updateName
	resumeFile.distributionPath = distributionPath
	resumeFile.resourceDirectoryPath = absUpdateDirectoryPath
	resumeFile.developer = WUMUCConfig.Username
	resumeFile.platformName = updateDescriptorV3.PlatformName
	resumeFile.updateNumber = updateDescriptorV3.UpdateNumber

	// Write resumeFile struct to a file
	saveResumeFile(&resumeFile)

	// clean un temp file
	signal.Stop(cleanupChannel)

	util.PrintInBold(fmt.Sprintf("Your update applies to the following products\n"))
	util.PrintInBold(fmt.Sprintf("\tCompatible products : %v \n", compatibleProducts))
	util.PrintInBold(fmt.Sprintf("\tPartially applicable products : %v \n", partiallyApplicableProducts))
	util.PrintInBold(fmt.Sprintf("\tNotify products : %v \n", notifyProducts))

	util.PrintInBold(fmt.Sprintf("Manually fill the `description`,"+
		"`instructions` and `bug_fixes` fields for above products in the update-descriptor3."+
		"yaml located inside %s directory\n", updateDirectoryPath))
	util.PrintInBold(fmt.Sprintf("When done please run 'wum-uc create --continue' to resume the update creation.\n"))
}

// This function will process the README.txt file and extract basic details of the update to populate the update
// -descriptor.yaml.
// If some data cannot be extracted, it will add default values and continue.
func processReadMe(updateDirectoryPath string, updateDescriptorV2 *util.UpdateDescriptorV2) string {
	logger.Debug("Processing README.txt started for filling in `update_number`," +
		"`platform_name` and `platform_version` in update-descriptor.yaml")
	// Construct the README.txt path
	readMePath := path.Join(updateDirectoryPath, constant.README_FILE)
	logger.Debug(fmt.Sprintf("README.txt Path: %v", readMePath))
	// Check whether the README.txt file exists
	_, err := os.Stat(readMePath)
	if err != nil {
		// If the file does not exist or any other error occur, return without printing warning messages
		logger.Debug(fmt.Sprintf("%s not found", readMePath))
		setBasicValuesInUpdateDescriptorV2(updateDescriptorV2)
		return ""
	}
	logger.Debug("README.txt found")
	// Read the README.txt file
	data, err := ioutil.ReadFile(readMePath)
	if err != nil {
		// If any error occurs, return without printing warning messages
		logger.Debug(fmt.Sprintf("Error occurred in processing README.txt: %v", err))
		setBasicValuesInUpdateDescriptorV2(updateDescriptorV2)
		return ""
	}
	// Convert the byte array to a string
	readMeDataString := string(data)
	logger.Debug("Processing README started")
	// Compile the regex
	regex, err := regexp.Compile(constant.PATCH_ID_REGEX)
	if err == nil {
		result := regex.FindStringSubmatch(readMeDataString)
		logger.Trace(fmt.Sprintf("PATCH_ID_REGEX result: %v", result))
		// Since the regex has 2 capturing groups, the result size will be 3 (because there is the full match)
		// If not match found, the size will be 0. We check whether the result size is not 0 to make sure both
		// capturing groups are identified.
		if len(result) != 0 {
			// Extract details
			updateDescriptorV2.UpdateNumber = result[2]
			updateDescriptorV2.PlatformVersion = result[1]
			platformsMap := viper.GetStringMapString(constant.PLATFORM_VERSIONS)
			logger.Trace(fmt.Sprintf("Platform Map: %v", platformsMap))
			// Get the platform details from the map
			platformName, found := platformsMap[result[1]]
			if found {
				logger.Debug("Platform name found in configs")
				updateDescriptorV2.PlatformName = platformName
			} else {
				//If the platform name is not found, request the user
				logger.Debug("No matching platform name found for:", result[1])
				util.PrintInBold("Enter platform name for platform version :", result[1])
				platformName, err := util.GetUserInput()
				util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
				updateDescriptorV2.PlatformName = platformName
			}
		} else {
			logger.Debug("PATCH_ID_REGEX results incorrect:", result)
			setBasicValuesInUpdateDescriptorV2(updateDescriptorV2)
		}
	} else {
		//If error occurred, set default values
		logger.Debug(fmt.Sprintf("Error occurred while processing PATCH_ID_REGEX: %v", err))
		setBasicValuesInUpdateDescriptorV2(updateDescriptorV2)
	}
	return readMeDataString
}

//This function will set basic values in the update-descriptor.yaml.
func setBasicValuesInUpdateDescriptorV2(updateDescriptorV2 *util.UpdateDescriptorV2) {
	logger.Debug("Setting values for `update_number`," +
		"`platform_version` and `platform_name` fields in update-descriptor." +
		"yaml")
	setUpdateNumber(updateDescriptorV2)
	setPlatformNameAndVersion(updateDescriptorV2)
}

// Process readme data for filling in remaining details of update-descriptor.yaml
func processReadMeData(readMeDataString *string, updateDescriptorV2 *util.UpdateDescriptorV2) {
	logger.Debug("Processing README.txt started for filling in `applies_to`," +
		"`bug_fixes` and `description` in update-descriptor.yaml")

	// Compile the regex
	regex, err := regexp.Compile(constant.APPLIES_TO_REGEX)
	if err == nil {
		result := regex.FindStringSubmatch(*readMeDataString)
		logger.Trace(fmt.Sprintf("APPLIES_TO_REGEX result: %v", result))
		// In the README, Associated Jiras section might not appear. If it does appear, result size will be 2.
		// If it does not appear, result size will be 3.
		if len(result) == 2 {
			// If the result size is 2, we know that 1st index contains the 1st capturing group.
			updateDescriptorV2.AppliesTo = util.ProcessString(result[1], ", ", true)
		} else if len(result) == 3 {
			// If the result size is 3, 1st or 2nd string might contain the match. So we concat them
			// together and trim the spaces. If one field has an empty string, it will be trimmed.
			updateDescriptorV2.AppliesTo = util.ProcessString(strings.TrimSpace(result[1]+result[2]), ", ",
				true)
		} else {
			logger.Debug("No matching results found for APPLIES_TO_REGEX:", result)
			setAppliesTo(updateDescriptorV2)
		}
	} else {
		// If error occurred, request user to fill in
		logger.Debug(fmt.Sprintf("Error occurred while processing APPLIES_TO_REGEX: %v", err))
		setAppliesTo(updateDescriptorV2)
	}

	// Compile the regex
	regex, err = regexp.Compile(constant.ASSOCIATED_JIRAS_REGEX)
	if err == nil {
		// Get all matches because there might be multiple Jiras.
		allResult := regex.FindAllStringSubmatch(*readMeDataString, -1)
		logger.Trace(fmt.Sprintf("APPLIES_TO_REGEX result: %v", allResult))
		updateDescriptorV2.BugFixes = make(map[string]string)
		// If no Jiras found, set 'N/A: N/A' as the value
		if len(allResult) == 0 {
			logger.Debug("No matching results found for ASSOCIATED_JIRAS_REGEX.")
			setBugFixes(updateDescriptorV2)
		} else {
			// If Jiras found, get summary for all Jiras
			logger.Debug("Matching results found for ASSOCIATED_JIRAS_REGEX")
			for i, match := range allResult {
				// Regex has a one capturing group. So the jira ID will be in the 1st index.
				logger.Debug(fmt.Sprintf("%d: %s", i, match[1]))
				logger.Debug(fmt.Sprintf("ASSOCIATED_JIRAS_REGEX results is correct: %v", match))
				updateDescriptorV2.BugFixes[match[1]] = util.GetJiraSummary(match[1])
			}
		}
	} else {
		// If error occurred, request user to fill in
		logger.Debug(fmt.Sprintf("Error occurred while processing ASSOCIATED_JIRAS_REGEX: %v", err))
		setBugFixes(updateDescriptorV2)
	}

	// Compile the regex
	regex, err = regexp.Compile(constant.DESCRIPTION_REGEX)
	if err == nil {
		// Get the match
		result := regex.FindStringSubmatch(*readMeDataString)
		logger.Trace(fmt.Sprintf("DESCRIPTION_REGEX result: %v", result))
		// If there is a match, process it and store it
		if len(result) != 0 {
			updateDescriptorV2.Description = util.ProcessString(result[1], "\n", false)
		} else {
			logger.Debug(fmt.Sprintf("No matching results found for DESCRIPTION_REGEX: %v", result))
			setDescription(updateDescriptorV2)
		}
	} else {
		// If error occurred, request user to fill in
		logger.Debug(fmt.Sprintf("Error occurred while processing DESCRIPTION_REGEX: %v", err))
		setDescription(updateDescriptorV2)
	}
	logger.Debug("Processing README finished")
}

//This function will set remaining values in the update-descriptor.yaml.
func setRemainingValuesInUpdateDescriptorsV2(updateDescriptorV2 *util.UpdateDescriptorV2) {
	logger.Debug("Setting values for `applies_to`,`bug_fixes` and `description` fields in update-descriptor." +
		"yaml")
	setAppliesTo(updateDescriptorV2)
	setBugFixes(updateDescriptorV2)
	setDescription(updateDescriptorV2)
}

// Sets the update number in update-descriptor.yaml
func setUpdateNumber(updateDescriptorV2 *util.UpdateDescriptorV2) {
	var updateNumber string
	for {
		util.PrintInBold("Enter 'update number': ")
		updateNum, err := util.GetUserInput()
		util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
		if len(updateNum) == 0 {
			util.PrintError(fmt.Sprintf("'update number' is empty"))
			continue
		}
		if !util.ValidateUpdateNumber(updateNum) {
			util.PrintError(fmt.Sprintf("'update number' is not valid. It should match '%s'.",
				constant.UPDATE_NUMBER_REGEX))
			continue
		}
		updateNumber = updateNum
		break
	}
	updateDescriptorV2.UpdateNumber = updateNumber
}

// Sets the platform name and version in update-descriptor.yaml
func setPlatformNameAndVersion(updateDescriptorV2 *util.UpdateDescriptorV2) {
userInputLoop:
	for {
		util.PrintInBold(fmt.Sprintf("Select the platform name and version from following: \n"))
		util.PrintInBold(fmt.Sprintf("\t1. wilkes \t 4.4.0\n"))
		util.PrintInBold(fmt.Sprintf("\t2. hamming \t 5.0.0\n"))
		util.PrintInBold(fmt.Sprintf("Enter your preference [1/2]: "))
		userInput, err := util.GetUserInput()
		if err != nil {
			util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
		}
		preference, err := strconv.Atoi(userInput)
		if err != nil {
			util.HandleErrorAndExit(err, "Error occurred while casting the user input to int")
		}
		switch preference {
		case 1:
			updateDescriptorV2.PlatformName = "wilkes"
			updateDescriptorV2.PlatformVersion = "4.4.0"
			fmt.Println(fmt.Sprintf("platform name: 'wilkes' and platform version: '4.4.0' selected\n"))
			break userInputLoop
		case 2:
			updateDescriptorV2.PlatformName = "hamming"
			updateDescriptorV2.PlatformVersion = "5.0.0"
			fmt.Println(fmt.Sprintf("platform name: 'hamming' and platform version: '5.0.0' selected\n"))
			break userInputLoop
		default:
			util.PrintError("Invalid input")
			continue
		}
	}
}

// Sets the applies to in update-descriptor.yaml
func setAppliesTo(updateDescriptorV2 *util.UpdateDescriptorV2) {
	util.PrintInBold(fmt.Sprintf("\nEnter applies to: "))
	appliesTo, err := util.GetUserInput()
	util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
	updateDescriptorV2.AppliesTo = appliesTo
}

// Sets the description in update-descriptor.yaml
func setDescription(updateDescriptorV2 *util.UpdateDescriptorV2) {
	util.PrintInBold(fmt.Sprintf("\nEnter the description: "))
	description, err := util.GetUserInput()
	fmt.Println()
	util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
	updateDescriptorV2.Description = description
}

// Sets the bug fixes in update-descriptor.yaml
func setBugFixes(updateDescriptorV2 *util.UpdateDescriptorV2) {
	util.PrintInBold("Enter Bug fixes,")
	fmt.Println()
	bugFixes := make(map[string]string)
userInputLoop:
	for {
		util.PrintInBold(fmt.Sprintf("\tEnter JIRA_KEY/GITHUB ISSUE URL: "))
		jiraKey, err := util.GetUserInput()
		util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
		if jiraKey == "" {
			if len(bugFixes) == 0 {
				util.PrintErrorWithTab("Empty input detected, please enter a valid JIRA_KEY/GITHUB ISSUE URL")
				continue
			}
			util.PrintInBold(fmt.Sprintf("\tEmpty input detected, are you done with adding bug fixes? [y/n]: "))
			preference, err := util.GetUserInput()
			util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
			userPreference := util.ProcessUserPreference(preference)
			switch userPreference {
			case constant.YES:
				break userInputLoop
			case constant.NO:
				continue
			default:
				util.PrintError("Invalid preference. Setting No as input.")
				continue
			}
		}
		jiraSummary := getJiraSummary(jiraKey)
		bugFixes[jiraKey] = jiraSummary
	}
	logger.Debug(fmt.Sprintf("bug_fixes: %v", bugFixes))
	updateDescriptorV2.BugFixes = bugFixes
}

// Used for getting JIRA_KEY_SUMMARY/GITHUB_ISSUE_SUMMARY for the given JIRA_KEY/GITHUB_ISSUE
func getJiraSummary(jiraKey string) string {
	var jiraSummary string
	for {
		util.PrintInBold(fmt.Sprintf("\tEnter JIRA_KEY_SUMMARY/GITHUB_ISSUE_SUMMARY for '%s': ", jiraKey))
		jiraSum, err := util.GetUserInput()
		util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
		if jiraSum == "" {
			util.PrintErrorWithTab(fmt.Sprintf("Empty input detected, "+
				"Enter a valid JIRA_KEY_SUMMARY/GITHUB_ISSUE_SUMMARY for '%s'", jiraKey))
			continue
		}
		jiraSummary = jiraSum
		break
	}
	return jiraSummary
}

// Creates the updateDescriptorV2 for saving.
func createUpdateDescriptorV2(updateDirectoryPath string, updateDescriptorV2 *util.UpdateDescriptorV2) {
	// Marshall update descriptor struct
	dataV2, err := yaml.Marshal(updateDescriptorV2)
	util.HandleErrorAndExit(err)

	dataStringV2 := string(dataV2)

	// Remove "" enclosing the update number
	dataStringV2 = strings.Replace(dataStringV2, "\"", "", -1)
	logger.Trace(fmt.Sprintf("update-descriptorV2:\n%s", dataStringV2))

	// Construct the update descriptor file path
	updateDescriptorFileV2 := filepath.Join(updateDirectoryPath, constant.UPDATE_DESCRIPTOR_V2_FILE)
	logger.Debug(fmt.Sprintf("updateDescriptorFileV2: %v", updateDescriptorFileV2))

	// Save update descriptor
	absDestinationV2 := saveUpdateDescriptorInDestination(updateDescriptorFileV2, dataStringV2, updateDirectoryPath)
	fmt.Println(fmt.Sprintf("'%s' has been successfully created in '%s'.", constant.UPDATE_DESCRIPTOR_V2_FILE,
		absDestinationV2))
}

// Creates the updateDescriptorV3 for saving.
func createUpdateDescriptorV3(updateDirectoryPath string, updateDescriptorV3 *util.UpdateDescriptorV3) {
	// Marshall update descriptor structs
	dataV3, err := yaml.Marshal(updateDescriptorV3)
	util.HandleErrorAndExit(err)
	dataStringV3 := string(dataV3)

	//remove " enclosing the update number
	dataStringV3 = strings.Replace(dataStringV3, "\"", "", -1)
	logger.Trace(fmt.Sprintf("update-descriptorV3:\n%s", dataStringV3))

	// Construct update descriptor file paths
	updateDescriptorFileV3 := filepath.Join(updateDirectoryPath, constant.UPDATE_DESCRIPTOR_V3_FILE)
	logger.Debug(fmt.Sprintf("updateDescriptorFileV3: %v", updateDescriptorFileV3))

	// Save update descriptors
	absDestinationV3 := saveUpdateDescriptorInDestination(updateDescriptorFileV3, dataStringV3, updateDirectoryPath)
	fmt.Println(fmt.Sprintf("'%s' has been successfully created in '%s'.", constant.UPDATE_DESCRIPTOR_V3_FILE,
		absDestinationV3))
}

// Save the given update descriptor in given location.
func saveUpdateDescriptorInDestination(updateDescriptorFilePath, dataString, destination string) string {
	// Cast the dataString to an array of bytes
	data := []byte(dataString)
	return util.WriteUpdateDescriptorInDestination(data, updateDescriptorFilePath, destination)
}

// This function will set the update name which will be used when creating the update zip.
func getUpdateName(updateDescriptorV2 *util.UpdateDescriptorV2, updateNamePrefix string) string {
	// Read the corresponding details from the struct
	platformVersion := updateDescriptorV2.PlatformVersion
	updateNumber := updateDescriptorV2.UpdateNumber
	updateName := updateNamePrefix + "-" + platformVersion + "-" + updateNumber
	return updateName
}

func downloadFile(directory, urlName, downloadUrl, fileName string) {
	url, exists := os.LookupEnv(urlName)
	if !exists {
		url = downloadUrl
		logger.Debug(fmt.Sprintf("Environment variable '%s' is not set. Getting file from: %s",
			urlName, downloadUrl))
	}
	err := util.DownloadFile(path.Join(directory, fileName), url)
	if err != nil {
		util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while getting the file '%v' "+
			"from: %s.", fileName, url))
	}
}

// This function will handle no match found for a file situations. User input is required and based on the user input,
// this function will decide how to proceed.
func handleNoMatch(filename string, isDir bool, allFilesMap map[string]data, rootNode *node,
	updateDescriptor *util.UpdateDescriptorV2) error {
	//todo: Check OSGi bundles in the plugins directory
	logger.Debug(fmt.Sprintf("[NO MATCH] %s", filename))
	util.PrintInBold(fmt.Sprintf("'%s' not found in distribution. ", filename))
	for {
		// Get the user preference
		util.PrintInBold("Do you want to add it as a new file? [Y/n]: ")
		preference, err := util.GetUserInput()
		if len(preference) == 0 {
			preference = "y"
		}
		util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")

		// Act according to the user preference
		userPreference := util.ProcessUserPreference(preference)
		switch userPreference {
		case constant.YES:
			// Handle the file/directory as new
			err = handleNewFile(filename, isDir, rootNode, allFilesMap, updateDescriptor)
			util.HandleErrorAndExit(err)
			//If no error, return nil
			return nil
		case constant.NO:
			util.PrintWarning(fmt.Sprintf("Skipping copying: %s", filename))
			return nil
		default:
			util.PrintError("Invalid preference. Enter Y for Yes or N for No.")
		}
	}
}

// This function will handle the situations where the user want to add a file as a new file which was not found in the
// distribution.
func handleNewFile(filename string, isDir bool, rootNode *node, allFilesMap map[string]data,
	updateDescriptor *util.UpdateDescriptorV2) error {
	logger.Debug(fmt.Sprintf("[HANDLE NEW] %s", filename))

readDestinationLoop:
	for {
		// Get user preference
		util.PrintInBold("Enter destination directory relative to PRODUCT_HOME: ")
		relativeLocationInDistribution, err := util.GetUserInput()
		// Trim the path separators at the beginning and the end of the path if present.
		relativeLocationInDistribution = strings.TrimPrefix(relativeLocationInDistribution,
			constant.PATH_SEPARATOR)
		relativeLocationInDistribution = strings.TrimSuffix(relativeLocationInDistribution,
			constant.PATH_SEPARATOR)
		util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
		logger.Debug("relativePath:", relativeLocationInDistribution)

		// Get the update root from the viper configs.
		updateRoot := viper.GetString(constant.UPDATE_ROOT)
		if len(updateRoot) == 0 {
			util.HandleErrorAndExit(errors.New("updateRoot path length is 0"))
		}

		// Check whether the directory which user entered is already in the distribution.
		var exists bool
		if isDir {
			// If currently processing a directory, construct the full path and check.
			fullPath := path.Join(relativeLocationInDistribution, filename)
			logger.Debug(fmt.Sprintf("Checking: %s", fullPath))
			exists = PathExists(rootNode, fullPath, true)
			logger.Debug(fmt.Sprintf("%s exists: %v", fullPath, exists))
		} else {
			// If currently processing a file, no need to construct the full path. We can directly check
			// the entered directory.
			logger.Debug("Checking:", relativeLocationInDistribution)
			exists = PathExists(rootNode, relativeLocationInDistribution, true)
			logger.Debug(relativeLocationInDistribution+" exists:", exists)
		}

		// If the directory is already in the distribution
		if exists {
			// If we are processing a directory
			if isDir {
				// Get all matching files. By matching files, we mean all the files which are in the
				// directory and subdirectories.
				allMatchingFiles := getAllMatchingFiles(filename, allFilesMap)
				logger.Debug(fmt.Sprintf("All matches: %v", allMatchingFiles))
				// Copy all matching files to the temp directory
				for _, match := range allMatchingFiles {
					logger.Debug(fmt.Sprintf("[Copy] %s ; From: %s ; To: %s", match, updateRoot,
						relativeLocationInDistribution))
					err = copyFile(match, updateRoot, relativeLocationInDistribution, rootNode,
						updateDescriptor)
					util.HandleErrorAndExit(err)
				}
			} else {
				// If we are processing a file, copy the file to the temp directory
				logger.Debug(fmt.Sprintf("[Copy] %s ; From: %s ; To: %s", filename, updateRoot,
					relativeLocationInDistribution))
				err = copyFile(filename, updateRoot, relativeLocationInDistribution, rootNode,
					updateDescriptor)
				util.HandleErrorAndExit(err)
			}
			break

		} else if len(relativeLocationInDistribution) > 0 {
			// If the distribution is not found and the relative location is not the distribution root
			util.PrintInBold("Entered relative path does not exist in the distribution. ")
			for {
				// Prompt the user
				util.PrintInBold("Copy anyway? [y/n/R]: ")
				preference, err := util.GetUserInput()
				if len(preference) == 0 {
					preference = "r"
				}
				util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")

				userPreference := util.ProcessUserPreference(preference)
				switch userPreference {
				case constant.YES:
					updateRoot := viper.GetString(constant.UPDATE_ROOT)
					// Get all matching files. By matching files, we mean all the files which are
					// in the directory and subdirectories.
					allMatchingFiles := getAllMatchingFiles(filename, allFilesMap)
					logger.Debug(fmt.Sprintf("Copying all matches:\n%s", allMatchingFiles))
					// Copy all matching files to the temp directory
					for _, match := range allMatchingFiles {
						logger.Debug(fmt.Sprintf("[Copy] %s ; From: %s ; To: %s", match,
							updateRoot, relativeLocationInDistribution))
						err = copyFile(match, updateRoot, relativeLocationInDistribution,
							rootNode, updateDescriptor)
						util.HandleErrorAndExit(err)
					}
					break readDestinationLoop
				case constant.NO:
					util.PrintWarning("Skipping copying", filename)
					return nil
				case constant.REENTER:
					continue readDestinationLoop
				default:
					util.PrintError("Invalid preference. Enter Y for Yes or N for No or R for " +
						"Re-enter.")
				}
			}
		} else {
			// If the user enters the distribution root
			updateRoot := viper.GetString(constant.UPDATE_ROOT)
			// Get all matching files. By matching files, we mean all the files which are in the directory
			// and subdirectories.
			allMatchingFiles := getAllMatchingFiles(filename, allFilesMap)
			logger.Debug(fmt.Sprintf("Copying all matches:\n%s", allMatchingFiles))
			// Copy all matching files to the temp directory
			for _, match := range allMatchingFiles {
				logger.Debug(fmt.Sprintf("[Copy] %s ; From: %s ; To: %s", match, updateRoot,
					relativeLocationInDistribution))
				err = copyFile(match, updateRoot, relativeLocationInDistribution, rootNode,
					updateDescriptor)
				util.HandleErrorAndExit(err)
			}
			break readDestinationLoop
		}
	}
	return nil
}

// This function will situations where a single match is found in the distribution.
func handleSingleMatch(filename string, matchingNode *node, isDir bool, allFilesMap map[string]data, rootNode *node,
	updateDescriptor *util.UpdateDescriptorV2) error {
	logger.Debug(fmt.Sprintf("[SINGLE MATCH] %s ; match: %s", filename, matchingNode.relativeLocation))
	updateRoot := viper.GetString(constant.UPDATE_ROOT)
	if isDir {
		// If we are processing a directory, get all matching files. By matching files, we mean all the files
		// which are in the directory and subdirectories.
		allMatchingFiles := getAllMatchingFiles(filename, allFilesMap)
		logger.Debug(fmt.Sprintf("All matches: %s", allMatchingFiles))
		// Copy all matching files to the temp directory
		for _, match := range allMatchingFiles {
			logger.Debug(fmt.Sprintf("match: %s", match))
			// Check md5 only if the md5 checking is not disabled
			if !viper.GetBool(constant.CHECK_MD5_DISABLED) {
				logger.Debug(fmt.Sprintf("Checking md5: %v", filename))
				data := allFilesMap[match]
				// Check whether the md5 matches or not
				fileLocation := path.Join(matchingNode.relativeLocation, match)
				md5Matches := CheckMD5(rootNode, strings.Split(fileLocation, "/"), data.md5)
				if md5Matches {
					util.PrintInfo(fmt.Sprintf("File '%v' not copied because MD5 matches with "+
						"the already existing file.", match))
					logger.Debug("MD5 matches. Ignoring file.")
					continue
				} else {
					logger.Debug("MD5 does not match. Copying the file.")
				}
			}
			// Copy the file to temp directory
			logger.Debug(fmt.Sprintf("[Copy] %s ; From: %s ; To: %s", match, updateRoot,
				matchingNode.relativeLocation))
			err := copyFile(match, updateRoot, matchingNode.relativeLocation, rootNode, updateDescriptor)
			util.HandleErrorAndExit(err)
		}
	} else {
		// Check md5 only if the md5 checking is not disabled
		if !viper.GetBool(constant.CHECK_MD5_DISABLED) {
			logger.Debug(fmt.Sprintf("Checking md5: %v", filename))
			data := allFilesMap[filename]
			// Check whether the md5 matches or not
			fileLocation := path.Join(matchingNode.relativeLocation, filename)
			md5Matches := CheckMD5(rootNode, strings.Split(fileLocation, "/"), data.md5)
			if md5Matches {
				util.PrintInfo(fmt.Sprintf("File '%v' not copied because MD5 matches with the "+
					"already existing file.", filename))
				logger.Debug("MD5 matches. Ignoring file.")
				// If md5 does not match, return
				return nil
			} else {
				logger.Debug("MD5 does not match. Copying the file.")
			}
		}
		// Copy the file to temp directory
		logger.Debug(fmt.Sprintf("[Copy] %s ; From: %s ; To: %s", filename, updateRoot,
			matchingNode.relativeLocation))
		err := copyFile(filename, updateRoot, matchingNode.relativeLocation, rootNode,
			updateDescriptor)
		util.HandleErrorAndExit(err)
	}
	return nil
}

// This function will handle multiple match situations. In here user input is required.
func handleMultipleMatches(filename string, isDir bool, matches map[string]*node, allFilesMap map[string]data,
	rootNode *node, updateDescriptor *util.UpdateDescriptorV2) error {

	util.PrintInfo(fmt.Sprintf("Multiple matches found for '%s' in the distribution.", filename))

	logger.Debug(fmt.Sprintf("[MULTIPLE MATCHES] %s", filename))
	locationTable, indexMap := generateLocationTable(filename, matches)
	locationTable.Render()
	logger.Debug(fmt.Sprintf("indexMap: %s", indexMap))
	skipCopying := false
	var selectedIndices []string
	// Loop while user enter valid preference or enter 0 to exit
	for {
		// Get user preference
		util.PrintInBold("Enter preference(s)[Multiple selections separated by commas, 0 to skip copying]: ")
		preferences, err := util.GetUserInput()
		util.HandleErrorAndExit(err)
		logger.Debug(fmt.Sprintf("preferences: %s", preferences))
		// Remove the new line at the end
		preferences = strings.TrimSpace(preferences)
		// Split the indices
		selectedIndices = strings.Split(preferences, ",")
		//Sort the locations
		sort.Strings(selectedIndices)
		logger.Debug(fmt.Sprintf("sorted: %s", preferences))

		length := len(indexMap)
		// Check whether the user preference is valid
		isValid, err := util.IsUserPreferencesValid(selectedIndices, length)
		if err != nil {
			util.PrintError("Invalid preferences. Please select indices where 0 <= index <= " +
				strconv.Itoa(length))
			continue
		}
		if !isValid {
			util.PrintError("Invalid preferences. Please select indices where 0 <= index <= " +
				strconv.Itoa(length))
		} else {
			logger.Debug("Entered preferences are valid.")
			if selectedIndices[0] == "0" {
				skipCopying = true
			}
			break
		}
	}
	// Check whether the user entered 0
	if skipCopying {
		logger.Debug(fmt.Sprintf("Skipping copying '%s'", filename))
		util.PrintWarning(fmt.Sprintf("0 entered. Skipping copying '%s'.", filename))
		return nil
	}
	updateRoot := viper.GetString(constant.UPDATE_ROOT)
	if isDir {
		// Copy the directory to all selected locations
		for _, selectedIndex := range selectedIndices {
			pathInDistribution := indexMap[selectedIndex]
			logger.Debug(fmt.Sprintf("[MULTIPLE MATCHES] Selected path: %s ; %s", selectedIndex,
				pathInDistribution))

			// Get all matching files (files which are in the directory and subdirectories)
			allMatchingFiles := getAllMatchingFiles(filename, allFilesMap)
			logger.Debug(fmt.Sprintf("matchingFiles: %s", allMatchingFiles))

			// Copy all the matching files to temp directory
			for _, match := range allMatchingFiles {
				logger.Debug(fmt.Sprintf("match: %s", match))
				// Check md5 if the md5 checking is not disabled
				if !viper.GetBool(constant.CHECK_MD5_DISABLED) {
					data := allFilesMap[match]
					// Check whether the md5 matches or not
					fileLocation := strings.Split(path.Join(pathInDistribution, match), "/")
					md5Matches := CheckMD5(rootNode, fileLocation, data.md5)
					if md5Matches {
						util.PrintInfo(fmt.Sprintf("File '%v' not copied because MD5 "+
							"matches with the already existing file.", match))
						logger.Debug("MD5 matches. Ignoring file.")
						continue
					}
					logger.Debug("MD5 does not match. Copying the file.")
				}
				logger.Debug(fmt.Sprintf("[Copy] %s ; From: %s ; To: %s", filename, updateRoot,
					pathInDistribution))
				err := copyFile(match, updateRoot, pathInDistribution, rootNode, updateDescriptor)
				util.HandleErrorAndExit(err)
			}
		}
	} else {
		// Copy the file to all selected locations
		for _, selectedIndex := range selectedIndices {
			pathInDistribution := indexMap[selectedIndex]
			// Check md5 if the md5 checking is not disabled
			if !viper.GetBool(constant.CHECK_MD5_DISABLED) {
				data := allFilesMap[filename]
				// Check whether the md5 matches or not
				fileLocation := strings.Split(path.Join(pathInDistribution, filename), "/")
				md5Matches := CheckMD5(rootNode, fileLocation, data.md5)
				if md5Matches {
					// If md5 matches, print warning msg and continue with the next selected
					// location
					util.PrintInfo(fmt.Sprintf("File '%v' not copied because MD5 matches "+
						"with the already existing file.", filename))
					logger.Debug("MD5 matches. Ignoring file.")
					continue
				}
				logger.Debug("MD5 does not match. Copying the file.")
			}
			// Copy the file to temp location
			logger.Debug(fmt.Sprintf("[MULTIPLE MATCHES] Selected path: %s ; %s", selectedIndex,
				pathInDistribution))
			logger.Debug(fmt.Sprintf("[Copy] %s ; From: %s ; To: %s", filename, updateRoot,
				pathInDistribution))
			err := copyFile(filename, updateRoot, pathInDistribution, rootNode, updateDescriptor)
			util.HandleErrorAndExit(err)
		}
	}
	return nil
}

// This function will return all matching files (all files in a directory and subdirectories) of the given filepath.
func getAllMatchingFiles(path string, allFilesMap map[string]data) []string {
	matches := make([]string, 0)
	for filePath, data := range allFilesMap {
		// Should not be a directory. Should have the path prefix (identifying that it is in the directory)
		// filePath != path because it should only return files within the provided directory. otherwise a file
		// can be matched if it has the same path as the given path.
		if !data.isDir && strings.HasPrefix(filePath, path) && filePath != path {
			matches = append(matches, filePath)
		}
	}
	return matches
}

// This function will read the directory in the given location and return 3 values and an error if any exists.
func readDirectory(root string, ignoredFiles map[string]bool) (map[string]data, map[string]bool, map[string]bool,
	error) {
	allFilesMap := make(map[string]data)
	rootLevelDirectoriesMap := make(map[string]bool)
	rootLevelFilesMap := make(map[string]bool)

	// Walk and read the directory structure
	filepath.Walk(root, func(absolutePath string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		//Convert all backslashes to slashes (to fix path issues in windows)
		absolutePath = filepath.ToSlash(absolutePath)

		//Ignore root directory
		if root == absolutePath {
			return nil
		}
		logger.Trace(fmt.Sprintf("[WALK] %s ; %v", absolutePath, fileInfo.IsDir()))
		//check current file in ignored files map. This is useful to ignore update-descriptor.yaml, etc in
		// update directory
		if ignoredFiles != nil {
			_, found := ignoredFiles[fileInfo.Name()]
			if found {
				return nil
			}
		}
		// Get the relative path. This is used as the key of the map
		trimPattern := root + "/"
		if strings.HasSuffix(root, "/") {
			trimPattern = root
		}

		relativePath := strings.TrimPrefix(absolutePath, trimPattern)
		// Create the data struct which will have the other details
		info := data{
			name:         fileInfo.Name(),
			relativePath: relativePath,
		}
		if fileInfo.IsDir() {
			logger.Trace(fmt.Sprintf("Directory: %s , %s", absolutePath, fileInfo.Name()))
			info.isDir = true
			logger.Debug(fmt.Sprintf("Checking: %s == %s", path.Join(root, fileInfo.Name()), absolutePath))
			// We need to only get the list of directories in the root level. Ignore other directories
			if path.Join(root, fileInfo.Name()) == absolutePath {
				logger.Debug(fmt.Sprintf("Paths are eqal. Adding %s to rootLevelDirectoriesMap",
					fileInfo.Name()))
				// Add the entry to the rootLevelDirectoriesMap
				rootLevelDirectoriesMap[fileInfo.Name()] = true
			}
		} else {
			// We need to only get the list of files in the root level. Ignore other files
			if path.Join(root, fileInfo.Name()) == absolutePath {
				rootLevelFilesMap[fileInfo.Name()] = false
			}

			// We need other information like md5 sum because we are storing details of all files in the
			// allFilesMap
			logger.Trace("[MD5] Calculating MD5")
			//If it is a file, calculate md5 sum
			md5Sum, err := util.GetMD5(absolutePath)
			if err != nil {
				return err
			}
			logger.Trace(fmt.Sprintf("%s : %s = %s", absolutePath, fileInfo.Name(), md5Sum))
			info.md5 = md5Sum
			info.isDir = false
		}
		// Add the entry to the allFilesMap
		allFilesMap[relativePath] = info
		return nil
	})
	return allFilesMap, rootLevelDirectoriesMap, rootLevelFilesMap, nil
}

// This function will read the zip file in the given location.
func readZip(location string) (node, error) {
	rootNode := createNewNode()
	fileMap := make(map[string]bool)
	// Create a reader out of the zip archive
	zipReader, err := zip.OpenReader(location)
	if err != nil {
		return rootNode, err
	}
	defer zipReader.Close()

	productName := viper.GetString(constant.PRODUCT_NAME)
	logger.Debug(fmt.Sprintf("productName: %s", productName))
	// Iterate through each file in the zip file
	for _, file := range zipReader.Reader.File {
		zippedFile, err := file.Open()
		if err != nil {
			return rootNode, err
		}
		data, err := ioutil.ReadAll(zippedFile)
		// Don't use defer here because otherwise there will be too many open files and it will cause a panic
		zippedFile.Close()

		// Calculate the md5 of the file
		hash := md5.New()
		hash.Write(data)
		md5Hash := hex.EncodeToString(hash.Sum(nil))

		// Get the relative path of the file
		logger.Trace(fmt.Sprintf("file.Name: %s", file.Name))

		relativePath := util.GetRelativePath(file)

		// Add the file to root node
		AddToRootNode(&rootNode, strings.Split(relativePath, "/"), file.FileInfo().IsDir(), md5Hash)
		if !file.FileInfo().IsDir() {
			fileMap[relativePath] = false
		}
	}
	return rootNode, nil
}

// This function will add a new node.
func AddToRootNode(root *node, path []string, isDir bool, md5Hash string) *node {
	logger.Trace("Checking: %s : %s", path[0], path)

	// If the current path element is the last element, add it as a new node.
	if len(path) == 1 {
		logger.Trace("End reached")
		newNode := createNewNode()
		newNode.name = path[0]
		newNode.isDir = isDir
		newNode.md5Hash = md5Hash
		if len(root.relativeLocation) == 0 {
			newNode.relativeLocation = path[0]
		} else {
			newNode.relativeLocation = root.relativeLocation + "/" + path[0]
		}
		newNode.parent = root
		root.childNodes[path[0]] = &newNode
	} else {
		// If there are more path elements than 1, that means we are currently processing a directory.
		logger.Trace(fmt.Sprintf("End not reached. checking: %v", path[0]))
		node, contains := root.childNodes[path[0]]
		// If the directory is already not in the tree, add it as a new node
		if !contains {
			logger.Trace(fmt.Sprintf("Creating new node: %v", path[0]))
			newNode := createNewNode()
			newNode.name = path[0]
			newNode.isDir = true
			if len(root.relativeLocation) == 0 {
				newNode.relativeLocation = path[0]
			} else {
				newNode.relativeLocation = root.relativeLocation + "/" + path[0]
			}
			newNode.parent = root
			root.childNodes[path[0]] = &newNode
			node = &newNode
		}
		// Recursively call the function for the rest of the path elements.
		AddToRootNode(node, path[1:], isDir, md5Hash)
	}
	return root
}

// This function is a helper function which calls NodeExists() and checks whether a node exists in the given path and
// the type(file/dir) is correct.
func PathExists(rootNode *node, relativePath string, isDir bool) bool {
	return NodeExists(rootNode, strings.Split(relativePath, "/"), isDir)
}

// This function checks whether a node exists in the given path and the type(file/dir) is correct.
func NodeExists(rootNode *node, path []string, isDir bool) bool {
	logger.Trace(fmt.Sprintf("All: %v", rootNode.childNodes))
	logger.Trace(fmt.Sprintf("Checking: %s", path[0]))
	childNode, found := rootNode.childNodes[path[0]]
	// If the path element is found, that means it is in the tree
	if found {
		// If there are more path elements than 1, continue recursively. Otherwise check whether it has the
		// provided type(file/dir) and return.
		logger.Trace(fmt.Sprintf("%s found", path[0]))
		if len(path) > 1 {
			return NodeExists(childNode, path[1:], isDir)
		} else {
			return childNode.isDir == isDir
		}
	}
	// If the path element is not found, return false
	logger.Trace(fmt.Sprintf("%s NOT found", path[0]))
	return false
}

// This function will check the MD5 hash of the file in the provided path in the distribution with the provided hash.
func CheckMD5(rootNode *node, path []string, md5 string) bool {
	logger.Trace(fmt.Sprintf("All: %v", rootNode.childNodes))
	logger.Trace(fmt.Sprintf("Checking: %s", path[0]))
	childNode, found := rootNode.childNodes[path[0]]
	// If the path element is found, that means it is in the tree
	if found {
		// If there are more path elements than 1, continue recursively. Otherwise check whether it has the
		// given md5 or not and return.
		logger.Trace(fmt.Sprintf("%s found", path[0]))
		if len(path) > 1 {
			return CheckMD5(childNode, path[1:], md5)
		} else {
			return childNode.isDir == false && childNode.md5Hash == md5
		}
	}
	// If the path element is not found, return false
	logger.Trace(fmt.Sprintf("%s NOT found", path[0]))
	return false
}

// This function will find all matches in distribution for the provided name.
func FindMatches(root *node, name string, isDir bool, matches map[string]*node) {
	// Check whether the given name is in the child nodes
	childNode, found := root.childNodes[name]
	if found {
		// If it is in child nodes, check whether the type matches
		if isDir == childNode.isDir {
			// If type matches, add it to the matches map
			matches[root.relativeLocation] = root
		}
	}
	// Regardless of whether the file is found or not, iterate through all sub directories to find all matches
	for _, childNode := range root.childNodes {
		if childNode.isDir {
			FindMatches(childNode, name, isDir, matches)
		}
	}
}

// This will return a map of files which would be ignored when reading the update directory.
func getIgnoredFilesInUpdate() map[string]bool {
	filesMap := make(map[string]bool)
	// Get the mandatory resource files and add to the the map
	for _, file := range viper.GetStringSlice(constant.RESOURCE_FILES_MANDATORY) {
		filesMap[file] = true
	}
	// Get the mandatory optional files and add to the the map
	for _, file := range viper.GetStringSlice(constant.RESOURCE_FILES_OPTIONAL) {
		filesMap[file] = true
	}
	// Get the files we are going to skip matching and add to the the map
	for _, file := range viper.GetStringSlice(constant.RESOURCE_FILES_SKIP) {
		filesMap[file] = true
	}
	return filesMap
}

// This will return a map of files which would be copied to the temp directory before creating the update zip. Key is
// the file name and value is whether the file is mandatory or not.
func getResourceFiles() map[string]bool {
	filesMap := make(map[string]bool)
	// Get the mandatory resource files and add to the the map
	for _, file := range viper.GetStringSlice(constant.RESOURCE_FILES_MANDATORY) {
		filesMap[file] = true
	}
	// Get the mandatory optional files and add to the the map
	for _, file := range viper.GetStringSlice(constant.RESOURCE_FILES_OPTIONAL) {
		filesMap[file] = false
	}
	return filesMap
}

// This function will marshal the update-descriptor.yaml file.
func marshalUpdateDescriptor(updateDescriptorV2 *util.UpdateDescriptorV2) ([]byte, error) {
	data, err := yaml.Marshal(&updateDescriptorV2)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// This function will save update descriptor to temp directory after modifying the file_changes section.
func saveUpdateDescriptor(updateDescriptorFilename string, data []byte) error {
	updateName := viper.GetString(constant.UPDATE_NAME)
	destination := path.Join(constant.TEMP_DIR, updateName, updateDescriptorFilename)
	// Open a new file for writing only
	file, err := os.OpenFile(
		destination,
		os.O_WRONLY|os.O_TRUNC|os.O_CREATE,
		0600,
	)
	defer file.Close()
	if err != nil {
		return err
	}
	// The update number will always have enclosing "" to indicate it is an string. So we need to remove that.
	updatedData := strings.Replace(string(data), "\"", "", 2)
	modifiedData := []byte(updatedData)
	// Write bytes to file
	_, err = file.Write(modifiedData)
	if err != nil {
		return err
	}
	return nil
}

// This function will copy resource files to the temp directory.
func copyResourceFilesToTempDir(resourceFilesMap map[string]bool) error {
	// Create the directories if they are not available
	updateName := viper.GetString(constant.UPDATE_NAME)
	destination := path.Join(constant.TEMP_DIR, updateName, constant.CARBON_HOME)
	util.CreateDirectory(destination)
	// Iterate through all resource files
	for filename, isMandatory := range resourceFilesMap {
		updateRoot := viper.GetString(constant.UPDATE_ROOT)
		source := path.Join(updateRoot, filename)
		destination = path.Join(constant.TEMP_DIR, updateName, filename)
		// Copy the file
		err := util.CopyFile(source, destination)
		if err != nil {
			// If an error occurs while copying, if the file is a mandatory file, return an error. If the
			// file is not mandatory, print a message and continue.
			if isMandatory {
				return err
			} else {
				fmt.Println(fmt.Sprintf("Optional resource file '%s' not copied.", filename))
			}
		}
	}
	return nil
}

// This will generate the location table and the index map which will be used to get user preference.
func generateLocationTable(filename string, locationsInDistribution map[string]*node) (*tablewriter.Table,
	map[string]string) {
	// This is used to show the information to the user.
	locationTable := tablewriter.NewWriter(os.Stdout)
	locationTable.SetAlignment(tablewriter.ALIGN_LEFT)
	locationTable.SetHeader([]string{"Index", "Matching Location"})

	// Add all locations to a new array
	allPaths := make([]string, 0)
	for distributionFilepath := range locationsInDistribution {
		allPaths = append(allPaths, distributionFilepath)
	}
	// Sort the array
	sort.Strings(allPaths)

	index := 1
	// This map will hold the location against the index. This will be used to copy files.
	indexMap := make(map[string]string)
	for _, distributionFilepath := range allPaths {
		logger.Debug(fmt.Sprintf("[TABLE] filepath: %s ; isDir: %v", distributionFilepath,
			locationsInDistribution[distributionFilepath].isDir))
		// Add the index and the location to the map
		indexMap[strconv.Itoa(index)] = distributionFilepath
		relativePath := path.Join("CARBON_HOME", distributionFilepath)
		// Add the relative location to the table
		locationTable.Append([]string{strconv.Itoa(index), path.Join(relativePath, filename)})
		index++
	}
	return locationTable, indexMap
}

//This function will copy the file/directory from update to temp location.
func copyFile(filename string, locationInUpdate, relativeLocationInTemp string, rootNode *node,
	updateDescriptor *util.UpdateDescriptorV2) error {
	logger.Debug(fmt.Sprintf("[FINAL][COPY ROOT] Name: %s ; IsDir: false ; From: %s ; To: %s", filename,
		locationInUpdate, relativeLocationInTemp))
	updateName := viper.GetString(constant.UPDATE_NAME)
	source := path.Join(locationInUpdate, filename)
	carbonHome := path.Join(constant.TEMP_DIR, updateName, constant.CARBON_HOME)
	destination := path.Join(carbonHome, relativeLocationInTemp)

	//Replace all / with OS specific path separators to handle OSs like Windows
	destination = strings.Replace(destination, "/", constant.PATH_SEPARATOR, -1)

	fullPath := path.Join(destination, filename)
	//Replace all / with OS specific path separators to handle OSs like Windows
	fullPath = strings.Replace(fullPath, "/", constant.PATH_SEPARATOR, -1)

	parentDirectory := path.Dir(fullPath)
	logger.Debug("parentDirectory:", parentDirectory)
	err := util.CreateDirectory(parentDirectory)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while creating '%v' directory.", parentDirectory))
	logger.Debug(fmt.Sprintf("[FINAL][COPY][TEMP] Name: %s; From: %s; To: %s", filename, source, fullPath))
	err = util.CopyFile(source, fullPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while copying file. Source: %v, Destination: %v",
		source, fullPath))

	prefix := carbonHome + "/"
	// Replace all / characters with the os path separator character. Otherwise errors will occur in OSs like
	// Windows
	prefix = strings.Replace(prefix, "/", constant.PATH_SEPARATOR, -1)
	logger.Debug(fmt.Sprintf("Trimming %s using %s", fullPath, prefix))
	relativePath := strings.TrimPrefix(fullPath, prefix)
	logger.Debug(fmt.Sprintf("relativePath: %s", relativePath))
	contains := PathExists(rootNode, relativePath, false)
	logger.Debug(fmt.Sprintf("contains: %v", contains))
	// If the file already in the distribution, add it as a modified file. Otherwise add it as a new file
	if contains {
		updateDescriptor.FileChanges.ModifiedFiles = append(updateDescriptor.FileChanges.ModifiedFiles,
			relativePath)
	} else {
		updateDescriptor.FileChanges.AddedFiles = append(updateDescriptor.FileChanges.AddedFiles,
			relativePath)
	}
	return nil
}

//This function will create a zip file from the source to the target folder
func ZipFile(source, target string) error {
	zipfile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer zipfile.Close()

	archive := zip.NewWriter(zipfile)
	defer archive.Close()

	info, err := os.Stat(source)
	if err != nil {
		return err
	}

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		if baseDir != "" {
			header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
		}
		if info.IsDir() {
			header.Name += "/"
		}
		header.Method = zip.Deflate

		//To support archives created under Windows and to be correctly handled in Linux.
		header.Name = filepath.ToSlash(header.Name)

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}

		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})
	return err
}

func setProductChangesInUpdateDescriptorV3(partialUpdatedProducts *util.PartialUpdatedProducts) *util.ProductChanges {
	productChanges := &util.ProductChanges{}
	productChanges.ProductName = partialUpdatedProducts.ProductName
	productChanges.ProductVersion = partialUpdatedProducts.BaseVersion + "." + partialUpdatedProducts.Tag
	productChanges.AddedFiles = partialUpdatedProducts.AddedFiles
	productChanges.RemovedFiles = partialUpdatedProducts.RemovedFiles
	productChanges.ModifiedFiles = partialUpdatedProducts.ModifiedFiles
	return productChanges
}

// This will append removed files to update-descriptor.yaml
func appendRemovedFilesToUpdateDescriptor(updateDescriptorV2 *util.UpdateDescriptorV2) {
userInputLoop:
	for {
		util.PrintInBold(fmt.Sprintf("Enter the path of a removed file relative to the PRODUCT_HOME, " +
			"press enter when the path is added\n"))
		removedFile, err := util.GetUserInput()
		util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
		if removedFile == "" {
			util.PrintInBold("Empty input detected, are you done with adding inputs? [y/n]: ")
			preference, err := util.GetUserInput()
			util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
			userPreference := util.ProcessUserPreference(preference)
			switch userPreference {
			case constant.YES:
				break userInputLoop
			case constant.NO:
				continue
			default:
				util.PrintError("Invalid preference. Setting No as input.")
				continue
			}
		}
		updateDescriptorV2.FileChanges.RemovedFiles = append(updateDescriptorV2.FileChanges.RemovedFiles, removedFile)
	}
}

// This function save '.wum-uc-resume.yaml' file for resuming update creation (wum-uc create --continue) in future.
func saveResumeFile(resumeFile *resumeFile) {
	data, err := yaml.Marshal(resumeFile)
	util.HandleErrorAndExit(err, "error occurred while marshalling the resume file.")
	util.WriteFileToDestination(data, wumucResumeFile)
	logger.Debug(fmt.Sprintf("%s file created successfully in %s \n", constant.WUMUC_RESUME_FILE, constant.WUM_UC_HOME))
}

/* This function will continue the update creation after manually modifying the relevant sections of the
update-descriptor3.yaml by the developer.*/
func continueResumedUpdateCreation() {
	resumedFile := resumeFile{}
	// Check for the existence of 'wum-uc-resume.yaml' file
	wumucResumeFile := filepath.Join(constant.WUM_UC_HOME, constant.WUMUC_RESUME_FILE)
	exits, err := util.IsFileExists(wumucResumeFile)
	if err != nil {
		util.HandleErrorAndExit(err, " error occurred while checking the existence of ", wumucResumeFile)
	}
	if !exits {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("no trace of a resumed update creation found, " +
			"please recreate the update.")))
	}

	// Read resumed update creation details
	data, err := ioutil.ReadFile(wumucResumeFile)
	if err != nil {
		util.HandleErrorAndExit(err, "error occurred while reading the ", wumucResumeFile)
	}
	err = yaml.Unmarshal(data, &resumedFile)
	if err != nil {
		util.HandleErrorAndExit(err, "error occurred while un-marshaling the ", wumucResumeFile)
	}

	// Check if the update zip is already being created
	if resumedFile.isUpdateZipCreated {
		// Todo check if additional steps and args are required to be passed and uncomment below line
		//commitUpdateToSVN(&resumedFile)

	} else {
		// Create the update zip from resumed state
		// Copy developer edited `update-descriptor3.yaml` to the temp location for creating the update.
		source := path.Join(resumedFile.resourceDirectoryPath, constant.UPDATE_DESCRIPTOR_V3_FILE)
		destination := path.Join(resumedFile.explodedUpdateDirectoryPath, constant.UPDATE_DESCRIPTOR_V3_FILE)
		updateZipName := resumedFile.updateName + ".zip"
		cleanupChannel := util.HandleInterrupts(func() {
			util.CleanUpFile(updateZipName)
			util.CleanUpFile(destination)
		})

		logger.Debug(fmt.Sprintf("Copying modified %s file to %s.", constant.UPDATE_DESCRIPTOR_V3_FILE,
			resumedFile.explodedUpdateDirectoryPath))
		err = util.CopyFile(source, destination)
		if err != nil {
			util.HandleErrorAndExit(err, fmt.Sprintf("error occured when copying the modified %s to %s.",
				constant.UPDATE_DESCRIPTOR_V3_FILE, resumedFile.explodedUpdateDirectoryPath))
		}
		logger.Debug(fmt.Sprintf("Resources required for '%s' successfully generated at %s.\n", resumedFile.updateName,
			resumedFile.explodedUpdateDirectoryPath))
		// Create the update zip
		createUpdateZip(&resumedFile)
		// Validate the created update zip
		validateUpdate(&resumedFile)

		signal.Stop(cleanupChannel)
		// Remove the temp directories and files
		util.CleanUpDirectory(constant.TEMP_DIR)

		// Update '.wum-uc-resume.yaml' file as the update zip created successfully
		resumedFile.isUpdateZipCreated = true
		saveResumeFile(&resumedFile)
		fmt.Println(fmt.Sprintf("'%s'.zip successfully created.\n", resumedFile.updateName))
		logger.Debug(fmt.Sprintf("%s successfully updated with the status of update zip creation", constant.WUMUC_RESUME_FILE))
		// Todo commit to SVN (uncomment the below line)
		//commitUpdateToSVN(&resumedFile)
		util.CleanUpFile(wumucResumeFile)
	}
}

// This function will create the update zip.
func createUpdateZip(resumeFile *resumeFile) {
	// Construct the update zip name
	updateZipName := resumeFile.updateName + ".zip"
	logger.Debug(fmt.Sprintf("Name of the update zip: %s", updateZipName))
	logger.Debug(fmt.Sprintf("Creating the update zip %s", updateZipName))
	err := ZipFile(resumeFile.explodedUpdateDirectoryPath, updateZipName)
	if err != nil {
		util.HandleErrorAndExit(err, "error occurred when compressing the update zip.")
	}
	logger.Debug(fmt.Sprintf("Update zip %s created successfully.", updateZipName))
}

// This function will validate the created update zip before committing it to the pointed SVN.
func validateUpdate(resumeFile *resumeFile) {
	// Get absolute location of the created update zip
	updateZipName := resumeFile.updateName + ".zip"
	updateZipPath, err := filepath.Abs(updateZipName)
	if err != nil {
		updateZipPath = updateZipName
	}
	startValidation(updateZipPath, resumeFile.distributionPath)
}

// This function will commit the create update zip to the SVN.
//func commitUpdateToSVN(resumeFile *resumeFile) {
func commitUpdateToSVN() {
	var stdOut, stdErr bytes.Buffer
	//-------------Testing----------------
	resumeFile := resumeFile{}
	resumeFile.updateName = "update0013"
	resumeFile.developer = "admin"
	resumeFile.platformName = "hamming"
	//-------------Testing-done----------------

	// Todo Checkout to see if the update no is there in SVN
	//Todo handle interuppts and delete created iupdate zip when commiting SVN failed.
	// Retry committing update when failed.
	// If not create the folder using a SVN commit
	// If it exists go to the folder see if the location is locked, if locked err. If not locked move the zip (
	// should only have the update zip) to old/old-T and SVN add the new file, then commit to the root of relevant update

	/*	// Request password from user for committing created update to the SVN
		reader := bufio.NewReader(os.Stdin)
		fmt.Fprintf(os.Stdout, "Enter password for %s for committing the update to the SVN: ", resumeFile.developer)
		password, err := reader.ReadString('\n')
		password= strings.TrimSpace(password)
		if err != nil {
			util.HandleErrorAndExit(err, constant.UNABLE_TO_READ_YOUR_INPUT_MSG)
		}*/

	// Todo send pointer of resumeFile to SVN methods
	//-------------Testing----------------

	password = "admin"

	//SVNURI := constant.SVN_UPDATE_REPO + "/" + resumeFile.platformName + "/" + constant.SVN_UPDATES
	//SVNURI := "http://172.17.0.2/svn/repo"
	SVNURI := "http://localhost:8090/svn/repo"
	//-------------Testing-done----------------

	updateSVNURI := SVNURI + "/" + constant.SVN_UPDATE + resumeFile.updateNumber

	// First need to checkout whether the given update is already committed to the SVN.
	SVNListCommand := exec.Command(constant.SVN_COMMAND, constant.LIST_COMMAND, updateSVNURI, constant.USER_NAME,
		resumeFile.developer, constant.PASSWORD, password)
	// Todo have two bytes.Buffers for stdOut and stdErr and then use command.Run()
	//Todo in here catch the error and then see if the echo_command in 0 or 1 (exit code)
	// Todo --non-interactive --no-auth-cache --username XXXX --password YYYY as https svn https://stackoverflow.com/questions/34687/subversion-ignoring-password-and-username-options
	//Todo https://stackoverflow.com/questions/10385551/get-exit-code-go use this to solve,
	//Todo make this repo private
	// do not call $? it just returns the exit code
	SVNListCommand.Stdout = &stdOut
	SVNListCommand.Stderr = &stdErr
	err := SVNListCommand.Run()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			// err interface value holds a value of type exec.ExitError
			ws := exitError.Sys().(syscall.WaitStatus)
			exitCode := ws.ExitStatus()
			logger.Debug(fmt.Sprintf("Error occured in SVNListCommand, \n err: %s , exit code: %d", err, exitCode))
			if exitCode == 1 {
				/*The update directory does not exists at SVN Repo.
				So we need to create the update directory at SVN and commit the created update zip to the SVN.*/
				commitNewUpdateToSVN(resumeFile, updateSVNURI)
			} else {
				// Error occurred due to an exit code other than zero
				util.HandleErrorAndExit(err, fmt.Sprintf("error occurred when checking the existance of %s at SVN, "+
					"exit code: %d", resumeFile.updateName, exitCode))
			}
		} else {
			util.HandleErrorAndExit(err, fmt.Sprintf("error occurred when checking the existance of %s at SVN.",
				resumeFile.updateName))
		}
	}
	// Successfully ran the 'svn ls' command,
	// so update directory should already exists in the SVN and the exit code should be zero
	commitUpgradedUpdateToSVN(resumeFile, updateSVNURI)
}

// This function creates the update directory at SVN and commit the created update zip to the SVN.
func commitNewUpdateToSVN(resumeFile resumeFile, updateSVNURI string) {
	//todo have byte.buffers
	// Todo print stdout and stderr in debug logs
	var stdOut, stdErr bytes.Buffer

	SVNCommitMsg := fmt.Sprintf("Add resources for %s", resumeFile.updateName)
	SVNMkdirCommand := exec.Command(constant.SVN_COMMAND, constant.MKDIR_COMMAND, constant.COMMIT_OPTION,
		fmt.Sprintf("%q", SVNCommitMsg), updateSVNURI, constant.USER_NAME,
		resumeFile.developer, constant.PASSWORD, password)
	SVNMkdirCommand.Stdout = &stdOut
	SVNMkdirCommand.Stderr = &stdErr
	err := SVNMkdirCommand.Run()
	logger.Debug(fmt.Sprintf("stdout of SVNMkdirCommand %s", stdOut.String()))
	if err != nil {
		logger.Debug(fmt.Sprintf("stderr of SVNMkdirCommand %s", stdErr.String()))
		util.HandleErrorAndExit(err, fmt.Sprintf("error occurred when creating %s directory at SVN.",
			resumeFile.updateNumber))
	}

	// Checkout the created update directory at SVN
	SVNCheckoutCommand := exec.Command(constant.SVN_COMMAND, constant.CHECKOUT_COMMAND, updateSVNURI,
		constant.USER_NAME, resumeFile.developer, constant.PASSWORD, password)
	SVNCheckoutCommand.Dir = WUMUCHome
	SVNCheckoutCommand.Stdout = &stdOut
	SVNCheckoutCommand.Stderr = &stdErr
	err = SVNCheckoutCommand.Run()
	logger.Debug(fmt.Sprintf("stdout of SVNCheckoutCommand %s", stdOut.String()))
	if err != nil {
		logger.Debug(fmt.Sprintf("stderr of SVNCheckoutCommand %s", stdErr.String()))
		util.HandleErrorAndExit(err, fmt.Sprintf("error occurred when checking out %s directory at SVN.",
			resumeFile.updateNumber))
	}

	// Copy the created update.zip file to the checkout location
	updateZipName := resumeFile.updateName + ".zip"
	updateDirectory := constant.SVN_UPDATE + resumeFile.updateNumber
	updateDirectoryPath := path.Join(WUMUCHome, updateDirectory)
	err = util.CopyFile(updateZipName, updateDirectoryPath)
	if err != nil {
		util.HandleErrorAndExit(err, fmt.Sprintf("error occured when copying the %s to %s", updateZipName,
			updateDirectoryPath))
	}
	logger.Debug(fmt.Sprintf("%s successfully copied to %s for commiting to the SVN", updateZipName, updateDirectoryPath))

	// Add the update directory to the SVN
	performSVNAddCommand(&resumeFile, false)

	// Commit the created update to SVN
	performSVNCommitCommand(&resumeFile)
	// Todo to keep or deleted the update directory after sucessfull commiting
}

// This function change the directory structure of given update at SVN and commit the newly created update zip.
func commitUpgradedUpdateToSVN(resumeFile resumeFile, updateSVNURI string) {
	var stdOut, stdErr bytes.Buffer

	updateDirectory := constant.SVN_UPDATE + resumeFile.updateNumber
	// As the update directory exists, we need to checkout the existing update directory from SVN
	SVNCheckoutCommand := exec.Command(constant.SVN_COMMAND, constant.CHECKOUT_COMMAND, updateSVNURI,
		constant.USER_NAME, resumeFile.developer, constant.PASSWORD, password)
	SVNCheckoutCommand.Dir = WUMUCHome
	SVNCheckoutCommand.Stdout = &stdOut
	SVNCheckoutCommand.Stderr = &stdErr
	err := SVNCheckoutCommand.Run()
	logger.Debug(fmt.Sprintf("stdout of SVNCheckoutCommand %s", stdOut.String()))
	if err != nil {
		logger.Debug(fmt.Sprintf("stderr of SVNCheckoutCommand %s", stdErr.String()))
		util.HandleErrorAndExit(err, fmt.Sprintf("error occurred when checkingout %s directory at SVN.",
			updateDirectory))
	}
	// Check if `old` directory exists
	updateDirectoryPath := path.Join(WUMUCHome, updateDirectory)
	oldUpdatesDirectoryPath := path.Join(updateDirectoryPath, constant.OLD_UPDATE_DIRECTORY)
	exists, err := util.IsDirectoryExists(oldUpdatesDirectoryPath)
	if err != nil {
		util.HandleErrorAndExit(err, fmt.Sprintf("error occurred when checking the existance of %s directory",
			oldUpdatesDirectoryPath))
	}
	if !exists {
		// Same update is being created for the second time
		// Create 'old-updates' directory for storing old updates
		err := util.CreateDirectory(oldUpdatesDirectoryPath)
		if err != nil {
			util.HandleErrorAndExit(err, fmt.Sprintf("error occurred when creating the directory %s", oldUpdatesDirectoryPath))
		}
		commitUpgradedUpdatesWithPreservingPreviousUpdatesAtSVN(&resumeFile)
	} else {
		// Same update is being created more than two times
		commitUpgradedUpdatesWithPreservingPreviousUpdatesAtSVN(&resumeFile)
	}

	// Todo Check whether the update zip exists.
}

// Todo doc here
func commitUpgradedUpdatesWithPreservingPreviousUpdatesAtSVN(resumeFile *resumeFile) {
	updateZipName := resumeFile.updateName + ".zip"
	updateDirectory := constant.SVN_UPDATE + resumeFile.updateNumber
	updateDirectoryPath := path.Join(WUMUCHome, updateDirectory)
	oldUpdatesDirectoryPath := path.Join(updateDirectoryPath, constant.OLD_UPDATE_DIRECTORY)

	// Copy previous update.zip to the `old` directory
	oldUpdateZipPath := path.Join(updateDirectoryPath, updateZipName)
	util.MoveFile(oldUpdateZipPath, oldUpdatesDirectoryPath)
	// Get the current timestamp
	utcTime := time2.Now().UTC()
	nanoSecUTCTime := utcTime.UnixNano()
	miliSecUTCTime := nanoSecUTCTime / 1000000
	// Append the current timestamp for previous update zip
	oldUpdateZipPath = path.Join(oldUpdatesDirectoryPath, updateZipName)
	newUpdateZipName := resumeFile.updateName + "." + string(miliSecUTCTime) + ".zip"
	newUpdateZipPath := path.Join(oldUpdatesDirectoryPath, newUpdateZipName)
	os.Rename(oldUpdateZipPath, newUpdateZipPath)
	// Copy newly created update.zip to the checkout directory
	err := util.CopyFile(updateZipName, updateDirectoryPath)
	if err != nil {
		util.HandleErrorAndExit(err, fmt.Sprintf("error occurred when copying %s to %s", updateZipName, updateDirectoryPath))
	}
	// Add file changes to the SVN pending change list
	performSVNAddCommand(resumeFile, true)
	// Commit the changes to SVN
	performSVNCommitCommand(resumeFile)
}

// This function add files to the SVN pending change list.
// Todo add logs
func performSVNAddCommand(resumeFile *resumeFile, isOldUpdatesExists bool) {
	var stdOut, stdErr bytes.Buffer

	updateZipName := resumeFile.updateName + ".zip"
	updateDirectory := constant.SVN_UPDATE + resumeFile.updateNumber
	updateDirectoryPath := path.Join(WUMUCHome, updateDirectory)

	if isOldUpdatesExists {
		// Add newly created update zip and old updates to SVN pending change list.
		oldUpdatesDirectoryPath := path.Join(updateDirectoryPath, constant.OLD_UPDATE_DIRECTORY)
		SVNAddCommand := exec.Command(constant.SVN_COMMAND, constant.ADD_COMMAND, oldUpdatesDirectoryPath, updateZipName)
		SVNAddCommand.Dir = updateDirectoryPath
		SVNAddCommand.Stdout = &stdOut
		SVNAddCommand.Stderr = &stdErr
		err := SVNAddCommand.Run()
		logger.Debug(fmt.Sprintf("stdout of SVNAddCommand %s", stdOut.String()))
		if err != nil {
			logger.Debug(fmt.Sprintf("stderr of SVNAddCommand %s", stdErr.String()))
			util.HandleErrorAndExit(err,
				fmt.Sprintf("error occurred when adding contents of %s directory to SVN pending change list."+
					"", updateDirectory))
		}
	} else {
		// Add newly created update zip to SVN pending change list.
		SVNAddCommand := exec.Command(constant.SVN_COMMAND, constant.ADD_COMMAND, updateZipName)
		SVNAddCommand.Dir = updateDirectoryPath
		SVNAddCommand.Stdout = &stdOut
		SVNAddCommand.Stderr = &stdErr
		err := SVNAddCommand.Run()
		logger.Debug(fmt.Sprintf("stdout of SVNAddCommand %s", stdOut.String()))
		if err != nil {
			logger.Debug(fmt.Sprintf("stderr of SVNAddCommand %s", stdErr.String()))
			util.HandleErrorAndExit(err,
				fmt.Sprintf("error occurred when adding contents of %s directory to SVN pending change list."+
					"", updateDirectory))
		}
	}
}

func performSVNCommitCommand(resumeFile *resumeFile) {
	var stdOut, stdErr bytes.Buffer
	updateDirectory := constant.SVN_UPDATE + resumeFile.updateNumber
	updateDirectoryPath := path.Join(WUMUCHome, updateDirectory)

	// Commit the created update to SVN
	SVNCommitMsg := fmt.Sprintf("Add %s", resumeFile.updateName)
	SVNCommitCommand := exec.Command(constant.SVN_COMMAND, constant.COMMIT_COMMAND, constant.COMMIT_OPTION,
		fmt.Sprintf("%q", SVNCommitMsg))
	SVNCommitCommand.Dir = updateDirectoryPath
	SVNCommitCommand.Stdout = &stdOut
	SVNCommitCommand.Stderr = &stdErr
	err := SVNCommitCommand.Run()
	logger.Debug(fmt.Sprintf("stdout of SVNCommitCommand %s", stdOut.String()))
	if err != nil {
		logger.Debug(fmt.Sprintf("stderr of SVNCommitCommand %s", stdErr.String()))
		util.HandleErrorAndExit(err, fmt.Sprintf("error occurred when commiting contents of %s directory to SVN.",
			updateDirectory))
	}
}
