import csv, re
D = '/Users/ji1wxs/PraditaProjects/AC_Project/docs/02-design/'
ops = list(csv.DictReader(open(D + 'operation-catalog.csv')))
tables = set(re.findall(r'^CREATE TABLE ([a-z_]+\.[a-z_0-9]+)', open(D + 'db/schema.sql').read(), re.M))

# prefix -> (module, default read tables, default write tables)
P = {
 'admin': ('reporting', 'assets.customers;assets.units;monitoring.alerts;maintenance.jobs;billing.invoices;energy.baselines', ''),
 'alerts': ('monitoring', 'monitoring.alerts', 'monitoring.alerts'),
 'attachments': ('maintenance', 'maintenance.attachments', 'maintenance.attachments'),
 'audit': ('audit', 'audit.audit_log', ''),
 'auth': ('identity', 'identity.users', ''),
 'automations': ('control', 'control.automations;control.automation_units;control.automation_runs', 'control.automations;control.automation_units'),
 'baselines': ('energy', 'energy.baselines', 'energy.baselines'),
 'capabilities': ('devices', 'devices.capabilities', 'devices.capabilities'),
 'certificates': ('maintenance', 'maintenance.certificates', 'maintenance.certificates'),
 'clientUsers': ('identity', 'identity.client_users;identity.memberships', 'identity.client_users;identity.memberships'),
 'commands': ('control', 'control.commands', 'control.commands'),
 'consents': ('identity', 'identity.consents', 'identity.consents'),
 'contractors': ('maintenance', 'maintenance.contractors', 'maintenance.contractors'),
 'contracts': ('billing', 'billing.contracts;billing.contract_units', 'billing.contracts;billing.contract_units'),
 'customers': ('assets', 'assets.customers', 'assets.customers'),
 'demo': ('platform', 'platform.tenants', 'platform.tenants'),
 'demoSession': ('identity', 'identity.memberships', ''),
 'devices': ('devices', 'devices.devices;devices.sensors;devices.device_bindings', 'devices.devices'),
 'diagnosticRuns': ('control', 'control.diagnostic_runs', 'control.diagnostic_runs'),
 'energy': ('energy', 'monitoring.measurements_1h;energy.baselines;energy.emission_factors', ''),
 'factors': ('energy', 'energy.emission_factors', 'energy.emission_factors'),
 'filterCare': ('maintenance', 'maintenance.filter_care_settings;maintenance.filter_cleanings', 'maintenance.filter_care_settings'),
 'firmwareCampaigns': ('devices', 'devices.firmware_campaigns;devices.firmware_campaign_devices', 'devices.firmware_campaigns;devices.firmware_campaign_devices'),
 'inquiries': ('billing', 'billing.inquiries', 'billing.inquiries'),
 'invoices': ('billing', 'billing.invoices', 'billing.invoices'),
 'jobs': ('maintenance', 'maintenance.jobs;maintenance.job_events', 'maintenance.jobs;maintenance.job_events'),
 'locations': ('assets', 'assets.properties;assets.spaces', 'assets.properties;assets.spaces'),
 'members': ('identity', 'identity.memberships;identity.membership_permissions;identity.membership_scopes;identity.qualification_grants', 'identity.memberships;identity.membership_permissions;identity.membership_scopes'),
 'mrv': ('energy', 'energy.mrv_reports', 'energy.mrv_reports'),
 'notifications': ('notify', 'notify.notifications', 'notify.notifications'),
 'offsets': ('energy', 'energy.offset_quotes;energy.offset_records;energy.offset_attempts', 'energy.offset_quotes;energy.offset_records;energy.offset_attempts'),
 'organizations': ('identity', 'identity.organizations', 'identity.organizations'),
 'parts': ('maintenance', 'maintenance.parts_catalog', ''),
 'payments': ('billing', 'billing.payments;billing.invoices', 'billing.payments;billing.invoices'),
 'payouts': ('billing', 'billing.payout_statements;billing.payout_lines;billing.payout_queries', 'billing.payout_statements;billing.payout_lines'),
 'plans': ('maintenance', 'maintenance.plans', 'maintenance.plans'),
 'policies': ('monitoring', 'monitoring.alert_policies;monitoring.default_rule_settings', 'monitoring.alert_policies'),
 'preferences': ('identity', 'identity.preferences', 'identity.preferences'),
 'properties': ('assets', 'assets.properties', 'assets.properties'),
 'rateCards': ('maintenance', 'maintenance.rate_cards', 'maintenance.rate_cards'),
 'reports': ('maintenance', 'maintenance.work_reports;maintenance.report_reviews', 'maintenance.work_reports'),
 'restrictions': ('restrictions', 'restrictions.restrictions;restrictions.restriction_invoices;restrictions.restriction_units', 'restrictions.restrictions;restrictions.restriction_units'),
 'session': ('identity', 'identity.users;identity.memberships;identity.membership_permissions;identity.membership_scopes', ''),
 'sla': ('maintenance', 'maintenance.sla_targets;maintenance.jobs', 'maintenance.sla_targets'),
 'spaces': ('assets', 'assets.spaces', 'assets.spaces'),
 'summaries': ('reporting', 'assets.units;monitoring.alerts;maintenance.jobs;billing.invoices', ''),
 'telemetry': ('monitoring', 'monitoring.measurements;monitoring.measurements_15m;monitoring.measurements_1h', ''),
 'twoFactor': ('identity', '', ''),
 'units': ('assets', 'assets.units', 'assets.units'),
 'ventilation': ('monitoring', 'monitoring.ventilation_logs', 'monitoring.ventilation_logs'),
 'voice': ('control', 'assets.units', ''),
 'writes': ('platform', 'platform.idempotency_keys', ''),
}
# per-operation overrides: (reads, writes)
O = {
 'alerts.acknowledge': (None, 'monitoring.alerts'),
 'attachments.getContent': ('maintenance.attachments', ''),
 'automations.fire': ('control.automations;control.automation_units', 'control.automation_runs;control.commands'),
 'automations.simulate': ('control.automations;control.automation_units', ''),
 'automations.nextRuns': ('control.automations;platform.scheduled_items', ''),
 'certificates.requestTraining': (None, 'maintenance.certificates'),
 'certificates.verify': (None, 'maintenance.certificates'),
 'commands.create': ('control.commands;assets.units;devices.devices;restrictions.restriction_units', 'control.commands'),
 'demo.advanceClock': ('platform.scheduled_items', 'platform.tenants;platform.scheduled_items'),
 'demo.reset': ('platform.tenants', 'platform.tenants'),
 'demo.trigger': ('platform.tenants', 'platform.tenants'),
 'demoSession.extend': ('identity.memberships', ''),
 'demoSession.signIn': ('identity.users;identity.memberships', ''),
 'demoSession.signOut': ('identity.memberships', ''),
 'demoSession.switchMembership': ('identity.memberships', ''),
 'devices.addResponseNote': ('devices.device_events', 'devices.device_event_notes'),
 'devices.bind': (None, 'devices.device_bindings;devices.device_operations'),
 'devices.calibrate': ('devices.sensors', 'devices.calibration_records;devices.device_operations'),
 'devices.calibrations': ('devices.calibration_records', ''),
 'devices.check': ('devices.devices;devices.device_operations', 'devices.device_operations'),
 'devices.events': ('devices.device_events;devices.device_event_notes', ''),
 'devices.operations': ('devices.device_operations', ''),
 'devices.register': (None, 'devices.devices;devices.sensors;devices.device_bindings;devices.device_operations'),
 'devices.updateFirmware': (None, 'devices.device_operations'),
 'energy.exportReport': (None, ''),
 'filterCare.getSettings': ('maintenance.filter_care_settings', ''),
 'filterCare.markCleaned': (None, 'maintenance.filter_cleanings;maintenance.filter_reminders'),
 'firmwareCampaigns.control': (None, 'devices.firmware_campaigns'),
 'inquiries.answer': (None, 'billing.inquiries'),
 'invoices.remind': ('billing.invoices', 'billing.invoices;notify.notifications'),
 'jobs.accept': ('maintenance.jobs;maintenance.offers', 'maintenance.jobs;maintenance.offers;maintenance.job_events'),
 'jobs.acknowledgeAssignment': ('maintenance.jobs;maintenance.assignments', 'maintenance.assignments;maintenance.job_events'),
 'jobs.addNote': ('maintenance.jobs', 'maintenance.job_notes;maintenance.job_events'),
 'jobs.assign': ('maintenance.jobs;identity.memberships;identity.qualification_grants;maintenance.unavailability', 'maintenance.jobs;maintenance.assignments;maintenance.job_events'),
 'jobs.decline': ('maintenance.jobs;maintenance.offers', 'maintenance.jobs;maintenance.offers;maintenance.job_events'),
 'jobs.events': ('maintenance.job_events', ''),
 'jobs.extendAccess': ('maintenance.assignments', 'maintenance.assignments;maintenance.job_events'),
 'jobs.offer': ('maintenance.jobs;maintenance.contractors', 'maintenance.jobs;maintenance.offers;maintenance.job_events'),
 'jobs.proposePartnerSlot': (None, 'maintenance.partner_slot_proposals;maintenance.job_events'),
 'jobs.proposeSlot': (None, 'maintenance.jobs;maintenance.slot_proposals;maintenance.job_events'),
 'jobs.resolvePartnerSlot': (None, 'maintenance.jobs;maintenance.partner_slot_proposals;maintenance.job_events'),
 'jobs.respondProposal': (None, 'maintenance.jobs;maintenance.slot_proposals;maintenance.job_events'),
 'jobs.withdrawPartnerSlot': (None, 'maintenance.partner_slot_proposals;maintenance.job_events'),
 'jobs.withdrawProposal': (None, 'maintenance.jobs;maintenance.slot_proposals;maintenance.job_events'),
 'jobs.review': ('maintenance.jobs;maintenance.work_reports', 'maintenance.jobs;maintenance.report_reviews;maintenance.job_events'),
 'jobs.saveDraft': ('maintenance.jobs;maintenance.work_reports', 'maintenance.work_reports'),
 'jobs.submit': ('maintenance.jobs;maintenance.work_reports', 'maintenance.jobs;maintenance.work_reports;maintenance.job_events'),
 'jobs.create': ('assets.units;maintenance.plans', 'maintenance.jobs;maintenance.job_alerts;maintenance.job_events'),
 'members.capacity': ('identity.memberships;maintenance.assignments;maintenance.unavailability', ''),
 'members.eligible': ('identity.memberships;identity.qualification_grants;maintenance.certificates;maintenance.unavailability', ''),
 'members.setUnavailability': (None, 'maintenance.unavailability'),
 'mrv.preview': ('monitoring.measurements_1h;energy.baselines;energy.emission_factors', ''),
 'mrv.factors': ('energy.emission_factors', ''),
 'mrv.recordReview': (None, 'energy.mrv_reports;energy.mrv_reviews'),
 'mrv.versions': ('energy.mrv_reports', ''),
 'notifications.preview': ('notify.templates', 'notify.notifications'),
 'notifications.recipients': ('identity.memberships;identity.preferences;identity.consents', ''),
 'payments.simulate': ('billing.invoices', 'billing.payments'),
 'payouts.generate': ('maintenance.jobs;maintenance.rate_cards', 'billing.payout_statements;billing.payout_lines'),
 'payouts.query': (None, 'billing.payout_queries'),
 'payouts.resolveQuery': (None, 'billing.payout_queries;billing.payout_lines'),
 'plans.generateNext': ('maintenance.plans', 'maintenance.plans;maintenance.jobs;maintenance.job_events'),
 'policies.delete': ('monitoring.alert_policies;control.automation_policies', 'monitoring.alert_policies;monitoring.default_rule_settings;control.automation_policies;assets.unit_alert_policies'),
 'policies.get': ('monitoring.alert_policies;monitoring.default_rule_settings;control.automation_policies;control.automation_policy_units;assets.unit_alert_policies', ''),
 'policies.list': ('monitoring.alert_policies;monitoring.default_rule_settings;control.automation_policies;control.automation_policy_units;assets.unit_alert_policies;assets.units', ''),
 'policies.save': ('monitoring.alert_policies;assets.customers;identity.memberships', 'monitoring.alert_policies;control.automation_policies;control.automation_policy_units'),
 'policies.setDefaultRule': ('monitoring.alert_policies;assets.customers', 'monitoring.default_rule_settings'),
 'reports.signOff': (None, 'maintenance.work_reports;maintenance.job_events'),
 'restrictions.forInvoice': ('restrictions.restriction_invoices;restrictions.restrictions', ''),
 'restrictions.execute': (None, 'restrictions.restrictions;restrictions.restriction_units;control.commands'),
 'restrictions.release': (None, 'restrictions.restrictions;restrictions.restriction_units;control.commands'),
 'restrictions.retry': (None, 'restrictions.restriction_units;control.commands'),
 'restrictions.schedule': ('billing.invoices;billing.contracts', 'restrictions.restrictions;restrictions.restriction_invoices;restrictions.restriction_units;platform.scheduled_items'),
 'sla.scorecard': ('maintenance.sla_targets;maintenance.jobs;maintenance.job_events', ''),
 'units.coverage': ('assets.units;billing.contracts;billing.contract_units', ''),
 'units.importCommit': ('assets.unit_import_previews;assets.unit_imports', 'assets.unit_imports;assets.properties;assets.spaces;assets.units;assets.unit_import_previews'),
 'units.importPreview': ('assets.customers;assets.properties;assets.spaces;assets.units;devices.capabilities;devices.devices;devices.device_bindings', ''),
 'units.importUndo': ('assets.unit_imports', 'assets.unit_imports;assets.units'),
 'units.resolveQr': ('assets.units;devices.device_bindings', ''),
 'units.setAlertPolicies': ('monitoring.alert_policies', 'assets.unit_alert_policies'),
 'ventilation.log': ('monitoring.measurements;assets.spaces;assets.units', 'monitoring.ventilation_logs'),
 'ventilation.list': ('monitoring.ventilation_logs;assets.units', ''),
 'telemetry.series': ('monitoring.measurements;monitoring.allergen_observations;assets.units', ''),
 'telemetry.summary': ('monitoring.measurements;assets.units', ''),
 'voice.resolveIntent': ('assets.units;control.commands', ''),
}
rows = []
bad = []
for o in ops:
    name = o['operation']; pre = name.split('.')[0]
    mod, rd, wr = P[pre]
    if name in O:
        r2, w2 = O[name]
        if r2 is not None: rd = r2
        wr = w2
    if o['mode'] != 'write': wr = ''
    if o['mode'] == 'write' and not wr and name != 'energy.exportReport':
        wr = ''
    side = []
    if o['mode'] == 'write':
        side = ['audit.audit_log', 'platform.idempotency_keys', 'platform.outbox']
    for t in (rd + ';' + wr).split(';'):
        t = t.strip()
        if t and t not in tables: bad.append((name, t))
    note = ''
    if pre == 'demoSession': note = 'session store (ElastiCache / local Valkey) via the BFF; no PostgreSQL row'
    if pre == 'twoFactor': note = 'TOTP factor lives in Cognito (Keycloak locally); audit row only'
    if name == 'auth.previewPasswordReset': note = 'preview only; real reset is a Cognito flow'
    if name == 'units.importPreview': note = 'stores the 30-minute preview in assets.unit_import_previews outside the read transaction (no business data)'
    if name == 'energy.exportReport': note = 'generated by the importexport worker; file in S3'
    if pre in ('admin','summaries'): note = 'read model assembled from module query interfaces (no cross-schema joins)'
    rows.append({'operation': name, 'mode': o['mode'], 'module': mod, 'reads': rd, 'writes': wr, 'write_side_effects': ';'.join(side), 'notes': note})
