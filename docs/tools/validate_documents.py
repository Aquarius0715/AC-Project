#!/usr/bin/env python3
"""Validate the current 1A document baseline. No application tests are executed."""
from pathlib import Path
import argparse
import csv
import hashlib
import json
import re
import sys
import subprocess
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[1]
RUN = ROOT / '04-agentic-sdlc/runs/TRANSLATION-EN-2026-09-17'
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--write-baseline', action='store_true', help='Rehash current documents after static checks; does not approve G1')
parser.add_argument('--tsc', type=Path, help='Path to installed TypeScript lib/tsc.js; optional semantic check')
args = parser.parse_args()
errors = []

def fail(message):
    errors.append(message)

def rows(name):
    with (ROOT / name).open(newline='') as stream:
        reader = csv.DictReader(stream)
        if len(reader.fieldnames) != len(set(reader.fieldnames)):
            fail(f'Duplicate CSV header: {name}')
        result = list(reader)
    for index, row in enumerate(result, 2):
        if None in row or any(value is None for value in row.values()):
            fail(f'CSV column mismatch: {name}:{index}')
    return result

def unique(items, key, label):
    values = [row[key] for row in items]
    if len(values) != len(set(values)):
        fail(f'Duplicate {label}')
    return set(values)

def anchors(path):
    result = set()
    occurrences = {}
    for line in path.read_text().splitlines():
        if re.match(r'^#{1,6} ', line):
            title = re.sub(r'^#+ ', '', line).strip().lower()
            slug = re.sub(r'[^\w\s\-]', '', title).replace(' ', '-')
            count = occurrences.get(slug, 0)
            occurrences[slug] = count + 1
            result.add(slug + (f'-{count}' if count else ''))
    return result

# Source records are immutable inputs, not prose to rewrite or lint.
markdown = [p for p in ROOT.rglob('*.md') if not any(x in p.parts for x in ['runs', '05-document-review', 'sources'])]
for path in markdown:
    content = path.read_text()
    if content.count('```') % 2:
        fail(f'Unclosed fence: {path.relative_to(ROOT)}')
    for url in re.findall(r'\]\(([^)]+)\)', content):
        if re.match(r'[a-z]+://', url):
            continue
        name, sep, fragment = url.partition('#')
        target = (path.parent / unquote(name)).resolve() if name else path
        if not target.exists():
            # The baseline is created by --write-baseline below.
            if args.write_baseline and target == RUN / 'spec-manifest.json':
                continue
            fail(f'Missing link: {path.relative_to(ROOT)} -> {url}')
        elif sep and target.suffix == '.md' and unquote(fragment) not in anchors(target):
            fail(f'Missing anchor: {path.relative_to(ROOT)} -> {url}')
    previous = None
    fenced = False
    for number, line in enumerate(content.splitlines(), 1):
        if line.startswith('```'):
            fenced = not fenced
        if fenced:
            continue
        if line.startswith('|'):
            columns = len(re.split(r'(?<!\\)\|', line))
            if previous is not None and columns != previous:
                fail(f'Markdown columns: {path.relative_to(ROOT)}:{number}')
            previous = columns
        else:
            previous = None

trace = rows('00-prepare/traceability.csv')
expected = {f'FR-{letter}{i:02}' for letter, n in [('C',13),('P',8),('T',12),('A',16),('X',7)] for i in range(1,n+1)} | {f'NFR-{i:02}' for i in range(1,9)}
if unique(trace, 'requirement_id', 'requirement') != expected:
    fail('Requirement coverage must be 64 original IDs')
acceptance = set()
for row in trace:
    req = (ROOT / row['requirement_file']).read_text()
    if row['requirement_id'] not in req:
        fail('Missing requirement '+row['requirement_id'])
    designs = [(ROOT / name).read_text() for name in row['design_file'].split(';')]
    for did in row['design_id'].split(';'):
        if not any(did in text for text in designs):
            fail('Missing design '+did)
    for field in ['acceptance_case_ids', 'source_acceptance_ids']:
        for case in filter(None, row[field].split(';')):
            if case not in req:
                fail('Missing acceptance '+case)
            acceptance.add(case)
    if row['execution_status'] != 'not_run':
        fail('Unsubstantiated app test status '+row['requirement_id'])
    for field in ['deterministic_contract', 'screen_catalog', 'operation_catalog']:
        if not (ROOT / row[field]).is_file():
            fail('Missing trace reference '+row[field])

operations = rows('02-design/operation-catalog.csv')
opnames = unique(operations, 'operation', 'operation')
opmap = {row['operation']: row for row in operations}
types = (ROOT / '02-design/service-contracts.ts').read_text()
typed = {name:(input_, result, mode) for name,input_,result,mode in re.findall(r"^  '([^']+)': \{input:(.*);result:(.*);mode:'(read|write)'\};$", types, re.M)}
if set(typed) != opnames or len(operations) != 137:
    fail('137 operation/TypeScript contract keys differ')
for row in operations:
    name = row['operation']
    if typed.get(name) != (row['input_contract'], row['result_contract'], row['mode']):
        fail('Contract drift: '+name)
    if not row['authorization'] or row['mode'] not in ['read', 'write']:
        fail('Missing mode/authorization '+name)
    if row['frontend_execution'] not in ['mock-service', 'local-preference', 'demo-only']:
        fail('Unexpected transport '+name)
    if row['result_contract'].startswith('Page<') and 'Query' not in row['input_contract']:
        fail('Unpageable Page '+name)
for name in ['telemetry.series','telemetry.summary','automations.simulate','notifications.preview']:
    if opmap[name]['mode'] != 'read':
        fail('Read operation marked write '+name)
if opmap['offsets.preview']['mode'] != 'write':
    fail('Quote snapshot must be a write')

screens = rows('03-uiux/screen-catalog.csv')
unique(screens, 'screen_id', 'screen ID')
unique(screens, 'route', 'screen route')
covered = set()
for screen in screens:
    covered.update(screen['requirement_ids'].split(';'))
    for op in filter(None,screen['operations'].split(';')):
        if op not in opnames:
            fail('Unknown screen operation '+op)
    for field in ['primary_queries','secondary_queries']:
        for op in screen[field].split(';'):
            if op != 'none' and (op not in opmap or opmap[op]['mode'] != 'read'):
                fail(f'{screen["screen_id"]}: non-read query {op}')
    # IR90 (REV19-027): public screens without data queries have their own state sets.
    public_states = {'SCR-X-login':'initial;loading;success;error;offline','SCR-X-forgot-password':'initial;loading;success;error;offline','SCR-X-demo':'initial;loading;success;error','SCR-X-forbidden':'success','SCR-X-not-found':'success'}
    if screen['screen_id'] in public_states:
        if screen['states'] != public_states[screen['screen_id']]:
            fail('Public screen states differ from IR90 '+screen['screen_id'])
    else:
        required_states = {'initial','loading','success','empty','error','offline','permission-denied'}
        if not required_states <= set(screen['states'].split(';')):
            fail('Missing UI states '+screen['screen_id'])
role_requirements = {x for x in expected if re.match(r'FR-[CPAT]', x)}
if not role_requirements <= covered:
    fail('Screen requirement gaps '+str(sorted(role_requirements-covered)))
components = rows('03-uiux/component-contracts.csv')
unique(components, 'component', 'component')
for row in components:
    if any(not value for value in row.values()):
        fail('Incomplete component '+row['component'])
queries = rows('02-design/query-catalog.csv')
query_names = unique(queries, 'operation', 'query')
if query_names != {row['operation'] for row in operations if 'Query' in row['input_contract']}:
    fail('Query allowlist coverage drift')

role_tables = 0
for role in ['client','contractor','technician','admin']:
    for line in (ROOT / f'02-design/{role}.md').read_text().splitlines():
        if not line.startswith('| DD-'):
            continue
        role_tables += 1
        cells = line.split('|')
        did = cells[1].split('/')[0].strip()
        for op in re.findall(r'[a-zA-Z]+\.[a-zA-Z]+', cells[3]):
            if op not in opmap or did not in opmap[op]['design_ids'].split(';'):
                fail('DD/catalog mismatch '+did+' '+op)
if role_tables != 49:
    fail('Expected 49 role requirement rows')
# FRV-011: detail-section service boundary sentences must equal the summary table (single source of operations).
for role in ['client','contractor','technician','admin']:
    text = (ROOT / f'02-design/{role}.md').read_text()
    top = {}
    for line in text.splitlines():
        if line.startswith('| DD-'):
            cells = line.split('|')
            top[cells[1].split('/')[0].strip()] = re.findall(r'[a-zA-Z]+\.[a-zA-Z]+', cells[3])
    for did, ops in top.items():
        section = re.search(r'### '+did+r' Details(.*?)(?=\n### |\Z)', text, re.S)
        if not section:
            fail('Missing detail section '+did); continue
        found = re.findall(r'Service boundary: `([^`]*)`', section.group(1))
        if not found or [o.strip() for o in found[0].split(',')] != ops:
            fail('Detail service boundary drift '+did)
fixes = rows('04-agentic-sdlc/acceptance-fixes.csv')
if unique(fixes,'review_id','review coverage') != {f'REV-{i:03}' for i in range(1,37)}:
    fail('36 review findings not covered')
unique(fixes,'case_id','fix case')
for row in fixes:
    if not all(row[key] for key in ['given','when','then','contract_section']) or row['execution_status'] != 'not_run':
        fail('Invalid acceptance plan '+row['case_id'])
independent_cases = rows('04-agentic-sdlc/acceptance-independent.csv')
if unique(independent_cases, 'finding_id', 'independent finding') != {f'IND-{i:03}' for i in range(1,8)}:
    fail('Independent review coverage must include IND-001 through IND-007')
for row in independent_cases:
    if row['execution_status'] != 'not_run' or not all(row[k] for k in ['given','when','then','contract']):
        fail('Invalid independent acceptance plan '+row['case_id'])
fixture = json.loads((ROOT / '04-agentic-sdlc/fixture-contract.json').read_text())
permission_type = re.search(r'export type Permission = ([^;]+);', types)[1]
permissions = set(re.findall(r"'([^']+)'", permission_type))
for actor in fixture['actors']:
    if not set(actor['permissions']) <= permissions:
        fail('Unknown fixture permission '+actor['membershipId'])
for path in markdown:
    if path.parent.name not in ['01-requirements','02-design','03-uiux']:
        continue
    for forbidden in ['Switch between Japanese and English', 'IDs and versions do not change', 'fewer than 9 characters', 'RTO (remote restriction', 'RTO (remote control support', 'i18n/en,ja','日英の切り替え','IDとバージョンは変わらない','9文字未満','RTO(遠隔制限','RTO(遠隔操作対応']:
        if forbidden in path.read_text():
            fail(f'Reintroduced contradiction: {path.name}: {forbidden}')

# Strict-review contract invariants: these validate document consistency, not app behavior.
strict_cases = rows('04-agentic-sdlc/acceptance-strict-review.csv')
if unique(strict_cases, 'finding_id', 'strict finding') != {f'REV-{i:03}' for i in range(1,22)}:
    fail('Strict review must cover 21 findings')
