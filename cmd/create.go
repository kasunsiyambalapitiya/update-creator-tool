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
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mholt/archiver"
	"github.com/olekukonko/tablewriter"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wso2/update-creator-tool/constant"
	"github.com/wso2/update-creator-tool/util"
	"gopkg.in/yaml.v2"
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
		util.HandleErrorAndExit(errors.New("Invalid number of argumants. Run 'wum-uc create --help' to " +
			"view help."))
	}
	createUpdate(args[0], args[1])
}

// This function will start the update creation process.
func createUpdate(updateDirectoryPath, distributionPath string) {

	// set debug level
	setLogLevel()
	logger.Debug("[create] command called")

	// Flow - First check whether the given locations exist and required files exist. Then start processing.
	// If one step fails, print error message and exit.

	//1) Check whether the given update directory exists
	exists, err := util.IsDirectoryExists(updateDirectoryPath)
	util.HandleErrorAndExit(err, "Error occurred while reading the update directory")
	logger.Debug(fmt.Sprintf("exists: %v", exists))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("Directory does not exist at '%s'. Update location "+
			"must be a directory.", updateDirectoryPath)))
	}
	updateRoot := strings.TrimSuffix(updateDirectoryPath, constant.PATH_SEPARATOR)
	logger.Debug(fmt.Sprintf("updateRoot: %s\n", updateRoot))
	viper.Set(constant.UPDATE_ROOT, updateRoot)

	//2) Check whether the update-descriptor.yaml file exists
	// Construct the update-descriptor.yaml file location
	updateDescriptorPath := path.Join(updateDirectoryPath, constant.UPDATE_DESCRIPTOR_FILE)
	exists, err = util.IsFileExists(updateDescriptorPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while reading the '%v'",
		constant.UPDATE_DESCRIPTOR_FILE))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("'%s' not found at '%s' directory.",
			constant.UPDATE_DESCRIPTOR_FILE, updateDirectoryPath)))
	}
	logger.Debug(fmt.Sprintf("Descriptor Exists. Location %s", updateDescriptorPath))

	//3) Check whether the given distribution exists
	exists, err = util.IsFileExists(distributionPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while checking '%s'", distributionPath))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("File does not exist at '%s'. Distribution must "+
			"be a zip file.", distributionPath)))
	}
	// Checks whether the given distribution is a zip file
	util.IsZipFile(constant.DISTRIBUTION, distributionPath)

	//4) Read update-descriptor.yaml and set the update name which will be used when creating the update zip file.
	updateDescriptor, err := util.LoadUpdateDescriptor(constant.UPDATE_DESCRIPTOR_FILE, updateDirectoryPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred when reading '%s' file.",
		constant.UPDATE_DESCRIPTOR_FILE))

	//5) Validate the file format
	err = util.ValidateUpdateDescriptor(updateDescriptor)
	util.HandleErrorAndExit(err, fmt.Sprintf("'%s' format is incorrect.", constant.UPDATE_DESCRIPTOR_FILE))

	// set the update name
	updateName := getUpdateName(updateDescriptor, constant.UPDATE_NAME_PREFIX)
	viper.Set(constant.UPDATE_NAME, updateName)

	// Get ignored files. These files wont be stored in the data structure. So matches will not be searched for
	// these files
	ignoredFiles := getIgnoredFilesInUpdate()
	logger.Debug(fmt.Sprintf("Ignored files: %v", ignoredFiles))

	//6) Traverse and read the update

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
	//7) Find matches

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
			err := handleNoMatch(directoryName, true, allFilesMap, &rootNode, updateDescriptor)
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
			err := handleSingleMatch(directoryName, match, true, allFilesMap, &rootNode, updateDescriptor)
			util.HandleErrorAndExit(err)
		// Multiple matches found in the distribution for the given directory
		default:
			// Handle the multiple matches situation
			logger.Debug("\nMultiple matches found\n")
			err := handleMultipleMatches(directoryName, true, matches, allFilesMap, &rootNode,
				updateDescriptor)
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
			err := handleNoMatch(fileName, false, allFilesMap, &rootNode, updateDescriptor)
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
			err := handleSingleMatch(fileName, match, false, allFilesMap, &rootNode, updateDescriptor)
			util.HandleErrorAndExit(err)
		// Multiple matches found in the distribution for the given file
		default:
			// Handle the multiple matches situation
			logger.Debug("Multiple matches found\n")
			err := handleMultipleMatches(fileName, false, matches, allFilesMap, &rootNode, updateDescriptor)
			util.HandleErrorAndExit(err)
		}
	}

	//8) Copy resource files (update-descriptor.yaml, etc) to temp directory
	resourceFiles := getResourceFiles()
	err = copyResourceFilesToTempDir(resourceFiles)
	util.HandleErrorAndExit(err, errors.New("Error occurred while copying resource files."))

	// Save the update-descriptor with the updated, newly added files to the temp directory
	data, err := marshalUpdateDescriptor(updateDescriptor)
	util.HandleErrorAndExit(err, "Error occurred while marshalling the update-descriptor.")
	err = saveUpdateDescriptor(constant.UPDATE_DESCRIPTOR_FILE, data)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while saving the '%v'.",
		constant.UPDATE_DESCRIPTOR_FILE))

	// Construct the update zip name
	updateZipName := updateName + ".zip"
	logger.Debug(fmt.Sprintf("updateZipName: %s", updateZipName))

	targetDirectory := path.Join(constant.TEMP_DIR, updateName)
	targetDirectory = strings.Replace(targetDirectory, "/", constant.PATH_SEPARATOR, -1)

	logger.Debug(fmt.Sprintf("targetDirectory: %s", targetDirectory))
	err = archiver.Zip.Make(updateZipName, []string{targetDirectory})
	util.HandleErrorAndExit(err)

	// Remove the temp directories
	util.CleanUpDirectory(constant.TEMP_DIR)

	signal.Stop(cleanupChannel)

	util.PrintInfo(fmt.Sprintf("'%s' successfully created.", updateZipName))
	util.PrintInfo(fmt.Sprintf("Validating '%s'\n", updateZipName))

	// Start the update file validation
	startValidation(updateZipName, distributionPath)
}

