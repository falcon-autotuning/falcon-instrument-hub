"""Typing for the messaging daemon module."""

from collections.abc import Awaitable, Callable
from typing import Any, TypeAlias

from falcon_core.instrument_interfaces.instrument import Instrument
from falcon_core.instrument_interfaces.names import InstrumentPort
from falcon_core.physics.device_structures import Connection
from nats.aio.client import Client
from nats.aio.msg import Msg
from numpy.typing import NDArray

from ..instrument_drivers.base_instrument_driver import BaseInstrumentDriver
from ..instrument_drivers.typing import Index, PropertyJson, PropertyName, PropertyValue
from ..interpreter.typing import ID

Dimension: "TypeAlias" = int
DimensionIndex: "TypeAlias" = str
ConnectionName: "TypeAlias" = str
Data: "TypeAlias" = str
AxisLabel: "TypeAlias" = str
Unit: "TypeAlias" = str
AxisMetadata: "TypeAlias" = dict[str, Unit | int]
AxisLabels: "TypeAlias" = dict[AxisLabel, AxisMetadata]
Domain: "TypeAlias" = dict[str, Data | AxisLabels]
Range: "TypeAlias" = dict[str, Data | Unit]
Ranges: "TypeAlias" = dict[AxisLabel, Range]
Domains: "TypeAlias" = dict[DimensionIndex, Domain]
Dimensions: "TypeAlias" = dict[DimensionIndex, Dimension]
Metadata: "TypeAlias" = dict[str, str | int | float]
__all__ = [
    "Connection",
    "NDArray",
    "InstrumentPort",
    "Instrument",
    "ID",
    "PropertyName",
    "Index",
    "Client",
    "Any",
    "Msg",
    "Awaitable",
    "Callable",
    "PropertyJson",
    "PropertyValue",
    "BaseInstrumentDriver",
]
