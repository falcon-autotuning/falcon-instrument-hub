"""An instrument daemon that runs instrument drivers."""

import contextlib
from typing import TYPE_CHECKING

from instrument_templates.constants import SUPPORTED_PROPERTIES
from instrument_templates.trigger import Trigger

from .api.instrument import RUNTIME_COMMANDS as DRIVER_RUNTIME_COMMANDS
from .dependancies import (
    Time,
    asyncio,
    json,
    nats,
    signal,
)

if TYPE_CHECKING:
    from instrument_templates.typing import PropertyName

    from .typing import Any, BaseInstrumentDriver, Client, Msg


class InstrumentDaemon:
    """A daemon that runs in the background and maintains and manages an instrument."""

    _nc: "Client"
    _unlock_state: list[bool]
    _loop: asyncio.AbstractEventLoop
    _url: str
    _instrument_name: str
    _instrument: "BaseInstrumentDriver"
    _debug: bool
    _set_queue: asyncio.Queue
    _is_locked: bool
    _mutex: asyncio.Lock

    def __init__(
        self,
        url: str,
        instrument_driver: type["BaseInstrumentDriver"],
        debug: bool = False,
    ):
        """Initialize the InstrumentDaemon.

        Args:
            url: The URL of the NATS server.
            instrument_class: The class of the instrument to be controlled.
        """
        self._mutex = asyncio.Lock()
        self._unlock_state = []
        self._url = url
        self._debug = debug
        self._instrument_name = instrument_driver.__name__
        self._instrument = instrument_driver()
        self._shutdown_event = asyncio.Event()
        self._set_queue = asyncio.Queue()
        self._is_locked = False

    async def start(self):
        """The main loop for the daemon."""
        print(f"Starting daemon for {self._instrument_name!s}", flush=self._debug)
        self._loop = asyncio.get_running_loop()

        try:
            # Try to connect to NATS with timeout
            print(f"Attempting to connect to NATS at {self._url}...", flush=self._debug)
            self._nc = await nats.connect(self._url)
            await self.log("Connected to NATS successfully")
            await self.confirm_initialization()
            await self.log(f"Instrument config released for {self._instrument_name}")
            await self.setup_subscriptions()
            await self.log(f"Setup all subscriptions for {self._instrument_name}")
            self._loop.create_task(self.process_set_queue())
            self._loop.create_task(self.publish_status())
            self._loop.create_task(self.message_consumer())
        except asyncio.TimeoutError:
            print(f"Failed to connect to NATS at {self._url} within timeout")
        except Exception as e:
            print(f"Error during startup: {e}")

        try:
            # Wait for shutdown signal or until the program is interrupted
            await self.log(
                f"Daemon {self._instrument_name} running, waiting for shutdown signal..."
            )
            # Just wait forever - let SIGTERM kill the process naturally
            while True:
                await asyncio.sleep(1.0)

        except (KeyboardInterrupt, asyncio.CancelledError):
            # Handle Ctrl+C or process termination
            await self.log(
                f"Daemon serving {self._instrument_name} interrupted, shutting down..."
            )
        finally:
            await self.log(f"Cleaning up {self._instrument_name}")
            # Simple cleanup without waiting for complex shutdown events
            if hasattr(self, "_nc") and self._nc:
                with contextlib.suppress(asyncio.TimeoutError, Exception):
                    await asyncio.wait_for(self._nc.close(), timeout=1.0)
            print(f"Daemon {self._instrument_name} shutdown complete")

    def _setup_signal_handlers(self):
        """Set up signal handlers for graceful shutdown."""
        print("Setting up signal handlers for SIGTERM and SIGINT", flush=self._debug)
        for sig in (signal.SIGTERM, signal.SIGINT):
            signal.signal(sig, self._signal_handler)
            print(f"Signal handler set for signal {sig}", flush=self._debug)

    def _signal_handler(self, signum, frame):
        """Handle shutdown signals."""
        print(
            f"Received signal {signum}, initiating graceful shutdown...",
            flush=self._debug,
        )

        # Instead of using call_soon_threadsafe, directly set the event
        # asyncio.Event.set() is thread-safe on its own
        try:
            self._shutdown_event.set()
            print(f"Signal {signum} handler completed", flush=self._debug)
        except Exception as e:
            print(f"Error in signal handler: {e}", flush=self._debug)
            # Force exit if we can't set the event
            import os

            os._exit(1)

    async def _cleanup_tasks(self):
        """Clean up all running tasks."""
        # Get all tasks except the current one
        tasks = [
            task
            for task in asyncio.all_tasks(self._loop)
            if task is not asyncio.current_task()
        ]

        if tasks:
            print(f"Cancelling {len(tasks)} running tasks...")
            # Cancel all tasks
            for task in tasks:
                task.cancel()

            # Wait for all tasks to complete cancellation
            try:
                await asyncio.wait_for(
                    asyncio.gather(*tasks, return_exceptions=True), timeout=3.0
                )
                print("All tasks cancelled successfully")
            except asyncio.TimeoutError:
                print("Some tasks didn't cancel within timeout, continuing...")
        else:
            print("No tasks to cancel")

    def specific_channel(
        self,
        channel: str,
    ) -> str:
        """Builds a specific channel for the daemon.

        Args:
            channel: The base channel.

        Returns:
            The specific channel for the daemon.
        """
        return f"{channel}.{self._instrument_name}"

    async def setup_subscriptions(self):
        """Set up subscriptions for the daemon."""
        subscriptions: list[tuple[str, Any]] = [
            (DRIVER_RUNTIME_COMMANDS.SET.COMM_CHANNEL, self.handle_set),
            (DRIVER_RUNTIME_COMMANDS.TRIGGER.COMM_CHANNEL, self.handle_trigger),
            (DRIVER_RUNTIME_COMMANDS.GET.COMM_CHANNEL, self.handle_get),
            (
                DRIVER_RUNTIME_COMMANDS.PERFORM_ARBITRARY_METHOD.COMM_CHANNEL,
                self.handle_arbitration,
            ),
        ]
        for channel, handle in subscriptions:
            await self._nc.subscribe(
                self.specific_channel(
                    channel=channel,
                ),
                cb=handle,
            )
            print(f"Handled setting up subscription {channel}", flush=self._debug)

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
                DRIVER_RUNTIME_COMMANDS.LOG.MESSAGE: message,
                DRIVER_RUNTIME_COMMANDS.LOG.TIMESTAMP: Time().time,
            }
        )
        await self.send_command(
            channel=self.specific_channel(
                channel=DRIVER_RUNTIME_COMMANDS.LOG.COMM_CHANNEL,
            ),
            message=message,
        )

    async def confirm_initialization(self):
        """Confirms the initialization of the daemon."""
        init, ports = self._instrument.to_json_config()
        init_message = json.dumps(
            {
                DRIVER_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.TIMESTAMP: Time().time,
                DRIVER_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.INIT: json.dumps(init),
                DRIVER_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.PORT: json.dumps(ports),
            }
        )
        await self.send_command(
            channel=self.specific_channel(
                channel=DRIVER_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.COMM_CHANNEL,
            ),
            message=init_message,
        )

    async def publish_status(self, refresh: float = 0.5) -> None:
        """Publishes the status of the daemon every refresh."""
        try:
            while not self._shutdown_event.is_set():
                pending = len([t for t in asyncio.all_tasks() if not t.done()])
                message = json.dumps(
                    {
                        DRIVER_RUNTIME_COMMANDS.STATUS.TIMESTAMP: Time().time,
                        DRIVER_RUNTIME_COMMANDS.STATUS.STATUS: pending > 1,
                    }
                )
                await self.send_command(
                    channel=self.specific_channel(
                        channel=DRIVER_RUNTIME_COMMANDS.STATUS.COMM_CHANNEL,
                    ),
                    message=message,
                )
                await asyncio.sleep(refresh)
            await self.log(
                f"Status publishing stopped due to shutdown event for {self._instrument_name}"
            )
        except asyncio.CancelledError:
            print("Status publishing cancelled")
            raise

    async def message_consumer(self):
        """Consume messages from the sync sender queue and send them async."""
        try:
            while (
                not self._shutdown_event.is_set()
                and hasattr(self._instrument, "_sync_sender")
                and self._instrument._sync_sender._running
            ):
                messages = self._instrument._sync_sender.get_queued_messages()
                for channel, message in messages:
                    await self.send_command(channel, message)
                await asyncio.sleep(0.01)  # Small delay to prevent busy waiting
            await self.log(f"Message consumer stopped for {self._instrument_name}")
        except asyncio.CancelledError:
            await self.log(f"Message consumer cancelled for {self._instrument_name}")
            raise
        except Exception as e:
            await self.log(f"Error in message consumer: {e}")

    async def handle_arbitration(
        self,
        msg: "Msg",
    ) -> None:
        """Handle a PERFORM_ARBITRARY_METHOD command.

        Args:
            msg: The NATS message.
        """
        try:
            data = json.loads(msg.data.decode())
            method_name = data.get(
                DRIVER_RUNTIME_COMMANDS.PERFORM_ARBITRARY_METHOD.METHOD
            )
            keywords = data.get(
                DRIVER_RUNTIME_COMMANDS.PERFORM_ARBITRARY_METHOD.KEYWORD_ARGS
            )
            kwargs: dict[str, Any] = json.loads(keywords)

            if not all([method_name, keywords is not None]):
                await self.log(
                    message="Invalid PERFORM_ARBITRARY_METHOD command",
                )
                return

            if not hasattr(self._instrument, method_name):
                await self.log(
                    message=f"Method {method_name} not found in {self._instrument_name}",
                )
                return

            # Check if the attribute is callable
            if not callable(getattr(self._instrument, method_name)):
                await self.log(
                    message=f"Method {method_name} is not callable in {self._instrument_name}",
                )
                return

            # Execute the method properly with the instance context
            def execute_method():
                # Call the method directly on the instrument instance
                return getattr(self._instrument, method_name)(**kwargs)

            # Run in executor to avoid blocking the event loop
            await self._loop.run_in_executor(None, execute_method)
            await self.log(
                message="PERFORM_ARBITRARY_METHOD command executed",
            )
        except Exception as e:
            await self.log(
                message=f"Error in PERFORM_ARBITRARY_METHOD command: {e!s}",
            )

    async def store_unlock_state_name(self, name: bool):
        """Stores the unlock store names.

        Args:
            name: The name of the unlock state to store (either setter or not)
        """
        async with self._mutex:
            self._unlock_state.append(name)

    async def handle_trigger(
        self,
        msg: "Msg",
    ) -> None:
        """Handle a TRIGGER command.

        Args:
            msg: The NATS message.
        """
        try:
            data = json.loads(msg.data.decode())
            process_id = data[DRIVER_RUNTIME_COMMANDS.TRIGGER.PROCESS_ID]
            assert isinstance(process_id, int), "process_id must be an integer"
            chunk_id = data[DRIVER_RUNTIME_COMMANDS.TRIGGER.CHUNK_ID]
            assert isinstance(chunk_id, int), "chunk_id must be an integer"
            is_setter = data[DRIVER_RUNTIME_COMMANDS.TRIGGER.IS_SETTER]
            assert isinstance(is_setter, bool), "is_setter must be a boolean"
            await self.store_unlock_state_name(is_setter)
            # Send executing message immediately
            timeout = self._instrument._timeout
            assert isinstance(timeout, (int, float)), (
                f"timeout must be a number, got {timeout}"
            )
            return_msg = {
                DRIVER_RUNTIME_COMMANDS.EXECUTING.PROCESS_ID: process_id,
                DRIVER_RUNTIME_COMMANDS.EXECUTING.CHUNK_ID: chunk_id,
                DRIVER_RUNTIME_COMMANDS.EXECUTING.TIMESTAMP: Time().time,
            }
            await self.send_command(
                channel=self.specific_channel(
                    DRIVER_RUNTIME_COMMANDS.EXECUTING.COMM_CHANNEL
                ),
                message=json.dumps(return_msg),
            )
            await self.log("Trigger command recieved, starting process...")

            # Start the trigger process concurrently
            async def run_trigger():
                try:
                    if is_setter:
                        await self.log("Starting process_setter_trigger in executor...")
                        await self._loop.run_in_executor(
                            None,
                            self._instrument._process_setter_triggers,
                        )
                    else:
                        await self.log("Starting process_getter_trigger in executor...")
                        await self._loop.run_in_executor(
                            None,
                            self._instrument._process_getter_triggers,
                        )
                except Exception as e:
                    await self.log(f"Error in process_trigger: {e}")

            self._loop.create_task(run_trigger())

            await asyncio.sleep(timeout)
            await self.try_to_leave(name=is_setter)
            await self.log(f"TRIGGER timeout reached ({timeout}s)")
            if not is_setter:
                await self.process_return_data(
                    process_id=process_id,
                    chunk_id=chunk_id,
                )

        except Exception as e:
            # Unlock on any error as well
            await self.log(
                message=f"Error in TRIGGER command: {e!s}",
            )
            await self.try_to_leave()

    async def process_return_data(
        self,
        process_id: int,
        chunk_id: int,
    ) -> None:
        """Process return data and sends it away.

        Args:
            process_id: The ID of the proces that requested the data
            chunk_id: The ID of the chunk that indexes the data
        """
        await self.log("Processing return data...")
        self._instrument._process_return_data()
        # Add debugging for the queue state
        await self.log("Checking return data queue...")
        if self._instrument._return_data._message_queue.empty():
            await self.log("There is nothing to process in the return data queue.")
            return
        queue_size = self._instrument._return_data._message_queue.qsize()
        await self.log(f"Queue size: {queue_size}")

        # Process any return data from setter instruments
        messages = self._instrument._return_data.get_queued_messages()
        await self.log(f"Got {len(messages)} messages from get_queued_messages()")

        if not messages:
            await self.log("No messages found in queue after trigger timeout")
            return
        for channel, message in messages:
            try:
                # Parse the message to add process_id and chunk_id
                return_data = json.loads(message)
                return_data[DRIVER_RUNTIME_COMMANDS.RETURN_DATA.PROCESS_ID] = process_id
                return_data[DRIVER_RUNTIME_COMMANDS.RETURN_DATA.CHUNK_ID] = chunk_id

                await self.send_command(
                    channel=self.specific_channel(
                        DRIVER_RUNTIME_COMMANDS.RETURN_DATA.COMM_CHANNEL
                    ),
                    message=json.dumps(return_data),
                )
                await self.log("Sent return data")
            except Exception as e:
                await self.log(f"Error processing return data: {e}")
        await self.log("No messages found in queue after trigger timeout")

    async def process_set_queue(self):
        """Process SET commands from the queue when unlocked."""
        try:
            while not self._shutdown_event.is_set():
                if self._is_locked:
                    # Queue is locked, wait a bit before checking again
                    await asyncio.sleep(0.1)
                    continue
                try:
                    # Wait for a SET command with a short timeout
                    set_data = await asyncio.wait_for(
                        self._set_queue.get(),
                        timeout=0.1,
                    )

                    property_name = set_data.get(DRIVER_RUNTIME_COMMANDS.SET.PROPERTY)
                    assert isinstance(property_name, str), (
                        f"The raw property name must be a string and not {type(property_name)}"
                    )

                    if property_name == SUPPORTED_PROPERTIES.ARM:
                        await self.process_arm_command(set_data=set_data)
                        continue

                    await self.process_set_command(
                        property_name=property_name,
                        set_data=set_data,
                    )

                except asyncio.TimeoutError:
                    # No items in queue, continue
                    continue

        except asyncio.CancelledError:
            await self.log(f"Set queue processor cancelled for {self._instrument_name}")
            raise
        except Exception as e:
            await self.log(f"Error in set queue processor: {e}")

    async def process_set_command(
        self,
        property_name: "PropertyName",
        set_data: dict[str, "Any"],
    ) -> None:
        """Process a SET command directly and run it in an external executor.

        Args:
            property_name: The name of the property to set.
            set_data: The data corresponding to the SET command.
        """
        index = set_data.get(DRIVER_RUNTIME_COMMANDS.SET.INDEX)
        assert isinstance(index, int), (
            f"The type of index must be an integer and not {type(index)}"
        )
        value = set_data.get(DRIVER_RUNTIME_COMMANDS.SET.VALUE)
        assert isinstance(value, (str | int | float)), (
            f"The type of the set value must be a string, int, or float and not {type(value)}"
        )
        await self._loop.run_in_executor(
            None,
            self._instrument.set_property,
            property_name,
            index,
            value,
        )
        await self.log(f"SET command executed: {property_name}={value}")

    async def process_arm_command(
        self,
        set_data: dict[str, "Any"],
    ) -> None:
        """Process an ARM command from the SET queue.

        Args:
            set_data: The data for the ARM command.
            value: The value associated with the ARM command, expected to be a list of trigger indexes.
        """
        process_id = set_data.get(DRIVER_RUNTIME_COMMANDS.SET.PROCESS_ID)
        chunk_id = set_data.get(DRIVER_RUNTIME_COMMANDS.SET.CHUNK_ID)
        value = set_data.get(DRIVER_RUNTIME_COMMANDS.SET.VALUE)
        assert isinstance(value, str), (
            "The stored value must be a json string for the arm command"
        )

        # If this is an ARM command - if so, lock the queue
        self._is_locked = True
        await self.fill_trigger_queue(raw_trigger_indexes=json.loads(value))

        return_data = {
            DRIVER_RUNTIME_COMMANDS.ARMED.PROCESS_ID: process_id,
            DRIVER_RUNTIME_COMMANDS.ARMED.CHUNK_ID: chunk_id,
            DRIVER_RUNTIME_COMMANDS.ARMED.TIMESTAMP: Time().time,
        }
        await self.send_command(
            channel=self.specific_channel(DRIVER_RUNTIME_COMMANDS.ARMED.COMM_CHANNEL),
            message=json.dumps(return_data),
        )
        await self.log(f"Queue locked due to ARM command on {self._instrument_name}")

    async def fill_trigger_queue(
        self,
        raw_trigger_indexes: dict[str, list[dict[str, str | int]]],
    ) -> None:
        """Fills the trigger queue on the instrument with the trigger indexes selected from the recieved ARM command.

        Args:
            raw_trigger_indexes: The list of the trigger indexes to place in the queue for the instrument.
        """
        assert isinstance(raw_trigger_indexes, dict), (
            f"Expected the type of the raw_trigger_indexes to be a dict, not {type(raw_trigger_indexes)}"
        )
        assert "setter" in raw_trigger_indexes, (
            f"Trigger dictionary missing required 'property' key: {raw_trigger_indexes}"
        )
        assert "getter" in raw_trigger_indexes, (
            f"Trigger dictionary missing required 'index' key: {raw_trigger_indexes}"
        )
        setters = raw_trigger_indexes["setter"]
        await self.log(f"DEBUG: Found setter triggers in the ARM command {setters}")
        getters = raw_trigger_indexes["getter"]
        await self.log(f"DEBUG: Found getter triggers in the ARM command {getters}")
        self.set_instrument_queue(
            trigger_indexes=setters,
            triggers=self._instrument._setter_triggers,
        )
        self.set_instrument_queue(
            trigger_indexes=getters,
            triggers=self._instrument._getter_triggers,
        )

    def set_instrument_queue(
        self,
        trigger_indexes: list[dict[str, str | int]],
        triggers: "list[Trigger]",
    ) -> None:
        """Sets the trigger queue for the insturment with the provided indexes.

        Args:
            trigger_indexes: The list of trigger indexes to set in the queue.
            queue: The queue in the instrument that needs to contain the instrument indexes.
        """
        for trigger in trigger_indexes:
            assert isinstance(trigger, dict), (
                f"Expected each trigger to be a dict, not {type(trigger)}"
            )
            assert "property" in trigger, (
                f"Trigger dictionary missing required 'property' key: {trigger}"
            )
            assert "index" in trigger, (
                f"Trigger dictionary missing required 'index' key: {trigger}"
            )
            readyTrigger = Trigger(
                property_name=str(trigger["property"]),
                index=int(trigger["index"]),
            )
            triggers.append(readyTrigger)

    async def try_to_leave(self, name: bool | None = None):
        """Unlock the set queue to allow processing SET commands."""
        async with self._mutex:
            if (len(self._unlock_state) == 1) != (name is None):
                self._is_locked = False
                self._unlock_state.clear()
                await self.log(f"Queue unlocked for {self._instrument_name}")
            elif name is not None:
                sname = "setter" if name else "getter"
                self._unlock_state.remove(name)
                await self.log(f"Its not my job to unlock the queue, I was a {sname}")
            else:
                await self.log("I died before getting a name.")

    async def handle_set(
        self,
        msg: "Msg",
    ) -> None:
        """Handle a SET command.

        Args:
            msg: The NATS message.
        """
        try:
            data = json.loads(msg.data.decode())
            property_name = data.get(DRIVER_RUNTIME_COMMANDS.SET.PROPERTY)
            index = data.get(DRIVER_RUNTIME_COMMANDS.SET.INDEX)
            value = data.get(DRIVER_RUNTIME_COMMANDS.SET.VALUE)
            process_id = data.get(DRIVER_RUNTIME_COMMANDS.SET.PROCESS_ID)
            chunk_id = data.get(DRIVER_RUNTIME_COMMANDS.SET.CHUNK_ID)

            if not all([property_name, index, value is not None]):
                await self.log(
                    message="Invalid SET command",
                )
                return
            # Add the SET command to the queue
            set_data = {
                DRIVER_RUNTIME_COMMANDS.SET.PROPERTY: property_name,
                DRIVER_RUNTIME_COMMANDS.SET.INDEX: index,
                DRIVER_RUNTIME_COMMANDS.SET.VALUE: value,
                DRIVER_RUNTIME_COMMANDS.SET.PROCESS_ID: process_id,
                DRIVER_RUNTIME_COMMANDS.SET.CHUNK_ID: chunk_id,
            }
            await self._set_queue.put(set_data)
            await self.log(f"SET command queued: {property_name}={value}")
        except Exception as e:
            await self.log(
                message=f"Error in SET command: {e!s}",
            )

    async def handle_get(
        self,
        msg: "Msg",
    ) -> None:
        """Handle a GET command.

        Args:
            msg: The NATS message.
        """
        try:
            data = json.loads(msg.data.decode())
            property_name = data.get(DRIVER_RUNTIME_COMMANDS.GET.PROPERTY)
            index = data.get(DRIVER_RUNTIME_COMMANDS.GET.INDEX)

            if not all([property_name, index]):
                await self.log(
                    message="Invalid GET command",
                )
                return
            # Locks the threads to make sure the calls are synchronous
            await self._loop.run_in_executor(
                None,
                self._instrument.get_property,
                property_name,
                index,
            )
        except Exception as e:
            await self.log(
                message=f"Error in GET command: {e!s}",
            )
