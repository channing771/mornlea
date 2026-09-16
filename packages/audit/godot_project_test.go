package archcheck_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGodotProjectBootstrapIsPureAndStable(t *testing.T) {
	root := repositoryRoot(t)
	project := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "project.godot"))
	if !strings.Contains(project, `run/main_scene="res://app/bootstrap/bootstrap.tscn"`) {
		t.Error("Godot project main scene must be the stable Bootstrap scene")
	}
	for _, relative := range []string{
		"apps/mornlea-godot/project.godot",
		"apps/mornlea-godot/app/bootstrap/bootstrap.tscn",
		"apps/mornlea-godot/app/bootstrap/setup_required.tscn",
	} {
		text := readBaselineDoc(t, root, filepath.FromSlash(relative))
		for _, forbidden := range []string{"MornleaClientBridge", ".gdextension", "addons/mornlea_bridge/bin"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s serializes native dependency %q", relative, forbidden)
			}
		}
	}
	bootstrap := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "app", "bootstrap", "bootstrap.gd"))
	if !strings.Contains(bootstrap, `res://app/bootstrap/setup_required.tscn`) {
		t.Error("pure-GDScript Bootstrap must route to the setup-required scene")
	}
	ignore := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", ".gitignore"))
	for _, required := range []string{".godot/", "addons/mornlea_bridge/bin/"} {
		if !strings.Contains(ignore, required) {
			t.Errorf("project .gitignore is missing %q", required)
		}
	}
	if strings.Contains(ignore, ".uid") {
		t.Error("Godot script and shader UID sidecars must remain tracked")
	}
}

func TestGodotBootstrapDoesNotInitiateNetwork(t *testing.T) {
	root := repositoryRoot(t)
	for _, relative := range []string{
		"apps/mornlea-godot/app/bootstrap/bootstrap.gd",
		"apps/mornlea-godot/app/bootstrap/setup_required.gd",
	} {
		text := readBaselineDoc(t, root, filepath.FromSlash(relative))
		for _, forbidden := range []string{
			"StreamPeerTCP",
			"PacketPeer",
			"HTTPClient",
			"HTTPRequest",
			"connect_to_host",
			"connect_to_url",
		} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s initiates network through %q", relative, forbidden)
			}
		}
	}
}

func TestGodotBootstrapDiagnosesIncompleteDistribution(t *testing.T) {
	root := repositoryRoot(t)
	bootstrap := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "app", "bootstrap", "bootstrap.gd"))
	for _, required := range []string{
		"python-extension",
		"python-plugin",
		"python-bridge",
		"python-interpreter",
		"python-stdlib",
		"project-bridge-extension",
		"project-bridge-library",
		"4.7-alpha-21",
		"initialize_pythonscript",
		"gdext_rust_init",
		"mismatched",
		"build-python-runtime.sh --verify --offline",
		"build-extension.sh --target %s --profile debug --verify",
		"python=not-imported",
		"network=not-started",
	} {
		if !strings.Contains(bootstrap, required) {
			t.Errorf("Bootstrap is missing distribution diagnostic %q", required)
		}
	}
	for _, forbidden := range []string{
		"res://features/",
		"res://platform/",
		"res://app/host/",
		"StreamPeerTCP",
		"HTTPClient",
		"HTTPRequest",
	} {
		if strings.Contains(bootstrap, forbidden) {
			t.Errorf("Bootstrap must not import features or initiate network through %q", forbidden)
		}
	}

	smoke := readBaselineDoc(t, root, filepath.Join("scripts", "godot", "openable-smoke.sh"))
	for _, required := range []string{
		"--without-native",
		"--without-python",
		"deny network*",
		"python-extension-version",
		"project-bridge-entry-symbol",
	} {
		if !strings.Contains(smoke, required) {
			t.Errorf("openable-smoke.sh is missing clean-state assertion %q", required)
		}
	}
}

