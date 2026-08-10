package mdcompose_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/mdcompose"
)

func TestDemoteToShiftsHierarchyIntoPlace(t *testing.T) {
	fragment := "Opening prose.\n\n## Section\n\ntext\n\n### Sub\n\nmore\n"
	got := mdcompose.DemoteTo(fragment, 4)
	want := "Opening prose.\n\n#### Section\n\ntext\n\n##### Sub\n\nmore\n"
	if got != want {
		t.Errorf("shallowest heading should land at level 4 with relative order kept:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestDemoteToLeavesDeepFragmentAlone(t *testing.T) {
	fragment := "#### Already deep\n\n##### Deeper\n"
	if got := mdcompose.DemoteTo(fragment, 3); got != fragment {
		t.Errorf("a fragment already below the target level must not be promoted:\ngot:\n%s", got)
	}
}

func TestDemoteToClampsAtTheATXFloor(t *testing.T) {
	fragment := "##### Five\n\n###### Six\n"
	got := mdcompose.DemoteTo(fragment, 6)
	want := "###### Five\n\n###### Six\n"
	if got != want {
		t.Errorf("demotion past level 6 must clamp, never emit a seventh hash:\ngot:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "#######") {
		t.Error("clamped output still carries a 7-hash line, which Markdown reads as prose")
	}
}

func TestDemoteToLeavesFencedContentUntouched(t *testing.T) {
	fragment := "## Section\n\n```sh\n# not a heading\n```\n\n~~~\n### also content\n~~~\n"
	got := mdcompose.DemoteTo(fragment, 3)
	want := "### Section\n\n```sh\n# not a heading\n```\n\n~~~\n### also content\n~~~\n"
	if got != want {
		t.Errorf("hashes inside code fences are content, not structure:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestDemoteToIgnoresIndentedCodeAndNonHeadingHashes(t *testing.T) {
	fragment := "## Section\n\n    # indented code, not a heading\n\n#no-space-is-not-a-heading\n"
	got := mdcompose.DemoteTo(fragment, 3)
	want := "### Section\n\n    # indented code, not a heading\n\n#no-space-is-not-a-heading\n"
	if got != want {
		t.Errorf("only ATX headings shift:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestDemoteToNoHeadings(t *testing.T) {
	fragment := "Just prose, no structure at all.\n"
	if got := mdcompose.DemoteTo(fragment, 4); got != fragment {
		t.Errorf("a heading-less fragment passes through unchanged, got:\n%s", got)
	}
}

func TestSplitLeadingHeading(t *testing.T) {
	tests := []struct {
		name      string
		fragment  string
		wantTitle string
		wantRest  string
	}{
		{
			name:      "leading heading becomes the title",
			fragment:  "# The posture\n\nGoal first.\n",
			wantTitle: "The posture",
			wantRest:  "Goal first.\n",
		},
		{
			name:      "closing sequence is dropped",
			fragment:  "## Title ##\n\nbody\n",
			wantTitle: "Title",
			wantRest:  "body\n",
		},
		{
			name:      "body opening with prose keeps everything",
			fragment:  "Opening prose.\n\n## Section\n",
			wantTitle: "",
			wantRest:  "Opening prose.\n\n## Section\n",
		},
		{
			name:      "leading fence is content, not a title",
			fragment:  "```\n# sample\n```\n",
			wantTitle: "",
			wantRest:  "```\n# sample\n```\n",
		},
		{
			name:      "blank lines before the heading are skipped",
			fragment:  "\n\n### Deep title\n\nbody\n",
			wantTitle: "Deep title",
			wantRest:  "body\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			title, rest := mdcompose.SplitLeadingHeading(tc.fragment)
			if title != tc.wantTitle {
				t.Errorf("title = %q, want %q", title, tc.wantTitle)
			}
			if rest != tc.wantRest {
				t.Errorf("rest = %q, want %q", rest, tc.wantRest)
			}
		})
	}
}

func TestTopHeadingLevel(t *testing.T) {
	tests := []struct {
		fragment string
		want     int
	}{
		{"prose only\n", 0},
		{"### Deep\n\n## Shallower\n", 2},
		{"```\n# fenced\n```\n", 0},
		{"# One\n", 1},
	}
	for _, tc := range tests {
		if got := mdcompose.TopHeadingLevel(tc.fragment); got != tc.want {
			t.Errorf("TopHeadingLevel(%q) = %d, want %d", tc.fragment, got, tc.want)
		}
	}
}
