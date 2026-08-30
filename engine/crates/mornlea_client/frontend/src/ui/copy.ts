// 前端固定文案常量：与退役 egui 菜单的既有文案逐字沿承（设置页控件、暂停层
// 标题/按钮/注明行）。主菜单标题、版本行、错误行与按钮表由 Go 权威下发，不
// 在此表内；本表是前端文案唯一权威，改动经桥下行状态与 Go 侧钉值测试守护。
export const SETTINGS_TITLE = "设置";
export const SETTINGS_AUDIO_LABEL = "总音量";
export const SETTINGS_TEXTURE_LABEL = "材质包目录";
export const SETTINGS_TEXTURE_PLACEHOLDER = "留空使用内嵌材质";
export const SETTINGS_TEXTURE_HINT = "材质包路径保存后将在下次启动生效";
export const SETTINGS_WINDOW_LABEL = "窗口大小";
export const SETTINGS_DIRTY_HINT = "有未保存的更改";
export const SETTINGS_SAVE_LABEL = "保存";
export const SETTINGS_CANCEL_LABEL = "取消更改";
export const SETTINGS_BACK_LABEL = "返回";

export const PAUSE_TITLE = "已暂停";
export const PAUSE_BACK_LABEL = "返回游戏";
export const PAUSE_QUIT_TO_MENU_LABEL = "退回主菜单";
export const PAUSE_REMOTE_NOTE = "远程世界不会暂停，服务端仍在推进";

// 窗口预设值与 Go `UISettingsWindow` 1/2/3 互钉；展示文案沿承退役 egui 菜单。
export const WINDOW_SIZE_PRESETS: readonly { value: "640x360" | "960x540" | "1280x720"; label: string }[] = [
  { value: "640x360", label: "640 × 360" },
  { value: "960x540", label: "960 × 540" },
  { value: "1280x720", label: "1280 × 720" },
];
