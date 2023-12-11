package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func syncPrintln(a ...any) {
	doSyncFunc(func() {
		fmt.Println(a...)
	})
}

func syncPrintStdErrln(a ...any) {
	doSyncFunc(func() {
		fmt.Fprintln(os.Stderr, a...)
	})
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
	Short: "Grit is a tool for interacting with multiple repos concurrently",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
