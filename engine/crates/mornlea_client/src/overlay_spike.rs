//! GameOverlay 前置 spike 的遗留通道:环境变量门控的验证臂与帧开销探针。
//!
//! **spike 遗留,Phase 1 验收后整体移除。** 生产侧的两态参与模型已迁至
//! `crate::overlay`(参与模式枚举、动作表与 WKWebView 子类);本模块只剩
//! spike 专属的三块能力,全部由环境变量显式开启,未设置时零参与:
//!
//! - **验证臂位**:`MORNLEA_SPIKE_OVERLAY` 把生产模式钉成固定档位,用于把
//!   「相位驱动」与「强制档位」两个变量分开验证。`off`(缺省)不强制,
//!   行为与生产完全一致;`menu` 强制 `Menu`(游戏相位回退到
//!   旧的隐藏语义,对照臂);`game` 强制 `GameOverlay`(与
//!   生产的游戏相位行为一致)。强制档位只改「模式来源」,不新增动作语义
//!   ——动作表仍是 [`crate::overlay::plan_transition`] 一份真相。
//! - **帧开销探针**:`MORNLEA_SPIKE_FPS=1` 时在 renderer present 边界采样,
//!   每 [`SUMMARY_FRAMES`] 帧向 stderr 输出一行摘要(帧耗时均值/分位数与
//!   帧间隔);只在内存里保留固定窗口的样本,不做任何文件 I/O——渲染热路径
//!   禁止阻塞 I/O,长尾数据由 stderr 重定向交给执行者自行落盘。
//! - **留痕日志**:验证臂位的挂载/卸载/相位切换各输出一行,便于执行者核对
//!   两态切换;生产路径(未设环境变量)静默,stderr 逐字节不变。
//!
//! 历史结论(design.md D7):S1 hitTest 分级、S2 合成开销两项判据均已达成,
//! 本模块由此从「验证可行性」转为「C 组 HUD 组件落地后的 S2 复测入口」
//! (`crate::spike_auto` 继续依赖 [`FrameProbe`] 与臂位标注)。
//!
//! 线程约束:本模块的所有 AppKit 相关入口都必须在创建窗口的 OS 主线程调用
//! (与 [`crate::webview`] 一致);纯函数与帧探针无线程要求(探针内部用
//! 原子量与互斥量,渲染线程独占访问)。

use std::collections::VecDeque;
use std::env;
use std::sync::{Mutex, OnceLock};
use std::time::Instant;

use crate::overlay::OverlayMode;

/// 验证臂位的环境变量。
pub(crate) const OVERLAY_ENV: &str = "MORNLEA_SPIKE_OVERLAY";

/// 帧开销探针的开关环境变量。
pub(crate) const FPS_ENV: &str = "MORNLEA_SPIKE_FPS";

/// 自驱动档的开关环境变量。
///
/// 与 [`OVERLAY_ENV`]/[`FPS_ENV`] 正交:开启后由
/// [`crate::spike_auto`] 在进程内驱动整套验证脚本(自动进入游戏、合成事件、
/// 断言、测量与结果落盘),执行者不再手工操作窗口。默认关闭,生产路径
/// 与手工 spike 档都保持零参与。
pub(crate) const AUTO_ENV: &str = "MORNLEA_SPIKE_AUTO";

/// 帧摘要输出的间隔帧数;60 Hz 下约两秒一行,足够覆盖长尾又不淹没 stderr。
const SUMMARY_FRAMES: u64 = 120;

/// 帧样本环形窗口长度;只保留最近 2048 帧,统计内存占用恒定。
const SAMPLE_WINDOW: usize = 2048;

/// 从环境变量原始值解析强制档位;未知值返回 `None`(由调用方告警并回落
/// 生产路径)。
///
/// `None` 表示「不强制」——生产路径,参与模式由下行相位逐次推导。
/// `1`/`game`/`overlay` 都强制 GameOverlay——`1` 保留给「只要打开开关
/// 就跑验证臂」的单命令用法,`menu` 才是显式的对照臂。
fn parse_forced_mode(raw: &str) -> Option<Option<OverlayMode>> {
    match raw.trim().to_ascii_lowercase().as_str() {
        "" | "0" | "off" => Some(None),
        "menu" => Some(Some(OverlayMode::Menu)),
        "1" | "game" | "overlay" => Some(Some(OverlayMode::GameOverlay)),
        _ => None,
    }
}

