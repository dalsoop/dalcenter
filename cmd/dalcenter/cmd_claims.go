package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/dalsoop/dalcenter/internal/daemon"
	"github.com/spf13/cobra"
)

func newClaimsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claims",
		Short: "Manage dal claims and improvement requests",
	}
	cmd.AddCommand(
		newClaimsListCmd(),
		newClaimsRespondCmd(),
	)
	return cmd
}

func newClaimsListCmd() *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List claims from dals",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemon.NewClient()
			if err != nil {
				return err
			}
			claims, err := client.Claims(status)
			if err != nil {
				return err
			}
			if len(claims) == 0 {
				if status != "" {
					fmt.Printf("No %s claims\n", status)
				} else {
					fmt.Println("No claims")
				}
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tDAL\tTYPE\tSTATUS\tTITLE")
			fmt.Fprintln(w, strings.Repeat("-", 80))
			for _, c := range claims {
				title := c.Title
				if len(title) > 40 {
					title = title[:40] + "..."
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", c.ID, c.Dal, c.Type, c.Status, title)
			}
			w.Flush()
			fmt.Printf("\nTotal: %d claim(s)\n", len(claims))
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Filter by status: open|acknowledged|resolved|rejected")
	return cmd
}

func newClaimsRespondCmd() *cobra.Command {
	var status, response string
	cmd := &cobra.Command{
		Use:   "respond <claim-id>",
		Short: "Respond to a dal claim",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemon.NewClient()
			if err != nil {
				return err
			}
			if err := client.ClaimRespond(args[0], status, response); err != nil {
				return err
			}
			fmt.Printf("[claims] %s → %s\n", args[0], status)
			if response != "" {
				fmt.Printf("[claims] response: %s\n", response)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "acknowledged", "Response status: acknowledged|resolved|rejected")
	cmd.Flags().StringVar(&response, "response", "", "Response message to dal")
	return cmd
}
