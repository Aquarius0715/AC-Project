---
document_id: DD-CONTRACTS
version: 0.21.0
status: proposed-frontend-contract
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# Frontend Input and Output Contracts and Mock Behavior

This document turns the [Common Detailed Design](common.md) into implementable inputs and outputs. Implement it together with each role's field tables and business rules. Values here define the phase 1A demo (DEC-09). Scope is browser-only screen models, forms, and mock services. This document does not define databases, server processing, API endpoints, or authentication. “Save,” “unique,” and “audit” refer to handling fictional data only in the browser, not guarantees of production persistence or security.

**Implementation baseline for 0.21.0**: Read all chapters of the [Deterministic Contracts](deterministic-contracts.md) and strict-review-contracts.md, the authorization columns of the operation catalog, and the screen catalog together. Do not guess values, permissions, asynchronous behavior, or recovery during implementation. These are demo design proposals, not approval for production business use.

## DDC-01 Shared Frontend Types and Display Consistency

| Type | Values and constraints | Storage and display |
|---|---|---|
| EntityId | 1–128 letters, digits, hyphens, or underscores; opaque, with no encoded meaning | Generated at creation and immutable. A URL value alone does not prove permission |
| Version | Integer >=1 | Increases by 1 on each update. Use the version shown in the form as `expectedVersion` |
| Instant | ISO 8601 with timezone; stored in UTC | Do not store local time strings unchanged |
| DateRange | from < to, maximum 366 days, [from,to) | 366 days is a demo UI limit, separate from future API limits |
| Money | amountMinor: nonnegative integer; currency: ISO currency code | Within JavaScript safe-integer range. Demo MYR uses 2 decimal places. Do not sum different currencies |
| Percentage | number/null | null for denominator 0. Round display to 1 decimal place. Keep negative reduction rates |
| Measurement | metric, value:number/null, unit, observedAt, receivedAt, origin, quality, isDemo | Normalize NaN/Infinity to value=null/quality=suspect before the canonical DTO. null means absent; 0 means measured zero. Measured/estimated/inspection and demo status are independent |
| ResourceRef | tenantId, entityId, version | Repository checks that the reference is in the same tenant and permitted scope |
| ListQuery | cursor?, limit=25, sort, filters | limit: 1–100. sort and filters use only each operation's allowlist. Unknown keys cause validation errors |
| Page<T> | items:T[], nextCursor:string/null, total:number, snapshotVersion:number | total is the nonnegative snapshot count after authorization, projection, and filtering. Empty means total=0. Phase 1A Page has no unknown-count state; UI initial/loading means not fetched |

Do not reuse rounded display values in later calculations. The demo rounds money to the currency's minor unit only at the final step. Use shared calculation functions to avoid intermediate floating-point rounding errors and test against demo expected values. Keep unit conversions in mappers or calculation functions.

### Shared Operation Types

[service-contracts.ts](service-contracts.ts) is the canonical type definition. Only these aliases are allowed: DemoViewContext=Context, DemoWriteOptions=WriteOptions, EntityId=ID, Version=number, DateRange=Range. Field tables explain responsibilities; they do not define optionality or complete DTOs. Refer to the types for all input/output fields, nulls, and discriminated unions. DDC-01/03 define common value constraints; [Deterministic Contracts](deterministic-contracts.md) define decision order, asynchronous behavior, and projections. Fix document conflicts instead of choosing during implementation.

One manual confirmation is one Action. Multi-unit automation triggers and restrictions return per-unit results.

### Example Inputs and Outputs Between Screens and Mock Services

```json
{
  "unitId": "unit-online-rto",
  "action": { "kind": "set_temperature", "celsius": 24 },
  "expectedUnitVersion": 7
}
```

```json
{
  "data": {
    "id": "command-demo-001",
    "tenantId": "tenant-a",
    "version": 1,
    "createdAt": "2026-09-14T01:00:00.000Z",
    "updatedAt": "2026-09-14T01:00:00.000Z",
    "unitId": "unit-online-rto",
    "actorMembershipId": "customer-a",
    "action": {
      "kind": "set_temperature",
      "celsius": 24
    },
    "diagnosticRunId": null,
    "jobId": null,
    "reason": null,
    "status": "requested",
    "delivery": "not_sent",
    "requestedAt": "2026-09-14T01:00:00.000Z",
    "sentAt": null,
    "acknowledgedAt": null,
    "expiresAt": "2026-09-14T01:00:30.000Z",
    "failureCode": null,
    "correlationId": "corr-demo-001"
  },
  "meta": {
    "correlationId": "corr-demo-001",
    "snapshotAt": "2026-09-14T01:00:00.000Z",
    "eventCursor": 1
  }
}
```

