// Package plugin defines all interfaces for the modular plugin system.
// Core code depends only on these interfaces, never on concrete plugin implementations.
package plugin

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Info describes a plugin for discovery and UI.
type Info struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Requires    []string `json:"requires"`
}

// Credentials carries login or registration request data.
type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthProvider authenticates and registers users. Multiple providers can be
// registered (e.g. local password auth, SAML, OIDC) and offered based on configuration.
type AuthProvider interface {
	Info() Info
	Name() string
	// Login checks credentials and returns a user identity.
	Login(ctx context.Context, creds Credentials) (*AuthResult, error)
	// Register creates a new user account. Returns an error if unsupported.
	Register(ctx context.Context, creds Credentials) (*AuthResult, error)
}

// SSOProvider is an optional extension of AuthProvider for redirect-based
// single sign-on (OIDC/SAML). Providers implement it alongside AuthProvider;
// the auth dispatch type-asserts and exposes the start/callback routes.
// State is an opaque value minted by the dispatcher and passed back
// unmodified to Callback for nonce/CSRF validation.
type SSOProvider interface {
	AuthProvider
	// BeginAuth returns the IdP redirect URL for the given callback URL.
	BeginAuth(ctx context.Context, callbackURL, state string) (redirectURL string, err error)
	// Callback consumes the IdP response (code etc.) and returns the user.
	Callback(ctx context.Context, r *http.Request, state string) (*AuthResult, error)
}

// AuthResult carries the authenticated user identity.
type AuthResult struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Plan   string `json:"plan"`
	Role   string `json:"role"`
}

// SessionHook receives callbacks at key points in a browser session's lifecycle.
// Hooks are fire-and-forget — errors are logged but do not block the operation.
type SessionHook interface {
	Info() Info
	OnCreate(ctx context.Context, session *SessionContext) error
	OnTerminate(ctx context.Context, session *SessionContext) error
}

// SessionContext carries metadata about an active browser session.
type SessionContext struct {
	SessionID   string
	UserID      string
	ContainerID string
	TargetURL   string
	BrowserType string
	VncPassword string
	Host        string
	Port        int
	CdpPort     int
}

// WebSocketPlugin handles a sub-path WebSocket endpoint under /ws/{sessionId}/.
// e.g. a plugin with SubPath() == "network" serves /ws/{sessionId}/network.
type WebSocketPlugin interface {
	Info() Info
	SubPath() string
	ServeWS(w http.ResponseWriter, r *http.Request, sessionID string)
}

// DLPProvider filters clipboard and keyboard data between client and browser.
type DLPProvider interface {
	Info() Info
	// FilterClipboard filters clipboard data. direction is "in" (client→browser) or "out" (browser→client).
	// Returns the filtered data (or original if allowed) and an error if blocked.
	FilterClipboard(data []byte, direction string) ([]byte, error)
}

// URLPolicy checks whether a URL is permitted for a given session/user.
type URLPolicy interface {
	Info() Info
	AllowURL(ctx context.Context, session *SessionContext, url string) (bool, string)
}

// AuditLogger receives structured audit events for compliance and observability.
type AuditLogger interface {
	Info() Info
	Log(ctx context.Context, event AuditEvent)
}

// AuditEvent is a structured loggable event.
type AuditEvent struct {
	Type      string                 // "session.create", "session.connect", "clipboard.paste", etc.
	SessionID string                 `json:"session_id,omitempty"`
	UserID    string                 `json:"user_id,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// SessionGuard inspects session creation requests before they are processed.
// Used for rate limiting, abuse prevention, and access control.
// Return an error to reject the request; nil to allow.
type SessionGuard interface {
	Info() Info
	// BeforeCreate is called before a new browser session is provisioned.
	// The guard receives the raw HTTP request (for IP, headers), the authenticated
	// user ID (uuid.Nil for anonymous), and any client-supplied fingerprint string.
	BeforeCreate(ctx context.Context, r *http.Request, userID uuid.UUID, fingerprint string) error
}

// FrontendPlugin exposes UI configuration to the frontend plugin registry.
type FrontendPlugin interface {
	Info() Info
	FrontendConfig() FrontendPluginConfig
}

// FrontendPluginConfig describes what the frontend should render for this plugin.
type FrontendPluginConfig struct {
	// Chunk is the URL of the plugin's ESM bundle (served by the module host
	// at /modules/{name}/frontend.js). Empty = no JS; the SPA renders only
	// the declarative pieces below. Present at runtime only for out-of-process
	// modules.
	Chunk         string               `json:"chunk,omitempty"`
	Route         *PluginRouteConfig   `json:"route,omitempty"`
	ToolbarButton *ToolbarButtonConfig `json:"toolbar_button,omitempty"`
	SidePanel     *SidePanelConfig     `json:"side_panel,omitempty"`
}

// PluginRouteConfig adds a route to the SPA.
type PluginRouteConfig struct {
	Path      string `json:"path"`
	Component string `json:"component"` // component ID the frontend resolves
}

// ToolbarButtonConfig adds a button to the viewer toolbar.
type ToolbarButtonConfig struct {
	ID    string `json:"id"`
	Icon  string `json:"icon"`
	Label string `json:"label"`
	Order int    `json:"order"`
}

// SidePanelConfig adds a side panel to the viewer page.
type SidePanelConfig struct {
	ComponentID string `json:"component_id"`
	Title       string `json:"title"`
}
