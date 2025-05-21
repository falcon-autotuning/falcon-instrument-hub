"""Unit tests for the launch_instrument_daemon script."""

import asyncio
import json
import os
import sys
from importlib import util
from unittest.mock import AsyncMock, MagicMock

import pytest

from instrument_server.constants import DAEMON_RUNTIME_COMMANDS, SUPPORTED_PROPERTIES
from instrument_server.instrument_daemons.base_instrument_daemon import (
    BaseInstrumentDaemon,
)
from instrument_server.instrument_daemons.sync_sender import SyncSender

bin_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "bin"))
sys.path.insert(0, bin_dir)
spec = util.spec_from_file_location(
    "launch_instrument_daemon", os.path.join(bin_dir, "launch_instrument_daemon.py")
)
launch_module = util.module_from_spec(spec)
spec.loader.exec_module(launch_module)
main = launch_module.main


class TestInstrumentDaemon(BaseInstrumentDaemon):
    """A simple test instrument daemon for testing."""

    def __init__(self, sync_sender):
        """Initialize the test instrument daemon."""
        super().__init__(sync_sender)

        # Define some test voltage states
        self.program_property(
            property_name=SUPPORTED_PROPERTIES.VOLTAGE_STATE,
            index="0",
            get_cmd=lambda: self._voltage_state_0,
            set_cmd=lambda value: setattr(self, "_voltage_state_0", value),
            bounds=(0, 10),
        )
        self.program_property(
            property_name=SUPPORTED_PROPERTIES.VOLTAGE_STATE,
            index="1",
            get_cmd=lambda: self._voltage_state_1,
            set_cmd=lambda value: setattr(self, "_voltage_state_1", value),
            bounds=(0, 10),
        )

        # Define a sample rate property
        self.program_property(
            property_name=SUPPORTED_PROPERTIES.SAMPLE_RATE,
            index="0",
            get_cmd=lambda: self._sample_rate,
            set_cmd=lambda value: setattr(self, "_sample_rate", value),
            bounds=(1, 1000),
        )

        # Initialize values
        self._voltage_state_0 = 0
        self._voltage_state_1 = 0
        self._sample_rate = 100


class MockMsg:
    """A mock NATS message for testing."""

    def __init__(self, subject, data):
        """Initialize with subject and data."""
        self.subject = subject
        self._data = data.encode() if isinstance(data, str) else data

    @property
    def data(self):
        """Get message data."""
        return self._data


@pytest.fixture
def mock_nc(monkeypatch):
    """Mock nats.connect, nc.publish, and nc.subscribe only."""
    mock_nc = MagicMock()
    mock_nc.publish = AsyncMock()
    mock_nc.subscribe = AsyncMock()
    monkeypatch.setattr("nats.connect", AsyncMock(return_value=mock_nc))
    return mock_nc


@pytest.fixture
def sync_sender(event_loop, mock_nc, sent_messages):
    """Create a SyncSender with mocked send function."""

    async def real_send(channel, message):
        await mock_nc.publish(channel, message.encode())
        sent_messages.append((channel, message))

    return SyncSender(real_send, event_loop)


@pytest.fixture
def sent_messages():
    """Create a list to track sent messages."""
    return []


@pytest.fixture
def daemon(sync_sender):
    """Create a test instrument daemon."""
    return TestInstrumentDaemon(sync_sender)


@pytest.fixture
def main_func():
    """Import the main function from launch_instrument_daemon."""
    return launch_module.main


@pytest.fixture
def event_loop():
    """Create an instance of the default event loop for each test case."""
    import asyncio

    loop = asyncio.new_event_loop()
    yield loop
    loop.close()


@pytest.mark.asyncio
async def test_initialization(event_loop, main_func, daemon, sent_messages):
    """Test that the daemon sends an initialization message at startup."""
    # Start main with appropriate parameters
    task = asyncio.create_task(
        main_func(
            running_demon=daemon,
            url="nats://localhost:4222",
            demon_name="TestInstrumentDaemon",
            loop=event_loop,
        )
    )

    # Give it time to initialize
    await asyncio.sleep(0.1)

    # Cancel the task to simulate shutdown
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task

    # Verify initialization message was sent
    init_messages = [
        msg
        for channel, msg in sent_messages
        if DAEMON_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.COMM_CHANNEL in channel
    ]

    assert len(init_messages) > 0, "Initialization message was not sent"

    # Parse the initialization message to ensure it has the right format
    init_data = json.loads(init_messages[0])
    assert DAEMON_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.INIT in init_data
    assert DAEMON_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.TIMESTAMP in init_data

    # Verify the configuration contains our properties
    config = init_data[DAEMON_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.INIT]
    assert "properties" in config
    assert SUPPORTED_PROPERTIES.VOLTAGE_STATE in config["properties"]
    assert SUPPORTED_PROPERTIES.SAMPLE_RATE in config["properties"]


