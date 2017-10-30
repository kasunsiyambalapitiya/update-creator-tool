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
	"archive/zip"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/mholt/archiver"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wso2/update-creator-tool/constant"
	"github.com/wso2/update-creator-tool/util"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// This struct used to store directory structure of the distribution.
type node struct {
	name         string
	isDir        bool
	relativePath string
	parent       *node
	childNodes   map[string]*node
	md5Hash      string
}

// Values used to print help command.
var (
	generateCmdUse       = "generate <newly_released_feature>"
	generateCmdShortDesc = "Generate updates zips for all the products that goes along with the given feature "
	generateCmdLongDesc  = dedent.Dedent(`
	This command will generate new update zips for all the products that goes along with the given feature
	between the updated distribution and the
	previous released distribution. It is required to run wum-uc init first and pass update directory location
	provided for init as the third input.
	<update_dist_loc>	path to the updated distribution
	<prev_dist_loc>		path to the previous distribution
	<update_dir>		path to the update directory where init was ran
	`)
)

// GenerateCmd represents the generate command.
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
}

// This function will be called when the generate command is called.
func initializeGenerateCommand(cmd *cobra.Command, args []string) {
	if len(args) != 1 {
		util.HandleErrorAndExit(errors.New("invalid number of arguments. Run 'wum-uc generate --help' to " +
			"view help"))
	}
	generateUpdate(args[0])
}

