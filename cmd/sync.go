package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/egymgmbh/go-prefix-writer/prefixer"
	"github.com/fatih/color"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/cobra"
)

func init() {
	pristine := false

	var cmdSync = &cobra.Command{
		Use:   "sync [flags] ORG_URL",
		Args:  cobra.ExactArgs(1),
		Short: `Clone or pull a collection of repos from GitHub or Gitlab in parallel`,
		Run: func(cmd *cobra.Command, args []string) {
			orgUrlArg := args[0]
			err := doSync(cmd.Context(), orgUrlArg, pristine)
			if err != nil {
				cmd.PrintErrln(err)
				os.Exit(1)
			}
		},
	}

	cmdSync.Flags().BoolVar(&pristine, "pristine", false, "Stash, reset and clean the repo first")

	rootCmd.AddCommand(cmdSync)
}

func doSync(ctx context.Context, orgUrlStr string, pristine bool) error {
	org := ""
	provider, err := RepoProviderFor(orgUrlStr)
	if err != nil {
		return fmt.Errorf("couldn't find provider for '%s': %w", orgUrlStr, err)
	}

	orgUrl, err := url.Parse(orgUrlStr)
	if err != nil {
		return fmt.Errorf("couldn't parse '%s': %w", orgUrlStr, err)
	}

	org, err = provider.GetOrgFromUrl(orgUrlStr)
	if err != nil {
		return fmt.Errorf("couldn't get org from '%s': %w", orgUrlStr, err)
	}

	localDir := getLocalDir(orgUrl)
	fmt.Fprintf(os.Stderr, "Syncing to '%s'\n", localDir)

	progressWriter := NewProgressWriter()
	progressWriter.Start()
	defer progressWriter.Stop()

	workerPool := NewSyncReposWorkerPool(progressWriter, pristine)

	err = provider.ListRepos(ctx, org, workerPool.AddCloneUrl)
	if err != nil {
		return fmt.Errorf("couldn't list repos for '%s': %w", orgUrlStr, err)
	}

	if err := workerPool.Wait(); err != nil {
		return err
	}

	return nil
}

// type cloneUrlChan chan string

type syncReposWorkerPool struct {
	errorPool      *pool.ErrorPool
	progressWriter *ProgressWriter
	pristine       bool
}

func NewSyncReposWorkerPool(progressWriter *ProgressWriter, pristine bool) *syncReposWorkerPool {
	return &syncReposWorkerPool{
		progressWriter: progressWriter,
		pristine:       pristine,
		errorPool:      pool.New().WithErrors(),
	}
}

func (p *syncReposWorkerPool) AddCloneUrl(cloneUrlStr string) {
	p.progressWriter.Total.Add(1)
	p.errorPool.Go(func() error {
		return doWork(cloneUrlStr, p.progressWriter, p.pristine)
	})
}

func (p *syncReposWorkerPool) Wait() error {
	return p.errorPool.Wait()
}

func doWork(cloneUrlStr string, progressWriter *ProgressWriter, pristine bool) error {
	gitUrl, _ := url.Parse(cloneUrlStr)
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

	err := c.doGet(gitUrl, "", localDir, pristine, false)
	if err != nil {
		return err
	}

	progressWriter.Complete.Add(1)

	return nil
}
