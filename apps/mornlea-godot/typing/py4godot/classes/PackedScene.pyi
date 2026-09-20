from py4godot.classes.Node import Node
from py4godot.classes.Resource import Resource

class PackedScene(Resource):
    def instantiate(self, edit_state: int = 0) -> Node | None: ...
