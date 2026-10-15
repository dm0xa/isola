package main

import (
	"encoding/json"
	"testing"
)

const cookieSecret = "super-secret-session-token"

func resetCookieCapture() func() {
	cookieMu.Lock()
	prevBuffer := cookieBuffer
	cookieBuffer = nil
	cookieMu.Unlock()
	return func() {
		cookieMu.Lock()
		cookieBuffer = prevBuffer
		cookieMu.Unlock()
	}
}

func cookieSnapshot() []cookieEntry {
	cookieMu.Lock()
	defer cookieMu.Unlock()
	out := make([]cookieEntry, len(cookieBuffer))
	copy(out, cookieBuffer)
	return out
}

func extraInfoWithSetCookie(setCookie string) json.RawMessage {
	payload, err := json.Marshal(map[string]any{
		"requestId": "1",
		"headers": map[string]string{
			"set-cookie": setCookie,
		},
		"statusCode": 200,
	})
	if err != nil {
		panic(err)
	}
	return payload
}

func TestParseSetCookieMetaParsesAttributes(t *testing.T) {
	got, ok := parseSetCookieMeta("sid=" + cookieSecret + "; Domain=.evil.test; Path=/app; Secure; HttpOnly; SameSite=Lax; Max-Age=3600")
	if !ok {
		t.Fatal("expected a parsed cookie")
	}
	if got.Name != "sid" || got.Value != cookieSecret || got.Domain != ".evil.test" || got.Path != "/app" ||
		got.SameSite != "Lax" || got.MaxAge != 3600 || !got.Secure || !got.HTTPOnly {
		t.Fatalf("meta wrong: %+v", got)
	}
}

func TestCookieTapFromSetCookieHeader(t *testing.T) {
	defer resetCookieCapture()()

	cookieTap(extraInfoWithSetCookie("sid=" + cookieSecret + "; Domain=.evil.test; Secure; HttpOnly"))

	got := cookieSnapshot()
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].Name != "sid" || got[0].Value != cookieSecret || got[0].Domain != ".evil.test" || !got[0].Secure || !got[0].HTTPOnly {
		t.Fatalf("entry wrong: %+v", got[0])
	}
}

func TestCookieTapSplitsMultipleSetCookie(t *testing.T) {
	defer resetCookieCapture()()

	cookieTap(extraInfoWithSetCookie("a=" + cookieSecret + "; Domain=a.test\nb=ok; Domain=b.test; Secure"))

	got := cookieSnapshot()
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
}

func TestCookieTapFromHeadersTextAndBlockedLine(t *testing.T) {
	defer resetCookieCapture()()

	payload, err := json.Marshal(map[string]any{
		"headersText": "HTTP/1.1 200 OK\r\nSet-Cookie: fromtext=" + cookieSecret + "; Domain=text.test; HttpOnly\r\nContent-Type: text/html\r\n\r\n",
		"blockedCookies": []map[string]string{
			{"cookieLine": "blocked=" + cookieSecret + "; Domain=blocked.test; Secure"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cookieTap(payload)

	got := cookieSnapshot()
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}
	names := map[string]cookieEntry{}
	for _, e := range got {
		names[e.Name] = e
	}
	if names["fromtext"].Domain != "text.test" || !names["fromtext"].HTTPOnly {
		t.Fatalf("headersText cookie wrong: %+v", names["fromtext"])
	}
	if names["blocked"].Domain != "blocked.test" || !names["blocked"].Secure {
		t.Fatalf("blocked cookie wrong: %+v", names["blocked"])
	}
}

func TestCookieTapUpdatesSameDomainAndName(t *testing.T) {
	defer resetCookieCapture()()

	cookieTap(extraInfoWithSetCookie("sid=one; Domain=.evil.test"))
	cookieTap(extraInfoWithSetCookie("sid=two; Domain=.evil.test; Secure"))
	cookieTap(extraInfoWithSetCookie("sid=three; Domain=other.test"))

	got := cookieSnapshot()
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2 (same domain+name updated)", len(got))
	}
	evil := cookieEntry{}
	other := cookieEntry{}
	for _, e := range got {
		if e.Domain == ".evil.test" {
			evil = e
		}
		if e.Domain == "other.test" {
			other = e
		}
	}
	if evil.Value != "two" || !evil.Secure || evil.Name != "sid" {
		t.Fatalf(".evil.test sid should be updated: %+v", evil)
	}
	if other.Value != "three" || other.Name != "sid" {
		t.Fatalf("other.test sid wrong: %+v", other)
	}
}

func TestCookieTapBroadcastIncludesValue(t *testing.T) {
	defer resetCookieCapture()()

	var msgs []map[string]any
	unreg := registerTabListener(func(msg map[string]any) {
		if msg["type"] == "cookie" {
			cp := map[string]any{}
			for k, v := range msg {
				cp[k] = v
			}
			msgs = append(msgs, cp)
		}
	})
	defer unreg()

	cookieTap(extraInfoWithSetCookie("sid=" + cookieSecret + "; Domain=.evil.test; Secure"))
	if len(msgs) != 1 {
		t.Fatalf("got %d broadcasts, want 1", len(msgs))
	}
	if msgs[0]["n"] != "sid" || msgs[0]["d"] != ".evil.test" || msgs[0]["s"] != true {
		t.Fatalf("broadcast wrong: %+v", msgs[0])
	}
	if msgs[0]["v"] != cookieSecret {
		t.Fatalf("broadcast should include value: %+v", msgs[0])
	}

	var replayed []map[string]any
	replayCookiesTo(func(msg map[string]any) { replayed = append(replayed, msg) })
	if len(replayed) != 1 || replayed[0]["v"] != cookieSecret {
		t.Fatalf("replay wrong: %+v", replayed)
	}
}

func TestAssociatedCookieValueIsIgnored(t *testing.T) {
	defer resetCookieCapture()()

	payload, err := json.Marshal(map[string]any{
		"associatedCookies": []map[string]any{
			{"cookie": map[string]any{
				"name": "sid", "value": cookieSecret, "domain": ".evil.test",
				"secure": true, "httpOnly": true,
			}},
		},
		"headers": map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	cookieTap(payload)
	if got := cookieSnapshot(); len(got) != 0 {
		t.Fatalf("associatedCookies must not be ingested (they carry values): %+v", got)
	}
}

// ExtraInfo is a Network.* event. Routing must still reach the cookie tap;
// a prefix match on Network. used to swallow it entirely.
func TestRouteCaptureEventFeedsCookieTapFromExtraInfo(t *testing.T) {
	defer resetCookieCapture()()
	defer resetNetCapture()()

	routeCaptureEvent(
		"Network.responseReceivedExtraInfo",
		extraInfoWithSetCookie("sid="+cookieSecret+"; Domain=.evil.test; HttpOnly"),
		"target-aaaa",
		"sess-1",
		1,
	)

	got := cookieSnapshot()
	if len(got) != 1 || got[0].Name != "sid" || !got[0].HTTPOnly {
		t.Fatalf("ExtraInfo did not reach cookie tap: %+v", got)
	}
	netMu.Lock()
	n := len(netRequests)
	netMu.Unlock()
	if n != 0 {
		t.Fatalf("ExtraInfo should not create a network row, got %d", n)
	}
}
