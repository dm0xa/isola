package main

import (
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	lastInputAt time.Time
	lastInputMu sync.Mutex
	// Start active — the first viewer connects and should see video immediately.
	// Idle mode throttles but never drops frames entirely.
	activityMode   = "active"
	activityModeMu sync.RWMutex
)

func initActivityTracker() {
	go activityLoop()
}

func noteInputActivity(significant bool) {
	lastInputMu.Lock()
	lastInputAt = time.Now()
	lastInputMu.Unlock()

	if significant {
		setActivityMode("active")
	}
}

func lastInputAgeMs() int64 {
	lastInputMu.Lock()
	defer lastInputMu.Unlock()
	if lastInputAt.IsZero() {
		return -1
	}
	return time.Since(lastInputAt).Milliseconds()
}

func getActivityMode() string {
	activityModeMu.RLock()
	defer activityModeMu.RUnlock()
	return activityMode
}

func activityIdleMS() time.Duration {
	if v := os.Getenv("ISOLA_ACTIVITY_IDLE_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return 15 * time.Second
}

func activityLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		age := lastInputAgeMs()
		if age < 0 {
			continue
		}
		if time.Duration(age)*time.Millisecond < activityIdleMS() {
			continue
		}
		if getActivityMode() == "idle" {
			continue
		}
		setActivityMode("idle")
	}
}

func setActivityMode(mode string) {
	activityModeMu.Lock()
	activityMode = mode
	activityModeMu.Unlock()
}
