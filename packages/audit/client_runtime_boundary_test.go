package archcheck_test

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// This file guards the portable client semantics packages `packages/client/runtime`
// and `packages/client/presentation`. They own mirror adoption, prediction
// stepping, and presentation value shaping for every client host, so they must
// stay free of six dependency families, each with a distinct delimitation:
//
//   - Authoritative server: no import of any `github.com/channing771/mornlea/packages/server`
//     package and no such package in the `go list -deps` closure. The closure
//     check also catches the leak when an otherwise allowed family package
//     (mesh, assets, shared) starts re-exporting server symbols.
//   - Godot: no import path containing "godot", no source reference to the
//     "mornlea-godot" application tree, and no godot package in the closure.
//     No Go godot binding exists yet; the guard future-proofs the migration.
//   - Client C ABI: the ABI has no separable import path (its Go wrappers live
//     in `packages/client/client` next to the mirrors runtime legitimately
//     composes), so the boundary is expressed at source level. The ABI entry
//     files are discovered as the production files of `packages/client/client`
//     that import "C" (today window.go, render.go, and camera_native.go, all
//     darwin-gated cgo). Their exported top-level declarations form the ABI
//     wrapper surface; guarded packages must not reference any of those
//     symbols through the client package qualifier, must not dot-import the
//     client package, must not use cgo themselves, and must not contain the
//     raw "mornlea_client" token family (repo-wide token ownership is
//     additionally pinned by TestNativeEngineBridgeBoundary).
//   - Darwin and platform gating: guarded files must form one
//     platform-independent file set. Every production file must satisfy
//     build.Context.MatchFile for GOOS darwin, linux, and windows, which
//     rejects per-platform build tags and filename suffixes alike; platform
//     syscalls via "syscall" or "golang.org/x/sys" are part of the OS import
//     family below.
//   - WebGPU and GPU/window bindings: guarded by the same substring matcher
//     as TestNoPackageImportsWebGPU ("webgpu"), extended with "vulkan" and
//     the glfw binding prefix, both for direct imports and the closure.
//   - Device I/O: guarded packages own semantics, not devices. The forbidden
//     import families are window/GL bindings, GPU API bindings, audio device
//     backends, HID/USB device access, and the OS file/process/syscall layer
//     ("os", "os/exec", "syscall", "golang.org/x/sys" prefixes), which
//     delimits file and device opens without a call-site scanner. Pure math
//     libraries such as the mathgl vector package stay explicitly allowed.
//
// The checker is a pure function over synthetic inputs so every prohibition
// has a mutation probe in TestClientRuntimeBoundaryGuardDetectsDrift; the
// real-tree assertion lives in TestClientRuntimeBoundaryProhibitions.

const (
	clientRuntimeUnitImportPrefix = "github.com/channing771/mornlea/packages/"
	clientRuntimeClientPackage    = clientRuntimeUnitImportPrefix + "client/client"
	clientRuntimeServerPrefix     = clientRuntimeUnitImportPrefix + "server"
	clientRuntimeGlfwPrefix       = "github.com/go-gl/glfw"
)

// clientRuntimeGuardedPackages lists the guarded semantics packages as
// repository-relative import paths.
var clientRuntimeGuardedPackages = []string{"packages/client/runtime", "packages/client/presentation"}

// clientRuntimeGuardedGOARCH lists the architectures paired with every
// guarded GOOS so arch-gated forks and architecture filename suffixes are
// rejected on every host instead of only on a mismatched machine.
var clientRuntimeGuardedGOARCH = []string{"amd64", "arm64"}

// clientRuntimeGuardedGOOS lists the platforms every guarded production file
// must build for. darwin, linux, and windows together reject any GOOS tag,
// filename suffix, or negated constraint that forks the file set.
var clientRuntimeGuardedGOOS = []string{"darwin", "linux", "windows"}

// clientRuntimeForbiddenImport is one forbidden import family. sample is a
// representative violating import path used by the drift probe so a family
// entry can never silently stop matching anything.
type clientRuntimeForbiddenImport struct {
	family  string
	sample  string
	matches func(importPath string) bool
}

