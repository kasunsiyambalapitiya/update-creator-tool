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

	"github.com/olekukonko/tablewriter"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wso2/update-creator-tool/constant"
	"github.com/wso2/update-creator-tool/util"
	"gopkg.in/yaml.v2"
	"regexp"
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

// This function will be called first and this will add flags to the command.
func init() {
	RootCmd.AddCommand(createCmd)

	createCmd.Flags().BoolVarP(&isDebugLogsEnabled, "debug", "d", util.EnableDebugLogs, "Enable debug logs")
	createCmd.Flags().BoolVarP(&isTraceLogsEnabled, "trace", "t", util.EnableTraceLogs, "Enable trace logs")

	createCmd.Flags().BoolP("md5", "m", util.CheckMd5Disabled, "Disable checking MD5 sum")
	viper.BindPFlag(constant.CHECK_MD5_DISABLED, createCmd.Flags().Lookup("md5"))
}

// This function will be called when the create command is called.
func initializeCreateCommand(cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		util.HandleErrorAndExit(errors.New("invalid number of argumants. Run 'wum-uc create --help' to " +
			"view help"))
	}
	createUpdate(args[0], args[1])
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
		skip := false
		if !exists {
		userInputLoop:
			for {
				util.PrintInBold(fmt.Sprintf("'%s'does not exists. Do you want to create '%s' directory?"+
					"[Y/n]: ", updateDirectoryPath, updateDirectoryPath))
				preference, err := util.GetUserInput()
				if len(preference) == 0 {
					preference = "y"
				}
				// Todo to remove redudant call, call this only if error is not null
				util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")

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
					skip = true
					break userInputLoop
				default:
					//Todo asked, as here the for loop doesnot breaks on default, will iterate till
					util.PrintError("Invalid preference. Enter Y for Yes or N for No.")
				}
			}
		}
		// If the skip is selected, exit
		if skip {
			util.HandleErrorAndExit(errors.New("directory creation skipped. Please enter a valid directory"))
		}
	}
	updateRoot := strings.TrimSuffix(updateDirectoryPath, constant.PATH_SEPARATOR)
	logger.Debug(fmt.Sprintf("updateRoot: %s\n", updateRoot))
	viper.Set(constant.UPDATE_ROOT, updateRoot)

	// Create new update descriptor structs
	updateDescriptorV2 := util.UpdateDescriptorV2{}
	updateDescriptorV3 := util.UpdateDescriptorV3{}

	//2) Process the README.txt file if it exists
	readMeDataString := getReadMeData(updateDirectoryPath)

	//4) Check whether the given distribution exists
	exists, err = util.IsFileExists(distributionPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while checking '%s'", distributionPath))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("File does not exist at '%s'. Distribution must "+
			"be a zip file.", distributionPath)))
	}
	// Checks whether the given distribution is a zip file
	util.IsZipFile(constant.DISTRIBUTION, distributionPath)

	//5) Validate UpdateDescriptorV2 after user inputs
	//Todo use this in validation
	/*	err = util.ValidateUpdateDescriptor(&updateDescriptorV2)
		util.HandleErrorAndExit(err, fmt.Sprintf("'%s' format is incorrect.", constant.UPDATE_DESCRIPTOR_V2_FILE))*/

	// Set the update name
	updateName := getUpdateName(&updateDescriptorV2, constant.UPDATE_NAME_PREFIX)
	viper.Set(constant.UPDATE_NAME, updateName)

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
	util.PrintInfo(fmt.Sprintf("Reading %s. Please wait...", distributionName))
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

	//9) Copy resource files (update-descriptor.yaml, etc) to temp directory
	resourceFiles := getResourceFiles()
	err = copyResourceFilesToTempDir(resourceFiles)
	util.HandleErrorAndExit(err, errors.New("Error occurred while copying resource files."))

	// Todo Check this comment Save the updated update-descriptor with newly added,
	// modified and removed files to the temp directory

	data, err := marshalUpdateDescriptor(&updateDescriptorV2)
	util.HandleErrorAndExit(err, "Error occurred while marshalling the update-descriptorV2.")
	err = saveUpdateDescriptor(constant.UPDATE_DESCRIPTOR_V2_FILE, data)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while saving the '%v'.",
		constant.UPDATE_DESCRIPTOR_V2_FILE))

	// Get partial updated file changes
	partialUpdatedFileResponse := util.GetPartialUpdatedFiles(&updateDescriptorV2)
	if partialUpdatedFileResponse.Backward_compatible {
		// Create update-descriptor.yaml
		if len(readMeDataString) != 0 {
			processReadMeData(&readMeDataString, &updateDescriptorV2)
		} else {
			setValuesForUpdateDescriptorsV2(&updateDescriptorV2)
		}
		// Reqeust the user to add removed files as they can't be identified by comparing.
		util.PrintInBold("Enter relative paths of removed files, please enter 'done' when you are finished entering")
		fmt.Println()
		//Todo uncomment
		/*	for {
			removedFile, err := util.GetUserInput()
			util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
			if strings.ToLower(removedFile) == "done" {
				return
			}
			updateDescriptorV2.File_changes.Removed_files = append(updateDescriptorV2.File_changes.Removed_files, removedFile)
		}*/
		createUpdateDescriptorV2(updateDirectoryPath, &updateDescriptorV2)
	}

	// Set values for UpdateDescriptorV3
	updateDescriptorV3.Update_number = partialUpdatedFileResponse.Update_number
	updateDescriptorV3.Platform_name = partialUpdatedFileResponse.Platform_name
	updateDescriptorV3.Platform_version = partialUpdatedFileResponse.Platform_version
	for _, partialUpdatedProducts := range partialUpdatedFileResponse.Compatible_products {
		productChanges := setProductChangesInUpdateDescriptorV3(&partialUpdatedProducts)
		updateDescriptorV3.Compatible_products = append(updateDescriptorV3.Compatible_products, *productChanges)
	}
	for _, partialUpdatedProducts := range partialUpdatedFileResponse.Applicable_products {
		productChanges := setProductChangesInUpdateDescriptorV3(&partialUpdatedProducts)
		updateDescriptorV3.Applicable_products = append(updateDescriptorV3.Applicable_products, *productChanges)
	}
	for _, partialUpdatedProducts := range partialUpdatedFileResponse.Notify_products {
		productChanges := setProductChangesInUpdateDescriptorV3(&partialUpdatedProducts)
		updateDescriptorV3.Notify_products = append(updateDescriptorV3.Notify_products, *productChanges)
	}
	createUpdateDescriptorV3(updateDirectoryPath, &updateDescriptorV3)
	// Save the updated update-descriptor3.yaml
	data, err = yaml.Marshal(updateDescriptorV3)
	util.HandleErrorAndExit(err, "Error occurred while marshalling the update-descriptorV3.")
	err = saveUpdateDescriptor(constant.UPDATE_DESCRIPTOR_V3_FILE, data)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while saving the '%v'.",
		constant.UPDATE_DESCRIPTOR_V3_FILE))

	// Construct the update zip name
	updateZipName := updateName + ".zip"
	logger.Debug(fmt.Sprintf("updateZipName: %s", updateZipName))

	targetDirectory := path.Join(constant.TEMP_DIR, updateName)
	targetDirectory = strings.Replace(targetDirectory, "/", constant.PATH_SEPARATOR, -1)

	logger.Debug(fmt.Sprintf("targetDirectory: %s", targetDirectory))
	err = ZipFile(targetDirectory, updateZipName)
	util.HandleErrorAndExit(err)

	// Remove the temp directories
	util.CleanUpDirectory(constant.TEMP_DIR)

	signal.Stop(cleanupChannel)

	util.PrintInfo(fmt.Sprintf("'%s' successfully created.", updateZipName))
	util.PrintInfo(fmt.Sprintf("Please manually fill the  `description` and `instructions` fields of compatible"+
		",applicable and notify products in the update-descriptor3."+
		"yaml located inside the created '%s'\n",
		updateZipName))
}

