package store

// Store aggregates the data stores used by the backend.
type Store struct {
	Sessions SessionStoreInterface
}
