"""A staircase buffered voltage source daemon."""

from typing import TYPE_CHECKING

from .constants import SUPPORTED_PROPERTIES
from .dc_voltage_source import DCVoltageSource

if TYPE_CHECKING:
    from instrument_server.instrument_drivers.typing import GetCommand

    from .typing import (
        Index,
        SetCommand,
        SetIndexedCommand,
        Staircase,
    )


class StaircaseVoltageSource(DCVoltageSource):
    """A staircase buffered voltage source driver."""

    _num_sim_waveforms: int
    _set_staircase: "SetIndexedCommand[Staircase]"
    _set_leader: "SetIndexedCommand[bool]"
    _staircase_bounds: tuple["Staircase", "Staircase"]

    def __init__(self, *args, **kwargs) -> None:
        """Initialize the staircase buffered voltage source daemon."""
        super().__init__(*args, **kwargs)
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
            property_name=SUPPORTED_PROPERTIES.LEADER,
            index=self._global_index,
            get_cmd=self._make_get_leader(),
            set_cmd=self._make_set_leader(),
        )
        self.program_property(
            property_name=SUPPORTED_PROPERTIES.NUMBER_SIMULTANEOUS_WAVEFORMS,
            index=self._global_index,
            get_cmd=lambda: self._num_sim_waveforms,
        )

        [
            self.program_property(
                property_name=SUPPORTED_PROPERTIES.STAIRCASE,
                index=index,
                bounds=self._staircase_bounds,
                get_cmd=self._make_get_staircase(idx=index),
                set_cmd=self._make_set_staircase(idx=index),
            )
            for index in self._indexes
        ]

    def _make_get_leader(self) -> "GetCommand[bool]":
        """Wraps the cache system since this is a derived quantity.

        Returns:
            A lambda function that returns the leader of the source.
        """
        return lambda: self._property_cache[SUPPORTED_PROPERTIES.LEADER][
            self._global_index
        ]  # type: ignore[no-untyped-call]

    def _make_set_leader(self) -> "SetCommand[bool]":
        """Sets the leadership.

        Returns:
            A lambda function that sets the leader of the source.
        """
        return lambda leader: self._set_leader(self._global_index, leader)

    def _make_get_staircase(self, idx: "Index") -> "GetCommand[Staircase]":
        """Wraps the cache system since this is a derived quantity.

        Args:
            idx: The index of the staircase.

        Returns:
            A lambda function that returns the staircase of the source.
        """
        return lambda: self._property_cache[SUPPORTED_PROPERTIES.STAIRCASE][idx]  # type: ignore[no-untyped-call]

    def _make_set_staircase(self, idx: "Index") -> "SetCommand[Staircase]":
        """Makes a wrapper for the set staircase command.

        Args:
            idx: The index of the staircase.

        Returns:
            A lambda function that sets the staircase of the source.
        """
        return lambda staircase: self._set_staircase(idx, staircase)
