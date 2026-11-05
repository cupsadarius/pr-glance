package github

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/cupsadarius/pr-glance/model"
)

const maxEvents = 10

// overviewQuery reads what the Overview shows beyond the checks: GitHub's merge
// verdict, how far the head is behind the base, the reviewers, the review
// threads and the recent timeline. compare takes the head commit, not the
// branch name, so it also works for a branch in a fork.
const overviewQuery = `query($id:ID!,$head:String!){node(id:$id){... on PullRequest{` +
	`mergeable mergeStateStatus baseRef{compare(headRef:$head){behindBy}} ` +
	`reviewRequests(first:20){nodes{requestedReviewer{__typename ... on User{login} ... on Team{slug} ... on Bot{login} ... on Mannequin{login}}}} ` +
	`latestReviews(first:20){nodes{author{login} state submittedAt url}} ` +
	`reviewThreads(first:100){totalCount nodes{isResolved path line comments(first:1){nodes{author{login} body url}}}} ` +
	`timelineItems(last:30,itemTypes:[PULL_REQUEST_COMMIT,PULL_REQUEST_REVIEW,ISSUE_COMMENT,REVIEW_REQUESTED_EVENT,READY_FOR_REVIEW_EVENT,MERGED_EVENT,CLOSED_EVENT,REOPENED_EVENT]){nodes{__typename ` +
	`... on PullRequestCommit{url commit{committedDate author{user{login}}}} ` +
	`... on PullRequestReview{author{login} state submittedAt url} ` +
	`... on IssueComment{author{login} createdAt url} ` +
	`... on ReviewRequestedEvent{actor{login} createdAt requestedReviewer{__typename ... on User{login} ... on Team{slug} ... on Bot{login} ... on Mannequin{login}}} ` +
	`... on ReadyForReviewEvent{actor{login} createdAt} ` +
	`... on MergedEvent{actor{login} createdAt url} ` +
	`... on ClosedEvent{actor{login} createdAt} ` +
	`... on ReopenedEvent{actor{login} createdAt}}}}}}`

type login struct{ Login string }

type reviewerNode struct {
	Type  string `json:"__typename"`
	Login string
	Slug  string
}

func (r reviewerNode) name() string {
	if r.Type == "Team" {
		return r.Slug
	}
	return r.Login
}

type requestNode struct {
	Reviewer reviewerNode `json:"requestedReviewer"`
}

type reviewNode struct {
	Author      login
	State       string
	SubmittedAt time.Time
	URL         string
}

type timelineNode struct {
	Type        string `json:"__typename"`
	URL         string
	Author      login
	Actor       login
	State       string
	SubmittedAt time.Time
	CreatedAt   time.Time
	Commit      struct {
		CommittedDate time.Time
		Author        struct{ User login }
	}
	RequestedReviewer reviewerNode
}

type overviewNode struct {
	Mergeable, MergeStateStatus string
	BaseRef                     *struct {
		Compare *struct{ BehindBy int }
	}
	ReviewRequests struct{ Nodes []requestNode }
	LatestReviews  struct{ Nodes []reviewNode }
	ReviewThreads  struct {
		TotalCount int
		Nodes      []struct {
			IsResolved bool
			Path       string
			Line       *int
			Comments   struct {
				Nodes []struct {
					Author    login
					Body, URL string
				}
			}
		}
	}
	TimelineItems struct{ Nodes []timelineNode }
}

// overview fills the Overview fields of s. It needs the head commit; without
// one it leaves them empty.
func (c Client) overview(ctx context.Context, cwd, host, id string, s *model.Snapshot) error {
	if s.HeadOID == "" {
		return nil
	}
	data, err := c.graphql(ctx, cwd, host, overviewQuery, "id="+id, "head="+s.HeadOID)
	if err != nil {
		return err
	}
	var response struct{ Node *overviewNode }
	if err := json.Unmarshal(data, &response); err != nil {
		return invalid("decode overview: %v", err)
	}
	n := response.Node
	if n == nil {
		return nil
	}
	s.MergeState, s.Mergeable = n.MergeStateStatus, n.Mergeable
	if n.BaseRef != nil && n.BaseRef.Compare != nil {
		behind := n.BaseRef.Compare.BehindBy
		s.Behind = &behind
	}
	s.ThreadCount = n.ReviewThreads.TotalCount
	for _, t := range n.ReviewThreads.Nodes {
		if t.IsResolved {
			continue
		}
		summary := model.ThreadSummary{Path: t.Path, Line: t.Line}
		if cs := t.Comments.Nodes; len(cs) > 0 {
			summary.Author, summary.Body, summary.URL = cs[0].Author.Login, cs[0].Body, cs[0].URL
		}
		s.OpenThreads = append(s.OpenThreads, summary)
	}
	s.Reviewers = reviewers(n.ReviewRequests.Nodes, n.LatestReviews.Nodes, s.Author)
	s.Activity = activity(n.TimelineItems.Nodes, s.Checks)
	return nil
}

