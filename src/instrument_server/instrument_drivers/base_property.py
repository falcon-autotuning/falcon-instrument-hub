"""A base property is something that can be set for each subunit of a daemon."""

from typing import TYPE_CHECKING

from .dependancies import Units

if TYPE_CHECKING:
    from .typing import Bounds, GetCommand, PropertyJson, SetCommand, SymbolUnit


class BaseProperty:
    """A base property is something that can be set for each subunit of a daemon."""

    _set_cmd: "SetCommand | None" = None
    _get_cmd: "GetCommand | None" = None
    _settable: bool = False
    _gettable: bool = False
    _bounds: tuple[int | float, int | float]
    _unit: "SymbolUnit" = Units.DIMENSIONLESS

    def __init__(
        self,
        get_cmd: "GetCommand | None" = None,
        set_cmd: "SetCommand | None" = None,
        bounds: tuple[int | float, int | float] | None = None,
        unit: "SymbolUnit" = Units.DIMENSIONLESS,
    ):
        """Initialize the base property.

        Args:
            get_cmd: The command to get the property.
            set_cmd: The command to set the property.
            bounds: Optional bounds of the property. This is a tuple of (min, max).
            unit: The unit of the property.

        """
        self._get_cmd = get_cmd
        self._set_cmd = set_cmd
        self._settable = set_cmd is not None
        self._gettable = get_cmd is not None
        self._bounds = bounds if bounds is not None else self._bounds
        self._unit = unit

    @property
    def get_cmd(self) -> "GetCommand":
        """Gets the property from the instrument inside."""
        if not self._gettable:
            msg = "This property is not gettable."
            raise AttributeError(msg)
        assert self._get_cmd is not None
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

    @property
    def unit(self) -> "SymbolUnit":
        """Gets the unit of the property.

        Returns:
            The unit of the property.

        """
        return self._unit

    def _to_json(self) -> "PropertyJson":
        """Convert the property to a JSON serializable dictionary.

        Returns:
            A dictionary representation of the property.

        """
        return {
            "bounds": self._bounds,
            "unit": self._unit.to_json(),
            "settable": self._settable,
        }
