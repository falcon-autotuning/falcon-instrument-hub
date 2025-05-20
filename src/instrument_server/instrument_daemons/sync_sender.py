"""A synchrnous sending handler attachment."""

import asyncio
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable


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
        future = asyncio.run_coroutine_threadsafe(
            self._async_send_func(channel, message), self._loop
        )
        # Block until the message is sent
        future.result()
