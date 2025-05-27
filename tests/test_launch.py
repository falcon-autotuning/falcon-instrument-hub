"""Launcher that bypasses standard plugins instrument loading."""

import runpy

from instrument_templates.base_instrument_driver import (
    BaseInstrumentDriver,
)
from instrument_templates.constants import SUPPORTED_PROPERTIES
from instrument_templates.registry_controls import add_driver


class TestInstrumentDriver(BaseInstrumentDriver):
    """A simple test instrument daemon for testing."""

    def __init__(self, sync_sender):
        """Initialize the test instrument daemon."""
        super().__init__(sync_sender)

        # Define some test voltage states
        self.program_property(
            property_name=SUPPORTED_PROPERTIES.VOLTAGE_STATE,
            index=0,
            get_cmd=lambda: self._voltage_state_0,
            set_cmd=lambda value: setattr(self, "_voltage_state_0", value),
            bounds=(0, 10),
        )
        self.program_property(
            property_name=SUPPORTED_PROPERTIES.VOLTAGE_STATE,
            index=1,
            get_cmd=lambda: self._voltage_state_1,
            set_cmd=lambda value: setattr(self, "_voltage_state_1", value),
            bounds=(0, 10),
        )

        # Define a sample rate property
        self.program_property(
            property_name=SUPPORTED_PROPERTIES.SAMPLE_RATE,
            index=0,
            get_cmd=lambda: self._sample_rate,
            set_cmd=lambda value: setattr(self, "_sample_rate", value),
            bounds=(1, 1000),
        )

        # Initialize values
        self._voltage_state_0 = 0
        self._voltage_state_1 = 0
        self._sample_rate = 100


add_driver(cls=TestInstrumentDriver)

if __name__ == "__main__":
    # This allows the driver to be run directly for testing purposes
    runpy.run_path("scripts/launch_instrument_daemon.py", run_name="__main__")
