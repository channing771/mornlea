package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// godotPilotReport is the P7 dual-client performance record. Missing
// comparable observations are omitted and marked non-comparable; they are
// never written as zero.
type godotPilotReport struct {
	SchemaVersion int                      `json:"schema_version"`
	Identity      godotPilotReportIdentity `json:"identity"`
	CPUFrame      godotPilotPercentiles    `json:"cpu_frame"`
	GPUFrame      godotPilotPercentiles    `json:"gpu_frame"`
	PythonHost    godotPilotPythonHost     `json:"python_host"`
	ColdStart     godotPilotColdStart      `json:"cold_start"`
	Terrain       godotPilotTerrain        `json:"terrain"`
	RSS           godotPilotRSS            `json:"rss"`
	InputLatency  godotPilotPercentiles    `json:"input_to_presentation"`
	Queues        godotPilotQueues         `json:"queues"`
}

type godotPilotReportIdentity struct {
	GitCommit                string                    `json:"git_commit"`
	WorktreeState            string                    `json:"worktree_state"`
	Platform                 godotPilotPlatform        `json:"platform"`
	GPU                      godotPilotGPU             `json:"gpu"`
	GodotVersion             string                    `json:"godot_version"`
	Py4GodotVersion          string                    `json:"py4godot_version"`
	Py4GodotSourceRevision   string                    `json:"py4godot_source_revision"`
	CPythonVersion           string                    `json:"cpython_version"`
	Catalog                  string                    `json:"catalog"`
	FeatureFamilies          []godotPilotFeatureFamily `json:"feature_families"`
	ProtocolVersion          int                       `json:"protocol_version"`
	EngineABIVersion         int                       `json:"engine_abi_version"`
	ClientABIVersion         int                       `json:"client_abi_version"`
	ClientCoreABIMajor       int                       `json:"client_core_abi_major"`
	BenchmarkScenarioVersion int                       `json:"benchmark_scenario_version"`
	Resolution               godotPilotResolution      `json:"resolution"`
	ViewDistance             godotPilotScalar          `json:"view_distance"`
	Seed                     godotPilotScalar          `json:"seed"`
	WarmupSeconds            int                       `json:"warmup_seconds"`
	Sample                   godotPilotSample          `json:"sample"`
	ValidSampleCount         int                       `json:"valid_sample_count"`
}

type godotPilotFeatureFamily struct {
	Family      int `json:"family"`
	Version     int `json:"version"`
	RecordLimit int `json:"record_limit"`
	RecordBytes int `json:"record_bytes"`
}

// godotPilotPercentiles is a three-quantile observation. When Comparable is
// false, Reason is required and the quantile fields must be omitted.
type godotPilotPercentiles struct {
	Comparable bool     `json:"comparable"`
	Reason     string   `json:"reason,omitempty"`
	Samples    *int     `json:"samples,omitempty"`
	P50MS      *float64 `json:"p50_ms,omitempty"`
	P95MS      *float64 `json:"p95_ms,omitempty"`
	P99MS      *float64 `json:"p99_ms,omitempty"`
}

// godotPilotScalar is one numeric observation. A missing comparable value
// uses Comparable=false plus Reason rather than a zero stand-in.
type godotPilotScalar struct {
	Comparable bool     `json:"comparable"`
	Reason     string   `json:"reason,omitempty"`
	Value      *float64 `json:"value,omitempty"`
}

type godotPilotPythonHost struct {
	Apply              godotPilotPercentiles `json:"apply"`
	Process            godotPilotPercentiles `json:"process"`
	AllocationPressure godotPilotPercentiles `json:"allocation_pressure"`
}

type godotPilotColdStart struct {
	LoginMS        godotPilotScalar `json:"login_ms"`
	VisibleWorldMS godotPilotScalar `json:"visible_world_ms"`
}

type godotPilotTerrain struct {
	PrepareDurationMS godotPilotScalar `json:"prepare_duration_ms"`
	UploadDurationMS  godotPilotScalar `json:"upload_duration_ms"`
	PackedBytes       godotPilotScalar `json:"packed_bytes"`
	ExpandedBytes     godotPilotScalar `json:"expanded_bytes"`
	RIDCount          godotPilotScalar `json:"rid_count"`
	MeshCount         godotPilotScalar `json:"mesh_count"`
	PeakRIDCount      godotPilotScalar `json:"peak_rid_count"`
}

type godotPilotRSS struct {
	PeakBytes   godotPilotScalar `json:"peak_bytes"`
	SteadyBytes godotPilotScalar `json:"steady_bytes"`
}

type godotPilotQueues struct {
	OverflowCount godotPilotScalar `json:"overflow_count"`
	UploadsFailed godotPilotScalar `json:"uploads_failed"`
}

