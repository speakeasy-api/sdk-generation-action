package run

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v63/github"
	"github.com/speakeasy-api/versioning-reports/versioning"

	"github.com/speakeasy-api/sdk-gen-config/workflow"

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
	GenerationVersion string
	OpenAPIDocVersion string
	Languages         map[string]LanguageGenInfo
}

type RunResult struct {
	GenInfo              *GenerationInfo
	OpenAPIChangeSummary string
	LintingReportURL     string
	ChangesReportURL     string
	VersioningReport     *versioning.MergedVersionReport
}

type Git interface {
	CheckDirDirty(dir string, ignoreMap map[string]string) (bool, string, error)
}

func Run(g Git, pr *github.PullRequest, wf *workflow.Workflow) (*RunResult, map[string]string, error) {
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

	langConfigs := map[string]*config.LanguageConfig{}

	installationURLs := map[string]string{}
	repoURL := getRepoURL()
	repoSubdirectories := map[string]string{}
	previousManagementInfos := map[string]config.Management{}

	getDirAndOutputDir := func(target workflow.Target) (string, string) {
		dir := "."
		if target.Output != nil {
			dir = *target.Output
		}

		dir = filepath.Join(environment.GetWorkspace(), dir)
		return dir, path.Join(workspace, "repo", dir)
	}

	includesTerraform := false

	// Load initial configs
	for targetID, target := range wf.Targets {
		if environment.SpecifiedTarget() != "" && environment.SpecifiedTarget() != "all" && environment.SpecifiedTarget() != targetID {
			continue
		}

		lang := target.Target
		dir, outputDir := getDirAndOutputDir(target)

		// Load the config so we can get the current version information
		loadedCfg, err := config.Load(outputDir)
		if err != nil {
			return nil, outputs, err
		}
		previousManagementInfos[targetID] = loadedCfg.LockFile.Management

		globalPreviousGenVersion, err = getPreviousGenVersion(loadedCfg.LockFile, lang, globalPreviousGenVersion)
		if err != nil {
			return nil, outputs, err
		}

		published := target.IsPublished() || target.Target == "go"
		fmt.Printf("Generating %s SDK in %s", lang, outputDir)

		installationURL := getInstallationURL(lang, dir)
		// TODO: Temporary check to fix Java. We may remove this entirely, pending conversation
		if installationURL == "" && target.Target != "java" {
			published = true // Treat as published if we don't have an installation URL
		}
		outputs[fmt.Sprintf("publish_%s", lang)] = fmt.Sprintf("%t", published)

		if installationURL != "" {
			installationURLs[targetID] = installationURL
		}
		if dir != "." {
			repoSubdirectories[targetID] = filepath.Clean(dir)
		} else {
			repoSubdirectories[targetID] = ""
		}
		if lang == "terraform" {
			includesTerraform = true
		}
	}

	// Run the workflow
	var runRes *cli.RunResults
	var changereport *versioning.MergedVersionReport

	changereport, runRes, err = versioning.WithVersionReportCapture[*cli.RunResults](context.Background(), func(ctx context.Context) (*cli.RunResults, error) {
		return cli.Run(wf.Targets == nil || len(wf.Targets) == 0, installationURLs, repoURL, repoSubdirectories)
	})
	if err != nil {
		return nil, outputs, err
	}
	if len(changereport.Reports) == 0 {
		// Assume it's not yet enabled (e.g. CLI version too old)
		changereport = nil
	}
	if changereport != nil && !changereport.MustGenerate() && !environment.ForceGeneration() && pr == nil {
		// no further steps
		fmt.Printf("No changes that imply the need for us to automatically regenerate the SDK.\n  Use \"Force Generation\" if you want to force a new generation.\n  Changes would include:\n-----\n%s", changereport.GetMarkdownSection())
		return &RunResult{
			GenInfo:              nil,
			VersioningReport:     changereport,
			OpenAPIChangeSummary: runRes.OpenAPIChangeSummary,
			LintingReportURL:     runRes.LintingReportURL,
			ChangesReportURL:     runRes.ChangesReportURL,
		}, outputs, nil
	}

	// For terraform, we also trigger "go generate ./..." to regenerate docs
	if includesTerraform {
		if err = cli.TriggerGoGenerate(); err != nil {
			return nil, outputs, err
		}
	}

	// Legacy logic: check for changes + dirty-check
	for targetID, target := range wf.Targets {
		if environment.SpecifiedTarget() != "" && environment.SpecifiedTarget() != "all" && environment.SpecifiedTarget() != targetID {
			continue
		}

		lang := target.Target
		dir, outputDir := getDirAndOutputDir(target)

		// Load the config again so we can compare the versions
		loadedCfg, err := config.Load(outputDir)
		if err != nil {
			return nil, outputs, err
		}
		currentManagementInfo := loadedCfg.LockFile.Management
		langCfg := loadedCfg.Config.Languages[lang]
		langConfigs[lang] = &langCfg

		outputs[fmt.Sprintf("%s_directory", lang)] = dir

		previousManagementInfo := previousManagementInfos[targetID]
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
			// OpenAPIDocVersion: docVersion, //TODO
			Languages: langGenInfo,
		}
	}

	return &RunResult{
		GenInfo:              genInfo,
		VersioningReport:     changereport,
		OpenAPIChangeSummary: runRes.OpenAPIChangeSummary,
		LintingReportURL:     runRes.LintingReportURL,
		ChangesReportURL:     runRes.ChangesReportURL,
	}, outputs, nil
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

func getRepoURL() string {
	return fmt.Sprintf("%s/%s.git", environment.GetGithubServerURL(), environment.GetRepo())
}
