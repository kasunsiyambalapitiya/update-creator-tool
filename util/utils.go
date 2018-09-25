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
	"strconv"
	"strings"
	"syscall"
	"time"

	"archive/zip"
	"bytes"
	"github.com/fatih/color"
	"github.com/ian-kent/go-log/log"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/wso2/update-creator-tool/constant"
	"golang.org/x/crypto/ssh/terminal"
	"gopkg.in/yaml.v2"
	"net/url"
	"regexp"
	"sort"
)

var logger = log.Logger()

// struct which is used to read update-descriptor.yaml
type UpdateDescriptorV2 struct {
	UpdateNumber    string            `yaml:"update_number"`
	PlatformVersion string            `yaml:"platform_version"`
	PlatformName    string            `yaml:"platform_name"`
	AppliesTo       string            `yaml:"applies_to"`
	BugFixes        map[string]string `yaml:"bug_fixes"`
	Description     string            `yaml:"description"`
	FileChanges     struct {
		AddedFiles    []string `yaml:"added_files"`
		RemovedFiles  []string `yaml:"removed_files"`
		ModifiedFiles []string `yaml:"modified_files"`
	} `yaml:"file_changes"`
}

// struct which is used to read update-descriptor3.yaml
type UpdateDescriptorV3 struct {
	UpdateNumber                string            `yaml:"update_number"`
	PlatformVersion             string            `yaml:"platform_version"`
	PlatformName                string            `yaml:"platform_name"`
	Md5sum                      string            `yaml:"md5sum"`
	Description                 string            `yaml:"description"`
	Instructions                string            `yaml:"instructions"`
	BugFixes                    map[string]string `yaml:"bug_fixes"`
	CompatibleProducts          []ProductChanges  `yaml:"compatible_products"`
	PartiallyApplicableProducts []ProductChanges  `yaml:"partially_applicable_products"`
}

type ProductChanges struct {
	ProductName    string   `yaml:"product_name"`
	ProductVersion string   `yaml:"product_version"`
	AddedFiles     []string `yaml:"added_files"`
	RemovedFiles   []string `yaml:"removed_files"`
	ModifiedFiles  []string `yaml:"modified_files"`
}

type PartialUpdateFileRequest struct {
	//WUMUCVersion    string   `json:"wum-uc-version"`
	UpdateNumber    string   `json:"update-no"`
	PlatformVersion string   `json:"platform-version"`
	PlatformName    string   `json:"platform-name"`
	AddedFiles      []string `json:"added-files,omitempty"`
	RemovedFiles    []string `json:"removed-files,omitempty"`
	ModifiedFiles   []string `json:"modified-files,omitempty"`
}

type PartialUpdatedFileResponse struct {
	UpdateNumber                string                   `json:"update-no"`
	PlatformVersion             string                   `json:"platform-version"`
	PlatformName                string                   `json:"platform-name"`
	BackwardCompatible          bool                     `json:"backward-compatible"`
	PartiallyApplicableProducts []PartialUpdatedProducts `json:"partially-applicable-products"`
	CompatibleProducts          []PartialUpdatedProducts `json:"compatible-products"`
	NotifyProducts              []PartialUpdatedProducts `json:"notify-products"`
}

type PartialUpdatedProducts struct {
	ProductName   string   `json:"product-name"`
	BaseVersion   string   `json:"base-version"`
	Tag           string   `json:"tag"`
	AddedFiles    []string `json:"added-files"`
	ModifiedFiles []string `json:"modified-files"`
	RemovedFiles  []string `json:"removed-files"`
}

type TokenResponse struct {
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
}

type Version struct {
	Version     string `json:"version"`
	ReleaseDate string `json:"release-date"`
}

type VersionResponse struct {
	Version
	IsCompatible   bool    `json:"is-compatible"`
	VersionMessage string  `json:"version-message"`
	LatestVersion  Version `json:"latest-version,omitempty"`
}

type TokenErrResp struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type ErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
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

// This function is used to delete files.
func CleanUpFile(path string) {
	logger.Debug(fmt.Sprintf("Deleting file %s", path))
	err := os.RemoveAll(path)
	if err != nil {
		logger.Debug(fmt.Sprintf("Error occurred while deleting '%s' file: %v", path, err))
		time.Sleep(time.Second * 1)
		err = os.RemoveAll(path)
		if err != nil {
			logger.Debug(fmt.Sprintf("Retry failed: %v", err))
			PrintInfo(fmt.Sprintf("Deleting '%s' failed. Please delete this file manually.",
				path))
		} else {
			logger.Debug(fmt.Sprintf("'%s' successfully deleted on retry", path))
		}
	}
	logger.Debug(fmt.Sprintf("'%s' successfully deleted", path))
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
	userInput, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(userInput), nil
}

