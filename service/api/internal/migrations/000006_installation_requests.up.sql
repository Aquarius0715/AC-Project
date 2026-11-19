-- Client installation requests (DEC-107, IR794, FR-C20): a customer owner asks HQ to install new aircons; HQ approves by
-- creating the planned units and the installation jobs (maintenance.jobs.installation_request_id links them) or
-- declines with a reason.

CREATE TABLE maintenance.installation_requests (   -- a customer owner's request for new aircons (DEC-107, FR-C20)
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id         uuid NOT NULL,
  customer_org_id   uuid NOT NULL,
  property_id       uuid NOT NULL,                 -- assets.properties of the customer (checked through the assets query interface, no cross-schema FK)
  space_id          uuid,
  quantity          int NOT NULL CHECK (quantity BETWEEN 1 AND 10),
  model_id          uuid,                          -- null = model not decided
  with_iot          boolean NOT NULL DEFAULT false,
  preferred_slots   jsonb NOT NULL,                -- Slot[3] (IR113)
  contact_window    text,
  note              text CHECK (char_length(note) <= 2000),
  status            text NOT NULL DEFAULT 'submitted' CHECK (status IN ('submitted','approved','declined','cancelled')),
  decline_reason    text CHECK (char_length(decline_reason) BETWEEN 5 AND 500),
  decided_by        uuid,
  decided_at        timestamptz,
  created_by        uuid NOT NULL,
  version           int NOT NULL DEFAULT 1,
  created_at        timestamptz NOT NULL DEFAULT platform.app_now(),
  updated_at        timestamptz NOT NULL DEFAULT platform.app_now(),
  UNIQUE (tenant_id, id),
  CHECK ((status = 'declined') = (decline_reason IS NOT NULL))
);
CREATE INDEX installation_requests_customer ON maintenance.installation_requests (tenant_id, customer_org_id, created_at DESC);
CREATE INDEX installation_requests_open ON maintenance.installation_requests (tenant_id, created_at) WHERE status = 'submitted';

ALTER TABLE maintenance.installation_requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE maintenance.installation_requests FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON maintenance.installation_requests
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON maintenance.installation_requests TO ac_app, ac_worker, ac_svc_maintenance;
GRANT SELECT ON maintenance.installation_requests TO ac_readonly;

ALTER TABLE maintenance.jobs ADD COLUMN installation_request_id uuid;
ALTER TABLE maintenance.jobs ADD FOREIGN KEY (tenant_id, installation_request_id)
  REFERENCES maintenance.installation_requests(tenant_id, id);
