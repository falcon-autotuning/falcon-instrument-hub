"""A synchrnous sending handler attachment."""

from typing import TYPE_CHECKING

from .constants import DAEMON_RUNTIME_COMMANDS
from .dependancies import asyncio, json, time

if TYPE_CHECKING:
    from .typing import Awaitable, Callable, PropertyValue


class SyncSender:
    """Provides synchronous message sending capabilities for instrument daemons.

    This class ensures that async operations are run synchronously by blocking until completion,
    which is necessary for sequential instrument operations.
    """

    def __init__(
        self,
        async_send_func: "Callable[[str, str], Awaitable[None]]",
        loop: asyncio.AbstractEventLoop,
    ):
        """Initialize the synchronous sender.

        Args:
            async_send_func: An async function that sends messages to a channel.
                Expected signature: async def func(channel: str, message: str) -> None
            loop: The event loop to run coroutines on.
        """
        self._async_send_func = async_send_func
        self._loop = loop

    def _sync_send(self, channel: str, message: str) -> None:
        """Synchronously send a message to a channel.

        This method blocks until the message is sent, ensuring sequential operation.

        Args:
            channel: The channel to send the message to.
            message: The message to send.
        """

        # Create a proper coroutine wrapper to ensure we pass a coroutine to run_coroutine_threadsafe
        async def _coroutine_wrapper():
            return await self._async_send_func(channel, message)

        # Pass the genuine coroutine object to run_coroutine_threadsafe
        future = asyncio.run_coroutine_threadsafe(_coroutine_wrapper(), self._loop)
        # Block until the message is sent
        future.result()

    def log(
        self,
        msg: str,
        daemon_name: str,
    ) -> None:
        """Log a message to the NATS server.

        Args:
            message: The message to log.
            daemon_name: The name of the daemon.
        """
        message = json.dumps({DAEMON_RUNTIME_COMMANDS.LOG.MESSAGE: msg})
        self._sync_send(
            channel=DAEMON_RUNTIME_COMMANDS.LOG.COMM_CHANNEL + f".{daemon_name}",
            message=message,
        )

    def return_get(
        self,
        value: "PropertyValue",
        daemon_name: str,
    ) -> None:
        """Return a value to the NATS server.

        Args:
            value: The value to return.
            daemon_name: The name of the daemon.
        """
        message = json.dumps(
            {
                DAEMON_RUNTIME_COMMANDS.RETURN_GET.VALUE: value,
                DAEMON_RUNTIME_COMMANDS.RETURN_GET.TIMESTAMP: str(time.time()),
            }
        )
        self._sync_send(
            channel=DAEMON_RUNTIME_COMMANDS.RETURN_GET.COMM_CHANNEL + f".{daemon_name}",
            message=message,
        )

    def perform_arbitrary_method(
        self,
        method: str,
        daemon_name: str,
        keyword_args: dict[str, str | int | float | None],
    ) -> None:
        """Requests an arbitrary method to be performed on the daemon.

        Args:
            method: The name of the method to perform.
            daemon_name: The name of the daemon.
            keyword_args: The keyword arguments to pass to the method.
        """
        message = json.dumps(
            {
                DAEMON_RUNTIME_COMMANDS.PERFORM_ARBITRARY_METHOD.METHOD: method,
                DAEMON_RUNTIME_COMMANDS.PERFORM_ARBITRARY_METHOD.KEYWORD_ARGS: keyword_args,
                DAEMON_RUNTIME_COMMANDS.PERFORM_ARBITRARY_METHOD.TIMESTAMP: str(
                    time.time()
                ),
            }
        )
        self._sync_send(
            channel=DAEMON_RUNTIME_COMMANDS.PERFORM_ARBITRARY_METHOD.COMM_CHANNEL
            + f".{daemon_name}",
            message=message,
        )
