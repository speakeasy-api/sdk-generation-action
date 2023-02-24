package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/go-github/v48/github"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/generate"
	"github.com/speakeasy-api/sdk-generation-action/internal/git"
	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
	"golang.org/x/exp/slices"
)

func main() {
	if environment.IsDebugMode() {
		envs := os.Environ()
		slices.SortFunc(envs, func(i, j string) bool {
			iKey, iValue, _ := strings.Cut(i, "=")
			jKey, jValue, _ := strings.Cut(j, "=")

			return iKey < jKey || (iKey == jKey && iValue < jValue)
		})

		for _, env := range envs {
			fmt.Println(env)
		}
	}

	var err error
	switch environment.GetMode() {
	case "release":
		err = releaseAction()
	default:
		err = genAction()
	}

	if err != nil {
		fmt.Printf("::error title=failed::%v\n", err)
		os.Exit(1)
	}
}

func genAction() error {
	accessToken := environment.GetAccessToken()
	if accessToken == "" {
		return errors.New("github access token is required")
	}

	g := git.New(accessToken)
	if err := g.CloneRepo(); err != nil {
		return err
	}

	var branchName string
	var pr *github.PullRequest

	if environment.GetMode() == "pr" {
		var err error
		branchName, pr, err = g.FindOrCreateBranch()
		if err != nil {
			return err
		}
	}

	if err := cli.Download(environment.GetPinnedSpeakeasyVersion(), g); err != nil {
		return err
	}

	genInfo, outputs, err := generate.Generate(g)
	if err != nil {
		return err
	}

	if genInfo != nil {
		docVersion := genInfo.OpenAPIDocVersion
		speakeasyVersion := genInfo.SpeakeasyVersion

		releaseInfo := releases.ReleasesInfo{
			ReleaseTitle:     environment.GetInvokeTime().Format("2006-01-02 15:04:05"),
			DocVersion:       docVersion,
			SpeakeasyVersion: speakeasyVersion,
			DocLocation:      environment.GetOpenAPIDocLocation(),
			Languages:        map[string]releases.LanguageReleaseInfo{},
		}

		supportedLanguages, err := cli.GetSupportedLanguages()
		if err != nil {
			return err
		}

		for _, lang := range supportedLanguages {
			langGenInfo, ok := genInfo.Languages[lang]

			if ok && outputs[fmt.Sprintf("%s_regenerated", lang)] == "true" && environment.IsLanguagePublished(lang) {
				releaseInfo.Languages[lang] = releases.LanguageReleaseInfo{
					PackageName: langGenInfo.PackageName,
					Version:     langGenInfo.Version,
					Path:        outputs[fmt.Sprintf("%s_directory", lang)],
				}
			}
		}

		if err := releases.UpdateReleasesFile(releaseInfo); err != nil {
			return err
		}

		commitHash, err := g.CommitAndPush(docVersion, speakeasyVersion)
		if err != nil {
			return err
		}

		outputs["commit_hash"] = commitHash

		switch environment.GetMode() {
		case "pr":
			if err := g.CreateOrUpdatePR(branchName, releaseInfo, pr); err != nil {
				return err
			}
		default:
			if environment.CreateGitRelease() {
				if err := g.CreateRelease(releaseInfo); err != nil {
					return err
				}
			}
		}
	}

	if err := setOutputs(outputs); err != nil {
		return err
	}

	return nil
}

func releaseAction() error {
	accessToken := environment.GetAccessToken()
	if accessToken == "" {
		return errors.New("github access token is required")
	}

	g := git.New(accessToken)
	if err := g.CloneRepo(); err != nil {
		return err
	}

	latestRelease, err := releases.GetLastReleaseInfo()
	if err != nil {
		return err
	}

	if environment.CreateGitRelease() {
		if err := g.CreateRelease(*latestRelease); err != nil {
			return err
		}
	}

	outputs := map[string]string{}

	for lang, info := range latestRelease.Languages {
		outputs[fmt.Sprintf("%s_regenerated", lang)] = "true"
		outputs[fmt.Sprintf("%s_directory", lang)] = info.Path
	}

	if err := setOutputs(outputs); err != nil {
		return err
	}

	return nil
}

func setOutputs(outputs map[string]string) error {
	fmt.Println("Setting outputs:")

	outputFile := os.Getenv("GITHUB_OUTPUT")

	f, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("error opening output file: %w", err)
	}
	defer f.Close()

	for k, v := range outputs {
		out := fmt.Sprintf("%s=%s\n", k, v)
		fmt.Print(out)

		if _, err := f.WriteString(out); err != nil {
			return fmt.Errorf("error writing output: %w", err)
		}
	}

	return nil
}
