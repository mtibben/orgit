package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	ignore "github.com/sabhiram/go-gitignore"
	"github.com/spf13/cobra"
)

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func getIgnore() *ignore.GitIgnore {
	i, err := ignore.CompileIgnoreFile(filepath.Join(getWorkspaceDir(), ".orgitignore"))
	if err != nil {
		return ignore.CompileIgnoreLines()
	}
	return i
}

func getWorkspaceDir() string {
	return sync.OnceValue(func() string {
		baseDirFromEnv := os.Getenv("ORGIT_WORKSPACE")
		if baseDirFromEnv != "" {
			return baseDirFromEnv
		}

		homedir, err := osUserHomeDirFunc()
		if err != nil {
			panic(err)
		}
		return filepath.Join(homedir, "orgit")
	})()
}

var rootCmd = &cobra.Command{
	Use:   "orgit",
	Short: "orgit is a tool for organising git repositories",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
