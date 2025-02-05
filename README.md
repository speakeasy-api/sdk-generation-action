# Speakeasy SDK Generation Action & Workflows

The `sdk-generation-action` provides both an action and workflows to generate Client SDKs from an OpenAPI document using the [Speakeasy CLI tool](https://github.com/speakeasy-api/speakeasy). You can use these to manage CI/CD (ie the automatic generation and publishing of Client SDKs) in a repo containing the generated SDKs.

The included workflows provides option for publishing the SDKs to various package managers once the action is successful, either via PR or directly to the repo.

This action provides a self contained solution for automatically generating new versions of a client SDK when either the reference OpenAPI doc is updated or the Speakeasy CLI that is used to generate the SDKs is updated.

For more information please see our docsite linked below. 

## Workflow usage

### [Generation Workflow Documentation](https://www.speakeasy.com/docs/workflow-reference/generation-reference)

### [Publishing Workflow](https://www.speakeasy.com/docs/workflow-reference/publishing-reference)


# Developing in this Repo
To test
```
make test
```

This should open a PR at https://github.com/speakeasy-api/sdk-generation-action-test-repo

# When you're ready to test it as a live action
Push your changes up to a branch

Navigate to https://github.com/speakeasy-api/sdk-generation-action-test-repo/actions/workflows/action-test.yaml
in the test repo.

Click "run workflow" and put in your branch!
