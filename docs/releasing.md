# Releasing pr-glance

Releases are prepared by automation and published only on an explicit instruction.
Nothing here runs by itself: the release workflow triggers on a pushed `v*` tag.

## Version sources

Two places carry the version and must agree:

- `VERSION`
- the git tag: always `v` + the file contents (`0.2.0` -> `v0.2.0`).

`.github/workflows/release.yml` refuses to publish when they disagree, and also
requires a completed successful CI run for the tagged commit.

Wrappers (`herdr-pr-glance`, `rex-pr-glance`) import this module at a tag, so a
tag is also a Go module version. Never move or delete a pushed tag.

## Steps

1. On a release branch, bump `VERSION`. Push the branch so CI runs and goes
   green for that commit.
2. Tag that same commit `vX.Y.Z` and push the tag.
3. The release workflow revalidates tag/`VERSION` agreement and the commit's
   green CI, then GoReleaser publishes the four archives
   (`darwin`/`linux` x `amd64`/`arm64`) and `checksums.txt`.
4. Verify the release page lists all five assets, and that
   `go list -m github.com/cupsadarius/pr-glance@vX.Y.Z` resolves.
5. Merge the release branch into the default branch.

## Local dry run

```sh
goreleaser check
goreleaser release --snapshot --clean
```

The snapshot writes `dist/` (git-ignored) with the four `.tar.gz` archives, each
containing exactly one executable `pr-glance` at the archive root, and
`checksums.txt`. `dist/<target>/pr-glance version` prints the built-in version
stamped through `-ldflags -X main.version=...`.
