# Merge-readiness Overview — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Replace the Overview body with layout D: GitHub's merge verdict, five condition rows, reviewers, open threads, stack tree and an activity feed.

**Architecture:** The GitHub client adds one GraphQL request per summary (`overviewQuery`) and two fields to the checks query; it returns everything in `model.Snapshot`. A pure `readiness(model.Snapshot)` in `internal/ui` decides the verdict and each row's state. `overviewBody` renders the sections as `bodyItem`s, so the cursor, click and scroll model stays as it is.

**Tech Stack:** Go 1.25, Bubble Tea v2, lipgloss v2, GitHub GraphQL via `gh api graphql`.

**Spec:** `herdr-glance-pr/docs/superpowers/specs/2026-11-05-split-and-readiness-overview-design.md`, Part 2. Prototype: `spike/variants/main.go -v combo`.

## Global Constraints

- Verdict from `mergeStateStatus` only: `CLEAN`, `HAS_HOOKS` → `Ready to merge`; `UNSTABLE` → `Ready to merge` + yellow `non-required checks fail`; `BLOCKED`, `BEHIND`, `DIRTY` → `Blocked`; `UNKNOWN`/empty → faint `Checking mergeability…`. State `MERGED` → `Merged`, `CLOSED` → `Closed`, draft → `Draft` (checked before mergeStateStatus).
- A row is red only on a GitHub signal: a failing check with `isRequired`; `reviewDecision` `CHANGES_REQUESTED` or `REVIEW_REQUIRED`; `mergeStateStatus == BEHIND` for Branch; `mergeable == CONFLICTING` for Mergeable. Conversations is never red (resolution rules are not readable without admin rights).
- `BLOCKED` with no red row → red line `blocked by a repository rule` under the verdict.
- Required-approval count (M) is not fetched: branch protection needs admin rights. Approvals shows the count and the decision only.
- 16 ANSI colours; layout holds at 44 columns; Comments and Reviews tabs unchanged.
- Keep: failing/running check rows and the fold row with their URLs; stack navigation (Enter, click, `]`, `[`, `\`).
- A nil `Zoom` hides `z` from the footer and the help.
- One summary refresh = discovery (`gh pr view`) + checks query page(s) + one `overviewQuery`.

## Review Focus

1. A PR with zero reviews, zero threads, no stack, no checks → five rows plus activity, no empty headings (test `TestOverviewAllEmptySections`).
2. `mergeStateStatus` `UNKNOWN` right after a push → faint verdict, no red rows from guesses (case in `TestReadiness`).
3. A fork PR: `compare` uses the head OID, not the branch name (test `TestOverviewQueryUsesHeadOID`).
4. 44-column pane: no row wider than the pane; long thread paths and bodies truncate with `…` (golden `goldenOverview44`).
5. A requested reviewer who already reviewed: one row, latest state wins (case in `TestReviewers`).

---

### Task 1: Model and GitHub fetch

**Files:** `model/types.go`; `internal/github/snapshot.go`; new `internal/github/overview.go`, `internal/github/overview_test.go`; `internal/github/snapshot_test.go`.

**Produces (model):**
```go
// Snapshot gains:
HeadOID     string
MergeState  string  // mergeStateStatus, upper case; "" when not reported
Mergeable   string  // MERGEABLE, CONFLICTING, UNKNOWN; "" when not reported
Behind      *int    // commits the head is behind the base; nil when unknown
Reviewers   []Reviewer
OpenThreads []ThreadSummary
ThreadCount int     // all review threads, resolved included
Activity    []Event // newest first, at most 10

// Check gains:
Required bool
At       time.Time // CheckRun completedAt or StatusContext createdAt

type Reviewer struct {
	Login string
	Team  bool
	State string // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED, REQUESTED
	At    time.Time
	URL   string
}
type ThreadSummary struct {
	Path, Author, Body, URL string
	Line                    *int
}
type EventKind string // EventPush, EventApproved, EventChanges, EventCommented, EventRequested, EventCheckFailed, EventMerged, EventClosed, EventReopened, EventReady
type Event struct {
	At    time.Time
	Kind  EventKind
	Actor string
	Text  string // "pushed 2 commits", "approved", "requested changes", "commented", "requested @x", "<check> failed", "merged", "closed", "reopened", "marked ready"
	URL   string
}
```

**GitHub:**
- `discoveryFields` gains `headRefOid`.
- `checksQuery` gains `$number:Int!`; CheckRun adds `completedAt isRequired(pullRequestNumber:$number)`, StatusContext adds `createdAt isRequired(pullRequestNumber:$number)`.
- `overviewQuery($id:ID!,$head:String!)`: `mergeable mergeStateStatus baseRef{compare(headRef:$head){behindBy}} reviewRequests(first:20){…} latestReviews(first:20){nodes{author{login} state submittedAt url}} reviewThreads(first:100){totalCount nodes{isResolved path line comments(first:1){nodes{author{login} body url}}}} timelineItems(last:30,itemTypes:[PULL_REQUEST_COMMIT,PULL_REQUEST_REVIEW,ISSUE_COMMENT,REVIEW_REQUESTED_EVENT,READY_FOR_REVIEW_EVENT,MERGED_EVENT,CLOSED_EVENT,REOPENED_EVENT]){…}` — shape verified live on 2026-11-05.
- Pure helpers in `overview.go`: `reviewers(requests, latest, author string) []model.Reviewer` (drop the PR author; one row per login; a pending request beats an older review; order CHANGES_REQUESTED, REQUESTED, APPROVED, COMMENTED, DISMISSED); `activity(timeline, checks) []model.Event` (consecutive commits by one author merge into one `pushed N commits` event at the last commit's time; failed checks with a time become `EventCheckFailed`; sort newest first; cap 10).
- A missing `baseRef` (deleted base) leaves `Behind` nil.

- [ ] Tests first: `TestOverviewQueryUsesHeadOID` (fake runner records the graphql args; `head=<oid>`), `TestSnapshotReadsOverviewFields` (fixture → MergeState, Mergeable, Behind=3, ThreadCount=4, 3 OpenThreads with path/line/author/body/url), `TestReviewers` (author dropped; re-request after a review → REQUESTED; team request → Team=true, Login=slug; order), `TestActivity` (3 commits by A then review by B then 1 commit by A → events `pushed 1 commit`, `approved`, `pushed 3 commits`; a failed required check at t → `e2e failed`; cap 10), `TestChecksCarryRequiredAndTime`.
- [ ] RED → implement → GREEN (`go test ./internal/github/ ./model/...`), whole suite green → commit.

### Task 2: `readiness`

**Files:** new `internal/ui/readiness.go`, `internal/ui/readiness_test.go`.

**Produces:**
```go
type level int // levelUnknown, levelOK, levelWarn, levelBad
type condition struct { name, detail string; level level; note string } // note: "(not required)"
type verdict struct { text string; level level; note string; rule bool } // rule: show "blocked by a repository rule"
func readiness(s model.Snapshot) (verdict, []condition) // conditions in order Checks, Approvals, Conversations, Branch, Mergeable
```
Row rules:
- Checks: required failing → bad `N failing`; other failing → warn `N failing (not required)`; running → warn `N running`; none → ok `no checks`; else ok `N passed`. Detail joins failing · running · passed when several apply.
- Approvals: count APPROVED reviewers. `APPROVED` → ok `N approvals`; `CHANGES_REQUESTED` → bad `changes requested`; `REVIEW_REQUIRED` → bad `review required` (+ `· N approvals` when N>0); empty decision → ok `N approvals` when N>0, else warn `no approvals (not required)`.
- Conversations: `ThreadCount == 0` → ok `no threads`; open>0 → warn `N unresolved of M`; else ok `all M resolved`.
- Branch: `Behind == nil` → unknown `unknown`; 0 → ok `up to date with BASE`; >0 and `BEHIND` → bad `N behind BASE`; >0 otherwise → warn `N behind BASE (not required)`.
- Mergeable: `CONFLICTING` → bad `conflicts with BASE`; `MERGEABLE` → ok `no conflicts`; else unknown `checking…`.
- [ ] Table test `TestReadiness` first: one case per verdict mapping, per row rule above, the `UNKNOWN` no-guess case, and `BLOCKED` with no red row → `rule=true`. RED → implement → GREEN → commit.

### Task 3: Overview body, header and footer

**Files:** `internal/ui/view.go` (`overviewBody`, `headerLines`, stack rendering), `internal/ui/checks.go` (`ciBlock` heading replaced), `internal/ui/keys.go` (`footerHints`, help without zoom), `internal/ui/view_test.go` (goldens + new tests).

Layout (each heading `Name ───` fills the width, faint rule, as in the spike):
1. Verdict line (bold, level colour) + optional note/rule line; bar of 5 segments (`━`, one per condition, level colour, 1-space gaps) when the pane is ≥ `minBarWidth`.
2. Blank, then 5 condition rows: `glyph name(15 cols) detail`; glyphs ✓ ok, ● warn, ✗ bad, ○ unknown. Failing and running check items, then the fold row, sit right after the Checks row, indented 2 (today's `ciBlock` items, `└ ` prefix).
3. `Reviewers` (hidden when empty): `@login` cyan, state glyph + word in level colour, faint age; each row a `bodyItem` with the review URL (none for requests).
4. `Threads  N open` (hidden when none open): file path line (cyan, head line), then one item per thread `├─L88  @author first line…`, `╰─` for the last of a file; item URL = first comment URL.
5. `Stack i/N` (hidden when not stacked): base branch faint, then today's entries top first with `├─`/`╰─` and `← here` on the shown one; selection behaviour unchanged.
6. `Activity` (hidden when empty): `glyph age @actor text`; glyphs ↑ push (magenta), ✓ approved (green), ✗ changes/check failed (red), ● comment/requested (yellow), ○ other (faint); each row an item with its URL. No gutter rows (height).
- Header: add a faint `head → base` line after the meta line (`prioCounts`).
- Footer and help omit `z` when `Model.Zoom == nil`.
- [ ] Tests first: update `goldenOverview44` and add `goldenOverviewReady44` (all green), `TestOverviewAllEmptySections`, `TestVerdictRuleLine`, `TestThreadRowsOpenThreadURL`, `TestActivityRowsOpenURL`, `TestZoomHintHiddenWithoutZoom`; keep every stack and check interaction test passing (adjust only expected text, never the asserted behaviour). RED → implement → GREEN → `go test -race ./...` → commit.

### Task 4: Docs, spike removal, release

- [ ] README Overview section and mockup describe the new layout; delete `spike/`; `VERSION` → `0.3.0`; commit.
- [ ] With the owner's go-ahead: push, CI, tag `v0.3.0`, release; bump `herdr-pr-glance` (→ 0.2.1) and `rex-pr-glance` (→ 0.1.1) to `pr-glance v0.3.0` with their release steps.
