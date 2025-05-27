"""An instrument daemon that runs instrument drivers."""

from typing import TYPE_CHECKING

from .dependancies import (
    DRIVER_RUNTIME_COMMANDS,
    InstrumentSyncSender,
    asyncio,
    json,
    nats,
    time,
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

    def __init__(
        self,
        url: str,
        instrument_driver: type["BaseInstrumentDriver"],
        loop: asyncio.AbstractEventLoop,
    ):
        """Initialize the InstrumentDaemon.

        Args:
            url: The URL of the NATS server.
            instrument_class: The class of the instrument to be controlled.
            loop: The event loop to use for synchronous operations.
        """
        self._url = url
        self._loop = loop
        self._instrument_name = instrument_driver.__name__
        sync_sender = InstrumentSyncSender(self.send_command, loop=loop)
        self._instrument = instrument_driver(
            sync_sender=sync_sender,
        )

    async def start(self):
        """The main loop for the daemon."""
        self._nc = await nats.connect(self._url)
        await self.confirm_initialization()
        await self.setup_subscriptions()
        self._loop.create_task(self.publish_status())

        forever = asyncio.Future()
        try:
            # Wait forever or until the program is interrupted
            await forever
        except asyncio.CancelledError:
            # Handle graceful shutdown if the future is cancelled
            print(f"Daemon serving {self._instrument_name} shutting down...")
        finally:
            # Properly drain the connection when exiting
            await self._nc.drain()

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
                DRIVER_RUNTIME_COMMANDS.LOG.MESSAGE: message,
                DRIVER_RUNTIME_COMMANDS.LOG.TIMESTAMP: str(time.time()),
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
        init_config = self._instrument.to_json_config()
        init_message = json.dumps(
            {
                DRIVER_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.INIT: init_config,
                DRIVER_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.TIMESTAMP: str(
                    time.time()
                ),
                DRIVER_RUNTIME_COMMANDS.CONFIRM_INITIALIZATION.INIT: json.dumps(
                    init_config
                ),
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
        while True:
            pending = len([t for t in asyncio.all_tasks() if not t.done()])
            message = json.dumps(
                {
                    DRIVER_RUNTIME_COMMANDS.STATUS.TIMESTAMP: str(time.time()),
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

            method = getattr(self._instrument, method_name)
            if not callable(method):
                await self.log(
                    message=f"Method {method_name} is not callable in {self._instrument_name}",
                )
                return

            def locked_method_call():
                with self._instrument._property_lock:
                    method(**kwargs)

            await self._loop.run_in_executor(None, locked_method_call)

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
            port = data.get(DRIVER_RUNTIME_COMMANDS.TRIGGER.TRIGGER_PORT)
            # Locks the threads to make sure the calls are synchronous
            await self._loop.run_in_executor(
                None,
                self._instrument.process_trigger,
                port,
            )
            await self.log(
                message="TRIGGER command executed",
            )
        except Exception as e:
            await self.log(
                message=f"Error in TRIGGER command: {e!s}",
            )

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

            if not all([property_name, index, value is not None]):
                await self.log(
                    message="Invalid SET command",
                )
                return
            # Locks the threads to make sure the calls are synchronous
            await self._loop.run_in_executor(
                None,
                self._instrument.set_property,
                property_name,
                index,
                value,
            )
            await self.log(
                message="SET command executed",
            )
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
            property_name = data.get("property_name")
            index = data.get("index")

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
