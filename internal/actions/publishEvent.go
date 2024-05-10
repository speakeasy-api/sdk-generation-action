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
		}

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
		case "pypi":
			processingErr = processPyPI(loadedCfg, event, path, version)
		case "npm":
			processingErr = processNPM(loadedCfg, event, path, version)
		case "packagist":
			processingErr = processPackagist(loadedCfg, event, path)
		case "nuget":
			processingErr = processNuget(loadedCfg, event, path, version)
		case "gems":
			processingErr = processGems(loadedCfg, event, path)
		case "sonatype":
			processingErr = processSonatype(loadedCfg, event, path)
		case "terraform":
			processingErr = processTerraform(loadedCfg, event, path, version)
		}

		if processingErr != nil {
			return processingErr
		}

		event.Success = strings.Contains(strings.ToLower(os.Getenv("GH_ACTION_RESULT")), "success")

		return nil
	})
}

func processPyPI(cfg *config.Config, event *shared.CliEvent, path string, version string) error {
	lang := "python"
	if cfg.Config == nil {
		return fmt.Errorf("empty config for %s language target in directory %s", lang, path)
	}

	langCfg, ok := cfg.Config.Languages[lang]
	if !ok {
		return fmt.Errorf("no %s config in directory %s", lang, path)
	}

	event.GenerateTarget = &lang

	var packageName string
	if name, ok := langCfg.Cfg["packageName"]; ok {
		if strName, ok := name.(string); ok {
			packageName = strName
		}
	}

	if packageName != "" {
		event.PublishPackageName = &packageName
	}

	if packageName != "" && version != "" {
		publishURL := fmt.Sprintf("https://pypi.org/project/%s/%s", packageName, version)
		event.PublishPackageURL = &publishURL
	}

	return nil
}

func processNPM(cfg *config.Config, event *shared.CliEvent, path string, version string) error {
	lang := "typescript"
	if cfg.Config == nil {
		return fmt.Errorf("empty config for %s language target in directory %s", lang, path)
	}

	langCfg, ok := cfg.Config.Languages[lang]
	if !ok {
		return fmt.Errorf("no %s config in directory %s", lang, path)
	}

	event.GenerateTarget = &lang

	var packageName string
	if name, ok := langCfg.Cfg["packageName"]; ok {
		if strName, ok := name.(string); ok {
			packageName = strName
		}
	}

	if packageName != "" {
		event.PublishPackageName = &packageName
	}

	if packageName != "" && version != "" {
		publishURL := fmt.Sprintf("https://www.npmjs.com/package/%s/v/%s", packageName, version)
		event.PublishPackageURL = &publishURL
	}

	return nil
}

func processPackagist(cfg *config.Config, event *shared.CliEvent, path string) error {
	lang := "php"
	if cfg.Config == nil {
		return fmt.Errorf("empty config for %s language target in directory %s", lang, path)
	}

	langCfg, ok := cfg.Config.Languages[lang]
	if !ok {
		return fmt.Errorf("no %s config in directory %s", lang, path)
	}

	event.GenerateTarget = &lang

	var packageName string
	if name, ok := langCfg.Cfg["packageName"]; ok {
		if strName, ok := name.(string); ok {
			packageName = strName
		}
	}

	if packageName != "" {
		event.PublishPackageName = &packageName
	}

	if packageName != "" {
		publishURL := fmt.Sprintf("https://packagist.org/packages/%s", packageName)
		event.PublishPackageURL = &publishURL
	}

	return nil
}

