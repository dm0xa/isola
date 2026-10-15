package main

import (
	"encoding/json"
	"testing"
)

// resetConsoleCapture snapshots and clears the console buffer and dedup set so a
// test starts clean and does not leak state into its siblings (mirrors
// resetNetCapture in cdp_capture_test.go).
func resetConsoleCapture() func() {
	consoleMu.Lock()
	prevBuffer, prevKeys := consoleBuffer, consoleSentKeys
	consoleBuffer, consoleSentKeys = nil, map[string]bool{}
	consoleMu.Unlock()
	return func() {
		consoleMu.Lock()
		consoleBuffer, consoleSentKeys = prevBuffer, prevKeys
		consoleMu.Unlock()
	}
}

func consoleEntriesSnapshot() []consoleEntry {
	consoleMu.Lock()
	defer consoleMu.Unlock()
	out := make([]consoleEntry, len(consoleBuffer))
	copy(out, consoleBuffer)
	return out
}

// The console panel must relay a page's own console output — not only Electron's
// built-in security warning — with level, text and tab attribution intact. This
// is the ordinary path the viewer depends on when the panel otherwise looks
// empty because a page simply has not logged anything yet.
func TestConsoleTapCapturesPageConsole(t *testing.T) {
	defer resetConsoleCapture()()

	// Args arrive as raw JSON, so string values keep their surrounding quotes.
	// This matches what the viewer renders for e.g. console.log('x', 42).
	consoleTap("Runtime.consoleAPICalled",
		json.RawMessage(`{"type":"log","args":[{"value":"hello"},{"value":42}]}`), 2)

	got := consoleEntriesSnapshot()
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].Level != "log" || got[0].Text != `"hello" 42` || got[0].Tab != 2 {
		t.Fatalf("unexpected entry: %+v", got[0])
	}
	if got[0].TS == 0 {
		t.Fatalf("entry has no capture timestamp: %+v", got[0])
	}
}

// A console event with no explicit type still lands as "log", and thrown errors
// are captured as "exception" from the exception description.
func TestConsoleTapDefaultsLevelAndHandlesExceptions(t *testing.T) {
	defer resetConsoleCapture()()

	consoleTap("Runtime.consoleAPICalled", json.RawMessage(`{"args":[{"value":"no type"}]}`), 1)
	consoleTap("Runtime.exceptionThrown",
		json.RawMessage(`{"exceptionDetails":{"text":"Uncaught","description":"TypeError: boom"}}`), 1)

	got := consoleEntriesSnapshot()
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].Level != "log" {
		t.Fatalf("missing type should default to log, got %q", got[0].Level)
	}
	if got[1].Level != "exception" || got[1].Text != "TypeError: boom" {
		t.Fatalf("exception entry wrong: %+v", got[1])
	}
}

// Repeat messages are collapsed (the tap dedups on level+text+tab), but the same
// text on a different tab is a distinct entry — so one chatty tab cannot mask
// another tab's output in the per-tab filter.
func TestConsoleTapDedupsPerTab(t *testing.T) {
	defer resetConsoleCapture()()

	params := json.RawMessage(`{"type":"warning","args":[{"value":"same"}]}`)
	consoleTap("Runtime.consoleAPICalled", params, 1)
	consoleTap("Runtime.consoleAPICalled", params, 1) // repeat on same tab → dropped
	consoleTap("Runtime.consoleAPICalled", params, 2) // other tab → kept

	got := consoleEntriesSnapshot()
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2 (same-tab repeat deduped, other tab kept)", len(got))
	}
	if got[0].Tab != 1 || got[1].Tab != 2 {
		t.Fatalf("tab attribution wrong: %+v", got)
	}
}

// consoleTap must ignore CDP methods it does not own rather than buffering junk.
func TestConsoleTapIgnoresUnrelatedMethods(t *testing.T) {
	defer resetConsoleCapture()()

	consoleTap("Network.requestWillBeSent", json.RawMessage(`{"requestId":"1"}`), 1)
	if got := consoleEntriesSnapshot(); len(got) != 0 {
		t.Fatalf("non-console method produced %d entries", len(got))
	}
}
