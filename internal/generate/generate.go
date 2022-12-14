package generate

import (
	"fmt"
	"path"

	"github.com/hashicorp/go-version"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

type GenerationInfo struct {
	SpeakeasyVersion  string
	OpenAPIDocVersion string
	ReleaseVersion    string
	PackageNames      map[string]string
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

	genConfigs := loadGeneratorConfigs(baseDir, langs)

	speakeasyVersion, err := cli.GetSpeakeasyVersion()
	if err != nil {
		return nil, nil, err
	}

	langGenerated := map[string]bool{}
	outputs := map[string]string{}

	for lang, cfg := range genConfigs {
		dir := langs[lang]

		var langCfg *langConfig

		langCfg = cfg.Config.GetLangConfig(lang)
		if langCfg == nil {
			langCfg = &langConfig{
				Version: "0.0.0",
			}
			cfg.Config.SetLangConfig(lang, langCfg)
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
			if err := writeConfigFile(cfg); err != nil {
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
				if err := writeConfigFile(cfg); err != nil {
					return nil, nil, err
				}

				fmt.Printf("Regenerating %s SDK did not result in any changes\n", lang)
			}
		} else {
			fmt.Println("No changes detected")
		}
	}

	releaseVersion := ""
	usingGoVersion := false

	if c, ok := genConfigs["go"]; ok {
		releaseVersion = c.Config.Go.Version
		usingGoVersion = true
	}

	regenerated := false

	packageNames := map[string]string{}

	for lang, cfg := range genConfigs {
		if langGenerated[lang] {
			outputs[lang+"_regenerated"] = "true"

			mgmtConfig := cfg.Config.Management

			mgmtConfig.SpeakeasyVersion = speakeasyVersion
			mgmtConfig.OpenAPIVersion = docVersion
			mgmtConfig.OpenAPIChecksum = docChecksum

			if err := writeConfigFile(cfg); err != nil {
				return nil, nil, err
			}

			langCfg := cfg.Config.GetLangConfig(lang)

			packageNames[lang] = langCfg.PackageName

			if !usingGoVersion {
				if releaseVersion == "" {
					releaseVersion = langCfg.Version
				} else {
					v, err := version.NewVersion(releaseVersion)
					if err != nil {
						return nil, nil, fmt.Errorf("error parsing version: %w", err)
					}

					v2, err := version.NewVersion(langCfg.Version)
					if err != nil {
						return nil, nil, fmt.Errorf("error parsing version: %w", err)
					}

					if v2.GreaterThan(v) {
						releaseVersion = langCfg.Version
					}
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
			ReleaseVersion:    releaseVersion,
			PackageNames:      packageNames,
		}
	}

	return genInfo, outputs, nil
}

func checkForChanges(speakeasyVersion, docVersion, docChecksum, sdkVersion string, mgmtConfig *management) (string, error) {
	if speakeasyVersion != mgmtConfig.SpeakeasyVersion || docVersion != mgmtConfig.OpenAPIVersion || docChecksum != mgmtConfig.OpenAPIChecksum {
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

		if mgmtConfig.OpenAPIVersion == "" {
			bumpMinor = true
		} else {
			currentDocV, err := version.NewVersion(docVersion)
			// If not a semver then we just deal with the checksum
			if err == nil {
				previousDocV, err := version.NewVersion(mgmtConfig.OpenAPIVersion)
				if err != nil {
					return "", fmt.Errorf("error parsing config openapi version: %w", err)
				}

				if currentDocV.Segments()[0] > previousDocV.Segments()[0] {
					fmt.Printf("OpenAPI doc version changed detected: %s > %s\n", mgmtConfig.OpenAPIVersion, docVersion)
					bumpMajor = true
					docVersionUpdated = true
				} else if currentDocV.Segments()[1] > previousDocV.Segments()[1] {
					fmt.Printf("OpenAPI doc version changed detected: %s > %s\n", mgmtConfig.OpenAPIVersion, docVersion)
					bumpMinor = true
					docVersionUpdated = true
				} else if currentDocV.Segments()[2] > previousDocV.Segments()[2] {
					fmt.Printf("OpenAPI doc version changed detected: %s > %s\n", mgmtConfig.OpenAPIVersion, docVersion)
					bumpPatch = true
					docVersionUpdated = true
				}
			} else {
				fmt.Println("::warning title=invalid_version::openapi version is not a semver")
			}
		}

		if mgmtConfig.OpenAPIChecksum == "" {
			bumpMinor = true
		} else if docChecksum != mgmtConfig.OpenAPIChecksum {
			bumpPatch = true

			fmt.Printf("OpenAPI doc checksum changed detected: %s > %s\n", mgmtConfig.OpenAPIChecksum, docChecksum)

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
		} else if bumpPatch {
			fmt.Println("Bumping SDK patch version")
			patch++
		}

		return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
	}

	return "", nil
}
