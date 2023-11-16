package generate

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-version"
	config "github.com/speakeasy-api/sdk-gen-config"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/document"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"gopkg.in/yaml.v3"
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

type versionInfo struct {
	generationVersion *version.Version
	docVersion        string
	docChecksum       string
	sdkVersion        string
}

var (
	v0 = version.Must(version.NewVersion("0.0.0"))
	v1 = version.Must(version.NewVersion("1.0.0"))
)

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

		newVersion, previousVersion, err := checkIfGenerationNeeded(cfg.Config, lang, globalPreviousGenVersion, versionInfo{
			generationVersion: generationVersion,
			docVersion:        docVersion,
			docChecksum:       docChecksum,
			sdkVersion:        sdkVersion,
		})
		if err != nil {
			return nil, outputs, err
		}
		globalPreviousGenVersion = previousVersion

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

func GenerateDocs(g Git) (*GenerationInfo, map[string]string, error) {
	// TODO: For now SDK Docs will generate into the root directory.
	rootDir := "."

	languages := environment.GetLanguages()
	languages = strings.ReplaceAll(languages, "\\n", "\n")
	langs := []string{}
	if err := yaml.Unmarshal([]byte(languages), &langs); err != nil {
		return nil, nil, fmt.Errorf("failed to parse languages: %w", err)
	}

	workspace := environment.GetWorkspace()
	outputs := map[string]string{}

	docPath, docChecksum, docVersion, err := document.GetOpenAPIFileInfo()
	if err != nil {
		return nil, nil, err
	}

	genConfigs, err := configuration.LoadGeneratorConfigs(workspace, map[string]string{
		"docs": rootDir,
	})
	if err != nil {
		return nil, outputs, err
	}

	cfg := genConfigs["docs"]

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

	langCfg, ok := cfg.Config.Languages["docs"]
	if !ok {
		langCfg = config.LanguageConfig{
			Version: "0.0.0",
		}

		cfg.Config.Languages["docs"] = langCfg
	}

	if cfg.Config.Management == nil {
		cfg.Config.Management = &config.Management{}
	}

	sdkVersion := langCfg.Version

	newVersion, previousVersion, err := checkIfGenerationNeeded(cfg.Config, "docs", globalPreviousGenVersion, versionInfo{
		generationVersion: generationVersion,
		docVersion:        docVersion,
		docChecksum:       docChecksum,
		sdkVersion:        sdkVersion,
	})
	if err != nil {
		return nil, outputs, err
	}
	globalPreviousGenVersion = previousVersion

	if newVersion != "" {
		fmt.Println("New version detected: ", newVersion)
		outputDir := path.Join(workspace, "repo", rootDir)

		langCfg.Version = newVersion
		cfg.Config.Languages["docs"] = langCfg

		if err := config.Save(cfg.ConfigDir, cfg.Config); err != nil {
			return nil, outputs, err
		}

		fmt.Printf("Generating SDK Docs in %s\n", outputDir)

		if err := cli.GenerateDocs(docPath, strings.Join(langs, ","), outputDir); err != nil {
			return nil, outputs, err
		}

		// Load the config again as it could have been modified by the generator
		loadedCfg, err := config.Load(outputDir)
		if err != nil {
			return nil, outputs, err
		}

		cfg.Config = loadedCfg

		outputs["docs_directory"] = rootDir

		dirty, err := g.CheckDirDirty(rootDir)
		if err != nil {
			return nil, outputs, err
		}

		if dirty {
			langGenerated["docs"] = true
		} else {
			langCfg.Version = sdkVersion
			cfg.Config.Languages["docs"] = langCfg

			if err := config.Save(cfg.ConfigDir, cfg.Config); err != nil {
				return nil, outputs, err
			}

			fmt.Printf("Regenerating SDK Docs did not result in any changes\n")
		}
	} else {
		fmt.Println("No changes detected")
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

func checkIfGenerationNeeded(cfg *config.Config, lang, globalPreviousGenVersion string, versionInfo versionInfo) (string, string, error) {
	bumpMajor := false
	bumpMinor := false
	bumpPatch := false

	isPreV1 := false
	var sdkV *version.Version

	if versionInfo.sdkVersion != "" {
		var err error
		sdkV, err = version.NewVersion(versionInfo.sdkVersion)
		if err != nil {
			return "", "", fmt.Errorf("error parsing sdk version %s: %w", versionInfo.sdkVersion, err)
		}

		isPreV1 = sdkV.LessThan(v1)
	} else {
		sdkV = v0
		isPreV1 = true
	}

	previousFeatureVersions, ok := cfg.Features[lang]

	if cli.IsAtLeastVersion(cli.GranularChangeLogVersion) && ok {
		latestFeatureVersions, err := cli.GetLatestFeatureVersions(lang)
		if err != nil {
			return "", "", err
		}

		if globalPreviousGenVersion != "" {
			globalPreviousGenVersion += ";"
		}

		globalPreviousGenVersion += fmt.Sprintf("%s:", lang)

		previousFeatureParts := []string{}

		for feature, previousVersion := range previousFeatureVersions {
			latestVersion, ok := latestFeatureVersions[feature]
			if !ok {
				bumpMinor = true // If the feature is no longer supported then we bump the minor version as we will consider it a feature change for now (maybe breaking though?)
				continue
			}

			previous, err := version.NewVersion(previousVersion)
			if err != nil {
				return "", "", fmt.Errorf("failed to parse previous feature version %s for feature %s: %w", previousVersion, feature, err)
			}

			latest, err := version.NewVersion(latestVersion)
			if err != nil {
				return "", "", fmt.Errorf("failed to parse latest feature version %s for feature %s: %w", latestVersion, feature, err)
			}

			if latest.GreaterThan(previous) {
				fmt.Printf("Feature version changed detected for %s: %s > %s\n", feature, previousVersion, latestVersion)

				if latest.Segments()[0] > previous.Segments()[0] {
					bumpMajor = true
				} else if latest.Segments()[1] > previous.Segments()[1] {
					bumpMinor = true
				} else {
					bumpPatch = true
				}
			}

			previousFeatureParts = append(previousFeatureParts, fmt.Sprintf("%s,%s", feature, previousVersion))
		}

		globalPreviousGenVersion += strings.Join(previousFeatureParts, ",")
	} else {
		// Older versions of the gen.yaml won't have a generation version
		previousGenVersion := cfg.Management.GenerationVersion
		if previousGenVersion == "" {
			previousGenVersion = cfg.Management.SpeakeasyVersion
		}

		if globalPreviousGenVersion == "" {
			globalPreviousGenVersion = previousGenVersion
		} else if previousGenVersion != "" {
			global, err := version.NewVersion(globalPreviousGenVersion)
			if err != nil {
				return "", "", fmt.Errorf("failed to parse global previous gen version %s: %w", globalPreviousGenVersion, err)
			}

			previous, err := version.NewVersion(previousGenVersion)
			if err != nil {
				return "", "", fmt.Errorf("failed to parse previous gen version %s: %w", previousGenVersion, err)
			}

			if previous.LessThan(global) {
				globalPreviousGenVersion = previousGenVersion
			}
		}

		genVersion, err := normalizeGenVersion(versionInfo.generationVersion.String())
		if err != nil {
			return "", "", err
		}

		if previousGenVersion == "" {
			previousGenVersion = "0.0.0"
		}

		previousGenerationVersion, err := normalizeGenVersion(previousGenVersion)
		if err != nil {
			return "", "", err
		}

		if !genVersion.Equal(previousGenerationVersion) {
			if previousGenVersion == "" {
				bumpMinor = true
			} else {
				fmt.Printf("Generation version changed detected: %s > %s\n", previousGenVersion, versionInfo.generationVersion)

				if genVersion.Segments()[0] > previousGenerationVersion.Segments()[0] {
					bumpMajor = true
				} else if genVersion.Segments()[1] > previousGenerationVersion.Segments()[1] {
					bumpMinor = true
				} else {
					bumpPatch = true
				}
			}
		}
	}

	if versionInfo.docVersion != cfg.Management.DocVersion || versionInfo.docChecksum != cfg.Management.DocChecksum || environment.ForceGeneration() {
		docVersionUpdated := false

		if cfg.Management.DocVersion == "" {
			bumpMinor = true
		} else {
			currentDocV, err := version.NewVersion(versionInfo.docVersion)
			// If not a semver then we just deal with the checksum
			if err == nil {
				previousDocV, err := version.NewVersion(cfg.Management.DocVersion)
				if err != nil {
					return "", "", fmt.Errorf("error parsing config openapi version %s: %w", cfg.Management.DocVersion, err)
				}

				if currentDocV.GreaterThan(previousDocV) {
					fmt.Printf("OpenAPI doc version changed detected: %s > %s\n", cfg.Management.DocVersion, versionInfo.docVersion)
					docVersionUpdated = true

					if currentDocV.Segments()[0] > previousDocV.Segments()[0] {
						bumpMajor = true
					} else if currentDocV.Segments()[1] > previousDocV.Segments()[1] {
						bumpMinor = true
					} else {
						bumpPatch = true
					}
				}
			} else {
				fmt.Println("::warning title=invalid_version::openapi version is not a semver")
			}
		}

		if cfg.Management.DocChecksum == "" {
			bumpMinor = true
		} else if versionInfo.docChecksum != cfg.Management.DocChecksum {
			bumpPatch = true

			fmt.Printf("OpenAPI doc checksum changed detected: %s > %s\n", cfg.Management.DocChecksum, versionInfo.docChecksum)

			if !docVersionUpdated {
				fmt.Println("::warning title=checksum_changed::openapi checksum changed but version did not")
			}
		}

		if environment.ForceGeneration() {
			fmt.Println("Forcing generation, bumping patch version")
			bumpMinor = true
		}
	}

	if bumpMajor || bumpMinor || bumpPatch {
		major := sdkV.Segments()[0]
		minor := sdkV.Segments()[1]
		patch := sdkV.Segments()[2]

		// We are assuming breaking changes are okay pre v1
		if isPreV1 && bumpMajor {
			bumpMajor = false
			bumpMinor = true
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

		return fmt.Sprintf("%d.%d.%d", major, minor, patch), globalPreviousGenVersion, nil
	}

	return "", globalPreviousGenVersion, nil
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
	case "ruby":
		base := fmt.Sprintf("%s/%s", environment.GetGithubServerURL(), environment.GetRepo())

		if subdirectory == "." {
			return base
		}

		return base + " -d " + subdirectory
	}

	// Neither Java nor C# support pulling directly from git
	return ""
}

func getRepoDetails(subdirectory string) (string, string) {
	subdirectory = filepath.Clean(subdirectory)

	return fmt.Sprintf("%s/%s.git", environment.GetGithubServerURL(), environment.GetRepo()), subdirectory
}
