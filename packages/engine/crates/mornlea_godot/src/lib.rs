mod bridge;

use godot::prelude::*;

// These constants make the compiled binding identity reviewable without exposing
// gameplay or client-core data through the provisional extension.
pub const GODOT_API_MAJOR: u8 = 4;
pub const GODOT_API_MINOR: u8 = 7;
pub const GODOT_RUST_VERSION: &str = "0.5.5";

struct MornleaGodotExtension;

// Godot owns extension lifetime; project resources are registered in scoped modules.
#[gdextension]
unsafe impl ExtensionLibrary for MornleaGodotExtension {}

#[cfg(test)]
mod tests {
    use super::{GODOT_API_MAJOR, GODOT_API_MINOR, GODOT_RUST_VERSION};

    #[test]
    fn dependency_identity_matches_the_pilot() {
        assert_eq!((GODOT_API_MAJOR, GODOT_API_MINOR), (4, 7));
        assert_eq!(GODOT_RUST_VERSION, "0.5.5");
    }
}
