package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/google/go-github/v48/github"
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/generate"
	"github.com/speakeasy-api/sdk-generation-action/internal/git"
	"github.com/speakeasy-api/sdk-generation-action/internal/releases"
)

func main() {
	if environment.IsDebugMode() {
		for _, env := range os.Environ() {
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
			ReleaseVersion:    genInfo.ReleaseVersion,
			OpenAPIDocVersion: docVersion,
			SpeakeasyVersion:  speakeasyVersion,
			OpenAPIDocPath:    environment.GetOpenAPIDocLocation(),
		}

		if genInfo.PackageNames["python"] != "" && outputs["python_regenerated"] == "true" {
			releaseInfo.PythonPackagePublished = environment.IsPythonPublished()
			releaseInfo.PythonPackageName = genInfo.PackageNames["python"]
			releaseInfo.PythonPath = outputs["python_directory"]
		}

		if genInfo.PackageNames["typescript"] != "" && outputs["typescript_regenerated"] == "true" {
			releaseInfo.NPMPackagePublished = environment.IsTypescriptPublished()
			releaseInfo.NPMPackageName = genInfo.PackageNames["typescript"]
			releaseInfo.TypescriptPath = outputs["typescript_directory"]
		}

		if outputs["go_regenerated"] == "true" {
			releaseInfo.GoPackagePublished = environment.CreateGitRelease()
			releaseInfo.GoPath = outputs["go_directory"]
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

	if latestRelease.PythonPackagePublished {
		outputs["python_regenerated"] = "true"
		outputs["python_directory"] = latestRelease.PythonPath
	}

	if latestRelease.NPMPackagePublished {
		outputs["typescript_regenerated"] = "true"
		outputs["typescript_directory"] = latestRelease.TypescriptPath
	}

	if latestRelease.GoPackagePublished {
		outputs["go_regenerated"] = "true"
		outputs["go_directory"] = latestRelease.GoPath
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
