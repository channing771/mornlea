package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestGodotPilotVisualCompareMarksUnmappedBaselinesNotCovered(t *testing.T) {
	root := t.TempDir()
	golden := filepath.Join(root, "testdata", "visual-golden")
	run := filepath.Join(root, "build", "visual", "godot-pilot", "run-1")
	writeTestPNG(t, filepath.Join(golden, "world", "terrain-noon.png"), 4, 4, 10)
	writeTestPNG(t, filepath.Join(golden, "ui", "hud-hotbar-first.png"), 4, 4, 20)
	if err := os.MkdirAll(filepath.Join(golden, "motion"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(golden, "motion", "break-burst.gif"), []byte("GIF89a"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := compareGodotPilotVisuals(run, golden, godotPilotVisualSemantics{SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, item := range report.Classifications {
		found[item.TrackedBaseline] = item.Classification
	}
	if found["world/terrain-noon.png"] != godotPilotVisualNotCovered {
		t.Fatalf("world baseline classification=%q", found["world/terrain-noon.png"])
	}
	if found["ui/hud-hotbar-first.png"] != godotPilotVisualNotCovered {
		t.Fatalf("ui baseline classification=%q", found["ui/hud-hotbar-first.png"])
	}
	if found["motion/break-burst.gif"] != godotPilotVisualHumanReview {
		t.Fatalf("motion classification=%q", found["motion/break-burst.gif"])
	}
}

func TestGodotPilotVisualCompareClassifiesMappedPixelsWithoutWritingGoldens(t *testing.T) {
	root := t.TempDir()
	golden := filepath.Join(root, "testdata", "visual-golden")
	run := filepath.Join(root, "build", "visual", "godot-pilot", "run-1")
	writeTestPNG(t, filepath.Join(golden, "world", "terrain-noon.png"), 4, 4, 10)
	writeTestPNG(t, filepath.Join(run, "world", "terrain-settled.png"), 4, 4, 10)
	if err := os.MkdirAll(filepath.Join(golden, "ui"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(golden, "motion"), 0o755); err != nil {
		t.Fatal(err)
	}
	table := godotPilotVisualSemantics{
		SchemaVersion: 1,
		Mappings: []godotPilotVisualMapping{{
			PilotCapture:    "world/terrain-settled.png",
			SemanticClass:   godotPilotVisualClassWorld,
			TrackedBaseline: "world/terrain-noon.png",
		}},
	}
	report, err := compareGodotPilotVisuals(run, golden, table)
	if err != nil {
		t.Fatal(err)
	}
	var world godotPilotVisualClassResult
	for _, item := range report.Classifications {
		if item.TrackedBaseline == "world/terrain-noon.png" {
			world = item
		}
	}
	if world.Classification != godotPilotVisualEquivalent {
		t.Fatalf("mapped identical capture classification=%q", world.Classification)
	}
	writeTestPNG(t, filepath.Join(run, "world", "terrain-settled.png"), 4, 4, 200)
	report, err = compareGodotPilotVisuals(run, golden, table)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range report.Classifications {
		if item.TrackedBaseline == "world/terrain-noon.png" && item.Classification != godotPilotVisualOutsideThreshold {
			t.Fatalf("divergent capture classification=%q", item.Classification)
		}
	}
	if _, err := os.Stat(filepath.Join(golden, "godot")); !os.IsNotExist(err) {
		t.Fatal("compare must not create a renderer-specific golden class")
	}
}

func TestGodotPilotVisualCompareRejectsTrackedGoldenRunDir(t *testing.T) {
	_, err := compareGodotPilotVisuals("testdata/visual-golden/world", "testdata/visual-golden", godotPilotVisualSemantics{SchemaVersion: 1})
	if err == nil {
		t.Fatal("a tracked golden run directory must be rejected")
	}
}

func writeTestPNG(t *testing.T, path string, width, height int, gray uint8) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = gray
		img.Pix[i+1] = gray
		img.Pix[i+2] = gray
		img.Pix[i+3] = 255
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}
