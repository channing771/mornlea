"""pytest 根夹具：把 workspace 根加入导入路径，使未发布的 `app.*` 可导入。"""

from __future__ import annotations

import sys
from pathlib import Path

_ROOT = Path(__file__).resolve().parent
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))
