"""This is the base instrument demon that holds and communicates the needs for all instruments."""

from typing import TYPE_CHECKING

from .base_property import BaseProperty
from .constants import SUPPORTED_PROPERTIES
from .dependancies import add_driver, threading
from .indexed_properties import IndexedProperties

if TYPE_CHECKING:
    from .typing import (
        Bounds,
        GetCommand,
        Index,
        PropertyName,
        PropertyValue,
        SetCommand,
        SyncSender,
    )


class BaseInstrumentDriver:
    """Handles the communication for an instrument.

    This driver assumes that all instruments are setup with modular repeated components. The user supplies many indexes for the many repeated parts, each with their own custom properties.

    Whenever these properties are set, the daemon will store a local copy in the cache to prevent the need to reissue the command to get the value from the instrument.
    """

    _properties: dict["PropertyName", "IndexedProperties"]
    _sync_sender: "SyncSender"
    _property_lock: threading.Lock
    _property_cache: dict["PropertyName", dict["Index", "PropertyValue"]]

    def __init_subclass__(cls):
        add_driver(
            daemon_name=cls.__name__,
            daemon_class=cls,
        )

    def __init__(self, sync_sender: "SyncSender"):
        """When instancing this subclass, make sure to specify the program_property method for each property of the daemon.

        Args:
            SyncSender: a synchronous message sender for the daemon.
        """
        self._property_lock = threading.Lock()
        self._sync_sender = sync_sender

    @property
    def properties(self) -> dict["PropertyName", "IndexedProperties"]:
        """The collection of properties for the daemon."""
        return self._properties

    def program_property(
        self,
        property_name: "PropertyName",
        index: "Index",
        get_cmd: "GetCommand | None" = None,
        bounds: "Bounds | None" = None,
        set_cmd: "SetCommand | None" = None,
    ) -> None:
        """Program a property for the daemon.

        Args:
            property_name: The name of the property.
            index: The index of the property.
            get_cmd: The command to get the property.
            set_cmd: The command to set the property.
            bounds: Optional bounds of the property. This is a tuple of (min, max).
        """
        assert property_name in [
            value
            for name, value in vars(SUPPORTED_PROPERTIES).items()
            if not name.startswith("__") and not callable(value)
        ], f"Property {property_name} is not supported by this daemon."
        if not hasattr(self, "_properties"):
            self._properties = {}

        if property_name not in self._properties:
            self._properties[property_name] = IndexedProperties()
            self._property_cache[property_name] = {}

        prop = BaseProperty(
            get_cmd=get_cmd,
            set_cmd=set_cmd,
            bounds=bounds,
        )
        self._properties[property_name][index] = prop

    def return_get(
        self,
        value: "PropertyValue",
    ) -> None:
        """Return the found value to the server.

        Args:
            value: The value to return.

        """
        self._sync_sender.return_get(
            value=value,
            daemon_name=self.__class__.__name__,
        )

    def request_arbitrary_command(
        self,
        instrument_name: str,
        method_name: str,
        keywords: dict[str, str | int | float | None],
    ) -> None:
        """Any instrument can request an arbitrary command to be performed.

        This can be used by a LEADER to make sure FOLLOWERs are in sync.

        Args:
            instrument_name: The name of the instrument.
            method_name: The name of the method to perform.
            keywords: The keyword arguments to pass to the method.

        """
        with self._property_lock:
            self._sync_sender.perform_arbitrary_method(
                daemon_name=instrument_name,
                method=method_name,
                keyword_args=keywords,
            )

    def set_property(
        self,
        property_name: "PropertyName",
        index: "Index",
        value: int | float | str,
    ) -> None:
        """Set a property for the daemon.

        Args:
            property_name: The name of the property.
            index: The index of the property.
            value: The value to set the property to.

        """
        assert self._properties[property_name][index]._settable, (
            "This property is not settable."
        )
        self._property_cache[property_name][index] = value
        with self._property_lock:
            self._properties[property_name][index].set_cmd(value)

    def get_property(
        self,
        property_name: "PropertyName",
        index: "Index",
    ):
        """Get a property for the daemon.

        Args:
            property_name: The name of the property.
            index: The index of the property.

        """
        if index not in self._property_cache[property_name]:
            with self._property_lock:
                value = self._properties[property_name][index].get_cmd()
        else:
            value = self._property_cache[property_name][index]
        self.return_get(value)

    def to_json_config(
        self,
    ) -> dict["PropertyName", dict["Index", dict[str, "bool | Bounds"]]]:
        """Convert the properties to a JSON serializable format.

        Returns:
            A dictionary of the properties.
        """
        return {
            prop_name: prop._to_json() for prop_name, prop in self._properties.items()
        }

    def process_trigger(self):
        """Triggers the instrument into motion to perform the selected action."""
        msg = "Need to implement the trigger method for this daemon."
        raise NotImplementedError(msg)
