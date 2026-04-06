# activity

A small tool that logs XLXd activity by tailing syslog, recording
transmissions to a SQLite database and broadcasting events over SSE
and optionally MQTT.

## Configuration

The program looks for `activity.toml` in the current directory by default.
A different path can be specified with the `-config` flag:

```
activity -config /etc/activity/activity.toml
```

An example config (`activity.toml.example`) is included in the
repository. The available settings are:

| Key | Description |
|-----|-------------|
| `db_path` | Path to the SQLite database file. Created automatically if it does not exist. |
| `log_file` | Path to the syslog file to tail (typically `/var/log/syslog`). |
| `http_addr` | Address and port for the HTTP server (e.g. `:8090`). |
| `timezone` | Timezone used to parse syslog timestamps (e.g. `Pacific/Auckland`). |
| `system` | Reflector identifier recorded in each activity row (e.g. `299`). |

### HTTP endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /api/activity/recent` | JSON array of activity records from the past 15 minutes. |
| `GET /api/activity/events` | Server-Sent Events stream; emits an event on each transmission start and end. |
| `GET /health` | Returns `{"status":"ok"}`. |

Each SSE event carries a JSON payload of the form:

```json
{"action":"on","record":{"id":"...","ts":1743900000000,"tsoff":0,"system":"299","module":"A","call":"ZL2ABC","via":"ZL2ABC-B"}}
```

`action` is `"on"` when a station starts transmitting and `"off"` when it stops.
`ts` and `tsoff` are Unix milliseconds (UTC).

### MQTT (`[mqtt]` section)

MQTT publishing is disabled by default (`enabled = false`).

| Key | Description |
|-----|-------------|
| `enabled` | Set to `true` to enable MQTT publishing. |
| `broker` | MQTT v5 broker URL (e.g. `mqtt://localhost:1883`). |
| `client_id` | Client identifier presented to the broker. |
| `topic_prefix` | Topic prefix; the module letter is appended (e.g. `xlx/299/module` → `xlx/299/module/A`). |

On-air events are published with `retain=true` so subscribers that connect
mid-transmission immediately receive the current state. When a transmission
ends the off-air event is published and then a zero-byte retained message is
sent to clear the retained state for that topic.

### Debugging
:w

The log level can be set at runtime via the `LOG_LEVEL` environment variable
(`DEBUG`, `INFO`, `WARN`, `ERROR`; default is `INFO`).

## History

Version 1 of this software used a [PocketBase](https://pocketbase.io) backend,
but the code has been reorganized to directly use SQLite and a purpose-built
SSE backend.
