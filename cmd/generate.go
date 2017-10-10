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
	"path"
	"os"
	"io"
	"github.com/mholt/archiver"
)

// Values used to print help command.
var (
	generateCmdUse       = "generate <update_dist_loc> <prev_dist_loc> <update_dir>"
	generateCmdShortDesc = "Generate a new update"
	generateCmdLongDesc  = dedent.Dedent(`This command will generate a new update zip by comparing the diff between
	the updated distribution and the previous released distribution. It is required to run wum-uc init first and provide
	the	update location given for init as the third input`)
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
	//viper.BindPFlag(constant.CHECK_MD5_DISABLED, generateCmd.Flags().Lookup("md5"))
}

// This function will be called when the generate command is called.
func initializeGenerateCommand(cmd *cobra.Command, args []string) {
	if len(args) != 3 {
		util.HandleErrorAndExit(errors.New("Invalid number of argumants. Run 'wum-uc generate --help' to " +
			"view help."))
	}
	generateUpdate(args[0], args[1], args[2])
}

//This function will start generating the update zip according to the diff between given two distributions
func generateUpdate(updatedDistPath, previousDistPath, updateDirectoryPath string) {
	// set debug level
	setLogLevel()
	logger.Debug("[generate] command called")

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

	//2) Check whether the update-descriptor.yaml file exists in the update directory
	checkFileExistance(updateDirectoryPath, constant.UPDATE_DESCRIPTOR_FILE)

	//3) Check whether the LICENSE.txt file file exists in the update directory
	checkFileExistance(updateDirectoryPath, constant.LICENSE_FILE)

	//4) Check whether the NOT_A_CONTRIBUTION.txt file exists in the update directory
	checkFileExistance(updateDirectoryPath, constant.NOT_A_CONTRIBUTION_FILE)

	//5) Check whether the given distributions exists
	checkDistributionPath(updatedDistPath, "updated")
	checkDistributionPath(previousDistPath, "previous")

	//6) Check whether the given distributions are zip files
	checkDistribution(updatedDistPath, "updated")
	checkDistribution(previousDistPath, "previous")

	//7) Read update-descriptor.yaml and set the update name which will be used when creating the update zip file.
	updateDescriptor, err := util.LoadUpdateDescriptor(constant.UPDATE_DESCRIPTOR_FILE, updateDirectoryPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred when reading '%s' file.",
		constant.UPDATE_DESCRIPTOR_FILE))

	//8) Validate the file format of the update-descriptor.yaml
	err = util.ValidateUpdateDescriptor(updateDescriptor)
	util.HandleErrorAndExit(err, fmt.Sprintf("'%s' format is incorrect.", constant.UPDATE_DESCRIPTOR_FILE))

	//9) Set the update name
	updateName := GetUpdateName(updateDescriptor, constant.UPDATE_NAME_PREFIX)
	viper.Set(constant.UPDATE_NAME, updateName)

	//10) Identify modified, added and removed files by comparing the diff between two given distributions
	//Get the distribution name
	distributionName := getDistributionName(updatedDistPath)
	// Read the updated distribution zip file
	logger.Debug("Reading updated distribution zip")
	util.PrintInfo(fmt.Sprintf("Reading the updated %s. Please wait...", distributionName))

	// rootNode is what we use as the root of the updated distribution when we populate tree like structure.
	rootNodeOfUpdatedDistribution := CreateNewNode()
	rootNodeOfUpdatedDistribution, err = ReadZip(updatedDistPath)
	logger.Debug("root node of the updated distribution received")
	util.HandleErrorAndExit(err)
	logger.Debug("Reading updated distribution zip finished")
	logger.Debug("Reading previously released distribution zip for finding removed and modified files")
	util.PrintInfo(fmt.Sprintf("Reading the previous %s. to get diff Please wait...", distributionName))

	zipReader, err := zip.OpenReader(previousDistPath)
	if err != nil {
		util.HandleErrorAndExit(err)
	}
	defer zipReader.Close()

	//slices for modified, changed and removed files from the update
	modifiedFiles := make(map[string]struct{})
	removedFiles := make(map[string]struct{})
	addedFiles := make(map[string]struct{})

	//iterate through each file to identify modified and removed files
	logger.Debug(fmt.Sprintf("Finding modified and removed files between the given 2 distributions"))
	for _, file := range zipReader.Reader.File {
		//open the file for calculating MD5
		zippedFile, err := file.Open()
		if err != nil {
			util.HandleErrorAndExit(err)
		}
		data, err := ioutil.ReadAll(zippedFile)
		// Don't use defer here because there will be too many open files and it will cause a panic
		zippedFile.Close()
		// Calculate the md5 of the file
		hash := md5.New()
		hash.Write(data)
		md5Hash := hex.EncodeToString(hash.Sum(nil))

		//name of the file
		fileName := file.Name
		logger.Trace(fmt.Sprintf("file.Name: %s and md5", fileName, md5Hash))

		if strings.HasSuffix(fileName, "/") {
			fileName = strings.TrimSuffix(fileName, "/")
		}
		// Get the relative location of the file
		var relativeLocation string

		if (strings.Contains(fileName, "/")) {
			relativeLocation = strings.SplitN(fileName, "/", 2)[1]
		} else {
			relativeLocation = ""
		}

		// Replace all \ with /. Otherwise it will cause issues in Windows OS.
		relativeLocation = filepath.ToSlash(relativeLocation)
		if strings.HasSuffix(relativeLocation, "/") {
			relativeLocation = strings.TrimSuffix(relativeLocation, "/")
		}
		logger.Trace(fmt.Sprintf("relativeLocation:%s", relativeLocation))

		fileNameStrings := strings.Split(fileName, "/")
		fileName = fileNameStrings[len(fileNameStrings)-1]
		logger.Trace(fmt.Sprintf("File Name %s", fileName))
		if relativeLocation != "" && !file.FileInfo().IsDir() {
			//Finding modified files
			findModifiedFiles(&rootNodeOfUpdatedDistribution, fileName, md5Hash, relativeLocation, modifiedFiles)
			//Finding removed files
			findRemovedOrNewlyAddedFiles(&rootNodeOfUpdatedDistribution, fileName, relativeLocation,
				rootNodeOfUpdatedDistribution.childNodes, removedFiles)
		}

	}
	logger.Debug(fmt.Sprintf("Done finding modified and removed files between the given 2 distributions"))
	// closing the ReadCloser of previous distribution
	zipReader.Close()
	logger.Debug(fmt.Sprintf("Closed the ReadCloser of previous distribution"))

	//finding newly added files to the previous distribution
	distributionName = getDistributionName(previousDistPath)
	// Read the distribution zip file
	logger.Debug("Reading previous distribution zip")
	util.PrintInfo(fmt.Sprintf("Reading the previous %s. Please wait...", distributionName))
	// rootNode is what we use as the root of the previous distribution when we populate tree like structure.
	rootNodeOfPreviousDistribution := CreateNewNode()
	rootNodeOfPreviousDistribution, err = ReadZip(previousDistPath)
	logger.Debug("root node of the previous distribution received")
	//util.PrintInfo(fmt.Sprintf("Recieved ", len(rootNodeOfPreviousDistribution.childNodes)))
	util.HandleErrorAndExit(err)
	logger.Debug("Reading previous distribution zip finished")
	logger.Debug("Reading updated distribution zip for finding newly added files")
	util.PrintInfo(fmt.Sprintf("Reading the updated %s. to get diff Please wait...", distributionName))

	zipReader, err = zip.OpenReader(updatedDistPath)
	if err != nil {
		util.HandleErrorAndExit(err)
	}
	defer zipReader.Close()
	// iterate through updated pack to identify the newly added files
	logger.Debug(fmt.Sprintf("Finding newly added files between the given 2 distributions"))
	for _, file := range zipReader.Reader.File {
		// we do not need to calculate the md5 of the file as we are filtering only the added files
		// name of the file
		fileName := file.Name
		logger.Trace(fmt.Sprintf("file.Name: %s", fileName))

		if strings.HasSuffix(fileName, "/") {
			fileName = strings.TrimSuffix(fileName, "/")
		}
		//ToDo make getting relative location a util method
		// Get the relative location of the file
		var relativeLocation string
		if (strings.Contains(fileName, "/")) {
			relativeLocation = strings.SplitN(fileName, "/", 2)[1]
		} else {
			relativeLocation = ""
		}

		// Replace all \ with /. Otherwise it will cause issues in Windows OS.
		relativeLocation = filepath.ToSlash(relativeLocation)
		logger.Trace(fmt.Sprintf("relativeLocation: %s", relativeLocation))

		fileNameStrings := strings.Split(fileName, "/")
		fileName = fileNameStrings[len(fileNameStrings)-1]
		logger.Trace(fmt.Sprintf("File Name %s", fileName))
		//Finding newly added files
		if relativeLocation != "" && !file.FileInfo().IsDir() {
			findRemovedOrNewlyAddedFiles(&rootNodeOfPreviousDistribution, fileName, relativeLocation,
				rootNodeOfPreviousDistribution.childNodes, addedFiles)
		}
		//zipReader.Close() // if this is causing panic we need to close it here
	}
	logger.Debug(fmt.Sprintf("Done finding newly added files between the given 2 distributions"))

	util.PrintInfo("Modified Files", modifiedFiles)
	util.PrintInfo("length", len(modifiedFiles))
	util.PrintInfo("removed Files", removedFiles)
	util.PrintInfo("length", len(removedFiles))
	util.PrintInfo("Added Files", addedFiles)
	util.PrintInfo("length", len(addedFiles))

	//11) Update added,removed and modified files in the the updateDescriptor struct
	filteredAddedFiles := alterUpdateDescriptor(modifiedFiles, removedFiles, addedFiles, updateDescriptor)

	//12) Copy files in the update location to a temp directory
	copyMandatoryFilesToTemp()

	//13) Save the updateDescriptor with newly added, removed and modified files to the the update-descriptor.yaml

	// Todo handle interrupts
	data, err := MarshalUpdateDescriptor(updateDescriptor)
	util.HandleErrorAndExit(err, "Error occurred while marshalling the update-descriptor.")
	err = SaveUpdateDescriptor(constant.UPDATE_DESCRIPTOR_FILE, data)
	logger.Debug(fmt.Sprintf("update-descriptor.yaml updated successfully"))
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while saving the (%v).",
		constant.UPDATE_DESCRIPTOR_FILE))

	//14) Extract newly added and modified files from the updated zip and copy them to the temp directory for
	// creating the update zip. The same zipReader used in reading the updated zip is used in here
	logger.Debug(fmt.Sprintf("Extracting newly added and modified files from the updated zip"))
	for _, file := range zipReader.Reader.File {
		var fileName string
		if strings.Contains(file.Name, "/") {
			fileName = strings.SplitN(file.Name, "/", 2)[1]
			//util.PrintInfo(fileName)
		} else {
			fileName = file.Name
		}

		// extracting newly added files from the updated distribution
		_, found := filteredAddedFiles[fileName]
		if found {
			logger.Debug(fmt.Sprintf("Copying newly added file %s to temp location", fileName))
			copyAlteredFileToTempDir(file, fileName)
		}
		// extracting modified files from the updated distribution
		_, found = modifiedFiles[fileName]
		if found {
			logger.Debug(fmt.Sprintf("Copying modifite file %s to temp location", fileName))
			copyAlteredFileToTempDir(file, fileName)
		}
	}
	zipReader.Close()
	logger.Debug(fmt.Sprintf("Copying newly added and modified files from updated zip to temp location"))

	//15) Create the update zip
	logger.Debug(fmt.Sprintf("Creating the update zip"))
	targetDirectory := path.Join(constant.TEMP_DIR, updateName)
	//make targetDirectory path compatible with windows OS
	targetDirectory = strings.Replace(targetDirectory, "/", constant.PATH_SEPARATOR, -2)
	updateZipName := updateName + ".zip"
	err = archiver.Zip.Make(path.Join(updateRoot, updateZipName), []string{targetDirectory})
	util.HandleErrorAndExit(err)
	logger.Debug(fmt.Sprintf("Creating the update zip completed successfully"))
	//16) Delete the temp directory
	util.CleanUpDirectory(path.Join(constant.TEMP_DIR))
	logger.Debug(fmt.Sprintf("Temp directory deleted successfully"))
}

