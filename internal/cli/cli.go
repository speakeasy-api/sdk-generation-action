package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
)

var (
	ChangeLogVersion               = version.Must(version.NewVersion("1.14.2"))
	UnpublishedInstallationVersion = version.Must(version.NewVersion("1.16.0"))
	MergeVersion                   = version.Must(version.NewVersion("1.21.3"))
	RepoDetailsVersion             = version.Must(version.NewVersion("1.23.1"))
	OutputTestsVersion             = version.Must(version.NewVersion("1.33.2"))
	LLMSuggestionVersion           = version.Must(version.NewVersion("1.47.1"))
)

func IsAtLeastVersion(version *version.Version) bool {
	sv, err := GetSpeakeasyVersion()
	if err != nil {
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
	sv, err := GetSpeakeasyVersion()
	if err != nil {
		return nil, err
	}

	// speakeasy versions before 1.14.2 don't support the generate sdk version command
	if sv.LessThan(ChangeLogVersion) {
		return sv, nil
	}

	out, err := runSpeakeasyCommand("generate", "sdk", "version")
	if err != nil {
		return nil, err
	}

	logging.Debug(out)

	r := regexp.MustCompile(`^Version:.*?v([0-9]+\.[0-9]+\.[0-9]+)`)

	v := r.FindStringSubmatch(strings.TrimSpace(out))[1]

	genVersion, err := version.NewVersion(v)
	if err != nil {
		return nil, fmt.Errorf("failed to parse generation version %s: %w", v, err)
	}

	return genVersion, nil
}

func GetChangelog(genVersion, previousGenVersion string) (string, error) {
	if !IsAtLeastVersion(ChangeLogVersion) {
		return "", nil
	}

	args := []string{}
	startVersionFlag := "-s"

	if previousGenVersion != "" {
		startVersionFlag = "-t"
		args = append(args, "-p", "v"+previousGenVersion)
	}

	args = append([]string{
		"generate",
		"sdk",
		"changelog",
		"-r",
		startVersionFlag,
		"v" + genVersion,
	}, args...)

	out, err := runSpeakeasyCommand(args...)
	if err != nil {
		return "", err
	}

	return out, nil
}

func Validate(docPath string) error {
	out, err := runSpeakeasyCommand("validate", "openapi", "-s", docPath)
	if err != nil {
		return fmt.Errorf("error validating openapi: %w - %s", err, out)
	}
	fmt.Println(out)
	return nil
}

func Suggest(docPath string) (string, error) {
	out, err := runSpeakeasyCommand("suggest", "--schema", docPath, "--auto-approve", "--max-suggestions", "20")
	if err != nil {
		return out, fmt.Errorf("error suggesting openapi fixes: %w - %s", err, "")
	}

	return out, nil
}

func Generate(docPath, lang, outputDir, installationURL string, published, outputTests bool, repoURL, repoSubDirectory string) error {
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

	if IsAtLeastVersion(UnpublishedInstallationVersion) {
		args = append(args, "-i", installationURL)
		if published {
			args = append(args, "-p")
		}
	}

	if IsAtLeastVersion(RepoDetailsVersion) {
		if repoURL != "" {
			args = append(args, "-r", repoURL)
		}
		if repoSubDirectory != "" {
			args = append(args, "-b", repoSubDirectory)
		}
	}

	if IsAtLeastVersion(OutputTestsVersion) && outputTests {
		args = append(args, "-t")
	}

	out, err := runSpeakeasyCommand(args...)
	if err != nil {
		return fmt.Errorf("error generating sdk: %w - %s", err, out)
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
	if !IsAtLeastVersion(MergeVersion) {
		return fmt.Errorf("speakeasy version %s does not support merging documents", MergeVersion)
	}

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
