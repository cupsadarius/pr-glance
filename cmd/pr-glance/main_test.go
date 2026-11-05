package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cupsadarius/pr-glance/model"
)

func TestVersionAndUnknownSubcommands(t *testing.T) {
	var out bytes.Buffer
	original := version
	version = "1.2.3-test"
	t.Cleanup(func() { version = original })
	if err := run([]string{"version"}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "1.2.3-test\n" {
		t.Fatalf("version output = %q", out.String())
	}
	for _, args := range [][]string{{"nope"}, {"snapshot"}, {"snapshot", "--nope"}, {"--nope"}} {
		out.Reset()
		if err := run(args, &out); err == nil {
			t.Fatalf("%v accepted", args)
		}
		if out.Len() != 0 {
			t.Fatalf("%v wrote to stdout: %q", args, out.String())
		}
	}
	if err := run([]string{"nope"}, &out); err == nil || !strings.Contains(err.Error(), `unknown subcommand "nope"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestSnapshotOnATemporaryCheckout(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-b", "feature")
	git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial")
	bin := t.TempDir()
	script := "#!/bin/sh\necho 'no pull requests found for branch \"feature\"' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out bytes.Buffer
	// gh reports no pull request: the summary is empty, not an error.
	if err := run([]string{"snapshot", "--cwd", dir}, &out); err != nil {
		t.Fatalf("%v %s", err, out.String())
	}
	var decoded struct {
		Source struct {
			Branch, Root, EmptyReason string
		}
	}
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if decoded.Source.Branch != "feature" || decoded.Source.Root == "" {
		t.Fatalf("source = %+v", decoded.Source)
	}
	if !strings.Contains(out.String(), `"EmptyReason": "no_pr"`) {
		t.Fatalf("summary missing the no-PR state: %s", out.String())
	}
}

func TestDefaultCWDIsWorkingDirectory(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	r, err := parseView(nil)
	if err != nil || r.cwd != wd {
		t.Fatalf("cwd = %q, want %q; error = %v", r.cwd, wd, err)
	}
	r, err = parseView([]string{"--cwd", "x"})
	if err != nil || r.cwd != "x" {
		t.Fatalf("cwd = %q, want %q; error = %v", r.cwd, "x", err)
	}
}

func TestFixedResolverShowsTheCheckout(t *testing.T) {
	dir := t.TempDir()
	s, err := fixedResolver{cwd: dir}.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.CWD != dir || !s.Visible || s.EmptyReason != model.NotGit {
		t.Fatalf("source = %+v", s)
	}
}

func TestStateDirFallsBackToXDGState(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_STATE_HOME", base)
	dir, err := stateDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, "pr-glance"); dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", home)
	if dir, err = stateDir(); err != nil || dir != filepath.Join(home, ".local", "state", "pr-glance") {
		t.Fatalf("dir = %q; error = %v", dir, err)
	}
}
