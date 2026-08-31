package serveview_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/serveview"
	"github.com/networkteam/sdd/internal/truncate"
)

func TestEffectivePrefersTheDeclaration(t *testing.T) {
	declared := serveview.Cap{MaxBytes: 100}
	registered := serveview.Cap{MaxBytes: 500, MaxItems: 5}
	if got := serveview.Effective(declared, registered); got != declared {
		t.Fatalf("Effective = %+v, want the declared cap", got)
	}
	if got := serveview.Effective(serveview.Cap{}, registered); got != registered {
		t.Fatalf("Effective = %+v, want the registration default", got)
	}
}

func TestBoundValueCutsStringsAtTheCap(t *testing.T) {
	s := strings.Repeat("line\n", 100)
	v, cut := serveview.BoundValue(s, serveview.Cap{MaxBytes: 42})
	text, ok := v.(string)
	if !ok || len(text) > 42 {
		t.Fatalf("bounded value = %#v", v)
	}
	if cut == nil || cut.Clean() || cut.TotalBytes != len(s) {
		t.Fatalf("cut = %+v", cut)
	}
	if v, cut := serveview.BoundValue("small", serveview.Cap{MaxBytes: 42}); v != "small" || cut != nil {
		t.Fatalf("under-cap string must pass clean, got %#v, %+v", v, cut)
	}
}

func TestBoundValueUnwrapsCarriers(t *testing.T) {
	carrier := truncate.Head([]string{"a", "b", "c"}, 1, "the-pull")
	v, cut := serveview.BoundValue(carrier, serveview.Cap{})
	items, ok := v.([]string)
	if !ok || len(items) != 1 {
		t.Fatalf("payload = %#v, want the kept items unwrapped", v)
	}
	if cut == nil || cut.Dropped != 2 || cut.Pull != "the-pull" {
		t.Fatalf("cut = %+v, want the producer's meta surfaced", cut)
	}
	clean := truncate.Head([]string{"a"}, 5, "")
	if _, cut := serveview.BoundValue(clean, serveview.Cap{}); cut != nil {
		t.Fatalf("a clean carrier surfaces no cut, got %+v", cut)
	}
}

func TestBoundValuePassesOtherValuesUncut(t *testing.T) {
	rows := []map[string]any{{"id": "a"}}
	v, cut := serveview.BoundValue(rows, serveview.Cap{MaxBytes: 1})
	if cut != nil {
		t.Fatalf("non-string non-carrier must pass uncut, got %+v", cut)
	}
	if _, ok := v.([]map[string]any); !ok {
		t.Fatalf("value changed shape: %#v", v)
	}
}

func TestDefaultBudgetCoversEveryPartKind(t *testing.T) {
	b := serveview.Default()
	if b.Total <= 0 {
		t.Fatal("default budget must carry an advisory total")
	}
	for _, kind := range []serveview.PartKind{
		serveview.PartText, serveview.PartEntryList, serveview.PartLineList, serveview.PartItemList,
		serveview.PartStoreValue, serveview.PartDraft, serveview.PartFraming, serveview.PartProduced,
	} {
		if b.Cap(kind).Zero() {
			t.Errorf("default budget has no cap for part kind %q", kind)
		}
	}
}
