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
from mpl_toolkits.mplot3d import Axes3D

if TYPE_CHECKING:
    from falcon_core.communications.messages.measurement_response import (
        MeasurementResponse,
    )
    from falcon_core.instrument_interfaces.names import Meter
    from instrument_templates.typing import Index


@pytest.fixture
def human_readable_knob_names() -> list[str]:
    """Returns the human readable knob names selected."""
    return ["B2", "B3", "B4"]


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
            outs[index] = ((1, 1, 1, 0), (5, 5, 5, 0), (1, 5, 5, -9), (1, 1, 5, 2))
        elif index == 2:
            outs[index] = ((2, 2, 2, 0), (6, 5, 5, 0), (2, 6, 6, 8), (5, 5, 2, -3))
        else:
            outs[index] = ((1, 1, 1, 1), (6, 6, 6, 1), (2, 2, 2, 1), (6, 2, 1, 1))
    return outs


def plane_from_points(
    p1: tuple[float, ...],
    p2: tuple[float, ...],
    p3: tuple[float, ...],
    p4: tuple[float, ...],
):
    """Generate a 3D plane function from 4 points.

    Args:
        p1, p2, p3, p4: Three points (x, y, z, w) that define the plane

    Returns:
        A function that takes (x, y, z) and returns w value on the plane
    """
    # Convert points to numpy arrays
    np1 = np.array(p1)
    np2 = np.array(p2)
    np3 = np.array(p3)
    np4 = np.array(p4)

    # For a 4D hyperplane: ax + by + cz + dw + e = 0
    # We need to solve the system of equations for the 4 points
    # Create matrix A where each row is [x, y, z, w, 1] for each point
    A = np.array(
        [
            [np1[0], np1[1], np1[2], np1[3], 1],
            [np2[0], np2[1], np2[2], np2[3], 1],
            [np3[0], np3[1], np3[2], np3[3], 1],
            [np4[0], np4[1], np4[2], np4[3], 1],
        ]
    )

    # Find the null space of A to get the hyperplane coefficients
    # Using SVD to find the null space
    _, _, V = np.linalg.svd(A)
    coefficients = V[-1]  # Last row of V is the null space vector

    # Extract coefficients: ax + by + cz + dw + e = 0
    a, b, c, d, e = coefficients

    def plane_function(x: float, y: float, z: float) -> float:
        """Calculate w value for given x, y, z coordinates on the hyperplane.

        Args:
            x: X coordinate
            y: Y coordinate
            z: Z coordinate

        Returns:
            W value on the hyperplane
        """
        if abs(d) < 1e-10:  # Check for near-zero denominator
            msg = "Cannot solve for w: hyperplane is degenerate"
            raise ValueError(msg)
        return -(a * x + b * y + c * z + e) / d

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
        inter3 = intercepts[index][3]
        plane = plane_from_points(inter0, inter1, inter2, inter3)
        for z in range(fullPointCount[2]):
            for y in range(fullPointCount[1]):
                for x in range(fullPointCount[0]):
                    # Generate a 2D linear function
                    w = plane(x, y, z)
                    w_rand = w * np.ones(time_points_per_datapoint) + np.random.uniform(
                        -10, 10, size=time_points_per_datapoint
                    )
                    outs[index].extend(w_rand.tolist())

    return outs


