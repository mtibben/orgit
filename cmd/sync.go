package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egymgmbh/go-prefix-writer/prefixer"
	"github.com/fatih/color"
	"github.com/google/go-github/v57/github"
	"github.com/jdxcode/netrc"
	"github.com/spf13/cobra"
	errgroup "golang.org/x/sync/errgroup"
)

type ProgressWriter struct {
	Complete atomic.Uint32
	Total    atomic.Uint32

	doneChan  chan bool
	waitGroup sync.WaitGroup
	ticker    *time.Ticker
}

func NewProgressWriter() *ProgressWriter {
	return &ProgressWriter{
		doneChan:  make(chan bool),
		waitGroup: sync.WaitGroup{},
		ticker:    time.NewTicker(time.Second),
	}
}

func (p *ProgressWriter) Start() {
	p.waitGroup.Add(1)
	go func() {
		defer p.waitGroup.Done()

		fmt.Print(saveCursorPosition)
		for {
			select {
			case <-p.ticker.C:
				fmt.Print(clearLine)
				fmt.Printf("Synced %d of %d repos", p.Complete.Load(), p.Total.Load())
			case <-p.doneChan:
				fmt.Print(clearLine)
				fmt.Printf("Synced %d of %d repos", p.Complete.Load(), p.Total.Load())
				fmt.Println()
				return
			}
		}
	}()
}

func (p *ProgressWriter) Stop() {
	p.ticker.Stop()
	p.doneChan <- true
	p.waitGroup.Wait()
}

var parseOrgUrl = regexp.MustCompile("(https?://)?github.com/(?P<org>[^/]+)")

var saveCursorPosition = "\033[s"
var clearLine = "\033[u\033[K"

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

			progressWriter := NewProgressWriter()
			progressWriter.Start()

			go func(ctxWorkerPool context.Context) {

				for {
					select {
					case repo := <-workerPoolJobChan:
						progressWriter.Total.Add(1)
						g.Go(func() error {
							defer progressWriter.Complete.Add(1)
							gitUrlStr := repo.GetCloneURL()
							gitUrl, _ := url.Parse(gitUrlStr)
							localDir := getLocalDir(gitUrl)

							relDir, _ := filepath.Rel(getWorkspaceDir(), localDir)

							progressType := "progressbar"
							c := cmdContext{}
							if progressType == "simple" {
								prefixWriter := prefixer.New(os.Stdout, func() string {
									return color.GreenString("[%s] ", relDir)
								})

								c = cmdContext{
									Stdout:   prefixWriter,
									Stderr:   prefixWriter,
									EchoFunc: color.Cyan,
								}
							} else {
								c = cmdContext{
									Stdout:   &bytes.Buffer{},
									Stderr:   &bytes.Buffer{},
									EchoFunc: func(format string, a ...interface{}) {},
								}
							}

							err := c.doGet(gitUrl, "", localDir, false, false)
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

			progressWriter.Stop()
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