//This function will be used to check for the availability of the given file in the given update directory location
func checkFileExistance(updateDirectoryPath, fileName string) {
	// Construct the relevant file location
	updateDescriptorPath := path.Join(updateDirectoryPath, fileName)
	exists, err := util.IsFileExists(updateDescriptorPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while reading the '%v'",
		fileName))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("'%s' not found at '%s' directory.",
			fileName, updateDirectoryPath)))
	}
	logger.Debug(fmt.Sprintf("%s exists. Location %s", fileName, updateDescriptorPath))
}

//This function checks whether the given distribution exists
func checkDistributionPath(distributionPath, distributionState string) {
	exists, err := util.IsFileExists(distributionPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while checking '%s' '%s' ", distributionState,
		distributionPath))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("File does not exist at '%s'. '%s' Distribution must "+
			"be a zip file.", distributionPath, distributionState)))
	}
	logger.Debug(fmt.Sprintf("The %s distribution exists in %s location", distributionState, distributionPath))
}

//This function checks whether the given distribution is a zip file
func checkDistribution(distributionPath string, distributionState string) {
	//ToDo to a seperate method and reuse in create.go
	if !strings.HasSuffix(distributionPath, ".zip") {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("Entered distribution path '%s' does not point to a "+
			"zip file.", distributionPath)))
	}
	logger.Debug(fmt.Sprintf("The %s distribution is a zip file", distributionState))
}

