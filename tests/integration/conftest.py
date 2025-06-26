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
from falcon_core.communications.messages.measurement_response import MeasurementResponse
from falcon_core.instrument_interfaces.names import Knob, Meter

from .server_api import RUNTIME_COMMANDS

if TYPE_CHECKING:
    from falcon_core.instrument_interfaces.names import InstrumentPort
    from instrument_templates.typing import Index


@pytest.fixture(scope="module")
def temp_dir():
    """Yields a temporary directory for test files."""
    temp_path = Path("~/Documents/instrument-server/test-outs").expanduser()
    temp_path.mkdir(parents=True, exist_ok=True)  # Create directory if it doesn't exist
    # Create plotting directory
    plot_dir = temp_path / "test_plotted_data"
    plot_dir.mkdir(exist_ok=True)
    return str(temp_path)


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


@pytest.fixture
def test_config_files(temp_dir, deviceConfig, wiremap):
    """Returns temporary config files for testing."""
    temp_path = Path(temp_dir)

    device_config_path = temp_path / "device_config.yaml"
    with Path.open(device_config_path, "w", encoding="utf-8") as f:
        yaml.dump(deviceConfig, f)

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
    inject_amnmeter_data,
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

    inject_amnmeter_data

    return active_knobs, active_meters


@pytest.fixture
def datapoints_time() -> float:
    """The time for each datapoint in seconds.
    Note: This only supports millisecond resolution.
    """
    return 0.05


@pytest.fixture
def sampleRate() -> float:
    """The fixed default sample rate for the amnmeter is 10000 samples per second."""
    return 10000


@pytest.fixture
def injectionData() -> dict["Index", list[float]]:
    """Returns the default injection data for the amnmeter, which is empty."""
    return {}


@pytest_asyncio.fixture
async def inject_amnmeter_data(injectionData: dict["Index", list[float]], nats_client):
    """Injects the amnmeter data into the test environment if supplied.

    Args:
        injectionData: The data to inject per index of the amnmeter.
        nats_client: The NATS client for publishing messages.
    """
    try:
        for index, data in injectionData.items():
            measurement_msg = {
                "index": index,
                "values": data,
            }

            await nats_client.publish(
                "amnmeter.data",
                json.dumps(measurement_msg).encode(),
            )
            print(f"📊 Injected data for amnmeter index {index}: {len(data)} values")
            await asyncio.sleep(0.01)  # Allow some time for processing
        return True
    except Exception as e:
        print(f"❌ Failed to inject amnmeter data: {e}")
        return False


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


