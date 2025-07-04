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
			"ruby": {
				PackageName: "org-package",
				Path:        "ruby",
				Version:     "1.2.3",
				URL:         "https://rubygems.org/gems/org-package/versions/1.2.3",
			},
			"csharp": {
				PackageName: "org.package",
				Path:        "csharp",
				Version:     "1.2.3",
				URL:         "https://www.nuget.org/packages/org.package/1.2.3",
			},
			"swift": {
				PackageName: "github.com/test/repo/swift",
				Path:        "swift",
				Version:     "1.2.3",
				URL:         "https://github.com/test/repo/releases/tag/swift/v1.2.3",
			},
		},
		LanguagesGenerated: map[string]releases.GenerationInfo{
			"typescript": {
				Path:    "typescript",
				Version: "1.2.3",
			},
			"python": {
				Path:    "python",
				Version: "1.2.3",
			},
			"go": {
				Path:    "go",
				Version: "1.2.3",
			},
			"php": {
				Path:    "php",
				Version: "1.2.3",
			},
			"java": {
				Path:    "java",
				Version: "1.2.3",
			},
			"terraform": {
				Path:    "terraform",
				Version: "0.0.5",
			},
			"ruby": {
				Path:    "ruby",
				Version: "1.2.3",
			},
			"csharp": {
				Path:    "csharp",
				Version: "1.2.3",
			},
			"swift": {
				Path:    "swift",
				Version: "1.2.3",
			},
		},
		LanguageChangelog: map[string]string{
			"typescript": "## Typescript SDK Changes Detected:\n* sdk.api.v1.users.profile.settings.updateAppearance(): **Added**\n* sdk.api.v1.users.profile.settings.getPrivacy(): **Deleted**\n* sdk.api.v1.users.profile.settings.getNotifications(): **Deprecated**\n* sdk.cloud.regions.clusters.nodes.getMetrics(): **Added** request.period [breaking]\n* sdk.admin.analytics.reports.revenue.daily.get_summary(): **Changed** response\n* sdk.organizations.projects.environments.deployments.logs.get(): **Added**\n* sdk.deleteAllUsers(): **Deleted**\n",
			"php":        "## Php SDK Changes Detected:\n* sdk->getuser(): **Added** request.x-api-key [breaking]\n",
			"go":         "## Go SDK Changes Detected:\n* sdk.UserService.GetProfile(): **Added**\n* sdk.UserService.DeleteAccount(): **Deleted**\n* sdk.UserService.UpdateSettings(): **Changed** request schema\n* sdk.AdminService.GenerateReport(): **Added** request.reportType [breaking]\n",
			"java":       "## Java SDK Changes Detected:\n* sdk.users.profile.Settings.updateAppearance(): **Added**\n* sdk.users.profile.Settings.getPrivacy(): **Deleted**\n* sdk.cloud.regions.clusters.nodes.getMetrics(): **Added** request.period [breaking]\n* sdk.admin.analytics.reports.revenue.daily.getSummary(): **Changed** response\n",
			"csharp":     "## Csharp SDK Changes Detected:\n* Sdk.Users.Profile.Settings.UpdateAppearance(): **Added**\n* Sdk.Users.Profile.Settings.GetPrivacy(): **Deleted**\n* Sdk.Cloud.Regions.Clusters.Nodes.GetMetrics(): **Added** request.period [breaking]\n* Sdk.Admin.Analytics.Reports.Revenue.Daily.GetSummary(): **Changed** response\n",
			"python":     "## Python SDK Changes Detected:\n* sdk.api.v1.users.profile.settings.update_appearance(): **Added**\n* sdk.api.v1.users.profile.settings.get_privacy(): **Deleted**\n* sdk.cloud.regions.clusters.nodes.get_metrics(): **Added** request.period [breaking]\n* sdk.admin.analytics.reports.revenue.daily.get_summary(): **Changed** response\n",
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
		LanguagesGenerated: map[string]releases.GenerationInfo{},
		LanguageChangelog:  map[string]string{},
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
			"ruby": {
				PackageName: "org-package",
				Path:        "ruby",
				Version:     "1.2.3",
				URL:         "https://rubygems.org/gems/org-package/versions/1.2.3",
			},
			"csharp": {
				PackageName: "org.package",
				Path:        "csharp",
				Version:     "1.2.3",
				URL:         "https://www.nuget.org/packages/org.package/1.2.3",
			},
			"swift": {
				PackageName: "github.com/test/repo/swift",
				Path:        "swift",
				Version:     "1.2.3",
				URL:         "https://github.com/test/repo/releases/tag/swift/v1.3.0",
			},
		},
		LanguagesGenerated: map[string]releases.GenerationInfo{},
		LanguageChangelog: map[string]string{
			"go":     "## Go SDK Changes Detected:\n* sdk.UserService.GetProfile(): **Added**\n* sdk.UserService.DeleteAccount(): **Deleted**\n* sdk.UserService.UpdateSettings(): **Changed** request schema\n* sdk.AdminService.GenerateReport(): **Added** request.reportType [breaking]\n",
			"java":   "## Java SDK Changes Detected:\n* sdk.users.profile.Settings.updateAppearance(): **Added**\n* sdk.users.profile.Settings.getPrivacy(): **Deleted**\n* sdk.cloud.regions.clusters.nodes.getMetrics(): **Added** request.period [breaking]\n* sdk.admin.analytics.reports.revenue.daily.getSummary(): **Changed** response\n",
			"csharp": "## Csharp SDK Changes Detected:\n* Sdk.Users.Profile.Settings.UpdateAppearance(): **Added**\n* Sdk.Users.Profile.Settings.GetPrivacy(): **Deleted**\n* Sdk.Cloud.Regions.Clusters.Nodes.GetMetrics(): **Added** request.period [breaking]\n* Sdk.Admin.Analytics.Reports.Revenue.Daily.GetSummary(): **Changed** response\n",
			"python": "## Python SDK Changes Detected:\n* sdk.api.v1.users.profile.settings.update_appearance(): **Added**\n* sdk.api.v1.users.profile.settings.get_privacy(): **Deleted**\n* sdk.cloud.regions.clusters.nodes.get_metrics(): **Added** request.period [breaking]\n* sdk.admin.analytics.reports.revenue.daily.get_summary(): **Changed** response\n",
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
			"ruby": {
				PackageName: "org-package",
				Path:        "ruby",
				Version:     "1.3.0",
				URL:         "https://rubygems.org/gems/org-package/versions/1.3.0",
			},
			"csharp": {
				PackageName: "org.package",
				Path:        "csharp",
				Version:     "1.3.0",
				URL:         "https://www.nuget.org/packages/org.package/1.3.0",
			},
			"swift": {
				PackageName: "github.com/test/repo/swift",
				Path:        "swift",
				Version:     "1.3.0",
				URL:         "https://github.com/test/repo/releases/tag/swift/v1.3.0",
			},
		},
		LanguagesGenerated: map[string]releases.GenerationInfo{
			"typescript": {
				Path:    "typescript",
				Version: "1.3.0",
			},
			"python": {
				Path:    "python",
				Version: "1.3.0",
			},
			"go": {
				Path:    "go",
				Version: "1.3.0",
			},
			"php": {
				Path:    "php",
				Version: "1.3.0",
			},
			"java": {
				Path:    "java",
				Version: "1.3.0",
			},
			"terraform": {
				Path:    "terraform",
				Version: "1.3.0",
			},
			"ruby": {
				Path:    "ruby",
				Version: "1.3.0",
			},
			"csharp": {
				Path:    "csharp",
				Version: "1.3.0",
			},
			"swift": {
				Path:    "swift",
				Version: "1.3.0",
			},
		},
		LanguageChangelog: map[string]string{
			"go":     "## Go SDK Changes Detected:\n* sdk.UserService.GetProfile(): **Added**\n* sdk.UserService.DeleteAccount(): **Deleted**\n* sdk.UserService.UpdateSettings(): **Changed** request schema\n* sdk.AdminService.GenerateReport(): **Added** request.reportType [breaking]\n",
			"java":   "## Java SDK Changes Detected:\n* sdk.users.profile.Settings.updateAppearance(): **Added**\n* sdk.users.profile.Settings.getPrivacy(): **Deleted**\n* sdk.cloud.regions.clusters.nodes.getMetrics(): **Added** request.period [breaking]\n* sdk.admin.analytics.reports.revenue.daily.getSummary(): **Changed** response\n",
			"csharp": "## Csharp SDK Changes Detected:\n* Sdk.Users.Profile.Settings.UpdateAppearance(): **Added**\n* Sdk.Users.Profile.Settings.GetPrivacy(): **Deleted**\n* Sdk.Cloud.Regions.Clusters.Nodes.GetMetrics(): **Added** request.period [breaking]\n* Sdk.Admin.Analytics.Reports.Revenue.Daily.GetSummary(): **Changed** response\n",
			"python": "## Python SDK Changes Detected:\n* sdk.api.v1.users.profile.settings.update_appearance(): **Added**\n* sdk.api.v1.users.profile.settings.get_privacy(): **Deleted**\n* sdk.cloud.regions.clusters.nodes.get_metrics(): **Added** request.period [breaking]\n* sdk.admin.analytics.reports.revenue.daily.get_summary(): **Changed** response\n",
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
		LanguagesGenerated: map[string]releases.GenerationInfo{},
		LanguageChangelog:  map[string]string{},
	}, *info)
}

