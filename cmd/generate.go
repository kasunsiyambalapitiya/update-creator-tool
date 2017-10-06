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
	generateCmdUse       = "generate <update_dist_loc> <dist_dist_loc> <update_dir>"
	generateCmdShortDesc = "generate a new update"
	generateCmdLongDesc  = dedent.Dedent(`This command will generate a new update zip by comparing the diff between
	the updated pack and the previous released distribution. It is required to run wum-uc init first and provide the
	update location given for init as the third input`)
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
	viper.BindPFlag(constant.CHECK_MD5_DISABLED, generateCmd.Flags().Lookup("md5"))
}

// This function will be called when the generate command is called.
func initializeGenerateCommand(cmd *cobra.Command, args []string) {
	if len(args) != 3 {
		util.HandleErrorAndExit(errors.New("Invalid number of argumants. Run 'wum-uc generate --help' to " +
			"view help."))
	}
	generateUpdate(args[0], args[1], args[2])
}

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
	checkDistribution(updatedDistPath)
	checkDistribution(previousDistPath)

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
	//use viper for getrting distributionName
	util.PrintInfo(fmt.Sprintf("Reading the updated %s. Please wait...", distributionName))
	// rootNode is what we use as the root of the updated distribution when we populate tree like structure.
	rootNodeOfUpdatedDistribution := CreateNewNode()
	rootNodeOfUpdatedDistribution, err = ReadZip(updatedDistPath)
	logger.Debug("root node of the updated distribution received")
	//util.PrintInfo(fmt.Sprintf("Recieved ", len(rootNodeOfUpdatedDistribution.childNodes)))
	util.HandleErrorAndExit(err)
	logger.Debug("Reading updated distribution zip finished")
	logger.Debug("Reading previuosly released distribution zip")
	util.PrintInfo(fmt.Sprintf("Reading the previous %s. to get diff Please wait...", distributionName))

	//rootNodeOfPreviousDistribution := CreateNewNode()
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
	for _, file := range zipReader.Reader.File {
		//open the file for calculating MD5
		zippedFile, err := file.Open()
		if err != nil {
			util.HandleErrorAndExit(err)
		}
		data, err := ioutil.ReadAll(zippedFile)
		// Don't use defer here because otherwise there will be too many open files and it will cause a panic
		zippedFile.Close()
		// Calculate the md5 of the file
		hash := md5.New()
		hash.Write(data)
		md5Hash := hex.EncodeToString(hash.Sum(nil))

		//name of the file
		fileName := file.Name
		fmt.Println("name of the file", fileName)
		//check this log
		logger.Trace(fmt.Sprintf("file.Name: %s and md5", fileName, md5Hash))
		//delete this print
		//fmt.Println("file.Name: and md5", fileName, md5Hash)

		if strings.HasSuffix(fileName, "/") {
			fileName = strings.TrimSuffix(fileName, "/")
		}
		// Get the relative path of the file
		var relativePath string

		if (strings.Contains(fileName, "/")) {
			relativePath = strings.SplitN(fileName, "/", 2)[1]
		} else {
			relativePath = ""
		}

		// Replace all \ with /. Otherwise it will cause issues in Windows OS.
		relativePath = filepath.ToSlash(relativePath)
		logger.Trace(fmt.Sprintf("relativePath: %s", relativePath))
		if strings.HasSuffix(relativePath, "/") {
			relativePath = strings.TrimSuffix(relativePath, "/")
		}
		//delete
		fmt.Println("relativePath:", relativePath)

		fileNameStrings := strings.Split(fileName, "/")
		fmt.Println("length", len(fileNameStrings))
		fileName = fileNameStrings[len(fileNameStrings)-1]
		fmt.Println(fileName)
		if relativePath != "" {
			//Finding modified files
			findModifiedFiles(&rootNodeOfUpdatedDistribution, fileName, md5Hash, relativePath, modifiedFiles)
			//Finding removed files
			findRemovedOrNewlyAddedFiles(&rootNodeOfUpdatedDistribution, fileName, relativePath,
				rootNodeOfUpdatedDistribution.childNodes, removedFiles)
		}

	}

	zipReader.Close()

	//finding newly added files to the previous distribution
	distributionName = getDistributionName(previousDistPath)
	// Read the distribution zip file
	logger.Debug("Reading previous distribution zip")
	//use viper for getrting distributionName
	util.PrintInfo(fmt.Sprintf("Reading the previous %s. Please wait...", distributionName))
	// rootNode is what we use as the root of the updated distribution when we populate tree like structure.
	rootNodeOfPreviousDistribution := CreateNewNode()
	rootNodeOfPreviousDistribution, err = ReadZip(previousDistPath)
	logger.Debug("root node of the previous distribution received")
	//util.PrintInfo(fmt.Sprintf("Recieved ", len(rootNodeOfPreviousDistribution.childNodes)))
	util.HandleErrorAndExit(err)
	logger.Debug("Reading previous distribution zip finished")
	logger.Debug("Reading updated distribution zip for finding newly added files")
	//check text content of the log
	util.PrintInfo(fmt.Sprintf("Reading the updated %s. to get diff Please wait...", distributionName))

	zipReader, err = zip.OpenReader(updatedDistPath)
	if err != nil {
		util.HandleErrorAndExit(err)
	}
	defer zipReader.Close()
	// iterate throug updated pack to identify the newly added files
	for _, file := range zipReader.Reader.File {
		// we donot need to calculate the md5 of the file as we are filtering only the added files
		// name of the file
		fileName := file.Name
		fmt.Println("name of the file", fileName)
		//check this log
		logger.Trace(fmt.Sprintf("file.Name: %s and md5", fileName))
		//delete this print
		//fmt.Println("file.Name: and md5", fileName, md5Hash)

		if strings.HasSuffix(fileName, "/") {
			fileName = strings.TrimSuffix(fileName, "/")
		}
		// Get the relative path of the file
		var relativePath string
		if (strings.Contains(fileName, "/")) {
			relativePath = strings.SplitN(fileName, "/", 2)[1]
		} else {
			relativePath = ""
		}

		// Replace all \ with /. Otherwise it will cause issues in Windows OS.
		relativePath = filepath.ToSlash(relativePath)
		logger.Trace(fmt.Sprintf("relativePath: %s", relativePath))
		//delete
		//fmt.Println("relativePath:", relativePath)

		fileNameStrings := strings.Split(fileName, "/")
		fmt.Println("length", len(fileNameStrings))
		fileName = fileNameStrings[len(fileNameStrings)-1]
		fmt.Println(fileName)
		//Finding newly added files
		if relativePath != "" {
			findRemovedOrNewlyAddedFiles(&rootNodeOfPreviousDistribution, fileName, relativePath,
				rootNodeOfPreviousDistribution.childNodes, addedFiles)
		}
		//zipReader.Close() // if this is causing panic we need to close it here
	}

	//fmt.Println("Modified files",modifiedFiles)
	util.PrintInfo("Modified Files", modifiedFiles)
	util.PrintInfo("length", len(modifiedFiles))
	//fmt.Println("removed Files",removedFiles)
	util.PrintInfo("removed Files", removedFiles)
	util.PrintInfo("length", len(removedFiles))
	//fmt.Println("Added Files",addedFiles)
	util.PrintInfo("Added Files", addedFiles)
	util.PrintInfo("length", len(addedFiles))

	//11) Update added,removed and modified files in the the updateDescriptor struct
	filteredAddedFiles := alterUpdateDescriptor(modifiedFiles, removedFiles, addedFiles, updateDescriptor)
	fmt.Println(filteredAddedFiles)

	//12) Copy files in the update location to a temp directory
	copyMandatoryFilesToTemp()

	//13) Save the updateDescriptor with newly added, removed and modified files to the the update-descriptor.yaml

	// Todo handle interrupts
	data, err := MarshalUpdateDescriptor(updateDescriptor)
	util.HandleErrorAndExit(err, "Error occurred while marshalling the update-descriptor.")
	err = SaveUpdateDescriptor(constant.UPDATE_DESCRIPTOR_FILE, data)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while saving the (%v).",
		constant.UPDATE_DESCRIPTOR_FILE))

	//14) Extract newly added and modified files from the updated zip and copy them to the temp directory for
	// creating the update zip using the same zipreader used in reading the updated zip
	for _, file := range zipReader.Reader.File {
		util.PrintInfo(file.Name)
		var fileName string
		if strings.Contains(file.Name, "/") {
			fileName = strings.SplitN(file.Name, "/", 2)[1]
			util.PrintInfo(fileName)
		} else {
			fileName = file.Name
			util.PrintInfo(fileName)
		}

		// extracting newly added files from the updated distribution
		_, found := filteredAddedFiles[fileName]
		if found {
			copyAlteredFileToTempDir(file, fileName)
		}
		// extracting modified files from the updated distribution
		_, found = modifiedFiles[fileName]
		if found {
			copyAlteredFileToTempDir(file, fileName)
		}
	}
	zipReader.Close()

	//15) Create the update zip
	//todo make the update zip in the temp dir
	targetDirectory := path.Join(updateRoot, constant.TEMP_DIR, updateName)
	//make targetDirectory path compatible with windows OS
	targetDirectory = strings.Replace(targetDirectory, "/", constant.PATH_SEPARATOR, -2)
	updateZipName := updateName + ".zip"
	err = archiver.Zip.Make(path.Join(updateRoot, updateZipName), []string{targetDirectory})
	util.HandleErrorAndExit(err)
	//16) Delete the temp directory
	util.CleanUpDirectory(path.Join(updateRoot, constant.TEMP_DIR))

}

