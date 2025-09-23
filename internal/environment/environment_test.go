package environment

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSourceBranch(t *testing.T) {
	tests := []struct {
		name          string
		githubRef     string
		githubHeadRef string
		expected      string
	}{
		{
			name:      "direct push to main",
			githubRef: "refs/heads/main",
			expected:  "main",
		},
		{
			name:      "direct push to master",
			githubRef: "refs/heads/master",
			expected:  "master",
		},
		{
			name:      "direct push to feature branch",
			githubRef: "refs/heads/feature/my-feature",
			expected:  "feature/my-feature",
		},
		{
			name:          "PR trigger",
			githubRef:     "refs/pull/123/merge",
			githubHeadRef: "feature/pr-branch",
			expected:      "feature/pr-branch",
		},
		{
			name:          "PR trigger with pulls",
			githubRef:     "refs/pulls/456/merge",
			githubHeadRef: "fix/bug-fix",
			expected:      "fix/bug-fix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			os.Setenv("GITHUB_REF", tt.githubRef)
			if tt.githubHeadRef != "" {
				os.Setenv("GITHUB_HEAD_REF", tt.githubHeadRef)
			} else {
				os.Unsetenv("GITHUB_HEAD_REF")
			}

			// Test
			result := GetSourceBranch()
			assert.Equal(t, tt.expected, result)

			// Cleanup
			os.Unsetenv("GITHUB_REF")
			os.Unsetenv("GITHUB_HEAD_REF")
		})
	}
}

func TestIsMainBranch(t *testing.T) {
	tests := []struct {
		name     string
		branch   string
		expected bool
	}{
		{
			name:     "main branch",
			branch:   "main",
			expected: true,
		},
		{
			name:     "master branch",
			branch:   "master",
			expected: true,
		},
		{
			name:     "feature branch",
			branch:   "feature/my-feature",
			expected: false,
		},
		{
			name:     "develop branch",
			branch:   "develop",
			expected: false,
		},
		{
			name:     "empty string",
			branch:   "",
			expected: false,
		},
		{
			name:     "main with prefix",
			branch:   "origin/main",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsMainBranch(tt.branch)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetTargetBaseBranch(t *testing.T) {
	tests := []struct {
		name          string
		githubRef     string
		githubHeadRef string
		expected      string
	}{
		{
			name:      "main branch direct push",
			githubRef: "refs/heads/main",
			expected:  "refs/heads/main",
		},
		{
			name:      "master branch direct push",
			githubRef: "refs/heads/master",
			expected:  "refs/heads/master",
		},
		{
			name:      "feature branch direct push",
			githubRef: "refs/heads/feature/my-feature",
			expected:  "refs/heads/feature/my-feature",
		},
		{
			name:          "PR from feature branch",
			githubRef:     "refs/pull/123/merge",
			githubHeadRef: "feature/pr-branch",
			expected:      "refs/heads/feature/pr-branch",
		},
		{
			name:          "PR from main branch",
			githubRef:     "refs/pull/456/merge",
			githubHeadRef: "main",
			expected:      "main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment
			os.Setenv("GITHUB_REF", tt.githubRef)
			if tt.githubHeadRef != "" {
				os.Setenv("GITHUB_HEAD_REF", tt.githubHeadRef)
			} else {
				os.Unsetenv("GITHUB_HEAD_REF")
			}

			// Test
			result := GetTargetBaseBranch()
			assert.Equal(t, tt.expected, result)

			// Cleanup
			os.Unsetenv("GITHUB_REF")
			os.Unsetenv("GITHUB_HEAD_REF")
		})
	}
}

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		name     string
		branch   string
		expected string
	}{
		{
			name:     "simple branch name",
			branch:   "feature",
			expected: "feature",
		},
		{
			name:     "branch with slash",
			branch:   "feature/my-feature",
			expected: "feature-my-feature",
		},
		{
			name:     "branch with underscore",
			branch:   "feature_my_feature",
			expected: "feature-my-feature",
		},
		{
			name:     "branch with spaces",
			branch:   "feature my feature",
			expected: "feature-my-feature",
		},
		{
			name:     "complex branch name",
			branch:   "feature/my_feature with spaces",
			expected: "feature-my-feature-with-spaces",
		},
		{
			name:     "branch with leading/trailing hyphens",
			branch:   "-feature-",
			expected: "feature",
		},
		{
			name:     "branch with multiple consecutive separators",
			branch:   "feature//my__feature",
			expected: "feature--my--feature",
		},
		{
			name:     "empty string",
			branch:   "",
			expected: "",
		},
		{
			name:     "only separators",
			branch:   "/_-",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeBranchName(tt.branch)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSourceBranchIntegration(t *testing.T) {
	// Test that GetSourceBranch works correctly with GetTargetBaseBranch
	tests := []struct {
		name           string
		githubRef      string
		githubHeadRef  string
		expectedSource string
		expectedTarget string
		expectedIsMain bool
	}{
		{
			name:           "main branch workflow",
			githubRef:      "refs/heads/main",
			expectedSource: "main",
			expectedTarget: "refs/heads/main",
			expectedIsMain: true,
		},
		{
			name:           "feature branch workflow",
			githubRef:      "refs/heads/feature/awesome-feature",
			expectedSource: "feature/awesome-feature",
			expectedTarget: "refs/heads/feature/awesome-feature",
			expectedIsMain: false,
		},
		{
			name:           "PR from feature branch",
			githubRef:      "refs/pull/123/merge",
			githubHeadRef:  "feature/pr-feature",
			expectedSource: "feature/pr-feature",
			expectedTarget: "refs/heads/feature/pr-feature",
			expectedIsMain: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			os.Setenv("GITHUB_REF", tt.githubRef)
			if tt.githubHeadRef != "" {
				os.Setenv("GITHUB_HEAD_REF", tt.githubHeadRef)
			} else {
				os.Unsetenv("GITHUB_HEAD_REF")
			}

			// Test source branch detection
			sourceBranch := GetSourceBranch()
			assert.Equal(t, tt.expectedSource, sourceBranch)

			// Test main branch detection
			isMain := IsMainBranch(sourceBranch)
			assert.Equal(t, tt.expectedIsMain, isMain)

			// Test target base branch
			targetBranch := GetTargetBaseBranch()
			assert.Equal(t, tt.expectedTarget, targetBranch)

			// Cleanup
			os.Unsetenv("GITHUB_REF")
			os.Unsetenv("GITHUB_HEAD_REF")
		})
	}
}
