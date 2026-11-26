-- Opt-in fixture for a fresh, isolated PostgreSQL database seeded from fixture-contract.json.
-- Expected business date: 2026-09-14 in Asia/Kuala_Lumpur. No commands are queued or sent.
BEGIN;
CREATE TEMP TABLE start_proposal_fixture ON COMMIT DROP AS
SELECT v.rule_id::uuid, v.rule_name, v.weekdays::jsonb, b.tenant_id, b.unit_id, b.device_id, b.id AS binding_id,
       b.customer_org_id AS org_id, m.id AS member_id, m.user_id
FROM (VALUES
  ('1188be39-61af-4bec-ac7d-bbf988f4dc01', 'Proposal fixture Wednesday', '[1,3,4]', 'AC-DEMO-0001'),
  ('1188be39-61af-4bec-ac7d-bbf988f4dc02', 'Proposal fixture Friday',    '[5]',     'AC-DEMO-0003'),
  ('1188be39-61af-4bec-ac7d-bbf988f4dc03', 'Proposal fixture unconfirmed','[3]',    'AC-DEMO-0004')
) AS v(rule_id, rule_name, weekdays, serial)
JOIN devices.devices d ON d.serial = v.serial
JOIN devices.device_bindings b ON b.id = d.binding_id AND b.unbound_at IS NULL
JOIN identity.memberships m ON m.organization_id = b.customer_org_id AND m.role = 'client'
JOIN identity.users u ON u.id = m.user_id AND u.display_name = 'customer-a';

DO $$
BEGIN
  IF (SELECT count(*) FROM start_proposal_fixture) <> 3 THEN
    RAISE EXCEPTION 'Load the standard development seed before the start proposal fixture';
  END IF;
END $$;

-- The standard seed dates its bindings on September 1. Move these fixture bindings before the test history.
UPDATE devices.device_bindings b SET bound_at = '2026-08-01T00:00:00Z'
FROM start_proposal_fixture f WHERE b.id = f.binding_id;

INSERT INTO control.automations (id, tenant_id, customer_org_id, kind, name, owner_membership_id,
  created_by_user_id, timezone, enabled, priority, definition)
SELECT rule_id, tenant_id, org_id, 'schedule', rule_name, member_id, user_id, 'Asia/Kuala_Lumpur', true, 50,
  jsonb_build_object('weekdays', weekdays, 'startLocal', '18:30', 'endLocal', '23:30', 'endsNextDay', false,
    'startAction', '{"kind":"set_temperature","celsius":24}'::jsonb, 'endAction', '{"kind":"set_power","power":false}'::jsonb)
FROM start_proposal_fixture;

INSERT INTO control.automation_units (tenant_id, automation_id, unit_id)
SELECT tenant_id, rule_id, unit_id FROM start_proposal_fixture;

CREATE TEMP TABLE start_proposal_actions ON COMMIT DROP AS
SELECT f.*, sample.at, sample.confirmed
FROM start_proposal_fixture f
JOIN (VALUES
  ('Proposal fixture Wednesday',   '2026-08-26T19:50:00+08:00'::timestamptz, true),
  ('Proposal fixture Wednesday',   '2026-09-02T20:00:00+08:00', true),
  ('Proposal fixture Wednesday',   '2026-09-09T20:10:00+08:00', true),
  ('Proposal fixture Friday',      '2026-08-28T21:05:00+08:00', true),
  ('Proposal fixture Friday',      '2026-09-04T21:15:00+08:00', true),
  ('Proposal fixture Friday',      '2026-09-11T21:25:00+08:00', true),
  ('Proposal fixture unconfirmed', '2026-08-26T20:00:00+08:00', false),
  ('Proposal fixture unconfirmed', '2026-09-02T20:00:00+08:00', false),
  ('Proposal fixture unconfirmed', '2026-09-09T20:00:00+08:00', false)
) AS sample(rule_name, at, confirmed) ON sample.rule_name = f.rule_name;

INSERT INTO control.commands (tenant_id, unit_id, device_id, actor_membership_id, source, action, status,
  delivery, requested_at, sent_at, acknowledged_at, expires_at, correlation_id)
SELECT tenant_id, unit_id, device_id, member_id, 'ui', '{"kind":"set_power","power":true}', 'acknowledged',
  'sent', at, at, at + interval '1 second', at + interval '30 seconds', 'start-proposal-web-fixture'
FROM start_proposal_actions;

INSERT INTO control.power_observations (tenant_id, binding_id, observed_at, received_at, power)
SELECT tenant_id, binding_id, at + sample.offset_from_request, at + sample.offset_from_request, sample.power
FROM start_proposal_actions
CROSS JOIN (VALUES (interval '-1 minute', false), (interval '1 minute', true)) AS sample(offset_from_request, power)
WHERE confirmed;
COMMIT;
