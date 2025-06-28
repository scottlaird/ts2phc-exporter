package parser

import (
	"fmt"
)

var (
	UBLOX_NMEA411 = map[string]BandData{
		"GN 1": {Constellation: "GPS", Name: "GPS", Band: "", Frequency: ""},
		"GN 2": {Constellation: "GLONASS", Name: "GLONASS", Band: "", Frequency: ""},
		"GN 3": {Constellation: "Galileo", Name: "Galileo", Band: "", Frequency: ""},
		"GN 4": {Constellation: "BeiDou", Name: "BeiDou", Band: "", Frequency: ""},
		"GN 5": {Constellation: "QZSS", Name: "QZSS", Band: "", Frequency: ""},
		"GN 6": {Constellation: "NavIC", Name: "NavIC", Band: "", Frequency: ""},
		"GP 0": {Constellation: "GPS", Name: "GPS", Band: "", Frequency: ""},

		"GP 1": {Constellation: "GPS", Name: "GPS L1", Band: "1", Frequency: "L1"},
		"GP 6": {Constellation: "GPS", Name: "GPS L2 CL", Band: "2", Frequency: "L2"},
		"GP 5": {Constellation: "GPS", Name: "GPS L2 CM", Band: "2", Frequency: "L2"},
		"GP 7": {Constellation: "GPS", Name: "GPS L5 I", Band: "5", Frequency: "L5"},
		"GP 8": {Constellation: "GPS", Name: "GPS L6 Q", Band: "6", Frequency: "L6"},

		"GL 0": {Constellation: "GLONASS", Name: "GLONASS unknown", Band: "1", Frequency: "L1"},
		"GL 1": {Constellation: "GLONASS", Name: "GLONASS L1", Band: "1", Frequency: "L1"},
		"GL 3": {Constellation: "GLONASS", Name: "GLONASS L2", Band: "2", Frequency: "L2"},

		"GA 0": {Constellation: "Galileo", Name: "Galileo unknown", Band: "", Frequency: ""},
		"GA 1": {Constellation: "Galileo", Name: "Galileo E5 aI/aQ", Band: "5", Frequency: "E5"},
		"GA 2": {Constellation: "Galileo", Name: "Galileo E5 bI/bQ", Band: "5", Frequency: "E5"},
		"GA 4": {Constellation: "Galileo", Name: "Galileo E6 A", Band: "6", Frequency: "E6"},
		"GA 5": {Constellation: "Galileo", Name: "Galileo E6 B/C", Band: "6", Frequency: "E6"},
		"GA 7": {Constellation: "Galileo", Name: "Galileo E1 B/C", Band: "1", Frequency: "E1"},

		"GB 0": {Constellation: "BeiDou", Name: "BeiDou unknown", Band: "", Frequency: ""},
		"GB 1": {Constellation: "BeiDou", Name: "BeiDou B1 D1/D2", Band: "1", Frequency: "B1"},
		"GB 3": {Constellation: "BeiDou", Name: "BeiDou B1 Cp", Band: "1", Frequency: "B1"},
		"GB 5": {Constellation: "BeiDou", Name: "BeiDou B2 ad/ap", Band: "2", Frequency: "B2"},
		"GB 8": {Constellation: "BeiDou", Name: "BeiDou B2I/B3I", Band: "2", Frequency: "B2"},

		"GQ 0": {Constellation: "QZSS", Name: "QZSS unknown", Band: "", Frequency: ""},
		"GQ 1": {Constellation: "QZSS", Name: "QZSS L1C/A", Band: "1", Frequency: "L1"},
		"GQ 4": {Constellation: "QZSS", Name: "QZSS L1S", Band: "1", Frequency: "L1"},
		"GQ 5": {Constellation: "QZSS", Name: "QZSS L2 CM", Band: "2", Frequency: "L2"},
		"GQ 6": {Constellation: "QZSS", Name: "QZSS L2 CL", Band: "2", Frequency: "L2"},
		"GQ 7": {Constellation: "QZSS", Name: "QZSS L5 I", Band: "5", Frequency: "L5"},
		"GQ 8": {Constellation: "QZSS", Name: "QZSS L5 Q", Band: "5", Frequency: "L5"},

		"GI 0": {Constellation: "NavIC", Name: "NavIC unknown", Band: "", Frequency: ""},
		"GI 1": {Constellation: "NavIC", Name: "NavIC L5 A", Band: "5", Frequency: "L5"},
	}

	GENERIC_NMEA410 = map[string]BandData{
		"GN": {Constellation: "", Name: "Generic GNSS", Band: "", Frequency: ""},
		"GP": {Constellation: "GPS", Name: "GPS", Band: "", Frequency: ""},
		"GL": {Constellation: "GLONASS", Name: "GLONASS", Band: "", Frequency: ""},
		"GA": {Constellation: "Galileo", Name: "Galileo", Band: "", Frequency: ""},
		"GB": {Constellation: "BeiDou", Name: "BeiDou", Band: "", Frequency: ""},
		"GQ": {Constellation: "QZSS", Name: "QZSS", Band: "", Frequency: ""},
		"GI": {Constellation: "NavIC", Name: "NavIC", Band: "", Frequency: ""},
	}
)

type BandData struct {
	SystemID      int64
	Name          string
	Constellation string
	Talker        string
	Band          string
	Frequency     string
}

func ParseBandDataWithSystemID(talker string, systemID int64) BandData {
	r := BandData{
		SystemID: systemID,
		Talker:   talker,
	}

	key := fmt.Sprintf("%s %d", r.Talker, r.SystemID)
	if v, ok := UBLOX_NMEA411[key]; ok {
		r.Constellation = v.Constellation
		r.Name = v.Name
		r.Band = v.Band
		r.Frequency = v.Frequency
	}

	return r
}

func ParseBandData(talker string) BandData {
	r := BandData{
		Talker: talker,
	}

	if v, ok := GENERIC_NMEA410[talker]; ok {
		r.Name = v.Name
		r.Band = v.Band
	}

	return r
}

type SatData struct {
	BandData
	SatID          int
	Azimuth        int
	Elevation      int
	SignalStrength int
}
