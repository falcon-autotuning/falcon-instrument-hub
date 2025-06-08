"""Full system integration tests."""

import asyncio
import json
import os
import subprocess
import tempfile
import time
from pathlib import Path

import nats
import pytest
import pytest_asyncio
import yaml
from falcon_core.communications import Time
from falcon_core.communications.messages import MeasurementRequest

from .server_api import RUNTIME_COMMANDS


@pytest.fixture(scope="module")
def temp_dir():
    """Yields a temporary directory for test files."""
    with tempfile.TemporaryDirectory() as temp_dir:
        yield temp_dir
        # Check for Go application logs
        log_dir = Path(temp_dir) / "log"
        print(f"Checking for logs in: {log_dir}")
        if log_dir.exists():
            print(f"Log directory contents: {list(log_dir.iterdir())}")
            for log_file in log_dir.glob("*.log"):
                print(f"Log file: {log_file}")
                try:
                    content = log_file.read_text()
                    print(f"Contents of {log_file.name}:\n{content}")
                except Exception as e:
                    print(f"Could not read {log_file}: {e}")
        else:
            print("Log directory does not exist")


@pytest.fixture
def externalProcessName():
    """Returns the name for the external process."""
    return "TestExternalProcess"


@pytest.fixture
def expectedInstruments():
    """Returns a list of instruments that should be running."""
    return ["LargeMultiChannelDac", "MultiChannelAmnmeter"]


@pytest.fixture
def expectedDaemons(expectedInstruments):
    """Returns a list of daemons that should be running."""
    return expectedInstruments + ["instrument-server"]


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
        "LargeMultiChannelDac.0": "S1",
        "LargeMultiChannelDac.1": "S2",
        "LargeMultiChannelDac.2": "S3",
        "LargeMultiChannelDac.3": "B1",
        "LargeMultiChannelDac.4": "B2",
        "LargeMultiChannelDac.5": "B3",
        "LargeMultiChannelDac.6": "B4",
        "LargeMultiChannelDac.7": "B5",
        "LargeMultiChannelDac.8": "B6",
        "LargeMultiChannelDac.9": "P1",
        "LargeMultiChannelDac.10": "P2",
        "LargeMultiChannelDac.11": "P3",
        "LargeMultiChannelDac.12": "P4",
        "LargeMultiChannelDac.13": "R1",
        "LargeMultiChannelDac.14": "R2",
        "LargeMultiChannelDac.15": "R3",
        "LargeMultiChannelDac.16": "R4",
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


@pytest.fixture(scope="module")
def go_runtime_process(test_config_files):
    """Start the Go runtime server."""
    binary_path = Path("runtime/bin/instrument-server")
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

    # Give it time to start
    time.sleep(10)

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

    yield process

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


@pytest_asyncio.fixture()
async def setup_instruments(nats_client, externalProcessName, expectedInstruments):
    """Setup instruments for testing."""
    # This fixture can be used to setup any required instruments
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

    yield "All setup"

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


@pytest_asyncio.fixture
async def nats_client():
    """Starts the nats client."""
    nc = await nats.connect("nats://localhost:4222")
    yield nc
    await nc.close()


@pytest.mark.asyncio
async def test_daemon_health_monitoring(
    nats_client,
    expectedDaemons,
    go_runtime_process,
    setup_instruments,
):
    """Test that daemons report their health status correctly."""
    status_msgs = {daemon_name: [] for daemon_name in expectedDaemons}

    async def status_handler(msg):
        # Extract instrument name from the subject
        subject_parts = msg.subject.split(".")
        assert len(subject_parts) > 1, f"Invalid name format in subject {subject_parts}"
        daemon_name = subject_parts[1]  # STATUS.<daemon_name>
        data = json.loads(msg.data.decode())
        status_msgs[daemon_name].append(data)

    await nats_client.subscribe(
        RUNTIME_COMMANDS.STATUS.COMM_CHANNEL + ".*",
        cb=status_handler,
    )

    # Wait for multiple status messages with early exit
    max_wait_time = 14.0
    check_interval = 0.5
    elapsed_time = 0.0

    while elapsed_time < max_wait_time:
        if all(len(msgs) >= 2 for msgs in status_msgs.values()):
            break
        await asyncio.sleep(check_interval)
        elapsed_time += check_interval

    assert all(len(msgs) >= 2 for msgs in status_msgs.values()), (
        f"Should receive multiple status messages, got {status_msgs} in {elapsed_time:.1f}s"
    )

    for daemon_name in expectedDaemons:
        latest_status = status_msgs[daemon_name][-1]
        assert RUNTIME_COMMANDS.STATUS.TIMESTAMP in latest_status
        assert RUNTIME_COMMANDS.STATUS.STATUS in latest_status
    print("Collected status messages:", status_msgs)


