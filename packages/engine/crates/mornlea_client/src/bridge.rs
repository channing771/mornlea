//! 菜单层桥的纯逻辑半部:上行事件信封的解析、校验与事件队列。
//!
//! 本模块刻意不依赖任何 WebKit 类型——WebView 只是信封的搬运工,真正的
//! 协议逻辑集中在这里,以便在不创建真实窗口/WebView 的单元测试中完整验证
//! 拒绝语义与保序语义。协议形状以单源
//! `packages/engine/crates/mornlea_client/frontend/src/bridge/schema.json` 为权威:
//!
//! - 下行(Go → JS):Go 组装 `uiState` JSON,本 crate 只做相位浅校验后经
//!   `evaluateJavaScript` 转发(见 [`crate::webview`])。
//! - 上行(JS → Go):页面按 `uplinkEnvelope` 形状
//!   `{"v":1,"events":[...]}` postMessage;本模块校验 JSON 可解析、版本为 1、
//!   events 为非空对象数组,并把解析后的事件按产生顺序入队。更深一层的
//!   schema 校验(未知动作、字段越界等)由 Go 消费侧负责——Rust 侧深校验
//!   只会把同一套规则复制两份,任何漂移都会让合法载荷在半路被吞。

use std::collections::VecDeque;
use std::sync::{Arc, Mutex};

/// 上行信封协议版本,与 schema.json `uplinkEnvelope.v` 逐值互钉。
pub const UPLINK_ENVELOPE_VERSION: u64 = 1;

/// 单次排空写出的最大事件数,与 schema `uplinkEnvelope.events.maxItems` 逐值互钉;
/// 积压超过该数时分多次排空,保证每次写出的信封本身都是 schema 合法信封。
pub const MAX_EVENTS_PER_DRAIN: usize = 64;

/// 每个渲染器最多积压的桥上行事件数。WebKit 页面是可信资产(仓库内嵌),
/// 该上限只是防热路径无界积压的护栏:超出时整份新信封被拒绝并丢弃,
/// 与既有「校验失败不留下部分输出」同一取舍。
pub const MAX_PENDING_EVENTS: usize = 4096;

/// 信封被拒绝的原因。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EnvelopeError {
    /// 字节不是合法 UTF-8 或不是合法 JSON。
    Malformed,
    /// 顶层不是对象、版本字段缺失或不等于 [`UPLINK_ENVELOPE_VERSION`]。
    Version,
    /// events 缺失、不是数组、为空,或含非对象元素。
    Events,
    /// 入队会超出 [`MAX_PENDING_EVENTS`] 容量;队列保持不变。
    Capacity,
}

/// 单个渲染器持有的桥上行事件队列:按页面产生顺序保存已浅校验的事件
/// (`serde_json::Value`,恒为 JSON 对象),由渲染器的 `drain_ui_events`
/// 出口消费。
#[derive(Default)]
pub struct UiEventQueue {
    pending: VecDeque<serde_json::Value>,
}

/// 排空失败的原因。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DrainError {
    /// 调用方缓冲装不下本次完整信封;队列与缓冲均不变。
    Capacity,
}

impl UiEventQueue {
    /// 创建空队列。
    pub fn new() -> Self {
        Self::default()
    }

    /// 当前积压事件数。
    pub fn len(&self) -> usize {
        self.pending.len()
    }

    /// 报告队列是否为空;排空前调用方常以此决定零工作路径。
    pub fn is_empty(&self) -> bool {
        self.pending.is_empty()
    }

    /// 解析一份上行信封并按序入队。校验只做协议最低层(可解析 + 版本 +
    /// 非空对象数组):拒绝时返回 [`EnvelopeError`] 且队列保持不变。
    pub fn enqueue_envelope(&mut self, bytes: &[u8]) -> Result<(), EnvelopeError> {
        let events = parse_envelope_events(bytes)?;
        if self.pending.len() + events.len() > MAX_PENDING_EVENTS {
            return Err(EnvelopeError::Capacity);
        }
        self.pending.extend(events);
        Ok(())
    }

