package archcheck_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Production script scopes for protocol-implementation markers. The Godot
// pilot must keep every wire-format decision in Go and Rust: Python and
// GDScript may orchestrate lifecycle around typed bridge values, but they
// must never encode or decode client-core records themselves.
var godotProtocolScriptDirs = []string{
	"app",
	"features",
	"platform",
	"shared",
	filepath.Join("addons", "mornlea_bridge"),
}

// Host scopes that must stay feature agnostic. The catalog resource graph,
// not these files, decides which features exist; the host may hand the
// bridge object to features but must not invoke their data-plane methods.
var godotHostScriptDirs = []string{
	filepath.Join("app", "host"),
	filepath.Join("app", "bootstrap"),
}

// Client-core record magics from the frozen ABI header, in both wire-text and
// little-endian word form, plus the shared "MC" byte-prefix form a Python
// byte-list copy would show.
var godotWireMagics = []string{"MCI1", "MCC1", "MCN1", "MCS1", "MCW1", "MCF1", "MCM1"}
var godotWireMagicWords = []uint64{
	0x3149434D, 0x3143434D, 0x314E434D, 0x3153434D, 0x3157434D, 0x3146434D, 0x314D434D,
}

var godotWireWordRegex = regexp.MustCompile(`(?i)\b0x[0-9a-f]{8}\b`)
var godotWireBytePrefixRegex = regexp.MustCompile(`0x4[dD]\s*,\s*0x4[3cCdD]\b`)
var godotWireCodecRegex = regexp.MustCompile(`\bstruct\s*\.\s*(?:pack|unpack|Struct)\b|\bfrom\s+struct\s+import\b`)
var godotCodecImportRegex = regexp.MustCompile(`(?m)^\s*(?:from|import)\s+([A-Za-z0-9_.]+)`)

// Codec-bearing import roots that would smuggle the Go protocol into the
// embedded interpreter, which is isolated from repository Python packages.
var godotForbiddenImportRoots = []string{
	"packages.shared.network",
	"packages.client",
	"packages.server",
}

// Bridge methods that belong to feature data-plane ownership. The host binds
// and forwards the bridge object; only features may invoke these names.
var godotGameplayBridgeCallRegex = regexp.MustCompile(
	`"(session_create|session_connect|session_poll|session_submit|session_step|` +
		`session_close|session_status_typed|pull_world|pull_frame|pull_status|pull_identity|` +
		`terrain_attach|terrain_ingest_world|terrain_frame|terrain_sections_json)"`,
)

// Concrete feature identities declared by the production catalog; a host file
// naming one of them has stopped being catalog driven.
var godotFeatureReferenceRegex = regexp.MustCompile(
	`res://(?:features|platform)/|` +
		`"(?:session|player_view|world|actors|ui|platform\.desktop\.(?:lifecycle|input|audio))"`,
)

type godotDisciplineFinding struct {
	path   string
	rule   string
	detail string
}

func (finding godotDisciplineFinding) String() string {
	return finding.path + ": " + finding.rule + ": " + finding.detail
}

// scanGodotScriptDiscipline walks the production script tree and reports every
// protocol-implementation and host-discipline violation. Tests use it both on
// the real project and on mutation fixtures, so the enforcement lives in one
// scanner rather than in per-test regexes.
func scanGodotScriptDiscipline(projectRoot string) []godotDisciplineFinding {
	findings := []godotDisciplineFinding{}
	hostDirs := map[string]bool{}
	for _, dir := range godotHostScriptDirs {
		hostDirs[dir] = true
	}
	for _, dir := range godotProtocolScriptDirs {
		findings = append(findings, scanGodotScriptDir(projectRoot, dir, hostDirs)...)
	}
	// Host directories outside the protocol set (none today) would still need
	// the agnostic scan, so walk them explicitly as well.
	for dir := range hostDirs {
		if !containsDir(godotProtocolScriptDirs, dir) {
			findings = append(findings, scanGodotScriptDir(projectRoot, dir, hostDirs)...)
		}
	}
	return findings
}

func containsDir(dirs []string, dir string) bool {
	for _, candidate := range dirs {
		if candidate == dir {
			return true
		}
	}
	return false
}

