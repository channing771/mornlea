# 设计 — 客户端烘焙队列近处优先

## 数据所有权与并发

- 优先级中心由 `Application` 每帧从相机姿态推导（区块坐标中心），经既有 `Mesher.Schedule` 调用面传入——mesher 仍是队列唯一所有者，worker 消费路径不变；中心在同一渲染帧内恒定，无跨 goroutine 竞争。
- `readySectionHeap` 的键从 `sectionKey` 改为 `(dist², sectionKey)` 复合键；中心变更时整体重建堆一次（O(n)，跨界频率下成本可忽略），同中心内 push/pop 维持堆序。
- 上传调度（`SectionScheduler.FlushUploads` 已近处优先）与 `DropOutside` 语义不动。

## 受影响文件

- `packages/client/client/mesher_ready_queue.go`（复合键与中心重建）
- `packages/client/client/mesher.go`（Schedule 签名与中心传递）
- `packages/client/client/mesher_ready_queue_test.go`（改钉新序：距离升序 + 字典序平局 + 中心移动重排）
- `packages/client/cmd/mornlea/app/app_frame.go`（传中心，若签名变化）

## 取舍

- **惰性重建堆 vs 每帧重算**：中心变化时一次 O(n) 重建，避免每帧 O(n log n) 全量重排；中心逐帧小幅漂移时可按区块粒度比较中心是否变化再触发重建。
- **不做「未烘焙段不遮挡可见性 BFS」**：放宽 BFS 会渲染无连接数据的段、破坏遮挡剔除正确性；源头（烘焙优先级）修复后，未烘焙段的可见性放大效应自然消退。
- **平局保留字典序**：既有 `sectionKeyLess` 作为复合键次序，保证同距排序确定、测试可钉。

## 验证

`go test ./packages/client/client -race -count=1`；`SCENES=<世界景子集> make visual-check`（零漂移）；`make dev-check`。