/// 从环境变量读取强制档位;变量缺失等价于空串(不强制),未知值打一行
/// stderr 后回落生产路径(宁可显式留痕,也不让拼写错误静默变成「以为在跑
/// spike」)。
pub(crate) fn forced_mode_from_env() -> Option<OverlayMode> {
    forced_mode_from_value(env::var(OVERLAY_ENV).ok().as_deref())
}

/// [`forced_mode_from_env`] 的纯函数核,便于在无环境变量的测试里钉值。
fn forced_mode_from_value(value: Option<&str>) -> Option<OverlayMode> {
    let parsed = value.map(parse_forced_mode).unwrap_or(Some(None));
    match parsed {
        Some(forced) => forced,
        None => {
            eprintln!("mornlea spike overlay: 未识别的 {OVERLAY_ENV} 值 {value:?},按生产路径处理");
            None
        }
    }
}

/// spike 通道是否开启(强制档位存在,或自驱动档开启)。
///
/// 生产路径(两个环境变量都未设置)返回 `false`,留痕日志因此静默——spike
/// 噪声(A 组的 `挂载 mode=…`、`相位切换 …` 各一行)不再进入生产 stderr。
pub(crate) fn active() -> bool {
    static ACTIVE: OnceLock<bool> = OnceLock::new();
    *ACTIVE.get_or_init(|| forced_mode_from_env().is_some() || crate::spike_auto::enabled())
}

/// spike 挂载留痕:验证臂与对照臂各一行,生产路径静默(无 spike 噪声)。
pub(crate) fn log_mount(mode: Option<OverlayMode>) {
    if !active() {
        return;
    }
    match mode {
        Some(mode) => eprintln!(
            "mornlea spike overlay: 挂载 forced_mode={mode:?} hitTest 分级 spike 生效(spike 遗留,Phase 1 验收后移除)"
        ),
        None => eprintln!(
            "mornlea spike overlay: 挂载 phase-driven(生产参与模式)留痕(spike 遗留,Phase 1 验收后移除)"
        ),
    }
}

/// spike 卸载留痕:与 [`log_mount`] 成对,验证挂载/卸载事件各出现一次。
pub(crate) fn log_unmount(mode: Option<OverlayMode>) {
    if !active() {
        return;
    }
    eprintln!("mornlea spike overlay: 卸载 forced_mode={mode:?} WebView 视图移出视图树");
}

/// 相位切换留痕:把动作表逐项打到 stderr,供执行者核对两态切换是否符合
/// 预期(切换次数少,日志量可忽略)。
pub(crate) fn log_phase_transition(mode: OverlayMode, phase_wants_visible: bool) {
    if !active() {
        return;
    }
    let transition = crate::overlay::plan_transition(mode, phase_wants_visible);
    eprintln!(
        "mornlea spike overlay: 相位切换 mode={mode:?} wants_visible={phase_wants_visible} \
         hide={} passthrough={} focus_game_view={}",
        transition.hide_view, transition.hit_test_passthrough, transition.focus_game_view,
    );
}

/// 帧探针:统计 present 边界的帧耗时与帧间隔,定期输出摘要。
///
/// 只在 `MORNLEA_SPIKE_FPS` 显式开启时才存在实例;关闭时 [`frame_probe`]
/// 返回 `None`,渲染路径除一次 `OnceLock` 读取外零开销。
pub(crate) struct FrameProbe {
    stats: Mutex<FrameStats>,
}

#[derive(Default)]
struct FrameStats {
    /// 帧耗时样本(微秒),环形窗口,只保留最近 [`SAMPLE_WINDOW`] 帧。
    frame_us: VecDeque<u32>,
    /// 帧间隔样本(微秒),与 `frame_us` 同窗口同长度。
    interval_us: VecDeque<u32>,
    /// 累计采样帧数(决定何时输出摘要)。
    frames: u64,
    /// 稳态采集起点帧号;`None` 表示未标记稳态窗口,样本照常入窗(手工
    /// spike 档的摘要行行为不变)。标记点之后丢弃 [`WARMUP_FRAMES`] 帧再
    /// 入窗,与 `spike-checklist.md` 的「丢前 5 个摘要行」预热口径一致。
    steady_from: Option<u64>,
    /// 上一帧结束时刻;首个样本没有间隔,记为 `None`。
    last: Option<Instant>,
}