//This function is used to extract out the distribution name from the given zip file
func getDistributionName(distributionZipName string) string {
	//ToDo make this a common method
	// Get the product name from the distribution path and set it as a viper config
	paths := strings.Split(distributionZipName, constant.PATH_SEPARATOR)
	distributionName := strings.TrimSuffix(paths[len(paths)-1], ".zip")
	viper.Set(constant.PRODUCT_NAME, distributionName)
	logger.Debug(fmt.Sprintf("Distribution name set in to the viper config"))
	return distributionName
}

//This function is used for identifying modified files between the given 2 distributions
//Todo check altered lift of addedfiles
func findModifiedFiles(root *Node, fileName string, md5Hash string, relativeLocation string,
	modifiedFiles map[string]struct{}) {
	logger.Trace(fmt.Sprintf("Checking %s file for modifications in %s relative path", fileName, relativeLocation))
	// Check whether the given fileName is in the child Nodes
	childNode, found := root.childNodes[fileName]
	if found && childNode.isDir == false && childNode.relativeLocation == relativeLocation && childNode.md5Hash !=
		md5Hash {
		logger.Trace(fmt.Sprintf("The file %s exists in the both distributions with mismatch md5, meaning they are "+
			"modified", fileName))
		_, found := modifiedFiles[childNode.relativeLocation]
		if !found {
			modifiedFiles[childNode.relativeLocation] = struct{}{}
			logger.Trace(fmt.Sprintf("Modified file %s is added to the modifiedFiles map", fileName))
		}
	}
	// Regardless of whether the file is found or not, iterate through all sub directories to find all matches
	for _, childNode := range root.childNodes {
		if childNode.isDir {
			findModifiedFiles(childNode, fileName, md5Hash, relativeLocation, modifiedFiles)
		}
	}
	logger.Trace(fmt.Sprintf("Checking %s file for modifications completed in %s relative path", fileName, relativeLocation))
}