func readGodotPilotReport(filePath string) (godotPilotReport, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return godotPilotReport{}, fmt.Errorf("open Godot pilot report: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var report godotPilotReport
	if err := decoder.Decode(&report); err != nil {
		return godotPilotReport{}, fmt.Errorf("decode Godot pilot report: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return godotPilotReport{}, errors.New("decode Godot pilot report: trailing JSON value")
		}
		return godotPilotReport{}, fmt.Errorf("decode Godot pilot report trailing data: %w", err)
	}
	if err := report.Validate(); err != nil {
		return godotPilotReport{}, err
	}
	return report, nil
}

// Validate rejects incomplete identity, zeroed missing fields, and recorded
// overflow. Performance magnitudes stay informational once identity is whole.
func (report godotPilotReport) Validate() error {
	if report.SchemaVersion != 1 {
		return fmt.Errorf("Godot pilot report schema_version=%d, want 1", report.SchemaVersion)
	}
	if err := report.Identity.validate(); err != nil {
		return err
	}
	for _, item := range []struct {
		name  string
		value godotPilotPercentiles
	}{
		{name: "cpu_frame", value: report.CPUFrame},
		{name: "gpu_frame", value: report.GPUFrame},
		{name: "python_host.apply", value: report.PythonHost.Apply},
		{name: "python_host.process", value: report.PythonHost.Process},
		{name: "python_host.allocation_pressure", value: report.PythonHost.AllocationPressure},
		{name: "input_to_presentation", value: report.InputLatency},
	} {
		if err := item.value.validate(item.name); err != nil {
			return err
		}
	}
	for _, item := range []struct {
		name  string
		value godotPilotScalar
	}{
		{name: "cold_start.login_ms", value: report.ColdStart.LoginMS},
		{name: "cold_start.visible_world_ms", value: report.ColdStart.VisibleWorldMS},
		{name: "terrain.prepare_duration_ms", value: report.Terrain.PrepareDurationMS},
		{name: "terrain.upload_duration_ms", value: report.Terrain.UploadDurationMS},
		{name: "terrain.packed_bytes", value: report.Terrain.PackedBytes},
		{name: "terrain.expanded_bytes", value: report.Terrain.ExpandedBytes},
		{name: "terrain.rid_count", value: report.Terrain.RIDCount},
		{name: "terrain.mesh_count", value: report.Terrain.MeshCount},
		{name: "terrain.peak_rid_count", value: report.Terrain.PeakRIDCount},
		{name: "rss.peak_bytes", value: report.RSS.PeakBytes},
		{name: "rss.steady_bytes", value: report.RSS.SteadyBytes},
		{name: "queues.overflow_count", value: report.Queues.OverflowCount},
		{name: "queues.uploads_failed", value: report.Queues.UploadsFailed},
	} {
		if err := item.value.validate(item.name); err != nil {
			return err
		}
	}
	if err := requireComparableNonNegative("queues.overflow_count", report.Queues.OverflowCount); err != nil {
		return err
	}
	if err := requireComparableNonNegative("queues.uploads_failed", report.Queues.UploadsFailed); err != nil {
		return err
	}
	if *report.Queues.OverflowCount.Value > 0 {
		return fmt.Errorf("Godot pilot report overflow_count=%g is data loss", *report.Queues.OverflowCount.Value)
	}
	if *report.Queues.UploadsFailed.Value > 0 {
		return fmt.Errorf("Godot pilot report uploads_failed=%g is data loss", *report.Queues.UploadsFailed.Value)
	}
	return nil
}

func (identity godotPilotReportIdentity) validate() error {
	if !validHexDigest(identity.GitCommit, 20) {
		return errors.New("Godot pilot report git_commit must be a full 40-character hexadecimal commit")
	}
	if identity.WorktreeState != "clean" && identity.WorktreeState != "dirty" {
		return fmt.Errorf("Godot pilot report worktree_state=%q, want clean or dirty", identity.WorktreeState)
	}
	if identity.Platform.OS != "darwin" && identity.Platform.OS != "windows" && identity.Platform.OS != "linux" {
		return fmt.Errorf("Godot pilot report platform.os=%q is not an approved desktop target", identity.Platform.OS)
	}
	if strings.TrimSpace(identity.Platform.Arch) == "" || strings.TrimSpace(identity.Platform.Version) == "" {
		return errors.New("Godot pilot report platform identity is incomplete")
	}
	if strings.TrimSpace(identity.GPU.Name) == "" || strings.TrimSpace(identity.GPU.API) == "" {
		return errors.New("Godot pilot report GPU identity is incomplete")
	}
	for _, item := range []struct {
		name, value string
	}{
		{name: "godot_version", value: identity.GodotVersion},
		{name: "py4godot_version", value: identity.Py4GodotVersion},
		{name: "py4godot_source_revision", value: identity.Py4GodotSourceRevision},
		{name: "cpython_version", value: identity.CPythonVersion},
		{name: "catalog", value: identity.Catalog},
	} {
		if strings.TrimSpace(item.value) == "" {
			return fmt.Errorf("Godot pilot report %s is incomplete", item.name)
		}
	}
	if !validHexDigest(identity.Py4GodotSourceRevision, 20) {
		return errors.New("Godot pilot report py4godot_source_revision must be a full 40-character hexadecimal commit")
	}
	if cleaned := path.Clean(identity.Catalog); cleaned == "." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return fmt.Errorf("Godot pilot report catalog %q must stay repository-relative", identity.Catalog)
	}
	if len(identity.FeatureFamilies) == 0 {
		return errors.New("Godot pilot report feature_families must not be empty")
	}
	seen := make(map[int]struct{}, len(identity.FeatureFamilies))
	for _, family := range identity.FeatureFamilies {
		if family.Family <= 0 || family.Version <= 0 || family.RecordLimit <= 0 || family.RecordBytes < 0 {
			return fmt.Errorf("Godot pilot report feature family is incomplete: %+v", family)
		}
		if _, exists := seen[family.Family]; exists {
			return fmt.Errorf("Godot pilot report feature family %d is duplicated", family.Family)
		}
		seen[family.Family] = struct{}{}
	}
	if identity.ProtocolVersion <= 0 || identity.EngineABIVersion <= 0 ||
		identity.ClientABIVersion <= 0 || identity.ClientCoreABIMajor <= 0 ||
		identity.BenchmarkScenarioVersion <= 0 {
		return errors.New("Godot pilot report protocol, ABI, or benchmark identity is incomplete")
	}
	if identity.Resolution.Width <= 0 || identity.Resolution.Height <= 0 {
		return errors.New("Godot pilot report resolution must be positive")
	}
	if err := identity.ViewDistance.validate("identity.view_distance"); err != nil {
		return err
	}
	if err := identity.Seed.validate("identity.seed"); err != nil {
		return err
	}
	if identity.WarmupSeconds <= 0 {
		return errors.New("Godot pilot report warmup_seconds must be positive")
	}
	if identity.Sample.StillSeconds <= 0 || identity.Sample.FlyingSeconds <= 0 ||
		identity.Sample.CooldownSeconds <= 0 || identity.Sample.MinimumGPUSamples <= 0 {
		return errors.New("Godot pilot report sample identity is incomplete")
	}
	if identity.ValidSampleCount <= 0 {
		return errors.New("Godot pilot report valid_sample_count must be positive")
	}
	return nil
}

