// Command mornlea-godot-core is the future Go c-shared client core for the
// Godot pilot. A later change builds this package with -buildmode=c-shared so
// the mornlea_godot Rust GDExtension can pull client runtime results through
// the client-core ABI declared in include/mornlea_client_core.h. Until the
// export surface lands, this package exists to pin that ABI with the
// cross-language consistency tests shared with the Rust consumer.
package main

// main is the placeholder required for the future c-shared build. The exported
// ABI surface is added by later client-core changes; nothing runs here yet.
func main() {}
