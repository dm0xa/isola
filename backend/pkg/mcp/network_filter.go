package mcp

import (
	"encoding/json"
	"net/url"
	"strings"
)

type networkEntry struct {
	URL    string `json:"url"`
	Method string `json:"method"`
	Status int    `json:"status"`
	Type   string `json:"type"`
}

func filterNetworkBody(body []byte, entryType, hostContains string, limit int) ([]byte, error) {
	var parsed struct {
		Entries []networkEntry `json:"entries"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body, err
	}
	filtered := make([]networkEntry, 0, len(parsed.Entries))
	for _, e := range parsed.Entries {
		if entryType != "" && !strings.EqualFold(e.Type, entryType) {
			continue
		}
		if hostContains != "" && !hostMatches(e.URL, hostContains) {
			continue
		}
		filtered = append(filtered, e)
		if limit > 0 && len(filtered) >= limit {
			break
		}
	}
	out, err := json.Marshal(map[string]any{"entries": filtered})
	if err != nil {
		return body, err
	}
	return out, nil
}

func hostMatches(rawURL, hostContains string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return strings.Contains(strings.ToLower(rawURL), strings.ToLower(hostContains))
	}
	host := strings.ToLower(u.Host)
	needle := strings.ToLower(strings.TrimSpace(hostContains))
	return strings.Contains(host, needle)
}
