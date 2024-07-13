package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/google/go-github/v57/github"
	"github.com/jdxcode/netrc"
	"github.com/sourcegraph/conc/pool"
	gitlab "github.com/xanzy/go-gitlab"
)

const apiPageSize = 100

var KnownGitProviders = []RepoProvider{
	NewGithubRepoProvider(),
}

func init() {
	glabHosts := strings.Split(os.Getenv("GITLAB_HOSTS"), ",")
	glabHosts = append(glabHosts, "gitlab.com")
	slices.Sort(glabHosts)
	glabHosts = slices.Compact(glabHosts)

	for _, host := range glabHosts {
		KnownGitProviders = append(KnownGitProviders, NewGitlabRepoProvider(host))
	}
}

type RepoProvider interface {
	IsMatch(s string) bool
	NormaliseGitUrl(s string) string
	GetOrgFromUrl(orgUrl string) (string, error)
	ListRepos(ctx context.Context, org string, includeArchived bool, repoUrlCallback func(remoteRepo)) error
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

func (gh GithubRepoProvider) ListRepos(ctx context.Context, org string, includeArchived bool, repoUrlCallback func(remoteRepo)) error {
	client := gh.getClient(ctx)
	_, _, err := client.Organizations.Get(ctx, org)
	if err == nil {
		return gh.ListReposByOrg(ctx, org, includeArchived, repoUrlCallback)
	}

	return gh.ListReposByUser(ctx, org, includeArchived, repoUrlCallback)
}

func (gh GithubRepoProvider) ListReposByUser(ctx context.Context, org string, includeArchived bool, repoUrlCallback func(remoteRepo)) error {
	client := gh.getClient(ctx)
	opt := &github.RepositoryListByUserOptions{
		ListOptions: github.ListOptions{
			PerPage: apiPageSize,
		},
	}
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cancelled listing Github repos for user %s: %w", org, ctx.Err())
		default:
			repos, resp, err := client.Repositories.ListByUser(ctx, org, opt)
			if err != nil {
				return fmt.Errorf("error listing repos for user %s: %w", org, err)
			}

			for _, repo := range repos {
				if repo.GetArchived() && !includeArchived {
					continue
				}
				r := remoteRepo{
					cloneUrl:   repo.GetCloneURL(),
					isArchived: repo.GetArchived(),
				}
				repoUrlCallback(r)
			}

			if resp.NextPage == 0 {
				return nil
			}
			opt.Page = resp.NextPage
		}
	}
}

func (gh GithubRepoProvider) ListReposByOrg(ctx context.Context, org string, includeArchived bool, repoUrlCallback func(remoteRepo)) error {
	client := gh.getClient(ctx)
	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{
			PerPage: apiPageSize,
		},
	}
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cancelled listing Github repos for org %s: %w", org, ctx.Err())
		default:
			repos, resp, err := client.Repositories.ListByOrg(ctx, org, opt)
			if err != nil {
				return fmt.Errorf("error listing repos for org %s: %w", org, err)
			}

			for _, repo := range repos {
				if repo.GetArchived() && !includeArchived {
					continue
				}
				r := remoteRepo{
					cloneUrl:   repo.GetCloneURL(),
					isArchived: repo.GetArchived(),
				}
				repoUrlCallback(r)
			}

			if resp.NextPage == 0 {
				return nil
			}
			opt.Page = resp.NextPage
		}
	}
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
	host string
}

func NewGitlabRepoProvider(host string) GitlabRepoProvider {
	return GitlabRepoProvider{
		genericRepoProvider: genericRepoProvider{
			prefix:       fmt.Sprintf("%s/", host),
			appendPrefix: "https://",
			appendSuffix: ".git",
			orgRegexp:    regexp.MustCompile(fmt.Sprintf("(https?://)?%s/(?P<org>.+)", host)),
		},
		host: host,
	}
}

func (gl GitlabRepoProvider) getClient() (*gitlab.Client, error) {
	gitlabToken := getNetrcPasswordForMachine(gl.host)
	options := []gitlab.ClientOptionFunc{}

	if logLevel == "debug" {
		options = append(options, gitlab.WithCustomLogger(log.New(os.Stderr, "", log.LstdFlags)))
	}

	client, err := gitlab.NewClient(gitlabToken, options...)
	if err != nil {
		return nil, fmt.Errorf("error creating gitlab client: %w", err)
	}

	return client, nil
}

func (gl GitlabRepoProvider) ListRepos(ctx context.Context, org string, includeArchived bool, cloneUrlFunc func(remoteRepo)) error {
	client, err := gl.getClient()
	if err != nil {
		return fmt.Errorf("error creating gitlab client: %w", err)
	}

	opt := gitlab.ListGroupProjectsOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: apiPageSize,
			OrderBy: "id",
			Sort:    "desc", // newest first, older repos have more chance of being archived
			Page:    1,
		},
		IncludeSubGroups: gitlab.Ptr(true),
		MinAccessLevel:   gitlab.Ptr(gitlab.DeveloperPermissions),
	}
	if !includeArchived {
		opt.Archived = gitlab.Ptr(false)
	}

	// use a worker pool to pull down data from gitlab in parallel
	gitlabRequestPool := pool.New().WithMaxGoroutines(3).WithContext(ctx).WithCancelOnError().WithFirstError()

	noMoreResults := false
	for {
		thisIterationOpt := opt
		gitlabRequestPool.Go(func(ctx context.Context) error {
			ps, resp, err := client.Groups.ListGroupProjects(org, &thisIterationOpt)
			if err != nil {
				return fmt.Errorf("error listing repos for org %s: %w", org, err)
			}

			for _, p := range ps {
				if p.RepositoryAccessLevel == "disabled" {
					continue
				}

				r := remoteRepo{
					cloneUrl:      p.HTTPURLToRepo,
					isArchived:    p.Archived,
					defaultBranch: p.DefaultBranch,
				}
				cloneUrlFunc(r)
			}

			if resp.NextPage == 0 {
				noMoreResults = true
			}

			return nil
		})
		opt.Page++

		if noMoreResults {
			break
		}
	}

	err = gitlabRequestPool.Wait()
	if err != nil {
		return fmt.Errorf("error waiting for gitlab requests to complete: %w", err)
	}

	return nil
}
