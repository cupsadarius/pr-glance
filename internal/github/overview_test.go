package github

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cupsadarius/pr-glance/model"
)

const overviewFixture = `{"data":{"node":{
"mergeable":"MERGEABLE","mergeStateStatus":"BEHIND",
"baseRef":{"compare":{"behindBy":3}},
"reviewRequests":{"nodes":[{"requestedReviewer":{"__typename":"User","login":"ioana"}}]},
"latestReviews":{"nodes":[{"author":{"login":"ana"},"state":"APPROVED","submittedAt":"2026-11-05T08:00:00Z","url":"https://github.com/o/r/pull/7#r1"}]},
"reviewThreads":{"totalCount":4,"nodes":[
 {"isResolved":false,"path":"a/query.go","line":88,"comments":{"nodes":[{"author":{"login":"radu"},"body":"N+1 here\nsecond line","url":"https://github.com/o/r/pull/7#d1"}]}},
 {"isResolved":true,"path":"a/query.go","line":90,"comments":{"nodes":[{"author":{"login":"radu"},"body":"done","url":"https://github.com/o/r/pull/7#d2"}]}},
 {"isResolved":false,"path":"a/query.go","line":141,"comments":{"nodes":[{"author":{"login":"radu"},"body":"index hint?","url":"https://github.com/o/r/pull/7#d3"}]}},
 {"isResolved":false,"path":"db/m.sql","line":null,"comments":{"nodes":[{"author":{"login":"ana"},"body":"down migration","url":"https://github.com/o/r/pull/7#d4"}]}}]},
"timelineItems":{"nodes":[]}}}}`

func TestOverviewQueryUsesHeadOID(t *testing.T) {
	r := &fixtureRunner{t: t, replies: []reply{{out: discovery("github.com", "o/r", "OPEN", false)}, {out: page(`[]`, false, "")}}}
	if _, err := (Client{Runner: r}).Snapshot(context.Background(), model.Source{CWD: "/source", Branch: "topic"}); err != nil {
		t.Fatal(err)
	}
	if len(r.overviewCalls) != 1 {
		t.Fatalf("overview queries = %d, want 1", len(r.overviewCalls))
	}
	args := strings.Join(r.overviewCalls[0], " ")
	if !strings.Contains(args, "head=abc123") || !strings.Contains(args, "id=PR_node") {
		t.Fatalf("overview args = %s", args)
	}
}

func TestSnapshotReadsOverviewFields(t *testing.T) {
	r := &fixtureRunner{t: t, overview: overviewFixture, replies: []reply{{out: discovery("github.com", "o/r", "OPEN", false)}, {out: page(`[]`, false, "")}}}
	got, err := (Client{Runner: r}).Snapshot(context.Background(), model.Source{CWD: "/source", Branch: "topic"})
	if err != nil {
		t.Fatal(err)
	}
	if got.HeadOID != "abc123" || got.MergeState != "BEHIND" || got.Mergeable != "MERGEABLE" {
		t.Fatalf("merge fields = %q %q %q", got.HeadOID, got.MergeState, got.Mergeable)
	}
	if got.Behind == nil || *got.Behind != 3 {
		t.Fatalf("behind = %v", got.Behind)
	}
	if got.ThreadCount != 4 || len(got.OpenThreads) != 3 {
		t.Fatalf("threads = %d / %d open", got.ThreadCount, len(got.OpenThreads))
	}
	first := got.OpenThreads[0]
	if first.Path != "a/query.go" || first.Line == nil || *first.Line != 88 || first.Author != "radu" ||
		first.Body != "N+1 here\nsecond line" || first.URL != "https://github.com/o/r/pull/7#d1" {
		t.Fatalf("first thread = %+v", first)
	}
	if got.OpenThreads[2].Line != nil {
		t.Fatal("a file-level thread has a line")
	}
	if len(got.Reviewers) != 2 || got.Reviewers[0].Login != "ioana" || got.Reviewers[0].State != "REQUESTED" ||
		got.Reviewers[1].Login != "ana" || got.Reviewers[1].State != "APPROVED" {
		t.Fatalf("reviewers = %+v", got.Reviewers)
	}
}

func TestMissingBaseLeavesBehindUnknown(t *testing.T) {
	r := &fixtureRunner{t: t, overview: `{"data":{"node":{"mergeable":"UNKNOWN","mergeStateStatus":"UNKNOWN","baseRef":null}}}`,
		replies: []reply{{out: discovery("github.com", "o/r", "OPEN", false)}, {out: page(`[]`, false, "")}}}
	got, err := (Client{Runner: r}).Snapshot(context.Background(), model.Source{CWD: "/source", Branch: "topic"})
	if err != nil || got.Behind != nil || got.MergeState != "UNKNOWN" {
		t.Fatalf("got %+v, err %v", got.Behind, err)
	}
}

