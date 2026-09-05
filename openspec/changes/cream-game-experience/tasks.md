## 1. 前端面板与鼠标交互

- [x] 1.1 先为 schema/Go 解码、面板 DOM 操作、原生参与切换写失败测试；在 frontend、client 与 app 完成语义快照/事件、统一奶油面板、人物只读页、HUD 动效与鼠标选择，退役生产 GPU 面板调用；运行 pnpm typecheck/test/build、cargo test -p mornlea_client --locked、client/app 定点 race 与 audit。

## 2. 同源物品图标

- [x] 2.1 assets 为全部注册物品补齐透明原创图标与层映射，前端槽位/配方和非方块掉落消费同源图片；先覆盖全 ID/未知值/透明/破损区分测试，再运行 assets/render/client/app 定点 race 与前端检查。

## 3. 人物细节和步态

- [x] 3.1 保持六件身体与固定容量，增加原创分面人物纹理和前向面部；按水平路程校准步态并覆盖同 tick 插值/垂直/静止/回退/传送；运行 assets/render 定点 race、Rust shader/客户端测试与 audit。

## 4. 掉落散列

- [x] 4.1 测试先行实现同格稳定分配、有界随机扰动与高密度缩放分层；覆盖 1/4/16/32 堆、重排、旋转/浮动包围体、支撑与死亡渐显，运行 render 和 app 掉落定点 race。

## 5. 整体验证与文档

- [ ] 5.1 按 visual-baseline skill 补全前端所有面板与物品 fixtures、人物/掉落 world 和运动演示，查看产物后更新允许的基线，记录完整命令；同步局部指南及必要当前文档，确认无生产 GPU 面板 fallback。
- [ ] 5.2 执行 gofmt、make frontend-check、make test-race、make dev-check（六模块 vet/short 与 Rust fmt/clippy/tests，覆盖等价 rust-check 命令且不重复运行）、openspec validate --all --strict --no-interactive 与整分支独立审查；将结果和裁决写入 ledger，所有真实失败必须修复后复核，不推送或合并。
