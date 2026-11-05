package ui

import (
	"fmt"
	"strings"

	"github.com/cupsadarius/pr-glance/model"
)

// level is how a merge condition stands. Bad is used only on a GitHub signal
// that the condition blocks the merge; warn marks what is worth a look.
type level int

const (
	levelUnknown level = iota
	levelOK
	levelWarn
	levelBad
)

type condition struct {
	name, detail string
	level        level
}

// verdict is the Overview's first line. rule asks for the "blocked by a
// repository rule" line: GitHub blocks the merge and no condition says why.
type verdict struct {
	text, note string
	level      level
	rule       bool
}

const notRequired = " (not required)"

// readiness decides the merge verdict and its five conditions from GitHub's
// own signals. The verdict follows mergeStateStatus, so it never disagrees
// with the merge button.
func readiness(s model.Snapshot) (verdict, []condition) {
	base := clean(s.BaseBranch)
	rows := []condition{checksCondition(s), approvalsCondition(s), conversationsCondition(s),
		branchCondition(s, base), mergeableCondition(s, base)}
	var v verdict
	switch {
	case strings.EqualFold(s.State, "MERGED"):
		v = verdict{text: "Merged"}
	case strings.EqualFold(s.State, "CLOSED"):
		v = verdict{text: "Closed"}
	case s.Draft || s.MergeState == "DRAFT":
		v = verdict{text: "Draft"}
	case s.MergeState == "CLEAN" || s.MergeState == "HAS_HOOKS":
		v = verdict{text: "Ready to merge", level: levelOK}
	case s.MergeState == "UNSTABLE":
		v = verdict{text: "Ready to merge", level: levelOK, note: "non-required checks fail"}
	case s.MergeState == "BLOCKED":
		v = verdict{text: "Blocked", level: levelBad, rule: !explained(s, rows)}
	case s.MergeState == "BEHIND" || s.MergeState == "DIRTY":
		v = verdict{text: "Blocked", level: levelBad}
	case s.MergeState == "":
		v = verdict{text: "Merge state unavailable"}
	default:
		v = verdict{text: "Checking mergeability…"}
	}
	return v, rows
}

// explained reports whether a red row or a running required check accounts
// for a block.
func explained(s model.Snapshot, rows []condition) bool {
	for _, r := range rows {
		if r.level == levelBad {
			return true
		}
	}
	for _, c := range s.Checks {
		if c.Required && c.State == model.CheckPending {
			return true
		}
	}
	return false
}

func checksCondition(s model.Snapshot) condition {
	c := s.CheckCounts
	if len(s.Checks) == 0 {
		return condition{name: "Checks", detail: "no checks", level: levelOK}
	}
	lvl := levelOK
	var parts []string
	if c.Failed > 0 {
		required := false
		for _, ch := range s.Checks {
			if failing(ch.State) && ch.Required {
				required = true
			}
		}
		part := fmt.Sprintf("%d failing", c.Failed)
		if required {
			lvl = levelBad
		} else {
			lvl, part = levelWarn, part+notRequired
		}
		parts = append(parts, part)
	}
	if c.Pending > 0 {
		parts = append(parts, fmt.Sprintf("%d running", c.Pending))
		lvl = max(lvl, levelWarn)
	}
	if c.Passed > 0 {
		parts = append(parts, fmt.Sprintf("%d passed", c.Passed))
	}
	if len(parts) == 0 {
		parts = append(parts, plural(len(s.Checks), "check"))
	}
	return condition{name: "Checks", detail: strings.Join(parts, " · "), level: lvl}
}

func approvalsCondition(s model.Snapshot) condition {
	n := 0
	for _, r := range s.Reviewers {
		if r.State == "APPROVED" {
			n++
		}
	}
	count := plural(n, "approval")
	switch strings.ToUpper(clean(s.ReviewDecision)) {
	case "APPROVED":
		return condition{name: "Approvals", detail: count, level: levelOK}
	case "CHANGES_REQUESTED":
		return condition{name: "Approvals", detail: "changes requested", level: levelBad}
	case "REVIEW_REQUIRED":
		detail := "review required"
		if n > 0 {
			detail += " · " + count
		}
		return condition{name: "Approvals", detail: detail, level: levelBad}
	}
	if n > 0 {
		return condition{name: "Approvals", detail: count, level: levelOK}
	}
	return condition{name: "Approvals", detail: "no approvals" + notRequired, level: levelWarn}
}

// conversationsCondition is never bad: whether a repository requires resolved
// conversations is not readable without admin rights.
func conversationsCondition(s model.Snapshot) condition {
	switch open := len(s.OpenThreads); {
	case s.ThreadCount == 0:
		return condition{name: "Conversations", detail: "no threads", level: levelOK}
	case open > 0:
		return condition{name: "Conversations", detail: fmt.Sprintf("%d unresolved of %d", open, s.ThreadCount), level: levelWarn}
	default:
		return condition{name: "Conversations", detail: fmt.Sprintf("all %d resolved", s.ThreadCount), level: levelOK}
	}
}

func branchCondition(s model.Snapshot, base string) condition {
	switch {
	case s.Behind == nil:
		return condition{name: "Branch", detail: "unknown"}
	case *s.Behind == 0:
		return condition{name: "Branch", detail: "up to date with " + base, level: levelOK}
	case s.MergeState == "BEHIND":
		return condition{name: "Branch", detail: fmt.Sprintf("%d behind %s", *s.Behind, base), level: levelBad}
	case s.MergeState == "CLEAN" || s.MergeState == "UNSTABLE" || s.MergeState == "HAS_HOOKS":
		return condition{name: "Branch", detail: fmt.Sprintf("%d behind %s", *s.Behind, base) + notRequired, level: levelWarn}
	default: // GitHub reports BLOCKED before BEHIND, so this one may still be required
		return condition{name: "Branch", detail: fmt.Sprintf("%d behind %s", *s.Behind, base), level: levelWarn}
	}
}

func mergeableCondition(s model.Snapshot, base string) condition {
	switch s.Mergeable {
	case "CONFLICTING":
		return condition{name: "Mergeable", detail: "conflicts with " + base, level: levelBad}
	case "MERGEABLE":
		return condition{name: "Mergeable", detail: "no conflicts", level: levelOK}
	default:
		return condition{name: "Mergeable", detail: "checking…"}
	}
}
