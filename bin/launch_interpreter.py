#!/usr/bin/env python3
"""A script that launches the measurement interpreter for the instrument server."""

import argparse

from instrument_server.interpreter.measurement_interpreter import MeasurementInterpreter


def get_driver_config_from_args() -> dict[str, str]:
    """Unpacks arguments from the command line and returns a dictionary of the arguments.

    Returns:
        A dictionary containing the configuration parameters.
    """
    parser = argparse.ArgumentParser(
        description="Launch a measurement interpreter.",
    )
    parser.add_argument(
        "url",
        type=str,
        help="URL for NATS server connection",
    )
    args = parser.parse_args()

    return {
        "url": args.url,
    }


if __name__ == "__main__":
    config = get_driver_config_from_args()
    url = config["url"]
    interpreter = MeasurementInterpreter(url=url)
    interpreter.start()
