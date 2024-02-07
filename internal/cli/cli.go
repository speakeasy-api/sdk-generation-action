package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"

	"github.com/hashicorp/go-version"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
)

var MinimumSupportedCLIVersion = version.Must(version.NewVersion("1.130.0"))

func IsAtLeastVersion(version *version.Version) bool {
	sv, err := GetSpeakeasyVersion()
	if err != nil {
		logging.Debug(err.Error())
		return false
	}

	return sv.GreaterThanOrEqual(version)
}

func GetSupportedLanguages() ([]string, error) {
	out, err := runSpeakeasyCommand("generate", "sdk", "--help")
	if err != nil {
		return nil, err
	}

	r := regexp.MustCompile(`available options: \[(.*?)\]`)

	langs := r.FindStringSubmatch(strings.TrimSpace(out))[1]

	return strings.Split(langs, ", "), nil
}

func Run(installationURLs map[string]string, repoURL string, repoSubdirectories map[string]string) error {
	args := []string{
		"run",
	}

	args = append(args, "-t", "all")

	urls, err := json.Marshal(installationURLs)
	if err != nil {
		return fmt.Errorf("error marshalling installation urls: %w", err)
	}
	args = append(args, "--installationURLs", string(urls))

	if repoURL != "" {
		args = append(args, "-r", repoURL)
	}

	subdirs, err := json.Marshal(repoSubdirectories)
	if err != nil {
		return fmt.Errorf("error marshalling repo subdirectories: %w", err)
	}
	args = append(args, "--repo-subdirs", string(subdirs))

	if environment.ForceGeneration() {
		fmt.Println("force input enabled - setting SPEAKEASY_FORCE_GENERATION=true")
		os.Setenv("SPEAKEASY_FORCE_GENERATION", "true")
	}

	out, err := runSpeakeasyCommand(args...)
	if err != nil {
		return fmt.Errorf("error running workflow: %w - %s", err, out)
	}
	fmt.Println(out)
	return nil
}

func RunDocs(docPath, langs, outputDir string) error {
	args := []string{
		"generate",
		"docs",
		"-s",
		docPath,
		"-l",
		langs,
		"-o",
		outputDir,
	}

	out, err := runSpeakeasyCommand(args...)
	if err != nil {
		return fmt.Errorf("error generating sdk docs: %w - %s", err, out)
	}
	fmt.Println(out)
	return nil
}

func TriggerGoGenerate() error {
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = filepath.Join(environment.GetWorkspace(), "repo", environment.GetWorkingDirectory())
	output, err := tidyCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error running command: go mod tidy - %w\n %s", err, string(output))
	}
	generateCmd := exec.Command("go", "generate", "./...")
	generateCmd.Dir = filepath.Join(environment.GetWorkspace(), "repo", environment.GetWorkingDirectory())
	output, err = generateCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error running command: go generate ./... - %w\n %s", err, output)
	}

	return nil
}

func GetSpeakeasyVersion() (*version.Version, error) {
	out, err := runSpeakeasyCommand("--version")
	if err != nil {
		return nil, err
	}

	logging.Debug(out)

	r := regexp.MustCompile(`speakeasy version ([0-9]+\.[0-9]+\.[0-9]+)`)

	v := r.FindStringSubmatch(strings.TrimSpace(out))[1]

	ver, err := version.NewVersion(v)
	if err != nil {
		return nil, fmt.Errorf("failed to parse speakeasy version %s: %w", v, err)
	}

	return ver, nil
}

func GetGenerationVersion() (*version.Version, error) {
	out, err := runSpeakeasyCommand("generate", "sdk", "version")
	if err != nil {
		return nil, err
	}

	logging.Debug(out)

	r := regexp.MustCompile(`(?m)^Version:.*?v([0-9]+\.[0-9]+\.[0-9]+)`)

	v := r.FindStringSubmatch(strings.TrimSpace(out))[1]

	genVersion, err := version.NewVersion(v)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generation version %s: %w", v, err)
	}

	return genVersion, nil
}

func GetLatestFeatureVersions(lang string) (map[string]string, error) {
	out, err := runSpeakeasyCommand("generate", "sdk", "version", "-l", lang)
	if err != nil {
		return nil, err
	}

	logging.Debug(out)

	r := regexp.MustCompile(`(?m)^  ([a-zA-Z]+): ([0-9]+\.[0-9]+\.[0-9]+)`)

	matches := r.FindAllStringSubmatch(out, -1)

	versions := map[string]string{}

	for _, subMatch := range matches {
		feature := subMatch[1]
		version := subMatch[2]

		versions[feature] = version
	}

	return versions, nil
}

