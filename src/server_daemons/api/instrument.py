"""Constants for the daemons contained for the instrument server."""

class CONFIRM_INITIALIZATION:
    """The substrings necessary for confirm initialization of a daemon and provide configuration."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "CONFIRM_INITIALIZATION"

    @property
    def INIT(self) -> str:
        """the configuration of the daemon, property_name and index indexed"""
        return "init"

    @property
    def PORT(self) -> str:
        """the configuration of the instrument ports"""
        return "port"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

class GET:
    """The substrings necessary for execute a get instruction on a sandboxed instrument."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "GET"

    @property
    def PROPERTY(self) -> str:
        """The name of the property that is to be set"""
        return "property"

    @property
    def INDEX(self) -> str:
        """The particular index of a instrument that is to be set"""
        return "index"

class LOG:
    """The substrings necessary for contains the necessary substrings for a logging style command."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "LOG"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

    @property
    def HASH(self) -> str:
        """the hash for the requesting unit"""
        return "hash"

    @property
    def MESSAGE(self) -> str:
        """The contents of the log message"""
        return "message"

class PERFORM_ARBITRARY_METHOD:
    """The substrings necessary for enact an arbitrary submethod for a given instrument daemon from the cli."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "PERFORM_ARBITRARY_METHOD"

    @property
    def METHOD(self) -> str:
        """The name of the method that is to be performed"""
        return "method"

    @property
    def KEYWORD_ARGS(self) -> str:
        """Arbitrary keyword arguments to be passes to the method"""
        return "keyword_args"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

class RETURN_DATA:
    """The substrings necessary for returns measured data."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "RETURN_DATA"

    @property
    def DATA(self) -> str:
        """The measured data collected on the instrument"""
        return "data"

    @property
    def PROPERTY(self) -> str:
        """The name of the property that is to be set"""
        return "property"

    @property
    def INDEX(self) -> str:
        """The particular index of a instrument that is to be set"""
        return "index"

class RETURN_GET:
    """The substrings necessary for response from a get instruction on a sandboxed instrument."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "RETURN_GET"

    @property
    def INDEX(self) -> str:
        """The particular index of a instrument that is to be set"""
        return "index"

    @property
    def VALUE(self) -> str:
        """The argument to be set inside the instrument"""
        return "value"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

    @property
    def PROPERTY(self) -> str:
        """The name of the property that is to be set"""
        return "property"

class SET:
    """The substrings necessary for execute a set instruction on a sandboxed instrument."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "SET"

    @property
    def PROPERTY(self) -> str:
        """The name of the property that is to be set"""
        return "property"

    @property
    def INDEX(self) -> str:
        """The particular index of a instrument that is to be set"""
        return "index"

    @property
    def VALUE(self) -> str:
        """The argument to be set inside the instrument"""
        return "value"

class STATUS:
    """The substrings necessary for provide the status of the process."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "STATUS"

    @property
    def STATUS(self) -> str:
        """At compilation of this message the state of the process"""
        return "status"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

class ARMED:
    """The substrings necessary for statement from an instrument indicating sets are complete and it is locked from further modifications.."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "ARMED"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

class TRIGGER:
    """The substrings necessary for execute a trigger/arm on a buffered instrument."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "TRIGGER"

    @property
    def PROPERTY(self) -> str:
        """The name of the property that is to be set"""
        return "property"

    @property
    def INDEX(self) -> str:
        """The particular index of a instrument that is to be set"""
        return "index"

class EXECUTING:
    """The substrings necessary for statement from an instrument indicating it is successfully triggered and executing a measurement.."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "EXECUTING"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"


class RUNTIME_COMMANDS:
    """All of the various runtime commands that a compiler may use."""

    CONFIRM_INITIALIZATION = CONFIRM_INITIALIZATION()
    GET = GET()
    LOG = LOG()
    PERFORM_ARBITRARY_METHOD = PERFORM_ARBITRARY_METHOD()
    RETURN_DATA = RETURN_DATA()
    RETURN_GET = RETURN_GET()
    SET = SET()
    STATUS = STATUS()
    ARMED = ARMED()
    TRIGGER = TRIGGER()
    EXECUTING = EXECUTING()
