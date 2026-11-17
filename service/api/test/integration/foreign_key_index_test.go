package integration_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
)

// unindexedForeignKeys are the foreign keys found without an index in R913, left for the decision of B-52 (most are
// small tables or keys that are never searched by). A key leaves the list when it gets its index.
func unindexedForeignKeys() []string {
	return []string{
		"assets.spaces FOREIGN KEY (tenant_id, parent_space_id) REFERENCES assets.spaces(tenant_id, id)",
		"assets.units FOREIGN KEY (tenant_id, space_id) REFERENCES assets.spaces(tenant_id, id)",
		"billing.inquiries FOREIGN KEY (invoice_id) REFERENCES billing.invoices(id)",
		"billing.invoices FOREIGN KEY (contract_id, contract_version) REFERENCES billing.contracts(id, version)",
		"billing.payout_lines FOREIGN KEY (statement_id) REFERENCES billing.payout_statements(id) ON DELETE CASCADE",
		"billing.payout_queries FOREIGN KEY (line_id) REFERENCES billing.payout_lines(id)",
		"billing.payout_queries FOREIGN KEY (statement_id) REFERENCES billing.payout_statements(id)",
		"billing.stripe_events FOREIGN KEY (payment_id) REFERENCES billing.payments(id)",
		"control.automation_runs FOREIGN KEY (command_id) REFERENCES control.commands(id)",
		"control.diagnostic_runs FOREIGN KEY (end_command_id) REFERENCES control.commands(id)",
		"control.diagnostic_runs FOREIGN KEY (start_command_id) REFERENCES control.commands(id)",
		"devices.calibration_records FOREIGN KEY (sensor_id) REFERENCES devices.sensors(id)",
		"devices.device_event_notes FOREIGN KEY (event_id) REFERENCES devices.device_events(id)",
		"devices.firmware_campaign_devices FOREIGN KEY (device_id) REFERENCES devices.devices(id)",
		"devices.firmware_campaign_devices FOREIGN KEY (operation_id) REFERENCES devices.device_operations(id)",
		"energy.mrv_reviews FOREIGN KEY (report_id, report_version) REFERENCES energy.mrv_reports(id, version)",
		"energy.offset_attempts FOREIGN KEY (record_id) REFERENCES energy.offset_records(id)",
		"energy.offset_records FOREIGN KEY (quote_id) REFERENCES energy.offset_quotes(id)",
		"identity.membership_permissions FOREIGN KEY (permission) REFERENCES identity.permissions(code)",
		"maintenance.attachments FOREIGN KEY (tenant_id, job_id) REFERENCES maintenance.jobs(tenant_id, id)",
		"maintenance.certificates FOREIGN KEY (renewal_of) REFERENCES maintenance.certificates(id)",
		"maintenance.job_events FOREIGN KEY (note_id) REFERENCES maintenance.job_notes(id)",
		"maintenance.job_notes FOREIGN KEY (tenant_id, job_id) REFERENCES maintenance.jobs(tenant_id, id)",
		"maintenance.jobs FOREIGN KEY (follow_up_of_job_id) REFERENCES maintenance.jobs(id)",
		"maintenance.partner_slot_proposals FOREIGN KEY (offer_id) REFERENCES maintenance.offers(id)",
		"monitoring.alerts FOREIGN KEY (previous_alert_id) REFERENCES monitoring.alerts(id)",
	}
}

// TestForeignKeysHaveIndexes (R913): PostgreSQL indexes the referenced key of a foreign key but not the referencing
// columns, so a lookup by them ("the jobs of this installation request") reads the whole table. A foreign key has an
// index whose leading columns hold the key's columns other than tenant_id, or is on the list of B-52.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestForeignKeysHaveIndexes(t *testing.T) {
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}

	defer func() { _ = conn.Close(ctx) }()

	rows, err := conn.Query(
		ctx,
		`SELECT c.conrelid::regclass::text || ' ' || pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		WHERE c.contype = 'f' AND c.connamespace NOT IN ('pg_catalog'::regnamespace, 'information_schema'::regnamespace)
			AND NOT EXISTS (
				SELECT 1 FROM pg_index i
				WHERE i.indrelid = c.conrelid
					AND (i.indkey::int2[])[0:cardinality(c.conkey) - 1] @> ARRAY(
						SELECT k FROM unnest(c.conkey) k
						WHERE k <> ALL (SELECT attnum FROM pg_attribute WHERE attrelid = c.conrelid AND attname = 'tenant_id')))
		ORDER BY 1`,
	)
	if err != nil {
		t.Fatal(err)
	}

	missing, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}

	known := unindexedForeignKeys()
	missing = slices.DeleteFunc(
		missing,
		func(key string) bool { return slices.Contains(known, key) },
	)

	if len(missing) > 0 {
		t.Errorf("foreign keys without an index on their columns: %q", missing)
	}
}