strict_ids = unique(strict_cases, 'case_id', 'strict case')
for case in strict_cases:
    if case['review_id'] != 'STRICT-DOC-0.8.0-2026-09-16' or case['execution_status'] != 'not_run':
        fail('Strict review evidence mismatch '+case['case_id'])
    if not all(case[k] for k in ['given','when','then','contract']):
        fail('Incomplete strict case '+case['case_id'])
    if case['decision_status'] not in ['specified','pending_user']:
        fail('Unknown decision status '+case['case_id'])
    contract_path, _, fragment = case['contract'].partition('#')
    target = ROOT / contract_path
    if not target.is_file() or fragment not in anchors(target):
        fail('Broken strict contract reference '+case['case_id'])
    if not set(case['requirement_ids'].split(';')) <= expected:
        fail('Unknown strict requirement '+case['case_id'])
for row in trace:
    for field in ['strict_review_contract','write_version_catalog']:
        if not (ROOT / row[field]).is_file():
            fail('Missing strict trace reference '+field)
    wanted = {case['case_id'] for case in strict_cases if row['requirement_id'] in case['requirement_ids'].split(';')}
    if set(filter(None,row['strict_review_case_ids'].split(';'))) != wanted:
        fail('Strict trace case mismatch '+row['requirement_id'])
versions = rows('02-design/write-version-catalog.csv')
if {v['operation'] for v in versions} != {o['operation'] for o in operations if o['mode']=='write'}:
    fail('Write version coverage drift')
if len({(v['operation'],v['branch']) for v in versions}) != len(versions):
    fail('Duplicate write version branch')
for v in versions:
    if v['expected_version'] not in ['required','omit'] or not all(v.values()):
        fail('Incomplete write version '+v['operation'])
    for source in v['read_source'].split(';'):
        # offsets.preview is deliberately a write returning the saved quote.
        if source != 'none' and (source not in opmap or (opmap[source]['mode']!='read' and source!='offsets.preview')):
            fail('Invalid version source '+source)
query_fields = re.search(r'filters\?:\{(.*?)\}\};', types)[1]
filter_names = set(re.findall(r'(\w+)\?:', query_fields))
for q in queries:
    if not set(filter(None,q['allowed_filters'].split(','))) - {'none'} <= filter_names:
        fail('Query filter not expressible in DTO '+q['operation'])
required_dependencies = {'SCR-T04':{'units.get'},'SCR-A09':{'contracts.list','invoices.list','units.list','units.get'},'SCR-A14':{'baselines.list','organizations.list','mrv.versions'},'SCR-T11':{'devices.calibrations','devices.operations','units.get','jobs.list'},'SCR-A04':{'devices.calibrations','devices.operations'},'SCR-A02':{'diagnosticRuns.list','diagnosticRuns.get'}}
for screen in screens:
    actual=set(filter(None,screen['operations'].split(';')))
    if not required_dependencies.get(screen['screen_id'],set()) <= actual:
        fail('Missing strict screen dependency '+screen['screen_id'])
    for field in ['primary_queries','secondary_queries']:
        if not set(screen[field].split(';')) - {'none'} <= actual:
            fail('Query absent from screen operations '+screen['screen_id'])
    if set(screen['data_contract'].split(';')) != {opmap[n]['result_contract'] for n in actual}:
        # Generic type members contain semicolons; compare serialized unique results instead.
        expected_contract=';'.join(dict.fromkeys(opmap[n]['result_contract'] for n in screen['operations'].split(';') if n))
        if screen['data_contract'] != expected_contract:
            fail('Screen DTO drift '+screen['screen_id'])
for sid,keys in {'SCR-C02':{'propertyId','spaceId'},'SCR-T11':{'deviceId','unitId','jobId'},'SCR-A14':{'reportId','reportVersion'}}.items():
    screen=next(x for x in screens if x['screen_id']==sid)
    if not keys <= set(screen['url_selection'].split(',')):
        fail('URL selection missing '+sid)
for actor in fixture['actors']:
    if not actor.get('organizationId') or 'scopes' not in actor or 'qualifications' not in actor:
        fail('Incomplete scoped fixture '+actor['membershipId'])
    for scope in actor.get('scopes',[]):
        if scope['kind'] not in ['tenant','organization','property','unit'] or not scope['id']:
            fail('Invalid fixture scope '+actor['membershipId'])
    for grant in actor.get('qualifications',[]):
        if grant['code'] not in ['demo_indoor','demo_outdoor','demo_electrical'] or grant['validFrom'] >= grant['validUntil']:
            fail('Invalid qualification grant '+actor['membershipId'])
for pattern in [r'JobOfferSummary = .*jobVersion:number',r'UnitDetail = .*effectiveControlPolicy:EffectiveControlPolicy',r'WorkReportDraft = .*InspectionItemInput\[\].*InspectionMeasurementInput\[\]',r'EnergySummary = .*baselineSnapshot:EnergyBaseline\|null;factorSnapshot:EmissionFactor\|null',r'DeviceEvent = .*recovery:DeviceRecovery\|null']:
    if not re.search(pattern,types):
        fail('Missing strict DTO invariant '+pattern)

# User-approved DEC-13 decisions must remain represented in contracts and test plans.
for case in strict_cases:
    if case['finding_id'] in ['REV-017','REV-018','REV-019'] and case['decision_status'] != 'specified':
        fail('Approved decision reverted to pending '+case['finding_id'])
if not any(v['operation']=='offsets.simulate' and v['branch']=='event=retry' and v['expected_version']=='required' for v in versions):
    fail('Offset retry version contract missing')
for pattern in [r"event:'retry';recordId:ID;attemptId:ID",r'OffsetRecord = .*attempts:OffsetAttempt\[\];currentAttemptId:ID\|null',r'Restriction = .*contractVersion:number',r'Contract = .*activeRestrictionIds:ID\[\]']:
    if not re.search(pattern,types):
        fail('Approved decision DTO missing '+pattern)

# DOC-0.10.0 design-cycle regression invariants. No app behavior is executed.
rereview_cases = rows('04-agentic-sdlc/acceptance-rereview.csv')
expected_rereview = {f'REREV-{i:03}' for i in range(1,9)} | {f'CYCLE-{i:03}' for i in range(1,9)}
if unique(rereview_cases,'finding_id','rereview finding') != expected_rereview:
    fail('Design cycle coverage must contain 8 REREV and 8 CYCLE findings')
unique(rereview_cases,'case_id','rereview acceptance')
for case in rereview_cases:
    name,_,fragment=case['contract'].partition('#')
    if not (ROOT/name).is_file() or fragment not in anchors(ROOT/name):
        fail('Broken rereview contract '+case['case_id'])
    if case['execution_status']!='not_run' or not all(case[k] for k in ['given','when','then']):
        fail('Invalid rereview acceptance '+case['case_id'])
    if not set(case['requirement_ids'].split(';')) <= expected:
        fail('Unknown rereview requirement '+case['case_id'])
for row in trace:
    wanted={c['case_id'] for c in rereview_cases if row['requirement_id'] in c['requirement_ids'].split(';')}
    if set(filter(None,row['rereview_case_ids'].split(';'))) != wanted:
        fail('Rereview trace drift '+row['requirement_id'])
if opmap['offsets.simulate']['authorization'] != 'client:self:event=request-or-retry | admin:offset.manage':
    fail('Offset event authorization drift')
t12=next(sc for sc in screens if sc['screen_id']=='SCR-T12')
if not {'devices.get','devices.events','alerts.get','alerts.acknowledge'} <= set(t12['operations'].split(';')):
    fail('T12 alert version read dependency missing')
t12_component=next(c for c in components if c['component']=='SCR-T12 Page')
if not all('alerts.get' in t12_component[field].split(';') for field in ['event','api_dependency']):
    fail('T12 component alert dependency missing')
for entity in ['DeviceOperation','DeviceEvent','CalibrationRecord']:
    if f'export type {entity} = Entity & DeviceHistoryScope &' not in types:
        fail('Device history provenance missing '+entity)
for field in ['bindingId:ID|null','unitIdAtOccurrence:ID','customerOrgIdAtOccurrence:ID']:
    if field not in re.search(r'export type DeviceHistoryScope = (.*);',types)[1]:
        fail('Device history scope field missing '+field)
for pattern in [r'Device = .*bindingId:ID\|null',r'DeviceEvent = .*alertIds:ID\[\]',r"eventType:'device';deviceId:ID;bindingId:ID\|null",r'UnitSummary = .*effectivePowerState:EffectivePowerState',r'UnitDetail = .*controlAvailability:ControlAvailability',r'Contract = .*hasUnresolvedRecovery:boolean',r'Restriction = .*recoveryCases:RestrictionRecoveryCase\[\]',r'EnergyBaseline = .*quality:BaselineQuality']:
    if not re.search(pattern,types):
        fail('Design cycle DTO invariant missing '+pattern)
for name in ['AdminSummary','Summary']:
    if 'powerUnknown:number' not in re.search(r'export type '+name+r' = (.*);',types)[1]:
        fail('Power unknown counter missing '+name)
air = re.search(r"kind:'air_quality';(.*?)\}\);",types)[1]
for field in ['severity:', 'channels:Channel[]', 'cooldownMinutes:number', 'escalateAfterMinutes:number']:
    if field not in air:
        fail('AirPolicy notification setting missing '+field)
notification_decision = re.search(r'export type NotificationDecision = (.*);',types)[1]
for value in ["'failed'","'cooldown'","'no_recipient'"]:
    if value not in notification_decision:
        fail('Notification simulation outcome missing '+value)
if typed['baselines.save'][0] != 'BaselineInput':
    fail('Baseline write must use input type, not output quality')
if 'organizationId' not in next(q for q in queries if q['operation']=='units.list')['allowed_filters'].split(','):
    fail('MRV organization unit filter missing')
fixture_input=fixture.get('airPolicyNotificationInput',{})
if set(['severity','channels','cooldownMinutes','escalateAfterMinutes']) - set(fixture_input):
    fail('AirPolicy acceptance input fixture incomplete')

# DOC-0.11.0 regression checks concern contracts, not execution of the application.
resolution_cases = rows('04-agentic-sdlc/acceptance-resolution.csv')
if unique(resolution_cases, 'finding_id', 'resolution finding') != ({f'REV-{i:03}' for i in range(1,19)} | {f'RC-{i:03}' for i in range(1,7)}):
    fail('Resolution coverage must contain 18 findings and 6 follow-through findings')
unique(resolution_cases, 'case_id', 'resolution case')
for case in resolution_cases:
    name, _, fragment = case['contract'].partition('#')
    if not (ROOT/name).is_file() or fragment not in anchors(ROOT/name):
        fail('Broken resolution contract '+case['case_id'])
    if case['execution_status'] != 'not_run' or not all(case[k] for k in ['given','when','then']):
        fail('Invalid resolution acceptance '+case['case_id'])
    wanted_status = 'deferred_candidate' if case['finding_id']=='REV-018' else 'specified'
    if case['disposition'] != wanted_status:
        fail('Incorrect resolution disposition '+case['case_id'])
    if not set(case['requirement_ids'].split(';')) <= expected:
        fail('Unknown resolution requirement '+case['case_id'])
