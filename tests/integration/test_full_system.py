"""Full system integration tests."""

import asyncio
import json
import os
import subprocess
from pathlib import Path
from typing import TYPE_CHECKING

import nats
import pytest
import pytest_asyncio
import yaml
from falcon_core.communications import Time
from falcon_core.communications.messages import MeasurementRequest
from falcon_core.constants import INSTRUMENT_TYPES
from falcon_core.instrument_interfaces.names import Knob, Meter, Meters
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

from .server_api import RUNTIME_COMMANDS

if TYPE_CHECKING:
    from falcon_core.instrument_interfaces.names import InstrumentPort


@pytest.fixture(scope="module")
def temp_dir():
    """Yields a temporary directory for test files."""
    temp_path = Path("~/Documents/instrument-server/test-outs").expanduser()
    temp_path.mkdir(parents=True, exist_ok=True)  # Create directory if it doesn't exist
    return str(temp_path)

    # with tempfile.TemporaryDirectory() as temp_dir:
    #     yield temp_dir
    #     # Check for Go application logs
    #     log_dir = Path(temp_dir) / "log"
    #     print(f"Checking for logs in: {log_dir}")
    #     if log_dir.exists():
    #         print(f"Log directory contents: {list(log_dir.iterdir())}")
    #         for log_file in log_dir.glob("*.log"):
    #             print(f"Log file: {log_file}")
    #             try:
    #                 content = log_file.read_text()
    #                 print(f"Contents of {log_file.name}:\n{content}")
    #             except Exception as e:
    #                 print(f"Could not read {log_file}: {e}")
    #     else:
    #         print("Log directory does not exist")


@pytest.fixture
def externalProcessName():
    """Returns the name for the external process."""
    return "TestExternalProcess"


@pytest.fixture
def expectedInstruments():
    """Returns a list of instruments that should be running."""
    return ["LargeMultiChannelDAC", "MultiChannelAmnmeter"]


@pytest.fixture
def serverName():
    """Returns the name of the server."""
    return "instrument-server"


@pytest.fixture
def expectedDaemons(expectedInstruments, serverName):
    """Returns a list of daemons that should be running."""
    return expectedInstruments + [serverName, "interpreter"]


