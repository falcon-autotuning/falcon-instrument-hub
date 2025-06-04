#!/usr/bin/env python3
"""A script that launches the measurement interpreter for the instrument server."""

import argparse
import asyncio

from server_daemons.interpreter_daemon import InterpreterDaemon


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
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    interpreter = InterpreterDaemon(url=url, loop=loop)
    try:
        # Run the main function in the loop
        loop.run_until_complete(
            interpreter.start(),
        )
    finally:
        # Ensure the loop is closed when we're done
        loop.close()
