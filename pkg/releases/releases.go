package releases

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
)

type LanguageReleaseInfo struct {
	PackageName string
	Path        string
	Version     string
	URL         string
}

type ReleasesInfo struct {
	ReleaseTitle      string
	DocVersion        string
	SpeakeasyVersion  string
	GenerationVersion string
	DocLocation       string
	Languages         map[string]LanguageReleaseInfo
}

func (r ReleasesInfo) String() string {
	releasesOutput := []string{}

	for lang, info := range r.Languages {
		pkgID := ""
		pkgURL := ""

		switch lang {
		case "go":
			pkgID = "Go"
			repoPath := os.Getenv("GITHUB_REPOSITORY")

			tag := fmt.Sprintf("v%s", info.Version)
			if info.Path != "." {
				tag = fmt.Sprintf("%s/%s", info.Path, tag)
			}

			pkgURL = fmt.Sprintf("https://github.com/%s/releases/tag/%s", repoPath, tag)
		case "typescript":
			pkgID = "NPM"
			pkgURL = fmt.Sprintf("https://www.npmjs.com/package/%s/v/%s", info.PackageName, info.Version)
		case "python":
			pkgID = "PyPI"
			pkgURL = fmt.Sprintf("https://pypi.org/project/%s/%s", info.PackageName, info.Version)
		case "php":
			pkgID = "Composer"
			pkgURL = fmt.Sprintf("https://packagist.org/packages/%s#v%s", info.PackageName, info.Version)
		case "java":
			pkgID = "Maven Central"
			lastDotIndex := strings.LastIndex(info.PackageName, ".")
			groupID := info.PackageName[:lastDotIndex]      // everything before last occurrence of '.'
			artifactID := info.PackageName[lastDotIndex+1:] // everything after last occurrence of '.'
			pkgURL = fmt.Sprintf("https://central.sonatype.com/artifact/%s/%s/%s", groupID, artifactID, info.Version)
		}

		if pkgID != "" {
			releasesOutput = append(releasesOutput, fmt.Sprintf("- [%s v%s] %s - %s", pkgID, info.Version, pkgURL, info.Path))
		}
	}

	if len(releasesOutput) > 0 {
		releasesOutput = append([]string{"\n### Releases"}, releasesOutput...)
	}

	return fmt.Sprintf(`%s## %s
### Changes
Based on:
- OpenAPI Doc %s %s
- Speakeasy CLI %s (%s) https://github.com/speakeasy-api/speakeasy%s`, "\n\n", r.ReleaseTitle, r.DocVersion, r.DocLocation, r.SpeakeasyVersion, r.GenerationVersion, strings.Join(releasesOutput, "\n"))
}

func UpdateReleasesFile(releaseInfo ReleasesInfo, dir string) error {
	releasesPath := GetReleasesPath(dir)

	logging.Debug("Updating releases file at %s", releasesPath)

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
	releaseInfoRegex     = regexp.MustCompile(`(?s)## (.*?)\n### Changes\nBased on:\n- OpenAPI Doc (.*?) (.*?)\n- Speakeasy CLI (.*?) (\((.*?)\))?.*?`)
	npmReleaseRegex      = regexp.MustCompile(`- \[NPM v(\d+\.\d+\.\d+)\] (https:\/\/www\.npmjs\.com\/package\/(.*?)\/v\/\d+\.\d+\.\d+) - (.*)`)
	pypiReleaseRegex     = regexp.MustCompile(`- \[PyPI v(\d+\.\d+\.\d+)\] (https:\/\/pypi\.org\/project\/(.*?)\/\d+\.\d+\.\d+) - (.*)`)
	goReleaseRegex       = regexp.MustCompile(`- \[Go v(\d+\.\d+\.\d+)\] (https:\/\/(github.com\/.*?)\/releases\/tag\/.*?\/?v\d+\.\d+\.\d+) - (.*)`)
	composerReleaseRegex = regexp.MustCompile(`- \[Composer v(\d+\.\d+\.\d+)\] (https:\/\/packagist\.org\/packages\/(.*?)#v\d+\.\d+\.\d+) - (.*)`)
	mavenReleaseRegex    = regexp.MustCompile(`- \[Maven Central v(\d+\.\d+\.\d+)\] (https:\/\/central\.sonatype\.com\/artifact\/(.*?)\/(.*?)\/.*?) - (.*)`)
)

func GetLastReleaseInfo(dir string) (*ReleasesInfo, error) {
	releasesPath := GetReleasesPath(dir)

	logging.Debug("Reading releases file at %s", releasesPath)

	data, err := os.ReadFile(releasesPath)
	if err != nil {
		return nil, fmt.Errorf("error reading releases file: %w", err)
	}

	return ParseReleases(string(data))
}

func ParseReleases(data string) (*ReleasesInfo, error) {
	releases := strings.Split(data, "\n\n")

	lastRelease := releases[len(releases)-1]

	matches := releaseInfoRegex.FindStringSubmatch(lastRelease)

	if len(matches) < 5 {
		return nil, fmt.Errorf("error parsing last release info")
	}

	genVersion := ""
	if len(matches) == 7 {
		genVersion = matches[6]
	} else {
		genVersion = matches[4]
	}

	info := &ReleasesInfo{
		ReleaseTitle:      matches[1],
		DocVersion:        matches[2],
		DocLocation:       matches[3],
		SpeakeasyVersion:  matches[4],
		GenerationVersion: genVersion,
		Languages:         map[string]LanguageReleaseInfo{},
	}

	npmMatches := npmReleaseRegex.FindStringSubmatch(lastRelease)

	if len(npmMatches) == 5 {
		info.Languages["typescript"] = LanguageReleaseInfo{
			Version:     npmMatches[1],
			URL:         npmMatches[2],
			PackageName: npmMatches[3],
			Path:        npmMatches[4],
		}
	}

	pypiMatches := pypiReleaseRegex.FindStringSubmatch(lastRelease)

	if len(pypiMatches) == 5 {
		info.Languages["python"] = LanguageReleaseInfo{
			Version:     pypiMatches[1],
			URL:         pypiMatches[2],
			PackageName: pypiMatches[3],
			Path:        pypiMatches[4],
		}
	}

	goMatches := goReleaseRegex.FindStringSubmatch(lastRelease)

	if len(goMatches) == 5 {
		packageName := goMatches[3]
		path := goMatches[4]

		if path != "." {
			packageName = fmt.Sprintf("%s/%s", packageName, strings.TrimPrefix(path, "./"))
		}

		info.Languages["go"] = LanguageReleaseInfo{
			Version:     goMatches[1],
			URL:         goMatches[2],
			PackageName: packageName,
			Path:        path,
		}
	}

	composerMatches := composerReleaseRegex.FindStringSubmatch(lastRelease)

	if len(composerMatches) == 5 {
		info.Languages["php"] = LanguageReleaseInfo{
			Version:     composerMatches[1],
			URL:         composerMatches[2],
			PackageName: composerMatches[3],
			Path:        composerMatches[4],
		}
	}

	mavenMatches := mavenReleaseRegex.FindStringSubmatch(lastRelease)

	if len(mavenMatches) == 6 {
		groupID := mavenMatches[3]
		artifact := mavenMatches[4]
		info.Languages["java"] = LanguageReleaseInfo{
			Version:     mavenMatches[1],
			URL:         mavenMatches[2],
			PackageName: fmt.Sprintf(`%s.%s`, groupID, artifact),
			Path:        mavenMatches[5],
		}
	}

	return info, nil
}

func GetReleasesPath(dir string) string {
	return path.Join(environment.GetWorkspace(), "repo", dir, "RELEASES.md")
}
