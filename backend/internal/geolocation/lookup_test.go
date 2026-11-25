package geolocation

import (
	"path/filepath"
	"testing"
)

func TestResolveCityDatabase(t *testing.T) {
	for _, dbName := range []string{"GeoLite2-City-Test.mmdb", "GeoIP2-City-Test.mmdb"} {
		t.Run(dbName, func(t *testing.T) {
			lookup, err := Open(filepath.Join("testdata", dbName))
			if err != nil {
				t.Fatal(err)
			}
			defer lookup.Close()

			got, found, err := lookup.Resolve("81.2.69.160")
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatal("expected test public IP to resolve")
			}
			if got.CountryCode != "GB" || got.CountryName != "United Kingdom" || got.RegionCode != "ENG" || got.RegionName != "England" {
				t.Fatalf("unexpected geographic result: %+v", got)
			}
			if got.Latitude == nil || got.Longitude == nil || *got.Latitude != 51.5142 || *got.Longitude != -0.0931 {
				t.Fatalf("unexpected test coordinates: %+v", got)
			}
		})
	}
}

func TestResolveSkipsNonPublicAndUnknownAddresses(t *testing.T) {
	lookup, err := Open(filepath.Join("testdata", "GeoLite2-City-Test.mmdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer lookup.Close()

	for _, address := range []string{"10.0.0.1", "127.0.0.1", "2.2.2.2"} {
		got, found, err := lookup.Resolve(address)
		if err != nil {
			t.Errorf("Resolve(%q): %v", address, err)
			continue
		}
		if found {
			t.Errorf("Resolve(%q) unexpectedly found data: %+v", address, got)
		}
	}
	if _, _, err := lookup.Resolve("not-an-ip"); err == nil {
		t.Fatal("invalid IP address was accepted")
	}
}
