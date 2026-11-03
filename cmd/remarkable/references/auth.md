---
metadata:
  created_on: 2026-10-07 07:04
  last_modified: 2026-11-03 12:00
  status: current
---

## `remarkable auth`

Authentication group: `pair`, `status`, `token`.

## `remarkable auth pair <code>`

Use an 8-character code from `https://my.remarkable.com/device/desktop/connect`.
Register a device and overwrite resolved credentials with `devicetoken` text,
mode `0600`, creating missing directories with mode `0700`. Emit `OK:` and the path;
`--config` selects that path.

## `remarkable auth status`

Check connectivity and complete item listing; propagate underlying listing errors.
Emit `status: connected`, `generation`, `items`, and `latency_ms` key-value lines.

## `remarkable auth token`

Renew and print the bearer user token as a raw stdout line. Treat it as a secret.
