# ts2phc-exporter

This is a Prometheus exporter for `ts2phc`, a part of the LinuxPTP
project.  It currently reads ts2phc logs via journalctl and exports
/metrics on port 8089.

This is a work in progress.  It's not done, it's full of debugging
code, it doesn't have flags for any of the things that should have
flags (like the HTTP port number!), and its NMEA parsing is still
weak.  But it works for me.

## Usage

There's a systemd unit file included.  Run `go build`, then copy
`ts2phc-exporter` to `/usr/local/bin`.  Copy `ts2phc-exporter.service`
into `/etc/systemd/system/`, then run `systemctl start
ts2phc-exporter`.

At the moment, there are only two flags:

* `--debug` enables debugging logs.
* `--listen_address` controls the HTTP address that `ts2phc` uses to
  listen for Prometheus `/metrics` requests.


