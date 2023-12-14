package cmd

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var osUserHomeDir = os.UserHomeDir

func mustParseGitRepo(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

var sshLocationFormat = regexp.MustCompile(`^([^@]+@[^:]+):(.+)$`)

func getGitUrl(origGitUrlStr string) (*url.URL, error) {
	gitUrlStr := origGitUrlStr

	// normalise git URL for known git providers
	for _, provider := range KnownGitProviders {
		if provider.IsMatch(origGitUrlStr) {
			gitUrlStr = provider.NormaliseGitUrl(origGitUrlStr)
			break
		}
	}

	gitUrl, err := url.Parse(gitUrlStr)
	if err != nil {
		if sshLocationFormat.MatchString(gitUrlStr) {
			parts := sshLocationFormat.FindStringSubmatch(gitUrlStr)
			gitUrl = mustParseGitRepo(fmt.Sprintf("ssh://%s/%s", parts[2], parts[3]))
		} else {
			return nil, fmt.Errorf("invalid git url '%s", origGitUrlStr)
		}
	}

	return gitUrl, nil
}

func parseArgsForGetCmd(projectUrl string) (gitUrl *url.URL, commitOrBranch string, err error) {
	arg0parts := strings.Split(projectUrl, "@")
	projectUrlStr := arg0parts[0]

	if len(arg0parts) > 1 {
		commitOrBranch = arg0parts[1]
	}

	gitUrl, err = getGitUrl(projectUrlStr)
	if err != nil {
		return nil, "", err
	}

	return gitUrl, commitOrBranch, nil
}

func getLocalDir(gitUrl *url.URL) string {
	localDir := filepath.Join(getWorkspaceDir(), gitUrl.Host, gitUrl.Path)
	localDir = strings.TrimSuffix(localDir, ".git")

	return localDir
}

func newShellCmd(shCmd string) *exec.Cmd {
	return exec.Command("sh", "-c", shCmd)
}

func (c *getCmdContext) doExec(shCmd string) (string, error) {
	cmd := newShellCmd(shCmd)
	cmd.Dir = c.WorkingDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("error executing '%s' in directory '%s': %w", shCmd, c.WorkingDir, err)
	}
	return strings.TrimSpace(string(out)), err
}

func dirExists(dir string) bool {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func (c *getCmdContext) echoEvalf(shCmd string, a ...any) error {
	return c.echoEval(fmt.Sprintf(shCmd, a...))
}

func (c *getCmdContext) echoEval(shCmd string) error {
	c.CmdEchoFunc(shCmd, c.WorkingDir)
	cmd := newShellCmd(shCmd)
	cmd.Dir = c.WorkingDir
	cmd.Stdout = c.Stdout
	cmd.Stderr = c.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error executing '%s' in directory '%s': %w", shCmd, c.WorkingDir, err)
	}
	return nil
}

func init() {
	var update bool

	var cmdGet = &cobra.Command{
		Use:   "get [flags] PROJECT_URL[@COMMIT]",
		Short: "Clone or checkout a Git repository into the workspace directory",
		Long: `Clone or checkout a Git repository into the workspace directory.

If the worktree has been modified, the changes are stashed.

Arguments:
  PROJECT_URL  URL of the gitlab or github project, or a relative path to a project in the default directory
  COMMIT       The ref name or hash to checkout. Defaults to the remote HEAD.
`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {

			gitUrl, branchOrCommit, err := parseArgsForGetCmd(args[0])
			if err != nil {
				cmd.PrintErrln(err)
				os.Exit(1)
			}

			dir := getLocalDir(gitUrl)

			getCmdContext := &getCmdContext{
				Stdout:      os.Stdout,
				Stderr:      os.Stderr,
				CmdEchoFunc: func(cmd, dir string) { color.Cyan(" + %s", cmd) },
				WorkingDir:  dir,
			}
			err = getCmdContext.doGet(gitUrl, branchOrCommit, update)
			if err != nil {
				cmd.PrintErrln(err)
				os.Exit(1)
			}
		},
	}

	cmdGet.Flags().BoolVar(&update, "update", false, "Stash uncommitted changes and switch to origin HEAD")

	rootCmd.AddCommand(cmdGet)
}