// This function will process the README.txt file and extract details to populate update-descriptor.yaml and
// update-descriptor3.yaml. If some data cannot be extracted, it will add default values and continue.
func getReadMeData(updateDirectoryPath string) string {
	// Construct the README.txt path
	readMePath := path.Join(updateDirectoryPath, constant.README_FILE)
	logger.Debug(fmt.Sprintf("README Path: %v", readMePath))
	// Check whether the README.txt file exists
	_, err := os.Stat(readMePath)
	if err != nil {
		// If the file does not exist or any other error occur, return without printing warning messages
		logger.Debug(fmt.Sprintf("%s not found", readMePath))
		return ""
	}
	// Read the README.txt file
	data, err := ioutil.ReadFile(readMePath)
	if err != nil {
		// If any error occurs, return without printing warning messages
		logger.Debug(fmt.Sprintf("Error occurred in processing README: %v", err))
		return ""
	}

	logger.Debug("README.txt found")

	// Convert the byte array to a string
	return string(data)
}

func processReadMeData(readMeDataString *string, updateDescriptorV2 *util.UpdateDescriptorV2) {
	logger.Debug("Processing README started")
	// Compile the regex
	regex, err := regexp.Compile(constant.PATCH_ID_REGEX)
	if err == nil {
		result := regex.FindStringSubmatch(*readMeDataString)
		logger.Trace(fmt.Sprintf("PATCH_ID_REGEX result: %v", result))
		// Since the regex has 2 capturing groups, the result size will be 3 (because there is the full match)
		// If not match found, the size will be 0. We check whether the result size is not 0 to make sure both
		// capturing groups are identified.
		if len(result) != 0 {
			// Extract details
			updateDescriptorV2.Update_number = result[2]
			updateDescriptorV2.Platform_version = result[1]
			platformsMap := viper.GetStringMapString(constant.PLATFORM_VERSIONS)
			logger.Trace(fmt.Sprintf("Platform Map: %v", platformsMap))
			// Get the platform details from the map
			platformName, found := platformsMap[result[1]]
			if found {
				logger.Debug("PlatformName found in configs")
				updateDescriptorV2.Platform_name = platformName
			} else {
				//If the platform name is not found, set default
				logger.Debug("No matching platform name found for:", result[1])
				util.PrintInBold("Enter platform name for platform version :", result[1])
				platformName, err := util.GetUserInput()
				util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
				updateDescriptorV2.Platform_name = platformName
			}
		} else {
			logger.Debug("PATCH_ID_REGEX results incorrect:", result)
			setUpdateNumber(updateDescriptorV2)
			setPlatformVersion(updateDescriptorV2)
			setPlatformName(updateDescriptorV2)
		}
	} else {
		//If error occurred, set default values
		logger.Debug(fmt.Sprintf("Error occurred while processing PATCH_ID_REGEX: %v", err))
		setUpdateNumber(updateDescriptorV2)
		setPlatformVersion(updateDescriptorV2)
		setPlatformName(updateDescriptorV2)
	}

	// Compile the regex
	regex, err = regexp.Compile(constant.APPLIES_TO_REGEX)
	if err == nil {
		result := regex.FindStringSubmatch(*readMeDataString)
		logger.Trace(fmt.Sprintf("APPLIES_TO_REGEX result: %v", result))
		// In the README, Associated Jiras section might not appear. If it does appear, result size will be 2.
		// If it does not appear, result size will be 3.
		if len(result) == 2 {
			// If the result size is 2, we know that 1st index contains the 1st capturing group.
			updateDescriptorV2.Applies_to = util.ProcessString(result[1], ", ", true)
		} else if len(result) == 3 {
			// If the result size is 3, 1st or 2nd string might contain the match. So we concat them
			// together and trim the spaces. If one field has an empty string, it will be trimmed.
			updateDescriptorV2.Applies_to = util.ProcessString(strings.TrimSpace(result[1]+result[2]), ", ",
				true)
		} else {
			logger.Debug("No matching results found for APPLIES_TO_REGEX:", result)
			setAppliesTo(updateDescriptorV2)
		}
	} else {
		//If error occurred, set default value
		logger.Debug(fmt.Sprintf("Error occurred while processing APPLIES_TO_REGEX: %v", err))
		setAppliesTo(updateDescriptorV2)
	}

	// Compile the regex
	regex, err = regexp.Compile(constant.ASSOCIATED_JIRAS_REGEX)
	if err == nil {
		// Get all matches because there might be multiple Jiras.
		allResult := regex.FindAllStringSubmatch(*readMeDataString, -1)
		logger.Trace(fmt.Sprintf("APPLIES_TO_REGEX result: %v", allResult))
		updateDescriptorV2.Bug_fixes = make(map[string]string)
		// If no Jiras found, set 'N/A: N/A' as the value
		if len(allResult) == 0 {
			logger.Debug("No matching results found for ASSOCIATED_JIRAS_REGEX. Setting default values.")
			updateDescriptorV2.Bug_fixes[constant.JIRA_NA] = constant.JIRA_NA
		} else {
			// If Jiras found, get summary for all Jiras
			logger.Debug("Matching results found for ASSOCIATED_JIRAS_REGEX")
			for i, match := range allResult {
				// Regex has a one capturing group. So the jira ID will be in the 1st index.
				logger.Debug(fmt.Sprintf("%d: %s", i, match[1]))
				logger.Debug(fmt.Sprintf("ASSOCIATED_JIRAS_REGEX results is correct: %v", match))
				updateDescriptorV2.Bug_fixes[match[1]] = util.GetJiraSummary(match[1])
			}
		}
	} else {
		//If error occurred, set default values
		logger.Debug(fmt.Sprintf("Error occurred while processing ASSOCIATED_JIRAS_REGEX: %v", err))
		logger.Debug("Setting default values to bug_fixes")
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
		//If error occurred, set default values
		logger.Debug(fmt.Sprintf("Error occurred while processing DESCRIPTION_REGEX: %v", err))
		setDescription(updateDescriptorV2)
	}
	logger.Debug("Processing README finished")
}