for row in trace:
    wanted = {c['case_id'] for c in resolution_cases if row['requirement_id'] in c['requirement_ids'].split(';')}
    if set(filter(None,row['resolution_case_ids'].split(';'))) != wanted:
        fail('Resolution trace drift '+row['requirement_id'])
    if row['resolution_contract'] != '02-design/review-resolution-contracts.md':
        fail('Resolution contract absent '+row['requirement_id'])
for op in ['jobs.accept','jobs.decline']:
    if typed[op][1] != 'JobDecisionReceipt':
        fail('Unsafe job decision response '+op)
if re.search(r'export type JobDecisionReceipt = (.*);',types)[1] != "{jobId:ID;jobVersion:number;offerId:ID;decision:'accept'|'decline'}":
    fail('Job receipt contains unexpected fields')
for op in ['restrictions.get','restrictions.list','restrictions.reconcile','restrictions.retry']:
    if 'restriction.override' not in opmap[op]['authorization'] or 'RestrictionRead' not in typed[op][1]:
        fail('Override read/recovery contract missing '+op)
if 'phase=release' not in opmap['restrictions.retry']['authorization']:
    fail('Override retry must be release only')
if 'noticeAt:' in typed['restrictions.schedule'][0]:
    fail('Notice time must not be caller controlled')
if typed['invoices.remind'] != ('{invoiceId:ID;recipientMembershipId:ID;channel:Channel;reason:string}', 'InvoiceReminderReceipt', 'write'):
    fail('Shared reminder write missing')
if not any(v['operation']=='invoices.remind' and v['read_source']=='invoices.get' and v['expected_version']=='required' for v in versions):
    fail('Reminder invoice version contract missing')
for pattern in [r'Session = .*generation:number;viewEpoch:number', r'Measurement = .*boundaryId:EnergyBoundaryId\|null;qualityReason:MeasurementIssue\|null;rawUnit:string\|null', r'RawMeasurement = .*unit:string', r"eventType:'telemetry';measurement:RawMeasurement", r'Notification = .*sourceAlertId:ID\|null;type:NotificationType', r'Restriction = .*noticeNotificationIds:ID\[\]']:
    if not re.search(pattern, types):
        fail('Resolution DTO invariant missing '+pattern)
for name in ['EnergyBaseline','EnergySummary','MRVConditions','BaselineInput']:
    if 'boundaryId:EnergyBoundaryId' not in re.search(r'export type '+name+r' = (.*);',types)[1]:
        fail('Energy boundary source missing '+name)
if 'sourceAlertId?:ID' not in typed['notifications.preview'][0]:
    fail('Alert notification classification source missing')
for sid, dependencies in {'SCR-A05':{'units.list','units.get'}, 'SCR-A08':{'invoices.remind'}, 'SCR-A10':{'restrictions.list','restrictions.get'}}.items():
    sc=next(x for x in screens if x['screen_id']==sid)
    co=next(x for x in components if x['component']==sid+' Page')
    if not dependencies <= set(sc['operations'].split(';')):
        fail('Resolution screen dependency missing '+sid)
    if not dependencies <= set(co['api_dependency'].split(';')):
        fail('Resolution component dependency missing '+sid)
a09=next(x for x in screens if x['screen_id']=='SCR-A09')
if a09['primary_queries'] != 'restrictions.list' or 'restriction.override' not in a09['entry']:
    fail('Override-only list entry inaccessible')
voice=next((x for x in components if x['component']=='VoiceContainer'), None)
if voice is None or not {'jobs.list','jobs.get','units.get','commands.create'} <= set(voice['api_dependency'].split(';')):
    fail('Voice command context dependencies missing')
if not next(x for x in components if x['component']=='VoicePanel')['api_dependency'].startswith('none'):
    fail('VoicePanel must be presentation only (IR90)')
if not any(a['permissions']==['restriction.override'] for a in fixture['actors']):
    fail('Override-only fixture missing')
for path in markdown:
    text=path.read_text()
    if re.search(r'(?:Implementation baseline for|Additional current|Additional contracts for current version) 0\.(?:8|9|1[0-9])\.0', text) or re.search(r'Read IR01[–~-](?:34|44|74|93)\b', text) or re.search(r'^version: 0\.(?:8|9|1[0-9])\.0$',text,re.M) or re.search(r'\*\*0\.(?:8|9|1[0-9])\.0の実装基準',text) or re.search(r'現行0\.(?:1[0-9])\.0の追加契約',text) or 'IR01〜34を併読' in text or 'IR01〜44を併読' in text or 'IR01〜74を併読' in text or 'IR01〜93を併読' in text:
        fail('Stale current version '+str(path.relative_to(ROOT)))

# Re-review of 0.11.0: verify the changed contract paths and trace coverage.
convergence = rows('04-agentic-sdlc/acceptance-convergence.csv')
if unique(convergence, 'finding_id', 'convergence finding') != {f'CV-{i:03}' for i in range(1,9)}:
    fail('Convergence coverage must contain CV-001 through CV-008')
unique(convergence, 'case_id', 'convergence case')
for case in convergence:
    path, _, fragment = case['contract'].partition('#')
    if not (ROOT/path).is_file() or fragment not in anchors(ROOT/path):
        fail('Broken convergence contract '+case['case_id'])
    if case['execution_status'] != 'not_run' or not all(case[k] for k in ['given','when','then']):
        fail('Invalid convergence acceptance '+case['case_id'])
    if not set(case['requirement_ids'].split(';')) <= expected:
        fail('Unknown convergence requirement '+case['case_id'])
for row in trace:
    wanted={c['case_id'] for c in convergence if row['requirement_id'] in c['requirement_ids'].split(';')}
    if set(filter(None,row['convergence_case_ids'].split(';'))) != wanted:
        fail('Convergence trace drift '+row['requirement_id'])
release_projection=re.search(r'export type RestrictionReleaseView = (.*);',types)[1]
for field in ['createdAt','updatedAt']:
    if "'"+field+"'" not in release_projection:
        fail('Release list sort field absent '+field)
for name in ['notifications.preview','notifications.recipients']:
    if 'payment_reminder:admin:billing.manage:overdue-unpaid-only' not in opmap[name]['authorization']:
        fail('Reminder preview authorization absent '+name)
resolution=(ROOT/'02-design/review-resolution-contracts.md').read_text()
if 'reports.saveDraft' in resolution or 'jobs.saveDraft InspectionMeasurementInput' not in resolution:
    fail('Inspection normalization operation invalid')
for required in ['Same-key retries of acceptance/decline and writes.getResult', 'can read the Contract and all target Units of the Restriction', 'observation', 'Duplicate unitId/metric pairs within one input']:
    if required not in resolution:
        fail('Convergence behavioral guard missing '+required)

projection_cases=rows('04-agentic-sdlc/acceptance-projection.csv')
if unique(projection_cases,'finding_id','projection finding') != {f'PV-{i:03}' for i in range(1,6)}:
    fail('Projection coverage must include five findings')
unique(projection_cases,'case_id','projection case')
for case in projection_cases:
    path,_,fragment=case['contract'].partition('#')
    if not (ROOT/path).is_file() or fragment not in anchors(ROOT/path):
        fail('Broken projection contract '+case['case_id'])
    if case['execution_status']!='not_run' or not all(case[k] for k in ['given','when','then']):
        fail('Invalid projection acceptance '+case['case_id'])
    if not set(case['requirement_ids'].split(';')) <= expected:
        fail('Unknown projection requirement '+case['case_id'])
for row in trace:
    wanted={c['case_id'] for c in projection_cases if row['requirement_id'] in c['requirement_ids'].split(';')}
    if set(filter(None,row['projection_case_ids'].split(';'))) != wanted:
        fail('Projection trace drift '+row['requirement_id'])
if typed['jobs.list'][1]!='Page<JobSummary|JobOfferSummary|JobHistorySnapshot>':
    fail('Expired job list projection absent')
if "status:'offered'|'accepted';severity:null" not in re.search(r'export type JobOfferSummary = (.*);',types)[1]:
    fail('Offer public search fields absent')
if 'asOf:Instant;severity:null;dueAt:null' not in re.search(r'export type JobHistorySnapshot = (.*);',types)[1]:
    fail('Frozen job history metadata absent')
if 'contractorOrgId:ID|null' not in re.search(r'export type JobHistorySnapshot = (.*);',types)[1]:
    fail('Internal technician history must allow null contractor')
jq=next(q for q in queries if q['operation']=='jobs.list')
if 'projection first' not in jq['filter_mapping'] or 'no hidden Job fields' not in jq['sort_mapping']:
    fail('Job query projection guard absent')
if 'invalidate the whole snapshot and return CONFLICT' not in resolution:
    fail('Snapshot projection downgrade guard absent')

# 0.14.0: approved publication boundary and loop findings.
loop_cases = rows('04-agentic-sdlc/acceptance-loop.csv')
if unique(loop_cases, 'finding_id', 'loop finding') != {f'REV-{i:03}' for i in range(1,6)}:
    fail('Loop coverage must contain five behavioral findings')
unique(loop_cases, 'case_id', 'loop case')
for case in loop_cases:
    path, _, fragment = case['contract'].partition('#')
    if not (ROOT/path).is_file() or fragment not in anchors(ROOT/path):
        fail('Broken loop contract '+case['case_id'])
    if case['execution_status'] != 'not_run' or not all(case[k] for k in ['given','when','then']):
        fail('Invalid loop acceptance '+case['case_id'])
    if not set(case['requirement_ids'].split(';')) <= expected:
        fail('Unknown loop requirement '+case['case_id'])
for row in trace:
    wanted = {c['case_id'] for c in loop_cases if row['requirement_id'] in c['requirement_ids'].split(';')}
    if set(filter(None,row['loop_case_ids'].split(';'))) != wanted:
        fail('Loop trace drift '+row['requirement_id'])
if 'redactedReportSummary:ReportHistorySummary' not in types:
    fail('Report history must not expose free text')
if "{hasReport:false;acceptance:'not_accepted'}" not in types:
    fail('Absent report cannot be accepted')
offer = re.search(r'export type JobOfferSummary = (.*);',types)[1]
if 'siteAddress:string|null' not in offer or 'regionLabel:' in offer:
    fail('Offer must use nullable installation address')
if "disabledReason:'capability_changed'|'unit_archived'|'consent_revoked'|null" not in types:
    fail('Rule disable reason absent')
if typed['capabilities.save'][0] != 'Save<Capability> & {changeReason?:string}':
    fail('Capability creation reason must be optional')
sq = next(q for q in queries if q['operation']=='summaries.get')
if not {'status','statuses','severity','overdueOnly'} <= set(sq['allowed_filters'].split(',')):
    fail('Job summary filter parity absent')
if f"{len(operations)} local service operations" not in (ROOT/'02-design/common.md').read_text():
    fail('Common operation count drift')

# Independent G1 round fixes (document assertions, not app behavior tests).
g1_cases = rows('04-agentic-sdlc/acceptance-independent-g1.csv')
if unique(g1_cases, 'finding_id', 'independent G1 finding') != {f'IRV-{i:03}' for i in range(1,7)}:
    fail('Independent G1 coverage must contain six findings')
