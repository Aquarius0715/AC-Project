#!/usr/bin/env python3
"""Mutation checks for the document validator; never edits the source documents."""
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
TYPE = '02-design/service-contracts.ts'
OPS = '02-design/operation-catalog.csv'
# Each mutation must be rejected for its intended reason, not merely manifest drift.
CASES = [
    ('accepted_decision_provenance_missing', [('00-prepare/internal/review-decisions-016.json', '"decided_by": "本会話のユーザー"', '"decided_by": ""')], 'Accepted review decision lacks provenance'),
    ('status_order_missing_state', [(TYPE, "export const JOB_STATUS_ORDER = ['requested','offered',", "export const JOB_STATUS_ORDER = ['offered',")], 'Job status order differs from accepted decision or enum'),
    ('default_sort_reverts_to_due_date', [('02-design/query-catalog.csv', ',status asc;id asc,', ',dueAt asc;id asc,')], 'Default job sort is not business order'),
    ('four_permission_requirement_returns', [('01-requirements/admin.md', '猶予・例外・取り消しはrestriction.manage、強制解除はrestriction.overrideの2種類の権限で認可する。', '猶予・例外・取り消し・手動解除を、それぞれ独立した権限で記録する。')], 'Restriction requirement still demands four permissions'),
    ('job_list_unit_set_missing', [('02-design/query-catalog.csv', 'jobs.list,"unitId,unitIds,', 'jobs.list,"unitId,')], 'Job list/summary shared filters differ'),
    ('audit_device_candidates_missing', [('03-uiux/screen-catalog.csv', 'audit.list;devices.list;devices.events', 'audit.list;devices.events')], 'Audit device selection dependency absent'),
    ('audit_primary_read_missing', [('03-uiux/screen-catalog.csv', ',audit.list,devices.list;devices.events;commands.get,', ',none,devices.list;devices.events;commands.get,')], 'Audit primary read absent'),
    ('review_ui_availability_missing', [(TYPE, 'reviewAvailability:ReviewAvailability;', '')], 'Report review availability absent'),
    ('stale_handoff_baseline', [('04-agentic-sdlc/README.md','runs/DOC-0.20.0/spec-manifest.json','runs/DOC-0.19.0/spec-manifest.json')], 'Current handoff baseline mismatch'),
    ('job_completed_time_missing', [(TYPE, 'startedAt:Instant|null;completedAt:Instant|null;contractorOrgId:', 'startedAt:Instant|null;contractorOrgId:')], 'Job completion timestamp source absent'),
    ('report_contributors_missing', [(TYPE, 'contributorUserIds:ID[]', 'contributorUserIds:ID')], 'Report revision contributors absent'),
    ('filtered_kpi_old_expectation', [('01-requirements/contractor.md', '②一覧1件、offerCount=1／activeCount=0／reviewCount=0', '②一覧1件、offerCount=1／activeCount=1／reviewCount=1')], 'Filtered partner KPI acceptance mismatch'),
    ('contributor_deny_missing', [('02-design/review-resolution-contracts.md', 'accept/returnともFORBIDDEN', 'accept/returnとも許可')], 'Contributing reviewer denial absent'),
    ('severity_resolved_included', [('02-design/review-resolution-contracts.md', 'status=open/acknowledged', 'status=resolved')], 'Job severity source and ranking absent'),

    ('history_free_text', [(TYPE, 'redactedReportSummary:ReportHistorySummary', 'redactedReportSummary:string|null')], 'Report history must not expose free text'),
    ('absent_report_accepted', [(TYPE, "{hasReport:false;acceptance:'not_accepted'}", "{hasReport:false;acceptance:'accepted'}")], 'Absent report cannot be accepted'),
    ('address_missing_null', [(TYPE, 'siteAddress:string|null', 'siteAddress:string')], 'Offer must use nullable installation address'),
    ('disabled_reason_missing', [(TYPE, "disabledReason:'capability_changed'|'unit_archived'|'consent_revoked'|null", 'disabledReason:string|null')], 'Rule disable reason absent'),
    ('stale_operation_count', [('02-design/common.md','137個のローカルサービス操作','136個のローカルサービス操作')], 'Common operation count drift'),

    ('unsafe_job_receipt', [(TYPE, 'export type JobDecisionReceipt = {jobId:ID;',
                            'export type JobDecisionReceipt = {unitId:ID;jobId:ID;')],
     'Job receipt contains unexpected fields'),
    ('caller_notice_time', [(TYPE, "'restrictions.schedule': {input:{contractId:ID;", "'restrictions.schedule': {input:{noticeAt:Instant;contractId:ID;")],
     'Notice time must not be caller controlled'),
    ('override_apply_permission', [(OPS, 'restriction.override:phase=release:', 'restriction.override:phase=apply:')],
     'Override retry must be release only'),
    ('missing_view_epoch', [(TYPE, 'generation:number;viewEpoch:number;', 'generation:number;')],
     'Resolution DTO invariant missing Session'),
    ('unknown_raw_unit_unrepresentable', [(TYPE, 'value:number|null;unit:string;observedAt:Instant;', 'value:number|null;unit:UnitSymbol;observedAt:Instant;')],
     'Resolution DTO invariant missing RawMeasurement'),
    ('missing_notification_type', [(TYPE, 'sourceAlertId:ID|null;type:NotificationType;', 'sourceAlertId:ID|null;')],
     'Resolution DTO invariant missing Notification'),
    ('missing_actual_boundary', [(TYPE, 'export type EnergySummary = {period:', 'export type EnergySummary = {period:'),
                                 (TYPE, 'tariffVersion:ID;boundaryId:EnergyBoundaryId;boundary:string;', 'tariffVersion:ID;boundary:string;')],
     'Energy boundary source missing EnergySummary'),
    ('override_entry_blocked', [('03-uiux/screen-catalog.csv', 'restriction.manage OR restriction.override', 'restriction.manage')],
     'Override-only list entry inaccessible'),
    ('release_sort_field_missing', [(TYPE, "'createdAt'|'updatedAt'|'unitIds'", "'createdAt'|'unitIds'")],
     'Release list sort field absent updatedAt'),
    ('reminder_read_gate_missing', [(OPS, 'payment_reminder:admin:billing.manage:overdue-unpaid-only', 'payment_reminder:authenticated')],
     'Reminder preview authorization absent'),
    ('inspection_operation_typo', [('02-design/review-resolution-contracts.md', 'jobs.saveDraftのInspectionMeasurementInput', 'reports.saveDraftのInspectionMeasurementInput')],
     'Inspection normalization operation invalid'),
    ('notice_recipient_scope_missing', [('02-design/review-resolution-contracts.md', '全対象Unitを閲覧できる宛先', '当該顧客の宛先')],
     'Convergence behavioral guard missing 全対象Unitを閲覧できる宛先'),
    ('job_history_list_missing', [(TYPE, 'Page<JobSummary|JobOfferSummary|JobHistorySnapshot>', 'Page<JobSummary|JobOfferSummary>')],
     'Expired job list projection absent'),
    ('offer_severity_disclosure', [(TYPE, "status:'offered'|'accepted';severity:null;", "status:'offered'|'accepted';severity:Severity;")],
     'Offer public search fields absent'),
    ('internal_history_null_missing', [(TYPE, 'status:JobStatus;contractorOrgId:ID|null;completedAt:', 'status:JobStatus;contractorOrgId:ID;completedAt:')],
     'Internal technician history must allow null contractor'),
    # 0.18.0 (REV18) guards
    ('payment_trigger_reintroduced', [(TYPE, "|{eventType:'simulator';enabled:boolean}", "|{eventType:'simulator';enabled:boolean}|{eventType:'payment';paymentId:ID}")],
     'Payment demo trigger must stay removed'),
    ('simulator_trigger_missing', [(TYPE, "|{eventType:'simulator';enabled:boolean}", "")],
     '0.18 DTO invariant missing'),
    ('demo_only_flag_removed', [(OPS, 'DD-C11,demo-only,', 'DD-C11,mock-service,')],
     'Demo-only operation set differs from IR60'),
    ('unassigned_filter_missing', [('02-design/query-catalog.csv', ',unassignedOnly"', '"')],
     'Unassigned unit filter absent'),
    ('session_dialog_missing', [('03-uiux/component-contracts.csv', 'SessionExpiryDialog,', 'SessionWarning,')],
     'Session expiry dialog contract absent'),
    ('superseded_wording_returns', [('02-design/client.md', '画面はこの発火のためにautomations.fireを呼びません(IR54)。', '画面はこの発火のためにautomations.fireを呼びません(IR54)。jobs/units/invoices/restrictionsなど')],
     'Superseded wording remains'),
    ('restriction_table_weakened', [('02-design/review-resolution-contracts.md', '| set_power power=false | 許可 | 許可 |', '| set_power power=false | 許可 | FORBIDDEN |')],
     '0.18 behavioral guard missing'),
    # 0.19.0 (REV19) guards
    ('extension_denial_returns', [('02-design/review-resolution-contracts.md', '利用者操作による延長はIR55のdemoSession.extendだけで行う（IR79）。', '利用者操作による延長はない。')],
     'Superseded wording remains (IR81)'),
    ('model_permission_variant_returns', [('02-design/admin.md', 'device.manage(設備管理)の権限があること(IR74)。', 'device.manage(設備管理)の権限、または型番を管理できる権限があること。')],
     'Superseded wording remains (IR81)'),
    ('environment_policy_variant_returns', [('02-design/admin.md', 'automation.policy.manageの権限があること(IR74)。', '環境に関するpolicy(方針)を管理する権限があること。')],
     'Superseded wording remains (IR81)'),
    ('room_display_name_returns', [('02-design/deterministic-contracts.md', 'roomの照合対象と候補の作り方はIR65（Space.nameの完全一致）', 'roomは表示名の完全一致')],
     'Superseded wording remains (IR81)'),
    ('customer_organization_count_returns', [('02-design/admin.md', 'IR40の定義（CustomerとOrganizationがともにactiveなCustomerの件数）です。', '稼働中(active)の顧客組織の数です。')],
     'Superseded wording remains (IR81)'),
    ('negative_saving_old_label_returns', [('01-requirements/client.md', 'savingPercentage=-20で表示「増加 20.0%」(IR80)', '増加20%')],
     'Superseded wording remains (IR81)'),
    ('s03_explicit_release_required_returns', [('04-agentic-sdlc/verification.md', '→明示のrestrictions.releaseが冪等に同じ状態を返すことを確認する→', '→解除を要求する→')],
     'Superseded wording remains (IR81)'),
    ('simulator_copies_estimates', [('02-design/review-resolution-contracts.md', 'がorigin=measured、quality=valid、value≠nullの場合だけ', 'がvalue≠nullの場合だけ')],
     '0.19 behavioral guard missing'),
    ('forecast_type_missing', [(TYPE, 'energyForecast:EnergyForecast;', '')],
     '0.19 DTO invariant missing'),
    ('login_return_to_missing', [('03-uiux/screen-catalog.csv', 'demoSession.signIn,"tab,returnTo",', 'demoSession.signIn,tab,')],
     'Login returnTo URL key absent'),
    ('work_not_started_state_missing', [('03-uiux/screen-catalog.csv', ';work-not-started', '')],
     'Work-not-started state absent'),
    ('initial_consent_granted', [('04-agentic-sdlc/fixture-contract.json', '"granted": false,', '"granted": true,')],
     'Initial consent absent'),
    ('kpi_patch_expectation_wrong', [('04-agentic-sdlc/fixture-contract.json', '"powerOff": 2,', '"powerOff": 3,')],
     'AT-A01-N patches do not produce'),
    ('seed_sensor_metrics_returns', [('04-agentic-sdlc/fixture-contract.json', '"createdByMembershipId": "system-demo",', '"sensorMetrics": [],\n      "createdByMembershipId": "system-demo",')],
     'Seed devices must list explicit sensors'),
    ('public_screen_states_regress', [('03-uiux/screen-catalog.csv', 'navigation,none,none,success,', 'navigation,none,none,initial;loading;success;empty;error;offline;permission-denied;not-found;stale,')],
     'Public screen states differ'),
    ('voice_panel_fetches', [('03-uiux/component-contracts.csv', 'none; VoiceContainerがvoice.resolveIntent', 'voice.resolveIntent; VoiceContainerがvoice.resolveIntent')],
     'VoicePanel must be presentation only'),
    ('calibration_invalidation_missing', [('02-design/review-resolution-contracts.md', '| device_operation | devices.operations, devices.calibrations, devices.get, units.get |', '| device_operation | devices.operations, devices.get, units.get |')],
     '0.19 behavioral guard missing'),
    ('acceptance_patch_key_missing', [('04-agentic-sdlc/fixture-contract.json', '"AT-P06-B": {', '"AT-P06-X": {')],
     'Acceptance patch key missing'),
    ('default_factor_value_changed', [('04-agentic-sdlc/fixture-contract.json', '"kgCO2ePerKWh": 0.5,', '"kgCO2ePerKWh": 0.4,')],
     'Default emission factor absent'),
    ('energy_series_value_changed', [('04-agentic-sdlc/fixture-contract.json', '"value": 80,', '"value": 70,')],
     'Acceptance energy patch kWh mismatch'),
    ('technician_start_codes_weakened', [('02-design/review-resolution-contracts.md', '| NOT_FOUND（順位3） |', '| CONFLICT（順位6） |')],
     '0.19 behavioral guard missing'),
    # 0.20.0 (independent G1-001..031) guards
    ('internal_technician_write_without_assignment', [('02-design/review-resolution-contracts.md', '社内でunit scope内ならFORBIDDEN（errors.assignment_required、D01順位4）', '社内でunit scope内なら許可')],
     '0.20 behavioral guard missing'),
    ('firmware_offline_creates_operation', [('02-design/review-resolution-contracts.md', '要求時にDevice.connection≠onlineならD01順位8のOFFLINE（DeviceOperationを作らない）', '要求時の接続にかかわらずqueuedで作成する')],
     '0.20 behavioral guard missing'),
    ('assigned_notification_template_changed', [('02-design/review-resolution-contracts.md', '| job.assigned（初回・再割当・延長） | schedule_change | job |', '| job.assigned（初回・再割当・延長） | job_update | job |')],
     '0.20 behavioral guard missing'),
    ('cancel_applied_becomes_cancelled', [('02-design/review-resolution-contracts.md', "| requested / applied | release_requested。releaseIntent.source='cancel'", '| requested / applied | cancelled。')],
     '0.20 behavioral guard missing'),
    ('release_intent_type_missing', [(TYPE, ';releaseIntent:ReleaseIntent|null}', '}')],
     '0.20 DTO invariant missing'),
    ('air_guidance_threshold_changed', [('02-design/review-resolution-contracts.md', '| co2 ≥ 1000 ppmで、Capability.ventilation=true', '| co2 ≥ 1200 ppmで、Capability.ventilation=true')],
     '0.20 behavioral guard missing'),
    ('stale_scope_rule_weakened', [('02-design/review-resolution-contracts.md', '認可を通る場合はCONFLICT（messageKey=errors.scope_changed、D01順位6、副作用0）', '認可を通る場合は旧Contextのまま実行する')],
     '0.20 behavioral guard missing'),
    ('fixture_measurement_out_of_range', [('04-agentic-sdlc/fixture-contract.json', '"value": 60,', '"value": 160,')],
     'Acceptance measurement out of range (IR97)'),
    ('fixture_assignment_job_desync', [('04-agentic-sdlc/fixture-contract.json', '"validUntil": "2026-09-20T00:00:00.000Z"', '"validUntil": "2026-09-21T00:00:00.000Z"')],
     'Assignment and job slot out of sync (IR97)'),
    ('report_input_component_missing', [('04-agentic-sdlc/fixture-contract.json', '"componentKey": "wiring",', '"componentKey": "filter",')],
     'Report draft input must cover 18 components (IR100)'),
    ('policy_input_disabled', [('04-agentic-sdlc/fixture-contract.json', '"enabled": true,\n        "priority": 50,\n        "kind": "alert",', '"enabled": false,\n        "priority": 50,\n        "kind": "alert",')],
     'Policy acceptance input incomplete (IR97)'),
    ('capability_tenant_mismatch', [('04-agentic-sdlc/fixture-contract.json', '"id": "cap-split-std-tb",\n        "tenantId": "tenant-b",', '"id": "cap-split-std-tb",\n        "tenantId": "tenant-a",')],
     'Unit capability tenant mismatch (IR101)'),
    ('allergen_seed_changed', [('04-agentic-sdlc/fixture-contract.json', '"availability": "unsupported",', '"availability": "not_measured",')],
     'Seed allergen observations differ (IR98)'),
    ('a12_co2_sensor_patch_missing', [('04-agentic-sdlc/fixture-contract.json', '"id": "sensor-tamper-co2",', '"id": "sensor-tamper-pm25",')],
     'AT-A12-N CO2 sensor patch absent (IR97)'),
    ('common_acceptance_table_missing', [('01-requirements/common.md', '| AT-X04-B |', '| AT-X04-Z |')],
     'Common acceptance table absent (IR101)'),
    ('g1_case_coverage_missing', [('04-agentic-sdlc/acceptance-review-020.csv', 'AT-G120-031,G1-031,', 'AT-G120-031,G1-030,')],
     '0.20 review must cover G1-001 through G1-031'),
    ('decision_059_missing', [('00-prepare/internal/review-decisions-020.json', '"id": "DEC-59",', '"id": "DEC-60",')],
     '0.20 decisions must be DEC-54 through DEC-59'),
    ('general_consent_wording_returns', [('01-requirements/client.md', '①位置同意granted=false', '①一般同意のみ')],
     'Superseded wording remains'),
    ('csv_due_at_old_expectation_returns', [('04-agentic-sdlc/acceptance-review-017.csv', '省略時dueAt=2026-09-15T04:00:00.000Z', '省略時dueAt=2026-09-15 12:00Z')],
     'Superseded wording remains'),
    ('csv_patch_key_missing', [('04-agentic-sdlc/acceptance-review-017.csv', 'acceptancePatches AT-REV17-005', 'acceptancePatches AT-REV17-099')],
     'Acceptance patch key missing'),
    ('c08_summary_dependency_missing', [('03-uiux/screen-catalog.csv', 'alerts.list;notifications.markRead;notifications.list;summaries.get,"tab,severity,unreadOnly"', 'alerts.list;notifications.markRead;notifications.list,"tab,severity,unreadOnly"')],
     'IR102 screen contract missing'),
    ('app_shell_switch_event_missing', [('03-uiux/component-contracts.csv', ';switchMembership(demoMembershipId),', ',')],
     'AppShell role switch event absent'),
]

