-- DEC-105: the IoT model is its own register. devices.capabilities keeps the air conditioner's controls; the sensors and
-- the firmware candidates of a model move to devices.iot_models. Nothing is deleted: the IoT rows are copies (same id
-- and version, so firmware campaigns and unit references stay valid), and capabilities.sensors / firmware_candidates
-- stay in place, unread, until a later migration drops them.

CREATE TABLE IF NOT EXISTS devices.iot_models (       -- IoT model register; one row per version
  id                  uuid NOT NULL,
  version             int NOT NULL,
  tenant_id           uuid NOT NULL,
  manufacturer        text NOT NULL,
  model               text NOT NULL,
  sensors             jsonb NOT NULL DEFAULT '[]',
  firmware_candidates text[] NOT NULL DEFAULT '{}',
  is_current          boolean NOT NULL DEFAULT true,
  created_at          timestamptz NOT NULL DEFAULT platform.app_now(),
  PRIMARY KEY (id, version)
);
CREATE UNIQUE INDEX IF NOT EXISTS iot_models_current ON devices.iot_models (id) WHERE is_current;
CREATE UNIQUE INDEX IF NOT EXISTS iot_models_model ON devices.iot_models (tenant_id, lower(manufacturer), lower(model)) WHERE is_current;

ALTER TABLE devices.iot_models ENABLE ROW LEVEL SECURITY;
ALTER TABLE devices.iot_models FORCE ROW LEVEL SECURITY;
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE schemaname = 'devices' AND tablename = 'iot_models' AND policyname = 'tenant_isolation') THEN
    CREATE POLICY tenant_isolation ON devices.iot_models
      USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
      WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
  END IF;
END $$;
GRANT SELECT, INSERT, UPDATE, DELETE ON devices.iot_models TO ac_app, ac_worker, ac_svc_equipment;
GRANT SELECT ON devices.iot_models TO ac_readonly;

-- the unit's planned IoT model, and the model a device was registered with (sensor check and firmware updates)
ALTER TABLE assets.units ADD COLUMN IF NOT EXISTS iot_model_id uuid;
ALTER TABLE assets.units ADD COLUMN IF NOT EXISTS iot_model_version int;
ALTER TABLE devices.devices ADD COLUMN IF NOT EXISTS iot_model_id uuid;
ALTER TABLE devices.devices ADD COLUMN IF NOT EXISTS iot_model_version int;

-- copy: every capability version that has a sensor or a firmware candidate becomes an IoT model version
INSERT INTO devices.iot_models (id, version, tenant_id, manufacturer, model, sensors, firmware_candidates, is_current, created_at)
SELECT c.id, c.version, c.tenant_id, c.manufacturer, c.model, c.sensors, c.firmware_candidates, c.is_current, c.created_at
FROM devices.capabilities c
WHERE (jsonb_array_length(c.sensors) > 0 OR cardinality(c.firmware_candidates) > 0)
  AND EXISTS (SELECT 1 FROM devices.capabilities k WHERE k.id = c.id AND (jsonb_array_length(k.sensors) > 0 OR cardinality(k.firmware_candidates) > 0))
ON CONFLICT (id, version) DO NOTHING;

UPDATE assets.units u SET iot_model_id = m.id, iot_model_version = m.version
FROM devices.iot_models m
WHERE m.id = u.model_id AND m.version = u.capability_version AND u.iot_model_id IS NULL;

UPDATE devices.devices d SET iot_model_id = u.iot_model_id, iot_model_version = u.iot_model_version
FROM assets.units u
WHERE u.id = d.unit_id AND u.iot_model_id IS NOT NULL AND d.iot_model_id IS NULL;
