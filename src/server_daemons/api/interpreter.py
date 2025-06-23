"""Constants for the daemons contained for the instrument server."""

class LOG:
    """The substrings necessary for contains the necessary substrings for a logging style command."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "LOG"

    @property
    def MESSAGE(self) -> str:
        """The contents of the log message"""
        return "message"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

    @property
    def HASH(self) -> str:
        """the hash for the requesting unit"""
        return "hash"

class MEASUREMENT_READY:
    """The substrings necessary for indicates that a meassurement is ready for the server to perform."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "MEASUREMENT_READY"

    @property
    def BUFFERED(self) -> str:
        """if this is a buffered measurement or not"""
        return "buffered"

    @property
    def PROCESS_ID(self) -> str:
        """A unique identifier for the process/ measurement and can index it"""
        return "process_id"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

    @property
    def GETTERS(self) -> str:
        """the connections that are ready to be measured"""
        return "getters"

    @property
    def SETTERS(self) -> str:
        """the connections that are to be set for the measurement"""
        return "setters"

    @property
    def REQUIREMENTS(self) -> str:
        """the instruments ports that are required to be set"""
        return "requirements"

class PROCESS_DATA:
    """The substrings necessary for used by interpreter to handle the need to collect some data."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "PROCESS_DATA"

    @property
    def DATA(self) -> str:
        """the data taken from the instruments for interpretation"""
        return "data"

    @property
    def PROCESS_ID(self) -> str:
        """A unique identifier for the process/ measurement and can index it"""
        return "process_id"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

class PROCESS_REQUEST:
    """The substrings necessary for a request to the interpreter to process an incoming measurement."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "PROCESS_REQUEST"

    @property
    def REQUEST(self) -> str:
        """The measurement request from FAlCon"""
        return "request"

    @property
    def CONFIGURATIONS(self) -> str:
        """The configurations of the instruments loaded into the instrument server"""
        return "configurations"

    @property
    def DATA_PATH(self) -> str:
        """The filepath to the spot in the HDF5 database to store the collected data at"""
        return "data_path"

    @property
    def PROCESS_ID(self) -> str:
        """A unique identifier for the process/ measurement and can index it"""
        return "process_id"

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

class UPDATE_DAEMON_PROPERTY:
    """The substrings necessary for issued to selectively update an instruments property in a daemon."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "UPDATE_DAEMON_PROPERTY"

    @property
    def VALUE(self) -> str:
        """The quantity"""
        return "value"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

    @property
    def PROPERTY(self) -> str:
        """The main subclass of property"""
        return "property"

    @property
    def NAME(self) -> str:
        """The human readable name from FAlCon to the wiremap, or at the very least a instrument type if unique"""
        return "name"

class UPLOAD_DATA:
    """The substrings necessary for used by the interpreter to hand data off the the runtime for falcon."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "UPLOAD_DATA"

    @property
    def DATA(self) -> str:
        """the jsonable measurement request for the FAlCon to unpack and use"""
        return "data"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"


class RUNTIME_COMMANDS:
    """All of the various runtime commands that a compiler may use."""

    LOG = LOG()
    MEASUREMENT_READY = MEASUREMENT_READY()
    PROCESS_DATA = PROCESS_DATA()
    PROCESS_REQUEST = PROCESS_REQUEST()
    STATUS = STATUS()
    UPDATE_DAEMON_PROPERTY = UPDATE_DAEMON_PROPERTY()
    UPLOAD_DATA = UPLOAD_DATA()
