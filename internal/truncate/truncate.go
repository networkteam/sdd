// Package truncate carries the shared cut mechanics for bounded serving
// (d-tac-qwc): pure functions that cut strings and item slices against byte
// or count caps, and the Cut accounting record every bound firing produces.
// Data stays separated from meta — a carrier pairs the payload with its Cut,
// so consumers render the payload while the cut travels out of band and each
// surface phrases its notice in its own register. This package carries no
// notice wording.
package truncate

import "unicode/utf8"

// Cut is the accounting record of one bound firing: what remains, what was
// dropped, and how to pull the remainder. The zero value means clean.
type Cut struct {
	// Part names what was cut — a lane, an inject id, a store field. Left to
	// the seam that knows the name; producers leave it empty.
	Part string
	// Dropped and Total count whole items on item cuts; zero on byte cuts.
	Dropped, Total int
	// KeptBytes and TotalBytes account the byte weight after and before.
	KeptBytes, TotalBytes int
	// Pull is a ready-to-run expression for the remainder; empty when none
	// applies.
	Pull string
}

// Clean reports whether the bound dropped nothing.
func (c Cut) Clean() bool { return c.Dropped == 0 && c.KeptBytes == c.TotalBytes }

// Carrier pairs a payload with its cut meta. A seam that receives one hands
// the payload on unchanged and routes the meta out of band.
type Carrier interface {
	Payload() any
	CutMeta() Cut
}

// List carries items and their cut meta; templates and projections consume
// Items, never the meta.
type List[T any] struct {
	Items []T
	Cut   Cut
}

func (l List[T]) Payload() any { return l.Items }
func (l List[T]) CutMeta() Cut { return l.Cut }

// Text carries rendered text and its cut meta.
type Text struct {
	Text string
	Cut  Cut
}

func (t Text) Payload() any { return t.Text }
func (t Text) CutMeta() Cut { return t.Cut }

// Items keeps whole items, in order, while their rendered sizes fit
// maxBytes — the byte-precise cut for producer-rendered chunks. size
// measures one item as it will be rendered. maxBytes <= 0 means unbounded.
func Items[T any](items []T, size func(T) int, maxBytes int, pull string) List[T] {
	total := 0
	for _, item := range items {
		total += size(item)
	}
	if maxBytes <= 0 || total <= maxBytes {
		return List[T]{Items: items, Cut: Cut{Total: len(items), KeptBytes: total, TotalBytes: total}}
	}
	kept, keptBytes := 0, 0
	for _, item := range items {
		n := size(item)
		if keptBytes+n > maxBytes {
			break
		}
		kept++
		keptBytes += n
	}
	return List[T]{Items: items[:kept], Cut: Cut{
		Dropped: len(items) - kept, Total: len(items),
		KeptBytes: keptBytes, TotalBytes: total, Pull: pull,
	}}
}

// Head keeps the first maxItems items — the count cut for typed data whose
// rendered size is unknowable before a template renders it; byte safety
// comes from measured allowances and the serve-size regression harness.
// maxItems <= 0 means unbounded.
func Head[T any](items []T, maxItems int, pull string) List[T] {
	if maxItems <= 0 || len(items) <= maxItems {
		return List[T]{Items: items, Cut: Cut{Total: len(items)}}
	}
	return List[T]{Items: items[:maxItems], Cut: Cut{
		Dropped: len(items) - maxItems, Total: len(items), Pull: pull,
	}}
}

// Bytes cuts rendered text at a line boundary under maxBytes, backing up to
// a rune start when the first line alone exceeds the cap, so a multi-byte
// rune is never split. maxBytes <= 0 means unbounded.
func Bytes(s string, maxBytes int, pull string) Text {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return Text{Text: s, Cut: Cut{KeptBytes: len(s), TotalBytes: len(s)}}
	}
	cut := s[:maxBytes]
	if i := lastNewline(cut); i > 0 {
		cut = cut[:i]
	} else {
		for len(cut) > 0 && !utf8.RuneStart(cut[len(cut)-1]) {
			cut = cut[:len(cut)-1]
		}
		// Backing up to a rune start can still leave the lead byte of a rune
		// whose continuation bytes were cut off.
		if r, size := utf8.DecodeLastRuneInString(cut); r == utf8.RuneError && size <= 1 && len(cut) > 0 {
			cut = cut[:len(cut)-1]
		}
	}
	return Text{Text: cut, Cut: Cut{KeptBytes: len(cut), TotalBytes: len(s), Pull: pull}}
}

func lastNewline(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' {
			return i
		}
	}
	return -1
}