// This function generates update zips for the products released with the give feature.
func generateUpdate(releasedFeature string) {
	// Set log level
	setLogLevel()
	logger.Debug("[generate] command called")

	// Filter out the feature name and the version
	featureName, _ := getFeatureNameAndVersion(releasedFeature)
	// Get the map of products being affected by the released feature from API call
	getAffectedProducts(featureName)
	// Get the locations of previous and updated distributions from API call

	// Generate the update for all the products

	// Identify changed files

	// Extract only the relevant files from updated distributions

	// Filter out only the lasted gz file (Next at the server, as it is having all the detail instructions)

	// Create a detailed update-descriptor for server

	// Create a mini update-descriptor for update

	// Check whether the given update directory exists
	exists, err := util.IsDirectoryExists(updateDirectoryPath)
	util.HandleErrorAndExit(err, "Error occurred while reading the update directory")
	logger.Debug(fmt.Sprintf("Exists: %v", exists))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("directory does not exist at '%s'. Update location "+
			"must be a directory.", updateDirectoryPath)))
	}
	updateRoot := strings.TrimSuffix(updateDirectoryPath, constant.PATH_SEPARATOR)
	logger.Debug(fmt.Sprintf("UpdateRoot: %s\n", updateRoot))
	viper.Set(constant.UPDATE_ROOT, updateRoot)

	// Check whether the update-descriptor.yaml file exists in the update directory
	checkFileExists(updateDirectoryPath, constant.UPDATE_DESCRIPTOR_FILE)

	// Check whether the LICENSE.txt file file exists in the update directory
	checkFileExists(updateDirectoryPath, constant.LICENSE_FILE)

	// Check whether the given distributions exists
	checkDistributionExists(updatedDistPath, "updated")
	checkDistributionExists(previousDistPath, "previous")

	// Check whether the given distributions are zip files
	util.IsZipFile("updated distribution", updatedDistPath)
	logger.Debug(fmt.Sprintf("Provided updated distribution is a zip file"))
	util.IsZipFile("previous distribution", previousDistPath)
	logger.Debug(fmt.Sprintf("Provided previous distribution is a zip file"))

	// Read update-descriptor.yaml and parse it to UpdateDescriptor struct
	updateDescriptor, err := util.LoadUpdateDescriptor(constant.UPDATE_DESCRIPTOR_FILE, updateDirectoryPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred when reading '%s' file.",
		constant.UPDATE_DESCRIPTOR_FILE))

	// Validate the file format of the update-descriptor.yaml
	err = util.ValidateUpdateDescriptor(updateDescriptor)
	util.HandleErrorAndExit(err, fmt.Sprintf("'%s' format is incorrect.", constant.UPDATE_DESCRIPTOR_FILE))

	// Set the update name which will be used when creating the update zip file
	updateName := getUpdateName(updateDescriptor, constant.UPDATE_NAME_PREFIX)
	viper.Set(constant.UPDATE_NAME, updateName)

	// Identify modified, added and removed files by comparing the diff between two given distributions
	// Get the distribution name
	distributionName := getDistributionName(updatedDistPath)
	// Read the updated distribution zip file
	logger.Info(fmt.Sprintf("Reading the updated %s. Please wait...", distributionName))

	// Get zipReaders of both distributions
	updatedDistributionReader := getZipReader(updatedDistPath)
	logger.Debug(fmt.Sprintf("Zip reader used for reading updated %s created successfully", distributionName))
	previousDistributionReader := getZipReader(previousDistPath)
	logger.Debug(fmt.Sprintf("Zip reader used for reading previous released %s created successfully", distributionName))

	defer updatedDistributionReader.Close()
	defer previousDistributionReader.Close()

	// RootNode is what we use as the root of the updated distribution when we populate tree like structure
	rootNodeOfUpdatedDistribution := createNewNode()
	rootNodeOfUpdatedDistribution, err = readZip(updatedDistributionReader, rootNodeOfUpdatedDistribution)
	util.HandleErrorAndExit(err)
	logger.Debug(fmt.Sprintf("Node tree for updated %s created successfully", distributionName))
	logger.Debug(fmt.Sprintf("Reading updated %s completed successfully", distributionName))
	logger.Info(fmt.Sprintf("Reading previously released %s. Please wait...", distributionName))

	// Maps for storing modified, changed, removed files and removed directories from the update
	modifiedFiles := make(map[string]struct{})
	removedFiles := make(map[string]struct{})
	addedFiles := make(map[string]struct{})
	removedDirectories := make(map[string]struct{})

	// Iterate through each file to identify modified, removed files and removed directories from the update
	logger.Debug(fmt.Sprintf("Finding modified, removed files and removed directories between updated and "+
		"previously released %s", distributionName))
	for _, file := range previousDistributionReader.Reader.File {
		// Open the file for calculating MD5
		zippedFile, err := file.Open()
		if err != nil {
			util.HandleErrorAndExit(err)
		}
		data, err := ioutil.ReadAll(zippedFile)
		if err != nil {
			util.HandleErrorAndExit(err)
		}
		// Don't use defer here as too many open files will cause a panic
		zippedFile.Close()
		// Calculate the md5 of the file
		hash := md5.New()
		hash.Write(data)
		md5Hash := hex.EncodeToString(hash.Sum(nil))

		// Name of the file
		fileName := file.Name
		logger.Trace(fmt.Sprintf("file.Name: %s and md5: %s", fileName, md5Hash))

		if strings.HasSuffix(fileName, "/") {
			fileName = strings.TrimSuffix(fileName, "/")
		}

		// Get the relative location of the file
		relativePath := util.GetRelativePath(file)

		fileNameStrings := strings.Split(fileName, "/")
		fileName = fileNameStrings[len(fileNameStrings)-1]

		if relativePath != "" {
			if file.FileInfo().IsDir() {
				// Finding removed directories
				findRemovedDirectories(rootNodeOfUpdatedDistribution, fileName, relativePath, removedDirectories)
			} else {
				// Finding modified files
				findModifiedFiles(rootNodeOfUpdatedDistribution, fileName, md5Hash, relativePath, modifiedFiles)
				// Finding removed files
				findRemovedFiles(rootNodeOfUpdatedDistribution, fileName, relativePath, removedDirectories, removedFiles)
			}
		}
	}
	logger.Debug(fmt.Sprintf("Finding modified, removed files and removed directories between updated and previuosly"+
		" released %s completed successfully", distributionName))

	// Identifying newly added files from update
	// Reading previous distribution zip file
	logger.Info(fmt.Sprintf("Reading the previous %s. Please wait...", distributionName))
	// RootNode is what we use as the root of the previous distribution when we populate tree like structure
	rootNodeOfPreviousDistribution := createNewNode()
	rootNodeOfPreviousDistribution, err = readZip(previousDistributionReader, rootNodeOfPreviousDistribution)
	util.HandleErrorAndExit(err)
	logger.Debug(fmt.Sprintf("Node tree for previous released %s created successfully", distributionName))
	logger.Debug(fmt.Sprintf("Reading previous released %s completed successfully", distributionName))
	logger.Info(fmt.Sprintf("Reading updated %s. Please wait...", distributionName))

	// Iterating through updated pack to identify the newly added files
	logger.Debug(fmt.Sprintf("Finding newly added files between updated and previous released %s", distributionName))
	for _, file := range updatedDistributionReader.Reader.File {
		// MD5 of the file is not calculated as we are filtering only for added files
		// Name of the file
		fileName := file.Name
		logger.Trace(fmt.Sprintf("File Name: %s", fileName))

		if strings.HasSuffix(fileName, "/") {
			fileName = strings.TrimSuffix(fileName, "/")
		}
		// Get the relative location of the file
		relativePath := util.GetRelativePath(file)

		fileNameStrings := strings.Split(fileName, "/")
		fileName = fileNameStrings[len(fileNameStrings)-1]
		if relativePath != "" && !file.FileInfo().IsDir() {
			// Finding newly added files
			findNewlyAddedFiles(rootNodeOfPreviousDistribution, fileName, relativePath, addedFiles)
		}
		//zipReader.Close() // if this is causing panic we need to close it here
	}
	logger.Debug(fmt.Sprintf("Finding newly added files between the given two %s distributions completed "+
		"successfully", distributionName))

	logger.Info("Modified Files : ", modifiedFiles)
	logger.Debug("Number of modified files : ", len(modifiedFiles))
	logger.Info("Removed Files : ", removedFiles)
	logger.Debug("Number of removed files : ", len(removedFiles))
	logger.Info("Added Files : ", addedFiles)
	logger.Debug("Number of added files : ", len(addedFiles))

	// Update added,removed and modified files in the updateDescriptor struct
	modifyUpdateDescriptor(modifiedFiles, removedFiles, addedFiles, removedDirectories, updateDescriptor)

	// Copy resource files in the update location to a temp directory
	copyResourceFilesToTemp()

	// Save the modified updateDescriptor to the the update-descriptor.yaml
	data, err := marshalUpdateDescriptor(updateDescriptor)
	util.HandleErrorAndExit(err, "Error occurred while marshalling the update-descriptor.")
	err = saveUpdateDescriptor(constant.UPDATE_DESCRIPTOR_FILE, data)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while saving the (%v).",
		constant.UPDATE_DESCRIPTOR_FILE))
	logger.Debug(fmt.Sprintf("update-descriptor.yaml updated successfully"))

	// Extract newly added and modified files from the updated zip and copy them to the temp directory for
	// creating the update zip.
	logger.Debug(fmt.Sprintf("Extracting newly added and modified files from the updated zip"))
	for _, file := range updatedDistributionReader.Reader.File {
		relativePath := util.GetRelativePath(file)

		// Extracting newly added files from the updated distribution
		_, found := addedFiles[relativePath]
		if found {
			logger.Debug(fmt.Sprintf("Copying newly added file %s to temp location", relativePath))
			copyFileToTempDir(file, relativePath)
		}
		// Extracting modified files from the updated distribution
		_, found = modifiedFiles[relativePath]
		if found {
			logger.Debug(fmt.Sprintf("Copying modified file %s to temp location", relativePath))
			copyFileToTempDir(file, relativePath)
		}
	}
	// Closing distribution readers
	previousDistributionReader.Close()
	updatedDistributionReader.Close()

	logger.Debug(fmt.Sprintf("Copying newly added and modified files from updated distribution to temp location " +
		"completed successfully"))

	// Create the update zip
	logger.Debug(fmt.Sprintf("Creating the update zip"))
	resourcesDirectory := path.Join(constant.TEMP_DIR, updateName)
	// Make resourcesDirectory path compatible with windows OS
	resourcesDirectory = strings.Replace(resourcesDirectory, "/", constant.PATH_SEPARATOR, -2)
	updateZipName := updateName + ".zip"
	err = archiver.Zip.Make(path.Join(updateRoot, updateZipName), []string{resourcesDirectory})
	util.HandleErrorAndExit(err)
	logger.Debug(fmt.Sprintf("Creating the update zip completed successfully"))

	// Delete the temp directory
	util.CleanUpDirectory(path.Join(constant.TEMP_DIR))
	logger.Info(fmt.Sprintf("Update for %s created successfully", distributionName))
}