@pytest.fixture
def fullPointCount() -> tuple[int, ...]:
    """Returns the number of points in the averaged measurement."""
    return (10, 20, 15)


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
    ckd3 = CoupledKnobDomain(
        [
            KnobDomain.from_knob(
                bounds=(0.2, -0.7),
                knob=knobs[2],
            )
        ]
    )
    sweep_axes = Axes([ckd1, ckd2, ckd3])
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
async def test_3D_measurement(
    measurement_response: "MeasurementResponse",
    meters: list["Meter"],
    fullPointCount: tuple[int, ...],
    temp_dir: Path,
    cleanup_instruments,  # needs to be there to ensure instruments are clearned up after tests
):
    """Test 3D measurement and plot 4D hyperplane visualization."""
    plot_dir = Path(temp_dir) / "test_plotted_data"
    plot_dir.mkdir(exist_ok=True)
    assert len(meters) == len(measurement_response.arrays), (
        "Not all meters have a response array."
    )

    for array in measurement_response.arrays:
        connection = array.connection
        assert connection is not None, "Connection should not be None."

        # Reshape 1D data back to 3D for plotting
        data_3d = np.array(array.array.data).reshape(
            fullPointCount[2],
            fullPointCount[1],
            fullPointCount[0],
        )

        # Create 4D hyperplane visualization
        fig = plt.figure(figsize=(15, 10))

        # Plot 1: 3D scatter with color-coded 4th dimension
        ax1 = Axes3D(fig, rect=[0.02, 0.52, 0.46, 0.46])

        # Create coordinate grids
        z_coords, y_coords, x_coords = np.meshgrid(
            np.arange(fullPointCount[2]),
            np.arange(fullPointCount[1]),
            np.arange(fullPointCount[0]),
            indexing="ij",
        )

        # Flatten coordinates and data
        x_flat = x_coords.ravel()
        y_flat = y_coords.ravel()
        z_flat = z_coords.ravel().astype(float)
        w_flat = data_3d.ravel()

        # 3D scatter plot with color representing 4th dimension
        scatter = ax1.scatter(
            x_flat, y_flat, z_flat, c=w_flat, cmap="viridis", s=20, alpha=0.6
        )
        ax1.set_xlabel("X Index")
        ax1.set_ylabel("Y Index")
        ax1.set_zlabel("Z Index")
        ax1.set_title(f"{connection.name} - 4D Data (Color = W value)")
        plt.colorbar(scatter, ax=ax1, shrink=0.8)

        # Plot 2: 2D slices through different Z planes
        ax2 = fig.add_subplot(222)

        # Show middle slice
        middle_z = fullPointCount[2] // 2
        slice_data = data_3d[middle_z, :, :]
        im2 = ax2.imshow(
            slice_data, cmap="viridis", aspect="auto", interpolation="nearest"
        )
        ax2.set_xlabel("X Index")
        ax2.set_ylabel("Y Index")
        ax2.set_title(f"{connection.name} - Z Slice at {middle_z}")
        plt.colorbar(im2, ax=ax2)

        # Plot 3: Multiple Z slices as subplots
        ax3 = fig.add_subplot(223)

        # Show 4 different Z slices
        z_slices = np.linspace(0, fullPointCount[2] - 1, 4, dtype=int)
        combined_slice = np.zeros((fullPointCount[1], fullPointCount[0] * 4))

        for i, z_idx in enumerate(z_slices):
            start_col = i * fullPointCount[0]
            end_col = (i + 1) * fullPointCount[0]
            combined_slice[:, start_col:end_col] = data_3d[z_idx, :, :]

        im3 = ax3.imshow(
            combined_slice, cmap="viridis", aspect="auto", interpolation="nearest"
        )
        ax3.set_xlabel("X Index (4 Z slices concatenated)")
        ax3.set_ylabel("Y Index")
        ax3.set_title(f"{connection.name} - Multiple Z Slices")
        plt.colorbar(im3, ax=ax3)

        # Plot 4: Isosurface-like contour plot
        ax4 = fig.add_subplot(224)

        # Average across Z dimension for 2D contour
        avg_data = np.mean(data_3d, axis=0)
        contour = ax4.contourf(avg_data, levels=20, cmap="viridis")
        ax4.set_xlabel("X Index")
        ax4.set_ylabel("Y Index")
        ax4.set_title(f"{connection.name} - Z-averaged Contour")
        plt.colorbar(contour, ax=ax4)

        plt.tight_layout()

        # Export the plot
        plot_path = plot_dir / f"test_4D_hyperplane_{connection.name}.png"
        fig.savefig(plot_path, dpi=150, bbox_inches="tight")
        plt.close(fig)

        # Create additional interactive-style plot with discrete points
        fig2, ax = plt.subplots(1, 1, figsize=(10, 8))

        # Create a discrete grid visualization
        for z in range(0, fullPointCount[2], max(1, fullPointCount[2] // 5)):
            x_grid, y_grid = np.meshgrid(
                np.arange(fullPointCount[0]), np.arange(fullPointCount[1])
            )

            # Plot discrete points for this Z slice
            w_values = data_3d[z, :, :]
            scatter = ax.scatter(
                x_grid.ravel(),
                y_grid.ravel(),
                c=w_values.ravel(),
                s=50,
                cmap="viridis",
                alpha=0.7,
                label=f"Z={z}",
                edgecolors="black",
                linewidth=0.5,
            )

        ax.set_xlabel("X Index")
        ax.set_ylabel("Y Index")
        ax.set_title(f"{connection.name} - Discrete 4D Data Points (Multiple Z layers)")
        ax.legend()
        ax.grid(True, alpha=0.3)

        # Add colorbar
        plt.colorbar(scatter, ax=ax, label="W Value")

        # Export the discrete plot
        plot_path2 = plot_dir / f"test_4D_discrete_{connection.name}.png"
        fig2.savefig(plot_path2, dpi=150, bbox_inches="tight")
        plt.close(fig2)
