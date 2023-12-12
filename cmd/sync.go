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
	progress := "auto"

	var cmdSync = &cobra.Command{
		Use:   "sync [flags] ORG_URL",
		Args:  cobra.ExactArgs(1),
		Short: `Clone or pull a collection of repos from GitHub or Gitlab in parallel`,
		Run: func(cmd *cobra.Command, args []string) {
			orgUrlArg := args[0]
			err := doSync(cmd.Context(), orgUrlArg, pristine, progress)
			if err != nil {
				cmd.PrintErrln(err)
				os.Exit(1)
			}
		},
	}

	cmdSync.Flags().BoolVar(&pristine, "pristine", false, "Stash, reset and clean the repo first")
	cmdSync.Flags().StringVar(&progress, "progress", "auto", "Set type of progress output (auto, tty, plain)")

	rootCmd.AddCommand(cmdSync)
}

func doSync(ctx context.Context, orgUrlStr string, pristine bool, progress string) error {
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

	workerPool := NewSyncReposWorkerPool(pristine, progress)

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
	progressWriter *ProgressWriter
	pristine       bool
	progressType   string
}

func NewSyncReposWorkerPool(pristine bool, progressType string) *syncReposWorkerPool {
	wp := syncReposWorkerPool{
		pristine:       pristine,
		errorPool:      pool.New().WithErrors(),
		progressType:   progressType,
		progressWriter: NewProgressWriter(progressType == "auto"),
	}
	return &wp
}

func (p *syncReposWorkerPool) GoCloneUrl(cloneUrlStr string) {
	p.progressWriter.AddTotal(1)
	p.errorPool.Go(func() error {
		return p.doWork(cloneUrlStr)
	})
}

func (p *syncReposWorkerPool) Wait() error {
	err := p.errorPool.Wait()
	if err == nil {
		p.progressWriter.Done()
	}
	return err
}

func (p *syncReposWorkerPool) GetCmdContext(localDir string) cmdContext {
	relDir, _ := filepath.Rel(getWorkspaceDir(), localDir)

	if p.progressType == "plain" {
		prefixWriter := prefixer.New(os.Stdout, func() string {
			return color.GreenString("[%s] ", relDir)
		})

		return cmdContext{
			Stdout:   prefixWriter,
			Stderr:   prefixWriter,
			EchoFunc: color.Cyan,
		}
	} else {
		return cmdContext{
			Stdout:   &bytes.Buffer{},
			Stderr:   &bytes.Buffer{},
			EchoFunc: func(format string, a ...interface{}) {},
		}
	}
}

func (p *syncReposWorkerPool) doWork(cloneUrlStr string) error {
	gitUrl, _ := url.Parse(cloneUrlStr)
	localDir := getLocalDir(gitUrl)
	c := p.GetCmdContext(localDir)

	err := c.doGet(gitUrl, "", localDir, p.pristine, false)
	if err != nil {
		return err
	}

	p.progressWriter.Println("Synced " + localDir)
	p.progressWriter.AddComplete(1)

	return nil
}