func scanGodotScriptDir(projectRoot, dir string, hostDirs map[string]bool) []godotDisciplineFinding {
	findings := []godotDisciplineFinding{}
	root := filepath.Join(projectRoot, filepath.FromSlash(dir))
	isHost := hostDirs[dir]
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == "__pycache__" || entry.Name() == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".py" && filepath.Ext(path) != ".pyi" && filepath.Ext(path) != ".gd" {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		relative, relErr := filepath.Rel(projectRoot, path)
		if relErr != nil {
			return nil
		}
		source := string(contents)
		findings = append(findings, protocolFindings(relative, source)...)
		if isHost {
			findings = append(findings, hostFindings(relative, source)...)
		}
		return nil
	})
	return findings
}

func protocolFindings(relative, source string) []godotDisciplineFinding {
	findings := []godotDisciplineFinding{}
	for _, magic := range godotWireMagics {
		if strings.Contains(source, magic) {
			findings = append(findings, godotDisciplineFinding{
				relative, "protocol-magic-text", magic,
			})
		}
	}
	for _, word := range godotWireWordRegex.FindAllString(source, -1) {
		value, parseErr := strconv.ParseUint(strings.ToLower(strings.TrimPrefix(word, "0x")), 16, 64)
		if parseErr != nil {
			continue
		}
		for _, magic := range godotWireMagicWords {
			if value == magic {
				findings = append(findings, godotDisciplineFinding{
					relative, "protocol-magic-word", word,
				})
			}
		}
	}
	for _, match := range godotWireBytePrefixRegex.FindAllString(source, -1) {
		findings = append(findings, godotDisciplineFinding{
			relative, "protocol-wire-byte-prefix", match,
		})
	}
	if location := godotWireCodecRegex.FindString(source); location != "" {
		findings = append(findings, godotDisciplineFinding{
			relative, "protocol-wire-record-codec", location,
		})
	}
	for _, match := range godotCodecImportRegex.FindAllStringSubmatch(source, -1) {
		module := match[1]
		for _, root := range godotForbiddenImportRoots {
			if module == root || strings.HasPrefix(module, root+".") {
				findings = append(findings, godotDisciplineFinding{
					relative, "protocol-codec-import", module,
				})
			}
		}
		for _, segment := range strings.Split(module, ".") {
			if strings.Contains(segment, "codec") {
				findings = append(findings, godotDisciplineFinding{
					relative, "protocol-codec-import", module,
				})
			}
		}
	}
	return findings
}

func hostFindings(relative, source string) []godotDisciplineFinding {
	findings := []godotDisciplineFinding{}
	if match := godotGameplayBridgeCallRegex.FindString(source); match != "" {
		findings = append(findings, godotDisciplineFinding{
			relative, "host-gameplay-bridge-call", match,
		})
	}
	if match := godotFeatureReferenceRegex.FindString(source); match != "" {
		findings = append(findings, godotDisciplineFinding{
			relative, "host-feature-reference", match,
		})
	}
	return findings
}

func findingsWithRule(findings []godotDisciplineFinding, prefix string) []godotDisciplineFinding {
	matched := []godotDisciplineFinding{}
	for _, finding := range findings {
		if strings.HasPrefix(finding.rule, prefix) {
			matched = append(matched, finding)
		}
	}
	return matched
}

// readGodotHostFile reads one project file whose absence must itself fail the
// structural seam assertions instead of silently passing them.
func readGodotHostFile(t *testing.T, projectRoot, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectRoot, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if len(data) == 0 {
		t.Fatalf("%s is empty: the seam assertions rely on its contents", name)
	}
	return string(data)
}

