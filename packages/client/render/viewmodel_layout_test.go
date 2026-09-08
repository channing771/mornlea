package render

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestViewmodelHUDFrontendParity(t *testing.T) {
	const frontend = "../../engine/crates/mornlea_client/frontend/src/"
	geometry, err := os.ReadFile(frontend + "hud/geometry.ts")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := os.ReadFile(frontend + "tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	css, err := os.ReadFile(frontend + "hud/hud.css")
	if err != nil {
		t.Fatal(err)
	}
	compact := regexp.MustCompile(`\s+`).ReplaceAllString(string(geometry), "")
	for _, formula := range []string{
		"HOTBAR_ROW_WIDTH=HOTBAR_SLOTS*HOTBAR_SLOT_SIZE+(HOTBAR_SLOTS-1)*HOTBAR_SLOT_GAP;",
		"DESIGN_WIDTH=HOTBAR_ROW_WIDTH+2*HOTBAR_PANEL_PADDING;",
		"DESIGN_HEIGHT=HOTBAR_BOTTOM_MARGIN+HOTBAR_SLOT_SIZE+HOTBAR_PANEL_PADDING+STATUS_HOTBAR_GAP+2*(STATUS_BAR_GAP+STATUS_ICON_SIZE)+PROGRESS_TRACK_GAP+PROGRESS_TRACK_HEIGHT+POPUP_TRACK_GAP+POPUP_ROW_HEIGHT;",
	} {
		if !strings.Contains(compact, formula) {
			t.Fatalf("frontend layout formula changed: %s", formula)
		}
	}
	values := map[string]float32{}
	for _, name := range []string{"HOTBAR_SLOTS", "HOTBAR_SLOT_SIZE", "HOTBAR_SLOT_GAP", "HOTBAR_PANEL_PADDING", "HOTBAR_BOTTOM_MARGIN", "STATUS_HOTBAR_GAP", "STATUS_BAR_GAP", "STATUS_ICON_SIZE", "EDGE_MARGIN", "PROGRESS_TRACK_GAP", "PROGRESS_TRACK_HEIGHT", "POPUP_TRACK_GAP", "POPUP_ROW_HEIGHT"} {
		match := regexp.MustCompile(`export const ` + name + ` = ([0-9]+);`).FindSubmatch(geometry)
		if len(match) != 2 {
			t.Fatalf("missing frontend constant %s", name)
		}
		n, _ := strconv.ParseFloat(string(match[1]), 32)
		values[name] = float32(n)
	}
	width := values["HOTBAR_SLOTS"]*values["HOTBAR_SLOT_SIZE"] + (values["HOTBAR_SLOTS"]-1)*values["HOTBAR_SLOT_GAP"] + 2*values["HOTBAR_PANEL_PADDING"]
	status := values["HOTBAR_BOTTOM_MARGIN"] + values["HOTBAR_SLOT_SIZE"] + values["HOTBAR_PANEL_PADDING"] + values["STATUS_HOTBAR_GAP"] + 2*values["STATUS_ICON_SIZE"] + values["STATUS_BAR_GAP"]
	height := status + values["STATUS_BAR_GAP"] + values["PROGRESS_TRACK_GAP"] + values["PROGRESS_TRACK_HEIGHT"] + values["POPUP_TRACK_GAP"] + values["POPUP_ROW_HEIGHT"]
	if width != viewmodelHUDWidth || height != viewmodelHUDHeight || status != viewmodelHUDStatusHeight || values["EDGE_MARGIN"] != viewmodelHUDEdge {
		t.Fatalf("frontend HUD changed: width=%v height=%v status=%v", width, height, status)
	}
	for _, token := range []string{"--hud-selected-lift: 5px;", "--hud-selected-scale: 1.08;", "--hud-select-border: 3px;"} {
		if !strings.Contains(string(tokens), token) {
			t.Fatalf("selected-slot envelope changed: %s", token)
		}
	}
	for _, rule := range []string{"var(--hud-selected-lift)", "scale(var(--hud-selected-scale))", "outline-offset: 0;"} {
		if !strings.Contains(string(css), rule) {
			t.Fatalf("selected-slot placement changed: %s", rule)
		}
	}
	if !strings.Contains(string(geometry), "Math.min(availableWidth / DESIGN_WIDTH, availableHeight / DESIGN_HEIGHT, 1)") {
		t.Fatal("HUD scale formula changed")
	}
}
