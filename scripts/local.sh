#!/usr/bin/env bash

MODE=$1

export INPUT_DEBUG=true
export INPUT_MODE=${MODE}
export INPUT_OPENAPI_DOC_LOCATION="https://docs.speakeasyapi.dev/openapi.yaml"
export INPUT_LANGUAGES="- go"
export INPUT_GITHUB_ACCESS_TOKEN=${GITHUB_ACCESS_TOKEN}
export INPUT_CREATE_RELEASE=true
export GITHUB_SERVER_URL="https://github.com"
export GITHUB_REPOSITORY_OWNER="speakeasy-api"
export GITHUB_REPOSITORY="speakeasy-api/sdk-generation-action-test-repo"

export GITHUB_REF="refs/heads/main"
export GITHUB_OUTPUT="./output.txt"

# Retaining this for now, in case we want to use .env files
#set -a
#source <(cat .env | sed -e '/^#/d;/^\s*$/d' -e "s/'/'\\\''/g" -e "s/=\(.*\)/='\1'/g")
#set +a

rm -rf ./repo || true
rm ./bin/speakeasy || true
rm output.txt || true
go run main.go