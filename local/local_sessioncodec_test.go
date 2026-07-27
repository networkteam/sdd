package local

import (
	"strings"
	"testing"

	app "github.com/networkteam/sdd/application"
)

// holderEraSessionLine reproduces the shape sdd actually wrote before the
// holder lease was replaced by attachment stamps: envelope version 1, metadata
// codec 1, and the retired Holder/HolderHistory pair alongside fields the model
// still defines. Relocation refused to read this shape, which stalled the whole
// sweep on any store carrying pre-existing sessions.
const holderEraSessionLine = `{"version":1,"metadata":{` +
	`"CodecVersion":1,"ID":"s_20260714-095459-e9ecfc96","Subject":"local",` +
	`"Project":"github.com/networkteam/sdd","Participant":"Christopher","Label":"",` +
	`"Holder":{"Subject":"local","MCPSessionID":"connection-0x1","ClientName":"codex-mcp-client",` +
	`"ClientVersion":"0.144.3","Generation":1,"LastActivity":"2026-07-14T07:54:59.623881Z",` +
	`"ExpiresAt":"2026-07-14T08:24:59.623881Z"},` +
	`"HolderHistory":[{"Subject":"local","Generation":1}],` +
	`"UpdatedAt":"2026-07-14T07:54:59.623881Z"}}`

func TestDecodeSessionLineReadsRetiredHolderFields(t *testing.T) {
	var line sessionLine
	if err := decodeSessionLine([]byte(holderEraSessionLine), &line); err != nil {
		t.Fatalf("decoding a holder-era session line: %v", err)
	}
	if line.Metadata == nil {
		t.Fatal("expected metadata on the decoded line")
	}
	if line.Metadata.ID != "s_20260714-095459-e9ecfc96" {
		t.Errorf("ID = %q, want the persisted session id", line.Metadata.ID)
	}
	if line.Metadata.Participant != "Christopher" {
		t.Errorf("Participant = %q, want it preserved across the retired-field drop", line.Metadata.Participant)
	}
	if line.Metadata.Project != "github.com/networkteam/sdd" {
		t.Errorf("Project = %q, want it preserved across the retired-field drop", line.Metadata.Project)
	}
	if line.Metadata.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero, want the persisted timestamp preserved")
	}
}

// The tolerance is scoped to fields the model actually retired. Anything else
// unknown still fails, so strict decoding keeps reporting real drift instead of
// silently accepting whatever it is handed.
func TestDecodeSessionLineRejectsUnretiredUnknownField(t *testing.T) {
	const line = `{"version":1,"metadata":{"CodecVersion":1,"ID":"s_x","Subject":"local","Speculative":true}}`
	var decoded sessionLine
	err := decodeSessionLine([]byte(line), &decoded)
	if err == nil {
		t.Fatal("expected an unknown field that was never part of the model to fail the decode")
	}
	if !strings.Contains(err.Error(), "Speculative") {
		t.Errorf("error = %v, want it to name the offending field", err)
	}
}

// A retired field is only skipped for the codec version that wrote it, so the
// table cannot quietly widen into blanket tolerance for future versions.
func TestDecodeSessionLineScopesToleranceToItsCodecVersion(t *testing.T) {
	unknownCodec := strings.Replace(holderEraSessionLine, `"CodecVersion":1`, `"CodecVersion":99`, 1)
	var decoded sessionLine
	if err := decodeSessionLine([]byte(unknownCodec), &decoded); err == nil {
		t.Fatal("expected Holder to stay unknown for a codec version that never retired it")
	}
}

func TestDecodeSessionLineReadsCurrentAndEventOnlyLines(t *testing.T) {
	for name, raw := range map[string]string{
		"current metadata": `{"version":1,"metadata":{"CodecVersion":1,"ID":"s_x","Subject":"local","Project":"p","Participant":"c","Label":"","UpdatedAt":"2026-07-26T10:00:00Z"}}`,
		"events only":      `{"version":1,"events":[{"CodecVersion":1,"Code":"mutation_intent","Payload":{}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			var decoded sessionLine
			if err := decodeSessionLine([]byte(raw), &decoded); err != nil {
				t.Fatalf("decoding %s: %v", name, err)
			}
		})
	}
}

func TestSupportedSessionCodecVersion(t *testing.T) {
	// The oldest and current versions coincide today; both bounds are asserted
	// anyway so a future bump keeps the whole supported range covered.
	cases := []struct {
		version uint32
		want    bool
	}{
		{0, false},
		{app.FirstSessionCodecVersion, true},
		{app.SessionCodecVersion, true},
		{app.SessionCodecVersion + 1, false},
	}
	for _, tc := range cases {
		if got := app.SupportedSessionCodecVersion(tc.version); got != tc.want {
			t.Errorf("SupportedSessionCodecVersion(%d) = %v, want %v", tc.version, got, tc.want)
		}
	}
}
