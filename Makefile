local:
	./scripts/local.sh

local-pr:
	./scripts/local.sh pr

local-release:
	INPUT_CREATE_RELEASE=true ./scripts/local.sh release