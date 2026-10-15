package main

import "testing"

func TestChordWritesClipboard(t *testing.T) {
	// Only the two chords that make the sandbox clipboard change and that the
	// viewer is waiting to hear about. Everything else must stay silent: a
	// false positive reads the clipboard on every keystroke, and the viewer
	// writes whatever comes back onto the user's host clipboard.
	cases := []struct {
		combo string
		want  bool
	}{
		{"ctrl+c", true},
		{"ctrl+C", true},
		{"meta+c", true}, // Cmd+C on macOS arrives as a meta chord
		{"ctrl+x", true},
		{"meta+x", true},

		{`ctrl+shift+c`, false}, // DevTools
		{"ctrl+alt+c", false},
		{"ctrl+shift+x", false},
		{"c", false},       // plain typing
		{"shift+c", false}, // capital C
		{"ctrl+v", false},  // paste flows viewer → sandbox
		{"ctrl+a", false},  // selection only, clipboard untouched
		{"ctrl+insert", false},
		{"Return", false},
		{"ctrl+shift+Tab", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := chordWritesClipboard(tc.combo); got != tc.want {
			t.Errorf("chordWritesClipboard(%q) = %v, want %v", tc.combo, got, tc.want)
		}
	}
}

func TestReadSandboxClipboardWithoutCDP(t *testing.T) {
	// No CDP client (browser gone / not attached yet): report nothing rather
	// than blocking or panicking — the viewer simply keeps its old clipboard.
	prev := cdpClient
	cdpClient = nil
	defer func() { cdpClient = prev }()

	text, err := readSandboxClipboard()
	if err != nil {
		t.Fatalf("readSandboxClipboard() error = %v, want nil", err)
	}
	if text != "" {
		t.Fatalf("readSandboxClipboard() = %q, want empty", text)
	}
}

func TestSyncSandboxClipboardSkipsEmpty(t *testing.T) {
	prev := cdpClient
	cdpClient = nil
	defer func() { cdpClient = prev }()

	wrote := false
	syncSandboxClipboard(func(map[string]any) { wrote = true })
	if wrote {
		t.Fatal("syncSandboxClipboard wrote an empty clipboard")
	}
}
