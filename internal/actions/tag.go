package actions

import (
	"github.com/speakeasy-api/sdk-generation-action/internal/cli"
	"github.com/speakeasy-api/sdk-generation-action/internal/configuration"
	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/logging"
	"github.com/speakeasy-api/sdk-generation-action/internal/registry"
	"golang.org/x/exp/maps"
)

func Tag() error {
	g, err := initAction()
	if err != nil {
		return err
	}

	if _, err = cli.Download("latest", g); err != nil {
		return err
	}

	tags := registry.ProcessRegistryTags()

	sources := environment.SpecifiedSources()
	targets := environment.SpecifiedCodeSamplesTargets()

	if len(sources) == 0 && len(targets) == 0 {
		wf, err := configuration.GetWorkflowAndValidateLanguages(false)
		if err != nil {
			return err
		}

		sources = maps.Keys(wf.Sources)
		targets = maps.Keys(wf.Targets)

		logging.Info("No sources or targets specified, using all sources and targets from workflow")
	}

	return cli.Tag(tags, sources, targets)
}
