//! Desktop semantic input names and their client-core wire vocabulary.

/// Actions collected by the desktop adapter. The names are stable Godot
/// InputMap identifiers; unsupported actions intentionally have no wire code.
pub const ACTIONS: &[&str] = &[
    "move_forward",
    "move_back",
    "move_left",
    "move_right",
    "jump",
    "sprint",
    "sneak",
    "primary",
    "secondary",
    "hotbar",
    "f5",
    "escape",
    "toggle_camera",
    "ui_cancel",
];

/// Actions that can be represented by the current input-family contract.
pub const SUPPORTED_ACTIONS: &[&str] = &[
    "move_forward",
    "move_back",
    "move_left",
    "move_right",
    "jump",
    "sprint",
    "sneak",
    "primary",
    "secondary",
];

/// Actions consumed by the Godot host without a client-core wire event.
pub const LOCAL_ACTIONS: &[&str] = &["f5", "escape"];

/// Return the producer action code, or `None` when no compatible semantic
/// channel exists. The adapter must drop `None` rather than invent a command.
pub fn wire_action(name: &str) -> Option<u32> {
    match name {
        "jump" => Some(3),
        "primary" => Some(4),
        "secondary" => Some(5),
        "sprint" => Some(6),
        "sneak" => Some(7),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::{ACTIONS, LOCAL_ACTIONS, SUPPORTED_ACTIONS, wire_action};

    #[test]
    fn desktop_action_set_is_explicit_and_has_no_mobile_controls() {
        assert!(ACTIONS.contains(&"move_forward"));
        assert!(ACTIONS.contains(&"hotbar"));
        assert!(LOCAL_ACTIONS.contains(&"f5"));
        assert!(LOCAL_ACTIONS.contains(&"escape"));
        assert!(
            !ACTIONS
                .iter()
                .any(|name| name.contains("touch") || name.contains("joystick"))
        );
    }

    #[test]
    fn unsupported_actions_have_no_wire_mapping() {
        assert_eq!(wire_action("hotbar"), None);
        assert_eq!(wire_action("f5"), None);
        assert_eq!(wire_action("escape"), None);
        assert_eq!(wire_action("toggle_camera"), None);
        assert!(SUPPORTED_ACTIONS.iter().all(|name| *name == "move_forward"
            || wire_action(name).is_some()
            || name.starts_with("move_")));
    }

    #[test]
    fn primary_and_secondary_reuse_existing_core_semantics() {
        assert_eq!(wire_action("primary"), Some(4));
        assert_eq!(wire_action("secondary"), Some(5));
    }
}
