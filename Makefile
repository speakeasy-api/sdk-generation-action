.PHONY: *

test-direct-mode:
	./testing/test.sh ./testing/direct-mode.env

test-direct-mode-multi-sdk:
	./testing/test.sh ./testing/direct-mode-multi-sdk.env

test-pr-mode:
	./testing/test.sh ./testing/pr-mode.env

test-push-code-samples-only:
	./testing/test.sh ./testing/push-code-samples-only.env

test-release-mode:
	./testing/test.sh ./testing/release-mode.env

test-release-mode-multi-sdk:
	./testing/test.sh ./testing/release-mode-multi-sdk.env

test-validate-action:
	./testing/test.sh ./testing/validate-action.env

test-overlay:
	./testing/test.sh ./testing/overlay-test.env