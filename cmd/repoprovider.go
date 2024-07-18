package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"

	"github.com/google/go-github/v57/github"
	netrc "github.com/jdx/go-netrc"
	"github.com/sourcegraph/conc/pool"
	gitlab "github.com/xanzy/go-gitlab"
)

const apiPageSize = 100

var KnownGitProviders = []RepoProvider{
	NewGithubRepoProvider(),
}

var ErrRepoNotFound = errors.New("repo not found")

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
	ListRepos(ctx context.Context, org string, includeArchived bool, remoteRepoChan chan RemoteRepo) error
	GetRepo(ctx context.Context, repoName string) (RemoteRepo, error)
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
}

func (p genericRepoProvider) IsMatch(s string) bool {
	return strings.HasPrefix(s, p.prefix)
}

func (p genericRepoProvider) NormaliseGitUrl(s string) string {
	return p.appendPrefix + s + p.appendSuffix
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

func (gh GithubRepoProvider) GetRepo(ctx context.Context, repoUrl string) (RemoteRepo, error) {
	client := gh.getClient(ctx)
	repoParts := strings.Split(repoUrl, "/")
	if len(repoParts) < 2 {
		return RemoteRepo{}, fmt.Errorf("invalid github repo url '%s'", repoUrl)
	}

	owner := repoParts[len(repoParts)-2]
	repo := repoParts[len(repoParts)-1]

	r, _, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return RemoteRepo{}, fmt.Errorf("error getting repo %s/%s: %w", owner, repo, err)
	}

	return RemoteRepo{
		RepoName:      MustParseRepoName(r.GetURL()),
		CloneUrl:      r.GetCloneURL(),
		IsArchived:    r.GetArchived(),
		DefaultBranch: r.GetDefaultBranch(),
	}, nil
}

func (gh GithubRepoProvider) ListRepos(ctx context.Context, org string, includeArchived bool, remoteRepoChan chan RemoteRepo) error {
	client := gh.getClient(ctx)
	_, _, err := client.Organizations.Get(ctx, org)
	if err == nil {
		return gh.ListReposByOrg(ctx, org, includeArchived, remoteRepoChan)
	}

	return gh.ListReposByUser(ctx, org, includeArchived, remoteRepoChan)
}

func (gh GithubRepoProvider) ListReposByUser(ctx context.Context, org string, includeArchived bool, remoteRepoChan chan RemoteRepo) error {
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
				r := RemoteRepo{
					RepoName:      MustParseRepoName(repo.GetURL()),
					CloneUrl:      repo.GetCloneURL(),
					IsArchived:    repo.GetArchived(),
					DefaultBranch: repo.GetDefaultBranch(),
				}
				remoteRepoChan <- r
			}

			if resp.NextPage == 0 {
				return nil
			}
			opt.Page = resp.NextPage
		}
	}
}

func (gh GithubRepoProvider) ListReposByOrg(ctx context.Context, org string, includeArchived bool, remoteRepoChan chan RemoteRepo) error {
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
				r := RemoteRepo{
					RepoName:      MustParseRepoName(repo.GetURL()),
					CloneUrl:      repo.GetCloneURL(),
					IsArchived:    repo.GetArchived(),
					DefaultBranch: repo.GetDefaultBranch(),
				}
				remoteRepoChan <- r
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
		},
		host: host,
	}
}

func (gl GitlabRepoProvider) getClient() (*gitlab.Client, error) {
	gitlabToken := getNetrcPasswordForMachine(gl.host)
	options := []gitlab.ClientOptionFunc{}

	if logLevelFlag == "debug" {
		options = append(options, gitlab.WithCustomLogger(log.New(os.Stderr, "", log.LstdFlags)))
	}

	client, err := gitlab.NewClient(gitlabToken, options...)
	if err != nil {
		return nil, fmt.Errorf("error creating gitlab client: %w", err)
	}

	return client, nil
}

func (gl GitlabRepoProvider) ListRepos(ctx context.Context, org string, includeArchived bool, remoteRepoChan chan RemoteRepo) error {
	client, err := gl.getClient()
	if err != nil {
		return fmt.Errorf("error creating gitlab client: %w", err)
	}

	err = gl.ListReposByOrg(ctx, client, org, includeArchived, remoteRepoChan)
	if errors.Is(err, gitlab.ErrNotFound) {
		return gl.ListReposByUser(ctx, client, org, includeArchived, remoteRepoChan)
	}

	return err
}