unique(g1_cases, 'case_id', 'independent G1 case')
for case in g1_cases:
    path, _, fragment = case['contract'].partition('#')
    if not (ROOT/path).is_file() or fragment not in anchors(ROOT/path):
        fail('Broken independent G1 contract '+case['case_id'])
    if case['execution_status'] != 'not_run' or not all(case[k] for k in ['given','when','then']):
        fail('Invalid independent G1 acceptance '+case['case_id'])
    if not set(case['requirement_ids'].split(';')) <= expected:
        fail('Unknown independent G1 requirement '+case['case_id'])
for row in trace:
    wanted = {c['case_id'] for c in g1_cases if row['requirement_id'] in c['requirement_ids'].split(';')}
    if set(filter(None,row['independent_g1_case_ids'].split(';'))) != wanted:
        fail('Independent G1 trace drift '+row['requirement_id'])
for name in ['README.md','04-agentic-sdlc/README.md']:
    links = re.findall(r'\]\(([^)]+spec-manifest\.json)\)', (ROOT/name).read_text())
    if len(links) != 1 or (ROOT/name).parent.joinpath(links[0]).resolve() != (RUN/'spec-manifest.json').resolve():
        fail('Current handoff baseline mismatch '+name)
job = re.search(r'export type MaintenanceJob = (.*);', types)[1]
if 'completedAt:Instant|null' not in job:
    fail('Job completion timestamp source absent')
if 'reviewAvailability:ReviewAvailability' not in re.search(r'export type WorkReport = (.*);', types)[1]:
    fail('Report review availability absent')
if 'ReportRevisionProvenance = {reportId:ID;reportVersion:number;contributorUserIds:ID[]}' not in types:
    fail('Report revision contributors absent')
if 'If it includes current Session.userId' not in resolution or 'both accept and return yield FORBIDDEN' not in resolution:
    fail('Contributing reviewer denial absent')
if 'status=open/acknowledged' not in resolution or 'critical>warning>normal' not in resolution:
    fail('Job severity source and ranking absent')
p01 = next(line for line in (ROOT/'01-requirements/contractor.md').read_text().splitlines() if line.startswith('| AT-P01-N |'))
if '① Three rows, offerCount=1/activeCount=1/reviewCount=1' not in p01 or '② One row, offerCount=1/activeCount=0/reviewCount=0' not in p01:
    fail('Filtered partner KPI acceptance mismatch')

# Cross-contract checks introduced by the 0.16.0 review.
query_by_operation = {r['operation']: r for r in queries}
job_filters = set(query_by_operation['jobs.list']['allowed_filters'].split(','))
summary_filters = set(query_by_operation['summaries.get']['allowed_filters'].split(','))
shared_job_filters = {'unitId', 'unitIds', 'customerId', 'propertyId', 'from', 'to', 'status', 'statuses', 'severity', 'overdueOnly'}
if not shared_job_filters <= job_filters & summary_filters:
    fail('Job list/summary shared filters differ')
audit_screen = next(r for r in screens if r['screen_id'] == 'SCR-A16')
if not {'devices.list', 'devices.events'} <= set(audit_screen['operations'].split(';')) or 'deviceId' not in audit_screen['url_selection'].split(','):
    fail('Audit device selection dependency absent')
if 'audit.list' not in audit_screen['primary_queries'].split(';'):
    fail('Audit primary read absent')
if any(wording in (ROOT/'02-design/implementation-contracts.md').read_text() for wording in ['totalが不明なとき', 'when total is unknown']):
    fail('Page total contradicts exact snapshot count')
decisions_016 = json.loads((ROOT/'00-prepare/internal/review-decisions-016.json').read_text())['decisions']
review_016_cases = rows('04-agentic-sdlc/acceptance-review-016.csv')
review_016_ids = unique(review_016_cases, 'case_id', '0.16 review case ID')
if len(review_016_cases) != 7:
    fail('Expected seven 0.16 review acceptance cases')
pending_016_issues = {d['issue_id'] for d in decisions_016 if d['status']=='pending_user'}
if pending_016_issues != {c['issue_id'] for c in review_016_cases if c['decision_status']=='pending_user'}:
    fail('Review decisions and acceptance readiness differ')
for item in trace:
    if any(case not in review_016_ids for case in item.get('review_016_case_ids','').split(';') if case):
        fail('Unknown 0.16 case in traceability: '+item['requirement_id'])
for decision in decisions_016:
    if decision.get('decision_status') != decision['status']:
        fail('Review decision schema status mismatch: '+decision['id'])
    if decision['status'] not in {'pending_user', 'accepted'}:
        fail('Unknown review decision status: '+decision['id'])
    if decision['status'] == 'accepted' and not decision.get('selected'):
        fail('Accepted review decision has no selection: '+decision['id'])
    if decision['status']=='accepted' and not all(decision.get(k) for k in ['decided_by','decided_at','answer']):
        fail('Accepted review decision lacks provenance: '+decision['id'])

accepted_sort = next(d for d in decisions_016 if d['id']=='DEC-18')
if accepted_sort['status']=='accepted':
    source = (ROOT/'02-design/service-contracts.ts').read_text()
    enum_match = re.search(r'export type JobStatus = ([^;]+);', source)
    order_match = re.search(r'export const JOB_STATUS_ORDER = \[([^\]]+)\]', source)
    enum_values = re.findall(r"'([^']+)'", enum_match.group(1)) if enum_match else []
    order_values = re.findall(r"'([^']+)'", order_match.group(1)) if order_match else []
    expected_order = accepted_sort['selected']['status_order']
    if order_values != expected_order or len(order_values)!=len(set(order_values)) or set(order_values)!=set(enum_values):
        fail('Job status order differs from accepted decision or enum')
    job_query = query_by_operation['jobs.list']
    if job_query['default_sort']!='status asc;id asc':
        fail('Default job sort is not business order')
    if '<'.join(expected_order) not in job_query['sort_mapping']:
        fail('Job query status ranking differs from accepted decision')
    for screen in screens:
        if 'jobs.list' in screen['operations'].split(';') and ('sort' not in screen['url_selection'].split(',') or 'IR34 onSortChange' not in screen['interaction']):
            fail('Job screen sort interaction missing: '+screen['screen_id'])
accepted_permissions = next(d for d in decisions_016 if d['id']=='DEC-17')
if accepted_permissions['status']=='accepted':
    if accepted_permissions['selected']['permissions']!=['restriction.manage','restriction.override']:
        fail('Restriction decision must retain two permissions')
    if 'four separate permissions' in (ROOT/'01-requirements/admin.md').read_text():
        fail('Restriction requirement still demands four permissions')

# 0.17.0 independent review (FRV) regression checks: document consistency only.
review_017 = rows('04-agentic-sdlc/acceptance-review-017.csv')
review_017_ids = unique(review_017, 'case_id', '0.17 review case ID')
if unique(review_017, 'issue_id', '0.17 finding') < {f'FRV-{i:03}' for i in [1,2,3,4,5,6,7,8,9,10]}:
    fail('0.17 review must cover FRV-001 through FRV-010')
for case in review_017:
    path, _, fragment = case['contract'].partition('#')
    if not (ROOT/path).is_file() or fragment not in anchors(ROOT/path):
        fail('Broken 0.17 review contract '+case['case_id'])
    if case['execution_status'] != 'not_run' or not all(case[k] for k in ['given','when','then']):
        fail('Invalid 0.17 review acceptance '+case['case_id'])
    if not set(case['requirement_ids'].split(';')) <= expected:
        fail('Unknown 0.17 review requirement '+case['case_id'])
for row in trace:
    wanted = {c['case_id'] for c in review_017 if row['requirement_id'] in c['requirement_ids'].split(';')}
    if set(filter(None, row.get('review_017_case_ids','').split(';'))) != wanted:
        fail('0.17 review trace drift '+row['requirement_id'])
decisions_017 = json.loads((ROOT/'00-prepare/internal/review-decisions-017.json').read_text())['decisions']
for decision in decisions_017:
    if decision['status'] != decision.get('decision_status') or decision['status'] not in {'proposed','accepted','pending_user'}:
        fail('0.17 decision status invalid '+decision['id'])
    if decision['status']=='proposed' and not all(decision.get(k) for k in ['proposed_by','proposed_at','owner','contract']) or decision.get('reversible') is not True:
        fail('0.17 proposed decision lacks provenance '+decision['id'])
    if decision['status']=='accepted' and not all(decision.get(k) for k in ['decided_by','decided_at','answer']):
        fail('0.17 accepted decision lacks provenance '+decision['id'])
    if decision['contract'] not in resolution:
        fail('0.17 decision contract section absent '+decision['id'])
for pattern in [r"eventType:'transport';operation:OperationName;outcome:'UNAVAILABLE'\|'TIMEOUT'\|'RATE_LIMITED'\|'DELAY'", r"exception:\{until:Instant;reason:string\|null\}\|null", r"installedAt:Instant\|null;serviceScope"]:
    if not re.search(pattern, types):
        fail('0.17 DTO invariant missing '+pattern)
if not any(s['screen_id']=='SCR-X-not-found' and s['route']=='*' for s in screens):
    fail('Not-found route screen absent')
for name in ['AppShell','RoleNavigation','ErrorBoundary','AsyncBoundary','EmptyState','TimeSeriesChart','NotificationPreview']:
    if not any(c['component']==name for c in components):
        fail('UX-05 component contract missing '+name)
for name in ['telemetry.series','telemetry.summary']:
    if 'expectedVersion' in opmap[name]['ui_validation']:
        fail('Read operation demands expectedVersion '+name)
for required in ['If already release_requested, idempotently return', 'advanceClock jumps do not consume it', "messageKey:'errors.network_disconnected'", 'saves dueAt=requestedEnd', 'with archived=true from all lists', 'one-to-one', '[to-1440 minutes,to)', "actorId='masked'", 'copying unit/staleAfterSeconds/boundaryId from the matching Capability.sensors definition', "'en-MY'"]:
    if required not in resolution:
        fail('0.17 behavioral guard missing '+required)
for role in ['common','client','admin','technician','contractor']:
    if any(wording in (ROOT/f'01-requirements/{role}.md').read_text() for wording in ['原則不可', 'generally not allowed']):
        fail('Vague permission wording remains '+role)
if any(len(s['purpose']) < 8 or s['purpose'].endswith(('（対応DD詳細の目的・フロー）', '(purpose and flow of the related DD details)')) for s in screens):
    fail('Screen purpose is boilerplate')

# 0.18.0 strict review (REV18) regression checks: document consistency only, not app behavior.
review_018 = rows('04-agentic-sdlc/acceptance-review-018.csv')
unique(review_018, 'case_id', '0.18 review case ID')
if unique(review_018, 'issue_id', '0.18 finding') != {f'REV18-{i:03}' for i in range(1,49)}:
    fail('0.18 review must cover REV18-001 through REV18-048')