// This function will process user input and identify the type of preference
func ProcessUserPreference(preference string) int {
	if strings.ToLower(preference) == "yes" || (len(preference) == 1 && strings.ToLower(preference) == "y") {
		return constant.YES
	} else if strings.ToLower(preference) == "no" || (len(preference) == 1 && strings.ToLower(preference) == "n") {
		return constant.NO
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
func LoadUpdateDescriptor(filename, updateDirectoryPath string) (*UpdateDescriptorV2, error) {
	//Construct the file path
	updateDescriptorPath := filepath.Join(updateDirectoryPath, filename)
	logger.Debug(fmt.Sprintf("updateDescriptorPath: %s", updateDescriptorPath))

	//Read the file
	updateDescriptor := UpdateDescriptorV2{}
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

// This function will validate the basic details of update-descriptor.yaml.
func ValidateBasicDetailsOfUpdateDescriptorV2(updateDescriptorV2 *UpdateDescriptorV2) error {
	if len(updateDescriptorV2.UpdateNumber) == 0 {
		return errors.New("'update_number' field not found.")
	}
	matches, err := regexp.MatchString(constant.UPDATE_NUMBER_REGEX, updateDescriptorV2.UpdateNumber)
	if err != nil {
		return err
	}
	if !matches {
		return errors.New(fmt.Sprintf("'update_number' is not valid. It should match '%s'.",
			constant.UPDATE_NUMBER_REGEX))
	}
	if len(updateDescriptorV2.PlatformVersion) == 0 {
		return errors.New("'platform_version' field not found.")
	}
	matches, err = regexp.MatchString(constant.KERNEL_VERSION_REGEX, updateDescriptorV2.PlatformVersion)
	if err != nil {
		return err
	}
	if !matches {
		return errors.New(fmt.Sprintf("'platform_version' is not valid. It should match '%s'.",
			constant.KERNEL_VERSION_REGEX))
	}
	if len(updateDescriptorV2.PlatformName) == 0 {
		return errors.New("'platform_name' field not found.")
	}
	return nil
}

func ValidateUpdateDescriptorV2(updateDescriptorV2 *UpdateDescriptorV2) error {
	ValidateBasicDetailsOfUpdateDescriptorV2(updateDescriptorV2)

	if len(updateDescriptorV2.AppliesTo) == 0 {
		return errors.New("'applies_to' field not found.")
	}
	if len(updateDescriptorV2.BugFixes) == 0 {
		return errors.New("'bug_fixes' field not found. Add 'N/A: N/A' if there are no bug fixes.")
	}
	if len(updateDescriptorV2.Description) == 0 {
		return errors.New("'description' field not found.")
	}
	return nil
}

// Validate the given update number with regex
func ValidateUpdateNumber(updateNumber string) bool {
	regex, err := regexp.Compile(constant.UPDATE_NUMBER_REGEX)
	if err != nil {
		HandleErrorAndExit(err)
	}
	return regex.MatchString(updateNumber)
}

// Validate the given platform version with regex
func ValidatePlatformVersion(platformVersion string) bool {
	regex, err := regexp.Compile(constant.KERNEL_VERSION_REGEX)
	if err != nil {
		HandleErrorAndExit(err)
	}
	return regex.MatchString(platformVersion)
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

func ValidateUpdateDescriptorV3(updateDescriptorV3 *UpdateDescriptorV3) error {
	if len(updateDescriptorV3.UpdateNumber) == 0 {
		return errors.New("'update_number' field not found.")
	}
	matches, err := regexp.MatchString(constant.UPDATE_NUMBER_REGEX, updateDescriptorV3.UpdateNumber)
	if err != nil {
		return err
	}
	if !matches {
		return errors.New(fmt.Sprintf("'update_number' is not valid. It should match '%s'.",
			constant.UPDATE_NUMBER_REGEX))
	}
	if len(updateDescriptorV3.PlatformVersion) == 0 {
		return errors.New("'platform_version' field not found.")
	}
	matches, err = regexp.MatchString(constant.KERNEL_VERSION_REGEX, updateDescriptorV3.PlatformVersion)
	if err != nil {
		return err
	}
	if !matches {
		return errors.New(fmt.Sprintf("'platform_version' is not valid. It should match '%s'.",
			constant.KERNEL_VERSION_REGEX))
	}
	if len(updateDescriptorV3.PlatformName) == 0 {
		return errors.New("'platform_name' field not found.")
	}

	// Generate md5sum for the content generated by wum-uc tool
	md5sum := GenerateMd5sumForGeneratedContent(updateDescriptorV3)
	if md5sum != updateDescriptorV3.Md5sum {
		HandleErrorAndExit(errors.New("Detected a change in added, " +
			"modified and removed files in compatible_products/applicable_products sections, " +
			"please recreate the update zip using `wum-uc create` command"))
	}
	isRequestedChangesMade(updateDescriptorV3)
	return nil
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

// This function is used to print error messages with a tab
func PrintErrorWithTab(args ...interface{}) {
	color.Set(color.FgRed, color.Bold)
	fmt.Println(append(append([]interface{}{"\n\t[ERROR]"}, args...), "\n")...)
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

// Download a file from given url to the given location.
func DownloadFile(file, url string) error {
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		return errors.New(fmt.Sprintf("Could not download the file from: %s", url))
	}
	// Create the file
	out, err := os.Create(file)
	if err != nil {
		return err
	}
	defer out.Close()
	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}
	return nil
}

// Download the content from given url as a byte array.
func GetContentFromUrl(url string) ([]byte, error) {
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return []byte{}, err
	}
	defer resp.Body.Close()
	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		logger.Debug(fmt.Sprintf("Could not download the file from: %s", url))
		return []byte{}, errors.New(fmt.Sprintf("Could not download the file from: %s", url))
	}
	// Read content
	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, err
	}
	return respBytes, nil
}

func createPartialUpdateFileRequest(updateDescriptorV2 *UpdateDescriptorV2) *PartialUpdateFileRequest {
	partialUpdateFileRequest := PartialUpdateFileRequest{}
	//partialUpdateFileRequest.WUMUCVersion = cmd.Version
	partialUpdateFileRequest.UpdateNumber = updateDescriptorV2.UpdateNumber
	partialUpdateFileRequest.PlatformName = updateDescriptorV2.PlatformName
	partialUpdateFileRequest.PlatformVersion = updateDescriptorV2.PlatformVersion
	if updateDescriptorV2.FileChanges.AddedFiles != nil {
		partialUpdateFileRequest.AddedFiles = updateDescriptorV2.FileChanges.AddedFiles
	}
	if updateDescriptorV2.FileChanges.ModifiedFiles != nil {
		partialUpdateFileRequest.ModifiedFiles = updateDescriptorV2.FileChanges.ModifiedFiles
	}
	if updateDescriptorV2.FileChanges.RemovedFiles != nil {
		partialUpdateFileRequest.RemovedFiles = updateDescriptorV2.FileChanges.RemovedFiles
	}
	return &partialUpdateFileRequest
}

// Used for receiving partial updates for the identified file changes
func GetPartialUpdatedFiles(updateDescriptorV2 *UpdateDescriptorV2) *PartialUpdatedFileResponse {
	// Create partial update request
	partialUpdateFileRequest := createPartialUpdateFileRequest(updateDescriptorV2)
	requestBody := new(bytes.Buffer)
	if err := json.NewEncoder(requestBody).Encode(partialUpdateFileRequest); err != nil {
		HandleErrorAndExit(err)
	}
	logger.Debug(fmt.Sprintf("Reqeust sent: %v", requestBody))
	// Invoke the API
	// Todo uncomment before production
	apiURL := GetWUMUCConfigs().URL + "/" + constant.PRODUCT_API_CONTEXT + "/" + constant.
		PRODUCT_API_VERSION + "/" + constant.APPLICABLE_PRODUCTS + "?" + constant.FILE_LIST_ONLY
	response := InvokePOSTRequest(apiURL, requestBody)
	if response.StatusCode != http.StatusOK {
		HandleUnableToConnectErrorAndExit(nil)
	}
	partialUpdatedFileResponse := PartialUpdatedFileResponse{}
	ProcessResponseFromServer(response, &partialUpdatedFileResponse)
	return &partialUpdatedFileResponse
}

// Used to invoke POST request with access tokens.
func InvokePOSTRequest(url string, body io.Reader) *http.Response {
	request, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		HandleUnableToConnectErrorAndExit(err)
	}
	wumucConfig := GetWUMUCConfigs()
	request.Header.Add(constant.HEADER_AUTHORIZATION, "Bearer "+wumucConfig.AccessToken)
	request.Header.Add(constant.HEADER_CONTENT_TYPE, constant.HEADER_VALUE_APPLICATION_JSON)
	return makeAPICall(request, false)
}

