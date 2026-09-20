package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	godotPilotVisualClassUI     = "ui"
	godotPilotVisualClassWorld  = "world"
	godotPilotVisualClassMotion = "motion"

	godotPilotVisualEquivalent       = "equivalent"
	godotPilotVisualOutsideThreshold = "outside-threshold"
	godotPilotVisualNotCovered       = "not-covered"
	godotPilotVisualHumanReview      = "human-review-only"
	godotPilotVisualSizeMismatch     = "size-mismatch"

	godotPilotMaxChannelDelta   = 2
	godotPilotMaxDiffPixelRatio = 0.0001
)

type godotPilotVisualSemantics struct {
	SchemaVersion int                       `json:"schema_version"`
	Mappings      []godotPilotVisualMapping `json:"mappings"`
}

type godotPilotVisualMapping struct {
	PilotCapture    string `json:"pilot_capture"`
	SemanticClass   string `json:"semantic_class"`
	TrackedBaseline string `json:"tracked_baseline"`
}

type godotPilotVisualCompareReport struct {
	SchemaVersion   int                           `json:"schema_version"`
	RunDir          string                        `json:"run_dir"`
	GoldenRoot      string                        `json:"golden_root"`
	Threshold       godotPilotVisualThreshold     `json:"threshold"`
	Classifications []godotPilotVisualClassResult `json:"classifications"`
}

type godotPilotVisualThreshold struct {
	MaxChannelDelta   int     `json:"max_channel_delta"`
	MaxDiffPixelRatio float64 `json:"max_diff_pixel_ratio"`
}

type godotPilotVisualClassResult struct {
	SemanticClass   string  `json:"semantic_class"`
	TrackedBaseline string  `json:"tracked_baseline"`
	PilotCapture    string  `json:"pilot_capture,omitempty"`
	Classification  string  `json:"classification"`
	Reason          string  `json:"reason,omitempty"`
	MaxChannelDelta int     `json:"max_channel_delta,omitempty"`
	DiffPixelRatio  float64 `json:"diff_pixel_ratio,omitempty"`
}

func readGodotPilotVisualSemantics(filePath string) (godotPilotVisualSemantics, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return godotPilotVisualSemantics{}, fmt.Errorf("open Godot visual semantics: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var table godotPilotVisualSemantics
	if err := decoder.Decode(&table); err != nil {
		return godotPilotVisualSemantics{}, fmt.Errorf("decode Godot visual semantics: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return godotPilotVisualSemantics{}, errors.New("decode Godot visual semantics: trailing JSON value")
		}
		return godotPilotVisualSemantics{}, fmt.Errorf("decode Godot visual semantics trailing data: %w", err)
	}
	if table.SchemaVersion != 1 {
		return godotPilotVisualSemantics{}, fmt.Errorf("Godot visual semantics schema_version=%d, want 1", table.SchemaVersion)
	}
	seenCapture := make(map[string]struct{}, len(table.Mappings))
	for _, mapping := range table.Mappings {
		if err := validateVisualRelative(mapping.PilotCapture, "pilot_capture"); err != nil {
			return godotPilotVisualSemantics{}, err
		}
		if err := validateVisualRelative(mapping.TrackedBaseline, "tracked_baseline"); err != nil {
			return godotPilotVisualSemantics{}, err
		}
		if mapping.SemanticClass != godotPilotVisualClassUI && mapping.SemanticClass != godotPilotVisualClassWorld {
			return godotPilotVisualSemantics{}, fmt.Errorf("Godot visual mapping class %q is not pixel-comparable", mapping.SemanticClass)
		}
		if _, exists := seenCapture[mapping.PilotCapture]; exists {
			return godotPilotVisualSemantics{}, fmt.Errorf("Godot visual mapping duplicates pilot_capture %q", mapping.PilotCapture)
		}
		seenCapture[mapping.PilotCapture] = struct{}{}
	}
	return table, nil
}

func validateVisualRelative(relative, field string) error {
	cleaned := path.Clean(relative)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || path.IsAbs(cleaned) {
		return fmt.Errorf("Godot visual %s %q must stay repository-relative", field, relative)
	}
	if strings.Contains(cleaned, "testdata/visual-golden/godot") {
		return errors.New("renderer-specific tracked visual classes are forbidden")
	}
	return nil
}

func compareGodotPilotVisuals(runDir, goldenRoot string, table godotPilotVisualSemantics) (godotPilotVisualCompareReport, error) {
	if err := rejectTrackedGoldenWrite(runDir); err != nil {
		return godotPilotVisualCompareReport{}, err
	}
	report := godotPilotVisualCompareReport{
		SchemaVersion: 1,
		RunDir:        filepath.ToSlash(runDir),
		GoldenRoot:    filepath.ToSlash(goldenRoot),
		Threshold: godotPilotVisualThreshold{
			MaxChannelDelta:   godotPilotMaxChannelDelta,
			MaxDiffPixelRatio: godotPilotMaxDiffPixelRatio,
		},
	}
	mapped := make(map[string]godotPilotVisualMapping, len(table.Mappings))
	for _, mapping := range table.Mappings {
		mapped[filepath.ToSlash(mapping.TrackedBaseline)] = mapping
	}
	for _, class := range []string{godotPilotVisualClassUI, godotPilotVisualClassWorld} {
		entries, err := listGoldenFiles(goldenRoot, class, ".png")
		if err != nil {
			return godotPilotVisualCompareReport{}, err
		}
		for _, relative := range entries {
			mapping, ok := mapped[relative]
			if !ok {
				report.Classifications = append(report.Classifications, godotPilotVisualClassResult{
					SemanticClass:   class,
					TrackedBaseline: relative,
					Classification:  godotPilotVisualNotCovered,
					Reason:          "the Godot pilot has no same-semantics capture for this tracked baseline",
				})
				continue
			}
			result, err := classifyMappedCapture(runDir, goldenRoot, mapping)
			if err != nil {
				return godotPilotVisualCompareReport{}, err
			}
			report.Classifications = append(report.Classifications, result)
		}
	}
	motion, err := listGoldenFiles(goldenRoot, godotPilotVisualClassMotion, ".gif")
	if err != nil {
		return godotPilotVisualCompareReport{}, err
	}
	for _, relative := range motion {
		report.Classifications = append(report.Classifications, godotPilotVisualClassResult{
			SemanticClass:   godotPilotVisualClassMotion,
			TrackedBaseline: relative,
			Classification:  godotPilotVisualHumanReview,
			Reason:          "motion GIFs are human-review evidence and are not pixel-compared",
		})
	}
	return report, nil
}

func classifyMappedCapture(runDir, goldenRoot string, mapping godotPilotVisualMapping) (godotPilotVisualClassResult, error) {
	result := godotPilotVisualClassResult{
		SemanticClass:   mapping.SemanticClass,
		TrackedBaseline: mapping.TrackedBaseline,
		PilotCapture:    mapping.PilotCapture,
	}
	pilotPath := filepath.Join(runDir, filepath.FromSlash(mapping.PilotCapture))
	if _, err := os.Stat(pilotPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result.Classification = godotPilotVisualNotCovered
			result.Reason = "the mapped pilot capture is missing from the run directory"
			return result, nil
		}
		return godotPilotVisualClassResult{}, err
	}
	got, err := readNRGBA(pilotPath)
	if err != nil {
		return godotPilotVisualClassResult{}, err
	}
	want, err := readNRGBA(filepath.Join(goldenRoot, filepath.FromSlash(mapping.TrackedBaseline)))
	if err != nil {
		return godotPilotVisualClassResult{}, err
	}
	if got.Bounds() != want.Bounds() {
		result.Classification = godotPilotVisualSizeMismatch
		result.Reason = fmt.Sprintf("pilot %v vs baseline %v", got.Bounds(), want.Bounds())
		return result, nil
	}
	diff := diffNRGBA(got, want)
	result.MaxChannelDelta = diff.maxChannelDelta
	result.DiffPixelRatio = diff.diffPixelRatio
	if diff.maxChannelDelta <= godotPilotMaxChannelDelta && diff.diffPixelRatio <= godotPilotMaxDiffPixelRatio {
		result.Classification = godotPilotVisualEquivalent
		return result, nil
	}
	result.Classification = godotPilotVisualOutsideThreshold
	result.Reason = "pixel difference exceeds the existing dual threshold and remains Go/No-Go evidence"
	return result, nil
}

