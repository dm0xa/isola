package main

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestParseEvaluateStringRejectsNonStrings pins the distinction that made a
// healthy renderer look blocked: Runtime.evaluate(`1`) answers
// {"result":{"type":"number","value":1}}, and a probe that promises a string
// must call that out as a bad shape rather than as "no answer" — the renderer
// DID answer, so it is alive and no recovery is warranted.
func TestParseEvaluateStringRejectsNonStrings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		payload string
		want    string
		wantErr bool
	}{
		{name: "string value", payload: `{"result":{"type":"string","value":"1"}}`, want: "1"},
		{name: "empty string value", payload: `{"result":{"type":"string","value":""}}`, want: ""},
		{name: "number value", payload: `{"result":{"type":"number","value":1}}`, wantErr: true},
		{name: "boolean value", payload: `{"result":{"type":"boolean","value":true}}`, wantErr: true},
		{name: "no result field", payload: `{}`, want: ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseEvaluateString(json.RawMessage(tc.payload))
			if tc.wantErr {
				if !errors.Is(err, errBadResultShape) {
					t.Fatalf("parseEvaluateString(%s) error = %v, want errBadResultShape", tc.payload, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseEvaluateString(%s) error = %v, want nil", tc.payload, err)
			}
			if got != tc.want {
				t.Fatalf("parseEvaluateString(%s) = %q, want %q", tc.payload, got, tc.want)
			}
		})
	}
}
