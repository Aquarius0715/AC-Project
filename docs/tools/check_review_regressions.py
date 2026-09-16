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
    ('stale_handoff_baseline', [('04-agentic-sdlc/README.md','runs/DOC-0.16.0/spec-manifest.json','runs/DOC-0.13.0/spec-manifest.json')], 'Current handoff baseline mismatch'),
    ('job_completed_time_missing', [(TYPE, 'startedAt:Instant|null;completedAt:Instant|null;contractorOrgId:', 'startedAt:Instant|null;contractorOrgId:')], 'Job completion timestamp source absent'),
    ('report_contributors_missing', [(TYPE, 'contributorUserIds:ID[]', 'contributorUserIds:ID')], 'Report revision contributors absent'),
    ('filtered_kpi_old_expectation', [('01-requirements/contractor.md', '②一覧1件、offerCount=1／activeCount=0／reviewCount=0', '②一覧1件、offerCount=1／activeCount=1／reviewCount=1')], 'Filtered partner KPI acceptance mismatch'),
    ('contributor_deny_missing', [('02-design/review-resolution-contracts.md', 'accept/returnともFORBIDDEN', 'accept/returnとも許可')], 'Contributing reviewer denial absent'),
    ('severity_resolved_included', [('02-design/review-resolution-contracts.md', 'status=open/acknowledged', 'status=resolved')], 'Job severity source and ranking absent'),

    ('history_free_text', [(TYPE, 'redactedReportSummary:ReportHistorySummary', 'redactedReportSummary:string|null')], 'Report history must not expose free text'),
    ('absent_report_accepted', [(TYPE, "{hasReport:false;acceptance:'not_accepted'}", "{hasReport:false;acceptance:'accepted'}")], 'Absent report cannot be accepted'),
    ('address_missing_null', [(TYPE, 'siteAddress:string|null', 'siteAddress:string')], 'Offer must use nullable installation address'),
    ('disabled_reason_missing', [(TYPE, "disabledReason:'capability_changed'|'unit_archived'|null", 'disabledReason:string|null')], 'Rule disable reason absent'),
    ('stale_operation_count', [('02-design/common.md','136個のローカルサービス操作','135個のローカルサービス操作')], 'Common operation count drift'),

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
