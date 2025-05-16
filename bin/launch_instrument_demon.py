#!/usr/bin/env python3
"""A script that can launch any instrument for the instrument server."""

import argparse  # noqa: I001
import asyncio
import json
import nats
from typing import TYPE_CHECKING
from instrument_server.constants import RUNTIME_COMMANDS
from instrument_server.instrument_demons.base_instrument_demon import (
    BaseInstrumentDemon,
)
from instrument_server import get_driver

import importlib
import importlib.metadata

from instrument_server.instrument_demons.message_config import MessageConfig

if TYPE_CHECKING:
    from falcon.typing import Any


for entry_point in importlib.metadata.entry_points(group="driver.plugins"):
    importlib.import_module(entry_point.value)


def get_driver_config_from_args():
    """Unpacks arguments from the command line and returns a dictionary of the arguments."""
    parser = argparse.ArgumentParser(
        description="Launch a BaseInstrument.",
    )
    parser.add_argument(
        "instrument_driver",
        type=str,
        help="Type of the control unit",
    )
    parser.add_argument(
        "message_config",
        type=str,
        help="Path to JSON file for message configuration",
    )

    args = parser.parse_args()

    # Load JSON files for globals, instance variables, and message config
    try:
        message_config_data = json.loads(args.message_config)
    except (FileNotFoundError, json.JSONDecodeError) as e:
        msg = f"Error loading JSON file: {e}"
        raise RuntimeError(msg)

    return {
        "instrument_driver": args.instrument_driver,
        "message_config": message_config_data,
    }


def build_message_config(config: dict[str, "Any"]) -> MessageConfig:
    """Returns the message config from the config."""
    message_config_raw = config["message_config"]
    assert message_config_raw is not None, "message_config cannot be None"
    assert isinstance(message_config_raw, dict), "message_config must be a dict"
    url = message_config_raw["url"]
    assert isinstance(url, str), "url must be a string"
    ping_timeout = message_config_raw.get("ping_timeout", 1.0)
    assert isinstance(ping_timeout, (int, float)), "ping_timeout must be a number"
    return MessageConfig(
        url=url,
        timeout_ping=ping_timeout,
    )


def build_demon_name(
    config: dict[str, "Any"],
) -> str:
    """Returns the demon name from the config."""
    raw_driver = config["instrument_driver"]
    assert raw_driver is not None, "instrument cannot be None"
    return raw_driver


def build_demon(demon_name: str) -> type[BaseInstrumentDemon]:
    selected_driver = get_driver(name=demon_name)
    assert selected_driver is not None, f"Demon {demon_name} not found"
    return selected_driver


async def main(
    running_demon: BaseInstrumentDemon,
    message_config: MessageConfig,
    demon_name: str,
) -> None:
    """The main function to process server requests.

    Args:
        running_demon: The demon running with the instrument.
        message_config: The message configuration for the demon.
        demon_name: The name of the demon.
    """

    async def message_handler(msg: nats.msg):
        subject = msg.subject
        data = msg.data.decode()
        print(f"Received a message on {subject}: {data}")
        # Do something corresponding to the message
        # For example:
        if subject == "channel1":
            print("Handling channel1")
        elif subject == "channel2":
            print("Handling channel2")
        elif subject == "channel3":
            print("Handling channel3")

    async def send_command(
        channel: str,
        command_str: str,
    ):
        """Send a command string to a specific channel."""
        await nc.publish(channel, command_str.encode())

    async def log(message):
        await send_command(
            RUNTIME_COMMANDS.LOG.COMM_CHANNEL + f".{demon_name}",
            message,
        )

    async def return_get(message):
        await send_command(
            RUNTIME_COMMANDS.RETURN_GET.COMM_CHANNEL + f".{demon_name}",
            message,
        )

    async def return_data(message):
        await send_command(
            RUNTIME_COMMANDS.RETURN_DATA.COMM_CHANNEL + f".{demon_name}",
            message,
        )

    nc = await nats.connect(
        message_config.url,
    )

    # Subscribe to all channels
    channels = [
        RUNTIME_COMMANDS.RECEIVE_PING.COMM_CHANNEL,
        RUNTIME_COMMANDS.SET.COMM_CHANNEL,
        RUNTIME_COMMANDS.GET.COMM_CHANNEL,
    ]
    for channel in channels:
        await nc.subscribe(channel, cb=message_handler)

    await log(f"Waiting for messages on channels: {channels}")
    try:
        while True:
            await asyncio.sleep(message_config.timeout_ping)
    finally:
        await nc.drain()


if __name__ == "__main__":
    config = get_driver_config_from_args()
    demon_name = build_demon_name(config)
    demon = build_demon(demon_name)
    running_demon = demon()
    message_config = build_message_config(config)
    asyncio.run(
        main(
            running_demon=running_demon,
            message_config=message_config,
            demon_name=demon_name,
        )
    )
