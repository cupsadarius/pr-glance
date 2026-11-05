package ui

import (
	"strings"
	"testing"

	"github.com/cupsadarius/pr-glance/model"
)

func intp(n int) *int { return &n }

// green is a pull request every condition passes.
func green() model.Snapshot {
	return model.Snapshot{
		State: "OPEN", BaseBranch: "main", ReviewDecision: "APPROVED",
		MergeState: "CLEAN", Mergeable: "MERGEABLE", Behind: intp(0),
		Checks:      []model.Check{{Name: "ci", State: model.CheckPassed}},
		CheckCounts: model.CheckCounts{Passed: 1},
		Reviewers:   []model.Reviewer{{Login: "ana", State: "APPROVED"}},
	}
}

func TestReadiness(t *testing.T) {
	type row struct {
		level  level
		detail string
	}
	cases := []struct {
		name    string
		edit    func(*model.Snapshot)
		verdict string
		vlevel  level
		note    string
		rule    bool
		rows    map[string]row // only the rows the case is about
	}{
		{name: "clean", verdict: "Ready to merge", vlevel: levelOK, rows: map[string]row{
			"Checks": {levelOK, "1 passed"}, "Approvals": {levelOK, "1 approval"}, "Conversations": {levelOK, "no threads"},
			"Branch": {levelOK, "up to date with main"}, "Mergeable": {levelOK, "no conflicts"}}},
		{name: "has hooks", edit: func(s *model.Snapshot) { s.MergeState = "HAS_HOOKS" }, verdict: "Ready to merge", vlevel: levelOK},
		{name: "unstable", edit: func(s *model.Snapshot) {
			s.MergeState = "UNSTABLE"
			s.Checks = append(s.Checks, model.Check{Name: "lint", State: model.CheckFailed})
			s.CheckCounts.Failed = 1
		}, verdict: "Ready to merge", vlevel: levelOK, note: "non-required checks fail",
			rows: map[string]row{"Checks": {levelWarn, "1 failing (not required) · 1 passed"}}},
		{name: "required failing", edit: func(s *model.Snapshot) {
			s.MergeState = "BLOCKED"
			s.Checks = append(s.Checks, model.Check{Name: "e2e", State: model.CheckFailed, Required: true}, model.Check{Name: "it", State: model.CheckPending})
			s.CheckCounts.Failed, s.CheckCounts.Pending = 1, 1
		}, verdict: "Blocked", vlevel: levelBad, rows: map[string]row{"Checks": {levelBad, "1 failing · 1 running · 1 passed"}}},
		{name: "running only", edit: func(s *model.Snapshot) {
			s.Checks = []model.Check{{Name: "it", State: model.CheckPending}}
			s.CheckCounts = model.CheckCounts{Pending: 1}
		}, verdict: "Ready to merge", vlevel: levelOK, rows: map[string]row{"Checks": {levelWarn, "1 running"}}},
		{name: "no checks", edit: func(s *model.Snapshot) { s.Checks, s.CheckCounts = nil, model.CheckCounts{} },
			verdict: "Ready to merge", vlevel: levelOK, rows: map[string]row{"Checks": {levelOK, "no checks"}}},
		{name: "changes requested", edit: func(s *model.Snapshot) {
			s.MergeState, s.ReviewDecision = "BLOCKED", "CHANGES_REQUESTED"
		}, verdict: "Blocked", vlevel: levelBad, rows: map[string]row{"Approvals": {levelBad, "changes requested"}}},
		{name: "review required", edit: func(s *model.Snapshot) {
			s.MergeState, s.ReviewDecision = "BLOCKED", "REVIEW_REQUIRED"
		}, verdict: "Blocked", vlevel: levelBad, rows: map[string]row{"Approvals": {levelBad, "review required · 1 approval"}}},
		{name: "no review rule, no approvals", edit: func(s *model.Snapshot) { s.ReviewDecision, s.Reviewers = "", nil },
			verdict: "Ready to merge", vlevel: levelOK, rows: map[string]row{"Approvals": {levelWarn, "no approvals (not required)"}}},
		{name: "open threads", edit: func(s *model.Snapshot) {
			s.ThreadCount, s.OpenThreads = 4, []model.ThreadSummary{{}, {}, {}}
		}, verdict: "Ready to merge", vlevel: levelOK, rows: map[string]row{"Conversations": {levelWarn, "3 unresolved of 4"}}},
		{name: "threads resolved", edit: func(s *model.Snapshot) { s.ThreadCount = 2 },
			verdict: "Ready to merge", vlevel: levelOK, rows: map[string]row{"Conversations": {levelOK, "all 2 resolved"}}},
		{name: "behind and required", edit: func(s *model.Snapshot) { s.MergeState, s.Behind = "BEHIND", intp(3) },
			verdict: "Blocked", vlevel: levelBad, rows: map[string]row{"Branch": {levelBad, "3 behind main"}}},
		{name: "behind, not required", edit: func(s *model.Snapshot) { s.Behind = intp(3) },
			verdict: "Ready to merge", vlevel: levelOK, rows: map[string]row{"Branch": {levelWarn, "3 behind main (not required)"}}},
		{name: "conflicts", edit: func(s *model.Snapshot) { s.MergeState, s.Mergeable = "DIRTY", "CONFLICTING" },
			verdict: "Blocked", vlevel: levelBad, rows: map[string]row{"Mergeable": {levelBad, "conflicts with main"}}},
		{name: "unknown guesses nothing", edit: func(s *model.Snapshot) {
			s.MergeState, s.Mergeable, s.Behind = "UNKNOWN", "UNKNOWN", nil
		}, verdict: "Checking mergeability…", vlevel: levelUnknown, rows: map[string]row{
			"Branch": {levelUnknown, "unknown"}, "Mergeable": {levelUnknown, "checking…"}}},
		{name: "not reported", edit: func(s *model.Snapshot) { s.MergeState, s.Mergeable = "", "" },
			verdict: "Merge state unavailable", vlevel: levelUnknown},
		{name: "blocked by a rule", edit: func(s *model.Snapshot) { s.MergeState = "BLOCKED" },
			verdict: "Blocked", vlevel: levelBad, rule: true},
		{name: "required check still running", edit: func(s *model.Snapshot) {
			s.MergeState = "BLOCKED"
			s.Checks = append(s.Checks, model.Check{Name: "e2e", State: model.CheckPending, Required: true})
			s.CheckCounts.Pending = 1
		}, verdict: "Blocked", vlevel: levelBad, rows: map[string]row{"Checks": {levelWarn, "1 running · 1 passed"}}},
		{name: "behind while blocked", edit: func(s *model.Snapshot) { s.MergeState, s.Behind = "BLOCKED", intp(3) },
			verdict: "Blocked", vlevel: levelBad, rule: true, rows: map[string]row{"Branch": {levelWarn, "3 behind main"}}},
		{name: "dirty, conflicts not yet known", edit: func(s *model.Snapshot) { s.MergeState, s.Mergeable = "DIRTY", "UNKNOWN" },
			verdict: "Blocked", vlevel: levelBad},
		{name: "merged", edit: func(s *model.Snapshot) { s.State, s.MergeState = "MERGED", "UNKNOWN" }, verdict: "Merged", vlevel: levelUnknown},
		{name: "closed", edit: func(s *model.Snapshot) { s.State = "CLOSED" }, verdict: "Closed", vlevel: levelUnknown},
		{name: "draft", edit: func(s *model.Snapshot) { s.Draft, s.MergeState = true, "DRAFT" }, verdict: "Draft", vlevel: levelUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := green()
			if c.edit != nil {
				c.edit(&s)
			}
			v, rows := readiness(s)
			if v.text != c.verdict || v.level != c.vlevel || v.note != c.note || v.rule != c.rule {
				t.Fatalf("verdict = %+v, want %q/%d note %q rule %t", v, c.verdict, c.vlevel, c.note, c.rule)
			}
			var names []string
			for _, r := range rows {
				names = append(names, r.name)
				want, ok := c.rows[r.name]
				if !ok {
					continue
				}
				if r.level != want.level || r.detail != want.detail {
					t.Fatalf("%s = %d %q, want %d %q", r.name, r.level, r.detail, want.level, want.detail)
				}
			}
			if strings.Join(names, ",") != "Checks,Approvals,Conversations,Branch,Mergeable" {
				t.Fatalf("rows = %v", names)
			}
		})
	}
}
