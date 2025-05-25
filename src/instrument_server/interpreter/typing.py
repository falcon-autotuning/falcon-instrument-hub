"""Typing for the interpreter module."""

from typing import TypeAlias

from falcon_core.communications.messages.measurement_request import MeasurementRequest
from falcon_core.instrument_interfaces.names import InstrumentPort
from nats.aio.client import Client

from ..daemons.interpreter_sync_sender import InterpreterSyncSender
from ..instrument_drivers.typing import (
    Bounds,
    DriverConfig,
    Index,
    PropertyJson,
    PropertyName,
)

ID: TypeAlias = str
NamedIndex: TypeAlias = str  # the abstract name coming from falcon
__all__ = [
    "DriverConfig",
    "PropertyJson",
    "InstrumentPort",
    "Bounds",
    "Index",
    "PropertyName",
    "MeasurementRequest",
    "Client",
    "InterpreterSyncSender",
]