// This function returns the feature name and the version
func getFeatureNameAndVersion(releasedFeature string) (featureName, featureVersion string) {
	// Check whether the releasedFeature
	releasedFeatureStrings := strings.Split(releasedFeature, ".feature-")
	featureName = releasedFeatureStrings[0]
	featureVersion = releasedFeatureStrings[1]
	return
}

// This function returns the products released with the given feature
func getAffectedProducts(featureName string) map[string]string {
	affectedProducts := make(map[string]string)
	apiUrl := util.GetAffectedProductsAPIURL()

}

// This function checks for the availability of the given file in the given update directory location.
func checkFileExists(updateDirectoryPath, fileName string) {
	// Construct the relevant file location
	updateDescriptorPath := path.Join(updateDirectoryPath, fileName)
	exists, err := util.IsFileExists(updateDescriptorPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while reading the '%v'",
		fileName))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("'%s' not found at '%s' directory.",
			fileName, updateDirectoryPath)))
	}
	logger.Debug(fmt.Sprintf("%s exists in given update directory location", fileName))
}

// This function checks whether the given distribution exists.
func checkDistributionExists(distributionPath, distributionState string) {
	exists, err := util.IsFileExists(distributionPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while reading '%s' distribution at '%s' ",
		distributionState, distributionPath))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("file does not exist at '%s'. '%s' distribution must "+
			"be a zip file.", distributionPath, distributionState)))
	}
	logger.Debug(fmt.Sprintf("The %s distribution exists in %s location", distributionState, distributionPath))
}

