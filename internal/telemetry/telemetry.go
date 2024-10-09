package telemetry

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	speakeasy "github.com/speakeasy-api/speakeasy-client-sdk-go/v3"
	"github.com/speakeasy-api/speakeasy-client-sdk-go/v3/pkg/models/operations"
	"github.com/speakeasy-api/speakeasy-client-sdk-go/v3/pkg/models/shared"
)

type ContextKey string

const ExecutionKeyEnvironmentVariable = "SPEAKEASY_EXECUTION_ID"
const SpeakeasySDKKey ContextKey = "speakeasy.SDK"
const WorkspaceIDKey ContextKey = "speakeasy.workspaceID"
const AccountTypeKey ContextKey = "speakeasy.accountType"

// a random UUID. Change this to fan-out executions with the same gh run id.
const speakeasyGithubActionNamespace = "360D564A-5583-4EF6-BC2B-99530BF036CC"

func NewContextWithSDK(ctx context.Context, apiKey string) (context.Context, *speakeasy.Speakeasy, string, error) {
	security := shared.Security{APIKey: &apiKey}

	opts := []speakeasy.SDKOption{speakeasy.WithSecurity(security)}
	if os.Getenv("SPEAKEASY_SERVER_URL") != "" {
		opts = append(opts, speakeasy.WithServerURL(os.Getenv("SPEAKEASY_SERVER_URL")))
	}

	sdk := speakeasy.New(opts...)
	validated, err := sdk.Auth.ValidateAPIKey(ctx)
	if err != nil {
		return ctx, nil, "", err
	}
	sdkWithWorkspace := speakeasy.New(speakeasy.WithSecurity(security), speakeasy.WithWorkspaceID(validated.APIKeyDetails.WorkspaceID))
	ctx = context.WithValue(ctx, SpeakeasySDKKey, sdkWithWorkspace)
	ctx = context.WithValue(ctx, WorkspaceIDKey, validated.APIKeyDetails.WorkspaceID)
	ctx = context.WithValue(ctx, AccountTypeKey, validated.APIKeyDetails.AccountTypeV2)
	return ctx, sdkWithWorkspace, validated.APIKeyDetails.WorkspaceID, err
}

func GetApiKey() string {
	return os.Getenv("SPEAKEASY_API_KEY")
}

func EnrichEventWithEnvironmentVariables(event *shared.CliEvent) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		return
	}
	ghActionOrg := os.Getenv("GITHUB_REPOSITORY_OWNER")
	ghActionRepoOrg := os.Getenv("GITHUB_REPOSITORY")
	event.GhActionOrganization = &ghActionOrg
	repo := strings.TrimPrefix(ghActionRepoOrg, ghActionOrg+"/")
	event.GhActionRepository = &repo
	runLink := fmt.Sprintf("%s/%s/actions/runs/%s", os.Getenv("GITHUB_SERVER_URL"), ghActionRepoOrg, os.Getenv("GITHUB_RUN_ID"))
	event.GhActionRunLink = &runLink

	ghActionVersion := os.Getenv("GH_ACTION_VERSION")
	if ghActionVersion != "" {
		event.GhActionVersion = &ghActionVersion
	}
}

func enrichHostName(event *shared.CliEvent) {
	hostname, err := os.Hostname()
	if err != nil {
		return
	}
	event.Hostname = &hostname
}

func Track(ctx context.Context, exec shared.InteractionType, fn func(ctx context.Context, event *shared.CliEvent) error) error {
	// Generate a unique ID for this event
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	runID := os.Getenv("GITHUB_RUN_ID")
	if runID == "" {
		return fmt.Errorf("no GITHUB_RUN_ID provided")
	}
	runAttempt := os.Getenv("GITHUB_RUN_ATTEMPT")
	if runAttempt == "" {
		return fmt.Errorf("no GITHUB_RUN_ATTEMPT provided")
	}
	executionKey := fmt.Sprintf("GITHUB_RUN_ID_%s, GITHUB_RUN_ATTEMPT_%s", runID, runAttempt)
	namespace, err := uuid.Parse(speakeasyGithubActionNamespace)
	if err != nil {
		return err
	}

	apiKey := GetApiKey()
	if apiKey == "" {
		return fmt.Errorf("no SPEAKEASY_API_KEY secret provided")
	}
	ctx, sdk, workspaceID, err := NewContextWithSDK(ctx, apiKey)
	if err != nil {
		return err
	}
	executionID := uuid.NewSHA1(namespace, []byte(executionKey)).String()
	_ = os.Setenv(ExecutionKeyEnvironmentVariable, executionID)

	// Prepare the initial CliEvent
	runEvent := &shared.CliEvent{
		CreatedAt:        time.Now(),
		ExecutionID:      executionID,
		ID:               id.String(),
		WorkspaceID:      workspaceID,
		InteractionType:  exec,
		LocalStartedAt:   time.Now(),
		SpeakeasyVersion: fmt.Sprintf(os.Getenv("GH_ACTION_VERSION")),
		Success:          false,
	}
	runEvent.WorkspaceID = workspaceID

	EnrichEventWithEnvironmentVariables(runEvent)
	enrichHostName(runEvent)

	// Execute the provided function, capturing any error
	err = fn(ctx, runEvent)

	// Update the event with completion details
	curTime := time.Now()
	runEvent.LocalCompletedAt = &curTime
	duration := runEvent.LocalCompletedAt.Sub(runEvent.LocalStartedAt).Milliseconds()
	runEvent.DurationMs = &duration

	// For publishing events runEvent success is set by publishEvent.go
	if exec != shared.InteractionTypePublish {
		runEvent.Success = err == nil
	}
	currentIntegrationEnvironment := "GITHUB_ACTIONS"
	runEvent.ContinuousIntegrationEnvironment = &currentIntegrationEnvironment

	// Attempt to flush any stored events (swallow errors)
	sdk.Events.Post(ctx, operations.PostWorkspaceEventsRequest{
		RequestBody: []shared.CliEvent{*runEvent},
		WorkspaceID: workspaceID,
	})

	return err

}
