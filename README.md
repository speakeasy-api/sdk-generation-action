<div align="center">
 <a href="https://www.speakeasy.com/" target="_blank">
   <picture>
       <source media="(prefers-color-scheme: light)" srcset="https://github.com/user-attachments/assets/21dd5d3a-aefc-4cd3-abee-5e17ef1d4dad">
       <source media="(prefers-color-scheme: dark)" srcset="https://github.com/user-attachments/assets/0a747f98-d228-462d-9964-fd87bf93adc5">
       <img width="100px" src="https://github.com/user-attachments/assets/21dd5d3a-aefc-4cd3-abee-5e17ef1d4dad#gh-light-mode-only" alt="Speakeasy">
   </picture>
 </a>
  <h1>Speakeasy</h1>
  <p>Build APIs your users love ❤️ with Speakeasy</p>
  <div>
   <a href="https://www.speakeasy.com/docs/introduction" target="_blank"><b>Docs Quickstart</b></a>&nbsp;&nbsp;//&nbsp;&nbsp;<a href="https://join.slack.com/t/speakeasy-dev/shared_invite/zt-1cwb3flxz-lS5SyZxAsF_3NOq5xc8Cjw" target="_blank"><b>Join us on Slack</b></a>
  </div>
 <br />

# Speakeasy SDK Generation Action & Workflows

> [!TIP]
> If you are are a first time user of Speakeasy and want to start setup from scratch, please see our [Quickstart Guide](https://www.speakeasy.com/docs/introduction). Following our quickstart guide will automatically setup this action and workflows for your API.

The `sdk-generation-action` provides both an action and workflows to generate Client SDKs from an OpenAPI document using the [Speakeasy CLI tool](https://github.com/speakeasy-api/speakeasy). You can use these to manage CI/CD (ie the automatic generation and publishing of Client SDKs) in a repo containing the generated SDKs.

The included workflows provides option for publishing the SDKs to various package managers once the action is successful, either via PR or directly to the repo.

This action provides a self contained solution for automatically generating new versions of a client SDK when either the reference OpenAPI doc is updated or the Speakeasy CLI that is used to generate the SDKs is updated.

Configuration for the supported workflows is documented in [seperate repository](https://github.com/speakeasy-api/sdk-gen-config). 

## Workflow Reference Documentation

### [Generation Workflow](https://www.speakeasy.com/docs/workflow-reference/generation-reference)

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
