"""Controls for the daemon registry."""

from . import _driver_registry


def add_driver(
    driver: str,
    driver_class: type,
) -> None:
    """Adds a driver to the registry.

    Args:
        driver_name: The name of the driver.
        driver_class: The class of the driver.
    """
    _driver_registry[driver] = driver_class


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
