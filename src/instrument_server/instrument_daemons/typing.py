"""This is typing for the instrument daemon modules."""

from collections.abc import Callable
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from typing import TypeAlias

SetCommand: "TypeAlias" = Callable[[int | float | str], None]
GetCommand: "TypeAlias" = Callable[[], int | float | str]
Index: "TypeAlias" = str
PropertyName: "TypeAlias" = str
Bounds: "TypeAlias" = tuple[int | float, int | float]
