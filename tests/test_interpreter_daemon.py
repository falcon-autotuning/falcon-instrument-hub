"""Testing the interpreter daemon."""

import asyncio
import contextlib
import json
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, call, patch

import numpy as np
import pytest
from falcon_core.instrument_interfaces.names import InstrumentPort
from falcon_core.physics.device_structures import Ohmic, PlungerGate
from instrument_templates.constants import SUPPORTED_PROPERTIES

from server_daemons.api.interpreter import (
    RUNTIME_COMMANDS as INTERPRETER_RUNTIME_COMMANDS,
)
from server_daemons.data_queue import DataEntry, DataQueue
from server_daemons.dependancies import (
    MeasurementRequest,
    MeasurementResponse,
)
from server_daemons.instructions import (
    Instruction,
    MeasurementInstructions,
)
from server_daemons.interpreter_daemon import InterpreterDaemon


class MockMsg:
    """Mock implementation of NATS message."""

    def __init__(self, data):
        self.data = data.encode() if isinstance(data, str) else data


class TestInterpreterDaemon:
    @pytest.fixture
    def mock_nats(self):
        """Create a mock for the time module."""
        with patch("server_daemons.interpreter_daemon.nats") as mock_nats:
            # Create a mock client with necessary methods
            mock_client = AsyncMock()
            mock_client.publish = AsyncMock()
            mock_client.subscribe = AsyncMock()
            mock_client.drain = AsyncMock()

            # Create a mock jetstream context
            mock_jetstream = AsyncMock()
            mock_jetstream.add_stream = AsyncMock()
            mock_jetstream.update_stream = AsyncMock()
            mock_jetstream.publish = AsyncMock()

            # Make jetstream() return the mock jetstream context
            mock_client.jetstream = MagicMock(return_value=mock_jetstream)

            # Make connect return the mock client
            mock_nats.connect.return_value = mock_client

            yield mock_nats, mock_client, mock_jetstream

    @pytest.fixture
    def port(self):
        """Create a mock InstrumentPort."""
        return InstrumentPort(
            default_name="device1",
            pseudo_name=PlungerGate("test_port"),
        )

    @pytest.fixture
    def interpreter_daemon(
        self,
        mock_nats,
    ):
        """Create an instance of InterpreterDaemon with a mock loop."""
        _, _, _ = mock_nats
        asyncio.new_event_loop()
        return InterpreterDaemon(url="nats://localhost:4222")

    @pytest.mark.asyncio
    async def test_initialization(self, interpreter_daemon):
        """Test that the interpreter daemon initializes correctly."""
        assert interpreter_daemon._url == "nats://localhost:4222"
        assert interpreter_daemon._data_queue == {}
        assert interpreter_daemon._measurement_groups == {}

    @pytest.mark.asyncio
    async def test_properties(self, interpreter_daemon):
        """Test the property getters."""
        assert interpreter_daemon.data_queue == interpreter_daemon._data_queue
        assert (
            interpreter_daemon.measurement_groups
            == interpreter_daemon._measurement_groups
        )

    @pytest.mark.asyncio
    async def test_start(self, interpreter_daemon, mock_nats):
        """Test the start method."""
        mock_nats_module, mock_client, mock_jetstream = mock_nats

        # Configure mock_nats.connect to return an awaitable that resolves to mock_client
        mock_nats_module.connect = AsyncMock(return_value=mock_client)

        # Mock the setup_subscriptions method
        interpreter_daemon.setup_subscriptions = AsyncMock()

        # Mock the publish_status method to avoid actually starting it
        publish_status_mock = AsyncMock()
        interpreter_daemon.publish_status = publish_status_mock

        # Create a task for the start method that we'll cancel after a short time
        start_task = asyncio.create_task(interpreter_daemon.start())

        # Give it a moment to run
        await asyncio.sleep(0.1)

        # Now cancel it to simulate shutdown
        start_task.cancel()

        # Wait for the task to be cancelled
        with contextlib.suppress(asyncio.CancelledError):
            await start_task

        # Verify that the daemon connected to NATS
        mock_nats_module.connect.assert_called_once_with("nats://localhost:4222")

        # Verify that jetstream was set up
        mock_client.jetstream.assert_called_once()
        mock_jetstream.add_stream.assert_called_once()

        # Verify that setup_subscriptions was called
        interpreter_daemon.setup_subscriptions.assert_called_once()

        # Verify that drain was called during shutdown (it's in a finally block)
        mock_client.drain.assert_called_once()

    @pytest.mark.asyncio
    async def test_publish_status(
        self,
        interpreter_daemon: InterpreterDaemon,
        mock_nats,
    ):
        """Test the publish_status method."""
        _, mock_client, _ = mock_nats

        interpreter_daemon._nc = mock_client

        # Mock asyncio.all_tasks to return a known set of tasks
        with patch("asyncio.all_tasks") as mock_all_tasks:
            # Create mock tasks
            mock_task1 = AsyncMock()
            mock_task1.done.return_value = False
            mock_task2 = AsyncMock()
            mock_task2.done.return_value = True

            # Make all_tasks return our mock tasks
            mock_all_tasks.return_value = [mock_task1, mock_task2]

            # Create a flag to control when to stop the status publishing
            stop_flag = asyncio.Event()

            # Mock sleep to set the flag after one iteration
            original_sleep = asyncio.sleep

            async def mock_sleep(duration):
                await original_sleep(0.01)  # Short sleep for test
                stop_flag.set()

            with patch("asyncio.sleep", side_effect=mock_sleep):
                # Start publish_status and stop it after one iteration
                task = asyncio.create_task(
                    interpreter_daemon.publish_status(refresh=0.5)
                )
                await stop_flag.wait()
                task.cancel()

                with contextlib.suppress(asyncio.CancelledError):
                    await task

            # Verify publish was called with the correct status
            assert mock_client.publish.call_count >= 1
            args = mock_client.publish.call_args_list[0][0]  # Get the first call args
            assert (
                args[0]
                == INTERPRETER_RUNTIME_COMMANDS.STATUS.COMM_CHANNEL + ".interpreter"
            )

            # Parse the message and verify content
            message_data = json.loads(args[1].decode())
            assert INTERPRETER_RUNTIME_COMMANDS.STATUS.TIMESTAMP in message_data
            # There's one task not done, so status should be False
            assert message_data[INTERPRETER_RUNTIME_COMMANDS.STATUS.STATUS] is False

    @pytest.mark.asyncio
    async def test_send_command(self, interpreter_daemon, mock_nats):
        """Test the send_command method."""
        _, mock_client, _ = mock_nats

        interpreter_daemon._nc = mock_client
        await interpreter_daemon.send_command("test_channel", "test_message")

        mock_client.publish.assert_called_once_with("test_channel", b"test_message")

    @pytest.mark.asyncio
    async def test_log(self, interpreter_daemon, mock_nats):
        """Test the log method."""
        _, mock_client, _ = mock_nats

        interpreter_daemon._nc = mock_client
        await interpreter_daemon.log("test_log_message")

        # Verify the correct message format is published to the log channel
        mock_client.publish.assert_called_once()
        args = mock_client.publish.call_args[0]
        assert args[0] == INTERPRETER_RUNTIME_COMMANDS.LOG.COMM_CHANNEL + ".interpreter"

        # Parse the message to verify content
        message_data = json.loads(args[1].decode())
        assert (
            message_data[INTERPRETER_RUNTIME_COMMANDS.LOG.MESSAGE] == "test_log_message"
        )
        assert INTERPRETER_RUNTIME_COMMANDS.LOG.TIMESTAMP in message_data

    @pytest.mark.asyncio
    async def test_update_daemon_property(self, interpreter_daemon, mock_nats):
        """Test the update_daemon_property method."""
        _, mock_client, _ = mock_nats

        interpreter_daemon._nc = mock_client
        await interpreter_daemon.update_daemon_property(
            property="voltage_state", name="device1", value=1.0
        )

        # Verify the correct message format
        mock_client.publish.assert_called_once()
        args = mock_client.publish.call_args[0]
        assert (
            args[0] == INTERPRETER_RUNTIME_COMMANDS.UPDATE_DAEMON_PROPERTY.COMM_CHANNEL
        )

        # Parse the message to verify content
        message_data = json.loads(args[1].decode())
        assert (
            message_data[INTERPRETER_RUNTIME_COMMANDS.UPDATE_DAEMON_PROPERTY.PROPERTY]
            == "voltage_state"
        )
        assert (
            message_data[INTERPRETER_RUNTIME_COMMANDS.UPDATE_DAEMON_PROPERTY.NAME]
            == "device1"
        )
        assert (
            message_data[INTERPRETER_RUNTIME_COMMANDS.UPDATE_DAEMON_PROPERTY.VALUE]
            == 1.0
        )

    @pytest.mark.asyncio
    async def test_deploy_measurement(
        self,
        interpreter_daemon: InterpreterDaemon,
        mock_nats,
    ):
        """Test the deploy_measurement method."""
        _, mock_client, _ = mock_nats

        interpreter_daemon._nc = mock_client
        port = InstrumentPort(
            default_name="device1",
            pseudo_name=Ohmic("test_port"),
        )
        await interpreter_daemon.deploy_measurement(id=345, getters=[port])

        # Verify the correct message format
        mock_client.publish.assert_called_once()
        args = mock_client.publish.call_args[0]
        assert args[0] == INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.COMM_CHANNEL

        # Parse the message to verify content
        message_data = json.loads(args[1].decode())
        assert (
            message_data[INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.PROCESS_ID]
            == 345
        )
        assert message_data[INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.GETTERS] == [
            port.to_json()
        ]

    @pytest.mark.asyncio
    async def test_setup_subscriptions(self, interpreter_daemon, mock_nats):
        """Test that subscriptions are set up correctly."""
        _, mock_client, _ = mock_nats

        interpreter_daemon._nc = mock_client
        await interpreter_daemon.setup_subscriptions()

        # Verify subscriptions were made
        assert mock_client.subscribe.call_count == 2

        # Check first subscription
        first_call = mock_client.subscribe.call_args_list[0]
        assert (
            first_call[0][0]  # First positional argument is channel
            == INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.COMM_CHANNEL
        )
        assert first_call[1]["cb"] == interpreter_daemon.handle_request

        # Check second subscription
        second_call = mock_client.subscribe.call_args_list[1]
        assert (
            second_call[0][0]  # First positional argument is channel
            == INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.COMM_CHANNEL
        )
        assert second_call[1]["cb"] == interpreter_daemon.handle_data

    @pytest.mark.asyncio
    @patch("server_daemons.interpreter_daemon.MeasurementRequest")
    async def test_handle_request(
        self,
        mock_measurement_request,
        interpreter_daemon: InterpreterDaemon,
        mock_nats,
    ):
        """Test the handle_request method."""
        _, mock_client, _ = mock_nats

        interpreter_daemon._nc = mock_client

        # Mock the necessary methods
        interpreter_daemon.log = AsyncMock()
        interpreter_daemon.process_request = AsyncMock(return_value=(5, (10, 10)))
        interpreter_daemon.deploy_measurements = AsyncMock()
        interpreter_daemon.load_and_export_data = AsyncMock()

        # Create a mock measurement request
        mock_request_obj = MagicMock()
        mock_measurement_request.from_json.return_value = mock_request_obj

        # Create a mock message with test data
        test_data = {
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.REQUEST: '{"type": "test_request"}',
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.PROCESS_ID: 42,
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.CONFIGURATIONS: '{"device1": {"prop1": "value1"}}',
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.DATA_PATH: "/tmp/test_data",
        }
        mock_msg = MockMsg(json.dumps(test_data))

        # Call handle_request with the mock message
        await interpreter_daemon.handle_request(mock_msg)

        # Verify the measurement request was processed correctly
        mock_measurement_request.from_json.assert_called_once_with(
            '{"type": "test_request"}'
        )

        # Verify process_request was called with the correct arguments
        interpreter_daemon.process_request.assert_called_once_with(
            request=mock_request_obj,
            configuration={"device1": {"prop1": "value1"}},
            id=42,
        )

        # Verify deploy_measurements was called
        interpreter_daemon.deploy_measurements.assert_called_once_with(
            measurement_id=42,
        )

        # Verify load_and_export_data was called with the correct arguments
        interpreter_daemon.load_and_export_data.assert_called_once_with(
            request=mock_request_obj,
            data_path=Path("/tmp/test_data"),
            shape=(10, 10),
            id=42,
            data_count=5,
        )

        # Verify log messages
        assert interpreter_daemon.log.call_count >= 2

    @pytest.mark.asyncio
    async def test_handle_request_with_exception(self, interpreter_daemon, mock_nats):
        """Test the handle_request method when an exception occurs."""
        _, mock_client, _ = mock_nats

        interpreter_daemon._nc = mock_client

        # Mock the log method
        interpreter_daemon.log = AsyncMock()

        # Create a mock message with invalid data that will cause an exception
        mock_msg = MockMsg("invalid_json")

        # Call handle_request with the mock message
        await interpreter_daemon.handle_request(mock_msg)

        # Verify error was logged
        interpreter_daemon.log.assert_called_once()
        assert "Error processing request" in interpreter_daemon.log.call_args[0][0]

    @pytest.mark.asyncio
    async def test_handle_data(self, interpreter_daemon: InterpreterDaemon, mock_nats):
        """Test the handle_data method with a mock message."""
        _, mock_client, _ = mock_nats

        interpreter_daemon._nc = mock_client
        interpreter_daemon.log = AsyncMock()

        # Create a mock message with test data
        test_data = {
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.PROCESS_ID: 345,
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.TIMESTAMP: "12345.67",
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.DATA: {
                "device1": [1.0, 2.0, 3.0]
            },
        }
        mock_msg = MockMsg(json.dumps(test_data))

        # Call handle_data with the mock message
        await interpreter_daemon.handle_data(mock_msg)  # type: ignore[no-untyped-call]

        # Verify the data was added to the queue
        assert 345 in interpreter_daemon.data_queue

        assert len(interpreter_daemon.data_queue[345]) == 1

        # Verify the message content
        entry = interpreter_daemon.data_queue[345][0]
        assert entry.timestamp == "12345.67"
        assert entry.data == test_data

        # Verify log message
        interpreter_daemon.log.assert_called_once_with("Data added to queue ....")

    @pytest.mark.asyncio
    async def test_handle_data_with_exception(self, interpreter_daemon, mock_nats):
        """Test the handle_data method when an exception occurs."""
        _, mock_client, _ = mock_nats

        interpreter_daemon._nc = mock_client

        # Mock the log method
        interpreter_daemon.log = AsyncMock()

        # Create a mock message with invalid data that will cause an exception
        mock_msg = MockMsg("invalid_json")

        # Call handle_data with the mock message
        await interpreter_daemon.handle_data(mock_msg)

        # Verify error was logged
        interpreter_daemon.log.assert_called_once()
        assert (
            "Error adding data to the queue" in interpreter_daemon.log.call_args[0][0]
        )

    @pytest.mark.asyncio
    async def test_deploy_measurements(
        self,
        interpreter_daemon: InterpreterDaemon,
        mock_nats,
        port: InstrumentPort,
    ):
        """Test the deploy_measurements method."""
        _, mock_client, _ = mock_nats

        interpreter_daemon._nc = mock_client

        # Mock the update_daemon_property method
        interpreter_daemon.update_daemon_property = AsyncMock()

        # Mock the deploy_measurement method
        interpreter_daemon.deploy_measurement = AsyncMock()

        port = InstrumentPort(
            default_name="device1",
            pseudo_name=PlungerGate("test_port"),
        )

        # Create test measurement instructions
        measurement_id = 4
        instr1 = Instruction(
            setters={port: {SUPPORTED_PROPERTIES.VOLTAGE_STATE: 1.0}},
            getters=[port],
        )
        instr2 = Instruction(
            setters={port: {SUPPORTED_PROPERTIES.VOLTAGE_STATE: 2.0}},
            getters=[port],
        )

        interpreter_daemon._measurement_groups[measurement_id] = (
            MeasurementInstructions(instructions=[instr1, instr2])
        )

        # Call deploy_measurements
        await interpreter_daemon.deploy_measurements(measurement_id=measurement_id)

        # Verify update_daemon_property was called for each setter
        assert interpreter_daemon.update_daemon_property.call_count == 2

        # Instead of checking specific call orders, check that each call was made with proper arguments
        update_calls = interpreter_daemon.update_daemon_property.call_args_list
        assert any(
            call[1].get("property") == SUPPORTED_PROPERTIES.VOLTAGE_STATE
            and call[1].get("value") == 1.0
            for call in update_calls
        )
        assert any(
            call[1].get("property") == SUPPORTED_PROPERTIES.VOLTAGE_STATE
            and call[1].get("value") == 2.0
            for call in update_calls
        )

        # Verify deploy_measurement was called for each instruction
        assert interpreter_daemon.deploy_measurement.call_count == 2
        interpreter_daemon.deploy_measurement.assert_has_calls(
            [
                call(id=measurement_id, getters=[port], setters={}),
                call(id=measurement_id, getters=[port], setters={}),
            ]
        )

    @pytest.mark.asyncio
    @patch("server_daemons.interpreter_daemon.HDF5Data")
    async def test_upload_data(
        self,
        mock_hdf5,
        interpreter_daemon: InterpreterDaemon,
        mock_nats,
    ):
        """Test the upload_data method."""
        _, mock_client, jetstream_client = mock_nats

        interpreter_daemon._nc = mock_client
        interpreter_daemon._js = jetstream_client

        # Create a mock response with a to_json method for serialization
        response = MagicMock(spec=MeasurementResponse)
        response.to_json.return_value = {"mock": "data"}

        # Mock json.dumps to handle the MeasurementResponse object
        with patch("json.dumps") as mock_dumps:
            mock_dumps.return_value = (
                '{"data": {"mock": "data"}, "timestamp": "12345.67"}'
            )

            # Call upload_data
            await interpreter_daemon.upload_data(response, id=1)

            # Verify the correct message was published
            mock_client.publish.assert_called_once()
            args = mock_client.publish.call_args[0]
            assert args[0] == INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.COMM_CHANNEL

    @pytest.mark.asyncio
    async def test_load_and_export_data(
        self,
        interpreter_daemon: InterpreterDaemon,
    ):
        """Test the load_and_export_data method."""
        # Mock the necessary methods
        interpreter_daemon.confirm_data_exists = AsyncMock()
        interpreter_daemon.get_data_point_counter_per_queue = MagicMock(return_value=5)
        interpreter_daemon.preprocess_voltage_states = MagicMock(
            return_value=[{"device1": 1.0}]
        )
        interpreter_daemon.average_shapeless_data = MagicMock(
            return_value={"device1": [1.0, 2.0, 3.0]}
        )
        interpreter_daemon.make_response = MagicMock()
        interpreter_daemon.store_in_database = MagicMock()
        interpreter_daemon.upload_data = AsyncMock()

        # Create mock request and response
        mock_request = MagicMock(spec=MeasurementRequest)
        mock_response = MagicMock(spec=MeasurementResponse)
        interpreter_daemon.make_response.return_value = mock_response

        # Call load_and_export_data
        await interpreter_daemon.load_and_export_data(
            request=mock_request,
            data_path=Path("/tmp/test_data"),
            shape=(3,),
            id=345,
            data_count=2,
        )

        # Verify the methods were called with correct arguments
        interpreter_daemon.confirm_data_exists.assert_called_once_with(
            id=345, data_count=2
        )

        interpreter_daemon.get_data_point_counter_per_queue.assert_called_once_with(
            shape=(3,), data_count=2
        )

        interpreter_daemon.preprocess_voltage_states.assert_called_once_with(
            id=345,
        )

        interpreter_daemon.average_shapeless_data.assert_called_once_with(
            id=345,
            number_of_bins=5,
            request=mock_request,
            voltage_state_array=[{"device1": 1.0}],
        )

        interpreter_daemon.make_response.assert_called_once_with(
            data_arrays={"device1": [1.0, 2.0, 3.0]}, shape=(3,)
        )

        interpreter_daemon.store_in_database.assert_called_once_with(
            response=mock_response,
            request=mock_request,
            id=345,
            data_path=Path("/tmp/test_data"),
        )

        interpreter_daemon.upload_data.assert_called_once_with(
            response=mock_response,
            id=345,
        )

    @pytest.mark.asyncio
    async def test_chunk_instructions_non_buffered(
        self,
        interpreter_daemon: InterpreterDaemon,
    ):
        """Test the chunk_instructions method with non-buffered data."""
        # Create mock data array
        mock_array = MagicMock()
        mock_array.data = np.array([[1.0, 2.0, 3.0], [4.0, 5.0, 6.0]])

        # Call chunk_instructions with buffered=False
        chunks = interpreter_daemon.chunk_instructions(mock_array, buffered=False)

        # Verify the chunks
        assert len(chunks) == 3
        np.testing.assert_array_equal(chunks[0], np.array([[1.0], [4.0]]))
        np.testing.assert_array_equal(chunks[1], np.array([[2.0], [5.0]]))
        np.testing.assert_array_equal(chunks[2], np.array([[3.0], [6.0]]))

    @pytest.mark.asyncio
    async def test_simple_chunk_instructions_buffered(
        self,
        interpreter_daemon: InterpreterDaemon,
    ):
        """Test the chunk_instructions method with buffered data."""
        # Create a staircased array where primary axis increases then resets
        # For each chunk, non-time axes must be constant
        data = np.array(
            [
                # First staircase (primary axis increases)
                [0.0, 1.0, 2.0, 3.0, 4.0, 0.0, 1.0, 2.0, 3.0, 4.0],
                # Secondary axis constant within each chunk
                [1.0, 1.0, 1.0, 1.0, 1.0, 2.0, 2.0, 2.0, 2.0, 2.0],
            ]
        )

        mock_array = MagicMock()
        mock_array.data = data

        # Call chunk_instructions with buffered=True
        chunks = interpreter_daemon.chunk_instructions(mock_array, buffered=True)

        # Verify the chunks - should split at the point where primary axis resets
        assert len(chunks) == 2
        np.testing.assert_array_equal(chunks[0], data[:, 0:5])
        np.testing.assert_array_equal(chunks[1], data[:, 5:10])

    @pytest.mark.asyncio
    async def test_chunk_instructions_buffered(
        self,
        interpreter_daemon: InterpreterDaemon,
    ):
        """Test the chunk_instructions method with buffered data."""
        # Create a staircased array where primary axis increases then resets
        # For each chunk, non-time axes must be constant
        staircase1 = [0.0, 1.0, 2.0, 3.0, 4.0]
        x1 = [1.0, 1.0, 1.0, 1.0, 1.0]
        staircase2 = [2.0, 2.5, 3.0]
        x2 = [1.0, 1.0, 1.0]
        data = np.array(
            [
                # First staircase (primary axis increases)
                staircase1 + staircase1 + staircase2,
                # Secondary axis constant within each chunk
                x1 + list(2 * np.array(x1)) + list(3 * np.array(x2)),
                list(5 * np.array(x1)) + list(3 * np.array(x1)) + x2,
            ]
        )

        mock_array = MagicMock()
        mock_array.data = data

        # Call chunk_instructions with buffered=True
        chunks = interpreter_daemon.chunk_instructions(mock_array, buffered=True)

        # Verify the chunks - should split at the point where primary axis resets
        assert len(chunks) == 3
        np.testing.assert_array_equal(chunks[0], data[:, 0:5])
        np.testing.assert_array_equal(chunks[1], data[:, 5:10])
        np.testing.assert_array_equal(chunks[2], data[:, 10:])

    @pytest.mark.asyncio
    async def test_negative_chunk_instructions_buffered(
        self,
        interpreter_daemon: InterpreterDaemon,
    ):
        """Test the chunk_instructions method with buffered data."""
        # Create a staircased array where primary axis increases then resets
        # For each chunk, non-time axes must be constant
        staircase1 = list(-1 * np.array([0.0, 1.0, 2.0, 3.0, 4.0]))
        x1 = [1.0, 1.0, 1.0, 1.0, 1.0]
        staircase2 = list(-1 * np.array([2.0, 2.5, 3.0]))
        x2 = [1.0, 1.0, 1.0]
        data = np.array(
            [
                # First staircase (primary axis increases)
                staircase1 + staircase1 + staircase2,
                # Secondary axis constant within each chunk
                x1 + list(2 * np.array(x1)) + list(3 * np.array(x2)),
                list(5 * np.array(x1)) + list(3 * np.array(x1)) + x2,
            ]
        )

        mock_array = MagicMock()
        mock_array.data = data

        # Call chunk_instructions with buffered=True
        chunks = interpreter_daemon.chunk_instructions(mock_array, buffered=True)

        # Verify the chunks - should split at the point where primary axis resets
        assert len(chunks) == 3
        np.testing.assert_array_equal(chunks[0], data[:, 0:5])
        np.testing.assert_array_equal(chunks[1], data[:, 5:10])
        np.testing.assert_array_equal(chunks[2], data[:, 10:])

    @pytest.mark.asyncio
    async def test_interject_ramps(
        self,
        interpreter_daemon: InterpreterDaemon,
        port: InstrumentPort,
    ):
        """Test the interject_ramps method."""
        # Create test instructions
        instr1 = Instruction(
            setters={port: {SUPPORTED_PROPERTIES.STAIRCASE: (10, 5, 0, 0.0, 1.0)}}
        )
        instr2 = Instruction(
            setters={port: {SUPPORTED_PROPERTIES.STAIRCASE: (10, 5, 0, 1.0, 2.0)}}
        )

        # Call interject_ramps
        result = interpreter_daemon.interject_ramps([instr1, instr2])

        # Verify the interjected ramps
        assert len(result) == 3
        assert result[0] == instr1
        # The middle instruction should set to the start value of the next instruction
        assert SUPPORTED_PROPERTIES.VOLTAGE_STATE in result[1].setters[port]
        assert result[1].setters[port][SUPPORTED_PROPERTIES.VOLTAGE_STATE] == 1.0
        assert result[2] == instr2

    @pytest.mark.asyncio
    async def test_confirm_data_exists(
        self,
        interpreter_daemon: InterpreterDaemon,
    ):
        """Test the confirm_data_exists method."""
        # Setup a mock data queue
        test_id = 4524
        interpreter_daemon._data_queue[test_id] = DataQueue()

        # Create a mock DataEntry with timestamp and data
        mock_entry = MagicMock(spec=DataEntry)
        mock_entry.timestamp = "12345.67"

        # Add the entry to the queue
        interpreter_daemon._data_queue[test_id].append(mock_entry)

        # Mock the log method
        interpreter_daemon.log = AsyncMock()

        # Call confirm_data_exists with matching data count
        await interpreter_daemon.confirm_data_exists(
            id=test_id, data_count=1, max_attempts=1, wait_time=0.1
        )
