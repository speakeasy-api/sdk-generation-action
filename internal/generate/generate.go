package generate

import (
	"fmt"
	"path"

	"github.com/hashicorp/go-version"
	config "github.com/speakeasy-api/sdk-gen-config"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

type LanguageGenInfo struct {
	PackageName string
	Version     string
}

type GenerationInfo struct {
	SpeakeasyVersion  string
	OpenAPIDocVersion string
	Languages         map[string]LanguageGenInfo
}

type Git interface {
	CheckDirDirty(dir string) (bool, error)
}

func Generate(g Git) (*GenerationInfo, map[string]string, error) {
	langs, err := getAndValidateLanguages(environment.GetLanguages())
	if err != nil {
		return nil, nil, err
	}

	docPath, docChecksum, docVersion, err := getOpenAPIFileInfo(environment.GetOpenAPIDocLocation())
	if err != nil {
		return nil, nil, err
	}

	baseDir := environment.GetBaseDir()

	genConfigs, err := loadGeneratorConfigs(baseDir, langs)
	if err != nil {
		return nil, nil, err
	}

	speakeasyVersion, err := cli.GetSpeakeasyVersion()
	if err != nil {
		return nil, nil, err
	}

	langGenerated := map[string]bool{}
	outputs := map[string]string{}

	for lang, cfg := range genConfigs {
		dir := langs[lang]

		langCfg, ok := cfg.Config.Languages[lang]
		if !ok {
			langCfg = config.LanguageConfig{
				Version: "0.0.0",
			}

			cfg.Config.Languages[lang] = langCfg
		}

		if cfg.Config.Management == nil {
			cfg.Config.Management = &config.Management{}
		}

		sdkVersion := langCfg.Version

		newVersion, err := checkForChanges(speakeasyVersion, docVersion, docChecksum, sdkVersion, cfg.Config.Management)
		if err != nil {
			return nil, nil, err
		}

		if newVersion != "" {
			fmt.Println("New version detected: ", newVersion)
			outputDir := path.Join(baseDir, "repo", dir)

			langCfg.Version = newVersion
			cfg.Config.Languages[lang] = langCfg

			if err := config.Save(cfg.ConfigDir, &cfg.Config); err != nil {
				return nil, nil, err
			}

			fmt.Printf("Generating %s SDK in %s\n", lang, outputDir)

			if err := cli.Generate(docPath, lang, outputDir); err != nil {
				return nil, nil, err
			}

			dirForOutput := dir
			if dirForOutput == "" {
				dirForOutput = "."
			}

			outputs[fmt.Sprintf("%s_directory", lang)] = dirForOutput

			dirty, err := g.CheckDirDirty(dir)
			if err != nil {
				return nil, nil, err
			}

			if dirty {
				langGenerated[lang] = true
			} else {
				langCfg.Version = sdkVersion
				cfg.Config.Languages[lang] = langCfg

				if err := config.Save(cfg.ConfigDir, &cfg.Config); err != nil {
					return nil, nil, err
				}

				fmt.Printf("Regenerating %s SDK did not result in any changes\n", lang)
			}
		} else {
			fmt.Println("No changes detected")
		}
	}

	regenerated := false

	langGenInfo := map[string]LanguageGenInfo{}

	for lang, cfg := range genConfigs {
		if langGenerated[lang] {
			outputs[lang+"_regenerated"] = "true"

			mgmtConfig := cfg.Config.Management

			mgmtConfig.SpeakeasyVersion = speakeasyVersion
			mgmtConfig.DocVersion = docVersion
			mgmtConfig.DocChecksum = docChecksum
			cfg.Config.Management = mgmtConfig

			if err := config.Save(cfg.ConfigDir, &cfg.Config); err != nil {
				return nil, nil, err
			}

			langCfg := cfg.Config.Languages[lang]

			switch lang {
			case "java":
				langGenInfo[lang] = LanguageGenInfo{
					PackageName: fmt.Sprintf("%s.%s", langCfg.Cfg["groupID"], langCfg.Cfg["artifactID"]),
					Version:     langCfg.Version,
				}
			default:
				langGenInfo[lang] = LanguageGenInfo{
					PackageName: fmt.Sprintf("%s", langCfg.Cfg["packageName"]),
					Version:     langCfg.Version,
				}
			}

			regenerated = true
		}
	}

	var genInfo *GenerationInfo

	if regenerated {
		genInfo = &GenerationInfo{
			SpeakeasyVersion:  speakeasyVersion,
			OpenAPIDocVersion: docVersion,
			Languages:         langGenInfo,
		}
	}

	return genInfo, outputs, nil
}

func checkForChanges(speakeasyVersion, docVersion, docChecksum, sdkVersion string, mgmtConfig *config.Management) (string, error) {
	force := environment.ForceGeneration()

	if speakeasyVersion != mgmtConfig.SpeakeasyVersion || docVersion != mgmtConfig.DocVersion || docChecksum != mgmtConfig.DocChecksum || force {
		bumpMajor := false
		bumpMinor := false
		bumpPatch := false

		if mgmtConfig.SpeakeasyVersion == "" {
			bumpMinor = true
		} else {
			previousSpeakeasyV, err := version.NewVersion(mgmtConfig.SpeakeasyVersion)
			if err != nil {
				return "", fmt.Errorf("error parsing config speakeasy version: %w", err)
			}

			currentSpeakeasyV, err := version.NewVersion(speakeasyVersion)
			if err != nil {
				return "", fmt.Errorf("error parsing speakeasy version: %w", err)
			}

			if currentSpeakeasyV.Segments()[0] > previousSpeakeasyV.Segments()[0] {
				fmt.Printf("Speakeasy version changed detected: %s > %s\n", mgmtConfig.SpeakeasyVersion, speakeasyVersion)
				bumpMajor = true
			} else if currentSpeakeasyV.Segments()[1] > previousSpeakeasyV.Segments()[1] {
				fmt.Printf("Speakeasy version changed detected: %s > %s\n", mgmtConfig.SpeakeasyVersion, speakeasyVersion)
				bumpMinor = true
			} else if currentSpeakeasyV.Segments()[2] > previousSpeakeasyV.Segments()[2] {
				fmt.Printf("Speakeasy version changed detected: %s > %s\n", mgmtConfig.SpeakeasyVersion, speakeasyVersion)
				bumpPatch = true
			}
		}

		docVersionUpdated := false

		if mgmtConfig.DocVersion == "" {
			bumpMinor = true
		} else {
			currentDocV, err := version.NewVersion(docVersion)
			// If not a semver then we just deal with the checksum
			if err == nil {
				previousDocV, err := version.NewVersion(mgmtConfig.DocVersion)
				if err != nil {
					return "", fmt.Errorf("error parsing config openapi version: %w", err)
				}

				if currentDocV.Segments()[0] > previousDocV.Segments()[0] {
					fmt.Printf("OpenAPI doc version changed detected: %s > %s\n", mgmtConfig.DocVersion, docVersion)
					bumpMajor = true
					docVersionUpdated = true
				} else if currentDocV.Segments()[1] > previousDocV.Segments()[1] {
					fmt.Printf("OpenAPI doc version changed detected: %s > %s\n", mgmtConfig.DocVersion, docVersion)
					bumpMinor = true
					docVersionUpdated = true
				} else if currentDocV.Segments()[2] > previousDocV.Segments()[2] {
					fmt.Printf("OpenAPI doc version changed detected: %s > %s\n", mgmtConfig.DocVersion, docVersion)
					bumpPatch = true
					docVersionUpdated = true
				}
			} else {
				fmt.Println("::warning title=invalid_version::openapi version is not a semver")
			}
		}

		if mgmtConfig.DocChecksum == "" {
			bumpMinor = true
		} else if docChecksum != mgmtConfig.DocChecksum {
			bumpPatch = true

			fmt.Printf("OpenAPI doc checksum changed detected: %s > %s\n", mgmtConfig.DocChecksum, docChecksum)

			if !docVersionUpdated {
				fmt.Println("::warning title=checksum_changed::openapi checksum changed but version did not")
			}
		}

		var major, minor, patch int

		if sdkVersion != "" {
			sdkV, err := version.NewVersion(sdkVersion)
			if err != nil {
				return "", fmt.Errorf("error parsing sdk version: %w", err)
			}

			major = sdkV.Segments()[0]
			minor = sdkV.Segments()[1]
			patch = sdkV.Segments()[2]
		}

		if bumpMajor {
			fmt.Println("Bumping SDK major version")
			major++
			minor = 0
			patch = 0
		} else if bumpMinor {
			fmt.Println("Bumping SDK minor version")
			minor++
			patch = 0
		} else if bumpPatch || environment.ForceGeneration() {
			fmt.Println("Bumping SDK patch version")
			patch++
		}

		return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
	}

	return "", nil
}
