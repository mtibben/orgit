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

// func mustDoExec(dir string, shCmd string) string {
// 	out, err := doExec(dir, shCmd)
// 	if err != nil {
// 		panic(err)
// 	}
// 	return out
// }

func doExec(dir string, shCmd string) (string, error) {
	cmd := newShellCmd(shCmd)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("error executing '%s' in directory '%s': %w", shCmd, dir, err)
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

func (c *cmdContext) echoEvalf(dir, shCmd string, a ...any) error {
	return c.echoEval(dir, fmt.Sprintf(shCmd, a...))
}

func (c *cmdContext) echoEval(dir, shCmd string) error {
	c.EchoFunc(" + %s", shCmd)
	cmd := newShellCmd(shCmd)
	cmd.Dir = dir
	cmd.Stdout = c.Stdout
	cmd.Stderr = c.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("error executing '%s' in directory '%s': %w", shCmd, dir, err)
	}
	return nil
}

func init() {
	var pristine bool

	var cmdCheckout = &cobra.Command{
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

			fmt.Fprintf(os.Stderr, "Getting to '%s'\n", dir)

			err = defaultCmdContext.doGet(gitUrl, branchOrCommit, dir, pristine, false)
			if err != nil {
				cmd.PrintErrln(err)
				os.Exit(1)
			}
		},
	}

	cmdCheckout.Flags().BoolVar(&pristine, "pristine", false, "Stash, reset and clean the repo first")

	rootCmd.AddCommand(cmdCheckout)
}

var defaultCmdContext = &cmdContext{
	Stdout:   os.Stdout,
	Stderr:   os.Stderr,
	EchoFunc: color.Cyan,
}

type cmdContext struct {
	Stdout   io.Writer
	Stderr   io.Writer
	EchoFunc func(format string, a ...interface{})
}

func (c *cmdContext) doGet(gitUrl *url.URL, branchOrCommit, dir string, pristine bool, quiet bool) error {
	if dirExists(dir) {
		// validate
		// color.Cyan(` + cd "%s"`, dir)

		if pristine {
			remoteOriginUrl, err := doExec(dir, `git config --get remote.origin.url`)
			if err != nil {
				return err
			}
			if remoteOriginUrl != gitUrl.String() {
				err := c.echoEvalf(dir, `git remote set-url origin %s`, gitUrl.String())
				if err != nil {
					return err
				}
			}
		}

		err := c.echoEvalf(dir, `git fetch`)
		if err != nil {
			return err
		}

		if branchOrCommit == "" {
			defaultBranch, err := doExec(dir, `git symbolic-ref --short refs/remotes/origin/HEAD`)
			if err != nil {
				logOut, err2 := doExec(dir, `git log -n 1`)
				if err2 != nil && strings.Contains(logOut, "does not have any commits yet") {
					return nil // nothing we can do on a repo without commits
				}
				return err
			}
			defaultBranch = strings.TrimPrefix(defaultBranch, "origin/")

			branchOrCommit = defaultBranch
		}

		gitStatusPorcelain, err := doExec(dir, "git status --porcelain")
		if err != nil {
			return err
		}
		isGitDirty := gitStatusPorcelain != ""
		if isGitDirty {
			if pristine {
				err = c.echoEval(dir, `git stash -u`)
				if err != nil {
					return err
				}
			} else {
				return fmt.Errorf("%s: git directory is dirty, please stash changes first or use --pristine", dir)
			}
		}

		_, err = doExec(dir, "git symbolic-ref HEAD")
		isABranch := err == nil

		if isABranch {
			err = c.echoEvalf(dir, `git checkout %s`, branchOrCommit)
			if err != nil {
				return err
			}

			localHead, err := doExec(dir, `git rev-parse HEAD`)
			if err != nil {
				return err
			}
			remoteHead, _ := doExec(dir, `git rev-parse @{u}`)

			needsFastForward := (localHead != remoteHead) && isABranch

			if needsFastForward {
				err = c.echoEvalf(dir, `git merge --ff-only @{u}`)
				if err != nil {
					return err
				}
			}
		} else {
			return fmt.Errorf("%s: git directory is in detached head state, please checkout a branch first", dir)
		}

		if pristine {
			err = c.echoEval(dir, `git clean -ffdx`)
			if err != nil {
				return err
			}
		}
	} else {
		gitCloneArgs := "--recursive"
		if quiet {
			gitCloneArgs = "--recursive --quiet"
		}
		err := c.echoEvalf("", `git clone %s %s %s`, gitCloneArgs, gitUrl, dir)
		if err != nil {
			return err
		}
		if branchOrCommit != "" {
			err = c.echoEvalf(dir, `git checkout %s`, branchOrCommit)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
