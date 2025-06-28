package main

import (
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/adrianmo/go-nmea"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/scottlaird/ts2phc-exporter/parser"
)

var (
	debug         = flag.Bool("debug", false, "Enable debugging output")
	listenAddress = flag.String("listen_address", ":8089", "Which HTTP port/address to listen on")

	nemaRE   = regexp.MustCompile(".*nmea sentence: (.*)")
	offsetRE = regexp.MustCompile(".*(/dev/ptp[0-9]+) offset +([-0-9]+) .* freq +([-+0-9]+)")

	satCounts        *prometheus.GaugeVec
	bandCounts       *prometheus.GaugeVec
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

type SatConstellation struct {
	Constellation string
	Name          string
	Band          string
	Frequency     string
}

type NMEAData struct {
	Sats                       []parser.SatData
	SatCounts                  map[SatConstellation]int64
	Locked                     bool
	TotalSatellites            int64
	PDOP, VDOP, HDOP, HDOP_GGA float64
	Device                     string
	Offset                     int
	Freq                       int
}

// By default go-nmea refuses to parse any NMEA sentences without a
// checksum.  This replaces the default checksum checker.
func ignoreCNC(sentence nmea.BaseSentence, rawFields string) error {
	return nil
}

func ResetNMEAData(nd *NMEAData) {
	nd.Sats = []parser.SatData{}
	nd.SatCounts = make(map[SatConstellation]int64)
	nd.Locked = false
	nd.TotalSatellites = 0
	nd.PDOP = 0
	nd.VDOP = 0
	nd.HDOP = 0
	nd.HDOP_GGA = 0
}

func PublishNMEAData(nd *NMEAData) {
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
		totalSatellites.Set(float64(nd.SatCounts[SatConstellation{Constellation: "GPS", Name: "GPS", Band: "L1"}])) // Not a terrible fallback if we don't have GGA data.
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

	bandCounts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ts2phc_band_counts",
			Help: "Current number of satellites by band",
		},
		[]string{"band"})
	reg.MustRegister(bandCounts)

	satLocked = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ts2phc_locked",
			Help: "Shows if GNSS is currently locked; 1 for locked, 0 for not.",
		})
	reg.MustRegister(satLocked)

	totalSatellites = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ts2phc_total_satellites",
			Help: "Current number of satellites in view",
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
	unknownTypeSeen := make(map[string]bool)

	slog.Info("Scanning ts2phc logs")
	cmd := exec.Command("journalctl", "-u", "ts2phc", "-f")

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

	nmeaParser := nmea.SentenceParser{
		CheckCRC: ignoreCNC,
	}

	scanner := bufio.NewScanner(logs)

	nd := &NMEAData{}
	ResetNMEAData(nd)

	for scanner.Scan() {
		line := scanner.Text()
		slog.Debug("Scanning line", "line", line)

		if nmeaMatch := nemaRE.FindStringSubmatch(line); nmeaMatch != nil {
			sentence := "$" + nmeaMatch[1]

			s, err := nmeaParser.Parse(sentence)
			if err != nil {
				fmt.Printf("---> NMEA match failed: %v\n", err)
			} else {
				switch s.DataType() {
				case nmea.TypeGSA:
					gsa := s.(nmea.GSA)
					bd := parser.ParseBandDataWithSystemID(gsa.Talker, gsa.SystemID)
					slog.Debug("Parsed GSA", "mode", gsa.Mode, "fixtype", gsa.FixType, "sv", gsa.SV, "pdop", gsa.PDOP, "hdop", gsa.HDOP, "vdop", gsa.VDOP, "system", bd.Name, "band", bd.Band)

					nd.PDOP = gsa.PDOP
					nd.VDOP = gsa.VDOP
					nd.HDOP = gsa.HDOP
				case nmea.TypeGSV:
					gsv := s.(nmea.GSV)
					bd := parser.ParseBandDataWithSystemID(gsv.Talker, gsv.SystemID)
					var sats int64

					slog.Debug("Parsed GSV", "seq", fmt.Sprintf("%d of %d", gsv.MessageNumber, gsv.TotalMessages), "numbersvsinview", gsv.NumberSVsInView, "info", gsv.Info, "system", bd.Name, "band", bd.Band)

					for _, sv := range gsv.Info {
						if sv.SNR > 0 {
							sats++
						}
					}
					if sats > 0 {
						nd.SatCounts[SatConstellation{Constellation: bd.Constellation, Name: bd.Name, Band: bd.Band, Frequency: bd.Frequency}] += sats
					}
				case nmea.TypeRMC:
					rmc := s.(nmea.RMC)
					slog.Debug("Parsed RMC", "validity", rmc.Validity)
					nd.Locked = rmc.Validity == "A"
				case nmea.TypeGGA:
					gga := s.(nmea.GGA)
					slog.Debug("Parsed GGA", "fixquality", gga.FixQuality, "numsatellites", gga.NumSatellites, "hdop", gga.HDOP)

					nd.TotalSatellites = gga.NumSatellites
					nd.HDOP_GGA = gga.HDOP
				case nmea.TypeTXT:
					txt := s.(nmea.TXT)
					slog.Debug("Parsed TXT", "seq", fmt.Sprintf("%d of %d", txt.Number, txt.TotalNumber), "message", txt.Message)
				default:
					if !unknownTypeSeen[s.DataType()] {
						slog.Info("Received unknown NMEA message type, only logging once", "type", s.DataType())
						unknownTypeSeen[s.DataType()] = true
					} else {
						slog.Debug("Received unknown NMEA message type, ignoring", "type", s.DataType())
					}

				}
			}
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
	slog.Error("scan loop finished")
}
