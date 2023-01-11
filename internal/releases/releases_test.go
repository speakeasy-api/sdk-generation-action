package releases

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReleases_ReversableSerialization_Success(t *testing.T) {
	r := ReleasesInfo{
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

	info, err := parseReleases(r.String())
	assert.NoError(t, err)
	assert.Equal(t, r, *info)
}

func TestReleases_ReversableSerializationMultiple_Success(t *testing.T) {
	r1 := ReleasesInfo{
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

	r2 := ReleasesInfo{
		ReleaseVersion:         "1.3.0",
		OpenAPIDocVersion:      "9.8.7",
		OpenAPIDocPath:         "https://example.com",
		SpeakeasyVersion:       "7.7.7",
		NPMPackagePublished:    true,
		NPMPackageName:         "@org/package",
		PythonPackagePublished: true,
		PythonPackageName:      "org-package",
		GoPackagePublished:     true,
	}

	info, err := parseReleases(r1.String() + r2.String())
	assert.NoError(t, err)
	assert.Equal(t, r2, *info)
}

func TestReleases_ParseVesselRelease_Success(t *testing.T) {
	releases := `

## Version 2.1.2
### Changes
Based on:
- OpenAPI Doc 2.0 https://vesselapi.github.io/yaml/openapi.yaml
- Speakeasy CLI 0.18.1 https://github.com/speakeasy-api/speakeasy
### Releases
- [NPM v2.1.2] https://www.npmjs.com/package/@vesselapi/nodesdk/v/2.1.2 - typescript-client-sdk
- [PyPI v2.1.2] https://pypi.org/project/vesselapi/2.1.2 - python-client-sdk
`

	info, err := parseReleases(releases)
	assert.NoError(t, err)
	assert.Equal(t, ReleasesInfo{
		ReleaseVersion:         "2.1.2",
		OpenAPIDocVersion:      "2.0",
		OpenAPIDocPath:         "https://vesselapi.github.io/yaml/openapi.yaml",
		SpeakeasyVersion:       "0.18.1",
		NPMPackagePublished:    true,
		NPMPackageName:         "@vesselapi/nodesdk",
		TypescriptPath:         "typescript-client-sdk",
		PythonPackagePublished: true,
		PythonPackageName:      "vesselapi",
		PythonPath:             "python-client-sdk",
	}, *info)
}
