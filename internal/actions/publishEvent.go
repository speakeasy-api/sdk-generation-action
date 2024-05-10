package actions

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	config "github.com/speakeasy-api/sdk-gen-config"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/telemetry"
	"github.com/speakeasy-api/speakeasy-client-sdk-go/v3/pkg/models/shared"
)

func PublishEvent() error {
	if _, err := initAction(); err != nil {
		return err
	}

	workspace := environment.GetWorkspace()
	path := filepath.Join(workspace, "repo")
	path = filepath.Join(path, os.Getenv("INPUT_TARGET_DIRECTORY"))

	return telemetry.Track(context.Background(), shared.InteractionTypePublish, func(ctx context.Context, event *shared.CliEvent) error {
		registryName := os.Getenv("INPUT_REGISTRY_NAME")
		if registryName != "" {
			event.PublishPackageRegistryName = &registryName
			fmt.Println("REGISTRY NAME")
			fmt.Println(registryName)
		}

		fmt.Println(path)

		loadedCfg, err := config.Load(path)
		if err != nil {
			return err
		}

		if loadedCfg.LockFile == nil {
			return fmt.Errorf("empty lock file for python language target in directory %s", path)
		}

		version := processLockFile(*loadedCfg.LockFile, event)

		var processingErr error
		switch os.Getenv("INPUT_REGISTRY_NAME") {
		case "pypy":
			processingErr = processPyPI(loadedCfg, event, path, version)
		}

		if processingErr != nil {
			return processingErr
		}

		if !strings.Contains(strings.ToLower(os.Getenv("GH_ACTION_RESULT")), "success") {
			return fmt.Errorf("failure in publishing: %s", os.Getenv("GH_ACTION_RESULT"))
		}

		return nil
	})
}

func processPyPI(cfg *config.Config, event *shared.CliEvent, path string, version string) error {
	if cfg.Config == nil {
		return fmt.Errorf("empty config for python language target in directory %s", path)
	}

	langCfg, ok := cfg.Config.Languages["python"]
	if !ok {
		return fmt.Errorf("no python config in directory %s", path)
	}

	var packageName string
	if name, ok := langCfg.Cfg["packageName"]; ok {
		if strName, ok := name.(string); ok {
			packageName = strName
		}
	}

	if packageName != "" {
		event.PublishPackageName = &packageName
		fmt.Println("PACKAGE NAME")
		fmt.Println(packageName)
	}

	if packageName != "" && version != "" {
		publishURL := fmt.Sprintf("https://pypi.org/project/%s/%s/", packageName, version)
		event.PublishPackageURL = &publishURL
		fmt.Println("PUBLISH URL")
		fmt.Println(publishURL)
	}

	return nil

}

func processLockFile(lockFile config.LockFile, event *shared.CliEvent) string {
	if lockFile.ID != "" {
		event.GenerateGenLockID = &lockFile.ID
		fmt.Println("Lock File ID")
		fmt.Println(lockFile.ID)
	}

	if lockFile.Management.ReleaseVersion != "" {
		event.PublishPackageVersion = &lockFile.Management.ReleaseVersion
		fmt.Println("RELEASE VERSION")
		fmt.Println(lockFile.Management.ReleaseVersion)
	}

	if lockFile.Management.SpeakeasyVersion != "" {
		event.SpeakeasyVersion = lockFile.Management.SpeakeasyVersion
	}

	return lockFile.Management.SpeakeasyVersion
}
