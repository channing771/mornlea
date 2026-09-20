extends Node

# Bootstrap intentionally owns diagnostics only; it never imports Python features or
# starts networking before both native extensions have passed identity checks.
const SETUP_REQUIRED_SCENE: PackedScene = preload("res://app/bootstrap/setup_required.tscn")
const APP_ROOT_SCENE_PATH := "res://app/host/app_root.tscn"
const EXPECTED_GODOT_VERSION := "4.7.2-stable"
const PYTHON_DESCRIPTOR := "res://addons/py4godot/python.gdextension"
const BRIDGE_DESCRIPTOR := "res://addons/mornlea_bridge/mornlea_bridge.gdextension"
const REQUIRED_ARTIFACTS := [
	{
		"id": "python-extension",
		"path": PYTHON_DESCRIPTOR,
	},
	{
		"id": "python-plugin",
		"path": "res://addons/py4godot/cpython-3.14.4-darwin64/python/bin/pythonscript.dylib",
	},
	{
		"id": "python-bridge",
		"path": "res://addons/py4godot/cpython-3.14.4-darwin64/python/bin/main.dylib",
	},
	{
		"id": "python-interpreter",
		"path": "res://addons/py4godot/cpython-3.14.4-darwin64/python/lib/libpython3.14.dylib",
	},
	{
		"id": "python-stdlib",
		"path": "res://addons/py4godot/cpython-3.14.4-darwin64/python/lib/python3.14/os.py",
	},
	{
		"id": "project-bridge-extension",
		"path": BRIDGE_DESCRIPTOR,
	},
	{
		"id": "project-bridge-library",
		"path": "res://addons/mornlea_bridge/bin/macos-universal/debug/libmornlea_godot.dylib",
	},
]


func _ready() -> void:
	var target := _target_triple()
	var missing := _missing_artifacts()
	var mismatched := _identity_mismatches(target)
	var prepare_command := (
		"scripts/godot/build-python-runtime.sh --verify --offline"
		+ " && scripts/godot/build-extension.sh --target %s --profile debug --verify" % target
	)
	if missing.is_empty() and mismatched.is_empty():
		if _handoff_to_python():
			print(
				(
					"[mornlea-bootstrap] state=ready target=%s missing=none mismatched=none"
					+ " python=imported network=not-started"
				)
				% target
			)
			return
		mismatched.append("python-host-load")
	var setup_required := SETUP_REQUIRED_SCENE.instantiate()
	add_child(setup_required)
	setup_required.configure(target, missing, mismatched, prepare_command)
	var missing_text := "none" if missing.is_empty() else ",".join(missing)
	var mismatched_text := "none" if mismatched.is_empty() else ",".join(mismatched)
	print(
		(
			"[mornlea-bootstrap] state=setup-required target=%s missing=%s mismatched=%s"
			+ " prepare=%s python=not-imported network=not-started"
		)
		% [target, missing_text, mismatched_text, prepare_command]
	)


func _handoff_to_python() -> bool:
	# The path is loaded only after both native distribution identities pass, so
	# opening an incomplete checkout never parses or instantiates Python scripts.
	var resource := ResourceLoader.load(APP_ROOT_SCENE_PATH, "PackedScene", ResourceLoader.CACHE_MODE_REUSE)
	if not resource is PackedScene:
		return false
	var app_root := (resource as PackedScene).instantiate()
	if app_root == null:
		return false
	add_child(app_root)
	return true


func _missing_artifacts() -> PackedStringArray:
	var missing := PackedStringArray()
	for artifact in REQUIRED_ARTIFACTS:
		if not FileAccess.file_exists(artifact.path):
			missing.append("%s@%s" % [artifact.id, artifact.path])
	return missing


func _identity_mismatches(target: String) -> PackedStringArray:
	var mismatched := PackedStringArray()
	var version := Engine.get_version_info()
	var actual_godot_version := "%s.%s.%s-%s" % [
		version.get("major", -1),
		version.get("minor", -1),
		version.get("patch", -1),
		version.get("status", "unknown"),
	]
	if actual_godot_version != EXPECTED_GODOT_VERSION:
		mismatched.append("godot-version")
	if target.begins_with("unsupported-"):
		mismatched.append("desktop-target")
	# Descriptor text is inspected without loading either extension, preserving the
	# project's ability to open when generated binaries are absent or incompatible.
	if FileAccess.file_exists(PYTHON_DESCRIPTOR):
		if not _file_contains(PYTHON_DESCRIPTOR, 'entry_symbol = "initialize_pythonscript"'):
			mismatched.append("python-extension-entry-symbol")
		if not _file_contains(PYTHON_DESCRIPTOR, 'version = "4.7-alpha-21"'):
			mismatched.append("python-extension-version")
		if not _file_contains(
			PYTHON_DESCRIPTOR,
			(
				'macos.debug.arm64 = '
				+ '"cpython-3.14.4-darwin64/python/bin/pythonscript.dylib"'
			)
		):
			mismatched.append("python-extension-target")
	if FileAccess.file_exists(BRIDGE_DESCRIPTOR):
		if not _file_contains(BRIDGE_DESCRIPTOR, 'entry_symbol = "gdext_rust_init"'):
			mismatched.append("project-bridge-entry-symbol")
		if not _file_contains(BRIDGE_DESCRIPTOR, 'compatibility_minimum = "4.7"'):
			mismatched.append("project-bridge-godot-api")
		if not _file_contains(
			BRIDGE_DESCRIPTOR,
			(
				'macos.debug.arm64 = '
				+ '"res://addons/mornlea_bridge/bin/macos-universal/debug/libmornlea_godot.dylib"'
			)
		):
			mismatched.append("project-bridge-target")
	return mismatched


func _file_contains(path: String, expected: String) -> bool:
	var file := FileAccess.open(path, FileAccess.READ)
	if file == null:
		return false
	return file.get_as_text().contains(expected)


func _target_triple() -> String:
	# Platform scope is deliberately desktop-only; unsupported targets fail closed.
	if OS.get_name() != "macOS":
		return "unsupported-desktop-target"
	match Engine.get_architecture_name():
		"arm64":
			return "aarch64-apple-darwin"
		"x86_64":
			return "x86_64-apple-darwin"
		_:
			return "unsupported-desktop-target"
