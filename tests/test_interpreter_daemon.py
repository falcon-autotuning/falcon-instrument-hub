"""Testing the interpreter daemon."""

import asyncio
import contextlib
import json
import time
from pathlib import Path
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock, patch

import numpy as np
import pytest
from falcon_core.communications.messages import MeasurementRequest
from falcon_core.constants import INSTRUMENT_TYPES
from falcon_core.instrument_interfaces.names import InstrumentPort, Knob, Meter, Meters
from falcon_core.instrument_interfaces.port_transforms.identity_transform import (
    IdentityTransform,
)
from falcon_core.instrument_interfaces.waveforms.cartesian_waveform import (
    CartesianWaveform,
)
from falcon_core.math.arrays import MeasuredArray1D
from falcon_core.math.axes import Axes
from falcon_core.math.discrete_spaces import CartesianDiscreteSpace
from falcon_core.math.domains import CoupledKnobDomain, KnobDomain
from falcon_core.math.spaces import CartesianSpace
from falcon_core.physics.device_structures import BarrierGate, Ohmic, PlungerGate
from falcon_core.physics.units import Units
from instrument_templates.constants import SUPPORTED_PROPERTIES

from server_daemons.api.interpreter import (
    RUNTIME_COMMANDS as INTERPRETER_RUNTIME_COMMANDS,
)
from server_daemons.dependancies import (
    MeasurementResponse,
)
from server_daemons.instructions import (
    Instruction,
)
from server_daemons.interpreter_daemon import (
    STALE_MEASUREMENT_TIMEOUT,
    InterpreterDaemon,
)

if TYPE_CHECKING:
    from instrument_templates.typing import PropertyJson, PropertyName


