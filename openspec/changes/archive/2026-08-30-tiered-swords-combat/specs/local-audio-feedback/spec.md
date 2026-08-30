## ADDED Requirements

### Requirement: CombatHit 以严格递增确认触发固定原创 cue

图形客户端 SHALL 只在收到本会话 `ServerTick` 严格大于上一条已接受 combat 确认的合法 `CombatHit` 时播放恰好一次 `CueCombatHit`，并 MUST 使用独立于全局 application server tick 的 combat 去重状态。输入、预测射线、目标 health 镜像、inventory 耐久变化、重复或陈旧确认 MUST NOT 播放该 cue。`CueCombatHit` MUST 由既有程序化 synth 和预分配播放队列生成，固定参数为 1323 samples、520→180 Hz、amplitude 10500；little-endian PCM SHA-256 MUST 为 `17752cdda0232ebb88b0e6db1e39fa4a4889e5469bac0c28a07044b677710dae`。音频设备不可用 MUST 继续无声降级，不得影响 hit marker 或权威命中事实。

#### Scenario: 新确认播放恰好一次固定 cue
- **GIVEN** combat 去重状态尚未接受任何确认
- **WHEN** 客户端收到合法 `CombatHit{ServerTick:1}`
- **THEN** MUST 播放恰好一次 `CueCombatHit`，生成 PCM 的样本数、频率、幅度和 SHA-256 MUST 与固定值完全一致

#### Scenario: 重复与陈旧确认无声
- **GIVEN** 客户端已经接受 `ServerTick=2` 的 combat 确认
- **WHEN** 随后收到 tick 2、1 或 0 的确认
- **THEN** MUST 不播放 cue，也 MUST 不重新武装反馈

#### Scenario: 非确认状态变化无声
- **GIVEN** 玩家持续 primary input，目标 health 或选中剑耐久镜像发生变化，但没有新鲜 `CombatHit`
- **WHEN** 客户端处理这些输入与镜像
- **THEN** MUST 不播放 `CueCombatHit`

#### Scenario: 音频不可用不吞命中反馈
- **GIVEN** 客户端使用无声音频路径
- **WHEN** 收到严格递增的合法 `CombatHit`
- **THEN** cue 播放 MUST 无声降级，combat 确认与 HUD marker MUST 仍正常接受
