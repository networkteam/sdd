package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/repos"
	localadapter "github.com/networkteam/sdd/local"
)

func recoverCmd() *cli.Command {
	return &cli.Command{
		Name:  "recover",
		Usage: "Inspect and explicitly recover durable pending writes",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "history", Usage: "Show actionable and closed recovery audit history without taking action"},
			&cli.StringFlag{Name: "session", Usage: "Session containing the pending mutation"},
			&cli.StringFlag{Name: "mutation", Usage: "Pending mutation ID"},
			&cli.StringFlag{Name: "verb", Usage: "Recovery verb: apply, discard, finalize-retry, abandon-unknown, or bind-target"},
			&cli.StringFlag{Name: "branch", Usage: "Concrete branch for bind-target"},
			&cli.StringFlag{Name: "reason", Usage: "Reason recorded in the immutable recovery audit"},
			&cli.BoolFlag{Name: "yes", Usage: "Confirm the explicitly selected recovery verb non-interactively"},
		},
		Action: withWriteGate(func(ctx context.Context, cmd *cli.Command) error {
			application, project, identity, err := buildLocalRecoveryApplication(ctx, cmd)
			if err != nil {
				return err
			}
			list, err := application.ListRecoveries(ctx, identity, project, cmd.Bool("history"))
			if err != nil {
				return err
			}
			if cmd.Bool("history") {
				renderRecoveryItems(list.Items)
				return nil
			}
			if len(list.Items) == 0 {
				fmt.Fprintln(os.Stdout, "No pending writes await recovery.")
				return nil
			}
			item, err := selectRecoveryItem(list.Items, cmd)
			if err != nil {
				return err
			}
			if recoveryNeedsReconciliation(item, cmd.String("verb")) {
				refreshed, err := application.ReconcileMutation(ctx, identity, project, sdd.RecoveryReconcileRequest{
					Session: item.Session, MutationID: item.MutationID,
				})
				if err != nil {
					return err
				}
				item = refreshed.Item
			}
			verb, err := selectRecoveryVerb(item, cmd)
			if err != nil {
				return err
			}
			target := sdd.MutationTarget{}
			if verb == sdd.RecoveryBindTarget {
				branch := strings.TrimSpace(cmd.String("branch"))
				if branch == "" {
					if !isTerminal(os.Stdin) {
						return fmt.Errorf("bind-target requires --branch in non-interactive mode")
					}
					branch, err = readRecoveryLine("Concrete target branch: ")
					if err != nil {
						return err
					}
				}
				target = sdd.MutationTarget{Project: project, Branch: branch}
			}
			reason := strings.TrimSpace(cmd.String("reason"))
			if reason == "" && isTerminal(os.Stdin) {
				reason, err = readRecoveryLine("Audit reason: ")
				if err != nil {
					return err
				}
			}
			if !cmd.Bool("yes") {
				if !isTerminal(os.Stdin) {
					return fmt.Errorf("recovery requires explicit confirmation; pass --yes with --session, --mutation, and --verb")
				}
				confirmed, err := promptConfirmation(fmt.Sprintf("Run %s for %s on %s?", verb, item.MutationID, recoveryTargetLabel(item)))
				if err != nil {
					return err
				}
				if !confirmed {
					return fmt.Errorf("recovery cancelled")
				}
			}
			result, err := application.RecoverMutation(ctx, identity, project, sdd.RecoveryRequest{
				Session: item.Session, MutationID: item.MutationID, Verb: verb, Reason: reason, Target: target,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, "Recovery recorded: %s · %s · %s\n", item.MutationID, verb, recoveryStateLabel(result.Item))
			return nil
		}),
	}
}

func buildLocalRecoveryApplication(ctx context.Context, cmd *cli.Command) (*sdd.Application, sdd.ProjectID, sdd.RequestIdentity, error) {
	graphDir, err := resolveGraphDir(cmd)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	sddDir, err := resolveSDDDir()
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	if cfg == nil || cfg.DefaultBranch == "" {
		return nil, "", sdd.RequestIdentity{}, fmt.Errorf("default_branch is required in .sdd/config.yaml before recovering mutations")
	}
	locations, err := repos.DefaultLocations()
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	storeLocations, err := resolveSessionLocations(sddDir, cfg, locations)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	project := sessionStoreProject(cfg)
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: project, GraphDir: graphDir})
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	sessions, err := localadapter.NewFilesystemSessionStore(storeLocations...)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStore(storeLocations...)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	targets, err := newLocalMutationTargets(project, filepath.Dir(sddDir))
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: project, DisplayName: filepath.Base(filepath.Dir(sddDir))}, DefaultBranch: cfg.DefaultBranch,
		Graph: graph, Targets: targets, Sessions: sessions, StagedBlobs: blobs,
		Recovery: sdd.RecoveryAuthorizerFunc(func(_ context.Context, request sdd.RecoveryAccessRequest) error {
			if request.Actor.Subject != request.OriginalSubject {
				return &sdd.ApplicationError{Code: sdd.ErrorWriteDenied, Message: "cross-principal recovery is not authorized by the local runtime"}
			}
			return nil
		}),
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) {
				return sdd.LLMResult{}, fmt.Errorf("recovery does not execute language models")
			},
		},

		LLMTimeout: time.Minute,
	})
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	access := &localRuntimeAccess{project: project, participant: cfg.Participant, runtime: runtime, dependencies: map[string]*sdd.ProjectRuntime{}}
	application, err := sdd.NewApplication(access)
	if err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	identity := sdd.RequestIdentity{Subject: "local"}
	if _, err := application.Info(ctx, identity, project, sdd.InfoRequest{}); err != nil {
		return nil, "", sdd.RequestIdentity{}, err
	}
	return application, project, identity, nil
}

