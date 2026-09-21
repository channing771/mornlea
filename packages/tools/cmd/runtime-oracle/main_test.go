package main

import (
	"flag"
	"sort"
	"strings"
	"testing"
)

// TestCLIFlagSetCannotRewriteTheFrozenCorpus pins that the runtime oracle CLI
// registers no flag which rewrites testdata/runtime-migration/contracts.json.
// The removed -write-inventory flag rebuilt the manifest from Discover alone,
// dropping every corpus case while still reporting success, so the CLI must
// never carry a manifest-rewriting entry point: a regeneration entry point may
// only return together with extended case discovery and a fail-closed
// case-loss check, which keeps manifest merges a controller-side manual step.
func TestCLIFlagSetCannotRewriteTheFrozenCorpus(t *testing.T) {
	var registered []string
	newFlagSet().VisitAll(func(f *flag.Flag) {
		registered = append(registered, f.Name)
	})
	sort.Strings(registered)
	// The exact expected flag-name set is empty: the oracle CLI takes no
	// options, so any re-introduced flag fails here.
	if len(registered) != 0 {
		t.Fatalf("runtime oracle CLI must register no flags, got %v", registered)
	}
}

// TestTestBinaryFlagsCannotRewriteTheFrozenCorpus pins that the test binary's
// default flag set registers no flag which rewrites the frozen corpus at
// InventoryRelPath. A package-level flag.Bool binds to that set at init, so the
// removed -update-runtime-inventory flag was reachable through `go test
// -update-runtime-inventory` and rewrote the manifest from DiscoverCases, a
// stub returning a single case, replacing the frozen 454-case corpus while the
// suite still reported success. The newFlagSet check above cannot observe
// package-level flags, so this assertion guards the default flag set directly;
// see newFlagSet for why no corpus-rewriting entry point may exist.
func TestTestBinaryFlagsCannotRewriteTheFrozenCorpus(t *testing.T) {
	if f := flag.Lookup("update-runtime-inventory"); f != nil {
		t.Fatalf("test binary must register no corpus-rewriting flag, got -%s", f.Name)
	}
}

// TestRunReconcilesTheFrozenInventoryAgainstLiveRegistries keeps the CLI's only
// behavior green: reconcile and validate the frozen corpus against the live
// registries discovered from the real repository root.
func TestRunReconcilesTheFrozenInventoryAgainstLiveRegistries(t *testing.T) {
	root := mustRepoRoot(t)
	var stdout strings.Builder
	if err := run(nil, &stdout); err != nil {
		t.Fatalf("run against repository root %s: %v", root, err)
	}
	if got := stdout.String(); !strings.HasPrefix(got, "inventory ok: ") {
		t.Fatalf("unexpected CLI output %q", got)
	}
}