assert not bad, bad
MOD = {'reporting':('Read models',''),'monitoring':('Monitoring & alerts','monitoring'),'maintenance':('Maintenance','maintenance'),'audit':('Audit','audit'),'identity':('Identity & access','identity'),'control':('Control','control'),'energy':('Energy & carbon','energy'),'devices':('Devices','devices'),'billing':('Billing','billing'),'assets':('Assets','assets'),'platform':('Demo','platform'),'notify':('Notifications','notify'),'restrictions':('Restrictions','restrictions')}
SCH2MOD = {v[1]:v[0] for v in MOD.values() if v[1]}
out=[]
for r in rows:
    name=r['operation']
    if name.startswith('writes.'): r['module']='audit'
    if name in ('members.capacity','members.eligible','members.setUnavailability'): r['module']='maintenance'
    mname, sch = MOD[r['module']]
    if name.startswith('writes.'): sch='platform'
    def split(ts):
        own=[t for t in ts.split(';') if t and t.split('.')[0]==sch]
        oth=[t for t in ts.split(';') if t and t.split('.')[0]!=sch]
        return own, oth
    ro, rx = split(r['reads']); wo, wx = split(r['writes'])
    calls = sorted({SCH2MOD.get(t.split('.')[0], t.split('.')[0]) + (' (write)' if t in wx else '') for t in rx+wx})
    out.append({'operation':name,'mode':r['mode'],'module':mname,'own_reads':';'.join(ro),'own_writes':';'.join(wo),'other_module_calls':';'.join(calls),'write_side_effects':r['write_side_effects'],'notes':r['notes']})
rows=out
#W
w = csv.DictWriter(open(D + 'operation-persistence-map.csv', 'w', newline=''), fieldnames=list(rows[0].keys()))
w.writeheader(); w.writerows(rows)
print(len(rows),'rows; writes without own table:',[r['operation'] for r in rows if r['mode']=='write' and not r['own_writes']])
