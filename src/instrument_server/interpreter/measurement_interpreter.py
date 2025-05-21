"""A measurement interpreter for the instrument server."""

import nats


class MeasurementInterpreter:
    """A class that interprets measurements from the instrument server.

    Following the rules specified by FAlCon, this processes the requests,
    prepares the instruments for the measurements, deploys to the server,
    recovers the raw data, processes it, and send it to the database for
    storage.
    """

    _url: str
    _nc: nats.aio.client.Client

    def __init__(self, url: str):
        """Initializes the MeasurementInterpreter with the given URL."""
        self._url = url

    async def start(self):
        """Starts the measurement interpreter."""
        self._nc = await nats.connect(self._url)
