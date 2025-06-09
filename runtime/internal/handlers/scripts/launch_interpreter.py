#!/usr/bin/env python3
"""A script that launches the measurement interpreter for the instrument server."""

import argparse
import asyncio

from server_daemons.interpreter_daemon import InterpreterDaemon


def get_url() -> str:
    """Unpacks arguments from the command line and returns a dictionary of the arguments.

    Returns:
        the url for the NATS server connection.
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
    url = args.url

    assert isinstance(url, str), f"Invalid url {args.url!s}"
    return url


if __name__ == "__main__":
    url = get_url()
    interpreter = InterpreterDaemon(url=url)
    asyncio.run(
        interpreter.start(),
    )
