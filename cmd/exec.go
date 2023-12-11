package cmd

import (
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

var outputMutex sync.Mutex

func doSyncFunc(f func()) {
	outputMutex.Lock()
	defer outputMutex.Unlock()
	f()
}

func doExecCmd(path string, theCmd string) {
	cmd := newShellCmd(theCmd)
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(out))

	doSyncFunc(func() {
		if len(outStr) > 0 || err != nil {
			exitCode := cmd.ProcessState.ExitCode()
			fmt.Printf("in %s: exit status %d\n", path, exitCode)
			fmt.Println(string(out))
			fmt.Println()
		}
	})
}

func init() {
	var cmdExec = &cobra.Command{
		Use:   "exec CMD...",
		Short: "Exec a command in each git directory",
		Args:  cobra.RangeArgs(1, 2),
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
					doExecCmd(path, strings.Join(args, " "))
				}()
			})
			wg.Wait()
		},
	}

	rootCmd.AddCommand(cmdExec)
}
