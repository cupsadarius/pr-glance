package ui

import (
	"strconv"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/cupsadarius/pr-glance/model"
)

// foldKey marks the fold row in Model.Expanded. Review-thread keys are GitHub
// node IDs, which never equal it.
const foldKey = "checks"

// minBarWidth is the narrowest pane that still draws the condition bar.
const minBarWidth = 24

// failing reports the states the pane treats as needing attention, the same
// four the snapshot counts as failed.
func failing(s model.CheckState) bool {
	switch s {
	case model.CheckFailed, model.CheckTimedOut, model.CheckCancelled, model.CheckActionRequired:
		return true
	}
	return false
}

// pendingGlyph is the spinner's current frame.
func (m *Model) pendingGlyph() string { return m.spin.View() }

// ciItems lists failing and running checks; the rest fold into one selectable
// summary row that enter expands in place.
func (m *Model) ciItems(w int) []bodyItem {
	s := m.Snapshot
	var items []bodyItem
	row := func(c model.Check) bodyItem {
		return bodyItem{lines: []string{m.checkLine(c, w)}, url: c.URL}
	}
	var folded []model.Check
	for _, c := range s.Checks {
		if failing(c.State) {
			items = append(items, row(c))
		}
	}
	for _, c := range s.Checks {
		switch {
		case c.State == model.CheckPending:
			items = append(items, row(c))
		case !failing(c.State):
			folded = append(folded, c)
		}
	}
	if len(folded) == 0 {
		return items
	}
	items = append(items, bodyItem{lines: []string{m.foldRow(w)}, fold: true})
	if m.Expanded[foldKey] {
		for _, c := range folded {
			items = append(items, row(c))
		}
	}
	return items
}

// checkLine is a check row whose running glyph is the spinner frame.
func (m *Model) checkLine(c model.Check, w int) string {
	if c.State == model.CheckPending {
		return checkRowGlyph(c, w, m.pendingGlyph())
	}
	return checkRow(c, w)
}

// foldRow summarises the folded checks. A green tick appears only when
// something actually passed.
func (m *Model) foldRow(w int) string {
	c := m.Snapshot.CheckCounts
	arrow := "▸"
	if m.Expanded[foldKey] {
		arrow = "▾"
	}
	spans := []span{{text: arrow + " ", style: pal.bold}}
	sep := false
	add := func(n int, text string, style lipgloss.Style) {
		if n <= 0 {
			return
		}
		if sep {
			spans = append(spans, span{text: " · ", style: pal.faint})
		}
		spans = append(spans, span{text: text, style: style})
		sep = true
	}
	add(c.Passed, "✓ "+strconv.Itoa(c.Passed)+" passed", pal.green)
	add(c.Neutral, strconv.Itoa(c.Neutral)+" neutral", pal.faint)
	add(c.Skipped, strconv.Itoa(c.Skipped)+" skipped", pal.faint)
	add(c.Unknown, strconv.Itoa(c.Unknown)+" unknown", pal.faint)
	return truncateSpans(spans, w)
}

// compact shortens a large diff count: whole thousands from 10,000 and whole
// millions from 1,000,000, always rounded down.
func compact(n int) string {
	switch {
	case n >= 1_000_000:
		return strconv.Itoa(n/1_000_000) + "M"
	case n >= 10_000:
		return strconv.Itoa(n/1_000) + "k"
	default:
		return strconv.Itoa(n)
	}
}
