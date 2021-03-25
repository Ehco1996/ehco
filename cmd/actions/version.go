package actions

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Ehco1996/ehco/internal/constant"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of ehco",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("ehco(https://github.com/Ehco1996) now version is:", constant.Version)
	},
}
