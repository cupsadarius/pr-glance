// Package glance is the public entry point of the pull request pane. A host
// (a multiplexer plugin or the pr-glance binary) supplies the checkout to
// show; glance fetches, caches and renders it.
package glance

import (
	"context"
	"errors"
	"runtime"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cupsadarius/pr-glance/command"
	"github.com/cupsadarius/pr-glance/internal/cache"
	"github.com/cupsadarius/pr-glance/internal/github"
	"github.com/cupsadarius/pr-glance/internal/ui"
)

// Resolver reports the checkout the pane shows. The pane polls it, so a host
// whose working directory changes returns the new one on the next call.
type Resolver = ui.SourceResolver

// ActionTimeout bounds the browser and zoom commands the pane triggers.
const ActionTimeout = 10 * time.Second

type Options struct {
	Resolver Resolver
	// Zoom toggles the pane's zoom in its host; nil makes the key a no-op.
	Zoom func() error
	// StateDir holds the discussion cache.
	StateDir string
}

// Run shows the pane until the user quits or ctx ends.
func Run(ctx context.Context, o Options) error {
	if o.Resolver == nil {
		return errors.New("glance: Options.Resolver is nil")
	}
	if o.StateDir == "" {
		return errors.New("glance: Options.StateDir is empty")
	}
	runner := command.ExecRunner{}
	m := ui.New(o.Resolver, github.Client{Runner: runner}, cache.New(o.StateDir), time.Now)
	m.Open = func(url string) error {
		ctx, cancel := context.WithTimeout(ctx, ActionTimeout)
		defer cancel()
		_, err := runner.Run(ctx, "", browserArgv(url)...)
		return err
	}
	m.Zoom = o.Zoom
	_, err := tea.NewProgram(m).Run()
	return err
}

// browserArgv opens a link without a shell; the URL is a single argument.
func browserArgv(url string) []string {
	if runtime.GOOS == "darwin" {
		return []string{"open", url}
	}
	return []string{"xdg-open", url}
}
