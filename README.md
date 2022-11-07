# Speakeasy SDK Generation Action

This action provides a self contained solution for automatically generating new versions of a client SDK either when the reference OpenAPI doc is updated or the Speakeasy CLI that is used to generate the SDKs is updated.

The action runs through the following steps:
- Downloads the latest (or pinned) version of the Speakeasy CLI
- Clones the Repo
- Downloads or loads the latest OpenAPI doc from a url or the repo
- Checks for changes to the OpenAPI doc and the Speakeasy CLI Version
- Generates a new SDK for the configured languages if necessary
- Creates a commit with the new SDK(s) and pushes it to the repo
- Publishes the new SDK(s) to the configured package manager (coming soon.)

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

If multiple languages are present we will treat this repo as a mono repo, if a single language is present as a single language repo.

### `create_release`

Whether to create a release for the new SDK version. Default `"true"`.

## Outputs

### `python_regenerated`
    
`true` if the Python SDK was regenerated

## Example usage

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
      - uses: speakeasy-api/sdk-generation-action@v2.4
        with:
          speakeasy_version: latest
          openapi_doc_location: https://docs.speakeasyapi.dev/openapi.yaml
          github_access_token: ${{ secrets.GITHUB_TOKEN }}
          languages: |-
            - go
```