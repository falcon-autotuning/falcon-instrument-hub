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
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

    @property
    def GETTERS(self) -> str:
        """the connections that are ready to be measured"""
        return "getters"

    @property
    def SETTERS(self) -> str:
        """the connections that are to be set when buffered"""
        return "setters"

    @property
    def PROCESS_ID(self) -> str:
        """A unique identifier for the process/ measurement and can index it"""
        return "process_id"

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

class CONFIRM_INITIALIZATION:
    """The substrings necessary for confirm initialization of a daemon and provide configuration."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "CONFIRM_INITIALIZATION"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

    @property
    def INIT(self) -> str:
        """the configuration of the daemon, property_name and index indexed"""
        return "init"

    @property
    def PORT(self) -> str:
        """the configuration of the instrument ports"""
        return "port"

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

    @property
    def INDEX(self) -> str:
        """The particular index of a instrument that is to be set"""
        return "index"

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

class SETUP_INSTRUMENT:
    """The substrings necessary for sets up an instrument on a instrument server."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "SETUP_INSTRUMENT"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

    @property
    def NAME(self) -> str:
        """the name of the instrument to startup"""
        return "name"

class DESTROY_INSTRUMENT:
    """The substrings necessary for shuts down an instrument on a instrument server."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "DESTROY_INSTRUMENT"

    @property
    def NAME(self) -> str:
        """the name of the instrument to stop"""
        return "name"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

class PERFORM_INSTRUMENT_METHOD:
    """The substrings necessary for enact an arbitrary submethod for a given instrument daemon from the cli."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "PERFORM_INSTRUMENT_METHOD"

    @property
    def INSTRUMENT(self) -> str:
        """Which instrument are we communicating with?"""
        return "instrument"

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

class BUSY:
    """The substrings necessary for if a process is currently running an action right now."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "BUSY"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

class PORT_REQUEST:
    """The substrings necessary for request all current instrument ports."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "PORT_REQUEST"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

class PORT_PAYLOAD:
    """The substrings necessary for all of the current instrument ports."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "PORT_PAYLOAD"

    @property
    def KNOBS(self) -> str:
        """All of the knobs attached to the instrument server"""
        return "knobs"

    @property
    def METERS(self) -> str:
        """All of the meters attached to the instrument server"""
        return "meters"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

class DEVICE_CONFIG_REQUEST:
    """The substrings necessary for a request for the device configuration."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "DEVICE_CONFIG_REQUEST"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

class DEVICE_CONFIG_RESPONSE:
    """The substrings necessary for a response containing the device configuration."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "DEVICE_CONFIG_RESPONSE"

    @property
    def RESPONSE(self) -> str:
        """The device config for use understanding the device layout"""
        return "response"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

class MEASURE_COMMAND:
    """The substrings necessary for issued to runtime to request a measurement from the instrument server."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "MEASURE_COMMAND"

    @property
    def HASH(self) -> str:
        """the hash for the requesting unit"""
        return "hash"

    @property
    def REQUEST(self) -> str:
        """the measurement request to be taken"""
        return "request"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

class MEASURE_RESPONSE:
    """The substrings necessary for recieve a response from the runtime as to the measurement performed."""

    @property
    def COMM_CHANNEL(self) -> str:
        """This is the communication channel to issue the command on."""
        return "MEASURE_RESPONSE"

    @property
    def RESPONSE(self) -> str:
        """the measurement response containing the information from the server"""
        return "response"

    @property
    def TIMESTAMP(self) -> str:
        """When the response was completed"""
        return "timestamp"

    @property
    def HASH(self) -> str:
        """the hash for the requesting unit"""
        return "hash"


class RUNTIME_COMMANDS:
    """All of the various runtime commands that a compiler may use."""

    LOG = LOG()
    MEASUREMENT_READY = MEASUREMENT_READY()
    PROCESS_DATA = PROCESS_DATA()
    PROCESS_REQUEST = PROCESS_REQUEST()
    STATUS = STATUS()
    UPDATE_DAEMON_PROPERTY = UPDATE_DAEMON_PROPERTY()
    UPLOAD_DATA = UPLOAD_DATA()
    CONFIRM_INITIALIZATION = CONFIRM_INITIALIZATION()
    GET = GET()
    PERFORM_ARBITRARY_METHOD = PERFORM_ARBITRARY_METHOD()
    RETURN_DATA = RETURN_DATA()
    RETURN_GET = RETURN_GET()
    SET = SET()
    TRIGGER = TRIGGER()
    SETUP_INSTRUMENT = SETUP_INSTRUMENT()
    DESTROY_INSTRUMENT = DESTROY_INSTRUMENT()
    PERFORM_INSTRUMENT_METHOD = PERFORM_INSTRUMENT_METHOD()
    BUSY = BUSY()
    PORT_REQUEST = PORT_REQUEST()
    PORT_PAYLOAD = PORT_PAYLOAD()
    DEVICE_CONFIG_REQUEST = DEVICE_CONFIG_REQUEST()
    DEVICE_CONFIG_RESPONSE = DEVICE_CONFIG_RESPONSE()
    MEASURE_COMMAND = MEASURE_COMMAND()
    MEASURE_RESPONSE = MEASURE_RESPONSE()
