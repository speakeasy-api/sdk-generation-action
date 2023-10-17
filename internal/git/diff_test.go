package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsGitDiffSignificant(t *testing.T) {
	type args struct {
		diff string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "detects no significant changes",
			args: args{
				diff: `diff --git a/gen.yaml b/gen.yaml
index 322c845..585bc5b 100644
--- a/gen.yaml
+++ b/gen.yaml
@@ -9,5 +9,5 @@ generation:
   sdkClassName: SDK
   sdkFlattening: false
 go:
-  version: 1.3.0
+  version: 1.3.1
   packageName: github.com/speakeasy-api/sdk-generation-action-test-repo
diff --git a/sdk.go b/sdk.go
index b26db52..fdc01f4 100755
--- a/sdk.go
+++ b/sdk.go
@@ -120,7 +120,7 @@ func WithSecurity(security shared.Security) SDKOption {
 func New(opts ...SDKOption) *SDK {
        sdk := &SDK{
                _language:   "go",
-               _sdkVersion: "1.3.0",
+               _sdkVersion: "1.3.1",
                _genVersion: "1.12.7",
+               _userAgent: "speakeasy-sdk/go 0.0.1 2.155.1 0.1.0-alpha openapi"
        }
        for _, opt := range opts {
`,
			},
			want: false,
		},
		{
			name: "detects significant changes",
			args: args{
				diff: `diff --git a/gen.yaml b/gen.yaml
index 322c845..585bc5b 100644
--- a/gen.yaml
+++ b/gen.yaml
@@ -9,5 +9,5 @@ generation:
   sdkClassName: SDK
   sdkFlattening: false
 go:
-  version: 1.3.0
+  version: 1.3.1
   packageName: github.com/speakeasy-api/sdk-generation-action-test-repo
diff --git a/sdk.go b/sdk.go
index b26db52..fdc01f4 100755
--- a/sdk.go
+++ b/sdk.go
@@ -120,7 +120,7 @@ func WithSecurity(security shared.Security) SDKOption {
 func New(opts ...SDKOption) *SDK {
        sdk := &SDK{
-               _language:   "go",
+               _language:   "crazygo",
                _sdkVersion: "1.3.0",
                _genVersion: "1.12.7",
        }
        for _, opt := range opts {
`,
			},
			want: true,
		},
		{
			name: "detects significant changes with tabs for spacing",
			args: args{
				// Important: Preserve tabs in the follow diff
				diff: `diff --git a/gen.yaml b/gen.yaml
index 322c845..585bc5b 100644
--- a/gen.yaml
+++ b/gen.yaml
@@ -9,5 +9,5 @@ generation:
   sdkClassName: SDK
   sdkFlattening: false
 go:
-  version: 1.3.0
+  version: 1.3.1
   packageName: github.com/speakeasy-api/sdk-generation-action-test-repo
diff --git a/sdk.go b/sdk.go
index b26db52..fdc01f4 100755
--- a/sdk.go
+++ b/sdk.go
@@ -120,7 +120,7 @@ func WithSecurity(security shared.Security) SDKOption {
 func New(opts ...SDKOption) *SDK {
				sdk := &SDK{
-								language:   "go",
+								_language:   "crazygo",
								_sdkVersion: "1.3.0",
								_genVersion: "1.12.7",
				}
				for _, opt := range opts {
`,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsGitDiffSignificant(tt.args.diff)
			assert.Equal(t, tt.want, got)
		})
	}
}