The screen calls `commands.create`. The mock returns `requested`. Keep room temperature 28 degrees and confirmed setpoint 26 degrees as they were at request time; update the setpoint to 24 degrees after a synthetic success event. Do not send HTTP requests or connect to real equipment.

On failure, do not return values that imply success. Reject the Promise with `DomainError` carrying the following attributes. This is an example recorded error.

```json
{
  "name": "DomainError",
  "message": "Resource changed",
  "code": "CONFLICT",
  "messageKey": "errors.resource_changed",
  "correlationId": "corr-demo-002",
  "fieldErrors": {},
  "retryAfterSeconds": null
}
```

Every service succeeds with `Promise<ServiceResult<T>>`. The operation catalog's `result_contract` describes T inside ServiceResult. Even void operations return `ServiceResult<void>`. Never resolve a failure object as success. Query hooks use successful `result.data` for display and pass caught `DomainError` to DDC-03 handling. Local preference changes use the same asynchronous convention.

```ts
try {
  const result = await commands.create(context, input, options);
  showRequestedCommand(result.data); // Show requested, not device success
} catch (error) {
  showDomainError(asDomainError(error)); // Normalize unknown exceptions to UNAVAILABLE
}
```

On `CONFLICT`, fetch current data, ask the user to confirm, and submit a new intent. When resending the same intent for transport reasons while awaiting a device response, reuse its original idempotencyKey.

## DDC-02 Read Projections and Related Fetches

Return only fields needed by each role, even for the same entity. Do not send all customers' data to the browser just to hide it in the UI. Apply these projections in mocks too.

| Projection | Required response fields | Exclusions and related lookups |
|---|---|---|
| UnitSummary | id, version, customerOrgId, propertyId, spaceId, displayName, modelId, capabilityVersion, connection, observedState, latestMeasurements, activeAlertCount | Exclude contract amounts and customer contacts |
| UnitDetail | UnitSummary + installedAt, components, serviceScope, capabilities, lastSeenAt, pendingCommandIds | Keep capabilities, observed settings, and requested settings in separate properties |
| JobOfferSummary | jobId, offerId, type, siteAddress, requiredQualifications, requestedSlot, dueAt, offerExpiresAt, termsVersion | Before acceptance, project the installation property's registered address to siteAddress. Exclude entry instructions, live telemetry, and customer billing |
| JobSummary | id, version, unitId, type, status, dueAt, requestedSlot, scheduledSlot, assignmentId, severity, isDemo | Exclude internal notes and customer billing. Use JobOfferSummary for unaccepted outsourced offers |
| JobDetail | id, version, unitId, type, status, requestedSlot, scheduledSlot, assignment, offer, draftReportRef, reportRefs (reportId/reportVersion), costs, eventCursor | For customers, exclude internal notes and unaccepted reports. Include only costs approved for customer display |
| JobHistorySnapshot | jobId, type, status, contractorOrgId, completedAt, ownDecisionEvents, redactedReportSummary | After external access expires, no live unit access, customer personal information, or new control actions |
| InvoiceDetail | id, version, contractId, contractVersion, amountMinor, currency, dueAt, status/paymentStatus, paymentRefs, restrictionIds | Return no card data. Fetch restrictions through restrictionIds or forInvoice |
| RestrictionDetail | id, version, contractId, causeInvoiceIds, rulesVersion, state, reason, executeAfter, graceUntil, exception, perUnit, events | Each unit has Command ID, state, and last confirmed time |
| DeviceDetail | id, version, unitId, serial, connection, lastSeenAt, sensors, calibrationRefs, firmwareVersion, activeOperation | Treat offline, power loss, and tamper as separate dimensions |
| EnergySummary | period, unitIds, totals, baselineRef, factorRef, tariffVersion, boundary, coverage, qualityWarnings | Uncalculable totals are null. Keep unrounded calculation values separate from display precision |
| AuditView | id, actorId, actorRoleAtTime, action, targetRef, occurredAt, correlationId, result, maskedBefore, maskedAfter, reason | Do not replace historical actor names with current Membership names |

### Additional Models

| Model | Fields and responsibilities |
|---|---|
| Offer | id, jobId, contractorOrgId, termsVersion, offeredAt, offerExpiresAt, accessValidFrom, accessValidUntil, decision, decidedBy, decidedAt, declineReason |
| Assignment | id, jobId, technicianMembershipId, validFrom, validUntil, scheduledStart, scheduledEnd, status, reason. Live access expires when the period ends |
| MaintenancePlan | id, unitId, recurrence, nextDueAt, generatedOccurrences. Phase 1A explicitly generates only the next monthly occurrence. Prevent duplicates using plan ID and scheduled date |
| JobNote | id, jobId, authorId, visibility(internal/customer), message, createdAt. Default: internal. Notification text follows the same visibility |
| Inquiry | id, customerId, invoiceId?, restrictionId?, subjectType, message, state(received/answered), reply?, createdAt. Demo accepts inquiries only in-app |
| CalibrationRecord | id, deviceId, sensorId, metric, unit, referenceValue, measuredValue, calibratedAt, actorId, isDemo. Append history; never rewrite it |
| DeviceOperation | id, deviceId, kind(check/calibrate/firmware), status(queued/running/succeeded/failed), targetVersion?, failureCode?, startedAt?, finishedAt? |
| OffsetQuote | id, version, amountKg, estimatedAmountMinor?, currency?, expiresAt, provider=unselected, scheme=demo, isDemo. Amount is null if price is undecided |
| OffsetRecord | quoteId, amountKg, state, previousState?, purchaseRef?, retirementRef?, demoCertificateRef?, eventHistory. All retirement is demo-only |

