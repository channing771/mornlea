package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

// newFlagSet builds the runtime oracle CLI flag set.
//
// The CLI deliberately registers no flags. The frozen manifest at
// InventoryRelPath is the corpus's single source of contract coverage, and no
// command may rewrite it: a regeneration entry point can only return together
// with extended case discovery and a fail-closed case-loss check, so manifest
// merges stay a controller-side manual step instead of a CLI hazard that could
// replace the frozen corpus with a case-less one while reporting success.
func newFlagSet() *flag.FlagSet {
	return flag.NewFlagSet("runtime-oracle", flag.ExitOnError)
}

// run parses args and reconciles the frozen inventory against the live
// registries. Reconciliation and validation is the CLI's only behavior; see
// newFlagSet for why no flag can rewrite the corpus.
func run(args []string, stdout io.Writer) error {
	flags := newFlagSet()
	if err := flags.Parse(args); err != nil {
		return err
	}

	root, err := RepositoryRoot()
	if err != nil {
		return err
	}
	families, live, err := Discover(root)
	if err != nil {
		return err
	}
	frozen, err := LoadInventory(filepath.Join(root, filepath.FromSlash(InventoryRelPath)))
	if err != nil {
		return err
	}
	if err := Reconcile(root, frozen, families, live); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "inventory ok: %d families\n", len(frozen.Families))
	return nil
}
