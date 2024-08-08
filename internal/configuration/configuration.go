package configuration

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

func GetAndValidateLanguages(checkLangSupported bool) (map[string]string, error) {
	languages := environment.GetLanguages()

	languages = strings.ReplaceAll(languages, "\\n", "\n")

	langs := []interface{}{}

	if err := yaml.Unmarshal([]byte(languages), &langs); err != nil {
		return nil, fmt.Errorf("failed to parse languages: %w", err)
	}

	if len(langs) == 0 {
		return nil, fmt.Errorf("no languages provided")
	}

	langCfgs := map[string]string{}

	numConfigs := len(langs)

	for _, l := range langs {
		langCfg, ok := l.(map[string]interface{})
		if ok {
			for l := range langCfg {
				path := langCfg[l].(string)

				langCfgs[l] = filepath.Clean(path)
			}

			continue
		}

		lang, ok := l.(string)
		if ok {
			if numConfigs > 1 {
				langCfgs[lang] = fmt.Sprintf("%s-client-sdk", lang)
			} else {
				langCfgs[lang] = ""
			}
			continue
		}

		return nil, fmt.Errorf("invalid language configuration: %v", l)
	}

	if !checkLangSupported {
		return langCfgs, nil
	}

	supportedLangs := cli.GetSupportedLanguages()

	for l := range langCfgs {
		if l == "docs" {
			return langCfgs, nil
		}

		if !slices.Contains(supportedLangs, l) {
			return nil, fmt.Errorf("unsupported language: %s", l)
		}
	}

	return langCfgs, nil
}