@pytest.mark.asyncio
async def test_set_command(daemon, sent_messages):
    """Test that SET commands are properly handled."""
    # Create SET message
    set_data = {
        DAEMON_RUNTIME_COMMANDS.SET.PROPERTY: SUPPORTED_PROPERTIES.VOLTAGE_STATE,
        DAEMON_RUNTIME_COMMANDS.SET.INDEX: "0",
        DAEMON_RUNTIME_COMMANDS.SET.VALUE: 5.0,
    }
    MockMsg(
        f"{DAEMON_RUNTIME_COMMANDS.SET.COMM_CHANNEL}.TestInstrumentDaemon",
        json.dumps(set_data),
    )

    # Verify property was set
    assert daemon._voltage_state_0 == 5.0

    # Verify log message was sent
    log_messages = [
        msg
        for channel, msg in sent_messages
        if DAEMON_RUNTIME_COMMANDS.LOG.COMM_CHANNEL in channel
    ]
    assert len(log_messages) > 0, "Log message was not sent after SET"


@pytest.mark.asyncio
async def test_get_command(daemon, sent_messages):
    """Test that GET commands are properly handled."""
    # Set a value first
    daemon._voltage_state_1 = 7.5

    # Create GET message
    get_data = {
        DAEMON_RUNTIME_COMMANDS.GET.PROPERTY: SUPPORTED_PROPERTIES.VOLTAGE_STATE,
        DAEMON_RUNTIME_COMMANDS.GET.INDEX: "1",
    }
    MockMsg(
        f"{DAEMON_RUNTIME_COMMANDS.GET.COMM_CHANNEL}.TestInstrumentDaemon",
        json.dumps(get_data),
    )

    # Verify return message was sent
    return_messages = [
        msg
        for channel, msg in sent_messages
        if DAEMON_RUNTIME_COMMANDS.RETURN_GET.COMM_CHANNEL in channel
    ]
    assert len(return_messages) > 0, "Return message was not sent after GET"

    # Parse the return message
    return_data = json.loads(return_messages[0])
    assert "value" in return_data
    assert return_data["value"] == 7.5
    assert DAEMON_RUNTIME_COMMANDS.RETURN_GET.TIMESTAMP in return_data


@pytest.mark.asyncio
async def test_error_handling(sent_messages):
    """Test that errors are properly handled."""
    # Create invalid SET message (missing value)
    invalid_set_data = {
        DAEMON_RUNTIME_COMMANDS.SET.PROPERTY: SUPPORTED_PROPERTIES.VOLTAGE_STATE,
        DAEMON_RUNTIME_COMMANDS.SET.INDEX: "0",
        # Value is missing
    }
    MockMsg(
        f"{DAEMON_RUNTIME_COMMANDS.SET.COMM_CHANNEL}.TestInstrumentDaemon",
        json.dumps(invalid_set_data),
    )

    # Verify error message was sent
    error_messages = [
        msg
        for channel, msg in sent_messages
        if DAEMON_RUNTIME_COMMANDS.LOG.COMM_CHANNEL in channel and "Invalid" in msg
    ]
    assert len(error_messages) > 0, "Error message was not sent for invalid command"


@pytest.mark.asyncio
async def test_property_bounds_config(daemon):
    """Test that property bounds are correctly configured."""
    # Verify the property boundaries are correctly configured
    daemon.properties[SUPPORTED_PROPERTIES.VOLTAGE_STATE]

    # Check if the bounds are correctly set in the configuration
    config = daemon.to_json_config()
    voltage_config = config["properties"][SUPPORTED_PROPERTIES.VOLTAGE_STATE]

    assert "0" in voltage_config
    assert "bounds" in voltage_config["0"]
    assert voltage_config["0"]["bounds"] == (0, 10)

    assert "1" in voltage_config
    assert "bounds" in voltage_config["1"]
    assert voltage_config["1"]["bounds"] == (0, 10)
