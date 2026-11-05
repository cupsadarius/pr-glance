package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cupsadarius/pr-glance/model"
)

// conditionWidth is the column the condition details start at.
const conditionWidth = 15

func levelStyle(l level) lipgloss.Style {
	switch l {
	case levelOK:
		return pal.green
	case levelWarn:
		return pal.yellow
	case levelBad:
		return pal.red
	default:
		return pal.faint
	}
}

func levelGlyph(l level) string {
	switch l {
	case levelOK:
		return "✓"
	case levelWarn:
		return "●"
	case levelBad:
		return "✗"
	default:
		return "○"
	}
}

// overviewBody lays out the merge verdict and its conditions, then reviewers,
// open threads, the stack and recent activity. Lines that introduce a group
// ride on the next item as its head, so the cursor and the row map skip them;
// whatever is left over trails the last item. Empty sections are left out.
func (m *Model) overviewBody(w int) ([]string, []bodyItem, []string) {
	paneW, _ := m.size()
	var items []bodyItem
	var pending []string
	add := func(it bodyItem) {
		it.head = append(pending, it.head...)
		pending = nil
		items = append(items, it)
	}
	v, conds := readiness(m.Snapshot)
	pending = append(pending, m.verdictLines(v, w)...)
	if paneW >= minBarWidth {
		pending = append(pending, conditionBar(conds, w))
	}
	pending = append(pending, "")
	for _, c := range conds {
		pending = append(pending, conditionLine(c, w))
		if c.name == "Checks" {
			for _, it := range m.ciItems(w - 2) {
				for i, l := range it.lines {
					it.lines[i] = "  " + l
				}
				add(it)
			}
		}
	}
	if len(m.Snapshot.Reviewers) > 0 {
		pending = append(pending, "", sectionRule("Reviewers", w))
		for _, r := range m.Snapshot.Reviewers {
			add(bodyItem{lines: []string{m.reviewerRow(r, w)}, url: r.URL})
		}
	}
	if open := m.Snapshot.OpenThreads; len(open) > 0 {
		pending = append(pending, "", sectionRule("Threads  "+strconv.Itoa(len(open))+" open", w))
		for i, t := range open {
			if i == 0 || open[i-1].Path != t.Path {
				pending = append(pending, styled(pal.cyan, truncatePath(clean(t.Path), w)))
			}
			last := i == len(open)-1 || open[i+1].Path != t.Path
			add(bodyItem{lines: []string{threadSummaryRow(t, last, w)}, url: t.URL})
		}
	}
	if s := m.Snapshot.Stack; s != nil {
		pending = append(pending, "", m.stackHeading(w))
		for i := len(s.Entries) - 1; i >= 0; i-- {
			e := s.Entries[i]
			pin := e.PR
			// The cursor gutter already points at the row it selects, so the
			// entry marker stands down there.
			add(bodyItem{lines: []string{m.stackRow(e, w, m.Cursor == len(items))}, url: e.PR.URL, pin: &pin})
		}
		if hidden := s.Size - len(s.Entries); hidden > 0 {
			pending = append(pending, styled(pal.faint, fmt.Sprintf("+%d more", hidden)))
		}
	}
	if len(m.Snapshot.Activity) > 0 {
		pending = append(pending, "", sectionRule("Activity", w))
		for _, e := range m.Snapshot.Activity {
			add(bodyItem{lines: []string{m.eventRow(e, w)}, url: e.URL})
		}
	}
	return nil, items, pending
}

// verdictLines is the verdict, its note, and the repository-rule line when
// GitHub blocks the merge for a reason no condition shows.
func (m *Model) verdictLines(v verdict, w int) []string {
	style := levelStyle(v.level)
	if v.text == "Merged" || v.text == "Closed" || v.text == "Draft" {
		style = prStateStyle(prState(m.Snapshot))
	}
	line := []span{{text: v.text, style: style.Bold(true)}}
	if v.note != "" {
		line = append(line, span{text: " · " + v.note, style: pal.yellow})
	}
	out := []string{truncateSpans(line, w)}
	if v.rule {
		out = append(out, truncateSpans([]span{{text: "blocked by a repository rule", style: pal.red}}, w))
	}
	return out
}

