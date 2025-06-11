"""Integration tests for the interpreter daemon."""

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
from falcon_core.communications.messages import MeasurementRequest
from falcon_core.constants import INSTRUMENT_TYPES
from falcon_core.instrument_interfaces.names import Knob, Meter, Meters
from falcon_core.instrument_interfaces.port_transforms.identity_transform import (
    IdentityTransform,
)
from falcon_core.instrument_interfaces.waveforms.cartesian_waveform import (
    CartesianWaveform,
)
from falcon_core.math.axes import Axes
from falcon_core.math.discrete_spaces import CartesianDiscreteSpace
from falcon_core.math.domains import CoupledKnobDomain, KnobDomain
from falcon_core.math.spaces import CartesianSpace
from falcon_core.physics.device_structures import BarrierGate, Ohmic
from falcon_core.physics.units import Units

from server_daemons.api.interpreter import (
    RUNTIME_COMMANDS as INTERPRETER_RUNTIME_COMMANDS,
)

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

    yield nc

    # Cleanup
    print("Closing NATS connection", flush=True)
    await nc.close()


@pytest.fixture
def daemon_process():
    """Fixture that manages the interpreter daemon process."""
    env = os.environ.copy()
    env["PYTHONPATH"] = f"{Path.cwd()}:{env.get('PYTHONPATH', '')}"

    process = None

    def start_process():
        nonlocal process
        print("Starting interpreter daemon subprocess...", flush=True)
        process = subprocess.Popen(
            [
                "python3",
                "./scripts/launch_interpreter.py",
                "nats://localhost:4222",
            ],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            env=env,
        )
        print(f"Interpreter subprocess started with PID: {process.pid}", flush=True)
        return process

    yield start_process

    # Cleanup
    if process:
        try:
            # Print output
            print_process_output(process)

            # Terminate the process
            print("Terminating interpreter process...", flush=True)
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
async def test_interpreter_status(nats_client, daemon_process, capfd):
    """Test that the interpreter daemon sends status messages."""
    print("\n=== STARTING INTERPRETER STATUS TEST ===", flush=True)

    # Set up message collection
    status_msgs = []

    # Subscribe to status channel
    status_channel = INTERPRETER_RUNTIME_COMMANDS.STATUS.COMM_CHANNEL + ".interpreter"

    async def status_handler(msg):
        print(f"Received status message: {msg.data.decode()}", flush=True)

    await subscribe_and_collect(
        nats_client,
        status_channel,
        status_msgs,
        status_handler,
    )

    # Start the daemon process
    daemon_process()

    # Wait for status messages
    received = await wait_for_messages(status_msgs)

    # Assert we got a message
    if received:
        print(f"✅ Success! Received {len(status_msgs)} status message(s)", flush=True)
        assert len(status_msgs) > 0, "No status messages received"
    else:
        print("❌ No status messages received within timeout period", flush=True)
        pytest.fail("Did not receive status message from daemon")

    # Capture any additional output
    captured = capfd.readouterr()
    print(f"Additional captured stdout: {captured.out}", flush=True)
    print(f"Additional captured stderr: {captured.err}", flush=True)


@pytest.mark.asyncio
async def test_interpreter_log(nats_client, daemon_process, capfd):
    """Test that the interpreter daemon can log messages."""
    print("\n=== STARTING INTERPRETER LOG TEST ===", flush=True)

    # Set up message collection
    log_msgs = []

    # Subscribe to log channel
    log_channel = INTERPRETER_RUNTIME_COMMANDS.LOG.COMM_CHANNEL + ".interpreter"

    async def log_handler(msg):
        print(f"Received log message: {msg.data.decode()}", flush=True)

    await subscribe_and_collect(nats_client, log_channel, log_msgs, log_handler)

    # Start the daemon process
    daemon_process()

    # Wait for log messages (might not get any immediately, but we're just testing subscription)
    await asyncio.sleep(1)

    # Send a process request that should trigger logging
    process_channel = INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.COMM_CHANNEL
    simple_request = {
        INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.REQUEST: json.dumps(
            {"type": "simple_test"}
        ),
        INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.PROCESS_ID: "test_id",
        INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.CONFIGURATIONS: json.dumps({}),
        INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.DATA_PATH: "/tmp/test_data",
    }

    print(f"Sending process request to channel {process_channel}...", flush=True)
    await nats_client.publish(process_channel, json.dumps(simple_request).encode())

    # Wait for log messages
    def has_log_message(msgs):
        return any(msg.data.decode() for msg in msgs)

    received = await wait_for_messages(log_msgs, condition=has_log_message)

    # Even if there's an error in processing, we should still get some log message
    if received:
        print(f"✅ Success! Received {len(log_msgs)} log message(s)", flush=True)
        assert len(log_msgs) > 0, "No log messages received"
    else:
        print("❌ No log messages received within timeout period", flush=True)
        pytest.fail("Did not receive log message from daemon")

    # Capture any additional output
    captured = capfd.readouterr()
    print(f"Additional captured stdout: {captured.out}", flush=True)
    print(f"Additional captured stderr: {captured.err}", flush=True)


