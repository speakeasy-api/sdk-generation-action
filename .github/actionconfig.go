package actionconfig

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	config "github.com/speakeasy-api/sdk-gen-config"
	"gopkg.in/yaml.v3"
)

const (
	publishIdentifier = "publish_"
	inputConfigKey    = "inputs"
	securityConfigKey = "secrets"
	schemaTokenKey    = "openapi_doc_auth_token"
)

//go:embed workflows/sdk-generation.yaml
var genActionYml string

//go:embed action-inputs-config.json
var actionInputsConfig string

//go:embed action-security-config.json
var actionSecurityConfig string

type SDKGenConfig struct {
	SDKGenLanguageConfig map[string][]config.SDKGenConfigField `json:"language_configs"`
	SDKGenCommonConfig   []config.SDKGenConfigField            `json:"common_config"`
}

func GenerateActionInputsConfig() (*SDKGenConfig, error) {
	var sdkGenConfig SDKGenConfig

	inputConfigFields, err := generateConfigFieldsFromGenAction(false)
	if err != nil {
		return nil, fmt.Errorf("failed to generate action inputs config fields: %w", err)
	}

	for _, inputConfigField := range inputConfigFields {
		if strings.Contains(inputConfigField.Name, publishIdentifier) {
			reqForPublishing := true
			inputConfigField.RequiredForPublishing = &reqForPublishing
			if inputConfigField.Language != nil && *inputConfigField.Language != "" {
				lang := *inputConfigField.Language
				if sdkGenConfig.SDKGenLanguageConfig == nil {
					sdkGenConfig.SDKGenLanguageConfig = make(map[string][]config.SDKGenConfigField)
				}
				sdkGenConfig.SDKGenLanguageConfig[lang] = append(sdkGenConfig.SDKGenLanguageConfig[lang], *inputConfigField)
			}
		} else {
			sdkGenConfig.SDKGenCommonConfig = append(sdkGenConfig.SDKGenCommonConfig, *inputConfigField)
		}
	}

	return &sdkGenConfig, nil
}

func GenerateActionSecurityConfig() (*SDKGenConfig, error) {
	var sdkGenConfig SDKGenConfig

	securityConfigFields, err := generateConfigFieldsFromGenAction(true)
	if err != nil {
		return nil, fmt.Errorf("failed to generate action security config fields: %w", err)
	}

	for _, securityConfigField := range securityConfigFields {
		if securityConfigField.Name != schemaTokenKey {
			securityConfigField.RequiredForPublishing = new(bool)
			*securityConfigField.RequiredForPublishing = true
		}

		if securityConfigField.Language != nil && *securityConfigField.Language != "" {
			lang := *securityConfigField.Language
			if sdkGenConfig.SDKGenLanguageConfig == nil {
				sdkGenConfig.SDKGenLanguageConfig = make(map[string][]config.SDKGenConfigField)
			}
			sdkGenConfig.SDKGenLanguageConfig[lang] = append(sdkGenConfig.SDKGenLanguageConfig[lang], *securityConfigField)
		} else {
			sdkGenConfig.SDKGenCommonConfig = append(sdkGenConfig.SDKGenCommonConfig, *securityConfigField)
		}
	}

	return &sdkGenConfig, nil
}

func generateConfigFieldsFromGenAction(security bool) ([]*config.SDKGenConfigField, error) {
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

	var configFields []*config.SDKGenConfigField
	if err := json.Unmarshal([]byte(configFile), &configFields); err != nil {
		return nil, fmt.Errorf("failed to parse action config json: %w", err)
	}

	for configName, configVal := range actionConfigMap["on"].(map[string]interface{})["workflow_call"].(map[string]interface{})[configKey].(map[string]interface{}) {
		sdkGenConfigEntry := &config.SDKGenConfigField{}
		for _, configField := range configFields {
			if configField.Name == configName {
				sdkGenConfigEntry = configField
			}
		}

		for configFieldKey, configFieldVal := range configVal.(map[string]interface{}) {
			switch configFieldKey {
			case "description":
				description := configFieldVal.(string)
				sdkGenConfigEntry.Description = &description
			case "required":
				sdkGenConfigEntry.Required = configFieldVal.(bool)
			case "default":
				var defaultValue any
				if sdkGenConfigEntry.Language != nil && *sdkGenConfigEntry.Language != "" {
					defaultValueBool, err := strconv.ParseBool(configFieldVal.(string))
					if err != nil {
						defaultValue = configFieldVal
					} else {
						defaultValue = defaultValueBool
					}
				} else {
					defaultValue = configFieldVal
				}
				sdkGenConfigEntry.DefaultValue = &defaultValue
			}
		}
	}

	return configFields, nil
}