//This function will be used to check for the availability of the given file in the update directory location
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

func checkDistributionPath(distributionPath, distributionState string) {
	exists, err := util.IsFileExists(distributionPath)
	util.HandleErrorAndExit(err, fmt.Sprintf("Error occurred while checking '%s' '%s' ", distributionState,
		distributionPath))
	if !exists {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("File does not exist at '%s'. '%s' Distribution must "+
			"be a zip file.", distributionPath, distributionState)))
	}
}

//This function checks whether the given distritbution is a zip file
func checkDistribution(distributionName string) {
	//to a seperate method and reuse in create.go
	if !strings.HasSuffix(distributionName, ".zip") {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf("Entered distribution path(%s) does not point to a "+
			"zip file.", distributionName)))
	}
}

func getDistributionName(distributionZipName string) string {

	//make this a common method
	// Get the product name from the distribution path and set it as a viper config
	paths := strings.Split(distributionZipName, constant.PATH_SEPARATOR)
	distributionName := strings.TrimSuffix(paths[len(paths)-1], ".zip")
	viper.Set(constant.PRODUCT_NAME, distributionName)
	return distributionName
}

//TOdo add Docs
//Todo add logs
//Todo check altered lift of addedfiles
func findModifiedFiles(root *Node, name string, md5Hash string, relativePath string,
	modifiedFiles map[string]struct{}) {
	// Check whether the given name is in the child Nodes
	childNode, found := root.childNodes[name]
	//fmt.Println("entered to findModified")
	if found && childNode.isDir == false && childNode.relativeLocation == relativePath && childNode.md5Hash !=
		md5Hash {
		//fmt.Println("found")
		_, found := modifiedFiles[childNode.relativeLocation]
		if !found {
			modifiedFiles[childNode.relativeLocation] = struct{}{}
		}

	}
	// Regardless of whether the file is found or not, iterate through all sub directories to find all matches
	for _, childNode := range root.childNodes {
		if childNode.isDir {
			findModifiedFiles(childNode, name, md5Hash, relativePath, modifiedFiles)
		}
	}
}

