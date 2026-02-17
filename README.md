<div align="center">
 <a href="https://www.speakeasy.com/" target="_blank">
  <img width="1500" height="500" alt="Speakeasy" src="https://github.com/user-attachments/assets/0e56055b-02a3-4476-9130-4be299e5a39c" />
 </a>
 <br />
 <br />
  <div>
   <a href="https://speakeasy.com/docs/create-client-sdks/" target="_blank"><b>Docs Quickstart</b></a>&nbsp;&nbsp;//&nbsp;&nbsp;<a href="https://go.speakeasy.com/slack" target="_blank"><b>Join us on Slack</b></a>
  </div>
 <br />

</div>

# Speakeasy SDK Generation Action & Workflows

> [!TIP]
> If you are a first-time user of Speakeasy and want to start setup from scratch, please see our [Quickstart Guide](https://www.speakeasy.com/docs/introduction). Following our quickstart guide will automatically setup this action and workflows for your API.

The `sdk-generation-action` provides both an action and workflows to generate Client SDKs from an OpenAPI document using the [Speakeasy CLI tool](https://github.com/speakeasy-api/speakeasy). You can use these to manage CI/CD (ie the automatic generation and publishing of Client SDKs) in a repo containing the generated SDKs.

The included workflows provide options for publishing the SDKs to various package managers once the action is successful, either via PR or directly to the repo.

This action provides a self-contained solution for automatically generating new versions of a client SDK when either the reference OpenAPI doc is updated or the Speakeasy CLI that is used to generate the SDKs is updated.

Configuration for the supported workflows is documented in [separate repository](https://github.com/speakeasy-api/sdk-gen-config). 

## Workflow Reference Documentation

### [Generation Workflow](https://www.speakeasy.com/docs/workflow-reference/generation-reference)

### [Publishing Workflow](https://www.speakeasy.com/docs/workflow-reference/publishing-reference)

# Developing in this Repo
To test
```
make test
```

This should open a PR on the [test action repository](https://github.com/speakeasy-api/sdk-generation-action-test-repo).

# When you're ready to test it as a live action

1) Push your changes up to a branch.
2) Navigate to the [test action](https://github.com/speakeasy-api/sdk-generation-action-test-repo/actions/workflows/action-test.yaml) in the test repo.
3) Click "run workflow" and put in your branch!

# Releasing

1. Merge your changes to `main`.
2. Run `./scripts/update-refs.sh` to pin all internal workflow refs to the latest commit SHA on `main`. Get this merged to `main` via a PR.
3. Once the refs PR is merged, tag `main` with the appropriate semver tag and create a release on GitHub:
   ```bash
   git tag v15.x.x
   git push --tags
   gh release create v15.x.x --generate-notes
   ```
4. Update the rolling major version tag:
   ```bash
   git tag -f v15 && git push --force --tags
   ```
