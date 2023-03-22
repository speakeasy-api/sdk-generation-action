package cli

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/go-version"
)

var ChangeLogVersion = version.Must(version.NewVersion("1.12.7"))

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

	r := regexp.MustCompile(`.*?([0-9]+\.[0-9]+\.[0-9]+)$`)

	return version.NewVersion(r.FindStringSubmatch(strings.TrimSpace(out))[1])
}

func GetGenerationVersion() (*version.Version, error) {
	sv, err := GetSpeakeasyVersion()
	if err != nil {
		return nil, err
	}

	// speakeasy versions before 1.12.7 don't support the generate sdk version command
	if sv.LessThan(ChangeLogVersion) {
		return sv, nil
	}

	out, err := runSpeakeasyCommand("generate", "sdk", "version")
	if err != nil {
		return nil, err
	}

	r := regexp.MustCompile(`^Version:.*?v([0-9]+\.[0-9]+\.[0-9]+)`)

	genVersion, err := version.NewVersion(r.FindStringSubmatch(strings.TrimSpace(out))[1])
	if err != nil {
		return nil, err
	}

	return genVersion, nil
}

func GetChangelog(genVersion, previousGenVersion string) (string, error) {
	sv, err := GetSpeakeasyVersion()
	if err != nil {
		return "", err
	}

	// speakeasy versions before 1.12.7 don't support the generate sdk changelog command
	if sv.LessThan(ChangeLogVersion) {
		return "", nil
	}

	args := []string{
		"generate",
		"sdk",
		"changelog",
		"-r",
		"-t",
		"v" + genVersion,
	}

	if previousGenVersion != "" {
		args = append(args, "-p", "v"+previousGenVersion)
	}

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
