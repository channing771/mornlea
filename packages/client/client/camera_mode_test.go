package client_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/client/client"
)

func TestCameraModeValuesMatchSpec(t *testing.T) {
	if client.CameraFirstPerson != 0 || client.CameraThirdPersonBack != 1 || client.CameraThirdPersonFront != 2 {
		t.Fatalf("视角模式取值 = %d/%d/%d，想要 0/1/2",
			client.CameraFirstPerson, client.CameraThirdPersonBack, client.CameraThirdPersonFront)
	}
}

func TestCameraModeCyclesFirstBackFrontFirst(t *testing.T) {
	mode := client.CameraFirstPerson
	for step, want := range []client.CameraMode{
		client.CameraThirdPersonBack, client.CameraThirdPersonFront, client.CameraFirstPerson,
	} {
		mode = mode.Next()
		if mode != want {
			t.Fatalf("第 %d 次切换后 = %d，想要 %d", step+1, mode, want)
		}
	}
}

func TestCameraModeValidRejectsOutOfRange(t *testing.T) {
	for _, mode := range []client.CameraMode{
		client.CameraFirstPerson, client.CameraThirdPersonBack, client.CameraThirdPersonFront,
	} {
		if !mode.Valid() {
			t.Fatalf("合法模式 %d 被判非法", mode)
		}
	}
	for _, mode := range []client.CameraMode{-1, 3, 99} {
		if mode.Valid() {
			t.Fatalf("越界模式 %d 被判合法", mode)
		}
	}
}
