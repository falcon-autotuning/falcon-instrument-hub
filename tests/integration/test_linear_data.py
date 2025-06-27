"""Full system integration test for linear data."""

from pathlib import Path
from typing import TYPE_CHECKING

import matplotlib.pyplot as plt
import numpy as np
import pytest
from falcon_core.communications.messages import MeasurementRequest
from falcon_core.constants import INSTRUMENT_TYPES
from falcon_core.instrument_interfaces.names import Knobs, Meter, Meters
from falcon_core.instrument_interfaces.port_transforms.identity_transform import (
    IdentityTransform,
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
    from falcon_core.instrument_interfaces.names import Knob
    from instrument_templates.typing import Index


@pytest.fixture
def deviceConfig():
    """Returns the device configuration for testing."""
    return {
        "ScreeningGates": "S1;S2;S3",
        "PlungerGates": "P1;P2;P3;P4",
        "Ohmics": "O1;O2;O3;O4",
        "BarrierGates": "B1;B2;B3;B4;B5;B6",
        "ReservoirGates": "R1;R2;R3;R4",
        "num-unique-channels": 2,
        "groups": {
            "group1": {
                "Name": "I_O1",
                "NumDots": 3,
                "ScreeningGates": "S1;S2",
                "ReservoirGates": "R1;R2",
                "PlungerGates": "P1;P2;P3",
                "BarrierGates": "B1;B2;B3:B4",
                "Order": "O1;R1;B1;P1;B2;P2;B3;P3;B4;R2;O2",
            },
            "group2": {
                "Name": "I_O3",
                "NumDots": 1,
                "ScreeningGates": "S2;S3",
                "ReservoirGates": "R3;R4",
                "PlungerGates": "P4",
                "BarrierGates": "B5;B6",
                "Order": "O3;R3;B5;P4;B6;R4;O4",
            },
        },
        "wiringDC": {
            "S1": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "S2": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "S3": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "P1": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "P2": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "P3": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "P4": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "O1": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "O2": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "O3": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "O4": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "R1": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "R2": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "R3": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "R4": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "B1": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "B2": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "B3": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "B4": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "B5": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
            "B6": {
                "resistance": 1000.0,
                "capacitance": 1e-12,
            },
        },
    }


@pytest.fixture
def wiremap():
    """Returns a wiremap for testing."""
    return {
        "LargeMultiChannelDAC.0": "S1",
        "LargeMultiChannelDAC.1": "S2",
        "LargeMultiChannelDAC.2": "S3",
        "LargeMultiChannelDAC.3": "B1",
        "LargeMultiChannelDAC.4": "B2",
        "LargeMultiChannelDAC.5": "B3",
        "LargeMultiChannelDAC.6": "B4",
        "LargeMultiChannelDAC.7": "B5",
        "LargeMultiChannelDAC.8": "B6",
        "LargeMultiChannelDAC.9": "P1",
        "LargeMultiChannelDAC.10": "P2",
        "LargeMultiChannelDAC.11": "P3",
        "LargeMultiChannelDAC.12": "P4",
        "LargeMultiChannelDAC.13": "R1",
        "LargeMultiChannelDAC.14": "R2",
        "LargeMultiChannelDAC.15": "R3",
        "LargeMultiChannelDAC.16": "R4",
        "MultiChannelAmnmeter.1": "O2",
        "MultiChannelAmnmeter.2": "O4",
    }


@pytest.fixture
def knobs(daemon_health_monitoring: tuple[list["Knob"], list[Meter]]):
    """Returns a list of active knobs."""
    selected_knobs = []
    active_knobs, _ = daemon_health_monitoring
    for knob in active_knobs:
        if knob.instrument_facing_name() == "B3":
            selected_knobs.append(knob)

    print(f"Selected knobs for measurement: {selected_knobs}")
    return selected_knobs


@pytest.fixture
def meters(daemon_health_monitoring: tuple[list["Knob"], list[Meter]]):
    """Returns a list of active meters."""
    selected_meters = []
    _, active_meters = daemon_health_monitoring
    for meter in active_meters:
        if meter.instrument_facing_name() == "O2":
            selected_meters.append(meter)

    print(f"Selected meters for measurement: {selected_meters}")
    return selected_meters


@pytest.fixture
def indexes() -> list[int]:
    """Returns the list of used indexes for the ammeter."""
    return [1]


@pytest.fixture
def intercepts(
    indexes: list[int],
) -> dict[int, tuple[tuple[float, float], tuple[float, float]]]:
    """The intercepts for the linear data line."""
    outs = {}
    for index in indexes:
        if index == 1:
            outs[index] = ((1, -7), (5, 2))
        elif index == 2:
            outs[index] = ((2, 2), (6, -1))
        else:
            outs[index] = ((1, 1), (1, 1))
    return outs


@pytest.fixture
def injectionData(
    sampleRate: int,
    datapoints_time: float,
    fullPointCount: int,
    intercepts: dict[int, tuple[tuple[float, float], tuple[float, float]]],
    indexes: list[int],
) -> dict["Index", list[float]]:
    """Returns the default injection data for the ammeter, which is empty."""
    outs: dict[Index, list[float]] = {}
    time_points_per_datapoint = int(sampleRate * datapoints_time)
    for index in indexes:
        outs[index] = []
        inter0 = intercepts[index][0]
        inter1 = intercepts[index][1]
        m = (inter1[1] - inter0[1]) / (inter1[0] - inter0[0])
        b = inter1[1] - m * inter1[0]
        for x in range(fullPointCount):
            y = m * x + b
            y_rand = y + np.random.uniform(-10, 10, size=time_points_per_datapoint)
            outs[index].extend(y_rand.tolist())

    return outs


@pytest.fixture
def fullPointCount() -> int:
    """Returns the number of points in the avearged measurement."""
    return 10


@pytest.fixture
def measurement_request(
    knobs: list["Knob"],
    meters: list[Meter],
    datapoints_time: float,
    fullPointCount: int,
):
    """Returns a measurement request for testing deployment."""
    space = CartesianSpace(deltas=[1 / fullPointCount])
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
    ports.append(
        Meter(
            default_name="timer",
            instrument_type=INSTRUMENT_TYPES.CLOCK,
            units=Units.SECOND,
        )
    )
    transform = IdentityTransform(port=knobs[0], ports=Knobs(knobs))
    return MeasurementRequest(
        message="test measurement",
        measurement_name="integration_test",
        waveforms=[waveform],
        getters=Meters(meters),
        meter_transforms=[transform],
        time_domain=KnobDomain(
            default_name="time",
            bounds=(0, datapoints_time),
            instrument_type=INSTRUMENT_TYPES.CLOCK,
            greater_bound_contained=False,
            units=Units.SECOND,
        ),
    )


@pytest.mark.asyncio
async def test_linear_measurement(
    measurement_response: "MeasurementResponse",
    meters: list[Meter],
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
    plot_path = plot_dir / "test_standard_linear_measurement.png"
    fig.savefig(plot_path)
    plt.close(fig)  # Clean up the figure