for case in review_018:
    path, _, fragment = case['contract'].partition('#')
    if not (ROOT/path).is_file() or fragment not in anchors(ROOT/path):
        fail('Broken 0.18 review contract '+case['case_id'])
    if case['execution_status'] != 'not_run' or not all(case[k] for k in ['given','when','then']):
        fail('Invalid 0.18 review acceptance '+case['case_id'])
    if case['decision_status'] not in {'proposed','specified'}:
        fail('Unknown 0.18 decision status '+case['case_id'])
    if not set(case['requirement_ids'].split(';')) <= expected:
        fail('Unknown 0.18 review requirement '+case['case_id'])
for row in trace:
    wanted = {c['case_id'] for c in review_018 if row['requirement_id'] in c['requirement_ids'].split(';')}
    if set(filter(None, row.get('review_018_case_ids','').split(';'))) != wanted:
        fail('0.18 review trace drift '+row['requirement_id'])
decisions_018 = json.loads((ROOT/'00-prepare/internal/review-decisions-018.json').read_text())['decisions']
resolution = (ROOT/'02-design/review-resolution-contracts.md').read_text()
if {d['id'] for d in decisions_018} != {f'DEC-{i}' for i in range(25,42)}:
    fail('0.18 decisions must be DEC-25 through DEC-41')
for decision in decisions_018:
    if decision['status'] != decision.get('decision_status') or decision['status'] not in {'proposed','accepted'}:
        fail('0.18 decision status invalid '+decision['id'])
    if not all(decision.get(k) for k in ['proposed_by','proposed_at','owner','contract','issue_id']) or decision.get('reversible') is not True:
        fail('0.18 proposed decision lacks provenance '+decision['id'])
    if '## '+decision['contract']+' ' not in resolution:
        fail('0.18 decision contract section absent '+decision['id'])
demo_only = {'demo.advanceClock','demo.reset','demo.trigger','demoSession.signIn','demoSession.signOut','demoSession.switchMembership','demoSession.extend','auth.previewPasswordReset','payments.simulate','offsets.simulate','automations.fire'}
if {r['operation'] for r in operations if r['frontend_execution']=='demo-only'} != demo_only:
    fail('Demo-only operation set differs from IR60')
for pattern in [r"\|\{eventType:'simulator';enabled:boolean\}", r"actorRoleAtTime:Role\|'system'", r"'units\.save': \{input:\{id\?:ID;customerOrgId:ID;propertyId:ID;spaceId:ID\|null;", r"'demoSession\.extend': \{input:Record<string,never>;result:Session;mode:'write'\}", r"export type ChangeEntityType = .*'cursor_only';", r"entityType:ChangeEntityType;", r"resources:Exclude<ChangeEntityType,'cursor_only'>\[\]", r"unassignedOnly\?:boolean"]:
    if not re.search(pattern, types):
        fail('0.18 DTO invariant missing '+pattern)
if "eventType:'payment'" in types:
    fail('Payment demo trigger must stay removed (IR59)')
if 'unassignedOnly' not in query_by_operation['units.list']['allowed_filters'].split(','):
    fail('Unassigned unit filter absent (IR62)')
for sid, keys in {'SCR-C01':{'propertyId','unitId'}, 'SCR-C02':{'powerState','connections','unassignedOnly'}, 'SCR-A01':{'customerId','propertyId'}, 'SCR-A02':{'customerId','propertyId','powerState','connections'}, 'SCR-C08':{'severity','unreadOnly'}, 'SCR-C06':{'unitIds','baselineId'}, 'SCR-A16':{'actorId','targetId','correlationId','result'}}.items():
    screen = next(x for x in screens if x['screen_id']==sid)
    if not keys <= set(screen['url_selection'].split(',')):
        fail('IR50 URL keys missing '+sid)
iot_ops = {'units.get','units.list','telemetry.series','telemetry.summary','devices.get','devices.list','devices.events','commands.create','commands.get','diagnosticRuns.create','diagnosticRuns.get','diagnosticRuns.list','restrictions.forInvoice','restrictions.get','restrictions.list','admin.summary','summaries.get','alerts.list','alerts.get'}
device_states = {'connecting','device-error','device-online','device-offline'}
for screen in screens:
    states = set(screen['states'].split(';'))
    has_iot = bool(set(filter(None, screen['operations'].split(';'))) & iot_ops)
    if has_iot and not device_states <= states:
        fail('IoT screen lacks device states '+screen['screen_id'])
    if not has_iot and states & device_states:
        fail('Non-IoT screen lists device states '+screen['screen_id'])
if not any(c['component']=='SessionExpiryDialog' for c in components):
    fail('Session expiry dialog contract absent (IR55)')
if 'extendSession' not in next(c for c in components if c['component']=='AppShell')['event']:
    fail('AppShell session extension event absent (IR55)')
# IR72/IR81: superseded wording must not remain. The resolution contract is checked too,
# except lines that explicitly quote what they replace.
STALE_LITERALS = ['① general consent only', 'jobs/units/invoices/restrictions, etc.', 'No extension through user action', 'Pass events from the demo clock to `automations.fire`', 'Non-RTO contracts show "No contract / General maintenance"', 'Return to a screen the user can use', 'Return to an allowed screen.', 'Return to a screen the user may view', 'tenantId (tenant ID), parent ID', 'fixed between immediate and 300 ms', 'Navigate to the home or list screen', '| boundary / assumptions | string/required | 1–2000 characters', '| ①② Count as unresolved', 'version=3 followed by 2 and duplicate 3', 'progress assigned→submitted→completed', 'special handling for seed delay', 'general usage consent', '① general consent only', 'factor units do not match', 'offline units are pending', 'region, year, unit, and source', 'Show SCR-X-not-found. Both use the same wording', 'clock2026-03-07']
STALE_PATTERNS = [r'No extension through user action', r'room matches the display name', r'Increase20%', '[Nn]o extension through user action', 'permission to manage model numbers', 'permission to manage environmental polic(?:y|ies)', 'room matches the display name', 'number of customer organizations', '20% increase|Increase20%|Increase rate20%', '→ request release →', 'return to the home screen', 'baseline is selected automatically under IR63', r'reportCategory=scope2_electricity', r'dueAt=2026-09-15 12:00Z', 'fan-only equipment has a ventilation button', '2026-03-07 / 2026-10-31']
QUOTE_MARKERS = ('replace', 'old forms', 'obsolete wording', 'old expression', 'old text', 'replaces', 'replaced', 'old wording', 'No outdated', '置換', '旧記述', '旧表現', '旧文', '旧表記', '旧句')
stale_targets = [p for p in markdown if p.parent.name in ['01-requirements','02-design','03-uiux']] + [ROOT/'04-agentic-sdlc/verification.md'] + sorted((ROOT/'04-agentic-sdlc').glob('acceptance-*.csv'))  # IR102: acceptance plans too
for path in stale_targets:
    for line in path.read_text().splitlines():
        if (path.name == 'review-resolution-contracts.md' or path.suffix == '.csv') and any(marker in line for marker in QUOTE_MARKERS):
            continue
        for stale in STALE_LITERALS:
            if stale in line:
                fail(f'Superseded wording remains (IR72): {path.name}: {stale}')
        for pattern in STALE_PATTERNS:
            if re.search(pattern, line):
                fail(f'Superseded wording remains (IR81): {path.name}: {pattern}')
for required in ['UTC minute boundary (seconds and milliseconds both 0)', '| set_power power=false | Allowed | Allowed |', 'viewing window is [Assignment.createdAt, scheduledEnd)', 'return Job.status from offered to requested', 'severity∈{critical, warning}', 'disabledReason=consent_revoked', 'demoSession.extend', '| in_progress / submitted | Not allowed | Not allowed (first use jobs.hold to set on_hold) | CONFLICT |', 'Exclude notifications for unreadable targets from items, total, and unread counts', 'Remove the payment branch from DemoTrigger', '| 1 | User-confirmed decisions (DEC-12/13/16/17/18/44/50) |', 'errors.device_power_lost', 'one second after creation', '"Increase {absolute value}"', 'createDemoRepository', 'Nonworking days are only Saturday/Sunday', '| session | All Queries']:
    if required not in resolution:
        fail('0.18 behavioral guard missing '+required)
seed = fixture.get('demoSeed', {})
for key in ['organizations','customers','properties','spaces','capabilities','units','devices','measurements','contracts','invoices','restrictions','alerts','notifications','jobs','offers','assignments','simulator']:
    if key not in seed:
        fail('demoSeed section missing '+key)
if seed:
    unit_ids = {u['id'] for u in seed['units']}
    if not {'unit-online-rto','unit-offline-rto','unit-non-rto','unit-limited','unit-other-customer'} <= unit_ids:
        fail('demoSeed units differ from verification fixtures')
    if any(c['customerId']=='cust-b' for c in seed['contracts']):
        fail('customer-b must have zero contracts (AT-C10-N, IR61)')
    if not any(u['id']=='unit-limited' and u['spaceId'] is None for u in seed['units']):
        fail('Unassigned seed unit absent (IR62)')
    if sum(a['status'] in ('open','acknowledged') and a['severity'] in ('critical','warning') for a in seed['alerts']) != 1:
        fail('Seed alertCount must be 1 (AT-C01-N, IR51)')
    if seed['simulator'].get('enabled') is not True:
        fail('Seed simulator default must be enabled (IR45)')
if 'iPadOS 17 Safari' not in (ROOT/'02-design/deterministic-contracts.md').read_text():
    fail('Tablet environment absent from D10')
if '**1A** is' not in (ROOT/'00-prepare/PrepareDocument.md').read_text():
    fail('PrepareDocument lacks 1A/1B definition')
if f"{len(operations)} local service operations" not in (ROOT/'02-design/common.md').read_text():
    fail('Common operation count drift (0.18)')
if 'review-resolution-contracts.md#ir72-' not in (ROOT/'README.md').read_text():
    fail('README precedence link absent (IR72)')

# 0.19.0 independent review (REV19) regression checks: document consistency only, not app behavior.
review_019 = rows('04-agentic-sdlc/acceptance-review-019.csv')
unique(review_019, 'case_id', '0.19 review case ID')
if unique(review_019, 'issue_id', '0.19 finding') != {f'REV19-{i:03}' for i in range(1,43)}:
    fail('0.19 review must cover REV19-001 through REV19-042')
for case in review_019:
    path, _, fragment = case['contract'].partition('#')
    if not (ROOT/path).is_file() or fragment not in anchors(ROOT/path):
        fail('Broken 0.19 review contract '+case['case_id'])
    if case['execution_status'] != 'not_run' or not all(case[k] for k in ['given','when','then']):
        fail('Invalid 0.19 review acceptance '+case['case_id'])
    if case['decision_status'] not in {'proposed','specified','accepted'}:
        fail('Unknown 0.19 decision status '+case['case_id'])
    if not set(case['requirement_ids'].split(';')) <= expected:
        fail('Unknown 0.19 review requirement '+case['case_id'])
for row in trace:
    wanted = {c['case_id'] for c in review_019 if row['requirement_id'] in c['requirement_ids'].split(';')}
    if set(filter(None, row.get('review_019_case_ids','').split(';'))) != wanted:
        fail('0.19 review trace drift '+row['requirement_id'])
