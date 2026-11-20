-- Independently measured AC power. Command acknowledgements and IR transmitter settings must never populate this table.
CREATE TABLE control.power_observations (
  tenant_id   uuid NOT NULL,
  binding_id  uuid NOT NULL,
  observed_at timestamptz NOT NULL,
  received_at timestamptz NOT NULL CHECK (received_at >= observed_at),
  power       boolean,                           -- NULL when the physical state could not be established
  PRIMARY KEY (tenant_id, binding_id, observed_at)
);

ALTER TABLE control.power_observations ENABLE ROW LEVEL SECURITY;
ALTER TABLE control.power_observations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON control.power_observations
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
GRANT SELECT ON control.power_observations TO ac_app, ac_worker, ac_svc_equipment, ac_readonly;
