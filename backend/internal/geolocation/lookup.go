package geolocation

import (
	"net/netip"
	"strings"

	"github.com/oschwald/geoip2-golang/v2"
)

type Result struct {
	CountryCode string
	CountryName string
	RegionCode  string
	RegionName  string
	Latitude    *float64
	Longitude   *float64
}

type Lookup struct{ reader *geoip2.Reader }

func Open(path string) (*Lookup, error) {
	reader, err := geoip2.Open(path)
	if err != nil {
		return nil, err
	}
	return &Lookup{reader: reader}, nil
}

func (l *Lookup) Close() error { return l.reader.Close() }

func (l *Lookup) Resolve(value string) (Result, bool, error) {
	ip, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil {
		return Result{}, false, err
	}
	if !ip.IsValid() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return Result{}, false, nil
	}
	record, err := l.reader.City(ip)
	if err != nil {
		return Result{}, false, err
	}
	if !record.HasData() || record.Country.ISOCode == "" {
		return Result{}, false, nil
	}
	result := Result{CountryCode: record.Country.ISOCode, CountryName: record.Country.Names.English}
	if len(record.Subdivisions) > 0 {
		result.RegionCode = record.Subdivisions[0].ISOCode
		result.RegionName = record.Subdivisions[0].Names.English
	}
	if result.RegionCode == "" {
		result.RegionCode = "COUNTRY"
	}
	if result.RegionName == "" {
		result.RegionName = result.CountryName
	}
	if record.Location.Latitude != nil {
		value := *record.Location.Latitude
		result.Latitude = &value
	}
	if record.Location.Longitude != nil {
		value := *record.Location.Longitude
		result.Longitude = &value
	}
	return result, true, nil
}
