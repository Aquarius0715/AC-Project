-- New aircon installation (DEC-106, IR789): a unit can be planned before it is installed, and an installation job
-- (created by HQ, origin hq_request) installs it. The report of an installation job carries the confirmed equipment
-- and its photos are tagged. At most one unfinished installation job exists per unit.

ALTER TABLE assets.units
  ADD COLUMN install_state text NOT NULL DEFAULT 'installed' CHECK (install_state IN ('planned','installed'));

ALTER TABLE maintenance.jobs DROP CONSTRAINT jobs_type_check;
ALTER TABLE maintenance.jobs
  ADD CONSTRAINT jobs_type_check CHECK (type IN ('periodic','reactive','preventive','installation'));
ALTER TABLE maintenance.jobs DROP CONSTRAINT jobs_origin_check;
ALTER TABLE maintenance.jobs
  ADD CONSTRAINT jobs_origin_check CHECK (origin IN ('client_request','periodic_plan','hq_request'));
CREATE UNIQUE INDEX jobs_one_installation ON maintenance.jobs (tenant_id, unit_id)
  WHERE type = 'installation' AND status NOT IN ('completed','cancelled');

ALTER TABLE maintenance.work_reports ADD COLUMN installation jsonb;
ALTER TABLE maintenance.attachments
  ADD COLUMN tag text CHECK (tag IN ('name_plate','indoor_unit','outdoor_unit'));
