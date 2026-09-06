package capture

import (
	"testing"
)

// 本文件锁定跨场景清场的双手接线：`resetCapturePresentation` 必须直达
// viewmodel 重置；删除该调用行本测试变红（应用层的首帧锁定测试只覆盖重置
// 本身的语义，不覆盖清场是否调用它）。

// resetViewmodelRecorder 包装真实场景应用，只记录 `ResetViewmodel` 的调用
// 次数，其余全部直通：清场的前置非空要求与镜像清理仍走真实装配。
type resetViewmodelRecorder struct {
	SceneApplication
	calls int
}

func (r *resetViewmodelRecorder) ResetViewmodel() {
	r.calls++
	r.SceneApplication.ResetViewmodel()
}

// TestResetCapturePresentationResetsViewmodel 锁定清场直达 viewmodel 重置：
// 包裹计数后走一遍公共清场，重置恰被调用一次。
func TestResetCapturePresentationResetsViewmodel(t *testing.T) {
	app := newCaptureAICompanionState()
	recorder := &resetViewmodelRecorder{SceneApplication: app}
	if err := resetCapturePresentation(recorder); err != nil {
		t.Fatalf("清场: %v", err)
	}
	if recorder.calls != 1 {
		t.Fatalf("ResetViewmodel 调用 = %d，想要 1（清场必须直达双手重置）", recorder.calls)
	}
}