// This function returns the update name which will be used when creating the update zip.
func getUpdateName(updateDescriptor *util.UpdateDescriptor, updateNamePrefix string) string {
	// Read the corresponding details from the struct
	platformVersion := updateDescriptor.Platform_version
	updateNumber := updateDescriptor.Update_number
	updateName := updateNamePrefix + "-" + platformVersion + "-" + updateNumber
	return updateName
}

// This function marshals the update-descriptor.yaml file.
func marshalUpdateDescriptor(updateDescriptor *util.UpdateDescriptor) ([]byte, error) {
	data, err := yaml.Marshal(&updateDescriptor)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// This function returns a zip.ReadCloser for the given distribution.
func getZipReader(distributionPath string) *zip.ReadCloser {
	zipReader, err := zip.OpenReader(distributionPath)
	if err != nil {
		util.HandleErrorAndExit(err)
	}
	return zipReader
}

// This creates and returns a new node which has initialized its childNodes map.
func createNewNode() *node {
	return &node{
		childNodes: make(map[string]*node),
	}
}

// This function reads the zip file in the given location and returns the root node of the formed tree.
func readZip(zipReader *zip.ReadCloser, rootNode *node) (*node, error) {
	// Iterate through each file in the zip file
	for _, file := range zipReader.Reader.File {
		zippedFile, err := file.Open()
		if err != nil {
			return rootNode, err
		}
		data, err := ioutil.ReadAll(zippedFile)
		if err != nil {
			util.HandleErrorAndExit(err)
		}
		// Close zippedFile after reading its data to avoid too many open files leading to a panic
		zippedFile.Close()

		// Calculate the md5 of the file
		hash := md5.New()
		hash.Write(data)
		md5Hash := hex.EncodeToString(hash.Sum(nil))

		// Get the relative path of the file
		logger.Trace(fmt.Sprintf("file.Name: %s", file.Name))

		relativePath := util.GetRelativePath(file)

		// Add the file to root node
		addToRootNode(rootNode, strings.Split(relativePath, "/"), file.FileInfo().IsDir(), md5Hash)
	}
	return rootNode, nil
}

// This function adds a new node to given root node.
func addToRootNode(root *node, path []string, isDir bool, md5Hash string) {
	logger.Trace("Checking: %s : %s", path[0], path)

	// If the current path element is the last element, add it as a new node.
	if len(path) == 1 {
		logger.Trace("End reached")
		newNode := createNewNode()
		newNode.name = path[0]
		newNode.isDir = isDir
		newNode.md5Hash = md5Hash
		if len(root.relativePath) == 0 {
			newNode.relativePath = path[0]
		} else {
			newNode.relativePath = root.relativePath + "/" + path[0]
		}
		newNode.parent = root
		root.childNodes[path[0]] = newNode
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
			if len(root.relativePath) == 0 {
				newNode.relativePath = path[0]
			} else {
				newNode.relativePath = root.relativePath + "/" + path[0]
			}
			newNode.parent = root
			root.childNodes[path[0]] = newNode
			node = newNode
		}
		// Recursively call the function for the rest of the path elements
		addToRootNode(node, path[1:], isDir, md5Hash)
	}
}

