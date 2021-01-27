package util

import (
	"context"

	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
)

// GithubClientSource describes a function that can create a
// github.Client without any other inputs.
type GithubClientSource func() *github.Client

func NewGithubClientSource(token string) GithubClientSource {
	return func() *github.Client {
		ctx := context.Background()
		tokenSource := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		oauthClient := oauth2.NewClient(ctx, tokenSource)
		return github.NewClient(oauthClient)
	}
}
