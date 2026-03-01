.PHONY: *

test: test-pr-mode
	echo "PR mode ran succesfully, check https://github.com/speakeasy-api/sdk-generation-action-test-repo/ to ensure there's a PR created"

test-direct-mode:
	./testing/test.sh ./testing/direct-mode.env

test-direct-mode-multi-sdk:
	./testing/test.sh ./testing/direct-mode-multi-sdk.env

test-pr-mode:
	docker compose run --rm main ./testing/test.sh ./testing/pr-mode.env

test-pr-mode-granular-commits:
	docker compose run --rm main ./testing/test.sh ./testing/pr-mode-granular-commits.env

test-pr-mode-signed-commits:
	docker compose run --rm main ./testing/test.sh ./testing/pr-mode-signed-commits.env

test-push-code-samples-only:
	docker compose run --rm main ./testing/test.sh ./testing/push-code-samples-only.env

test-release-mode:
	docker compose run --rm main ./testing/test.sh ./testing/release-mode.env

test-release-mode-multi-sdk:
	docker compose run --rm main ./testing/test.sh ./testing/release-mode-multi-sdk.env

test-validate-action:
	docker compose run --rm main ./testing/test.sh ./testing/validate-action.env

test-overlay:
	docker compose run --rm main ./testing/test.sh ./testing/overlay-test.env

test-manual-repo-url:
	docker compose run --rm main ./testing/test.sh ./testing/manual-repo-url.env

# Integration tests run the full workflow E2E against a real GitHub repo.
# They require the following environment variables:
#
#   GITHUB_TOKEN       GitHub personal access token (or `gh auth token`)
#                      with repo scope on speakeasy-api/sdk-generation-action-test-repo
#
#   SPEAKEASY_API_KEY  Speakeasy platform API key for SDK generation
#
# Example:
#   export GITHUB_TOKEN=$(gh auth token)
#   export SPEAKEASY_API_KEY=ey...
#   make test-integration
test-integration:
ifndef GITHUB_TOKEN
	$(error GITHUB_TOKEN is not set — export a GitHub token with repo scope, e.g. export GITHUB_TOKEN=$$(gh auth token))
endif
ifndef SPEAKEASY_API_KEY
	$(error SPEAKEASY_API_KEY is not set — export your Speakeasy API key, e.g. export SPEAKEASY_API_KEY=ey...)
endif
	docker compose run --rm -e SPEAKEASY_ACCEPTANCE=1 main go test -v -tags=integration -timeout 900s ./integration_test/...