// Used to invoke GET request with basicAuth
func InvokeGetRequest(url string) *http.Response {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		HandleUnableToConnectErrorAndExit(err)
	}
	wumucConfig := GetWUMUCConfigs()
	request.SetBasicAuth(wumucConfig.BasicAuth.Username, string(wumucConfig.BasicAuth.Password))
	return makeAPICall(request, true)
}

func HandleUnableToConnectErrorAndExit(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "wum-uc: %v\n", "unable to connect to WUM servers")
		logger.Error(err.Error())
	}
	fmt.Fprintf(os.Stderr, "wum-uc: %v\n", constant.UNABLE_TO_CONNECT_WUM_SERVERS)
	os.Exit(1)
}

func makeAPICall(request *http.Request, isBasicAuth bool) *http.Response {
	var readerCloser1, readerCloser2 io.ReadCloser
	if !isBasicAuth {
		// Getting a copy of the request body for use when access token is renewed
		buf, _ := ioutil.ReadAll(request.Body)
		readerCloser1 = ioutil.NopCloser(bytes.NewBuffer(buf))
		readerCloser2 = ioutil.NopCloser(bytes.NewBuffer(buf))
		// Invoke request
		request.Body = readerCloser1
	}

	timeout := time.Duration(constant.WUMUC_API_CALL_TIMEOUT * time.Minute)
	httpResponse := invokeRequest(request, timeout)

	// When authorization is token based and the status codes are 400 or 401 we need to renew the access token
	if !isBasicAuth && (httpResponse.StatusCode == http.StatusBadRequest || httpResponse.StatusCode == http.
		StatusUnauthorized) {
		// Expired access token. Renew the access token and update config.yaml. If the refresh token is
		// invalid, Authenticate() will notify and exit.
		Authenticate()
		// Load the updated config.yaml with new access token
		wumucConfig := LoadWUMUCConfig(viper.GetString(constant.WUM_UC_HOME))
		fmt.Println("Retrying request with renewed Access Token...")
		// Setting the new access token for backed up request
		request.Header.Set(constant.HEADER_AUTHORIZATION, "Bearer "+wumucConfig.AccessToken)
		// Setting the request body backed up
		request.Body = readerCloser2
		return invokeRequest(request, timeout)
	}
	return httpResponse
}

