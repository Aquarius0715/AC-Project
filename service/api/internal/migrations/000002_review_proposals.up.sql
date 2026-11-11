-- IR785: the schema changes of IR778, IR782 and IR784, as the first migration after the baseline (000001 is frozen).
-- Each statement tolerates a database that has it already, so one built from docs/02-design/db/schema.sql by hand
-- (the dev database patched with ALTER) migrates too.

-- IR778: two saves at once never store one version number twice
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'rate_cards_contractor_org_id_version_key') THEN
    ALTER TABLE maintenance.rate_cards ADD CONSTRAINT rate_cards_contractor_org_id_version_key UNIQUE (contractor_org_id, version);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'sla_targets_tenant_id_plan_type_version_key') THEN
    ALTER TABLE maintenance.sla_targets ADD CONSTRAINT sla_targets_tenant_id_plan_type_version_key UNIQUE (tenant_id, plan_type, version);
  END IF;
END $$;

-- IR782: the report version a file was added in
ALTER TABLE maintenance.attachments ADD COLUMN IF NOT EXISTS added_version int NOT NULL DEFAULT 1;

-- IR784: when HQ was told that nobody acknowledged an alert in its policy's escalation time
ALTER TABLE monitoring.alerts ADD COLUMN IF NOT EXISTS escalated_at timestamptz;
