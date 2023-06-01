package generate

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/hashicorp/go-version"
	config "github.com/speakeasy-api/sdk-gen-config"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/document"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

type LanguageGenInfo struct {
	PackageName string
	Version     string
}

type GenerationInfo struct {
	SpeakeasyVersion  string
	GenerationVersion string
	OpenAPIDocVersion string
	Languages         map[string]LanguageGenInfo
}

type Git interface {
	CheckDirDirty(dir string) (bool, error)
}

func Generate(g Git) (*GenerationInfo, map[string]string, error) {
	langs, err := configuration.GetAndValidateLanguages(true)
	if err != nil {
		return nil, nil, err
	}

	workspace := environment.GetWorkspace()
	outputs := map[string]string{}

	docPath, docChecksum, docVersion, err := document.GetOpenAPIFileInfo()
	if err != nil {
		return nil, nil, err
	}

	genConfigs, err := configuration.LoadGeneratorConfigs(workspace, langs)
	if err != nil {
		return nil, outputs, err
	}

	speakeasyVersion, err := cli.GetSpeakeasyVersion()
	if err != nil {
		return nil, outputs, fmt.Errorf("failed to get speakeasy version: %w", err)
	}
	generationVersion, err := cli.GetGenerationVersion()
	if err != nil {
		return nil, outputs, fmt.Errorf("failed to get generation version: %w", err)
	}

	langGenerated := map[string]bool{}

	globalPreviousGenVersion := ""

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

		// Older versions of the gen.yaml won't have a generation version
		previousGenVersion := cfg.Config.Management.GenerationVersion
		if previousGenVersion == "" {
			previousGenVersion = cfg.Config.Management.SpeakeasyVersion
		}

		if globalPreviousGenVersion == "" {
			globalPreviousGenVersion = previousGenVersion
		} else if previousGenVersion != "" {
			global, err := version.NewVersion(globalPreviousGenVersion)
			if err != nil {
				return nil, outputs, fmt.Errorf("failed to parse global previous gen version %s: %w", globalPreviousGenVersion, err)
			}

			previous, err := version.NewVersion(previousGenVersion)
			if err != nil {
				return nil, outputs, fmt.Errorf("failed to parse previous gen version %s: %w", previousGenVersion, err)
			}

			if previous.LessThan(global) {
				globalPreviousGenVersion = previousGenVersion
			}
		}

		newVersion, err := checkForChanges(generationVersion, previousGenVersion, docVersion, docChecksum, sdkVersion, cfg.Config.Management)
		if err != nil {
			return nil, outputs, err
		}

		if newVersion != "" {
			fmt.Println("New version detected: ", newVersion)
			outputDir := path.Join(workspace, "repo", dir)

			langCfg.Version = newVersion
			cfg.Config.Languages[lang] = langCfg

			if err := config.Save(cfg.ConfigDir, cfg.Config); err != nil {
				return nil, outputs, err
			}

			fmt.Printf("Generating %s SDK in %s\n", lang, outputDir)

			published := environment.IsLanguagePublished(lang)
			installationURL := getInstallationURL(lang, dir)
			if installationURL == "" {
				published = true // Treat as published if we don't have an installation URL
			}

			repoURL, repoSubdirectory := getRepoDetails(dir)

			if err := cli.Generate(docPath, lang, outputDir, installationURL, published, environment.ShouldOutputTests(), repoURL, repoSubdirectory); err != nil {
				return nil, outputs, err
			}

			// Load the config again as it could have been modified by the generator
			loadedCfg, err := config.Load(outputDir)
			if err != nil {
				return nil, outputs, err
			}

			cfg.Config = loadedCfg
			genConfigs[lang] = cfg

			dirForOutput := dir
			if dirForOutput == "" {
				dirForOutput = "."
			}

			outputs[fmt.Sprintf("%s_directory", lang)] = dirForOutput

			dirty, err := g.CheckDirDirty(dir)
			if err != nil {
				return nil, outputs, err
			}

			if dirty {
				langGenerated[lang] = true
			} else {
				langCfg.Version = sdkVersion
				cfg.Config.Languages[lang] = langCfg

				if err := config.Save(cfg.ConfigDir, cfg.Config); err != nil {
					return nil, outputs, err
				}

				fmt.Printf("Regenerating %s SDK did not result in any changes\n", lang)
			}
		} else {
			fmt.Println("No changes detected")
		}
	}

	outputs["previous_gen_version"] = globalPreviousGenVersion

	regenerated := false

	langGenInfo := map[string]LanguageGenInfo{}

	for lang, cfg := range genConfigs {
		if langGenerated[lang] {
			outputs[lang+"_regenerated"] = "true"

			mgmtConfig := cfg.Config.Management

			mgmtConfig.SpeakeasyVersion = speakeasyVersion.String()
			mgmtConfig.GenerationVersion = generationVersion.String()
			mgmtConfig.DocVersion = docVersion
			mgmtConfig.DocChecksum = docChecksum
			cfg.Config.Management = mgmtConfig

			if err := config.Save(cfg.ConfigDir, cfg.Config); err != nil {
				return nil, outputs, err
			}

			langCfg := cfg.Config.Languages[lang]

			switch lang {
			case "java":
				langGenInfo[lang] = LanguageGenInfo{
					PackageName: fmt.Sprintf("%s.%s", langCfg.Cfg["groupID"], langCfg.Cfg["artifactID"]),
					Version:     langCfg.Version,
				}
			case "terraform":
				langGenInfo[lang] = LanguageGenInfo{
					PackageName: fmt.Sprintf("%s/%s", langCfg.Cfg["author"], langCfg.Cfg["packageName"]),
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
			SpeakeasyVersion:  speakeasyVersion.String(),
			GenerationVersion: generationVersion.String(),
			OpenAPIDocVersion: docVersion,
			Languages:         langGenInfo,
		}
	}

	return genInfo, outputs, nil
}

func normalizeGenVersion(v string) (*version.Version, error) {
	genVersion, err := version.NewVersion(v)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generation version %s for normalization: %w", v, err)
	}

	// To avoid a major version bump to SDKs when this feature is released we need to normalize the major version to 1
	// The reason for this is that prior to this we were using the speakeasy version which had a major version of 1 while the generation version had a major version of 2
	// If the generation version is bumped in the future we will just start using that version
	if genVersion.Segments()[0] == 2 {
		v = fmt.Sprintf("%d.%d.%d", 1, genVersion.Segments()[1], genVersion.Segments()[2])
		genVersion, err := version.NewVersion(v)
		if err != nil {
			return nil, fmt.Errorf("failed to parse normalized generation version %s for normalization: %w", v, err)
		}

		return genVersion, nil
	}

	return genVersion, nil
}

