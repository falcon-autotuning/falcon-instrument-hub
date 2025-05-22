"""A synchrnous sending handler attachment."""

from typing import TYPE_CHECKING

from .dependancies import asyncio

if TYPE_CHECKING:
    from .typing import Awaitable, Callable


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
