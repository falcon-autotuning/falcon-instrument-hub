"""Unit tests for the launch_instrument_daemon script."""

import asyncio
import os
import subprocess
from pathlib import Path

import nats
import pytest
from instrument_templates.constants import DRIVER_RUNTIME_COMMANDS

from tests.test_launch import TestInstrumentDriver


@pytest.mark.asyncio
async def test_initialization():
    """Test that the daemon sends an initialization message at startup."""
    env = os.environ.copy()
    env["PYTHONPATH"] = f"{Path.cwd()}:{env.get('PYTHONPATH', '')}"
    out = subprocess.run(
        [
            "python3",
            "tests/test_launch.py",
            TestInstrumentDriver.__name__,
            "nats://localhost:4222",
        ],
        capture_output=True,
        check=True,
        text=True,
        env=env,
    )
    print(out)
    # Connect to NATS
    nc = await nats.connect("nats://localhost:4222")

    msgs = []

    async def message_handler(msg):
        msgs.append(msg)

    async def _wait_for_message(msgs):
        while not msgs:
            await asyncio.sleep(0.1)

    # Subscribe to the subject you expect the daemon to publish to
    await nc.subscribe(
        DRIVER_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.COMM_CHANNEL
        + f".{TestInstrumentDriver.__name__}",
        cb=message_handler,
    )

    # Wait for a message or timeout
    try:
        await asyncio.wait_for(asyncio.create_task(_wait_for_message(msgs)), timeout=5)
    except TimeoutError:
        pytest.fail("Did not receive initialization message from daemon")

    # Cleanup
    await nc.close()