// clientRuntimeForbiddenImports lists every forbidden import family for the
// guarded packages. Entries stay conservative and name-based: prefixes pin a
// known binding module, substrings pin a binding family, and the OS entry
// delimits file and device I/O at the package level.
var clientRuntimeForbiddenImports = []clientRuntimeForbiddenImport{
	{
		family: "authoritative server",
		sample: "github.com/channing771/mornlea/packages/server/sim/runtime",
		matches: func(importPath string) bool {
			return strings.HasPrefix(importPath, clientRuntimeServerPrefix+"/") || importPath == clientRuntimeServerPrefix
		},
	},
	{
		family: "Godot",
		sample: "github.com/godot-go/godot-go/gd",
		matches: func(importPath string) bool {
			return strings.Contains(importPath, "godot")
		},
	},
	{
		family:  "cgo FFI",
		sample:  "C",
		matches: func(importPath string) bool { return importPath == "C" },
	},
	{
		family:  "window or GL binding",
		sample:  "github.com/go-gl/glfw/v3.3/glfw",
		matches: func(importPath string) bool { return strings.HasPrefix(importPath, clientRuntimeGlfwPrefix) },
	},
	{
		family: "GPU API binding",
		sample: "github.com/example/experimental-webgpu-go",
		matches: func(importPath string) bool {
			return strings.Contains(importPath, "webgpu") || strings.Contains(importPath, "vulkan")
		},
	},
	{
		family: "audio device backend",
		sample: "github.com/hajimehoshi/oto/v3",
		matches: func(importPath string) bool {
			for _, prefix := range []string{"github.com/hajimehoshi/oto", "github.com/faiface/beep", "github.com/gen2brain/malgo", "github.com/gordonklaus/portaudio"} {
				if strings.HasPrefix(importPath, prefix) {
					return true
				}
			}
			return false
		},
	},
	{
		family: "HID or USB device access",
		sample: "github.com/google/gousb",
		matches: func(importPath string) bool {
			for _, prefix := range []string{"github.com/sstallion/go-hid", "github.com/karalabe/usb", "github.com/google/gousb"} {
				if strings.HasPrefix(importPath, prefix) {
					return true
				}
			}
			return false
		},
	},
	{
		family: "OS file, process, or syscall layer",
		sample: "os",
		matches: func(importPath string) bool {
			// The whole os/ subtree joins os/exec and syscall: os/signal and
			// os/user are kernel/device-adjacent surfaces the same way.
			return importPath == "os" || strings.HasPrefix(importPath, "os/") ||
				importPath == "syscall" || strings.HasPrefix(importPath, "golang.org/x/sys")
		},
	},
}

