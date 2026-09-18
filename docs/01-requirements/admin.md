---
document_id: REQ-A
version: 0.21.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# Administrator and HQ requirements

**0.21.0 implementation baseline**: Read all chapters of [deterministic contracts](../02-design/deterministic-contracts.md) and strict-review-contracts.md, the authorization column in the operation catalog, and the screen catalog. Do not guess numbers, permissions, asynchronous behavior, or recovery during implementation. These are demo design proposals, not production business approval.

## Purpose and assumptions

This document reorganizes the [original company requirements in English (SRC-06)](../00-prepare/sources/company-requirements-original.txt), the primary source.

Trace information through company original → BIZ groups (categories of company requests) → FR (functional requirements) here → detailed design/acceptance criteria. Each feature separates company requests from design additions. Screen fields, input limits, state changes, and priorities are frontend proposals, not detailed company approvals. Reference mocks guide appearance. Requirements and acceptance criteria make original goals concrete through production instructions and added design details.

This document covers what users can view, enter, and do in the frontend. Registration, billing, receipts, device actions, and notifications are all mocked. Real backend processing, storage, and authentication are not requirements here.

Administrators manage customers, units, maintenance, billing, control policies, and environmental results for their managed organizations.

Required reading: [PrepareDocument](../00-prepare/PrepareDocument.md) and [common requirements](common.md). All common authentication, language, voice, permission, notification, and non-functional requirements apply unchanged.

P0 items are core foundational features. P1 items must also be completed in phase 1A. Verify each row under its `AT-A` number. Separate company-request coverage from concrete screen proposals and provisional values.

## Functional requirements and acceptance criteria

| Requirement ID | Priority | Status/basis | Requirement | Acceptance criteria |
|---|---|---|---|---|
| FR-A01 | P0 | Company original SRC-06 + added design details / BIZ-04, BIZ-08 | Overall dashboard | Show customer/unit counts, operation rate, alerts, maintenance, unpaid amounts, power, and estimated savings from a prorated assumed baseline (IR78). Show rate denominators and periods. |
| FR-A02 | P0 | Company original SRC-06 + added design details / BIZ-07 | Organization/location/unit registers | Create/edit customer→property→area/floor/room→unit. Reject inconsistent parent-child relationships. |
| FR-A03 | P0 | Added design details (supporting a company goal) / BIZ-04 | Four roles/scope management | Configure four roles, internal/external technicians, delegation periods, scope, and actions. Permission changes affect old open screens. |
| FR-A04 | P1 | Company original SRC-06 + added design details / BIZ-06, BIZ-20 | Model/IoT capabilities | Manage brands, models, controls, sensors, and firmware. Disable unsupported actions. |
| FR-A05 | P0 | Company original SRC-06 + added design details / BIZ-08, BIZ-11, BIZ-17 | Alert/notification policies | Set metric, threshold, duration, recipients, channels, and escalation rules; trigger demo alerts. |
| FR-A06 | P0 | Company original SRC-06 + added design details / BIZ-12 | Maintenance plans/quality/costs | Create scheduled/reactive/preventive plans and choose internal assignment or contractor delegation. Review deadlines, quality, estimated/actual costs. |
| FR-A07 | P0 | Company original SRC-06 + added design details / BIZ-21 | Contract plans | Save RTO, general maintenance, energy-saving, and environmental plans linked to units. |
| FR-A08 | P0 | Company original SRC-06 + added design details / BIZ-21, BIZ-22 | Invoices/receipts/reminders | Create invoices and confirm simulated payments. Reminder previews also appear in customer screens. |
| FR-A09 | P0 | Company original SRC-06 + added design details / BIZ-21 | Notice/restriction/release after payment | Simulate notice→execution request→device application and payment→release request→device response under contract rules. Keep processing pending offline. |
| FR-A10 | P0 | Added design details (supporting a company goal) / BIZ-21 | Grace/exceptions/manual release/audit | Authorize grace/exception/cancel with restriction.manage and forced release with restriction.override: two permissions. Trace reason, actor, and before/after. |
| FR-A11 | P1 | Company original SRC-06 + added design details / BIZ-14, BIZ-16, BIZ-17 | Automation policies | Review control proposals based on occupancy, time-of-use tariffs, peak adjustment, and simulated solar/battery data. |
| FR-A12 | P1 | Company original SRC-06 + added design details / BIZ-18, BIZ-19 | Air-quality policies | Set per-metric thresholds/notifications. Send demo ventilation requests only to supported units; notify unsupported units only. |
| FR-A13 | P1 | Company original SRC-06 + added design details / BIZ-23, BIZ-25 | Energy-saving analysis | Show actuals versus matched baseline, savings percentage, quality, period, and baseline version. Negative savings show increase, not zero (IR68). |
| FR-A14 | P1 | Company original SRC-06 + added design details / BIZ-25 | Digital MRV demo | Create report previews with measurements, calculation methods, factors, quality, and review history. |
| FR-A15 | P1 | Company original SRC-06 + added design details / BIZ-24, BIZ-26 | Offset demo | Separately show simulated quotes, purchase requests, retirement records, and proof states. |
| FR-A16 | P0 | Company original SRC-06 + added design details / BIZ-20 | Unusual actions/audit | Filter tamper suspicions, communication faults, denied events, and action history; trace by correlation ID. Four results: success, denied, failed, pending (IR90). |

## Business boundaries and dependencies

Use only the [common permission matrix](common.md) to decide permissions. Check screen visibility and write access separately, then recheck just before service calls. Do not reuse old screen permissions after capability, work-period, or contract changes.

