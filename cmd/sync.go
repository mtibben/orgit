package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

	ignore "github.com/sabhiram/go-gitignore"
	"github.com/sourcegraph/conc/pool"
	"github.com/spf13/cobra"
)

var logLevelFlag = "info"

const archiveDir = ".archive"
const trashDir = ".trash"

func init() {
	noCloneFlag := false
	noUpdateFlag := false
	noArchiveFlag := false
	tidyFlag := false

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
			err := doSync(cmd.Context(), orgUrlArg, !noCloneFlag, !noUpdateFlag, !noArchiveFlag, tidyFlag, logLevelFlag)
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

	cmdSync.Flags().BoolVar(&noCloneFlag, "no-clone", false, "Don't clone repos")
	cmdSync.Flags().BoolVar(&noUpdateFlag, "no-update", false, "Don't update repos")
	cmdSync.Flags().BoolVar(&noArchiveFlag, "no-archive", false, "Don't archive repos to $ORGIT_WORSPACE/.archive")
	cmdSync.Flags().BoolVar(&tidyFlag, "tidy", false, "Tidy up the workspace, moving repos missing on the remote to $ORGIT_WORSPACE/.trash")
	cmdSync.Flags().StringVar(&logLevelFlag, "log-level", "info", "Set the log level (debug, verbose, info, quiet)")

	rootCmd.AddCommand(cmdSync)
}

var dryRun = false