// clientRuntimeBoundaryViolations checks the guarded-package contract against
// decoupled inputs: sources maps repository-relative production file paths to
// source text, abiSurface is the exported declaration set of the client C ABI
// entry files, and deps maps each guarded package to its `go list -deps`
// closure. Empty return means the contract holds. Unparsable files count as
// violations so the guard fails closed.
func clientRuntimeBoundaryViolations(sources map[string]string, abiSurface map[string]bool, deps map[string][]string) []string {
	var violations []string
	for _, path := range slices.Sorted(maps.Keys(sources)) {
		source := sources[path]
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, path, source, 0)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s fails to parse and is treated as a violation: %v", path, err))
			continue
		}
		importNames := make(map[string]string, len(parsed.Imports))
		for _, specification := range parsed.Imports {
			importPath, err := strconv.Unquote(specification.Path.Value)
			if err != nil {
				violations = append(violations, fmt.Sprintf("%s has an unparsable import path: %v", path, err))
				continue
			}
			// Family prohibitions apply to every import shape, including blank
			// imports: they still add the module-graph edge and link the
			// foreign code into the guarded closure.
			for _, forbidden := range clientRuntimeForbiddenImports {
				if forbidden.matches(importPath) {
					violations = append(violations, fmt.Sprintf(
						"%s imports %s from the forbidden %s family; guarded client semantics packages must stay platform-independent and device-free",
						path, importPath, forbidden.family))
				}
			}
			name := filepath.Base(importPath)
			if specification.Name != nil {
				name = specification.Name.Name
			}
			if name == "_" {
				continue
			}
			if name == "." {
				if importPath == clientRuntimeClientPackage {
					violations = append(violations, fmt.Sprintf(
						"%s must not dot-import %s: the client C ABI wrapper symbols would become referenceable without a package qualifier",
						path, importPath))
				}
				continue
			}
			importNames[name] = importPath
		}
		if strings.Contains(source, "mornlea_client") {
			violations = append(violations, fmt.Sprintf(
				"%s contains a raw client C ABI token; only the ABI owner package in packages/client/client may touch the mornlea_client ABI", path))
		}
		if strings.Contains(source, "mornlea-godot") {
			violations = append(violations, fmt.Sprintf(
				"%s references the mornlea-godot application tree; guarded client semantics packages must stay Godot-free", path))
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			importPath, imported := importNames[identifier.Name]
			if !imported || importPath != clientRuntimeClientPackage || !abiSurface[selector.Sel.Name] {
				return true
			}
			violations = append(violations, fmt.Sprintf(
				"%s references client C ABI symbol %s at %s; the ABI wrapper surface is owned by the cgo entry files of packages/client/client and must not leak into semantics packages",
				path, selector.Sel.Name, fileSet.Position(selector.Pos())))
			return true
		})
	}
	for _, pkg := range slices.Sorted(maps.Keys(deps)) {
		for _, dependency := range deps[pkg] {
			if dependency == clientRuntimeServerPrefix || strings.HasPrefix(dependency, clientRuntimeServerPrefix+"/") {
				violations = append(violations, fmt.Sprintf(
					"%s transitively depends on the authoritative server package %s; a dependency family package has leaked server symbols into the client semantics closure",
					pkg, dependency))
			}
			if strings.Contains(dependency, "godot") {
				violations = append(violations, fmt.Sprintf(
					"%s transitively depends on the Godot package %s", pkg, dependency))
			}
			if strings.Contains(dependency, "webgpu") || strings.Contains(dependency, "vulkan") || strings.HasPrefix(dependency, clientRuntimeGlfwPrefix) {
				violations = append(violations, fmt.Sprintf(
					"%s transitively depends on the GPU or window binding %s", pkg, dependency))
			}
		}
	}
	return violations
}

// clientRuntimeABISurface discovers the Go-visible client C ABI surface: the
// exported top-level declarations of every production file in
// `packages/client/client` that imports "C". Discovery is dynamic so new cgo
// entry files join the surface automatically, and anchor symbols fail loudly
// if the ABI entry files are renamed or moved.
func clientRuntimeABISurface(t *testing.T, root string) map[string]bool {
	t.Helper()
	abiOwner := filepath.Join(root, "packages", "client", "client")
	surface := make(map[string]bool)
	entryFiles := 0
	for _, path := range goFiles(t, abiOwner) {
		importsOnly, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s imports: %v", path, err)
		}
		hasCgo := false
		for _, imported := range importsOnly.Imports {
			if strings.Trim(imported.Path.Value, `"`) == "C" {
				hasCgo = true
				break
			}
		}
		if !hasCgo {
			continue
		}
		entryFiles++
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range parsed.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if declaration.Name.IsExported() {
					surface[declaration.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, specification := range declaration.Specs {
					switch specification := specification.(type) {
					case *ast.TypeSpec:
						if specification.Name.IsExported() {
							surface[specification.Name.Name] = true
						}
					case *ast.ValueSpec:
						for _, name := range specification.Names {
							if name.IsExported() {
								surface[name.Name] = true
							}
						}
					}
				}
			}
		}
	}
	if entryFiles == 0 {
		t.Fatalf("no cgo entry files found under %s: the client C ABI surface discovery must not silently degrade to an empty set", abiOwner)
	}
	for _, anchor := range []string{"ClientABIVersion", "NewWindow", "NativeViewProj", "Renderer", "RenderFrame", "EncodeRenderFrame"} {
		if !surface[anchor] {
			t.Fatalf("expected the client C ABI surface to still declare %s; the cgo entry files of packages/client/client changed shape and this guard must be re-derived", anchor)
		}
	}
	return surface
}

