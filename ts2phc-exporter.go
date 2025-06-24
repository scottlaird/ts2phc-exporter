package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/adrianmo/go-nmea"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	debug       = flag.Bool("debug", false, "Enable debugging output")
	nmeaVariant = flag.String("nmea_variant", "4.11", "Which NMEA version to speak.  Use 4.11 for ublox F9 and F10, and 4.10 for M8.")

	nemaRE   = regexp.MustCompile(".*nmea sentence: (.*)")
	offsetRE = regexp.MustCompile(".*(/dev/ptp[0-9]+) offset +([-0-9]+) .* freq +([-+0-9]+)")

	UBLOX_NMEA411 = map[string]string{
		"GN 1": "GPS",
		"GN 2": "GLONASS",
		"GN 3": "Galileo",
		"GN 4": "BeiDou",
		"GN 5": "QZSS",
		"GN 6": "NavIC",
		"GP 0": "GPS x",
		"GP 1": "GPS L1",
		"GP 6": "GPS L2 CL",
		"GP 5": "GPS L2 CM",
		"GP 7": "GPS L5 I",
		"GP 8": "GPS L6 Q",
		"GL 0": "GLONASS unknown",
		"GL 1": "GLONASS L1",
		"GL 3": "GLONASS L2",
		"GA 0": "Galileo unknown",
		"GA 1": "Galileo E5 aI/aQ",
		"GA 2": "Galileo E5 bI/bQ",
		"GA 4": "Galileo E6 A",
		"GA 5": "Galileo E6 B/C",
		"GA 7": "Galileo E1 B/C",
		"GB 0": "BeiDou unknown",
		"GB 1": "BeiDou B1 D1/D2",
		"GB 3": "BeiDou B1 Cp",
		"GB 5": "BeiDou B2 ad/ap",
		"GB 8": "BeiDou B2I/B3I",
		//"GP 1": "SBAS L1C/A",
		"GQ 0": "QZSS unknown",
		"GQ 1": "QZSS L1C/A",
		"GQ 4": "QZSS L1S",
		"GQ 5": "QZSS L2 CM",
		"GQ 6": "QZSS L2 CL",
		"GQ 7": "QZSS L5 I",
		"GQ 8": "QZSS L5 Q",
		"GI 0": "NavIC unknown",
		"GI 1": "NavIC L5 A",
	}
	UBLOX_NMEA410 = map[string]string{
		"GN": "Generic GNSS",
		"GP": "GPS",
		"GL": "GLONASS",
		"GA": "Galileo",
		"GB": "BeiDou",
		"GQ": "QZSS",
		"GI": "NavIC",
	}

	satAzElevCounts  *prometheus.CounterVec
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

// By default go-nmea refuses to parse any NMEA sentences without a
// checksum.  This replaces the default checksum checker.
func ignoreCNC(sentence nmea.BaseSentence, rawFields string) error {
	return nil
}

// This really needs to be flagged by receiver vendor and/or NMEA rev.
// Assuming Ublox / NMEA 4.11 for now.
//
// We need two pieces of info to decode this:
//
// 1.  We need the first 2 bytes of the NMEA message
// 2.  The system ID from the GSV, etc message.
//
// If the systemID is 0, then I think we're seeing "we used to see
// these sats but lost them" update.
func systemIDName(sentence string, systemID int64) string {
	switch *nmeaVariant {
	case "4.10":
		key := sentence[1:3]
		if v, ok := UBLOX_NMEA410[key]; ok {
			return v
		}

		return key

	case "4.11":
		key := fmt.Sprintf("%s %d", sentence[1:3], systemID)
		if v, ok := UBLOX_NMEA411[key]; ok {
			return v
		}

		// Not a huge fan of this here.
		return key
	default:
		return sentence[1:3]
	}
}

type NMEAData struct {
	SatAzElevCounts            map[string]map[string]int
	SatCounts                  map[string]int64
	Locked                     bool
	TotalSatellites            int64
	PDOP, VDOP, HDOP, HDOP_GGA float64
	Device                     string
	Offset                     int
	Freq                       int
}

