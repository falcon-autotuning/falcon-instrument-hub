"""A buffered current sink instrument driver."""

from .constants import SUPPORTED_PROPERTIES
from .dc_current_sink import DCCurrentSink


class BufferedCurrentSink(DCCurrentSink):
    """A buffered current sink driver."""

    def __init__(self, *args, **kwargs) -> None:
        """Initialize the buffered current sink driver."""
        super().__init__(*args, **kwargs)
        self.program_property(
            property_name=SUPPORTED_PROPERTIES.SUPPORTS_BUFFERED_MEASUREMENTS,
            index=self._global_index,
            get_cmd=lambda: True,
        )
