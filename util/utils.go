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

package util

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"archive/zip"
	"github.com/fatih/color"
	"github.com/ian-kent/go-log/log"
	"github.com/pkg/errors"
	"github.com/wso2/update-creator-tool/constant"
	"gopkg.in/yaml.v2"
)

var logger = log.Logger()

// struct which is used to read update-descriptor.yaml
type UpdateDescriptor struct {
	Update_number    string
	Platform_version string
	Platform_name    string
	Applies_to       string
	Bug_fixes        map[string]string
	Description      string
	File_changes     struct {
		Added_files    []string
		Removed_files  []string
		Modified_files []string
	}
}

// Structs to get the summary field from the jira response
type Fields struct {
	Summary string `json:"summary"`
}

type JiraResponse struct {
	Fields Fields `json:"fields"`
}

// This will return the md5 hash of the file in the given filepath
func GetMD5(filepath string) (string, error) {
	var result []byte
	file, err := os.Open(filepath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(result)), nil
}

// This function is used to delete the temporary directories
func CleanUpDirectory(path string) {
	logger.Debug(fmt.Sprintf("Deleting temporary files: %s", path))
	err := DeleteDirectory(path)
	if err != nil {
		logger.Debug(fmt.Sprintf("Error occurred while deleting %s directory: %v", path, err))
		time.Sleep(time.Second * 1)
		err = DeleteDirectory(path)
		if err != nil {
			logger.Debug(fmt.Sprintf("Retry failed: %v", err))
			PrintInfo(fmt.Sprintf("Deleting '%s' failed. Please delete this directory manually.",
				path))
		} else {
			logger.Debug(fmt.Sprintf("'%s' successfully deleted on retry", path))
			logger.Debug("Temporary files successfully deleted")
		}
	} else {
		logger.Debug(fmt.Sprintf("'%s' successfully deleted", path))
		logger.Debug("Temporary files successfully deleted")
	}
}

// This function handles keyboard interrupts
func HandleInterrupts(cleanupFunc func()) chan<- os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		<-c
		PrintInfo("Keyboard interrupt received.")
		cleanupFunc()
		os.Exit(1)
	}()
	return c
}

// This function will create all directories in the given path if they do not exist
func CreateDirectory(path string) error {
	return os.MkdirAll(path, 0700)
}

// This function will delete all directories in the given path
func DeleteDirectory(path string) error {
	return os.RemoveAll(path)
}

// This function will get user input
func GetUserInput() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	preference, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(preference), nil
}

// This function will process user input and identify the type of preference
func ProcessUserPreference(preference string) int {
	if strings.ToLower(preference) == "yes" || (len(preference) == 1 && strings.ToLower(preference) == "y") {
		return constant.YES
	} else if strings.ToLower(preference) == "no" || (len(preference) == 1 && strings.ToLower(preference) == "n") {
		return constant.NO
	} else if strings.ToLower(preference) == "reenter" || strings.ToLower(preference) == "re-enter" ||
		(len(preference) == 1 && strings.ToLower(preference) == "r") {
		return constant.REENTER
	}
	return constant.OTHER
}

// This function will validate user input in cases of user can enter comma separated values
func IsUserPreferencesValid(preferences []string, noOfAvailableChoices int) (bool, error) {
	length := len(preferences)
	if length == 0 {
		return false, errors.New("No preferences entered.")
	}
	first, err := strconv.Atoi(preferences[0])
	if err != nil {
		return false, err
	}
	message := fmt.Sprintf("Invalid preferences. Please select indices where %s>= index >=1.",
		strconv.Itoa(noOfAvailableChoices))
	if first < 0 {
		return false, errors.New(message)
	}
	last, err := strconv.Atoi(preferences[length-1])
	if err != nil {
		return false, err
	}
	if last > noOfAvailableChoices {
		return false, errors.New(message)
	}
	return true, nil
}

// This function will read update-descriptor.yaml
func LoadUpdateDescriptor(filename, updateDirectoryPath string) (*UpdateDescriptor, error) {
	//Construct the file path
	updateDescriptorPath := filepath.Join(updateDirectoryPath, filename)
	logger.Debug(fmt.Sprintf("updateDescriptorPath: %s", updateDescriptorPath))

	//Read the file
	updateDescriptor := UpdateDescriptor{}
	yamlFile, err := ioutil.ReadFile(updateDescriptorPath)
	if err != nil {
		return nil, err
	}
	//Un-marshal the update-descriptor file to updateDescriptor struct
	err = yaml.Unmarshal(yamlFile, &updateDescriptor)
	if err != nil {
		return nil, err
	}
	logger.Debug(fmt.Sprintf("updateDescriptor: %v", updateDescriptor))
	return &updateDescriptor, nil
}