func ResetNMEAData(nd *NMEAData) {
	nd.SatAzElevCounts = make(map[string]map[string]int)
	nd.SatCounts = make(map[string]int64)
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
		satCounts.With(prometheus.Labels{"constellation": c}).Set(float64(v))
	}

	for a, ec := range nd.SatAzElevCounts {
		for e, v := range ec {
			satAzElevCounts.With(prometheus.Labels{"az": a, "elev": e}).Add(float64(v))
		}
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
		totalSatellites.Set(float64(nd.SatCounts["GPS"])) // Not a terrible fallback if we don't have GGA data.
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

	reg := prometheus.NewRegistry()

	satAzElevCounts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ts2phc_sat_az_elev_counts",
			Help: "Number of satellites seen at each azimuth and elevation",
		},
		[]string{"az", "elev"})
	reg.MustRegister(satAzElevCounts)

	satCounts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ts2phc_sat_counts",
			Help: "Current number of satellites by constellation",
		},
		[]string{"constellation"})
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
	http.ListenAndServe(":8089", nil)
}

func ReadLogs() {
	cmd := exec.Command("journalctl", "-u", "ts2phc", "-f")

	logs, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}

	err = cmd.Start()
	if err != nil {
		panic(err)
	}

	nmeaParser := nmea.SentenceParser{
		CheckCRC: ignoreCNC,
	}

	scanner := bufio.NewScanner(logs)

	nd := &NMEAData{}
	ResetNMEAData(nd)

	for scanner.Scan() {
		line := scanner.Text()
		if *debug {
			fmt.Printf("-> %s\n", line)
		}

		if nmeaMatch := nemaRE.FindStringSubmatch(line); nmeaMatch != nil {
			sentence := "$" + nmeaMatch[1]

			s, err := nmeaParser.Parse(sentence)
			if err != nil {
				fmt.Printf("---> NMEA match failed: %v\n", err)
			} else {
				switch s.DataType() {
				case nmea.TypeGSA:
					gsa := s.(nmea.GSA)
					if *debug {
						fmt.Printf("---> GSA: mode=%s fixtype=%s SV=%v pdop=%f hdop=%f vdop=%f systemID=%s\n", gsa.Mode, gsa.FixType, gsa.SV, gsa.PDOP, gsa.HDOP, gsa.VDOP, systemIDName(sentence, gsa.SystemID))
					}
					nd.PDOP = gsa.PDOP
					nd.VDOP = gsa.VDOP
					nd.HDOP = gsa.HDOP
				case nmea.TypeGSV:
					gsv := s.(nmea.GSV)
					if *debug {
						fmt.Printf("---> GSV: %d/%d numbersvsinview=%d info=%v systemID=%s\n", gsv.MessageNumber, gsv.TotalMessages, gsv.NumberSVsInView, gsv.Info, systemIDName(sentence, gsv.SystemID))
					}

					if *nmeaVariant == "4.10" || gsv.SystemID != 0 {
						for _, sv := range gsv.Info {
							az := strconv.Itoa(int(sv.Azimuth))
							elev := strconv.Itoa(int(sv.Elevation))

							if _, ok := nd.SatAzElevCounts[az]; !ok {
								nd.SatAzElevCounts[az] = make(map[string]int)
							}
							nd.SatAzElevCounts[az][elev]++
						}
						nd.SatCounts[systemIDName(sentence, gsv.SystemID)] = gsv.NumberSVsInView
					}
				case nmea.TypeRMC:
					rmc := s.(nmea.RMC)
					if *debug {
						fmt.Printf("---> RMC: validity=%s\n", rmc.Validity)
					}
					nd.Locked = rmc.Validity == "A"
				case nmea.TypeGGA:
					gga := s.(nmea.GGA)
					if *debug {
						fmt.Printf("---> GGA: fixquality=%s numsatellites=%d hdop=%f separation=%f\n", gga.FixQuality, gga.NumSatellites, gga.HDOP, gga.Separation)
					}

					nd.TotalSatellites = gga.NumSatellites
					nd.HDOP_GGA = gga.HDOP
				case nmea.TypeTXT:
					txt := s.(nmea.TXT)
					if *debug {
						fmt.Printf("---> TXT: %d/%d %q\n", txt.Number, txt.TotalNumber, txt.Message)
					}
				default:
					if *debug {
						fmt.Printf("---> got NMEA type %s\n", s.DataType())
					}
				}
			}
		} else if offsetMatch := offsetRE.FindStringSubmatch(line); offsetMatch != nil {
			if *debug {
				fmt.Printf("--> Offset: %s | %s | %s\n", offsetMatch[1], offsetMatch[2], offsetMatch[3])
			}

			nd.Device = offsetMatch[1]
			nd.Offset, _ = strconv.Atoi(offsetMatch[2])
			nd.Freq, _ = strconv.Atoi(offsetMatch[3])

			PublishNMEAData(nd)
			ResetNMEAData(nd)
		} else {
			if *debug {
				fmt.Printf("--> no match: %s\n", line)
			}
		}

	}
}
