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
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wso2/update-creator-tool/constant"
	"github.com/wso2/update-creator-tool/util"
	"gopkg.in/yaml.v2"
)

var (
	initCmdUse       = "init"
	initCmdShortDesc = "Generate '" + constant.UPDATE_DESCRIPTOR_V2_FILE + "' file template"
	initCmdLongDesc  = dedent.Dedent(`
		This command will generate the 'update-descriptor.yaml' file. If
		the user does not specify a directory, it will use the current
		working directory. It will fill the data using any available
		README.txt file in the old patch format. If README.txt is not
		found, it will fill values using default values which you need
		to edit manually.`)
	initCmdExampleV1 = dedent.Dedent(`
		update_number: 0001
		platform_version: 4.4.0
		platform_name: wilkes
		applies_to: All the products based on carbon 4.4.1
		bug_fixes:
		  CARBON-15395: Upgrade Hazelcast version to 3.5.2
		  <Multiple JIRAs or GITHUB Issues>
		description: |
		  This update contain the relavent fixes for upgrading Hazelcast version
		  to its latest 3.5.2 version. When applying this update it requires a
		  full cluster estart since if the nodes has multiple client versions of;
		  Hazelcast it can cause issues during connectivity.
		file_changes:
		  added_files: []
		  removed_files: []
		  modified_files: []
		`)
	initCmdExampleV2 = dedent.Dedent(`
		update_number: 2000
		platform_name: wilkes
		platform_version: 4.4.0
		compatible_products:
		- product_name: wso2am
		 product_version: 2.1.0.sec
		 description: "Description"
		 instructions: "Instructions"
		 bug_fixes:
		   N/A: N/A
		 added_files: []
		 removed_files:
		 - repository/components/plugins/org.wso2.carbon.logging.admin.ui_4.4.7.jar
		 modified_files:
		 - repository/components/plugins/activity-all_5.21.0.wso2v1.jar
		applicable_products: []
		notify-products: []
		`)
	isSampleEnabled bool
)

// initCmd represents the validate command
var initCmd = &cobra.Command{
	Use:   initCmdUse,
	Short: initCmdShortDesc,
	Long:  initCmdLongDesc,
	Run:   initializeInitCommand,
}

//This function will be called first and this will add flags to the command.
func init() {
	RootCmd.AddCommand(initCmd)

	initCmd.Flags().BoolVarP(&isDebugLogsEnabled, "debug", "d", util.EnableDebugLogs, "Enable debug logs")
	initCmd.Flags().BoolVarP(&isTraceLogsEnabled, "trace", "t", util.EnableTraceLogs, "Enable trace logs")
	initCmd.Flags().BoolVarP(&isSampleEnabled, "sample", "s", false, "Show sample file")
}

//This function will be called when the create command is called.
func initializeInitCommand(cmd *cobra.Command, args []string) {
	logger.Debug("[Init] called")
	if isSampleEnabled {
		logger.Debug("-s flag found. Printing sample...")
		fmt.Printf("Sample update-descriptor.yaml \n %s \n\nSample update-descriptor3.yaml \n %s \n", initCmdExampleV1,
			initCmdExampleV2)
	} else {
		switch len(args) {
		case 0:
			logger.Debug("Initializing current working directory.")
			initCurrentDirectory()
		case 1:
			logger.Debug("Initializing directory:", args[0])
			initDirectory(args[0])
		default:
			logger.Debug("Invalid number of arguments:", args)
			util.HandleErrorAndExit(errors.New("Invalid number of arguments. Run 'wum-uc init --help' to view " +
				"help."))
		}
	}
}

//This function will be called if no arguments are provided by the user.
func initCurrentDirectory() {
	currentDirectory := "./"
	initDirectory(currentDirectory)
}

