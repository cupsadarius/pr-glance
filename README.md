# PR Glance

PR Glance shows the GitHub pull request for a checkout's branch in a terminal
pane: PR identity, statistics, CI checks, review decision, comments and review
threads. It reads everything through your authenticated `gh` and is read-only
against GitHub: it displays data and opens links.

```text
 OPEN  #3630  acme/service           12s ago
Re-request denied approvals when the
reviewer list changes
@author · 143 commits · 8 files · +284 -76

Overview  Comments  Reviews

 CI  ⠋ 1 running · ✗ 4 failing
 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━────────────
›× unit tests                         failed
 × deploy preview                  timed out
 × e2e                             cancelled
 × migrate                   action required
 ⠋ integration tests                 pending
 ▸ ✓ 2 passed · 1 neutral · 1 skipped · 1 u…

 Review  changes requested

r refresh o browser z zoom ? help q close
```

The mockup is the Overview rendered in a 44-column pane. Content wraps to the
pane width and follows terminal resizing.

Colour comes from the terminal's 16 ANSI colours, so Glance follows your theme
and defines no custom palette: green for passed checks, additions and
approvals, red for failures, deletions and changes requested, yellow for
pending checks, stale data and rate limits, cyan for authors and file paths.

## Hosts

PR Glance runs on its own, or inside a terminal multiplexer through a wrapper
that tells it which checkout to show:

- [herdr-pr-glance](https://github.com/cupsadarius/herdr-pr-glance): a Herdr
  plugin that follows the working pane of the current tab.
- [rex-pr-glance](https://github.com/cupsadarius/rex-pr-glance): a Rex plugin
  that follows the focused block of the current window.
- `pr-glance` (this repository): a standalone binary for one checkout, for any
  other terminal or multiplexer.

Wrappers import the Go package `github.com/cupsadarius/pr-glance/glance`
(`glance.Run`, `glance.Checkout`) and the types in `.../model`.

## Prerequisites

- `git`
- `gh`, authenticated (`gh auth login`)
- macOS or Linux, amd64 or arm64

## Install

Download the archive for your platform from the
[releases page](https://github.com/cupsadarius/pr-glance/releases) and put
`pr-glance` on your `PATH`, or build from source:

```sh
go install github.com/cupsadarius/pr-glance/cmd/pr-glance@latest
```

## Usage

```sh
pr-glance                      # the checkout in the current directory
pr-glance --cwd PATH           # another checkout
pr-glance snapshot --cwd PATH  # print one summary as JSON, for diagnosis
pr-glance version
```

## Controls

| Input | Action |
| --- | --- |
| `1`, `2`, `3` or click a section | Overview, Comments, Reviews |
| `j`, `k` or `↓`, `↑`; `pgup`, `pgdn`; mouse wheel | Navigate and scroll |
| Enter / click the CI fold row | Show or hide the folded checks |
| Enter / click a thread | Expand or collapse |
| Enter / click a stack row | Show that pull request of the stack |
| `]`, `[` | Move to the stack entry above or below |
| `\` | Return to the branch's own pull request |
| `r` | Refresh the active section, ignoring cache freshness |
| `o` | Open the selected item's URL, falling back to the PR URL; does nothing on the CI fold row |
| Click the PR title or number | Open the pull request in the browser |
| `z` | Toggle pane zoom, when the host supports it |
| `?` | Help: the full key list; `?` or Escape closes it |
| `q`, Escape or `ctrl+c` | Close Glance (Escape closes help first when it is open) |

Only the up and down arrows are bound; left and right do nothing.

### Sections

- **Overview** — the header's first line holds the state badge, PR number,
  repository and refresh age, plus `stale`, `pinned` or rate-limit status when
  they apply. The title follows, then a meta line: author · commits · files ·
  `+adds -dels`, with counts from 10,000 shortened (`+25k`, `+1M`). The body
  starts with CI: failing checks, then running checks, under a coloured bar,
  with passed, neutral, skipped and unknown checks folded into one row that
  Enter or a click expands. An empty check list means "No checks", not "all
  passed". The review decision comes next, then the stack.
- **Stacks** — when the PR belongs to a GitHub stack, Overview lists the whole
  stack after CI and the review decision, top first, with each entry's number,
  state, title and review decision, and marks the one on screen. Selecting an
  entry — Enter, a click, or `]` and `[` — shows and polls that pull request
  instead of the branch's own; the header then says `pinned` and `\` goes
  back. Stacks made in the GitHub UI and with `gh stack` are the same thing,
  and reading one costs no extra request. Stacks larger than 50 entries show
  the first 50.
- **Comments** — top-level PR conversation comments, loaded on demand.
- **Reviews** — submitted reviews and inline review threads, loaded on demand.
  Unresolved threads come first; threads start collapsed.

Comment and review bodies are rendered as Markdown with Glamour, so headings,
lists, code blocks and tables keep their shape. Links are shown but are not
clickable — use `o` to open the selected item in a browser. Very large bodies
fall back to plain text.

### Refresh behaviour

- The checkout and branch are re-read every 2 seconds, so a branch switch shows
  up quickly.
- The Overview summary is polled from GitHub every 60 seconds while the host
  reports the pane as visible. The standalone binary always counts as visible.
- Comments and Reviews are never fetched automatically. They load when you open
  the section, or when you press `r`.
- Cached discussions are considered fresh for 5 minutes. Opening a section with
  a fresh cache makes no GitHub request; an expired cache is shown marked stale
  while a fetch runs.
- Failures keep the previous data for the same PR and mark it stale.

## Cache

Discussion payloads are stored as JSON under `STATE_DIR/cache/*.json`, one
entry per PR and section, with user-private permissions (directory `0700`,
files `0600`). The standalone binary uses `$XDG_STATE_HOME/pr-glance`, default
`~/.local/state/pr-glance`; wrappers choose their own. Filenames are hashes of
the canonical PR URL and section. Entries older than 7 days are removed during
later cache use. The cache holds discussion text only; it stores no tokens.

## Troubleshooting

| Symptom | What to do |
| --- | --- |
| "No PR for this branch" | gh found no PR for the checkout and branch. Push the branch and open a PR; Glance never creates one. |
| Authentication error | Run `gh auth login`. The pane shows the same hint. |
| "rate limited, retry in Ns" | GitHub applied a rate limit. Glance honours the retry time when GitHub supplies one, otherwise waits five minutes. Manual refresh respects the same cooldown. |
| "stale" marker | The last refresh failed or the data is past its interval. The shown data is the last good result for the same PR. |