// This function returns the distribution name of the given zip file and sets it as viper config.
func getDistributionName(distributionPath string) string {
	paths := strings.Split(distributionPath, constant.PATH_SEPARATOR)
	distributionName := strings.TrimSuffix(paths[len(paths)-1], ".zip")
	viper.Set(constant.PRODUCT_NAME, distributionName)
	logger.Debug(fmt.Sprintf("Distribution name set in to the viper config successfully"))
	return distributionName
}

// This function identifies modified files between given two distributions.
func findModifiedFiles(root *node, fileName string, md5Hash string, relativePath string,
	modifiedFiles map[string]struct{}) {
	logger.Trace(fmt.Sprintf("Checking %s file for modifications in %s relative path", fileName,
		relativePath))
	// Check whether the given file exists in the given relative path in any child node
	found, node := pathExists(root, relativePath, false)
	if found && node.md5Hash != md5Hash {
		logger.Trace(fmt.Sprintf("The file %s exists in the both distributions with mismatched md5, so the file is "+
			"being modified", fileName))

		modifiedFiles[node.relativePath] = struct{}{}
		logger.Trace(fmt.Sprintf("Modified file %s added to the modifiedFiles map successfully", fileName))
	}
	logger.Trace(fmt.Sprintf("Checking %s file exists in %s relative path for modifications completed successfuly",
		fileName, relativePath))
}

// This function identifies removed directory paths between given two distributions.
func findRemovedDirectories(root *node, fileName string, relativePath string, removedDirectoryPaths map[string]struct{}) {
	logger.Trace(fmt.Sprintf("Checking the existance of %s directory in %s relative path", fileName, relativePath))
	// Check whether the given directory exists in the given relative path in any child node
	found, _ := pathExists(root, relativePath, true)

	if !found {
		logger.Trace(fmt.Sprintf("The %s directory not found in the given %s relative path", fileName, relativePath))
		parentDirExits := false
		// Check whether its parent directory has already been added for removal
		if len(removedDirectoryPaths) != 0 {
			for parentDirectory, _ := range removedDirectoryPaths {
				if strings.HasPrefix(relativePath, parentDirectory) {
					parentDirExits = true
					logger.Trace(fmt.Sprintf("The parent directory of %s directory has already been added for "+
						"removal", relativePath))
				}
			}
			// Add the directory to removedDirectoryPaths map if its parent directory has not been listed for removal
			if !parentDirExits {
				logger.Trace(fmt.Sprintf("The parent directory of %s directory has not been added for removal",
					relativePath))
				removedDirectoryPaths[relativePath] = struct{}{}
				logger.Trace(fmt.Sprintf("Removed %s directory added to the removedDirectoryPaths map successfully",
					relativePath))
			}
		} else {
			logger.Trace(fmt.Sprintf("The %s directory not found in the given %s relative path, its been removed "+
				"from the update", fileName, relativePath))
			removedDirectoryPaths[relativePath] = struct{}{}
			logger.Trace(fmt.Sprintf("Removed %s directory added to the removedDirectoryPaths map successfully",
				relativePath))
		}
	} else {
		logger.Trace(fmt.Sprintf("The %s directory found in the given relative path %s, it is not a removed "+
			"directory", fileName, relativePath))
	}
}

