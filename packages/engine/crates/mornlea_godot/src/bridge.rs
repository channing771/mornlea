use godot::classes::{IRefCounted, RefCounted};
use godot::prelude::*;

use crate::lifecycle;

const CLIENT_CORE_ABI_MAJOR: i64 = 1;
const CLIENT_CORE_ABI_MINOR: i64 = 0;
const GODOT_API_MAJOR: i64 = 4;
const GODOT_API_MINOR: i64 = 7;
const GODOT_RUST_VERSION: &str = "0.5.5";
#[cfg(test)]
pub(crate) const BRIDGE_CLASS_NAME: &str = "MornleaClientBridge";
#[cfg(test)]
pub(crate) const IDENTITY_METHODS: [&str; 7] = [
    "client_core_abi_major",
    "client_core_abi_minor",
    "godot_api_major",
    "godot_api_minor",
    "godot_rust_version",
    "lifecycle_stage",
    "supports_godot_api",
];

/// Minimal identity surface used to qualify the Godot/Rust integration boundary.
///
/// Gameplay methods remain absent until the versioned client-core ABI is implemented.
#[derive(GodotClass)]
#[class(base=RefCounted)]
struct MornleaClientBridge {
    #[base]
    base: Base<RefCounted>,
}

#[godot_api]
impl IRefCounted for MornleaClientBridge {
    fn init(base: Base<RefCounted>) -> Self {
        Self { base }
    }
}

#[godot_api]
impl MornleaClientBridge {
    // Identity calls are deliberately allocation-free except for the Godot string
    // conversion and do not create a second path to engine or gameplay state.
    #[func]
    fn client_core_abi_major() -> i64 {
        CLIENT_CORE_ABI_MAJOR
    }

    #[func]
    fn client_core_abi_minor() -> i64 {
        CLIENT_CORE_ABI_MINOR
    }

    #[func]
    fn godot_api_major() -> i64 {
        GODOT_API_MAJOR
    }

    #[func]
    fn godot_api_minor() -> i64 {
        GODOT_API_MINOR
    }

    #[func]
    fn godot_rust_version() -> GString {
        GODOT_RUST_VERSION.into()
    }

    #[func]
    fn lifecycle_stage() -> GString {
        lifecycle::current_stage().into()
    }

    #[func]
    fn supports_godot_api(major: i64, minor: i64) -> bool {
        lifecycle::supports_godot_api(major, minor)
    }
}

#[cfg(test)]
mod tests {
    use super::{
        CLIENT_CORE_ABI_MAJOR, CLIENT_CORE_ABI_MINOR, GODOT_API_MAJOR, GODOT_API_MINOR,
        GODOT_RUST_VERSION,
    };

    #[test]
    fn bridge_identity_matches_pinned_dependencies() {
        assert_eq!((CLIENT_CORE_ABI_MAJOR, CLIENT_CORE_ABI_MINOR), (1, 0));
        assert_eq!((GODOT_API_MAJOR, GODOT_API_MINOR), (4, 7));
        assert_eq!(GODOT_RUST_VERSION, "0.5.5");
    }
}