## DDC-03 Input Validation, Display, and Failures

| Validation point | Processing |
|---|---|
| URL load | Validate IDs, enums, and dates. Under D01, invalid resource IDs show not-found; invalid filters/dates/enums show a VALIDATION condition-correction screen. Never automatically resolve to another customer's data |
| Field blur/submit | Validate required fields, formats, character limits, and ordering constraints in role tables. Count Unicode code points after trimming leading/trailing whitespace |
| Immediately before mutation | Recheck actor, tenant, role, scope, period, current status, capability, contract, and expectedVersion |
| Adapter response | Validate DTO schema. Unknown enums are UNAVAILABLE. Never show raw internal exceptions |
| Update notification | Use entityId/version/eventId to remove duplicate or old events. Refetch after missing sequence numbers or disconnection |

| Mock DomainError | UI behavior | Inputs and retry |
|---|---|---|
| VALIDATION | Show error summary and fieldErrors; focus the first invalid field | Keep inputs for correction; do not resend unconditionally |
| UNAUTHENTICATED | End the session and go to login | Discard old-scope Queries, unsaved screen drafts/photos, and temporary URLs. Keep saved reports and Blobs in shared Repository under DDC-08 |
| FORBIDDEN | Show action denied. Follow IR57: primary query shows permission-denied in place; writes keep the screen and inputs | Discard target data when permissions change. Do not retry repeatedly |
| NOT_FOUND | Show unavailable. Follow IR57: primary query shows not-found in place; writes keep the screen and inputs | Do not reveal details that disclose existence |
| CONFLICT | Explain the change, fetch current data, and allow comparison | Never overwrite automatically. Reconfirm intent and send a new request |
| OFFLINE (target device offline) | Show device last-contact time and pending/unavailable actions | No new controls. Query existing sends; do not assume success |
| UNAVAILABLE + messageKey=errors.network_disconnected (simulated network loss, IR37) | Show screen offline, last-success time, and “Updates stopped” | Unsubscribe and recover with a fresh snapshot after reconnection. Separate this from the device-offline display |
| TIMEOUT | Show unresolved processing and correlation ID | For writes, query writes.getResult by key, then retry with the same idempotency key and original payload. Do not assume the original never ran |
| RATE_LIMITED | Show waiting time and retry guidance | Show mock retryAfterSeconds. Keep form inputs as usual |
| UNAVAILABLE | Show an in-screen error and allow retry | Automatically retry reads at most twice. Retry writes only on explicit user action |

Follow IR83 for refreshing/failure after successful data and for batching subscription invalidations. Never replace KPI values with zero or healthy values on errors. If keeping previous values, mark stale and show last-success time. Do not keep them after target or role changes.

## DDC-04 Asynchronous and Mock Execution Rules

This section and DDC-01 define demo timing, limits, counts, and periods. If requirements or individual DDs repeat values, this section is authoritative. Change it first, then align every other occurrence.

| Setting | Phase 1A default and behavior |
|---|---|
| Mock wait timeout | 10 seconds. Normal read wait is fixed at fixture.defaultWaitMs=300 ms (IR74). Control tests with an injected clock |
| Command expiry | 30 seconds after request (fictional demo value). Scenario events control sending and acknowledgement |
| Telemetry stale | Per sensor; seed: 120 seconds. Stale when now − observedAt > staleAfterSeconds |
| Offer expiry | Seed: 24 hours later. Accept only while now < offerExpiresAt. Unanswered expiry returns the job to requested (IR48) |
| Notice period | Seed rule: 24 hours; executeAfter >= noticeAt + 24 hours. Not a commercial rule |
| OffsetQuote validity | Demo: 15 minutes. Request requires now < expiresAt |
| Write processing | Shared-memory transition functions check input/version, then update state and display history together. No database transactions are designed |
| Invalidate | Invalidate affected entities and related summaries using scoped Query keys. Do not manually copy data in UI |
| Role change | Abort pending requests and remove old Queries. Fetch the new-scope session before rendering. Discard late responses using IR17 Repository instance/generation and viewEpoch |
| Reload/reset | Follow DEC-07. Reload restores seed. Role switches preserve tab business data. Reset initializes clock/object URLs and increments generation by 1 |

