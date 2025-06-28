package parser

import (
	"fmt"
	"github.com/adrianmo/go-nmea"
	"log/slog"
)

var (
	unknownTypeSeen = map[string]bool{}
)

type SatConstellation struct {
	Constellation string
	Name          string
	Band          string
	Frequency     string
}

type NMEAData struct {
	Sats                       []SatData
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

func ParseNMEALogEntry(sentence string, nd *NMEAData) {
	sentence = "$" + sentence

	nmeaParser := nmea.SentenceParser{
		CheckCRC: ignoreCNC,
	}

	s, err := nmeaParser.Parse(sentence)
	if err != nil {
		slog.Info("NMEA parser failed", "error", err)
	} else {
		switch s.DataType() {
		case nmea.TypeGSA:
			gsa := s.(nmea.GSA)
			bd := ParseBandDataWithSystemID(gsa.Talker, gsa.SystemID)
			slog.Debug("Parsed GSA", "mode", gsa.Mode, "fixtype", gsa.FixType, "sv", gsa.SV, "pdop", gsa.PDOP, "hdop", gsa.HDOP, "vdop", gsa.VDOP, "system", bd.Name, "band", bd.Band)

			nd.PDOP = gsa.PDOP
			nd.VDOP = gsa.VDOP
			nd.HDOP = gsa.HDOP
		case nmea.TypeGSV:
			gsv := s.(nmea.GSV)
			bd := ParseBandDataWithSystemID(gsv.Talker, gsv.SystemID)
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
}
