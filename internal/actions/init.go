package actions

import (
	"errors"

	"github.com/speakeasy-api/sdk-generation-action/internal/environment"
	"github.com/speakeasy-api/sdk-generation-action/internal/git"
)

func initAction() (*git.Git, error) {
	accessToken := environment.GetAccessToken()
	if accessToken == "" {
		return nil, errors.New("github access token is required")
	}

	g := git.New(accessToken)
	r, err := g.CloneRepo(environment.GetRepo())
	if err != nil {
		return nil, err
	}
	g.SetRepo(r)
	return g, nil
}
