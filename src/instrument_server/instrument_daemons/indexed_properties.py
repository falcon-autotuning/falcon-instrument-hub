"""A collection of indexed properties for a daemon."""

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .base_property import BaseProperty
    from .typing import Bounds, Index


class IndexedProperties:
    """A collection of indexed properties for a daemon."""

    _values: dict["Index", "BaseProperty"]

    def __init__(self, properties: dict["Index", "BaseProperty"] = {}):
        """Initialize the IndexedProperties class."""
        self._values = properties

    @property
    def properties(self) -> dict["Index", "BaseProperty"]:
        """Return the properties."""
        return self._values

    def __getitem__(self, index: "Index") -> "BaseProperty":
        """Get an indexed property."""
        return self._values[index]

    def __setitem__(self, index: "Index", value: "BaseProperty") -> None:
        """Set an indexed property."""
        self._values[index] = value

    def __iter__(self):
        """Iterate over the properties."""
        yield from self._values.items()

    def __len__(self) -> int:
        """Get the number of properties."""
        return len(self._values)

    def index(self, index: "Index") -> bool:
        """Check if an index is in the properties."""
        return index in self._values

    def indices(self) -> list["Index"]:
        """Get a list of indices."""
        return list(self._values.keys())

    def _to_json(self) -> dict["Index", dict[str, "bool | Bounds"]]:
        """Convert the properties to a JSON serializable format.

        Returns:
            A dictionary of the properties.
        """
        return {index: prop._to_json() for index, prop in self._values.items()}
