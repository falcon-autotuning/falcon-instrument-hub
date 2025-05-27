"""Typing for the messaging daemon module."""

from collections.abc import Awaitable, Callable
from typing import Any

from falcon_core.instrument_interfaces.names import InstrumentPort
from falcon_core.math.arrays.base_array import BaseArray
from falcon_core.math.arrays.measured_array_1D import MeasuredArray1D
from falcon_core.physics.device_structures import Connection
from falcon_core.typing import Instrument
from instrument_templates.base_instrument_driver import BaseInstrumentDriver
from instrument_templates.typing import Index, PropertyJson, PropertyName, PropertyValue
from nats.aio.client import Client
from nats.aio.msg import Msg
from numpy.typing import NDArray

type Dimension = int
type DimensionIndex = str
type ConnectionName = str
type Data = str
type AxisLabel = str
type Unit = str
type AxisMetadata = dict[str, Unit | int]
type AxisLabels = dict[AxisLabel, AxisMetadata]
type Domain = dict[str, Data | AxisLabels]
type Range = dict[str, Data | Unit]
type Ranges = dict[AxisLabel, Range]
type Domains = dict[DimensionIndex, Domain]
type Dimensions = dict[DimensionIndex, Dimension]
type Metadata = dict[str, str | int | float]
type ID = str
__all__ = [
    "Connection",
    "BaseArray",
    "MeasuredArray1D",
    "NDArray",
    "InstrumentPort",
    "Instrument",
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