func checkForChanges(generationVersion *version.Version, previousGenVersion, docVersion, docChecksum, sdkVersion string, mgmtConfig *config.Management) (string, error) {
	force := environment.ForceGeneration()

	genVersion, err := normalizeGenVersion(generationVersion.String())
	if err != nil {
		return "", err
	}

	if previousGenVersion == "" {
		previousGenVersion = "0.0.0"
	}

	previousGenerationVersion, err := normalizeGenVersion(previousGenVersion)
	if err != nil {
		return "", err
	}

	if !genVersion.Equal(previousGenerationVersion) || docVersion != mgmtConfig.DocVersion || docChecksum != mgmtConfig.DocChecksum || force {
		bumpMajor := false
		bumpMinor := false
		bumpPatch := false

		if previousGenVersion == "" {
			bumpMinor = true
		} else {
			if genVersion.Segments()[0] > previousGenerationVersion.Segments()[0] {
				fmt.Printf("Generation version changed detected: %s > %s\n", previousGenVersion, generationVersion)
				bumpMajor = true
			} else if genVersion.Segments()[1] > previousGenerationVersion.Segments()[1] {
				fmt.Printf("Generation version changed detected: %s > %s\n", previousGenVersion, generationVersion)
				bumpMinor = true
			} else if genVersion.Segments()[2] > previousGenerationVersion.Segments()[2] {
				fmt.Printf("Generation version changed detected: %s > %s\n", previousGenVersion, generationVersion)
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
					return "", fmt.Errorf("error parsing config openapi version %s: %w", mgmtConfig.DocVersion, err)
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
				return "", fmt.Errorf("error parsing sdk version %s: %w", sdkVersion, err)
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

func getInstallationURL(lang, subdirectory string) string {
	subdirectory = filepath.Clean(subdirectory)

	switch lang {
	case "go":
		base := fmt.Sprintf("%s/%s", environment.GetGithubServerURL(), environment.GetRepo())

		if subdirectory == "." {
			return base
		}

		return base + "/" + subdirectory
	case "typescript":
		if subdirectory == "." {
			return fmt.Sprintf("%s/%s", environment.GetGithubServerURL(), environment.GetRepo())
		} else {
			return fmt.Sprintf("https://gitpkg.now.sh/%s/%s", environment.GetRepo(), subdirectory)
		}
	case "python":
		base := fmt.Sprintf("%s/%s.git", environment.GetGithubServerURL(), environment.GetRepo())

		if subdirectory == "." {
			return base
		}

		return base + "#subdirectory=" + subdirectory
	case "php":
		// PHP doesn't support subdirectories
		if subdirectory == "." {
			return fmt.Sprintf("%s/%s", environment.GetGithubServerURL(), environment.GetRepo())
		}
	}

	// Java doesn't support pulling directly from git
	return ""
}

func getRepoDetails(subdirectory string) (string, string) {
	subdirectory = filepath.Clean(subdirectory)

	return fmt.Sprintf("%s/%s.git", environment.GetGithubServerURL(), environment.GetRepo()), subdirectory
}