//This function will start the init process.
func initDirectory(destination string) {
	logger.Debug("Initializing started.")
	// Check whether the provided directory exists
	exists, err := util.IsDirectoryExists(destination)
	logger.Debug(fmt.Sprintf("'%s' directory exists: %v", destination, exists))

	// If the directory does not exists, prompt the user
	skip := false
	if !exists {
	userInputLoop:
		for {
			util.PrintInBold(fmt.Sprintf("'%s'does not exists. Do you want to create '%s' directory?"+
				"[Y/n]: ", destination, destination))
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
					destination, destination))
				err := util.CreateDirectory(destination)
				util.HandleErrorAndExit(err)
				logger.Debug(fmt.Sprintf("'%s' directory created.", destination))
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
		util.HandleErrorAndExit(errors.New("Directory creation skipped. Please enter a valid directory."))
	}

	// Create new update descriptor structs
	updateDescriptorV2 := util.UpdateDescriptorV2{}
	updateDescriptorV3 := util.UpdateDescriptorV3{}

	// Download the LICENSE.txt
	downloadFile(destination, constant.LICENSE_URL, constant.LICENSE_DOWNLOAD_URL, constant.LICENSE_FILE)

	// Download the NOT_A_CONTRIBUTION.txt
	downloadFile(destination, constant.NOT_A_CONTRIBUTION_URL, constant.NOT_A_CONTRIBUTION_DOWNLOAD_URL,
		constant.NOT_A_CONTRIBUTION_FILE)

	// Process README.txt and parse values
	processReadMe(destination, &updateDescriptorV2, &updateDescriptorV3)

	// Marshall update descriptor structs
	dataV2, err := yaml.Marshal(&updateDescriptorV2)
	util.HandleErrorAndExit(err)
	dataV3, err := yaml.Marshal(&updateDescriptorV3)
	util.HandleErrorAndExit(err)

	dataStringV2 := string(dataV2)
	dataStringV3 := string(dataV3)

	//remove " enclosing the update number
	dataStringV2 = strings.Replace(dataStringV2, "\"", "", -1)
	logger.Debug(fmt.Sprintf("update-descriptorV2:\n%s", dataStringV2))
	dataStringV3 = strings.Replace(dataStringV3, "\"", "", -1)
	logger.Debug(fmt.Sprintf("update-descriptorV3:\n%s", dataStringV3))

	// Construct update descriptor file paths
	updateDescriptorFileV2 := filepath.Join(destination, constant.UPDATE_DESCRIPTOR_V2_FILE)
	logger.Debug(fmt.Sprintf("updateDescriptorFileV2: %v", updateDescriptorFileV2))
	updateDescriptorFileV3 := filepath.Join(destination, constant.UPDATE_DESCRIPTOR_V3_FILE)
	logger.Debug(fmt.Sprintf("updateDescriptorFileV3: %v", updateDescriptorFileV3))

	// Save update descriptors
	absDestinationV2 := saveUpdateDescriptorInDestination(updateDescriptorFileV2, dataStringV2, destination)
	util.PrintInfo(fmt.Sprintf("'%s' has been successfully created at '%s'.", constant.UPDATE_DESCRIPTOR_V2_FILE,
		absDestinationV2))
	absDestinationV3 := saveUpdateDescriptorInDestination(updateDescriptorFileV3, dataStringV3, destination)
	util.PrintInfo(fmt.Sprintf("'%s' has been successfully created at '%s'.", constant.UPDATE_DESCRIPTOR_V3_FILE,
		absDestinationV3))

	//Print whats next
	color.Set(color.Bold)
	fmt.Println("\nWhat's next?")
	color.Unset()
	fmt.Println(fmt.Sprintf("\trun 'wum-uc init --sample' to view a sample '%s' file.",
		constant.UPDATE_DESCRIPTOR_V2_FILE))
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

