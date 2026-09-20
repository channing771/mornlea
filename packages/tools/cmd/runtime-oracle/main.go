package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	writeInventory := flag.Bool("write-inventory", false, "rewrite testdata/runtime-migration/contracts.json from live registries")
	flag.Parse()

	root, err := RepositoryRoot()
	if err != nil {
		fatal(err)
	}
	families, live, err := Discover(root)
	if err != nil {
		fatal(err)
	}
	inventory := Inventory{
		SchemaVersion: inventorySchemaVersion,
		Identities:    live,
		Families:      families,
	}
	path := filepath.Join(root, filepath.FromSlash(InventoryRelPath))
	if *writeInventory {
		encoded, err := encodeInventory(inventory)
		if err != nil {
			fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("wrote %s (%d families)\n", InventoryRelPath, len(families))
		return
	}

	frozen, err := LoadInventory(path)
	if err != nil {
		fatal(err)
	}
	if err := Reconcile(root, frozen, families, live); err != nil {
		fatal(err)
	}
	fmt.Printf("inventory ok: %d families\n", len(frozen.Families))
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "%v\n", err)
	os.Exit(1)
}
