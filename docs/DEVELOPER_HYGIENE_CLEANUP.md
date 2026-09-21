# Developer Comprehension and Repository Hygiene Cleanup

This tracker addresses **Developer Comprehension and Repository Hygiene** in
[CODEBASE_REVIEW.md](CODEBASE_REVIEW.md). The review and
[previous cleanup tracker](DEPRECATED_CODE_CLEANUP.md) remain unchanged.
Starting revision: `2f2cce5`.

## Changes

| Review item | Status | Changes |
| --- | --- | --- |
| Obsolete runtime/protocol documentation | Updated | Rewrote `docs/server-interpreter.md` and `docs/nats-protocol.md` around current handlers, cereal envelopes, annotations, gRPC measurement jobs, and CLI buffer reads. Updated the documentation index, README, and misleading handler/startup comments. Current package documentation already reflects this architecture. |
| Historical refactor plans | Already resolved | `PORT_REFACTOR.md` and `REFACTOR_STATUS.md` were removed before this task. They remain in Git history and are not restored or described as current guidance. |
| Python templates flag and old naming | Remote change preserved; remaining cleanup completed | The merged `79c1ec2` already removes `--packages`, its variable, and logging. Kept those removals and the merged schema formatting; removed leftover Python initialization TODOs and retired initialization-message comments. Updated startup naming to Falcon Instrument Hub. Added a regression check that the Python flag remains absent. |
| Noisy logger and misleading page size | Updated | Internal startup/heartbeat/batch/shutdown diagnostics and batch echoes are opt-in through `LoggerOptions` or CLI `--log-diagnostics`, default off. File logs and real error/loss warnings remain enabled. Renamed `pageSize` to `batchBytes`; it is a 4096-byte batching target, not an OS page size. Diagnostic queue stats use a locked snapshot. |
| Viewer storage assumptions and nonportable demos | Updated | Documented that the viewer consumes legacy/exported JSON, not current JetStream cereal results, HDF5, or SQLite records. Made all checked-in demo file references dataset-relative. Added one shared reader for all averaged plot types and raw traces, supporting dataset-relative paths, existing absolute paths, and relocation of missing old absolute paths by basename. Added viewer tests against copied real demo datasets. |
| Tracked generated artifacts | Untracked; local files preserved | Removed seven artifact paths from Git's index with `git rm --cached` and added nested runtime output and sidecar ignore patterns. No source fixtures or user measurement databases are deleted. |
| Misleading Python test aliases | Retired | Old names now fail immediately with a migration notice instead of running `test-go` and implying distinct measurement coverage. Use `test-go-short`, `test-go`, or `test-schema` explicitly. |

## Behavioral and Compatibility Notes

- `--log-diagnostics` is a CLI option; no hub YAML/schema setting is added.
  `NewLogger` remains source-compatible and quiet by default. DEBUG file entries
  are unaffected; this setting controls the writer's internal diagnostics.
- Relative viewer references resolve against the dataset root rather than CWD.
  An existing absolute file retains precedence. Only missing absolute files
  trigger relocation; permission/directory/parse errors are not silently replaced.
  Optional missing/invalid raw data still permits an averaged plot.
- Retired test target names return a nonzero exit status. Callers must migrate;
  there is no claim that the Go suite tests every legacy measurement category.
- Current limitations are documented, not changed: measurement-specific argument
  dispatch, capability-specific published names/duplicate physical connections,
  fallback metadata/config formats, global ISS lifecycle, and disconnected SQLite
  archival. These belong to the review's separate behavior/integration findings.
- The legacy Lua authoring guide is explicitly marked non-authoritative and
  points to current routing/annotation documentation and controller Teal examples.

## Verification

Pending: formatting, logger/viewer/CLI regression tests, short Go suite, binary
builds, retired-target behavior, ignore checks, Markdown links, and review integrity.

Live ISS/controller integration tests will not be run because existing helpers
can stop another system daemon. No installed binaries are replaced.

## Artifact Inventory

The following removals are staged deliberately by `git rm --cached`; their local
copies still exist and are ignored. Source/documentation edits are not staged.

- `runtime/main`
- `runtime/cmd/main`
- `runtime/dataviewer`
- `runtime/log/falcon-runtime_2026-05-18_16-21-55.log`
- `runtime/datacache/measurements.db-shm`
- `runtime/datacache/measurements.db-wal`
- `nohup.out`

No commit is made. Removing tracked artifacts prevents them from appearing in
future source revisions; it does not purge older Git history.
