"""This is typing for the instrument daemon modules."""

from collections.abc import Callable
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from typing import TypeAlias

SetCommand: "TypeAlias" = Callable[[int | float | str], None]
GetCommand: "TypeAlias" = Callable[[], int | float | str]
Index: "TypeAlias" = float
SetIndexedCommand: "TypeAlias" = Callable[[Index, int | float | str], None]
GetIndexedCommand: "TypeAlias" = Callable[[Index], int | float | str]
PropertyName: "TypeAlias" = str
Bounds: "TypeAlias" = tuple[int | float, int | float]
