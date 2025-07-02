"""Full system integration test for collection of 2D dataset."""

from pathlib import Path
from typing import TYPE_CHECKING

import matplotlib.pyplot as plt
import numpy as np
import pytest
from falcon_core.communications.messages import MeasurementRequest
from falcon_core.constants import INSTRUMENT_TYPES
from falcon_core.instrument_interfaces.names import Knob, Knobs, Meters
from falcon_core.instrument_interfaces.port_transforms.constant_transform import (
    ConstantTransform,
)
from falcon_core.instrument_interfaces.waveforms.cartesian_waveform import (
    CartesianWaveform,
)
from falcon_core.math.axes import Axes
from falcon_core.math.discrete_spaces import CartesianDiscreteSpace
from falcon_core.math.domains import CoupledKnobDomain, KnobDomain
from falcon_core.math.spaces import CartesianSpace
from falcon_core.physics.units import Units

if TYPE_CHECKING:
    from falcon_core.communications.messages.measurement_response import (
        MeasurementResponse,
    )
    from falcon_core.instrument_interfaces.names import Meter
    from instrument_templates.typing import Index


@pytest.fixture
def human_readable_knob_names() -> list[str]:
    """Returns the human readable knob names selected."""
    return ["B3", "B4"]


@pytest.fixture
def human_readable_meter_names() -> list[str]:
    """Returns the human readable meter names selected."""
    return ["O2"]


@pytest.fixture
def intercepts(
    meterIndexes: list[int],
) -> dict[int, tuple[tuple[float, ...], ...]]:
    """The intercepts for the linear data line."""
    outs = {}
    for index in meterIndexes:
        if index == 1:
            outs[index] = ((1, 1, 0), (5, 5, 0), (1, 5, -9))
        elif index == 2:
            outs[index] = ((2, 2, 0), (6, 5, 0), (2, 6, 8))
        else:
            outs[index] = ((1, 1, 1), (6, 6, 1), (2, 2, 1))
    return outs


def plane_from_points(
    p1: tuple[float, ...],
    p2: tuple[float, ...],
    p3: tuple[float, ...],
):
    """Generate a 2D plane function from 3 points.

    Args:
        p1, p2, p3: Three points (x, y, z) that define the plane

    Returns:
        A function that takes (x, y) and returns z value on the plane
    """
    # Convert points to vectors
    np1 = np.array(p1)
    np2 = np.array(p2)
    np3 = np.array(p3)

    # Calculate plane equation: ax + by + cz + d = 0
    # Using cross product of two vectors in the plane
    v1 = np2 - np1
    v2 = np3 - np1

    # Normal vector (a, b, c) = v1 × v2
    normal = np.cross(v1, v2)
    d = np.dot(normal, np1)

    def plane_function(x: float, y: float) -> float:
        """Calculate z value for given x, y coordinates on the plane."""
        return -(normal[0] * x + normal[1] * y - d) / normal[2]

    return plane_function


@pytest.fixture
def injectionData(
    sampleRate: int,
    datapoints_time: float,
    fullPointCount: tuple[int, ...],
    intercepts: dict[int, tuple[tuple[float, ...], ...]],
    meterIndexes: list[int],
) -> dict["Index", list[float]]:
    """Returns the default injection data for the ammeter, which is empty."""
    outs: dict[Index, list[float]] = {}
    time_points_per_datapoint = int(sampleRate * datapoints_time)
    for index in meterIndexes:
        outs[index] = []
        inter0 = intercepts[index][0]
        inter1 = intercepts[index][1]
        inter2 = intercepts[index][2]
        plane = plane_from_points(inter0, inter1, inter2)
        for y in range(fullPointCount[1]):
            for x in range(fullPointCount[0]):
                # Generate a 2D linear function
                z = plane(x, y)
                z_rand = z * np.ones(time_points_per_datapoint) + np.random.uniform(
                    -10, 10, size=time_points_per_datapoint
                )
                outs[index].extend(z_rand.tolist())

    return outs


@pytest.fixture
def fullPointCount() -> tuple[int, ...]:
    """Returns the number of points in the averaged measurement."""
    return (10, 20)


@pytest.fixture
def measurement_request(
    knobs: list["Knob"],
    meters: list["Meter"],
    datapoints_time: float,
    deltas: list[float],
):
    """Returns a measurement request for testing deployment."""
    space = CartesianSpace(deltas=deltas)
    ckd1 = CoupledKnobDomain(
        [
            KnobDomain.from_knob(
                bounds=(0, 0.5),
                knob=knobs[0],
            )
        ]
    )
    ckd2 = CoupledKnobDomain(
        [
            KnobDomain.from_knob(
                bounds=(0.2, 0.7),
                knob=knobs[1],
            )
        ]
    )
    sweep_axes = Axes([ckd1, ckd2])
    space = CartesianDiscreteSpace(space=space, axes=sweep_axes)
    waveform = CartesianWaveform(space=space, transforms=[])
    ports: list[Meter] = []
    ports.extend(meters)
    knobs.append(
        Knob(
            default_name="clock",
            instrument_type=INSTRUMENT_TYPES.CLOCK,
            units=Units.SECOND,
        )
    )
    transform = ConstantTransform(ports=Knobs(knobs), scale=1.0)
    return MeasurementRequest(
        message="test measurement",
        measurement_name="integration_test",
        waveforms=[waveform],
        getters=Meters(meters),
        meter_transforms={meter: transform for meter in meters},
        time_domain=KnobDomain(
            default_name="time",
            bounds=(0, datapoints_time),
            instrument_type=INSTRUMENT_TYPES.CLOCK,
            greater_bound_contained=False,
            units=Units.SECOND,
        ),
    )


@pytest.mark.asyncio
async def test_2D_measurement(
    measurement_response: "MeasurementResponse",
    meters: list["Meter"],
    fullPointCount: tuple[int, ...],
    temp_dir: Path,
    cleanup_instruments,  # needs to be there to ensure instruments are clearned up after tests
):
    plot_dir = Path(temp_dir) / "test_plotted_data"
    plot_dir.mkdir(exist_ok=True)
    assert len(meters) == len(measurement_response.arrays), (
        "Not all meters have a response array."
    )
    fig = plt.figure()
    ax = fig.add_subplot()
    for array in measurement_response.arrays:
        connection = array.connection
        assert connection is not None, "Connection should not be None."

        # Reshape 1D data back to 2D for plotting
        data_2d = np.array(array.array.data).reshape(
            fullPointCount[1], fullPointCount[0]
        )

        # Plot as 2D heatmap with no interpolation (nearest neighbor)
        im = ax.imshow(data_2d, cmap="viridis", interpolation="nearest", aspect="auto")
        ax.set_xlabel("X Index")
        ax.set_ylabel("Y Index")
        ax.set_title(f"{connection.name} - 2D Data")

        # Add colorbar
        plt.colorbar(im, ax=ax)

    # Export the plot to the directory
    plot_path = plot_dir / "test_buffered_2D_measurement.png"
    fig.savefig(plot_path)
    plt.close(fig)  # Clean up the figure
