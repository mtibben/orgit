package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/cobra"
)

func getWorkspaceDir() string {
	return sync.OnceValue(func() string {
		baseDirFromEnv := os.Getenv("GITORG_WORKSPACE")
		if baseDirFromEnv != "" {
			return baseDirFromEnv
		}

		homedir, err := osUserHomeDir()
		if err != nil {
			panic(err)
		}
		return filepath.Join(homedir, "gitorg")
	})()
}

var rootCmd = &cobra.Command{
	Use:   "gitorg",
	Short: "Gitorg is a tool for organising git repositories",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