//This function will set values to the update-descriptor.yaml.
func setValuesForUpdateDescriptorsV2(updateDescriptorV2 *util.UpdateDescriptorV2) {
	logger.Debug("Setting values for update-descriptor.yaml")
	setUpdateNumber(updateDescriptorV2)
	setPlatformName(updateDescriptorV2)
	setPlatformVersion(updateDescriptorV2)
	setDescription(updateDescriptorV2)
	setAppliesTo(updateDescriptorV2)
	setBugFixes(updateDescriptorV2)
}

func setUpdateNumber(updateDescriptorV2 *util.UpdateDescriptorV2) {
	util.PrintInBold("Enter update number: ")
	updateNumber, err := util.GetUserInput()
	util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
	updateDescriptorV2.Update_number = updateNumber
}

func setPlatformName(updateDescriptorV2 *util.UpdateDescriptorV2) {
	util.PrintInBold("Enter platform name: ")
	platformName, err := util.GetUserInput()
	util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
	updateDescriptorV2.Platform_name = platformName
}

func setPlatformVersion(updateDescriptorV2 *util.UpdateDescriptorV2) {
	util.PrintInBold("Enter platform version: ")
	platformVersion, err := util.GetUserInput()
	util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
	updateDescriptorV2.Platform_version = platformVersion
}