// clientRuntimeGuardedSources collects the production Go sources of the
// guarded packages as repository-relative paths, including future
// subpackages. A guarded package without production files fails the test
// instead of degrading to an empty scan.
func clientRuntimeGuardedSources(t *testing.T, root string) map[string]string {
	t.Helper()
	sources := make(map[string]string)
	for _, pkg := range clientRuntimeGuardedPackages {
		directory := filepath.Join(root, filepath.FromSlash(pkg))
		produced := 0
		err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			sources[filepath.ToSlash(relative)] = string(data)
			produced++
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", pkg, err)
		}
		if produced == 0 {
			t.Fatalf("%s contains no production Go files; the boundary guard must not silently degrade to an empty scan", pkg)
		}
	}
	return sources
}

// clientRuntimeGuardedDeps resolves each guarded package's full dependency
// closure with `go list -deps` from the client module, so the server, Godot,
// and GPU-binding prohibitions hold on the module graph and not only on
// source text.
func clientRuntimeGuardedDeps(t *testing.T, root string) map[string][]string {
	t.Helper()
	deps := make(map[string][]string)
	for _, pkg := range clientRuntimeGuardedPackages {
		relative := strings.TrimPrefix(pkg, "packages/client/")
		command := exec.Command("go", "list", "-deps", "./"+relative)
		command.Dir = filepath.Join(root, "packages", "client")
		output, err := command.Output()
		if err != nil {
			t.Fatalf("go list -deps %s: %v", pkg, err)
		}
		deps[pkg] = strings.Fields(string(output))
	}
	return deps
}

