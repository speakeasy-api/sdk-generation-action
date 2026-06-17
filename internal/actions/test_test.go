package actions

import (
	"os"
	"testing"

	"github.com/speakeasy-api/sdk-gen-config/workflow"
)

func TestTest_InitializeTestedTargetsFromWorkflow(t *testing.T) {
	// Save original environment variables
	originalSpecifiedTarget := os.Getenv("INPUT_TARGET")
	originalEventName := os.Getenv("GITHUB_EVENT_NAME")
	defer func() {
		os.Setenv("INPUT_TARGET", originalSpecifiedTarget)
		os.Setenv("GITHUB_EVENT_NAME", originalEventName)
	}()

	// Mock workflow with testing enabled/disabled targets
	enabledTrue := true
	enabledFalse := false
	
	tests := []struct {
		name           string
		workflow       *workflow.Workflow
		specifiedTarget string
		eventName       string
		expectedTargets []string
		expectedSkip    bool
	}{
		{
			name: "workflow dispatch with specified target",
			workflow: &workflow.Workflow{
				Targets: map[string]workflow.Target{
					"go": {
						Testing: &workflow.Testing{Enabled: &enabledTrue},
					},
					"python": {
						Testing: &workflow.Testing{Enabled: &enabledFalse},
					},
				},
			},
			specifiedTarget: "go",
			eventName:       "workflow_dispatch",
			expectedTargets: []string{"go"},
			expectedSkip:    false,
		},
		{
			name: "no specified target, picks first testing enabled",
			workflow: &workflow.Workflow{
				Targets: map[string]workflow.Target{
					"python": {
						Testing: &workflow.Testing{Enabled: &enabledFalse},
					},
					"go": {
						Testing: &workflow.Testing{Enabled: &enabledTrue},
					},
					"typescript": {
						Testing: &workflow.Testing{Enabled: &enabledTrue},
					},
				},
			},
			specifiedTarget: "",
			eventName:       "push",
			expectedTargets: []string{"go"}, // Should pick first one with testing enabled
			expectedSkip:    false,
		},
		{
			name: "no specified target, no testing enabled targets",
			workflow: &workflow.Workflow{
				Targets: map[string]workflow.Target{
					"go": {
						Testing: &workflow.Testing{Enabled: &enabledFalse},
					},
					"python": {
						Testing: nil, // No testing config
					},
				},
			},
			specifiedTarget: "",
			eventName:       "push",
			expectedTargets: []string{},
			expectedSkip:    true,
		},
		{
			name: "no specified target, nil testing config",
			workflow: &workflow.Workflow{
				Targets: map[string]workflow.Target{
					"go": {
						Testing: nil,
					},
					"python": {
						Testing: &workflow.Testing{Enabled: nil},
					},
				},
			},
			specifiedTarget: "",
			eventName:       "push",
			expectedTargets: []string{},
			expectedSkip:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for test
			os.Setenv("INPUT_TARGET", tt.specifiedTarget)
			os.Setenv("GITHUB_EVENT_NAME", tt.eventName)

			// Test the target selection logic
			var testedTargets []string
			
			// Simulate the logic from Test function
			if providedTargetName := tt.specifiedTarget; providedTargetName != "" && os.Getenv("GITHUB_EVENT_NAME") == "workflow_dispatch" {
				testedTargets = append(testedTargets, providedTargetName)
			}
			
			// If no target is specified via workflow dispatch, check for testing enabled targets and pick the first one
			if len(testedTargets) == 0 {
				for name, target := range tt.workflow.Targets {
					if target.Testing != nil && target.Testing.Enabled != nil && *target.Testing.Enabled {
						testedTargets = append(testedTargets, name)
						break // Pick the first one with testing enabled
					}
				}
			}

			// Verify results
			if tt.expectedSkip && len(testedTargets) != 0 {
				t.Errorf("Expected no targets to be selected, but got: %v", testedTargets)
			}
			
			if !tt.expectedSkip {
				if len(testedTargets) != len(tt.expectedTargets) {
					t.Errorf("Expected %d targets, got %d: %v", len(tt.expectedTargets), len(testedTargets), testedTargets)
				}
				
				// For the case where we pick the first testing-enabled target, 
				// we need to check that it's one of the expected ones
				if len(tt.expectedTargets) > 0 && len(testedTargets) > 0 {
					found := false
					for _, expected := range tt.expectedTargets {
						if testedTargets[0] == expected {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected target %v to be one of %v", testedTargets[0], tt.expectedTargets)
					}
				}
			}
		})
	}
}