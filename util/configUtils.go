/*
 * Copyright (c) 2018, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 *
 */

package util

import (
	"errors"
	"fmt"
	"github.com/wso2/update-creator-tool/constant"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
)

type WUMUCConfig struct {
	Username     string
	URL          string
	TokenURL     string
	AppKey       string
	RefreshToken string
	AccessToken  string
	BasicAuth
}

type BasicAuth struct {
	Username string
	Password []byte
}

var wumucConfig WUMUCConfig
var wumucConfigFilePath string

// Load the wum-uc configuration from the config.yaml file. If the file is does not exists
// create a new config.yaml file and add default values.
// Validate the configuration, if it exists.
func LoadWUMUCConfig(wumucLocalRepo string) *WUMUCConfig {
	wumucConfig = WUMUCConfig{}
	wumucConfigFilePath = filepath.Join(wumucLocalRepo, constant.WUMUC_CONFIG_FILE)
	exists, err := IsFileExists(wumucConfigFilePath)
	if err != nil {
		HandleErrorAndExit(err, fmt.Sprintf("Error occured while reading the %v file", wumucConfigFilePath))
	}
	if !exists {
		logger.Info("Populating config.yaml")
		basicAuth := BasicAuth{
			Username: constant.WUMUC_BASIC_AUTH_USERNAME,
			Password: []byte(constant.WUMUC_BASIC_AUTH_PASSWORD),
		}
		wumucConfig = WUMUCConfig{
			URL:       constant.WUMUC_AUTHENTICATION_URL,
			TokenURL:  constant.WUMUC_AUTHENTICATION_URL + "/" + constant.TOKEN_API_CONTEXT,
			AppKey:    constant.BASE64_ENCODED_CONSUMER_KEY_AND_SECRET,
			BasicAuth: basicAuth,
		}

		// Write the wum-uc configuration to the config file.
		WriteConfigFile(&wumucConfig, wumucConfigFilePath)
		return &wumucConfig
	} else {
		data, err := ioutil.ReadFile(wumucConfigFilePath)
		if err != nil {
			HandleErrorAndExit(err)
		}

		err = yaml.Unmarshal(data, &wumucConfig)
		if err != nil {
			HandleErrorAndExit(err, fmt.Sprintf("unable to load wum-uc configuration from '%v'.", wumucConfigFilePath))
		}

		// Validate config.yaml
		wumucConfig.validate()
		return &wumucConfig
	}
}

// Store the given wum-uc local repository
func SetWUMUCLocalRepo(localRepoPath string) {
	if localRepoPath == "" {
		HandleErrorAndExit(errors.New("local repository path is not available"))
	}

	// Create the local repo, if it does not exists
	if _, err := os.Stat(localRepoPath); err != nil {
		if err := os.MkdirAll(localRepoPath, 0755); err != nil {
			HandleErrorAndExit(errors.New(fmt.Sprintf("error occured in creating wum-uc home directory at %s", localRepoPath)), err)
		}
	}
}

// Write wum-uc configuration to the config file.
func WriteConfigFile(wumucConfig *WUMUCConfig, wumucConfigFilePath string) error {
	logger.Debug(fmt.Sprintf("Writing wum-uc configs to %s file", wumucConfigFilePath))
	data, err := yaml.Marshal(wumucConfig)
	if err != nil {
		return err
	}
	// Open a new file for writing only
	file, err := os.OpenFile(
		wumucConfigFilePath,
		os.O_WRONLY|os.O_TRUNC|os.O_CREATE,
		0600,
	)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	if err != nil {
		return err
	}
	return nil
}

// Validate wum-uc configurations
func (wumucConfig *WUMUCConfig) validate() {
	if wumucConfig.URL == "" {
		HandleErrorAndExit(errors.New("invalid configurations, missing value for URL key"))
	}
	if wumucConfig.TokenURL == "" {
		HandleErrorAndExit(errors.New("invalid configurations, missing value for TokenURL key"))
	}
	if wumucConfig.AppKey == "" {
		HandleErrorAndExit(errors.New("invalid configurations, missing value for AppKey key"))
	}
}

// Returns a pointer to wumuc configuration.
func GetWUMUCConfigs() *WUMUCConfig {
	if &wumucConfig == nil {
		HandleErrorAndExit(errors.New("wum-uc configuration are not available"))
	}
	return &wumucConfig
}
