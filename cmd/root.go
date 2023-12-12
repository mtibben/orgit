package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/cobra"
)

var doMutex = sync.Mutex{}

func do(f func()) {
	doMutex.Lock()
	defer doMutex.Unlock()
	f()
}

func getWorkspaceDir() string {
	baseDirFromEnv := os.Getenv("GRIT_WORKSPACE")
	if baseDirFromEnv != "" {
		return baseDirFromEnv
	}

	homedir, err := osUserHomeDir()
	if err != nil {
		panic(err)
	}
	return filepath.Join(homedir, "Developer", "src")
}

var rootCmd = &cobra.Command{
	Use:   "grit",
	Short: "Grit is a tool for organising git repositories",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
