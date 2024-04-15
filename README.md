# Speakeasy SDK Generation Action & Workflows

The `sdk-generation-action` provides both an action and workflows to generate Client SDKs from an OpenAPI document using the [Speakeasy CLI tool](https://github.com/speakeasy-api/speakeasy). You can use these to manage CI/CD (ie the automatic generation and publishing of Client SDKs) in a repo containing the generated SDKs.

The included workflows provides option for publishing the SDKs to various package managers once the action is successful, either via PR or directly to the repo.

This action provides a self contained solution for automatically generating new versions of a client SDK when either the reference OpenAPI doc is updated or the Speakeasy CLI that is used to generate the SDKs is updated.

The action can be used in a number of ways:

- Configured to run validation check on an OpenAPI doc, this is known as the `validate` action.
- Configured to generate and commit directly to a branch in the repo (such as `main` or `master`). This mode is known as the `generate` action in `direct` mode.
- Configured to generate and commit to an auto-generated branch, then create a PR to merge the changes back into the main repo. This mode is known as the `generate` action in `pr` mode.
- Configured to apply suggestions to an OpenAPI doc and create a PR to merge the changes back into the main repo. This mode is known as the `suggest` action.

For more information please see our docsite linked below. 

## Workflow usage

### [Generation Workflow Documentation](https://www.speakeasyapi.dev/docs/workflow-reference/generation-reference)

### [Publishing Workflow](https://www.speakeasyapi.dev/docs/workflow-reference/publishing-reference)