func findRemovedOrNewlyAddedFiles(root *Node, fileName string, relativeLocation string,
	childNodesOfRootOfParentDistribution map[string]*Node, matches map[string]struct{}) bool {
	// need to remove if there is a slash at the end of the relativeLocation path
	//fmt.Println("relative loc before: ", relativeLocation)
	//relativeLocation = strings.TrimSuffix(relativeLocation, "/")
	//fmt.Println(relativeLocation)
	// Check whether a file exists in the given relative path in any child Node
	_, found := root.childNodes[fileName]
	//_, recorded := matches[relativeLocation]

	//checking whether the file is in the correct relative location
	if found {
		if root.childNodes[fileName].relativeLocation != relativeLocation {
			found = false
		} else {
			return true
		}
	}
	if !found {
		for _, childNode := range root.childNodes {
			if childNode.isDir {
				found = findRemovedOrNewlyAddedFiles(childNode, fileName, relativeLocation,
					childNodesOfRootOfParentDistribution, matches)
				if found {
					//Todo check here
					break
				}
			}
		}

	}
	//after going through all the childnodes if it is still false means, that relative location is not present
	parentRootNode := reflect.DeepEqual(childNodesOfRootOfParentDistribution, root.childNodes)
	if !found && parentRootNode {
		matches[relativeLocation] = struct{}{}
	}
	return found
}