func TestGodotScriptsDoNotImplementProtocolMarkers(t *testing.T) {
	root := repositoryRoot(t)
	projectRoot := filepath.Join(root, "apps", "mornlea-godot")
	findings := scanGodotScriptDiscipline(projectRoot)
	for _, finding := range findingsWithRule(findings, "protocol-") {
		t.Errorf("production script implements protocol details: %s", finding)
	}

	// The feature host must receive the native bridge object itself. A Python
	// facade interposed on the data path is where wire knowledge would creep
	// back in, so the app scene must provide the native node and the app root
	// must not reference the retired static facade.
	appScene := readGodotHostFile(t, projectRoot, filepath.Join("app", "host", "app_root.tscn"))
	if !strings.Contains(appScene, `type="MornleaClientBridge"`) {
		t.Error("app_root.tscn must provide the native bridge node as the typed host service")
	}
	appRoot := readGodotHostFile(t, projectRoot, filepath.Join("app", "host", "app_root.py"))
	if strings.Contains(appRoot, "BridgeHost") {
		t.Error("app_root.py must not reference the retired static bridge facade")
	}

	mutations := []struct {
		name     string
		relative string
		source   string
		expected string
	}{
		{
			name:     "wire magic text in a feature",
			relative: filepath.Join("features", "world", "world_feature.py"),
			source:   "from __future__ import annotations\nMAGIC = \"MCI1\"\n",
			expected: "protocol-magic-text",
		},
		{
			name:     "wire magic word in the host",
			relative: filepath.Join("app", "host", "feature_host.py"),
			source:   "from __future__ import annotations\nIDENTITY = 0x3149434D\n",
			expected: "protocol-magic-word",
		},
		{
			name:     "wire byte prefix in a platform adapter",
			relative: filepath.Join("platform", "desktop", "input", "desktop_input_feature.py"),
			source:   "from __future__ import annotations\nBATCH = [0x4D, 0x43, 0x4E, 0x31, 0, 0]\n",
			expected: "protocol-wire-byte-prefix",
		},
		{
			name:     "struct record decode in a feature",
			relative: filepath.Join("features", "world", "world_feature.py"),
			source:   "from __future__ import annotations\nimport struct\nWORDS = struct.unpack(\"<I\", record)\n",
			expected: "protocol-wire-record-codec",
		},
		{
			name:     "network codec import in the bridge addon",
			relative: filepath.Join("addons", "mornlea_bridge", "bridge_host.py"),
			source:   "from __future__ import annotations\nfrom packages.shared.network import codec\n",
			expected: "protocol-codec-import",
		},
		{
			name:     "generic codec module import in a feature",
			relative: filepath.Join("features", "world", "world_feature.py"),
			source:   "from __future__ import annotations\nimport world_packet_codec\n",
			expected: "protocol-codec-import",
		},
		{
			name:     "wire magic in bootstrap GDScript",
			relative: filepath.Join("app", "bootstrap", "bootstrap.gd"),
			source:   "extends Node\nconst WIRE_MAGIC := \"MCW1\"\n",
			expected: "protocol-magic-text",
		},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			fixture := newGodotHostDisciplineFixture(t)
			writeGodotFixtureFile(t, filepath.Join(fixture, filepath.FromSlash(mutation.relative)), mutation.source)
			mutated := findingsWithRule(scanGodotScriptDiscipline(fixture), "protocol-")
			if len(mutated) == 0 {
				t.Fatalf("scanner accepted a protocol-implementation probe: %s", mutation.name)
			}
			found := false
			for _, finding := range mutated {
				if finding.rule == mutation.expected && strings.HasSuffix(filepath.ToSlash(finding.path), mutation.relative) {
					found = true
				}
			}
			if !found {
				t.Fatalf(
					"probe %s reported the wrong rule or file: %v (want %s in %s)",
					mutation.name, mutated, mutation.expected, mutation.relative,
				)
			}
		})
	}

	fixture := newGodotHostDisciplineFixture(t)
	if leaked := scanGodotScriptDiscipline(fixture); len(leaked) != 0 {
		t.Fatalf("clean fixture produced findings: %v", leaked)
	}
}

