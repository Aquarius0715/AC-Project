package iotbridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// errBadLinks refuses a links file whose entry has no device or no unit, or names a device twice.
var errBadLinks = errors.New("bad links file")

// linkRecord is one entry of a links file; the file may hold more keys (the load test marks its probes), which are
// ignored here.
type linkRecord struct {
	Device     string            `json:"device"`
	Unit       string            `json:"unit"`
	Sensors    map[string]string `json:"sensors"`
	Connection string            `json:"connection"`
}

// LoadLinks reads the links of devices that are not in the demo fixture, from a file {"links":[{device, unit, sensors,
// connection}]} (the load test writes one with cmd/loadseed). An empty path reads nothing. A device with no connection
// is taken as online, as Links does.
func LoadLinks(path string) ([]Link, error) {
	if path == "" {
		return nil, nil
	}

	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, apperr.Wrap(err, "os.ReadFile")
	}

	var file struct {
		Links []linkRecord `json:"links"`
	}

	err = json.Unmarshal(raw, &file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	seen := map[string]bool{}
	out := make([]Link, 0, len(file.Links))

	for _, record := range file.Links {
		if record.Device == "" || record.Unit == "" || seen[record.Device] {
			return nil, fmt.Errorf("%w: %s: device %q", errBadLinks, path, record.Device)
		}

		seen[record.Device] = true

		connection := record.Connection
		if connection == "" {
			connection = connectionOnline
		}

		out = append(
			out,
			Link{
				Device:     record.Device,
				Unit:       record.Unit,
				Sensors:    record.Sensors,
				Connection: connection,
			},
		)
	}

	return out, nil
}
