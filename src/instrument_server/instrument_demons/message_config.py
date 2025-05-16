"""Holds the message config for an instrument demon."""


class MessageConfig:
    """The API for receiving messages."""

    _url: str
    _timeout_ping: float

    def __init__(
        self,
        url: str,
        timeout_ping: float = 1.0,
    ):
        """Initialize the message config.

        Args:
            url : the url to connect to the nats instance.
            timeout_ping : the timeout for receiving messages in seconds
        """
        self._url = url
        self._timeout_ping = timeout_ping

    @property
    def url(self) -> str:
        """Returns the url for the nats instance."""
        return self._url

    @property
    def timeout_ping(self) -> float:
        """Returns the number of seconds to wait for a message."""
        return self._timeout_ping