// Invoke the client request and handle error scenarios
func invokeRequest(request *http.Request, timeout time.Duration) *http.Response {
	response := SendRequest(request, timeout)
	logger.Debug("Status code %v", response.StatusCode)
	handleErrorResponses(response)
	return response
}

// Send the HTTP request to the server. This does not handle any error scenarios
func SendRequest(request *http.Request, timeout time.Duration) *http.Response {
	client := &http.Client{
		Timeout: timeout,
	}
	response, err := client.Do(request)
	if err != nil {
		// Here we need to print the exact error to the console. A non-2xx response doesn't cause an error.
		// This method throws errors when the user doesn't have internet connectivity or there is an issue
		// with the token URL or for timeout errors.
		HandleUnableToConnectErrorAndExit(err)
	}
	return response
}

// Handle HTTP Status Codes of the Response
// Notify and return if 401 or 404
// Fail and exit if not 200, 201, or 202
func handleErrorResponses(response *http.Response) {
	if response.StatusCode == http.StatusTooManyRequests {
		HandleErrorAndExit(errors.New(constant.TOO_MANY_REQUESTS_ERROR_MSG + constant.CONTINUED_ERROR_REPORT_MSG))
	}

	if response.StatusCode == http.StatusInternalServerError {
		HandleUnableToConnectErrorAndExit(nil)
	}

	if response.StatusCode == http.StatusForbidden {
		return
	}

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusBadRequest {
		fmt.Println(fmt.Sprintf("wum-uc: %v", constant.INVALID_EXPIRED_REFRESH_TOKEN_MSG))
		return
	}

	if response.StatusCode == http.StatusNotFound {
		fmt.Println("wum-uc: resource not found")
		errorResponse := ErrorResponse{}
		ProcessResponseFromServer(response, &errorResponse)
		HandleErrorAndExit(errors.New(errorResponse.Error.Message))
	}

	if response.StatusCode == http.StatusConflict {
		fmt.Println("wum-uc: conflict")
		errorResponse := ErrorResponse{}
		ProcessResponseFromServer(response, &errorResponse)
		HandleErrorAndExit(errors.New(errorResponse.Error.Message))
	}

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated &&
		response.StatusCode != http.StatusAccepted {
		HandleUnableToConnectErrorAndExit(nil)
	}
}