// This function will set the update name which will be used when creating the update zip.
func getUpdateName(updateDescriptor *util.UpdateDescriptor, updateNamePrefix string) string {
	// Read the corresponding details from the struct
	platformVersion := updateDescriptor.Platform_version
	updateNumber := updateDescriptor.Update_number
	updateName := updateNamePrefix + "-" + platformVersion + "-" + updateNumber
	return updateName
}

// This function will handle no match found for a file situations. User input is required and based on the user input,
// this function will decide how to proceed.
func handleNoMatch(filename string, isDir bool, allFilesMap map[string]data, rootNode *node,
	updateDescriptor *util.UpdateDescriptor) error {
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
	updateDescriptor *util.UpdateDescriptor) error {
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
	updateDescriptor *util.UpdateDescriptor) error {
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
	rootNode *node, updateDescriptor *util.UpdateDescriptor) error {

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

		var relativePath string
		if strings.Contains(file.Name, "/") {
			relativePath = strings.SplitN(file.Name, "/", 2)[1]
		} else {
			relativePath = file.Name
		}

		// Replace all \ with /. Otherwise it will cause issues in Windows OS.
		relativePath = filepath.ToSlash(relativePath)
		logger.Trace(fmt.Sprintf("relativePath: %s", relativePath))

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
func marshalUpdateDescriptor(updateDescriptor *util.UpdateDescriptor) ([]byte, error) {
	data, err := yaml.Marshal(&updateDescriptor)
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
	updateDescriptor *util.UpdateDescriptor) error {
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