For normal customer/diagnostic Commands, reject conflicting new requests while the same unit has an unfinished request. Post-payment release is separate coordination: keep release intent, then use D03 per-unit evidence to distinguish undelivered, unapplied, applied, and unknown results. Send release Commands only for applied units; confirmed unapplied units are not_required. Never send apply and release concurrently while unresolved.

`AbortController` stops communication or screen updates; it does not cancel already accepted device or payment processing. If future external cancellation is needed, implement it in the adapter after the API contract is defined.

## Notifications and Visibility

| Trigger event | Recipients and visible content | Changes that must not occur |
|---|---|---|
| alert.opened / severity_changed | Notify customer and HQ, plus active assigned technicians. Contractors receive only necessary summaries for accepted units | Marking read does not resolve alerts |
| job.requested | Tell the customer it was received and HQ that it is a new request | Requested slots are not confirmed bookings |
| job.offered / accepted / declined | Notify the offered contractor and HQ. Customers see only “Being arranged” | Do not expose detailed customer information before acceptance |
| job.assigned / schedule_changed | Share confirmed schedule and necessary information with customer, HQ, assigned technician, and accepted contractor | Do not notify other companies' technicians |
| report.submitted / returned | Notify quality reviewer, authoring technician, and HQ. Customer sees progress only | Do not expose internal reports or notes to customers |
| job.completed | Share accepted reports with customer, HQ, accepted contractor, and assigned technician | Do not automatically resolve alerts |
| restriction.scheduled / changed | Tell affected customer and authorized HQ the reason, schedule, targets, and release conditions | Do not share billing details with contractors or technicians |
| payment.confirmed | Notify customer and HQ with billing permission, including related release events | Do not mark released without acknowledgement |
| device.fault / operation_failed | Notify assigned technician and HQ; give customer only necessary summary | Do not assume a fault cause or theft without checking |
| inquiry.received / answered | Notify customer and authorized HQ handler | Do not send real email or WhatsApp messages |

The table summarizes visibility. IR95 is authoritative for whether to generate notifications, templateKey, channel, and recipient Membership selection. In-app notifications store translation keys and params and render in the selected language. Email/WhatsApp are preview or simulated, never actual delivery records. Notifications carry a target reference ID and snapshot scope at notification time. Recheck current permissions when navigating from a notification.

## DDC-05 Frontend Boundary Completion Criteria

- Map each [Operation Catalog](operation-catalog.csv) operation to screen input types, return values, and mock results.
- UI accesses and changes data through service interfaces, without direct dependency on mock implementations.
- Use synthetic success, accepted/pending, failure, conflict, and access-denied results to verify expected display and recovery.
- Provide an adapter replacement boundary for a future real API. Replacement alone does not guarantee production connectivity; define D11 production contracts first. HTTP adapter implementation/testing, production authentication, and server processing design are outside these completion criteria.

## DDC-06 Screen Structure and Local State

Each role page uses shared Shell and UI patterns. Pages compose URLs, Query hooks, and forms; primitives do not import Repository directly. Keep RHF forms separate using target ID as key. Refetching must not automatically reset unsaved dirty values.

Keep short-lived state, such as open dialogs and selected tabs, in the nearest component. Store targets/periods/list pages in URL, observations/history in Query, and business data in Repository, each in one place. Restore meaningful filters on Back. Do not add unnecessary cross-screen Context.

If the main grouped data fetch fails, show a page error. If an independent support panel fails, show an error only there. For example, keep unit details visible if only history fails, and retry history alone. Do not enable actions if capability or restriction data needed for eligibility is unavailable.

## DDC-07 Shared Screen Fields and Processing

| Screen/action | Inputs and defaults | Processing and completion | Failure/cancellation |
|---|---|---|---|
| /login | demoActorId: unselected; returnTo: optional relative route | Set a known actor's Membership in the mock session and open its role home. No real email or password needed | Reject missing/unknown actors. Discard external returnTo URLs |
| /forgot-password | demoEmail: empty; valid format, at most 254 characters | Show the same demo completion message for registered and unregistered addresses: “Instructions will be shown if applicable” | Send no real email. Invalid format shows a field error |
| /settings/preferences | locale=en, zone=Asia/Kuala_Lumpur, currency=MYR (read-only) | Apply locale immediately. zone is a display setting. currency is a fixed MYR demo display attribute. Show invoices with Money.currency without conversion. Data/contract currencies do not change | Reject unsupported locale or invalid IANA timezone |
| Shared notifications | unreadOnly=false, cursor=null, limit=25 | Show scoped notifications as Pages. Allow mark-read mutations and navigation via reference IDs | Expired-scope links show a safe “Unavailable” message |
| Voice/text panel | text: empty; intent: unresolved; target: unselected | Resolve intent and target candidates; confirm changes and use existing Command handling. Queries are read-only | Unrecognized input, duplicate names, or cancelled confirmation create zero Commands |
| /demo | scenarioId (1–64-character label), eventType, clockAdvance, reset | Use only allowed synthetic events. IR37 covers transport/network fault injection; IR36 covers clock jumps. Reset updates generation | Reject arbitrary URLs, scripts, and real-device payloads |

