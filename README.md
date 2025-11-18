# activity

A small tool that tails syslog to log XLXd activity.

## Building

To build the program, run the following command:

```bash
go build
```

## Running

To run the program, execute the following command:

```bash
./activity
```

The program will start tailing the `/var/log/syslog` file and will start an SSE server on port 8080.

