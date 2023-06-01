package releases_test

import (
	"os"
	"testing"

	"github.com/speakeasy-api/sdk-generation-action/pkg/releases"
	"github.com/stretchr/testify/assert"
)

func TestReleases_ReversableSerialization_Success(t *testing.T) {
	os.Setenv("GITHUB_REPOSITORY", "test/repo")

	r := releases.ReleasesInfo{
		ReleaseTitle:      "2023-02-22",
		DocVersion:        "9.8.7",
		DocLocation:       "https://example.com",
		SpeakeasyVersion:  "6.6.6",
		GenerationVersion: "v7.7.7",
		Languages: map[string]releases.LanguageReleaseInfo{
			"typescript": {
				PackageName: "@org/package",
				Path:        "typescript",
				Version:     "1.2.3",
				URL:         "https://www.npmjs.com/package/@org/package/v/1.2.3",
			},
			"python": {
				PackageName: "org-package",
				Path:        "python",
				Version:     "1.2.3",
				URL:         "https://pypi.org/project/org-package/1.2.3",
			},
			"go": {
				PackageName: "github.com/test/repo/go",
				Path:        "go",
				Version:     "1.2.3",
				URL:         "https://github.com/test/repo/releases/tag/go/v1.2.3",
			},
			"php": {
				PackageName: "org/package",
				Path:        "php",
				Version:     "1.2.3",
				URL:         "https://packagist.org/packages/org/package#v1.2.3",
			},
			"java": {
				PackageName: "com.group.artifact",
				Path:        "java",
				Version:     "1.2.3",
				URL:         "https://central.sonatype.com/artifact/com.group/artifact/1.2.3",
			},
			"terraform": {
				PackageName: "speakeasy-api/speakeasy",
				Path:        "terraform",
				Version:     "0.0.5",
				URL:         "https://registry.terraform.io/providers/speakeasy-api/speakeasy/0.0.5",
			},
		},
	}

	info, err := releases.ParseReleases(r.String())
	assert.NoError(t, err)
	assert.Equal(t, r, *info)
}

func TestReleases_GoPackageNameConstruction_Success(t *testing.T) {
	os.Setenv("GITHUB_REPOSITORY", "test/repo")

	r := releases.ReleasesInfo{
		ReleaseTitle:     "2023-02-22",
		DocVersion:       "9.8.7",
		DocLocation:      "https://example.com",
		SpeakeasyVersion: "6.6.6",
		Languages: map[string]releases.LanguageReleaseInfo{
			"go": {
				PackageName: "github.com/test/repo",
				Path:        ".",
				Version:     "1.2.3",
				URL:         "https://github.com/test/repo/releases/tag/v1.2.3",
			},
		},
	}

	info, err := releases.ParseReleases(r.String())
	assert.NoError(t, err)
	assert.Equal(t, r, *info)
}

func TestReleases_ReversableSerializationMultiple_Success(t *testing.T) {
	os.Setenv("GITHUB_REPOSITORY", "test/repo")

	r1 := releases.ReleasesInfo{
		ReleaseTitle:     "Version 1.2.3",
		DocVersion:       "9.8.7",
		DocLocation:      "https://example.com",
		SpeakeasyVersion: "6.6.6",
		Languages: map[string]releases.LanguageReleaseInfo{
			"typescript": {
				PackageName: "@org/package",
				Path:        "typescript",
				Version:     "1.2.3",
				URL:         "https://www.npmjs.com/package/@org/package/v/1.2.3",
			},
			"python": {
				PackageName: "org-package",
				Path:        "python",
				Version:     "1.2.3",
				URL:         "https://pypi.org/project/org-package/1.2.3",
			},
			"go": {
				PackageName: "github.com/test/repo/go",
				Path:        "go",
				Version:     "1.2.3",
				URL:         "https://github.com/test/repo/releases/tag/go/v1.2.3",
			},
			"php": {
				PackageName: "org/package",
				Version:     "1.2.3",
			},
			"terraform": {
				PackageName: "speakeasy-api/speakeasy",
				Path:        "terraform",
				Version:     "1.2.3",
				URL:         "https://registry.terraform.io/providers/speakeasy-api/speakeasy/1.2.3",
			},
			"java": {
				PackageName: "com.group.artifact",
				Path:        "java",
				Version:     "1.2.3",
				URL:         "https://central.sonatype.com/artifact/com.group/artifact/1.2.3",
			},
		},
	}

	r2 := releases.ReleasesInfo{
		ReleaseTitle:     "1.3.0",
		DocVersion:       "9.8.7",
		DocLocation:      "https://example.com",
		SpeakeasyVersion: "7.7.7",
		Languages: map[string]releases.LanguageReleaseInfo{
			"typescript": {
				PackageName: "@org/package",
				Path:        "typescript",
				Version:     "1.3.0",
				URL:         "https://www.npmjs.com/package/@org/package/v/1.3.0",
			},
			"python": {
				PackageName: "org-package",
				Path:        "python",
				Version:     "1.3.0",
				URL:         "https://pypi.org/project/org-package/1.3.0",
			},
			"go": {
				PackageName: "github.com/test/repo/go",
				Path:        "go",
				Version:     "1.3.0",
				URL:         "https://github.com/test/repo/releases/tag/go/v1.3.0",
			},
			"php": {
				PackageName: "org/package",
				Path:        "php",
				Version:     "1.3.0",
				URL:         "https://packagist.org/packages/org/package#v1.3.0",
			},
			"java": {
				PackageName: "com.group.artifact",
				Path:        "java",
				Version:     "1.3.0",
				URL:         "https://central.sonatype.com/artifact/com.group/artifact/1.3.0",
			},
			"terraform": {
				PreviousVersion: "1.2.3",
				PackageName:     "speakeasy-api/speakeasy",
				Path:            "terraform",
				Version:         "1.3.0",
				URL:             "https://registry.terraform.io/providers/speakeasy-api/speakeasy/1.3.0",
			},
		},
	}

	info, err := releases.ParseReleases(r1.String() + r2.String())
	assert.NoError(t, err)
	assert.Equal(t, r2, *info)
}

