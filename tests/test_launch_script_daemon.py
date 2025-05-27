"""Unit tests for the launch_instrument_daemon script."""

import asyncio
import fcntl
import os
import subprocess
from pathlib import Path

import nats
import pytest
from instrument_templates.constants import DRIVER_RUNTIME_COMMANDS

from tests.test_launch import TestInstrumentDriver


@pytest.mark.asyncio
async def test_initialization(capfd):
    """Test that the daemon sends an initialization message at startup."""
    print("\n=== STARTING DAEMON TEST ===", flush=True)

    env = os.environ.copy()
    env["PYTHONPATH"] = f"{Path.cwd()}:{env.get('PYTHONPATH', '')}"

    # Connect to NATS and subscribe before launching the subprocess
    print("Connecting to NATS...", flush=True)
    try:
        nc = await nats.connect("nats://localhost:4222")
        print("Successfully connected to NATS", flush=True)
    except Exception as e:
        print(f"NATS connection error: {e}", flush=True)
        pytest.fail(f"Failed to connect to NATS: {e}")
        return

    msgs = []

    async def message_handler(msg):
        print(f"Received NATS message: {msg.subject}", flush=True)
        msgs.append(msg)

    subscription_channel = (
        DRIVER_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.COMM_CHANNEL
        + f".{TestInstrumentDriver.__name__}"
    )
    print(f"Subscribing to channel: {subscription_channel}", flush=True)

    await nc.subscribe(subscription_channel, cb=message_handler)
    print("Subscription created", flush=True)
    await asyncio.sleep(0.5)  # Give more time for subscription to settle

    # Start the daemon process without waiting for it to complete
    process = None
    try:
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

        # Wait for the initialization message or timeout
        print("Waiting for messages...", flush=True)
        try:
            # Wait for messages with explicit timeout and status updates
            wait_time = 0
            max_wait = 5  # seconds
            while not msgs and wait_time < max_wait:
                await asyncio.sleep(0.5)
                wait_time += 0.5
                print(f"Still waiting... ({wait_time}s elapsed)", flush=True)

            if msgs:
                print(f"✅ Success! Received {len(msgs)} message(s)", flush=True)
                assert len(msgs) > 0, "No initialization messages received"
            else:
                print("❌ No messages received within timeout period", flush=True)
                pytest.fail("Did not receive initialization message from daemon")

        except Exception as e:
            print(f"Error while waiting for messages: {e}", flush=True)
            pytest.fail(f"Exception during message wait: {e}")

    finally:
        # Capture output and display it
        print("\n=== PROCESS OUTPUT ===", flush=True)
        if process:
            # Don't wait for the process to complete since it's designed to run indefinitely
            # Instead, read any available output from the pipes without blocking
            if process.stdout:
                try:
                    # Make stdout non-blocking

                    fd = process.stdout.fileno()
                    fl = fcntl.fcntl(fd, fcntl.F_GETFL)
                    fcntl.fcntl(fd, fcntl.F_SETFL, fl | os.O_NONBLOCK)

                    # Try to read available data
                    stdout_data = process.stdout.read() or ""
                    print(f"STDOUT: {stdout_data}", flush=True)
                except Exception as e:
                    print(f"Error reading stdout: {e}", flush=True)

            if process.stderr:
                try:
                    # Make stderr non-blocking
                    fd = process.stderr.fileno()
                    fl = fcntl.fcntl(fd, fcntl.F_GETFL)
                    fcntl.fcntl(fd, fcntl.F_SETFL, fl | os.O_NONBLOCK)

                    # Try to read available data
                    stderr_data = process.stderr.read() or ""
                    print(f"STDERR: {stderr_data}", flush=True)
                except Exception as e:
                    print(f"Error reading stderr: {e}", flush=True)

            # Terminate the process
            print("Terminating process...", flush=True)
            process.terminate()

        # Close NATS connection
        print("Closing NATS connection", flush=True)
        await nc.close()

        # Capture and print any output not already shown
        captured = capfd.readouterr()
        print(f"Additional captured stdout: {captured.out}", flush=True)
        print(f"Additional captured stderr: {captured.err}", flush=True)