// Get an access token from WSO2 Update with the given username and the password using the
// 'password' grant type of Oauth2.
// This method returns an error only if the username or password is incorrect.
func GetAccessToken(username string, password []byte, wumucConfig *WUMUCConfig, scope string) (*TokenResponse, error) {
	payload := url.Values{}
	payload.Add("grant_type", "password")
	payload.Add("username", username)
	payload.Add("password", string(password))

	if len(scope) > 0 {
		payload.Add("scope", scope)
	}
	// Get an access token and a refresh token
	fmt.Fprintln(os.Stderr, "Authenticating...")
	return InvokeTokenAPI(&payload, wumucConfig, constant.RETRIEVE_ACCESS_TOKEN)
}

// Renew access token and persist in the config.yaml. Token API sends a new pair of an access token and a refresh token.
func Authenticate() {
	wumucConfig := GetWUMUCConfigs()

	// Refresh token cannot be empty.
	if wumucConfig.RefreshToken == "" {
		HandleErrorAndExit(errors.New(constant.YOU_HAVENT_INITIALIZED_WUMUC_YET_MSG + " " + constant.
			RUN_WUMUC_INIT_TO_CONTINUE_MSG))
	}

	tokenResponse, err := RenewAccessToken(wumucConfig)
	if err != nil {
		HandleErrorAndExit(err)
	}

	wumucConfig.RefreshToken = tokenResponse.RefreshToken
	wumucConfig.AccessToken = tokenResponse.AccessToken

	WriteConfigFile(wumucConfig, wumucConfigFilePath)
}

func RenewAccessToken(wumucConfig *WUMUCConfig) (*TokenResponse, error) {
	payload := url.Values{}
	payload.Add("grant_type", "refresh_token")
	payload.Add("refresh_token", wumucConfig.RefreshToken)
	// Invoke APIM token API
	return InvokeTokenAPI(&payload, wumucConfig, constant.RENEW_REFRESH_TOKEN)

}

// Invokes the configured token API of the API gateway. This method can be used to get access tokens
// as well as renew access tokens using the refresh token.
func InvokeTokenAPI(payload *url.Values, wumucConfig *WUMUCConfig, tokenType string) (*TokenResponse, error) {
	request, err := http.NewRequest(http.MethodPost, wumucConfig.TokenURL, bytes.NewBufferString(payload.Encode()))
	if err != nil {
		HandleUnableToConnectErrorAndExit(err)
	}
	request.Header.Add(constant.HEADER_AUTHORIZATION, "Basic "+wumucConfig.AppKey)
	request.Header.Add(constant.HEADER_CONTENT_TYPE, constant.HEADER_VALUE_X_WWW_FORM_URLENCODED)

	response := SendRequest(request, time.Duration(constant.WUMUC_UPDATE_TOKEN_TIMEOUT*time.Minute))
	logger.Debug("Response status code %d\n", response.StatusCode)

	tokenResponse := TokenResponse{}
	if response.StatusCode != http.StatusAccepted && response.StatusCode != http.StatusOK {
		tokenErrorResponse := TokenErrResp{}
		ProcessResponseFromServer(response, &tokenErrorResponse)

		if constant.RETRIEVE_ACCESS_TOKEN == tokenType && http.StatusBadRequest == response.StatusCode && constant.
			INVALID_GRANT == tokenErrorResponse.Error {
			return &tokenResponse, errors.New("Invalid Credentials.")
		} else if constant.RENEW_REFRESH_TOKEN == tokenType && http.StatusBadRequest == response.StatusCode && constant.
			INVALID_GRANT == tokenErrorResponse.Error {
			return &tokenResponse, errors.New("Your session has timed out, run 'wum-uc init' to continue")
		} else {
			HandleUnableToConnectErrorAndExit(errors.New(tokenErrorResponse.Error + ":" + tokenErrorResponse.
				ErrorDescription))
		}
	}
	ProcessResponseFromServer(response, &tokenResponse)
	return &tokenResponse, nil
}

// Used to unmarshal the given json response to the provided struct
func ProcessResponseFromServer(response *http.Response, v interface{}) {
	defer response.Body.Close()
	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		logger.Error(constant.ERROR_READING_RESPONSE_MSG)
		HandleErrorAndExit(err)
	}
	logger.Debug(fmt.Sprintf("Response received: %v", string(data)))
	if err = json.Unmarshal(data, v); err != nil {
		logger.Error(constant.ERROR_READING_RESPONSE_MSG)
		HandleErrorAndExit(err)
	}
}