//This function is used for identifying removed and newly added files between given two zip files
func findRemovedOrNewlyAddedFiles(root *Node, fileName string, relativeLocation string,
	childNodesOfRootOfParentDistribution map[string]*Node, matches map[string]struct{}) bool {
	logger.Trace(fmt.Sprintf("Checking %s file to identify as a removed or newly added in %s relative path",
		fileName, relativeLocation))
	// Check whether a file exists in the given relative path in any child Node
	_, found := root.childNodes[fileName]

	//checking whether the file is in the correct relative location, this is done as multiple files will have the
	// same file name but they exists in different relative locations
	if found {
		if root.childNodes[fileName].relativeLocation != relativeLocation {
			found = false
			logger.Trace(fmt.Sprintf("The %s file is not found in the same relative path, so it can be either "+
				"removed or newly added file", fileName))
		} else {
			logger.Trace(fmt.Sprintf("The %s file is found in the same relative path, so it is niether removed or "+
				"newly added file", fileName))
			return true
		}
	}
	if !found {
		for _, childNode := range root.childNodes {
			if childNode.isDir {
				found = findRemovedOrNewlyAddedFiles(childNode, fileName, relativeLocation,
					childNodesOfRootOfParentDistribution, matches)
				if found {
					logger.Trace(fmt.Sprintf("The %s file is found in the same relative path, so it is niether removed or "+
						"newly added file", fileName))
					//This is to break out every for loop started for searching the relevant file, if the the file is
					// found with the same relative location.
					break
				}
			}
		}

	}
	//after going through all the childnodes if it is still false means, that relative location is not present among
	// in any nodes
	parentRootNode := reflect.DeepEqual(childNodesOfRootOfParentDistribution, root.childNodes)
	if !found && parentRootNode {
		matches[relativeLocation] = struct{}{}
	}
	return found
}

