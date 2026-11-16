-- Customer away/home presence (DEC-109, IR826): presence.report keeps, per member and pinned property, only whether the
-- member's device is inside the property's away/home radius. The household's first arrival and last departure become
-- location facts for the property's units. A withdrawn location consent deletes the member's rows.

CREATE TABLE control.presence_states (          -- DEC-109/IR826: a member's device inside a property's away/home radius; no position is kept
  tenant_id     uuid NOT NULL,
  membership_id uuid NOT NULL,                   -- the reporting client member (identity; consent location_automation)
  property_id   uuid NOT NULL,                   -- assets.properties with a pin (read through Assets, no cross-schema FK)
  inside        boolean NOT NULL,
  changed_at    timestamptz NOT NULL,            -- when inside last changed
  reported_at   timestamptz NOT NULL,            -- the last report that kept or changed it
  PRIMARY KEY (tenant_id, membership_id, property_id)
);
CREATE INDEX presence_states_property ON control.presence_states (tenant_id, property_id) WHERE inside;

ALTER TABLE control.presence_states ENABLE ROW LEVEL SECURITY;
ALTER TABLE control.presence_states FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON control.presence_states
  USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
  WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
GRANT SELECT, INSERT, UPDATE, DELETE ON control.presence_states TO ac_app, ac_worker, ac_svc_equipment;
GRANT SELECT ON control.presence_states TO ac_readonly;
