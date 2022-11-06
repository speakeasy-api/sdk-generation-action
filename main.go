package main

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/go-git/go-git/v5"
	"github.com/hashicorp/go-version"
	"github.com/invopop/yaml"
)

var baseDir = "/"

func init() {
	// Allows us to run this locally
	if os.Getenv("SPEAKEASY_ENVIRONMENT") == "local" {
		baseDir = "./"
	}
}

func main() {
	if err := runAction(); err != nil {
		fmt.Printf("::error title=failed::%v\n", err)
		os.Exit(1)
	}
}

func runAction() error {
	debug := os.Getenv("INPUT_DEBUG") == "true"

	if debug {
		for _, env := range os.Environ() {
			fmt.Println(env)
		}
	}

	pinnedSpeakeasyVersion := os.Getenv("INPUT_SPEAKEASY_VERSION")
	openAPIDocLoc := os.Getenv("INPUT_OPENAPI_DOC_LOCATION")
	languages := os.Getenv("INPUT_LANGUAGES")

	accessToken := os.Getenv("INPUT_GITHUB_ACCESS_TOKEN")
	if accessToken == "" {
		return errors.New("github access token is required")
	}

	if err := downloadSpeakeasy(pinnedSpeakeasyVersion); err != nil {
		return err
	}

	langs, err := getAndValidateLanguages(languages)
	if err != nil {
		return err
	}

	g, err := cloneRepo(accessToken)
	if err != nil {
		return err
	}

	genConfigs := loadGeneratorConfigs(langs)

	speakeasyVersion, err := getSpeakeasyVersion()
	if err != nil {
		return err
	}

	openAPIDocPath, checksum, version, err := getOpenAPIFileInfo(openAPIDocLoc)
	if err != nil {
		return err
	}

	regenerated := false

	for lang, cfg := range genConfigs {
		dir := langs[lang]
		langCfg, ok := cfg.Config[lang]
		if !ok {
			langCfg = map[string]string{
				"version": "0.0.0",
			}
		}
		sdkVersion := langCfg["version"]

		newVersion, err := checkForChanges(speakeasyVersion, version, checksum, sdkVersion, cfg.Config["management"])
		if err != nil {
			return err
		}

		if newVersion != "" {
			fmt.Println("New version detected: ", newVersion)
			out, err := runSpeakeasyCommand("generate", "sdk", "-s", openAPIDocPath, "-l", lang, "-o", path.Join(baseDir, "repo", dir))
			if err != nil {
				return fmt.Errorf("error generating sdk: %w - %s", err, out)
			}
			fmt.Println(out)

			cfg.Config[lang]["version"] = newVersion

			data, err := yaml.Marshal(cfg.Config)
			if err != nil {
				return fmt.Errorf("error marshaling config: %w", err)
			}

			if err := os.WriteFile(cfg.ConfigPath, data, os.ModePerm); err != nil {
				return fmt.Errorf("error writing config: %w", err)
			}

			regenerated = true
		} else {
			fmt.Println("No changes detected")
		}
	}

	if regenerated {
		w, err := g.Worktree()
		if err != nil {
			return fmt.Errorf("error getting worktree: %w", err)
		}

		if _, err := w.Add("."); err != nil {
			return fmt.Errorf("error adding changes: %w", err)
		}

		// TODO maybe print commit hash
		if _, err := w.Commit(fmt.Sprintf("ci: regenerated with OpenAPI Doc %s, Speakeay CLI %s", version, speakeasyVersion), &git.CommitOptions{
			All: true,
		}); err != nil {
			return fmt.Errorf("error committing changes: %w", err)
		}

		if err := g.Push(&git.PushOptions{
			Auth: getGithubAuth(accessToken),
		}); err != nil {
			return fmt.Errorf("error pushing changes: %w", err)
		}
	}

	return nil
}

func checkForChanges(speakeasyVersion, docVersion, docChecksum, sdkVersion string, mgmtConfig map[string]string) (string, error) {
	if speakeasyVersion != mgmtConfig["speakeasy-version"] || docVersion != mgmtConfig["openapi-version"] || docChecksum != mgmtConfig["openapi-checksum"] {
		bumpMajor := false
		bumpMinor := false
		bumpPatch := false

		if mgmtConfig["speakeasy-version"] == "" {
			bumpMinor = true
		} else {
			previousSpeakeasyV, err := version.NewVersion(mgmtConfig["speakeasy-version"])
			if err != nil {
				return "", fmt.Errorf("error parsing config speakeasy version: %w", err)
			}

			currentSpeakeasyV, err := version.NewVersion(speakeasyVersion)
			if err != nil {
				return "", fmt.Errorf("error parsing speakeasy version: %w", err)
			}

			if currentSpeakeasyV.Segments()[0] > previousSpeakeasyV.Segments()[0] {
				fmt.Printf("Speakeasy version changed detected: %s > %s\n", mgmtConfig["speakeasy-version"], speakeasyVersion)
				bumpMajor = true
			} else if currentSpeakeasyV.Segments()[1] > previousSpeakeasyV.Segments()[1] {
				fmt.Printf("Speakeasy version changed detected: %s > %s\n", mgmtConfig["speakeasy-version"], speakeasyVersion)
				bumpMinor = true
			} else if currentSpeakeasyV.Segments()[2] > previousSpeakeasyV.Segments()[2] {
				fmt.Printf("Speakeasy version changed detected: %s > %s\n", mgmtConfig["speakeasy-version"], speakeasyVersion)
				bumpPatch = true
			}
		}

		docVersionUpdated := false

		if mgmtConfig["openapi-version"] == "" {
			bumpMinor = true
		} else {
			currentDocV, err := version.NewVersion(docVersion)
			// If not a semver then we just deal with the checksum
			if err == nil {
				previousDocV, err := version.NewVersion(mgmtConfig["openapi-version"])
				if err != nil {
					return "", fmt.Errorf("error parsing config openapi version: %w", err)
				}

				if currentDocV.Segments()[0] > previousDocV.Segments()[0] {
					fmt.Printf("OpenAPI doc version changed detected: %s > %s\n", mgmtConfig["openapi-version"], docVersion)
					bumpMajor = true
					docVersionUpdated = true
				} else if currentDocV.Segments()[1] > previousDocV.Segments()[1] {
					fmt.Printf("OpenAPI doc version changed detected: %s > %s\n", mgmtConfig["openapi-version"], docVersion)
					bumpMinor = true
					docVersionUpdated = true
				} else if currentDocV.Segments()[2] > previousDocV.Segments()[2] {
					fmt.Printf("OpenAPI doc version changed detected: %s > %s\n", mgmtConfig["openapi-version"], docVersion)
					bumpPatch = true
					docVersionUpdated = true
				}
			} else {
				fmt.Println("::warning title=invalid_version::openapi version is not a semver")
			}
		}

		if mgmtConfig["openapi-checksum"] == "" {
			bumpMinor = true
		} else if docChecksum != mgmtConfig["openapi-checksum"] {
			bumpPatch = true

			fmt.Printf("OpenAPI doc checksum changed detected: %s > %s\n", mgmtConfig["openapi-checksum"], docChecksum)

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
		}
		if bumpMinor {
			fmt.Println("Bumping SDK minor version")
			minor++
		}
		if bumpPatch {
			fmt.Println("Bumping SDK patch version")
			patch++
		}

		mgmtConfig["speakeasy-version"] = speakeasyVersion
		mgmtConfig["openapi-version"] = docVersion
		mgmtConfig["openapi-checksum"] = docChecksum

		return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
	}

	return "", nil
}
