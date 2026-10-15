package modulesdk

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// contextT aliases the context type used by plugin interface methods.
type contextT = context.Context

// decodeB64/encodeB64 wrap the contract's base64 clipboard transport.
func decodeB64(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
func encodeB64(b []byte) string          { return base64.StdEncoding.EncodeToString(b) }

func writeOK(w http.ResponseWriter, v interface{}) { writeJSONStatus(w, http.StatusOK, v) }

func writeJSONStatus(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, reason string) {
	writeJSONStatus(w, status, ErrorBody{Reason: reason})
}

// userIDOrNil parses a user id, mapping empty or malformed values to uuid.Nil
// so anonymous traffic behaves exactly as it does for in-process guards.
func userIDOrNil(id string) uuid.UUID {
	if id == "" {
		return uuid.Nil
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil
	}
	return u
}

// recreateRequest rebuilds a minimal *http.Request from the relayed
// GuardRequest so guard plugins can read method/path/remote address/headers
// exactly like their in-process counterparts.
func recreateRequest(orig *http.Request, gr GuardRequest) *http.Request {
	r, err := http.NewRequest(gr.Method, "http://isola-module"+gr.Path, nil)
	if err != nil {
		return orig
	}
	for k, v := range gr.Headers {
		r.Header.Set(k, v)
	}
	r.RemoteAddr = gr.RemoteAddr
	return r
}

// FlattenHeaders folds the forwarded header multi-map into the wire map
// (last value wins — guards only need identity-relevant headers). Shared by
// the host's guard adapter and the SDK's request re-creator.
func FlattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		if len(vs) > 0 {
			out[k] = vs[len(vs)-1]
		}
	}
	return out
}
