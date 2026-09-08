package capture

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/core"
)

// heldItemsCatalogue 按稳定物品 ID 遍历全部注册项，包含空手与损坏形态。
func heldItemsCatalogue() []core.ItemStack {
	stacks := []core.ItemStack{{}}
	for id := core.ItemID(1); id < core.ItemIDMax; id++ {
		if !core.RegisteredItem(id) {
			continue
		}
		durability, _ := core.ItemMaxDurability(id)
		stacks = append(stacks, core.ItemStack{Item: id, Count: 1, Durability: durability})
	}
	return stacks
}

// heldItemsFrameCount 保留前后中立段，并为当前档位录制恰好一个采掘周期和完整攻击窗口。
func heldItemsFrameCount(stack core.ItemStack) int {
	_, period := render.ViewmodelSwingParams(render.ViewmodelTierOf(stack))
	return int(period) + 20
}

// applyHeldItemsFrame 只驱动抓帧专用的确认镜像与合成 tick，不改变线上触发语义。
func applyHeldItemsFrame(app SceneApplication, stack core.ItemStack, frame int) error {
	if frame < 0 || frame >= heldItemsFrameCount(stack) {
		return fmt.Errorf("手持目录帧越界: %d", frame)
	}
	_, period := render.ViewmodelSwingParams(render.ViewmodelTierOf(stack))
	app.SetServerTick(uint64(frame + 1))
	overlay := hud.MiningOverlay{}
	if frame >= 4 && frame < 4+int(period) {
		overlay = handSwingMotionOverlay(frame)
	}
	app.SetMiningOverlay(overlay)
	if frame == 8+int(period) {
		app.ObserveCombatHitForCapture(uint64(frame + 1))
	}
	return nil
}

// RunHeldItemsMotion 输出中立目录 GIF 与逐物品动作，单次只保留一件物品的有界原始帧。
// 同目录附带原尺寸审查帧、带 ID 的联系图和名称索引，不进入任何基线目录。
func RunHeldItemsMotion(app SceneApplication, outPath string) error {
	if outPath == "" {
		return fmt.Errorf("motion 输出路径为空")
	}
	if app == nil {
		return fmt.Errorf("motion 演示缺少应用实例")
	}
	stacks := heldItemsCatalogue()
	if len(stacks) > motionMaxFrames {
		return fmt.Errorf("手持目录超过录制预算: %d", len(stacks))
	}
	if err := prepareCaptureApplication(app); err != nil {
		return err
	}
	app.SetWorldTimeFrozen(true)
	defer app.SetWorldTimeFrozen(false)
	stage := captureScene{Name: "held-items-motion", WarmupFrames: 8, Prepare: prepareTargetBlockFeedback, Apply: applyHandMiningCaptureState}
	if _, err := captureSceneImage(app, stage); err != nil {
		return err
	}
	app.SetViewmodelSuppressed(false)
	dir := outPath + "-items"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	catalogue := make([]*image.NRGBA, 0, len(stacks))
	const columns = 4
	sheets := map[string]*image.NRGBA{}
	for _, name := range []string{"neutral", "mining-positive", "mining-negative", "attack-peak", "recovered"} {
		sheets[name] = image.NewNRGBA(image.Rect(0, 0, columns*320, ((len(stacks)+columns-1)/columns)*200))
	}
	var index strings.Builder
	index.WriteString("# Held items preview\n\nEach item GIF: 20 Hz; four neutral frames, one mining cycle, four neutral frames, confirmed attack and recovery.\nContact sheets use full-frame 50% thumbnails with stable item IDs; PNGs retain original 640x360 pixels.\n\n")
	for n, stack := range stacks {
		app.ResetViewmodel()
		app.ResetCombatFeedback()
		app.ResetItemPopupBaseline()
		if err := confirmHandCaptureBackpack(app, stack); err != nil {
			return err
		}
		frames, err := captureBoundedMotionFrames(heldItemsFrameCount(stack), func(frame int) (*image.NRGBA, error) {
			if err := applyHeldItemsFrame(app, stack, frame); err != nil {
				return nil, err
			}
			if _, err := app.RenderFrame(captureDrainMax); err != nil {
				return nil, err
			}
			return bgraToNRGBA(app.Renderer().Readback(), captureWidth, captureHeight), nil
		})
		if err != nil {
			return fmt.Errorf("手持 %d: %w", stack.Item, err)
		}
		prefix := fmt.Sprintf("item-%03d", stack.Item)
		data, err := encodeMotionGIF(frames, 5)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, prefix+".gif"), data, 0644); err != nil {
			return err
		}
		catalogue = append(catalogue, frames[0])
		_, period := render.ViewmodelSwingParams(render.ViewmodelTierOf(stack))
		points := map[string]int{"neutral": 0, "mining-positive": 4 + int(period+2)/4, "mining-negative": 4 + int(3*period+2)/4, "attack-peak": 8 + int(period) + 3, "recovered": len(frames) - 1}
		for name, frameIndex := range points {
			frame := frames[frameIndex]
			if err := writePNG(filepath.Join(dir, prefix+"-"+name+".png"), frame); err != nil {
				return err
			}
			sheet := sheets[name]
			x, y := (n%columns)*320, (n/columns)*200
			for py := 0; py < 180; py++ {
				for px := 0; px < 320; px++ {
					sheet.SetNRGBA(x+px, y+py, frame.NRGBAAt(px*2, py*2))
				}
			}
			draw.Draw(sheet, image.Rect(x, y+180, x+320, y+200), image.NewUniform(color.NRGBA{24, 24, 24, 255}), image.Point{}, draw.Src)
			drawer := font.Drawer{Dst: sheet, Src: image.White, Face: basicfont.Face7x13, Dot: fixed.P(x+6, y+195)}
			drawer.DrawString(fmt.Sprintf("%s / frame %02d", prefix, frameIndex))
		}
		name, _ := core.ItemDisplayName(stack.Item)
		if stack.Item == core.ItemNone {
			name = "空手"
		}
		fmt.Fprintf(&index, "- %s: %s (%d frames)\n", prefix, name, len(frames))
		fmt.Printf("已生成 held-items %s (%s)\n", prefix, name)
	}
	data, err := encodeMotionGIF(catalogue, 50)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return err
	}
	for name, sheet := range sheets {
		if err := writePNG(filepath.Join(dir, name+"-contact.png"), sheet); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, "README.md"), []byte(index.String()), 0644)
}
