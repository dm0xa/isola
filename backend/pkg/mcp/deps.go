package mcp

import (
	"github.com/dm0xa/isola/backend/pkg/api"
	"github.com/dm0xa/isola/backend/pkg/config"
	"github.com/dm0xa/isola/backend/pkg/store"
)

// Deps wires the MCP server into the existing backend.
type Deps struct {
	API   *api.Handler
	Store *store.Store
	Cache *store.MemCache
	Cfg   *config.Config
}
