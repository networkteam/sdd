package model

import (
	"reflect"
	"testing"
)

func TestCountGraphCommits(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"whitespace only", "\n  \n\n", 0},
		{"single hash", "abc123\n", 1},
		{"multiple hashes", "abc123\ndef456\n789abc\n", 3},
		{"no trailing newline", "abc123", 1},
		{"blank line between", "abc123\n\ndef456\n", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountGraphCommits(tc.in)
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestParseMergeTreeConflicts(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"clean merge — OID only", "abc1234567890abcdef\n", nil},
		{"clean merge — no trailing newline", "abc1234567890abcdef", nil},
		{"empty output", "", nil},
		{
			"single conflict",
			"abc1234567890abcdef\npath/to/file.go\n",
			[]string{"path/to/file.go"},
		},
		{
			"multiple conflicts",
			"treeoid\npath/a.go\npath/b.md\n",
			[]string{"path/a.go", "path/b.md"},
		},
		{
			"blank line terminates path list (trailing messages ignored)",
			"treeoid\npath/a.go\npath/b.md\n\nAuto-merging path/a.go\nCONFLICT (content): Merge conflict in path/a.go\n",
			[]string{"path/a.go", "path/b.md"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseMergeTreeConflicts(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}
