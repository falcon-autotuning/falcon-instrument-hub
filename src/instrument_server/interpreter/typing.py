"""Typing for the interpreter module."""

from typing import TypeAlias

from falcon_core.communications.messages.measurement_request import MeasurementRequest
from nats.aio.client import Client

from ..daemons.interpreter_sync_sender import InterpreterSyncSender
from ..instrument_drivers.typing import DriverConfig, Index, PropertyName

ID: TypeAlias = str

__all__ = [
    "DriverConfig",
    "Index",
    "PropertyName",
    "MeasurementRequest",
    "Client",
    "InterpreterSyncSender",
]