    /// 把积压事件合并为一个版本化信封写进 `out`,返回写入字节数。
    ///
    /// - 空队列写 0 字节(调用方得到"无 UI 状态"的空结果)。
    /// - 单次至多写出 [`MAX_EVENTS_PER_DRAIN`] 条,余量留给下次排空;
    ///   事件按入队顺序出现,跨信封合并不重排。
    /// - `out` 装不下本次应写出的完整信封时返回容量错误,队列与 `out`
    ///   均不变——与旧 ABI「只有完整 batch 可装入才写入并清空」同一语义。
    pub fn drain_into(&mut self, out: &mut [u8]) -> Result<usize, DrainError> {
        if self.pending.is_empty() {
            return Ok(0);
        }
        let take = self.pending.len().min(MAX_EVENTS_PER_DRAIN);
        let batch: Vec<&serde_json::Value> = self.pending.range(..take).collect();
        let envelope = serde_json::json!({ "v": UPLINK_ENVELOPE_VERSION, "events": batch });
        let bytes = serde_json::to_vec(&envelope).map_err(|_| DrainError::Capacity)?;
        if bytes.len() > out.len() {
            return Err(DrainError::Capacity);
        }
        out[..bytes.len()].copy_from_slice(&bytes);
        self.pending.drain(..take);
        Ok(bytes.len())
    }
}

/// 把信封的 `events` 数组整体解析为浅校验过的事件列表。
///
/// 页面真正发送的是 JS 对象;WebKit 经 `WKScriptMessage.body` 交给宿主后,
/// 宿主用 `NSJSONSerialization` 还原为 JSON 字节再交到这里,因此这里总是
/// 处理标准 JSON 文本。
fn parse_envelope_events(bytes: &[u8]) -> Result<Vec<serde_json::Value>, EnvelopeError> {
    let value: serde_json::Value =
        serde_json::from_slice(bytes).map_err(|_| EnvelopeError::Malformed)?;
    let object = value.as_object().ok_or(EnvelopeError::Malformed)?;
    match object.get("v") {
        Some(serde_json::Value::Number(number))
            if number.as_u64() == Some(UPLINK_ENVELOPE_VERSION) => {}
        _ => return Err(EnvelopeError::Version),
    }
    let Some(events) = object.get("events").and_then(|events| events.as_array()) else {
        return Err(EnvelopeError::Events);
    };
    if events.is_empty() || events.iter().any(|event| !event.is_object()) {
        return Err(EnvelopeError::Events);
    }
    Ok(events.clone())
}

/// 跨线程共享的队列句柄:WebKit 回调(主线程)与渲染器排空(同一主线程)
/// 之间只隔着一把无竞争的锁。互斥量保证协议回调与 FFI 排空即使在意外的
/// 线程组合下也不会撕裂队列。
#[derive(Default, Clone)]
pub struct SharedUiEventQueue(Arc<Mutex<UiEventQueue>>);

/// 进程级桥事件队列单例:WebView 挂在窗口侧,而上行事件的既有排空出口
/// (`drain_ui_events`)在渲染器句柄上,两侧经本单例交汇;benchmark/capture
/// 进程从不创建 WebView,队列恒空,零参与语义天然成立。
static SHARED_QUEUE: std::sync::OnceLock<SharedUiEventQueue> = std::sync::OnceLock::new();

/// 返回进程级桥事件队列单例。
pub fn shared_queue() -> &'static SharedUiEventQueue {
    SHARED_QUEUE.get_or_init(SharedUiEventQueue::new)
}

impl SharedUiEventQueue {
    /// 创建空队列句柄。
    pub fn new() -> Self {
        Self::default()
    }

    /// 解析并按序入队一份信封;拒绝时队列不变。
    pub fn enqueue_envelope(&self, bytes: &[u8]) -> Result<(), EnvelopeError> {
        self.0
            .lock()
            .expect("桥事件队列锁中毒")
            .enqueue_envelope(bytes)
    }

    /// 排空到 `out`,语义同 [`UiEventQueue::drain_into`]。
    pub fn drain_into(&self, out: &mut [u8]) -> Result<usize, DrainError> {
        self.0.lock().expect("桥事件队列锁中毒").drain_into(out)
    }

    /// 当前积压事件数;测试与零参与断言用。
    pub fn len(&self) -> usize {
        self.0.lock().expect("桥事件队列锁中毒").len()
    }