// This function will validate the update-descriptor.yaml
func ValidateUpdateDescriptor(updateDescriptor *UpdateDescriptor) error {
	if len(updateDescriptor.Update_number) == 0 {
		return errors.New("'update_number' field not found.")
	}
	matches, err := regexp.MatchString(constant.UPDATE_NUMBER_REGEX, updateDescriptor.Update_number)
	if err != nil {
		return err
	}
	if !matches {
		return errors.New(fmt.Sprintf("'update_number' is not valid. It should match '%s'.",
			constant.UPDATE_NUMBER_REGEX))
	}
	if len(updateDescriptor.Platform_version) == 0 {
		return errors.New("'platform_version' field not found.")
	}
	matches, err = regexp.MatchString(constant.KERNEL_VERSION_REGEX, updateDescriptor.Platform_version)
	if err != nil {
		return err
	}
	if !matches {
		return errors.New(fmt.Sprintf("'platform_version' is not valid. It should match '%s'.",
			constant.KERNEL_VERSION_REGEX))
	}
	if len(updateDescriptor.Platform_name) == 0 {
		return errors.New("'platform_name' field not found.")
	}
	if len(updateDescriptor.Applies_to) == 0 {
		return errors.New("'applies_to' field not found.")
	}
	if len(updateDescriptor.Bug_fixes) == 0 {
		return errors.New("'bug_fixes' field not found. Add 'N/A: N/A' if there are no bug fixes.")
	}
	if len(updateDescriptor.Description) == 0 {
		return errors.New("'description' field not found.")
	}
	return nil
}

// Check whether the given string is in the given slice
func IsStringIsInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// Copies file source to destination
func CopyFile(source string, dest string) (err error) {
	logger.Debug(fmt.Sprintf("[CopyFile] Copying %s to %s.", source, dest))
	sf, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sf.Close()
	df, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer df.Close()
	_, err = io.Copy(df, sf)
	if err == nil {
		si, err := os.Stat(source)
		if err != nil {
			return os.Chmod(dest, si.Mode())
		}
	}
	return
}

// Recursively copies a directory tree, attempting to preserve permissions
func CopyDir(source string, dest string) (err error) {
	logger.Debug(fmt.Sprintf("[CopyFile] Copying %s to %s.", source, dest))
	// get properties of source dir
	fi, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return errors.New("Source is not a directory")
	}
	//Create the destination directory if it does not exist
	_, err = os.Open(dest)
	if os.IsNotExist(err) {
		// create dest dir
		err = os.MkdirAll(dest, fi.Mode())
		if err != nil {
			return err
		}
	}
	entries, err := ioutil.ReadDir(source)
	for _, entry := range entries {
		sfp := source + "/" + entry.Name()
		dfp := dest + "/" + entry.Name()
		if entry.IsDir() {
			err = CopyDir(sfp, dfp)
			if err != nil {
				return err
			}
		} else {
			// perform copy
			err = CopyFile(sfp, dfp)
			if err != nil {
				return err
			}
		}
	}
	return
}

// Check whether the given location contains a directory
func IsDirectoryExists(location string) (bool, error) {
	logger.Debug(fmt.Sprintf("Checking %s", location))
	locationInfo, err := os.Stat(location)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debug("Does not exist")
			return false, nil
		} else {
			logger.Debug("Other error")
			return false, err
		}
	}
	if locationInfo.IsDir() {
		logger.Debug("Is a directory")
		return true, nil
	} else {
		logger.Debug("Is not a directory")
		return false, nil
	}
}