func setAppliesTo(updateDescriptorV2 *util.UpdateDescriptorV2) {
	util.PrintInBold("Enter applies to: ")
	appliesTo, err := util.GetUserInput()
	util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
	updateDescriptorV2.Applies_to = appliesTo
}

func setDescription(updateDescriptorV2 *util.UpdateDescriptorV2) {
	util.PrintInBold("Enter description: ")
	description, err := util.GetUserInput()
	util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
	updateDescriptorV2.Description = description
}

func setBugFixes(updateDescriptorV2 *util.UpdateDescriptorV2) {
	util.PrintInBold("Enter Bug fixes, please enter 'done' when you are finished adding")
	fmt.Println()
	bugFixes := make(map[string]string)
	for {
		// Todo refactor them to constants, and change constant.JIRA_KEY_DEFAULT and try to make them on using ||
		util.PrintInBold("Enter JIRA_KEY/GITHUB ISSUE URL: ")
		jiraKey, err := util.GetUserInput()
		util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
		if strings.ToLower(jiraKey) == "done" {
			if len(bugFixes) == 0 {
				bugFixes[constant.JIRA_NA] = constant.JIRA_NA
			}
			logger.Debug(fmt.Sprintf("bug_fixes: %v", bugFixes))
			updateDescriptorV2.Bug_fixes = bugFixes
			return
		}
		util.PrintInBold("Enter JIRA_KEY SUMMARY/GITHUB_ISSUE_SUMMARY: ")
		jiraSummary, err := util.GetUserInput()
		util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
		if strings.ToLower(jiraSummary) == "done" {
			if len(bugFixes) == 0 {
				bugFixes[constant.JIRA_NA] = constant.JIRA_NA
			}
			logger.Debug(fmt.Sprintf("bug_fixes: %v", bugFixes))
			updateDescriptorV2.Bug_fixes = bugFixes
			return
		}
		bugFixes[jiraKey] = jiraSummary
	}
}