//This function is used to update the updateDescriptor with the added, removed and modified files from the update
func alterUpdateDescriptor(modifiedFiles, removedFiles, addedFiles map[string]struct{},
	updateDescriptor *util.UpdateDescriptor) map[string]struct{} {
	logger.Debug(fmt.Sprintf("Altering UpdateDescriptor started"))
	filteredAddedFiles := make(map[string]struct{})
	featurePrefix := "wso2/lib/features/"

	//append modified files
	logger.Debug(fmt.Sprintf("Appending modified files to the UpdateDescriptor started"))
	for modifiedFile, _ := range modifiedFiles {
		updateDescriptor.File_changes.Modified_files = append(updateDescriptor.File_changes.Modified_files, modifiedFile)
	}
	logger.Debug(fmt.Sprintf("Appending modified files to the UpdateDescriptor finished successfully"))

	//append removed files
	logger.Debug(fmt.Sprintf("Appending removed files to the UpdateDescriptor started"))
	//map[string]struct{} is used here as it is trival to search for an element in a slice
	removedFeatureNames := make(map[string]struct{})
	for removedFile, _ := range removedFiles {
		//need to keep track of the features being removed as we only specify the relevant feature directories to be
		//removed on update-descriptor.yaml, without mentioning the files and subdirectories in them
		if strings.HasPrefix(removedFile, featurePrefix) {
			//extracting the relevant feature name to be saved in the map for future filtering
			removedFeatureName := strings.SplitN(strings.TrimPrefix(removedFile, featurePrefix), "/", 2)[0]
			_, found := removedFeatureNames[removedFeatureName]
			// if the removedFeature's root directory which is in "wso2/lib/features/" is present in the map of
			// removedFeatureNames, it's root directory has already been added for removal (as the complete feature
			// directory)
			if !found {
				removedFeatureNames[removedFeatureName] = struct{}{}
				//adding only the root directory of the removed feature to the updateDescriptor
				updateDescriptor.File_changes.Removed_files = append(updateDescriptor.File_changes.Removed_files,
					featurePrefix+removedFeatureName)
				//ToDo ask shall we put "/" at the end of the directory to indicate it is a directory, this will not cause troubles with the node.relative location
				//as we are not using nodes or any files in updated distribution for removing files in the previous
				// distribution. We just remove those in the previous distribution
			}
		} else {
			updateDescriptor.File_changes.Removed_files = append(updateDescriptor.File_changes.Removed_files, removedFile)
		}
	}
	logger.Debug(fmt.Sprintf("Appending removed files to the UpdateDescriptor finished successfully"))

	//append newly added files
	logger.Debug(fmt.Sprintf("Appending added files to the UpdateDescriptor started"))
	for addedFile, _ := range addedFiles {
		filteredAddedFiles[addedFile] = struct{}{}
		updateDescriptor.File_changes.Added_files = append(updateDescriptor.File_changes.Added_files, addedFile)
	}
	logger.Debug(fmt.Sprintf("Appending added files to the UpdateDescriptor finished successfully"))
	logger.Debug(fmt.Sprintf("Altering UpdateDescriptor finished successfully"))
	return filteredAddedFiles
}

