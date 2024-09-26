package actions

import (
	"fmt"
	"os/exec"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
)

// SetupEnvironment will install runtime environment dependencies.
//
// For example if pnpm is desired instead of npm for target compilation and
// publishing, then an input (pnpm_version in this case) should be set to a
// non-empty value and this logic will install the dependency.
func SetupEnvironment() error {
	if pnpmVersion := environment.GetPnpmVersion(); pnpmVersion != "" {
		pnpmPackageSpec := "pnpm@" + pnpmVersion
		cmd := exec.Command("npm", "install", "-g", pnpmPackageSpec)

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("error installing %s: %w", pnpmPackageSpec, err)
		}
	}

	return nil
}
