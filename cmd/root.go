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
	"os"

	"github.com/ian-kent/go-log/layout"
	"github.com/ian-kent/go-log/levels"
	"github.com/ian-kent/go-log/log"
	"github.com/mitchellh/go-homedir"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wso2/update-creator-tool/constant"
	"github.com/wso2/update-creator-tool/util"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

var (
	Version   string
	BuildDate string
	WUMUCHome string

	//Create the logger
	logger = log.Logger()

	isDebugLogsEnabled = false
	isTraceLogsEnabled = false
)

var cfgFile string

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "wum-uc",
	Short: "This tool is used to create and validate updates",
	Long:  "This tool is used to create and validate updates.",
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(-1)
	}
}

func init() {
	cobra.OnInitialize(setLogLevel, checkPrerequisites, initConfig, checkWUMUCVersion)
}

// This function checks the existence of prerequisite programs needed for running 'wum-uc' tool.
func checkPrerequisites() {
	// Check whether `SVN` is in the system's PATH
	isAvailable, err := isSVNCommandAvailableInPath()
	if isAvailable == false {
		logger.Debug(err)
		util.HandleErrorAndExit(errors.New("svn executable not found in system $PATH, " +
			"please install `svn` before using `wum-uc`."))
	}
}

// This function checks whether `SVN` command is available in host machine.
func isSVNCommandAvailableInPath() (bool, error) {
	SVNPath, err := exec.LookPath(constant.SVN_COMMAND)
	if err != nil {
		return false, err
	}
	logger.Debug(fmt.Sprintf("%s executable found in %s", constant.SVN_COMMAND, SVNPath))
	return true, nil
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// enable ability to specify config file via flag
		viper.SetConfigFile(cfgFile)
	}

	setDefaultValues()

	// Check whether the user has specified the WUM_UC_HOME environment variable.
	WUMUCHome = os.Getenv(constant.WUM_UC_HOME)
	if WUMUCHome == "" {
		// User has not specified WUM_UC_HOME.
		// Get the home directory of the current user.
		homeDirPath, err := homedir.Dir()
		if err != nil {
			util.HandleErrorAndExit(err, "Cannot determine the current user's home directory.")
		}
		WUMUCHome = filepath.Join(homeDirPath, constant.WUMUC_HOME_DIR_NAME)
		logger.Debug(fmt.Sprintf("wum-uc home directory path: %s", WUMUCHome))
		util.SetWUMUCLocalRepo(WUMUCHome)
	}
	viper.Set(constant.WUM_UC_HOME, WUMUCHome)
	util.LoadWUMUCConfig(WUMUCHome)

	viper.SetConfigName("config") // name of config file (without extension)
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.wum-uc")
	//viper.AutomaticEnv()

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		logger.Debug(fmt.Sprintf("Config file found: %v", viper.ConfigFileUsed()))
	} else {
		logger.Debug("Config file not found.")
	}

	logger.Debug(fmt.Sprintf("PATH_SEPARATOR: %s", constant.PATH_SEPARATOR))
	logger.Debug("Config Values: ---------------------------")
	logger.Debug(fmt.Sprintf("%s: %s", constant.CHECK_MD5_DISABLED, viper.GetString(constant.CHECK_MD5_DISABLED)))
	logger.Debug(fmt.Sprintf("%s: %s", constant.RESOURCE_FILES_MANDATORY,
		viper.GetStringSlice(constant.RESOURCE_FILES_MANDATORY)))
	logger.Debug(fmt.Sprintf("%s: %s", constant.RESOURCE_FILES_OPTIONAL,
		viper.GetStringSlice(constant.RESOURCE_FILES_OPTIONAL)))
	logger.Debug(fmt.Sprintf("%s: %s", constant.RESOURCE_FILES_SKIP,
		viper.GetStringSlice(constant.RESOURCE_FILES_SKIP)))
	logger.Debug(fmt.Sprintf("%s: %s", constant.PLATFORM_VERSIONS,
		viper.GetStringMapString(constant.PLATFORM_VERSIONS)))
	logger.Debug("-----------------------------------------")
}

//This function will set the log level
func setLogLevel() {
	//Setting default time format. This will be used in loggers. Otherwise complete date and time will be printed
	layout.DefaultTimeLayout = "15:04:05"
	//Setting new STDOUT layout to logger
	logger.Appender().SetLayout(layout.Pattern("[%d] [%p] %m"))

	//Set the log level. If the log level is not given, set the log level to default level
	if isDebugLogsEnabled {
		logger.SetLevel(levels.DEBUG)
		logger.Debug("Debug logs enabled")
	} else if isTraceLogsEnabled {
		logger.SetLevel(levels.TRACE)
		logger.Trace("Trace logs enabled")
	} else {
		logger.SetLevel(constant.DEFAULT_LOG_LEVEL)
	}
	logger.Debug("[LOG LEVEL]", logger.Level())
}

