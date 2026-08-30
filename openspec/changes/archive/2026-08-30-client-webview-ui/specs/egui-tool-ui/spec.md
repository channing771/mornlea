# egui-tool-ui Delta

## REMOVED Requirements

### Requirement: 交互客户端启动停留在主菜单

### Requirement: 主菜单参考经典标题画面布局

### Requirement: 禁用按钮不产生事件

### Requirement: 进入游戏执行延迟的世界装配

### Requirement: 退出游戏关闭客户端

### Requirement: 菜单期间游戏输入不生效

### Requirement: egui 集成的技术边界

### Requirement: client ABI v9 结构化设置事件扩展

### Requirement: 无 UI 帧时 egui 零参与

### Requirement: 菜单字体只经 ABI 上传一次

### Requirement: 调试面板 layout v3 段

### Requirement: 调试面板事件上行

### Requirement: 游戏内 Esc 打开暂停覆盖层

### Requirement: 本地权威暂停门冻结模拟并可恢复

### Requirement: 退回主菜单安全拆解会话

> 本 capability 整体退役：全部行为语义平移进新 capability `webview-menu-ui`（见本 change 对应 delta），呈现技术从 egui 即时模式替换为进程内 WKWebView + Vite/TS/React。`egui`/`egui-wgpu` 依赖随之删除。
