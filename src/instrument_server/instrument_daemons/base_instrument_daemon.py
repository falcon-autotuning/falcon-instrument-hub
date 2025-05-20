"""This is the base instrument demon that holds and communicates the needs for all instruments."""

import json
from typing import TYPE_CHECKING

from ..constants import RUNTIME_COMMANDS
from .base_property import BaseProperty
from .indexed_properties import IndexedProperties
from .sync_sender import SyncSender

if TYPE_CHECKING:
    from .typing import Bounds, GetCommand, Index, PropertyName, SetCommand


class BaseInstrumentDaemon:
    """Handles the communication for an instrument.

    This daemon assumes that all instruments are setup with modular repeated components. The user supplies many indexes for the many repeated parts, each with their own custom properties.
    """

    _properties: dict["PropertyName", "IndexedProperties"]
    _sync_sender: "SyncSender"

    def __init__(self, sync_sender: SyncSender):
        """When instancing this subclass, make sure to specify the program_property method for each property of the daemon.

        Args:
            SyncSender: a synchronous message sender for the daemon.
        """
        self._sync_sender = sync_sender

    @property
    def sync_sender(self) -> "SyncSender":
        """The synchronous message sender for the daemon."""
        return self._sync_sender

    @property
    def properties(self) -> dict["PropertyName", "IndexedProperties"]:
        """The collection of properties for the daemon."""
        return self._properties

    def program_property(
        self,
        property_name: "PropertyName",
        index: "Index",
        get_cmd: "GetCommand",
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
        if not hasattr(self, "_properties"):
            self._properties = {}

        if property_name not in self._properties:
            self._properties[property_name] = IndexedProperties()

        prop = BaseProperty(
            get_cmd=get_cmd,
            set_cmd=set_cmd,
            bounds=bounds,
        )
        self._properties[property_name][index] = prop

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
        value = self._properties[property_name][index].get_cmd()
        message = json.dumps(
            {
                RUNTIME_COMMANDS.RETURN_GET.VALUE: value,
            }
        )
        self.sync_sender.send(
            RUNTIME_COMMANDS.RETURN_GET.COMM_CHANNEL,
            message,
        )

    def to_json_config(
        self,
    ) -> dict[str, dict[str, dict[str, dict[str, "bool | Bounds"]]]]:
        """Convert the properties to a JSON serializable format.

        Returns:
            A dictionary of the properties.
        """
        return {
            "properties": {
                prop_name: prop._to_json()
                for prop_name, prop in self._properties.items()
            }
        }
