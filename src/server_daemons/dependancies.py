"""Dependancies for the messaging daemon module."""

import asyncio
import json
import signal
from pathlib import Path

import nats
import numpy as np
from falcon_core.communications import Time
from falcon_core.communications.hdf5.data import HDF5Data
from falcon_core.communications.messages.measurement_request import MeasurementRequest
from falcon_core.communications.messages.measurement_response import MeasurementResponse
from falcon_core.math.arrays import MeasuredArray, MeasuredArray1D
from falcon_core.math.labelled_arrays import (
    LabelledMeasuredArray,
    LabelledMeasuredArrays,
)
from instrument_templates.constants import SUPPORTED_PROPERTIES
from instrument_templates.instrument_sync_sender import InstrumentSyncSender

__all__ = [
    "SUPPORTED_PROPERTIES",
    "Time",
    "signal",
    "InstrumentSyncSender",
    "Path",
    "HDF5Data",
    "MeasuredArray",
    "MeasuredArray1D",
    "LabelledMeasuredArray",
    "LabelledMeasuredArrays",
    "np",
    "MeasurementRequest",
    "MeasurementResponse",
    "asyncio",
    "json",
    "nats",
]