decisions_019 = json.loads((ROOT/'00-prepare/internal/review-decisions-019.json').read_text())['decisions']
if {d['id'] for d in decisions_019} != {f'DEC-{i}' for i in range(42,54)}:
    fail('0.19 decisions must be DEC-42 through DEC-53')
for decision in decisions_019:
    if decision['status'] != decision.get('decision_status') or decision['status'] not in {'proposed','accepted'}:
        fail('0.19 decision status invalid '+decision['id'])
    if not all(decision.get(k) for k in ['proposed_by','proposed_at','owner','contract','issue_id']) or decision.get('reversible') is not True:
        fail('0.19 proposed decision lacks provenance '+decision['id'])
    if decision['status']=='accepted' and not all(decision.get(k) for k in ['decided_by','decided_at','answer']):
        fail('0.19 accepted decision lacks provenance '+decision['id'])
if {d['id'] for d in decisions_019 if d['status']=='accepted'} != {'DEC-44','DEC-50'}:
    fail('0.19 user-accepted decisions must be DEC-44 and DEC-50')
    if '## '+decision['contract']+' ' not in resolution:
        fail('0.19 decision contract section absent '+decision['id'])
for required in ['screen-catalog state `work-not-started`', 'has origin=measured, quality=valid, and value≠null', 'predictedBaselineKWh = baselineKWh ÷ (|U|×baselineMinutes) × expectedUnitMinutes', 'User-triggered extension is only through IR55 demoSession.extend', '`/login?returnTo=<encodeURIComponent(pathname+search)>`', 'Do not return to skeleton (loading)', 'granted=false, grantedAt=null, revokedAt=null, version=1', 'messageKey=errors.offer_expired', 'require 1–1000 Unicode code points after trimming, regardless of DD wording', '| gridRegion | summary.factorSnapshot.region |', 'scheduledEnd−15 minutes', 'Job.scheduledSlot to [new Assignment.scheduledStart, scheduledEnd)', '| device_operation | devices.operations, devices.calibrations, devices.get, units.get |', 'normal < warning < critical', 'Otherwise, normalize set as a new row']:
    if required not in resolution:
        fail('0.19 behavioral guard missing '+required)
for pattern in [r"energyForecast:EnergyForecast;", r"export type EnergyForecast = \{period:Range;", r"sequence\?:number;eventId:ID\};"]:
    if not re.search(pattern, types):
        fail('0.19 DTO invariant missing '+pattern)
screen_by_id = {x['screen_id']:x for x in screens}
if 'returnTo' not in screen_by_id['SCR-X-login']['url_selection'].split(','):
    fail('Login returnTo URL key absent (IR82)')
for sid in ['SCR-T02','SCR-T04','SCR-T07','SCR-T10','SCR-T11']:
    if 'work-not-started' not in screen_by_id[sid]['states'].split(';'):
        fail('Work-not-started state absent '+sid)
for sid, keys in {'SCR-A13':{'unitIds','baselineId'}, 'SCR-A11':{'policyId'}, 'SCR-A12':{'policyId'}}.items():
    if not keys <= set(screen_by_id[sid]['url_selection'].split(',')):
        fail('IR90 URL keys missing '+sid)
for name in ['KpiCard','VoiceContainer']:
    if not any(c['component']==name for c in components):
        fail('IR90 component contract missing '+name)
if 'refreshing' not in (ROOT/'02-design/deterministic-contracts.md').read_text():
    fail('Refreshing state absent from D10 (IR83)')
if seed:
    consents = {c['membershipId']:c for c in seed.get('consents', [])}
    for actor in fixture['actors']:
        if actor['role']=='client' and not (actor['membershipId'] in consents and consents[actor['membershipId']]['granted'] is False and consents[actor['membershipId']]['version']==1):
            fail('Initial consent absent (IR84) '+actor['membershipId'])
    if any('sensorMetrics' in d or 'sensors' not in d for d in seed['devices']):
        fail('Seed devices must list explicit sensors (IR91)')
    sensor_ids = {s['id'] for d in seed['devices'] for s in d['sensors']}
    if any(m['sensorId'] not in sensor_ids for m in seed['measurements']):
        fail('Seed measurement references unknown sensor (IR91)')
    tenant_units = sorted(u['id'] for u in seed['units'] if not u['archived'] and next(o for o in seed['organizations'] if o['id']==u['customerOrgId'])['tenantId']=='tenant-a')
    if not any(b['id']=='baseline-demo-tenant-a' and sorted(b['unitIds'])==tenant_units and b['method']=='demo_fixed' and b['boundaryId']=='ac_input_electricity' for b in seed['baselines']):
        fail('Seed forecast baseline absent or mismatched (IR78)')
    # IR85: recompute the SR27 power classification for the fixed AT-A01-N patch set.
    patch_case = fixture.get('acceptancePatches', {}).get('AT-A01-N')
    if not patch_case:
        fail('AT-A01-N acceptance patches absent (IR85)')
    else:
        import copy
        from datetime import datetime
        state = copy.deepcopy(seed)
        for patch in patch_case['patches']:
            rows_ = state[patch['entity']]
            row = next((r for r in rows_ if r['id']==patch['id']), None)
            if row is None:
                rows_.append({'id':patch['id'], **patch['set']})
            else:
                row.update(patch['set'])
        now = datetime.fromisoformat(patch_case['clock'].replace('Z','+00:00'))
        counts = {'on':0,'off':0,'unknown':0}
        for unit in state['units']:
            org = next(o for o in state['organizations'] if o['id']==unit['customerOrgId'])
            if org['tenantId']!='tenant-a' or unit['archived']:
                continue
            device = next((d for d in state['devices'] if d['unitId']==unit['id']), None)
            obs = unit['observedState']
            power = sorted([m for m in state['measurements'] if m['unitId']==unit['id'] and m['metric']=='power'], key=lambda m:(m['observedAt'], m['sequence']), reverse=True)
            fresh = lambda at: at is not None and 0 <= (now-datetime.fromisoformat(at.replace('Z','+00:00'))).total_seconds() <= 120
            ok = device and device['connection']=='online' and isinstance(obs['power'], bool) and fresh(obs['observedAt']) and power and power[0]['origin']=='measured' and power[0]['quality']=='valid' and power[0]['value'] is not None and fresh(power[0]['observedAt'])
            counts['on' if ok and obs['power'] else 'off' if ok else 'unknown'] += 1
        exp = patch_case['expected']
        if (counts['on'],counts['off'],counts['unknown']) != (exp['powerOn'],exp['powerOff'],exp['powerUnknown']):
            fail('AT-A01-N patches do not produce the expected classification (IR85): '+str(counts))
# IR92: acceptance patches referenced by acceptance text must exist and apply to demoSeed sections.
import copy as _copy
from datetime import datetime as _dt, timedelta as _td
AP = fixture.get('acceptancePatches', {})
acceptance_text = ''.join((ROOT/f'01-requirements/{r}.md').read_text() for r in ['common','client','contractor','technician','admin']) + (ROOT/'04-agentic-sdlc/verification.md').read_text()
acceptance_csv_text = ''.join(p.read_text() for p in sorted((ROOT/'04-agentic-sdlc').glob('acceptance-*.csv')))  # IR97
referenced_keys = set(re.findall(r'\["((?:AT-|shared:)[^"]+)"\]', acceptance_text + acceptance_csv_text.replace('""','"'))) | set(re.findall(r'acceptancePatches (AT-[A-Za-z0-9.\-]*[A-Za-z0-9])', acceptance_csv_text))
case_keys = {k for k in AP if k.startswith('AT-')}
for key in sorted(referenced_keys - set(AP)):
    fail('Acceptance patch key missing (IR92) '+key)
for key in sorted(case_keys - referenced_keys):
    fail('Acceptance patch not referenced by acceptance text (IR92) '+key)
ir92 = resolution[resolution.index('## IR92 '):resolution.index('## IR93 ')] if '## IR92 ' in resolution and '## IR93 ' in resolution else ''
listed = set(re.findall(r'AT-(?:[A-Z]\d\d|REV\d\d|G\d{3})-[A-Z0-9]+(?:\.\d)?', ir92.split('6. ')[1].split('\n')[0])) if '6. ' in ir92 else set()
if listed != case_keys:
    fail('IR92 acceptance patch case list differs from fixture: '+str(sorted(listed ^ case_keys)))
def _instant(value):
    return _dt.fromisoformat(value.replace('Z','+00:00'))
def _expand(key, trail=()):
    if key in trail or key not in AP:
        fail('Acceptance patch include unresolved (IR92) '+key)
        return []
    items = []
    for inc in AP[key].get('include', []):
        if not inc.startswith('shared:'):
            fail('Acceptance patch include must be shared (IR92) '+inc)
        items += _expand(inc, trail+(key,))
    return items + AP[key].get('patches', [])
series_keys = {'idPrefix','unitId','sensorId','metric','unit','boundaryId','from','to','stepSeconds','value','origin','quality','sequenceStart','skip'}
METRIC_RANGE = {('temperature','°C'):(-50,100), ('humidity','%'):(0,100), ('co2','ppm'):(0,10000), ('pm25','µg/m³'):(0,1000), ('power','kW'):(0,100), ('vibration','mm/s'):(0,100), ('refrigerant_pressure','kPa'):(0,5000)}  # D07
sensor_ids_all = {s['id'] for d in seed.get('devices', []) for s in d.get('sensors', [])}
def validate_notification_scope(notifications, actors, label):
    memberships = {a['membershipId']:a for a in actors}
    for notification in notifications:
        recipient = memberships.get(notification.get('recipientMembershipId'))
        if recipient is None:
            fail('Notification scope snapshot invalid (IR106) '+label+': recipient absent')
            continue
        value = notification.get('scopeVersionAtCreation', recipient.get('scopeVersion'))
        # bool is an int subclass in Python but is not a JSON integer scope version.
        if type(value) is not int or value < 0:
            fail('Notification scope snapshot invalid (IR106) '+label+': '+notification['id'])