// Initialize wum-uc with WSO2 credentials. If both the username and the password are specified,
// then use them to get an access token.
// If only the username is specified, prompt for the password.
// If both of the username and the password are not specified, then prompt for username and
// the password from the user. User can attempt to give credentials three times.
func Init(username string, password []byte) {
	logger.Debug("Initializing wum-uc with user's WSO2 Credentials")

	// Get WUMUC configurations
	wumucConfig := GetWUMUCConfigs()
	var tokenResponse *TokenResponse
	var err error
	// If user has supplied both username and password by -u and -p flags
	if username != "" && len(password) != 0 {
		// Validate email address
		if !isValidateEmailAddress(username) {
			HandleErrorAndExit(errors.New(constant.INVALID_EMAIL_ADDRESS))
		}
		tokenResponse, err = GetAccessToken(username, password, wumucConfig, "")
		if err != nil {
			HandleUnableToConnectErrorAndExit(errors.New("Invalid Credentials. " +
				"Please enter valid WSO2 credentials to continue"))
		}
	} else if len(password) == 0 {
		if username == "" {
			username = wumucConfig.Username
		}
		username, tokenResponse = getAccessTokenFromUserCreds(username, 1, wumucConfig)
	} else {
		HandleUnableToConnectErrorAndExit(errors.New(constant.USERNAME_PASSWORD_EMPTY_MSG))
	}
	WUMUCHomePath := viper.GetString(constant.WUM_UC_HOME)
	wumucConfig.Username = username
	wumucConfig.RefreshToken = tokenResponse.RefreshToken
	wumucConfig.AccessToken = tokenResponse.AccessToken
	WriteConfigFile(wumucConfig, filepath.Join(WUMUCHomePath, constant.WUMUC_CONFIG_FILE))
}

// Get credentials from the user. Maximum password attempts is 3. If the user specify both the
// username and the password, then get an access token.
func getAccessTokenFromUserCreds(username string, attempt int, wumucConfig *WUMUCConfig) (string, *TokenResponse) {
	validEmail, username, password := getCredentials(username)
	// Handle empty and invalid inputs from user for both username and password
	if (!validEmail || len(password) == 0) && attempt < 3 {
		fmt.Fprintln(os.Stderr)
		return getAccessTokenFromUserCreds("", attempt+1, wumucConfig)
	} else if (!validEmail || len(password) == 0) && attempt == 3 {
		HandleUnableToConnectErrorAndExit(errors.New("Invalid Credentials. " +
			"Please enter your WSO2 credentials to continue"))
	}

	// Handle non-empty inputs from user for both username and password
	tokenResponse, err := GetAccessToken(username, password, wumucConfig, "")
	if err != nil && attempt < 3 {
		// Authentication failure
		fmt.Fprintln(os.Stderr)
		PrintError(err)
		return getAccessTokenFromUserCreds(username, attempt+1, wumucConfig)

	} else if err != nil && attempt == 3 {
		HandleUnableToConnectErrorAndExit(errors.New("Invalid Credentials. " +
			"Please enter your WSO2 credentials to continue"))
	}
	return username, tokenResponse
}

// Prompt for the username and the password from the user.
func getCredentials(username string) (bool, string, []byte) {
	var password []byte
	fmt.Fprintln(os.Stderr, constant.ENTER_YOUR_CREDENTIALS_MSG)

	if username == "" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Email: ")
		uName, err := reader.ReadString('\n')
		if err != nil {
			HandleErrorAndExit(err, constant.UNABLE_TO_READ_YOUR_INPUT_MSG)
		}
		username = strings.TrimSpace(uName)
		// Validate email address
		validEmail := isValidateEmailAddress(username)
		if !validEmail {
			fmt.Fprintln(os.Stderr, constant.INVALID_EMAIL_ADDRESS)
			return validEmail, "", password
		}
	}
	// Validate email address received from user input with -u flag
	validEmail := isValidateEmailAddress(username)
	if !validEmail {
		fmt.Fprintln(os.Stderr, constant.INVALID_EMAIL_ADDRESS)
		return validEmail, "", password
	}

	fmt.Fprintf(os.Stderr, "Password for '%v': ", strings.TrimSpace(username))
	password, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		HandleErrorAndExit(err, constant.UNABLE_TO_READ_YOUR_INPUT_MSG)
	}
	fmt.Fprintln(os.Stderr)
	// As email already validated
	return true, username, password
}