func at(s string) time.Time {
	v, _ := time.Parse(time.RFC3339, s)
	return v
}

func TestReviewers(t *testing.T) {
	requests := []requestNode{
		{Reviewer: reviewerNode{Type: "User", Login: "radu"}},
		{Reviewer: reviewerNode{Type: "Team", Slug: "platform"}},
	}
	latest := []reviewNode{
		{Author: login{"alice"}, State: "COMMENTED", SubmittedAt: at("2026-11-05T07:00:00Z")}, // the PR author
		{Author: login{"radu"}, State: "CHANGES_REQUESTED", SubmittedAt: at("2026-11-05T07:00:00Z"), URL: "u1"},
		{Author: login{"ana"}, State: "APPROVED", SubmittedAt: at("2026-11-05T08:00:00Z"), URL: "u2"},
		{Author: login{"vlad"}, State: "COMMENTED", SubmittedAt: at("2026-11-05T09:00:00Z"), URL: "u3"},
		{Author: login{"mara"}, State: "CHANGES_REQUESTED", SubmittedAt: at("2026-11-05T06:00:00Z"), URL: "u4"},
	}
	got := reviewers(requests, latest, "alice")
	var order []string
	for _, r := range got {
		order = append(order, r.Login+":"+r.State)
	}
	want := "mara:CHANGES_REQUESTED platform:REQUESTED radu:REQUESTED ana:APPROVED vlad:COMMENTED"
	if strings.Join(order, " ") != want {
		t.Fatalf("reviewers = %s\nwant      %s", strings.Join(order, " "), want)
	}
	for _, r := range got {
		if r.Login == "platform" && !r.Team {
			t.Fatal("team request is not marked as a team")
		}
		if r.Login == "ana" && (r.URL != "u2" || !r.At.Equal(at("2026-11-05T08:00:00Z"))) {
			t.Fatalf("ana = %+v", r)
		}
	}
}

