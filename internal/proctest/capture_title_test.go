package proctest_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/proctest"
	sdd "github.com/networkteam/sdd/pkg/application"
)

// A capture writes the title from the summary call, the summary check shows
// it, and a correction may replace the title alone.
func TestCapture_TitleIsWrittenShownAndCorrectable(t *testing.T) {
	cases := []struct {
		name        string
		corrections map[string]any
		wantTitle   string
		wantSummary string
	}{
		{name: "title alone", corrections: map[string]any{"correctedTitle": "corrected title."}, wantTitle: "Corrected title", wantSummary: "A generated summary."},
		{name: "summary alone", corrections: map[string]any{"correctedSummary": "The corrected summary."}, wantTitle: "Generated title", wantSummary: "The corrected summary."},
		{name: "both", corrections: map[string]any{"correctedTitle": "corrected title", "correctedSummary": "The corrected summary."}, wantTitle: "Corrected title", wantSummary: "The corrected summary."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			world, session := newCaptureWorld(t, "title-"+strings.ReplaceAll(tc.name, " ", "-"))
			before := entryIDsOnDisk(t, world.GraphDir)
			serve := session.Start(t, "capture", nil)
			instance := serve.Instance
			session.Report(t, instance, captureDraft())
			serve = session.Answer(t, instance, "playback", "confirm", nil, "publish the observation")
			proctest.RequireStep(t, serve, "verifySummary")
			if !strings.Contains(serve.Instructions, "Generated title: Generated title") {
				t.Errorf("the summary check should show the generated title:\n%s", serve.Instructions)
			}
			if !strings.Contains(serve.Instructions, model.TitleRule) {
				t.Errorf("the summary check should state the title rule a corrected title follows:\n%s", serve.Instructions)
			}
			entryID := writtenEntryID(t, world.GraphDir, before)
			if got := proctest.LoadEntry(t, world.GraphDir, entryID).Title; got != "Generated title" {
				t.Fatalf("the capture should write the title from the summary call, got %q", got)
			}

			serve = session.Answer(t, instance, "verifySummary", "drifted", tc.corrections, "")
			proctest.RequireStatus(t, serve, "completed")
			entry := proctest.LoadEntry(t, world.GraphDir, entryID)
			if entry.Title != tc.wantTitle || entry.Summary != tc.wantSummary {
				t.Errorf("each correction should replace only its own field, got title %q summary %q", entry.Title, entry.Summary)
			}
		})
	}
}

func TestCapture_DriftedWithoutACorrectionIsRefused(t *testing.T) {
	world, session := newCaptureWorld(t, "title-no-correction")
	before := entryIDsOnDisk(t, world.GraphDir)
	serve := session.Start(t, "capture", nil)
	instance := serve.Instance
	session.Report(t, instance, captureDraft())
	session.Answer(t, instance, "playback", "confirm", nil, "publish the observation")
	entryID := writtenEntryID(t, world.GraphDir, before)

	if _, err := session.AnswerErr(t, instance, "verifySummary", "drifted", nil, ""); err == nil || !strings.Contains(err.Error(), "correctedTitle") {
		t.Fatalf("a drifted answer carrying no correction should be refused naming the fields, got %v", err)
	}
	entry := proctest.LoadEntry(t, world.GraphDir, entryID)
	if entry.Title != "Generated title" || entry.Summary != "A generated summary." {
		t.Errorf("a refused correction should leave the entry as written, got title %q summary %q", entry.Title, entry.Summary)
	}
}

func TestCapture_MultiLineCorrectedTitleIsRefused(t *testing.T) {
	world, session := newCaptureWorld(t, "title-multi-line")
	before := entryIDsOnDisk(t, world.GraphDir)
	serve := session.Start(t, "capture", nil)
	instance := serve.Instance
	session.Report(t, instance, captureDraft())
	session.Answer(t, instance, "playback", "confirm", nil, "publish the observation")
	entryID := writtenEntryID(t, world.GraphDir, before)

	corrections := map[string]any{"correctedTitle": "Corrected\ntitle"}
	if _, err := session.AnswerErr(t, instance, "verifySummary", "drifted", corrections, ""); err == nil || !strings.Contains(err.Error(), "more than one line") {
		t.Fatalf("a title spanning two lines should be refused, got %v", err)
	}
	if got := proctest.LoadEntry(t, world.GraphDir, entryID).Title; got != "Generated title" {
		t.Errorf("a refused title should leave the stored one, got %q", got)
	}
}