//This function will set values to the update-descriptor.yaml and update-descriptorV2.yaml.
func setValuesForUpdateDescriptors(updateDescriptorV2 *util.UpdateDescriptorV2, updateDescriptorV3 *util.UpdateDescriptorV3) {
	logger.Debug("Setting values for update descriptors:")
	setCommonValuesForBothUpdateDescriptors(updateDescriptorV2, updateDescriptorV3)
	setDescription(updateDescriptorV2)
	setAppliesTo(updateDescriptorV2)
	setBugFixes(updateDescriptorV2)
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

func setCommonValuesForBothUpdateDescriptors(updateDescriptorV2 *util.UpdateDescriptorV2, updateDescriptorV3 *util.UpdateDescriptorV3) {
	util.PrintInBold("Enter update number: ")
	updateNumber, err := util.GetUserInput()
	util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
	updateDescriptorV2.Update_number = updateNumber
	updateDescriptorV3.Update_number = updateNumber

	util.PrintInBold("Enter platform name: ")
	platformName, err := util.GetUserInput()
	util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
	updateDescriptorV2.Platform_name = platformName
	updateDescriptorV3.Platform_name = platformName

	util.PrintInBold("Enter platform version: ")
	platformVersion, err := util.GetUserInput()
	util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
	updateDescriptorV2.Platform_version = platformVersion
	updateDescriptorV3.Platform_version = platformVersion
}

//This function will process the readme file and extract details to populate update-descriptor.
// yaml and update-descriptorV2.yaml. If some data
// cannot be extracted, it will add default value and continue.
func processReadMe(directory string, updateDescriptorV2 *util.UpdateDescriptorV2,
	updateDescriptorV3 *util.UpdateDescriptorV3) {
	logger.Debug("Processing README started")
	// Construct the README.txt path
	readMePath := path.Join(directory, constant.README_FILE)
	logger.Debug(fmt.Sprintf("README Path: %v", readMePath))
	// Check whether the README.txt file exists
	_, err := os.Stat(readMePath)
	if err != nil {
		// If the file does not exist or any other error occur, return without printing warning messages
		logger.Debug(fmt.Sprintf("%s not found", readMePath))
		setValuesForUpdateDescriptors(updateDescriptorV2, updateDescriptorV3)
		return
	}
	// Read the README.txt file
	data, err := ioutil.ReadFile(readMePath)
	if err != nil {
		// If any error occurs, return without printing warning messages
		logger.Debug(fmt.Sprintf("Error occurred in processing README: %v", err))
		setValuesForUpdateDescriptors(updateDescriptorV2, updateDescriptorV3)
		return
	}

	logger.Debug("README.txt found")

	// Convert the byte array to a string
	stringData := string(data)
	// Compile the regex
	regex, err := regexp.Compile(constant.PATCH_ID_REGEX)
	if err == nil {
		result := regex.FindStringSubmatch(stringData)
		logger.Trace(fmt.Sprintf("PATCH_ID_REGEX result: %v", result))
		// Since the regex has 2 capturing groups, the result size will be 3 (because there is the full match)
		// If not match found, the size will be 0. We check whether the result size is not 0 to make sure both
		// capturing groups are identified.
		if len(result) != 0 {
			// Extract details
			updateDescriptorV2.Update_number = result[2]
			updateDescriptorV3.Update_number = result[2]
			updateDescriptorV2.Platform_version = result[1]
			updateDescriptorV3.Platform_version = result[1]
			platformsMap := viper.GetStringMapString(constant.PLATFORM_VERSIONS)
			logger.Trace(fmt.Sprintf("Platform Map: %v", platformsMap))
			// Get the platform details from the map
			platformName, found := platformsMap[result[1]]
			if found {
				logger.Debug("PlatformName found in configs")
				updateDescriptorV2.Platform_name = platformName
				updateDescriptorV3.Platform_name = platformName
			} else {
				//If the platform name is not found, set default
				logger.Debug("No matching platform name found for:", result[1])
				util.PrintInBold("Enter platform name for platform version :", result[1])
				platformName, err := util.GetUserInput()
				util.HandleErrorAndExit(err, "Error occurred while getting input from the user.")
				updateDescriptorV2.Platform_name = platformName
				updateDescriptorV3.Platform_name = platformName
			}
		} else {
			logger.Debug("PATCH_ID_REGEX results incorrect:", result)
		}
	} else {
		//If error occurred, set default values
		logger.Debug(fmt.Sprintf("Error occurred while processing PATCH_ID_REGEX: %v", err))
		setCommonValuesForBothUpdateDescriptors(updateDescriptorV2, updateDescriptorV3)
	}

	// Compile the regex
	regex, err = regexp.Compile(constant.APPLIES_TO_REGEX)
	if err == nil {
		result := regex.FindStringSubmatch(stringData)
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
		allResult := regex.FindAllStringSubmatch(stringData, -1)
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
		result := regex.FindStringSubmatch(stringData)
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
