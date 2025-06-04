"""Test signal handling in isolation without NATS dependency."""

import os
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path

import pytest


class TestSignalHandling:
    """Test suite for signal handling without NATS dependency."""

    def setup_method(self):
        """Set up test environment."""
        self.temp_dir = tempfile.mkdtemp()

    def teardown_method(self):
        """Clean up test environment."""
        import shutil

        shutil.rmtree(self.temp_dir, ignore_errors=True)

    def create_test_daemon_script(self):
        """Create a simple test daemon script that doesn't require NATS."""
        script_content = '''#!/usr/bin/env python3
import asyncio
import signal
import sys

class TestDaemon:
    def __init__(self):
        self._shutdown_event = asyncio.Event()
        self._setup_signal_handlers()

    def _setup_signal_handlers(self):
        """Set up signal handlers for graceful shutdown."""
        for sig in (signal.SIGTERM, signal.SIGINT):
            signal.signal(sig, self._signal_handler)

    def _signal_handler(self, signum, frame):
        """Handle shutdown signals."""
        print(f"Received signal {signum}, initiating graceful shutdown...", flush=True)
        # Set the shutdown event to trigger cleanup
        asyncio.get_event_loop().call_soon_threadsafe(self._shutdown_event.set)

    async def _cleanup_tasks(self):
        """Clean up all running tasks."""
        # Get all tasks except the current one
        tasks = [task for task in asyncio.all_tasks() if task is not asyncio.current_task()]

        if tasks:
            print(f"Cancelling {len(tasks)} running tasks...", flush=True)
            # Cancel all tasks
            for task in tasks:
                task.cancel()

            # Wait for all tasks to complete cancellation
            await asyncio.gather(*tasks, return_exceptions=True)
            print("All tasks cancelled successfully", flush=True)

    async def start(self):
        """Main daemon loop."""
        print("Test daemon started", flush=True)

        # Create a background task to simulate work
        async def background_work():
            while True:
                await asyncio.sleep(0.1)

        task = asyncio.create_task(background_work())

        try:
            # Wait for shutdown signal
            await self._shutdown_event.wait()
            print("Shutdown event received", flush=True)
        except asyncio.CancelledError:
            print("Daemon cancelled", flush=True)
        finally:
            # Cancel background task
            task.cancel()
            await self._cleanup_tasks()
            print("Daemon shutdown complete", flush=True)

if __name__ == "__main__":
    daemon = TestDaemon()
    try:
        asyncio.run(daemon.start())
        print("Exiting cleanly", flush=True)
        sys.exit(0)
    except KeyboardInterrupt:
        print("Interrupted by keyboard", flush=True)
        sys.exit(0)
    except Exception as e:
        print(f"Error: {e}", flush=True)
        sys.exit(1)
'''

        script_path = Path(self.temp_dir) / "test_daemon.py"
        script_path.write_text(script_content)
        script_path.chmod(0o755)
        return script_path

    def test_sigterm_graceful_shutdown(self):
        """Test that SIGTERM causes graceful shutdown."""
        script_path = self.create_test_daemon_script()

        # Start the daemon process
        process = subprocess.Popen(
            [sys.executable, str(script_path)],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            preexec_fn=os.setsid,  # Create new process group
        )

        # Give it time to start up
        time.sleep(0.5)

        # Verify process is running
        assert process.poll() is None, "Process should be running"

        # Send SIGTERM
        start_time = time.time()
        os.killpg(os.getpgid(process.pid), signal.SIGTERM)

        # Wait for graceful shutdown
        try:
            stdout, stderr = process.communicate(timeout=5)
            shutdown_time = time.time() - start_time

            print(f"STDOUT: {stdout}")
            print(f"STDERR: {stderr}")

            # Verify graceful shutdown
            assert process.returncode == 0, (
                f"Process should exit cleanly, got {process.returncode}"
            )
            assert shutdown_time < 3.0, f"Shutdown took too long: {shutdown_time}s"
            assert "Received signal 15" in stdout, (
                f"Should log signal reception. stdout: {stdout}"
            )
            assert "All tasks cancelled successfully" in stdout, (
                f"Should cancel tasks gracefully. stdout: {stdout}"
            )

        except subprocess.TimeoutExpired:
            # Force kill if it doesn't shut down
            os.killpg(os.getpgid(process.pid), signal.SIGKILL)
            process.wait()
            pytest.fail("Process did not shut down gracefully within timeout")

    def test_sigint_graceful_shutdown(self):
        """Test that SIGINT (Ctrl+C) causes graceful shutdown."""
        script_path = self.create_test_daemon_script()

        # Start the daemon process
        process = subprocess.Popen(
            [sys.executable, str(script_path)],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            preexec_fn=os.setsid,
        )

        # Give it time to start up
        time.sleep(0.5)

        # Verify process is running
        assert process.poll() is None, "Process should be running"

        # Send SIGINT
        start_time = time.time()
        os.killpg(os.getpgid(process.pid), signal.SIGINT)

        # Wait for graceful shutdown
        try:
            stdout, stderr = process.communicate(timeout=5)
            shutdown_time = time.time() - start_time

            print(f"STDOUT: {stdout}")
            print(f"STDERR: {stderr}")

            # Verify graceful shutdown
            assert process.returncode == 0, (
                f"Process should exit cleanly, got {process.returncode}"
            )
            assert shutdown_time < 3.0, f"Shutdown took too long: {shutdown_time}s"
            assert "Received signal 2" in stdout, (
                f"Should log signal reception. stdout: {stdout}"
            )

        except subprocess.TimeoutExpired:
            # Force kill if it doesn't shut down
            os.killpg(os.getpgid(process.pid), signal.SIGKILL)
            process.wait()
            pytest.fail("Process did not shut down gracefully within timeout")

    def test_multiple_signals_handled_correctly(self):
        """Test that multiple signals don't cause issues."""
        script_path = self.create_test_daemon_script()

        # Start the daemon process
        process = subprocess.Popen(
            [sys.executable, str(script_path)],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            preexec_fn=os.setsid,
        )

        # Give it time to start up
        time.sleep(0.5)

        # Verify process is running
        assert process.poll() is None, "Process should be running"

        # Send multiple SIGTERM signals quickly
        start_time = time.time()
        for _ in range(3):
            try:
                os.killpg(os.getpgid(process.pid), signal.SIGTERM)
                time.sleep(0.1)
            except ProcessLookupError:
                # Process already terminated, which is fine
                break

        # Wait for shutdown
        try:
            stdout, stderr = process.communicate(timeout=5)
            shutdown_time = time.time() - start_time

            print(f"STDOUT: {stdout}")
            print(f"STDERR: {stderr}")

            # Should still shut down gracefully
            assert process.returncode == 0, (
                f"Process should exit cleanly, got {process.returncode}"
            )
            assert shutdown_time < 3.0, f"Shutdown took too long: {shutdown_time}s"

        except subprocess.TimeoutExpired:
            os.killpg(os.getpgid(process.pid), signal.SIGKILL)
            process.wait()
            pytest.fail("Process did not handle multiple signals gracefully")


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
