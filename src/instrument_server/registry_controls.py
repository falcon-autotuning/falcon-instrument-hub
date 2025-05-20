"""Controls for the daemon registry."""

from ..instrument_server import _daemon_registry


def add_daemon(
    daemon_name: str,
    daemon_class: type,
) -> None:
    """Adds a daemon to the registry.

    Args:
        daemon_name: The name of the daemon.
        daemon_class: The class of the daemon.
    """
    _daemon_registry[daemon_name] = daemon_class


def find_daemon(
    daemon_name: str,
) -> type:
    """Finds a daemon in the registry.

    Args:
        daemon_name: The name of the daemon.

    Returns:
        The class of the daemon.
    """
    return _daemon_registry[daemon_name]
