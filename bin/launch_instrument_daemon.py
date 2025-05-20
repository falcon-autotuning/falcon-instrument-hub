#!/usr/bin/env python3
"""A script that can launch any instrument for the instrument server."""

import argparse
import asyncio
import datetime
import importlib
import importlib.metadata
import json
from typing import TYPE_CHECKING, Any

import nats

from instrument_server.constants import RUNTIME_COMMANDS
from instrument_server.instrument_daemons.sync_sender import SyncSender
from instrument_server.registry_controls import find_daemon

if TYPE_CHECKING:
    from nats.aio.msg import Msg

    from instrument_server.instrument_daemons.base_instrument_daemon import (
        BaseInstrumentDaemon,
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
        help="Type of the control unit",
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


def build_daemon(config: dict[str, Any]) -> type["BaseInstrumentDaemon"]:
    """Builds the daemon class from its name.

    Args:
        daemon_name: The name of the daemon.

    Returns:
        The daemon class.
    """
    daemon_name = config["instrument_driver"]
    assert daemon_name is not None, "instrument cannot be None"
    selected_driver = find_daemon(daemon_name=daemon_name)
    assert selected_driver is not None, f"daemon {daemon_name} not found"
    return selected_driver


def specific_channel(channel: str, daemon_name: str) -> str:
    """Builds a specific channel for the daemon.

    Args:
        channel: The base channel.
        daemon_name: The name of the daemon.

    Returns:
        The specific channel for the daemon.
    """
    return f"{channel}.{daemon_name}"


async def main(
    url: str,
    daemon_class: type["BaseInstrumentDaemon"],
    loop: asyncio.AbstractEventLoop,
) -> None:
    """The main function to process server requests.

    Args:
        running_daemon: The daemon running with the instrument.
        url: The URL of the NATS server.
        daemon_name: The name of the daemon.
        loop: The event loop to use for synchronous operations.
    """
    daemon_name = daemon_class.__name__

    async def send_command(
        channel: str,
        message: str,
    ) -> None:
        """Send a command string to a specific channel.

        Args:
            channel: The channel to send the command to.
            message: The message to send.
        """
        await nc.publish(channel, message.encode())

    async def log(
        message: str,
        daemon_name: str,
    ) -> None:
        """Log a message to the NATS server.

        Args:
            nc: The NATS client.
            message: The message to log.
            daemon_name: The name of the daemon.
        """
        message = json.dumps({RUNTIME_COMMANDS.LOG.MESSAGE: message})
        await send_command(
            channel=specific_channel(
                channel=RUNTIME_COMMANDS.LOG.COMM_CHANNEL,
                daemon_name="instrument_server",
            ),
            message=message,
        )

    async def handle_set(
        msg: "Msg",
    ) -> None:
        """Handle a SET command.

        Args:
            msg: The NATS message.
        """
        try:
            data = json.loads(msg.data.decode())
            property_name = data.get(RUNTIME_COMMANDS.SET.PROPERTY)
            index = data.get(RUNTIME_COMMANDS.SET.INDEX)
            value = data.get(RUNTIME_COMMANDS.SET.VALUE)

            if not all([property_name, index, value is not None]):
                await log(
                    message="Invalid SET command",
                    daemon_name=daemon_name,
                )
                return

            running_daemon.set_property(
                property_name=property_name,
                index=index,
                value=value,
            )
            await log(
                message="SET command executed",
                daemon_name=daemon_name,
            )
        except Exception as e:
            await log(
                message=f"Error in SET command: {e!s}",
                daemon_name=daemon_name,
            )

    async def handle_get(
        msg: "Msg",
    ) -> None:
        """Handle a GET command.

        Args:
            msg: The NATS message.
        """
        try:
            data = json.loads(msg.data.decode())
            property_name = data.get("property_name")
            index = data.get("index")

            if not all([property_name, index]):
                await log(
                    message="Invalid GET command",
                    daemon_name=daemon_name,
                )
                return

            running_daemon.get_property(property_name, index)
        except Exception as e:
            await log(
                message=f"Error in GET command: {e!s}",
                daemon_name=daemon_name,
            )

    nc = await nats.connect(url)

    # Create a SyncSender for the daemon
    sync_sender = SyncSender(send_command, loop=loop)
    running_daemon = daemon_class(
        sync_sender=sync_sender,
    )

    # Send initialization confirmation
    init_config = running_daemon.to_json_config()
    init_message = json.dumps(
        {
            RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.INIT: init_config,
            RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.TIMESTAMP: str(
                datetime.datetime.now()
            ),
            RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.INIT: json.dumps(init_config),
        }
    )
    await send_command(
        channel=specific_channel(
            channel=RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.COMM_CHANNEL,
            daemon_name=daemon_name,
        ),
        message=init_message,
    )

    # Subscribe to command channels
    await nc.subscribe(
        specific_channel(
            channel=RUNTIME_COMMANDS.SET.COMM_CHANNEL, daemon_name=daemon_name
        ),
        cb=handle_set,
    )
    await nc.subscribe(
        specific_channel(
            channel=RUNTIME_COMMANDS.GET.COMM_CHANNEL, daemon_name=daemon_name
        ),
        cb=handle_get,
    )

    # Create a future that will keep the main task running indefinitely
    # This will keep the connection open until an external signal interrupts
    forever = asyncio.Future()

    try:
        # Wait forever or until the program is interrupted
        await forever
    except asyncio.CancelledError:
        # Handle graceful shutdown if the future is cancelled
        print(f"Daemon {daemon_name} shutting down...")
    finally:
        # Properly drain the connection when exiting
        await nc.drain()


if __name__ == "__main__":
    config = get_driver_config_from_args()
    daemon_class = build_daemon(config)

    # Create the event loop that will be shared with SyncSender
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    url = config["url"]

    try:
        # Run the main function in the loop
        loop.run_until_complete(
            main(
                daemon_class=daemon_class,
                url=url,
                loop=loop,  # Pass the loop to main
            )
        )
    finally:
        # Ensure the loop is closed when we're done
        loop.close()
