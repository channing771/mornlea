package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/client/client"
)

func TestGodotPilotBaselineManifestIsComplete(t *testing.T) {
	manifest, err := readGodotPilotBaselineManifest(filepath.Join("..", "..", "..", "testdata", "godot-pilot", "baseline-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ProtocolVersion != 44 || manifest.EngineABIVersion != 11 ||
		manifest.ClientABIVersion != 19 || manifest.ClientCoreABIMajor != 1 ||
		manifest.BenchmarkScenarioVersion != 23 {
		t.Fatalf("unexpected baseline identities: %+v", manifest)
	}
}

func TestGodotPilotBaselineSourceReportsMatchManifest(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	manifest, err := readGodotPilotBaselineManifest(filepath.Join(repositoryRoot, "testdata", "godot-pilot", "baseline-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range manifest.SourceReports {
		data, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(source.Path)))
		if err != nil {
			t.Fatalf("read source report %s: %v", source.Path, err)
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != source.SHA256 {
			t.Errorf("source report %s SHA-256 = %s, want %s", source.Path, got, source.SHA256)
		}
		var report client.PerfReport
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatalf("decode source report %s: %v", source.Path, err)
		}
		if report.ScenarioVersion != manifest.BenchmarkScenarioVersion {
			t.Errorf("source report %s scenario_version = %d, want %d", source.Path, report.ScenarioVersion, manifest.BenchmarkScenarioVersion)
		}
		if report.GitCommit != manifest.GitCommit {
			t.Errorf("source report %s git_commit = %s, want %s", source.Path, report.GitCommit, manifest.GitCommit)
		}
		if err := validateReportProvenance(source.Path, report); err != nil {
			t.Error(err)
		}
		if err := validateV6Report(source.Path, report); err != nil {
			t.Error(err)
		}
	}
}

func TestGodotPilotBaselineValidationRejectsIncompleteIdentity(t *testing.T) {
	for name, mutate := range map[string]func(*godotPilotBaselineManifest){
		"schema": func(manifest *godotPilotBaselineManifest) {
			manifest.SchemaVersion = 0
		},
		"commit": func(manifest *godotPilotBaselineManifest) {
			manifest.GitCommit = "not-a-commit"
		},
		"worktree": func(manifest *godotPilotBaselineManifest) {
			manifest.WorktreeState = "unknown"
		},
		"platform": func(manifest *godotPilotBaselineManifest) {
			manifest.Platform.OS = "web"
		},
		"gpu": func(manifest *godotPilotBaselineManifest) {
			manifest.GPU.Name = ""
		},
		"resolution": func(manifest *godotPilotBaselineManifest) {
			manifest.Resolution.Width = 0
		},
		"view distance": func(manifest *godotPilotBaselineManifest) {
			manifest.ViewDistance = 1
		},
		"scenes": func(manifest *godotPilotBaselineManifest) {
			manifest.Scenes = nil
		},
		"duplicate scene": func(manifest *godotPilotBaselineManifest) {
			manifest.Scenes = append(manifest.Scenes, manifest.Scenes[0])
		},
		"warmup": func(manifest *godotPilotBaselineManifest) {
			manifest.Warmup.Seconds = 0
		},
		"sample": func(manifest *godotPilotBaselineManifest) {
			manifest.Sample.MinimumGPUSamples = 0
		},
		"protocol": func(manifest *godotPilotBaselineManifest) {
			manifest.ProtocolVersion = 0
		},
		"engine ABI": func(manifest *godotPilotBaselineManifest) {
			manifest.EngineABIVersion = 0
		},
		"client ABI": func(manifest *godotPilotBaselineManifest) {
			manifest.ClientABIVersion = 0
		},
		"benchmark": func(manifest *godotPilotBaselineManifest) {
			manifest.BenchmarkScenarioVersion = 0
		},
		"reports": func(manifest *godotPilotBaselineManifest) {
			manifest.SourceReports = nil
		},
		"visual root": func(manifest *godotPilotBaselineManifest) {
			manifest.VisualBaselines.Root = ""
		},
	} {
		t.Run(name, func(t *testing.T) {
			manifest := completeGodotPilotBaselineManifest()
			mutate(&manifest)
			if err := manifest.Validate(); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}

func TestGodotPilotBaselineReaderRejectsUnknownAndTrailingData(t *testing.T) {
	valid := `{"schema_version":1,"git_commit":"84f3e0e75dff6987e47cf2aa50f50d888e85eafb","worktree_state":"dirty","platform":{"os":"darwin","arch":"arm64","version":"macOS 26.6.2"},"gpu":{"name":"Apple M2","api":"Metal 4"},"resolution":{"width":2560,"height":1440},"view_distance":32,"scenes":["benchmark-v23"],"warmup":{"seconds":10},"sample":{"still_seconds":60,"flying_seconds":120,"cooldown_seconds":30,"minimum_gpu_samples":128},"protocol_version":44,"engine_abi_version":11,"client_abi_version":19,"client_core_abi_major":1,"benchmark_scenario_version":23,"source_reports":[{"path":"testdata/godot-pilot/legacy-rust-memory-v23.json","sha256":"e588b40e268f317fee1acaa4ff5c1e0ae035cc2ec98dc170e9dc90ce6750339e"}],"visual_baselines":{"root":"testdata/visual-golden","ui_images":31,"world_images":31,"motion_gifs":11}}`
	for name, source := range map[string]string{
		"unknown field": strings.TrimSuffix(valid, "}") + `,"unexpected":true}`,
		"trailing data": valid + `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.json")
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readGodotPilotBaselineManifest(path); err == nil {
				t.Fatal("expected strict decoding failure")
			}
		})
	}
}

func completeGodotPilotBaselineManifest() godotPilotBaselineManifest {
	return godotPilotBaselineManifest{
		SchemaVersion: 1,
		GitCommit:     "84f3e0e75dff6987e47cf2aa50f50d888e85eafb",
		WorktreeState: "dirty",
		Platform: godotPilotPlatform{
			OS: "darwin", Arch: "arm64", Version: "macOS 26.6.2",
		},
		GPU:          godotPilotGPU{Name: "Apple M2", API: "Metal 4"},
		Resolution:   godotPilotResolution{Width: 2560, Height: 1440},
		ViewDistance: 32,
		Scenes:       []string{"benchmark-v23"},
		Warmup:       godotPilotWarmup{Seconds: 10},
		Sample: godotPilotSample{
			StillSeconds: 60, FlyingSeconds: 120, CooldownSeconds: 30, MinimumGPUSamples: 128,
		},
		ProtocolVersion:          44,
		EngineABIVersion:         11,
		ClientABIVersion:         19,
		ClientCoreABIMajor:       1,
		BenchmarkScenarioVersion: 23,
		SourceReports: []godotPilotSourceReport{{
			Path: "testdata/godot-pilot/legacy-rust-memory-v23.json", SHA256: "e588b40e268f317fee1acaa4ff5c1e0ae035cc2ec98dc170e9dc90ce6750339e",
		}},
		VisualBaselines: godotPilotVisualBaselines{
			Root: "testdata/visual-golden", UIImages: 31, WorldImages: 31, MotionGIFs: 11,
		},
	}
}