@pytest.fixture(scope="module")
def test_config_files(temp_dir):
    """Returns temporary config files for testing."""
    temp_path = Path(temp_dir)

    device_config = {
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

    device_config_path = temp_path / "device_config.yaml"
    with Path.open(device_config_path, "w", encoding="utf-8") as f:
        yaml.dump(device_config, f)

    # Create wiremap
    wiremap = {
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

    wiremap_path = temp_path / "wiremap.yaml"
    with Path.open(wiremap_path, "w", encoding="utf-8") as f:
        yaml.dump(wiremap, f)

    return {
        "device_config": str(device_config_path),
        "wiremap": str(wiremap_path),
        "working_dir": str(temp_path),
    }


@pytest_asyncio.fixture
async def nats_client():
    """Yields the nats client."""
    nc = await nats.connect("nats://localhost:4222")
    yield nc
    await nc.close()


@pytest_asyncio.fixture
async def setup_status(nats_client, expectedDaemons: list[str]):
    """Returns the received status messages."""
    status_msgs: dict[str, list[str]] = {
        daemon_name: [] for daemon_name in expectedDaemons
    }

    async def status_handler(msg):
        # Extract instrument name from the subject
        subject_parts = msg.subject.split(".")
        if len(subject_parts) > 1:
            daemon_name = subject_parts[1]  # STATUS.<daemon_name>
            if daemon_name in expectedDaemons:
                data = json.loads(msg.data.decode())
                status_msgs[daemon_name].append(data)
                print(f"📊 Status from {daemon_name}: {data}")

    # Subscribe to both status and log messages
    await nats_client.subscribe(
        RUNTIME_COMMANDS.STATUS.COMM_CHANNEL + ".*",
        cb=status_handler,
    )
    return status_msgs


@pytest_asyncio.fixture
async def go_runtime_process(
    test_config_files: dict[str, str],
    setup_status: dict[str, list[str]],
    serverName: str,
):
    """Start the Go runtime server."""
    max_wait_time = 20.0
    check_interval = 4.0
    elapsed_time = 0.0
    min_msg_count = 2
    binary_path = Path("runtime/bin/instrument-server")

    status_msgs = setup_status

    if not binary_path.exists():
        pytest.skip("Go binary not built. Run 'make build-go' first.")

    env = os.environ.copy()
    process = subprocess.Popen(
        [
            str(binary_path),
            "start",
            "--packages",
            "instrument_test_suite @ git+ssh://git@github.com/falcon-autotuning/instrument-test-suite.git@main",
            "--device-config",
            test_config_files["device_config"],
            "--wiremap",
            test_config_files["wiremap"],
            "--working-dir",
            test_config_files["working_dir"],
            "--nats-url",
            "nats://localhost:4222",
        ],
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    print("🔍 Monitoring for instrument-server")
    print("Waiting for status messages...")

    # Wait for multiple status messages with early exit
    while elapsed_time < max_wait_time:
        print(f"⏱️  {elapsed_time:.1f}s - Status messages received:")
        print(f"  {serverName}: {len(status_msgs[serverName])} messages")
        if len(status_msgs[serverName]) >= min_msg_count:
            print("✅ Server reported multiple status messages!")
            break
        await asyncio.sleep(check_interval)
        elapsed_time += check_interval

    # Show final summary
    print(f"\n📋 Final Summary after {elapsed_time:.1f}s:")
    status_count = len(status_msgs[serverName])
    print(f"  {serverName}: {status_count} status")

    assert len(status_msgs[serverName]) >= min_msg_count, (
        f"Should receive multiple status messages, got {status_msgs[serverName]} in {elapsed_time:.1f}s"
    )

    latest_status = status_msgs[serverName][-1]
    assert RUNTIME_COMMANDS.STATUS.TIMESTAMP in latest_status
    assert RUNTIME_COMMANDS.STATUS.STATUS in latest_status

    print("✅ Server is healthy and reporting status")

    # Cleanup - show logs if process died unexpectedly
    if process.poll() is not None and process.returncode != 0:
        print(f"Go process failed to start. Exit code: {process.returncode}")
        # Process has already terminated, safe to get output
        try:
            stdout, stderr = process.communicate(timeout=1)
        except subprocess.TimeoutExpired:
            stdout, stderr = "", ""

        print(f"STDOUT:\n{stdout}")
        print(f"STDERR:\n{stderr}")

        pytest.fail(
            f"Go runtime process failed to start with exit code {process.returncode}"
        )

    yield status_msgs

    if process.poll() is not None and process.returncode != 0:
        print(f"Go process died. Exit code: {process.returncode}")

    try:
        stdout, stderr = process.communicate(timeout=1)
    except subprocess.TimeoutExpired:
        stdout, stderr = "", ""

    print(f"STDOUT:\n{stdout}")
    print(f"STDERR:\n{stderr}")

    # Cleanup
    process.terminate()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()


@pytest_asyncio.fixture
async def setup_port_payload(nats_client):
    """Sets up the nats client to handle port payloads."""
    active_knobs = []
    active_meters = []
    conversions: list[tuple[str, type[InstrumentPort], list[InstrumentPort]]] = [
        (RUNTIME_COMMANDS.PORT_PAYLOAD.KNOBS, Knob, active_knobs),
        (RUNTIME_COMMANDS.PORT_PAYLOAD.METERS, Meter, active_meters),
    ]

    async def port_handler(msg):
        main_contents = json.loads(msg.data.decode())
        for conversion in conversions:
            if main_contents.get(conversion[0]):
                raw_data = json.loads(main_contents[conversion[0]])
            else:
                raw_data = []
            conversion[2].clear()
            if raw_data:
                conversion[2].extend(
                    [conversion[1].from_json(data) for data in raw_data]
                )

    await nats_client.subscribe(
        RUNTIME_COMMANDS.PORT_PAYLOAD.COMM_CHANNEL + ".external.*",
        cb=port_handler,
    )
    return active_knobs, active_meters


@pytest_asyncio.fixture
async def port_request_sender(nats_client):
    """Returns a function to send port requests."""

    async def send_port_request():
        port_request = {
            RUNTIME_COMMANDS.PORT_REQUEST.TIMESTAMP: Time().time,
        }
        await nats_client.publish(
            RUNTIME_COMMANDS.PORT_REQUEST.COMM_CHANNEL + ".external.instrument-server",
            json.dumps(port_request).encode(),
        )

    return send_port_request


@pytest_asyncio.fixture
async def setup_instruments(
    nats_client,
    externalProcessName,
    expectedInstruments,
    setup_port_payload,
    port_request_sender,
):
    """Setup instruments for testing."""
    max_wait_time = 30.0
    check_interval = 4.0
    elapsed_time = 0.0
    active_knobs, active_meters = setup_port_payload
    # Send setup requests for all instruments
    for instrument in expectedInstruments:
        setupconfig = {
            RUNTIME_COMMANDS.SETUP_INSTRUMENT.NAME: instrument,
            RUNTIME_COMMANDS.SETUP_INSTRUMENT.TIMESTAMP: Time().time,
        }

        await nats_client.publish(
            RUNTIME_COMMANDS.SETUP_INSTRUMENT.COMM_CHANNEL
            + ".external."
            + externalProcessName,
            json.dumps(setupconfig).encode(),
        )
        print(f"📡 Sent setup request for {instrument}")

    # Wait for instruments to be available by checking port responses
    while elapsed_time < max_wait_time:
        await port_request_sender()
        await asyncio.sleep(check_interval)
        elapsed_time += check_interval
        print(
            f"⏱️  {elapsed_time:.1f}s - Found {len(active_knobs)} knobs, {len(active_meters)} meters"
        )
        if active_knobs or active_meters:
            print("✅ Instruments are available and responding!")
            break

    if not (active_knobs or active_meters):
        pytest.fail(f"No instruments became available after {elapsed_time:.1f}s")

    return active_knobs, active_meters


@pytest_asyncio.fixture(scope="function", autouse=False)
async def cleanup_instruments(
    nats_client,
    expectedInstruments,
    externalProcessName,
):
    """Cleanup fixture that runs at the very end."""
    yield  # This runs before cleanup

    # Cleanup happens here
    print("🧹 Starting instrument cleanup...")
    for instrument in expectedInstruments:
        setupconfig = {
            RUNTIME_COMMANDS.DESTROY_INSTRUMENT.NAME: instrument,
            RUNTIME_COMMANDS.DESTROY_INSTRUMENT.TIMESTAMP: Time().time,
        }
        await nats_client.publish(
            RUNTIME_COMMANDS.DESTROY_INSTRUMENT.COMM_CHANNEL
            + ".external."
            + externalProcessName,
            json.dumps(setupconfig).encode(),
        )
        print(f"🗑️  Sent destroy request for {instrument}")

    print("✅ Instrument cleanup complete")


@pytest_asyncio.fixture
async def daemon_health_monitoring(
    expectedDaemons,
    go_runtime_process,
    setup_instruments,
):
    """Test that daemons report their health status correctly."""
    max_wait_time = 20.0
    check_interval = 1.0
    elapsed_time = 0.0

    status_msgs = go_runtime_process
    active_knobs, active_meters = setup_instruments
    print(f"🔍 Monitoring for daemons: {expectedDaemons}")
    print("Waiting for status messages...")

    # Wait for multiple status messages with early exit
    while elapsed_time < max_wait_time:
        instrument_daemons = [name for name in expectedDaemons]
        sum(len(status_msgs[name]) for name in instrument_daemons)

        print(f"⏱️  {elapsed_time:.1f}s - Status messages received:")
        for daemon_name, msgs in status_msgs.items():
            print(f"  {daemon_name}: {len(msgs)} messages")

        # Check if we have enough status messages
        if all(len(msgs) >= 2 for msgs in status_msgs.values()):
            print("✅ All daemons reported multiple status messages!")
            break

        await asyncio.sleep(check_interval)
        elapsed_time += check_interval

    # Show final summary
    print(f"\n📋 Final Summary after {elapsed_time:.1f}s:")
    for daemon_name in expectedDaemons:
        status_count = len(status_msgs[daemon_name])
        print(f"  {daemon_name}: {status_count} status")

    assert all(len(msgs) >= 2 for msgs in status_msgs.values()), (
        f"Should receive multiple status messages, got {status_msgs} in {elapsed_time:.1f}s"
    )

    for daemon_name in expectedDaemons:
        latest_status = status_msgs[daemon_name][-1]
        assert RUNTIME_COMMANDS.STATUS.TIMESTAMP in latest_status
        assert RUNTIME_COMMANDS.STATUS.STATUS in latest_status

    print("✅ All daemons are healthy and reporting status")
    return active_knobs, active_meters


@pytest.fixture
def knobs(daemon_health_monitoring: tuple[list[Knob], list[Meter]]):
    """Returns a list of active knobs."""
    selected_knobs = []
    active_knobs, active_meters = daemon_health_monitoring
    for knob in active_knobs:
        if knob.instrument_facing_name() == "B3":
            selected_knobs.append(knob)

    print(f"Selected knobs for measurement: {selected_knobs}")
    return selected_knobs


@pytest.fixture
def meters(daemon_health_monitoring: tuple[list[Knob], list[Meter]]):
    """Returns a list of active meters."""
    selected_meters = []
    active_knobs, active_meters = daemon_health_monitoring
    for meter in active_meters:
        if meter.instrument_facing_name() == "O2":
            selected_meters.append(meter)

    print(f"Selected meters for measurement: {selected_meters}")
    return selected_meters


@pytest.fixture
def measurement_request(knobs: list[Knob], meters: list[Meter]):
    """Returns a measurement request for testing deployment."""
    space = CartesianSpace(deltas=[0.1])
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
    transform = IdentityTransform(port=meters[0], ports=Meters(ports))
    return MeasurementRequest(
        message="test measurement",
        measurement_name="integration_test",
        waveforms=[waveform],
        meter_transforms=[transform],
    )


# @pytest.mark.asyncio
# async def test_interpreter_flow(
#     nats_client,
#     measurement_request,
#     daemon_health_monitoring,
#     temp_dir,
# ):
#     """Test a complete interpreter flow from request to data upload."""
#     max_wait_time = 34.0
#     check_interval = 0.5
#     elapsed_time = 0.0
#     upload_msgs = []
#
#     async def upload_handler(msg):
#         upload_msgs.append(json.loads(msg.data.decode()))
#
#     await nats_client.subscribe(
#         RUNTIME_COMMANDS.UPLOAD_DATA.COMM_CHANNEL,
#         cb=upload_handler,
#     )
#
#     print(daemon_health_monitoring)
#
#     process_request = {
#         RUNTIME_COMMANDS.PROCESS_REQUEST.REQUEST: measurement_request.to_json(),
#         RUNTIME_COMMANDS.PROCESS_REQUEST.PROCESS_ID: 1,
#         RUNTIME_COMMANDS.PROCESS_REQUEST.CONFIGURATIONS: json.dumps({}),
#         RUNTIME_COMMANDS.PROCESS_REQUEST.DATA_PATH: str(Path(temp_dir) / "data"),
#     }
#
#     await nats_client.publish(
#         RUNTIME_COMMANDS.PROCESS_REQUEST.COMM_CHANNEL,
#         json.dumps(process_request).encode(),
#     )
#     while elapsed_time < max_wait_time:
#         print(f"⏱️  {elapsed_time:.1f}s - Upload messages received:")
#         print(f"  {len(upload_msgs)} messages")
#         if upload_msgs:
#             print("✅ Upload message received!")
#             break
#         await asyncio.sleep(check_interval)
#         elapsed_time += check_interval
#
#     # Show final summary
#     print(f"\n📋 Final Summary after {elapsed_time:.1f}s:")
#     print("Collected list of upload messages:", upload_msgs)
#     assert upload_msgs, (
#         f"Should receive a upload message, got None in {elapsed_time:.1f}s"
#     )
#


@pytest.mark.asyncio
async def test_full_measurement_flow(
    nats_client,
    measurement_request,
    externalProcessName,
    cleanup_instruments,
):
    """Test a complete measurement flow from request to data upload."""
    max_wait_time = 8.2
    check_interval = 1.0
    elapsed_time = 0.0
    upload_msgs = []

    async def upload_handler(msg):
        upload_msgs.append(json.loads(msg.data.decode()))

    await nats_client.subscribe(
        RUNTIME_COMMANDS.MEASURE_RESPONSE.COMM_CHANNEL
        + ".external."
        + externalProcessName,
        cb=upload_handler,
    )

    process_request = {
        RUNTIME_COMMANDS.MEASURE_COMMAND.REQUEST: measurement_request.to_json(),
        RUNTIME_COMMANDS.MEASURE_COMMAND.HASH: 692,
        RUNTIME_COMMANDS.MEASURE_COMMAND.TIMESTAMP: Time().time,
    }

    await nats_client.publish(
        RUNTIME_COMMANDS.MEASURE_COMMAND.COMM_CHANNEL
        + ".external."
        + externalProcessName,
        json.dumps(process_request).encode(),
    )
    while elapsed_time < max_wait_time:
        print(f"⏱️  {elapsed_time:.1f}s - Upload messages received:")
        print(f"  {len(upload_msgs)} messages")
        if upload_msgs:
            print("✅ Upload message received!")
            break
        await asyncio.sleep(check_interval)
        elapsed_time += check_interval

    # Show final summary
    print(f"\n📋 Final Summary after {elapsed_time:.1f}s:")
    print("Collected list of upload messages:", upload_msgs)
    assert upload_msgs, (
        f"Should receive a upload message, got None in {elapsed_time:.1f}s"
    )
