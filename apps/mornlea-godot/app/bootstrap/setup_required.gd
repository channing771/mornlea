extends Control

# This view depends only on built-in Godot controls so diagnostics survive a missing
# Python runtime or project bridge.

func configure(
	target: String,
	missing: PackedStringArray,
	mismatched: PackedStringArray,
	prepare_command: String
) -> void:
	%Target.text = "Target: %s" % target
	%Missing.text = "Missing: %s" % ("none" if missing.is_empty() else ", ".join(missing))
	%Mismatched.text = (
		"Identity mismatch: %s" % ("none" if mismatched.is_empty() else ", ".join(mismatched))
	)
	%Command.text = "Preparation command: %s" % prepare_command
