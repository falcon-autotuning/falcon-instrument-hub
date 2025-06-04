"""Simple test script to verify daemon signal handling."""

import os
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path


def create_simple_daemon_script(temp_dir):
    """Create a minimal daemon script for testing."""
    script_content = '''#!/usr/bin/env python3
import asyncio
import signal
import sys
import os

# Add the src directory to Python path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from server_daemons.instrument_daemon import InstrumentDaemon
from instrument_templates.base_instrument_driver import BaseInstrumentDriver

class TestDriver(BaseInstrumentDriver):
    """Minimal test driver."""
    def __init__(self, sync_sender=None):
        super().__init__(sync_sender=sync_sender)
        print("TestDriver initialized", flush=True)

    def to_json_config(self):
        return {"driver": "TestDriver", "status": "initialized"}

async def main():
    """Main function."""
    try:
        # Get command line arguments
        if len(sys.argv) < 3:
            print("Usage: script.py <driver_name> <nats_url>", flush=True)
            sys.exit(1)

        driver_name = sys.argv[1]
        nats_url = sys.argv[2]

        print(f"Starting daemon with driver: {driver_name}, NATS: {nats_url}", flush=True)

        # Create event loop
        loop = asyncio.get_event_loop()

        # Create daemon
        daemon = InstrumentDaemon(
            url=nats_url,
            instrument_driver=TestDriver,
            loop=loop
        )

        # Start daemon
        await daemon.start()

    except KeyboardInterrupt:
        print("Interrupted by keyboard", flush=True)
    except Exception as e:
        print(f"Error: {e}", flush=True)
        raise
    finally:
        print("Daemon main function complete", flush=True)

if __name__ == "__main__":
    try:
        asyncio.run(main())
        print("Daemon exited cleanly", flush=True)
        sys.exit(0)
    except Exception as e:
        print(f"Daemon failed: {e}", flush=True)
        sys.exit(1)
'''

    script_path = Path(temp_dir) / "simple_daemon.py"
    script_path.write_text(script_content)
    script_path.chmod(0o755)
    return script_path


def test_simple_daemon_signal_handling():
    """Test daemon signal handling with minimal setup."""
    temp_dir = tempfile.mkdtemp()

    try:
        # Create test script
        script_path = create_simple_daemon_script(temp_dir)

        # Start daemon process
        print("Starting daemon process...")
        process = subprocess.Popen(
            [sys.executable, str(script_path), "TestDriver", "nats://127.0.0.1:9999"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            preexec_fn=os.setsid,
        )

        # Give it time to start
        time.sleep(2.0)

        # Verify it's running
        if process.poll() is not None:
            stdout, stderr = process.communicate()
            print(f"Process exited early! stdout: {stdout}, stderr: {stderr}")
            return False

        print("Process is running, sending SIGTERM...")

        # Send SIGTERM
        start_time = time.time()
        os.killpg(os.getpgid(process.pid), signal.SIGTERM)

        # Wait for shutdown
        try:
            stdout, stderr = process.communicate(timeout=10)
            shutdown_time = time.time() - start_time

            print(f"Shutdown time: {shutdown_time:.2f}s")
            print(f"Return code: {process.returncode}")
            print(f"STDOUT:\n{stdout}")
            print(f"STDERR:\n{stderr}")

            # Check if shutdown was graceful
            success = (
                process.returncode == 0
                and shutdown_time < 8.0
                and (
                    "Received signal 15" in stdout
                    and "Shutdown signal received" in stdout
                    and "shutdown complete" in stdout
                )
            )

            if success:
                print("✅ Signal handling test PASSED")
            else:
                print("❌ Signal handling test FAILED")
                print(f"   - Return code OK: {process.returncode == 0}")
                print(
                    f"   - Shutdown time OK: {shutdown_time < 8.0} ({shutdown_time:.2f}s)"
                )
                print(f"   - Signal received: {'Received signal 15' in stdout}")
                print(f"   - Shutdown complete: {'shutdown complete' in stdout}")

            return success

        except subprocess.TimeoutExpired:
            print("❌ Process did not shut down within timeout")
            os.killpg(os.getpgid(process.pid), signal.SIGKILL)
            process.wait()
            return False

    except Exception as e:
        print(f"❌ Test failed with exception: {e}")
        return False
    finally:
        # Cleanup
        import shutil

        shutil.rmtree(temp_dir, ignore_errors=True)


if __name__ == "__main__":
    print("Testing daemon signal handling...")
    success = test_simple_daemon_signal_handling()
    sys.exit(0 if success else 1)