Shared logical operations are `demoSession.signIn/signOut/switchMembership/extend`, `auth.previewPasswordReset`, `preferences.get/update`, `voice.resolveIntent`, `notifications.list/markRead`, and `demo.trigger/reset`. demo/auth/voice are mock-only in phase 1A; real authentication integration is outside scope. Keep preferences in a Provider for infrequently changed display settings, separate from business Repository data and invoice currencies.

### Permission Names and Eligible Roles

| Capability | Eligible roles and constraints |
|---|---|
| control.execute | Customer and HQ, within permitted use/management scope and contract/device capabilities |
| control.diagnose / device.maintain | Technicians within active assignments; external technicians only during the work period. Resolution requires reason and evidence |
| dashboard.read / asset.manage | HQ registry reading/writing. asset.manage alone may read managed units. Apply operation-catalog support-read rules |
| alert.resolve | Technicians within active assignments and HQ. Reason and evidence IDs required |
| job.manage / contract.manage / billing.manage | HQ, within the target tenant |
| partner.accept / partner.assign / partner.review | Contractors, only for their company's delegations. Reject self-approval by the same userId |
| identity.manage / device.manage | HQ, within the same tenant. Users cannot increase their own permissions; another person must make the change |
| alert.policy.manage / automation.policy.manage / energy.manage | HQ, for policies/analysis of managed targets |
| restriction.manage / restriction.override | Independent HQ capabilities, not automatically granted to normal HQ |
| mrv.manage / offset.manage / audit.read | Separate HQ capabilities, distinct from customer analysis/record viewing |

Check both the role allowlist and permission. Reject unknown capability names. Eligibility for a permission is not a default grant. Give each seed actor only explicitly defined capabilities.

### Support Operation Inputs

Creating Inquiry requires `invoiceId` or `restrictionId`, `subjectType`, and `message` (1–2000 characters). HQ answers with `inquiryId`, `reply` (1–2000 characters), and `expectedVersion`, moving `received` to `answered`. Customers can read replies. Closing is optional in phase 1A; do not show it if unimplemented.

Call `mrv.saveDraft` for MRV drafts. The first call returns a new mock ID. Edit with the retained ID and version. This document does not define communication destinations or storage methods.

Include support get/list operations in monitoring/control data fetches and the operation catalog. Every UI edit button needs both a detail-read contract and a save contract.

## DDC-Source Additions. Display Models for Company Requests

The primary source is SRC-06. These are frontend display extensions, not API transport specifications. The relevant DD defines details, required fields, and missing-value handling.

| Existing logical operation/result | Display extension | Design using it |
|---|---|---|
| alerts.list / Alert | causeCode, evidenceKind, evidenceText, observedAt | DD-C08, DD-T07, DD-A05 |
| telemetry.series / air-quality model | allergenObservation availability, substance, value, unit, source, observation time | DD-C07, DD-A12 |
| payments.simulate / Payment, invoices.list / invoice display model | Map `Payment.method` to display `paymentMethod`. Payment.method allows only `demo_credit_card` / `demo_debit_card`. `demo_instructions` is a UI choice returning only a notification preview through the instructions event; it does not change Invoice.paymentMethod. Unselected/manual confirmation uses null. Keep the type during processing and failure | DD-C11, DD-A08 |
| MRV report preview | Scope 2 category, organization, period, sites, regional factor, calculation boundary, quality | DD-A14 |
| offsets.preview / OffsetQuote | marketConcept: future_concept, not selected, unverified, not connected | DD-C13, DD-A15 |

Keep display mappings above in feature-specific pure functions. Mock adapters return synthetic normal, missing, and failed results. Future real API adapters map into these screen models; screens do not call external APIs directly.

## DDC-08 Multi-Resource, Revisit, and Cross-Role Contracts

These are phase 1A design proposals under DEC-10, not final commercial business rules. They apply to DD inputs, outputs, and postconditions. Authorize every operation with explicit resource IDs and current `DemoViewContext`; do not implicitly use a screen's selected ID. In addition to catalog inputs, pass `context` first. Final read options (`{signal?: AbortSignal}`) may carry AbortSignal. Pass mutation `DemoWriteOptions` as the third argument.

### 1. Reads, History, and Attachments

