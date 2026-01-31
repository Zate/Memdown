package hook

import (
	"github.com/spf13/cobra"
)

var HookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Hook subcommands for Claude Code integration",
}

func init() {
	HookCmd.AddCommand(sessionStartCmd, promptSubmitCmd, stopCmd)
}
