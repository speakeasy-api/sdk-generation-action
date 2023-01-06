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