func listGoldenFiles(goldenRoot, class, suffix string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(goldenRoot, class))
	if err != nil {
		return nil, fmt.Errorf("list %s goldens: %w", class, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		names = append(names, path.Join(class, entry.Name()))
	}
	return names, nil
}

func rejectTrackedGoldenWrite(runDir string) error {
	cleaned := filepath.ToSlash(filepath.Clean(runDir))
	if strings.Contains(cleaned, "testdata/visual-golden") {
		return errors.New("Godot visual compare must not write or read a run directory under testdata/visual-golden")
	}
	if !strings.Contains(cleaned, "build/visual/godot-pilot") {
		return errors.New("Godot visual compare run directory must stay under build/visual/godot-pilot")
	}
	return nil
}

func readNRGBA(path string) (*image.NRGBA, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoded, err := png.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	if nrgba, ok := decoded.(*image.NRGBA); ok {
		return nrgba, nil
	}
	bounds := decoded.Bounds()
	converted := image.NewNRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			converted.Set(x, y, decoded.At(x, y))
		}
	}
	return converted, nil
}

type nrgbaDiff struct {
	maxChannelDelta int
	diffPixelRatio  float64
}

func diffNRGBA(got, want *image.NRGBA) nrgbaDiff {
	bounds := got.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	result := nrgbaDiff{}
	diffPixels := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			gi, wi := got.PixOffset(x, y), want.PixOffset(x, y)
			maxDelta := 0
			for channel := 0; channel < 3; channel++ {
				delta := int(got.Pix[gi+channel]) - int(want.Pix[wi+channel])
				if delta < 0 {
					delta = -delta
				}
				if delta > maxDelta {
					maxDelta = delta
				}
			}
			if maxDelta > result.maxChannelDelta {
				result.maxChannelDelta = maxDelta
			}
			if maxDelta > 0 {
				diffPixels++
			}
		}
	}
	if width*height > 0 {
		result.diffPixelRatio = float64(diffPixels) / float64(width*height)
	}
	return result
}

func writeGodotPilotVisualCompareReport(filePath string, report godotPilotVisualCompareReport) error {
	if err := rejectTrackedGoldenWrite(filePath); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0o600)
}
