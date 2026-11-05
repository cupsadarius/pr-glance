package glance

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cupsadarius/pr-glance/command"
	"github.com/cupsadarius/pr-glance/model"
)

// Checkout fills Root, Branch and HEAD for the directory in s.CWD, or sets
// EmptyReason when the directory is not a usable checkout.
func Checkout(ctx context.Context, s model.Source) (model.Source, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if s.CWD == "" {
		s.EmptyReason = model.NoDirectory
		return s, nil
	}
	run := func(args ...string) (string, error) {
		out, err := command.ExecRunner{}.Run(ctx, s.CWD, append([]string{"git"}, args...)...)
		return strings.TrimSuffix(out, "\n"), err
	}
	root, err := run("rev-parse", "--show-toplevel")
	if err != nil {
		var e *command.Error
		if errors.As(err, &e) && e.ExitCode == 128 && strings.Contains(e.Stderr, "not a git repository") {
			s.EmptyReason = model.NotGit
			return s, nil
		}
		return s, err
	}
	s.Root = root
	branch, err := run("symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		var e *command.Error
		if !errors.As(err, &e) || e.ExitCode != 1 {
			return s, err
		}
		s.EmptyReason = model.DetachedHEAD
	} else {
		s.Branch = strings.TrimPrefix(branch, "refs/heads/")
	}
	head, err := run("rev-parse", "--verify", "HEAD")
	if err != nil {
		var e *command.Error
		if s.Branch != "" && errors.As(err, &e) && e.ExitCode == 128 {
			s.EmptyReason = model.UnbornHEAD
			return s, nil
		}
		return s, err
	}
	s.HEAD = head
	return s, nil
}