func TestGodotDesktopOnlyFeatureSkeleton(t *testing.T) {
	root := repositoryRoot(t)
	manifestPaths := []string{
		"features/session/feature.tres",
		"features/player_view/feature.tres",
		"features/world/feature.tres",
		"features/actors/feature.tres",
		"features/ui/feature.tres",
		"platform/desktop/input/feature.tres",
		"platform/desktop/lifecycle/feature.tres",
		"platform/desktop/audio/feature.tres",
	}
	catalog := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "config", "feature_catalog.tres"))
	for _, relative := range manifestPaths {
		fullPath := filepath.Join("apps", "mornlea-godot", filepath.FromSlash(relative))
		manifest := readBaselineDoc(t, root, fullPath)
		if !strings.Contains(manifest, "enabled = false") {
			t.Errorf("unimplemented Godot skeleton %s must stay explicitly disabled", relative)
		}
		if !strings.Contains(catalog, "res://"+relative) {
			t.Errorf("explicit feature catalog is missing %s", relative)
		}
	}
	for _, relative := range []string{
		"platform/mobile",
		"platform/android",
		"platform/ios",
		"platform/web",
		"platform/console",
	} {
		path := filepath.Join(root, "apps", "mornlea-godot", filepath.FromSlash(relative))
		if _, err := os.Lstat(path); err == nil {
			t.Errorf("unsupported Godot platform directory exists: %s", relative)
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("inspect unsupported Godot platform directory %s: %v", relative, err)
		}
	}
}

func TestGodotFeatureHostIsPythonOwnedAndExplicit(t *testing.T) {
	root := repositoryRoot(t)
	projectRoot := filepath.Join(root, "apps", "mornlea-godot")
	for _, relative := range []string{
		"app/host/app_root.gd",
		"app/host/feature_catalog.gd",
		"app/host/feature_host.gd",
		"app/host/feature_manifest.gd",
	} {
		path := filepath.Join(projectRoot, filepath.FromSlash(relative))
		if _, err := os.Stat(path); err == nil {
			t.Errorf("retired GDScript host still exists: %s", relative)
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("inspect retired GDScript host %s: %v", relative, err)
		}
	}

	host := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "app", "host", "feature_host.py"))
	for _, required := range []string{
		"HOST_PROTOCOL_MAJOR = 1",
		"HOST_PROTOCOL_MINOR = 0",
		"plan_catalog",
		"activate_catalog",
		"validate_feature",
		"bind_host",
		"reset_features",
		"deactivate_features",
		"required_bridge_families",
		"entry_scene_path.startswith(\"res://\")",
	} {
		if !strings.Contains(host, required) {
			t.Errorf("Python feature host is missing %q", required)
		}
	}

	catalog := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "config", "feature_catalog.tres"))
	for _, required := range []string{
		"metadata/host_protocol_major = 1",
		"metadata/host_protocol_minor = 0",
		"metadata/manifest_paths = PackedStringArray",
	} {
		if !strings.Contains(catalog, required) {
			t.Errorf("explicit Python feature catalog is missing %q", required)
		}
	}

	appScene := readBaselineDoc(t, root, filepath.Join("apps", "mornlea-godot", "app", "host", "app_root.tscn"))
	for _, required := range []string{"res://app/host/app_root.py", "res://app/host/feature_host.py"} {
		if !strings.Contains(appScene, required) {
			t.Errorf("Python app-root scene is missing %q", required)
		}
	}
	if strings.Contains(appScene, ".gd\"") {
		t.Error("Python app-root scene must not retain a GDScript host")
	}

	contractCheck := readBaselineDoc(t, root, filepath.Join("scripts", "godot", "feature-contract-check.sh"))
	for _, required := range []string{
		"res://tests/scenes/feature_contract_check.tscn",
		"Python feature contract checks passed.",
		"--extensibility-probe",
	} {
		if !strings.Contains(contractCheck, required) {
			t.Errorf("feature-contract-check.sh is missing %q", required)
		}
	}
}