func TestReleases_ParseVesselRelease_Success(t *testing.T) {
	releasesStr := `

## Version 2.1.2
### Changes
Based on:
- OpenAPI Doc 2.0 https://vesselapi.github.io/yaml/openapi.yaml
- Speakeasy CLI 0.18.1 https://github.com/speakeasy-api/speakeasy
### Releases
- [NPM v2.1.2] https://www.npmjs.com/package/@vesselapi/nodesdk/v/2.1.2 - typescript-client-sdk
- [PyPI v2.1.2] https://pypi.org/project/vesselapi/2.1.2 - python-client-sdk
`

	info, err := releases.ParseReleases(releasesStr)
	assert.NoError(t, err)
	assert.Equal(t, releases.ReleasesInfo{
		ReleaseTitle:     "Version 2.1.2",
		DocVersion:       "2.0",
		DocLocation:      "https://vesselapi.github.io/yaml/openapi.yaml",
		SpeakeasyVersion: "0.18.1",
		Languages: map[string]releases.LanguageReleaseInfo{
			"typescript": {
				PackageName: "@vesselapi/nodesdk",
				Path:        "typescript-client-sdk",
				Version:     "2.1.2",
				URL:         "https://www.npmjs.com/package/@vesselapi/nodesdk/v/2.1.2",
			},
			"python": {
				PackageName: "vesselapi",
				Path:        "python-client-sdk",
				Version:     "2.1.2",
				URL:         "https://pypi.org/project/vesselapi/2.1.2",
			},
		},
	}, *info)
}

func TestReleases_ParseCodatRelease_Success(t *testing.T) {
	releasesStr := `

## Version 1.1.0
### Changes
Based on:
- OpenAPI Doc v1 https://api.codat.io/swagger/v1/swagger.json
- Speakeasy CLI 0.21.0 https://github.com/speakeasy-api/speakeasy
### Releases
- [NPM v1.1.0] https://www.npmjs.com/package/@codatio/codat-ts/v/1.1.0 - typescript-client-sdk
- [PyPI v1.1.0] https://pypi.org/project/codatapi/1.1.0 - python-client-sdk
- [Go v1.1.0] https://github.com/speakeasy-sdks/codat-sdks/releases/tag/v1.1.0 - go-client-sdk`

	info, err := releases.ParseReleases(releasesStr)
	assert.NoError(t, err)
	assert.Equal(t, releases.ReleasesInfo{
		ReleaseTitle:     "Version 1.1.0",
		DocVersion:       "v1",
		DocLocation:      "https://api.codat.io/swagger/v1/swagger.json",
		SpeakeasyVersion: "0.21.0",
		Languages: map[string]releases.LanguageReleaseInfo{
			"typescript": {
				PackageName: "@codatio/codat-ts",
				Path:        "typescript-client-sdk",
				Version:     "1.1.0",
				URL:         "https://www.npmjs.com/package/@codatio/codat-ts/v/1.1.0",
			},
			"python": {
				PackageName: "codatapi",
				Path:        "python-client-sdk",
				Version:     "1.1.0",
				URL:         "https://pypi.org/project/codatapi/1.1.0",
			},
			"go": {
				PackageName: "github.com/speakeasy-sdks/codat-sdks/go-client-sdk",
				Path:        "go-client-sdk",
				Version:     "1.1.0",
				URL:         "https://github.com/speakeasy-sdks/codat-sdks/releases/tag/v1.1.0",
			},
		},
	}, *info)
}
