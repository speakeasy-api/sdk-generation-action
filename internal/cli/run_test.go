package cli

import (
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
