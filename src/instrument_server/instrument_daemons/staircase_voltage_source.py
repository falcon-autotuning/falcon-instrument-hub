"""A staircase buffered voltage source daemon."""

from typing import TYPE_CHECKING

import numpy as np

from ..constants import SUPPORTED_PROPERTIES
from .base_instrument_daemon import BaseInstrumentDaemon

if TYPE_CHECKING:
    from .typing import GetIndexedCommand, Index, SetCommand, SetIndexedCommand


class StaircaseVoltageSource(BaseInstrumentDaemon):
    """A staircase buffered voltage source daemon.

    This class implements a staircase buffered voltage source.
    """

    _compiled: bool

    _num_sim_waveforms: int
    _sub_source_count: int
    _indexes: list["Index"]
    _global_index = -1
    _set_trigger: "SetCommand"
    _set_voltage: "SetIndexedCommand"
    _get_voltage: "GetIndexedCommand"
    _set_slope: "SetIndexedCommand"
    _get_slope: "GetIndexedCommand"
    _voltage_bounds: tuple[float, float]
    _slope_bounds: tuple[float, float]
    _step_width_bounds: tuple[float, float]

    _step_widths: dict["Index", float]
    _num_steps: dict["Index", int]
    _repeats: dict["Index", int]

    def __init__(self, *args, **kwargs) -> None:
        """Initialize the staircase buffered voltage source daemon."""
        super().__init__(*args, **kwargs)
        self._indexes = [
            count
            for count in np.linspace(
                start=1,
                stop=self._sub_source_count,
                num=self._sub_source_count,
            )
        ]

        self.program_property(
            property_name=SUPPORTED_PROPERTIES.TRIGGER,
            index=self._global_index,
            get_cmd=lambda: "None",
            set_cmd=self._set_trigger,
        )
        self.program_property(
            property_name=SUPPORTED_PROPERTIES.SUPPORTS_ARBITRARY_OFFSET,
            index=self._global_index,
            get_cmd=lambda: True,
        )
        self.program_property(
            property_name=SUPPORTED_PROPERTIES.SUPPORTS_ARBITRARY_SCALING,
            index=self._global_index,
            get_cmd=lambda: True,
        )
        self.program_property(
            property_name=SUPPORTED_PROPERTIES.NUMBER_SIMULTANEOUS_WAVEFORMS,
            index=self._global_index,
            get_cmd=lambda: self._num_sim_waveforms,
        )
        self.program_property(
            property_name=SUPPORTED_PROPERTIES.TRIGGER_READY,
            index=self._global_index,
            get_cmd=lambda: self._compiled,
        )

        [
            self.program_property(
                property_name=SUPPORTED_PROPERTIES.STAIRCASE_STEP_WIDTH,
                index=index,
                bounds=self._step_width_bounds,
                set_cmd=lambda width: self._step_widths.__setitem__(index, width),
            )
            for index in self._indexes
        ]
        [
            self.program_property(
                property_name=SUPPORTED_PROPERTIES.VOLTAGE_STATE,
                index=index,
                bounds=self._voltage_bounds,
                get_cmd=lambda: self._get_voltage(index),
                set_cmd=lambda voltage: self._set_voltage(index, voltage),
            )
            for index in self._indexes
        ]
        [
            self.program_property(
                property_name=SUPPORTED_PROPERTIES.SLOPE,
                index=index,
                bounds=self._slope_bounds,
                get_cmd=lambda: self._get_slope(index),
                set_cmd=lambda voltage: self._set_slope(index, voltage),
            )
            for index in self._indexes
        ]
