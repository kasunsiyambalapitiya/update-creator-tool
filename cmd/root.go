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
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/wso2/update-creator-tool/constant"
	"github.com/wso2/update-creator-tool/util"
	"path/filepath"
)

var (
	Version   string
	BuildDate string

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
	cobra.OnInitialize(initConfig)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	setLogLevel()
	if cfgFile != "" {
		// enable ability to specify config file via flag
		viper.SetConfigFile(cfgFile)
	}

	setDefaultValues()

	// Check whether the user has specified the WUM_UC_HOME environment variable.
	WUMUCHome := os.Getenv(constant.WUM_UC_HOME)
	if WUMUCHome == "" {
		// User has not specified WUM_UC_HOME.
		// Get the home directory of the current user.
		homeDirPath, err := homedir.Dir()
		if err != nil {
			util.HandleErrorAndExit(err, "Cannot determine the current user's home directory.")
		}
		WUMUCHome = filepath.Join(homeDirPath, constant.WUMUC_HOME_DIR_NAME)
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
