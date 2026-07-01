package store_test

import (
	"testing"

	"goalstakes/internal/store"
)

// TestMemoryStore runs the full Store contract suite against the in-memory
// implementation. The memory store is used by later phases to unit-test the
// service layer (IF0), so it must satisfy every contract the pgx store does.
func TestMemoryStore(t *testing.T) {
	runStoreSuite(t, func(t *testing.T) (store.Store, func()) {
		return store.NewMemory(), func() {}
	})
}