var reviewerRank = map[string]int{"CHANGES_REQUESTED": 0, "REQUESTED": 1, "APPROVED": 2, "COMMENTED": 3, "DISMISSED": 4}

// reviewers merges open review requests with each reviewer's latest review,
// leaving out the pull request's author. An open request means GitHub asks
// for a new review, so it replaces an older review by the same person.
func reviewers(requests []requestNode, latest []reviewNode, author string) []model.Reviewer {
	var out []model.Reviewer
	seen := map[string]bool{}
	for _, r := range requests {
		name := r.Reviewer.name()
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, model.Reviewer{Login: name, Team: r.Reviewer.Type == "Team", State: "REQUESTED"})
	}
	for _, r := range latest {
		name := r.Author.Login
		if name == "" || name == author || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, model.Reviewer{Login: name, State: r.State, At: r.SubmittedAt, URL: r.URL})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := rank(out[i].State), rank(out[j].State)
		if a != b {
			return a < b
		}
		return out[i].Login < out[j].Login
	})
	return out
}

func rank(state string) int {
	if r, ok := reviewerRank[state]; ok {
		return r
	}
	return len(reviewerRank)
}

// activity turns timeline items and failed checks into feed events, newest
// first. Consecutive commits by one author read as one push.
func activity(timeline []timelineNode, checks []model.Check) []model.Event {
	var out []model.Event
	pushes := 0
	for _, n := range timeline {
		if n.Type != "PullRequestCommit" {
			pushes = 0
		}
		switch n.Type {
		case "PullRequestCommit":
			who := n.Commit.Author.User.Login
			if pushes > 0 && out[len(out)-1].Actor == who {
				pushes++
				last := &out[len(out)-1]
				last.At, last.URL, last.Text = n.Commit.CommittedDate, n.URL, fmt.Sprintf("pushed %d commits", pushes)
				continue
			}
			pushes = 1
			out = append(out, model.Event{At: n.Commit.CommittedDate, Kind: model.EventPush, Actor: who, Text: "pushed 1 commit", URL: n.URL})
		case "PullRequestReview":
			e := model.Event{At: n.SubmittedAt, Actor: n.Author.Login, URL: n.URL}
			switch n.State {
			case "APPROVED":
				e.Kind, e.Text = model.EventApproved, "approved"
			case "CHANGES_REQUESTED":
				e.Kind, e.Text = model.EventChanges, "requested changes"
			default:
				e.Kind, e.Text = model.EventCommented, "reviewed"
			}
			out = append(out, e)
		case "IssueComment":
			out = append(out, model.Event{At: n.CreatedAt, Kind: model.EventCommented, Actor: n.Author.Login, Text: "commented", URL: n.URL})
		case "ReviewRequestedEvent":
			out = append(out, model.Event{At: n.CreatedAt, Kind: model.EventRequested, Actor: n.Actor.Login, Text: "requested @" + n.RequestedReviewer.name()})
		case "ReadyForReviewEvent":
			out = append(out, model.Event{At: n.CreatedAt, Kind: model.EventReady, Actor: n.Actor.Login, Text: "marked ready"})
		case "MergedEvent":
			out = append(out, model.Event{At: n.CreatedAt, Kind: model.EventMerged, Actor: n.Actor.Login, Text: "merged", URL: n.URL})
		case "ClosedEvent":
			out = append(out, model.Event{At: n.CreatedAt, Kind: model.EventClosed, Actor: n.Actor.Login, Text: "closed"})
		case "ReopenedEvent":
			out = append(out, model.Event{At: n.CreatedAt, Kind: model.EventReopened, Actor: n.Actor.Login, Text: "reopened"})
		}
	}
	for _, c := range checks {
		if c.State == model.CheckFailed && !c.At.IsZero() {
			out = append(out, model.Event{At: c.At, Kind: model.EventCheckFailed, Text: c.Name + " failed", URL: c.URL})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	if len(out) > maxEvents {
		out = out[:maxEvents]
	}
	return out
}
