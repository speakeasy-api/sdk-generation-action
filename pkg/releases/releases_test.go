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
		ReleaseVersion:         "1.2.3",
		OpenAPIDocVersion:      "9.8.7",
		OpenAPIDocPath:         "https://example.com",
		SpeakeasyVersion:       "6.6.6",
		NPMPackagePublished:    true,
		NPMPackageName:         "@org/package",
		NPMPackageUrl:          "https://www.npmjs.com/package/@org/package/v/1.2.3",
		PythonPackagePublished: true,
		PythonPackageName:      "org-package",
		PythonPackageURL:       "https://pypi.org/project/org-package/1.2.3",
		GoPackagePublished:     true,
		GoPackageURL:           "https://github.com/test/repo/releases/tag/v1.2.3",
	}

	info, err := releases.ParseReleases(r.String())
	assert.NoError(t, err)
	assert.Equal(t, r, *info)
}

func TestReleases_ReversableSerializationMultiple_Success(t *testing.T) {
	os.Setenv("GITHUB_REPOSITORY", "test/repo")

	r1 := releases.ReleasesInfo{
		ReleaseVersion:         "1.2.3",
		OpenAPIDocVersion:      "9.8.7",
		OpenAPIDocPath:         "https://example.com",
		SpeakeasyVersion:       "6.6.6",
		NPMPackagePublished:    true,
		NPMPackageName:         "@org/package",
		PythonPackagePublished: true,
		PythonPackageName:      "org-package",
		GoPackagePublished:     true,
	}

	r2 := releases.ReleasesInfo{
		ReleaseVersion:         "1.3.0",
		OpenAPIDocVersion:      "9.8.7",
		OpenAPIDocPath:         "https://example.com",
		SpeakeasyVersion:       "7.7.7",
		NPMPackagePublished:    true,
		NPMPackageName:         "@org/package",
		NPMPackageUrl:          "https://www.npmjs.com/package/@org/package/v/1.3.0",
		PythonPackagePublished: true,
		PythonPackageName:      "org-package",
		PythonPackageURL:       "https://pypi.org/project/org-package/1.3.0",
		GoPackagePublished:     true,
		GoPackageURL:           "https://github.com/test/repo/releases/tag/v1.3.0",
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
		ReleaseVersion:         "2.1.2",
		OpenAPIDocVersion:      "2.0",
		OpenAPIDocPath:         "https://vesselapi.github.io/yaml/openapi.yaml",
		SpeakeasyVersion:       "0.18.1",
		NPMPackagePublished:    true,
		NPMPackageName:         "@vesselapi/nodesdk",
		NPMPackageUrl:          "https://www.npmjs.com/package/@vesselapi/nodesdk/v/2.1.2",
		TypescriptPath:         "typescript-client-sdk",
		PythonPackagePublished: true,
		PythonPackageName:      "vesselapi",
		PythonPath:             "python-client-sdk",
		PythonPackageURL:       "https://pypi.org/project/vesselapi/2.1.2",
	}, *info)
}