//This is used to copy mandatory files of an update, that exists in given update location to a temp location for
// creating the update zip
func copyMandatoryFilesToTemp() {
	logger.Debug(fmt.Sprintf("Copying mandatory files of an update to temp location started"))
	//Get the update name from viper config
	updateName := viper.GetString(constant.UPDATE_NAME)
	//Get the update location from viper config
	updateRoot := viper.GetString(constant.UPDATE_ROOT)
	updateDescriptorFileName := constant.UPDATE_DESCRIPTOR_FILE
	licenseTxtFileName := constant.LICENSE_FILE
	notAContributionFileName := constant.NOT_A_CONTRIBUTION_FILE

	//copy update-descriptor.yaml to temp location
	copyMandatoryFileToTemp(updateDescriptorFileName, updateRoot, updateName)
	//copy LICENSE.TXT to temp location
	copyMandatoryFileToTemp(licenseTxtFileName, updateRoot, updateName)
	//copy NOT_A_CONTRIBUTION.txt to temp location
	copyMandatoryFileToTemp(notAContributionFileName, updateRoot, updateName)
	logger.Debug(fmt.Sprintf("Copying mandatory files of an update to temp location completed successfully"))
}

// This is used to copy modified and newly added files to the temp location for creating the update zip
func copyAlteredFileToTempDir(file *zip.File, fileName string) {
	//Get the update name from viper config
	updateName := viper.GetString(constant.UPDATE_NAME)
	destination := path.Join(constant.TEMP_DIR, updateName, constant.CARBON_HOME, fileName)
	//Replace all / with OS specific path separators to handle OSs like Windows
	destination = strings.Replace(destination, "/", constant.PATH_SEPARATOR, -1)

	//Need to create the relevant parent directories in the destination before writing the file
	parentDirectory := filepath.Dir(destination)
	err := util.CreateDirectory(parentDirectory)
	util.HandleErrorAndExit(err, fmt.Sprint("Error occured when creating the (%v) directory", parentDirectory))
	//Open new file for writing only
	newFile, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		util.HandleErrorAndExit(err, fmt.Sprintf("Error occured when opening the (%s) file for writing", destination))
	}
	defer newFile.Close()

	zippedFile, err := file.Open()
	if err != nil {
		util.HandleErrorAndExit(err, fmt.Sprintf("Error occured when opening the (%s)file", fileName))
	}

	//copying the contents of the file
	_, err = io.Copy(newFile, zippedFile)
	if err != nil {
		util.HandleErrorAndExit(err, fmt.Sprintf("Error occured when copying the content of (%s)file to temp",
			fileName))
	}
	zippedFile.Close()
}

//ToDo add logs and user outs for all functions
//This function will be used to copy given mandatory file to the temp location for creating the update zip
func copyMandatoryFileToTemp(fileName, updateRoot, updateName string) {
	logger.Debug(fmt.Sprintf("Copying mandatory file %s to temp location", fileName))
	source := path.Join(updateRoot, fileName)
	// we donot need to replace the path seperator as this file currently exits in the system, so it can be open by
	// os package by default
	//ToDo change so that works on current location's temp directory
	//destination := path.Join(updateRoot, constant.TEMP_DIR, updateName, fileName)
	destination := path.Join(constant.TEMP_DIR, updateName, fileName)
	//Replace all / with OS specific path separators to handle OSs like Windows
	destination = strings.Replace(destination, "/", constant.PATH_SEPARATOR, -1)
	// may need to change the implementations once the PR#19 merged
	parentDirectory := path.Dir(destination)
	logger.Debug("parent directory:", parentDirectory)
	err := util.CreateDirectory(parentDirectory)
	util.HandleErrorAndExit(err, fmt.Sprint("Error occured when creating the (%v) directory", parentDirectory))
	err = util.CopyFile(source, destination)
	util.HandleErrorAndExit(err, fmt.Sprint("Error occured when copying source file (%v) to destination (%v)",
		source, destination))
	util.HandleErrorAndExit(err)
	logger.Debug(fmt.Sprintf("Copying mandatory file %s to temp location completed", fileName))
}