func createUpdateDescriptorV2(updateDirectoryPath string, updateDescriptorV2 *util.UpdateDescriptorV2) {
	// Marshall update descriptor structs
	dataV2, err := yaml.Marshal(updateDescriptorV2)
	util.HandleErrorAndExit(err)

	dataStringV2 := string(dataV2)

	//remove " enclosing the update number
	dataStringV2 = strings.Replace(dataStringV2, "\"", "", -1)
	logger.Debug(fmt.Sprintf("update-descriptorV2:\n%s", dataStringV2))

	// Construct update descriptor file paths
	updateDescriptorFileV2 := filepath.Join(updateDirectoryPath, constant.UPDATE_DESCRIPTOR_V2_FILE)
	logger.Debug(fmt.Sprintf("updateDescriptorFileV2: %v", updateDescriptorFileV2))

	// Save update descriptors
	absDestinationV2 := saveUpdateDescriptorInDestination(updateDescriptorFileV2, dataStringV2, updateDirectoryPath)
	util.PrintInfo(fmt.Sprintf("'%s' has been successfully created at '%s'.", constant.UPDATE_DESCRIPTOR_V2_FILE,
		absDestinationV2))
}

func createUpdateDescriptorV3(updateDirectoryPath string, updateDescriptorV3 *util.UpdateDescriptorV3) {
	// Marshall update descriptor structs
	dataV3, err := yaml.Marshal(updateDescriptorV3)
	util.HandleErrorAndExit(err)
	dataStringV3 := string(dataV3)

	//remove " enclosing the update number
	dataStringV3 = strings.Replace(dataStringV3, "\"", "", -1)
	logger.Debug(fmt.Sprintf("update-descriptorV3:\n%s", dataStringV3))

	// Construct update descriptor file paths
	updateDescriptorFileV3 := filepath.Join(updateDirectoryPath, constant.UPDATE_DESCRIPTOR_V3_FILE)
	logger.Debug(fmt.Sprintf("updateDescriptorFileV3: %v", updateDescriptorFileV3))

	// Save update descriptors
	absDestinationV3 := saveUpdateDescriptorInDestination(updateDescriptorFileV3, dataStringV3, updateDirectoryPath)
	util.PrintInfo(fmt.Sprintf("'%s' has been successfully created at '%s'.", constant.UPDATE_DESCRIPTOR_V3_FILE,
		absDestinationV3))
}