// conditionBar draws one segment per condition in the condition's colour.
func conditionBar(conds []condition, w int) string {
	n := len(conds)
	seg := (w - (n - 1)) / n
	if seg < 1 {
		return ""
	}
	extra := (w - (n - 1)) % n
	parts := make([]string, n)
	for i, c := range conds {
		run := seg
		if i < extra {
			run++ // the first segments share the remainder, so the bar fills w
		}
		parts[i] = styled(levelStyle(c.level), strings.Repeat("━", run))
	}
	return strings.Join(parts, " ")
}

func conditionLine(c condition, w int) string {
	style := levelStyle(c.level)
	name := c.name + strings.Repeat(" ", max(1, conditionWidth-len(c.name)))
	return truncateSpans([]span{{text: levelGlyph(c.level) + " ", style: style},
		{text: name, style: pal.bold}, {text: c.detail, style: style}}, w)
}

// sectionRule is a bold section name followed by a faint rule to the edge.
func sectionRule(title string, w int) string {
	shown := ansi.Truncate(title, w, "…")
	rest := w - ansi.StringWidth(shown) - 1
	if rest < 1 {
		return styled(pal.bold, shown)
	}
	return styled(pal.bold, shown) + " " + styled(pal.faint, strings.Repeat("─", rest))
}

var reviewWords = map[string]string{
	"APPROVED": "approved", "CHANGES_REQUESTED": "changes requested", "COMMENTED": "commented",
	"DISMISSED": "dismissed", "REQUESTED": "requested",
}

var reviewLevels = map[string]level{"APPROVED": levelOK, "CHANGES_REQUESTED": levelBad, "REQUESTED": levelWarn}

func (m *Model) reviewerRow(r model.Reviewer, w int) string {
	word, ok := reviewWords[r.State]
	if !ok {
		word = strings.ToLower(clean(r.State))
	}
	l := reviewLevels[r.State]
	name := "@" + clean(r.Login)
	if r.Team {
		name += " (team)"
	}
	row := []span{{text: name, style: pal.cyan}, {text: "  ", style: pal.none},
		{text: levelGlyph(l) + " " + word, style: levelStyle(l)}}
	if !r.At.IsZero() {
		row = append(row, span{text: "  " + ago(m.now().Sub(r.At)), style: pal.faint})
	}
	return truncateSpans(row, w)
}

// threadSummaryRow is one open thread under its file: the line, the author
// and the first line of the first comment.
func threadSummaryRow(t model.ThreadSummary, last bool, w int) string {
	branch := "├─"
	if last {
		branch = "╰─"
	}
	where := "file"
	if t.Line != nil {
		where = "L" + strconv.Itoa(*t.Line)
	}
	body := ""
	for l := range strings.Lines(clean(t.Body)) {
		if body = strings.TrimSpace(l); body != "" {
			break
		}
	}
	row := []span{{text: branch + " ", style: pal.faint}, {text: fmt.Sprintf("%-5s ", where), style: pal.faint}}
	if t.Author != "" {
		row = append(row, span{text: "@" + clean(t.Author) + " ", style: pal.cyan})
	}
	row = append(row, span{text: body, style: pal.none})
	return truncateSpans(row, w)
}

var eventGlyphs = map[model.EventKind]span{
	model.EventPush:        {text: "↑", style: pal.magenta},
	model.EventApproved:    {text: "✓", style: pal.green},
	model.EventChanges:     {text: "✗", style: pal.red},
	model.EventCheckFailed: {text: "✗", style: pal.red},
	model.EventCommented:   {text: "●", style: pal.yellow},
	model.EventRequested:   {text: "●", style: pal.yellow},
	model.EventMerged:      {text: "✓", style: pal.magenta},
}

func (m *Model) eventRow(e model.Event, w int) string {
	glyph, ok := eventGlyphs[e.Kind]
	if !ok {
		glyph = span{text: "○", style: pal.faint}
	}
	row := []span{glyph, {text: fmt.Sprintf(" %-3s ", ago(m.now().Sub(e.At))), style: pal.faint}}
	if e.Actor != "" {
		row = append(row, span{text: "@" + clean(e.Actor) + " ", style: pal.cyan})
	}
	row = append(row, span{text: clean(e.Text), style: pal.none})
	return truncateSpans(row, w)
}
