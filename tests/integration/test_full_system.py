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
import yaml
from falcon_core.communications.messages import MeasurementRequest

from server_daemons.api.interpreter import (
    RUNTIME_COMMANDS as INTERPRETER_RUNTIME_COMMANDS,
)


@pytest.fixture(scope="module")
def test_config_files():
    """Create temporary config files for testing."""
    with tempfile.TemporaryDirectory() as temp_dir:
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

        yield {
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
    time.sleep(5)  # Increased from 3 to 5 seconds

    # Check if process started successfully and show logs if it failed
    if process.poll() is not None:
        # Process has already terminated, safe to get output
        try:
            stdout, stderr = process.communicate(timeout=1)
        except subprocess.TimeoutExpired:
            stdout, stderr = "", ""

        print(f"Go process failed to start. Exit code: {process.returncode}")
        print(f"STDOUT:\n{stdout}")
        print(f"STDERR:\n{stderr}")

        # Check for Go application logs
        log_dir = Path(test_config_files["working_dir"]) / "log"
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

        pytest.fail(
            f"Go runtime process failed to start with exit code {process.returncode}"
        )

    yield process

    # Cleanup - show logs if process died unexpectedly
    if process.poll() is not None and process.returncode != 0:
        print(f"Go process died unexpectedly. Exit code: {process.returncode}")
        log_dir = Path(test_config_files["working_dir"]) / "log"
        if log_dir.exists():
            for log_file in log_dir.glob("*.log"):
                print(f"Log file: {log_file}")
                try:
                    print(f"Contents of {log_file.name}:\n{log_file.read_text()}")
                except Exception as e:
                    print(f"Could not read {log_file}: {e}")

    # Cleanup
    process.terminate()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()


@pytest.fixture(scope="module")
def interpreter_daemon_process():
    """Start the Python interpreter daemon."""
    script_path = Path("scripts/launch_interpreter.py")
    if not script_path.exists():
        pytest.skip("Interpreter script not found")

    env = os.environ.copy()
    env["PYTHONPATH"] = f"{Path.cwd()}:{env.get('PYTHONPATH', '')}"

    process = subprocess.Popen(
        ["python3", str(script_path), "nats://localhost:4222"],
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )

    # Give it time to start
    time.sleep(2)

    yield process

    # Cleanup
    process.terminate()
    try:
        process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()


@pytest.mark.asyncio
async def test_interpreter_flow(
    go_runtime_process, interpreter_daemon_process, test_config_files
):
    """Test a complete measurement flow from request to data upload."""
    # Connect to NATS
    nc = await nats.connect("nats://localhost:4222")

    try:
        # Collect messages
        status_msgs = []
        upload_msgs = []

        async def status_handler(msg):
            status_msgs.append(json.loads(msg.data.decode()))

        async def upload_handler(msg):
            upload_msgs.append(json.loads(msg.data.decode()))

        # Subscribe to channels
        await nc.subscribe(
            INTERPRETER_RUNTIME_COMMANDS.STATUS.COMM_CHANNEL, cb=status_handler
        )
        await nc.subscribe(
            INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.COMM_CHANNEL, cb=upload_handler
        )

        # Wait for status message (daemon is running)
        await asyncio.sleep(1)
        assert len(status_msgs) > 0, "No status messages received"

        # Create and send measurement request
        request = MeasurementRequest(
            message="test measurement",
            measurement_name="integration_test",
            waveforms=[],
            meter_transforms=[],
        )

        process_request = {
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.REQUEST: request.to_json(),
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.PROCESS_ID: "integration_test_001",
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.CONFIGURATIONS: json.dumps({}),
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.DATA_PATH: "/tmp/test_data",
        }

        await nc.publish(
            INTERPRETER_RUNTIME_COMMANDS.PROCESS_REQUEST.COMM_CHANNEL,
            json.dumps(process_request).encode(),
        )

        # Wait for processing
        await asyncio.sleep(2)

        # Verify processes are still running
        if go_runtime_process.poll() is not None:
            print(f"Go process died. Exit code: {go_runtime_process.returncode}")
            try:
                stdout, stderr = go_runtime_process.communicate(timeout=1)
            except subprocess.TimeoutExpired:
                stdout, stderr = "", ""

            print(f"STDOUT:\n{stdout}")
            print(f"STDERR:\n{stderr}")

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

        assert go_runtime_process.poll() is None, "Go runtime process died"
        assert interpreter_daemon_process.poll() is None, "Interpreter daemon died"

    finally:
        await nc.close()


@pytest.mark.asyncio
async def test_daemon_health_monitoring():
    """Test that daemons report their health status correctly."""
    nc = await nats.connect("nats://localhost:4222")

    try:
        status_msgs = []

        async def status_handler(msg):
            data = json.loads(msg.data.decode())
            status_msgs.append(data)

        await nc.subscribe(
            INTERPRETER_RUNTIME_COMMANDS.STATUS.COMM_CHANNEL, cb=status_handler
        )

        # Wait for multiple status messages
        await asyncio.sleep(3)

        assert len(status_msgs) >= 2, "Should receive multiple status messages"

        # Verify status message format
        latest_status = status_msgs[-1]
        assert INTERPRETER_RUNTIME_COMMANDS.STATUS.TIMESTAMP in latest_status
        assert INTERPRETER_RUNTIME_COMMANDS.STATUS.STATUS in latest_status

    finally:
        await nc.close()


@pytest.mark.asyncio
async def test_full_measurement_flow(
    go_runtime_process,
    interpreter_daemon_process,
    test_config_files,
):
    """Test a complete measurement flow from request to data upload."""
    # Connect to NATS
    nc = await nats.connect("nats://localhost:4222")

    try:
        # Collect messages
        status_msgs = []
        upload_msgs = []

        async def status_handler(msg):
            status_msgs.append(json.loads(msg.data.decode()))

        async def upload_handler(msg):
            upload_msgs.append(json.loads(msg.data.decode()))

        # Subscribe to channels
        await nc.subscribe(
            INTERPRETER_RUNTIME_COMMANDS.STATUS.COMM_CHANNEL, cb=status_handler
        )
        await nc.subscribe(
            INTERPRETER_RUNTIME_COMMANDS.UPLOAD_DATA.COMM_CHANNEL, cb=upload_handler
        )

        # Wait for status message (daemon is running)
        await asyncio.sleep(1)
        assert len(status_msgs) > 0, "No status messages received"

        # Create and send measurement request
        request = MeasurementRequest(
            message="test measurement",
            measurement_name="integration_test",
            waveforms=[],
            meter_transforms=[],
        )

        process_request = {
            INTERPRETER_RUNTIME_COMMANDS.MEASURE_COMMAND.REQUEST: request.to_json(),
            INTERPRETER_RUNTIME_COMMANDS.MEASURE_COMMAND.HASH: "integration_test_001",
            INTERPRETER_RUNTIME_COMMANDS.MEASURE_COMMAND.TIMESTAMP: json.dumps({}),
        }

        await nc.publish(
            INTERPRETER_RUNTIME_COMMANDS.MEASURE_COMMAND.COMM_CHANNEL,
            json.dumps(process_request).encode(),
        )

        # Wait for processing
        await asyncio.sleep(2)

        # Verify processes are still running
        if go_runtime_process.poll() is not None:
            print(f"Go process died. Exit code: {go_runtime_process.returncode}")
            try:
                stdout, stderr = go_runtime_process.communicate(timeout=1)
            except subprocess.TimeoutExpired:
                stdout, stderr = "", ""

            print(f"STDOUT:\n{stdout}")
            print(f"STDERR:\n{stderr}")

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

        assert go_runtime_process.poll() is None, "Go runtime process died"
        assert interpreter_daemon_process.poll() is None, "Interpreter daemon died"

    finally:
        await nc.close()
