-- A property's map pin and away/home radius (DEC-109, IR823): HQ sets the pin in Admin; technician check-in and
-- customer location automations measure against it. The latitude and longitude are set or cleared together.
ALTER TABLE assets.properties
  ADD COLUMN IF NOT EXISTS latitude double precision CHECK (latitude BETWEEN -90 AND 90),
  ADD COLUMN IF NOT EXISTS longitude double precision CHECK (longitude BETWEEN -180 AND 180),
  ADD COLUMN IF NOT EXISTS geofence_radius_m int CHECK (geofence_radius_m BETWEEN 50 AND 2000);
ALTER TABLE assets.properties DROP CONSTRAINT IF EXISTS properties_location_pair;
ALTER TABLE assets.properties ADD CONSTRAINT properties_location_pair CHECK ((latitude IS NULL) = (longitude IS NULL));
