package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

type godotPilotBaselineManifest struct {
	SchemaVersion            int                       `json:"schema_version"`
	GitCommit                string                    `json:"git_commit"`
	WorktreeState            string                    `json:"worktree_state"`
	Platform                 godotPilotPlatform        `json:"platform"`
	GPU                      godotPilotGPU             `json:"gpu"`
	Resolution               godotPilotResolution      `json:"resolution"`
	ViewDistance             int                       `json:"view_distance"`
	Scenes                   []string                  `json:"scenes"`
	Warmup                   godotPilotWarmup          `json:"warmup"`
	Sample                   godotPilotSample          `json:"sample"`
	ProtocolVersion          int                       `json:"protocol_version"`
	EngineABIVersion         int                       `json:"engine_abi_version"`
	ClientABIVersion         int                       `json:"client_abi_version"`
	ClientCoreABIMajor       int                       `json:"client_core_abi_major"`
	BenchmarkScenarioVersion int                       `json:"benchmark_scenario_version"`
	SourceReports            []godotPilotSourceReport  `json:"source_reports"`
	VisualBaselines          godotPilotVisualBaselines `json:"visual_baselines"`
}

type godotPilotPlatform struct {
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	Version string `json:"version"`
}

type godotPilotGPU struct {
	Name string `json:"name"`
	API  string `json:"api"`
}

type godotPilotResolution struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type godotPilotWarmup struct {
	Seconds int `json:"seconds"`
}

type godotPilotSample struct {
	StillSeconds      int `json:"still_seconds"`
	FlyingSeconds     int `json:"flying_seconds"`
	CooldownSeconds   int `json:"cooldown_seconds"`
	MinimumGPUSamples int `json:"minimum_gpu_samples"`
}

type godotPilotSourceReport struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type godotPilotVisualBaselines struct {
	Root        string `json:"root"`
	UIImages    int    `json:"ui_images"`
	WorldImages int    `json:"world_images"`
	MotionGIFs  int    `json:"motion_gifs"`
}

func readGodotPilotBaselineManifest(filePath string) (godotPilotBaselineManifest, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return godotPilotBaselineManifest{}, fmt.Errorf("open Godot pilot baseline manifest: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest godotPilotBaselineManifest
	if err := decoder.Decode(&manifest); err != nil {
		return godotPilotBaselineManifest{}, fmt.Errorf("decode Godot pilot baseline manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return godotPilotBaselineManifest{}, errors.New("decode Godot pilot baseline manifest: trailing JSON value")
		}
		return godotPilotBaselineManifest{}, fmt.Errorf("decode Godot pilot baseline manifest trailing data: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return godotPilotBaselineManifest{}, err
	}
	return manifest, nil
}

// Validate rejects incomplete run identity before any baseline can influence the
// migration decision. Performance values remain informational elsewhere.
func (manifest godotPilotBaselineManifest) Validate() error {
	if manifest.SchemaVersion != 1 {
		return fmt.Errorf("Godot pilot baseline schema_version=%d, want 1", manifest.SchemaVersion)
	}
	if !validHexDigest(manifest.GitCommit, 20) {
		return errors.New("Godot pilot baseline git_commit must be a full 40-character hexadecimal commit")
	}
	if manifest.WorktreeState != "clean" && manifest.WorktreeState != "dirty" {
		return fmt.Errorf("Godot pilot baseline worktree_state=%q, want clean or dirty", manifest.WorktreeState)
	}
	if manifest.Platform.OS != "darwin" && manifest.Platform.OS != "windows" && manifest.Platform.OS != "linux" {
		return fmt.Errorf("Godot pilot baseline platform.os=%q is not an approved desktop target", manifest.Platform.OS)
	}
	if strings.TrimSpace(manifest.Platform.Arch) == "" || strings.TrimSpace(manifest.Platform.Version) == "" {
		return errors.New("Godot pilot baseline platform identity is incomplete")
	}
	if strings.TrimSpace(manifest.GPU.Name) == "" || strings.TrimSpace(manifest.GPU.API) == "" {
		return errors.New("Godot pilot baseline GPU identity is incomplete")
	}
	if manifest.Resolution.Width <= 0 || manifest.Resolution.Height <= 0 {
		return errors.New("Godot pilot baseline resolution must be positive")
	}
	if manifest.ViewDistance < 2 || manifest.ViewDistance > 64 {
		return fmt.Errorf("Godot pilot baseline view_distance=%d is outside 2..64", manifest.ViewDistance)
	}
	if err := validateGodotPilotScenes(manifest.Scenes); err != nil {
		return err
	}
	if manifest.Warmup.Seconds <= 0 {
		return errors.New("Godot pilot baseline warmup.seconds must be positive")
	}
	if manifest.Sample.StillSeconds <= 0 || manifest.Sample.FlyingSeconds <= 0 ||
		manifest.Sample.CooldownSeconds <= 0 || manifest.Sample.MinimumGPUSamples <= 0 {
		return errors.New("Godot pilot baseline sample identity is incomplete")
	}
	if manifest.ProtocolVersion <= 0 || manifest.EngineABIVersion <= 0 ||
		manifest.ClientABIVersion <= 0 || manifest.ClientCoreABIMajor <= 0 ||
		manifest.BenchmarkScenarioVersion <= 0 {
		return errors.New("Godot pilot baseline protocol, ABI, or benchmark identity is incomplete")
	}
	if err := validateGodotPilotReports(manifest.SourceReports); err != nil {
		return err
	}
	if strings.TrimSpace(manifest.VisualBaselines.Root) == "" ||
		manifest.VisualBaselines.UIImages <= 0 || manifest.VisualBaselines.WorldImages <= 0 ||
		manifest.VisualBaselines.MotionGIFs <= 0 {
		return errors.New("Godot pilot visual-baseline identity is incomplete")
	}
	return nil
}

func validateGodotPilotScenes(scenes []string) error {
	if len(scenes) == 0 {
		return errors.New("Godot pilot baseline scenes must not be empty")
	}
	seen := make(map[string]struct{}, len(scenes))
	for _, scene := range scenes {
		scene = strings.TrimSpace(scene)
		if scene == "" {
			return errors.New("Godot pilot baseline scene identity must not be empty")
		}
		if _, exists := seen[scene]; exists {
			return fmt.Errorf("Godot pilot baseline scene %q is duplicated", scene)
		}
		seen[scene] = struct{}{}
	}
	return nil
}

func validateGodotPilotReports(reports []godotPilotSourceReport) error {
	if len(reports) == 0 {
		return errors.New("Godot pilot baseline source_reports must not be empty")
	}
	seen := make(map[string]struct{}, len(reports))
	for _, report := range reports {
		cleaned := path.Clean(report.Path)
		if cleaned == "." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
			return fmt.Errorf("Godot pilot baseline report path %q must stay repository-relative", report.Path)
		}
		if _, exists := seen[cleaned]; exists {
			return fmt.Errorf("Godot pilot baseline report path %q is duplicated", cleaned)
		}
		seen[cleaned] = struct{}{}
		if !validHexDigest(report.SHA256, 32) {
			return fmt.Errorf("Godot pilot baseline report %q has an invalid SHA-256", cleaned)
		}
	}
	return nil
}

func validHexDigest(value string, byteLength int) bool {
	if len(value) != byteLength*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == byteLength
}
