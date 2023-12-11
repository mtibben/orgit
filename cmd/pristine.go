package cmd

import (
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/spf13/cobra"
)

func cleanString(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsGraphic(r) {
			return r
		}
		return ' '
	}, s)
}

func doExecQuiet(dir string, shCmd string) error {
	cmd := newShellCmd(shCmd)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s: exit status %d: %s", dir, shCmd, cmd.ProcessState.ExitCode(), cleanString(string(out)))
	}
	return nil
}

func doExecQuietWithOutput(dir string, shCmd string) (string, error) {
	cmd := newShellCmd(shCmd)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	singleLineOut := cleanString(string(out))
	if err != nil {
		return singleLineOut, fmt.Errorf("%s: %s: exit status %d: %s", dir, shCmd, cmd.ProcessState.ExitCode(), singleLineOut)
	}
	return singleLineOut, nil
}

func doPristine(path string) {
	err := doExecQuiet(path, `git fetch`)
	if err != nil {
		syncPrintln(err.Error())
		return
	}

	commit, err := doExecQuietWithOutput(path, `git symbolic-ref --short refs/remotes/origin/HEAD`)
	if err != nil {
		syncPrintln(err.Error())
		return
	}

	err = doExecQuiet(path, `git stash -u`)
	if err != nil {
		syncPrintln(err.Error())
		return
	}

	err = doExecQuiet(path, fmt.Sprintf(`git reset --hard %s`, commit))
	if err != nil {
		syncPrintln(err.Error())
		return
	}

	err = doExecQuiet(path, `git clean -ffdx`)
	if err != nil {
		syncPrintln(err.Error())
		return
	}
}

func init() {
	var cmdPristine = &cobra.Command{
		Use:   "pristine [DIR]",
		Short: "Return git directory to pristine state by stashing, resetting, and cleaning",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			baseDir := getWorkspaceDir()
			if len(args) == 1 {
				baseDir = args[0]
			}

			wg := sync.WaitGroup{}
			forEachGitDirIn(baseDir, func(path string) {
				wg.Add(1)
				go func() {
					defer wg.Done()

					doPristine(path)
				}()
			})
			wg.Wait()
		},
	}

	rootCmd.AddCommand(cmdPristine)
}
