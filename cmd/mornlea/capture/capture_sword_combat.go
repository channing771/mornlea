package capture

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	application "github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
)

// captureSwordCombatPlayerID 是合法的 UUIDv4，占位远端受击玩家。
var captureSwordCombatPlayerID = core.PlayerID{6: 0x40, 8: 0x80, 15: 0x77}

// captureSwordCombatInitialPos 是受击前的权威位置。
var captureSwordCombatInitialPos = mgl32.Vec3{5.5, 1, 4}

// captureSwordCombatKnockedPos 是 0.35 水平击退后的位置。
var captureSwordCombatKnockedPos = captureSwordCombatInitialPos.Add(mgl32.Vec3{0, 0, -0.35})

// prepareSwordCombat 复用既有开阔草地与空气邻域 helper，不复制地形构建器。
func prepareSwordCombat(app SceneApplication) error {
	return prepareAICompanion(app)
}

// applySwordCombatCaptureState 注入固定相机、选中非满耐久铁剑、远端玩家以及命中标记。
func applySwordCombatCaptureState(app SceneApplication) error {
	if err := resetCapturePresentation(app); err != nil {
		return err
	}
	app.SetWorldTimeTicks(6000)
	// 这是静态确认状态，不是选中变化；丢弃前序场景的选中基线，避免装入铁剑时触发弹条。
	app.ResetItemPopupBaseline()
	*app.Camera() = client.Camera{
		Pos: mgl32.Vec3{5.5, 3.2, 9.5}, Yaw: 0, Pitch: -0.05,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	app.SetCenter(application.CameraChunk(app.Camera().Pos))
	app.SetBlockTargetReset(false)
	if app.Panel() != nil {
		app.Panel().SetVisible(false)
	}
	// 非满耐久铁剑 Durability 125，选中态。
	inv := core.Inventory{}
	inv.Hotbar.Selected = 2
	inv.Hotbar.Slots[2] = core.ItemStack{Item: core.ItemIronSword, Count: 1, Durability: 125}
	if err := app.Inventory().Apply(network.InventoryState{Inventory: inv}); err != nil {
		return fmt.Errorf("装入剑场景背包: %w", err)
	}
	app.SetInventoryOpen(false)
	app.SetInventorySource(-1)
	if app.RemotePlayers() == nil {
		return fmt.Errorf("sword-combat 需要远端玩家追踪器")
	}
	spawn := network.RemotePlayerSpawn{
		PlayerID:    captureSwordCombatPlayerID,
		DisplayName: "Target",
		ServerTick:  1,
		Position:    captureSwordCombatInitialPos,
	}
	if err := app.RemotePlayers().Apply(spawn); err != nil {
		return fmt.Errorf("装入受击远端玩家 spawn: %w", err)
	}
	states := network.RemotePlayerStates{
		ServerTick: 2,
		Players: []network.RemotePlayerState{{
			PlayerID:  captureSwordCombatPlayerID,
			Dimension: core.Overworld,
			Position:  captureSwordCombatKnockedPos,
			Yaw:       0,
			Pitch:     0,
		}},
	}
	if err := app.RemotePlayers().Apply(states); err != nil {
		return fmt.Errorf("装入受击远端玩家 state: %w", err)
	}
	app.ArmCombatMarker()
	return nil
}

// pinSwordCombatVolatile 在收敛后重新武装 marker，保证最终帧落在 6 帧窗口内。
func pinSwordCombatVolatile(app SceneApplication) error {
	app.ArmCombatMarker()
	return nil
}
