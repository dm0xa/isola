package main

import "sync"

// Shared configuration updated at startup and on resize.
var (
	currentMu          sync.RWMutex
	currentWidth       int
	currentHeight      int
	currentFPS         int
	currentBitrateKbps int
)
