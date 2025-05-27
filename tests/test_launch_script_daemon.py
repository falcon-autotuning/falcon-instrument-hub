"""Unit tests for the launch_instrument_daemon script."""

import asyncio
import fcntl
import json
import os
import subprocess
import time
from pathlib import Path
from typing import TYPE_CHECKING

import nats
import pytest
import pytest_asyncio
from instrument_templates.constants import DRIVER_RUNTIME_COMMANDS, SUPPORTED_PROPERTIES

from tests.test_launch import TestInstrumentDriver

if TYPE_CHECKING:
    from collections.abc import Callable

    from nats.aio.client import Client as NatsClient
    from nats.aio.msg import Msg


# Configure pytest to always show output
def pytest_configure(config):
    config.option.capture = "no"


@pytest_asyncio.fixture
async def nats_client():
    """Fixture that provides a connected NATS client."""
    print("Connecting to NATS...", flush=True)
    try:
        nc = await nats.connect("nats://localhost:4222")
        print("Successfully connected to NATS", flush=True)
    except Exception as e:
        print(f"NATS connection error: {e}", flush=True)
        pytest.fail(f"Failed to connect to NATS: {e}")
        return

    yield nc

    # Cleanup
    print("Closing NATS connection", flush=True)
    await nc.close()


@pytest.fixture
def daemon_process():
    """Fixture that manages a daemon process."""
    env = os.environ.copy()
    env["PYTHONPATH"] = f"{Path.cwd()}:{env.get('PYTHONPATH', '')}"

    process = None

    def start_process():
        nonlocal process
        print("Starting daemon subprocess...", flush=True)
        process = subprocess.Popen(
            [
                "python3",
                "tests/test_launch.py",
                TestInstrumentDriver.__name__,
                "nats://localhost:4222",
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            env=env,
        )
        print(f"Subprocess started with PID: {process.pid}", flush=True)
        return process

    yield start_process

    # Cleanup
    if process:
        try:
            # Print output
            print_process_output(process)

            # Terminate the process
            print("Terminating process...", flush=True)
            process.terminate()
            try:
                process.wait(timeout=2)
            except subprocess.TimeoutExpired:
                print("Process didn't terminate gracefully, forcing kill", flush=True)
                process.kill()
        except Exception as e:
            print(f"Error during process cleanup: {e}", flush=True)


def print_process_output(process):
    """Helper function to print process output."""
    print("\n=== PROCESS OUTPUT ===", flush=True)

    try:
        # Make stdout non-blocking
        if process.stdout:
            fd = process.stdout.fileno()
            fl = fcntl.fcntl(fd, fcntl.F_GETFL)
            fcntl.fcntl(fd, fcntl.F_SETFL, fl | os.O_NONBLOCK)
            stdout_data = process.stdout.read() or ""
            print(f"STDOUT: {stdout_data}", flush=True)

        # Make stderr non-blocking
        if process.stderr:
            fd = process.stderr.fileno()
            fl = fcntl.fcntl(fd, fcntl.F_GETFL)
            fcntl.fcntl(fd, fcntl.F_SETFL, fl | os.O_NONBLOCK)
            stderr_data = process.stderr.read() or ""
            print(f"STDERR: {stderr_data}", flush=True)
    except Exception as e:
        print(f"Error reading process output: {e}", flush=True)


async def subscribe_and_collect(
    nc: "NatsClient",
    channel: str,
    collector: list["Msg"],
    callback: "Callable | None" = None,
):
    """Helper function to subscribe to a channel and collect messages."""

    async def message_handler(msg: "Msg"):
        if callback:
            await callback(msg)
        collector.append(msg)

    print(f"Subscribing to channel: {channel}", flush=True)
    await nc.subscribe(channel, cb=message_handler)
    print(f"Subscription created for {channel}", flush=True)
    await asyncio.sleep(0.1)  # Give time for subscription to settle


async def wait_for_messages(
    msgs: list["Msg"],
    condition: "Callable[[list[Msg]], bool]| None" = None,
    timeout: float = 5.0,
) -> bool:
    """Wait for messages to be received with timeout and optional condition."""
    wait_time = 0
    while wait_time < timeout:
        if msgs and (condition is None or condition(msgs)):
            return True
        await asyncio.sleep(0.5)
        wait_time += 0.5
        print(f"Waiting for messages... ({wait_time}s elapsed)", flush=True)
    return False


@pytest.mark.asyncio
async def test_initialization(nats_client, daemon_process, capfd):
    """Test that the daemon sends an initialization message at startup."""
    print("\n=== STARTING DAEMON INITIALIZATION TEST ===", flush=True)

    # Set up message collection
    init_msgs = []

    # Subscribe to initialization channel
    init_channel = (
        DRIVER_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.COMM_CHANNEL
        + f".{TestInstrumentDriver.__name__}"
    )
    await subscribe_and_collect(nats_client, init_channel, init_msgs)

    # Start the daemon process
    daemon_process()

    # Wait for initialization message
    received = await wait_for_messages(init_msgs)

    # Assert we got a message
    if received:
        print(
            f"✅ Success! Received {len(init_msgs)} initialization message(s)",
            flush=True,
        )
        assert len(init_msgs) > 0, "No initialization messages received"
    else:
        print(
            "❌ No initialization messages received within timeout period", flush=True
        )
        pytest.fail("Did not receive initialization message from daemon")

    # Capture any additional output
    captured = capfd.readouterr()
    print(f"Additional captured stdout: {captured.out}", flush=True)
    print(f"Additional captured stderr: {captured.err}", flush=True)


@pytest.mark.asyncio
async def test_set_and_get_properties(nats_client, daemon_process, capfd):
    """Test that the daemon correctly handles SET and GET commands."""
    print("\n=== STARTING SET/GET TEST ===", flush=True)

    # Set up message collections
    log_msgs = []
    get_response_msgs = []

    # Subscribe to log messages
    log_channel = (
        DRIVER_RUNTIME_COMMANDS.LOG.COMM_CHANNEL + f".{TestInstrumentDriver.__name__}"
    )

    async def log_handler(msg):
        print(f"Received log message: {msg.data.decode()}", flush=True)

    await subscribe_and_collect(nats_client, log_channel, log_msgs, log_handler)

    # Subscribe to get response
    get_response_channel = (
        DRIVER_RUNTIME_COMMANDS.RETURN_GET.COMM_CHANNEL
        + f".{TestInstrumentDriver.__name__}"
    )

    async def get_response_handler(msg):
        print(f"Received GET response: {msg.data.decode()}", flush=True)

    await subscribe_and_collect(
        nats_client, get_response_channel, get_response_msgs, get_response_handler
    )

    # Start the daemon process
    daemon_process()

    # Wait a moment for the daemon to initialize
    await asyncio.sleep(1)

    # Send SET command to set VOLTAGE_STATE index 1 to 3
    set_channel = (
        DRIVER_RUNTIME_COMMANDS.SET.COMM_CHANNEL + f".{TestInstrumentDriver.__name__}"
    )
    set_data = {
        DRIVER_RUNTIME_COMMANDS.SET.PROPERTY: SUPPORTED_PROPERTIES.VOLTAGE_STATE,
        DRIVER_RUNTIME_COMMANDS.SET.INDEX: 1,
        DRIVER_RUNTIME_COMMANDS.SET.VALUE: 3,
    }

    print(f"Sending SET command to channel {set_channel}...", flush=True)
    await nats_client.publish(set_channel, json.dumps(set_data).encode())

    # Wait for SET confirmation
    def set_confirmed(msgs):
        return any("SET command executed" in msg.data.decode() for msg in msgs)

    set_received = await wait_for_messages(log_msgs, condition=set_confirmed)
    assert set_received, "SET command not confirmed"
    print("SET command confirmed", flush=True)

    # Send GET command to retrieve VOLTAGE_STATE index 1
    get_channel = (
        DRIVER_RUNTIME_COMMANDS.GET.COMM_CHANNEL + f".{TestInstrumentDriver.__name__}"
    )
    get_data = {
        DRIVER_RUNTIME_COMMANDS.GET.PROPERTY: SUPPORTED_PROPERTIES.VOLTAGE_STATE,
        DRIVER_RUNTIME_COMMANDS.GET.INDEX: 1,
    }

    print(f"Sending GET command to channel {get_channel}...", flush=True)
    await nats_client.publish(get_channel, json.dumps(get_data).encode())

    # Wait for GET response
    get_received = await wait_for_messages(get_response_msgs)
    assert get_received, "No GET response received"

    # Verify the value is 3
    response_data = json.loads(get_response_msgs[0].data.decode())
    assert response_data.get(DRIVER_RUNTIME_COMMANDS.RETURN_GET.VALUE) == 3, (
        f"Expected value 3, got {response_data.get(DRIVER_RUNTIME_COMMANDS.RETURN_GET.VALUE)}"
    )
    print(f"✅ GET response verified: {response_data}", flush=True)

    # Capture any additional output
    captured = capfd.readouterr()
    print(f"Additional captured stdout: {captured.out}", flush=True)
    print(f"Additional captured stderr: {captured.err}", flush=True)


@pytest.mark.asyncio
async def test_perform_arbitrary_method(nats_client, daemon_process, capfd):
    """Test that the daemon correctly handles PERFORM_ARBITRARY_METHOD commands."""
    print("\n=== STARTING PERFORM_ARBITRARY_METHOD TEST ===", flush=True)

    # Set up message collection
    log_msgs = []

    # Subscribe to log messages
    log_channel = (
        DRIVER_RUNTIME_COMMANDS.LOG.COMM_CHANNEL + f".{TestInstrumentDriver.__name__}"
    )

    async def log_handler(msg):
        print(f"Received log message: {msg.data.decode()}", flush=True)

    await subscribe_and_collect(nats_client, log_channel, log_msgs, log_handler)

    # Start the daemon process
    daemon_process()

    # Wait a moment for the daemon to initialize
    await asyncio.sleep(1)

    # Send PERFORM_ARBITRARY_METHOD command
    arbitration_channel = (
        DRIVER_RUNTIME_COMMANDS.PERFORM_ARBITRARY_METHOD.COMM_CHANNEL
        + f".{TestInstrumentDriver.__name__}"
    )

    # Create a method that sets a property value
    # We'll use set_property method with special parameters
    arbitrary_data = {
        DRIVER_RUNTIME_COMMANDS.PERFORM_ARBITRARY_METHOD.METHOD: "set_property",
        DRIVER_RUNTIME_COMMANDS.PERFORM_ARBITRARY_METHOD.TIMESTAMP: str(time.time()),
        DRIVER_RUNTIME_COMMANDS.PERFORM_ARBITRARY_METHOD.KEYWORD_ARGS: json.dumps(
            {
                "property_name": SUPPORTED_PROPERTIES.VOLTAGE_STATE,
                "index": 1,
                "value": 5,
            }
        ),
    }

    print(
        f"Sending PERFORM_ARBITRARY_METHOD command to channel {arbitration_channel}...",
        flush=True,
    )
    await nats_client.publish(arbitration_channel, json.dumps(arbitrary_data).encode())

    # Wait for confirmation that the arbitrary method was executed
    def arbitrary_confirmed(msgs):
        return any(
            "PERFORM_ARBITRARY_METHOD command executed" in msg.data.decode()
            for msg in msgs
        )

    arbitrary_received = await wait_for_messages(
        log_msgs, condition=arbitrary_confirmed
    )
    assert arbitrary_received, "PERFORM_ARBITRARY_METHOD command not confirmed"
    print("PERFORM_ARBITRARY_METHOD command confirmed", flush=True)

    # Capture any additional output
    captured = capfd.readouterr()
    print(f"Additional captured stdout: {captured.out}", flush=True)
    print(f"Additional captured stderr: {captured.err}", flush=True)