// capturedEntry runs a capture to its summary check and returns the written
// entry's ID.
func capturedEntry(t *testing.T, connID string) (*proctest.World, *proctest.Session, string) {
	t.Helper()
	world, session := newCaptureWorld(t, connID)
	before := entryIDsOnDisk(t, world.GraphDir)
	serve := session.Start(t, "capture", nil)
	session.Report(t, serve.Instance, captureDraft())
	serve = session.Answer(t, serve.Instance, "playback", "confirm", nil, "publish the observation")
	proctest.RequireStep(t, serve, "verifySummary")
	return world, session, writtenEntryID(t, world.GraphDir, before)
}

// A title correction read from a document that moved meanwhile is a conflict
// only when the stored title differs; a replayed correction is recognized.
func TestApplication_TitleCorrectionOfAMovedDocument(t *testing.T) {
	cases := []struct {
		name         string
		title        string
		wantConflict bool
	}{
		{name: "differing title", title: "A late title", wantConflict: true},
		{name: "replayed title", title: "generated title."},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			world, session, entryID := capturedEntry(t, "title-conflict-"+strings.ReplaceAll(tc.name, " ", "-"))
			_, err := world.App.ReplaceSummary(t.Context(), world.Identity, "proctest", session.WF.Binding(), sdd.SummaryReplacement{
				Publication:  sdd.PublicationKey{Session: session.ID, Sequence: uint64(90 + i), Discriminator: "replaceSummary:" + entryID},
				EntryID:      entryID,
				ExpectedBlob: sdd.GitBlobID([]byte("the document as some earlier read saw it")),
				Title:        tc.title,
			})
			var appErr *sdd.ApplicationError
			if tc.wantConflict != (errors.As(err, &appErr) && appErr.Code == sdd.ErrorGraphConflict) {
				t.Fatalf("a moved document should conflict only on a differing title, got %v", err)
			}
			if !tc.wantConflict && err != nil {
				t.Fatalf("a replayed correction should be recognized, got %v", err)
			}
			if tc.wantConflict && !strings.Contains(appErr.Message, `"Generated title"`) {
				t.Errorf("the conflict should name the current title, got %q", appErr.Message)
			}
			if got := proctest.LoadEntry(t, world.GraphDir, entryID).Title; got != "Generated title" {
				t.Errorf("the stored title should stay, got %q", got)
			}
		})
	}
}

func TestApplication_TitleCorrectionIsNormalized(t *testing.T) {
	world, session, entryID := capturedEntry(t, "title-normalized")
	replace := func(sequence uint64, title string) error {
		_, err := world.App.ReplaceSummary(t.Context(), world.Identity, "proctest", session.WF.Binding(), sdd.SummaryReplacement{
			Publication: sdd.PublicationKey{Session: session.ID, Sequence: sequence, Discriminator: "replaceSummary:" + entryID},
			EntryID:     entryID,
			Title:       title,
		})
		return err
	}
	if err := replace(90, `  “corrected” "daemon" title!  `); err != nil {
		t.Fatalf("ReplaceSummary: %v", err)
	}
	entry := proctest.LoadEntry(t, world.GraphDir, entryID)
	if entry.Title != "Corrected daemon title" || entry.Summary != "A generated summary." {
		t.Errorf("the corrected title should be stored normalized beside the stored summary, got title %q summary %q", entry.Title, entry.Summary)
	}
	err := replace(91, ` „…“ `)
	var appErr *sdd.ApplicationError
	if !errors.As(err, &appErr) || appErr.Code != sdd.ErrorInvalidArgument {
		t.Fatalf("a title of nothing but quotes and punctuation should be refused, got %v", err)
	}
	err = replace(92, "Corrected\ndaemon title")
	if !errors.As(err, &appErr) || appErr.Code != sdd.ErrorInvalidArgument || !strings.Contains(appErr.Message, "more than one line") {
		t.Fatalf("a title spanning two lines should be refused, got %v", err)
	}
	if got := proctest.LoadEntry(t, world.GraphDir, entryID).Title; got != "Corrected daemon title" {
		t.Errorf("a refused title should leave the stored one, got %q", got)
	}
}
