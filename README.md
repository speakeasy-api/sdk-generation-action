# Speakeasy SDK Generation Action & Workflows

The `sdk-generation-action` provides both an action and workflows to generate Client SDKs from an OpenAPI document using the [Speakeasy CLI tool](https://github.com/speakeasy-api/speakeasy). You can use these to manage CI/CD (ie the automatic generation and publishing of Client SDKs) in a repo containing the generated SDKs.

You can find more information about our Client SDK Generator for OpenAPI Documents here: <https://docs.speakeasyapi.dev/docs/using-speakeasy/client-sdks/index.html>

The included workflows provides option for publishing the SDKs to various package managers once the action is successful, either via PR or directly to the repo.

This action provides a self contained solution for automatically generating new versions of a client SDK when either the reference OpenAPI doc is updated or the Speakeasy CLI that is used to generate the SDKs is updated.

The action can be used in a number of ways:

- Configured to run validation check on an OpenAPI doc, this is known as the `validate` action.  
- Configured to generate and commit directly to a branch in the repo (such as `main` or `master`). This mode is known as the `generate` action in `direct` mode.
- Configured to generate and commit to an auto-generated branch, then create a PR to merge the changes back into the main repo. This mode is known as the `generate` action in `pr` mode.
- Configured to apply suggestions to an OpenAPI doc and create a PR to merge the changes back into the main repo. This mode is known as the `suggest` action.

## Workflow usage

### Generation Workflow

The `.github/workflows/sdk-generation.yaml` workflow provides a reusable workflow for generating the SDKs. This workflow can be used to generate the SDKs and publish them to various package managers if using the `direct` mode of the action or to generate the SDKs and create a PR to merge the changes back into the main repo if using the `pr` mode of the action. If using `pr` mode you can then use the `.github/workflows/speakeasy_sdk_publish.yml` workflow to publish the SDKs to various package managers on merge into the main branch.

The workflow will also validate the provided OpenAPI document and provide addressable warnings and errors in the workflow output.

Below is example configuration of a workflow using the `pr` mode of the action:

```yaml
name: Generate

on:
  workflow_dispatch: # Allows manual triggering of the workflow to generate SDK
    inputs:
      force:
        description: "Force generation of SDKs"
        type: boolean
        default: false
  schedule:
    - cron: 0 0 * * * # Runs every day at midnight

jobs:
  generate:
    uses: speakeasy-api/sdk-generation-action/.github/workflows/sdk-generation.yaml@v14 # Import the sdk generation workflow which will handle the generation of the SDKs and publishing to the package managers in 'direct' mode.
    with:
      speakeasy_version: latest
      openapi_docs: |
        - location: https://docs.speakeasyapi.dev/openapi.yaml
      languages: |-
        - python
      publish_python: true # Tells the generation action to generate artifacts for publishing to PyPi
      mode: pr
      force: ${{ github.event.inputs.force }}
    secrets:
      speakeasy_api_key: ${{ secrets.SPEAKEASY_API_KEY }}
      github_access_token: ${{ secrets.GITHUB_TOKEN }}
      pypi_token: ${{ secrets.PYPI_TOKEN }}
```

### Publishing Workflow

When using the action or workflow in `pr` mode you can use the `.github/workflows/sdk-publish.yaml` workflow to publish the SDKs to various package managers on merge into the main branch, and create a Github release for the new SDK version.

Below is example configuration of a workflow using the `pr` mode of the action:

```yaml
name: Publish

on:
  on:
  push: # Will trigger when the RELEASES.md file is updated by the merged PR from the generation workflow
    paths:
      - 'RELEASES.md'
    branches:
      - main

jobs:
  publish:
    uses: speakeasy-api/sdk-generation-action/.github/workflows/sdk-publish.yaml@v14 # Import the sdk publish workflow which will handle the publishing to the package managers
    with:
      publish_python: true # Tells the publish action to publish the Python SDK to PyPi
      create_release: true
    secrets:
      github_access_token: ${{ secrets.GITHUB_TOKEN }}
      pypi_token: ${{ secrets.PYPI_TOKEN }}
```

### Validation Workflow

The `.github/workflows/sdk-generation.yaml` workflow also provides the ability to run the `validate` action to validate an OpenAPI document.

Below is example configuration of a workflow using the `validate` action:

```yaml
name: Validate

on:
  workflow_dispatch: {} # Allows manual triggering of the workflow to validate the OpenAPI doc
  schedule:
    - cron: 0 0 * * * # Runs every day at midnight

jobs:
  validate:
    uses: speakeasy-api/sdk-generation-action@v14 # Use the action directly which will handle the validation of the OpenAPI doc
    with:
      speakeasy_version: latest
      openapi_docs: |
        - location: https://docs.speakeasyapi.dev/openapi.yaml
      speakeasy_api_key: ${{ secrets.SPEAKEASY_API_KEY }}
      github_access_token: ${{ secrets.GITHUB_TOKEN }}
```

### Suggestion Workflow

The `.github/workflows/sdk-suggestion.yaml` workflow provides a reusable workflow for applying suggestions to the provided OpenAPI doc(s). This workflow can be used to create a PR to merge the changes containing the applied suggestions back into the main repo.

Below is example configuration of a workflow using the `suggest` action:

```yaml
name: Suggest

on:
  workflow_dispatch: {} # Allows manual triggering of the workflow to suggest OpenAPI document

jobs:
  suggest:
    uses: speakeasy-api/sdk-generation-action/.github/workflows/sdk-suggestion.yaml@v14 # Import the sdk suggestion workflow which will handle applying suggestions to the OpenAPI document and creating a resulting PR.
    with:
      speakeasy_version: latest
      openapi_docs: |
        - ./openapi.yaml
      openapi_doc_output: ./openapi.yaml
      max_suggestions: 5
    secrets:
      github_access_token: ${{ secrets.GITHUB_TOKEN }}
      speakeasy_api_key: ${{ secrets.SPEAKEASY_API_KEY }}
      openai_api_key: ${{ secrets.OPENAI_API_KEY }}
```

Note: `openapi_docs` can also be configured to take in multiple and remote inputs. The above example showcases a local file input.
`openapi_doc_output` may also be any local filepath. In this case, both values are the same, so the resulting PR will contain the changes to the original file.

## Generation

### Direct Mode

The action runs through the following steps:

- Downloads the latest (or pinned) version of the Speakeasy CLI
- Clones the associated repo
- Downloads or loads the latest OpenAPI doc from a url or the repo
- Validates the OpenAPI doc
- Checks for changes to the OpenAPI doc and the Speakeasy CLI Version
- Generates a new SDK for the configured languages if necessary
- Creates a commit with the new SDK(s) and pushes it to the repo
- Optionally creates a Github release for the new commit

### PR Mode

The action runs through the following steps:

- Downloads the latest (or pinned) version of the Speakeasy CLI
- Clones the associated repo
- Creates a branch (or updates an existing branch) for the new SDK version
- Downloads or loads the latest OpenAPI doc from a url or the repo
- Validates the OpenAPI doc
- Checks for changes to the OpenAPI doc and the Speakeasy CLI Version
- Generates a new SDK for the configured languages if necessary
- Creates a commit with the new SDK(s) and pushes it to the repo
- Creates a PR from the new branch to the main branch or updates an existing PR

## Publishing

Publishing is provided by using the included reusable workflows. These workflows can be used to publish the SDKs to various package managers. See below for more information.

### Java (Maven)

Java publishing is supported by publishing to a staging repository provider (OSSRH). In order to publish, you must do the following:

- If you've never published to Maven before, you must set up a staging repository (OSSRH). Follow the instructions [here](https://central.sonatype.org/publish/publish-guide/) to do so.
- You will need a GPG key to sign the artifacts. Follow the instructions [here](https://central.sonatype.org/publish/requirements/gpg/) to create one. An abbreviated guide is provided below.
  - Install gnupg on your machine (e.g. `brew install gnupg`)
  - Run `gpg --gen-key`. Note the keyId (e.g. `CA925CD6C9E8D064FF05B4728190C4130ABA0F98`) and shortId (last 8 characters of the keyId, e.g. `0ABA0F98`).
  - Run `gpg --keyserver keys.openpgp.org --send-keys <your_keyId>`
  - Run `gpg --export-secret-keys --armor <your_shortId> > secret_key.asc`
  - `secret_key.asc` will contain your GPG secret key
- Add your GPG secret key and passphrase as GitHub secrets
- Add your OSSRH (e.g. Sonatype) username and password as GitHub secrets
- Populate the `secrets` section of the workflow file with your secrets. For example:
  - `ossrh_username: ${{ secrets.OSSRH_USERNAME }}`
  - `ossrh_password: ${{ secrets.OSSRH_PASSWORD }}`
  - `java_gpg_secret_key: ${{ secrets.JAVA_GPG_SECRET_KEY }}`
  - `java_gpg_passphrase: ${{ secrets.JAVA_GPG_PASSPHRASE }}`
- In the workflow file, set `publish_java: true`
- In the `java` section of `gen.yaml`, ensure the groupId you've provided matches your OSSRH org and the artifact name you want. For example:
  - `groupID: com.example`
  - `artifactID: example-sdk`
- In the `java` section of `gen.yaml`, provide the additional configuration required for publishing to Maven. The below fields are required:
  - `ossrhURL: https://s01.oss.sonatype.org/service/local/staging/deploy/maven2/`
  - `githubURL: github.com/org/repo`
  - `companyName: My Company`
  - `companyURL: https://www.mycompany.com`
  - `companyEmail: info@mycompany.com`

### C# (Nuget)

C# publishing is supported by Nuget, an can be configured by following these instructions:

- You will need a Nuget API key to publish to nuget. 
  - Populate the `secrets` section of the workflow with `nuget_api_key: ${{ secrets.NUGET_API_KEY }}` (note: this assumes that the api key is set as a github action secret named `NUGET_API_KEY`).
  - A Nuget API key can be obtained by creating an account at [nuget.org](https://www.nuget.org).
    - When creating your Nuget API key, ensure that the `Package Owner` field is set to the user or organization that you would like to "own" your SDK artifact.
    - Ensure that the API key has the relevant `Push` scoped (if the package already exists, the api key may not need `Push new packages and package versions` permissions).
    - Ensure that the `Glob Pattern` and `Available Packages` fields are populated in a way that will permit publishing of your SDK (the `packageName` specified in `gen.yaml` is used).
- Add `publish_csharp: true` to the `with` section of both the `generation.yaml` and `publish.yaml` (if using in `pr` mode).

### Terraform Registry

Publishing a generated terraform provider is possible through the configuration of this action. In order to publish, you must do the following:

1. Ensure that the repository you made is called `terraform-provider-{NAME}`, where `NAME` is lowercase. The repository must be public. For this reason, terraform provider generation does not support operating in monorepo mode.
2. Create and export a signing key to sign your provider releases with. See [Github's documentation](https://docs.github.com/en/authentication/managing-commit-signature-verification/generating-a-new-gpg-key) for more information. This will need to be generated using either RSA or DSA algorithms. Take note of the following values
  1. The GPG private key.
  2. The GPG passphrase.
  3. The GPG Public Key
3. Add the ASCII-armored public key to the terraform registry.
4. Ensure that the following secrets are available to your repository. These will be configured automatically if entered into the speakeasy UI.
  1. `TERRAFORM_GPG_PRIVATE_KEY`: The GPG private key.
  2. `TERRAFORM_GPG_PASSPHRASE`: The GPG passphrase.
5. Once the initial release of your provider is complete (after executing this github action), you will need to manually add the provider to the terraform registry. Follow [the terraform registry instructions](https://registry.terraform.io/publish/provider) to begin this process, and agree to any Terraform Terms and Conditions. You will need to be an organizational admin to complete this step. This step needs only be performed once: subsequent updates will happen automatically.

## Validation

The action runs through the following steps:

- Downloads the latest (or pinned) version of the Speakeasy CLI
- Clones the associated repo
- Downloads or loads the latest OpenAPI doc from a url or the repo
- Validates the OpenAPI doc using the Speakeasy CLI Validation Tool
- Returns success or failure based on detected warnings/errors

## Suggestion

The suggestion action does the following:

- Downloads the latest or pinned version of the Speakeasy CLI.
- Clones the associated repo.
- Downloads or loads the latest OpenAPI doc from a URL or the repo.
- Generates suggestions for the OpenAPI doc, applies them, and outputs to a local filepath using the Speakeasy CLI.
- Creates a PR with this modified document.
- Adds PR comments containing the validation error for that line number, the suggested fix for that error, and an explanation of the fix.

## Inputs

### `speakeasy_api_key`

**Required** The Speakeasy API Key to use to authenticate the CLI run by the action. Create a new API Key in the [Speakeasy Platform](https://app.speakeasyapi.dev).

### `openai_api_key`

The OpenAI API Key to use to authenticate GPT requests issued by the `suggest` action. Create a new API Key in the [OpenAI Platform](https://platform.openai.com/account/api-keys).

### `action`

The action to run, valid options are `validate`, `generate`, `finalize`, `suggest`, `finalize-suggest`, or `release`, defaults to `generate`.

### `mode`

The mode to run the action in, valid options are `direct` or `pr`, defaults to `direct`.

- `direct` will create a commit with the changes to the SDKs and push them directly to the branch the workflow is configure to run on (normally 'main' or 'master'). If `create_release` is `true` this will happen immediately after the commit is created on the branch.
- `pr` will instead create a new branch to commit the changes to the SDKs to and then create a PR from this branch. The sdk-publish workflow will then need to be configured to run when the PR is merged to publish the SDKs and create a release.

### `speakeasy_version`

The version of the Speakeasy CLI to use or `"latest"`. Default `"latest"`.

### `openapi_docs`

**Required** A yaml string containing a list of OpenAPI documents to use, if multiple documents are provided they will be merged together, prior to generation.

If the document lives within the repo a relative path can be provided, if the document is hosted publicly a URL can be provided.

If the documents are hosted privately a URL can be provided along with the `openapi_doc_auth_header` and `openapi_doc_auth_token` inputs.
Each document will be fetched using the provided auth header and token, so they need to be valid for all documents.

Example:

```yaml
openapi_docs: |
  - https://example.com/openapi1.json
  - https://example.com/openapi2.json
```

### `openapi_doc_location`

**Required** The location of the OpenAPI document to use, either a relative path within the repo or a URL to a publicly hosted document.

### `openapi_doc_auth_header`

The auth header to use when fetching the OpenAPI document if it is not publicly hosted. For example `Authorization`. If using a private speakeasy hosted document use `x-api-key`. This header will be populated with the `openapi_doc_auth_token` provided.

### `openapi_doc_auth_token`

The auth token to use when fetching the OpenAPI document if it is not publicly hosted. For example `Bearer <token>` or `<token>`.

### `openapi_doc_output`

The path to output the modified OpenAPI spec to when running the `suggest` action. Defaults to `./openapi.yaml`.

### `max_suggestions`

The maximum number of suggestions to apply when running the `suggest` action. Defaults to `5`.

### `github_access_token`

**Required** A GitHub access token with write access to the repo.

### `languages`

**Required** A yaml string containing a list of languages to generate SDKs for example:

```yaml
languages: |
  - go: ./go-sdk # specifying a output directory
  - python # using default output of ./python-client-sdk
  - typescript # using default output of ./typescript-client-sdk
  - java # using default output of ./java-client-sdk
  - php # using default output of ./php-client-sdk
  - ruby # using default output of ./ruby-client-sdk
  - terraform # (single language repo only)
```

If multiple languages are present we will treat the repo as a mono repo, if a single language is present as a single language repo.

### `create_release`

Whether to create a release for the new SDK version if using `direct` mode. Default `"true"`.
This will also create a tag for the release, allowing the Go SDK to be retrieved via a tag with Go modules.

### `publish_python`

**(Workflow Only)** Whether to publish the Python SDK to PyPi. Default `"false"`.  
**Note**: Needs to be set in the generate and publish workflows if using `pr` mode.

### `publish_typescript`

**(Workflow Only)** Whether to publish the TypeScript SDK to NPM. Default `"false"`.
**Note**: Needs to be set in the generate and publish workflows if using `pr` mode.

### `publish_terraform`

**(Workflow Only)** Whether to publish the Terraform Provider to the Terraform Registry. Default `"false"`.

### `publish_java`

**(Workflow Only)** Whether to publish the Java SDK to the OSSRH URL configured in gen.yaml. Default `"false"`.

### `publish_php`

**(Workflow Only)** Whether to publish the PHP SDK for Composer. Default `"false"`.
**Note**: Needs to be set in the generate and publish workflows if using `pr` mode.

### `publish_ruby`

**(Workflow Only)** Whether to publish the Ruby SDK to Rubygems. Default `"false"`
**Note**: Needs to be set in the generate and publish workflows if using `pr` mode.

### `working_directory`
The working directory to use when running Speakeasy CLI commands in the action. If not specified, 
the root of the repo will be used.

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

### `java_regenerated`

`true` if the Java SDK was regenerated

### `java_directory`

The directory the Java SDK was generated in

### `php_regenerated`

`true` if the PHP SDK was regenerated

### `php_directory`

The directory the PHP SDK was generated in

### `terraform_regenerated`

`true` if the Terraform Provider was regenerated

### `terraform_directory`

The directory the Terraform Provider was generated in

### `ruby_regenerated`

`true` if the Ruby SDK was regenerated

### `ruby_directory`

The directory the Ruby SDK was generated in

### `cli_output`

The output of the Speakeasy CLI command used in the `suggest` action