validate_notification_scope(seed.get('notifications', []), fixture['actors'], 'seed')
for key in sorted(k for k in AP if k.startswith('AT-') or k.startswith('shared:')):
    state = _copy.deepcopy(seed); actors = _copy.deepcopy(fixture['actors'])
    generated = []
    for patch in _expand(key):
        entity = patch.get('entity')
        rows_ = actors if entity == 'memberships' else state.get(entity)
        if not isinstance(rows_, list):
            fail(f'Acceptance patch entity invalid (IR92) {key}: {entity}')
            continue
        if 'series' in patch:
            spec = patch['series']
            if entity != 'measurements' or set(spec) != series_keys or spec['sensorId'] not in sensor_ids_all:
                fail(f'Acceptance series patch invalid (IR92) {key}')
                continue
            at = _instant(spec['from']); end = _instant(spec['to']); k = 0
            while at < end:
                skipped = any(_instant(s['from']) <= at < _instant(s['to']) for s in spec['skip'])
                if not skipped:
                    row = {'id':f"{spec['idPrefix']}-{k}", 'unitId':spec['unitId'], 'sensorId':spec['sensorId'], 'metric':spec['metric'], 'value':spec['value'], 'unit':spec['unit'], 'observedAt':at.isoformat().replace('+00:00','Z'), 'origin':spec['origin'], 'quality':spec['quality'], 'boundaryId':spec['boundaryId']}
                    rows_.append(row); generated.append(row)
                at += _td(seconds=spec['stepSeconds']); k += 1
            continue
        if not isinstance(patch.get('set'), dict) or not patch.get('id'):
            fail(f'Acceptance patch row invalid (IR92) {key}')
            continue
        id_field = 'membershipId' if entity == 'memberships' else 'id'
        row = next((r for r in rows_ if r.get(id_field) == patch['id']), None)
        if row is None:
            rows_.append({id_field:patch['id'], **patch['set']})
        else:
            unknown = set(patch['set']) - set(row)
            if entity == 'notifications':
                unknown.discard('scopeVersionAtCreation')  # optional seed field, required after IR106 normalization
            if unknown:
                fail(f'Acceptance patch sets unknown fields (IR92) {key}: {sorted(unknown)}')
            row.update(patch['set'])
    validate_notification_scope(state.get('notifications', []), actors, key)
    # IR97 1-3: fixture invariants hold after every patch set.
    for m in state['measurements']:
        low_high = METRIC_RANGE.get((m['metric'], m['unit']))
        if m.get('origin','measured')=='measured' and m.get('quality','valid')=='valid' and m['value'] is not None and (low_high is None or not low_high[0] <= m['value'] <= low_high[1]):
            fail(f"Acceptance measurement out of range (IR97) {key}: {m['id']}")
    for a in state['assignments']:
        job = next((j for j in state['jobs'] if j['id']==a['jobId']), None)
        if a['scheduledStart']!=a['validFrom'] or a['scheduledEnd']!=a['validUntil'] or (job and job.get('assignmentId')==a['id'] and job.get('scheduledSlot')!={'startAt':a['scheduledStart'],'endAt':a['scheduledEnd']}):
            fail(f"Assignment and job slot out of sync (IR97) {key}: {a['id']}")
    for patch in AP.get(key, {}).get('patches', []):
        fields = set(patch.get('set', {}))
        if (patch.get('entity')=='restrictions' and 'state' in fields) or (patch.get('entity') in ('invoices','payments') and 'status' in fields and patch['id'] in {x['id'] for x in seed.get(patch['entity'], [])}) or (patch.get('entity')=='jobs' and fields=={'status'}):
            fail(f"State-only patch must be created by operations (IR97) {key}: {patch['id']}")
    query = AP.get(key, {}).get('query'); expected_values = AP.get(key, {}).get('expected', {})
    if query and 'kWh' in expected_values:
        start, stop = _instant(query['from']), _instant(query['to'])
        slots = [m for m in state['measurements'] if m['unitId'] in query['unitIds'] and m['metric']=='power' and m['origin']=='measured' and m['quality']=='valid' and start <= _instant(m['observedAt']) < stop and _instant(m['observedAt']).second == 0]
        kwh = sum(m['value'] for m in slots) / 60
        minutes = int((stop-start).total_seconds() // 60) * len(query['unitIds'])
        if abs(kwh - expected_values['kWh']) > 1e-9:
            fail(f'Acceptance energy patch kWh mismatch (IR92) {key}: {kwh}')
        if 'coverage' in expected_values and abs(len(slots)/minutes - expected_values['coverage']) > 1e-9:
            fail(f'Acceptance energy patch coverage mismatch (IR92) {key}')
if not any(fct['id']==fixture.get('defaultEmissionFactorId') and fct['kgCO2ePerKWh']==fixture['energy']['factorKgPerKWh'] for fct in seed.get('factors', [])):
    fail('Default emission factor absent from demoSeed (IR92)')
for required in ['Interpreting acceptance Given', '| The technician has never had an Assignment (for example, unassigned job-internal-a in requested) | NOT_FOUND (priority 3) |', 'errors.assignment_ended', '1A does not reject overlapping contract periods that include the same equipment']:
    if required not in resolution:
        fail('0.19 behavioral guard missing '+required)
report_019 = {'review_019_cases':len(review_019), 'proposed_decisions_019':sum(d['status']=='proposed' for d in decisions_019)}

# 0.20.0 independent G1 (G1-001..031) regression checks: document consistency only, not app behavior.
review_020 = rows('04-agentic-sdlc/acceptance-review-020.csv')
unique(review_020, 'case_id', '0.20 review case ID')
if unique(review_020, 'issue_id', '0.20 finding') != {f'G1-{i:03}' for i in range(1,32)}:
    fail('0.20 review must cover G1-001 through G1-031')
for case in review_020:
    path, _, fragment = case['contract'].partition('#')
    if not (ROOT/path).is_file() or fragment not in anchors(ROOT/path):
        fail('Broken 0.20 review contract '+case['case_id'])
    if case['execution_status'] != 'not_run' or not all(case[k] for k in ['given','when','then']):
        fail('Invalid 0.20 review acceptance '+case['case_id'])
    if case['decision_status'] not in {'proposed','specified'}:
        fail('Unknown 0.20 decision status '+case['case_id'])
    if not set(case['requirement_ids'].split(';')) <= expected:
        fail('Unknown 0.20 review requirement '+case['case_id'])
for row in trace:
    wanted = {c['case_id'] for c in review_020 if row['requirement_id'] in c['requirement_ids'].split(';')}
    if set(filter(None, row.get('review_020_case_ids','').split(';'))) != wanted:
        fail('0.20 review trace drift '+row['requirement_id'])
decisions_020 = json.loads((ROOT/'00-prepare/internal/review-decisions-020.json').read_text())['decisions']
if {d['id'] for d in decisions_020} != {f'DEC-{i}' for i in range(54,60)}:
    fail('0.20 decisions must be DEC-54 through DEC-59')
for decision in decisions_020:
    if decision['status'] != 'proposed' or decision.get('decision_status') != 'proposed' or decision.get('reversible') is not True or not all(decision.get(k) for k in ['proposed_by','proposed_at','owner','contract','issue_id']):
        fail('0.20 proposed decision lacks provenance '+decision['id'])
    for section in decision['contract'].split(';'):
        if '## '+section+' ' not in resolution:
            fail('0.20 decision contract section absent '+decision['id'])
    if decision['id'] not in resolution:
        fail('0.20 decision not referenced by contract '+decision['id'])
for required in ['| Operation without jobId in its input type; the user has never had an Assignment for the target Unit | For internal technicians within unit scope: FORBIDDEN (errors.assignment_required, D01 priority 4)', 'If Device.connection≠online when devices.updateFirmware is requested, return D01 priority-8 OFFLINE (do not create a DeviceOperation)', 'Omit technicians who fail this condition from candidates; direct selection returns FORBIDDEN (errors.technician_out_of_scope)', 'members.eligible and members.capacity return only role=technician Memberships', '| job.assigned (initial assignment, reassignment, extension) | schedule_change | job |', 'Do not send to the Membership (actor) that performed the triggering operation', "| requested / applied | release_requested. Set releaseIntent.source='cancel'", '| released / cancelled | CONFLICT (D01 priority 6) |', 'Values with origin=measured and quality=valid must be in range; otherwise throw a fixture-defect exception', 'Acceptance tests start with simulator=false by default', '| co2 ≥ 1000 ppm, Capability.ventilation=true, and ventilationLevels includes low |', '| pm25 ≥ 35 µg/m³ | air.guidance.clean', '8 indoor, 5 outdoor, and 5 electrical', '1. The items componentKey set matches Unit.components at submission time', 'If authorized, return CONFLICT (messageKey=errors.scope_changed, D01 priority 6, zero side effects)', 'UNAUTHENTICATED (messageKey=errors.membership_inactive, D01 priority 2)', 'VALIDATION (fieldErrors.contractId, messageKey=errors.restriction_ineligible, D01 priority 7)', 'At 30 seconds or later it is failed (aggregate remains release_requested)', 'exclude the active Assignment being replaced for the same jobId']:
    if required not in resolution:
        fail('0.20 behavioral guard missing '+required)
for pattern in [r"releaseIntent:ReleaseIntent\|null\}", r"export type ReleaseIntent = \{source:'payment'\|'exception'\|'override'\|'manual'\|'cancel';", r"'inquiry'\|'job_update'\|'device_operation';params:", r"\{eventType:'allergen';observation:\{unitId:ID\} & AllergenObservation\}", r"\{eventType:'load_alert';unitId:ID;"]:
    if not re.search(pattern, types):
        fail('0.20 DTO invariant missing '+pattern)
if seed:
    allergen = {(o['unitId'], o['availability']) for o in seed.get('allergenObservations', [])}
    if allergen != {('unit-online-rto','available'), ('unit-non-rto','unsupported')}:
        fail('Seed allergen observations differ (IR98)')
    cap_tenant = {c['id']:c.get('tenantId') for c in seed['capabilities']}
    for unit in seed['units']:
        org = next(o for o in seed['organizations'] if o['id']==unit['customerOrgId'])
        if cap_tenant.get(unit['modelId']) != org['tenantId']:
            fail('Unit capability tenant mismatch (IR101) '+unit['id'])
    draft = AP.get('shared:report-draft-all-normal', {}).get('input', {})
    keys = [i['componentKey'] for i in draft.get('items', [])]
    if len(keys) != 18 or len(set(keys)) != 18 or any(i['result']!='normal' for i in draft.get('items', [])) or len(draft.get('workText','')) != 50 or draft.get('nextAction') != {'kind':'none'}:
        fail('Report draft input must cover 18 components (IR100)')
    for key in ['AT-A05-N','AT-A11-N','AT-A12-N']:
        policy_input = AP.get(key, {}).get('input', {})
        if policy_input.get('enabled') is not True or not policy_input.get('unitIds') or 'kind' not in policy_input:
            fail('Policy acceptance input incomplete (IR97) '+key)
    if 'sensor-tamper-co2' not in json.dumps(AP.get('AT-A12-N', {})):
        fail('AT-A12-N CO2 sensor patch absent (IR97)')
common_req = (ROOT/'01-requirements/common.md').read_text()
if '### Concrete common acceptance criteria' not in common_req or any(f'| AT-X0{i}-{k} |' not in common_req for i in range(1,8) for k in 'NEB'):
    fail('Common acceptance table absent (IR101)')
if 'summaries.get' not in screen_by_id['SCR-C08']['operations'].split(';') or 'jobId' not in screen_by_id['SCR-P03']['url_selection'].split(','):
    fail('IR102 screen contract missing (SCR-C08 summaries.get / SCR-P03 jobId)')
if 'switchMembership' not in next(c for c in components if c['component']=='AppShell')['event']:
    fail('AppShell role switch event absent (IR102)')
report_020 = {'review_020_cases':len(review_020), 'proposed_decisions_020':sum(d['status']=='proposed' for d in decisions_020)}

# 0.21.0 independent G1 corrections. These checks validate specification contracts,
# not an implemented event bus or policy engine.
def contract_section(number):
    match = re.search(r'^## IR'+str(number)+r' .*?(?=^## |\Z)', resolution, re.M | re.S)
    return match.group(0) if match else ''

def contract_table(section):
    return [[cell.strip().strip('`') for cell in line.strip().strip('|').split('|')]
            for line in section.splitlines() if line.startswith('|')
            and not re.match(r'^\|[\s:|-]+\|$', line)]

change_type = re.search(r'export type ChangeEntityType = ([^;]+);', types)
change_entities = set(re.findall(r"'([^']+)'", change_type.group(1))) if change_type else set()
invalidation = {cells[0]:{op.strip() for op in cells[1].split(',')}
                for cells in contract_table(contract_section(71)) if len(cells)==2}
if 'allergen_observation' not in change_entities:
    fail('Allergen change entity absent (IR105)')
if invalidation.get('allergen_observation') != {'telemetry.series'}:
    fail('Allergen query invalidation differs (IR105)')

notification_rows = {cells[0]:cells[1:] for cells in contract_table(contract_section(104))
                     if len(cells)==3 and cells[0]!='templateKey'}
expected_notification_rows = {
    'alert':['maintenance→cleaning_due; sensor/tamper/reconciliation_required→fault; quality→quality', 'sourceAlert.severity'],
    'quality':['quality','sourcePolicy.severity'],
    'payment_reminder':['payment_reminder','warning'],
    'restriction':['restriction','scheduled/requested/applied/release_requested→warning; released/cancelled→normal'],
    'device_operation':['device_operation','warning'],
    **{key:[key,'normal'] for key in ['schedule_change','report_return','completion','payment','inquiry','job_update']},
}
if notification_rows != expected_notification_rows:
    fail('Notification type or severity mapping differs (IR104)')
notification_enum = re.search(r'export type NotificationType = ([^;]+);', types)
notification_types = set(re.findall(r"'([^']+)'", notification_enum.group(1))) if notification_enum else set()
for template, (classification, _) in notification_rows.items():
    targets = {'cleaning_due','fault','quality'} if template=='alert' else {classification}
    if not targets <= notification_types:
        fail('Notification mapped type absent from DTO (IR104) '+template)
for cells in contract_table(contract_section(95)):
    if len(cells)==4 and cells[0]!='Event' and cells[1] not in notification_rows:
        fail('Business notification template unmapped (IR104) '+cells[1])
for number, required in {
    95:['templateKey=alert follows IR10 Alert classification; all other templates use the same name for templateKey and type'],
    103:["Do not use pre-save history or a late Fact's observedAt as the start time", 'A new save starts the counter at 0', 'Before it is met, notifications are suppressed/not_due'],
    104:['device_operation.failed is an asynchronous system event', 'actor=system-demo'],
    105:["ChangeEvent.entityType='allergen_observation'", 'A retry with the same DemoTrigger.eventId adds neither a row nor a change event', 'C07/A12 subscribes to this resource'],
    106:['if scopeVersionAtCreation is omitted, set it from Membership.scopeVersion found through recipientMembershipId after all patches are applied', 'Validate an explicit scopeVersionAtCreation as a nonnegative integer and keep it', 'Later Membership.scopeVersion changes do not alter existing notification values'],
}.items():
    for guard in required:
        if guard not in contract_section(number):
            fail(f'0.21 behavioral guard missing (IR{number}) '+guard)

# Follow the fixture's sequence, checking elapsed time rather than accepting a
# backdated observation as evidence of a duration already spent under the policy.
a12 = AP.get('AT-A12-N', {})
try:
    duration = a12['input']['durationSeconds']
    initial = a12['evaluation']; final = a12['finalEvaluation']
    start = _instant(a12['clock'])
    if duration != 60 or _instant(initial['occurredAt']) != start or any(_instant(f['observedAt']) != start for f in initial['facts']):
        raise ValueError('initial observation is not at policy evaluation start')
    if (set(initial['unitIds']) != set(a12['input']['unitIds'])
            or {f['unitId'] for f in initial['facts']} != set(initial['unitIds'])
            or final['facts'] != initial['facts'] or final['unitIds'] != initial['unitIds']
            or initial['eventId'] == final['eventId']
            or (_instant(final['occurredAt'])-start).total_seconds() != duration):
        raise ValueError('final evaluation does not preserve the held facts and elapsed duration')
    elapsed = 0; assertions = []; calls = []
    for step in a12['flow']:
        operation = step['operation']
        if operation=='clock.tick':
            if step.get('stepSeconds')!=1 or type(step.get('seconds')) is not int or step['seconds']<=0:
                raise ValueError('clock must advance by ordinary one-second ticks')
            elapsed += step['seconds']
        elif operation=='assert':
            if step['elapsedSeconds'] != elapsed:
                raise ValueError('boundary assertion is at the wrong elapsed time')
            assertions.append(elapsed)
        elif operation in ('policies.save','automations.fire'):
            calls.append((operation,step['inputRef'],elapsed))
        else:
            raise ValueError('unsupported duration flow operation')
    if (elapsed != duration or assertions != [0,duration-1,duration]
            or calls != [('policies.save','input',0),('automations.fire','evaluation',0),('automations.fire','finalEvaluation',duration)]):
        raise ValueError('save, evaluate and boundary clock sequence differs')
    expected_boundaries = [dict(elapsedSeconds=e, alertCount=a, notificationCount=n, commandCount=c)
                           for e,a,n,c in [(0,0,0,0),(duration-1,0,0,0),(duration,2,2,1)]]
    if a12['expected']['boundaries'] != expected_boundaries:
        raise ValueError('policy-scoped boundary counts differ')
except (KeyError, TypeError, ValueError) as error:
    fail('A12 continuity fixture invalid (IR103): '+str(error))

review_021_path = ROOT/'04-agentic-sdlc/acceptance-review-021.csv'
review_021 = rows(str(review_021_path.relative_to(ROOT))) if review_021_path.exists() else []
if unique(review_021, 'issue_id', '0.21 finding') != {f'G120-{i:03}' for i in range(1,6)}:
    fail('0.21 review must cover G120-001 through G120-005')
unique(review_021, 'case_id', '0.21 review case ID')
for case in review_021:
    path, _, fragment = case['contract'].partition('#')
    if not (ROOT/path).is_file() or fragment not in anchors(ROOT/path):
        fail('Broken 0.21 review contract '+case['case_id'])
    if case['execution_status']!='not_run' or not all(case[k] for k in ['given','when','then']):
        fail('Invalid 0.21 review acceptance '+case['case_id'])
    if case['decision_status'] not in {'proposed','specified'} or not set(case['requirement_ids'].split(';')) <= expected:
        fail('Invalid 0.21 review status or requirement '+case['case_id'])
for row in trace:
    wanted = {c['case_id'] for c in review_021 if row['requirement_id'] in c['requirement_ids'].split(';')}
    if set(filter(None,row.get('review_021_case_ids','').split(';'))) != wanted:
        fail('0.21 review trace drift '+row['requirement_id'])
decisions_021_path = ROOT/'00-prepare/internal/review-decisions-021.json'
decisions_021 = json.loads(decisions_021_path.read_text())['decisions'] if decisions_021_path.exists() else []
if {d['id'] for d in decisions_021} != {'DEC-60','DEC-61'}:
    fail('0.21 decisions must be DEC-60 and DEC-61')
for decision in decisions_021:
    if decision['status']!='proposed' or decision.get('decision_status')!='proposed' or decision.get('reversible') is not True or not all(decision.get(k) for k in ['proposed_by','proposed_at','owner','contract','issue_id']):
        fail('0.21 proposed decision lacks provenance '+decision['id'])
    if any('## '+section+' ' not in resolution for section in decision['contract'].split(';')) or decision['id'] not in resolution:
        fail('0.21 decision contract absent '+decision['id'])

typescript_check = 'not_run'
if args.tsc:
    checked = subprocess.run(['node', str(args.tsc), '--strict', '--noEmit', '--target', 'ES2022', '--lib', 'ES2022,DOM', str(ROOT / '02-design/service-contracts.ts')], capture_output=True, text=True)
    typescript_check = 'passed' if checked.returncode == 0 else 'failed'
    if checked.returncode:
        fail('TypeScript semantic check failed: '+checked.stdout+checked.stderr)

# A reproducible content baseline, including its validator and immutable source inputs.
spec_files = []
for path in sorted(ROOT.rglob('*')):
    rel = path.relative_to(ROOT)
    if not path.is_file() or any(part in ['runs','05-document-review','__pycache__'] for part in rel.parts):
        continue
    if path.suffix not in ['.md','.csv','.json','.ts','.txt','.py']:
        continue
    spec_files.append({'path':str(rel),'sha256':hashlib.sha256(path.read_bytes()).hexdigest()})
baseline = hashlib.sha256(json.dumps(spec_files,ensure_ascii=False,sort_keys=True,separators=(',',':')).encode()).hexdigest()
manifest_path = RUN / 'spec-manifest.json'
if args.write_baseline and not errors:
    RUN.mkdir(parents=True, exist_ok=True)
    manifest_path.write_text(json.dumps({'version':'0.21.0','spec_baseline_id':baseline,'hash_algorithm':'sha256','canonicalization':'UTF-8 JSON(spec_files), ensure_ascii=False, sort_keys=True, separators=(comma,colon)','spec_files':spec_files},ensure_ascii=False,indent=2)+'\n')
elif not args.write_baseline:
    if not manifest_path.exists():
        fail('Missing current baseline; run --write-baseline after correcting specifications')
    else:
        manifest = json.loads(manifest_path.read_text())
        if manifest['spec_files'] != spec_files or manifest['spec_baseline_id'] != baseline:
            fail('Baseline drift: regenerate only after reviewing the specification diff')
report = {'scope':'document-static-only','requirements':len(trace),'acceptance_bundles':len(acceptance),'role_details':role_tables,'operations':len(operations),'screens':len(screens),'components':len(components),'query_contracts':len(queries),'fix_cases':len(fixes),'independent_fix_cases':len(independent_cases),'strict_fix_cases':len(strict_cases),'rereview_fix_cases':len(rereview_cases),'pending_business_decisions':sum(c['decision_status']=='pending_user' for c in strict_cases),'write_version_branches':len(versions),'spec_files':len(spec_files),'baseline':baseline,'application_tests':'not_run','typescript_semantic_check':typescript_check,'errors':errors}
report.update(independent_g1_cases=len(g1_cases), loop_cases=len(loop_cases), projection_cases=len(projection_cases), convergence_cases=len(convergence), resolution_cases=len(resolution_cases), deferred_candidates=sum(c['disposition']=='deferred_candidate' for c in resolution_cases))
report['pending_business_decisions'] += sum(d['status']=='pending_user' for d in decisions_016)
report['review_016_cases'] = len(review_016_cases)
report['review_017_cases'] = len(review_017)
report['proposed_decisions'] = sum(d['status']=='proposed' for d in decisions_017)
report['review_018_cases'] = len(review_018)
report['proposed_decisions_018'] = sum(d['status']=='proposed' for d in decisions_018)
report.update(report_019)
report.update(report_020)
report.update(review_021_cases=len(review_021), proposed_decisions_021=len(decisions_021))
print(json.dumps(report,ensure_ascii=False,indent=2))
sys.exit(1 if errors else 0)
