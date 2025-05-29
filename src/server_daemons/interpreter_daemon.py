"""A measurement interpreter for the instrument server."""

from typing import TYPE_CHECKING

from .api.interpreter import RUNTIME_COMMANDS as INTERPRETER_RUNTIME_COMMANDS
from .data_queue import DataEntry, DataQueue
from .dependancies import (
    SUPPORTED_PROPERTIES,
    HDF5Data,
    LabelledMeasuredArray,
    LabelledMeasuredArrays,
    MeasuredArray,
    MeasurementRequest,
    MeasurementResponse,
    Path,
    asyncio,
    json,
    nats,
    np,
    time,
)
from .instructions import Instruction, MeasurementInstructions

if TYPE_CHECKING:
    from .typing import (
        ID,
        Any,
        BaseArray,
        Client,
        Getters,
        InstrumentPort,
        Msg,
        NDArray,
        PropertyJson,
        PropertyName,
        PropertyValue,
        Setters,
    )


class InterpreterDaemon:
    """A daemon that processes messages for the measurement interpretter."""

    _url: str
    _nc: "Client"
    _loop: asyncio.AbstractEventLoop
    _data_queue: dict["ID", "DataQueue"]
    _measurement_groups: dict[
        "ID",
        "MeasurementInstructions",
    ]

    def __init__(
        self,
        url: str,
        loop: asyncio.AbstractEventLoop,
    ):
        """Initializes the MeasurementInterpreter and stores communication information."""
        self._url = url
        self._loop = loop
        self._data_queue = {}
        self._measurement_groups = {}

    @property
    def data_queue(self) -> dict["ID", "DataQueue"]:
        """Gets the data from the queue."""
        return self._data_queue

    @property
    def measurement_groups(self) -> dict["ID", "MeasurementInstructions"]:
        """Returns the measurement groups."""
        return self._measurement_groups

    async def start(self):
        """Starts the measurement interpreter."""
        self._nc = await nats.connect(self._url)
        await self.setup_subscriptions()
        self._loop.create_task(self.publish_status())

        forever = asyncio.Future()
        try:
            # Wait forever or until the program is interrupted
            await forever
        except asyncio.CancelledError:
            # Handle graceful shutdown if the future is cancelled
            print("Interpreter lost connection, shutting down...")
        finally:
            # Properly drain the connection when exiting
            await self._nc.drain()

    async def publish_status(self, refresh: float = 0.5) -> None:
        """Publishes the status of the daemon every refresh."""
        while True:
            pending = len([t for t in asyncio.all_tasks() if not t.done()])
            message = json.dumps(
                {
                    INTERPRETER_RUNTIME_COMMANDS.STATUS.TIMESTAMP: str(time.time()),
                    INTERPRETER_RUNTIME_COMMANDS.STATUS.STATUS: pending > 1,
                }
            )
            await self.send_command(
                channel=INTERPRETER_RUNTIME_COMMANDS.STATUS.COMM_CHANNEL,
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
        message = json.dumps(
            {
                INTERPRETER_RUNTIME_COMMANDS.LOG.MESSAGE: message,
                INTERPRETER_RUNTIME_COMMANDS.LOG.TIMESTAMP: str(time.time()),
            }
        )
        await self.send_command(
            channel=INTERPRETER_RUNTIME_COMMANDS.LOG.COMM_CHANNEL,
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
                INTERPRETER_RUNTIME_COMMANDS.UPDATE_DAEMON_PROPERTY.NAME: name,
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
        setters: "Setters" = {},
    ) -> None:
        """Deploys a measurement to the local runtime server.

        Setters are only required if the measurment is buffered and they need to be triggered

        Args:
            id: The ID of the measurement.
            getters: The getters to deploy.
            setters: The setters to deploy (optional, defaults to empty).
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
            }
        )

        await self.send_command(
            channel=INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.COMM_CHANNEL,
            message=message,
        )

    async def upload_data(
        self,
        response: "MeasurementResponse",
    ) -> None:
        """Sends the measurement response to the server.

        Args:
            response: The measurement response to send.
        """
        message = json.dumps(
            {
                INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.DATA: response,
                INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.TIMESTAMP: str(time.time()),
            }
        )
        await self.send_command(
            channel=INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.COMM_CHANNEL,
            message=message,
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

    async def handle_request(
        self,
        msg: "Msg",
    ) -> None:
        """Handle a PROCESS_REQUEST command.

        Args:
            msg: The NATS message.
        """
        try:
            data = json.loads(msg.data.decode())
            request = data.get(INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.REQUEST)
            id = data.get(INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.PROCESS_ID)
            configuration = data.get(
                INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.CONFIGURATIONS
            )
            data_path = Path(
                data.get(INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.DATA_PATH)
            )
            unpacked_configuration = json.loads(configuration)
            assert isinstance(unpacked_configuration, dict)
            measurement_request = MeasurementRequest.from_json(request)
            await self.log("Measurement unpacked, processing ....")
            data_count, shape = self.process_request(
                request=measurement_request,
                configuration=unpacked_configuration,
                id=id,
            )
            await self.deploy_measurements(measurement_id=id)
            await self.log("Measurement successfully deployed ....")
            await self.load_and_export_data(
                request=measurement_request,
                data_path=data_path,
                shape=shape,
                id=id,
                data_count=data_count,
            )

        except Exception as e:
            await self.log(f"Error processing request: {e}")

    def process_request(
        self,
        request: "MeasurementRequest",
        configuration: dict["InstrumentPort", "PropertyJson"],
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
        [waveform._space._space.compile() for waveform in request.waveforms]

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
            raise RuntimeError(msg)

        # Prioritize buffered whenever possible
        buffered = all(
            [
                configuration[knob].get(
                    SUPPORTED_PROPERTIES.SUPPORTS_BUFFERED_MEASUREMENTS, False
                )
                for domain in valid_waveform._space._axes
                for knob in domain.knobs
            ]
        )
        raw_time_trace = valid_waveform._space._space._space
        unit_domain = valid_waveform._space._space.domain
        axes_domains = valid_waveform._space._axes
        instructions = []
        chunks = self.chunk_instructions(
            raw_time_trace=raw_time_trace,
            buffered=buffered,
        )
        getters = [transform.port for transform in request.meter_transforms]

        for chunk in chunks:
            instruction = Instruction(
                getters=getters,
                buffered=buffered,
            )
            for i, couple_domain in enumerate(axes_domains):
                raw_space = chunk[i, :]
                for domain in couple_domain:
                    v_start = unit_domain.transform(
                        value=raw_space[0],
                        other=domain.domain,
                    )
                    if not buffered or i != 0:
                        instruction.add_setter(
                            instrument=domain.label,
                            properties={SUPPORTED_PROPERTIES.VOLTAGE_STATE: v_start},
                        )
                        continue
                    v_stop = unit_domain.transform(
                        value=raw_space[-1],
                        other=domain.domain,
                    )
                    instruction.add_setter(
                        instrument=domain.label,
                        properties={
                            SUPPORTED_PROPERTIES.STAIRCASE: (
                                request.time_domain.domain.range * 1000,  # [msec]
                                len(raw_space),
                                0,
                                v_start,
                                v_stop,
                            )
                        },
                    )
            instructions.append(instruction)

        collected_measurements = len(instructions)
        if buffered:
            # inject ramps in between each instruction
            instructions = self.interject_ramps(instructions=instructions)

        self._measurement_groups[id] = MeasurementInstructions(
            instructions=instructions
        )
        shape = valid_waveform._space._space.shape
        assert isinstance(shape, tuple), "Invalid shape for waveform data."
        return (collected_measurements, shape)

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
            # treat each column as a chunk of shape (n_axes, 1)
            data = raw_time_trace.data
            return [data[:, i : i + 1] for i in range(data.shape[1])]
        # Find chunk boundaries where the primary axis stops staircasing
        primary_axis = raw_time_trace.data[0, :]
        dominate_polarity = np.sign(np.mean(np.sign(np.diff(primary_axis))))
        breaks = np.where(np.sign(np.diff(primary_axis)) != dominate_polarity)[0] + 1
        chunks = np.split(raw_time_trace.data, breaks, axis=1)
        # in a staircase, all the values on the other axes are the same
        # Check that within each chunk, each non-time axis is constant (column-wise)
        for chunk in chunks:
            if chunk.shape[0] == 0:
                continue
            other_axes = chunk[1:, :]
            # For each column, all values must be the same
            if not bool(
                np.all(np.array([np.all(row == row[0]) for row in other_axes]))
            ):
                msg = "Within a chunk, each non-time axis must be constant."
                raise ValueError(msg)
            first_row = chunk[0, :]
            assert np.all(np.sign(np.diff(first_row)) == dominate_polarity), (
                "Chunks must all have the same polarity."
            )
        return chunks

    def interject_ramps(
        self,
        instructions: list[Instruction],
    ) -> list[Instruction]:
        """Interjects ramps between each instruction.

        Args:
            instructions: The list of instructions to interject ramps into.

        Returns:
            the list of instructions with ramps interjected.
        """
        new_instructions = []
        for i, instruction in enumerate(instructions):
            if i == 0:
                new_instructions.append(instruction)
                continue
            new_instruction = Instruction(
                setters={
                    port: (
                        {
                            SUPPORTED_PROPERTIES.VOLTAGE_STATE: properties[  # type: ignore[reportOptionalMemberAccess]
                                SUPPORTED_PROPERTIES.STAIRCASE
                            ][3]
                        }
                        if SUPPORTED_PROPERTIES.VOLTAGE_STATE not in properties
                        else properties
                    )
                    for port, properties in instruction.setters.items()
                },
                buffered=True,
            )
            new_instructions.append(new_instruction)
            new_instructions.append(instruction)
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
        for step in self.measurement_groups[measurement_id]:
            for connection, props in step.setters.items():
                for prop, value in props.items():
                    await self.update_daemon_property(
                        property=prop,
                        name=connection,
                        value=value,
                    )
            await self.deploy_measurement(
                id=measurement_id,
                getters=step.getters,
                setters=step.setters if step.buffered else {},
            )

    async def handle_data(
        self,
        msg: "Msg",
    ) -> None:
        """Handle a PROCESS_DATA command.

        Args:
            msg: The NATS message.
        """
        try:
            data = json.loads(msg.data.decode())
            instrument_data = data.get(INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.DATA)
            id = str(data.get(INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.PROCESS_ID))
            timestamp = str(
                data.get(INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.TIMESTAMP)
            )
            assert isinstance(instrument_data, dict)
            if id not in self.data_queue:
                self._data_queue[id] = DataQueue()
            entry = DataEntry(timestamp=timestamp, data=data)
            queue = self.data_queue[id]
            queue.append(entry)
            await self.log("Data added to queue ....")
        except Exception as e:
            await self.log(f"Error adding data to the queue: {e}")

    async def load_and_export_data(
        self,
        request: "MeasurementRequest",
        shape: tuple[int, ...],
        data_path: Path,
        id: "ID",
        data_count: int,
    ) -> None:
        """Load and export data to a file. Finishes by sending the completed result away.

        This method works with the raw timetrace data to process it into the form requested, following the instructions.

        Args:
            request: The measurement request.
            shape: The shape of the data.
            data_path: The path to the data file.
            id: The ID of the measurement.
            data_count: The number of data points to load.
        """
        await self.confirm_data_exists(
            id=id,
            data_count=data_count,
        )
        number_of_bins = self.get_data_point_counter_per_queue(
            shape=shape,
            data_count=data_count,
        )
        name_attribute_maps = self.preprocess_voltage_states(id=id)
        final_data = self.average_shapeless_data(
            id=id,
            number_of_bins=number_of_bins,
            request=request,
            voltage_state_array=name_attribute_maps,
        )
        response = self.make_response(
            data_arrays=final_data,
            shape=shape,
        )
        self.store_in_database(
            response=response,
            request=request,
            id=id,
            data_path=data_path,
        )
        await self.upload_data(response=response)

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
            timestamp=str(time.time()),
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

    def average_shapeless_data(
        self,
        id: "ID",
        number_of_bins: int,
        request: MeasurementRequest,
        voltage_state_array: list[dict[str, float]],
    ) -> dict["InstrumentPort", list[float]]:
        """Computes the average over the data and stores it in a 1D array to be reshaped later.

        Args:
            id: The ID of the measurement.
            number_of_bins: The number of bins to average over.
            request: The measurement request.
            voltage_state_array: The different votlage states to use when computing the average.

        Returns:
            the computed average data.
        """
        final_data: dict[InstrumentPort, list[float]] = {}
        for queue in self.data_queue[id]:
            for port, individual_data in queue.data.items():
                datas = individual_data.even_divisions(divisions=number_of_bins)
                transform = next(
                    (t for t in request.meter_transforms if t.port == port), None
                )
                assert transform is not None, f"Transform not found for port {port}"
                analytic_transform = transform
                time_bounds = request.time_domain.domain.bounds
                num_points = len(datas[0])
                t_array = np.linspace(
                    start=time_bounds[0],
                    stop=time_bounds[1],
                    num=num_points,
                )
                for j, data in enumerate(datas):
                    data_arr = np.array(data)
                    vectorized_transform = np.vectorize(
                        lambda t: analytic_transform.transform(
                            t=t, **voltage_state_array[j]
                        )  # type : ignore[reportOptionalMemberAccess]
                    )
                    transformed = vectorized_transform(t_array)
                    masked = (transformed * data_arr)[transformed != 0]
                    computation = np.mean(masked) if masked.size > 0 else 0.0
                    if port not in final_data:
                        final_data[port] = []
                    final_data[port].append(float(computation))

        return final_data

    def preprocess_voltage_states(self, id: "ID") -> list[dict[str, float]]:
        """Preprocesses the voltage states for the measurement.

        Modifies the setup stored in teh measurement_groups.

        Args:
            id: The ID of the measurement.

        Returns:
            the preprocessed voltage states.
        """
        name_attribute_maps: list[dict[str, float]] = []
        for instr in self.measurement_groups[id]:
            if not instr.getters:
                continue
            first_property_map = list(instr.setters.values())[0]
            if SUPPORTED_PROPERTIES.STAIRCASE in first_property_map:
                staircase = first_property_map[SUPPORTED_PROPERTIES.STAIRCASE]
                assert isinstance(staircase, tuple), (
                    "STAIRCASE must be a tuple of numbers."
                )
                num_steps = int(staircase[1])
                for step in range(num_steps):
                    map = {}
                    for port, property_map in instr.setters.items():
                        staircase = property_map[SUPPORTED_PROPERTIES.STAIRCASE]
                        assert isinstance(staircase, tuple), (
                            "STAIRCASE must be a tuple of numbers."
                        )
                        map[port.instrument_facing_name()] = (
                            (float(staircase[4]) - float(staircase[3]))
                            * step
                            / (num_steps - 1)
                        ) + float(staircase[3])
                    name_attribute_maps.append(map)
            elif SUPPORTED_PROPERTIES.VOLTAGE_STATE in first_property_map:
                map = {}
                for port, property_map in instr.setters.items():
                    value = property_map[SUPPORTED_PROPERTIES.VOLTAGE_STATE]
                    assert isinstance(value, float), "Invalid set command."
                    map[port.instrument_facing_name()] = value
                name_attribute_maps.append(map)
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
        assert isinstance(expected_data_points_per_queue, int), (
            "Uneven division, not sure how many data points to expect"
        )
        return expected_data_points_per_queue

    async def confirm_data_exists(
        self,
        id: "ID",
        data_count: int,
        max_attempts: int = 10,
        wait_time=0.5,
    ) -> None:
        """Confirms that data exists in the queue.

        Args:
            max_attempts: The maximum number of attempts to check for data.
            wait_time: The time to wait between attempts. [sec]
            id: The ID of the measurement.
            data_count: The number of data points to check for.
        """
        log_attempts = 0
        while True:
            queue = self.data_queue.get(id, [])
            current_count = len(queue)
            if current_count > data_count:
                await self.log(
                    f"Error: Unexpected number of data messages received for {id} (expected {data_count}, got {current_count})"
                )
                return
            if current_count < data_count:
                if log_attempts >= max_attempts:
                    await self.log(
                        f"Error: Not enough data messages received for {id} after waiting (expected {data_count}, got {current_count})"
                    )
                    return
                await self.log(
                    f"Waiting for more data for {id} (expected {data_count}, got {current_count})"
                )
                log_attempts += 1
                await asyncio.sleep(wait_time)
                continue
            break