func saveUpdateDescriptorInDestination(updateDescriptorFilePath, dataString, destination string) string {
	file, err := os.OpenFile(
		updateDescriptorFilePath,
		os.O_WRONLY|os.O_TRUNC|os.O_CREATE,
		0600,
	)
	util.HandleErrorAndExit(err)
	defer file.Close()

	// Write bytes to file
	_, err = file.Write([]byte(dataString))
	if err != nil {
		util.HandleErrorAndExit(err)
	}

	// Get the absolute location
	absDestination, err := filepath.Abs(destination)
	if err != nil {
		absDestination = destination
	}
	return absDestination
}

// Todo delete them
/*func checkUpdateDescriptors(updateDirectoryPath string, updateDescriptorV2 *util.UpdateDescriptorV2,
	updateDescriptorV3 *util.UpdateDescriptorV3) {
	exists := checkUpdateDescriptor(updateDirectoryPath, constant.UPDATE_DESCRIPTOR_V2_FILE)
	if exists {
		// validate its content
	} else {
		// Create the file in provided location
	}
	checkUpdateDescriptor(updateDirectoryPath, constant.UPDATE_DESCRIPTOR_V3_FILE)
}*/

/*
func checkUpdateDescriptor(updateDirectoryPath, updateDescriptor string) bool {
	// Construct the update-descriptor file location
	updateDescriptorPath := path.Join(updateDirectoryPath, updateDescriptor)
	exists, err := util.IsFileExists(updateDescriptorPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while checking for the '%s'",
		updateDescriptor))
	logger.Debug(fmt.Sprintf("%s exists. Location %s", updateDescriptor, updateDescriptorPath))
	return exists
}*/

// This function will set the update name which will be used when creating the update zip.
func getUpdateName(updateDescriptor *util.UpdateDescriptorV2, updateNamePrefix string) string {
	// Read the corresponding details from the struct
	platformVersion := updateDescriptor.Platform_version
	updateNumber := updateDescriptor.Update_number
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
		util.PrintInBold("Do you want to add it as a new file? [y/N]: ")
		preference, err := util.GetUserInput()
		if len(preference) == 0 {
			preference = "n"
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
		util.PrintInBold("Enter destination directory relative to CARBON_HOME: ")
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
			util.HandleErrorAndExit(errors.New("updateRoot path length is 0."))
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

// This function will save update descriptor after modifying the file_changes section.
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
		updateName := viper.GetString(constant.UPDATE_NAME)
		source := path.Join(updateRoot, filename)
		destination := path.Join(constant.TEMP_DIR, updateName, filename)
		// Copy the file
		err := util.CopyFile(source, destination)
		if err != nil {
			// If an error occurs while copying, if the file is a mandatory file, return an error. If the
			// file is not mandatory, print a message and continue.
			if isMandatory {
				return err
			} else {
				util.PrintInfo(fmt.Sprintf("Optional resource file '%s' not copied.", filename))
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
		updateDescriptor.File_changes.Modified_files = append(updateDescriptor.File_changes.Modified_files,
			relativePath)
	} else {
		updateDescriptor.File_changes.Added_files = append(updateDescriptor.File_changes.Added_files,
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
	defaultBugFixes := map[string]string{
		constant.DEFAULT_JIRA_KEY: constant.DEFAULT_JIRA_SUMMARY,
	}
	productChanges.Product_name = partialUpdatedProducts.Product_name
	productChanges.Product_version = partialUpdatedProducts.Base_version + "." + partialUpdatedProducts.Tag
	productChanges.Description = constant.DEFAULT_DESCRIPTION
	productChanges.Instructions = constant.DEFAULT_INSTRUCTIONS
	productChanges.Bug_fixes = defaultBugFixes
	productChanges.Added_files = partialUpdatedProducts.Added_files
	productChanges.Removed_files = partialUpdatedProducts.Removed_files
	productChanges.Modified_files = partialUpdatedProducts.Modified_files
	return productChanges
}
