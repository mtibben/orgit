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

func isGitRepo(dir string) bool {
	return dirExists(filepath.Join(dir, ".git"))
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
		Use:   "get [flags] PROJECT_URL[@COMMIT]...",
		Short: "Clone or checkout a git repository into the workspace directory",
		Long: `Clone or checkout a git repository into the workspace directory.

If the worktree has been modified, the changes are stashed.

Arguments:
  PROJECT_URL  URL of the gitlab or github project, or a relative path to a project in the default directory
  COMMIT       The ref name or hash to checkout. Defaults to the remote HEAD.
`,
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			for _, gitUrlArg := range args {
				gitUrl, branchOrCommit, err := parseArgsForGetCmd(gitUrlArg)
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
			}
		},
	}

	cmdGet.Flags().BoolVar(&update, "update", false, "Stash uncommitted changes and pull the latest changes from the remote")

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
		if !isGitRepo(c.WorkingDir) {
			return fmt.Errorf("'%s' already exists but is not a git repository", c.WorkingDir)
		}
		if update {
			fmt.Fprintf(os.Stderr, "In '%s'\n", c.WorkingDir)
			return c.doUpdate(gitUrl, branchOrCommit)
		}
	} else {
		return c.doClone(gitUrl.String(), branchOrCommit)
	}
	return nil
}

func (c *getCmdContext) findDefaultBranchNameWithSetHead() (string, error) {
	// or perhaps we need to resync?
	setHeadResult, err := c.doExec(`git remote set-head origin --auto`)
	if err != nil {
		return "", err
	}

	// regex extract the branch name from the output 'origin/HEAD set to BRANCH'
	matches := regexp.MustCompile(`origin/HEAD set to (.+)`).FindStringSubmatch(setHeadResult)
	if len(matches) != 2 {
		return "", fmt.Errorf("can't find default branch name in output '%s'", setHeadResult)
	}

	return matches[1], nil
}

var errNoCommits = fmt.Errorf("no commits")

func (c *getCmdContext) hasNoCommits() bool {
	logOut, err := c.doExec(`git log -n 1`)
	return err != nil && strings.Contains(logOut, "does not have any commits yet")
}

func (c *getCmdContext) getDefaultBranchName() (string, error) {
	defaultBranch, err1 := c.doExec(`git symbolic-ref --short refs/remotes/origin/HEAD`)
	if err1 != nil {

		if c.hasNoCommits() {
			return "", errNoCommits
		}

		var err2 error
		defaultBranch, err2 = c.findDefaultBranchNameWithSetHead()
		if err2 != nil {
			// can't find default branch name, return the original err1
			return "", err1
		}
	} else {
		defaultBranch = strings.TrimPrefix(defaultBranch, "origin/")
	}

	return defaultBranch, nil
}

func (c *getCmdContext) isDetachedHead() bool {
	out, err := c.doExec(`git rev-parse --abbrev-ref --symbolic-full-name HEAD`)
	if err != nil {
		panic(err)
	}
	return out == "HEAD"
}

func (c *getCmdContext) isSymbolicRef(ref string) bool {
	err := c.echoEvalf(`git symbolic-ref %s`, ref)
	return err == nil
}

func (c *getCmdContext) fixRemoteConfig(gitUrl *url.URL) error {
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

	return nil
}

func (c *getCmdContext) isLocked() bool {
	return fileExists(filepath.Join(c.WorkingDir, ".git", "index.lock"))
}

func (c *getCmdContext) stash() error {
	err := c.echoEval(`git stash push --include-untracked --message "orgit"`)
	if err != nil {
		if c.hasNoCommits() {
			return errNoCommits
		}
	}
	return err
}

func (c *getCmdContext) isABranch(branchOrCommit string) bool {
	return !c.isSymbolicRef(branchOrCommit)
}

func (c *getCmdContext) doUpdate(gitUrl *url.URL, branchOrCommit string) error {
	if c.isLocked() {
		return fmt.Errorf("can't update '%s', another git process seems to be running in this repository: .git/index.lock exists", c.WorkingDir)
	}

	err := c.fixRemoteConfig(gitUrl)
	if err != nil {
		return err
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

	// don't want to clobber a git repo in a detached state
	if c.isDetachedHead() {
		return fmt.Errorf("can't update '%s', HEAD is detached", c.WorkingDir)
	}

	// optimistically stash any uncommitted changes
	err = c.stash()
	if err == errNoCommits {
		return nil // nothing we can do on a repo without commits
	} else if err != nil {
		return err
	}

	err = c.echoEvalf(`git checkout %s`, branchOrCommit)
	if err != nil {
		return err
	}

	if c.isABranch(branchOrCommit) {
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

	err := c.echoEvalf(`git clone --recursive %s %s`, gitUrl, destinationDir)
	if err != nil {
		return err
	}
	if branchOrCommit != "" {
		return c.echoEvalf(`git checkout %s`, branchOrCommit)
	}

	return nil
}
