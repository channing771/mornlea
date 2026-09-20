package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// InventoryRelPath is the repository-relative contract inventory freeze.
const InventoryRelPath = "testdata/runtime-migration/contracts.json"

// RepositoryRoot walks from the working directory to the go.work that
// identifies the Mornlea repository. The oracle never assumes a fixed
// relative depth so tests and the command binary share one lookup.
func RepositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("runtime-oracle: repository root not found from %s", dir)
		}
		dir = parent
	}
}
