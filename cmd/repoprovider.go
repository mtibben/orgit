package cmd

import (
	"context"
	"fmt"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-github/v57/github"
	"github.com/jdxcode/netrc"
	gitlab "github.com/xanzy/go-gitlab"
)

var KnownGitProviders = []RepoProvider{
	NewGithubRepoProvider(),
	NewGitlabRepoProvider(),
}

type RepoProvider interface {
	IsMatch(s string) bool
	NormaliseGitUrl(s string) string
	GetOrgFromUrl(orgUrl string) (string, error)
	ListRepos(ctx context.Context, org string, cloneUrlFunc func(string)) error
}

func RepoProviderFor(s string) (RepoProvider, error) {
	for _, provider := range KnownGitProviders {
		if provider.IsMatch(s) {
			return provider, nil
		}
	}
	return nil, fmt.Errorf("no provider found for '%s'", s)
}

type genericRepoProvider struct {
	prefix       string
	appendPrefix string
	appendSuffix string
	orgRegexp    *regexp.Regexp
}

func (p genericRepoProvider) IsMatch(s string) bool {
	return strings.HasPrefix(s, p.prefix)
}

func (p genericRepoProvider) NormaliseGitUrl(s string) string {
	return p.appendPrefix + s + p.appendSuffix
}

func (p genericRepoProvider) GetOrgFromUrl(orgUrlArg string) (string, error) {
	orgIndex := p.orgRegexp.SubexpIndex("org")
	if orgIndex == -1 {
		return "", fmt.Errorf("invalid org url '%s", orgUrlArg)
	}
	return p.orgRegexp.FindStringSubmatch(orgUrlArg)[orgIndex], nil
}

func (p genericRepoProvider) ListRepos(ctx context.Context, org string, cloneUrlFunc func(string)) error {
	return fmt.Errorf("no ListRepos implementation for '%s'", p.prefix)
}

type GithubRepoProvider struct {
	genericRepoProvider
}

func NewGithubRepoProvider() GithubRepoProvider {
	return GithubRepoProvider{
		genericRepoProvider{
			prefix:       "github.com/",
			appendPrefix: "https://",
			appendSuffix: ".git",
			orgRegexp:    regexp.MustCompile("(https?://)?github.com/(?P<org>[^/]+)"),
		},
	}
}

func (gh GithubRepoProvider) getClient(ctx context.Context) *github.Client {
	githubToken := getNetrcPasswordForMachine("api.github.com")

	if githubToken != "" {
		return github.NewTokenClient(ctx, githubToken)
	}

	return github.NewClient(nil)
}

func (gh GithubRepoProvider) ListRepos(ctx context.Context, org string, cloneUrlFunc func(string)) error {
	client := gh.getClient(ctx)
	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{
			PerPage: 40,
		},
	}
	for {
		repos, resp, err := client.Repositories.ListByOrg(ctx, org, opt)
		if err != nil {
			return err
		}

		for _, repo := range repos {
			if !repo.GetArchived() {
				cloneUrlFunc(repo.GetCloneURL())
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return nil
}

func getNetrcPasswordForMachine(machine string) string {
	usr, err := user.Current()
	if err == nil {
		n, err := netrc.Parse(filepath.Join(usr.HomeDir, ".netrc"))
		if err == nil {
			machine := n.Machine(machine)
			if machine != nil {
				return machine.Get("password")
			}
		}
	}
	return ""
}

type GitlabRepoProvider struct {
	genericRepoProvider
}

func NewGitlabRepoProvider() GitlabRepoProvider {
	return GitlabRepoProvider{
		genericRepoProvider{
			prefix:       "gitlab.com/",
			appendPrefix: "https://",
			appendSuffix: ".git",
			orgRegexp:    regexp.MustCompile("(https?://)?gitlab.com/(?P<org>.+)"),
		},
	}
}

func (gl GitlabRepoProvider) getClient(ctx context.Context) (*gitlab.Client, error) {
	gitlabToken := getNetrcPasswordForMachine("gitlab.com")
	return gitlab.NewClient(gitlabToken)
}

func (gl GitlabRepoProvider) ListRepos(ctx context.Context, org string, cloneUrlFunc func(string)) error {
	client, err := gl.getClient(ctx)
	if err != nil {
		return err
	}

	opt := &gitlab.ListGroupProjectsOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: 20,
			Page:    1,
		},
		Archived:         gitlab.Ptr(false),
		IncludeSubGroups: gitlab.Ptr(true),
	}

	for {
		// Get the first page with projects.
		ps, resp, err := client.Groups.ListGroupProjects(org, opt)
		if err != nil {
			return err
		}
		for _, p := range ps {
			cloneUrlFunc(p.HTTPURLToRepo)
		}

		// Exit the loop when we've seen all pages.
		if resp.NextPage == 0 {
			break
		}

		// Update the page number to get the next page.
		opt.Page = resp.NextPage
	}

	return nil
}
