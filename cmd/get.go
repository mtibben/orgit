package cmd

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

type RepoProvider struct {
	prefix       string
	appendPrefix string
	appendSuffix string
}

func (p RepoProvider) IsMatch(s string) bool {
	return strings.HasPrefix(s, p.prefix)
}

func (p RepoProvider) NormaliseGitUrl(s string) string {
	return p.appendPrefix + s + p.appendSuffix
}

var KnownGitProviders = []RepoProvider{
	{"github.com/", "https://", ".git"},
	{"gitlab.com/", "https://", ".git"},
}

// const defaultBaseUrl = "https://gitlab.com/vistaprint-org"

// var osGetwd = os.Getwd
var osUserHomeDir = os.UserHomeDir

// func mustGetCwd() string {
// 	cwd, err := osGetwd()
// 	if err != nil {
// 		panic(err)
// 	}
// 	return cwd
// }

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

// func newShellCmd(shCmd string) *exec.Cmd {
// 	args, err := shellwords.Parse(shCmd)
// 	if err != nil {
// 		panic(err)
// 	}
// 	return exec.Command(args[0], args[1:]...)
// }

func mustExec(dir string, shCmd string) string {
	out, err := doExec(dir, shCmd)
	if err != nil {
		fmt.Printf("in directory '%s' executing '%s': %s\n", dir, shCmd, out)
		panic(err)
	}
	return out
}

func doExec(dir string, shCmd string) (string, error) {
	cmd := newShellCmd(shCmd)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func isGitDirty(dir string) bool {
	return mustExec(dir, "git status --porcelain") != ""
	// return mustExec(dir, "git diff-index HEAD") != "" // fails when HEAD is not present
}

func dirExists(dir string) bool {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func (c *cmdContext) mustEchoEvalf(dir, shCmd string, a ...any) {
	c.mustEchoEval(dir, fmt.Sprintf(shCmd, a...))
}

func (c *cmdContext) mustEchoEval(dir, shCmd string) {
	c.EchoFunc(" + %s", shCmd)
	cmd := newShellCmd(shCmd)
	cmd.Dir = dir
	cmd.Stdout = c.Stdout
	cmd.Stderr = c.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatalf("in directory '%s' executing '%s': %s", dir, shCmd, err)
	}
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
			remoteOriginUrl := mustExec(dir, `git config --get remote.origin.url`)
			if remoteOriginUrl != gitUrl.String() {
				c.mustEchoEvalf(dir, `git remote set-url origin %s`, gitUrl.String())
			}
		}

		c.mustEchoEvalf(dir, `git fetch`)

		if pristine {
			if branchOrCommit == "latest" {
				branchOrCommit = mustExec(dir, `git symbolic-ref --short refs/remotes/origin/HEAD`)
			} else {
				branchOrCommit = fmt.Sprintf("origin/%s", branchOrCommit)
			}
		}

		if isGitDirty(dir) {
			if pristine {
				c.mustEchoEval(dir, `git stash -u`)
			} else {
				return errors.New("Git directory is dirty, please stash changes first")
			}
		}

		c.mustEchoEvalf(dir, `git checkout %s`, branchOrCommit)

		localHead := mustExec(dir, `git rev-parse HEAD`)
		remoteHead := mustExec(dir, `git rev-parse @{u}`)
		if localHead != remoteHead {
			c.mustEchoEvalf(dir, `git merge --ff-only @{u}`)
		}

		if pristine {
			c.mustEchoEval(dir, `git clean -ffdx`)
		}
	} else {

		gitCloneArgs := "--recursive"
		if quiet {
			gitCloneArgs = "--recursive --quiet"
		}
		c.mustEchoEvalf("", `git clone %s %s %s`, gitCloneArgs, gitUrl, dir)
		if branchOrCommit != "" {
			c.mustEchoEvalf(dir, `git checkout %s`, branchOrCommit)
		}
	}

	return nil
}
