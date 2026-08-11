# slurm-dashboard Agent Guide

## Scope

`slurm-dashboard` is a focused terminal UI for monitoring a user's Slurm jobs, inspecting job metadata, and tailing stdout/stderr logs.

- Keep the app centered on Slurm job monitoring and log inspection.
- Do not expand toward a general cluster administration suite unless the user explicitly asks for that direction.
- Prefer improving the existing live/history jobs workflow over adding new global modes.

## Architecture Invariants

- Bubble Tea commands must return typed messages; they must not mutate render-visible model state from background work.
- `Model.Update` and `TailModel.Update` are the only places that apply UI state transitions for their models.
- Keep the main model as a router for main-view concerns. Put log-tail behavior in `TailModel`, Slurm command/parsing behavior in `slurm.go`, and styling in the style/theme files.
- When adding a new mode or overlay, give it clear ownership and isolate its key handling before adding branches to the global key dispatch.
- If a change requires adding several unrelated fields to `Model` or `TailModel`, stop and introduce a smaller owned state struct first.

## Data Representation

- Keep Slurm data typed as `Job` and related structs until the final table-rendering boundary.
- Do not rely on table row positions for application logic. Table rows are display output; selection and actions should use typed model state.
- When parsing positional CLI output, convert it to named fields immediately at the parser boundary.
- If a column is optional or responsive, map display values by column identity, not by slice length or hard-coded indexes.

## Concurrency Rules

- Background commands may run Slurm, `tail`, clipboard, or pager processes, but they return messages to the main loop instead of mutating models.
- Include request/session IDs for async work that can become stale.
- Ignore stale responses in `Update`; do not let older command results overwrite newer user intent.
- `View` methods should stay pure: no process spawning, filesystem reads, channel operations, or state mutation.

## Change Discipline

- Keep tests focused on the changed behavior, especially stale async responses, responsive layout, table selection, and tail session handling.
- Avoid broad rewrites of `main.go` or `tail.go` in feature work. Extract one owned concept at a time.
- Preserve the prebuilt-binary distribution model described in `README.md`.
