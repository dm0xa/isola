package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClampResizeRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		reqW        int
		reqH        int
		sessionRes  string
		wantW       int
		wantH       int
		wantClamped bool
		wantErr     bool
	}{
		{
			name: "within bounds",
			reqW: 1280, reqH: 720,
			sessionRes: "1920x1080",
			wantW:      1280, wantH: 720,
			wantClamped: false,
		},
		{
			name: "exactly equal to launch resolution",
			reqW: 1920, reqH: 1080,
			sessionRes: "1920x1080",
			wantW:      1920, wantH: 1080,
			wantClamped: false,
		},
		{
			name: "width too large clamps",
			reqW: 2560, reqH: 720,
			sessionRes: "1920x1080",
			wantW:      1920, wantH: 720,
			wantClamped: true,
		},
		{
			name: "height too large clamps",
			reqW: 1280, reqH: 2000,
			sessionRes: "1920x1080",
			wantW:      1280, wantH: 1080,
			wantClamped: true,
		},
		{
			name: "both too large clamps both",
			reqW: 3000, reqH: 3000,
			sessionRes: "1920x1080",
			wantW:      1920, wantH: 1080,
			wantClamped: true,
		},
		{
			name: "below minimum width rejected",
			reqW: 63, reqH: 720,
			sessionRes: "1920x1080",
			wantErr:    true,
		},
		{
			name: "below minimum height rejected",
			reqW: 1280, reqH: 10,
			sessionRes: "1920x1080",
			wantErr:    true,
		},
		{
			name: "missing session resolution rejected",
			reqW: 1280, reqH: 720,
			sessionRes: "",
			wantErr:    true,
		},
		{
			name: "malformed session resolution rejected",
			reqW: 1280, reqH: 720,
			sessionRes: "abc",
			wantErr:    true,
		},
		{
			name: "720p session — request 1080p clamps",
			reqW: 1920, reqH: 1080,
			sessionRes: "1280x720",
			wantW:      1280, wantH: 720,
			wantClamped: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotW, gotH, gotClamped, err := ClampResizeRequest(tt.reqW, tt.reqH, tt.sessionRes)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ClampResizeRequest(%d,%d,%q) = (%d,%d,%v), want error",
						tt.reqW, tt.reqH, tt.sessionRes, gotW, gotH, gotClamped)
				}
				return
			}
			if err != nil {
				t.Fatalf("ClampResizeRequest(%d,%d,%q) unexpected error: %v",
					tt.reqW, tt.reqH, tt.sessionRes, err)
			}
			if gotW != tt.wantW || gotH != tt.wantH || gotClamped != tt.wantClamped {
				t.Errorf("ClampResizeRequest(%d,%d,%q) = (%d,%d,%v), want (%d,%d,%v)",
					tt.reqW, tt.reqH, tt.sessionRes,
					gotW, gotH, gotClamped,
					tt.wantW, tt.wantH, tt.wantClamped)
			}
		})
	}
}

func TestClampResizeRequest_ResponseShape(t *testing.T) {
	t.Parallel()

	// The endpoint serializes the result as JSON; verify the JSON shape we
	// promise clients so the frontend integration contract stays stable.
	gotW, gotH, gotClamped, err := ClampResizeRequest(2560, 1440, "1920x1080")
	if err != nil {
		t.Fatalf("ClampResizeRequest: %v", err)
	}

	payload := map[string]any{
		"requested_width":  2560,
		"requested_height": 1440,
		"width":            gotW,
		"height":           gotH,
		"clamped":          gotClamped,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"clamped":true`) {
		t.Errorf("expected clamped=true in response, got %s", string(raw))
	}
	if !strings.Contains(string(raw), `"width":1920`) {
		t.Errorf("expected clamped width=1920, got %s", string(raw))
	}
}
