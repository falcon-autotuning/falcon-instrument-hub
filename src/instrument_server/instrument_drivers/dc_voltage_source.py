"""A staircase buffered voltage source daemon."""

from typing import TYPE_CHECKING

from .base_instrument_driver import BaseInstrumentDriver
from .constants import SUPPORTED_PROPERTIES
from .dependancies import np

if TYPE_CHECKING:
    from instrument_server.instrument_drivers.typing import GetCommand

    from .typing import (
        GetIndexedCommand,
        Index,
        SetCommand,
        SetIndexedCommand,
    )


class DCVoltageSource(BaseInstrumentDriver):
    """A generic voltage source driver."""

    _sub_source_count: int
    _indexes: list["Index"]
    _global_index = -1
    _set_voltage: "SetIndexedCommand[float]"
    _get_voltage: "GetIndexedCommand[float]"
    _set_slope: "SetIndexedCommand[float]"
    _get_slope: "GetIndexedCommand[float]"
    _voltage_bounds: tuple[float, float]
    _slope_bounds: tuple[float, float]

    def __init__(self, *args, **kwargs) -> None:
        """Initialize the DC voltage source driver."""
        super().__init__(*args, **kwargs)
        self._indexes = [
            count
            for count in np.linspace(
                start=1,
                stop=self._sub_source_count,
                num=self._sub_source_count,
            )
        ]

        [
            (
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