@pytest_asyncio.fixture
async def measurement_response(
    nats_client,
    measurement_request,
    externalProcessName,
    cleanup_instruments,
):
    """Test a complete measurement flow from request to data upload."""
    max_wait_time = 5.0
    check_interval = 0.1
    elapsed_time = 0.0
    upload_msgs = []
    measurement_response = None
    jetstream_data = None

    # Create JetStream context
    nats_client.jetstream()

    async def upload_handler(msg):
        """Handle upload notifications and extract data from JetStream."""
        try:
            upload_data = json.loads(msg.data.decode())
            upload_msgs.append(upload_data)
            print(f"📦 Received upload notification: {upload_data}")

            # First, check if we have response data
            response_data = upload_data.get(RUNTIME_COMMANDS.MEASURE_RESPONSE.RESPONSE)
            if not response_data:
                print("⚠️  No response data found in upload notification")
                print(f"Available keys: {list(upload_data.keys())}")
                return

            # Try to parse the response data
            try:
                unpacked_data = json.loads(response_data)
            except json.JSONDecodeError as e:
                print(f"❌ Failed to parse response data as JSON: {e}")
                print(f"Raw response data: {response_data}")
                return

            # Check for required JetStream parameters
            data_channel = unpacked_data.get("data_channel")
            stream_name = unpacked_data.get("stream_name")

            if not data_channel:
                print("⚠️  Missing data_channel in response")
                print(f"Available keys in response: {list(unpacked_data.keys())}")
                return

            if not stream_name:
                print("⚠️  Missing stream_name in response")
                print(f"Available keys in response: {list(unpacked_data.keys())}")
                return

            print(f"🌊 JetStream info - Channel: {data_channel}, Stream: {stream_name}")

            # Now try to access JetStream
            try:
                js = nats_client.jetstream()
                print("✅ JetStream context created")
            except Exception as js_setup_error:
                print(f"❌ Failed to create JetStream context: {js_setup_error}")
                return

            # Check if stream exists
            try:
                stream_info = await js.stream_info(stream_name)
                print(
                    f"📊 Stream info: {stream_info.state.messages} messages in stream {stream_name}"
                )
            except Exception as stream_info_error:
                print(
                    f"❌ Failed to get stream info for {stream_name}: {stream_info_error}"
                )
                return

            # Check if there are messages
            if stream_info.state.messages == 0:
                print("⚠️  No messages found in stream")
                return

            # Try to get the message
            try:
                print(f"📥 Attempting to fetch message from subject: {data_channel}")
                msg_data = await js.get_last_msg(stream_name, data_channel)
                print("✅ Successfully retrieved message from JetStream")
            except Exception as fetch_error:
                print(f"❌ Failed to fetch message: {fetch_error}")
                return

            # Parse the JetStream message
            try:
                nonlocal measurement_response, jetstream_data
                jetstream_data = json.loads(msg_data.data.decode())
                print(f"📊 JetStream message keys: {list(jetstream_data.keys())}")
            except json.JSONDecodeError as parse_error:
                print(f"❌ Failed to parse JetStream message: {parse_error}")
                print(f"Raw message data: {msg_data.data.decode()}")
                return

            # Extract measurement data
            if "data" in jetstream_data:
                measurement_response = jetstream_data["data"]
                print("✅ Extracted measurement data from 'data' key")
            else:
                measurement_response = jetstream_data
                print("✅ Using full JetStream message as measurement response")

        except json.JSONDecodeError as json_error:
            print(f"❌ Failed to parse upload notification: {json_error}")
            print(f"Raw message: {msg.data.decode()}")
        except Exception as e:
            print(f"❌ Unexpected error processing upload message: {e}")
            print(f"Raw message: {msg.data.decode()}")

    # Subscribe to upload notifications
    await nats_client.subscribe(
        RUNTIME_COMMANDS.MEASURE_RESPONSE.COMM_CHANNEL
        + ".external."
        + externalProcessName,
        cb=upload_handler,
    )

    # Send measurement request
    process_request = {
        RUNTIME_COMMANDS.MEASURE_COMMAND.REQUEST: measurement_request.to_json(),
        RUNTIME_COMMANDS.MEASURE_COMMAND.HASH: 692,
        RUNTIME_COMMANDS.MEASURE_COMMAND.TIMESTAMP: Time().time,
    }

    print("🚀 Sending measurement request...")
    await nats_client.publish(
        RUNTIME_COMMANDS.MEASURE_COMMAND.COMM_CHANNEL
        + ".external."
        + externalProcessName,
        json.dumps(process_request).encode(),
    )

    # Wait for upload messages and measurement completion
    while elapsed_time < max_wait_time:
        print(f"⏱️  {elapsed_time:.1f}s - Upload messages: {len(upload_msgs)}")

        if upload_msgs:
            print("📨 Upload notification received!")
            # Give some time for JetStream data to arrive
            await asyncio.sleep(0.5)

            if measurement_response:
                print("✅ Complete measurement response received!")
                break

        await asyncio.sleep(check_interval)
        elapsed_time += check_interval

    # Verify we received upload notifications
    assert upload_msgs, (
        f"Should receive upload messages, got None in {elapsed_time:.1f}s"
    )

    print(f"\n📋 Final Summary after {elapsed_time:.1f}s:")
    print(f"📦 Upload notifications: {len(upload_msgs)}")

    for i, msg in enumerate(upload_msgs):
        print(
            f"  Notification {i + 1}: data_channel={msg.get('data_channel')}, stream_name={msg.get('stream_name')}"
        )

    # Verify we got measurement data
    if measurement_response:
        print("✅ Measurement response successfully retrieved from JetStream")
        print(f"📊 Response type: {type(measurement_response)}")

        if isinstance(measurement_response, dict):
            print(f"📋 Response keys: {list(measurement_response.keys())}")

            # Check for measurement data
            if "measurement_name" in measurement_response:
                print(
                    f"📝 Measurement name: {measurement_response['measurement_name']}"
                )

            if "data_arrays" in measurement_response:
                data_arrays = measurement_response["data_arrays"]
                print(
                    f"📈 Data arrays found: {list(data_arrays.keys()) if isinstance(data_arrays, dict) else type(data_arrays)}"
                )

        print("✅ Full measurement flow completed successfully!")

    else:
        print("❌ No measurement response retrieved from JetStream")
        if jetstream_data:
            print(f"🔍 Raw JetStream data received: {jetstream_data}")
        else:
            print("🔍 No JetStream data received at all")

        # Print upload notifications for debugging
        for i, msg in enumerate(upload_msgs):
            print(f"  Upload notification {i + 1}: {msg}")

    # Final assertions
    assert upload_msgs, "Should receive at least one upload notification"
    assert measurement_response is not None, (
        "Should retrieve measurement response from JetStream"
    )
    return MeasurementResponse.from_json(measurement_response)