// This function identifies removed files between given two distributions in which their parent directories are not
// listed for removal.
func findRemovedFiles(root *node, fileName string, relativePath string, removedDirectoryPaths map[string]struct{}, removedFiles map[string]struct{}) {
	logger.Trace(fmt.Sprintf("Checking %s file in %s relative path to identify it as a removed file",
		fileName, relativePath))
	// Check whether the given file exists in the given relative path in any child node
	found, _ := pathExists(root, relativePath, false)

	if !found {
		logger.Trace(fmt.Sprintf("The %s file not found in the given %s relative path", fileName, relativePath))
		parentDirExits := false
		// Check whether its parent directory has already been added for removal
		if len(removedDirectoryPaths) != 0 {
			for parentDirectory, _ := range removedDirectoryPaths {
				if strings.HasPrefix(relativePath, parentDirectory) {
					parentDirExits = true
					logger.Trace(fmt.Sprintf("The parent directory of %s file has already been added for removal",
						relativePath))
				}
			}
		}
		// Add the file to removedFiles map if its parent directory has not been listed for removal
		if !parentDirExits {
			logger.Trace(fmt.Sprintf("The parent directory of %s has not been added for removal", relativePath))
			removedFiles[relativePath] = struct{}{}
			logger.Trace(fmt.Sprintf("Removed %s file added to the removedFiles map successfully", relativePath))
		}
	} else {
		logger.Trace(fmt.Sprintf("The %s file found in the given relative path %s, it is not a removed file",
			fileName, relativePath))
	}
}

// This function identifies newly added files between given two distributions.
func findNewlyAddedFiles(root *node, fileName string, relativePath string, addedFiles map[string]struct{}) {
	logger.Trace(fmt.Sprintf("Checking %s file to identify it as a newly added in %s relative path",
		fileName, relativePath))
	// Check whether the given file exists in the given relative path in any child node
	found, _ := pathExists(root, relativePath, false)

	if !found {
		logger.Trace(fmt.Sprintf("The %s file not found in the given relative path %s, so it is a newly added file",
			fileName, relativePath))
		addedFiles[relativePath] = struct{}{}
		logger.Trace(fmt.Sprintf("Newly added %s file added to the addedFiles map successfully", relativePath))
	} else {
		logger.Trace(fmt.Sprintf("The %s file found in the given relative path %s, it is not a newly added file",
			fileName, relativePath))
	}
}

// This function is a helper function which calls nodeExists() and checks whether a node exists in the given path and
// the type(file/dir) is correct.
func pathExists(rootNode *node, relativePath string, isDir bool) (bool, *node) {
	return nodeExists(rootNode, strings.Split(relativePath, "/"), isDir)
}

// This function checks whether a node exists in the given path and the type(file/dir) is correct.
func nodeExists(rootNode *node, path []string, isDir bool) (bool, *node) {
	logger.Trace(fmt.Sprintf("All: %v", rootNode.childNodes))
	logger.Trace(fmt.Sprintf("Checking: %s", path[0]))
	childNode, found := rootNode.childNodes[path[0]]
	// If the path element is found, that means it is in the tree
	if found {
		// If there are more path elements than 1, continue recursively. Otherwise check whether it has the
		// provided type(file/dir) and return
		logger.Trace(fmt.Sprintf("%s found", path[0]))
		if len(path) > 1 {
			return nodeExists(childNode, path[1:], isDir)
		} else {
			return childNode.isDir == isDir, childNode
		}
	}
	// If the path element is not found, return false and nil for node
	logger.Trace(fmt.Sprintf("%s NOT found", path[0]))
	return false, nil
}

// This function modifies the updateDescriptor with the added, removed, modified files and removed directories.
func modifyUpdateDescriptor(modifiedFiles, removedFiles, addedFiles, removedDirectories map[string]struct{},
	updateDescriptor *util.UpdateDescriptor) {
	logger.Debug(fmt.Sprintf("Modifying UpdateDescriptor"))

	// Appending modified files
	logger.Debug(fmt.Sprintf("Appending modified files to the UpdateDescriptor"))
	for modifiedFile, _ := range modifiedFiles {
		updateDescriptor.File_changes.Modified_files = append(updateDescriptor.File_changes.Modified_files,
			modifiedFile)
	}
	logger.Debug(fmt.Sprintf("Appending modified files to the UpdateDescriptor finished successfully"))

	// Appending removed files and directories
	logger.Debug(fmt.Sprintf("Appending removed files and directories to the UpdateDescriptor"))
	// Appending removed directories
	for removedDirectory, _ := range removedDirectories {
		updateDescriptor.File_changes.Removed_files = append(updateDescriptor.File_changes.Removed_files,
			removedDirectory)
	}
	// Appending removed files
	for removedFile, _ := range removedFiles {
		updateDescriptor.File_changes.Removed_files = append(updateDescriptor.File_changes.Removed_files,
			removedFile)
	}
	logger.Debug(fmt.Sprintf("Appending removed files and directories to the UpdateDescriptor finished successfully"))

	// Appending newly added files
	logger.Debug(fmt.Sprintf("Appending newly added files to the UpdateDescriptor"))
	for addedFile, _ := range addedFiles {
		updateDescriptor.File_changes.Added_files = append(updateDescriptor.File_changes.Added_files, addedFile)
	}
	logger.Debug(fmt.Sprintf("Appending newly added files to the UpdateDescriptor finished successfully"))
	logger.Debug(fmt.Sprintf("Modifying UpdateDescriptor finished successfully"))
}

