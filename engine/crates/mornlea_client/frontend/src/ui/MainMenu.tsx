// 主菜单屏：标题/按钮列/错误行/版本行全部由 Go 下行状态权威驱动，前端只做
// 呈现与点击回传（动作 id 与 Go menuAction* 常量互钉，见 bridge/schema.json）。
// 按钮呈现走 pixel.tsx 桥接的 PixelButton：仅换呈现面，disabled/点击回传
// 语义与改造前逐项一致。
import type { MenuState, UplinkEvent } from "../bridge/client";
import { PixelButton } from "./pixel";

export interface MainMenuProps {
  menu: MenuState;
  onEvent: (event: UplinkEvent) => void;
}

export function MainMenu({ menu, onEvent }: MainMenuProps) {
  return (
    <section className="menu-screen">
      <h1 className="menu-title">{menu.title}</h1>
      <div className="menu-buttons">
        {menu.buttons.map((button) => (
          <PixelButton
            key={button.id}
            type="button"
            className="menu-button"
            disabled={!button.enabled}
            onClick={() => {
              onEvent({ type: "action", id: button.id });
            }}
          >
            {button.label}
          </PixelButton>
        ))}
      </div>
      {menu.error !== "" && (
        <p className="menu-error" role="alert">
          {menu.error}
        </p>
      )}
      <p className="menu-version">{menu.version}</p>
    </section>
  );
}