| Operation | Read/write meaning | Permissions and revisits |
|---|---|---|
| policies.list/get | `Policy` includes id, version, kind(alert/automation/air_quality), unitIds, enabled, and relevant DD inputs. `get` returns the current saved version | `alert` requires `alert.policy.manage`; `automation`/`air_quality` require `automation.policy.manage`. Read/write only within scope. Initialize revisited forms through `get` |
| jobs.get | `draftReportRef` and `reportRefs` each pair `reportId` with `reportVersion`. An uncreated draft is null | To create the first draft, technician calls `saveDraft` with `jobId` to generate an ID. Resume existing drafts through `reports.get` |
| reports.get | Return `WorkReport` matching `jobId`, `reportId`, and `reportVersion`. Include items/measurements/parts/workText/nextAction/attachmentRefs, author, version, submission, and acceptance data | Active assignees see their job's draft/submitted versions; quality reviewers see submitted versions; customers see accepted versions only. Past versions are immutable. After external delegation expires, only JobHistorySnapshot is visible; deny report body fetches |
| attachments.add | Add JPEG/PNG to an existing draft's `jobId`/`reportId`. Validate MIME, content, size, and count; keep Blob and `Attachment` in the same mock | Requires active technician assignment and editable draft. `Attachment` has id/jobId/reportId/blobId/name/mime/size/status |
| attachments.getContent | Return the Blob for `attachmentId` linked to the specified `reportVersion`, not a URL | Same visibility rules as `reports.get`. Screen creates temporary URLs using `URL.createObjectURL`, revokes on leave/switch, and refetches/recreates on revisit |
| inquiries.list | Customers receive `Inquiry` linked to their own `invoiceId`/`restrictionId`; HQ receives those within `billing.manage` scope | Customers see received/answered and reply. Open the same invoice through answer-notification target.id, then select inquiryId. Never return another customer's reply |
| devices.addResponseNote | Append `responseNote` (1–1000 characters) to `deviceId`/`eventId` with author, time, and correlation ID | Requires `device.maintain` and active assignment. Notes do not change connection or tamper state. Recovery is a separate `/demo` event |

Keep image bodies in shared Repository `Map<blobId, Blob>`. Sign-out and role switching do not delete shared Blobs. Locally selected unsaved images may be discarded on leave. Remove draft photos through differences in `jobs.saveDraft.attachmentIds`. Keep Blobs referenced by older submitted versions; release unreferenced Blobs. Reset releases all Blobs and temporary URLs. Submission failure must not lose text or saved images.

`WorkReportDraft` inputs are jobId, reportId (absent initially), items (InspectionItemInput), measurements (InspectionMeasurementInput), parts, workText, nextAction, and attachmentIds. The first response returns reportId/version; updates require `expectedVersion`. Drafts may be incomplete. Submit validates each DD's required fields. Never directly edit submitted/past versions. Rework or reassignment creates a new draft version referencing the old one, preserving past versions and each record's author.

### 2. Test Runs

```ts
type CreateDiagnosticRunInput = {
  jobId: string;
  unitId: string;
  startAction: UnitAction;
  endAction: UnitAction;
  durationMinutes: number; // Integer 1–15
  reason: string;          // 1–1000 characters
  expectedUnitVersion: number;
  expectedJobVersion: number;
};
type DiagnosticRun = {
  id: string; tenantId: string; version: number;
  jobId: string; unitId: string; actorMembershipId: string;
  startAction: UnitAction; endAction: UnitAction;
  durationMinutes: number; reason: string;
  state: 'awaiting_start' | 'running' | 'end_requested' |
         'completed' | 'start_failed' | 'end_failed' | 'end_blocked';
  startCommandId: string; endCommandId: string | null;
  startedAt: string | null; endAt: string | null;
  failureCode: string | null;
};
```

Use `commands.create` for one-off diagnostic actions. For timed test runs, `diagnosticRuns.create` creates the run and start Command; `get` returns that run. At creation, validate both actions for capabilities, restrictions, `control.diagnose`, active assignment, online status, and no firmware update. An active run (`awaiting_start`/`running`/`end_requested`) or unfinished Command on the same unit causes `CONFLICT`. `start_failed`/`end_failed`/`end_blocked` are terminal history states; after rechecking state, explicit recovery actions remain allowed. Start and end Commands both carry `diagnosticRunId`.

