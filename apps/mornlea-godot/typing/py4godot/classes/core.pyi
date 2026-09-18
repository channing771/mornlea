class Vector3:
    @staticmethod
    def new3(x: float, y: float, z: float) -> Vector3: ...

class Color:
    @staticmethod
    def new4(r: float, g: float, b: float, a: float) -> Color: ...

class PackedByteArray:
    @staticmethod
    def from_list(values: list[int]) -> PackedByteArray: ...