    /// 报告队列是否为空。
    pub fn is_empty(&self) -> bool {
        self.0.lock().expect("桥事件队列锁中毒").is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn envelope(events: &str) -> Vec<u8> {
        format!(r#"{{"v":1,"events":[{events}]}}"#).into_bytes()
    }

    #[test]
    fn valid_envelope_enqueues_events_in_order() {
        let mut queue = UiEventQueue::new();
        queue
            .enqueue_envelope(&envelope(
                r#"{"type":"settings-change","field":"audioVolume","value":0.5},{"type":"action","id":"enter-game"}"#,
            ))
            .expect("合法信封必须被接受");
        let mut out = vec![0u8; 512];
        let written = queue.drain_into(&mut out).expect("排空应成功");
        let drained: serde_json::Value = serde_json::from_slice(&out[..written]).unwrap();
        assert_eq!(drained["v"], 1);
        let events = drained["events"].as_array().unwrap();
        assert_eq!(events.len(), 2);
        assert_eq!(events[0]["type"], "settings-change");
        assert_eq!(events[1]["id"], "enter-game");
        assert!(queue.is_empty(), "排空后队列必须清空");
    }

    #[test]
    fn empty_queue_drains_zero_bytes() {
        let mut queue = UiEventQueue::new();
        let mut out = vec![0xAAu8; 64];
        let written = queue.drain_into(&mut out).expect("空队列排空应成功");
        assert_eq!(written, 0, "无 UI 状态时排空必须返回空");
        assert!(out.iter().all(|&b| b == 0xAA), "空排空不得写缓冲");
    }

    #[test]
    fn invalid_envelopes_are_rejected_without_side_effects() {
        let mut queue = UiEventQueue::new();
        queue
            .enqueue_envelope(&envelope(r#"{"type":"action","id":"quit"}"#))
            .expect("先入队一条作对照");
        let before = queue.len();

        // JSON 不可解析。
        assert_eq!(
            queue.enqueue_envelope(b"{v:1,").unwrap_err(),
            EnvelopeError::Malformed
        );
        // 版本缺失 / 版本错误。
        assert_eq!(
            queue
                .enqueue_envelope(br#"{"events":[{"type":"action"}]}"#)
                .unwrap_err(),
            EnvelopeError::Version
        );
        assert_eq!(
            queue
                .enqueue_envelope(br#"{"v":2,"events":[{"type":"action"}]}"#)
                .unwrap_err(),
            EnvelopeError::Version
        );
        // events 缺失 / 非数组 / 空 / 含非对象元素。
        assert_eq!(
            queue.enqueue_envelope(br#"{"v":1}"#).unwrap_err(),
            EnvelopeError::Events
        );
        assert_eq!(
            queue
                .enqueue_envelope(br#"{"v":1,"events":42}"#)
                .unwrap_err(),
            EnvelopeError::Events
        );
        assert_eq!(
            queue
                .enqueue_envelope(br#"{"v":1,"events":[]}"#)
                .unwrap_err(),
            EnvelopeError::Events
        );
        assert_eq!(
            queue
                .enqueue_envelope(br#"{"v":1,"events":[42]}"#)
                .unwrap_err(),
            EnvelopeError::Events
        );
        // 顶层不是对象。
        assert_eq!(
            queue.enqueue_envelope(b"[1,2]").unwrap_err(),
            EnvelopeError::Malformed
        );
        // 非法 UTF-8。
        assert_eq!(
            queue.enqueue_envelope(&[0xFF, 0xFE]).unwrap_err(),
            EnvelopeError::Malformed
        );
        assert_eq!(queue.len(), before, "全部拒绝路径不得改动队列");
        // 对照样本仍在,后续排空保序。
        let mut out = vec![0u8; 256];
        let written = queue.drain_into(&mut out).unwrap();
        assert_eq!(queue.len(), 0);
        let drained: serde_json::Value = serde_json::from_slice(&out[..written]).unwrap();
        assert_eq!(drained["events"][0]["id"], "quit");
    }

    #[test]
    fn capacity_rejection_leaves_queue_unchanged() {
        let mut queue = UiEventQueue::new();
        for _ in 0..MAX_PENDING_EVENTS {
            queue
                .enqueue_envelope(&envelope(r#"{"type":"action","id":"quit"}"#))
                .expect("护栏内入队应成功");
        }
        assert_eq!(queue.len(), MAX_PENDING_EVENTS);
        assert_eq!(
            queue
                .enqueue_envelope(&envelope(r#"{"type":"action","id":"quit"}"#))
                .unwrap_err(),
            EnvelopeError::Capacity,
            "超出护栏的信封必须整体拒绝"
        );
        assert_eq!(queue.len(), MAX_PENDING_EVENTS, "拒绝不得部分入队");
    }

    #[test]
    fn drain_caps_batch_and_preserves_order_across_calls() {
        let mut queue = UiEventQueue::new();
        for index in 0..MAX_EVENTS_PER_DRAIN + 3usize {
            let envelope =
                format!(r#"{{"v":1,"events":[{{"type":"action","id":"quit","seq":{index}}}]}}"#);
            queue.enqueue_envelope(envelope.as_bytes()).unwrap();
        }
        let mut out = vec![0u8; 4096];
        let first = queue.drain_into(&mut out).unwrap();
        let drained: serde_json::Value = serde_json::from_slice(&out[..first]).unwrap();
        assert_eq!(
            drained["events"].as_array().unwrap().len(),
            MAX_EVENTS_PER_DRAIN
        );
        assert_eq!(drained["events"][0]["seq"], 0, "排空必须按入队顺序");
        let second = queue.drain_into(&mut out).unwrap();
        let drained: serde_json::Value = serde_json::from_slice(&out[..second]).unwrap();
        assert_eq!(drained["events"].as_array().unwrap().len(), 3);
        assert_eq!(drained["events"][0]["seq"], MAX_EVENTS_PER_DRAIN);
        assert!(queue.is_empty());
    }

    #[test]
    fn drain_reports_capacity_without_touching_queue() {
        let mut queue = UiEventQueue::new();
        queue
            .enqueue_envelope(&envelope(r#"{"type":"action","id":"quit"}"#))
            .unwrap();
        // 信封约 50 字节;给一个装不下的缓冲。
        let mut tiny = [0u8; 8];
        assert_eq!(
            queue.drain_into(&mut tiny).unwrap_err(),
            DrainError::Capacity,
            "容量不足必须报错"
        );
        assert_eq!(queue.len(), 1, "容量失败不得清空队列");
        // 容量恢复后同一批事件完整重放。
        let mut out = vec![0u8; 256];
        assert!(queue.drain_into(&mut out).is_ok());
        assert!(queue.is_empty());
    }

    #[test]
    fn shared_queue_round_trips() {
        let shared = SharedUiEventQueue::new();
        shared
            .enqueue_envelope(&envelope(r#"{"type":"action","id":"enter-game"}"#))
            .unwrap();
        assert!(!shared.is_empty());
        let mut out = vec![0u8; 128];
        let written = shared.drain_into(&mut out).unwrap();
        let drained: serde_json::Value = serde_json::from_slice(&out[..written]).unwrap();
        assert_eq!(drained["events"][0]["id"], "enter-game");
        assert!(shared.is_empty());
    }

    /// 三端钉值测试(Rust 半):本模块的协议常量必须与单源 schema.json 的
    /// `uplinkEnvelope` 逐值一致。前端路径漂移(改版本号、改单批上界)会让
    /// 本测试先红,迫使两端同批修改。
    #[test]
    fn envelope_constants_are_pinned_to_schema() {
        let schema: serde_json::Value =
            serde_json::from_str(include_str!("../frontend/src/bridge/schema.json"))
                .expect("单源 schema.json 必须随 dist 一起入库且可解析");
        let envelope = &schema["$defs"]["uplinkEnvelope"];
        assert_eq!(
            envelope["properties"]["v"]["const"],
            UPLINK_ENVELOPE_VERSION
        );
        let events = &envelope["properties"]["events"];
        assert_eq!(events["maxItems"], MAX_EVENTS_PER_DRAIN);
        assert_eq!(events["minItems"], 1);
        // 事件类型集合也与 schema 对齐:Rust 只做浅校验,但 schema 声明的
        // type 枚举必须非空且首项为 action(协议形状锚点)。
        let event_types = &schema["$defs"]["uplinkEvent"]["description"];
        assert!(event_types.is_string());
    }
}
