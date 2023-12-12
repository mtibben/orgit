package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/99designs/grit/syncprinter"
	"github.com/spf13/cobra"
)

var ignoreDirs []string = []string{
	".archive", // compatibility with git-workspace
}

func printDirs(baseDir, dir string, printFullPath, flagDirty bool) {
	fullDir := filepath.Join(baseDir, dir)
	if printFullPath {
		dir = fullDir
	}
	if flagDirty {
		out, err := doExecQuietWithOutput(fullDir, "git status --porcelain")
		if err != nil {
			syncprinter.Println(err.Error())
			return
		}

		if len(out) > 0 {
			fmt.Println(dir)
		}
	} else {
		fmt.Println(dir)
	}
}

func init() {
	var flagDirty bool
	var printFullPath bool

	var cmdList = &cobra.Command{
		Use:   "list",
		Short: "List git repositories",
		Long:  "List git repositories in DIR, or in the workspace path if DIR is not specified.",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			baseDir := getWorkspaceDir()

			wg := sync.WaitGroup{}
			forEachGitDirIn(baseDir, func(relativeDir string) {
				wg.Add(1)
				go func() {
					defer wg.Done()

					printDirs(baseDir, relativeDir, printFullPath, flagDirty)
				}()
			})
			wg.Wait()
		},
	}

	cmdList.Flags().BoolVar(&flagDirty, "dirty", false, "Filter by git directories with uncommitted changes")
	cmdList.Flags().BoolVar(&printFullPath, "full-path", false, "Print the absolute path of each git directory")
	rootCmd.AddCommand(cmdList)
}

func forEachGitDirIn(baseDir string, doFunc func(path string)) {
	fsys := os.DirFS(baseDir)
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			panic(err)
		}

		if d.IsDir() {
			if slices.Contains(ignoreDirs, d.Name()) {
				return fs.SkipDir
			}

			if i, err := fs.Stat(fsys, filepath.Join(path, ".git")); err == nil {
				if i.IsDir() {
					doFunc(path)
				}
				return fs.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
}
