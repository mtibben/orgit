package cmd

import (
	"context"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"regexp"

	"github.com/google/go-github/v57/github"
	"github.com/jdxcode/netrc"
	"github.com/spf13/cobra"
	errgroup "golang.org/x/sync/errgroup"
)

var parseOrgUrl = regexp.MustCompile("(https?://)?github.com/(?P<org>[^/]+)")

func init() {

	var cmdSync = &cobra.Command{
		Use:  "sync [flags] ORG_URL",
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {

			orgURL := args[0]
			if !parseOrgUrl.MatchString(orgURL) {
				cmd.PrintErrf("invalid org url '%s", orgURL)
				os.Exit(1)
			}

			orgIndex := parseOrgUrl.SubexpIndex("org")
			org := parseOrgUrl.FindStringSubmatch(orgURL)[orgIndex]
			client := getGithubClient(cmd.Context())

			g, _ := errgroup.WithContext(cmd.Context())
			workerPoolJobChan := make(repoChan, 20)
			ctxWorkerPool := context.Background()
			go func(ctxWorkerPool context.Context) {
				for {
					select {
					case repo := <-workerPoolJobChan:
						g.Go(func() error {
							gitUrlStr := repo.GetCloneURL()
							gitUrl, _ := url.Parse(gitUrlStr)
							localDir := getLocalDir(gitUrl)

							err := doGet(gitUrl, "", localDir, false, true)
							if err != nil {
								return err
							}
							return nil
						})
					case <-ctxWorkerPool.Done():
						break
					}
				}
			}(ctxWorkerPool)
			defer ctxWorkerPool.Done()

			err := listAllReposInOrg(cmd.Context(), client, org, workerPoolJobChan)
			if err != nil {
				panic(err)
			}

			if err := g.Wait(); err != nil {
				cmd.PrintErrln(err)
				os.Exit(1)
			}

		},
	}

	rootCmd.AddCommand(cmdSync)
}

type repoChan chan *github.Repository

func getGithubClient(ctx context.Context) *github.Client {
	usr, err := user.Current()
	if err == nil {
		n, err := netrc.Parse(filepath.Join(usr.HomeDir, ".netrc"))
		if err == nil {
			githubToken := n.Machine("github.com").Get("password")
			if githubToken != "" {
				return github.NewTokenClient(ctx, githubToken)
			}
		}
	}
	return github.NewClient(nil)
}

func listAllReposInOrg(ctx context.Context, client *github.Client, org string, workerPoolJobChan repoChan) error {
	opt := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{
			PerPage: 20,
		},
	}
	for {
		repos, resp, err := client.Repositories.ListByOrg(ctx, org, opt)
		if err != nil {
			return err
		}

		for _, repo := range repos {
			if !repo.GetArchived() {
				workerPoolJobChan <- repo
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return nil
}
