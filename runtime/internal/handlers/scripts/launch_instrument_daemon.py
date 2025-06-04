#!/usr/bin/env python3
"""A script that can launch any instrument for the instrument server."""

import argparse
import asyncio
import importlib
import importlib.metadata
from typing import TYPE_CHECKING, Any

from instrument_templates.registry_controls import find_driver

from server_daemons.instrument_daemon import InstrumentDaemon

if TYPE_CHECKING:
    from instrument_templates.base_instrument_driver import (
        BaseInstrumentDriver,
    )

# Load all driver plugins
for entry_point in importlib.metadata.entry_points(group="driver.plugins"):
    importlib.import_module(entry_point.value)


def get_driver_config_from_args() -> dict[str, Any]:
    """Unpacks arguments from the command line and returns a dictionary of the arguments.

    Returns:
        A dictionary containing the configuration parameters.
    """
    parser = argparse.ArgumentParser(
        description="Launch a BaseInstrument.",
    )
    parser.add_argument(
        "instrument_driver",
        type=str,
        help="Type of instrument driver to launch",
    )
    parser.add_argument(
        "url",
        type=str,
        help="URL for NATS server connection",
    )

    args = parser.parse_args()

    return {
        "instrument_driver": args.instrument_driver,
        "url": args.url,
    }


def build_instrument_driver(config: dict[str, Any]) -> type["BaseInstrumentDriver"]:
    """Builds the instrument driver from its name.

    Args:
        config: The configuration dictionary containing the driver name.

    Returns:
        The driver class.
    """
    instrument_name = config["instrument_driver"]
    assert instrument_name is not None, "instrument cannot be None"
    selected_driver = find_driver(driver_name=instrument_name)
    assert selected_driver is not None, f"daemon {instrument_name} not found"
    return selected_driver


if __name__ == "__main__":
    config = get_driver_config_from_args()
    daemon_class = build_instrument_driver(config)
    print("Found daemon class:", daemon_class)

    # Create the event loop that will be shared with SyncSender
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    url = config["url"]
    daemon = InstrumentDaemon(
        url=url,
        instrument_driver=daemon_class,
        loop=loop,
    )
    try:
        # Run the main function in the loop
        loop.run_until_complete(
            daemon.start(),
        )
    finally:
        # Ensure the loop is closed when we're done
        loop.close()
