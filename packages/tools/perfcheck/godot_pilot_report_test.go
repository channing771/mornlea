package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGodotPilotReportValidationAcceptsCompleteIdentity(t *testing.T) {
	report := completeGodotPilotReport()
	if err := report.Validate(); err != nil {
		t.Fatalf("complete report must validate: %v", err)
	}
}

func TestGodotPilotReportValidationRejectsIncompleteIdentityAndZeroedGaps(t *testing.T) {
	t.Run("comparable gpu without quantiles", func(t *testing.T) {
		broken := completeGodotPilotReport()
		broken.GPUFrame = godotPilotPercentiles{Comparable: true}
		if err := broken.Validate(); err == nil {
			t.Fatal("comparable GPU frame without quantiles must fail")
		}
	})

	t.Run("non-comparable gpu with zeros", func(t *testing.T) {
		broken := completeGodotPilotReport()
		zero := 0.0
		broken.GPUFrame = godotPilotPercentiles{
			Comparable: false,
			Reason:     "headless Godot did not expose GPU timestamps",
			P50MS:      &zero,
			P95MS:      &zero,
			P99MS:      &zero,
		}
		if err := broken.Validate(); err == nil {
			t.Fatal("non-comparable fields must omit numeric values rather than record zero")
		}
	})

	t.Run("overflow is data loss", func(t *testing.T) {
		broken := completeGodotPilotReport()
		lost := 1.0
		broken.Queues.OverflowCount.Value = &lost
		if err := broken.Validate(); err == nil {
			t.Fatal("recorded overflow must be a hard failure")
		}
	})

	t.Run("missing python runtime identity", func(t *testing.T) {
		broken := completeGodotPilotReport()
		broken.Identity.CPythonVersion = ""
		if err := broken.Validate(); err == nil {
			t.Fatal("incomplete Python identity must fail")
		}
	})
}

func TestGodotPilotReportReaderRejectsUnknownAndTrailingData(t *testing.T) {
	data, err := json.Marshal(completeGodotPilotReport())
	if err != nil {
		t.Fatal(err)
	}
	valid := string(data)
	for name, source := range map[string]string{
		"unknown field": strings.TrimSuffix(valid, "}") + `,"unexpected":true}`,
		"trailing data": valid + `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "report.json")
			if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readGodotPilotReport(path); err == nil {
				t.Fatal("expected strict decoding failure")
			}
		})
	}
}

func TestGodotPilotReportRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	data, err := json.Marshal(completeGodotPilotReport())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readGodotPilotReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Identity.BenchmarkScenarioVersion != 23 || got.Identity.Catalog != "apps/mornlea-godot/config/feature_catalog.tres" {
		t.Fatalf("unexpected round-trip identity: %+v", got.Identity)
	}
}

func completeGodotPilotReport() godotPilotReport {
	samples := 128
	p50, p95, p99 := 4.0, 6.0, 9.0
	zero := 0.0
	viewDistance := 2.0
	seed := 20260726.0
	loginMS := 120.0
	visibleMS := 800.0
	prepareMS := 3.5
	uploadMS := 1.2
	packed := 4096.0
	expanded := 102400.0
	rids := 4.0
	meshes := 2.0
	peakRSS := 350000000.0
	steadyRSS := 280000000.0
	return godotPilotReport{
		SchemaVersion: 1,
		Identity: godotPilotReportIdentity{
			GitCommit:              "84f3e0e75dff6987e47cf2aa50f50d888e85eafb",
			WorktreeState:          "dirty",
			Platform:               godotPilotPlatform{OS: "darwin", Arch: "arm64", Version: "macOS 26.6.2"},
			GPU:                    godotPilotGPU{Name: "Apple M2", API: "Metal 4"},
			GodotVersion:           "4.7.2-stable",
			Py4GodotVersion:        "4.7-alpha21",
			Py4GodotSourceRevision: "d8e17428deeb0428587349b663f6da26cd71ef3a",
			CPythonVersion:         "3.14.4",
			Catalog:                "apps/mornlea-godot/config/feature_catalog.tres",
			FeatureFamilies: []godotPilotFeatureFamily{
				{Family: 1, Version: 1, RecordLimit: 1, RecordBytes: 16},
			},
			ProtocolVersion:          44,
			EngineABIVersion:         11,
			ClientABIVersion:         19,
			ClientCoreABIMajor:       1,
			BenchmarkScenarioVersion: 23,
			Resolution:               godotPilotResolution{Width: 2560, Height: 1440},
			ViewDistance:             godotPilotScalar{Comparable: true, Value: &viewDistance},
			Seed:                     godotPilotScalar{Comparable: true, Value: &seed},
			WarmupSeconds:            10,
			Sample: godotPilotSample{
				StillSeconds: 60, FlyingSeconds: 120, CooldownSeconds: 30, MinimumGPUSamples: 128,
			},
			ValidSampleCount: 128,
		},
		CPUFrame: godotPilotPercentiles{Comparable: true, Samples: &samples, P50MS: &p50, P95MS: &p95, P99MS: &p99},
		GPUFrame: godotPilotPercentiles{Comparable: false, Reason: "headless Godot did not expose GPU timestamps"},
		PythonHost: godotPilotPythonHost{
			Apply:              godotPilotPercentiles{Comparable: true, Samples: &samples, P50MS: &p50, P95MS: &p95, P99MS: &p99},
			Process:            godotPilotPercentiles{Comparable: true, Samples: &samples, P50MS: &p50, P95MS: &p95, P99MS: &p99},
			AllocationPressure: godotPilotPercentiles{Comparable: true, Samples: &samples, P50MS: &p50, P95MS: &p95, P99MS: &p99},
		},
		ColdStart: godotPilotColdStart{
			LoginMS:        godotPilotScalar{Comparable: true, Value: &loginMS},
			VisibleWorldMS: godotPilotScalar{Comparable: true, Value: &visibleMS},
		},
		Terrain: godotPilotTerrain{
			PrepareDurationMS: godotPilotScalar{Comparable: true, Value: &prepareMS},
			UploadDurationMS:  godotPilotScalar{Comparable: true, Value: &uploadMS},
			PackedBytes:       godotPilotScalar{Comparable: true, Value: &packed},
			ExpandedBytes:     godotPilotScalar{Comparable: true, Value: &expanded},
			RIDCount:          godotPilotScalar{Comparable: true, Value: &rids},
			MeshCount:         godotPilotScalar{Comparable: true, Value: &meshes},
			PeakRIDCount:      godotPilotScalar{Comparable: true, Value: &rids},
		},
		RSS: godotPilotRSS{
			PeakBytes:   godotPilotScalar{Comparable: true, Value: &peakRSS},
			SteadyBytes: godotPilotScalar{Comparable: true, Value: &steadyRSS},
		},
		InputLatency: godotPilotPercentiles{Comparable: true, Samples: &samples, P50MS: &p50, P95MS: &p95, P99MS: &p99},
		Queues: godotPilotQueues{
			OverflowCount: godotPilotScalar{Comparable: true, Value: &zero},
			UploadsFailed: godotPilotScalar{Comparable: true, Value: &zero},
		},
	}
}
