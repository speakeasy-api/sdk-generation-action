package releases

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

type ReleasesInfo struct {
	ReleaseVersion         string
	OpenAPIDocVersion      string
	SpeakeasyVersion       string
	OpenAPIDocPath         string
	PythonPackagePublished bool
	PythonPackageName      string
	PythonPath             string
	NPMPackagePublished    bool
	NPMPackageName         string
	TypescriptPath         string
	GoPackagePublished     bool
	GoPath                 string
}

func (r ReleasesInfo) String() string {
	releasesOutput := []string{}

	if r.NPMPackagePublished {
		releasesOutput = append(releasesOutput, fmt.Sprintf("- [NPM v%s] https://www.npmjs.com/package/%s/v/%s - %s", r.ReleaseVersion, r.NPMPackageName, r.ReleaseVersion, r.TypescriptPath))
	}

	if r.PythonPackagePublished {
		releasesOutput = append(releasesOutput, fmt.Sprintf("- [PyPI v%s] https://pypi.org/project/%s/%s - %s", r.ReleaseVersion, r.PythonPackageName, r.ReleaseVersion, r.PythonPath))
	}

	if r.GoPackagePublished {
		repoPath := os.Getenv("GITHUB_REPOSITORY")
		releasesOutput = append(releasesOutput, fmt.Sprintf("- [Go v%s] https://github.com/%s/releases/tag/v%s - %s", r.ReleaseVersion, repoPath, r.ReleaseVersion, r.GoPath))
	}

	if len(releasesOutput) > 0 {
		releasesOutput = append([]string{"\n### Releases"}, releasesOutput...)
	}

	return fmt.Sprintf(`%s## Version %s
### Changes
Based on:
- OpenAPI Doc %s %s
- Speakeasy CLI %s https://github.com/speakeasy-api/speakeasy%s`, "\n\n", r.ReleaseVersion, r.OpenAPIDocVersion, r.OpenAPIDocPath, r.SpeakeasyVersion, strings.Join(releasesOutput, "\n"))
}

func UpdateReleasesFile(releaseInfo ReleasesInfo) error {
	releasesPath := getReleasesPath()

	f, err := os.OpenFile(releasesPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("error opening releases file: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(releaseInfo.String())
	if err != nil {
		return fmt.Errorf("error writing to releases file: %w", err)
	}

	return nil
}

var (
	releaseInfoRegex = regexp.MustCompile(`(?s)## Version (\d+\.\d+\.\d+)\n### Changes\nBased on:\n- OpenAPI Doc (\d+\.\d+\.\d+) (.*?)\n- Speakeasy CLI (\d+\.\d+\.\d+).*?`)
	npmReleaseRegex  = regexp.MustCompile(`- \[NPM v\d+\.\d+\.\d+\] https:\/\/www.npmjs.com\/package\/(.*?)\/v\/\d+\.\d+\.\d+ - (.*)`)
	pypiReleaseRegex = regexp.MustCompile(`- \[PyPI v\d+\.\d+\.\d+\] https:\/\/pypi.org\/project\/(.*?)\/\d+\.\d+\.\d+ - (.*)`)
	goReleaseRegex   = regexp.MustCompile(`- \[Go v\d+\.\d+\.\d+\].*? - (.*)`)
)

func GetLastReleaseInfo() (*ReleasesInfo, error) {
	releasesPath := getReleasesPath()

	data, err := os.ReadFile(releasesPath)
	if err != nil {
		return nil, fmt.Errorf("error reading releases file: %w", err)
	}

	return parseReleases(string(data))
}

func parseReleases(data string) (*ReleasesInfo, error) {
	releases := strings.Split(data, "\n\n")

	lastRelease := releases[len(releases)-1]

	matches := releaseInfoRegex.FindStringSubmatch(lastRelease)

	if len(matches) != 5 {
		return nil, fmt.Errorf("error parsing last release info")
	}

	info := &ReleasesInfo{
		ReleaseVersion:    matches[1],
		OpenAPIDocVersion: matches[2],
		OpenAPIDocPath:    matches[3],
		SpeakeasyVersion:  matches[4],
	}

	npmMatches := npmReleaseRegex.FindStringSubmatch(lastRelease)

	if len(npmMatches) == 3 {
		info.NPMPackagePublished = true
		info.NPMPackageName = npmMatches[1]
		info.TypescriptPath = npmMatches[2]
	}

	pypiMatches := pypiReleaseRegex.FindStringSubmatch(lastRelease)

	if len(pypiMatches) == 3 {
		info.PythonPackagePublished = true
		info.PythonPackageName = pypiMatches[1]
		info.PythonPath = pypiMatches[2]
	}

	goMatches := goReleaseRegex.FindStringSubmatch(lastRelease)

	if len(goMatches) == 2 {
		info.GoPackagePublished = true
		info.GoPath = goMatches[1]
	}

	return info, nil
}

func getReleasesPath() string {
	baseDir := environment.GetBaseDir()

	return path.Join(baseDir, "repo", "RELEASES.md")
}