func (gl GitlabRepoProvider) ListReposByOrg(ctx context.Context, client *gitlab.Client, org string, includeArchived bool, remoteRepoChan chan RemoteRepo) error {
	opt := gitlab.ListGroupProjectsOptions{
		ListOptions:      defaultGitlabListOptions,
		MinAccessLevel:   gitlab.Ptr(gitlab.DeveloperPermissions),
		IncludeSubGroups: gitlab.Ptr(true),
	}
	if !includeArchived {
		opt.Archived = gitlab.Ptr(false)
	}

	gitlabRequestPool := gl.newListReposWorkerPool(ctx)

	noMoreResults := false
	contextCancelled := false
	for ; !contextCancelled && !noMoreResults; opt.Page++ {
		thisIterationOpt := opt
		gitlabRequestPool.Go(func(ctx context.Context) error {
			if ctx.Err() != nil {
				contextCancelled = true
				return fmt.Errorf("context cancelled, not making request: %w", ctx.Err())
			}
			ps, resp, err := client.Groups.ListGroupProjects(org, &thisIterationOpt)
			if err != nil {
				return fmt.Errorf("error listing repos for org %s: %w", org, err)
			}

			for _, p := range ps {
				if p.RepositoryAccessLevel == "disabled" {
					continue
				}

				r := RemoteRepo{
					RepoName:      MustParseRepoName(p.WebURL),
					CloneUrl:      p.HTTPURLToRepo,
					IsArchived:    p.Archived,
					DefaultBranch: p.DefaultBranch,
				}
				remoteRepoChan <- r
			}

			if resp.NextPage == 0 {
				noMoreResults = true
			}

			return nil
		})
	}

	err := gitlabRequestPool.Wait()
	if err != nil {
		return fmt.Errorf("error during gitlab request: %w", err)
	}

	return nil
}

var defaultGitlabListOptions = gitlab.ListOptions{
	PerPage: apiPageSize,
	OrderBy: "id",
	Sort:    "desc", // newest first, older repos have more chance of being archived
	Page:    1,
}

// use a pool of 3 workers to pull down data from gitlab
func (gl GitlabRepoProvider) newListReposWorkerPool(ctx context.Context) *pool.ContextPool {
	return pool.New().WithMaxGoroutines(3).WithContext(ctx).WithCancelOnError().WithFirstError()
}

func (gl GitlabRepoProvider) ListReposByUser(ctx context.Context, client *gitlab.Client, user string, includeArchived bool, remoteRepoChan chan RemoteRepo) error {
	opt := gitlab.ListProjectsOptions{
		ListOptions:    defaultGitlabListOptions,
		MinAccessLevel: gitlab.Ptr(gitlab.DeveloperPermissions),
	}
	if !includeArchived {
		opt.Archived = gitlab.Ptr(false)
	}

	gitlabRequestPool := gl.newListReposWorkerPool(ctx)

	noMoreResults := false
	contextCancelled := false
	for ; !contextCancelled && !noMoreResults; opt.Page++ {
		thisIterationOpt := opt
		gitlabRequestPool.Go(func(ctx context.Context) error {
			if ctx.Err() != nil {
				contextCancelled = true
				return fmt.Errorf("context cancelled, not making request: %w", ctx.Err())
			}
			ps, resp, err := client.Projects.ListUserProjects(user, &thisIterationOpt)
			if err != nil {
				return fmt.Errorf("error listing repos for user %s: %w", user, err)
			}

			for _, p := range ps {
				if p.RepositoryAccessLevel == "disabled" {
					continue
				}

				r := RemoteRepo{
					RepoName:      MustParseRepoName(p.WebURL),
					CloneUrl:      p.HTTPURLToRepo,
					IsArchived:    p.Archived,
					DefaultBranch: p.DefaultBranch,
				}

				remoteRepoChan <- r
			}

			if resp.NextPage == 0 {
				noMoreResults = true
			}

			return nil
		})
	}

	err := gitlabRequestPool.Wait()
	if err != nil {
		return fmt.Errorf("error during gitlab request: %w", err)
	}

	return nil
}

func (gl GitlabRepoProvider) GetRepo(ctx context.Context, repoName string) (RemoteRepo, error) {
	client, err := gl.getClient()
	if err != nil {
		return RemoteRepo{}, fmt.Errorf("error creating gitlab client: %w", err)
	}

	p, _, err := client.Projects.GetProject(repoName, nil)
	if errors.Is(err, gitlab.ErrNotFound) {
		return RemoteRepo{}, ErrRepoNotFound
	}
	if err != nil {
		return RemoteRepo{}, fmt.Errorf("error getting project %s: %w", repoName, err)
	}

	return RemoteRepo{
		RepoName:      MustParseRepoName(p.WebURL),
		CloneUrl:      p.HTTPURLToRepo,
		IsArchived:    p.Archived,
		DefaultBranch: p.DefaultBranch,
	}, nil
}