// Used to generate the md5sum required in validating the update-descriptor3.yaml for identifying whether the developer
// has edited beyond what he/she has to edit.
func GenerateMd5sumForGeneratedContent(updateDescriptorV3 *UpdateDescriptorV3) string {
	var buffer bytes.Buffer
	var addedFileString string
	var modifiedFileString string
	var removedFileString string

	// Sorting the product changes of update-descriptor3.yaml in ascending order on product names and versions
	compatibleProductChangesMap := make(map[string]ProductChanges)
	for _, productChange := range updateDescriptorV3.CompatibleProducts {
		var buffer bytes.Buffer
		buffer.WriteString(productChange.ProductName)
		buffer.WriteString("-")
		buffer.WriteString(productChange.ProductVersion)
		compatibleProductChangesMap[buffer.String()] = productChange
	}

	// Get productIds of compatibleProductChangesMap for sorting
	var sortedCompatibleProducts []string
	for productId := range compatibleProductChangesMap {
		sortedCompatibleProducts = append(sortedCompatibleProducts, productId)
	}

	// Sorting the compatible products list on product name and version wise
	sort.Slice(sortedCompatibleProducts, func(i, j int) bool {
		productId1 := strings.Split(sortedCompatibleProducts[i], "-")
		productId2 := strings.Split(sortedCompatibleProducts[j], "-")
		// If product names are different
		if productId1[0] != productId2[0] {
			return sortedCompatibleProducts[i] < sortedCompatibleProducts[j]
		} else {
			// Product names are same, check for their version when comparing
			productVersion1 := strings.Split(productId1[1], ".")
			productVersion2 := strings.Split(productId2[1], ".")

			for k := 0; k < len(productVersion1); k++ {
				if productVersion1[k] != productVersion2[k] {
					return sortedCompatibleProducts[i] < sortedCompatibleProducts[j]
				}
			}
		}
		return sortedCompatibleProducts[i] < sortedCompatibleProducts[j]
	})
	logger.Debug("Sorted compatible products list: ", sortedCompatibleProducts)

	partiallyApplicableProductChangesMap := make(map[string]ProductChanges)
	for _, productChange := range updateDescriptorV3.PartiallyApplicableProducts {
		var buffer bytes.Buffer
		buffer.WriteString(productChange.ProductName)
		buffer.WriteString("-")
		buffer.WriteString(productChange.ProductVersion)
		partiallyApplicableProductChangesMap[buffer.String()] = productChange
	}

	// Get productIds of partiallyApplicableProductChangesMap for sorting
	var sortedPartialApplicableProducts []string
	for productId := range partiallyApplicableProductChangesMap {
		sortedPartialApplicableProducts = append(sortedPartialApplicableProducts, productId)
	}

	// Sorting the partially applicable products list on product name and version wise
	sort.Slice(sortedPartialApplicableProducts, func(i, j int) bool {
		productId1 := strings.Split(sortedPartialApplicableProducts[i], "-")
		productId2 := strings.Split(sortedPartialApplicableProducts[j], "-")
		// If product names are different
		if productId1[0] != productId2[0] {
			return sortedPartialApplicableProducts[i] < sortedPartialApplicableProducts[j]
		} else {
			// Product names are same, check for their version when comparing
			productVersion1 := strings.Split(productId1[1], ".")
			productVersion2 := strings.Split(productId2[1], ".")

			for k := 0; k < len(productVersion1); k++ {
				if productVersion1[k] != productVersion2[k] {
					return sortedPartialApplicableProducts[i] < sortedPartialApplicableProducts[j]
				}
			}
		}
		return sortedPartialApplicableProducts[i] < sortedPartialApplicableProducts[j]
	})
	logger.Debug("Sorted partially applicable products list: ", sortedPartialApplicableProducts)

	// Appending product changes of compatible products to buffer
	var tempCompatibleProducts []ProductChanges
	for _, productId := range sortedCompatibleProducts {
		addedFileString = strings.Join(compatibleProductChangesMap[productId].AddedFiles, ",")
		modifiedFileString = strings.Join(compatibleProductChangesMap[productId].ModifiedFiles, ",")
		removedFileString = strings.Join(compatibleProductChangesMap[productId].RemovedFiles, ",")
		buffer.WriteString(addedFileString)
		buffer.WriteString(modifiedFileString)
		buffer.WriteString(removedFileString)
		buffer.WriteString(compatibleProductChangesMap[productId].ProductName)
		buffer.WriteString(compatibleProductChangesMap[productId].ProductVersion)
		tempCompatibleProducts = append(tempCompatibleProducts, compatibleProductChangesMap[productId])
	}

	// Replacing compatible products in updateDescriptorV3 with sorted product names and versions
	updateDescriptorV3.CompatibleProducts = tempCompatibleProducts

	// Appending product changes of partially updated products to buffer
	var tempPartiallyApplicableProducts []ProductChanges
	for _, productId := range sortedPartialApplicableProducts {
		addedFileString = strings.Join(partiallyApplicableProductChangesMap[productId].AddedFiles, ",")
		modifiedFileString = strings.Join(partiallyApplicableProductChangesMap[productId].ModifiedFiles, ",")
		removedFileString = strings.Join(partiallyApplicableProductChangesMap[productId].RemovedFiles, ",")
		buffer.WriteString(addedFileString)
		buffer.WriteString(modifiedFileString)
		buffer.WriteString(removedFileString)
		buffer.WriteString(partiallyApplicableProductChangesMap[productId].ProductName)
		buffer.WriteString(partiallyApplicableProductChangesMap[productId].ProductVersion)
		tempPartiallyApplicableProducts = append(tempPartiallyApplicableProducts, partiallyApplicableProductChangesMap[productId])
	}

	// Replacing partially updated products in updateDescriptorV3 with sorted product names and versions
	updateDescriptorV3.PartiallyApplicableProducts = tempPartiallyApplicableProducts

	// Appending the update_no, platform_version and platform_name to the buffer
	buffer.WriteString(updateDescriptorV3.UpdateNumber)
	buffer.WriteString(updateDescriptorV3.PlatformVersion)
	buffer.WriteString(updateDescriptorV3.PlatformName)

	return fmt.Sprintf("%x", md5.Sum(buffer.Bytes()))
}