@pytest.mark.asyncio
async def test_interpreter_flow(
    go_runtime_process,
    nats_client,
    setup_instruments,
):
    """Test a complete interpreter flow from request to data upload."""
    # Collect messages
    upload_msgs = []

    async def upload_handler(msg):
        upload_msgs.append(json.loads(msg.data.decode()))

    # Subscribe to channels
    await nats_client.subscribe(
        RUNTIME_COMMANDS.UPLOAD_DATA.COMM_CHANNEL,
        cb=upload_handler,
    )

    # Create and send measurement request
    request = MeasurementRequest(
        message="test measurement",
        measurement_name="integration_test",
        waveforms=[],
        meter_transforms=[],
    )

    process_request = {
        RUNTIME_COMMANDS.PROCESS_REQUEST.REQUEST: request.to_json(),
        RUNTIME_COMMANDS.PROCESS_REQUEST.PROCESS_ID: "integration_test_001",
        RUNTIME_COMMANDS.PROCESS_REQUEST.CONFIGURATIONS: json.dumps({}),
        RUNTIME_COMMANDS.PROCESS_REQUEST.DATA_PATH: "/tmp/test_data",
    }

    await nats_client.publish(
        RUNTIME_COMMANDS.PROCESS_REQUEST.COMM_CHANNEL,
        json.dumps(process_request).encode(),
    )

    # Wait for multiple status messages with early exit
    max_wait_time = 14.0
    check_interval = 0.5
    elapsed_time = 0.0

    while elapsed_time < max_wait_time:
        if upload_msgs:
            break
        await asyncio.sleep(check_interval)
        elapsed_time += check_interval

    print("Collected list of upload messages:", upload_msgs)
    assert upload_msgs, (
        f"Should receive multiple status messages, got None in {elapsed_time:.1f}s"
    )
    assert go_runtime_process.poll() is None, "Go runtime process died"


@pytest.mark.asyncio
async def test_full_measurement_flow(
    go_runtime_process,
    nats_client,
    setup_instruments,
):
    """Test a complete measurement flow from request to data upload."""
    # Connect to NATS
    externalProcessName = "Test"

    # Collect messages
    upload_msgs = []

    async def upload_handler(msg):
        upload_msgs.append(json.loads(msg.data.decode()))

    await nats_client.subscribe(
        RUNTIME_COMMANDS.MEASURE_RESPONSE.COMM_CHANNEL, cb=upload_handler
    )

    # Setup instruments

    # Create and send measurement request
    request = MeasurementRequest(
        message="test measurement",
        measurement_name="integration_test",
        waveforms=[],
        meter_transforms=[],
    )

    process_request = {
        RUNTIME_COMMANDS.MEASURE_COMMAND.REQUEST: request.to_json(),
        RUNTIME_COMMANDS.MEASURE_COMMAND.HASH: 692,
        RUNTIME_COMMANDS.MEASURE_COMMAND.TIMESTAMP: Time().time,
    }

    await nats_client.publish(
        RUNTIME_COMMANDS.MEASURE_COMMAND.COMM_CHANNEL
        + ".external."
        + externalProcessName,
        json.dumps(process_request).encode(),
    )

    # Wait for processing
    await asyncio.sleep(10)

    try:
        stdout, stderr = go_runtime_process.communicate(timeout=5)
        print(f"STDOUT:\n{stdout}")
        print(f"STDERR:\n{stderr}")
    except subprocess.TimeoutExpired:
        print("Timeout exceeded while waiting for Go process output")

    # Show Go application logs
    log_dir = Path(test_config_files["working_dir"]) / "log"
    print(f"Checking for logs in: {log_dir}")
    print(f"Log dir exists: {log_dir.exists()}")
    if log_dir.exists():
        print(f"Log directory contents: {list(log_dir.iterdir())}")
        for log_file in log_dir.glob("*.log"):
            print(f"Log file: {log_file}")
            try:
                content = log_file.read_text()
                print(f"Contents of {log_file.name}:\n{content}")
            except Exception as e:
                print(f"Could not read {log_file}: {e}")
    else:
        print("Log directory does not exist")

    assert upload_msgs, "No upload messages received"
    print(upload_msgs)
    assert go_runtime_process.poll() is None, "Go runtime process died"