//This function is used to update the updateDescriptor with the added, removed and modified files from the update
func alterUpdateDescriptor(modifiedFiles, removedFiles, addedFiles map[string]struct{},
	updateDescriptor *util.UpdateDescriptor) map[string]struct{} {
	//Todo needs to filterout other folders in META-INF
	filteredAddedFiles := make(map[string]struct{})
	featurePrefix := "wso2/lib/features/"

	//append modified files
	for modifiedFile, _ := range modifiedFiles {
		updateDescriptor.File_changes.Modified_files = append(updateDescriptor.File_changes.Modified_files, modifiedFile)
	}

	//append removed files
	//map[string]struct{} is used here as it is trival to search for an element in a slice
	removedFeatureNames := make(map[string]struct{})
	//Todo refactor delete word to remove to be consistent with the UpdateDescriptor struct
	for removedFile, _ := range removedFiles {
		//need to keep track of the features being removed as we only specify the relevant feature directories to be
		//removed on update-descriptor.yaml, without mentioning the files and subdirectories in them
		if strings.HasPrefix(removedFile, featurePrefix) {
			//extracting the relevant feature name to be saved in the map for future filtering
			removedFeatureName := strings.SplitN(strings.TrimPrefix(removedFile, featurePrefix), "/", 2)[0]
			_, found := removedFeatureNames[removedFeatureName]
			// if the removedFeature's root directory which is in "wso2/lib/features/" is present, it's root
			// directory has already been added for removal (as the complete feature directory)
			if !found {
				removedFeatureNames[removedFeatureName] = struct{}{}
				//adding only the root directory of the removed feature to the updateDescriptor
				updateDescriptor.File_changes.Removed_files = append(updateDescriptor.File_changes.Removed_files,
					featurePrefix+removedFeatureName)
				//ToDo ask shall we put "/" at the end of the directory to indicate it is a directory, this will not cause troubles with the node.relative location
				//as we are not using them for deleted files. We just delete those in the previous distribution
			}
		} else {
			updateDescriptor.File_changes.Removed_files = append(updateDescriptor.File_changes.Removed_files, removedFile)
		}
	}

	//append newly added files
	for addedFile, _ := range addedFiles {
		//need to filter out root directories of newl added features, as they will be automatically created when
		// coping the files and sub directories in them during updating
		//check whether the addedFile exists inside the "wso2/lib/features/"
		if strings.HasPrefix(addedFile, featurePrefix) {
			//Todo do we need to consider the platform indendependence in here for "/"
			if strings.Contains(strings.TrimPrefix(addedFile, featurePrefix), "/") {
				// if it contains "/" then addedFile is either a file or a subdirectory inside the above root feature
				// directory
				filteredAddedFiles[addedFile] = struct{}{}
				updateDescriptor.File_changes.Added_files = append(updateDescriptor.File_changes.Added_files, addedFile)
			}
		} else {
			filteredAddedFiles[addedFile] = struct{}{}
			updateDescriptor.File_changes.Added_files = append(updateDescriptor.File_changes.Added_files, addedFile)
		}
	}
	return filteredAddedFiles
}

//This will be used to copy mandatory files of an update that exists in given update location to a temp location for
// creating the update zip
func copyMandatoryFilesToTemp() {
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

}

//ToDo change so that works on current location's temp directory
//Todo double check fmt.Sprintf()
func copyAlteredFileToTempDir(file *zip.File, fileName string) {
	//Get the update name from viper config
	updateName := viper.GetString(constant.UPDATE_NAME)
	//Get the update location from viper config
	updateRoot := viper.GetString(constant.UPDATE_ROOT)
	destination := path.Join(updateRoot, constant.TEMP_DIR, updateName, constant.CARBON_HOME, fileName)
	//Replace all / with OS specific path separators to handle OSs like Windows
	destination = strings.Replace(destination, "/", constant.PATH_SEPARATOR, -1)

	//Need to create the relevant parent directories in the destination before writing the file
	parentDirectory := filepath.Dir(destination)
	err := util.CreateDirectory(parentDirectory)
	util.HandleErrorAndExit(err, fmt.Sprint("Error occured when creating the (%v) directory", parentDirectory))
	//open new file for writing only
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
func copyMandatoryFileToTemp(fileName, updateRoot, updateName string) {
	source := path.Join(updateRoot, fileName)
	// we donot need to replace the path seperator as this file currently exits in the system, so it can be open by
	// os package by default
	//ToDo change so that works on current location's temp directory
	destination := path.Join(updateRoot, constant.TEMP_DIR, updateName, fileName)
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
}
