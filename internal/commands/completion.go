package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for bramble.

To load completions:

Bash:
  $ source <(bramble completion bash)
  # To install permanently:
  $ bramble completion bash > /etc/bash_completion.d/bramble

Zsh:
  $ bramble completion zsh > "${fpath[1]}/_bramble"

Fish:
  $ bramble completion fish | source
  # To install permanently:
  $ bramble completion fish > ~/.config/fish/completions/bramble.fish

PowerShell:
  PS> bramble completion powershell | Out-String | Invoke-Expression`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return rootCmd.GenBashCompletion(os.Stdout)
			case "zsh":
				return rootCmd.GenZshCompletion(os.Stdout)
			case "fish":
				return rootCmd.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("bramble-cli: unsupported shell: %s", args[0])
			}
		},
	}
}
