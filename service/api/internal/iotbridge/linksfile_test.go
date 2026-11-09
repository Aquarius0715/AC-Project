package iotbridge_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/iotbridge"
)

func writeLinks(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "links.json")

	err := os.WriteFile(path, []byte(body), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return path
}

func TestLoadLinksReadsTheDevicesOfAFile(t *testing.T) {
	t.Parallel()

	path := writeLinks(t, `{"links":[
		{"device":"d1","unit":"u1","sensors":{"temperature":"s1"},"connection":"online","probe":true},
		{"device":"d2","unit":"u2","sensors":{}}]}`)

	links, err := iotbridge.LoadLinks(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(links) != 2 || links[0].Device != "d1" || links[0].Sensors["temperature"] != "s1" ||
		links[1].Connection != "online" {
		t.Errorf("links = %+v", links)
	}

	// the bridge resolves a reading of such a device the way it resolves a demo one
	resolver := iotbridge.NewResolver(links)
	if resolver == nil {
		t.Fatal("no resolver")
	}
}

func TestLoadLinksRefusesAFileThatCannotBeTrusted(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"no device":   `{"links":[{"unit":"u"}]}`,
		"no unit":     `{"links":[{"device":"d"}]}`,
		"a duplicate": `{"links":[{"device":"d","unit":"u"},{"device":"d","unit":"v"}]}`,
		"broken":      `[`,
	} {
		_, err := iotbridge.LoadLinks(writeLinks(t, body))
		if err == nil {
			t.Errorf("%s: LoadLinks accepted the file", name)
		}
	}

	_, err := iotbridge.LoadLinks(filepath.Join(t.TempDir(), "missing.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: %v", err)
	}

	links, err := iotbridge.LoadLinks("")
	if err != nil || links != nil {
		t.Errorf("an empty path reads nothing: %v %v", links, err)
	}
}
