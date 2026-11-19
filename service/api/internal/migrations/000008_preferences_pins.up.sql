-- The ACs a user pinned to their Overview (IR819): kept per user, so the pins follow them to another device.
ALTER TABLE identity.preferences ADD COLUMN IF NOT EXISTS pinned_unit_ids uuid[] NOT NULL DEFAULT '{}'
  CHECK (cardinality(pinned_unit_ids) <= 8);
