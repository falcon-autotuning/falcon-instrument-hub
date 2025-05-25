"""A data queue for the interpreter daemon."""

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from falcon_core.math.arrays.measured_array_1D import MeasuredArray1D

    from .typing import InstrumentPort, TypeAlias

Data: "TypeAlias" = dict["InstrumentPort", "MeasuredArray1D"]


class DataEntry:
    """A single entry of data into the queue.

    There are two important properties of a data entry:
        the time it was created
        the data itself
    This holds both of these properties.
    """

    _timestamp: str
    _data: Data

    def __init__(self, timestamp: str, data: Data):
        """Initialize the data entry."""
        self._timestamp = timestamp
        self._data = data

    @property
    def timestamp(self) -> str:
        """Returns the timestamp of the data entry."""
        return self._timestamp

    @property
    def data(self) -> Data:
        """Returns the data of the data entry."""
        return self._data


class DataQueue:
    """A queue for storing data from the interpreter daemon.

    This can hold multiple data sets, to make sure that all data is collected for packaging later.

    """

    _queue: list[DataEntry]

    def __init__(self, queue: list[DataEntry] = []):
        """Initialize the data queue."""
        self._queue = queue

    @property
    def queue(self) -> list[DataEntry]:
        """Returns the queue of data entries."""
        self._queue.sort(key=lambda x: x.timestamp)
        return self._queue

    def __getitem__(self, index: int) -> DataEntry:
        """Get a data entry from the queue."""
        return self.queue[index]

    def __setitem__(self, index: int, entry: DataEntry) -> None:
        """Set a data entry in the queue."""
        self.queue[index] = entry

    def __len__(self) -> int:
        """The length of the queue."""
        return len(self.queue)

    def __iter__(self):
        """Iterate over the queue."""
        return iter(self.queue)

    def append(self, entry: DataEntry) -> None:
        """Append a data entry to the queue. These entries are sorted by timestamp."""
        if not self.queue or self.queue[-1].timestamp <= entry.timestamp:
            self.queue.append(entry)
        else:
            # Scan backwards to find the correct slot (should be very close to the end)
            i = len(self.queue) - 1
            while i >= 0 and self.queue[i].timestamp > entry.timestamp:
                i -= 1
            self.queue.insert(i + 1, entry)