/// 稳态窗口的预热丢弃帧数;120 帧一行摘要,即丢弃前 5 行摘要对应的样本。
const WARMUP_FRAMES: u64 = 5 * SUMMARY_FRAMES;

/// 稳态窗口统计:一列样本的均值与分位数(微秒)。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct SampleStats {
    /// 算术平均。
    pub mean: u32,
    /// 中位数(近邻取值)。
    pub p50: u32,
    /// 95 分位(近邻取值)。
    pub p95: u32,
    /// 最大值。
    pub max: u32,
}

/// 一次稳态采集的完整读数:样本数与帧耗时/帧间隔两列统计。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct WindowStats {
    /// 入窗样本数(帧耗时与帧间隔同长,间隔列少首个样本)。
    pub samples: usize,
    /// present 边界 CPU 耗时(微秒)。
    pub frame_us: SampleStats,
    /// 相邻帧间隔(微秒);帧率证据以这一列为准。
    pub interval_us: SampleStats,
}

impl FrameProbe {
    fn new() -> Self {
        Self {
            stats: Mutex::new(FrameStats {
                frame_us: VecDeque::with_capacity(SAMPLE_WINDOW),
                interval_us: VecDeque::with_capacity(SAMPLE_WINDOW),
                frames: 0,
                steady_from: None,
                last: None,
            }),
        }
    }

    /// 取一帧的起点时刻;与 [`Self::record`] 成对使用。
    pub(crate) fn now(&self) -> Instant {
        Instant::now()
    }

    /// 标记一次稳态采集:丢弃接下来 [`WARMUP_FRAMES`] 帧的样本并清空既有
    /// 窗口,此后入窗的样本只反映标记之后的稳态。帧计数与 `last` 保留,
    /// 帧间隔在边界处依然连续,不会混入一次虚假间隔。
    ///
    /// 由自驱动档在每组测量开始时调用;手工 spike 档不调用,摘要行行为不变。
    pub(crate) fn begin_steady_window(&self) {
        let mut stats = self.stats.lock().expect("spike 帧探针锁中毒");
        stats.frame_us.clear();
        stats.interval_us.clear();
        stats.steady_from = Some(stats.frames + WARMUP_FRAMES);
    }

    /// 读取当前稳态窗口统计;入窗样本不足以覆盖一次摘要间隔时返回 `None`
    /// (样本过少说明该组测量没有真正进入稳态,宁可缺数也不给伪统计)。
    pub(crate) fn window_stats(&self) -> Option<WindowStats> {
        let mut stats = self.stats.lock().expect("spike 帧探针锁中毒");
        let samples = stats.frame_us.len();
        if samples < SUMMARY_FRAMES as usize {
            return None;
        }
        let frame_us = sample_stats(stats.frame_us.make_contiguous())?;
        let interval_us = sample_stats(stats.interval_us.make_contiguous())?;
        Some(WindowStats {
            samples,
            frame_us,
            interval_us,
        })
    }

    /// 采样一帧:样本入环形窗口,累计到摘要间隔就输出一行摘要。锁只被渲染
    /// 线程持有,粒度为一次窗口写入与一次摘要计算。
    pub(crate) fn record(&self, started: Instant) {
        let now = Instant::now();
        let frame_us = u32::try_from(now.duration_since(started).as_micros()).unwrap_or(u32::MAX);
        let summary = {
            let mut stats = self.stats.lock().expect("spike 帧探针锁中毒");
            let interval_us = stats
                .last
                .map(|last| u32::try_from(now.duration_since(last).as_micros()).unwrap_or(u32::MAX))
                .unwrap_or(0);
            stats.last = Some(now);
            // 已标记稳态窗口时,预热帧不入窗:窗口内容因此始终与最近一次
            // begin_steady_window 之后的测量对应;未标记时照常入窗。
            if stats.steady_from.is_none_or(|from| stats.frames >= from) {
                push_capped(&mut stats.frame_us, frame_us);
                if stats.frames > 0 {
                    push_capped(&mut stats.interval_us, interval_us);
                }
            }
            stats.frames += 1;
            if stats.frames.is_multiple_of(SUMMARY_FRAMES) {
                Some(format_stats(&mut stats))
            } else {
                None
            }
        };
        // stderr 写在锁外:摘要行不属于窗口状态,且不阻塞下一帧的采样。
        if let Some(line) = summary {
            eprintln!("{line}");
        }
    }
}

