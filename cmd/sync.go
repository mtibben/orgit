package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/cobra"
)

func init() {
	pristine := false
	logLevel := "auto"

	var cmdSync = &cobra.Command{
		Use:   "sync [flags] ORG_URL",
		Args:  cobra.ExactArgs(1),
		Short: `Clone or pull a collection of repos from GitHub or Gitlab in parallel`,
		Run: func(cmd *cobra.Command, args []string) {
			orgUrlArg := args[0]
			err := doSync(cmd.Context(), orgUrlArg, pristine, logLevel)
			if err != nil {
				cmd.PrintErrln(err)
				os.Exit(1)
			}
		},
	}

	cmdSync.Flags().BoolVar(&pristine, "pristine", false, "Stash, reset and clean the repo first")
	cmdSync.Flags().StringVar(&logLevel, "log-level", "info", "Set the log level (debug, verbose, info, quiet)")

	rootCmd.AddCommand(cmdSync)
}

func doSync(ctx context.Context, orgUrlStr string, pristine bool, loglevel string) error {
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
	logger := NewProgressLogger(loglevel)
	logger.Info(fmt.Sprintf("Syncing to '%s'", localDir))

	workerPool := NewSyncReposWorkerPool(pristine, logger)

	err = provider.ListRepos(ctx, org, workerPool.GoCloneUrl)
	if err != nil {
		return fmt.Errorf("couldn't list repos for '%s': %w", orgUrlStr, err)
	}

	if err := workerPool.Wait(); err != nil {
		return err
	}

	return nil
}

type syncReposWorkerPool struct {
	errorPool      *pool.ErrorPool
	progressWriter *ProgressLogger
	pristine       bool
}

func NewSyncReposWorkerPool(pristine bool, progressWriter *ProgressLogger) *syncReposWorkerPool {
	wp := syncReposWorkerPool{
		pristine:       pristine,
		errorPool:      pool.New().WithErrors(),
		progressWriter: progressWriter,
	}
	return &wp
}

func (p *syncReposWorkerPool) GoCloneUrl(cloneUrlStr string) {
	p.progressWriter.AddTotalToProgress(1)
	p.errorPool.Go(func() error {
		return p.doWork(cloneUrlStr)
	})
}

func (p *syncReposWorkerPool) Wait() error {
	err := p.errorPool.Wait()
	if err == nil {
		p.progressWriter.Info("Done")
	} else {
		p.progressWriter.Info("Done with errors")
	}
	return err
}

func (p *syncReposWorkerPool) doWork(cloneUrlStr string) error {
	gitUrl, _ := url.Parse(cloneUrlStr)
	localDir := getLocalDir(gitUrl)
	c := getCmdContext{
		Stdout:      p.progressWriter.WriterFor(localDir),
		Stderr:      p.progressWriter.WriterFor(localDir),
		CmdEchoFunc: p.progressWriter.EventExecCmd,
		Dir:         localDir,
	}
	err := c.doGet(gitUrl, "", p.pristine, false)
	if err != nil {
		p.progressWriter.EventSyncedRepoError(localDir)
		return err
	}

	p.progressWriter.EventSyncedRepo(localDir)

	return nil
}
