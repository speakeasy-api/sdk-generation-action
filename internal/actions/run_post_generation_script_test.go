package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunPostGenerationScript(t *testing.T) {
	tests := []struct {
		name      string
		script    string
		wantErr   bool
		errSubstr string
		// optional: inspect the repo dir after running
		assertDir func(t *testing.T, repoDir string)
	}{
		{
			name:    "empty script is a no-op",
			script:  "",
			wantErr: false,
		},
		{
			name:    "whitespace-only script is a no-op",
			script:  "   \n\t  ",
			wantErr: false,
		},
		{
			name:    "successful script returns nil",
			script:  "echo hello",
			wantErr: false,
		},
		{
			name:      "non-zero exit is wrapped",
			script:    "exit 7",
			wantErr:   true,
			errSubstr: "post_generation_script failed",
		},
		{
			name:   "script runs in the repo working directory",
			script: "echo from-script > marker.txt",
			assertDir: func(t *testing.T, repoDir string) {
				t.Helper()
				b, err := os.ReadFile(filepath.Join(repoDir, "marker.txt"))
				if err != nil {
					t.Fatalf("expected marker.txt in %s: %v", repoDir, err)
				}
				if strings.TrimSpace(string(b)) != "from-script" {
					t.Fatalf("unexpected marker contents: %q", string(b))
				}
			},
		},
		{
			name:   "honors working_directory subdir",
			script: "echo nested > nested.txt",
			assertDir: func(t *testing.T, repoDir string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(repoDir, "nested.txt")); err != nil {
					t.Fatalf("expected nested.txt in working_directory %s: %v", repoDir, err)
				}
			},
		},
		{
			name:    "multiline script with set -e fails fast",
			script:  "set -e\nfalse\necho should-not-run",
			wantErr: true,
		},
	}

	t.Run("env is scrubbed of secrets", func(t *testing.T) {
		repoDir := setupScriptEnv(t, "", `printenv > env.txt`)
		t.Setenv("INPUT_GITHUB_ACCESS_TOKEN", "ghs_secrettoken")
		t.Setenv("INPUT_SPEAKEASY_API_KEY", "sk_secretkey")
		t.Setenv("GITHUB_TOKEN", "ghs_runnersecret")
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "oidcsecret")
		t.Setenv("PATH", os.Getenv("PATH"))
		t.Setenv("GITHUB_REPOSITORY", "owner/repo")

		if err := runPostGenerationScript(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		envBytes, err := os.ReadFile(filepath.Join(repoDir, "env.txt"))
		if err != nil {
			t.Fatalf("read env.txt: %v", err)
		}
		envOut := string(envBytes)
		for _, secret := range []string{"ghs_secrettoken", "sk_secretkey", "ghs_runnersecret", "oidcsecret", "INPUT_POST_GENERATION_SCRIPT", "INPUT_GITHUB_ACCESS_TOKEN", "INPUT_SPEAKEASY_API_KEY"} {
			if strings.Contains(envOut, secret) {
				t.Fatalf("script env leaked %q:\n%s", secret, envOut)
			}
		}
		for _, allowed := range []string{"PATH=", "GITHUB_REPOSITORY=owner/repo"} {
			if !strings.Contains(envOut, allowed) {
				t.Fatalf("expected %q in script env:\n%s", allowed, envOut)
			}
		}
	})

	t.Run("cli_environment_variables are passed through", func(t *testing.T) {
		repoDir := setupScriptEnv(t, "", `printenv > env.txt`)
		t.Setenv("INPUT_CLI_ENVIRONMENT_VARIABLES", "MY_TOOL_VERSION=1.2.3\nFEATURE_FLAG=on")

		if err := runPostGenerationScript(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		envOut, err := os.ReadFile(filepath.Join(repoDir, "env.txt"))
		if err != nil {
			t.Fatalf("read env.txt: %v", err)
		}
		for _, want := range []string{"MY_TOOL_VERSION=1.2.3", "FEATURE_FLAG=on"} {
			if !strings.Contains(string(envOut), want) {
				t.Fatalf("expected %q in script env:\n%s", want, envOut)
			}
		}
	})

	t.Run("timeout kills a long-running script", func(t *testing.T) {
		setupScriptEnv(t, "", "sleep 30")

		original := postGenerationScriptTimeout
		postGenerationScriptTimeout = 100 * time.Millisecond
		defer func() { postGenerationScriptTimeout = original }()

		start := time.Now()
		err := runPostGenerationScript()
		elapsed := time.Since(start)

		if err == nil {
			t.Fatalf("expected timeout error, got nil")
		}
		if !strings.Contains(err.Error(), "timeout") {
			t.Fatalf("expected timeout error, got %q", err.Error())
		}
		if elapsed > 5*time.Second {
			t.Fatalf("timeout did not kill the script promptly: took %s", elapsed)
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workingDir := ""
			if tt.name == "honors working_directory subdir" {
				workingDir = "sdks/python"
			}
			repoDir := setupScriptEnv(t, workingDir, tt.script)

			err := runPostGenerationScript()

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.assertDir != nil {
				tt.assertDir(t, repoDir)
			}
		})
	}
}

func setupScriptEnv(t *testing.T, workingDir, script string) string {
	t.Helper()
	workspace := t.TempDir()
	repoDir := filepath.Join(workspace, "repo", workingDir)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Setenv("GITHUB_WORKSPACE", workspace)
	t.Setenv("INPUT_WORKING_DIRECTORY", workingDir)
	t.Setenv("INPUT_POST_GENERATION_SCRIPT", script)
	return repoDir
}