Real device control, external notifications, payments, and API authentication belong to 1B. In 1A, provide interactive simulations of success, rejection, failure, and missing data.

## Completion criteria

- Meet all FR-A and applicable FR-X/NFR requirements. Do not relabel unfinished features as out of scope to claim completion.
- Map acceptance criteria to screens, services, and failure behavior in the [detailed design](../02-design/admin.md).
- Follow the [verification plan](../04-agentic-sdlc/verification.md) and retain evidence for every AT-A and applicable scenario.
- Return open business decisions to PrepareDocument’s OPEN register. Report provisional mock decisions.

## Feature use cases and business rules (0.6.0)

The table above is an index. The following sections detail entry conditions, steps, results, and acceptance criteria. Numbers ①②… in acceptance cells identify subcases in order (for example AT-C01-E.01); matching Given/Then numbers are paired. Use fixed fixtures from the [verification plan](../04-agentic-sdlc/verification.md). Thresholds and detailed operating rules absent from the original are 1A proposals under DEC-09, not confirmed production rules.

### FR-A01 Overall dashboard

- **Company request basis**: SRC-06 BIZ-04, BIZ-08 — Visual dashboards for customers, internal/external technicians, and administrators/HQ; prompt fault detection/notifications before or at occurrence.
- **Added design details**: Overall totals and links to the next work screen.

- **Entry conditions**: HQ Membership can view target-tenant totals.
- **Main flow**: Filter customer/site/period → check customer count, units, operation, alerts, maintenance, billing, power → open matching lists from KPIs.
- **Business rule BR-A01**: Operation rate is powerOn among active units with known latest state. Always also show total units and unknown count. Customer count includes Customers whose Customer.status and Organization.status are both active (one-to-one in 1A, IR40). Overdue billing uses unpaid amounts. Count only unresolved critical/warning alerts (IR51). Estimate savings by prorating an assumed baseline matching the exact unit set; without one, show “Baseline not set” (IR78).
- **Resulting business state**: Read-only. Dashboard and linked lists share period, scope, and metric definitions.
- **Boundaries/prohibitions**: Do not silently include unknown units in the rate denominator. Separate currencies rather than adding them. With zero records, show that the rate cannot be calculated.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A01-N | hq-operator. Apply fixture-contract.json `acceptancePatches["AT-A01-N"]` (IR85) to demoSeed: tenant-a has two ON, two OFF, one unknown unit; two active customers and overdue unpaid 120.00MYR remain as seeded. When: Open `/admin` for today, follow unit KPI to list | ① 50.0% operation rate, denominator four; also show five total/one unknown ② Customer count 2 ③ Unpaid 120.00MYR ④ List URL contains scope/selected powerState, not period; Back restores period ⑤ Savings card labeled “Estimate (prorated assumed baseline, demo)” (IR78) |
| AT-A01-E | Open totals with ① Unknown units ② Unpaid MYR and USD ③ Zero units | Show unknown separately, exclude from rate denominator. Sum MYR/USD separately. Zero-unit rate is “Cannot calculate (null).” |
| AT-A01-B | ① After `acceptancePatches["AT-A01-N"]`: ON2/OFF2/unknown1 ② Mixed active/inactive customers ③ Overdue unpaid invoices | ① 50%, unknown1 shown ② Count only active ③ Determine from unpaid amount |