func doSync(ctx context.Context, orgUrlStr string, clone, update, archive, tidy bool, loglevel string) (err error) {
	ctx, ctxCancel := context.WithCancel(ctx)

	logger := NewProgressLogger(loglevel)

	workerPool := NewSyncReposWorkerPool(ctx, clone, update, archive, logger)
	var workerPoolWait = sync.OnceFunc(func() {
		werr := workerPool.Wait()
		if werr != nil {
			logger.EndProgressLine("didn't fully complete")
		}
		if err == nil {
			err = werr
		}
		logger.EndProgressLine("done")
	})
	defer workerPoolWait()

	// channel to trigger cancellation
	// try to shutdown gracefully by waiting for workers to finish
	gracefulShutdownTrigger := make(chan os.Signal, 1)
	go func() {
		<-gracefulShutdownTrigger
		logger.EndProgressLine("cancelled")
		logger.Info("Aborting sync...")
		ctxCancel()      // cancel the context, closing the ctx.Done channel
		workerPoolWait() // wait for all workers to finish
		os.Exit(1)
	}()

	// catch Ctrl-C, SIGINT, SIGTERM, SIGQUIT and gracefully shutdown
	signal.Notify(gracefulShutdownTrigger, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	repoProvider, err := RepoProviderFor(orgUrlStr)
	if err != nil {
		return fmt.Errorf("couldn't find provider for '%s': %w", orgUrlStr, err)
	}

	repoPath, err := ParseRepoName(orgUrlStr)
	if err != nil {
		return fmt.Errorf("couldn't parse '%s': %w", orgUrlStr, err)
	}

	logger.Info(fmt.Sprintf("Syncing to '%s'", repoPath.LocalPathAbsolute()))

	err = repoProvider.ListRepos(ctx, repoPath.Path, archive, workerPool.remoteReposChan)
	close(workerPool.remoteReposChan) // close the channel to signal that no more repos will be sent
	if err != nil && !errors.Is(err, context.Canceled) {
		err = fmt.Errorf("couldn't list repos for '%s': %w", orgUrlStr, err)
	}

	workerPoolWait()
	if tidy && ctx.Err() != context.Canceled {
		logger.Info("Tidying...")
		tidier := TidyAction{
			repoProvider: repoProvider,
			logger:       logger,
			remoteRepos:  workerPool.getAllRemoteRepos(),
		}
		tidier.Tidy(ctx, repoPath)
	}

	return err
}

func osMove(oldpath, newpath string) error {
	if dryRun {
		return nil
	}

	err := os.MkdirAll(filepath.Dir(newpath), 0755)
	if err != nil {
		return fmt.Errorf("couldn't create parent dir: %w", err)
	}
	err = os.Rename(oldpath, newpath)
	if err != nil {
		return fmt.Errorf("couldn't move directory: %w", err)
	}
	return nil
}

type TidyAction struct {
	repoProvider RepoProvider
	logger       *ProgressLogger
	remoteRepos  []string
}

func (t *TidyAction) Tidy(ctx context.Context, repoPath RepoName) {
	wg := sync.WaitGroup{}
	err := fs.WalkDir(os.DirFS(getWorkspaceDir()), repoPath.String(), func(relativePath string, d fs.DirEntry, err error) error {
		if err != nil {
			panic(err)
		}

		if !d.IsDir() {
			return t.Trash(relativePath)
		}

		if slices.Contains(ignoreDirs, d.Name()) {
			return fs.SkipDir
		}

		if t.HasAlreadyProcessedRepo(relativePath) {
			return fs.SkipDir
		}

		if t.IsParentDirectoryForProcessedRepo(relativePath) {
			return nil
		}

		absolutePath := filepath.Join(getWorkspaceDir(), relativePath)

		if isGitRepo(absolutePath) {
			reponame := MustParseRepoName(relativePath)

			wg.Add(1)
			go func(reponame RepoName) {
				defer wg.Done()
				tidyErr := t.doTidy(ctx, reponame)
				if tidyErr != nil {
					t.logger.Info(tidyErr.Error())
				}
			}(reponame)

			return fs.SkipDir
		}

		err = t.Trash(relativePath)
		if err != nil {
			t.logger.Info(err.Error())
		}

		return fs.SkipDir
	})
	if err != nil {
		panic(err)
	}

	wg.Wait()
}

// FIXME: remoteRepos is in blah/one/two but pathname is a file path which will cause issues on windows
func (t *TidyAction) HasAlreadyProcessedRepo(pathname string) bool {
	for _, r := range t.remoteRepos {
		if pathname == r {
			return true
		}

		// check if r is a parent directory of pathname
		if strings.HasPrefix(pathname, r+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

// FIXME: remoteRepos is in blah/one/two but pathname is a file path which will cause issues on windows
func (t *TidyAction) IsParentDirectoryForProcessedRepo(pathname string) bool {
	for _, r := range t.remoteRepos {
		if strings.HasPrefix(r, pathname+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func (t *TidyAction) Trash(pathRelative string) error {
	pathAbsolute := filepath.Join(getWorkspaceDir(), pathRelative)
	trashPath := filepath.Join(getTrashDir(), pathRelative)

	err := osMove(pathAbsolute, trashPath)
	if err != nil {
		return fmt.Errorf("couldn't move '%s' to '%s': %w", pathRelative, trashPath, err)
	}
	t.logger.Info(fmt.Sprintf("Moved '%s' to '%s'", pathAbsolute, trashPath))

	return nil
}

func (t *TidyAction) doTidy(ctx context.Context, oldRepoName RepoName) error {
	newRepo, err := t.repoProvider.GetRepo(ctx, oldRepoName.Path)
	if errors.Is(err, ErrRepoNotFound) {
		err = t.Trash(oldRepoName.String())
		if err != nil {
			return fmt.Errorf("couldn't trash '%s': %w", oldRepoName.String(), err)
		} else {
			return nil
		}
	}
	if err != nil {
		return fmt.Errorf("couldn't get repo '%s': %w", oldRepoName.String(), err)
	}

	if newRepo.RepoName.LocalPathAbsolute() != oldRepoName.LocalPathAbsolute() {
		err := osMove(oldRepoName.LocalPathAbsolute(), newRepo.RepoName.LocalPathAbsolute())
		if err != nil {
			return fmt.Errorf("couldn't move '%s' to '%s': %w", oldRepoName.LocalPathAbsolute(), newRepo.RepoName.LocalPathAbsolute(), err)
		}

		t.logger.Info(fmt.Sprintf("Moved '%s' to '%s'", oldRepoName.LocalPathAbsolute(), newRepo.RepoName.LocalPathAbsolute()))
	} else {
		return fmt.Errorf("Expected new repo to have a different local dir: " + newRepo.RepoName.LocalPathAbsolute())
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
	remoteReposChan         chan RemoteRepo
	remoteReposChanFinished chan bool

	remoteRepos sync.Map
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
		remoteReposChan:         make(chan RemoteRepo, RemoteReposChannelSize),
		remoteReposChanFinished: make(chan bool),
		remoteRepos:             sync.Map{},
	}

	go p.startRemoteReposChanListener()
	return p
}

func (p *syncReposWorkerPool) getAllRemoteRepos() (repos []string) {
	p.remoteRepos.Range(func(key, value interface{}) bool {
		repos = append(repos, key.(string))
		return true
	})
	return
}

func (p *syncReposWorkerPool) createJob(r RemoteRepo) func(context.Context) error {
	if r.RepoName.String() == "" {
		panic("RepoName is empty")
	}
	return func(ctx context.Context) error {
		if ctx.Err() == context.Canceled {
			return ctx.Err()
		}

		p.remoteRepos.Store(r.RepoName.String(), r)

		err := p.doWork(r)
		if err != nil {
			p.progressWriter.InfoWithSignalInteruptRaceDelay(ctx, err.Error())
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

func (p *syncReposWorkerPool) waitForRemoteReposChan() {
	<-p.remoteReposChanFinished
	close(p.remoteReposChanFinished)
}

type RemoteRepo struct {
	RepoName      RepoName
	CloneUrl      string
	IsArchived    bool
	DefaultBranch string
}

func (p *syncReposWorkerPool) Wait() error {
	p.waitForRemoteReposChan()

	err := p.workerPool.Wait()
	if err != nil {
		return fmt.Errorf("couldn't sync all repos: %w", err)
	}
	return nil
}

func (p *syncReposWorkerPool) canIgnore(r RemoteRepo) bool {
	cleanRepoName := MustParseRepoName(r.CloneUrl)
	if p.ignore.MatchesPath(cleanRepoName.String()) {
		p.progressWriter.EventIgnoredRepo(cleanRepoName.String())
		return true
	}

	return false
}

func (p *syncReposWorkerPool) doWork(r RemoteRepo) error {
	gitUrl, _ := url.Parse(r.CloneUrl)
	localDir := getLocalDir(gitUrl)
	localDirExists := dirExists(localDir)

	if r.IsArchived {
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
			err := c.doUpdate(gitUrl, r.DefaultBranch)
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
	newArchivedDir := filepath.Join(getWorkspaceDir(), archiveDir, rel)
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
