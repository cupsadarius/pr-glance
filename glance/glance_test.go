package glance

import "testing"

func TestBrowserArgvPassesTheURLAsOneArgument(t *testing.T) {
	argv := browserArgv("https://example.invalid/a b?x=1&y=2")
	if len(argv) != 2 || argv[1] != "https://example.invalid/a b?x=1&y=2" {
		t.Fatalf("argv = %q", argv)
	}
}
