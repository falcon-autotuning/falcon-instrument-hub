#!/usr/bin/env python3
"""A script that can launch any instrument for the instrument server."""

import argparse
import asyncio
import importlib
import importlib.metadata

from instrument_templates.registry_controls import find_driver

from server_daemons.instrument_daemon import InstrumentDaemon

# Load all driver plugins
for entry_point in importlib.metadata.entry_points(group="driver.plugins"):
    importlib.import_module(entry_point.value)


def unpack_args() -> tuple[type, str]:
    """Unpacks arguments from the command line and returns a dictionary of the arguments.

    Returns:
        A dictionary containing the configuration parameters.
    """
    print("Parsing command line arguments...", flush=True)
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
    print(f"Parsed args: {args}", flush=True)

    instrument_driver = args.instrument_driver
    assert isinstance(instrument_driver, str), (
        f"Invalid driver name {instrument_driver!s}"
    )
    print(f"Looking for driver: {instrument_driver}", flush=True)

    selected_driver = find_driver(instrument_driver)
    print(f"Found driver: {selected_driver}", flush=True)

    url = args.url
    assert isinstance(url, str), f"Invalid url {url!s}"
    print(f"Using URL: {url}", flush=True)

    return selected_driver, url


if __name__ == "__main__":
    print("Starting main execution...", flush=True)
    try:
        driver, url = unpack_args()
        print(f"Found driver class: {driver.__name__}", flush=True)
        print(f"Using NATS URL: {url}", flush=True)

        daemon = InstrumentDaemon(
            url=url,
            instrument_driver=driver,
        )
        print("Created daemon instance", flush=True)

        print("Starting daemon...", flush=True)
        asyncio.run(daemon.start())
        print("Daemon completed", flush=True)
    except Exception as e:
        print(f"Error in main: {e}", flush=True)
        import traceback

        traceback.print_exc()
        raise
