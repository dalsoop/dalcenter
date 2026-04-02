package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dalsoop/dalcenter/internal/daemon"
	"github.com/spf13/cobra"
)

func newPipelineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pipeline",
		Short: "Dalroot communication pipeline (MM-based)",
	}
	cmd.AddCommand(
		newPipelineInitCmd(),
		newPipelineSendCmd(),
		newPipelineReceiveCmd(),
		newPipelineBroadcastCmd(),
		newPipelineSyncCmd(),
		newPipelineHealthCmd(),
		newPipelineListCmd(),
	)
	return cmd
}

func newPipelineInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <pane-id>",
		Short: "Initialize pipeline channel for a dalroot pane",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemon.NewClient()
			if err != nil {
				return err
			}
			ch, err := client.PipelineInit(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("[pipeline] initialized: pane=%s channel=%s\n", ch.PaneID, ch.ChannelName)
			return nil
		},
	}
}

func newPipelineSendCmd() *cobra.Command {
	var paneID string
	cmd := &cobra.Command{
		Use:   "send <message>",
		Short: "Send a message to pane's MM channel",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if paneID == "" {
				paneID = os.Getenv("DALROOT_PANE_ID")
			}
			if paneID == "" {
				return fmt.Errorf("--pane or DALROOT_PANE_ID required")
			}
			client, err := daemon.NewClient()
			if err != nil {
				return err
			}
			message := strings.Join(args, " ")
			if err := client.PipelineSend(paneID, message); err != nil {
				return err
			}
			fmt.Printf("[pipeline] sent to %s\n", paneID)
			return nil
		},
	}
	cmd.Flags().StringVar(&paneID, "pane", "", "Pane ID (default: DALROOT_PANE_ID env)")
	return cmd
}

func newPipelineReceiveCmd() *cobra.Command {
	var paneID string
	cmd := &cobra.Command{
		Use:   "receive",
		Short: "Fetch unread messages from pane's MM channel",
		RunE: func(cmd *cobra.Command, args []string) error {
			if paneID == "" {
				paneID = os.Getenv("DALROOT_PANE_ID")
			}
			if paneID == "" {
				return fmt.Errorf("--pane or DALROOT_PANE_ID required")
			}
			client, err := daemon.NewClient()
			if err != nil {
				return err
			}
			msgs, err := client.PipelineReceive(paneID)
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				fmt.Println("No messages")
				return nil
			}
			for _, m := range msgs {
				ts := time.UnixMilli(m.CreatedAt).Format("15:04")
				fmt.Printf("[%s] %s: %s\n", ts, m.Username, m.Message)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&paneID, "pane", "", "Pane ID (default: DALROOT_PANE_ID env)")
	return cmd
}

func newPipelineBroadcastCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "broadcast <message>",
		Short: "Send message to all dalroot pipeline channels",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemon.NewClient()
			if err != nil {
				return err
			}
			message := strings.Join(args, " ")
			if err := client.PipelineBroadcast(message); err != nil {
				return err
			}
			fmt.Println("[pipeline] broadcast sent")
			return nil
		},
	}
}

func newPipelineSyncCmd() *cobra.Command {
	var paneID string
	cmd := &cobra.Command{
		Use:   "sync [message]",
		Short: "Sync pipeline: post user message to MM + fetch unread (UserPromptSubmit hook)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if paneID == "" {
				paneID = os.Getenv("DALROOT_PANE_ID")
			}
			if paneID == "" {
				return fmt.Errorf("--pane or DALROOT_PANE_ID required")
			}
			client, err := daemon.NewClient()
			if err != nil {
				return err
			}
			message := strings.Join(args, " ")
			msgs, err := client.PipelineSync(paneID, message)
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				return nil
			}
			for _, m := range msgs {
				ts := time.UnixMilli(m.CreatedAt).Format("15:04")
				fmt.Printf("[%s] %s: %s\n", ts, m.Username, m.Message)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&paneID, "pane", "", "Pane ID (default: DALROOT_PANE_ID env)")
	return cmd
}

func newPipelineHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check pipeline health (MM connectivity + channel count)",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemon.NewClient()
			if err != nil {
				return err
			}
			h, err := client.PipelineHealth()
			if err != nil {
				return err
			}
			fmt.Printf("MM: %s\n", h.MM)
			fmt.Printf("Channels: %d\n", h.Channels)
			if h.Error != "" {
				fmt.Printf("Error: %s\n", h.Error)
				os.Exit(1)
			}
			return nil
		},
	}
}

func newPipelineListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List pane→channel mappings",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := daemon.NewClient()
			if err != nil {
				return err
			}
			channels, err := client.PipelineList()
			if err != nil {
				return err
			}
			if len(channels) == 0 {
				fmt.Println("No pipeline channels")
				return nil
			}
			fmt.Printf("%-20s %-30s %s\n", "PANE", "CHANNEL", "ID")
			fmt.Println(strings.Repeat("-", 80))
			for _, ch := range channels {
				fmt.Printf("%-20s %-30s %s\n", ch.PaneID, ch.ChannelName, ch.ChannelID)
			}
			return nil
		},
	}
}
