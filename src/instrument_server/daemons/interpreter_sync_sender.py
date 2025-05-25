"""A special sync sender for the measurement interpreter."""

from typing import TYPE_CHECKING

from .constants import INTERPRETER_RUNTIME_COMMANDS
from .dependancies import json, time
from .sync_sender import SyncSender

if TYPE_CHECKING:
    from .dependancies import MeasurementResponse
    from .typing import (
        ID,
        Connection,
        Index,
        PropertyName,
        PropertyValue,
    )


class InterpreterSyncSender(SyncSender):
    """A synchronous sender for the measurement interpreter."""

    def log(self, msg: str) -> None:
        """Log a message to the NATS server.

        Args:
            msg: The message to log.
        """
        self._sync_send(
            channel=INTERPRETER_RUNTIME_COMMANDS.LOG.COMM_CHANNEL,
            message=msg,
        )

    def deploy_measurement(
        self,
        id: "ID",
        setters: dict["PropertyName", "Index"],
        getters: dict["PropertyName", "Index"],
    ) -> None:
        """Deploys a measurement to the local runtime server.

        Args:
            id: The ID of the measurement.
            setters: The setters to deploy.
            getters: The getters to deploy.
        """
        message = json.dumps(
            {
                INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.PROCESS_ID: id,
                INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.SETTERS: setters,
                INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.GETTERS: getters,
            }
        )

        self._sync_send(
            channel=INTERPRETER_RUNTIME_COMMANDS.MEASUREMENT_READY.COMM_CHANNEL,
            message=message,
        )

    def update_daemon_property(
        self,
        property: "PropertyName",
        name: "Connection",
        value: "PropertyValue",
    ) -> None:
        """Updates a property in a daemon.

        Args:
            property: The property to update.
            name: The name of the connection or instrument.
            value: The value to set.
        """
        message = json.dumps(
            {
                INTERPRETER_RUNTIME_COMMANDS.UPDATE_DAEMON_PROPERTY.PROPERTY: property,
                INTERPRETER_RUNTIME_COMMANDS.UPDATE_DAEMON_PROPERTY.NAME: name.name,
                INTERPRETER_RUNTIME_COMMANDS.UPDATE_DAEMON_PROPERTY.VALUE: value,
            }
        )
        self._sync_send(
            channel=INTERPRETER_RUNTIME_COMMANDS.UPDATE_DAEMON_PROPERTY.COMM_CHANNEL,
            message=message,
        )

    def upload_data(
        self,
        data: "MeasurementResponse",
    ) -> None:
        """Uploads the interpreted data to the server for storing in the database.

        Args:
            data: The data to upload.
        """
        message = json.dumps(
            {
                INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.DATA: data.to_json(),
                INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.TIMESTAMP: str(time.time()),
            }
        )
        self._sync_send(
            channel=INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.COMM_CHANNEL,
            message=message,
        )
