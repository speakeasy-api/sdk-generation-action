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
)

var securityConfigFieldPrefixToLanguage = map[string]string{
	"npm":       "typescript",
	"pypi":      "python",
	"packagist": "php",
	"java":      "java",
	"ossrh":     "java",
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
			*inputConfigField.RequiredForPublishing = true
			if strings.Contains(inputConfigField.Name, publishIdentifier) {
				lang := strings.Split(inputConfigField.Name, "_")[1]
				sdkGenConfig.SdkGenLanguageConfig[lang] = append(sdkGenConfig.SdkGenLanguageConfig[lang], inputConfigField)
			}
		} else {
			sdkGenConfig.SdkGenCommonConfig = append(sdkGenConfig.SdkGenCommonConfig, inputConfigField)
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
			*securityConfigField.RequiredForPublishing = true
		}
		prefix := strings.Split(securityConfigField.Name, "_")[0]
		if _, ok := securityConfigFieldPrefixToLanguage[prefix]; ok {
			lang := securityConfigFieldPrefixToLanguage[prefix]
			sdkGenConfig.SdkGenLanguageConfig[lang] = append(sdkGenConfig.SdkGenLanguageConfig[lang], securityConfigField)
		} else {
			sdkGenConfig.SdkGenCommonConfig = append(sdkGenConfig.SdkGenCommonConfig, securityConfigField)
		}
	}

	return &sdkGenConfig, nil
}

func generateConfigFieldsFromGenAction(security bool) ([]config.SdkGenConfigField, error) {
	configKey := "inputs"
	configFile := actionInputsConfig

	if security {
		configKey = "security"
		configFile = actionSecurityConfig
	}

	actionConfigMap := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(genActionYml), &actionConfigMap); err != nil {
		return nil, fmt.Errorf("failed to parse generation action yaml: %w", err)
	}

	var configFields []config.SdkGenConfigField

	for _, configVal := range actionConfigMap["on"].(map[string]interface{})["workflow_call"].(map[string]interface{})[configKey].(map[string]interface{}) {
		sdkGenConfigEntry := config.SdkGenConfigField{}
		if err := json.Unmarshal([]byte(configFile), &sdkGenConfigEntry); err != nil {
			return nil, fmt.Errorf("failed to parse action config json: , err: %w", err)
		}

		for configFieldKey, configFieldVal := range configVal.(map[string]interface{}) {
			switch configFieldKey {
			case "description":
				*sdkGenConfigEntry.Description = configFieldVal.(string)
			case "required":
				sdkGenConfigEntry.Required = configFieldVal.(bool)
			case "default":
				*sdkGenConfigEntry.DefaultValue = configFieldVal.(string)
			}
		}
		configFields = append(configFields, sdkGenConfigEntry)
	}

	return configFields, nil
}
