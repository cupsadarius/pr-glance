// Package model contains values shared by Glance's services and UI.
package model

import "time"

type EmptyReason string

const (
	NoWorkingPane EmptyReason = "no_working_pane"
	NoDirectory   EmptyReason = "no_directory"
	NotGit        EmptyReason = "not_git"
	DetachedHEAD  EmptyReason = "detached_head"
	UnbornHEAD    EmptyReason = "unborn_head"
)

type Source struct {
	WorkspaceID string
	TabID       string
	PaneID      string
	CWD         string
	Root        string
	Branch      string
	HEAD        string
	Visible     bool
	EmptyReason EmptyReason
}

const NoPR EmptyReason = "no_pr"

type PR struct {
	Host       string
	Repository string
	Number     int
	NodeID     string
	URL        string
}
type CheckState string

const (
	CheckFailed         CheckState = "failed"
	CheckTimedOut       CheckState = "timed_out"
	CheckCancelled      CheckState = "cancelled"
	CheckActionRequired CheckState = "action_required"
	CheckPending        CheckState = "pending"
	CheckPassed         CheckState = "passed"
	CheckNeutral        CheckState = "neutral"
	CheckSkipped        CheckState = "skipped"
	CheckUnknown        CheckState = "unknown"
)

type Check struct {
	Name, URL, Status, Conclusion string
	State                         CheckState
	// Required reports whether the base branch's rules require this check.
	Required bool
	// At is when the check completed (CheckRun) or was reported (status);
	// zero when GitHub gave no time.
	At time.Time
}
type CheckCounts struct{ Failed, Pending, Passed, Neutral, Skipped, Unknown int }

// StackEntry is one pull request of a stack, as GitHub reports it. Position 1
// is the bottom of the stack, closest to the trunk.
type StackEntry struct {
	Position                             int
	PR                                   PR
	Title, State, HeadBranch, BaseBranch string
	ReviewDecision                       string
	Draft                                bool
}

// Stack is a GitHub pull request stack. Entries are sorted by Position
// ascending and hold at most the first page GitHub returns, so Size can
// exceed len(Entries).
type Stack struct {
	Number, Size int
	BaseBranch   string
	Entries      []StackEntry
}
type Snapshot struct {
	PR                                          *PR
	EmptyReason                                 EmptyReason
	Title, Author, State                        string
	Draft                                       bool
	BaseBranch, HeadBranch, HeadRepository      string
	ReviewDecision                              string
	Commits, ChangedFiles, Additions, Deletions int
	Checks                                      []Check
	CheckCounts                                 CheckCounts
	// Stack is nil unless the pull request belongs to a stack; StackPosition
	// is then its position within it, and zero otherwise.
	Stack         *Stack
	StackPosition int
	// HeadOID is the head commit; MergeState and Mergeable are GitHub's
	// mergeStateStatus and mergeable, empty when not reported.
	HeadOID, MergeState, Mergeable string
	// Behind is how many commits the head is behind the base; nil when unknown.
	Behind      *int
	Reviewers   []Reviewer
	OpenThreads []ThreadSummary
	// ThreadCount counts every review thread, resolved ones included.
	ThreadCount int
	// Activity is the newest events first, at most ten.
	Activity  []Event
	FetchedAt time.Time
}

// Reviewer is one person or team on the review: the latest review they left,
// or REQUESTED while a review request is open.
type Reviewer struct {
	Login string
	Team  bool
	State string // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, REQUESTED
	At    time.Time
	URL   string
}

// ThreadSummary is an unresolved review thread and its first comment. Line is
// nil for a comment on the whole file.
type ThreadSummary struct {
	Path, Author, Body, URL string
	Line                    *int
}

type EventKind string

const (
	EventPush        EventKind = "push"
	EventApproved    EventKind = "approved"
	EventChanges     EventKind = "changes"
	EventCommented   EventKind = "commented"
	EventRequested   EventKind = "requested"
	EventCheckFailed EventKind = "check_failed"
	EventMerged      EventKind = "merged"
	EventClosed      EventKind = "closed"
	EventReopened    EventKind = "reopened"
	EventReady       EventKind = "ready"
)

// Event is one line of the Overview's activity feed.
type Event struct {
	At    time.Time
	Kind  EventKind
	Actor string
	Text  string
	URL   string
}
type ErrorKind string

const (
	RateLimitError       ErrorKind = "rate_limit"
	AuthenticationError  ErrorKind = "authentication"
	NetworkError         ErrorKind = "network"
	GitHubError          ErrorKind = "github"
	InvalidResponseError ErrorKind = "invalid_response"
)

type FetchError struct {
	// RetryAt is the known rate-limit retry time; zero requests a five-minute fallback.
	RetryAt time.Time
	Kind    ErrorKind
	Err     error
}

func (e *FetchError) Error() string { return string(e.Kind) + ": " + e.Err.Error() }
func (e *FetchError) Unwrap() error { return e.Err }

// Section separates independently fetched and cached discussion payloads.
type Section string

const (
	Overview Section = "overview"
	Comments Section = "comments"
	Reviews  Section = "reviews"
)

type Comment struct {
	ID, Author, Body, URL string
	CreatedAt             time.Time
}
type Review struct {
	Comment
	State string
}
type ReviewThread struct {
	ID, Path, URL      string
	Line               *int
	Resolved, Outdated bool
	Comments           []Comment
}
type Discussion struct {
	PR        PR
	Section   Section
	Comments  []Comment
	Reviews   []Review
	Threads   []ReviewThread
	Complete  bool
	FetchedAt time.Time
}
type CacheEntry struct {
	SchemaVersion int
	PR            PR
	Section       Section
	FetchedAt     time.Time
	Data          Discussion
}

// DiscussionFreshness is the single cache-freshness policy, shared by the disk
// cache and by the UI's staleness check.
const DiscussionFreshness = 300 * time.Second

func (e CacheEntry) Fresh(now time.Time) bool { return now.Sub(e.FetchedAt) < DiscussionFreshness }
