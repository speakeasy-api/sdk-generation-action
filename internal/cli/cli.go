package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
)

var ChangeLogVersion = version.Must(version.NewVersion("1.14.2"))

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

	r := regexp.MustCompile(`.*?([0-9]+\.[0-9]+\.[0-9]+)$`)

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
	sv, err := GetSpeakeasyVersion()
	if err != nil {
		return "", err
	}

	// speakeasy versions before 1.14.2 don't support the generate sdk changelog command
	if sv.LessThan(ChangeLogVersion) {
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

func Generate(docPath, lang, outputDir string) error {
	out, err := runSpeakeasyCommand("generate", "sdk", "-s", docPath, "-l", lang, "-o", outputDir, "-y")
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
