package main

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/invopop/yaml"
	"golang.org/x/exp/slices"
)

type genConfig struct {
	ConfigPath string
	Config     map[string]map[string]string
}

func loadGeneratorConfigs(langConfigs map[string]string) map[string]genConfig {
	genConfigs := map[string]genConfig{}

	sharedCache := map[string]map[string]map[string]string{}

	for lang, dir := range langConfigs {
		configPath := path.Join(baseDir, "repo", dir, "gen.yaml")

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			configPath = path.Join(baseDir, "repo", "gen.yaml")
		}

		cfg, ok := sharedCache[configPath]
		if !ok {
			cfg = map[string]map[string]string{}

			fmt.Println("Loading generator config: ", configPath)

			data, err := os.ReadFile(configPath)
			if err == nil {
				_ = yaml.Unmarshal(data, &cfg)
			}

			if cfg["management"] == nil {
				cfg["management"] = map[string]string{
					"openapi-version":   "",
					"openapi-checksum":  "",
					"speakeasy-version": "",
				}
			}

			sharedCache[configPath] = cfg
		}

		genConfig := genConfig{
			ConfigPath: configPath,
			Config:     cfg,
		}

		genConfigs[lang] = genConfig
	}

	return genConfigs
}

func getSupportedLanguages() ([]string, error) {
	out, err := runSpeakeasyCommand("generate", "sdk", "--help")
	if err != nil {
		return nil, err
	}

	r := regexp.MustCompile(`available options: \[(.*?)\]`)

	langs := r.FindStringSubmatch(strings.TrimSpace(out))[1]

	return strings.Split(langs, ", "), nil
}

func getAndValidateLanguages(languages string) (map[string]string, error) {
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
				langCfgs[l] = langCfg[l].(string)
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

	supportedLangs, err := getSupportedLanguages()
	if err != nil {
		return nil, fmt.Errorf("failed to get supported languages: %w", err)
	}

	for l := range langCfgs {
		if !slices.Contains(supportedLangs, l) {
			return nil, fmt.Errorf("unsupported language: %s", l)
		}
	}

	return langCfgs, nil
}
