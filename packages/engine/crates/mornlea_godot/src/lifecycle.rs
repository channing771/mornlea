use std::panic::{AssertUnwindSafe, catch_unwind};
use std::sync::{Mutex, OnceLock};

use godot::prelude::InitStage;

const SUPPORTED_GODOT_API: (i64, i64) = (4, 7);
static RUNTIME_LIFECYCLE: OnceLock<Mutex<Lifecycle>> = OnceLock::new();

/// Ordered extension stages owned by the Mornlea adapter.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum Stage {
    Scene,
    Editor,
    MainLoop,
}

impl Stage {
    pub(crate) fn label(self) -> &'static str {
        match self {
            Self::Scene => "scene",
            Self::Editor => "editor",
            Self::MainLoop => "main-loop",
        }
    }

    fn from_godot(stage: InitStage) -> Option<Self> {
        match stage {
            InitStage::Scene => Some(Self::Scene),
            InitStage::Editor => Some(Self::Editor),
            InitStage::MainLoop => Some(Self::MainLoop),
            _ => None,
        }
    }
}

/// Stable failures returned at the native boundary instead of unwinding into Godot.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum BoundaryFailure {
    InvalidTransition,
    Panic,
    Poisoned,
}

impl std::fmt::Display for BoundaryFailure {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::InvalidTransition => formatter.write_str("invalid lifecycle transition"),
            Self::Panic => formatter.write_str("Rust panic at Godot boundary"),
            Self::Poisoned => formatter.write_str("lifecycle state is poisoned"),
        }
    }
}

/// Small deterministic state machine shared by unit tests and extension callbacks.
#[derive(Debug, Default)]
pub(crate) struct Lifecycle {
    active: Vec<Stage>,
}

impl Lifecycle {
    pub(crate) fn initialize(&mut self, stage: Stage) -> Result<(), BoundaryFailure> {
        let valid = matches!(
            (self.active.as_slice(), stage),
            ([], Stage::Scene)
                | ([Stage::Scene], Stage::Editor | Stage::MainLoop)
                | ([Stage::Scene, Stage::Editor], Stage::MainLoop)
        );
        if !valid {
            return Err(BoundaryFailure::InvalidTransition);
        }
        self.active.push(stage);
        Ok(())
    }

    pub(crate) fn deinitialize(&mut self, stage: Stage) -> Result<(), BoundaryFailure> {
        if self.active.last().copied() != Some(stage) {
            return Err(BoundaryFailure::InvalidTransition);
        }
        self.active.pop();
        Ok(())
    }

    pub(crate) fn stage(&self) -> Option<Stage> {
        self.active.last().copied()
    }
}

pub(crate) fn supports_godot_api(major: i64, minor: i64) -> bool {
    (major, minor) == SUPPORTED_GODOT_API
}

pub(crate) fn catch_boundary<T>(operation: impl FnOnce() -> T) -> Result<T, BoundaryFailure> {
    catch_unwind(AssertUnwindSafe(operation)).map_err(|_| BoundaryFailure::Panic)
}

pub(crate) fn initialize_godot_stage(stage: InitStage) -> Result<Option<Stage>, BoundaryFailure> {
    let Some(stage) = Stage::from_godot(stage) else {
        return Ok(None);
    };
    catch_boundary(|| {
        let mut lifecycle = runtime_lifecycle()
            .lock()
            .map_err(|_| BoundaryFailure::Poisoned)?;
        lifecycle.initialize(stage)?;
        Ok(Some(stage))
    })?
}

pub(crate) fn deinitialize_godot_stage(stage: InitStage) -> Result<Option<Stage>, BoundaryFailure> {
    let Some(stage) = Stage::from_godot(stage) else {
        return Ok(None);
    };
    catch_boundary(|| {
        let mut lifecycle = runtime_lifecycle()
            .lock()
            .map_err(|_| BoundaryFailure::Poisoned)?;
        lifecycle.deinitialize(stage)?;
        Ok(Some(stage))
    })?
}

pub(crate) fn current_stage() -> &'static str {
    runtime_lifecycle()
        .lock()
        .ok()
        .and_then(|lifecycle| lifecycle.stage())
        .map(Stage::label)
        .unwrap_or("inactive")
}

fn runtime_lifecycle() -> &'static Mutex<Lifecycle> {
    RUNTIME_LIFECYCLE.get_or_init(|| Mutex::new(Lifecycle::default()))
}
