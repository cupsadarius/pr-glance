package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/cupsadarius/pr-glance/model"
)

// readyFixture is a pull request with nothing to show beyond the five rows.
func readyFixture(m *Model, now time.Time) {
	m.Source = model.Source{CWD: "/repo", Root: "/repo", Branch: "feature/retry", Visible: true}
	m.Snapshot = model.Snapshot{
		PR:    &model.PR{Host: "github.com", Repository: "acme/service", Number: 3630, URL: "https://github.com/acme/service/pull/3630"},
		Title: "Retry", Author: "author", State: "OPEN", BaseBranch: "main", HeadBranch: "feature/retry",
		MergeState: "CLEAN", Mergeable: "MERGEABLE", Behind: intp(0), Commits: 1, ChangedFiles: 1,
		FetchedAt: now.Add(-12 * time.Second),
	}
}

// busyReadyFixture is a ready pull request with every optional section shown.
func busyReadyFixture(m *Model, now time.Time) {
	readyFixture(m, now)
	s := &m.Snapshot
	s.ReviewDecision = "APPROVED"
	s.Checks = []model.Check{{Name: "ci", State: model.CheckPassed}, {Name: "lint", State: model.CheckPassed}}
	s.CheckCounts = model.CheckCounts{Passed: 2}
	s.Reviewers = []model.Reviewer{{Login: "ana", State: "APPROVED", At: now.Add(-2 * time.Hour), URL: "https://r/1"},
		{Login: "platform-reviewers-with-a-long-name", Team: true, State: "REQUESTED"}}
	s.ThreadCount = 3
	s.OpenThreads = []model.ThreadSummary{
		{Path: "internal/store/query/builder.go", Line: intp(88), Author: "radu", Body: "N+1 here, batch the lookups before the loop", URL: "https://t/1"},
		{Path: "internal/store/query/builder.go", Line: intp(141), Author: "radu", Body: "index hint?", URL: "https://t/2"},
		{Path: "db/migrate.sql", Author: "ana", Body: "down migration", URL: "https://t/3"},
	}
	s.Activity = []model.Event{
		{At: now.Add(-6 * time.Minute), Kind: model.EventPush, Actor: "author", Text: "pushed 3 commits", URL: "https://c/1"},
		{At: now.Add(-2 * time.Hour), Kind: model.EventApproved, Actor: "ana", Text: "approved", URL: "https://r/1"},
		{At: now.Add(-50 * time.Hour), Kind: model.EventReady, Actor: "author", Text: "marked ready for review"},
	}
}

func TestOverviewAllEmptySections(t *testing.T) {
	m, now := viewHarness()
	readyFixture(m, *now)
	got := plainView(m)
	for _, want := range []string{"Ready to merge", "Checks", "Approvals", "Conversations", "Branch", "Mergeable", "feature/retry → main"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	for _, absent := range []string{"Reviewers", "Threads", "Stack", "Activity"} {
		if strings.Contains(got, absent) {
			t.Fatalf("empty section %q shown in:\n%s", absent, got)
		}
	}
}

func TestVerdictRuleLine(t *testing.T) {
	m, now := viewHarness()
	readyFixture(m, *now)
	m.Snapshot.MergeState = "BLOCKED"
	got := plainView(m)
	if !strings.Contains(got, "Blocked") || !strings.Contains(got, "blocked by a repository rule") {
		t.Fatalf("no rule line in:\n%s", got)
	}
}

func itemWithURL(t *testing.T, m *Model, url string) bodyItem {
	t.Helper()
	_, items, _ := m.overviewBody(m.contentWidth())
	for _, it := range items {
		if it.url == url {
			return it
		}
	}
	t.Fatalf("no body item opens %s", url)
	return bodyItem{}
}

func TestThreadRowsOpenThreadURL(t *testing.T) {
	m, now := viewHarness()
	readyFixture(m, *now)
	m.Snapshot.ThreadCount = 3
	m.Snapshot.OpenThreads = []model.ThreadSummary{
		{Path: "a/query.go", Line: intp(88), Author: "radu", Body: "N+1 here\nmore", URL: "https://t/1"},
		{Path: "a/query.go", Line: intp(141), Author: "radu", Body: "index hint?", URL: "https://t/2"},
		{Path: "db/m.sql", Author: "ana", Body: "down migration", URL: "https://t/3"},
	}
	it := itemWithURL(t, m, "https://t/1")
	if line := ansi.Strip(strings.Join(it.lines, "")); !strings.Contains(line, "L88") || !strings.Contains(line, "@radu") || strings.Contains(line, "more") {
		t.Fatalf("thread row = %q", line)
	}
	got := plainView(m)
	if !strings.Contains(got, "Threads  2 open") && !strings.Contains(got, "Threads  3 open") {
		t.Fatalf("no threads heading in:\n%s", got)
	}
	if strings.Count(got, "a/query.go") != 1 {
		t.Fatalf("a file heads its threads once:\n%s", got)
	}
}

func TestActivityRowsOpenURL(t *testing.T) {
	m, now := viewHarness()
	readyFixture(m, *now)
	m.Snapshot.Activity = []model.Event{
		{At: now.Add(-6 * time.Minute), Kind: model.EventCheckFailed, Text: "e2e failed", URL: "https://ci/e2e"},
		{At: now.Add(-2 * time.Hour), Kind: model.EventApproved, Actor: "ana", Text: "approved", URL: "https://r/1"},
	}
	it := itemWithURL(t, m, "https://r/1")
	if line := ansi.Strip(strings.Join(it.lines, "")); !strings.Contains(line, "@ana approved") || !strings.Contains(line, "2h") {
		t.Fatalf("activity row = %q", line)
	}
	if !strings.Contains(plainView(m), "Activity") {
		t.Fatal("no activity heading")
	}
}

func TestReviewerRows(t *testing.T) {
	m, now := viewHarness()
	readyFixture(m, *now)
	m.Snapshot.Reviewers = []model.Reviewer{
		{Login: "radu", State: "CHANGES_REQUESTED", At: now.Add(-40 * time.Minute), URL: "https://r/2"},
		{Login: "ioana", State: "REQUESTED"},
	}
	it := itemWithURL(t, m, "https://r/2")
	if line := ansi.Strip(strings.Join(it.lines, "")); !strings.Contains(line, "@radu") || !strings.Contains(line, "changes requested") || !strings.Contains(line, "40m") {
		t.Fatalf("reviewer row = %q", line)
	}
	if !strings.Contains(plainView(m), "@ioana") {
		t.Fatal("requested reviewer missing")
	}
}

func TestZoomHintHiddenWithoutZoom(t *testing.T) {
	m, now := viewHarness()
	readyFixture(m, *now)
	if !strings.Contains(plainView(m), "z zoom") {
		t.Fatal("zoom hint missing while zoom works")
	}
	m.Zoom = nil
	if strings.Contains(plainView(m), "z zoom") {
		t.Fatal("zoom hint shown without a zoom action")
	}
	m.ShowHelp = true
	if strings.Contains(plainView(m), "zoom") {
		t.Fatal("help lists zoom without a zoom action")
	}
}
