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
 3. archive local repos that have been archived remotely by moving them to $GITORG_WORKSPACE/.archive
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
	logger := NewProgressLogger(loglevel)
	defer logger.EndProgressLine("done")

	ctx = handleSigterm(ctx, logger)

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
	logger.Info(fmt.Sprintf("Syncing to '%s'", localDir))
	workerPool := NewSyncReposWorkerPool(ctx, clone, update, archive, logger)

	err = provider.ListRepos(ctx, org, archive, workerPool.GoGetUrl)
	if err != nil {
		return fmt.Errorf("couldn't list repos for '%s': %w", orgUrlStr, err)
	}

	if err := workerPool.Wait(); err != nil {
		return err
	}

	return nil
}

func handleSigterm(ctx context.Context, logger *ProgressLogger) context.Context {
	var cancelFunc context.CancelFunc
	ctx, cancelFunc = context.WithCancel(ctx)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		logger.EndProgressLine("cancelling...")
		cancelFunc()
		os.Exit(1)
	}()

	return ctx
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

const MaxGoroutines = 100

func NewSyncReposWorkerPool(ctx context.Context, clone, update, archive bool, progressWriter *ProgressLogger) *syncReposWorkerPool {
	p := &syncReposWorkerPool{
		workerPool:              pool.New().WithErrors().WithMaxGoroutines(MaxGoroutines).WithContext(ctx),
		cloneRepos:              clone,
		updateRepos:             update,
		archiveRepos:            archive,
		progressWriter:          progressWriter,
		ignore:                  getIgnore(),
		remoteReposChan:         make(chan remoteRepo, MaxGoroutines*5),
		remoteReposChanFinished: make(chan bool),
	}

	go p.startRemoteReposChanListener()
	return p
}

func (p *syncReposWorkerPool) createJob(r remoteRepo) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return p.doWork(r)
	}
}

func (p *syncReposWorkerPool) startRemoteReposChanListener() {
	for r := range p.remoteReposChan {
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

func (p *syncReposWorkerPool) GoGetUrl(r remoteRepo) {
	if p.canIgnore(r) {
		return
	}
	p.progressWriter.AddTotalToProgress(1)
	p.remoteReposChan <- r
}

func (p *syncReposWorkerPool) Wait() error {
	close(p.remoteReposChan)
	<-p.remoteReposChanFinished
	return p.workerPool.Wait()
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
