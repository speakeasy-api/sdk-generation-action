#!/usr/bin/env bash

ENV_FILE=$1

# Default environment variables not subject to change by different tests
export INPUT_DEBUG=true
export INPUT_OPENAPI_DOC_LOCATION="https://docs.speakeasyapi.dev/openapi.yaml"
export INPUT_GITHUB_ACCESS_TOKEN=${GITHUB_ACCESS_TOKEN}
export GITHUB_SERVER_URL="https://github.com"
export GITHUB_REPOSITORY_OWNER="speakeasy-api"
export GITHUB_REF="refs/heads/main"
export GITHUB_OUTPUT="./output.txt"
export GITHUB_WORKFLOW="test"

set -o allexport && source ${ENV_FILE} && set +o allexport

rm -rf ./repo || true
rm ./bin/speakeasy || true
rm output.txt || true
go run main.go