// Check whether the given location contains a file
func IsFileExists(location string) (bool, error) {
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

// This function is used to handle errors (print proper error message and exit if an error exists)
func HandleErrorAndExit(err error, customMessage ...interface{}) {
	if err != nil {
		//call the PrintError method and exit
		if len(customMessage) == 0 {
			PrintError(fmt.Sprintf("%s", err.Error()))
		} else {
			PrintError(append(customMessage, err.Error())...)
		}
		os.Exit(1)
	}
}

// This function is used to print error messages
func PrintError(args ...interface{}) {
	color.Set(color.FgRed, color.Bold)
	fmt.Println(append(append([]interface{}{"\n[ERROR]"}, args...), "\n")...)
	color.Unset()
}

// This function is used to print warning messages
func PrintWarning(args ...interface{}) {
	color.Set(color.FgRed, color.Bold)
	fmt.Println(append([]interface{}{"[WARNING]"}, args...)...)
	color.Unset()
}

// This function is used to print info messages
func PrintInfo(args ...interface{}) {
	fmt.Println(append([]interface{}{"[INFO]"}, args...)...)
}

// This function is used to print text in bold
func PrintInBold(args ...interface{}) {
	color.Set(color.Bold)
	fmt.Print(args...)
	color.Unset()
}

// This function will get the Jira summary associated with the given jira id. If an error occur, we just simply ignore
// the error and return the default response.
func GetJiraSummary(id string) string {
	defaultResponse := constant.JIRA_SUMMARY_DEFAULT
	logger.Debug(fmt.Sprintf("Getting Jira summary for: %s", id))
	req, err := http.NewRequest("GET", constant.JIRA_API_URL+id, nil)
	logger.Trace(fmt.Sprintf("Request: %v", req))
	if err != nil {
		logger.Debug(fmt.Sprintf("Error occurred while creating a new request: %v", err))
		return defaultResponse
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Debug(fmt.Sprintf("Error occurred while requesting: %v", err))
		return defaultResponse
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		logger.Debug(fmt.Sprintf("Error occurred while getting response body: %v", err))
		return defaultResponse
	}
	responseBody := string(body)
	logger.Debug(fmt.Sprintf("Response body: %v", responseBody))

	jiraResponse := JiraResponse{}
	err = json.Unmarshal(body, &jiraResponse)
	if err != nil {
		logger.Debug(fmt.Sprintf("Error occurred while unmarshalling json. Error: %v", err))
		return defaultResponse
	}
	logger.Debug(fmt.Sprintf("jiraResponse: %v", jiraResponse))
	if len(jiraResponse.Fields.Summary) > 0 {
		return jiraResponse.Fields.Summary
	}
	logger.Debug("Summary field not found in the jira response")
	return defaultResponse
}

// This function will do the following operations on the provided string.
// 1) Replace \r with \n - Some older files have MAC OS 9 line endings (\r) and this will cause issues when processing
//    these strings using regular expressions.
// 2) Replace \t with four spaces. This is done to prevent ugly encoding in description section in the
//    update-description.yaml file.
// 3) Will remove preceding and trailering spaces if trimAll is true, otherwise it will only remove trailering spaces.
//    This is done to preserve proper formatting in the description section of the update-description.yaml.
// Delimiter is provided from outside so that this function can be used to clean and concat various types of strings.
func ProcessString(data, delimiter string, trimAll bool) string {
	data = strings.TrimSpace(data)
	data = strings.Replace(data, "\r", "\n", -1)
	data = strings.Replace(data, "\t", "    ", -1)
	contains := strings.Contains(data, "\n")
	if !contains {
		return data
	}
	allLines := ""
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		if trimAll {
			allLines = allLines + strings.TrimSpace(line) + delimiter
		} else {
			allLines = allLines + strings.TrimRight(line, " ") + delimiter
		}
	}
	return strings.TrimSuffix(allLines, delimiter)
}

// This function checks whether the given file is a zip file.
// archiveType 		type of the archive
// archiveFilePath	path to the archive file
func IsZipFile(archiveType, archiveFilePath string) {
	if !strings.HasSuffix(archiveFilePath, ".zip") {
		HandleErrorAndExit(errors.New(fmt.Sprintf("%s must be a zip file. Entered file '%s' is not a valid zip file"+
			".", archiveType, archiveFilePath)))
	}
}

// This function will return the relative path of the given file.
// file	file in which the relative path is to be obtained
func GetRelativePath(file *zip.File) (relativePath string) {
	if strings.Contains(file.Name, "/") {
		relativePath = strings.SplitN(file.Name, "/", 2)[1]
	} else {
		relativePath = file.Name
	}
	logger.Trace(fmt.Sprintf("relativePath: %s", relativePath))
	return
}
