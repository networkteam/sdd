package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	sdd "github.com/networkteam/sdd/pkg/application"
)

// sessionsCmd is the authenticated listing the MCP surface deliberately lacks:
// handles are issued there, never published, so a lost handle is recovered
// here, where the OS user is the principal (d-cpt-aen).
func sessionsCmd() *cli.Command {
	return &cli.Command{
		Name:  "sessions",
		Usage: "List this project's dialogue sessions with open work; abandon stale ones",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			application, project, identity, err := buildLocalStoreApplication(ctx, cmd)
			if err != nil {
				return err
			}
			sessions, err := application.ListWorkflowSessions(ctx, identity, project)
			if err != nil {
				return err
			}
			renderSessions(sessions)
			return nil
		},
		Commands: []*cli.Command{sessionsAbandonCmd()},
	}
}

func sessionsAbandonCmd() *cli.Command {
	return &cli.Command{
		Name:      "abandon",
		Usage:     "Tear down a session by its ID: its open moves are discarded, held WIP markers are left standing",
		ArgsUsage: "<session-id>",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "reason", Usage: "Reason recorded with the abandon"},
		},
		Action: withWriteGate(func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return fmt.Errorf("pass exactly one session ID (see `sdd sessions`)")
			}
			application, _, identity, err := buildLocalStoreApplication(ctx, cmd)
			if err != nil {
				return err
			}
			result, err := application.AbandonWorkflowSession(ctx, identity, sdd.WorkflowResumeRequest{
				SessionID: sdd.SessionID(strings.TrimSpace(cmd.Args().First())), ClientName: "sdd sessions abandon",
			}, cmd.String("reason"))
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "Abandoned %s", result.Session)
			if result.Label != "" {
				fmt.Fprintf(os.Stdout, " %q", result.Label)
			}
			fmt.Fprintln(os.Stdout)
			for _, instance := range result.Discarded {
				fmt.Fprintf(os.Stdout, "  discarded: %s at %s\n", instance.Procedure, instance.Step)
			}
			for _, marker := range result.HeldMarkers {
				fmt.Fprintf(os.Stdout, "  WIP marker left standing: %s\n", marker)
			}
			return nil
		}),
	}
}

func renderSessions(sessions []sdd.WorkflowSessionSummary) {
	shown := 0
	for _, session := range sessions {
		if len(session.Open) == 0 {
			continue
		}
		shown++
		activity := "idle"
		if session.Active {
			activity = "active"
		}
		line := fmt.Sprintf("%s · %s · last %s", session.Session, activity, session.LastActivity.Local().Format(time.RFC3339))
		if session.Attachment != nil && session.Attachment.ClientName != "" {
			line += " · " + session.Attachment.ClientName
		}
		if session.Branch != "" {
			line += " · branch " + session.Branch
		}
		fmt.Fprintln(os.Stdout, line)
		if session.Label != "" {
			fmt.Fprintf(os.Stdout, "  %s\n", session.Label)
		}
		for _, instance := range session.Open {
			fmt.Fprintf(os.Stdout, "  - %s: %s at %s\n", instance.Instance, instance.Procedure, instance.Step)
		}
	}
	if shown == 0 {
		fmt.Fprintln(os.Stdout, "No sessions with open work.")
	}
}
