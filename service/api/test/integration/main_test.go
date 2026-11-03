package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// TestMain runs the suite, then checks that every job event action the suite wrote has a title in the web's job history
// (jobEventTitle, shared by the contractor's and HQ's job pages). Without one the history shows the raw action; IR439
// and IR440 each added an action that needed its title by hand.
func TestMain(m *testing.M) {
	code := m.Run()
	if code == 0 {
		err := checkJobEventTitles()
		if err != nil {
			fmt.Fprintln(os.Stderr, "job event titles:", err)

			code = 1
		}
	}

	os.Exit(code)
}

// errNoTitleTable and errMissingTitles are the failures of the job event title check.
var (
	errNoTitleTable  = errors.New("jobEventTitle not found in partnerOverview.ts")
	errMissingTitles = errors.New("no jobEventTitle in service/web/shared/lib/partnerOverview.ts")
)

// webTitles is the key set of jobEventTitle in service/web/shared/lib/partnerOverview.ts.
func webTitles() (map[string]bool, error) {
	src, err := os.ReadFile("../../../web/shared/lib/partnerOverview.ts")
	if err != nil {
		return nil, apperr.Wrap(err, "os.ReadFile")
	}

	source := string(src)

	start := strings.Index(source, "export const jobEventTitle")
	if start < 0 {
		return nil, errNoTitleTable
	}

	end := strings.Index(source[start:], "};")

	keys := map[string]bool{}
	for _, k := range regexp.MustCompile(`"?([a-z_]+(?:\.[a-z_]+)?)"?\s*:\s*"`).FindAllStringSubmatch(
		source[start:start+end],
		-1) {
		keys[k[1]] = true
	}

	return keys, nil
}

func checkJobEventTitles() error {
	titles, err := webTitles()
	if err != nil {
		return err
	}

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		return nil // no database: the suite skipped its database tests too
	}
	defer func() { _ = conn.Close(ctx) }()

	rows, err := conn.Query(ctx, `SELECT DISTINCT action FROM maintenance.job_events`)
	if err != nil {
		return apperr.Wrap(err, "select maintenance.job_events")
	}

	actions, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return apperr.Wrap(err, "select maintenance.job_events")
	}

	var missing []string

	for _, a := range actions {
		if !titles[a] {
			missing = append(missing, a)
		}
	}

	slices.Sort(missing)

	if len(missing) > 0 {
		return fmt.Errorf("%w for %v", errMissingTitles, missing)
	}

	return nil
}
