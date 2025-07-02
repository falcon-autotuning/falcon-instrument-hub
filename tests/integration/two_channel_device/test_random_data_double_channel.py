"""Full system integration test for random data with mean 0."""

from pathlib import Path
from typing import TYPE_CHECKING

import matplotlib.pyplot as plt
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


@pytest.fixture
def human_readable_knob_names() -> list[str]:
    """Returns the human readable knob names selected."""
    return ["B3"]


@pytest.fixture
def human_readable_meter_names() -> list[str]:
    """Returns the human readable meter names selected."""
    return ["O2", "O4"]


@pytest.fixture
def fullPointCount() -> tuple[int, ...]:
    """Returns the number of points in the averaged measurement."""
    return (10,)


@pytest.fixture
def measurement_request(
    knobs: list["Knob"],
    meters: list["Meter"],
    datapoints_time: float,
    fullPointCount: tuple[int, ...],
):
    """Returns a measurement request for testing deployment."""
    space = CartesianSpace(deltas=[1 / count for count in fullPointCount])
    ckd = CoupledKnobDomain(
        [
            KnobDomain.from_knob(
                bounds=(0, 0.5),
                knob=knobs[0],
            )
        ]
    )
    sweep_axes = Axes([ckd])
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
        meter_transforms={meter: transform for meter in meters},
        getters=Meters(meters),
        time_domain=KnobDomain(
            default_name="time",
            bounds=(0, datapoints_time),
            instrument_type=INSTRUMENT_TYPES.CLOCK,
            greater_bound_contained=False,
            units=Units.SECOND,
        ),
    )


@pytest.mark.asyncio
async def test_standard_random_measurement(
    measurement_response: "MeasurementResponse",
    meters: list["Meter"],
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
        ax.plot(array.array.data)
        ax.set_ylabel(connection.name)

        # Export the plot to the directory
        plot_path = plot_dir / f"test_standard_random_double_{connection.name}.png"
        fig.savefig(plot_path)
        plt.close(fig)  # Clean up to the figure