func TestReleases_ParseReleaseWithSdkChangelog_Success(t *testing.T) {
	releasesStr := `

## Version 2.1.2
### Changes
## Typescript SDK Changes Detected:
* sdk.createUser(): **Added** request.email, **Changed** response [breaking]
## Go SDK Changes Detected:
* Sdk.Api.V1.Users.Profile.Settings.UpdateAppearance(): **Added**
* Sdk.Api.V1.Users.Profile.Settings.GetPrivacy(): **Added** request.Request
* Sdk.Api.V1.Users.Profile.Settings.GetNotifications(): **Deprecated**
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
		LanguagesGenerated: map[string]releases.GenerationInfo{},
		LanguageChangelog: map[string]string{
			"typescript": "## Typescript SDK Changes Detected:\n* sdk.createUser(): **Added** request.email, **Changed** response [breaking]\n",
			"go":         "## Go SDK Changes Detected:\n* Sdk.Api.V1.Users.Profile.Settings.UpdateAppearance(): **Added**\n* Sdk.Api.V1.Users.Profile.Settings.GetPrivacy(): **Added** request.Request\n* Sdk.Api.V1.Users.Profile.Settings.GetNotifications(): **Deprecated**\n",
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
		LanguagesGenerated: map[string]releases.GenerationInfo{},
		LanguageChangelog:  map[string]string{},
	}, *info)
}

func TestReleases_ParseCodatPreRelease_Success(t *testing.T) {
	releasesStr := `

## Version 1.1.0
### Changes
Based on:
- OpenAPI Doc v1 https://api.codat.io/swagger/v1/swagger.json
- Speakeasy CLI 0.21.0 https://github.com/speakeasy-api/speakeasy
### Releases
- [NPM v1.1.0-alpha] https://www.npmjs.com/package/@codatio/codat-ts/v/1.1.0-alpha - typescript-client-sdk
- [PyPI v1.1.0-beta.1] https://pypi.org/project/codatapi/1.1.0-beta.1 - python-client-sdk
- [Go v1.1.0-alpha.12] https://github.com/speakeasy-sdks/codat-sdks/releases/tag/v1.1.0-alpha.12 - go-client-sdk`

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
				Version:     "1.1.0-alpha",
				URL:         "https://www.npmjs.com/package/@codatio/codat-ts/v/1.1.0-alpha",
			},
			"python": {
				PackageName: "codatapi",
				Path:        "python-client-sdk",
				Version:     "1.1.0-beta.1",
				URL:         "https://pypi.org/project/codatapi/1.1.0-beta.1",
			},
			"go": {
				PackageName: "github.com/speakeasy-sdks/codat-sdks/go-client-sdk",
				Path:        "go-client-sdk",
				Version:     "1.1.0-alpha.12",
				URL:         "https://github.com/speakeasy-sdks/codat-sdks/releases/tag/v1.1.0-alpha.12",
			},
		},
		LanguagesGenerated: map[string]releases.GenerationInfo{},
		LanguageChangelog:  map[string]string{},
	}, *info)
}

