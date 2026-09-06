package capture

import "testing"

// staticSuppressionRecorder 记录静态装配的抑制调用：内嵌接口只覆写装配实
// 际调用的方法，其余保持 nil（装配不碰它们）。
type staticSuppressionRecorder struct {
	SceneApplication
	calls      int
	suppressed bool
}

func (r *staticSuppressionRecorder) SetViewmodelSuppressed(suppressed bool) {
	r.calls++
	r.suppressed = suppressed
}

// TestStaticCaptureAssemblySuppressesViewmodel 锁定静态 runner 装配接线：
// 静态装配恰好打开一次抑制（motion/GIF runner 不经此函数）；抑制的段级语
// 义（确认满背包加标记仍无 viewmodel 段）由 app 包测试锁定，本测试只锁接线。
func TestStaticCaptureAssemblySuppressesViewmodel(t *testing.T) {
	recorder := &staticSuppressionRecorder{}
	suppressStaticViewmodel(recorder)
	if recorder.calls != 1 || !recorder.suppressed {
		t.Fatalf("静态装配抑制调用=%d 次 suppressed=%v，想要恰好 1 次 true",
			recorder.calls, recorder.suppressed)
	}
}
