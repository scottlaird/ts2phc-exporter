package main

import (
	"bufio"
	"flag"
	"log/slog"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/hpcloud/tail"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/scottlaird/ts2phc-exporter/parser"
)

var (
	debug          = flag.Bool("debug", false, "Enable debugging output")
	listenAddress  = flag.String("listen_address", ":8089", "Which HTTP port/address to listen on")
	journalctlUnit = flag.String("u", "ts2phc", "Systemd journal unit to read for ts2phc logs")
	logfile        = flag.String("logfile", "", "Logfile to read for ts2phc logs (overrides journalctl default)")

	nemaRE   = regexp.MustCompile(".*nmea sentence: (.*)")
	offsetRE = regexp.MustCompile(".*(/dev/ptp[0-9]+) offset +([-0-9]+) .* freq +([-+0-9]+)")

	satCounts        *prometheus.GaugeVec
	satLocked        prometheus.Gauge
	totalSatellites  prometheus.Gauge
	PDOP, VDOP, HDOP prometheus.Gauge
	offsetCount      prometheus.Counter
	offsetSum        prometheus.Gauge
	offsetSumSquared prometheus.Counter
	freqCount        prometheus.Counter
	freqSum          prometheus.Gauge
	freqSumSquared   prometheus.Counter
)

func ResetNMEAData(nd *parser.NMEAData) {
	nd.Sats = []parser.SatData{}
	nd.SatCounts = make(map[parser.SatConstellation]int64)
	nd.Locked = false
	nd.TotalSatellites = 0
	nd.PDOP = 0
	nd.VDOP = 0
	nd.HDOP = 0
	nd.HDOP_GGA = 0
}

func PublishNMEAData(nd *parser.NMEAData) {
	//fmt.Printf("=> %+v\n", nd)

	for c, v := range nd.SatCounts {
		satCounts.With(prometheus.Labels{"constellation": c.Constellation, "name": c.Name, "band": c.Band, "frequency": c.Frequency}).Set(float64(v))
	}

	PDOP.Set(nd.PDOP)
	VDOP.Set(nd.VDOP)
	if nd.HDOP_GGA > 0 {
		HDOP.Set(nd.HDOP_GGA)
	} else {
		HDOP.Set(nd.HDOP)
	}
	if nd.TotalSatellites > 0 {
		totalSatellites.Set(float64(nd.TotalSatellites))
	} else {
		totalSatellites.Set(float64(nd.SatCounts[parser.SatConstellation{Constellation: "GPS", Name: "GPS", Band: "L1"}])) // Not a terrible fallback if we don't have GGA data.
	}
	if nd.Locked {
		satLocked.Set(1)
	} else {
		satLocked.Set(0)
	}

	offsetCount.Inc()
	offsetSum.Add(float64(nd.Offset))
	offsetSumSquared.Add(float64(nd.Offset * nd.Offset))

	freqCount.Inc()
	freqSum.Add(float64(nd.Freq))
	freqSumSquared.Add(float64(nd.Freq * nd.Freq))
}

func main() {
	flag.Parse()

	if *debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	reg := prometheus.NewRegistry()

	satCounts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ts2phc_sat_counts",
			Help: "Current number of satellites by constellation",
		},
		[]string{"constellation", "name", "band", "frequency"})
	reg.MustRegister(satCounts)

	satLocked = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ts2phc_locked",
			Help: "Shows if GNSS is currently locked; 1 for locked, 0 for not.",
		})
	reg.MustRegister(satLocked)

	totalSatellites = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ts2phc_total_satellites",
			Help: "Current number of satellites used, according to the GNSS module.  This may be less than the sum of ts2phc_sat_counts, depending on the module.  F9Ts seem to limit this to 12, for instance.",
		})
	reg.MustRegister(totalSatellites)

	PDOP = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ts2phc_pdop",
			Help: "Position Dilution of Precision",
		})
	reg.MustRegister(PDOP)

	VDOP = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ts2phc_vdop",
			Help: "Vertical Dilution of Precision",
		})
	reg.MustRegister(VDOP)

	HDOP = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ts2phc_hdop",
			Help: "Horizontal Dilution of Precision",
		})
	reg.MustRegister(HDOP)

	offsetCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "ts2phc_offset_count",
			Help: "count of offset entries",
		})
	reg.MustRegister(offsetCount)

	offsetSum = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ts2phc_offset_sum",
			Help: "sum of offset entries",
		})
	reg.MustRegister(offsetSum)

	offsetSumSquared = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "ts2phc_offset_sum_squared",
			Help: "sum of square of offset entries",
		})
	reg.MustRegister(offsetSumSquared)

	freqCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "ts2phc_freq_count",
			Help: "count of freq entries",
		})
	reg.MustRegister(freqCount)

	freqSum = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ts2phc_freq_sum",
			Help: "sum of freq entries",
		})
	reg.MustRegister(freqSum)

	freqSumSquared = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "ts2phc_freq_sum_squared",
			Help: "sum of square of freq entries",
		})
	reg.MustRegister(freqSumSquared)

	go ReadLogs()

	http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	slog.Info("Starting HTTP listener, listening for /metrics", "address", *listenAddress)
	err := http.ListenAndServe(*listenAddress, nil)
	if err != nil {
		slog.Error("HTTP server failed", "error", err)
	}
}

func ReadLogs() {
	var scanner *bufio.Scanner
	nd := &parser.NMEAData{}
	ResetNMEAData(nd)

	if *logfile != "" {
		t, err := tail.TailFile(*logfile, tail.Config{Follow: true})
		if err != nil {
			slog.Error("Unable to open logfile", "error", err, "logfile", *logfile)
			return
		}
		slog.Info("Scanning logfile", "logfile", *logfile)

		for line := range t.Lines {
			ParseLine(line.Text, nd)
		}

	} else {
		cmd := exec.Command("journalctl", "-u", *journalctlUnit, "-f")

		logs, err := cmd.StdoutPipe()
		if err != nil {
			slog.Error("Failed to create journalctl pipe", "error", err)
			return
		}

		err = cmd.Start()
		if err != nil {
			slog.Error("Failed to run journalctl", "error", err)
			return
		}
		slog.Info("Scanning ts2phc logs", "unit", *journalctlUnit)

		scanner = bufio.NewScanner(logs)
		for scanner.Scan() {
			line := scanner.Text()
			ParseLine(line, nd)
		}
	}

	slog.Error("scan loop finished")
}

func ParseLine(line string, nd *parser.NMEAData) {
	slog.Debug("Scanning line", "line", line)

	if nmeaMatch := nemaRE.FindStringSubmatch(line); nmeaMatch != nil {
		parser.ParseNMEALogEntry(nmeaMatch[1], nd)
	} else if offsetMatch := offsetRE.FindStringSubmatch(line); offsetMatch != nil {
		slog.Debug("Offset", "device", offsetMatch[1], "offset", offsetMatch[2], "freq", offsetMatch[3])

		nd.Device = offsetMatch[1]
		nd.Offset, _ = strconv.Atoi(offsetMatch[2])
		nd.Freq, _ = strconv.Atoi(offsetMatch[3])

		PublishNMEAData(nd)
		ResetNMEAData(nd)
	} else {
		if *debug {
			slog.Debug("Unknown log line", "line", line)
		}
	}
}
