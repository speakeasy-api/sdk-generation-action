#!/usr/bin/env bash
set -euo pipefail

# Get the current commit SHA
CURRENT_SHA=$(git rev-parse HEAD)

echo "Updating refs to: $CURRENT_SHA"

# Process each workflow file and composite action
for file in .github/workflows/*.yaml .github/workflows/*.yml publish-pypi/action.yml; do
  [[ -f "$file" ]] || continue

  echo "Processing: $file"

  yq -i "
    # Update ref for checkout steps that check out speakeasy-api/sdk-generation-action
    (.. | select(has(\"steps\")).steps[] |
      select(.uses != null and (.uses | test(\"^actions/checkout\")) and .with.repository == \"speakeasy-api/sdk-generation-action\")
    ).with.ref = \"$CURRENT_SHA\"
    |
    # Update GH_ACTION_VERSION env var in any step that has it
    (.. | select(has(\"steps\")).steps[] |
      select(.env.GH_ACTION_VERSION != null)
    ).env.GH_ACTION_VERSION = \"$CURRENT_SHA\"
  " "$file"
done

echo "Done!"
