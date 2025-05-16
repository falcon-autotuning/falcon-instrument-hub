class BASE_COMMAND:
    """Contains the substrings for a base command."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        msg = "This is an abstract base class. Must implement in subclass."
        raise NotImplementedError(msg)


class LOG(BASE_COMMAND):
    """Contains the substrings for a log command."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "LOG"

    @property
    def MESSAGE(self) -> str:
        """This is the message to issue the command."""
        return "message"


class RESPONSE(BASE_COMMAND):
    """A response contains a timestamp."""

    @property
    def TIMESTAMP(self) -> str:
        """This is the timestamp when the response was completed."""
        return "timestamp"


class ACTIVATE_MEASURE(BASE_COMMAND):
    """Contains the substrings necessary to activate a measurement."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "ACTIVATE_MEASURE"

    @property
    def REQUEST(self) -> str:
        """This is the request to issue the command.

        This message is a Jsonable string.
        """
        return "request"


class MEASURE_RESPONSE(BASE_COMMAND):
    """Contains the substrings necessary to provide a measurement result."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "MEASURE_RESPONSE"

    @property
    def RESPONSE(self) -> str:
        """This is the response to issue the command.

        This message is a Jsonable string.
        """
        return "response"


class RUNTIME_COMMANDS:
    """All of the various runtime commands that an instrument needs to support."""

    LOG = LOG()
    RECEIVE_PING = RECEIVE_PING()
    SET = SET()
    GET = GET()
    RETURN_GET = RETURN_GET()
    RETURN_DATA = RETURN_DATA()
