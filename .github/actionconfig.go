package actionconfig

import (
	_ "embed"
	"encoding/json"
	"fmt"
	config "github.com/speakeasy-api/sdk-gen-config"
	"gopkg.in/yaml.v3"
	"strings"
)

const (
	publishIdentifier = "publish_"
	inputConfigKey    = "inputs"
	securityConfigKey = "secrets"
)

var secretConfigFieldToLanguage = map[string]string{
	"npm_token":           "typescript",
	"pypi_token":          "python",
	"packagist_username":  "php",
	"packagist_token":     "php",
	"java_gpg_secret_key": "java",
	"java_gpg_passphrase": "java",
	"ossrh_username":      "java",
	"ossrh_password":      "java",
}

//go:embed workflows/sdk-generation.yaml
var genActionYml string

//go:embed action-inputs-config.json
var actionInputsConfig string

//go:embed action-security-config.json
var actionSecurityConfig string

func GenerateActionInputsConfig() (*config.SdkGenConfig, error) {
	var sdkGenConfig config.SdkGenConfig

	inputConfigFields, err := generateConfigFieldsFromGenAction(false)
	if err != nil {
		return nil, fmt.Errorf("failed to generate action inputs config fields: %w", err)
	}

	for _, inputConfigField := range inputConfigFields {
		if strings.Contains(inputConfigField.Name, publishIdentifier) || inputConfigField.Name == "create_release" {
			inputConfigField.RequiredForPublishing = new(bool)
			*inputConfigField.RequiredForPublishing = true
			if strings.Contains(inputConfigField.Name, publishIdentifier) {
				lang := strings.Split(inputConfigField.Name, "_")[1]
				if sdkGenConfig.SdkGenLanguageConfig == nil {
					sdkGenConfig.SdkGenLanguageConfig = make(map[string][]config.SdkGenConfigField)
				}
				sdkGenConfig.SdkGenLanguageConfig[lang] = append(sdkGenConfig.SdkGenLanguageConfig[lang], *inputConfigField)
			}
		} else {
			sdkGenConfig.SdkGenCommonConfig = append(sdkGenConfig.SdkGenCommonConfig, *inputConfigField)
		}
	}

	return &sdkGenConfig, nil
}

func GenerateActionSecurityConfig() (*config.SdkGenConfig, error) {
	var sdkGenConfig config.SdkGenConfig

	securityConfigFields, err := generateConfigFieldsFromGenAction(true)
	if err != nil {
		return nil, fmt.Errorf("failed to generate action security config fields: %w", err)
	}

	for _, securityConfigField := range securityConfigFields {
		if securityConfigField.Name != "openapi_doc_auth_token" {
			securityConfigField.RequiredForPublishing = new(bool)
			*securityConfigField.RequiredForPublishing = true
		}

		if _, ok := secretConfigFieldToLanguage[securityConfigField.Name]; ok {
			lang := secretConfigFieldToLanguage[securityConfigField.Name]
			if sdkGenConfig.SdkGenLanguageConfig == nil {
				sdkGenConfig.SdkGenLanguageConfig = make(map[string][]config.SdkGenConfigField)
			}
			sdkGenConfig.SdkGenLanguageConfig[lang] = append(sdkGenConfig.SdkGenLanguageConfig[lang], *securityConfigField)
		} else {
			sdkGenConfig.SdkGenCommonConfig = append(sdkGenConfig.SdkGenCommonConfig, *securityConfigField)
		}
	}

	return &sdkGenConfig, nil
}

func generateConfigFieldsFromGenAction(security bool) ([]*config.SdkGenConfigField, error) {
	configKey := inputConfigKey
	configFile := actionInputsConfig

	if security {
		configKey = securityConfigKey
		configFile = actionSecurityConfig
	}

	actionConfigMap := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(genActionYml), &actionConfigMap); err != nil {
		return nil, fmt.Errorf("failed to parse generation action yaml: %w", err)
	}

	var configFields []*config.SdkGenConfigField
	if err := json.Unmarshal([]byte(configFile), &configFields); err != nil {
		return nil, fmt.Errorf("failed to parse action config json: %w", err)
	}

	for configName, configVal := range actionConfigMap["on"].(map[string]interface{})["workflow_call"].(map[string]interface{})[configKey].(map[string]interface{}) {
		sdkGenConfigEntry := &config.SdkGenConfigField{}
		for _, configField := range configFields {
			if configField.Name == configName {
				sdkGenConfigEntry = configField
			}
		}

		for configFieldKey, configFieldVal := range configVal.(map[string]interface{}) {
			switch configFieldKey {
			case "description":
				sdkGenConfigEntry.Description = new(string)
				*sdkGenConfigEntry.Description = configFieldVal.(string)
			case "required":
				sdkGenConfigEntry.Required = configFieldVal.(bool)
			case "default":
				sdkGenConfigEntry.DefaultValue = new(string)
				*sdkGenConfigEntry.DefaultValue = configFieldVal.(string)
			}
		}
	}

	return configFields, nil
}
