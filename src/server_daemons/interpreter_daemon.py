"""A measurement interpreter for the instrument server."""

import time
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

from falcon_core.math.domains import Domain
from nats.js.api import RetentionPolicy, StorageType, StreamConfig

from .api.interpreter import RUNTIME_COMMANDS as INTERPRETER_RUNTIME_COMMANDS
from .dependancies import (
    SUPPORTED_PROPERTIES,
    HDF5Data,
    LabelledMeasuredArray,
    LabelledMeasuredArrays,
    MeasuredArray,
    MeasuredArray1D,
    MeasurementRequest,
    MeasurementResponse,
    Path,
    Time,
    asyncio,
    json,
    nats,
    np,
)
from .instructions import Instruction, MeasurementInstructions
from .typing import (
    InstrumentPort,
    Meter,
)

if TYPE_CHECKING:
    from falcon_core.math.domains import Domain
    from nats.aio.client import Client
    from nats.aio.msg import Msg
    from nats.js import JetStreamContext

    from .typing import (
        ID,
        BaseArray,
        BaseLabelledDomain,
        BaseWaveform,
        Getters,
        Knob,
        Meters,
        NDArray,
        PortTransform,
        PropertyJson,
        PropertyName,
        PropertyValue,
        Requirements,
        Sequence,
        Setters,
        array1D,
    )
TIMEOUT_SCALE_FACTOR = 1.5
DEFAULT_SLOPE = 100  # [V/sec]
DEFAULT_SAMPLE_RATE = 10_000  # [samples/sec]
MAX_NUM_DATA_POINTS = 10_000
STALE_MEASUREMENT_TIMEOUT = 3600  # [sec]
STALE_MEASUREMENT_CHECKUP = 60.0  # [sec]


@dataclass
class DataEntry:
    """Represents a single data entry for a measurement."""

    measurement_id: "ID"
    chunk_id: "ID"
    data: dict[Meter, MeasuredArray1D]
    timestamp: float


@dataclass
class PendingMeasurement:
    """Represents a measurement that is waiting for data collection to complete."""

    measurement_id: "ID"
    expected_count: int
    data_path: Path
    shape: tuple[int, ...]
    request: MeasurementRequest
    collected_data: list[DataEntry] = field(default_factory=list)
    created_at: int = Time().time

    @property
    def is_complete(self) -> bool:
        """Check if enough data has been collected."""
        return len(self.collected_data) >= self.expected_count

    @property
    def completion_percentage(self) -> float:
        """Get completion percentage."""
        return (
            (len(self.collected_data) / self.expected_count) * 100
            if self.expected_count > 0
            else 0
        )

    def add_data_entry(
        self,
        chunk_id: int,
        data: dict[Meter, MeasuredArray1D],
    ) -> None:
        """Add a data entry to this measurement."""
        entry = DataEntry(
            measurement_id=self.measurement_id,
            chunk_id=chunk_id,
            data=data,
            timestamp=time.time(),
        )
        self.collected_data.append(entry)

    def get_sorted_data(self) -> list[DataEntry]:
        """Get data entries sorted by chunk_id."""
        return sorted(self.collected_data, key=lambda x: x.chunk_id)

    def get_sorted_chunk_data(self) -> dict[int, dict[Meter, MeasuredArray1D]]:
        """Get data organized by chunk_id, preserving the chunk structure."""
        # Sort data by chunk_id first
        sorted_entries = self.get_sorted_data()

        # Organize data by chunk_id
        chunk_data = {}
        for entry in sorted_entries:
            chunk_id = entry.chunk_id
            if chunk_id not in chunk_data:
                chunk_data[chunk_id] = {}

            # Copy the InstrumentPort -> MeasuredArray1D mapping for this chunk
            for instrument_port, measured_array in entry.data.items():
                if isinstance(instrument_port, Meter) and isinstance(
                    measured_array, MeasuredArray1D
                ):
                    chunk_data[chunk_id][instrument_port] = measured_array

        return chunk_data