Design: [DD-A01](../02-design/admin.md#dd-a01-details). Assess parent AT-A01 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-A02 Organization, location, and unit registers

- **Company request basis**: SRC-06 BIZ-07 — Manage home/office, area, floor, room, and space.
- **Added design details**: Registration, editing, and archiving.

- **Entry conditions**: Register-edit permission for managed organizations; customer/property/unit relationships available.
- **Main flow**: Create customer organization → property/location hierarchy → register split unit model/location → search/edit/relocate/archive.
- **Business rule BR-A02**: New unit customerOrgId/spaceId/modelId must agree. installedAt may be null; reject future dates (IR44). Exclude archived properties/locations/units from all lists/totals/candidates; only HQ asset.manage may retrieve them individually read-only (IR39). Units linked to contracts/jobs/IoT cannot be physically deleted. Cross-tenant transfer is out of scope for 1A.
- **Resulting business state**: Keep IDs, increment version by one on each successful change, and record before/after/reason. Relocation updates current location and keeps old location in history.
- **Boundaries/prohibitions**: Reject another customer's room, hierarchy cycles, nonexistent modelId, and deletion of in-use units. Do not register new units for inactive customers.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A02-N | hq-operator. When: Register customer organization → home property → floor/room → split unit with valid modelId → relocate with reason → archive | ① IDs/version=1 ② Current location updated, old location in history ③ archived; reject if in use. Excluded from units.list/summaries/KPI denominator; HQ units.get returns archived:true, customer units.get NOT_FOUND (IR39) |
| AT-A02-E | ① customer-b spaceId ② Cyclic parents ③ Nonexistent modelId ④ Physically delete contracted unit ⑤ Register for inactive customer | ①② VALIDATION (inconsistent references within same tenant) ③ NOT_FOUND ④ CONFLICT ⑤ VALIDATION |
| AT-A02-B | ① Same-customer spaceId ② Other-customer spaceId ③ Unit linked to contract/job/IoT ④ Unused unit | ① Success ② Rejected ③ No physical deletion; active contract/job/Device binding also makes archive CONFLICT; archive allowed with only ended history ④ Deletion allowed |

Design: [DD-A02](../02-design/admin.md#dd-a02-details). Assess parent AT-A02 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-A03 Four roles and scope management

- **Company request basis**: SRC-06 BIZ-04 — Visual dashboards for customers, internal/external technicians, and administrators/HQ.
- **Added design details**: Permission, membership, and assignment-period management.

- **Entry conditions**: identity.manage; target's current role/scope/valid period known.
- **Main flow**: Select user/membership → set four-role and internal/external technician type → set scope/period/individual capabilities → review changes → save.
- **Business rule BR-A03**: Roles and capabilities are separate. Admin does not automatically receive restriction.manage/override. External technician unit access needs an end date. Only another HQ identity administrator may grant a person elevated permissions.
- **Resulting business state**: Update Membership version/scopeVersion. Clear old session screen caches; evaluate next writes under new permissions.
- **Boundaries/prohibitions**: Reject other-tenant scope, external technician without end date, start≥end, and self-granted override. Reject old-screen saves after revocation.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A03-N | identity.manage. When: Save tech-external-b with role=technician, employment=external, validUntil=2026-09-30, permissions=[control.diagnose] | ① Membership version+1, scopeVersion updated ② Old session cache cleared ③ Audit reason retained |
| AT-A03-E | ① Other-tenant scope ② External validUntil=null ③ start≥end ④ Grant own restriction.override ⑤ Save old pre-revocation version | ① NOT_FOUND ②③ VALIDATION ④ FORBIDDEN ⑤ CONFLICT |
| AT-A03-B | ① Initial role=admin capabilities ② External technician expiry ③ Permission grant by self/another HQ | ① No restriction.manage/override ② End date required ③ Self rejected, another HQ allowed |

Design: [DD-A03](../02-design/admin.md#dd-a03-details). Assess parent AT-A03 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-A04 Model and IoT capability register

- **Company request basis**: SRC-06 BIZ-06, BIZ-20 — HVAC in Phase 2, broader brands/split/central/cassette support; small low-cost in-unit devices/firmware and removal/theft protection/notifications.
- **Added design details**: Model capabilities and IoT register editing.

- **Entry conditions**: device.manage (IR74). Capability values belong to a demo register.
- **Main flow**: Register model → set temperature/mode/fan/sensor/ventilation/firmware choices → check affected units → save new capability version.
- **Business rule BR-A04**: Unverified capabilities are false/unknown. Do not infer supported modes from product category. Disable existing automation rules that no longer match new capabilities, with a reason.
- **Resulting business state**: Update Capability version and Unit control choices. Preserve unfinished Command content as history; never rewrite it to match new capabilities.
- **Boundaries/prohibitions**: Reject min>max, step<=0, or modeControl=true with empty modes. Do not include unsupported firmware versions.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A04-N | device.manage. When: Register model with min=16, max=30, step=1, modes=[cool,dry], ventilation=false | ① Capability version=1 ② Applied to Unit choices ③ Unfinished Command content unchanged |
| AT-A04-E | ① min>max ② step=0 ③ modeControl=true, modes=[] ④ Unsupported firmware candidate | All VALIDATION, zero saves |
| AT-A04-B | ① Register unverified capabilities ② Change capabilities to conflict with existing automation | ① false/unknown ② Affected rules disabled with reason |

Design: [DD-A04](../02-design/admin.md#dd-a04-details). Assess parent AT-A04 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-A05 Alert and notification policies

**Requirement made concrete from the original — Open-window/poor-insulation load notifications (BIZ-17)**

Notify about increased load possibly caused by open windows or poor insulation, with target unit, time, evidence, and checking/maintenance links.

**Additional acceptance AT-A05-SRC**: Use three `acceptancePatches["AT-A05-SRC"]` fixtures (IR98): suspected open window, inspection evidence of poor insulation, and no evidence. Each has distinct wording/evidence/time. Reading does not resolve the alert.

- **Company request basis**: SRC-06 BIZ-08, BIZ-11, BIZ-17 — Prompt fault detection/notification before/at occurrence; early vibration/high-temperature/low-refrigerant/tiny-leak/clogged-filter detection; routine/weather-based operation and open-window/poor-insulation load notifications.
- **Added design details**: Threshold settings and alert processing.

- **Entry conditions**: alert.policy.manage and recipient-view permission; metric units/target units known.
- **Main flow**: Set target/metric/comparison/duration → recipients/channels/escalation delay → save → test trigger/recovery with synthetic values.
- **Business rule BR-A05**: Metric units are fixed. Do not use missing/stale data as normal threshold readings; route to connection/quality notices. Cooldown suppresses repeated notices; severity change is a new notification reason.
- **Resulting business state**: Save Policy version. Matching conditions create Alert and Notification preview. Notification read status differs from Alert acknowledgment.
- **Boundaries/prohibitions**: Reject no recipients, zero duration, and recovery thresholds inconsistent with comparison direction. Check just-below/equal thresholds and duration boundaries.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A05-N | alert.policy.manage. When: Save `acceptancePatches["AT-A05-N"]` input (name, unitIds=[unit-online-rto], temperature gte 30, recoveryThreshold 28, durationSeconds 60, warning, recipient customer-a, inApp, cooldown 5, escalate 60, enabled=true); simulator=false; inject unit-online-rto temperature 30.0°C via demo.trigger and advance 60 seconds | ① Policy version=1 ② One Alert, one Notification to customer-a (deliveryState=simulated) ③ Reading notification leaves Alert unchanged |
| AT-A05-E | Save recipients=[], duration=0 seconds, or reversed recovery threshold. Also test just-below/equal threshold and just-before/equal duration | Invalid recipients/duration/recovery direction return VALIDATION, no save. For gte 100, 99 does not trigger; 100 triggers after the full duration, never just before it. |
| AT-A05-B | ① missing/stale ② Same severity recurs during cooldown ③ Severity increases | ① Exclude from measurement evaluation; quality notification ② Notification suppressed ③ New notification |

Design: [DD-A05](../02-design/admin.md#dd-a05-details). Assess parent AT-A05 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-A06 Maintenance plans, quality, and costs

- **Company request basis**: SRC-06 BIZ-12 — Scheduled, reactive, preventive, and general non-RTO maintenance.
- **Added design details**: Intake, outsourcing, and quality review.

- **Entry conditions**: job.manage; target units and internal/outsourced options known.
- **Main flow**: Register maintenance type/unit/deadline → assign internally or offer to contractor → track schedule/progress → record quality review/actual costs.
- **Business rule BR-A06**: HQ sets internal assignee/schedule. Contractors assign their own staff after accepting an Offer. Recurring plans show next occurrence; create only one job per plan/occurrence. IR48 governs unanswered Offer expiry; IR56 governs cancellable states.
- **Resulting business state**: Link Job, Assignment, Offer, cost lines, and quality history by jobId. Work completion alone does not resolve Alert.
- **Boundaries/prohibitions**: Check re-offer after decline, confirmed schedule overlaps, overdue work, and quality returns. Aggregate estimated/actual costs separately by currency without conversion.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A06-N | job.manage. When: Create reactive unit-non-rto job → offer contractor-a (offerExpiresAt=2026-09-15T01:00Z, accessValidFrom=2026-09-14T01:00Z, accessValidUntil=2026-09-22T00:00Z) → accept → assign tech-external-a 2026-09-21 10:00–12:00 Asia/Kuala_Lumpur → advance to 2026-09-21T02:00Z → technician start→submit → contractor-a approves → HQ records actual costs | ① Same jobId: requested→offered→accepted→assigned→in_progress→submitted→completed ② costLines saved ③ Alert resolution assessed separately |
| AT-A06-E | Re-offer after decline / overlap confirmed schedules / overdue / return report / estimated MYR and actual USD | New offerId, requested→offered after decline. Cannot save confirmed overlap. Overdue shows warning without automatic state change. Return becomes rework_requested. Costs grouped by currency. |
| AT-A06-B | ① HQ internal assignment ② Contractor assigns after acceptance ③ Generate same plan/date twice | ① assigned ② accepted then assigned ③ Second CONFLICT, still one job |

Design: [DD-A06](../02-design/admin.md#dd-a06-details). Assess parent AT-A06 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-A07 Contract plans

- **Company request basis**: SRC-06 BIZ-21 — Administrators can reduce/stop cooling for continuing unpaid RTO or similar contracts. The original names both rent-to-own and other programs to add later.
- **Added design details**: Plan types and contract editing steps.

- **Entry conditions**: contract.manage; customer and linked units in the same tenant.
- **Main flow**: Set plan/period/price/units → restriction eligibility/rules version → review → save/revise contract.
- **Business rule BR-A07**: General maintenance has restrictionEligible=false. Even RTO requires explicit eligibility. Contract revisions do not retroactively change issued invoices.
- **Resulting business state**: Keep contract versions; new invoices reference the new version, old invoices the original. Contract expiry does not mean a real unit has stopped.
- **Boundaries/prohibitions**: Reject other-customer units, reversed periods, negative prices, and restrictions on general maintenance. Do not block monitoring/maintenance of uncontracted units.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A07-N | contract.manage. When: Save customerId=cust-b, unitIds=[unit-other-customer], planType=rto, restrictionEligible=true, rulesVersion=demo-v1 → issue invoice → revise | ① Contract version=1 ② Invoice references version=1 ③ Contract version=2, existing invoice retains original version |
| AT-A07-E | ① cust-b unit-other-customer on customerId=cust-a contract ② Reversed period ③ Negative price ④ Enable restriction for general ⑤ Monitor uncontracted unit | ①②③④ One error under D01 ⑤ Monitoring allowed |
| AT-A07-B | ① general ② Ineligible rto ③ Eligible rto ④ Revise after invoice issuance | ①② Not restrictable ③ Restrictable ④ Old invoice unchanged |

Design: [DD-A07](../02-design/admin.md#dd-a07-details). Assess parent AT-A07 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-A08 Invoices, receipts, and reminders

**Requirement made concrete from the original — Card types and payment guidance (BIZ-22)**

HQ can view the client's chosen demo credit/debit card or payment-instruction type and result. Clients perform payment actions; authorized HQ users confirm receipts and preview guidance.

**Additional acceptance AT-A08-SRC**: After each client payment method's processing/success/failure simulation, HQ sees the same method/state on the invoice. Before action, show “Not selected.” Retain method after failure. Guidance previews alone leave unpaid.

- **Company request basis**: SRC-06 BIZ-21, BIZ-22 — Reduce/stop cooling for continuing unpaid RTO bills; WhatsApp/email links to card payments/instructions.
- **Added design details**: Invoicing and simulated receipt confirmation.

- **Entry conditions**: billing.manage; contract/invoice/demo-payment targets can be matched.
- **Main flow**: Create invoice from contract version → filter deadline/state → check demo payment or confirm authorized receipt → check reminder preview/customer display.
- **Business rule BR-A08**: 1A supports full payment only. Reconfirming the same invoiceId/paymentReference returns the same result. Reminders target unpaid overdue invoices; show exceptions/disputes too.
- **Resulting business state**: Audit Payment confirmed/Invoice paid. When all related Restriction causeInvoiceIds are paid, scheduled→cancelled and requested/applied→release_requested. If any remain unpaid, do not release; show pending device responses and remaining count.
- **Boundaries/prohibitions**: Reject amount/currency mismatch, reference reuse on another invoice, and charging paid invoices again. If payment arrives just before reminder execution, recheck and stop the reminder.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A08-N | billing.manage, overdue unpaid invoice. When: Preview invoice-overdue-a → invoices.remind → customer-a payments.simulate(initiate→processing→confirm) → remind again after paid | ① Preview saves zero records ② Explicit action creates one customer notification and Invoice version+1 ③ Payment confirmed/Invoice paid; scheduled restriction cancelled, requested/applied release_requested ④ New reminder after paid CONFLICT, zero extra notifications |
| AT-A08-E | ① Amount mismatch ② Currency mismatch ③ Same reference on different invoice ④ Charge paid invoice again ⑤ Payment just before reminder | ①② VALIDATION ③ CONFLICT ④ CONFLICT ⑤ Stop reminder |
| AT-A08-B | ① Full payment ② Short payment ③ Reconfirm same invoice/reference ④ Remind before due/overdue/paid invoices | ① confirmed ② VALIDATION ③ Same result ④ Only overdue eligible |

**Additional acceptance AT-A08-R01 (revisit, conflict, cross-role)**

| Acceptance ID | Given / When | Then |
|---|---|---|
| AT-A08-R01 | Record full manual payment for unpaid invoice with zero Payments; resend same reference | One confirmed Payment, method remains null, Invoice paid; no double count. Manual payment on processing invoice returns CONFLICT. |

Design: [DD-A08](../02-design/admin.md#dd-a08-details). Assess parent AT-A08 using all N/E/B/R01 and applicable SRC cases in traceability.

### FR-A09 Notices, restrictions, and release after payment

- **Company request basis**: SRC-06 BIZ-21 — Administrators can reduce/stop cooling for continuing unpaid RTO or similar contracts.
- **Added design details**: Notice, execution confirmation, and device responses.

- **Entry conditions**: restriction.manage, eligible RTO contract, unpaid state, and confirmed target capabilities.
- **Main flow**: Enter notice reason/targets/policy/time → customer preview → recheck invoices/grace/exceptions at execution → request application per unit → follow payment-triggered release and device responses.
- **Business rule BR-A09**: Payment confirmation, grace/exception, and override transitions automatically trigger release. `restrictions.release` rejects unpaid cases without grace/exception and is idempotent in release_requested (IR35). Notice reason is customer-visible text (IR42). Reaching a UI deadline does not automatically stop real devices. In 1A, HQ explicitly confirms simulated requests. Do not set applied/released until all targets respond. Power stop and temperature limit are separate policies. IR46 governs allowed actions during application.
- **Resulting business state**: Create Restriction and per-unit Commands. After all causeInvoiceIds are confirmed paid, scheduled→cancelled, requested/applied→release_requested. If any remain unpaid, retain state.
- **Boundaries/prohibitions**: If all cause invoices are paid just before execution, cancel with no apply request. Block application during grace/exception, without notice, or on unsupported devices. Partial offline targets keep overall state from applied; show per-unit pending. Failed release stays release_requested; late apply response cannot restore applied.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A09-N | hq-restriction-manager, overdue invoice-overdue-a, online unit-online-rto/offline unit-offline-rto. When: schedule(power_off) → advance to executeAfter (session survives under IR36) → execute → online unit responds → full payment → restrictions.release → all units respond | ① scheduled ② requested, two Commands ③ One unit applied, aggregate requested ④ Payment confirmation sets release_requested and creates one remove Command for online applied unit; following release is idempotent, same state/no extra Commands (IR35) ⑤ released |
| AT-A09-E | ① All invoices paid just before execute ② During grace ③ Schedule with zero customer notice Memberships (no client can view every target Unit, IR102) ④ Unsupported device ⑤ Partly offline ⑥ Release failure ⑦ Late apply response after release requested | ① cancelled, zero Commands ②④ One error under D01 ③ VALIDATION (IR05), zero Restrictions ⑤ Per-unit pending ⑥ Remains release_requested ⑦ Never returns to applied |
| AT-A09-B | ① Just before notice deadline ② Exactly deadline ③ No HQ confirmation ④ Some/all of two units respond | ① Rejected ② Can execute ③ Zero requests ④ Partial=requested, all successful=applied |

**Additional acceptance AT-A09-R01 (revisit, conflict, cross-role)**

| Acceptance ID | Given / When | Then |
|---|---|---|
| AT-A09-R01 | `acceptancePatches["AT-A09-R01"]`: restriction-limited-a has causes invoice-overdue-a and invoice-b-a. Pay invoice-overdue-a, then invoice-b-a | First payment keeps restriction with one remaining. All paid triggers release. Power remains OFF after power_off release; temperature setting remains after limit release. Applied units need successful remove; confirmed undelivered/unapplied units need not_required evidence. Only evidence for all units permits released. |

Design: [DD-A09](../02-design/admin.md#dd-a09-details). Assess parent AT-A09 using all N/E/B/R01 and applicable SRC cases in traceability.

### FR-A10 Grace periods, exceptions, manual release, and audit

- **Company request basis**: SRC-06 BIZ-21 — Administrators can reduce/stop cooling for continuing unpaid RTO or similar contracts.
- **Added design details**: Grace, exception, and audit steps.

- **Entry conditions**: restriction.manage for grace/exception/cancel; restriction.override for forced release. Independent permissions; neither grants the other.
- **Main flow**: Check current restriction/device application → choose grace/exception/cancel/manual release → confirm reason/expiry/impact → save → track per-unit results.
- **Business rule BR-A10**: Cancel before application becomes cancelled. After application is requested, follow release even if device application is unknown. Expired grace/exceptions do not automatically reapply; conditions need rechecking.
- **Resulting business state**: Record before/after, grace/exception expiry, release reason/actor. Manual release does not settle unpaid Invoice.
- **Boundaries/prohibitions**: Reject HQ without override, empty reasons, and past grace deadlines. Cancelling requested does not prove application was impossible.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A10-N | restriction.manage and override. When: Exempt applied restriction until future date → override_release | ① Exception saved with before/after ② release_requested, Invoice remains unpaid |
| AT-A10-E | ① Manual release without override ② Empty reason ③ Past expiry ④ Cancel while requested | ① FORBIDDEN ②③ VALIDATION ④ release_requested with releaseIntent.source=cancel (IR96); follow release without assuming zero application risk |
| AT-A10-B | hq-restriction-manager creates states through normal actions (IR97 item 3): ① Cancel scheduled Restriction as in AT-C12-N ② Cancel that Restriction after execute makes requested ③ Exempt seed applied restriction-limited-a ④ Advance to ③ exception expiry | ① cancelled ② release_requested, releaseIntent.source=cancel (IR96) ③ Save exception, release_requested ④ No automatic reapplication |

Design: [DD-A10](../02-design/admin.md#dd-a10-details). Assess parent AT-A10 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-A11 Automation policies

- **Company request basis**: SRC-06 BIZ-14, BIZ-16, BIZ-17 — Scheduling, pre-arrival cooling, stop when empty; off-peak pre-cooling, peak-price avoidance, solar/battery integration; routines/weather and open-window/poor-insulation load notifications.
- **Added design details**: Condition setup and simulation.

- **Entry conditions**: automation.policy.manage; target units/capabilities known.
- **Main flow**: Select occupancy/tariff/peak/solar/battery conditions → actions/priority → conflict preview → evaluate synthetic events.
- **Business rule BR-A11**: Priority order: capabilities/active restrictions → HQ policy → customer rules. Within each tier, higher numeric priority wins; ties use ascending ID. Missing/expired data skips execution with a reason.
- **Resulting business state**: Save policy version. Simulation returns selected/suppressed rules and reasons. Actual triggers also use shared Commands.
- **Boundaries/prohibitions**: HQ overrides customer rules; ties always produce the same result. Missing solar data never causes assumed execution.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A11-N | automation.policy.manage. When: Save `acceptancePatches["AT-A11-N"]` input (name, unitIds=[unit-online-rto], tariff gt 0.6 MYR_per_kWh, set_temperature 26, priority=60, timezone, enabled=true); evaluate automations.simulate(occurredAt=now, facts=[unit-online-rto tariff 0.7 MYR_per_kWh, observedAt=now, valid]) | ① Policy version=1 ② Simulation returns selected/suppressed rules with reasons ③ Actual trigger uses Command |
| AT-A11-E | HQ/customer conflict / two HQ rules with same priority / solar=null | Choose HQ, suppress customer. Equal priority uses ascending ID deterministically. solar=null skips with reason and zero Commands for that rule. |
| AT-A11-B | ① HQ/customer match together ② Same tier priority 50/60 ③ Equal priority IDs a/b ④ Null condition | ① HQ wins ② 60 wins ③ a wins ④ Skip |

**Additional acceptance AT-A11-R01 (revisit, conflict, cross-role)**

| Acceptance ID | Given / When | Then |
|---|---|---|
| AT-A11-R01 | Save policy → leave → return → open same policyId | Initialize every form input from saved version via policies.get. Edit/save increments version. Old-version save returns CONFLICT and retains unsaved input. |

Design: [DD-A11](../02-design/admin.md#dd-a11-details). Assess parent AT-A11 using all N/E/B/R01 and applicable SRC cases in traceability.

### FR-A12 Air-quality policies

**Requirement made concrete from the original — Air quality including allergens (BIZ-18)**

Alongside CO₂, dust, and humidity, show allergen data availability and source. Not measured does not mean not detected.

**Additional acceptance AT-A12-SRC**: Switch among not measured (unit-limited), unsupported (unit-non-rto), and synthetic observations (unit-online-rto), three IR98 fixtures, plus missing-unit `acceptancePatches["AT-A12-SRC.4"]`. Do not show zero/safe for unmeasured data. Numeric values without units show unknown.

- **Company request basis**: SRC-06 BIZ-18, BIZ-19 — CO₂, dust, humidity, and allergens with cleaning/ventilation guidance; fresh air when CO₂ rises to improve air quality.
- **Added design details**: Ventilation rules and missing-data handling.

- **Entry conditions**: automation.policy.manage (IR74); metric/ventilation capability definitions on devices.
- **Main flow**: Set metric/threshold/duration/recovery → choose notification only or ventilation request too → check target capabilities → simulate evaluation.
- **Business rule BR-A12**: Automatic ventilation requests only for ventilation=true targets. Others get notifications only. Show behavior for each target before saving.
- **Resulting business state**: Save environmental policy version and notification preview. Track responses to requests; confirm actual indoor improvement through later measurements.
- **Boundaries/prohibitions**: Do not mix ppm and µg/m³ thresholds. Unmeasured data cannot prove normal/recovery. Do not substitute fan Commands for unsupported ventilation.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A12-N | hq-operator with automation.policy.manage. Save `acceptancePatches["AT-A12-N"]` input at 01:00Z; fire evaluation (both units co2=1100 ppm, observedAt=occurredAt=01:00Z). Follow flow: 59 normal one-second ticks, then one more; get finalEvaluation same-tick result at 01:01Z (IR103). simulator=false | ① Policy.version=1; at start/59 seconds, zero target-policy Alerts/Notifications/Commands ② At 60 seconds, one Alert and one warning/inApp/simulated notification to hq-operator per Unit; unit-online-rto requested (ventilate low), one Command; unit-non-rto suppressed/invalid_capability, zero Commands ③ Same-tick refetch/resend changes no counts; improvement unconfirmed until later measurements |
| AT-A12-E | ① Use µg/m³ for ppm threshold ② Reading null ③ ventilation=false, fan=true | ① VALIDATION ② Excluded from evaluation ③ No fan substitution, zero Commands |
| AT-A12-B | Use `acceptancePatches["AT-A12-N"]` and IR103 save/current Fact/60 seconds of normal ticks; apply same CO₂ policy to one supported and one unsupported unit | Through 59 seconds, zero notifications/ventilation requests. At 60 seconds, notify both, request ventilation only for supported unit. Show per-target behavior before save. |

Design: [DD-A12](../02-design/admin.md#dd-a12-details). Assess parent AT-A12 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-A13 Energy-saving analysis

- **Company request basis**: SRC-06 BIZ-23, BIZ-25 — Visualize power/cost against normal operation; expect 10–20% or more waste reduction; baseline comparison, MRV, corporate emissions reporting with regional factors, and future credit creation.
- **Added design details**: Baseline versions and calculation conditions.

- **Entry conditions**: energy.manage, period data in managed scope, and baseline evidence input available.
- **Main flow**: Specify baseline targets/period/method → check comparison conditions → show actual difference → inspect quality/evidence.
- **Business rule BR-A13**: Baseline retains unit set, boundary, period conditions, and model version. Without a weather/other adjustment model, do not label adjusted. Never clamp negative savings to zero.
- **Resulting business state**: Each baseline save creates a new version. Existing MRV reports retain their baseline versions.
- **Boundaries/prohibitions**: Check zero baseline, missing actuals, mismatched unit sets/boundaries. Baseline 100/actual 80 gives savings 20kWh/20%; 100/120 gives DTO −20kWh/−20%, displayed “Increase 20.0 kWh”/“Increase 20.0%” (IR68).

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A13-N | energy.manage, `acceptancePatches["AT-A13-N"]` (unit-online-rto actual 80kWh). When: Save unitIds=[unit-online-rto], [2026-09-14T00:00Z, 01:00Z), boundaryId=ac_input_electricity, method=demo_fixed, baseline 100kWh → compare actual 80kWh | ① Baseline version=1 ② Difference 20kWh/20% ③ Without adjustment model, no adjusted label |
| AT-A13-E | Same patches as AT-C06-E.2–.4: zero baseline / missing actuals / mismatched unit set or boundary / compare 100:80 and 100:120 | Zero baseline: percentage null. Missing actuals: quality note. Mismatch: cannot calculate difference. 100:80 gives 20kWh/20%; 100:120 gives DTO −20kWh/−20%, “Increase 20.0 kWh / Increase 20.0%” (IR68). |
| AT-A13-B | ① Same boundary 100/80 (`acceptancePatches["AT-A13-N"]`) ② 100/120 (two units in `acceptancePatches["AT-C06-E.3"]`) ③ Update baseline version | ① 20kWh/20% ② DTO −20kWh/−20%, “Increase 20.0 kWh / Increase 20.0%” ③ New version, old MRV unchanged |

Design: [DD-A13](../02-design/admin.md#dd-a13-details). Assess parent AT-A13 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-A14 Digital MRV demo

**Requirement made concrete from the original — Scope 2 report preview (BIZ-25)**

Preview corporate electricity-related Scope 2 emissions with organization, period, sites, regional factors, baseline comparison, and data quality.

**Additional acceptance AT-A14-SRC**: The same usage with a different factor version changes result/version. Missing factor shows “Calculation incomplete.” Exclude out-of-period/out-of-scope site data.

- **Company request basis**: SRC-06 BIZ-25 — Baseline comparison, MRV, corporate emissions reporting using regional factors, and future credit creation.
- **Added design details**: Report fields, evidence, and previews.

- **Entry conditions**: mrv.manage; period, units, baseline version, factor version, and boundary selected.
- **Main flow**: Set calculation conditions → preview measurements/quality/results → check evidence → save draft → show demo review history/report preview.
- **Business rule BR-A14**: Factors require region/year/source; unit fixed to kgCO₂e/kWh (IR88/IR102). Use demo_reviewed, distinct from formal external certification. Changed input versions create a new report version; preserve old results.
- **Resulting business state**: MRVReport stores factor/baseline snapshot references, results, quality, and reviewHistory. Preview alone creates no finalized record.
- **Boundaries/prohibitions**: Check missing factor, coverage=0, and duplicate reviews of the same version. Never label unverified values certified.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A14-N | mrv.manage, `acceptancePatches["AT-A14-N"]` (actual 80kWh, baseline-energy-100, factor-demo-2026). When: Set organizationId=org-customer-a, unitIds=[unit-online-rto], same period → preview → save draft → recordReview | ① Preview alone: zero records ② MRVReport draft with factor/baseline snapshots ③ One reviewHistory, status=demo_reviewed |
| AT-A14-E | No factor / preview coverage=0 / send same review twice for same version | Missing factor/zero coverage show “Calculation incomplete,” not certified. Repeated review returns existing result without more history. |
| AT-A14-B | ① Region/year/source present ② Missing ③ Save after input version change | ① Can calculate ② Calculation incomplete ③ New report version, old results retained |

**Additional acceptance AT-A14-R01 (revisit, conflict, cross-role)**

| Acceptance ID | Given / When | Then |
|---|---|---|
| AT-A14-R01 | Reports a/b both version=1; record demo review for reportId=a | Only a gains reviewHistory; b unchanged. Missing reportId returns VALIDATION. Revised baseline/factor does not change old report values/reference versions. |

Design: [DD-A14](../02-design/admin.md#dd-a14-details). Assess parent AT-A14 using all N/E/B/R01 and applicable SRC cases in traceability.

### FR-A15 Offset demo

**Requirement made concrete from the original — Future carbon market integration display (BIZ-26)**

Show optional offsets separately from future tokenization/market integration concepts. The market concept display is this design team's proposal.

**Additional acceptance AT-A15-SRC**: No offset selection creates no request. Opening the market concept creates no balance, real proof, or trade result; show a state distinct from demo retirement.

- **Company request basis**: SRC-06 BIZ-24, BIZ-26 — Optional carbon offset/exchange platform, tokenized savings, distributed ledgers, micro-offsets, market trading, and real-time carbon market APIs.
- **Added design details**: Demo retirement and market concept previews.

- **Entry conditions**: offset.manage; clearly labeled simulated transaction.
- **Main flow**: Set amount/purpose → demo quote → request → demo purchase confirmation → demo retirement → proof preview.
- **Business rule BR-A15**: Purchase request, confirmation, and retirement are separate events. 1A supports only retiring the whole amount of one record. Do not add own calculated emissions savings to purchased balances.
- **Resulting business state**: Keep quoted→demo_requested→demo_purchased→demo_retired history. Proof references use DEMO- and are not real certificates.
- **Boundaries/prohibitions**: Reject retirement before purchase, double retirement, zero amount, expired quote, and other-tenant records. On failure, set failed and retain previous state.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A15-N | offset.manage, `acceptancePatches["AT-C13-N"]`. When: Quote customerId=cust-a, purpose=Demo offset, period=[2026-09-14T00:00Z, 01:00Z), unitIds=[unit-online-rto], amountKg=1 → offsets.simulate request(quoteId,quoteVersion) → purchase_confirm → retire(recordId,attemptId,eventId). Every event demoConfirmed=true | ① quoted ② demo_requested ③ demo_purchased ④ demo_retired, demoCertificateRef prefixed DEMO- |
| AT-A15-E | ① Retire before purchase ② Retire twice ③ amountKg=0 ④ Expired quote ⑤ Other-tenant record ⑥ Purchase failure | ①② CONFLICT ③ VALIDATION ④ CONFLICT ⑤ NOT_FOUND ⑥ failed, previousState retained |
| AT-A15-B | ① quoted ② requested ③ purchased ④ retired ⑤ failed | Distinct stage labels; purchase, confirmation, and retirement are separate events |

Design: [DD-A15](../02-design/admin.md#dd-a15-details). Assess parent AT-A15 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-A16 Unusual actions and audit

- **Company request basis**: SRC-06 BIZ-20 — Small low-cost in-unit devices/firmware and removal/theft protection/notifications.
- **Added design details**: Audit filters and correlation IDs.

- **Entry conditions**: audit.read; authorized managed-tenant projection without sensitive data.
- **Main flow**: Search period/actor/target/result/correlation ID → open history details → compare related Command/Job/Restriction states.
- **Business rule BR-A16**: Separate success, denial, failure, and pending (accepted, awaiting result; IR90). Mask secrets/contacts in before/after. Only Repository business events append records; audit UI offers no create/edit/delete. A browser demo cannot guarantee tamper prevention.
- **Resulting business state**: Read-only. Keep filters in URL, never sensitive body text.
- **Boundaries/prohibitions**: Correlation searches cover only authorized records; other-tenant and nonexistent IDs return the same successful empty set. Reject delete-equivalent calls and reversed periods. Role switching must not rewrite historical actors.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-A16-N | audit.read. When: Search period/correlationId → open details | ① Success/denial/failure shown separately ② before/after masked ③ URL has filters, not body |
| AT-A16-E | ① Other-tenant correlationId ② Delete-equivalent action ③ start≥end ④ Switch viewer Membership | ① Successful empty set (items=[],total=0,nextCursor=null), same as nonexistent ID ② No action ③ VALIDATION ④ Historical actor unchanged |
| AT-A16-B | ① Success ② Denial ③ Failure event ④ Historical actor viewed from another Membership | ①②③ Distinct results ④ actorRoleAtTime unchanged |

Design: [DD-A16](../02-design/admin.md#dd-a16-details). Assess parent AT-A16 using all N/E/B and applicable SRC/R01 cases in traceability.

0.9.0 correction contracts: Read [strict review correction contracts](../02-design/strict-review-contracts.md) and [operation version contracts](../02-design/write-version-catalog.csv) together.

Approval applied 2026-09-16: FR-A07/A09 reject contract edits during active restrictions and allow them after cancellation/release completes (SR19). FR-A15 retries only the failed stage of the same failed record with a new attempt; retirement failure retains the purchased reference (SR18).

Additional current 0.21.0 contracts: Read [re-review correction contracts](../02-design/review-resolution-contracts.md) IR01–106. They override older text on the same issues; use IR72 for conflict priority.

0.15.0: FR-A06 quality review uses IR29 completion times and IR31 self-approval prohibition for all contributors.

Job lists support ascending/descending sorting by status (business order), severity, and deadline. Default: status in business order (IR34). Sort all results before pagination; language changes do not change order. Also use AT-REV16-005 for acceptance.
