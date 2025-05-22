"""A special synchronous sender for the instrument driver."""

from typing import TYPE_CHECKING

from .constants import DAEMON_RUNTIME_COMMANDS
from .dependancies import json, time
from .sync_sender import SyncSender

if TYPE_CHECKING:
    from .typing import PropertyValue


class InstrumentSyncSender(SyncSender):
    """A synchronous sender for the instrument driver."""

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
