"""A staircase buffered voltage source daemon."""

from typing import TYPE_CHECKING

import numpy as np

from ..constants import SUPPORTED_PROPERTIES
from .base_instrument_daemon import BaseInstrumentDaemon

if TYPE_CHECKING:
    from instrument_server.instrument_daemons.typing import GetCommand

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
    _repeat_bounds: tuple[int, int]
    _num_steps_bounds: tuple[int, int]

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
                property_name=SUPPORTED_PROPERTIES.STAIRCASE_STEPS,
                index=index,
                bounds=self._num_steps_bounds,
                set_cmd=self._make_set_num_steps(idx=index),
            )
            for index in self._indexes
        ]
        [
            self.program_property(
                property_name=SUPPORTED_PROPERTIES.STAIRCASE_REPEAT,
                index=index,
                bounds=self._repeat_bounds,
                set_cmd=self._make_set_repeat(idx=index),
            )
            for index in self._indexes
        ]
        [
            self.program_property(
                property_name=SUPPORTED_PROPERTIES.STAIRCASE_STEP_WIDTH,
                index=index,
                bounds=self._step_width_bounds,
                set_cmd=self._make_set_step_width(idx=index),
            )
            for index in self._indexes
        ]
        [
            self.program_property(
                property_name=SUPPORTED_PROPERTIES.VOLTAGE_STATE,
                index=index,
                bounds=self._voltage_bounds,
                get_cmd=self._make_get_voltage(idx=index),
                set_cmd=self._make_set_voltage(idx=index),
            )
            for index in self._indexes
        ]
        [
            self.program_property(
                property_name=SUPPORTED_PROPERTIES.SLOPE,
                index=index,
                bounds=self._slope_bounds,
                get_cmd=self._make_get_slope(idx=index),
                set_cmd=self._make_set_slope(idx=index),
            )
            for index in self._indexes
        ]

    def _make_get_voltage(self, idx: "Index") -> "GetCommand[float]":
        """Makes a wrapper for the get voltage command.

        Args:
            idx: The index of the voltage source.

        Returns:
                A lambda function that returns the voltage of the source.
        """
        return lambda: self._get_voltage(idx)

    def _make_get_slope(self, idx: "Index") -> "GetCommand[float]":
        """Makes a wrapper for the get slope command.

        Args:
            idx: The index of the voltage source.

        Returns:
                A lambda function that returns the slope of the source.
        """
        return lambda: self._get_slope(idx)

    def _make_set_voltage(self, idx: "Index") -> "SetCommand[float]":
        """Makes a wrapper for the set voltage command.

        Args:
            idx: The index of the voltage source.

        Returns:
                A lambda function that sets the voltage of the source.
        """
        return lambda voltage: self._set_voltage(idx, voltage)

    def _make_set_slope(self, idx: "Index") -> "SetCommand[float]":
        """Makes a wrapper for the set slope command.

        Args:
            idx: The index of the voltage source.

        Returns:
                A lambda function that sets the slope of the source.
        """
        return lambda slope: self._set_slope(idx, slope)

    def _make_set_step_width(self, idx: "Index") -> "SetCommand[float]":
        """Set the step width of the staircase.

        Args:
            idx: The index of the voltage source.
            step_width: The step width of the staircase.

        Returns:
            a lambda function that sets the step width of the staircase.
        """
        return lambda step_width: self._step_widths.__setitem__(idx, step_width)

    def _make_set_num_steps(self, idx: "Index") -> "SetCommand[int]":
        """Set the number of steps of the staircase.

        Args:
            idx: The index of the voltage source.
            num_steps: The number of steps of the staircase.

        Returns:
            a lambda function that sets the number of steps of the staircase.
        """
        return lambda num_steps: self._num_steps.__setitem__(idx, num_steps)

    def _make_set_repeat(self, idx: "Index") -> "SetCommand[int]":
        """Set the number of repeats of the staircase.

        Args:
            idx: The index of the voltage source.
            repeat: The number of repeats of the staircase.

        Returns:
            a lambda function that sets the number of repeats of the staircase.
        """
        return lambda repeat: self._repeats.__setitem__(idx, repeat)