type getCmdContext struct {
	WorkingDir  string
	Stdout      io.Writer
	Stderr      io.Writer
	CmdEchoFunc func(cmd, dir string)
}

func (c *getCmdContext) doGet(gitUrl *url.URL, branchOrCommit string, update bool) error {
	if dirExists(c.WorkingDir) {
		if update {
			fmt.Fprintf(os.Stderr, "In '%s'\n", c.WorkingDir)
			err := c.doUpdate(gitUrl, branchOrCommit)
			if err != nil {
				return err
			}
		}
		// FIXME: check if the dir is actually a git repo
	} else {
		err := c.doClone(gitUrl.String(), branchOrCommit)
		if err != nil {
			return err
		}
	}
	return nil
}

var errNoCommits = fmt.Errorf("no commits")

func (c *getCmdContext) getDefaultBranchName() (string, error) {
	defaultBranch, err := c.doExec(`git symbolic-ref --short refs/remotes/origin/HEAD`)
	if err != nil {
		logOut, err2 := c.doExec(`git log -n 1`)
		if err2 != nil && strings.Contains(logOut, "does not have any commits yet") {
			return "", errNoCommits
		}
		return "", err
	}
	defaultBranch = strings.TrimPrefix(defaultBranch, "origin/")

	return defaultBranch, nil
}

func (c *getCmdContext) isGitDirty() (bool, error) {
	gitStatusPorcelain, err := c.doExec("git status --porcelain")
	if err != nil {
		return false, err
	}

	return gitStatusPorcelain != "", nil
}

func (c *getCmdContext) isDetatched() bool {
	_, err := c.doExec("git symbolic-ref HEAD")
	return err != nil
}

func (c *getCmdContext) doUpdate(gitUrl *url.URL, branchOrCommit string) error {
	remoteOriginUrl, err := c.doExec(`git config --get remote.origin.url`)
	if err != nil {
		return err
	}
	if remoteOriginUrl != gitUrl.String() {
		err := c.echoEvalf(`git remote set-url origin %s`, gitUrl.String())
		if err != nil {
			return err
		}
	}

	err = c.echoEvalf(`git fetch origin`)
	if err != nil {
		return err
	}

	if branchOrCommit == "" {
		branchOrCommit, err = c.getDefaultBranchName()
		if err == errNoCommits {
			return nil // nothing we can do on a repo without commits
		} else if err != nil {
			return err
		}
	}

	if c.isDetatched() {
		return fmt.Errorf("can't update '%s', git directory is in detached head state", c.WorkingDir)
	}

	isGitDirty, err := c.isGitDirty()
	if err != nil {
		return err
	}
	if isGitDirty {
		err = c.echoEval(`git stash -u`)
		if err != nil {
			return err
		}
	}

	err = c.echoEvalf(`git checkout %s`, branchOrCommit)
	if err != nil {
		return err
	}

	localHead, err := c.doExec(`git rev-parse HEAD`)
	if err != nil {
		return err
	}
	remoteHead, _ := c.doExec(`git rev-parse @{u}`)

	needsFastForward := (localHead != remoteHead)
	if needsFastForward {
		err = c.echoEvalf(`git merge --ff-only @{u}`)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *getCmdContext) doClone(gitUrl, branchOrCommit string) error {
	destinationDir := c.WorkingDir
	c.WorkingDir = ""
	gitCloneArgs := "--recursive"
	if branchOrCommit != "" {
		gitCloneArgs = fmt.Sprintf("%s --branch %s", gitCloneArgs, branchOrCommit)
	}

	err := c.echoEvalf(`git clone %s %s %s`, gitCloneArgs, gitUrl, destinationDir)
	if err != nil {
		return err
	}

	return nil
}
