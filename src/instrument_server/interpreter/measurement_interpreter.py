"""A measurement interpreter for the instrument server."""

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .typing import (
        ID,
        DriverConfig,
        InterpreterSyncSender,
        MeasurementRequest,
    )


class MeasurementInterpreter:
    """A class that interprets measurements from the instrument server.

    Following the rules specified by FAlCon, this processes the requests,
    prepares the instruments for the measurements, deploys to the server,
    recovers the raw data, processes it, and send it to the database for
    storage.
    """

    _sync_sender: "InterpreterSyncSender"

    def __init__(
        self,
        sync_sender: "InterpreterSyncSender",
    ) -> None:
        """Initializes the MeasurementInterpreter."""
        self._sync_sender = sync_sender

    def process_request(
        self,
        request: "MeasurementRequest",
        configuration: dict[str, "DriverConfig"],
        id: "ID",
    ) -> None:
        """Processes the measurement request.

        Args:
            request: The measurement request to process.
            id: The ID of the request.
        """