def validate(root):
    result = subprocess.run([sys.executable, str(root/'tools/validate_documents.py'), '--write-baseline'],
                            capture_output=True, text=True)
    return result.returncode, json.loads(result.stdout)

results = []
with tempfile.TemporaryDirectory(prefix='ac-document-regression-') as directory:
    copied = Path(directory)/'docs'
    shutil.copytree(ROOT, copied, ignore=shutil.ignore_patterns('05-document-review', '__pycache__'))
    code, report = validate(copied)
    if code or report['errors']:
        raise SystemExit('Unmodified copied specification must pass first: '+str(report['errors']))
    for name, mutations, expected in CASES:
        originals = {}
        try:
            for relative, old, new in mutations:
                path = copied/relative
                originals.setdefault(path, path.read_bytes())
                content = path.read_text()
                if old not in content:
                    raise AssertionError('Mutation target missing: '+name)
                path.write_text(content.replace(old, new))
            code, report = validate(copied)
            passed = code != 0 and any(expected in error for error in report['errors'])
            results.append({'case':name, 'detected':passed, 'expected_diagnostic':expected})
        finally:
            for path, data in originals.items():
                path.write_bytes(data)
print(json.dumps({'scope':'document-validator-mutations-only', 'application_tests':'not_run',
                  'results':results, 'passed':all(r['detected'] for r in results)}, indent=2))
sys.exit(0 if all(r['detected'] for r in results) else 1)
