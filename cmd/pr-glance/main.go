// Command pr-glance shows the pull request of one checkout's branch.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cupsadarius/pr-glance/command"
	"github.com/cupsadarius/pr-glance/glance"
	"github.com/cupsadarius/pr-glance/internal/github"
	"github.com/cupsadarius/pr-glance/model"
)

// version is replaced at release build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = "usage: pr-glance [--cwd PATH] | snapshot --cwd PATH | version"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "pr-glance: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return view(ctx, args)
	}
	switch args[0] {
	case "snapshot":
		return snapshot(ctx, args[1:], stdout)
	case "version":
		_, err := fmt.Fprintln(stdout, version)
		return err
	}
	return fmt.Errorf("unknown subcommand %q\n%s", args[0], usage)
}

// fixedResolver shows one checkout. Nothing reports focus, so the pane always
// counts as visible.
type fixedResolver struct{ cwd string }

func (r fixedResolver) Resolve(ctx context.Context) (model.Source, error) {
	s, err := glance.Checkout(ctx, model.Source{CWD: r.cwd})
	s.Visible = true
	return s, err
}

func parseView(args []string) (fixedResolver, error) {
	flags := flag.NewFlagSet("pr-glance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", "", "checkout directory to show")
	if err := flags.Parse(args); err != nil {
		return fixedResolver{}, fmt.Errorf("%w\n%s", err, usage)
	}
	if *cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fixedResolver{}, err
		}
		*cwd = wd
	}
	return fixedResolver{cwd: *cwd}, nil
}

func view(ctx context.Context, args []string) error {
	r, err := parseView(args)
	if err != nil {
		return err
	}
	dir, err := stateDir()
	if err != nil {
		return err
	}
	return glance.Run(ctx, glance.Options{Resolver: r, StateDir: dir})
}

// stateDir is the cache home: $XDG_STATE_HOME/pr-glance, else
// ~/.local/state/pr-glance.
func stateDir() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	dir := filepath.Join(base, "pr-glance")
	return dir, os.MkdirAll(dir, 0o755)
}

// snapshot prints one summary for an explicit checkout, for diagnosis. It
// fetches no discussion.
func snapshot(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	cwd := flags.String("cwd", "", "checkout directory to inspect")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *cwd == "" {
		return errors.New("snapshot requires --cwd PATH")
	}
	source, err := glance.Checkout(ctx, model.Source{CWD: *cwd})
	if err != nil {
		return err
	}
	out := struct {
		Source   model.Source
		Snapshot *model.Snapshot `json:",omitempty"`
	}{Source: source}
	if source.EmptyReason == "" {
		s, err := (github.Client{Runner: command.ExecRunner{}}).Snapshot(ctx, source)
		if err != nil {
			return err
		}
		out.Snapshot = &s
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(out)
}
