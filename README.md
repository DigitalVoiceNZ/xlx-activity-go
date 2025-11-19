# activity

A small tool that tails syslog to log XLXd activity.

## Building

To build the program, run the following command:

```bash
go build
```

## Configuration

The application can be configured using environment variables. All
configuration options have sensible defaults for typical XLX system
deployments.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SYSTEM_NAME` | `299` | System identifier - usually a numeric code for the XLX system |
| `TIMEZONE` | `Pacific/Auckland` | Timezone for timestamp parsing (IANA timezone format) |
| `DB_PATH` | `./pb_data/data.db` | Path to the SQLite database file |
| `SSE_ADDR` | `:8080` | Address and port for the SSE server (format: `:port` or `host:port`) |
| `LOG_PATH` | `/var/log/syslog` | Path to the system log file to monitor |
| `LOG_LEVEL` | `INFO` | Application log level (`DEBUG`, `INFO`, `WARN`, `ERROR`) |

### Configuration Examples

#### Using environment variables directly:
```bash
export SYSTEM_NAME=123
export TIMEZONE=UTC
export SSE_ADDR=:9090
./activity
```

#### Using a .env file:
Copy `.env.example` to `.env` and modify the values:
```bash
cp .env.example .env
# Edit .env with your preferred values
./activity
```

## Running

To run the program with default configuration:

```bash
./activity
```

### Database Initialization

On the initial run, you need to create the SQLite database file and its
schema. Use the `--create-db` flag for this:

```bash
./activity --create-db
```

**Important:** This flag should only be used once to set up a new
database. Subsequent runs should omit this flag. If the database file
specified by `DB_PATH` does not exist and `--create-db` is not used, the
application will exit with an error suggesting a configuration problem or
that the database needs to be created.

The program will then start monitoring the configured log file and provide
an SSE endpoint for real-time activity updates.

## History

This program was originally written for the Digital Voice NZ
[XLX299 dashboard](https://xlx299.nz/).
