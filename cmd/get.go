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
	cmd.Dir = c.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("error executing '%s' in directory '%s': %w", shCmd, c.Dir, err)
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
	c.CmdEchoFunc(shCmd, c.Dir)
	cmd := newShellCmd(shCmd)
	cmd.Dir = c.Dir
	cmd.Stdout = c.Stdout
	cmd.Stderr = c.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error executing '%s' in directory '%s': %w", shCmd, c.Dir, err)
	}
	return nil
}

func init() {
	var pristine bool

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
			quiet := false

			if !quiet {
				fmt.Fprintf(os.Stderr, "In '%s'\n", dir)
			}

			getCmdContext := &getCmdContext{
				Stdout:      os.Stdout,
				Stderr:      os.Stderr,
				CmdEchoFunc: func(cmd, dir string) { color.Cyan(" + %s", cmd) },
				Dir:         dir,
			}
			err = getCmdContext.doGet(gitUrl, branchOrCommit, pristine, quiet)
			if err != nil {
				cmd.PrintErrln(err)
				os.Exit(1)
			}
		},
	}

	cmdGet.Flags().BoolVar(&pristine, "pristine", false, "Stash, reset and clean the repo first")

	rootCmd.AddCommand(cmdGet)
}

type getCmdContext struct {
	Dir         string
	Stdout      io.Writer
	Stderr      io.Writer
	CmdEchoFunc func(cmd, dir string)
}

func (c *getCmdContext) doGet(gitUrl *url.URL, branchOrCommit string, pristine bool, quiet bool) error {
	if dirExists(c.Dir) {
		if pristine {
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
		}

		err := c.echoEvalf(`git fetch`)
		if err != nil {
			return err
		}

		if branchOrCommit == "" {
			defaultBranch, err := c.doExec(`git symbolic-ref --short refs/remotes/origin/HEAD`)
			if err != nil {
				logOut, err2 := c.doExec(`git log -n 1`)
				if err2 != nil && strings.Contains(logOut, "does not have any commits yet") {
					return nil // nothing we can do on a repo without commits
				}
				return err
			}
			defaultBranch = strings.TrimPrefix(defaultBranch, "origin/")

			branchOrCommit = defaultBranch
		}

		gitStatusPorcelain, err := c.doExec("git status --porcelain")
		if err != nil {
			return err
		}
		isGitDirty := gitStatusPorcelain != ""
		if isGitDirty {
			if pristine {
				err = c.echoEval(`git stash -u`)
				if err != nil {
					return err
				}
			} else {
				return fmt.Errorf("%s: git directory is dirty, please stash changes first or use --pristine", c.Dir)
			}
		}

		_, err = c.doExec("git symbolic-ref HEAD")
		isABranch := err == nil

		if isABranch {
			err = c.echoEvalf(`git checkout %s`, branchOrCommit)
			if err != nil {
				return err
			}

			localHead, err := c.doExec(`git rev-parse HEAD`)
			if err != nil {
				return err
			}
			remoteHead, _ := c.doExec(`git rev-parse @{u}`)

			needsFastForward := (localHead != remoteHead) && isABranch

			if needsFastForward {
				err = c.echoEvalf(`git merge --ff-only @{u}`)
				if err != nil {
					return err
				}
			}
		} else {
			return fmt.Errorf("%s: git directory is in detached head state, please checkout a branch first", c.Dir)
		}

		if pristine {
			err = c.echoEval(`git clean -ffdx`)
			if err != nil {
				return err
			}
		}
	} else {
		destinationDir := c.Dir
		c.Dir = ""
		gitCloneArgs := "--recursive"
		if quiet {
			gitCloneArgs = "--recursive --quiet"
		}
		err := c.echoEvalf(`git clone %s %s %s`, gitCloneArgs, gitUrl, destinationDir)
		if err != nil {
			return err
		}
		if branchOrCommit != "" {
			err = c.echoEvalf(`git checkout %s`, branchOrCommit)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
