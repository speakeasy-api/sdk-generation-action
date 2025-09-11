package actions

import "testing"

func TestGetDirAndShouldUseReleasesMD(t *testing.T) {
	type args struct {
		files           []string
		dir             string
		usingReleasesMd bool
	}
	tests := []struct {
		name  string
		args  args
		want  string
		want1 bool
	}{
		{
			name: "RELEASES.md found",
			args: args{
				files:           []string{"./RELEASES.md", "some/other/file.go"},
				dir:             ".",
				usingReleasesMd: false,
			},
			want:  ".",
			want1: true,
		},
		{
			name: "RELEASES.md found in subdirectory",
			args: args{
				files:           []string{"subdir/RELEASES.md", "some/other/file.go"},
				dir:             ".",
				usingReleasesMd: false,
			},
			want:  "subdir",
			want1: true,
		},
		{
			name: "gen.lock found",
			args: args{
				files:           []string{".speakeasy/gen.lock", "some/other/file.go"},
				dir:             ".",
				usingReleasesMd: false,
			},
			want:  ".",
			want1: false,
		},
		{
			name: "gen.lock found in subdirectory",
			args: args{
				files:           []string{"subdir/.speakeasy/gen.lock", "some/other/file.go"},
				dir:             ".",
				usingReleasesMd: false,
			},
			want:  "subdir",
			want1: false,
		},
		{
			name: "no relevant files found",
			args: args{
				files:           []string{"some/file.go", "another/file.js"},
				dir:             ".",
				usingReleasesMd: false,
			},
			want:  ".",
			want1: false,
		},
		{
			name: "gen.lock takes precedence over RELEASES.md",
			args: args{
				files:           []string{".speakeasy/gen.lock", "RELEASES.md"},
				dir:             ".",
				usingReleasesMd: false,
			},
			want:  ".",
			want1: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := GetDirAndShouldUseReleasesMD(tt.args.files, tt.args.dir, tt.args.usingReleasesMd)
			if got != tt.want {
				t.Errorf("GetDirAndShouldUseReleasesMD() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("GetDirAndShouldUseReleasesMD() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