func processNuget(cfg *config.Config, event *shared.CliEvent, path string, version string) error {
	lang := "csharp"
	if cfg.Config == nil {
		return fmt.Errorf("empty config for %s language target in directory %s", lang, path)
	}

	langCfg, ok := cfg.Config.Languages[lang]
	if !ok {
		return fmt.Errorf("no %s config in directory %s", lang, path)
	}

	event.GenerateTarget = &lang

	var packageName string
	if name, ok := langCfg.Cfg["packageName"]; ok {
		if strName, ok := name.(string); ok {
			packageName = strName
		}
	}

	if packageName != "" {
		event.PublishPackageName = &packageName
	}

	if packageName != "" && version != "" {
		publishURL := fmt.Sprintf("https://www.nuget.org/packages/%s/%s", packageName, version)
		event.PublishPackageURL = &publishURL
	}

	return nil
}

func processGems(cfg *config.Config, event *shared.CliEvent, path string) error {
	lang := "ruby"
	if cfg.Config == nil {
		return fmt.Errorf("empty config for %s language target in directory %s", lang, path)
	}

	langCfg, ok := cfg.Config.Languages[lang]
	if !ok {
		return fmt.Errorf("no %s config in directory %s", lang, path)
	}

	event.GenerateTarget = &lang

	var packageName string
	if name, ok := langCfg.Cfg["packageName"]; ok {
		if strName, ok := name.(string); ok {
			packageName = strName
		}
	}

	if packageName != "" {
		event.PublishPackageName = &packageName
	}

	if packageName != "" {
		publishURL := fmt.Sprintf("https://rubygems.org/gems/%s", packageName)
		event.PublishPackageURL = &publishURL
	}

	return nil
}

func processSonatype(cfg *config.Config, event *shared.CliEvent, path string) error {
	lang := "java"
	if cfg.Config == nil {
		return fmt.Errorf("empty config for %s language target in directory %s", lang, path)
	}

	langCfg, ok := cfg.Config.Languages[lang]
	if !ok {
		return fmt.Errorf("no %s config in directory %s", lang, path)
	}

	event.GenerateTarget = &lang

	var groupID string
	if name, ok := langCfg.Cfg["groupID"]; ok {
		if strName, ok := name.(string); ok {
			groupID = strName
		}
	}

	var artifactID string
	if name, ok := langCfg.Cfg["artifactID"]; ok {
		if strName, ok := name.(string); ok {
			artifactID = strName
		}
	}

	// TODO: Figure out how to better represent java published package and the publish URL
	if groupID != "" && artifactID != "" {
		combinedPackage := fmt.Sprintf("%s:%s", groupID, artifactID)
		event.PublishPackageName = &combinedPackage
	}

	return nil
}

func processTerraform(cfg *config.Config, event *shared.CliEvent, path string, version string) error {
	lang := "terraform"
	if cfg.Config == nil {
		return fmt.Errorf("empty config for %s language target in directory %s", lang, path)
	}

	langCfg, ok := cfg.Config.Languages[lang]
	if !ok {
		return fmt.Errorf("no %s config in directory %s", lang, path)
	}

	event.GenerateTarget = &lang

	var packageName string
	if name, ok := langCfg.Cfg["packageName"]; ok {
		if strName, ok := name.(string); ok {
			packageName = strName
		}
	}

	var author string
	if name, ok := langCfg.Cfg["author"]; ok {
		if strName, ok := name.(string); ok {
			author = strName
		}
	}

	if packageName != "" {
		event.PublishPackageName = &packageName
	}

	if packageName != "" && author != "" && version != "" {
		publishURL := fmt.Sprintf("https://registry.terraform.io/providers/%s/%s/%s", author, packageName, version)
		event.PublishPackageURL = &publishURL
	}

	return nil
}

func processLockFile(lockFile config.LockFile, event *shared.CliEvent) string {
	if lockFile.ID != "" {
		event.GenerateGenLockID = &lockFile.ID
	}

	if lockFile.Management.ReleaseVersion != "" {
		event.PublishPackageVersion = &lockFile.Management.ReleaseVersion
	}

	if lockFile.Management.SpeakeasyVersion != "" {
		event.SpeakeasyVersion = lockFile.Management.SpeakeasyVersion
	}

	return lockFile.Management.ReleaseVersion
}