func TestReleases_GenerateReleaseMd(t *testing.T) {
	releaseInfo := releases.ReleasesInfo{
		ReleaseTitle:      "2025-06-24 19:00:10",
		DocVersion:        "",
		DocLocation:       "",
		SpeakeasyVersion:  "0.0.1",
		GenerationVersion: "2.623.3",
		Languages:         map[string]releases.LanguageReleaseInfo{},
		LanguagesGenerated: map[string]releases.GenerationInfo{
			"typescript": {Version: "0.1.0", Path: "."},
		},
		LanguageChangelog: map[string]string{
			"typescript": "## Typescript SDK Changes Detected:\n* sdk.createUser(): **Added** request.email, **Changed** response [breaking]\n",
		},
	}

	expected := `

## 2025-06-24 19:00:10
### Changes
## Typescript SDK Changes Detected:
* sdk.createUser(): **Added** request.email, **Changed** response [breaking]
Based on:
- OpenAPI Doc  
- Speakeasy CLI 0.0.1 (2.623.3) https://github.com/speakeasy-api/speakeasy
### Generated
- [typescript v0.1.0] .`

	actual := releaseInfo.String()

	if actual != expected {
		t.Errorf("Expected:\n%s\nGot:\n%s", expected, actual)
	}
}

func TestReleases_GenerateReleaseMdForMultipleLanguages(t *testing.T) {
	releaseInfo := releases.ReleasesInfo{
		ReleaseTitle:      "2025-06-24 19:00:10",
		DocVersion:        "",
		DocLocation:       "",
		SpeakeasyVersion:  "0.0.1",
		GenerationVersion: "2.623.3",
		Languages:         map[string]releases.LanguageReleaseInfo{},
		LanguagesGenerated: map[string]releases.GenerationInfo{
			"typescript": {Version: "0.1.0", Path: "."},
			"go":         {Version: "0.1.0", Path: "."},
		},
		LanguageChangelog: map[string]string{
			"typescript": "## Typescript SDK Changes Detected:\n* sdk.createUser(): **Added** request.email, **Changed** response [breaking]\n",
			"go":         "## Go SDK Changes Detected:\n* Sdk.Api.V1.Users.Profile.Settings.UpdateAppearance(): **Added**\n* Sdk.Api.V1.Users.Profile.Settings.GetPrivacy(): **Added** request.Request\n* Sdk.Api.V1.Users.Profile.Settings.GetNotifications(): **Deprecated**\n",
		},
	}

	expected := `

## 2025-06-24 19:00:10
### Changes
## Typescript SDK Changes Detected:
* sdk.createUser(): **Added** request.email, **Changed** response [breaking]
## Go SDK Changes Detected:
* Sdk.Api.V1.Users.Profile.Settings.UpdateAppearance(): **Added**
* Sdk.Api.V1.Users.Profile.Settings.GetPrivacy(): **Added** request.Request
* Sdk.Api.V1.Users.Profile.Settings.GetNotifications(): **Deprecated**
Based on:
- OpenAPI Doc  
- Speakeasy CLI 0.0.1 (2.623.3) https://github.com/speakeasy-api/speakeasy
### Generated
- [typescript v0.1.0] .
- [go v0.1.0] .`
	actual := releaseInfo.String()

	if actual != expected {
		t.Errorf("\n**Expected**:\n%s\n\n**Got**:\n%s", expected, actual)
	}
}
func TestReleases_GenerateReleaseMdWhenNoSdkChangelogPresent(t *testing.T) {
	releaseInfo := releases.ReleasesInfo{
		ReleaseTitle:      "2025-06-24 19:00:10",
		DocVersion:        "",
		DocLocation:       "",
		SpeakeasyVersion:  "0.0.1",
		GenerationVersion: "2.623.3",
		Languages:         map[string]releases.LanguageReleaseInfo{},
		LanguagesGenerated: map[string]releases.GenerationInfo{
			"typescript": {Version: "0.1.0", Path: "."},
			"go":         {Version: "0.1.0", Path: "."},
		},
		LanguageChangelog: map[string]string{},
	}

	expected := `

## 2025-06-24 19:00:10
### Changes
Based on:
- OpenAPI Doc  
- Speakeasy CLI 0.0.1 (2.623.3) https://github.com/speakeasy-api/speakeasy
### Generated
- [typescript v0.1.0] .
- [go v0.1.0] .`

	actual := releaseInfo.String()

	if actual != expected {
		t.Errorf("\n**Expected**:\n%s\n\n**Got**:\n%s", expected, actual)
	}
}
