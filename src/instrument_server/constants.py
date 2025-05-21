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


class RETURN_DATA(BASE_COMMAND):
    """Contains the substrings necessary to return measured data."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "MEASURE_RESPONSE"

    @property
    def DATA(self) -> str:
        """This is the response to issue the command.

        This message is a Jsonable string.
        """
        return "response"


class SET(BASE_COMMAND):
    """Contains the substrings necessary to arbitrate a set instruction to a instrument daemon."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "SET"

    @property
    def PROPERTY(self) -> str:
        """This is the name of the property that is to be set."""
        return "property"

    @property
    def INDEX(self) -> str:
        """This is the particular index of the daemon that is to be set."""
        return "index"

    @property
    def VALUE(self) -> str:
        """This is the value that is to be set."""
        return "value"


class GET(BASE_COMMAND):
    """Contains the substrings necessary to arbitrate a get instruction to a instrument daemon."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "GET"

    @property
    def PROPERTY(self) -> str:
        """This is the name of the property that is to be got."""
        return "property"

    @property
    def INDEX(self) -> str:
        """This is the particular index of the daemon that is to be got."""
        return "index"


class RETURN_GET(RESPONSE):
    """Contains the response to the GET command."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "RETURN_GET"

    @property
    def VALUE(self) -> str:
        """This is the value that is to be returned."""
        return "value"


class PERFORM_ARBITRARY_METHOD(RESPONSE):
    """Contains the substrings necessary to enact an arbitrary submethod for a given instrument daemon from the CLI."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "PERFORM_ARBITRARY_METHOD"

    @property
    def METHOD(self) -> str:
        """This is the name of the method that is to be performed."""
        return "method"

    @property
    def KEYWORD_ARGS(self) -> str:
        """This is the keyword arguments that are to be passed to the method.

        This is a json dictionary of str|int|float|None
        """
        return "keyword_args"


class CONFIRM_INITIALIZATION(RESPONSE):
    """The substrings necessary to confirm the initialization of a daemon."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "CONFIRM_INITIALIZATION"

    @property
    def INIT(self) -> str:
        """This is the initialization message to issue the command.

        this is a json payload containing the configuration of the daemon
        """
        return "init"


class TRIGGER(BASE_COMMAND):
    """The substrings necessary to trigger a buffered instrument."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "TRIGGER"

    @property
    def TRIGGER_PORT(self) -> str:
        """This is the port to send/recieve a trigger pulse on.

        Collectors receive the trigger pulse on this port.
        Senders send the trigger pulse on this port.
        """
        return "trigger_port"


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


class DAEMON_RUNTIME_COMMANDS:
    """All of the various runtime commands that an instrument needs to support."""

    LOG = LOG()
    SET = SET()
    GET = GET()
    RETURN_GET = RETURN_GET()
    CONFIRM_INITIALIZATION = CONFIRM_INITIALIZATION()
    RETURN_DATA = RETURN_DATA()
    PERFORM_ARBITRARY_METHOD = PERFORM_ARBITRARY_METHOD()
    TRIGGER = TRIGGER()  # used to trigger buffered instruments
    STATUS = STATUS()


class SUPPORTED_PROPERTIES:
    """All of the various properties that an instrument daemon could need to support."""

    # voltage source instruments
    VOLTAGE_STATE = "voltage_state"
    SLOPE = "slope"

    # signal recovery instruments
    CURRENT_STATE = "current_state"
    NUMBER_OF_BINS = "number_of_bins"
    SAMPLE_RATE = "sample_rate"
    TIMEOUT = "timeout"

    # buffered instruments
    # combo parameter: step_width, num_steps, repeats, v_stop
    STAIRCASE = "staircase"
    # TODO: continue generalizing properties. such as AC control, other DC control, etc.

    # buffered instrument global controls
    LEADER = "leader"  # boolean, if not true then this is a follower

    # buffered instrument constants
    SUPPORTS_ARBITRARY_SCALING = "arbitrary_scaling"
    SUPPORTS_ARBITRARY_OFFSET = "arbitrary_offset"
    NUMBER_SIMULTANEOUS_WAVEFORMS = "num_sim_waveforms"
    MAXIMUM_WAVEFORM_EXTENT = "maximum_wavefrom_extent"


class INTERPRETER_RUNTIME_COMMANDS:
    """All of the various runtime commands that a compiler may use."""

    LOG = LOG()
    REQUEST_DAEMON_CONFIGURATIONS = REQUEST_DAEMON_CONFIGURATIONS()
    UPDATE_DAEMON_PROPERTY = UPDATE_DAEMON_PROPERTY()
    PROCESS_REQUEST = PROCESS_REQUEST()
    MEASUREMENT_READY = MEASUREMENT_READY()
    PROCESS_DATA = PROCESS_DATA()
    UPLOAD_DATA = UPLOAD_DATA()
