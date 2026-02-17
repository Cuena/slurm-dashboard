# slurm-dashboard

Terminal UI for monitoring and inspecting Slurm jobs, with live log tailing for `stdout` and `stderr`.

Tested on MareNostrum 5.

## Features

- Live jobs view from `squeue` (auto refresh every 5 seconds)
- History mode from `sacct` (default: last 3 days, configurable)
- Fast filtering by text and status (`All`, `Running`, `Pending`)
- Job inspection panel (`scontrol` in live mode, `sacct` in history mode)
- Job cancel with confirmation (`scancel`)
- Log tail view for both streams or single stream (`stdout` / `stderr`)
- Log tools: follow/pause, search, pane switch, copy selection, open in pager
- Fallback log path resolution for older jobs via archive convention
- Configurable theme and surface style via environment variables

## Requirements

- Slurm CLI tools: `squeue`, `sacct`, `scontrol`, `scancel`
- `tail`
- Optional: `vim` or `$PAGER` for opening full logs from tail view
- No local build needed (use the shipped `slurm-dashboard` binary artifact)

## Distribution Model

This project is distributed as a prebuilt binary artifact.

- End users run `slurm-dashboard` directly
- End users do not need Go or any local build step

## Install On Cluster

Example:

```bash
scp slurm-dashboard <user>@<login-node>:/home/<user>/bin/
ssh <user>@<login-node>
chmod +x /home/<user>/bin/slurm-dashboard
```

If `/home/<user>/bin` is on your `PATH`, you can run it as `slurm-dashboard`.

## Run

```bash
slurm-dashboard
```

Or:

```bash
./slurm-dashboard
```

## Main View Controls

- `q`: quit
- `/` or `f`: filter jobs
- `h`: toggle live/history mode
- `g`: cycle status filter
- `r`: refresh now
- `i` or `Enter`: inspect selected job
- `c`: cancel selected job
- `l`: open logs (both)
- `o`: open `stdout`
- `e`: open `stderr`
- `Tab`: switch focus between jobs and details
- `Ctrl+y`: copy selected detail value
- `v`: view full selected detail value
- `m`: toggle mouse
- `?`: expanded help

## Log View Controls

- `q` or `Esc`: back to main view
- `o` / `e` / `l`: stdout / stderr / both
- `f`: toggle follow
- `p`: pause
- `/`: search
- `n` / `N`: next/previous search match
- `Tab`: switch active pane (in dual pane mode)
- `s`: toggle split layout
- `x`: toggle borders
- `y`: copy mode
- `Ctrl+y`: copy selection
- `Y`: copy full active pane
- `v`: open active log in pager (`$PAGER` or `vim -R`)
- `m`: toggle mouse
- `?`: expanded help

Copy uses OSC52, so clipboard support depends on your terminal/tmux setup.

## Environment Variables

- `SLURM_DASHBOARD_THEME=auto|dark|light`: UI theme selection.
- `SLURM_DASHBOARD_SURFACES=transparent|solid`: background style (terminal-dependent).
- `SLURM_DASHBOARD_PALETTE=dracula-soft|classic`: color palette.
- `SLURM_DASHBOARD_HISTORY_DAYS=<positive-integer>` (default: `3`): history window for `sacct` mode.
- `SLURM_DASHBOARD_LOG_ARCHIVE_DIR=/path/to/log/archive`
  - Used for the "archive convention" fallback when Slurm metadata is missing for old jobs.
  - Default (if unset): `~/.slurm-dashboard/logs` (often private to you).
  - Optional: point this to a shared project directory (and ensure permissions) if you want to share logs.

## Log Recovery For Old Jobs

When Slurm metadata is no longer available for old jobs, the dashboard checks this archive convention:

- `$SLURM_DASHBOARD_LOG_ARCHIVE_DIR/<jobid>.out` (default: `~/.slurm-dashboard/logs/<jobid>.out`)
- `$SLURM_DASHBOARD_LOG_ARCHIVE_DIR/<jobid>.err` (default: `~/.slurm-dashboard/logs/<jobid>.err`)

Compatibility names are also checked:

- `$SLURM_DASHBOARD_LOG_ARCHIVE_DIR/slurm-<jobid>.out`
- `$SLURM_DASHBOARD_LOG_ARCHIVE_DIR/slurm-<jobid>.err`

Recommended `sbatch` directives:

```bash
# Private (default slurm-dashboard location):
#SBATCH --output=$HOME/.slurm-dashboard/logs/%j.out
#SBATCH --error=$HOME/.slurm-dashboard/logs/%j.err
#
# Shared (optional):
#SBATCH --output=/absolute/shared/path/slurm-dashboard/logs/%j.out
#SBATCH --error=/absolute/shared/path/slurm-dashboard/logs/%j.err
```

One-time setup:

```bash
# Private (default):
mkdir -p "$HOME/.slurm-dashboard/logs"

# Shared (optional):
export SLURM_DASHBOARD_LOG_ARCHIVE_DIR="/absolute/shared/path/slurm-dashboard/logs"
mkdir -p "$SLURM_DASHBOARD_LOG_ARCHIVE_DIR"
```

For array jobs:

```bash
# Private:
sbatch \
  --output="$HOME/.slurm-dashboard/logs/%A_%a.out" \
  --error="$HOME/.slurm-dashboard/logs/%A_%a.err" \
  your_array_job.sbatch

# Shared (after exporting SLURM_DASHBOARD_LOG_ARCHIVE_DIR):
sbatch \
  --output="$SLURM_DASHBOARD_LOG_ARCHIVE_DIR/%A_%a.out" \
  --error="$SLURM_DASHBOARD_LOG_ARCHIVE_DIR/%A_%a.err" \
  your_array_job.sbatch
```
