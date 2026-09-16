// Command mornlea-godot-core is the Go c-shared client core for the Godot
// pilot. A later change builds this package with -buildmode=c-shared so the
// mornlea_godot Rust GDExtension can pull client runtime results through the
// client-core ABI declared in include/mornlea_client_core.h. The identity,
// lifecycle, connection, and input exports (create, destroy, status identity,
// the ABI version accessor, connect begin/poll, disconnect, and submit input)
// live in exports.go; the remaining family exports land with their later
// changes. Until the c-shared build lands, this package also pins the ABI with
// the cross-language consistency tests shared with the Rust consumer.
package main

// main is the placeholder required for the c-shared build. The exported ABI
// surface in exports.go is the real entry point; nothing runs here.
func main() {}
