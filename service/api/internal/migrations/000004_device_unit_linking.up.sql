-- A device can be registered before it is planned for a unit (spare or pre-installation stock): target_unit_id is
-- optional. Binding rules (one active binding per device and per unit) are unchanged: device_bindings keeps its two
-- partial unique indexes.

ALTER TABLE devices.devices ALTER COLUMN target_unit_id DROP NOT NULL;

-- The IoT model of a device is the device's own (devices.devices.iot_model_id); the unit's iot_model_id is only a
-- plan. Rows where the two disagree are reported, never rewritten: the device and its history stay as registered.
DO $$
DECLARE
  n int;
BEGIN
  SELECT count(*) INTO n
  FROM assets.units u
  JOIN devices.devices d ON d.unit_id = u.id
  WHERE u.iot_model_id IS NOT NULL AND d.iot_model_id IS DISTINCT FROM u.iot_model_id;
  IF n > 0 THEN
    RAISE NOTICE '% unit(s) have a planned IoT model that differs from the IoT model of their bound device (left as it is)', n;
  END IF;
END $$;
