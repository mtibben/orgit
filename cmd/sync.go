package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	ignore "github.com/sabhiram/go-gitignore"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/cobra"
)

var logLevel = "info"

func init() {
	noClone := false
	noUpdate := false
	noArchive := false

	var cmdSync = &cobra.Command{
		Use:   "sync [flags] ORG_URL",
		Args:  cobra.ExactArgs(1),
		Short: `Clone and update all repos from a GitHub/GitLab user/org/group`,
		Long: `Syncing will:
 1. clone all repositories from a GitHub/GitLab user/org/group
 2. update local repos by stashing uncommitted changes and switching to origin HEAD
 3. archive local repos that have been archived remotely by moving them to $ORGIT_WORKSPACE/.archive
`,
		Run: func(cmd *cobra.Command, args []string) {
			orgUrlArg := args[0]
			err := doSync(cmd.Context(), orgUrlArg, !noClone, !noUpdate, !noArchive, logLevel)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			workspace := getWorkspaceDir()
			return []string{workspace}, cobra.ShellCompDirectiveFilterDirs
		},
	}

	cmdSync.Flags().BoolVar(&noClone, "no-clone", false, "Don't clone repos")
	cmdSync.Flags().BoolVar(&noUpdate, "no-update", false, "Don't update repos")
	cmdSync.Flags().BoolVar(&noArchive, "no-archive", false, "Don't archive repos")
	cmdSync.Flags().StringVar(&logLevel, "log-level", "info", "Set the log level (debug, verbose, info, quiet)")

	rootCmd.AddCommand(cmdSync)
}

func doSync(ctx context.Context, orgUrlStr string, clone, update, archive bool, loglevel string) error {
	ctx, ctxCancel := context.WithCancel(ctx)

	logger := NewProgressLogger(loglevel)
	defer logger.EndProgressLine("done")

	workerPool := NewSyncReposWorkerPool(ctx, clone, update, archive, logger)

	// channel to trigger cancellation
	// try to shutdown gracefully by waiting for workers to finish
	gracefulShutdownTrigger := make(chan os.Signal, 1)
	go func() {
		<-gracefulShutdownTrigger
		logger.LogInfo = false
		logger.EndProgressLine("cancelling...")
		logger.LogRealtimeProgress = false
		ctxCancel()           // cancel the context, closing the ctx.Done channel
		_ = workerPool.Wait() // wait for all workers to finish
		os.Exit(1)
	}()

	// catch Ctrl-C, SIGINT, SIGTERM, SIGQUIT and gracefully shutdown
	signal.Notify(gracefulShutdownTrigger, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	org := ""
	repoProvider, err := RepoProviderFor(orgUrlStr)
	if err != nil {
		return fmt.Errorf("couldn't find provider for '%s': %w", orgUrlStr, err)
	}

	orgUrl, err := url.Parse(orgUrlStr)
	if err != nil {
		return fmt.Errorf("couldn't parse '%s': %w", orgUrlStr, err)
	}

	org, err = repoProvider.GetOrgFromUrl(orgUrlStr)
	if err != nil {
		return fmt.Errorf("couldn't get org from '%s': %w", orgUrlStr, err)
	}

	localDir := getLocalDir(orgUrl)
	logger.Info(fmt.Sprintf("Syncing to '%s'", localDir))

	err = repoProvider.ListRepos(ctx, org, archive, workerPool.remoteReposChan)
	close(workerPool.remoteReposChan) // close the channel to signal that no more repos will be sent
	if err != nil {
		return fmt.Errorf("couldn't list repos for '%s': %w", orgUrlStr, err)
	}

	err = workerPool.Wait()
	if err != nil {
		return fmt.Errorf("Sync didn't fully complete")
	}

	return nil
}

type syncReposWorkerPool struct {
	workerPool              *pool.ContextPool
	progressWriter          *ProgressLogger
	cloneRepos              bool
	updateRepos             bool
	archiveRepos            bool
	ignore                  *ignore.GitIgnore
	remoteReposChan         chan remoteRepo
	remoteReposChanFinished chan bool
}

const SyncWorkerPoolSize = 100
const RemoteReposChannelSize = SyncWorkerPoolSize * 20 // buffer 20 repos per worker

func NewSyncReposWorkerPool(ctx context.Context, clone, update, archive bool, progressWriter *ProgressLogger) *syncReposWorkerPool {
	p := &syncReposWorkerPool{
		workerPool:              pool.New().WithMaxGoroutines(SyncWorkerPoolSize).WithContext(ctx),
		cloneRepos:              clone,
		updateRepos:             update,
		archiveRepos:            archive,
		progressWriter:          progressWriter,
		ignore:                  getIgnorePatterns(),
		remoteReposChan:         make(chan remoteRepo, RemoteReposChannelSize),
		remoteReposChanFinished: make(chan bool),
	}

	go p.startRemoteReposChanListener()
	return p
}

func (p *syncReposWorkerPool) createJob(r remoteRepo) func(context.Context) error {
	return func(ctx context.Context) error {
		err := p.doWork(r)
		if err != nil {
			p.progressWriter.Info(err.Error())
			return fmt.Errorf("error doing work: %w", err)
		}

		return nil
	}
}

func (p *syncReposWorkerPool) startRemoteReposChanListener() {
	for r := range p.remoteReposChan {
		if p.canIgnore(r) {
			continue
		}
		p.progressWriter.AddTotalToProgress(1)

		// start a new goroutine for each job
		p.workerPool.Go(p.createJob(r))
	}
	p.remoteReposChanFinished <- true
}

func cleanName(s string) string {
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimSuffix(s, ".git")

	return s
}

type remoteRepo struct {
	cloneUrl      string
	isArchived    bool
	defaultBranch string
}

func (p *syncReposWorkerPool) Wait() error {
	<-p.remoteReposChanFinished

	err := p.workerPool.Wait()
	if err != nil {
		return fmt.Errorf("couldn't sync all repos: %w", err)
	}
	return nil
}

func (p *syncReposWorkerPool) canIgnore(r remoteRepo) bool {
	cleanRepoName := cleanName(r.cloneUrl)
	if p.ignore.MatchesPath(cleanRepoName) {
		p.progressWriter.EventIgnoredRepo(cleanRepoName)
		return true
	}

	return false
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
			p.progressWriter.EventIgnoredArchivedRepo(localDir)
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
			err := c.doUpdate(gitUrl, r.defaultBranch)
			if err != nil {
				p.progressWriter.EventSyncedRepoError(localDir)
				return fmt.Errorf("error updating: %w", err)
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
				return fmt.Errorf("error cloning: %w", err)
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
		return fmt.Errorf("couldn't get relative path for '%s': %w", localDir, err)
	}
	newArchivedDir := filepath.Join(getWorkspaceDir(), ".archive", rel)
	if dirExists(newArchivedDir) {
		return fmt.Errorf("can't archive '%s', dir '%s' already exists", localDir, newArchivedDir)
	}

	parentDir := filepath.Dir(newArchivedDir)
	if !dirExists(parentDir) {
		err := os.MkdirAll(parentDir, 0755)
		if err != nil {
			return fmt.Errorf("couldn't create parent dir '%s': %w", parentDir, err)
		}
	}

	err = os.Rename(localDir, newArchivedDir)
	if err != nil {
		return fmt.Errorf("couldn't move '%s' to '%s': %w", localDir, newArchivedDir, err)
	}

	p.progressWriter.Info(fmt.Sprintf("Archived '%s' to '%s'", localDir, newArchivedDir))
	return nil
}
