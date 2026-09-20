from py4godot.classes.Node import Node

class Node3D(Node):
    """Typed base for Python-authored 3D compartment scenes.

    The stub intentionally declares no transform API: the near-field terrain
    compartment is an addressable scene anchor whose vertices, mesh buffers,
    and rendering-server instances are owned by the Rust renderer, so Python
    scripts need the ``Node`` surface plus the Node3D identity for scene
    attachment and spatial ownership only.
    """