/// 环形窗口写入:满时弹出最旧样本,统计始终反映最近 [`SAMPLE_WINDOW`] 帧。
fn push_capped(samples: &mut VecDeque<u32>, value: u32) {
    if samples.len() == SAMPLE_WINDOW {
        samples.pop_front();
    }
    samples.push_back(value);
}

/// 生成一行摘要:样本数、均值、p50/p95/max 与帧间隔的同类统计。
///
/// 输出带 `mornlea spike fps:` 前缀,便于执行者用 grep 从重定向的 stderr
/// 中抽取;分位数按最近窗口内的样本线性选取(近邻取值,足够判定长尾)。
fn format_stats(stats: &mut FrameStats) -> String {
    // make_contiguous 把环形缓冲整理成连续切片(必要时原地旋转),统计无需
    // 再复制一份样本。
    format!(
        "mornlea spike fps: frames={} frame_us({}) interval_us({})",
        stats.frames,
        summarize(stats.frame_us.make_contiguous()),
        summarize(stats.interval_us.make_contiguous()),
    )
}

/// 对一组微秒样本给出 `mean=… p50=… p95=… max=…` 片段;空窗口输出 `n/a`。
fn summarize(samples: &[u32]) -> String {
    match sample_stats(samples) {
        Some(stats) => format!(
            "mean={} p50={} p95={} max={}",
            stats.mean, stats.p50, stats.p95, stats.max
        ),
        None => "n/a".to_string(),
    }
}

/// 计算一组微秒样本的均值与分位数;空样本返回 `None`。
///
/// 分位数按 `ceil(len × ratio)` 近邻取值,与摘要行的既有口径一致,保证
/// stderr 摘要与自驱动读数可互相对照。
fn sample_stats(samples: &[u32]) -> Option<SampleStats> {
    if samples.is_empty() {
        return None;
    }
    let mut sorted = samples.to_vec();
    sorted.sort_unstable();
    let pick = |ratio: f64| -> u32 {
        let index = ((sorted.len() as f64) * ratio).ceil() as usize;
        sorted[index.clamp(1, sorted.len()) - 1]
    };
    let mean = sorted.iter().map(|value| u64::from(*value)).sum::<u64>() / sorted.len() as u64;
    let mean = u32::try_from(mean).unwrap_or(u32::MAX);
    Some(SampleStats {
        mean,
        p50: pick(0.50),
        p95: pick(0.95),
        max: sorted[sorted.len() - 1],
    })
}

static FRAME_PROBE: OnceLock<Option<FrameProbe>> = OnceLock::new();

/// 返回帧探针;未开启时返回 `None`(调用方按 `Option` 短路,零额外工作)。
pub(crate) fn frame_probe() -> Option<&'static FrameProbe> {
    FRAME_PROBE
        .get_or_init(|| {
            if probe_enabled_from_env() {
                eprintln!(
                    "mornlea spike fps: 帧探针开启,present 边界每 {SUMMARY_FRAMES} 帧输出摘要"
                );
                Some(FrameProbe::new())
            } else {
                None
            }
        })
        .as_ref()
}

/// 读取帧探针开关。
fn probe_enabled_from_env() -> bool {
    probe_enabled_from_value(env::var(FPS_ENV).ok().as_deref())
}

/// [`probe_enabled_from_env`] 的纯函数核,便于在测试里钉值。
fn probe_enabled_from_value(value: Option<&str>) -> bool {
    match value {
        None | Some("") => false,
        Some(raw) => !matches!(
            raw.trim().to_ascii_lowercase().as_str(),
            "0" | "off" | "false"
        ),
    }
}

/// 读取自驱动档开关;与探针开关同一取值口径(`1` 开、`0`/`off`/`false` 关)。
pub(crate) fn auto_enabled_from_env() -> bool {
    auto_enabled_from_value(env::var(AUTO_ENV).ok().as_deref())
}

/// [`auto_enabled_from_env`] 的纯函数核,便于在测试里钉值。
fn auto_enabled_from_value(value: Option<&str>) -> bool {
    probe_enabled_from_value(value)
}

