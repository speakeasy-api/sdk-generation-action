#!/usr/bin/env bash

function run_action() {
    rm -rf ./repo || true
    rm ./bin/speakeasy || true
    go run main.go
}

# Default environment variables not subject to change by different tests
export INPUT_DEBUG=true
export INPUT_OPENAPI_DOCS=$(cat <<EOF
- openapi-invalid.yaml
EOF
)
export SPEAKEASY_ENVIRONMENT=local
export GITHUB_WORKSPACE="./"

set -o allexport && source ./testing/suggestions.env && set +o allexport

export INPUT_ACTION="validate"
export INPUT_WRITE_SUGGESTIONS="true"
run_action