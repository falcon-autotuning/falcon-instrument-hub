"""This is typing for the instrument daemon modules."""

from collections.abc import Callable
from typing import TYPE_CHECKING, TypeVar

from ..daemons.instrument_sync_sender import InstrumentSyncSender
from ..daemons.interpreter_sync_sender import InterpreterSyncSender

if TYPE_CHECKING:
    from typing import TypeAlias

Property: "TypeAlias" = str | int | float
ComboProperty: "TypeAlias" = tuple[Property, ...]
PropertyValue: "TypeAlias" = Property | ComboProperty
T = TypeVar("T", bound=PropertyValue)
SetCommand: "TypeAlias" = Callable[[T], None]
GetCommand: "TypeAlias" = Callable[[], T]
Index: "TypeAlias" = float
SetIndexedCommand: "TypeAlias" = Callable[[Index, T], None]
GetIndexedCommand: "TypeAlias" = Callable[[Index], T]
PropertyName: "TypeAlias" = str
Bounds: "TypeAlias" = tuple[T, T]
Staircase = tuple[int, int, int, float]
DriverConfig: "TypeAlias" = dict[PropertyName, dict[Index, dict[str, bool | Bounds]]]
__all__ = [
    "InstrumentSyncSender",
    "InterpreterSyncSender",
]