// Check whether user has filled requested information after update-descriptor3.yaml is been created
func isRequestedChangesMade(updateDescriptorV3 *UpdateDescriptorV3) bool {
	// Check if relevant fields are empty
	if len(updateDescriptorV3.Description) == 0 {
		HandleErrorAndExit(errors.New(fmt.Sprintf(
			"value for description key in update-descriptor3.yaml is empty.")))
	}
	if len(updateDescriptorV3.BugFixes) == 0 {
		HandleErrorAndExit(errors.New(fmt.Sprintf(
			"value for bug_fixes key in update-descriptor3.yaml is empty.")))
	}
	// Check if relevant fields contain the default value generated in update creation
	if updateDescriptorV3.Description == constant.DEFAULT_DESCRIPTION {
		HandleErrorAndExit(errors.New(fmt.Sprintf(
			"value for description key in update-descriptor3.yaml contains the default value. " +
				"Enter a valid description")))
	}
	if updateDescriptorV3.Instructions == constant.DEFAULT_INSTRUCTIONS {
		HandleErrorAndExit(errors.New(fmt.Sprintf(
			"value for intructions key in update-descriptor3.yaml contains the default value. " +
				"Enter either valid instructions or leave a blank.")))
	}
	_, exists := updateDescriptorV3.BugFixes[constant.DEFAULT_JIRA_KEY]
	if exists {
		HandleErrorAndExit(errors.New(fmt.Sprintf(
			"value for bug_fixes key in update-descriptor3.yaml contains the default value.")))
	}
	return true
}

func isValidateEmailAddress(username string) bool {
	regex, err := regexp.Compile(constant.EMAIL_ADDRESS_REGEX)
	if err != nil {
		HandleErrorAndExit(err)
	}
	return regex.MatchString(username)
}

// Write the content of given update descriptor passed as a byte array to the destination file.
func WriteUpdateDescriptorInDestination(data []byte, filePath, destination string) string {
	err := WriteFileToDestination(data, filePath)
	if err != nil {
		HandleErrorAndExit(err, fmt.Sprintf("error occurred in writing to %s file", filePath))
	}
	// Get the absolute location
	absDestination, err := filepath.Abs(destination)
	if err != nil {
		absDestination = destination
	}
	return absDestination
}

// Write the content passed as byte array to the destination file.
func WriteFileToDestination(data []byte, filePath string) error {
	file, err := os.OpenFile(
		filePath,
		os.O_WRONLY|os.O_TRUNC|os.O_CREATE,
		0600,
	)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write bytes to file
	_, err = file.Write(data)
	if err != nil {
		return err
	}
	logger.Trace(fmt.Sprintf("Writing content to %s completed successfully", filePath))
	return nil
}
