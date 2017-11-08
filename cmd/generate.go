package cmd

import (
	"archive/zip"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/fatih/color"

	"io/ioutil"
	"os"
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

// This function generates an update zip by comparing the diff between given two distributions.
func generateUpdate(updatedDistPath, previousDistPath string) {

	// Check whether the given distributions exists
	checkDistributionExists(updatedDistPath, "updated")
	checkDistributionExists(previousDistPath, "previous")

	// Check whether the given distributions are zip files
	isZipFile("updated distribution", updatedDistPath)
	logger.Debug(fmt.Sprintf("Provided updated distribution is a zip file"))
	isZipFile("previous distribution", previousDistPath)
	logger.Debug(fmt.Sprintf("Provided previous distribution is a zip file"))

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

	// RootNode is what we use as the root of the updated distribution when populating the tree like structure
	rootNodeOfUpdatedDistribution := createNewNode()
	rootNodeOfUpdatedDistribution, err := readZip(updatedDistributionReader, rootNodeOfUpdatedDistribution)
	handleErrorAndExit(err)
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
			handleErrorAndExit(err)
		}
		data, err := ioutil.ReadAll(zippedFile)
		if err != nil {
			handleErrorAndExit(err)
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
		relativePath := getRelativePath(file)

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
	// RootNode is what we use as the root of the previous distribution when populating tree like structure
	rootNodeOfPreviousDistribution := createNewNode()
	rootNodeOfPreviousDistribution, err = readZip(previousDistributionReader, rootNodeOfPreviousDistribution)
	handleErrorAndExit(err)
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
		relativePath := getRelativePath(file)

		fileNameStrings := strings.Split(fileName, "/")
		fileName = fileNameStrings[len(fileNameStrings)-1]
		if relativePath != "" && !file.FileInfo().IsDir() {
			// Finding newly added files
			findNewlyAddedFiles(rootNodeOfPreviousDistribution, fileName, relativePath, addedFiles)
		}
		//zipReader.Close() // if this is causing panic close it here
	}
	logger.Debug(fmt.Sprintf("Finding newly added files between the given two %s distributions completed "+
		"successfully", distributionName))

	logger.Info("Modified Files : ", modifiedFiles)
	logger.Debug("Number of modified files : ", len(modifiedFiles))
	logger.Info("Removed Directories : ",removedDirectories)
	logger.Debug("Number of Removed Directories : ", len(removedDirectories))
	logger.Info("Removed Files : ", removedFiles)
	logger.Debug("Number of removed files : ", len(removedFiles))
	logger.Info("Added Files : ", addedFiles)
	logger.Debug("Number of added files : ", len(addedFiles))
}

// This function checks whether the given distribution exists.
func checkDistributionExists(distributionPath, distributionState string) {
	exists, err := isFileExists(distributionPath)
	handleErrorAndExit(err, fmt.Sprintf("Error occurred while reading '%s' distribution at '%s' ",
		distributionState, distributionPath))
	if !exists {
		handleErrorAndExit(errors.New(fmt.Sprintf("file does not exist at '%s'. '%s' distribution must "+
			"be a zip file.", distributionPath, distributionState)))
	}
	logger.Debug(fmt.Sprintf("The %s distribution exists in %s location", distributionState, distributionPath))
}

// Check whether the given location contains a file
func isFileExists(location string) (bool, error) {
	locationInfo, err := os.Stat(location)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		} else {
			return false, err
		}
	}
	if locationInfo.IsDir() {
		return false, nil
	} else {
		return true, nil
	}
}

// This function checks whether the given file is a zip file.
// archiveType 		type of the archive
// archiveFilePath	path to the archive file
func isZipFile(archiveType, archiveFilePath string) {
	if !strings.HasSuffix(archiveFilePath, ".zip") {
		handleErrorAndExit(errors.New(fmt.Sprintf("%s must be a zip file. Entered file '%s' does "+
			"not have .zip extension.", archiveType, archiveFilePath)))
	}
}

// This function is used to handle errors (print proper error message and exit if an error exists)
func handleErrorAndExit(err error, customMessage ...interface{}) {
	if err != nil {
		// Call the printError method and exit
		if len(customMessage) == 0 {
			printError(fmt.Sprintf("%s", err.Error()))
		} else {
			printError(append(customMessage, err.Error())...)
		}
		os.Exit(1)
	}
}

// This function is used to print error messages
func printError(args ...interface{}) {
	color.Set(color.FgRed, color.Bold)
	fmt.Println(append(append([]interface{}{"\n[ERROR]"}, args...), "\n")...)
	color.Unset()
}

// This function returns a zip.ReadCloser for the given distribution.
func getZipReader(distributionPath string) *zip.ReadCloser {
	zipReader, err := zip.OpenReader(distributionPath)
	if err != nil {
		handleErrorAndExit(err)
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
			handleErrorAndExit(err)
		}
		// Close zippedFile after reading its data to avoid too many open files leading to a panic
		zippedFile.Close()

		// Calculate the md5 of the file
		hash := md5.New()
		hash.Write(data)
		md5Hash := hex.EncodeToString(hash.Sum(nil))

		// Get the relative path of the file
		logger.Trace(fmt.Sprintf("file.Name: %s", file.Name))

		relativePath := getRelativePath(file)

		// Add the file to root node
		addToRootNode(rootNode, strings.Split(relativePath, "/"), file.FileInfo().IsDir(), md5Hash)
	}
	return rootNode, nil
}

// This function will return the relative path of the given file.
// file	file in which the relative path is to be obtained
func getRelativePath(file *zip.File) (relativePath string) {
	if strings.Contains(file.Name, "/") {
		relativePath = strings.SplitN(file.Name, "/", 2)[1]
	} else {
		relativePath = file.Name
	}
	logger.Trace(fmt.Sprintf("relativePath: %s", relativePath))
	return
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

// This function returns the distribution name of the given zip file.
func getDistributionName(distributionPath string) string {
	paths := strings.Split(distributionPath, string(os.PathSeparator))
	distributionName := strings.TrimSuffix(paths[len(paths)-1], ".zip")
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
