# Speakeasy SDK Generation Action & Workflows

The `sdk-generation-action` provides both an action and a workflow to generate Client SDKs from an OpenAPI document using the [Speakeasy CLI tool](https://github.com/speakeasy-api/speakeasy). You can use these to manage CI/CD (ie the automatic generation and publishing of Client SDKs) in a repo containing the generated SDKs.

You can find more information about our Client SDK Generator for OpenAPI Documents here: https://docs.speakeasyapi.dev/docs/using-speakeasy/client-sdks/index.html

The included workflow provides options for publishing the SDKs to various package managers once the action is successful.

This action provides a self contained solution for automatically generating new versions of a client SDK either when either the reference OpenAPI doc is updated or the Speakeasy CLI that is used to generate the SDKs is updated.

The action runs through the following steps:

- Downloads the latest (or pinned) version of the Speakeasy CLI
- Clones the associated repo
- Downloads or loads the latest OpenAPI doc from a url or the repo
- Checks for changes to the OpenAPI doc and the Speakeasy CLI Version
- Generates a new SDK for the configured languages if necessary
- Creates a commit with the new SDK(s) and pushes it to the repo
- Optionally creates a Github release for the new commit

## Inputs

### `speakeasy_version`

The version of the Speakeasy CLI to use or `"latest"`. Default `"latest"`.

### `openapi_doc_location`

**Required** The location of the OpenAPI document to use, either a relative path within the repo or a URL to a publicly hosted document.

### `github_access_token`

**Required** A GitHub access token with write access to the repo.

### `languages`

**Required** A yaml string containing a list of languages to generate SDKs for example:

```yaml
languages: |
  - go: ./go-sdk # specifying a output directory
  - python # using default output of ./python-client-sdk
  - typescript # using default output of ./typescript-client-sdk
```

If multiple languages are present we will treat the repo as a mono repo, if a single language is present as a single language repo.

### `create_release`

Whether to create a release for the new SDK version. Default `"true"`.
This will also create a tag for the release, allowing the Go SDK to be retrieved via a tag with Go modules.

### `publish_python`

**(Workflow Only)** Whether to publish the Python SDK to PyPi. Default `"false"`.

### `publish_typescript`

**(Workflow Only)** Whether to publish the TypeScript SDK to NPM. Default `"false"`.

## Outputs

### `python_regenerated`

`true` if the Python SDK was regenerated

### `python_directory`

The directory the Python SDK was generated in

### `typescript_regenerated`

`true` if the Typescript SDK was regenerated

### `typescript_directory`

The directory the Typescript SDK was generated in

### `go_regenerated`

`true` if the Go SDK was regenerated

### `go_directory`

The directory the Go SDK was generated in

## Example Workflow usage

`.github/workflows/speakeasy_sdk_generation.yml`

```yaml
name: Generate

on:
  workflow_dispatch: {} # Allows manual triggering of the workflow to generate SDK
  schedule:
    - cron: 0 0 * * * # Runs every day at midnight

jobs:
  generate:
    uses: speakeasy-api/sdk-generation-action/.github/workflows/sdk-generation.yaml@v4 # Import the sdk generation workflow which will handle the generation of the SDKs and publishing to the package managers
    with:
      speakeasy_version: latest
      openapi_doc_location: https://docs.speakeasyapi.dev/openapi.yaml
      languages: |-
        - python
      publish_python: true
    secrets:
      github_access_token: ${{ secrets.GITHUB_TOKEN }}
      pypi_token: ${{ secrets.PYPI_TOKEN }}
```

## Example Action usage

`.github/workflows/speakeasy_sdk_generation.yml`

```yaml
name: Generate

on:
  workflow_dispatch: {} # Allows manual triggering of the workflow to generate SDK
  schedule:
    - cron: 0 0 * * * # Runs every day at midnight

jobs:
  generate:
    name: Generate SDK
    runs-on: ubuntu-latest
    steps:
      - uses: speakeasy-api/sdk-generation-action@v4
        with:
          speakeasy_version: latest
          openapi_doc_location: https://docs.speakeasyapi.dev/openapi.yaml
          github_access_token: ${{ secrets.GITHUB_TOKEN }}
          languages: |-
            - go
```
