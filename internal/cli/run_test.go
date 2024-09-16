package cli

import (
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getLintingReportURL(t *testing.T) {
	type args struct {
		out string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "success",
			args: args{
				out: `INFO    Running source validationtest...
INFO    Validating OpenAPI spec...

INFO    Using ruleset thisRuleset from validationtest/.speakeasy/lint.yaml
INFO    Validation report available to view at: https://app.speakeasy.com/org/test/test/linting-report/7aebdf7581f7f04430709644c2e304b7
WARN    validation warn: [line 12] any-paths - More than a single path exists, there are 1
INFO    
OpenAPI spec validation complete. 0 errors, 1 warnings, 0 hints`,
			},
			want: "https://app.speakeasy.com/org/test/test/linting-report/7aebdf7581f7f04430709644c2e304b7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := getLintingReportURL(tt.args.out)
			assert.Equal(t, tt.want, url)
		})
	}
}

func Test_environmentVariables(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple case",
			input:    "FOO=bar",
			expected: []string{"FOO=bar"},
		},
		{
			name:     "trimmed",
			input:    " FOO=bar",
			expected: []string{"FOO=bar"},
		},
		{
			name:     "quotes",
			input:    `FOO="bar baz"`,
			expected: []string{`FOO=bar baz`},
		},
		{
			name:     "multi input",
			input:    "FOO=bar\nBAR=qux",
			expected: []string{"BAR=qux", "FOO=bar"},
		},
		{
			name:     "quoted input with newline",
			input:    "  FOO=\"foo\nbar\n\"\nother_input=value",
			expected: []string{"FOO=foo\nbar\n", "other_input=value"},
		},
		{
			name:     "windows newlines",
			input:    "FOO=bar\r\nBAR=qux",
			expected: []string{"BAR=qux", "FOO=bar"},
		},
		{
			name:     "empty input",
			input:    "",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the environment variable
			os.Setenv("INPUT_ENV_VARS", tt.input)

			// Call the function
			result := environment.SpeakeasyEnvVars()

			// Validate the result
			assert.Equal(t, tt.expected, result, "The result should match the expected output")

			// Clean up
			os.Unsetenv("INPUT_ENV_VARS")
		})
	}
}