// This function gets the resource files that exists in given update location and copies them to a temp location.
func copyResourceFilesToTemp() {
	logger.Debug(fmt.Sprintf("Copying mandatory files of an update to temp location"))
	// Obtain map of files to be copied to the temp directory with file name as the key and boolean specifying
	// mandatory or optional as the value
	resourceFiles := getResourceFiles()
	err := copyResourceFilesToTempDir(resourceFiles)
	util.HandleErrorAndExit(err, errors.New("error occurred while copying resource files"))
	logger.Debug(fmt.Sprintf("Copying resource files of an update to temp location completed successfully"))
}

// This returns a map of files which would be copied to the temp directory before creating the update zip. Key is
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

// This function copies resource files to the temp directory.
func copyResourceFilesToTempDir(resourceFilesMap map[string]bool) error {
	// Create the directories if they are not available
	updateName := viper.GetString(constant.UPDATE_NAME)
	updateRoot := viper.GetString(constant.UPDATE_ROOT)
	destination := path.Join(constant.TEMP_DIR, updateName, constant.CARBON_HOME)
	err := util.CreateDirectory(destination)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occured when creating the %s directory", destination))

	// Iterate through all resource files
	for filename, isMandatory := range resourceFilesMap {
		source := path.Join(updateRoot, filename)
		destination := path.Join(constant.TEMP_DIR, updateName, filename)
		// Copy the file
		err := util.CopyFile(source, destination)
		if err != nil {
			// If an error occurs while copying, if the file is a mandatory file, return an error. If the file is not
			// mandatory, print a message and continue
			if isMandatory {
				return err
			} else {
				logger.Info(fmt.Sprintf("Optional resource file '%s' not copied.", filename))
			}
		}
	}
	return nil
}

// This function saves update descriptor after modifying the file_changes section.
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
	// The update number will always have enclosing "" to indicate it is an string. So we need to remove that
	updatedData := strings.Replace(string(data), "\"", "", 2)
	modifiedData := []byte(updatedData)
	// Write bytes to file
	_, err = file.Write(modifiedData)
	if err != nil {
		return err
	}
	return nil
}

// This function copies the given file to the temp location for creating the update zip.
func copyFileToTempDir(file *zip.File, fileName string) {
	// Get the update name from viper config
	updateName := viper.GetString(constant.UPDATE_NAME)
	destination := path.Join(constant.TEMP_DIR, updateName, constant.CARBON_HOME, fileName)
	// Replace all / with OS specific path separators to handle OSs like Windows
	destination = strings.Replace(destination, "/", constant.PATH_SEPARATOR, -1)

	// Need to create the relevant parent directories in the destination before writing to the file
	parentDirectory := filepath.Dir(destination)
	err := util.CreateDirectory(parentDirectory)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occured when creating the %s directory", parentDirectory))
	// Open new file for writing only
	newFile, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		util.HandleErrorAndExit(err, fmt.Sprintf("Error occured when opening the (%s) file for writing", destination))
	}
	defer newFile.Close()

	zippedFile, err := file.Open()
	if err != nil {
		util.HandleErrorAndExit(err, fmt.Sprintf("Error occured when opening the (%s)file", fileName))
	}

	// Copying the contents of the file
	_, err = io.Copy(newFile, zippedFile)
	if err != nil {
		util.HandleErrorAndExit(err, fmt.Sprintf("Error occured when copying the content of (%s)file to temp",
			fileName))
	}
	zippedFile.Close()
}
