// Package modulehost discovers, launches, supervises, and adapts out-of-process
// isola modules: standalone binaries speaking contract v1 (JSON over a Unix
// domain socket) that are bridged into the in-process plugin registry.
//
// The host is strictly opt-in: with ISOLA_MODULES_DIR unset nothing starts, no
// sockets open, and the binary behaves exactly like a build without this
// package. Deployments that want first-party modules set the env var.
package modulehost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manifest is the plugin.json describing an installed module package.
type Manifest struct {
	Module      string       `json:"module"`
	Version     string       `json:"version"`
	API         string       `json:"api"`
	Description string       `json:"description,omitempty"`
	Icon        string       `json:"icon,omitempty"`
	Feature     string       `json:"feature,omitempty"` // license feature id (informational here)
	SubPath     string       `json:"subpath,omitempty"` // websocket subpath override (else from /module/info)
	Settings    []any        `json:"settings,omitempty"`
	Backend     BackendSpec  `json:"backend"`
	Frontend    FrontendSpec `json:"frontend,omitempty"`
	Requires    RequiresSpec `json:"requires,omitempty"`
	Restart     bool         `json:"requires_restart"` // true = setting changes need a module restart
}

// BackendSpec points at the module binary.
type BackendSpec struct {
	Bin        string   `json:"bin"`                  // executable path relative to the package dir
	Implements []string `json:"implements,omitempty"` // optional static hint; /module/info is authoritative
}

// FrontendSpec describes the UI chunk and declarative UI surface.
type FrontendSpec struct {
	Entry         string       `json:"entry,omitempty"` // JS chunk path relative to the package dir
	Route         *UIRoute     `json:"route,omitempty"`
	ToolbarButton *UIToolbar   `json:"toolbar_button,omitempty"`
	SidePanel     *UISidePanel `json:"side_panel,omitempty"`
}

// RequiresSpec declares external dependencies the host injects via env.
type RequiresSpec struct {
	DB    bool `json:"db"`    // inject ISOLA_DB_DSN
	Redis bool `json:"redis"` // inject ISOLA_REDIS_URL
}

// UIRoute mirrors plugin.PluginRouteConfig for the manifest.
type UIRoute struct {
	Path      string `json:"path"`
	Component string `json:"component"`
}

// UIToolbar mirrors plugin.ToolbarButtonConfig.
type UIToolbar struct {
	ID    string `json:"id"`
	Icon  string `json:"icon"`
	Label string `json:"label"`
	Order int    `json:"order"`
}

// UISidePanel mirrors plugin.SidePanelConfig.
type UISidePanel struct {
	ComponentID string `json:"component_id"`
	Title       string `json:"title"`
}

// disabledMarker is the file that turns a module off without deleting it.
// The administrator toggles enable/disable by creating/removing it; changes take
// effect on backend restart (or module restart for requires_restart modules).
const disabledMarker = ".disabled"

// Discover scans dir for module packages: one subdirectory each containing a
// readable plugin.json with a module name. Directories carrying a .disabled
// marker are returned with Enabled=false. Malformed packages are skipped with
// a logged warning.
func Discover(dir string) (found []*Discovered, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("modulehost: read modules dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		pkgDir := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(filepath.Join(pkgDir, "plugin.json"))
		if err != nil {
			continue // not a module package
		}
		var m Manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			fmt.Fprintf(os.Stderr, "modulehost: skipping %s: bad plugin.json: %v\n", e.Name(), err)
			continue
		}
		if m.Module == "" {
			m.Module = e.Name()
		}
		if m.Backend.Bin == "" {
			fmt.Fprintf(os.Stderr, "modulehost: skipping %s: plugin.json has no backend.bin\n", e.Name())
			continue
		}
		_, statErr := os.Stat(filepath.Join(pkgDir, m.Backend.Bin))
		found = append(found, &Discovered{
			Manifest: m,
			Dir:      pkgDir,
			Enabled:  !fileExists(filepath.Join(pkgDir, disabledMarker)),
			BinFound: statErr == nil,
		})
	}
	return found, nil
}

// Discovered is one module package found on disk.
type Discovered struct {
	Manifest Manifest
	Dir      string
	Enabled  bool
	BinFound bool
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