func renderRecoveryItems(items []sdd.RecoveryItem) {
	if len(items) == 0 {
		fmt.Fprintln(os.Stdout, "No recovery history.")
		return
	}
	for index, item := range items {
		fmt.Fprintf(os.Stdout, "%d. %s · %s · %s · owner %s · session %s\n", index+1, item.MutationID, recoveryStateLabel(item), recoveryTargetLabel(item), item.OriginalSubject, item.Session)
		if item.LastEvidence != "" {
			fmt.Fprintf(os.Stdout, "   evidence: %s\n", item.LastEvidence)
		}
	}
}

// recoveryStateLabel renders delivery state, qualified by the reason where one
// applies, and marks the items a recovery verb actually touched — so a write that
// simply succeeded never reads as recovered.
func recoveryStateLabel(item sdd.RecoveryItem) string {
	label := string(item.State)
	if item.Reason != "" {
		label += " (" + string(item.Reason) + ")"
	}
	if item.Recovered {
		label += " · recovered"
	}
	return label
}

func selectRecoveryItem(items []sdd.RecoveryItem, cmd *cli.Command) (sdd.RecoveryItem, error) {
	session, mutation := sdd.SessionID(strings.TrimSpace(cmd.String("session"))), strings.TrimSpace(cmd.String("mutation"))
	if session != "" || mutation != "" {
		if session == "" || mutation == "" {
			return sdd.RecoveryItem{}, fmt.Errorf("--session and --mutation must be supplied together")
		}
		for _, item := range items {
			if item.Session == session && item.MutationID == mutation {
				return item, nil
			}
		}
		return sdd.RecoveryItem{}, fmt.Errorf("pending mutation %s in session %s was not found", mutation, session)
	}
	if !isTerminal(os.Stdin) {
		return sdd.RecoveryItem{}, fmt.Errorf("multiple pending writes require --session and --mutation in non-interactive mode")
	}
	renderRecoveryItems(items)
	choice, err := readRecoveryChoice("Select pending write: ", len(items))
	if err != nil {
		return sdd.RecoveryItem{}, err
	}
	return items[choice-1], nil
}

func selectRecoveryVerb(item sdd.RecoveryItem, cmd *cli.Command) (sdd.RecoveryVerb, error) {
	if raw := strings.TrimSpace(cmd.String("verb")); raw != "" {
		return parseRecoveryVerb(raw)
	}
	if !isTerminal(os.Stdin) {
		return "", fmt.Errorf("--verb is required in non-interactive mode")
	}
	verbs := recoveryVerbs(item)
	for index, verb := range verbs {
		fmt.Fprintf(os.Stdout, "%d. %s\n", index+1, verb)
	}
	choice, err := readRecoveryChoice("Select recovery action: ", len(verbs))
	if err != nil {
		return "", err
	}
	return verbs[choice-1], nil
}

func recoveryNeedsReconciliation(item sdd.RecoveryItem, explicitVerb string) bool {
	return strings.TrimSpace(explicitVerb) == "" && !item.LegacyUnroutable && item.Reason == sdd.RecoveryReasonOutcomeUnknown
}

// recoveryVerbs offers the actions that answer what a pending item is waiting
// on, so it keys on the reason rather than on delivery state.
func recoveryVerbs(item sdd.RecoveryItem) []sdd.RecoveryVerb {
	if item.LegacyUnroutable {
		return []sdd.RecoveryVerb{sdd.RecoveryBindTarget}
	}
	switch item.Reason {
	case sdd.RecoveryReasonNotApplied:
		return []sdd.RecoveryVerb{sdd.RecoveryApply, sdd.RecoveryDiscard}
	case sdd.RecoveryReasonFinalizationOwed:
		return []sdd.RecoveryVerb{sdd.RecoveryFinalizeRetry}
	default:
		return []sdd.RecoveryVerb{sdd.RecoveryAbandonUnknown}
	}
}

func parseRecoveryVerb(raw string) (sdd.RecoveryVerb, error) {
	verb := sdd.RecoveryVerb(raw)
	switch verb {
	case sdd.RecoveryApply, sdd.RecoveryDiscard, sdd.RecoveryFinalizeRetry, sdd.RecoveryAbandonUnknown, sdd.RecoveryBindTarget:
		return verb, nil
	default:
		return "", fmt.Errorf("invalid recovery verb %q", raw)
	}
}

func recoveryTargetLabel(item sdd.RecoveryItem) string {
	if item.LegacyUnroutable {
		return "target binding required"
	}
	if item.Target.Project == "" && item.Target.Branch == "" {
		return "no recorded target"
	}
	return fmt.Sprintf("%s@%s", item.Target.Project, item.Target.Branch)
}

func readRecoveryChoice(prompt string, count int) (int, error) {
	line, err := readRecoveryLine(prompt)
	if err != nil {
		return 0, err
	}
	choice, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || choice < 1 || choice > count {
		return 0, fmt.Errorf("choice must be between 1 and %d", count)
	}
	return choice, nil
}

func readRecoveryLine(prompt string) (string, error) {
	fmt.Fprint(os.Stdout, prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
