package migrate

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pradita/ac-project/service/api/internal/migrations"
)

// breakingMarker is the line a migration names its reason on when the code before it cannot run on the schema after
// it: the shared test environment then restarts the whole stack instead of rolling one service at a time (IR855).
const breakingMarker = "-- migrate:breaking"

// requiredWithoutDefault is the one rule a DEFAULT in the same statement answers.
const requiredWithoutDefault = "adds a required column without a default"

type breakingRule struct {
	why string
	re  *regexp.Regexp
}

// breakingRules lists what the services of the deployed tag cannot run on: a rolling deployment applies the migration
// while they still serve (IR855). A new column takes a default or null, and a rename is a new column, a copy and,
// later, a migration that drops the old one under breakingMarker.
func breakingRules() []breakingRule {
	return []breakingRule{
		{
			"drops a table, a view or a schema",
			regexp.MustCompile(`\bdrop\s+(table|view|materialized\s+view|schema)\b`),
		},
		{"drops a column", regexp.MustCompile(`\bdrop\s+column\b`)},
		{"renames", regexp.MustCompile(`\brename\b`)},
		{
			"changes a column's type",
			regexp.MustCompile(`\balter\s+column\s+\S+\s+(set\s+data\s+)?type\b`),
		},
		{"makes a column required", regexp.MustCompile(`\bset\s+not\s+null\b`)},
		{requiredWithoutDefault, regexp.MustCompile(`\badd\s+column\b[^,]*\bnot\s+null\b`)},
	}
}

var (
	lineComment = regexp.MustCompile(`--[^\n]*`)
	hasDefault  = regexp.MustCompile(`\bdefault\b`)
)

// breakingParts names what each statement of one migration does that the code before it cannot run on.
func breakingParts(sql string) []string {
	var found []string

	rules := breakingRules()

	for stmt := range strings.SplitSeq(strings.ToLower(lineComment.ReplaceAllString(sql, "")), ";") {
		for _, rule := range rules {
			if !rule.re.MatchString(stmt) {
				continue
			}

			if rule.why == requiredWithoutDefault && hasDefault.MatchString(stmt) {
				continue
			}

			found = append(found, rule.why+": "+strings.Join(strings.Fields(stmt), " "))
		}
	}

	return found
}

// breakingReason is the reason a migration gives on its breakingMarker line: "" when it has none.
func breakingReason(sql string) (string, bool) {
	for line := range strings.SplitSeq(sql, "\n") {
		if reason, ok := strings.CutPrefix(strings.TrimSpace(line), breakingMarker); ok {
			return strings.TrimSpace(reason), true
		}
	}

	return "", false
}

// compatProblems checks every migration after the baseline of fsys.
func compatProblems(t *testing.T, fsys fs.FS) []string {
	t.Helper()

	list, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}

	var problems []string

	for _, migration := range list {
		if migration.Version == 1 { // the baseline builds an empty database: nothing runs on it before
			continue
		}

		reason, marked := breakingReason(migration.SQL)
		if marked && reason == "" {
			problems = append(problems, migration.Name+": "+breakingMarker+" gives no reason")
		}

		if marked {
			continue
		}

		for _, part := range breakingParts(migration.SQL) {
			problems = append(problems, migration.Name+": "+part)
		}
	}

	return problems
}

// TestMigrationsKeepTheDeployedCodeRunning: a migration runs while the services of the tag before it still serve
// (IR855), so it only adds, unless it says why not on a breakingMarker line.
func TestMigrationsKeepTheDeployedCodeRunning(t *testing.T) {
	t.Parallel()

	for _, problem := range compatProblems(t, migrations.FS) {
		t.Errorf(
			"%s: the deployed code cannot run on it; add instead, or name the reason on a %q line (IR855)",
			problem,
			breakingMarker,
		)
	}
}

func TestBreakingMigrationsAreFound(t *testing.T) {
	t.Parallel()

	file := func(sql string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(sql), Mode: 0, ModTime: time.Time{}, Sys: nil}
	}
	fsys := fstest.MapFS{
		"000001_base.up.sql": file("CREATE TABLE a.t (id int, name text);"),
		"000002_adds.up.sql": file("ALTER TABLE a.t ADD COLUMN n int NOT NULL DEFAULT 0;\n" +
			"ALTER TABLE a.t ADD COLUMN m text;\n-- drop column x later\n" +
			"ALTER TABLE a.t ALTER COLUMN name DROP NOT NULL;"),
		"000003_drops.up.sql":     file("ALTER TABLE a.t DROP COLUMN name;"),
		"000004_rename_id.up.sql": file("ALTER TABLE a.t RENAME COLUMN id TO key;"),
		"000005_types.up.sql": file(
			"ALTER TABLE a.t ALTER COLUMN n TYPE bigint;\nALTER TABLE a.t ALTER COLUMN m SET NOT NULL;",
		),
		"000006_needs.up.sql": file("ALTER TABLE a.t ADD COLUMN k int NOT NULL;"),
		"000007_marked.up.sql": file(
			"-- migrate:breaking the old reader of a.u is gone since test1.0.0r20\nDROP TABLE a.u;",
		),
		"000008_blank.up.sql": file("-- migrate:breaking\nDROP VIEW a.v;"),
	}

	got := strings.Join(compatProblems(t, fsys), "\n")

	for _, want := range []string{
		"drops: drops a column: alter table a.t drop column name",
		"rename_id: renames: alter table a.t rename column id to key",
		"types: changes a column's type: alter table a.t alter column n type bigint",
		"types: makes a column required: alter table a.t alter column m set not null",
		"needs: adds a required column without a default: alter table a.t add column k int not null",
		"blank: -- migrate:breaking gives no reason",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	for _, quiet := range []string{"adds", "marked", "base"} {
		if strings.Contains(got, quiet+":") {
			t.Errorf("%s reported:\n%s", quiet, got)
		}
	}
}
