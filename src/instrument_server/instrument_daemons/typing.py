"""This is typing for the instrument daemon modules."""

from collections.abc import Callable
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from typing import TypeAlias
from typing import TypeVar

T = TypeVar("T", bound=str | int | float)
SetCommand: "TypeAlias" = Callable[[T], None]
GetCommand: "TypeAlias" = Callable[[], T]
Index: "TypeAlias" = float
SetIndexedCommand: "TypeAlias" = Callable[[Index, T], None]
GetIndexedCommand: "TypeAlias" = Callable[[Index], T]
PropertyName: "TypeAlias" = str
Bounds: "TypeAlias" = tuple[int | float, int | float]