#[cfg(test)]
mod tests {
    use super::{
        AUTO_ENV, FPS_ENV, OVERLAY_ENV, auto_enabled_from_value, forced_mode_from_value,
        parse_forced_mode, probe_enabled_from_value, sample_stats,
    };
    use crate::overlay::OverlayMode;

    /// 档位解析:缺失/空串/`0`/`off` 都不强制(生产路径,模式由相位推导),
    /// `menu` 强制 Menu(对照臂:游戏相位回退旧隐藏语义),
    /// `1`/`game`/`overlay` 强制 GameOverlay(与生产的游戏相位行为一致),
    /// 大小写与首尾空白不敏感。
    #[test]
    fn forced_mode_parsing_covers_all_documented_values() {
        assert_eq!(forced_mode_from_value(None), None);
        for raw in ["", " ", "0", "off", "OFF"] {
            assert_eq!(forced_mode_from_value(Some(raw)), None, "raw={raw:?}");
        }
        assert_eq!(
            forced_mode_from_value(Some("menu")),
            Some(OverlayMode::Menu)
        );
        assert_eq!(
            forced_mode_from_value(Some("Menu")),
            Some(OverlayMode::Menu)
        );
        for raw in ["1", "game", "GAME", "overlay"] {
            assert_eq!(
                forced_mode_from_value(Some(raw)),
                Some(OverlayMode::GameOverlay),
                "raw={raw:?}"
            );
        }
        // 未知值显式回落生产路径,不让拼写错误静默变成「以为在跑 spike」。
        assert_eq!(forced_mode_from_value(Some("GameOverlay")), None);
    }

    /// `parse_forced_mode` 与 [`forced_mode_from_value`] 的一致性:未知值经
    /// `map` 后仍要落回「不强制」而不是 panic;强制档位只能是生产的两个态,
    /// 不引入第三种参与语义。
    #[test]
    fn parse_forced_mode_returns_none_for_unknown_value() {
        assert_eq!(parse_forced_mode("nope"), None);
        assert_eq!(
            parse_forced_mode("game"),
            Some(Some(OverlayMode::GameOverlay))
        );
        assert_eq!(parse_forced_mode("menu"), Some(Some(OverlayMode::Menu)));
        assert_eq!(parse_forced_mode(""), Some(None));
    }

    /// 环境变量名是契约的一部分:checklist 里的启动命令依赖它,改名必须
    /// 同步文档。
    #[test]
    fn env_variable_names_are_stable() {
        assert_eq!(OVERLAY_ENV, "MORNLEA_SPIKE_OVERLAY");
        assert_eq!(FPS_ENV, "MORNLEA_SPIKE_FPS");
    }

    /// 探针开关:默认关闭(无头路径零参与的前提),`1` 开启,`0`/`false`
    /// 显式关闭,其余非空值按开启处理。
    #[test]
    fn probe_gating_defaults_to_disabled() {
        assert!(!probe_enabled_from_value(None));
        assert!(!probe_enabled_from_value(Some("")));
        assert!(!probe_enabled_from_value(Some("0")));
        assert!(!probe_enabled_from_value(Some("false")));
        assert!(probe_enabled_from_value(Some("1")));
        assert!(probe_enabled_from_value(Some(" true ")));
    }

    /// 自驱动档开关沿用探针开关的取值口径:缺省关闭,`1` 开启,`0`/`off`
    /// 显式关闭。变量名是 checklist 启动命令契约的一部分。
    #[test]
    fn auto_gating_defaults_to_disabled_and_env_name_is_stable() {
        assert!(!auto_enabled_from_value(None));
        assert!(!auto_enabled_from_value(Some("")));
        assert!(!auto_enabled_from_value(Some("0")));
        assert!(!auto_enabled_from_value(Some("false")));
        assert!(auto_enabled_from_value(Some("1")));
        assert!(auto_enabled_from_value(Some(" TRUE ")));
        assert_eq!(AUTO_ENV, "MORNLEA_SPIKE_AUTO");
    }

    /// 稳态窗口统计必须与摘要行同口径:空样本无读数,分位数近邻取值。
    #[test]
    fn sample_stats_matches_summary_semantics() {
        assert_eq!(sample_stats(&[]), None);
        let got = sample_stats(&[300, 100, 200, 400]).expect("非空样本必有读数");
        assert_eq!(got.mean, 250);
        assert_eq!(got.p50, 200);
        assert_eq!(got.p95, 400);
        assert_eq!(got.max, 400);
    }
}
