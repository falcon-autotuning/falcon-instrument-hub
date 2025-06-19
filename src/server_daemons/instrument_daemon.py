"""An instrument daemon that runs instrument drivers."""

import contextlib
from typing import TYPE_CHECKING

from instrument_templates.constants import SUPPORTED_PROPERTIES

from .api.instrument import RUNTIME_COMMANDS as DRIVER_RUNTIME_COMMANDS
from .dependancies import (
    Time,
    asyncio,
    json,
    nats,
    signal,
)

if TYPE_CHECKING:
    from .typing import Any, BaseInstrumentDriver, Client, Msg


class InstrumentDaemon:
    """A daemon that runs in the background and maintains and manages an instrument."""

    _nc: "Client"
    _loop: asyncio.AbstractEventLoop
    _url: str
    _instrument_name: str
    _instrument: "BaseInstrumentDriver"
    _debug: bool
    _set_queue: asyncio.Queue
    _is_locked: bool

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
                await self._loop.run_in_executor(
                    None,
                    self._instrument.process_trigger,
                )

            self._loop.create_task(run_trigger())

            await asyncio.sleep(timeout)
            await self.unlock_set_queue()
            await self.log(
                f"TRIGGER timeout reached ({timeout}s) - instrument unlocked"
            )
            # Process any return data from setter instruments
            while not self._instrument._return_data._message_queue.empty():
                await self.log("Queue is not empty, processing return data....")
                try:
                    return_data = (
                        self._instrument._return_data._message_queue.get_nowait()
                    )
                    # Add process_id and chunk_id to the return data message
                    return_data[DRIVER_RUNTIME_COMMANDS.RETURN_DATA.PROCESS_ID] = (
                        process_id
                    )
                    return_data[DRIVER_RUNTIME_COMMANDS.RETURN_DATA.CHUNK_ID] = chunk_id

                    await self.send_command(
                        channel=self.specific_channel(
                            DRIVER_RUNTIME_COMMANDS.RETURN_DATA.COMM_CHANNEL
                        ),
                        message=json.dumps(return_data),
                    )
                except Exception as e:
                    await self.log(f"Error processing return data: {e}")

            await self.log("All return data processed after TRIGGER command")

        except Exception as e:
            # Unlock on any error as well
            await self.unlock_set_queue()
            await self.log(
                message=f"Error in TRIGGER command: {e!s} - instrument unlocked",
            )

    async def process_set_queue(self):
        """Process SET commands from the queue when unlocked."""
        try:
            while not self._shutdown_event.is_set():
                if not self._is_locked:
                    try:
                        # Wait for a SET command with a short timeout
                        set_data = await asyncio.wait_for(
                            self._set_queue.get(),
                            timeout=0.1,
                        )

                        property_name = set_data.get(
                            DRIVER_RUNTIME_COMMANDS.SET.PROPERTY
                        )
                        index = set_data.get(DRIVER_RUNTIME_COMMANDS.SET.INDEX)
                        value = set_data.get(DRIVER_RUNTIME_COMMANDS.SET.VALUE)
                        process_id = set_data.get(
                            DRIVER_RUNTIME_COMMANDS.SET.PROCESS_ID
                        )
                        chunk_id = set_data.get(DRIVER_RUNTIME_COMMANDS.SET.CHUNK_ID)

                        # Check if this is an ARM command - if so, lock the queue
                        if property_name == SUPPORTED_PROPERTIES.ARM:
                            self._is_locked = True
                            return_data = {
                                DRIVER_RUNTIME_COMMANDS.ARMED.PROCESS_ID: process_id,
                                DRIVER_RUNTIME_COMMANDS.ARMED.CHUNK_ID: chunk_id,
                                DRIVER_RUNTIME_COMMANDS.ARMED.TIMESTAMP: Time().time,
                            }
                            await self.send_command(
                                channel=self.specific_channel(
                                    DRIVER_RUNTIME_COMMANDS.ARMED.COMM_CHANNEL
                                ),
                                message=json.dumps(return_data),
                            )
                            await self.log(
                                f"Queue locked due to ARM command on {self._instrument_name}"
                            )

                        # Process the SET command
                        await self._loop.run_in_executor(
                            None,
                            self._instrument.set_property,
                            property_name,
                            index,
                            value,
                        )
                        await self.log(f"SET command executed: {property_name}={value}")

                    except asyncio.TimeoutError:
                        # No items in queue, continue
                        continue
                else:
                    # Queue is locked, wait a bit before checking again
                    await asyncio.sleep(0.1)

        except asyncio.CancelledError:
            await self.log(f"Set queue processor cancelled for {self._instrument_name}")
            raise
        except Exception as e:
            await self.log(f"Error in set queue processor: {e}")

    async def unlock_set_queue(self):
        """Unlock the set queue to allow processing SET commands."""
        self._is_locked = False
        await self.log(f"Queue unlocked for {self._instrument_name}")

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
            process_id = int(data.get(DRIVER_RUNTIME_COMMANDS.SET.PROCESS_ID))
            chunk_id = int(data.get(DRIVER_RUNTIME_COMMANDS.SET.CHUNK_ID))

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
