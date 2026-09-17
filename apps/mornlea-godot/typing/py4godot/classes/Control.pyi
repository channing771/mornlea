from py4godot.classes.Node import Node

class Control(Node):
    """Typed base for Python-authored Control feature scenes.

    The stub intentionally declares no layout API: feature scenes serialize
    anchors and layout in their ``.tscn`` files, while feature scripts only
    need the ``Node`` surface plus the Control identity for scene attachment.
    """
