package actions

import (
	"fmt"

	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/generate"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
)

func Generate() error {
	g, err := initAction()
	if err != nil {
		return err
	}

	resolvedVersion, err := cli.Download(environment.GetPinnedSpeakeasyVersion(), g)
	if err != nil {
		return err
	}

	mode := environment.GetMode()

	branchName := ""

	if mode == environment.ModePR {
		var err error
		branchName, _, err = g.FindExistingPR("", environment.ActionGenerate)
		if err != nil {
			return err
		}
	}

	branchName, err = g.FindOrCreateBranch(branchName, environment.ActionGenerate)
	if err != nil {
		return err
	}

	success := false
	defer func() {
		if !success && !environment.IsDebugMode() {
			if err := g.DeleteBranch(branchName); err != nil {
				logging.Debug("failed to delete branch %s: %v", branchName, err)
			}
		}
	}()

	genInfo, outputs, err := generate.Generate(g)
	outputs["resolved_speakeasy_version"] = resolvedVersion
	if err != nil {
		if err := setOutputs(outputs); err != nil {
			logging.Debug("failed to set outputs: %v", err)
		}
		return err
	}

	if genInfo != nil {
		docVersion := genInfo.OpenAPIDocVersion
		speakeasyVersion := genInfo.SpeakeasyVersion

		releaseInfo := releases.ReleasesInfo{
			ReleaseTitle:       environment.GetInvokeTime().Format("2006-01-02 15:04:05"),
			DocVersion:         docVersion,
			SpeakeasyVersion:   speakeasyVersion,
			GenerationVersion:  genInfo.GenerationVersion,
			DocLocation:        environment.GetOpenAPIDocLocation(),
			Languages:          map[string]releases.LanguageReleaseInfo{},
			LanguagesGenerated: map[string]releases.GenerationInfo{},
		}

		supportedLanguages, err := cli.GetSupportedLanguages()
		if err != nil {
			return err
		}

		for _, lang := range supportedLanguages {
			langGenInfo, ok := genInfo.Languages[lang]

			if ok && outputs[fmt.Sprintf("%s_regenerated", lang)] == "true" {
				path := outputs[fmt.Sprintf("%s_directory", lang)]

				releaseInfo.LanguagesGenerated[lang] = releases.GenerationInfo{
					Version: langGenInfo.Version,
					Path:    path,
				}

				if environment.IsLanguagePublished(lang) {
					releaseInfo.Languages[lang] = releases.LanguageReleaseInfo{
						PackageName: langGenInfo.PackageName,
						Version:     langGenInfo.Version,
						Path:        path,
					}
				}
				if lang == "terraform" {
					// Also trigger "go generate ./..." to regenerate docs
					err = cli.TriggerGoGenerate()
					if err != nil {
						return err
					}
				}
			}
		}

		releasesDir, err := getReleasesDir()
		if err != nil {
			return err
		}

		if err := releases.UpdateReleasesFile(releaseInfo, releasesDir); err != nil {
			return err
		}

		if _, err := g.CommitAndPush(docVersion, speakeasyVersion, "", environment.ActionGenerate); err != nil {
			return err
		}
	}

	outputs["branch_name"] = branchName

	if err := setOutputs(outputs); err != nil {
		return err
	}

	success = true

	return nil
}
