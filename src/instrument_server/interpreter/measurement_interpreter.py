"""A measurement interpreter for the instrument server."""

from typing import TYPE_CHECKING

from ..constants import SUPPORTED_PROPERTIES
from .dependancies import HDF5Data

if TYPE_CHECKING:
    from .dependancies import Path
    from .typing import (
        ID,
        InstrumentPort,
        InterpreterSyncSender,
        MeasurementRequest,
        PropertyJson,
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
        configuration: dict[
            "InstrumentPort",
            "PropertyJson",
        ],
        id: "ID",
    ) -> None:
        """Processes the measurement request.

        Args:
            request: The measurement request to process.
            id: The ID of the request.
            configuration: The configuration of the instruments.

        Raises:
            RuntimeError: If no valid waveform is found.
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

        # Need to decide which mode to compile the measurement into
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
        if buffered:
            # we only support staircase right now
            pass
        else:
            pass

    def process_data(
        self,
        data: str | list[str],
        data_path: "Path",
        id: "ID",
    ) -> None:
        """Processes the current data and prepares it for upload to the database.

        Args:
            data: The data to process.
                if the data is a list, it is buffered data
                if the data is a string, it is a single measurement
            id: The ID of the request.
            data_path: The path to the spot to store the data in the database.
        """
        # TODO: somehow generate a MeasurementResponse object from the data
        response
        assert isinstance(response, MeasurementResponse), "Invalid response type"
        HDF5Data.from_communications()
