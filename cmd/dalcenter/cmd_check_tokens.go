package main

import (
	"fmt"
	"os"

	"github.com/dalsoop/dalcenter/internal/daemon"
	"github.com/spf13/cobra"
)

func newCheckTokensCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check-tokens",
		Short: "Check for shared MM bot tokens across teams (self-message bug)",
		RunE: func(cmd *cobra.Command, args []string) error {
			dupes := daemon.CheckBridgeTokens()
			if dupes == nil {
				fmt.Println("OK: all teams use unique MM bot tokens (or no bridge configs found)")
				return nil
			}

			fmt.Fprintf(os.Stderr, "WARNING: shared MM bot tokens detected!\n\n")
			fmt.Fprintf(os.Stderr, "When teams share a bot token, matterbridge treats messages from\n")
			fmt.Fprintf(os.Stderr, "other teams as self-messages and drops them silently.\n\n")

			for token, teams := range dupes {
				masked := token
				if len(masked) > 8 {
					masked = masked[:4] + "..." + masked[len(masked)-4:]
				}
				fmt.Fprintf(os.Stderr, "  Token %s shared by: %v\n", masked, teams)
			}

			fmt.Fprintf(os.Stderr, "\nFix: assign a unique bot account+token per team in each\n")
			fmt.Fprintf(os.Stderr, "     /etc/dalcenter/<team>.matterbridge.toml\n")
			return fmt.Errorf("duplicate bridge tokens found")
		},
	}
}