//This function will set the default values of the configurations
func setDefaultValues() {
	viper.SetDefault(constant.RESOURCE_FILES_MANDATORY, util.ResourceFiles_Mandatory)
	viper.SetDefault(constant.RESOURCE_FILES_OPTIONAL, util.ResourceFiles_Optional)
	viper.SetDefault(constant.RESOURCE_FILES_SKIP, util.ResourceFiles_Skip)
	viper.SetDefault(constant.PLATFORM_VERSIONS, util.PlatformVersions)
}

// This function checks whether the current version of 'wum-uc' still being supported for creating wum updates.
func checkWUMUCVersion() {
	logger.Debug("wum-uc version check started")
	// Check if last update check timestamp is older than one day.
	wumucUpdateTimestampFilePath := filepath.Join(WUMUCHome, constant.WUMUC_CACHE_DIRECTORY, constant.WUMUC_UPDATE_CHECK_TIMESTAMP_FILENAME)
	exists, err := util.IsFileExists(wumucUpdateTimestampFilePath)
	if err != nil {
		logger.Error(fmt.Sprintf("%v error occurred when checking the existance of %s file", err,
			wumucUpdateTimestampFilePath))
		checkWithWUMUCAdmin()
		return
	}
	if !exists {
		logger.Debug(fmt.Sprintf("%s file doesnot exists, hence checking for latest versions of 'wum-uc'",
			wumucUpdateTimestampFilePath))
		checkWithWUMUCAdmin()
	} else {
		// Check whether the last checked timestamp is greater than one day
		data, err := ioutil.ReadFile(wumucUpdateTimestampFilePath)
		if err != nil {
			logger.Error(fmt.Sprintf("%v error occurred when reading the content of %s", err,
				wumucUpdateTimestampFilePath))
			checkWithWUMUCAdmin()
			return
		}
		oldUpdateTimestamp, err := strconv.ParseInt(string(data), 10, 64)
		if err != nil {
			logger.Error(fmt.Sprintf("%v error occurred when parsing the last update checked timestamp %d", err,
				oldUpdateTimestamp))
			checkWithWUMUCAdmin()
			return
		}
		logger.Debug(fmt.Sprintf("last update checked timestamp %d", oldUpdateTimestamp))
		if time.Now().UTC().Sub(time.Unix(oldUpdateTimestamp, 0)).Hours() > constant.WUMUC_UPDATE_CHECK_INTERVAL_IN_HOURS {
			checkWithWUMUCAdmin()
		}
	}
}

/*This function connects with 'wumucadmin' micro service to check whether the current version of 'wum-`
If the current 'wum-uc' version is not supported,
it will print the error and exists with requesting users to migrate to the new version.
If the current version of 'wum-uc' is still being supported, the update creation continues.
*/
func checkWithWUMUCAdmin() {
	apiURL := util.GetWUMUCConfigs().VersionURL + "/" + constant.WUMUCADMIN_API_CONTEXT + "/" + constant.
		VERSION + "/" + Version

	response := util.InvokeGetRequest(apiURL)
	versionResponse := util.VersionResponse{}
	util.ProcessResponseFromServer(response, &versionResponse)
	// Exit if the current version is no longer supported for creating updates
	if !versionResponse.IsCompatible {
		util.HandleErrorAndExit(errors.New(fmt.Sprintf(versionResponse.
			VersionMessage+"\n\t Latest version: %s \n\t Released date: %s\n",
			versionResponse.LatestVersion.Version, versionResponse.LatestVersion.ReleaseDate)))
	}
	// If there is a new version of wum-uc being released
	if len(versionResponse.LatestVersion.Version) != 0 {
		// Print new version details if exists and continue creating the update
		util.PrintInfo(fmt.Sprintf(versionResponse.VersionMessage+"\n\t Latest version: %s \n\t Released date: %s\n",
			versionResponse.LatestVersion.Version, versionResponse.LatestVersion.ReleaseDate))
	}
	// Write the current timestamp to 'wum-uc-update' cache file for future reference
	utcTime := time.Now().UTC().Unix()
	logger.Debug(fmt.Sprintf("Current timestamp  %v", utcTime))
	cacheDirectoryPath := filepath.Join(WUMUCHome, constant.WUMUC_CACHE_DIRECTORY)
	err := util.CreateDirectory(cacheDirectoryPath)
	if err != nil {
		logger.Error(fmt.Sprintf("%v error occured in creating the directory %s for saving %s cache file", err,
			cacheDirectoryPath, constant.WUMUC_UPDATE_CHECK_TIMESTAMP_FILENAME))
	}
	wumucUpdateTimestampFilePath := filepath.Join(cacheDirectoryPath, constant.WUMUC_UPDATE_CHECK_TIMESTAMP_FILENAME)
	err = util.WriteFileToDestination([]byte(strconv.FormatInt(utcTime, 10)), wumucUpdateTimestampFilePath)
	if err != nil {
		logger.Error(fmt.Sprintf("%v error occurred in writing to %s file", err, wumucUpdateTimestampFilePath))
	}
}
