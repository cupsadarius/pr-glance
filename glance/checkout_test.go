package glance

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cupsadarius/pr-glance/command"
	"github.com/cupsadarius/pr-glance/model"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	v, e := (command.ExecRunner{}).Run(context.Background(), dir, append([]string{"git"}, args...)...)
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func TestRealCheckouts(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "repo space; $(not-shell)")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-b", "main")
	git(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial")
	check := func(cwd, branch string, reason model.EmptyReason) {
		t.Helper()
		got, err := Checkout(context.Background(), model.Source{CWD: cwd})
		if err != nil || got.Branch != branch || got.EmptyReason != reason {
			t.Fatalf("%+v %v", got, err)
		}
	}
	nested := filepath.Join(dir, "nested")
	os.Mkdir(nested, 0700)
	check(nested, "main", "")
	git(t, dir, "checkout", "-b", "feature")
	check(nested, "feature", "")
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, dir, "worktree", "add", "-b", "linked", linked)
	check(linked, "linked", "")
	check(dir, "feature", "")
	git(t, linked, "checkout", "--detach")
	check(linked, "", model.DetachedHEAD)
	check(t.TempDir(), "", model.NotGit)
}

func TestUnbornAndMissingDirectory(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	s, e := Checkout(context.Background(), model.Source{CWD: dir})
	if e != nil || s.EmptyReason != model.UnbornHEAD {
		t.Fatalf("%+v %v", s, e)
	}
	if _, e = Checkout(context.Background(), model.Source{CWD: filepath.Join(dir, "missing")}); e == nil {
		t.Fatal("missing directory treated as non-Git")
	}
}

func TestBranchNameWithMatchingTag(t *testing.T) {
	for _, branch := range []string{"main", "feature/topic"} {
		t.Run(branch, func(t *testing.T) {
			dir := t.TempDir()
			git(t, dir, "init", "-b", branch)
			git(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial")
			git(t, dir, "tag", branch)
			got, err := Checkout(context.Background(), model.Source{CWD: dir})
			if err != nil || got.Branch != branch {
				t.Fatalf("branch = %q, want %q; error = %v", got.Branch, branch, err)
			}
		})
	}
}
