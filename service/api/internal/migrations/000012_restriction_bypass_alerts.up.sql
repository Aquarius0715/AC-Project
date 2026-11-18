-- DEC-110 / IR844: a device fault reported while its unit observes a restriction (the node went silent, lost power or
-- was opened) opens a restriction_bypass Alert, one per unit: the restriction is sent by infrared, so a hidden,
-- unplugged or opened node may mean the restriction no longer holds.
ALTER TABLE monitoring.alerts DROP CONSTRAINT alerts_type_check;
ALTER TABLE monitoring.alerts ADD CONSTRAINT alerts_type_check
  CHECK (type IN ('sensor','quality','maintenance','tamper','reconciliation_required','restriction_bypass'));
