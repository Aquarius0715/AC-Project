-- Opt-in power ranges verified for one sensor on one binding. There are no default ranges.
CREATE TABLE control.start_power_bands (
  tenant_id       uuid NOT NULL,
  binding_id      uuid NOT NULL,
  sensor_id       uuid NOT NULL,
  verified_from   timestamptz NOT NULL,
  standby_max_kw  double precision NOT NULL CHECK (standby_max_kw >= 0),
  running_min_kw  double precision NOT NULL CHECK (running_min_kw > standby_max_kw AND running_min_kw < 'Infinity'),
  PRIMARY KEY (tenant_id, binding_id)
);

ALTER TABLE control.start_power_bands ENABLE ROW LEVEL SECURITY;
ALTER TABLE control.start_power_bands FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON control.start_power_bands
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
GRANT SELECT ON control.start_power_bands TO ac_app, ac_worker, ac_svc_equipment, ac_readonly;
