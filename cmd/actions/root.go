package actions

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use: "root",
}

func Execute() {
	cmd, _, err := rootCmd.Find(os.Args[1:])
	// default cmd if no cmd is given
	if err == nil && cmd.Use == rootCmd.Use {
		args := append([]string{ehcoCmd.Use}, os.Args[1:]...)
		rootCmd.SetArgs(args)
	}
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
