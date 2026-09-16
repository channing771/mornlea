extends SceneTree

# The identity probe stays in GDScript because it validates native registration before
# the Python host is allowed to participate in the project lifecycle.
var _failures := PackedStringArray()


func _initialize() -> void:
	call_deferred("_run")


func _run() -> void:
	_expect(ClassDB.class_exists("MornleaClientBridge"), "MornleaClientBridge is not registered")
	if not _failures.is_empty():
		_finish()
		return
	_expect(_identity("client_core_abi_major") == 1, "unexpected client-core ABI major")
	_expect(_identity("client_core_abi_minor") == 0, "unexpected client-core ABI minor")
	_expect(_identity("godot_api_major") == 4, "unexpected Godot API major")
	_expect(_identity("godot_api_minor") == 7, "unexpected Godot API minor")
	_expect(_identity("godot_rust_version") == "0.5.5", "unexpected godot-rust version")
	_expect(_identity("lifecycle_stage") == "main-loop", "unexpected extension lifecycle stage")
	_expect(
		ClassDB.class_call_static("MornleaClientBridge", "supports_godot_api", 4, 7),
		"pinned Godot API was rejected"
	)
	_expect(
		not ClassDB.class_call_static("MornleaClientBridge", "supports_godot_api", 4, 6),
		"wrong Godot API was accepted"
	)
	var engine_version := Engine.get_version_info()
	_expect(engine_version.major == 4 and engine_version.minor == 7, "runtime Godot API is not 4.7")
	_finish()


func _identity(method: StringName) -> Variant:
	return ClassDB.class_call_static("MornleaClientBridge", method)


func _expect(condition: bool, message: String) -> void:
	if not condition:
		_failures.append(message)


func _finish() -> void:
	if _failures.is_empty():
		print("Godot bridge identity check passed.")
		quit(0)
		return
	for failure in _failures:
		push_error(failure)
	quit(1)
