package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func screenshotHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "png"
	}
	if format != "png" && format != "jpeg" {
		http.Error(w, "format must be png or jpeg", http.StatusBadRequest)
		return
	}
	if cdpClient == nil || !cdpClient.Available() {
		http.Error(w, "cdp not available", http.StatusServiceUnavailable)
		return
	}
	params := map[string]interface{}{
		"format":                format,
		"fromSurface":           true,
		"captureBeyondViewport": false,
	}
	if format == "jpeg" {
		q := 85
		if qs := strings.TrimSpace(r.URL.Query().Get("quality")); qs != "" {
			if n, err := strconv.Atoi(qs); err == nil && n >= 1 && n <= 100 {
				q = n
			}
		}
		params["quality"] = q
	}

	raw, err := cdpClient.callWithResult("Page.captureScreenshot", params, 15*time.Second)
	if err != nil {
		log.Printf("screenshot: captureScreenshot: %v", err)
		http.Error(w, "screenshot failed", http.StatusInternalServerError)
		return
	}
	var parsed struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Data == "" {
		http.Error(w, "screenshot empty", http.StatusInternalServerError)
		return
	}

	width, height := currentCaptureSize()
	mime := "image/png"
	if format == "jpeg" {
		mime = "image/jpeg"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"format":      format,
		"mime":        mime,
		"width":       width,
		"height":      height,
		"data_base64": parsed.Data,
	})
}

func currentCaptureSize() (int, int) {
	currentMu.Lock()
	w, h := currentWidth, currentHeight
	currentMu.Unlock()
	if w > 0 && h > 0 {
		return w, h
	}
	return 1280, 720
}