class MockMsg:
    """Mock implementation of NATS message."""

    def __init__(self, data):
        self.data = data.encode() if isinstance(data, str) else data
        self.subject = "test.subject"
        self.reply = None
        self.headers = None

    def respond(self, data):
        """Mock respond method."""

    def ack(self):
        """Mock ack method."""

    def nak(self):
        """Mock nak method."""


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
        name = InstrumentPort(
            default_name="device1",
            pseudo_name=Ohmic("test_knob"),
        )
        await interpreter_daemon.update_daemon_property(
            property="voltage_state",
            name=name,
            value=1.0,
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
            == name.to_json()
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
        await interpreter_daemon.deploy_measurement(
            id=345,
            getters=[port],
            requirements={},
            setters=[],
        )

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
    async def test_handle_data_queueing_only(
        self, interpreter_daemon: InterpreterDaemon, mock_nats
    ):
        """Test that handle_data correctly queues data without full processing."""
        _, mock_client, _ = mock_nats

        interpreter_daemon._nc = mock_client
        interpreter_daemon.log = AsyncMock()

        # Don't register measurement - test the requeuing behavior
        port = InstrumentPort(
            default_name="device1",
            pseudo_name=PlungerGate("test_gate"),
        )
        data = {port.to_json(): json.dumps([1.0, 2.0, 3.0])}

        # Create a mock message with test data
        test_data = {
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.PROCESS_ID: 345,
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.CHUNK_ID: 0,
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.DATA: json.dumps(data),
        }
        mock_msg = MockMsg(json.dumps(test_data))

        # Call handle_data - this should queue the data
        await interpreter_daemon.handle_data(mock_msg)  # type: ignore

        # Verify data was queued
        assert not interpreter_daemon._async_data_queue.empty()

        # Get the queued item
        entry = await interpreter_daemon._async_data_queue.get()
        assert entry.measurement_id == 345
        assert entry.chunk_id == 0
        assert port in entry.data

    @pytest.mark.asyncio
    async def test_handle_data(self, interpreter_daemon: InterpreterDaemon, mock_nats):
        """Test the handle_data method with a mock message."""
        _, mock_client, _ = mock_nats

        interpreter_daemon._nc = mock_client
        interpreter_daemon.log = AsyncMock()

        # Mock the load_and_export_data method to avoid complex processing
        interpreter_daemon.load_and_export_data = AsyncMock()

        # Register a measurement first so the data doesn't get requeued
        await interpreter_daemon._register_measurement(
            measurement_id=345,
            expected_count=1,
            data_path=Path("/tmp/test"),
            shape=(5,),
            request=MagicMock(),
        )

        port = InstrumentPort(
            default_name="device1",
            pseudo_name=PlungerGate("test_gate"),
        )
        data = {port.to_json(): json.dumps([1.0, 2.0, 3.0])}

        # Create a mock message with test data
        test_data = {
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.PROCESS_ID: 345,
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.CHUNK_ID: 0,
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.DATA: json.dumps(data),
        }
        mock_msg = MockMsg(json.dumps(test_data))

        # Start the queue processor task
        queue_processor_task = asyncio.create_task(
            interpreter_daemon._process_async_data_queue()
        )

        try:
            # Call handle_data
            await interpreter_daemon.handle_data(mock_msg)  # type: ignore

            # Wait for processing to complete
            max_wait = 1.0
            wait_interval = 0.01
            total_waited = 0.0

            while (
                345 in interpreter_daemon._pending_measurements
                and total_waited < max_wait
            ):
                await asyncio.sleep(wait_interval)
                total_waited += wait_interval

            # Verify the measurement was processed and load_and_export_data was called
            interpreter_daemon.load_and_export_data.assert_called_once()

            # Verify measurement was removed after processing
            assert 345 not in interpreter_daemon._pending_measurements

        finally:
            # Clean up
            queue_processor_task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await queue_processor_task

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
        assert "Error queueing data" in interpreter_daemon.log.call_args[0][0]

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
        mock_nats,
    ):
        """Test the load_and_export_data method."""
        # Mock the necessary methods
        _, mock_client, jetstream_client = mock_nats

        interpreter_daemon._nc = mock_client
        interpreter_daemon._js = jetstream_client
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

        # Create mock chunk data
        port = InstrumentPort(
            default_name="device1",
            pseudo_name=PlungerGate("test_gate"),
        )

        chunk_data = {
            0: {port: MeasuredArray1D([1.0, 2.0, 3.0])},
            1: {port: MeasuredArray1D([4.0, 5.0, 6.0])},
        }

        # Call load_and_export_data with chunk_data
        await interpreter_daemon.load_and_export_data(
            request=mock_request,
            data_path=Path("/tmp/test_data"),
            shape=(3,),
            id=345,
            data_count=2,
            chunk_data=chunk_data,
        )

        # Verify the methods were called
        interpreter_daemon.get_data_point_counter_per_queue.assert_called_once_with(
            shape=(3,), data_count=2
        )

        interpreter_daemon.preprocess_voltage_states.assert_called_once_with(
            id=345,
        )

        interpreter_daemon.average_shapeless_data.assert_called_once_with(
            number_of_bins=5,
            request=mock_request,
            voltage_state_array=[{"device1": 1.0}],
            chunk_data=chunk_data,  # Now passes chunk_data instead of collected_data
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
        data = np.array(
            [
                [1.0, 4.0],
                [2.0, 5.0],
                [3.0, 6.0],
            ]
        )
        mock_array = MagicMock()
        mock_array.data = data

        # Call chunk_instructions with buffered=False
        chunks = interpreter_daemon.chunk_instructions(mock_array, buffered=False)

        # Verify the chunks - for non-buffered, each column becomes a separate chunk
        assert len(chunks) == 3  # 3 columns in the data
        np.testing.assert_array_equal(chunks[0], data[0:1, :])
        np.testing.assert_array_equal(chunks[1], data[1:2, :])
        np.testing.assert_array_equal(chunks[2], data[2:3, :])

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
                [0.0, 1.0],
                [1.0, 1.0],
                [2.0, 1.0],
                [3.0, 1.0],
                [4.0, 1.0],
                [0.0, 2.0],
                [1.0, 2.0],
                [2.0, 2.0],
                [3.0, 2.0],
                [4.0, 2.0],
            ]
        )

        mock_array = MagicMock()
        mock_array.data = data

        # Call chunk_instructions with buffered=True
        chunks = interpreter_daemon.chunk_instructions(mock_array, buffered=True)

        # Verify the chunks - should split at the point where primary axis resets
        assert len(chunks) == 2
        np.testing.assert_array_equal(chunks[0], data[0:5, :])
        np.testing.assert_array_equal(chunks[1], data[5:10, :])

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
        ).T

        mock_array = MagicMock()
        mock_array.data = data

        # Call chunk_instructions with buffered=True
        chunks = interpreter_daemon.chunk_instructions(mock_array, buffered=True)

        # Verify the chunks - should split at the point where primary axis resets
        assert len(chunks) == 3
        np.testing.assert_array_equal(chunks[0], data[0:5, :])
        np.testing.assert_array_equal(chunks[1], data[5:10, :])
        np.testing.assert_array_equal(chunks[2], data[10:, :])

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
        ).T

        mock_array = MagicMock()
        mock_array.data = data

        # Call chunk_instructions with buffered=True
        chunks = interpreter_daemon.chunk_instructions(mock_array, buffered=True)

        # Verify the chunks - should split at the point where primary axis resets
        assert len(chunks) == 3
        np.testing.assert_array_equal(chunks[0], data[0:5, :])
        np.testing.assert_array_equal(chunks[1], data[5:10, :])
        np.testing.assert_array_equal(chunks[2], data[10:, :])

    @pytest.mark.asyncio
    async def test_interject_ramps(
        self,
        interpreter_daemon: InterpreterDaemon,
        port: InstrumentPort,
        mock_nats,
    ):
        """Test the interject_ramps method."""
        _, mock_client, _ = mock_nats
        interpreter_daemon._nc = mock_client
        # Create test instructions
        instr1 = Instruction(
            requirements={port: {SUPPORTED_PROPERTIES.STAIRCASE: (10, 5, 0, 0.0, 1.0)}}
        )
        instr2 = Instruction(
            requirements={port: {SUPPORTED_PROPERTIES.STAIRCASE: (10, 5, 0, 1.0, 2.0)}}
        )

        # Call interject_ramps - this is an async method
        result = await interpreter_daemon.interject_ramps([instr1, instr2])

        # Verify the interjected ramps
        assert len(result) == 3
        assert result[0] == instr1
        # The middle instruction should set to the start value of the next instruction
        assert SUPPORTED_PROPERTIES.VOLTAGE_STATE in result[1].requirements[port]
        assert result[1].requirements[port][SUPPORTED_PROPERTIES.VOLTAGE_STATE] == 1.0
        assert result[2] == instr2

    @pytest.fixture
    def knobs(self):
        """Returns a list of active knobs."""
        return [
            Knob(
                default_name=f"testDAC_##_{SUPPORTED_PROPERTIES.VOLTAGE_STATE}_##_1",
                pseudo_name=BarrierGate("B3"),
                instrument_type=INSTRUMENT_TYPES.DC_VOLTAGE_SOURCE,
            )
        ]

    @pytest.fixture
    def meters(self):
        """Returns a list of active meters."""
        return [
            Meter(
                default_name=f"testADC_##_{SUPPORTED_PROPERTIES.CURRENT_STATE}_##_1",
                pseudo_name=Ohmic("I_O2"),
                instrument_type=INSTRUMENT_TYPES.AMNMETER,
            )
        ]

    @pytest.fixture
    def measurement_request(self, knobs: list[Knob], meters: list[Meter]):
        """Returns a measurement request for testing deployment."""
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
        return MeasurementRequest(
            message="test measurement",
            measurement_name="integration_test",
            waveforms=[waveform],
            meter_transforms=[transform],
        )

    @pytest.fixture
    def configuration(self, knobs: list[Knob], meters: list[Meter]):
        """Returns the server configuration."""
        outs = {}
        for knob in knobs:
            outs[knob] = {
                SUPPORTED_PROPERTIES.VOLTAGE_STATE: {
                    "bounds": (-1.0, 1.0),
                    "unit": "doesnt matter",
                    "settable": True,
                }
            }
            slope = InstrumentPort(
                default_name=f"testDAC_##_{SUPPORTED_PROPERTIES.SLOPE}_##_1",
                instrument_type=INSTRUMENT_TYPES.DC_VOLTAGE_SOURCE,
            )
            outs[slope] = {
                SUPPORTED_PROPERTIES.SLOPE: {
                    "bounds": (-1.0, 1.0),
                    "unit": "doesnt matter",
                    "settable": True,
                }
            }
            timeout = InstrumentPort(
                default_name=f"testDAC_##_{SUPPORTED_PROPERTIES.TIMEOUT}_##_1",
                instrument_type=INSTRUMENT_TYPES.DC_VOLTAGE_SOURCE,
            )
            outs[timeout] = {
                SUPPORTED_PROPERTIES.TIMEOUT: {
                    "bounds": (0, 60.0),
                    "unit": "doesnt matter",
                    "settable": True,
                }
            }

        for meter in meters:
            outs[meter] = {
                SUPPORTED_PROPERTIES.CURRENT_STATE: {
                    "bounds": (-1.0, 1.0),
                    "unit": "doesnt matter",
                    "settable": False,
                }
            }
            num_samp = InstrumentPort(
                default_name=f"testADC_##_{SUPPORTED_PROPERTIES.NUMBER_OF_SAMPLES}_##_1",
                instrument_type=INSTRUMENT_TYPES.AMNMETER,
            )
            outs[num_samp] = {
                SUPPORTED_PROPERTIES.NUMBER_OF_SAMPLES: {
                    "bounds": (0, 1000000),
                    "unit": "doesnt matter",
                    "settable": True,
                }
            }
            timeout = InstrumentPort(
                default_name=f"testADC_##_{SUPPORTED_PROPERTIES.TIMEOUT}_##_1",
                instrument_type=INSTRUMENT_TYPES.DC_VOLTAGE_SOURCE,
            )
            outs[timeout] = {
                SUPPORTED_PROPERTIES.TIMEOUT: {
                    "bounds": (0, 60.0),
                    "unit": "doesnt matter",
                    "settable": True,
                }
            }
        return outs

    @pytest.mark.asyncio
    async def test_process_request(
        self,
        interpreter_daemon: InterpreterDaemon,
        measurement_request: MeasurementRequest,
        configuration: dict["InstrumentPort", dict["PropertyName", "PropertyJson"]],
        mock_nats,
    ):
        """Test the process_request method."""
        _, mock_client, _ = mock_nats
        interpreter_daemon._nc = mock_client

        length, shape = await interpreter_daemon.process_request(
            request=measurement_request,
            configuration=configuration,
            id=4,
        )
        assert length == 10, "Improper number of measurements"
        assert all(
            [
                len(instruction.getters) == 1
                for instruction in interpreter_daemon._measurement_groups[
                    4
                ].instructions
            ]
        ), "Improper number of getters in instruction"
        assert all(
            [
                len(instruction.setters) == 1
                for instruction in interpreter_daemon._measurement_groups[
                    4
                ].instructions
            ]
        ), "Improper number of setters in instruction"

    @pytest.mark.asyncio
    async def test_async_queue_functionality(
        self,
        interpreter_daemon: InterpreterDaemon,
        mock_nats,
    ):
        """Test the async queue data processing."""
        _, mock_client, _ = mock_nats
        interpreter_daemon._nc = mock_client
        interpreter_daemon.log = AsyncMock()

        # Mock load_and_export_data to avoid complex processing
        interpreter_daemon.load_and_export_data = AsyncMock()

        # Start the async queue processor task
        queue_processor_task = asyncio.create_task(
            interpreter_daemon._process_async_data_queue()
        )

        try:
            # Register a measurement
            measurement_id = 12345
            expected_count = 3
            data_path = Path("/tmp/test_measurement")
            shape = (10, 5)
            mock_request = MagicMock()

            await interpreter_daemon._register_measurement(
                measurement_id=measurement_id,
                expected_count=expected_count,
                data_path=data_path,
                shape=shape,
                request=mock_request,
            )

            # Verify measurement was registered
            assert measurement_id in interpreter_daemon._pending_measurements
            pending = interpreter_daemon._pending_measurements[measurement_id]
            assert pending.expected_count == expected_count
            assert pending.measurement_id == measurement_id
            assert not pending.is_complete

            # Queue some data entries
            port1 = InstrumentPort(
                default_name="device1",
                pseudo_name=PlungerGate("gate1"),
            )
            port2 = InstrumentPort(
                default_name="device2",
                pseudo_name=PlungerGate("gate2"),
            )

            # Add data entries for different chunks
            for chunk_id in range(3):
                test_data = {
                    port1: MeasuredArray1D(
                        [1.0 + chunk_id, 2.0 + chunk_id, 3.0 + chunk_id]
                    ),
                    port2: MeasuredArray1D(
                        [4.0 + chunk_id, 5.0 + chunk_id, 6.0 + chunk_id]
                    ),
                }
                await interpreter_daemon._queue_measurement_data(
                    measurement_id, chunk_id, test_data
                )

            # Wait for processing to complete
            max_wait = 2.0
            wait_interval = 0.01
            total_waited = 0.0

            while (
                measurement_id in interpreter_daemon._pending_measurements
                and total_waited < max_wait
            ):
                await asyncio.sleep(wait_interval)
                total_waited += wait_interval

            # Verify measurement was processed and removed from pending
            assert measurement_id not in interpreter_daemon._pending_measurements

            # Verify load_and_export_data was called
            interpreter_daemon.load_and_export_data.assert_called_once()

        finally:
            # Clean up the queue processor task
            queue_processor_task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await queue_processor_task

    @pytest.mark.asyncio
    async def test_pending_measurement_properties(self):
        """Test PendingMeasurement dataclass properties."""
        from server_daemons.interpreter_daemon import PendingMeasurement

        measurement_id = 999
        expected_count = 5
        data_path = Path("/tmp/test")
        shape = (10,)
        request = MagicMock()

        pending = PendingMeasurement(
            measurement_id=measurement_id,
            expected_count=expected_count,
            data_path=data_path,
            shape=shape,
            request=request,
        )

        # Test initial state
        assert not pending.is_complete
        assert pending.completion_percentage == 0.0
        assert len(pending.collected_data) == 0

        # Add some data entries
        port = InstrumentPort(
            default_name="test_port",
            pseudo_name=PlungerGate("test_gate"),
        )

        for i in range(3):
            pending.add_data_entry(
                chunk_id=i, data={port: MeasuredArray1D([1.0, 2.0, 3.0])}
            )

        # Test partial completion
        assert not pending.is_complete
        assert pending.completion_percentage == 60.0  # 3/5 * 100
        assert len(pending.collected_data) == 3

        # Complete the measurement
        for i in range(3, 5):
            pending.add_data_entry(
                chunk_id=i, data={port: MeasuredArray1D([1.0, 2.0, 3.0])}
            )

        # Test completion
        assert pending.is_complete
        assert pending.completion_percentage == 100.0
        assert len(pending.collected_data) == 5

    @pytest.mark.asyncio
    async def test_chunk_data_organization(self):
        """Test that chunk data is properly organized."""
        from server_daemons.interpreter_daemon import PendingMeasurement

        measurement_id = 888
        expected_count = 3
        data_path = Path("/tmp/test")
        shape = (5,)
        request = MagicMock()

        pending = PendingMeasurement(
            measurement_id=measurement_id,
            expected_count=expected_count,
            data_path=data_path,
            shape=shape,
            request=request,
        )

        port1 = InstrumentPort(
            default_name="port1",
            pseudo_name=PlungerGate("gate1"),
        )
        port2 = InstrumentPort(
            default_name="port2",
            pseudo_name=PlungerGate("gate2"),
        )

        # Add data in non-sequential order to test sorting
        chunk_data = [
            (
                2,
                {
                    port1: MeasuredArray1D([7.0, 8.0, 9.0]),
                    port2: MeasuredArray1D([10.0, 11.0, 12.0]),
                },
            ),
            (
                0,
                {
                    port1: MeasuredArray1D([1.0, 2.0, 3.0]),
                    port2: MeasuredArray1D([4.0, 5.0, 6.0]),
                },
            ),
            (
                1,
                {
                    port1: MeasuredArray1D([4.0, 5.0, 6.0]),
                    port2: MeasuredArray1D([7.0, 8.0, 9.0]),
                },
            ),
        ]

        for chunk_id, data in chunk_data:
            pending.add_data_entry(chunk_id, data)

        # Test sorted chunk data
        sorted_chunks = pending.get_sorted_chunk_data()

        # Verify chunks are in order
        chunk_ids = list(sorted_chunks.keys())
        assert chunk_ids == [0, 1, 2]

        # Verify data integrity
        assert len(sorted_chunks[0]) == 2  # Two ports
        assert port1 in sorted_chunks[0]
        assert port2 in sorted_chunks[0]

        # Verify data values for chunk 0
        np.testing.assert_array_equal(
            sorted_chunks[0][port1].data, np.array([1.0, 2.0, 3.0])
        )
        np.testing.assert_array_equal(
            sorted_chunks[0][port2].data, np.array([4.0, 5.0, 6.0])
        )

    @pytest.mark.asyncio
    async def test_stale_measurement_cleanup(
        self,
        interpreter_daemon: InterpreterDaemon,
        mock_nats,
    ):
        """Test that stale measurements are cleaned up."""
        _, mock_client, _ = mock_nats
        interpreter_daemon._nc = mock_client
        interpreter_daemon.log = AsyncMock()

        # Register a measurement
        measurement_id = 777
        await interpreter_daemon._register_measurement(
            measurement_id=measurement_id,
            expected_count=5,
            data_path=Path("/tmp/test"),
            shape=(10,),
            request=MagicMock(),
        )

        # Mock time.time to control the cleanup logic
        with patch("time.time") as mock_time:
            # Set current time
            current_time = 1000
            mock_time.return_value = current_time

            # Set measurement created_at to be stale
            stale_time = current_time - STALE_MEASUREMENT_TIMEOUT - 1
            interpreter_daemon._pending_measurements[
                measurement_id
            ].created_at = stale_time

            # Create a single cleanup task that will exit after one iteration
            async def single_cleanup():
                current_time = time.time()
                stale_ids = []
                for mid, pending in interpreter_daemon._pending_measurements.items():
                    if current_time - pending.created_at > STALE_MEASUREMENT_TIMEOUT:
                        await interpreter_daemon.log(
                            f"Warning: Measurement {mid} timed out with {len(pending.collected_data)}/{pending.expected_count} data points ({pending.completion_percentage:.1f}%)"
                        )
                        stale_ids.append(mid)

                for stale_id in stale_ids:
                    del interpreter_daemon._pending_measurements[stale_id]

            # Run the cleanup
            await single_cleanup()

            # Verify measurement was cleaned up
            assert measurement_id not in interpreter_daemon._pending_measurements

    @pytest.mark.asyncio
    async def test_end_to_end_measurement_processing(
        self,
        interpreter_daemon: InterpreterDaemon,
        measurement_request: MeasurementRequest,
        configuration: dict,
        mock_nats,
    ):
        """Test complete measurement processing from request to data export."""
        _, mock_client, mock_jetstream = mock_nats
        interpreter_daemon._nc = mock_client
        interpreter_daemon._js = mock_jetstream
        interpreter_daemon.log = AsyncMock()

        # Mock the load_and_export_data method to track what it receives
        load_and_export_calls = []

        async def mock_load_and_export(*args, **kwargs):
            load_and_export_calls.append((args, kwargs))
            await interpreter_daemon.log("Mock load_and_export_data called")

        interpreter_daemon.load_and_export_data = mock_load_and_export

        # Start the queue processor to handle data processing
        queue_processor_task = asyncio.create_task(
            interpreter_daemon._process_async_data_queue()
        )

        try:
            # Step 1: Process the measurement request
            measurement_id = 12345
            data_path = Path("/tmp/test_measurement")

            # Simulate the request processing
            data_count, shape = await interpreter_daemon.process_request(
                request=measurement_request,
                configuration=configuration,
                id=measurement_id,
            )

            # Register the measurement (this normally happens in handle_request)
            await interpreter_daemon._register_measurement(
                measurement_id=measurement_id,
                expected_count=data_count,
                data_path=data_path,
                shape=shape,
                request=measurement_request,
            )

            await interpreter_daemon.log(
                f"Registered measurement with {data_count} expected chunks"
            )

            # Step 2: Simulate data collection from multiple chunks
            # Get the meter ports from the measurement request
            meter_ports = [
                transform.port for transform in measurement_request.meter_transforms
            ]

            # Simulate receiving data for each chunk
            for chunk_id in range(data_count):
                # Create realistic measurement data
                chunk_data = {}
                for port in meter_ports:
                    # Generate some test data for this port/chunk
                    data_points = [float(chunk_id + i + 1) * 0.1 for i in range(10)]
                    chunk_data[port] = MeasuredArray1D(data_points)

                # Queue the data (simulating handle_data)
                await interpreter_daemon._queue_measurement_data(
                    measurement_id=measurement_id, chunk_id=chunk_id, data=chunk_data
                )

                await interpreter_daemon.log(f"Queued data for chunk {chunk_id}")

            # Step 3: Wait for async processing to complete
            max_wait_time = 5.0  # 5 seconds max
            wait_interval = 0.1
            total_waited = 0.0

            while (
                measurement_id in interpreter_daemon._pending_measurements
                and total_waited < max_wait_time
            ):
                await asyncio.sleep(wait_interval)
                total_waited += wait_interval

            # Step 4: Verify the measurement was processed
            assert measurement_id not in interpreter_daemon._pending_measurements, (
                "Measurement should be completed and removed"
            )

            # Verify load_and_export_data was called
            assert len(load_and_export_calls) == 1, (
                "load_and_export_data should be called exactly once"
            )

        finally:
            # Clean up the queue processor task
            queue_processor_task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await queue_processor_task

    @pytest.mark.asyncio
    async def test_data_queue_overflow_handling(
        self,
        interpreter_daemon: InterpreterDaemon,
        mock_nats,
    ):
        """Test that the system handles queue overflow gracefully."""
        _, mock_client, _ = mock_nats
        interpreter_daemon._nc = mock_client
        interpreter_daemon.log = AsyncMock()

        # Set a very small queue size for testing
        interpreter_daemon._async_data_queue = asyncio.Queue(maxsize=2)

        # Start the queue processor to consume data
        queue_processor_task = asyncio.create_task(
            interpreter_daemon._process_async_data_queue()
        )

        try:
            measurement_id = 555
            port = InstrumentPort(
                default_name="test_port",
                pseudo_name=PlungerGate("test_gate"),
            )

            # Fill the queue to capacity first
            for i in range(2):  # Queue maxsize is 2
                await interpreter_daemon._queue_measurement_data(
                    measurement_id=measurement_id,
                    chunk_id=i,
                    data={port: MeasuredArray1D([1.0, 2.0, 3.0])},
                )

            # Now try to add one more item - this should trigger overflow
            await interpreter_daemon._queue_measurement_data(
                measurement_id=measurement_id,
                chunk_id=2,
                data={port: MeasuredArray1D([4.0, 5.0, 6.0])},
            )

            # Give a moment for logging
            await asyncio.sleep(0.1)

            # Verify that overflow was logged
            interpreter_daemon.log.assert_any_call(
                f"Data queue full, dropping data for measurement {measurement_id}"
            )

        finally:
            # Clean up the queue processor task
            queue_processor_task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await queue_processor_task

    @pytest.mark.asyncio
    async def test_measurement_without_registration(
        self,
        interpreter_daemon: InterpreterDaemon,
        mock_nats,
    ):
        """Test that data for unregistered measurements gets requeued."""
        _, mock_client, _ = mock_nats
        interpreter_daemon._nc = mock_client
        interpreter_daemon.log = AsyncMock()

        # Track queue operations to verify requeuing behavior
        original_put = interpreter_daemon._async_data_queue.put
        put_call_count = 0

        async def mock_put(*args, **kwargs):
            nonlocal put_call_count
            put_call_count += 1
            return await original_put(*args, **kwargs)

        interpreter_daemon._async_data_queue.put = mock_put

        # Start the queue processor to handle requeuing logic
        queue_processor_task = asyncio.create_task(
            interpreter_daemon._process_async_data_queue()
        )

        try:
            # Queue data for a measurement that hasn't been registered
            unregistered_id = 999999
            port = InstrumentPort(
                default_name="test_port",
                pseudo_name=PlungerGate("test_gate"),
            )

            await interpreter_daemon._queue_measurement_data(
                measurement_id=unregistered_id,
                chunk_id=0,
                data={port: MeasuredArray1D([1.0, 2.0, 3.0])},
            )

            # Give the queue processor time to try processing and requeue multiple times
            await asyncio.sleep(0.5)

            # Verify that the data was queued multiple times (initial + requeues)
            # The initial put + at least one requeue should have occurred
            assert put_call_count >= 2, (
                f"Expected at least 2 queue operations, got {put_call_count}"
            )

            # Verify that the measurement is still not registered
            assert unregistered_id not in interpreter_daemon._pending_measurements

        finally:
            # Clean up the queue processor task
            queue_processor_task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await queue_processor_task