class InterpreterDaemon:
    """A daemon that processes messages for the measurement interpretter."""

    _url: str
    _nc: "Client"
    _js: "JetStreamContext"
    _loop: asyncio.AbstractEventLoop
    _measurement_groups: dict[
        "ID",
        "MeasurementInstructions",
    ]
    _debug: bool

    # Add async queue attributes with proper types
    _async_data_queue: asyncio.Queue[DataEntry]
    _pending_measurements: dict["ID", PendingMeasurement]
    _queue_processor_task: asyncio.Task | None

    def __init__(
        self,
        url: str,
        debug: bool = True,
    ):
        """Initializes the MeasurementInterpreter and stores communication information."""
        self._url = url
        self._debug = debug
        self._data_queue = {}
        self._measurement_groups = {}

        # Initialize async queue components with proper types
        self._async_data_queue = asyncio.Queue(maxsize=MAX_NUM_DATA_POINTS)
        self._pending_measurements = {}
        self._queue_processor_task = None

    @property
    def measurement_groups(self) -> dict["ID", "MeasurementInstructions"]:
        """Returns the measurement groups."""
        return self._measurement_groups

    async def start(self):
        """Starts the measurement interpreter."""
        self._nc = await nats.connect(self._url)
        await self.log(f"Connected to NATS server at {self._url}")
        self._loop = asyncio.get_running_loop()
        await self.setup_subscriptions()
        await self.setup_jetstream()
        self._loop.create_task(self.publish_status())

        # Start the async queue processor
        self._queue_processor_task = self._loop.create_task(
            self._process_async_data_queue()
        )
        self._loop.create_task(self._cleanup_stale_measurements())

        forever = asyncio.Future()
        try:
            # Wait forever or until the program is interrupted
            await forever
        except asyncio.CancelledError:
            # Handle graceful shutdown if the future is cancelled
            print("Interpreter lost connection, shutting down...")
        finally:
            # Cancel background tasks
            if self._queue_processor_task:
                self._queue_processor_task.cancel()
            # Properly drain the connection when exiting
            await self._nc.drain()

    async def setup_jetstream(self):
        """Set up JetStream stream for large data transfers."""
        print("Setting up Jetstream...", flush=self._debug)
        try:
            self._js = self._nc.jetstream()

            # Create or update stream
            stream_config = StreamConfig(
                name="MEASUREMENT_DATA",
                subjects=["measurement.data.>"],
                retention=RetentionPolicy.LIMITS,
                max_age=24 * 60 * 60,  # 24 hours in seconds
                max_msgs=10000,
                max_bytes=1024 * 1024 * 1024,  # 1GB
                storage=StorageType.FILE,
            )

            print("Created my stream-config", flush=self._debug)

            try:
                await self._js.add_stream(stream_config)
                print("Stream created successfully", flush=self._debug)
            except Exception as e:
                if "stream name already in use" in str(e).lower():
                    # Update existing stream
                    await self._js.update_stream(stream_config)
                else:
                    raise

        except Exception:
            raise

    async def publish_status(self, refresh: float = 0.5) -> None:
        """Publishes the status of the daemon every refresh."""
        while True:
            pending = len([t for t in asyncio.all_tasks() if not t.done()])
            message = json.dumps(
                {
                    INTERPRETER_RUNTIME_COMMANDS.STATUS.TIMESTAMP: Time().time,
                    INTERPRETER_RUNTIME_COMMANDS.STATUS.STATUS: pending > 1,
                }
            )
            await self.send_command(
                channel=INTERPRETER_RUNTIME_COMMANDS.STATUS.COMM_CHANNEL
                + ".interpreter",
                message=message,
            )
            await asyncio.sleep(refresh)

    async def send_command(
        self,
        channel: str,
        message: str,
    ) -> None:
        """Send a command string to a specific channel.

        Args:
            channel: The channel to send the command to.
            message: The message to send.
        """
        await self._nc.publish(channel, message.encode())

    async def log(
        self,
        message: str,
    ) -> None:
        """Log a message to the NATS server.

        Args:
            message: The message to log.
        """
        print(f"Logging message: {message}", flush=self._debug)
        message = json.dumps(
            {
                INTERPRETER_RUNTIME_COMMANDS.LOG.MESSAGE: message,
                INTERPRETER_RUNTIME_COMMANDS.LOG.TIMESTAMP: Time().time,
            }
        )
        await self.send_command(
            channel=INTERPRETER_RUNTIME_COMMANDS.LOG.COMM_CHANNEL + ".interpreter",
            message=message,
        )

    async def update_daemon_property(
        self,
        property: "PropertyName",
        name: "InstrumentPort",
        value: "PropertyValue",
    ) -> None:
        """Updates a property in a daemon.

        Args:
            property: The property to update.
            name: The name of the connection or instrument.
            value: The value to set.
        """
        message = json.dumps(
            {
                INTERPRETER_RUNTIME_COMMANDS.UPDATE_DAEMON_PROPERTY.PROPERTY: property,
                INTERPRETER_RUNTIME_COMMANDS.UPDATE_DAEMON_PROPERTY.NAME: name.to_json(),
                INTERPRETER_RUNTIME_COMMANDS.UPDATE_DAEMON_PROPERTY.VALUE: value,
            }
        )
        await self.send_command(
            channel=INTERPRETER_RUNTIME_COMMANDS.UPDATE_DAEMON_PROPERTY.COMM_CHANNEL,
            message=message,
        )

    async def deploy_measurement(
        self,
        id: "ID",
        getters: "Getters",
        setters: "Setters",
        requirements: "Requirements",
    ) -> None:
        """Deploys a measurement to the local runtime server.

        Args:
            id: The ID of the measurement.
            getters: The getters to deploy.
            setters: The setters to deploy.
        """
        message = json.dumps(
            {
                INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.PROCESS_ID: id,
                INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.GETTERS: [
                    getter.to_json() for getter in getters
                ],
                INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.SETTERS: [
                    setter.to_json() for setter in setters
                ],
                INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.REQUIREMENTS: [
                    json.dumps(
                        {
                            "setter": setter.to_json(),
                            "property": list(values.keys()),
                            "values": list(values.values()),
                        }
                    )
                    for setter, values in requirements.items()
                ],
            }
        )

        await self.send_command(
            channel=INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.COMM_CHANNEL,
            message=message,
        )

    async def upload_data(
        self,
        response: "MeasurementResponse",
        id: "ID",
    ) -> None:
        """Sends the measurement response to the server.

        Args:
            response: The measurement response to send.
            id: The ID of the measurement.
        """
        # TODO: Upload raw time trace too
        await self.log(f"Preparing to upload data for ProcessID: {id}")
        data_channel = f"measurement.data.{id}"
        message = json.dumps(
            {
                INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.DATA: response.to_json(),
                INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.PROCESS_ID: id,
                INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.TIMESTAMP: Time().time,
            }
        )
        await self._js.publish(
            data_channel,
            message.encode(),
            stream="MEASUREMENT_DATA",
        )
        data = json.dumps(
            {
                "data_channel": data_channel,
                "stream_name": "MEASUREMENT_DATA",
            }
        )
        notification = json.dumps(
            {
                INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.DATA: data,
                INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.TIMESTAMP: Time().time,
                INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.PROCESS_ID: id,
            }
        )

        await self.send_command(
            channel=INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.COMM_CHANNEL,
            message=notification,
        )

    async def setup_subscriptions(self):
        """Set up subscriptions for the daemon."""
        subscriptions: list[tuple[str, Any]] = [
            (
                INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.COMM_CHANNEL,
                self.handle_request,
            ),
            (
                INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.COMM_CHANNEL,
                self.handle_data,
            ),
        ]
        for channel, handle in subscriptions:
            await self._nc.subscribe(
                channel,
                cb=handle,
            )
            await self.log(f"Subscribed to channel: {channel}")

    async def handle_request(self, msg: "Msg") -> None:
        """Handle a PROCESS_REQUEST command."""
        try:
            data = json.loads(msg.data.decode())
            request = data.get(INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.REQUEST)
            id = int(data.get(INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.PROCESS_ID))
            configuration = data.get(
                INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.CONFIGURATIONS
            )
            data_path = Path(
                data.get(INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.DATA_PATH)
            )

            unpacked_configuration = json.loads(configuration)
            expanded_config = await self.readConfigurationPorts(unpacked_configuration)
            measurement_request = MeasurementRequest.from_json(request)

            await self.log("Measurement unpacked, processing ....")

            data_count, shape = await self.process_request(
                request=measurement_request,
                configuration=expanded_config,
                id=id,
            )

            await self.log("Request successfully processed and chunked...")

            # Register the measurement for async data collection
            await self._register_measurement(
                measurement_id=id,
                expected_count=data_count,
                data_path=data_path,
                shape=shape,
                request=measurement_request,
            )

            await self.deploy_measurements(measurement_id=id)
            await self.log("Measurement successfully deployed ....")

            # Log that we're waiting (but don't actually block)
            await self.log(
                f"Waiting for more data for id {id} (expected {data_count}, got 0)"
            )

        except Exception as e:
            await self.log(f"Error processing request: {e}")

    async def readConfigurationPorts(
        self,
        configuration: dict[str, dict["PropertyName", "PropertyJson"]],
    ) -> dict["InstrumentPort", dict["PropertyName", "PropertyJson"]]:
        """Returns unjsoned Instrumentport configurations."""
        return {
            InstrumentPort.from_json(key): value for key, value in configuration.items()
        }

    async def _register_measurement(
        self,
        measurement_id: "ID",
        expected_count: int,
        data_path: Path,
        shape: tuple[int, ...],
        request: MeasurementRequest,
    ) -> None:
        """Register a measurement for async data collection."""
        pending = PendingMeasurement(
            measurement_id=measurement_id,
            expected_count=expected_count,
            data_path=data_path,
            shape=shape,
            request=request,
        )

        self._pending_measurements[measurement_id] = pending
        await self.log(
            f"Registered measurement {measurement_id}, expecting {expected_count} chunks"
        )

    async def _queue_measurement_data(
        self,
        measurement_id: "ID",
        chunk_id: int,
        data: dict[Meter, MeasuredArray1D],
    ) -> None:
        """Queue measurement data for async processing."""
        entry = DataEntry(
            measurement_id=measurement_id,
            chunk_id=chunk_id,
            data=data,
            timestamp=Time().time,
        )

        try:
            # Use put_nowait to immediately detect if queue is full
            self._async_data_queue.put_nowait(entry)
        except asyncio.QueueFull:
            await self.log(
                f"Data queue full, dropping data for measurement {measurement_id}"
            )

    async def _process_async_data_queue(self):
        """Process data entries from the async queue."""
        while True:
            try:
                # Get data from queue (blocks until data is available)
                entry = await self._async_data_queue.get()

                measurement_id = entry.measurement_id
                chunk_id = entry.chunk_id
                data = entry.data

                # Check if we have a registered measurement for this ID
                if measurement_id not in self._pending_measurements:
                    await self.log(
                        f"Received data for unregistered measurement {measurement_id}, requeuing..."
                    )
                    # Requeue the data - it might arrive before the measurement registration
                    await asyncio.sleep(0.1)  # Small delay to prevent tight loop
                    await self._async_data_queue.put(entry)
                    continue

                pending = self._pending_measurements[measurement_id]

                # Check if this chunk ID is already collected
                existing_chunk_ids = [
                    entry.chunk_id for entry in pending.collected_data
                ]
                if chunk_id in existing_chunk_ids:
                    await self.log(
                        f"Received duplicate data for measurement {measurement_id}, chunk {chunk_id}, throwing it away"
                    )
                    continue

                # Add the data to the pending measurement
                pending.add_data_entry(chunk_id, data)

                await self.log(
                    f"Collected data {len(pending.collected_data)}/{pending.expected_count} for measurement {measurement_id} ({pending.completion_percentage:.1f}%)"
                )

                # Check if measurement is complete
                if pending.is_complete:
                    await self.log(
                        f"Measurement {measurement_id} complete, processing..."
                    )

                    # Get sorted chunk data and remove from pending
                    chunk_data = pending.get_sorted_chunk_data()
                    del self._pending_measurements[measurement_id]

                    # Process the complete measurement
                    await self.load_and_export_data(
                        request=pending.request,
                        data_path=pending.data_path,
                        shape=pending.shape,
                        id=measurement_id,
                        data_count=pending.expected_count,
                        chunk_data=chunk_data,
                    )

            except Exception as e:
                await self.log(f"Error processing data from queue: {e}")
                await asyncio.sleep(0.1)  # Prevent tight error loop

    async def _cleanup_stale_measurements(self):
        """Background task to clean up stale measurements."""
        while True:
            try:
                await asyncio.sleep(STALE_MEASUREMENT_CHECKUP)

                current_time = time.time()

                stale_ids = []
                for measurement_id, pending in self._pending_measurements.items():
                    if current_time - pending.created_at > STALE_MEASUREMENT_TIMEOUT:
                        await self.log(
                            f"Warning: Measurement {measurement_id} timed out with {len(pending.collected_data)}/{pending.expected_count} data points ({pending.completion_percentage:.1f}%)"
                        )
                        stale_ids.append(measurement_id)

                for measurement_id in stale_ids:
                    del self._pending_measurements[measurement_id]

            except Exception as e:
                await self.log(f"Error in cleanup task: {e}")

    def find_matching_port(
        self,
        configuration: dict["InstrumentPort", dict["PropertyName", "PropertyJson"]],
        default_name: str,
        property: "PropertyName",
    ) -> "InstrumentPort | None":
        """Processes the configuration and the search parameters and tries to find the matching port.

        Args:
            configuration: The configuration to search in.
            default_name: The defualt name to search for in the configuration
            property: The property of the port we are searching for.

        Returns:
            The port if found, else None.
        """
        for key, values in configuration.items():
            if key.default_name == default_name and property in values:
                return key
        return None

    def parse_default_name(self, name: str) -> list[str]:
        """Returns the defualt name broken doen into parts."""
        name_parts = name.split("_##_")
        assert len(name_parts) == 3, (
            f"The formatting of the default name {name} is incorrect, expected 3 parts."
        )
        return name_parts

    def find_similar_port(
        self,
        similar_port_name: str,
        configuration: dict["InstrumentPort", dict["PropertyName", "PropertyJson"]],
    ) -> InstrumentPort:
        """Returns a similar port to the selected one in the configuration."""
        name_parts = self.parse_default_name(similar_port_name)
        default_name = (
            name_parts[0]
            + "_##_"
            + SUPPORTED_PROPERTIES.NUMBER_OF_SAMPLES
            + "_##_"
            + name_parts[2]
        )
        port = self.find_matching_port(
            configuration=configuration,
            default_name=default_name,
            property=SUPPORTED_PROPERTIES.NUMBER_OF_SAMPLES,
        )
        assert port is not None, (
            f"Failed to find a matching port in the configuration. Search for {default_name} and property {SUPPORTED_PROPERTIES.NUMBER_OF_SAMPLES}"
        )
        return port

    async def process_request(
        self,
        request: "MeasurementRequest",
        configuration: dict["InstrumentPort", dict["PropertyName", "PropertyJson"]],
        id: "ID",
    ) -> tuple[int, tuple[int, ...]]:
        """Processes an incoming request, breaks it down into pieces, and stores the results into measurement groups.

        Allows for multiple measurements to be processed at once.

        Args:
            request: The measurement request to process.
            configuration: The configuration for the instrument setup.
            id: The ID of the measurement.

        Returns:
            the number of unique collected measurments
            the end shape of the compiled data.

        Raises:
            RuntimeError: If no valid waveform is found in the request.
        """
        # TODO: add in knob_transforms parsing, this only supports cartesian type waveforms
        # TODO: support leakage matrix style discrete measurements
        await self.log("Compiling waveform ...")
        [waveform._space._space.compile() for waveform in request.waveforms]
        await self.log("Waveform compiled successfully.")

        valid_waveform = next(
            (
                waveform
                for waveform in request.waveforms
                if waveform._space._space._space.shape[1]
                == waveform._space._axes.dimension
            ),
            None,
        )
        if valid_waveform is None:
            msg = "No valid waveform found."
            await self.log(msg)
            raise RuntimeError(msg)

        # Prioritize buffered whenever possible
        buffered = await self.decide_buffered(
            getters=request.getters,
            valid_waveform=valid_waveform,
            configuration=configuration,
        )
        raw_time_trace = valid_waveform._space._space._space
        unit_domain = valid_waveform._space._space.domain
        axes_domains = valid_waveform._space._axes
        instructions = []
        await self.log("Chunking instructions ...")
        chunks = self.chunk_instructions(
            raw_time_trace=raw_time_trace,
            buffered=buffered,
        )
        await self.log("Chunks completed")
        await self.log(f"The chunks are: {chunks}")
        getters = request.getters
        await self.log("Selected getters for the measurement.")
        sample_rates = self.collect_sample_rates(
            configuration=configuration,
            getters=getters.ports,
        )
        step_width = self.collect_step_width(request=request)
        number_of_samples = self.calculate_number_of_samples_per_step(
            step_width=step_width,
            sample_rates=sample_rates,
        )
        for count, chunk in enumerate(chunks):
            instruction = Instruction(
                getters=getters.ports,
                buffered=buffered,
            )
            for meter in getters:
                properties = self.meter_property_generation(
                    step_width=step_width,
                    number_of_samples=number_of_samples[meter],
                    buffered=buffered,
                    num_x_points=len(chunk[:, 0]),
                )
                port = self.find_similar_port(
                    similar_port_name=meter.default_name,
                    configuration=configuration,
                )
                instruction.add_requirement(
                    instrument=port,
                    properties=properties,
                )
            for i, couple_domain in enumerate(axes_domains):
                buffered_dimension = buffered and (i == 0)
                raw_space = np.array(chunk[:, i])
                for domain in couple_domain:
                    properties = self.knob_property_generation(
                        unit_domain=unit_domain,
                        raw_space=raw_space,
                        domain=domain,
                        step_width=step_width,
                        buffered_dimension=buffered_dimension,
                    )
                    instruction.add_requirement(
                        instrument=domain.label,
                        properties=properties,
                    )
                    instruction.add_setter(domain.label)
            instructions.append(instruction)
        await self.log(f"All {len(chunks)} chunks were added")

        collected_measurements = len(instructions)
        if buffered:
            # inject ramps in between each instruction
            instructions = await self.interject_ramps(
                instructions=instructions,
                configuration=configuration,
            )

        self._measurement_groups[id] = MeasurementInstructions(
            instructions=instructions
        )
        shape = valid_waveform._space._space.shape
        assert isinstance(shape, tuple), "Invalid shape for waveform data."
        return (collected_measurements, shape)

    async def decide_buffered(
        self,
        valid_waveform: "BaseWaveform",
        configuration: dict[InstrumentPort, dict["PropertyName", "PropertyJson"]],
        getters: "Meters",
    ) -> bool:
        """Returns a flag indicating if a buffered measurement was selected or not to be performed."""
        ports = {
            **{
                knob: SUPPORTED_PROPERTIES.VOLTAGE_STATE
                for domain in valid_waveform._space._axes
                for knob in domain.knobs
            },
            **{meter: SUPPORTED_PROPERTIES.CURRENT_STATE for meter in getters},
        }
        buffered = all(
            [
                configuration[port][property].get(
                    SUPPORTED_PROPERTIES.SUPPORTS_BUFFERED_MEASUREMENTS, False
                )
                for port, property in ports.items()
            ]
        )

        await self.log(f"The configuration of the instrument server is {configuration}")
        if buffered:
            await self.log("Buffered measurements enabled.")
        else:
            await self.log("Standard measurement selected.")
        return buffered

    def meter_property_generation(
        self,
        step_width: float,
        number_of_samples: int,
        buffered: bool = False,
        num_x_points: int = 1,
    ) -> dict["PropertyName", "PropertyValue"]:
        """Generates the properties for a meter based on the step width and number of samples.

        Args:
            step_width: The width of each step in milliseconds.
            number_of_samples: The number of samples to take.
            buffered: Whether the measurement is buffered or not.
            num_x_points: The number of x points in the measurement.

        Returns:
            the properties for the meter.
        """
        if not buffered:
            properties = {
                SUPPORTED_PROPERTIES.TIMEOUT: TIMEOUT_SCALE_FACTOR
                * step_width
                / 1000,  # [sec]
                SUPPORTED_PROPERTIES.NUMBER_OF_SAMPLES: number_of_samples,
            }
        else:
            properties = {
                SUPPORTED_PROPERTIES.TIMEOUT: (TIMEOUT_SCALE_FACTOR - 1 + num_x_points)
                * step_width
                / 1000,  # [sec]
                SUPPORTED_PROPERTIES.NUMBER_OF_SAMPLES: int(
                    number_of_samples * num_x_points
                ),
            }
        return properties

    def knob_property_generation(
        self,
        unit_domain: "Domain",
        raw_space: "array1D",
        domain: "BaseLabelledDomain",
        step_width: float,
        buffered_dimension: bool,
    ) -> dict["PropertyName", "PropertyValue"]:
        """Generates the properties for a knob in the measurement.

        Args:
            unit_domain: The unit domain to transform the values.
            raw_space: The array "time-trace" to pull times from
            domain: The combination of the label and voltage bounds for this knob.
            step_width: The time width of this step in msec.
            buffered_dimension: A flag indicating if this is a legal buffered dimension.

        Returns:
            The properties collected for this knob.
        """
        v_start = unit_domain.transform(
            value=raw_space[0],
            other=domain.domain,
        )
        properties: dict[PropertyName, PropertyValue] = {}
        if not buffered_dimension:
            properties = {
                SUPPORTED_PROPERTIES.VOLTAGE_STATE: v_start,
                SUPPORTED_PROPERTIES.TIMEOUT: TIMEOUT_SCALE_FACTOR
                * step_width
                / 1000,  # [sec]
            }
        else:
            v_stop = unit_domain.transform(
                value=raw_space[-1],
                other=domain.domain,
            )
            properties = {
                SUPPORTED_PROPERTIES.STAIRCASE: (
                    step_width,  # [msec]
                    len(raw_space),
                    0,
                    v_start,
                    v_stop,
                ),
                SUPPORTED_PROPERTIES.TIMEOUT: (
                    TIMEOUT_SCALE_FACTOR * step_width / 1000  # [sec]
                ),
            }
        return properties

    def collect_sample_rates(
        self,
        configuration: dict[InstrumentPort, dict["PropertyName", "PropertyJson"]],
        getters: "Sequence[InstrumentPort]",
    ) -> dict[InstrumentPort, int]:
        """Colleects and enforces that the samples rates must be integer samples per second."""
        outs = {}
        for meter in getters:
            sample_rate = configuration[meter].get(
                SUPPORTED_PROPERTIES.SAMPLE_RATE, DEFAULT_SAMPLE_RATE
            )
            assert isinstance(sample_rate, int), (
                f"Sample rate {sample_rate} must be an integer."
            )
            assert sample_rate > 0, f"Sample rate {sample_rate} must be greater than 0."
            assert sample_rate % 1000 == 0, (
                f"Sample rate {sample_rate} must be divisible by 1000."
            )
            outs[meter] = sample_rate
        return outs

    def collect_step_width(self, request: MeasurementRequest) -> int:
        """Returns the nearest msecond equivalent of the step width."""
        return int(np.ceil(request.time_domain.domain.range * 1000))  # [msec]

    def calculate_number_of_samples_per_step(
        self,
        step_width: int,
        sample_rates: dict[InstrumentPort, int],
    ) -> dict[InstrumentPort, int]:
        """Produces the number of samples per each step of the measurement.

        Args:
            step_width: the width of each step in milliseconds
            sample_rates: the various sample rates of the instruments in samples per second. Must be divisible by 1000.

        Returns:
            the number of samples for each meter
        """
        # TODO: unlock this from only multiples of 1000
        return {
            meter: int(np.ceil(step_width * sample_rate / 1000))
            for meter, sample_rate in sample_rates.items()
        }

    def chunk_instructions(
        self,
        raw_time_trace: "BaseArray",
        buffered: bool = False,
    ) -> list["NDArray[np.floating]"]:
        """Chunks the raw time trace data into staircased segments.

        Args:
            raw_time_trace: The raw time trace data to chunk.
            buffered: Whether the data is buffered or not.

        Returns:
            the chunks of the raw time trace data, where each chunk is a staircased segment.

        Raises:
            ValueError: If the data is not staircased correctly.
        """
        if not buffered:
            # treat each row as a chunk of shape (1, n_axes)
            data = raw_time_trace.data
            return [data[i : i + 1, :] for i in range(data.shape[0])]
        # Find chunk boundaries where the primary axis stops staircasing
        primary_axis = raw_time_trace.data[:, 0]
        dominate_polarity = np.sign(np.mean(np.sign(np.diff(primary_axis))))
        breaks = np.where(np.sign(np.diff(primary_axis)) != dominate_polarity)[0] + 1
        chunks = np.split(raw_time_trace.data, breaks, axis=0)
        # in a staircase, all the values on the other axes are the same
        # Check that within each chunk, each non-time axis is constant (column-wise)
        for chunk in chunks:
            if chunk.shape[0] == 0:
                continue
            other_axes = chunk[:, 1:]
            # For each column, all values must be the same
            if not bool(
                np.all(np.array([np.all(col == col[0]) for col in other_axes.T]))
            ):
                msg = "Within a chunk, each non-time axis must be constant."
                raise ValueError(msg)
            first_row = chunk[:, 0]
            assert np.all(np.sign(np.diff(first_row)) == dominate_polarity), (
                "Chunks must all have the same polarity."
            )
        return chunks

    async def interject_ramps(
        self,
        instructions: list[Instruction],
        configuration: dict[
            "InstrumentPort", dict["PropertyName", "PropertyJson"]
        ] = {},
    ) -> list[Instruction]:
        """Interjects ramps between each instruction.

        Args:
            instructions: The list of instructions to interject ramps into.
            configuration: The configuration for the instruments.

        Returns:
            the list of instructions with ramps interjected.
        """
        new_instructions = []
        await self.log("Interjecting ramps between instructions ...")

        for i, instruction in enumerate(instructions):
            if i == 0:
                new_instructions.append(instruction)
                continue
            requirements: dict[InstrumentPort, dict[str, Any]] = {}
            for knob, properties in instruction.requirements.items():
                slope = configuration.get(knob, DEFAULT_SLOPE)
                staircase = properties[SUPPORTED_PROPERTIES.STAIRCASE]
                assert isinstance(staircase, tuple), (
                    "The staircase property must be a tuple."
                )
                assert isinstance(slope, (int, float)), "The slope must be a number."
                vstart = staircase[3]
                assert isinstance(vstart, (int, float)), "The vstart must be a number."
                vstop = staircase[4]
                assert isinstance(vstop, (int, float)), "The vstop must be a number."
                timeout = abs(vstop - vstart) / slope  # [sec]
                requirements[knob] = {
                    SUPPORTED_PROPERTIES.VOLTAGE_STATE: properties[  # type: ignore[reportOptionalMemberAccess]
                        SUPPORTED_PROPERTIES.STAIRCASE
                    ][3],
                    SUPPORTED_PROPERTIES.TIMEOUT: TIMEOUT_SCALE_FACTOR * timeout,
                }
            new_instruction = Instruction(
                requirements=requirements,
                setters=list(requirements.keys()),  # type: ignore[]
                buffered=True,
            )
            new_instructions.append(new_instruction)
            new_instructions.append(instruction)
        await self.log("Ramps interjected successfully.")
        return new_instructions

    async def deploy_measurements(
        self,
        measurement_id: "ID",
    ) -> None:
        """Deploys all steps for a given measurement_id.

        For each step:
        - Sets all properties for each connection using UPDATE_DAEMON_PROPERTY.
        - After all properties are set, sends MEASUREMENT_READY with all getters.

        Args:
            measurement_id: The ID of the measurement to deploy.
        """
        if measurement_id not in self.measurement_groups:
            return
        for i, step in enumerate(self.measurement_groups[measurement_id]):
            await self.log(
                f"Step {i + 1} of {len(self.measurement_groups[measurement_id])} deploying for measurement {measurement_id}."
            )
            await self.deploy_measurement(
                id=measurement_id,
                requirements=step.requirements,
                getters=step.getters,
                setters=step.setters,
            )
            await asyncio.sleep(0.05)  # Allow some time for the deployment to settle

    async def handle_data(
        self,
        msg: "Msg",
    ) -> None:
        """Handle a PROCESS_DATA command."""
        try:
            data = json.loads(msg.data.decode())
            raw_data_payload = data.get(INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.DATA)
            id = int(data.get(INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.PROCESS_ID))
            # Extract chunk_id if available, default to 0
            chunk_id = data.get(INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.CHUNK_ID, 0)

            # Parse the data
            data_payload = json.loads(raw_data_payload)
            modified_payload = {}
            for key, value in data_payload.items():
                modified_payload[Meter.from_json(key)] = MeasuredArray1D(
                    np.array(json.loads(value))
                )

            # Queue the data asynchronously
            await self._queue_measurement_data(id, chunk_id, modified_payload)

        except Exception as e:
            await self.log(f"Error queueing data: {e}")

    async def load_and_export_data(
        self,
        request: "MeasurementRequest",
        shape: tuple[int, ...],
        data_path: Path,
        id: "ID",
        data_count: int,
        chunk_data: dict["ID", dict[Meter, MeasuredArray1D]],
    ) -> None:
        """Load and export data to a file. Finishes by sending the completed result away.

        This method works with the raw timetrace data to process it into the form requested, following the instructions.

        Args:
            request: The measurement request.
            shape: The shape of the data.
            data_path: The path to the data file.
            id: The ID of the measurement.
            data_count: The number of data points to load.
            collected_data: Pre collected data from the async processor.
        """
        number_of_bins = self.get_data_point_counter_per_queue(
            shape=shape,
            data_count=data_count,
        )
        name_attribute_maps = await self.preprocess_voltage_states(id=id)
        await self.log(f"The number of bins {number_of_bins}")
        aligned_sub_chunks = self.divide_to_sub_chunks(
            chunk_data=chunk_data,
            number_of_bins=number_of_bins,
        )
        await self.log(f"The aligned sub chunks are {aligned_sub_chunks}")
        final_data = await self.average_shapeless_data(
            request=request,
            voltage_state_array=name_attribute_maps,
            aligned_sub_chunks=aligned_sub_chunks,
        )
        # makes sense to here
        response = self.make_response(
            data_arrays=final_data,
            shape=shape,
        )
        await self.log(f"The response {response}")
        await self.log(f"The response final data is {response.arrays.arrays}")
        await self.log(f"Finishing making a response for ProcessId {id}")
        self.store_in_database(
            response=response,
            request=request,
            id=id,
            data_path=data_path,
        )
        await self.upload_data(response=response, id=id)

    def store_in_database(
        self,
        response: "MeasurementResponse",
        request: MeasurementRequest,
        id: "ID",
        data_path: Path,
    ) -> None:
        """Stores the measurement response in a database.

        Args:
            response: The measurement response to store.
            request: The measurement request that caused the response.
            id: The ID of the measurement.
            data_path: The path to where the database will store the file.
        """
        hdf = HDF5Data.from_communications(
            request=request,
            response=response,
            unique_id=id,
            timestamp=Time().time,
            measurement_title=request.measurement_name,
        )
        hdf.to_file(path=data_path)

    def make_response(
        self,
        data_arrays: dict["InstrumentPort", list[float]],
        shape: tuple[int, ...],
    ) -> MeasurementResponse:
        """Creates a MeasurementResponse object from the data arrays.

        Args:
            data_arrays: The data arrays to include in the response.
            shape: The shape of the data arrays.

        Returns:
            the built MeasurementResponse object.
        """
        # convert to numpy arrays
        final_arrays: dict[InstrumentPort, np.ndarray] = {}
        for port, array in data_arrays.items():
            final_arrays[port] = np.array(array).reshape(shape)

        return MeasurementResponse(
            arrays=LabelledMeasuredArrays(
                [
                    LabelledMeasuredArray.from_port(
                        port=port,
                        array=MeasuredArray(array),
                    )
                    for port, array in final_arrays.items()
                ]
            )
        )

    @staticmethod
    def divide_to_sub_chunks(
        chunk_data: dict["ID", dict["Meter", MeasuredArray1D]],
        number_of_bins: int,
    ) -> list[dict["Meter", "array1D"]]:
        """Divides each chunk of data into sub-chunks.

        In the case of a standard measurement, this will do nothing.

        Returns:
            an ordered list of the measured data for each sub_chunk.
        """
        outs: list[dict[Meter, array1D]] = []
        # Process chunks in sorted order by ID
        for chunk_id in sorted(chunk_data.keys()):
            collected_data = chunk_data[chunk_id]
            divided_chunks: dict[Meter, tuple[array1D, ...]] = {}
            for port, individual_data in collected_data.items():
                divided_chunks[port] = individual_data.even_divisions(
                    divisions=number_of_bins
                )

            # Unravel the divided chunks and append them to outs
            for i in range(number_of_bins):
                sub_chunk: dict[Meter, array1D] = {}
                for port, divisions in divided_chunks.items():
                    sub_chunk[port] = divisions[i]
                outs.append(sub_chunk)

        return outs

    async def average_shapeless_data(
        self,
        request: MeasurementRequest,
        voltage_state_array: list[dict["Knob", float]],
        aligned_sub_chunks: list[dict["Meter", "array1D"]],
    ) -> dict[InstrumentPort, list[float]]:
        """Computes the average over the data and stores it in a 1D array to be reshaped later.

        Args:
            id: The ID of the measurement.
            number_of_bins: The number of bins to average over.
            request: The measurement request.
            voltage_state_array: The different voltage states to use when computing the average.

        Returns:
            the computed average data.
        """
        final_data: dict[InstrumentPort, list[float]] = {}
        time_bounds = request.time_domain.domain.bounds
        await self.log(f"The time bounds are {time_bounds}")
        num_states = len(voltage_state_array)
        num_sub_chunks = len(aligned_sub_chunks)
        assert num_states == num_sub_chunks, (
            f"The number of voltage states must match the number of end measurement points, {num_states} != {num_sub_chunks}"
        )

        # Precompute all transforms for all possible ports in sub_chunks
        all_ports: set[Meter] = set()
        for sub_chunk in aligned_sub_chunks:
            all_ports.update(sub_chunk.keys())

        port_transforms: dict[InstrumentPort, PortTransform] = {}
        for port in all_ports:
            for indexport, transform in request.meter_transforms.items():
                if indexport.default_name == port.default_name:
                    port_transforms[port] = transform
                    break
            else:
                await self.log(
                    f"The type of the port we are searching for is {type(port)}"
                )
                msg = f"Transform not found for port {port} in the available ports {request.meter_transforms.keys()}"
                raise ValueError(msg)

        data_length = self.sub_chunk_length(aligned_sub_chunks=aligned_sub_chunks)

        # Generate time array once since all data has the same length
        assert data_length is not None, "No data found in sub_chunks"
        t_array = self.generate_time_array(
            time_bounds=time_bounds,
            num_points=data_length,
        )

        voltage_state_keys = {
            name: name.instrument_facing_name() for name in voltage_state_array[0]
        }
        for sub_chunk, voltage_states in zip(aligned_sub_chunks, voltage_state_array):
            voltage_state_dict = {
                voltage_state_keys[name]: potential
                for name, potential in voltage_states.items()
            }
            for port, data in sub_chunk.items():
                asyncio.create_task(
                    self.log(
                        f"The instrument facing name for the port is {port.instrument_facing_name()} and the voltage states are {voltage_states}"
                    )
                )

                transform_func = port_transforms[port].transform
                vectorized_func = np.frompyfunc(
                    lambda t: transform_func(t=t, **voltage_state_dict), 1, 1
                )
                transformed = vectorized_func(t_array).astype(np.float64)

                # Use numpy boolean indexing efficiently
                nonzero_indices = transformed != 0
                if np.any(nonzero_indices):
                    computation = np.mean((transformed * data)[nonzero_indices])
                else:
                    computation = 0.0

                if port not in final_data:
                    final_data[port] = []
                final_data[port].append(float(computation))

        await self.log("Final data successfully averaged")

        return final_data

    @staticmethod
    def sub_chunk_length(
        aligned_sub_chunks: list[dict["Meter", "array1D"]],
    ) -> int:
        """Returns the length of the sub-chunks and enforces consistent length."""
        data_length = None
        for sub_chunk in aligned_sub_chunks:
            for port, data in sub_chunk.items():
                if data_length is None:
                    data_length = len(data)
                else:
                    assert len(data) == data_length, (
                        f"All data arrays must have the same length. Expected {data_length}, got {len(data)} for port {port}"
                    )
        assert isinstance(data_length, int), (
            "No sub_chunks were available to compare against"
        )
        return data_length

    @staticmethod
    def generate_time_array(
        time_bounds: tuple[float, float],
        num_points: int,
    ) -> np.ndarray:
        """Returns a time array based on the bounds and the number of points."""
        return np.linspace(
            start=time_bounds[0],
            stop=time_bounds[1],
            num=num_points,
        )

    async def preprocess_voltage_states(self, id: "ID") -> list[dict["Knob", float]]:
        """Preprocesses the voltage states for the measurement.

        Modifies the setup stored in teh measurement_groups.

        Args:
            id: The ID of the measurement.

        Returns:
            the preprocessed voltage states.
        """
        name_attribute_maps: list[dict[Knob, float]] = []
        await self.log(
            f"Beginning to preprocess voltage states for measurement id {id}"
        )
        for instr in self.measurement_groups[id]:
            if not instr.getters:
                continue
            buffered_maps = []
            standard_set_map = instr.retrieve_voltage_states()
            if num_divisions := instr.contains_buffered_measurement():
                buffered_maps = instr.retrieve_buffered_voltage_states(num_divisions)
            if buffered_maps:
                for map in buffered_maps:
                    total = {**standard_set_map, **map}
                    name_attribute_maps.append(total)
            else:
                name_attribute_maps.append(standard_set_map)
        return name_attribute_maps

    def get_data_point_counter_per_queue(
        self,
        shape: tuple[int, ...],
        data_count: int,
    ) -> int:
        """Calculates the number of data points per queue.

        Args:
            shape: The shape of the data.
            data_count: The number of data queues collected.

        Returns:
            the number of data points per queue.
        """
        product = 1
        for dim in shape:
            product *= dim
        expected_data_points_per_queue = product / data_count
        assert product % data_count == 0, (
            f"Uneven division {expected_data_points_per_queue}, not sure how many data points to expect"
        )
        return int(expected_data_points_per_queue)
