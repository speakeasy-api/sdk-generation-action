#!/usr/bin/env bash

ENV_FILE=$1

function run_action() {
    rm -rf ./repo || true
    rm -f ./bin/speakeasy || true
    go run main.go
}

# Default environment variables not subject to change by different tests
#export INPUT_SPEAKEASY_VERSION="v1.240.0" # Uncomment to test specific versions otherwise uses latest

if [ "$RUN_FINALIZE" = "true" ]; then
    BRANCH_NAME=$(go run testing/getoutput.go -output branch_name)
    PREVIOUS_GEN_VERSION=$(go run testing/getoutput.go -output previous_gen_version)
    export INPUT_BRANCH_NAME=${BRANCH_NAME}
    export INPUT_PREVIOUS_GEN_VERSION=${PREVIOUS_GEN_VERSION}
fi

set -o allexport && source ${ENV_FILE} && set +o allexport

rm -f output.txt
INPUT_ACTION="run-workflow"
run_action
