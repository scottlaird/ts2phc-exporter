# ts2phc-exporter

This is a Prometheus exporter for `ts2phc`, a part of the LinuxPTP
project.  It currently reads ts2phc logs via journalctl and exports
/metrics on port 8089.

This is a work in progress, but it's getting closer to being complete.
I'm using it with 4 different systems, with 1x ublox M8T, 2x F9T, and
1x F10.  Other GNSS modules should work but are untested.

## Installing

You'll need Go 1.23 installed.  You can compile `ts2phc-exporter` by
cloning this repository and running `go build` to produce a
`ts2phc-exporter` binary.  Then copy that to `/usr/local/bin`.
There's a systemd unit file included.

To get useful GPS statistics, you'll want to add `-l 7` to `ts2phc`'s
command line, so that it logs NMEA sentences to its logs.  Without
this, there won't be much data for `ts2phc-exporter` to work with.

## Metrics

This calculates a handful of metrics from `ts2phc`.  These break down into a few groups:

- Time-sync metrics.  You can calculate how accurately `ts2phc`
  maintains its clock via the `ts2phc_offset*` and `ts2phc_freq_*`
  metrics.  For each, there is a `sum`, a `count`, and a `sum_squared`
  metric.  So, the offset average is `ts2phc_offset_sum /
  ts2phc_offset_count` and the RMS average is
  `sqrt(ts2phc_offset_sum_squared / ts2phc_offset_count)`
  
- GPS accuracy metrics.  The `ts2phc_locked` metric will be 1 if the
  GNSS module has a working lock on enough satellites.  You can see
  how many satellites are currently being used by looking at
  `ts2phc_total_satellites`.  The `ts2phc_*dop` metrics calculate
  various dilution-of-precision metrics; for now, `ts2phc_hdop` is
  probably the most useful.
  
- Finally, `ts2phc_sat_counts` breaks down which satellite
  constellations and which frequency bands are being used.  This
  probably requires a GNSS module that supports NMEA 4.11+; it works
  with F9 and F10 receivers from ublox, but not the M8 or earlier
  series.  On those, you'll only see which constellations are in use,
  not which frequencies are available.

```
# HELP promhttp_metric_handler_errors_total Total number of internal errors encountered by the promhttp metric handler.
# TYPE promhttp_metric_handler_errors_total counter
promhttp_metric_handler_errors_total{cause="encoding"} 0
promhttp_metric_handler_errors_total{cause="gathering"} 0
# HELP ts2phc_freq_count count of freq entries
# TYPE ts2phc_freq_count counter
ts2phc_freq_count 1353
# HELP ts2phc_freq_sum sum of freq entries
# TYPE ts2phc_freq_sum gauge
ts2phc_freq_sum -9.476421e+06
# HELP ts2phc_freq_sum_squared sum of square of freq entries
# TYPE ts2phc_freq_sum_squared counter
ts2phc_freq_sum_squared 6.6373889601e+10
# HELP ts2phc_hdop Horizontal Dilution of Precision
# TYPE ts2phc_hdop gauge
ts2phc_hdop 0.72
# HELP ts2phc_locked Shows if GNSS is currently locked; 1 for locked, 0 for not.
# TYPE ts2phc_locked gauge
ts2phc_locked 1
# HELP ts2phc_offset_count count of offset entries
# TYPE ts2phc_offset_count counter
ts2phc_offset_count 1353
# HELP ts2phc_offset_sum sum of offset entries
# TYPE ts2phc_offset_sum gauge
ts2phc_offset_sum -232
# HELP ts2phc_offset_sum_squared sum of square of offset entries
# TYPE ts2phc_offset_sum_squared counter
ts2phc_offset_sum_squared 177326
# HELP ts2phc_pdop Position Dilution of Precision
# TYPE ts2phc_pdop gauge
ts2phc_pdop 1.27
# HELP ts2phc_sat_counts Current number of satellites by constellation
# TYPE ts2phc_sat_counts gauge
ts2phc_sat_counts{band="1",constellation="BeiDou",frequency="B1",name="BeiDou B1 Cp"} 6
ts2phc_sat_counts{band="1",constellation="GPS",frequency="L1",name="GPS L1"} 11
ts2phc_sat_counts{band="1",constellation="Galileo",frequency="E1",name="Galileo E1 B/C"} 6
# HELP ts2phc_total_satellites Current number of satellites used, according to the GNSS module.  This may be less than the sum of ts2phc_sat_counts, depending on the module.  F9Ts seem to limit this to 12, for instance.
# TYPE ts2phc_total_satellites gauge
ts2phc_total_satellites 12
# HELP ts2phc_vdop Vertical Dilution of Precision
# TYPE ts2phc_vdop gauge
ts2phc_vdop 1.05
```

## Usage

There's a systemd unit file included.  Run `go build`, then copy
`ts2phc-exporter` to `/usr/local/bin`.  Copy `ts2phc-exporter.service`
into `/etc/systemd/system/`, then run `systemctl start
ts2phc-exporter`.

### General Flags

* `--debug` enables debugging logs.
* `--listen_address` controls the HTTP address that `ts2phc` uses to
  listen for Prometheus `/metrics` requests.

### Log Processing Flags

By default, `ts2phc-exporter` will ask `journalctl` on Linux for the
logs from the `ts2phc` service.  You can change the service name via
the `-u` flag, or avoid using `journalctl` by specifying a log file
name with `--logfile`.

### Database Logging of Satellite Traces

You can log a record of each satellite observation to a database by:

1.  Creating a suitable DB schema.  There's one for clustered
    Clickhouse DBs included here, but Postgresql and MySQL should both
    be usable, if untested.
2.  Run `ts2phc-exporter` with both a `DB_DRIVER` and `DSN` environment variable plus a table name in the `--dbtable` flag.  For example:

```
DB_DRIVER=clickhouse DSN="clickhouse://default@localhost:9000/default" ts2phc-exporter --dbtable gps.satobservations
```

For users who log multiple systems to the same DB, you can use the
`--receiver` flag to change the receiver name logged (defaults to
`hostname`) and `--antenna` to add an antenna name.  In my
environment, I have 4 different systems running `ts2phc-exporter`,
connected to 2 different GNSS antennas via a pair of antenna
splitters.  So, two of the systems run with `--antenna=west1` and two
with `--antenna==east1`, and this will let me generate
antenna-specific statistics from the DB trivially.
