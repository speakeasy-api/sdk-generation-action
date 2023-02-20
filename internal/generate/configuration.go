package generate

import (
	"fmt"
	"path"
	"strings"

	config "github.com/speakeasy-api/sdk-gen-config"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

type genConfig struct {
	ConfigDir string
	Config    config.Config
}

func loadGeneratorConfigs(baseDir string, langConfigs map[string]string) (map[string]*genConfig, error) {
	genConfigs := map[string]*genConfig{}

	sharedCache := map[string]config.Config{}

	for lang, dir := range langConfigs {
		configDir := path.Join(baseDir, "repo", dir)

		if err := cli.ValidateConfig(configDir); err != nil {
			return nil, err
		}

		cfg, ok := sharedCache[configDir]
		if !ok {
			fmt.Println("Loading generator config: ", configDir)

			loaded, err := config.Load(configDir)
			if err != nil {
				return nil, err
			}

			cfg = *loaded
			sharedCache[configDir] = cfg
		}

		genConfig := genConfig{
			ConfigDir: configDir,
			Config:    cfg,
		}

		genConfigs[lang] = &genConfig
	}

	return genConfigs, nil
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

	supportedLangs, err := cli.GetSupportedLanguages()
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
