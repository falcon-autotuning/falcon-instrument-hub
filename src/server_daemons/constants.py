"""Constants for the dawmons contained for the instrument server."""


class BASE_COMMAND:
    """Contains the substrings for a base command."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        msg = "This is an abstract base class. Must implement in subclass."
        raise NotImplementedError(msg)


class RESPONSE(BASE_COMMAND):
    """A response contains a timestamp."""

    @property
    def TIMESTAMP(self) -> str:
        """This is the timestamp when the response was completed."""
        return "timestamp"


class LOG(RESPONSE):
    """Contains the substrings for a log command."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "LOG"

    @property
    def MESSAGE(self) -> str:
        """This is the message to issue the command."""
        return "message"


class STATUS(RESPONSE):
    """The substrings necessary to provide the status of the instrument daemon."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "STATUS"

    @property
    def STATUS(self) -> str:
        """This is the status of the instrument daemon.

        This is boolean.
        """
        return "status"


class UPDATE_DAEMON_PROPERTY(RESPONSE):
    """The substrings necessary to update the daemon property."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "UPDATE_DAEMON_PROPERTY"

    @property
    def PROPERTY(self) -> str:
        """This is the property that is to be updated."""
        return "property"

    @property
    def NAME(self) -> str:
        """This is the name of the property that is to be updated."""
        return "name"

    @property
    def VALUE(self) -> str:
        """This is the value of the property that is to be updated."""
        return "value"


class PROCESS_REQUEST(BASE_COMMAND):
    """The substrings necessary to process a request."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "PROCESS_REQUEST"

    @property
    def REQUEST(self) -> str:
        """This is the request to issue the command.

        This message is a Jsonable string.
        """
        return "request"

    @property
    def CONFIGURATIONS(self) -> str:
        """This is the configurations of the daemon.

        This is a json payload containing the configuration of the daemons
        """
        return "configurations"

    @property
    def DATA_PATH(self) -> str:
        """The data directory to save the data to."""
        return "data_path"

    @property
    def PROCESS_ID(self) -> str:
        """A unique identifier for the process that is to be processed."""
        return "process_id"


class MEASUREMENT_READY(RESPONSE):
    """The substrings necessary to indicate that a measurement is ready."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "MEASUREMENT_READY"

    @property
    def GETTERS(self) -> str:
        """This is the getters that are ready to be measured.

        this is json containing a dictionary of properties indexed by names for each connection that needs to be got
        """
        return "getters"

    @property
    def PROCESS_ID(self) -> str:
        """A unique identifier for the process that is to be processed."""
        return "process_id"


class PROCESS_DATA(RESPONSE):
    """The substrings necessary to process data."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "PROCESS_DATA"

    @property
    def DATA(self) -> str:
        """This is the data to issue the command.

        Regardless of measurement type, this is a json object.
        The dictionary contains keys, which are InstrumentPort from the Meters.
        If buffered measurement the values are Jsonable array.
        If not buffered, the values are a data point.
        """
        return "data"

    @property
    def PROCESS_ID(self) -> str:
        """A unique identifier for the process that is to be processed."""
        return "process_id"


class UPLOAD_DATA(RESPONSE):
    """The substrings necessary to probe runtime to load the data into the database and handoff to FAlCon.

    The interpreter will upload the data to the database independantly.
    """

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "UPLOAD_DATA"

    @property
    def DATA(self) -> str:
        """This is the data to issue the command.

        This is Jsonable MeasurementResponse.
        """
        return "data"


class INTERPRETER_RUNTIME_COMMANDS:
    """All of the various runtime commands that a compiler may use."""

    LOG = LOG()
    UPDATE_DAEMON_PROPERTY = UPDATE_DAEMON_PROPERTY()
    PROCESS_REQUEST = PROCESS_REQUEST()
    MEASUREMENT_READY = MEASUREMENT_READY()
    PROCESS_DATA = PROCESS_DATA()
    UPLOAD_DATA = UPLOAD_DATA()
    STATUS = STATUS()
