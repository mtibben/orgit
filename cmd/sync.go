package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/cobra"
)

func init() {
	noClone := false
	noUpdate := false
	noArchive := false
	logLevel := "auto"

	var cmdSync = &cobra.Command{
		Use:   "sync [flags] ORG_URL",
		Args:  cobra.ExactArgs(1),
		Short: `Clone or pull a collection of repos from GitHub or Gitlab in parallel`,
		Long: `syncing will:
1. clone a collection of repos from GitHub or Gitlab in parallel
2. update existing repos to origin HEAD by stashing uncommitted changes and pulling
3. archive repos that have been archived on the remote by moving them to $GRIT_WORKSPACE/.archive
`,
		Run: func(cmd *cobra.Command, args []string) {
			orgUrlArg := args[0]
			err := doSync(cmd.Context(), orgUrlArg, !noArchive, !noUpdate, !noClone, logLevel)
			if err != nil {
				cmd.PrintErrln(err)
				os.Exit(1)
			}
		},
	}

	cmdSync.Flags().BoolVar(&noClone, "no-clone", false, "Don't clone repos")
	cmdSync.Flags().BoolVar(&noArchive, "no-archive", false, "Don't archive repos")
	cmdSync.Flags().BoolVar(&noUpdate, "no-update", false, "Don't update repos")
	cmdSync.Flags().StringVar(&logLevel, "log-level", "info", "Set the log level (debug, verbose, info, quiet)")

	rootCmd.AddCommand(cmdSync)
}

func doSync(ctx context.Context, orgUrlStr string, archive, update, clone bool, loglevel string) error {
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

	workerPool := NewSyncReposWorkerPool(archive, update, clone, logger)

	err = provider.ListRepos(ctx, org, archive, workerPool.GoGetUrl)
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
	archiveRepos   bool
	updateRepos    bool
	cloneRepos     bool
}

func NewSyncReposWorkerPool(archiveRepos, updateRepos, cloneRepos bool, progressWriter *ProgressLogger) *syncReposWorkerPool {
	wp := syncReposWorkerPool{
		errorPool:      pool.New().WithErrors(),
		archiveRepos:   archiveRepos,
		updateRepos:    updateRepos,
		cloneRepos:     cloneRepos,
		progressWriter: progressWriter,
	}
	return &wp
}

type remoteRepo struct {
	cloneUrl   string
	isArchived bool
}

func (p *syncReposWorkerPool) GoGetUrl(r remoteRepo) {
	p.progressWriter.AddTotalToProgress(1)
	p.errorPool.Go(func() error {
		return p.doWork(r)
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

func (p *syncReposWorkerPool) doWork(r remoteRepo) error {
	gitUrl, _ := url.Parse(r.cloneUrl)
	localDir := getLocalDir(gitUrl)
	localDirExists := dirExists(localDir)

	if r.isArchived {
		if localDirExists {
			if p.archiveRepos {
				err := p.archive(localDir)
				if err != nil {
					return fmt.Errorf("couldn't archive '%s': %w", localDir, err)
				}
				p.progressWriter.EventArchivedRepo(localDir)
			} else {
				p.progressWriter.EventSkippedRepo(localDir)
			}
		} else {
			// no action required, remove this repo from total
			p.progressWriter.AddTotalToProgress(-1)
		}
		return nil
	}

	c := getCmdContext{
		Stdout:      p.progressWriter.WriterFor(localDir),
		Stderr:      p.progressWriter.WriterFor(localDir),
		CmdEchoFunc: p.progressWriter.EventExecCmd,
		WorkingDir:  localDir,
	}
	if localDirExists {
		if p.updateRepos {
			err := c.doUpdate(gitUrl, "")
			if err != nil {
				p.progressWriter.EventSyncedRepoError(localDir)
				return err
			}
			p.progressWriter.EventUpdatedRepo(localDir)
		} else {
			p.progressWriter.EventSkippedRepo(localDir)
		}
	} else {
		if p.cloneRepos {
			err := c.doClone(gitUrl.String(), "")
			if err != nil {
				p.progressWriter.EventSyncedRepoError(localDir)
				return err
			}
			p.progressWriter.EventClonedRepo(localDir)
		} else {
			p.progressWriter.EventSkippedRepo(localDir)
		}
	}

	return nil
}

func (p *syncReposWorkerPool) archive(localDir string) error {
	rel, err := filepath.Rel(getWorkspaceDir(), localDir)
	if err != nil {
		return err
	}
	newArchivedDir := filepath.Join(getWorkspaceDir(), ".archive", rel)
	if dirExists(newArchivedDir) {
		return fmt.Errorf("can't archive '%s', dir '%s' already exists", localDir, newArchivedDir)
	}

	parentDir := filepath.Dir(newArchivedDir)
	if !dirExists(parentDir) {
		err := os.MkdirAll(parentDir, 0755)
		if err != nil {
			return err
		}
	}

	return os.Rename(localDir, newArchivedDir)
}