func TestGodotHostIsFeatureAgnosticAcrossInjection(t *testing.T) {
	root := repositoryRoot(t)
	projectRoot := filepath.Join(root, "apps", "mornlea-godot")
	findings := scanGodotScriptDiscipline(projectRoot)
	for _, finding := range findingsWithRule(findings, "host-") {
		t.Errorf("host file is not feature agnostic: %s", finding)
	}

	// The host binds the scene-provided native bridge node; this structural
	// seam is what keeps host code free of bridge-facade and feature knowledge.
	appScene := readGodotHostFile(t, projectRoot, filepath.Join("app", "host", "app_root.tscn"))
	if !strings.Contains(appScene, `type="MornleaClientBridge"`) {
		t.Error("app_root.tscn must provide the native bridge node for host binding")
	}

	mutations := []struct {
		name     string
		relative string
		source   string
		expected string
	}{
		{
			name:     "branching on a concrete feature id",
			relative: filepath.Join("app", "host", "feature_host.py"),
			source:   "from __future__ import annotations\nif feature_id == \"world\":\n    pass\n",
			expected: "host-feature-reference",
		},
		{
			name:     "feature entry path outside the catalog",
			relative: filepath.Join("app", "host", "app_root.py"),
			source:   "from __future__ import annotations\nSCENE = \"res://features/session/feature_root.tscn\"\n",
			expected: "host-feature-reference",
		},
		{
			name:     "platform adapter path in the host",
			relative: filepath.Join("app", "host", "app_root.py"),
			source:   "from __future__ import annotations\nPATH = \"res://platform/desktop/lifecycle/feature.tres\"\n",
			expected: "host-feature-reference",
		},
		{
			name:     "host invoking a feature-owned submit method",
			relative: filepath.Join("app", "host", "feature_host.py"),
			source:   "from __future__ import annotations\nbridge.call(\"session_submit\", batch)\n",
			expected: "host-gameplay-bridge-call",
		},
		{
			name:     "bootstrap pulling a world record",
			relative: filepath.Join("app", "bootstrap", "bootstrap.gd"),
			source:   "extends Node\nvar world = bridge.call(\"pull_world\")\n",
			expected: "host-gameplay-bridge-call",
		},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			fixture := newGodotHostDisciplineFixture(t)
			writeGodotFixtureFile(t, filepath.Join(fixture, filepath.FromSlash(mutation.relative)), mutation.source)
			mutated := findingsWithRule(scanGodotScriptDiscipline(fixture), "host-")
			if len(mutated) == 0 {
				t.Fatalf("scanner accepted a host-discipline probe: %s", mutation.name)
			}
			found := false
			for _, finding := range mutated {
				if finding.rule == mutation.expected && strings.HasSuffix(filepath.ToSlash(finding.path), mutation.relative) {
					found = true
				}
			}
			if !found {
				t.Fatalf(
					"probe %s reported the wrong rule or file: %v (want %s in %s)",
					mutation.name, mutated, mutation.expected, mutation.relative,
				)
			}
		})
	}
}

// newGodotHostDisciplineFixture mirrors the minimal production script layout
// the discipline scanner covers, so mutation probes exercise exactly the
// scopes under review without generated runtimes or native binaries.
func newGodotHostDisciplineFixture(t *testing.T) string {
	t.Helper()
	fixture := t.TempDir()
	files := map[string]string{
		filepath.Join("app", "host", "app_root.tscn"): "[gd_scene format=3]\n\n" +
			"[node name=\"AppRoot\" type=\"Node\"]\n\n" +
			"[node name=\"ClientBridge\" type=\"MornleaClientBridge\" parent=\".\"]\n",
		filepath.Join("app", "host", "app_root.py"):                    "from __future__ import annotations\n",
		filepath.Join("app", "host", "feature_host.py"):                "from __future__ import annotations\n",
		filepath.Join("app", "bootstrap", "bootstrap.gd"):              "extends Node\n",
		filepath.Join("features", "world", "world_feature.py"):         "from __future__ import annotations\n",
		filepath.Join("platform", "desktop", "input", "feature.py"):    "from __future__ import annotations\n",
		filepath.Join("addons", "mornlea_bridge", "bridge_host.py"):    "from __future__ import annotations\n",
		filepath.Join("addons", "mornlea_bridge", "bin", "skipped.py"): "from __future__ import annotations\n",
		filepath.Join("features", "world", "__pycache__", "cached.py"): "from __future__ import annotations\nMAGIC = \"MCI1\"\n",
	}
	for relative, contents := range files {
		writeGodotFixtureFile(t, filepath.Join(fixture, relative), contents)
	}
	return fixture
}
