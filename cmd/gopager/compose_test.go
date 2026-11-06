package main

import (
	"testing"

	"github.com/gopager/gopager/internal/platform/postgres"
)

// mustComposeCore builds the shared store set and services the way run does, once per test.
func mustComposeCore(t *testing.T, store *postgres.Store) *core {
	t.Helper()
	shared, err := composeCore(store)
	if err != nil {
		t.Fatal(err)
	}
	return shared
}
