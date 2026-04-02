package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dalsoop/dalcenter/internal/daemon"
	"github.com/spf13/cobra"
)

func newChannelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channel",
		Short: "Manage Mattermost channels",
	}
	cmd.AddCommand(
		newChannelCreateCmd(),
		newChannelDeleteCmd(),
		newChannelListCmd(),
	)
	return cmd
}

func newChannelCreateCmd() *cobra.Command {
	var purpose string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an MM channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemon.NewClient()
			if err != nil {
				return err
			}
			ch, err := client.ChannelCreate(args[0], purpose)
			if err != nil {
				return err
			}
			fmt.Printf("[channel] created %s (id=%s)\n", ch.Name, ch.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&purpose, "purpose", "", "Channel purpose/description")
	return cmd
}

func newChannelDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an MM channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemon.NewClient()
			if err != nil {
				return err
			}
			if err := client.ChannelDelete(args[0]); err != nil {
				return err
			}
			fmt.Printf("[channel] deleted %s\n", args[0])
			return nil
		},
	}
}

func newChannelListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List MM channels in the team",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemon.NewClient()
			if err != nil {
				return err
			}
			channels, err := client.ChannelList()
			if err != nil {
				return err
			}
			if len(channels) == 0 {
				fmt.Println("No channels")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTYPE\tPURPOSE")
			for _, ch := range channels {
				purpose := ch.Purpose
				if len(purpose) > 50 {
					purpose = purpose[:50] + "..."
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", ch.Name, ch.Type, purpose)
			}
			w.Flush()
			fmt.Printf("\nTotal: %d channel(s)\n", len(channels))
			return nil
		},
	}
}
