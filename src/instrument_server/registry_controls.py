"""Controls for the daemon registry."""

from . import _driver_registry


def add_driver(
    daemon_name: str,
    daemon_class: type,
) -> None:
    """Adds a daemon to the registry.

    Args:
        daemon_name: The name of the daemon.
        daemon_class: The class of the daemon.
    """
    _driver_registry[daemon_name] = daemon_class


def find_driver(
    driver_name: str,
) -> type:
    """Finds a driver in the registry.

    Args:
        driver_name: The name of the driver.

    Returns:
        The class of the driver.
    """
    return _driver_registry[driver_name]
