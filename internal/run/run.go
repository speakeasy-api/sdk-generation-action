package run

import (
	"fmt"
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"os"
	"path"
	"path/filepath"
	"strings"

	config "github.com/speakeasy-api/sdk-gen-config"
	"github.com/speakeasy-api/sdk-gen-config/workflow"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
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

type Git interface {
	CheckDirDirty(dir string, ignoreMap map[string]string) (bool, string, error)
}

func Run(g Git) (*GenerationInfo, map[string]string, error) {
	workspace := environment.GetWorkspace()
	outputs := map[string]string{}

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

	wf, err := getWorkflow()
	if err != nil {
		return nil, outputs, fmt.Errorf("failed to load workflow file: %w", err)
	}

	langs := make([]string, len(wf.Targets))
	for _, target := range wf.Targets {
		langs = append(langs, target.Target)
	}

	if err := configuration.AssertLangsSupported(langs); err != nil {
		return nil, outputs, err
	}

	langConfigs := map[string]*config.LanguageConfig{}

	for targetID, target := range wf.Targets {
		lang := target.Target
		dir := "."
		if target.Output != nil {
			dir = *target.Output
		}

		outputDir := path.Join(workspace, "repo", dir)

		// Load the config so we can get the current version information
		loadedCfg, err := config.Load(outputDir)
		if err != nil {
			return nil, outputs, err
		}
		previousManagementInfo := loadedCfg.LockFile.Management

		globalPreviousGenVersion, err = getPreviousGenVersion(loadedCfg.LockFile, lang, globalPreviousGenVersion)
		if err != nil {
			return nil, outputs, err
		}

		fmt.Printf("Generating %s SDK in %s\n", lang, outputDir)

		published := environment.IsLanguagePublished(lang)
		installationURL := getInstallationURL(lang, dir)
		if installationURL == "" {
			published = true // Treat as published if we don't have an installation URL
		}

		repoURL, repoSubdirectory := getRepoDetails(dir)

		// TODO: this should be openapi location, not target.Source
		if err = runLang(targetID, lang, target.Source, outputDir, installationURL, published, repoURL, repoSubdirectory); err != nil {
			return nil, outputs, err
		}

		// Load the config again so we can compare the versions
		loadedCfg, err = config.Load(outputDir)
		if err != nil {
			return nil, outputs, err
		}
		currentManagementInfo := loadedCfg.LockFile.Management
		langCfg := loadedCfg.Config.Languages[lang]
		langConfigs[lang] = &langCfg

		outputs[fmt.Sprintf("%s_directory", lang)] = dir

		dirty, dirtyMsg, err := g.CheckDirDirty(dir, map[string]string{
			previousManagementInfo.ReleaseVersion:    currentManagementInfo.ReleaseVersion,
			previousManagementInfo.GenerationVersion: currentManagementInfo.GenerationVersion,
			previousManagementInfo.ConfigChecksum:    currentManagementInfo.ConfigChecksum,
			previousManagementInfo.DocVersion:        currentManagementInfo.DocVersion,
			previousManagementInfo.DocChecksum:       currentManagementInfo.DocChecksum,
		})
		if err != nil {
			return nil, outputs, err
		}

		if dirty {
			langGenerated[lang] = true
			fmt.Printf("Regenerating %s SDK resulted in significant changes %s\n", lang, dirtyMsg)
		} else {
			fmt.Printf("Regenerating %s SDK did not result in any changes\n", lang)
		}
	}

	outputs["previous_gen_version"] = globalPreviousGenVersion

	regenerated := false

	langGenInfo := map[string]LanguageGenInfo{}

	for lang := range langGenerated {
		outputs[lang+"_regenerated"] = "true"

		langCfg := langConfigs[lang]

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

	var genInfo *GenerationInfo

	if regenerated {
		genInfo = &GenerationInfo{
			SpeakeasyVersion:  speakeasyVersion.String(),
			GenerationVersion: generationVersion.String(),
			//OpenAPIDocVersion: docVersion, //TODO
			Languages: langGenInfo,
		}
	}

	return genInfo, outputs, nil
}

func runLang(targetID, target, docPath, outputDir, installationURL string, published bool, repoURL, repoSubdirectory string) error {
	// Docs is not yet supported by `run`
	if target == "docs" {
		docsLanguages := environment.GetDocsLanguages()
		docsLanguages = strings.ReplaceAll(docsLanguages, "\\n", "\n")
		docsLangs := []string{}
		if err := yaml.Unmarshal([]byte(docsLanguages), &docsLangs); err != nil {
			return fmt.Errorf("failed to parse docs languages: %w", err)
		}

		if err := cli.GenerateDocs(docPath, strings.Join(docsLangs, ","), outputDir); err != nil {
			return err
		}
	} else {
		if err := cli.Run(targetID, installationURL, published, repoURL, repoSubdirectory); err != nil {
			return err
		}

		// For terraform, also trigger "go generate ./..." to regenerate docs
		if target == "terraform" {
			if err := cli.TriggerGoGenerate(); err != nil {
				return err
			}
		}
	}

	return nil
}

func getWorkflow() (*workflow.Workflow, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	wf, _, err := workflow.Load(wd)
	if err != nil {
		return nil, err
	}

	return wf, err
}

func getPreviousGenVersion(lockFile *config.LockFile, lang, globalPreviousGenVersion string) (string, error) {
	previousFeatureVersions, ok := lockFile.Features[lang]
	if !ok {
		return globalPreviousGenVersion, nil
	}

	if globalPreviousGenVersion != "" {
		globalPreviousGenVersion += ";"
	}

	globalPreviousGenVersion += fmt.Sprintf("%s:", lang)

	previousFeatureParts := []string{}

	for feature, previousVersion := range previousFeatureVersions {
		previousFeatureParts = append(previousFeatureParts, fmt.Sprintf("%s,%s", feature, previousVersion))
	}

	globalPreviousGenVersion += strings.Join(previousFeatureParts, ",")

	return globalPreviousGenVersion, nil
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
