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
	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"github.com/wso2/update-creator-tool/constant"
	"github.com/wso2/update-creator-tool/util"
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
	//Print whats next
	color.Set(color.Bold)
	fmt.Println("\nWhat's next?")
	color.Unset()
	fmt.Println(fmt.Sprintf("\trun 'wum-uc init --sample' to view samples of '%s' and '%s' files.",
		constant.UPDATE_DESCRIPTOR_V2_FILE, constant.UPDATE_DESCRIPTOR_V3_FILE))
}

/*// yaml and update-descriptorV2.yaml. If some data
// cannot be extracted, it will add default value and continue.
func processReadMe2(directory string, updateDescriptorV2 *util.UpdateDescriptorV2,
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
}*/