func TestActivity(t *testing.T) {
	commit := func(who, when string) timelineNode {
		n := timelineNode{Type: "PullRequestCommit", URL: "c-" + when}
		n.Commit.CommittedDate = at(when)
		n.Commit.Author.User.Login = who
		return n
	}
	timeline := []timelineNode{
		commit("darius", "2026-11-05T08:00:00Z"),
		{Type: "PullRequestReview", Author: login{"ana"}, State: "APPROVED", SubmittedAt: at("2026-11-05T08:30:00Z"), URL: "r1"},
		commit("darius", "2026-11-05T09:00:00Z"),
		commit("darius", "2026-11-05T09:01:00Z"),
		commit("darius", "2026-11-05T09:02:00Z"),
		{Type: "ReviewRequestedEvent", Actor: login{"darius"}, CreatedAt: at("2026-11-05T09:03:00Z"), RequestedReviewer: reviewerNode{Type: "User", Login: "radu"}},
	}
	checks := []model.Check{
		{Name: "e2e", State: model.CheckFailed, URL: "k1", At: at("2026-11-05T09:10:00Z")},
		{Name: "lint", State: model.CheckPassed, At: at("2026-11-05T09:11:00Z")},
		{Name: "old", State: model.CheckFailed}, // no time: not an event
	}
	got := activity(timeline, checks)
	var lines []string
	for _, e := range got {
		lines = append(lines, string(e.Kind)+"|"+e.Actor+"|"+e.Text+"|"+e.URL)
	}
	want := []string{
		"check_failed||e2e failed|k1",
		"requested|darius|requested @radu|",
		"push|darius|pushed 3 commits|c-2026-11-05T09:02:00Z",
		"approved|ana|approved|r1",
		"push|darius|pushed 1 commit|c-2026-11-05T08:00:00Z",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("activity =\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	var many []timelineNode
	for i := 0; i < 15; i++ {
		many = append(many, timelineNode{Type: "IssueComment", Author: login{"x"}, CreatedAt: at("2026-11-05T09:00:00Z").Add(time.Duration(i) * time.Minute)})
	}
	if n := len(activity(many, nil)); n != 10 {
		t.Fatalf("activity keeps %d events, want 10", n)
	}
}

func TestChecksCarryRequiredAndTime(t *testing.T) {
	nodes := `[{"__typename":"CheckRun","name":"e2e","status":"COMPLETED","conclusion":"FAILURE","completedAt":"2026-11-05T09:10:00Z","isRequired":true},` +
		`{"__typename":"StatusContext","context":"ci/legacy","state":"SUCCESS","createdAt":"2026-11-05T09:00:00Z","isRequired":false}]`
	r := &fixtureRunner{t: t, replies: []reply{{out: discovery("github.com", "o/r", "OPEN", false)}, {out: page(nodes, false, "")}}}
	got, err := (Client{Runner: r}).Snapshot(context.Background(), model.Source{CWD: "/source", Branch: "topic"})
	if err != nil {
		t.Fatal(err)
	}
	var e2e, legacy model.Check
	for _, c := range got.Checks {
		switch c.Name {
		case "e2e":
			e2e = c
		case "ci/legacy":
			legacy = c
		}
	}
	if !e2e.Required || !e2e.At.Equal(at("2026-11-05T09:10:00Z")) || legacy.Required || !legacy.At.Equal(at("2026-11-05T09:00:00Z")) {
		t.Fatalf("e2e = %+v, legacy = %+v", e2e, legacy)
	}
	if !strings.Contains(strings.Join(r.calls[1], " "), "isRequired(pullRequestId:$id)") {
		t.Fatalf("checks query does not ask which checks are required: %v", r.calls[1])
	}
}

func TestOverviewErrorKeepsTheSnapshot(t *testing.T) {
	r := &fixtureRunner{t: t, overview: `{"errors":[{"type":"FORBIDDEN","message":"Resource not accessible by integration"}],"data":{"node":null}}`,
		replies: []reply{{out: discovery("github.com", "o/r", "OPEN", false)}, {out: page(`[{"__typename":"CheckRun","name":"e2e","status":"COMPLETED","conclusion":"SUCCESS"}]`, false, "")}}}
	got, err := (Client{Runner: r}).Snapshot(context.Background(), model.Source{CWD: "/source", Branch: "topic"})
	if err != nil {
		t.Fatalf("an overview error must not fail the summary: %v", err)
	}
	if got.PR == nil || len(got.Checks) != 1 || got.MergeState != "" {
		t.Fatalf("got PR %v, %d checks, merge state %q", got.PR, len(got.Checks), got.MergeState)
	}
}

func TestOpenThreadsGroupByFile(t *testing.T) {
	thread := func(path, url string) string {
		return `{"isResolved":false,"path":"` + path + `","line":1,"comments":{"nodes":[{"author":{"login":"x"},"body":"b","url":"` + url + `"}]}}`
	}
	fixture := `{"data":{"node":{"mergeStateStatus":"CLEAN","reviewThreads":{"totalCount":3,"nodes":[` +
		thread("a.go", "1") + `,` + thread("b.go", "2") + `,` + thread("a.go", "3") + `]}}}}`
	r := &fixtureRunner{t: t, overview: fixture, replies: []reply{{out: discovery("github.com", "o/r", "OPEN", false)}, {out: page(`[]`, false, "")}}}
	got, err := (Client{Runner: r}).Snapshot(context.Background(), model.Source{CWD: "/source", Branch: "topic"})
	if err != nil {
		t.Fatal(err)
	}
	var urls []string
	for _, th := range got.OpenThreads {
		urls = append(urls, th.URL)
	}
	if strings.Join(urls, " ") != "1 3 2" {
		t.Fatalf("thread order = %v, want threads of one file together, files in first-seen order", urls)
	}
}

func TestActivitySkipsEventsWithoutATime(t *testing.T) {
	pending := []timelineNode{{Type: "PullRequestReview", Author: login{"ana"}, State: "PENDING"}}
	if got := activity(pending, nil); len(got) != 0 {
		t.Fatalf("an event without a time has no place in the feed: %+v", got)
	}
}

func TestScores(t *testing.T) {
	bodies := []scoreNode{
		{Author: login{"greptile-apps"}, Body: "<h3>Confidence Score: 4/5</h3>\n[Medium impact]", URL: "g1", UpdatedAt: at("2026-11-10T15:34:00Z")},
		{Author: login{"darius"}, Body: "Merge confidence: 6/10\nthe judge scored 3/10", URL: "d1", UpdatedAt: at("2026-11-10T10:00:00Z")},
		{Author: login{"darius"}, Body: "**Merge confidence:** 8/10\nReviewer regrade: the judge scored 6/10", URL: "d2", UpdatedAt: at("2026-11-10T16:38:00Z")},
		{Author: login{"darius"}, Body: "The score and detail are in the sticky.", URL: "d3", UpdatedAt: at("2026-11-11T09:00:00Z")},
		{Author: login{"bogdan"}, Body: "fixed, thanks", URL: "b1", UpdatedAt: at("2026-11-10T17:00:00Z")},
		{Author: login{"bot"}, Body: "Score: 12/10", URL: "x1", UpdatedAt: at("2026-11-10T18:00:00Z")},
	}
	got := scores(bodies)
	var rows []string
	for _, s := range got {
		rows = append(rows, fmt.Sprintf("%s:%g/%g:%s", s.Author, s.Value, s.Max, s.URL))
	}
	want := "darius:8/10:d2 greptile-apps:4/5:g1"
	if strings.Join(rows, " ") != want {
		t.Fatalf("scores = %s\nwant     %s", strings.Join(rows, " "), want)
	}
}
