"""A DC current sink driver for the instrument server."""

from typing import TYPE_CHECKING

from .base_instrument_driver import BaseInstrumentDriver
from .constants import SUPPORTED_PROPERTIES
from .dependancies import np

if TYPE_CHECKING:
    from .typing import (
        GetCommand,
        GetIndexedCommand,
        Index,
        SetCommand,
        SetIndexedCommand,
    )


class DCCurrentSink(BaseInstrumentDriver):
    """A generic current sink driver."""

    _sub_source_count: int
    _indexes: list["Index"]
    _global_index = -1
    _get_current: "GetIndexedCommand[float]"
    _set_number_of_bins: "SetIndexedCommand[int]"
    _get_number_of_bins: "GetIndexedCommand[int]"
    _set_sample_rate: "SetIndexedCommand[float]"
    _get_sample_rate: "GetIndexedCommand[float]"
    _set_timeout: "SetIndexedCommand[float]"
    _number_of_bins_bounds: tuple[int, int]
    _sample_rate_bounds: tuple[int, int]

    def __init__(self, *args, **kwargs) -> None:
        """Initialize the DC current sink driver."""
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
                    property_name=SUPPORTED_PROPERTIES.CURRENT_STATE,
                    index=index,
                    get_cmd=self._make_get_current(idx=index),
                ),
                self.program_property(
                    property_name=SUPPORTED_PROPERTIES.TIMEOUT,
                    index=index,
                    set_cmd=self._make_set_timeout(idx=index),
                ),
                self.program_property(
                    property_name=SUPPORTED_PROPERTIES.NUMBER_OF_BINS,
                    index=index,
                    bounds=self._number_of_bins_bounds,
                    get_cmd=self._make_get_number_of_bins(idx=index),
                    set_cmd=self._make_set_number_of_bins(idx=index),
                ),
                self.program_property(
                    property_name=SUPPORTED_PROPERTIES.SAMPLE_RATE,
                    index=index,
                    bounds=self._number_of_bins_bounds,
                    get_cmd=self._make_get_sample_rate(idx=index),
                    set_cmd=self._make_set_sample_rate(idx=index),
                ),
            )
            for index in self._indexes
        ]

    def _make_get_current(self, idx: "Index") -> "GetCommand[float]":
        """Makes a wrapper for the get current command.

        Args:
            idx: The index of the current sink.

        Returns:
            A GetIndexedCommand that gets the current for the given index.
        """
        return lambda: self._get_current(idx)

    def _make_set_timeout(self, idx: "Index") -> "SetCommand[float]":
        """Makes a wrapper for the set timeout command.

        Args:
            idx: The index of the current sink.

        Returns:
            A SetIndexedCommand that sets the timeout for the given index.
        """
        return lambda timeout: self._set_timeout(idx, timeout)

    def _make_get_number_of_bins(self, idx: "Index") -> "GetCommand[int]":
        """Makes a wrapper for the get number of bins command.

        Args:
            idx: The index of the current sink.

        Returns:
            A GetIndexedCommand that gets the number of bins for the given index.
        """
        return lambda: self._get_number_of_bins(idx)

    def _make_set_number_of_bins(self, idx: "Index") -> "SetCommand[int]":
        """Makes a wrapper for the set number of bins command.

        Args:
            idx: The index of the current sink.

        Returns:
            A SetIndexedCommand that sets the number of bins for the given index.
        """
        return lambda number_of_bins: self._set_number_of_bins(idx, number_of_bins)

    def _make_get_sample_rate(self, idx: "Index") -> "GetCommand[float]":
        """Makes a wrapper for the get sample rate command.

        Args:
            idx: The index of the current sink.

        Returns:
            A GetIndexedCommand that gets the sample rate for the given index.
        """
        return lambda: self._get_sample_rate(idx)

    def _make_set_sample_rate(self, idx: "Index") -> "SetCommand[float]":
        """Makes a wrapper for the set sample rate command.

        Args:
            idx: The index of the current sink.

        Returns:
            A SetIndexedCommand that sets the sample rate for the given index.
        """
        return lambda sample_rate: self._set_sample_rate(idx, sample_rate)
