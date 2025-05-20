"""A base property is something that can be set for each subunit of a daemon."""

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .typing import Bounds, GetCommand, SetCommand


class BaseProperty:
    """A base property is something that can be set for each subunit of a daemon."""

    _set_cmd: "SetCommand | None" = None
    _get_cmd: "GetCommand"
    _settable: bool = False
    _bounds: tuple[int | float, int | float]

    def __init__(
        self,
        get_cmd: "GetCommand",
        set_cmd: "SetCommand | None" = None,
        bounds: tuple[int | float, int | float] | None = None,
    ):
        """Initialize the base property.

        Args:
            get_cmd: The command to get the property.
            set_cmd: The command to set the property.
            bounds: Optional bounds of the property. This is a tuple of (min, max).

        """
        self._get_cmd = get_cmd
        self._set_cmd = set_cmd
        self._settable = set_cmd is not None
        self._bounds = bounds if bounds is not None else self._bounds

    @property
    def get_cmd(self) -> "GetCommand":
        """Gets the property from the instrument inside."""
        return self._get_cmd

    @property
    def set_cmd(
        self,
    ) -> "SetCommand":
        """Sets the property onto the instument inside.

        Args:
            value: The value to set the property to.

        Raises:
            AttributeError: If the property is not settable.
        """
        if self._settable:
            assert self._set_cmd is not None
            return self._set_cmd
        msg = "This property is not settable."
        raise AttributeError(msg)

    @property
    def bounds(self) -> "Bounds":
        """Gets the bounds of the property.

        Returns:
            The bounds of the property.

        """
        return self._bounds

    def _to_json(self) -> dict[str, "bool | Bounds"]:
        """Convert the property to a JSON serializable dictionary.

        Returns:
            A dictionary representation of the property.

        """
        return {
            "bounds": self._bounds,
            "settable": self._settable,
        }