@pytest.mark.asyncio
async def test_process_request_and_data(nats_client, daemon_process, capfd):
    """Test that the interpreter daemon can process requests and data."""
    print("\n=== STARTING PROCESS REQUEST AND DATA TEST ===", flush=True)

    # Set up message collection
    log_msgs = []
    measurement_ready_msgs = []
    upload_data_msgs = []

    # Subscribe to channels
    log_channel = INTERPRETER_RUNTIME_COMMANDS.LOG.COMM_CHANNEL
    measurement_ready_channel = (
        INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.COMM_CHANNEL
    )
    upload_data_channel = INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.COMM_CHANNEL

    await subscribe_and_collect(nats_client, log_channel, log_msgs)
    await subscribe_and_collect(
        nats_client, measurement_ready_channel, measurement_ready_msgs
    )
    await subscribe_and_collect(nats_client, upload_data_channel, upload_data_msgs)

    # Start the daemon process
    daemon_process()
    # Give the daemon time to start up and connect
    await asyncio.sleep(2.0)  # Increased wait time

    # Create a simplified MeasurementRequest
    knobs = [
        Knob(
            default_name="test_knob",
            pseudo_name=BarrierGate("B3"),
        )
    ]
    meters = [
        Meter(
            default_name="test_meter",
            pseudo_name=Ohmic("O2"),
        )
    ]
    space = CartesianSpace(deltas=[0.1])
    ckd = CoupledKnobDomain(
        [
            KnobDomain.from_knob(
                bounds=(0, 0.5),
                knob=knobs[0],
            )
        ]
    )
    sweep_axes = Axes([ckd])
    space = CartesianDiscreteSpace(space=space, axes=sweep_axes)
    waveform = CartesianWaveform(space=space, transforms=[])
    ports: list[Meter] = []
    ports.extend(meters)
    ports.append(
        Meter(
            default_name="timer",
            instrument_type=INSTRUMENT_TYPES.CLOCK,
            units=Units.SECOND,
        )
    )
    transform = IdentityTransform(port=meters[0], ports=Meters(ports))
    request = MeasurementRequest(
        message="test measurement",
        measurement_name="integration_test",
        waveforms=[waveform],
        meter_transforms=[transform],
    )

    # Send process request
    process_request_channel = INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.COMM_CHANNEL
    process_request = {
        INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.REQUEST: request.to_json(),
        INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.PROCESS_ID: 42,
        INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.CONFIGURATIONS: json.dumps({}),
        INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.DATA_PATH: "/tmp/test_data",
    }

    print(
        f"Sending process request to channel {process_request_channel}...", flush=True
    )
    await nats_client.publish(
        process_request_channel, json.dumps(process_request).encode()
    )

    # Wait for log message indicating request was received

    def request_received(msgs):
        if not msgs:
            return False
        for msg in msgs:
            msg_content = msg.data.decode()
            print(f"Checking log message: {msg_content}", flush=True)
            if any(
                keyword in msg_content
                for keyword in [
                    "Error processing request",
                    "Processing request",
                    "successfully",
                ]
            ):
                return True
        return False

    received = await wait_for_messages(log_msgs, condition=request_received)
    assert received, "No confirmation of process request received"

    # Now send process data
    process_data_channel = INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.COMM_CHANNEL
    process_data = {
        INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.PROCESS_ID: 42,
        INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.TIMESTAMP: str(time.time()),
        INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.DATA: {"device1": [1.0, 2.0, 3.0]},
    }

    print(f"Sending process data to channel {process_data_channel}...", flush=True)
    await nats_client.publish(process_data_channel, json.dumps(process_data).encode())

    # Wait for log message indicating data was added to queue
    def data_received(msgs):
        return any("Data added to queue" in msg.data.decode() for msg in msgs)

    received = await wait_for_messages(log_msgs, condition=data_received)

    if received:
        print("✅ Successfully added data to queue", flush=True)
    else:
        print("❌ Failed to add data to queue", flush=True)
        pytest.fail("Did not receive confirmation that data was added to queue")

    # Capture any additional output
    captured = capfd.readouterr()
    print(f"Additional captured stdout: {captured.out}", flush=True)
    print(f"Additional captured stderr: {captured.err}", flush=True)
