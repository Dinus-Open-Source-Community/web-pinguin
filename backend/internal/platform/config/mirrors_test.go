package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadMirrorsRejectsUnsafeOrMalformedTargets(t *testing.T) {
	encode := func(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
	t.Setenv("MIRROR_TARGETS", "")
	entries := []string{
		encode("valid") + "|" + encode("https://mirror.example.org/"),
		encode("credential") + "|" + encode("https://user:secret@mirror.example.org/"),
		encode("plain-http") + "|" + encode("http://mirror.example.org/"),
		encode("private") + "|" + encode("https://10.0.0.8/"),
		encode(" ") + "|" + encode("https://other.example.org/"),
		"not-base64|also-not-base64",
	}
	t.Setenv("MIRROR_TARGETS", strings.Join(entries, ","))

	got := loadMirrors()
	if len(got) != 1 || got[0].Name != "valid" || got[0].URL != "https://mirror.example.org/" {
		t.Fatalf("loadMirrors() = %#v, want only the valid public HTTPS target", got)
	}
}
