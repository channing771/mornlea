class Vector3:
    @staticmethod
    def new3(x: float, y: float, z: float) -> Vector3: ...

class PackedByteArray:
    @staticmethod
    def from_list(values: list[int]) -> PackedByteArray: ...
