"""A staircase buffered voltage source daemon."""

from typing import TYPE_CHECKING

import numpy as np

from ..constants import SUPPORTED_PROPERTIES
from .base_instrument_daemon import BaseInstrumentDaemon

if TYPE_CHECKING:
    from instrument_server.instrument_daemons.typing import GetCommand

    from .typing import (
        GetIndexedCommand,
        Index,
        PropertyName,
        SetCommand,
        SetIndexedCommand,
        Staircase,
    )


class StaircaseVoltageSource(BaseInstrumentDaemon):
    """A staircase buffered voltage source daemon.

    This class implements a staircase buffered voltage source.
    """

    _can_compile: bool

    _num_sim_waveforms: int
    _sub_source_count: int
    _indexes: list["Index"]
    _global_index = -1
    _set_voltage: "SetIndexedCommand"
    _get_voltage: "GetIndexedCommand"
    _set_slope: "SetIndexedCommand"
    _get_slope: "GetIndexedCommand"
    _voltage_bounds: tuple[float, float]
    _slope_bounds: tuple[float, float]
    _staircase_bounds: tuple["Staircase", "Staircase"]

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
            get_cmd=lambda: self._can_compile,
        )

        [
            (
                self.program_property(
                    property_name=SUPPORTED_PROPERTIES.STAIRCASE,
                    index=index,
                    bounds=self._staircase_bounds,
                    set_cmd=self._make_staircase(idx=index),
                ),
                self.program_property(
                    property_name=SUPPORTED_PROPERTIES.VOLTAGE_STATE,
                    index=index,
                    bounds=self._voltage_bounds,
                    get_cmd=self._make_get_voltage(idx=index),
                    set_cmd=self._make_set_voltage(idx=index),
                ),
                self.program_property(
                    property_name=SUPPORTED_PROPERTIES.SLOPE,
                    index=index,
                    bounds=self._slope_bounds,
                    get_cmd=self._make_get_slope(idx=index),
                    set_cmd=self._make_set_slope(idx=index),
                ),
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

    def set_property(
        self,
        property_name: "PropertyName",
        index: "Index",
        value: int | float | str,
    ) -> None:
        staircase_property = property_name in {
            SUPPORTED_PROPERTIES.STAIRCASE_REPEAT,
            SUPPORTED_PROPERTIES.STAIRCASE_STEPS,
            SUPPORTED_PROPERTIES.STAIRCASE_STEP_WIDTH,
            SUPPORTED_PROPERTIES.STAIRCASE_STOP,
        }
        if staircase_property:
            self._can_compile = False
        super().set_property(property_name, index, value)
        if staircase_property and (
            index in self._repeats
            and index in self._num_steps
            and index in self._step_widths
        ):
            self.compile()
            self._can_compile = True

    def compile(self) -> None:
        """Compile the various staircase waveforms for the device.

        Raises:
            RuntimeError: If the user wants to compile more waveforms than supported.

        """
        unique_fingerprints = set(
            zip(
                self._repeats.values(),
                self._num_steps.values(),
                self._step_widths.values(),
            )
        )
        if len(unique_fingerprints) > self._num_sim_waveforms:
            msg = f"Cannot compile {len(unique_fingerprints)} waveforms, only {self._num_sim_waveforms} supported."
            raise RuntimeError(msg)
