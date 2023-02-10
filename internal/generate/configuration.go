package generate

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/oleiade/reflections"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

type management struct {
	OpenAPIChecksum  string `yaml:"openapi-checksum"`
	OpenAPIVersion   string `yaml:"openapi-version"`
	SpeakeasyVersion string `yaml:"speakeasy-version"`
}

type langConfig struct {
	Version     string         `yaml:"version"`
	PackageName string         `yaml:"packagename"`
	Cfg         map[string]any `yaml:",inline"`
}

type config struct {
	Management *management    `yaml:"management,omitempty"`
	Go         *langConfig    `yaml:"go,omitempty"`
	Typescript *langConfig    `yaml:"typescript,omitempty"`
	Python     *langConfig    `yaml:"python,omitempty"`
	Java       *langConfig    `yaml:"java,omitempty"`
	PHP        *langConfig    `yaml:"php,omitempty"`
	Cfg        map[string]any `yaml:",inline"`
}

func (c *config) GetLangConfig(lang string) *langConfig {
	field, _ := reflections.GetField(c, strings.Title(lang))
	return field.(*langConfig)
}

func (c *config) SetLangConfig(lang string, cfg *langConfig) {
	_ = reflections.SetField(c, strings.Title(lang), cfg)
}

type genConfig struct {
	ConfigPath string
	Config     config
}

func loadGeneratorConfigs(baseDir string, langConfigs map[string]string) map[string]genConfig {
	genConfigs := map[string]genConfig{}

	sharedCache := map[string]config{}

	for lang, dir := range langConfigs {
		configPath := path.Join(baseDir, "repo", dir, "gen.yaml")

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			configPath = path.Join(baseDir, "repo", "gen.yaml")
		}

		cfg, ok := sharedCache[configPath]
		if !ok {
			fmt.Println("Loading generator config: ", configPath)

			data, err := os.ReadFile(configPath)
			if err == nil {
				_ = yaml.Unmarshal(data, &cfg)
			}

			if cfg.Management == nil {
				cfg.Management = &management{}
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

func writeConfigFile(cfg genConfig) error {
	data, err := yaml.Marshal(cfg.Config)
	if err != nil {
		return fmt.Errorf("error marshaling config: %w", err)
	}

	if err := os.WriteFile(cfg.ConfigPath, data, os.ModePerm); err != nil {
		return fmt.Errorf("error writing config: %w", err)
	}

	return nil
}