| Current state | Event | Next state and result |
|---|---|---|
| awaiting_start | Start Command acknowledged before deadline | `running`; `startedAt` is acknowledgement time; `endAt` is `startedAt + durationMinutes` |
| awaiting_start | Start failure or 30-second expiry | `start_failed`; do not start the end timer or show started |
| running | now>=endAt; recheck original assignment, permissions, current capabilities, restrictions, and online status, then request end | `end_requested`; create one `endAction` Command |
| running | At end: assignment expiry, permission removal, capability/restriction change, or FW conflict | `end_blocked`; create zero requests and warn HQ/technician “Not stopped; check required” |
| running | Offline at end | `end_failed`; show “Not stopped; check required” and a requery path, not success |
| end_requested | End Command acknowledged before deadline | `completed`; display observed values from the acknowledged `endAction`. Show “Stopped” only if `endAction` is power OFF |
| end_requested | Failure or expiry | `end_failed`; retain unconfirmed ending and Command history |

At `create`, require now + 30 seconds + duration < assignment.validUntil. Reject schedules expected to outlast assignment. Still recheck at end if permissions expire or are removed during the run. An assignee with current permission recovers through an explicit diagnostic action. Do not resend automatically or rewrite old failed runs as `completed`.

Keep timers as subscriptions to the shared mock clock, not screen-owned timers. Sign-out, navigation away, and role switching abort only viewing requests; accepted run business events continue. Re-evaluate current permissions using the Membership from creation, never the switched-to actor. Reset changes Repository generation and discards old runs and events.

### 3. Manual Payments and Multiple Invoices

- `payments.confirm(paymentId, ...)` confirms an existing `processing` Payment. `payments.recordManual(invoiceId, ...)` creates a new `confirmed` Payment with `method=null` for an `unpaid` invoice with no Payment or only failed history. Both require `billing.manage`.
- `recordManual` requires the full invoice amount and matching currency. `reason`: 1–1000 characters; `paymentReference`: 1–128. If an `initiated` or `processing` Payment exists, return `CONFLICT` and requery after its result is final. Never create a new payment for a `paid` invoice.
- Repeated confirmation with the same tenant/invoice/reference and matching amount/currency returns existing success before checking `expectedVersion`. Reusing a reference for another invoice or changing its amount returns `CONFLICT`. A new intent/key does not allow duplicate payment.
- Update Payment `confirmed`, Invoice `paid`, audit, notifications, and related restriction evaluation together in one mock transition. Successful customer card events use the same finalization function. If simulated payment and manual confirmation compete, only one finalizes.
- Derive Invoice `paymentMethod`/`paymentStatus` from the latest Payment `method`/`status`. If manual payment succeeds after card failure, current method becomes null while old card type remains in history. `paymentRefs` returns id, version, method, status, reference, and confirmedAt as needed for display.

On restriction creation, `causeInvoiceIds` lists all overdue unpaid invoices in the same contract. Reject an empty set. Recalculate in the mock, verify the input set matches, then freeze it. Match contract, customer, and currency; never mix contracts. `restrictions.forInvoice` searches membership in this set.

Only when every cause invoice is `paid`, automatically move `scheduled` to `cancelled` and `requested`/`applied` to `release_requested`. One payment alone keeps application state and shows the remaining unpaid cause count. `processing` is not `paid`. Do not automatically add later invoices to existing notices.

Phase 1A allows one active restriction (scheduled/requested/applied/release_requested) per unit. Reject overlapping new `schedule` as `CONFLICT`. After the existing restriction becomes `cancelled` or `released`, a new cause set may have a new notice ID. Proposed permissions: `restriction.override` for manual release; `restriction.manage` for release due to grace/exception after application. Neither changes Invoice. release with unpaid invoices and no grace/exception returns FORBIDDEN; in release_requested it idempotently returns current state (IR35). Keep this distinct from exception-operation permission paths.

### 4. Restriction Commands and Release Observations

```ts
type RestrictionPolicy =
  | { kind: 'temperature_limit'; minimumCoolingSetpoint: number }
  | { kind: 'power_off' };
type RestrictionAction =
  | { kind: 'apply_restriction'; restrictionId: string;
      rulesVersion: string; policy: RestrictionPolicy }
  | { kind: 'remove_restriction'; restrictionId: string;
      rulesVersion: string };
```

`RestrictionAction` is an internal action created only by shared mock restriction transitions. Reject it through `commands.create`, voice, customer automation, or diagnosis. `Command.action` is `UnitAction` or `RestrictionAction`. Device confirmed state holds `observedRestriction: {restrictionId, rulesVersion, policy} | null` and confirmation time, separately from ordinary settings.

| Acknowledged action | Confirmed state change | Values kept unchanged |
|---|---|---|
| Apply temperature_limit | Save to `observedRestriction`. Raise a setpoint below the minimum to the minimum; otherwise keep it | Power state and room temperature |
| Apply power_off | Save to `observedRestriction` and set confirmed power OFF | Setpoint and room temperature |
| remove_restriction | Set `observedRestriction` for the target ID/version to null. Recalculate ordinary action eligibility under SR05/SR26. Keep restrictions if a successor or unresolved case exists | Do not turn ON automatically or restore the pre-application temperature. Keep power and setpoint as they are at release |