// clientRuntimePlatformGatingViolations asserts that every production Go file
// of the directory participates in the darwin, linux, and windows builds.
// build.Context.MatchFile evaluates both build tags and filename suffixes, so
// a per-platform fork in either shape is rejected while a genuinely
// platform-independent file set passes on every host.
func clientRuntimePlatformGatingViolations(t *testing.T, directory string) []string {
	t.Helper()
	var violations []string
	checked := 0
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		checked++
		parent := filepath.Dir(path)
		relative, relErr := filepath.Rel(directory, path)
		if relErr != nil {
			relative = name
		}
		for _, goos := range clientRuntimeGuardedGOOS {
			for _, goarch := range clientRuntimeGuardedGOARCH {
				context := build.Default
				context.GOOS = goos
				context.GOARCH = goarch
				context.CgoEnabled = true
				matched, err := context.MatchFile(parent, name)
				if err != nil {
					violations = append(violations, fmt.Sprintf("%s cannot be evaluated for GOOS=%s GOARCH=%s: %v", relative, goos, goarch, err))
					continue
				}
				if !matched {
					violations = append(violations, fmt.Sprintf(
						"%s is excluded from the GOOS=%s GOARCH=%s build; guarded client semantics packages must keep one platform-independent file set with no per-platform build tags or filename suffixes",
						relative, goos, goarch))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", directory, err)
	}
	if checked == 0 {
		t.Fatalf("%s contains no production Go files; the platform guard must not silently degrade to an empty scan", directory)
	}
	return violations
}

// TestClientRuntimeBoundaryProhibitions feeds the real source tree, the
// discovered client C ABI surface, and the real dependency closures into the
// boundary checker, and additionally probes platform independence of the
// guarded file sets.
func TestClientRuntimeBoundaryProhibitions(t *testing.T) {
	root := repositoryRoot(t)
	abiSurface := clientRuntimeABISurface(t, root)
	sources := clientRuntimeGuardedSources(t, root)
	deps := clientRuntimeGuardedDeps(t, root)
	if violations := clientRuntimeBoundaryViolations(sources, abiSurface, deps); len(violations) > 0 {
		t.Errorf("client runtime/presentation boundary violated the contract, %d violations:\n%s", len(violations), strings.Join(violations, "\n"))
	}
	for _, pkg := range clientRuntimeGuardedPackages {
		if violations := clientRuntimePlatformGatingViolations(t, filepath.Join(root, filepath.FromSlash(pkg))); len(violations) > 0 {
			t.Errorf("%s is not platform-independent, %d violations:\n%s", pkg, len(violations), strings.Join(violations, "\n"))
		}
	}
}

// TestClientRuntimeBoundaryGuardDetectsDrift proves with synthetic inputs
// that every prohibition of the checker actually rejects: a guard logic that
// stopped matching anything would otherwise stay silently green on a
// conforming tree.
func TestClientRuntimeBoundaryGuardDetectsDrift(t *testing.T) {
	contractSurface := map[string]bool{"NativeViewProj": true, "NewWindow": true, "Renderer": true, "RenderFrame": true}
	contractDeps := func() map[string][]string {
		return map[string][]string{
			"packages/client/runtime": {
				"github.com/channing771/mornlea/packages/client/assets",
				"github.com/channing771/mornlea/packages/client/client",
				"github.com/channing771/mornlea/packages/client/mesh",
				"github.com/channing771/mornlea/packages/client/presentation",
				"github.com/channing771/mornlea/packages/shared/core",
				"github.com/channing771/mornlea/packages/shared/network",
				"github.com/channing771/mornlea/packages/shared/physics",
				"github.com/go-gl/mathgl/mgl32",
			},
			"packages/client/presentation": {
				"github.com/channing771/mornlea/packages/client/mesh",
				"github.com/channing771/mornlea/packages/shared/core",
			},
		}
	}
	// contractSources is a minimal compliant synthetic tree: runtime composes
	// the client mirrors and mathgl vectors, presentation shapes shared values,
	// and neither touches any forbidden family.
	contractSources := func() map[string]string {
		return map[string]string{
			"packages/client/runtime/step.go": `package runtime

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	client "github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/shared/core"
)

func step(camera *client.Camera) presentation.Frame {
	_ = mgl32.Ident4()
	_ = math.Pi
	_ = core.BlockPos{}
	return presentation.Frame{}
}
`,
			"packages/client/presentation/values.go": `package presentation

import "github.com/channing771/mornlea/packages/shared/core"

func value(position core.BlockPos) int { return 1 }
`,
		}
	}
	if violations := clientRuntimeBoundaryViolations(contractSources(), contractSurface, contractDeps()); len(violations) != 0 {
		t.Fatalf("synthetic contract-conforming sources must not be flagged: %v", violations)
	}

	for _, mutation := range []struct {
		name   string
		path   string
		source string
		marker string
	}{
		{
			name:   "client ABI symbol reference",
			path:   "packages/client/runtime/camera.go",
			source: "package runtime\n\nimport client \"github.com/channing771/mornlea/packages/client/client\"\n\nfunc viewProj() { _ = client.NativeViewProj }\n",
			marker: "client C ABI symbol NativeViewProj",
		},
		{
			name:   "client ABI dot import",
			path:   "packages/client/runtime/adopt.go",
			source: "package runtime\n\nimport . \"github.com/channing771/mornlea/packages/client/client\"\n",
			marker: "must not dot-import",
		},
		{
			name:   "raw client ABI header token",
			path:   "packages/client/presentation/frame.go",
			source: "package presentation\n\nconst abiHeader = \"mornlea_client.h\"\n",
			marker: "raw client C ABI token",
		},
		{
			name:   "godot application tree reference",
			path:   "packages/client/runtime/mesh.go",
			source: "package runtime\n\nconst assetRoot = \"apps/mornlea-godot/assets\"\n",
			marker: "mornlea-godot application tree",
		},
		{
			name:   "nested subpackage still joins the scan",
			path:   "packages/client/runtime/internal/device.go",
			source: "package internal\n\nimport _ \"github.com/godot-go/godot-go/gd\"\n",
			marker: "forbidden Godot family",
		},
		{
			name:   "unparsable file fails closed",
			path:   "packages/client/runtime/broken.go",
			source: "package runtime\n\nfunc broken( {\n",
			marker: "fails to parse",
		},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			sources := contractSources()
			sources[mutation.path] = mutation.source
			violations := clientRuntimeBoundaryViolations(sources, contractSurface, contractDeps())
			if len(violations) != 1 || !strings.Contains(violations[0], mutation.marker) {
				t.Fatalf("mutation %s must yield exactly one violation containing %q: %v", mutation.name, mutation.marker, violations)
			}
		})
	}

	// Every forbidden import family entry must reject its representative
	// sample, so no list entry can go stale and match nothing.
	for _, forbidden := range clientRuntimeForbiddenImports {
		t.Run("forbidden import family "+forbidden.family, func(t *testing.T) {
			sources := contractSources()
			sources["packages/client/runtime/device.go"] = fmt.Sprintf("package runtime\n\nimport _ %q\n", forbidden.sample)
			violations := clientRuntimeBoundaryViolations(sources, contractSurface, contractDeps())
			if len(violations) != 1 || !strings.Contains(violations[0], forbidden.family) {
				t.Fatalf("forbidden family %s must reject import %q: %v", forbidden.family, forbidden.sample, violations)
			}
		})
	}

	for _, closure := range []struct {
		name       string
		dependency string
		marker     string
	}{
		{
			name:       "transitive server dependency",
			dependency: "github.com/channing771/mornlea/packages/server/sim/runtime",
			marker:     "transitively depends on the authoritative server package",
		},
		{
			name:       "transitive godot dependency",
			dependency: "github.com/godot-go/godot-go/gd",
			marker:     "transitively depends on the Godot package",
		},
		{
			name:       "transitive GPU binding dependency",
			dependency: "github.com/oliverbestmann/webgpu",
			marker:     "transitively depends on the GPU or window binding",
		},
		{
			name:       "transitive window binding dependency",
			dependency: "github.com/go-gl/glfw/v3.3/glfw",
			marker:     "transitively depends on the GPU or window binding",
		},
	} {
		t.Run(closure.name, func(t *testing.T) {
			deps := contractDeps()
			deps["packages/client/runtime"] = append(deps["packages/client/runtime"], closure.dependency)
			violations := clientRuntimeBoundaryViolations(contractSources(), contractSurface, deps)
			if len(violations) != 1 || !strings.Contains(violations[0], closure.marker) {
				t.Fatalf("closure mutation %s must yield exactly one violation containing %q: %v", closure.name, closure.marker, violations)
			}
		})
	}

	// Platform-independence drift: a filename suffix and a build tag must each
	// fork the file set and be rejected, while a clean directory passes every
	// context. The probe runs against a temporary directory so the mutation
	// never touches the real tree.
	writeGuarded := func(t *testing.T, files map[string]string) string {
		t.Helper()
		directory := t.TempDir()
		for name, source := range files {
			if err := os.WriteFile(filepath.Join(directory, name), []byte(source), 0o600); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
		return directory
	}
	clean := map[string]string{"step.go": "package runtime\n\nfunc step() {}\n"}
	if violations := clientRuntimePlatformGatingViolations(t, writeGuarded(t, clean)); len(violations) != 0 {
		t.Fatalf("platform-independent synthetic files must not be flagged: %v", violations)
	}
	for _, platform := range []struct {
		name   string
		files  map[string]string
		marker string
	}{
		{
			name:   "filename suffix fork",
			files:  map[string]string{"camera.go": "package runtime\n\nfunc camera() {}\n", "camera_darwin.go": "package runtime\n\nfunc cameraDarwin() {}\n"},
			marker: "camera_darwin.go is excluded from the GOOS=",
		},
		{
			name:   "build tag fork",
			files:  map[string]string{"window.go": "//go:build darwin\n\npackage runtime\n\nfunc window() {}\n"},
			marker: "window.go is excluded from the GOOS=",
		},
	} {
		t.Run(platform.name, func(t *testing.T) {
			violations := clientRuntimePlatformGatingViolations(t, writeGuarded(t, platform.files))
			if len(violations) != 4 {
				t.Fatalf("a darwin-only fork must be excluded from exactly the linux and windows builds on both guarded architectures: %v", violations)
			}
			for _, violation := range violations {
				if !strings.Contains(violation, platform.marker) {
					t.Fatalf("platform violation must contain %q: %v", platform.marker, violations)
				}
			}
		})
	}
}
