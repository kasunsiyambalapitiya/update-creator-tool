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
	username     string
	URL          string
	TokenURL     string
	AppKey       string
	RefreshToken string
	AccessToken  string
}

var wumucConfig WUMUCConfig

// Load the wum-uc configuration from the config.yaml file. If the file is does not exists
// create a new config.yaml file and add default values.
// Validate the configuration, if it exists.
func LoadWUMUCConfig(wumucLocalRepo string) *WUMUCConfig {
	wumucConfig = WUMUCConfig{}
	wumucConfigFilePath := filepath.Join(wumucLocalRepo, constant.WUMUC_CONFIG_FILE)
	exists, err := IsFileExists(wumucConfigFilePath)
	if err != nil {
		HandleErrorAndExit(err, fmt.Sprintf("Error occured while reading the %v file", wumucConfigFilePath))
	}
	if !exists {
		logger.Info("Populating config.yaml")
		wumucConfig = WUMUCConfig{
			URL:      constant.WUMUC_AUTHENTICATION_URL,
			TokenURL: constant.WUMUC_AUTHENTICATION_URL + "/" + constant.TOKEN_API_CONTEXT,
			AppKey:   constant.BASE64_ENCODED_CONSUMER_KEY_AND_SECRET,
		}

		// Write the wumuc configuration to the config file.
		WriteConfigFile(&wumucConfig, wumucConfigFilePath)
		return &wumucConfig
	} else {
		data, err := ioutil.ReadFile(wumucConfigFilePath)
		if err != nil {
			HandleErrorAndExit(err)
		}

		err = yaml.Unmarshal(data, &wumucConfig)
		if err != nil {
			HandleErrorAndExit(err, fmt.Sprintf("unable to load wumuc configuration from '%v'.", wumucConfigFilePath))
		}

		// Validate config.yaml
		wumucConfig.validate()
		return &wumucConfig
	}
}

// Todo check the visibility too
func WriteConfigFile(wumucConfig *WUMUCConfig, wumucConfigFilePath string) error {
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

//Todo
func (wumucConfig *WUMUCConfig) validate() {
	if wumucConfig.URL == "" {
		HandleErrorAndExit(errors.New("invalid configurations, missing value for URL element"))
	}
	if wumucConfig.TokenURL == "" {
		HandleErrorAndExit(errors.New("invalid configurations, missing value for TokenURL element"))
	}
	if wumucConfig.AppKey == "" {
		HandleErrorAndExit(errors.New("invalid configurations, missing value for AppKey element"))
	}
}

//Todo
func GetWUMUCConfigs() *WUMUCConfig {
	if &wumucConfig == nil {
		HandleErrorAndExit(errors.New("wum-uc configuration are not available"))
	}
	return &wumucConfig
}