func (sample godotPilotPercentiles) validate(name string) error {
	if sample.Comparable {
		if strings.TrimSpace(sample.Reason) != "" {
			return fmt.Errorf("Godot pilot report %s is comparable and must not carry a reason", name)
		}
		if sample.Samples == nil || *sample.Samples <= 0 {
			return fmt.Errorf("Godot pilot report %s samples must be positive when comparable", name)
		}
		if sample.P50MS == nil || sample.P95MS == nil || sample.P99MS == nil {
			return fmt.Errorf("Godot pilot report %s quantiles are incomplete", name)
		}
		if *sample.P50MS < 0 || *sample.P95MS < 0 || *sample.P99MS < 0 {
			return fmt.Errorf("Godot pilot report %s quantiles must be non-negative", name)
		}
		if *sample.P50MS > *sample.P95MS || *sample.P95MS > *sample.P99MS {
			return fmt.Errorf("Godot pilot report %s quantiles are not monotonic", name)
		}
		return nil
	}
	if strings.TrimSpace(sample.Reason) == "" {
		return fmt.Errorf("Godot pilot report %s must explain why it is not comparable", name)
	}
	if sample.Samples != nil || sample.P50MS != nil || sample.P95MS != nil || sample.P99MS != nil {
		return fmt.Errorf("Godot pilot report %s must omit numeric fields when it is not comparable", name)
	}
	return nil
}

func (sample godotPilotScalar) validate(name string) error {
	if sample.Comparable {
		if strings.TrimSpace(sample.Reason) != "" {
			return fmt.Errorf("Godot pilot report %s is comparable and must not carry a reason", name)
		}
		if sample.Value == nil {
			return fmt.Errorf("Godot pilot report %s value is missing", name)
		}
		return nil
	}
	if strings.TrimSpace(sample.Reason) == "" {
		return fmt.Errorf("Godot pilot report %s must explain why it is not comparable", name)
	}
	if sample.Value != nil {
		return fmt.Errorf("Godot pilot report %s must omit value when it is not comparable", name)
	}
	return nil
}

func requireComparableNonNegative(name string, sample godotPilotScalar) error {
	if !sample.Comparable || sample.Value == nil {
		return fmt.Errorf("Godot pilot report %s must be a comparable observation", name)
	}
	if *sample.Value < 0 {
		return fmt.Errorf("Godot pilot report %s must be non-negative", name)
	}
	return nil
}
