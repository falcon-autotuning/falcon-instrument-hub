"""A measurement interpreter for the instrument server."""

from typing import TYPE_CHECKING

from .constants import INTERPRETER_RUNTIME_COMMANDS
from .dependancies import (
    MeasurementInterpreter,
    MeasurementRequest,
    asyncio,
    json,
    nats,
    time,
)
from .interpreter_sync_sender import InterpreterSyncSender

if TYPE_CHECKING:
    from .typing import Any, Client, Msg


class InterpreterDaemon:
    """A daemon that processes messages for the measurement interpretter."""

    _url: str
    _nc: "Client"
    _loop: asyncio.AbstractEventLoop
    _interpreter: MeasurementInterpreter

    def __init__(
        self,
        url: str,
        loop: asyncio.AbstractEventLoop,
    ):
        """Initializes the MeasurementInterpreter and stores communication information."""
        sync_sender = InterpreterSyncSender(self.send_command, loop=loop)
        self._interpreter = MeasurementInterpreter(sync_sender=sync_sender)
        self._url = url
        self._loop = loop

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
            unpacked_configuration = json.loads(configuration)
            assert isinstance(unpacked_configuration, dict)
            measurement_request = MeasurementRequest.from_json(request)
            await self.log("Measurement unpacked, processing ....")
            self._interpreter.process_request(
                request=measurement_request,
                configuration=unpacked_configuration,
                id=id,
            )
        except Exception as e:
            await self.log(f"Error processing request: {e}")

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
            id = data.get(INTERPRETER_RUNTIME_COMMANDS.PROCESS_DATA.PROCESS_ID)
            await self.log("Data unpacked, processing ....")
            self._interpreter.process_data(data=instrument_data, id=id)
        except Exception as e:
            await self.log(f"Error processing data: {e}")