Do not change observations before acknowledgement. Ordinary action policy still enforces restrictions during apply/release requests to prevent bypass. Per-unit release completion requires both success for the same restrictionId/version/Command ID and a null observation. Aggregate state becomes `released` only with released or not_required evidence for every unit. If apply is unresolved, keep release intent and query under D03; use `remove` if applied or `not_required` if confirmed unapplied. Late apply events never return `release_requested` to `applied`.

### 5. Shared Terminology and Input-Type Rules

Canonical names are `UnitAction`, `parentSpaceId`, `planType=rto`, `channel=inApp/email/whatsapp`, and proof-reference prefix `DEMO-`. The table defines canonical role identifiers; other names are translated display labels.

| Role | role enum | route prefix | permission prefix | Organization kind | Formal role name (short form) | English display |
|---|---|---|---|---|---|---|
| Client | client | /customer | control.* | customer | Client (customer) | Client |
| Contractor | contractor | /partner | partner.* | contractor | Contractor | Contractor |
| Technician | technician | /technician | control.diagnose, device.maintain, alert.resolve | contractor or operator | Technician (internal/external) | Technician |
| Administrator | admin | /admin | job/contract/billing/identity/device/restriction/mrv/offset/audit/*.policy | operator | Administrator (HQ) | Admin / HQ |

Canonical resources: ACUnit (AC equipment), Device (IoT device), MaintenanceJob (customer work request), Offer (HQ delegation offer to a contractor), Assignment, Inquiry (customer inquiry), Alert, and Notification. “Request” means Job; “delegation” means Offer. Company-facing PrepareDocument uses formal names. “Customer,” “contractor,” and “HQ” in requirements/designs are short role names. Never infer routes or enum values from labels. preview/simulated/failed are `deliveryState`, not channel values. UI “RTO,” “customer,” and “HQ” are translated labels separate from stored enums.

Each `*Input` carries the corresponding DD inputs as named properties. For save, omit id on creation and require it for existing records. Updates require `expectedVersion`. `PolicyInput` uses `kind` to distinguish DD-A05, DD-A11, and DD-A12 schemas. `AutomationInput.kind` distinguishes `schedule` (DD-C04) and `event` (DD-C05), with shared `id?`/`name`/`unitIds`/`timezone`/`enabled`/`priority` (default 50). Event conditions: `occupancy={occupied:boolean}`, `location={event:arrival/departure}`, `pattern={localTime:HH:mm}` (IR52 evaluation), `weather={metric:temperature,operator:gt/gte/lt/lte,value:number}`. HQ occupancy uses the same form; its demo-only conditions are `tariff={operator,value,unit:MYR_per_kWh}`, `peak={active:boolean}`, and `solar/battery={operator,value,unit:kW}`. Missing values never count as matched conditions.

Use only the [Query Contract](query-catalog.csv) allowlist for `ListQuery` filters/sort/defaults. Search names in prose are display labels; do not add unregistered keys. Resolve aliases through filter_mapping/sort_mapping; unknown keys return VALIDATION.


### 6. Triggering Automation and Policies

Trigger/simulate input is EvaluationInput; results are SimulationResult/FireResult. Apply D02's same-snapshot evaluation, arbitration of all candidates, owner actor, per-unit results, and event deduplication. Retire old single-rule inputs that bypass other policies. simulate creates no Commands/audit/notifications. fire authorizes the caller separately from suppression within evaluation.

## DDC-09 Shared Audit and Test Rules

Business changes record action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt. Successful reads do not add business history. Record access denials with masked content. Recipients and publication timing follow “Notifications and Visibility.” Read-only screens do not mutate. Validate all Repository inputs, with or without forms. Apply DDC-03 shared rules for authorization/version checks immediately before saving and failure recovery.

MRV review resubmission with the same `reportId`, `reportVersion`, and comment returns the existing result without adding history. A different comment on the same reviewed version returns `CONFLICT`. A new report version requires a new review.

Customer notes allow only `visibility=customer`; contractor notes allow `internal` or `customer`. UI recipientRole is a recipient-filter label. Pass `notifications.preview` a recipientMembershipId selected from current job participants. Multiple candidates require selection; zero candidates prevents sending. `internal` forbids disclosure to `customer_contact`. Do not allow arbitrary addresses or copied billing-record references. Do not claim complete automatic detection of free-text content. Use only fictional demo data.

0.9.0 correction contracts: Read the [Strict Review Correction Contracts](strict-review-contracts.md) and [Per-Operation Version Contract](write-version-catalog.csv) together.

Additional contracts for current version 0.21.0: Read IR01–106 in the [Re-review Correction Contracts](review-resolution-contracts.md). They take priority over older text on the same topic; follow IR72 for conflict precedence.