func GetChangelog(lang, genVersion, previousGenVersion string, targetVersions map[string]string, previousVersions map[string]string) (string, error) {
	targetVersionsStrings := []string{}

	for feature, targetVersion := range targetVersions {
		targetVersionsStrings = append(targetVersionsStrings, fmt.Sprintf("%s,%s", feature, targetVersion))
	}

	args := []string{
		"generate",
		"sdk",
		"changelog",
		"-r",
		"-l",
		lang,
		"-t",
		strings.Join(targetVersionsStrings, ","),
	}

	if previousVersions != nil {
		previosVersionsStrings := []string{}

		for feature, previousVersion := range previousVersions {
			previosVersionsStrings = append(previosVersionsStrings, fmt.Sprintf("%s,%s", feature, previousVersion))
		}

		args = append(args, "-p", strings.Join(previosVersionsStrings, ","))
	}

	out, err := runSpeakeasyCommand(args...)
	if err != nil {
		return "", err
	}

	return out, nil
}

func Validate(docPath string, maxValidationWarnings, maxValidationErrors int) error {
	var (
		maxWarns  = strconv.Itoa(maxValidationWarnings)
		maxErrors = strconv.Itoa(maxValidationErrors)
	)
	out, err := runSpeakeasyCommand("validate", "openapi", "-s", docPath, "--max-validation-warnings", maxWarns, "--max-validation-errors", maxErrors)
	if err != nil {
		return fmt.Errorf("error validating openapi: %w - %s", err, out)
	}
	fmt.Println(out)
	return nil
}

func Suggest(docPath, maxSuggestions, docOutputPath string) (string, error) {
	out, err := runSpeakeasyCommand("suggest", "--schema", docPath, "--auto-approve", "--output-file", docOutputPath, "--max-suggestions", maxSuggestions, "--level", "hint", "--serial")
	if err != nil {
		return out, fmt.Errorf("error suggesting openapi fixes: %w - %s", err, "")
	}

	return out, nil
}

func Generate(docPath, lang, outputDir, installationURL string, published, outputTests bool, repoURL, repoSubDirectory string) error {
	outputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return err
	}
	args := []string{
		"generate",
		"sdk",
		"-s",
		docPath,
		"-l",
		lang,
		"-o",
		outputDir,
		"-y",
	}

	args = append(args, "-i", installationURL)
	if published {
		args = append(args, "-p")
	}

	if repoURL != "" {
		args = append(args, "-r", repoURL)
	}
	if repoSubDirectory != "" {
		args = append(args, "-b", repoSubDirectory)
	}

	if outputTests {
		args = append(args, "-t")
	}

	if environment.ForceGeneration() {
		fmt.Println("force input enabled - setting SPEAKEASY_FORCE_GENERATION=true")
		os.Setenv("SPEAKEASY_FORCE_GENERATION", "true")
	}

	out, err := runSpeakeasyCommand(args...)
	if err != nil {
		return fmt.Errorf("error generating sdk: %w - %s", err, out)
	}
	fmt.Println(out)
	return nil
}

func GenerateDocs(docPath, langs, outputDir string) error {
	args := []string{
		"generate",
		"docs",
		"-s",
		docPath,
		"-l",
		langs,
		"-o",
		outputDir,
	}

	out, err := runSpeakeasyCommand(args...)
	if err != nil {
		return fmt.Errorf("error generating sdk docs: %w - %s", err, out)
	}
	fmt.Println(out)
	return nil
}

func ValidateConfig(configDir string) error {
	out, err := runSpeakeasyCommand("validate", "config", "-d", configDir)
	if err != nil {
		return fmt.Errorf("error validating config: %w - %s", err, out)
	}
	fmt.Println(out)
	return nil
}

func MergeDocuments(files []string, output string) error {
	args := []string{
		"merge",
		"-o",
		output,
	}

	for _, f := range files {
		args = append(args, "-s", f)
	}

	out, err := runSpeakeasyCommand(args...)
	if err != nil {
		return fmt.Errorf("error merging documents: %w - %s", err, out)
	}
	fmt.Println(out)
	return nil
}

func ApplyOverlay(overlayPath, inPath, outPath string) error {
	args := []string{
		"overlay",
		"apply",
		"-o",
		overlayPath,
		"-s",
		inPath,
	}

	out, err := runSpeakeasyCommand(args...)
	if err != nil {
		return fmt.Errorf("error applying overlay: %w - %s", err, out)
	}

	if err := os.WriteFile(outPath, []byte(out), os.ModePerm); err != nil {
		return fmt.Errorf("error writing overlay output: %w", err)
	}

	return nil
}
