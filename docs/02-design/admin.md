---
document_id: DD-A
version: 0.21.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# Administrator and HQ Detailed Design

This design follows the company's original requests and the related requirements. It covers three areas.

- Available features.
- Fields shown on screens.
- Possible states and errors.

This document defines the processing needed for each FR (functional requirement) and acceptance criteria to test. Reference mock screens are used only to guide the shared UI appearance.

**Implementation baseline for 0.21.0**: Read all chapters of the [Deterministic Contracts](deterministic-contracts.md) and strict-review-contracts.md, the authorization columns of the operation catalog, and the screen catalog together. Do not guess values, permissions, asynchronous behavior, or recovery during implementation. These are demo design proposals, not approval for production business use.

## Inputs and Responsibilities

The primary source is the [Original Company Requirements (SRC-06)](../00-prepare/sources/company-requirements-original.txt). Screens, inputs, states, and acceptance criteria follow the requirements reorganized from this source.

Inputs: [Role Requirements](../01-requirements/admin.md), [Common Requirements](../01-requirements/common.md).

Required reading: [Common Detailed Design](common.md), [UIUX Specification](../03-uiux/UIUXSpecification.md).

The following designs frontend fields, displays, and mock behavior. Registration, assignment, payment receipt, restrictions, and audit on screens only change fictional demo data in shared mock memory. This does not request server implementation or database design.

Treat route parameters as untrusted input and always validate them. “Service name” means an operation in the shared Repository. Rows with the same route describe different functions on one screen. Implement these five states for every row.

- loading
- empty
- error
- forbidden
- not-found

Show retry only for recoverable errors. For forbidden and not-found, follow IR57 and do not show retry.

## Screen and Process Design

| Design ID / requirement | Route / main component | Read and action contracts | Input, processing, validation | Errors and prohibited actions |
|---|---|---|---|---|
| DD-A01 / FR-A01 | `/admin` / `AdminOverview` | `admin.summary` | Specify organization and period. Show both target and unknown unit counts with operating rates | Never automatically classify unmeasured units as inactive or normal |
| DD-A02 / FR-A02 | `/admin/units` / `AssetRegistry` | `organizations.list`、`organizations.save`、`customers.list`、`customers.save`、`properties.save`、`spaces.save`、`units.save`、`units.archive`、`units.list`、`units.get`、`properties.list`、`spaces.list`、`capabilities.list`、`units.delete`、`commands.create`、`commands.get`、`diagnosticRuns.list`、`diagnosticRuns.get` | Require a 1–120-character name. Enter parent ID, modelId, split type, and installation date. Derive tenantId from Session, not input (IR74) | Do not delete spaces in use; move related units first. Prefer archiving (taking out of use) to deletion |
| DD-A03 / FR-A03 | `/admin/settings/access` / `AccessManager` | `members.list`、`members.save`、`organizations.list` | Use identity.manage permission, membershipId, role, scope, and validFrom/Until. Separate HQ permissions by function | Demo policy forbids indefinite external assignments. Prevent users from granting themselves stronger permissions |
| DD-A04 / FR-A04 | `/admin/devices` / `DeviceRegistry` | `capabilities.list`、`capabilities.save`、`devices.list`、`devices.get`、`devices.register`、`devices.bind`、`devices.check`、`devices.calibrate`、`devices.updateFirmware`、`units.list`、`units.get`、`devices.calibrations`、`devices.operations` | Temperature requires min<=max and step>0. Specify allowed modes/fan levels and explicit ventilation support | Show the impact of capability changes incompatible with current Commands. Do not assume supported features |
| DD-A05 / FR-A05 | `/admin/alerts` / `AlertPolicyEditor` | `alerts.list`、`policies.save`、`notifications.preview`、`policies.list`、`policies.get`、`alerts.get`、`alerts.acknowledge`、`alerts.resolve`、`notifications.recipients`, units.list, units.get | Use matching units. Require upper/lower thresholds, positive duration, and recipients. Thresholds are fictional demo values | Marking a notification read does not resolve the Alert. External sending is preview only |
| DD-A06 / FR-A06 | `/admin/jobs` / `MaintenanceCoordinator` | `jobs.list`、`jobs.create`、`jobs.offer`、`jobs.assign`、`jobs.review`、`jobs.saveCost`、`jobs.hold`、`jobs.resumeHold`、`jobs.cancel`、`plans.save`、`plans.generateNext`、`jobs.get`、`reports.get`、`attachments.getContent`、`members.eligible`、`organizations.list`、`jobs.extendAccess`、`plans.list`、`plans.get`、`units.list` | Enter maintenance type, target, and deadline. Choose internal or outsourced work. Enter nonnegative costs with currency. Contractors accept before assigning their staff | Re-offer to another contractor after decline. Judge completion separately from alert resolution. Do not send real contractor payments |
| DD-A07 / FR-A07 | `/admin/billing/contracts` / `ContractEditor` | `contracts.list`、`contracts.save`、`customers.list`、`units.list` | Enter contract type, customerId, unitIds, period, and price. Store restriction eligibility per contract | Do not apply RTO remote-stop restrictions to general maintenance contracts. Changes affecting finalized invoices require new versions |
| DD-A08 / FR-A08 | `/admin/billing` / `BillingManager` | `invoices.list`、`invoices.create`、`payments.confirm`、`notifications.preview`、`inquiries.list`、`inquiries.answer`、`payments.recordManual`、`contracts.list`、`invoices.get`、`notifications.recipients`, invoices.remind | Use billing.manage permission, contract, amount, deadline, and payment reference. Manual confirmation requires a reason | Do not double-count a payment reference. Navigation alone must not confirm payment |
| DD-A09 / FR-A09 | `/admin/restrictions` / `RestrictionManager` | `restrictions.schedule`、`restrictions.execute`、`restrictions.release`、`commands.get`、`restrictions.list`、`restrictions.get`、`restrictions.retry`、`restrictions.reconcile`、`contracts.list`、`invoices.list`、`units.list`、`units.get` | Enter restriction.manage permission, contract, units, reason, notice deadline, and restriction details. Recheck conditions immediately before execution | Reject paid, grace, exception, or unsupported-unit cases. Allowed unit actions during restrictions follow IR46. Failure/expiry remains unapplied |
| DD-A10 / FR-A10 | `/admin/restrictions/:id` / `RestrictionException` | `restrictions.defer`、`restrictions.exempt`、`restrictions.cancel`、`restrictions.override`、`audit.list`、`restrictions.get`、`restrictions.retry`、`restrictions.reconcile`, restrictions.list | Use override permission, reason, and expiry. When cancellation overlaps an execution request, check state and switch to release if needed | Manual release does not rewrite payment state. History cannot be deleted |
| DD-A11 / FR-A11 | `/admin/settings/automation` / `ControlPolicy` | `policies.save`、`automations.simulate`、`automations.fire`、`policies.list`、`policies.get`、`units.list`、`units.get` | Enter units, priority, trigger event, action, and stop conditions. Prioritize contract restrictions and safety capabilities | Suppress automatic execution when data is unavailable. Send no real commands to external power equipment |
| DD-A12 / FR-A12 | `/admin/settings/air-quality` / `AirPolicy` | `policies.save`、`automations.simulate`、`automations.fire`、`telemetry.series`、`policies.list`、`policies.get`、`units.get`、`commands.get`、`notifications.recipients`、`units.list` | Fix ppm, µg/m³, °C, and % to their matching metrics | Do not guarantee health or safety. Fan-only capability must not issue ventilation Commands |
| DD-A13 / FR-A13 | `/admin/energy` / `EnergyAnalysis` | `energy.summary`、`baselines.list`、`baselines.save`、`units.list` | Enter baseline period, boundary, model version, and unit set. Validate period overlaps and missing data | Cannot calculate without a baseline. Do not guarantee reductions such as 10–20% or more |
| DD-A14 / FR-A14 | `/admin/mrv` / `MRVWorkspace` | `mrv.preview`、`mrv.saveDraft`、`mrv.recordReview`、`factors.list`、`factors.save`、`mrv.list`、`mrv.get`、`baselines.list`、`organizations.list`、`units.list`、`mrv.versions` | Require period, units, baseline version, factor version, and boundary. List evidence | Note missing data and estimates. Use “Demo review,” not “Externally verified” |
| DD-A15 / FR-A15 | `/admin/offsets` / `OffsetRegistry` | `offsets.preview`、`offsets.simulate`、`offsets.list`、`customers.list`、`units.list` | Quantity must be >0. Label scheme/provider “Not selected” and require the demo flag | Do not copy emission amounts into credit balances. No real trading or certificate issuance |
| DD-A16 / FR-A16 | `/admin/audit` / `AuditExplorer` | `audit.list`、`devices.events` | Use audit.read permission, period, actor, target, and event type. Mask confidential values | Show denied and successful actions separately. No deletion or changes through the screen. Demo records are not guaranteed tamper-proof |

## Shared Implementation Steps

1. Check the session and assigned scope; validate IDs and URL filters against schemas.
2. Call mock services through Queries and receive display data.
3. Use React Hook Form and shared schemas for forms, including capability and period validation.
4. Immediately before a change, check the target version, permissions, and current state. Confirm the target and reason before major actions.
5. Change shared demo state through the Repository and emit an event with a correlation ID. Invalidate related Queries and fetch the latest state.
6. Keep the display visible while awaiting a response. Show success, denial, and failure separately. Keep form inputs after submission failure.

## Handoff to Testing

For each DD-A number, check matching AT-A acceptance criteria, the error cases above, and unauthorized direct calls. The [Verification Plan](../04-agentic-sdlc/verification.md) is the source of truth for test data and cross-role scenarios. If examples such as character limits change, update schemas, documents, and boundary tests together.

## Detailed Feature Specifications (0.6.0)

Keep form values in RHF (React Hook Form) and validate them with schemas. Read-only screens need no form validation. Follow input/output contract DDC-03/09 for audit, notifications, and shared errors. Display read-only values from a single Query source. The [Implementation Contracts](implementation-contracts.md) define shared types, paging, time, and errors; the following adds individual conditions. Follow [UIUX](../03-uiux/UIUXSpecification.md) UX-04/08 tokens and patterns for appearance.

### DD-A01 Details

**Source mapping**: SRC-06 BIZ-04, BIZ-08 → FR-A01 → DD-A01. Source category: original company requirements SRC-06 + design additions. Design additions: overall summaries and navigation to responsible staff. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A01 / Main display pattern: **UI-OVERVIEW**. Service boundary: `admin.summary`.

**Initial view and prerequisites**: The HQ Membership may view summaries for the target tenant. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| customerId / propertyId | ID/optional | Within the managed tenant | Filter targets |
| customerCount | Read-only | Count where both Customer.status and Organization.status are active (IR40) | Customer count |
| from / to | datetime/required | From today/7d/30d/custom presets (SR17); maximum 366 days (IR74) | Period |
| counts / rates | Read-only | Numerator/denominator/unknownCount/asOf | Operating state |
| amountsByCurrency | Read-only array | Separate by currency | Unpaid amount |
| energySummary | Read-only | Actual period results and quality. Reduction fields are null (comparison in A13) | Energy-saving results |
| energyForecast | Read-only | IR78: forecast prorated from a demo_fixed baseline matching the target unit set. For null show “No target units / No baseline / Cannot calculate” | Expected reduction |

**Steps**

1. Filter by customer, site, and period. Check customer counts, units, operation, alerts, maintenance, billing, and energy. Open lists with the same conditions from KPI values.
2. Apply these business rules to reads and actions. Operating rate is the powerOn share of units with known current state. Always show total units and unknown units together. Customer count follows IR40 (Customers where both Customer and Organization are active). Determine overdue billing from unpaid amounts.
3. This screen is read-only. Keep periods, scope, and definitions consistent between the dashboard and lists opened from it.
4. Queries to update: `admin summary (on related events)`.

**Boundary cases and failures**: Do not silently include unknown units in the operating-rate denominator. Do not sum different currencies; display them separately. A rate with zero data is not calculable.

**Verification**: Check the traceability entries under AT-A01 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A02 Details

**Source mapping**: SRC-06 BIZ-07 → FR-A02 → DD-A02. Source category: original company requirements SRC-06 + design additions. Design additions: registration, editing, and archiving (taking out of use). Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A02 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `organizations.list, organizations.save, customers.list, customers.save, properties.save, spaces.save, units.save, units.archive, units.list, units.get, properties.list, spaces.list, capabilities.list, units.delete, commands.create, commands.get, diagnosticRuns.list, diagnosticRuns.get`.

**Initial view and prerequisites**: Permission to edit the managed organization register; customer/property/unit relationships can be traced correctly. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| organization.name / customer.name | string/required | 1–120 characters | Customer register |
| property.kind / name | enum and string/required | home/office, 1–120 characters | Property |
| space.parentSpaceId / kind | ID and enum | Same property; no parent-child cycles | Hierarchy |
| unit.modelId / spaceId | modelId: ID/required; spaceId: ID/null | Valid model. spaceId is in the same property; null means no assigned space (IR62) | Unit |
| unit.type / installedAt | enum and date/type required; installedAt nullable | split; no future completed installation dates; null means “Not registered” (IR44) | Type / installation date |
| serviceScope | enum array/required | Target inspection groups | Maintenance scope |
| changeReason | string/required for relocation and similar changes | 1–1000 characters | Change reason |
| property.address / accessInstructions | string/optional | 0–500 / 0–1000 characters; fictional values only | Address is siteAddress under IR25 even before acceptance of the company's Offer. Entry instructions are visible only after acceptance within the valid period |

**Steps**

1. Create the customer organization, then the property and space hierarchy. Register a split unit's model and location. Then search, edit, relocate, or archive.
2. Apply these business rules to reads and actions. On unit registration, check consistency of customerOrgId, spaceId, and modelId. Units linked to contracts, jobs, or IoT cannot be physically deleted. Moving units between tenants is outside phase 1A.
3. Keep immutable IDs, versions, before/after values, and change reasons. Relocation updates the current location and preserves the original in history.
4. Queries to update: `organizations / customers / properties / spaces / units / audit`。

**Boundary cases and failures**: Reject another customer's room, hierarchy cycles, nonexistent modelId, or deletion of units in use. Do not register new units for inactive customers.

**Verification**: Check the traceability entries under AT-A02 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A03 Details

**Source mapping**: SRC-06 BIZ-04 → FR-A03 → DD-A03. Source category: design additions supporting company goals. Design additions: permissions, memberships, and assignment periods. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A03 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `members.list, members.save, organizations.list`.

**Initial view and prerequisites**: identity.manage permission; the target's current role, scope, and valid period are fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| userId / organizationId | ID/required | Active user and their organization | Target |
| role | enum/required | client/contractor/technician/admin | Role |
| employment | enum/required for technicians | internal/external | Employment type |
| permissions | enum array/required | Choose from the role's allowed permissions | Capabilities |
| scopes | ScopeRef array/required | Role-specific types under SR03. Empty means zero business targets | Target scope |
| validFrom / validUntil | validFrom: datetime/required; validUntil: datetime/null | validUntil required only for external technicians (IR74) | Valid period |
| reason | string/required | 1–1000 characters | Change reason |

**Steps**

1. Select user and organization. Choose one of four roles; for technicians, also select internal/external. Set scope, period, and individual permissions. Confirm and save.
2. Apply these business rules to reads and actions. Keep role and permissions separate. Admin status does not automatically grant restriction.manage or override. External technicians must not have unit access without an end date. Increasing one's own permissions requires another HQ permission administrator to perform the change.
3. Update Membership version and scopeVersion. Discard caches from old sessions; evaluate subsequent mutations using new permissions.
4. Queries to update: `members / session scope / all affected query caches / audit`。

**Boundary cases and failures**: Reject other-tenant scopes, external access without an end date, validFrom at or after validUntil, and adding override to oneself. Reject saves from old screens after permissions expire.

**Verification**: Check the traceability entries under AT-A03 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A04 Details

**Source mapping**: SRC-06 BIZ-06, BIZ-20 → FR-A04 → DD-A04. Source category: original company requirements SRC-06 + design additions. Design additions: model capabilities and IoT register editing. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A04 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `capabilities.list, capabilities.save, devices.list, devices.get, devices.register, devices.bind, devices.check, devices.calibrate, devices.updateFirmware, units.list, units.get, devices.calibrations, devices.operations`.

**Initial view and prerequisites**: device.manage permission (IR74). Capability values are managed as a demo register. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| manufacturer / model | string/required | 1–120 characters each; unique combination | Model |
| control / ventilation | boolean/required | Default: false | Capability availability |
| modeControl / fanControl | boolean/required | Default: false. true requires nonempty modes/fanLevels; false requires empty lists (D14) | Mode/fan control |
| ventilationLevels | Fan array/required when ventilation=true | Nonempty, including low (D08); empty when false | Ventilation levels |
| min / max / step | number/required when temperature supported | min<=max, step>0, consistent | Temperature (°C) |
| modes / fanLevels | enum array/required when supported | No duplicates | Supported options |
| sensors | array/optional | metric/unit/staleAfterSeconds | Measurement capabilities |
| firmwareCandidates | version array/optional | Supported demo versions only | Firmware |
| changeReason | string/required on update | 1–1000 characters | Version change reason |

**Steps**

1. Register the model. Configure temperature, modes, fan levels, sensors, ventilation, and firmware candidates. Check affected units and save a new capability version.
2. Apply these business rules to reads and actions. Use false or unknown for unconfirmed capabilities. Do not infer modes from product category alone. Disable existing automation rules that conflict with new capabilities and show the reason.
3. Update Capability version and available unit actions. Keep unfinished Command contents in history; do not rewrite them to match new capabilities.
4. Queries to update: `capabilities / units / devices / automations / audit`。

**Boundary cases and failures**: Reject min>max, step<=0, or enabled mode control with no modes. Do not include unsupported firmware versions as candidates.

**Verification**: Check the traceability entries under AT-A04 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A05 Details

**Display and Processing for Load Alerts from Open Windows or Poor Insulation (BIZ-17)**

Add these three entries to Alert returned by `alerts.list`.

- causeCode: window_open/insulation_loss/unknown
- evidenceKind: demo_observation/inferred/inspection
- evidenceText (evidence description), observedAt (observation time)

causeCode and evidenceKind are required. Use unknown when evidence is unavailable; do not hard-code claims such as “Electricity cost doubles.” Label inferred content “Suspected” and inspected content “Inspection record.” Notification details link to the same unitId or maintenance request screen.

Verification: AT-A05-SRC. Use fixtures for suspected open window, inspection record of poor insulation, and no evidence. Check distinct wording, evidence, and times, and that marking read does not resolve an alert.

**Source mapping**: SRC-06 BIZ-08, BIZ-11, BIZ-17 → FR-A05 → DD-A05. Source category: original company requirements SRC-06 + design additions. Design additions: threshold settings and alert handling. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A05 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `alerts.list, policies.save, notifications.preview, policies.list, policies.get, alerts.get, alerts.acknowledge, alerts.resolve, notifications.recipients, units.list, units.get`.

**Initial view and prerequisites**: alert.policy.manage and recipient read permissions; metric units and target units are fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| unitIds / metric | ID array and enum/required | Within supported capabilities | Targets |
| operator / threshold | enum and number/required | gt/gte/lt/lte comparisons; finite number | Trigger condition |
| durationSeconds | integer/required | 1–86400 | Duration (seconds) |
| recoveryThreshold | number/required | Hysteresis matching comparison direction | Recovery threshold |
| severity | enum/required | warning/critical | Severity |
| recipientMembershipIds / channels | array/required | At least one each; inApp/email/whatsapp | Recipients / channels |
| escalateAfterMinutes / cooldownMinutes | integer/required | 1–1440 / 1–1440 | Escalation delay / duplicate suppression |
| name / unitIds | Required | 1–120 trimmed characters / within scope, nonempty, no duplicates | IR07 shared inputs |
| timezone / enabled / priority | Required | IANA name / boolean / integer 0–100. New UI shows Preferences.timezone / false / 50 | IR07 shared inputs |

**Steps**

1. Set target units, metric, comparison, and duration. Specify recipients, channels, and escalation delay. After saving, combine conditions to check trigger and recovery behavior.
2. Apply these business rules to reads and actions. Units are fixed by metric. Do not use missing or stale data to judge normal thresholds; treat them as connection/data-quality notices. Suppress repeats with cooldown. Record severity changes as new notification reasons.
3. Save Policy version. On trigger, create an Alert and Notification preview. Keep notification read status separate from Alert acknowledgement.
4. Queries to update: `policies / alerts / notifications / admin summary / audit`。

**Boundary cases and failures**: Reject zero recipients, zero duration, and recovery thresholds inconsistent with comparison direction. Check just-before, exact-threshold, and duration boundaries.

**Verification**: Check the traceability entries under AT-A05 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A06 Details

**Source mapping**: SRC-06 BIZ-12 → FR-A06 → DD-A06. Source category: original company requirements SRC-06 + design additions. Design additions: request intake, delegation, and quality review. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A06 / Main display pattern: **UI-LIST / UI-DETAIL / UI-FORM**. Service boundary: `jobs.list, jobs.create, jobs.offer, jobs.assign, jobs.review, jobs.saveCost, jobs.hold, jobs.resumeHold, jobs.cancel, plans.save, plans.generateNext, jobs.get, reports.get, attachments.getContent, members.eligible, organizations.list, jobs.extendAccess, plans.list, plans.get, units.list`.

**Initial view and prerequisites**: job.manage permission; target units and internal/contractor options are fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| unitId / type | Required | Target split unit; periodic/reactive/preventive | Request details |
| dueAt | datetime/optional | At least requestedEnd. HQ only; defaults to requestedEnd (IR38) | Deadline |
| deliveryMode | enum/required | internal/contractor | Delivery type |
| assigneeId / contractorOrgId | ID/conditionally required | Match delivery type | Contractor / assignee |
| recurrence | structure/optional for periodic work | monthly, interval 1–12 months, next date; each explicit action generates only the next occurrence and updates next date (D16) | Recurring plan |
| costLines | array/optional | kind=estimate/actual, amountMinor>=0, currency, description | Costs |
| reviewDecision / reason | Conditionally required | accept/return; hold and cancellation also require reasons. Cancellable states follow IR56 | Quality decision / exception reason |

**Steps**

1. Register maintenance type, unit, and deadline. Assign internally or offer to a contractor. Check schedule and progress, review quality, and record actual costs.
2. Apply these business rules to reads and actions. HQ confirms internal assignees and schedules. Contractors accept before assigning their own staff. Recurring plans show the next generation date; allow only one job per plan occurrence.
3. Link Job, Assignment, Offer, cost lines, and review history to the same jobId. Judge work completion separately from Alert resolution.
4. Queries to update: `jobs / plans / offers / assignments / costs / notifications / audit`。

**Boundary cases and failures**: Check re-offering after decline, overlapping confirmed schedules, overdue jobs, and returns after failed quality review. Summarize estimate and actual costs separately by currency, without conversion.

**Verification**: Check the traceability entries under AT-A06 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A07 Details

**Source mapping**: SRC-06 BIZ-21 → FR-A07 → DD-A07. Source category: original company requirements SRC-06 + design additions. Design additions: plan and contract editing. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A07 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `contracts.list, contracts.save, customers.list, units.list`.

**Initial view and prerequisites**: contract.manage permission; customer and linked units are in the same tenant. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| customerId / unitIds | Required | Customer's active units | Contract targets |
| planType | enum/required | rto (Rent to Own)/general/energy/environment | Plan type |
| startAt / endAt | datetime/required | Start before end | Period |
| priceMinor / currency | integer and enum/required | >=0; demo defaults to MYR | Price |
| restrictionEligible | boolean/required | Default: false; true allowed only for rto | Restriction eligibility |
| rulesVersion | ID/required if restrictions eligible | Demo version, not approved production rules | Applied conditions |

**Steps**

1. Set plan, period, price, and units. Specify RTO restriction eligibility and rule version. Confirm, then save or revise the contract.
2. Apply these business rules to reads and actions. General maintenance uses restrictionEligible=false. Even RTO is eligible only when explicitly specified. Contract revisions do not retroactively change issued invoices.
3. Keep contract versions. New invoices reference the new version; past invoices keep the original. Contract expiry does not mean a real device has stopped.
4. Queries to update: `contracts / customer payments / audit`。

**Boundary cases and failures**: Reject other customers' units, reversed periods, negative prices, and restrictions for general maintenance. Do not block monitoring or maintenance for units without contracts.

**Verification**: Check the traceability entries under AT-A07 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A08 Details

**Checking Card Types and Payment Instructions (BIZ-22)**

HQ views the simulated payment method chosen by the client in DD-C11 through invoice display data from `invoices.list`. paymentMethod is one of the following.

- demo_credit_card
- demo_debit_card
- null (not selected or manual payment)

paymentStatus shows the latest simulated Payment state. With no action or a manual payment, method=null. Viewing instructions does not change existing Invoice.paymentMethod. Keep the selected type during processing and after failure. Manual confirmation alone must not invent or overwrite method; show paymentReference and confirmation reason.

Use `notifications.preview` to check the invoice, channel, and selected payment method. HQ performs these actions.

- Confirm an existing Payment: `payments.confirm`.
- Manually record payment for an invoice with no Payment: `payments.recordManual`.

Do not place card-selection forms or customer payment actions such as `payments.simulate` on HQ screens. Viewing instructions or previews alone does not set paid.

Verification: AT-A08-SRC. Reproduce processing, success, and failure for each payment method on the client side, then check matching method and state on the same HQ invoice. Check that no action means unselected, failure keeps the selected type, and instruction previews leave the invoice unpaid.

**Source mapping**: SRC-06 BIZ-21, BIZ-22 → FR-A08 → DD-A08. Source category: original company requirements SRC-06 + design additions. Design additions: invoices and simulated payment confirmation. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A08 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `invoices.list, invoices.create, payments.confirm, notifications.preview, inquiries.list, inquiries.answer, payments.recordManual, contracts.list, invoices.get, notifications.recipients, invoices.remind`.

**Initial view and prerequisites**: billing.manage permission; contracts, invoices, and simulated payment targets can be matched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| contractId / period | Required | Billing period within contract period | Invoice details |
| amountMinor / currency | Required | Positive integer; matches contract currency | Amount |
| dueAt | datetime/required | After now only. Create overdue invoices through demoSeed or demo.advanceClock (IR90) | Deadline |
| paymentReference | string/required on confirmation | 1–128 characters; unique within tenant | Payment reference |
| confirmedAmountMinor | integer/required on confirmation | Matches full invoice amount | Confirmed amount |
| reason | string/required for manual confirmation | 1–1000 characters | Confirmation basis |
| channel | enum/required for reminder | email/whatsapp/inApp. Record simulation only after an explicit action following preview; no external send | Contact channel |
| inquiryId / reply | ID and string/required for inquiry response | Within managed scope; reply 1–2000 characters | In-app customer response |

**Steps**

1. Create invoices from a contract version. Filter by deadline or state. Check simulated payment results or have an authorized person confirm payment. Preview reminders, explicitly confirm invoices.remind, and check the saved customer-visible notification.
2. Apply these business rules to reads and actions. Phase 1A supports full payment only. Repeated confirmation of the same invoiceId/paymentReference returns the same result. Reminders target unpaid overdue invoices. Also show exceptions and disputes.
3. Record Payment confirmed, Invoice paid, and audit history. Once all causeInvoiceIds of a related Restriction are paid, move scheduled to cancelled and requested/applied to release_requested. If any remain unpaid, do not release; show pending device responses and remaining count.
4. Queries to update: `invoices / payments / restrictions / notifications / audit`。

**Boundary cases and failures**: Reject amount/currency mismatches, reusing a reference on another invoice, and rebilling paid invoices. Recheck and stop reminders attempted immediately after payment is confirmed.

**Verification**: Check the traceability entries under AT-A08 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A09 Details

**Source mapping**: SRC-06 BIZ-21 → FR-A09 → DD-A09. Source category: original company requirements SRC-06 + design additions. Design additions: advance notices, execution confirmation, and device acknowledgements. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A09 / Main display pattern: **UI-LIST / UI-DETAIL / UI-FORM**. Service boundary: `restrictions.schedule, restrictions.execute, restrictions.release, commands.get, restrictions.list, restrictions.get, restrictions.retry, restrictions.reconcile, contracts.list, invoices.list, units.list, units.get`.

**Initial view and prerequisites**: restriction.manage permission, an eligible RTO contract, unpaid status, and confirmed target-unit capabilities. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| contractId / causeInvoiceIds / unitIds | Required | Fix all overdue unpaid invoices of the same contract as causes. At most one active restriction per unit | Targets |
| policy.kind | enum/required | temperature_limit/power_off | Restriction type |
| policy.minimumCoolingSetpoint | number/required for temperature limit | Within device min/max/step. Lower temperature settings are prohibited | Cooling limit |
| executeAfter | datetime/required | At least accepted now+24 hours. Repository sets noticeAt to now; display read-only (IR05) | Notice schedule |
| reason / rulesVersion | string and ID/required | 1–1000 characters; demo version. Show reason unchanged on the customer's restriction explanation screen (IR42) | Basis |
| expectedVersion | integer/required on execution | Current version | Conflict check |

**Steps**

1. Enter notice reason, targets, details, and executeAfter. Before saving, show form confirmation; after schedule succeeds, check assigned noticeAt and saved notice. At execution, recheck invoices, grace, and exceptions. Request application per unit. After payment, track release requests and acknowledgements.
2. Apply these business rules to reads and actions. Reaching a deadline on screen does not automatically stop real equipment. Phase 1A simulates only explicitly confirmed HQ actions. Application requires success evidence for every unit; release requires released/not_required evidence for every unit (D03). Power-off and temperature limits are separate policies.
3. Create Restriction and per-unit Commands. When all causeInvoiceIds have confirmed payment, move scheduled to cancelled or requested/applied to release_requested. If any remain unpaid, keep the current state.
4. Queries to update: `restrictions / commands / units / customer billing / notifications / audit`。

**Boundary cases and failures**: If all cause invoices are paid immediately before execution, set cancelled and create no apply requests. Grace, exceptions, missing notice, and unsupported devices also prevent application. If some units are offline, do not set the whole restriction applied; show per-unit pending states. Failed release stays release_requested; late apply acknowledgements must not return it to applied.

**Verification**: Check the traceability entries under AT-A09 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A10 Details

**Source mapping**: SRC-06 BIZ-21 → FR-A10 → DD-A10. Source category: design additions supporting company goals. Design additions: grace periods, exceptions, and audit steps. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A10 / Main display pattern: **UI-DETAIL / UI-FORM / UI-TIMELINE**. Service boundary: `restrictions.defer, restrictions.exempt, restrictions.cancel, restrictions.override, audit.list, restrictions.get, restrictions.retry, restrictions.reconcile, restrictions.list`.

**Initial view and prerequisites**: Grace/exception actions require restriction.manage; manual release requires restriction.override. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| restrictionId | ID/required | Within managed scope | Target |
| action | enum/required | defer/exempt/cancel/override_release. Map defer→`restrictions.defer` (until required), exempt→`restrictions.exempt` (until required), cancel→`restrictions.cancel`, override_release→`restrictions.override` (restriction.override required). Every action requires reason | Exception action |
| until | ISO datetime/required for grace or exception | Future date/time | Expiry |
| reason | string/required | 1–1000 characters (IR87) | Exception basis |
| expectedVersion | integer/required | Refetched version | Conflict check |

**Steps**

1. Query current state and device application status. Choose grace, exception, cancellation, or manual release. Confirm reason, expiry, and impact, save, and track per-unit results.
2. Apply these business rules to reads and actions. Cancellation follows IR96: scheduled→cancelled; requested/applied→release_requested with releaseIntent.source=cancel; release_requested is idempotent; released/cancelled returns CONFLICT. Grace/exception expiry does not reapply automatically; conditions must be checked again.
3. Record before/after grace and exception details, expiry, release reason, and actor. Manual release does not clear unpaid Invoice balances.
4. Queries to update: `restrictions / commands / audit / notifications`。

**Boundary cases and failures**: Reject HQ users without override permission, empty reasons, and past grace dates. Cancellation from requested must not assume application is impossible.

**Verification**: Check the traceability entries under AT-A10 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A11 Details

**Source mapping**: SRC-06 BIZ-14, BIZ-16, BIZ-17 → FR-A11 → DD-A11. Source category: original company requirements SRC-06 + design additions. Design additions: condition settings and simulation. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A11 / Main display pattern: **UI-FORM**. Service boundary: `policies.save, automations.simulate, automations.fire, policies.list, policies.get, units.list, units.get`.

**Initial view and prerequisites**: automation.policy.manage permission; target units and control capabilities are fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| name / unitIds | Required | 1–120 trimmed characters; within scope, nonempty, no duplicates | Policy name / units |
| condition.type | enum/required | occupancy/tariff/peak/solar/battery | Condition type |
| condition (Condition type) | discriminated union/required | Tariff thresholds, time ranges, outputs, etc., with units | Evaluation values |
| action | UnitAction/required | Within capabilities and restrictions | Action |
| timezone / enabled / priority | Required | IANA name / boolean / integer 0–100 (higher wins). New UI: Preferences.timezone / false / 50 | IR07 shared inputs |

**Steps**

1. Select occupancy, tariff, peak, solar, or battery conditions. Set action and priority. Preview conflicts, then evaluate combined events.
2. Apply these business rules to reads and actions. Priority is capabilities/active restrictions → HQ policies → customer rules. Within a level, higher priority wins; ties use ascending ID order. Skip execution for unavailable or expired data and show the reason.
3. Save policy version. `automations.simulate` returns only selected/suppressed rules and reasons; it creates no Command. Trigger through `automations.fire` with DemoWriteOptions for duplicate prevention. It passes through shared Command policy and returns commandIds (DDC-08§6).
4. Queries to update: `policies / automations / simulation results / audit`。

**Boundary cases and failures**: Check HQ priority over customer rules, deterministic results for ties, and no automatic execution when solar data is unavailable.

**Verification**: Check the traceability entries under AT-A11 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A12 Details

**Air Quality Display and Processing, Including Allergens (BIZ-18)**

Add allergenObservation to air-quality display data returned by `telemetry.series`. availability is one of the following.

- available
- not_measured
- unsupported

Show substance, value, unit, observedAt, and sourceLabel only when data is available. available requires evidence and time; a numeric value also requires a unit. Treat incomplete information as unknown. Do not derive allergen amounts from PM2.5. Show CO₂ in ppm and electricity-related emissions separately in kgCO₂e. Without ventilation capability, show guidance only.

Verification: AT-A12-SRC. Switch among not-measured, unsupported, and synthetic-observation fixtures. Unmeasured data must not show 0 or “Safe.” A number without a unit must show unknown.

**Source mapping**: SRC-06 BIZ-18, BIZ-19 → FR-A12 → DD-A12. Source category: original company requirements SRC-06 + design additions. Design additions: ventilation rules and missing-data handling. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A12 / Main display pattern: **UI-FORM / UI-ANALYSIS**. Service boundary: `policies.save, automations.simulate, automations.fire, telemetry.series, policies.list, policies.get, units.get, commands.get, notifications.recipients, units.list`.

**Initial view and prerequisites**: automation.policy.manage permission (IR74); target metric and ventilation capability are defined for the device. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| unitIds / metric | Required | Metric exists in both each Unit's Capability.sensors and its currently bound Device.sensors (IR21/IR43) | Targets |
| threshold / recoveryThreshold | number/required | Unit fixed per metric | Trigger/recovery conditions |
| durationSeconds | integer/required | 1–86400 | Duration (seconds) |
| responseMode | enum/required | notify_only/notify_and_ventilate | Response mode |
| severity / channels | Required | warning/critical; nonempty channels. New: unselected | Notification settings SR28 |
| cooldownMinutes / escalateAfterMinutes | integer/required | 1–1440; new: not entered | Repeat/unacknowledged notices SR28 |
| recipientMembershipIds | ID array/required | At least one active recipient | Recipients |
| name / unitIds | Required | 1–120 trimmed characters / within scope, nonempty, no duplicates | IR07 shared inputs |
| timezone / enabled / priority | Required | IANA name / boolean / integer 0–100. New UI: Preferences.timezone / false / 50 | IR07 shared inputs |

**Steps**

1. Set metric, thresholds, duration, and recovery conditions. Choose notification only or also ventilation. Check capabilities and run a simulated evaluation.
2. Apply these business rules to reads and actions. Request automatic ventilation only for ventilation=true targets. Others receive notifications only. Show each target's action before saving.
3. Save environmental policy version and notification previews. `automations.fire` creates ventilate Commands only for ventilation=true targets (DDC-08§6); track device acknowledgements. Verify indoor-air improvement with later measurements.
4. Queries to update: `policies / alerts / commands / notifications / audit`。

**Boundary cases and failures**: Do not mix ppm and µg/m³ thresholds. Do not use unmeasured data to judge normality or recovery. Do not substitute fan Commands for unsupported ventilation.

**Verification**: Check the traceability entries under AT-A12 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A13 Details

**Source mapping**: SRC-06 BIZ-23, BIZ-25 → FR-A13 → DD-A13. Source category: original company requirements SRC-06 + design additions. Design additions: baseline versions and calculation conditions. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A13 / Main display pattern: **UI-ANALYSIS / UI-FORM**. Service boundary: `energy.summary, baselines.list, baselines.save, units.list`.

**Initial view and prerequisites**: energy.manage permission, period data within managed scope, and the ability to enter baseline-model evidence. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| unitIds / period | Required | Same comparison set; maximum 366 days | Targets |
| method | enum/required | demo_fixed/demo_period_comparison | Baseline method |
| baselineKWh | number/required for demo_fixed | >=0; label fictional. Do not enter for demo_period_comparison (SR29) | Baseline |
| boundaryId | enum/required | ac_input_electricity/whole_building_electricity (IR11) | Calculation boundary ID |
| boundary | string/required | 1–500 characters (IR11) | Boundary description |
| assumptions | string/required | 1–2000 characters | Assumptions |
| source | string/required | Demo source. Repository assigns version; do not enter it | Evidence |

**Steps**

1. Set baseline targets, period, and method. Check comparison conditions. Show differences from actual results, with detailed quality and evidence.
2. Apply these business rules to reads and actions. Keep baseline with unit set, boundary, period conditions, and model version. Do not label results adjusted when weather or other adjustment models do not exist. Do not clamp negative reductions (increases) to 0.
3. Saving a baseline creates a new version. Do not later change baseline versions referenced by existing MRV reports.
4. Queries to update: `baselines / energy / audit`。

**Boundary cases and failures**: Check zero baseline, missing actual data, different unit sets, and different boundaries. Example: baseline 100, actual 80 shows “Reduction 20.0 kWh / Reduction 20.0%.” Baseline 100, actual 120 has DTO values -20kWh/-20% and shows “Increase 20.0 kWh” / “Increase 20.0%” (IR68).

**Verification**: Check the traceability entries under AT-A13 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A14 Details

**Scope 2 Report Preview Display and Processing (BIZ-25)**

Add the following items to report-screen data.

- reportCategory (Scope 2 electricity demo summary label), organizationId, period, siteIds (target sites).
- gridRegion, factorValue, factorUnit, factorYear, factorVersion.
- boundaryDescription, coverageRatio.

These are display labels, not new DTO fields. Follow IR88's mapping for each source.

If factor, region, or boundary is missing, show “Calculation incomplete”; do not fill with 0. Show energy savings separately from electricity-related emissions. Keep “Demo — unverified” visible on the screen and saved-version previews.

Verification: AT-A14-SRC. With the same consumption, changing factor version changes the converted result and version. Missing factors show “Calculation incomplete.” Exclude sites outside the period or scope.

**Source mapping**: SRC-06 BIZ-25 → FR-A14 → DD-A14. Source category: original company requirements SRC-06 + design additions. Design additions: report fields, evidence, and previews. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A14 / Main display pattern: **UI-ANALYSIS / UI-FORM / UI-DETAIL**. Service boundary: `mrv.preview, mrv.saveDraft, mrv.recordReview, factors.list, factors.save, mrv.list, mrv.get, baselines.list, organizations.list, units.list, mrv.versions`.

**Initial view and prerequisites**: mrv.manage permission; period, units, baseline version, factor version, and boundary are selected. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| unitIds / from / to | Required | Within managed scope; start before end | Targets |
| baselineId / factorId / boundary | Required | Versioned, immutable references | Calculation conditions |
| factor.region / year / kgCO2ePerKWh / source | Required | Integer year; factor >=0; source explicitly marked demo | Factor |
| reviewComment | string/required on review | 1–1000 characters (IR87) | Demo review comment |
| reportVersion | integer/required on update | Current version | Conflict check |

**Steps**

1. Set calculation conditions. Preview measurements, quality, and results. Check evidence, save a draft, then display demo review history and report preview.
2. Apply these business rules to reads and actions. Factor region, year, and source are required (unit fixed to kgCO₂e/kWh, IR102). Review history is demo_reviewed, distinct from external certification. Input-version changes create new report versions and preserve old results.
3. Save factor/baseline snapshot references, calculation results, quality, and reviewHistory in MRVReport. Viewing a preview alone creates no finalized record.
4. Queries to update: `mrv / review history / audit`。

**Boundary cases and failures**: Check missing factors, coverage 0, and duplicate review of one report version. Unverified values must not appear certified.

**Verification**: Check the traceability entries under AT-A14 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A15 Details

**Display and Processing for Future Carbon-Market Integration (BIZ-26)**

Add marketConcept to display results from `offsets.preview`. Require these fields.

- stage=future_concept (future concept stage).
- providerLabel=Not selected.
- verificationStatus=unverified.
- ledgerStatus=not_connected.

Show energy savings, estimated emission reductions, and simulated purchase/retirement records separately. Provide no market prices, real token balances, or trade-execution buttons. Explain that future partners and verification conditions are undecided. The UI only shows asynchronous Repository results, allowing a future data adapter to be added later.

Verification: AT-A15-SRC. Without selecting an offset, no request is created. Opening the market-concept screen generates no balance, certificate, or trade result, and shows a state distinct from simulated retirement.

**Source mapping**: SRC-06 BIZ-24, BIZ-26 → FR-A15 → DD-A15. Source category: original company requirements SRC-06 + design additions. Design additions: simulated retirement and market-concept previews. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A15 / Main display pattern: **UI-LIST / UI-FORM / UI-DETAIL**. Service boundary: `offsets.preview, offsets.simulate, offsets.list, customers.list, units.list`.

**Initial view and prerequisites**: offset.manage permission; the screen states that transactions are simulated. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| customerId / purpose | Required | Within managed scope; 1–1000 characters | Applicant / purpose |
| amountKg | number/required | >0 and <=100000; at most 3 decimal places | Requested quantity |
| quoteId / version | Required after quote | Valid demo quote | Confirmation target |
| provider / scheme | Read-only | unselected/demo | Unselected label |
| demoConfirmed | boolean/required | Default: false | Confirm no real transaction |
| demoCertificateRef | Read-only | DEMO- prefix; show only after retirement | Simulated certificate |

**Steps**

1. Set quantity and purpose. Get a simulated quote, submit a simulated request, confirm demo purchase, retire in the demo, and preview proof information.
2. Apply these business rules to reads and actions. Purchase request, purchase confirmation, and retirement are separate events. Phase 1A supports only retiring a record's full quantity at once. Do not add calculated company emission reductions to purchased balances.
3. Keep quoted → demo_requested → demo_purchased → demo_retired in history. Prefix proof references with DEMO- and do not output them as real certificates.
4. Queries to update: `offsets / offset events / audit`。

**Boundary cases and failures**: Reject retirement before purchase, duplicate retirement, quantity 0, expired quotes, and other-tenant record actions. On failure, keep failed and the previous stage.

**Verification**: Check the traceability entries under AT-A15 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A16 Details

**Source mapping**: SRC-06 BIZ-20 → FR-A16 → DD-A16. Source category: original company requirements SRC-06 + design additions. Design additions: audit filters and correlation IDs. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A16 / Main display pattern: **UI-TIMELINE / UI-DETAIL**. Service boundary: `audit.list, devices.events`.

**Initial view and prerequisites**: audit.read permission; audit projections stay within the managed tenant and contain no confidential data. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| from / to | ISO datetime/required | Maximum 366 days | Search period |
| actorId / targetId / correlationId | ID/optional | Within managed scope | Filters |
| result | enum/optional | success/denied/failed/pending (accepted and awaiting result; append final result with the same correlationId, IR90) | Result |
| cursor / limit | string and integer | Default: 25; maximum: 100 | Paging |
| before / after / reason | Read-only | Confidential parts masked | Before/after values and reason |

**Steps**

1. Search by period, actor, target, result, and correlation ID. Open history details and compare related Command, Job, and Restriction states.
2. Apply these business rules to reads and actions. Show success, denied, failed, and pending separately. Mask secrets and contacts in before/after. Only Repository business events append records. This screen cannot add, edit, or delete them. A browser-only demo does not guarantee tamper-proof records.
3. This screen is read-only. Keep search conditions in the URL, without confidential text.
4. Queries to update: `none (audit read-only)`.

**Boundary cases and failures**: Search correlation IDs within the authorized set. Other-tenant and nonexistent correlation IDs return the same successful empty result. Reject delete-equivalent calls and reversed periods. Role switching must not change historical actors to different people.

**Verification**: Check the traceability entries under AT-A16 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

## HQ Paths for Shared Operations (FR-X04/X06)

Show CommandPanel at /admin/units?unitId=:id; users with control.execute use commands.create/get. Require a 1–1000-character reason. Acknowledgement/resolution at /admin/alerts?alertId=:id requires alert.resolve; registration, binding, connection checks, calibration, and FW updates at /admin/devices?deviceId=:id require device.manage. Share inputs and states with DD-C03/DD-T11 and deterministic contracts D01/D05. Without permission, allow target viewing only and show why actions are disabled.

DD-A08: On invoice selection, fetch invoices.get and select paymentId/version for confirmation from InvoiceDetail.paymentRefs. payments.confirm uses the Payment version; recordManual uses the Invoice version. Do not invent IDs when no Payment exists.

Convert condition forms to the Condition type's discriminated union. occupancy is {type,occupied}, location is {type,event}, pattern is {type,localTime}, weather is {type,metric:"temperature",operator,value}, tariff is {type,operator,value,unit:"MYR_per_kWh"}, peak is {type,active}, and solar/battery is {type,operator,value,unit:"kW"}. Do not send an extra params wrapper. Use weather_temperature for weather Fact.metric; do not confuse it with the room-temperature Fact temperature.

0.9.0 correction contracts: Read the [Strict Review Correction Contracts](strict-review-contracts.md) and [Per-Operation Version Contract](write-version-catalog.csv) together.

2026-09-16 approved updates: A07 disables saving contracts with active restrictions and shows the reason; Repository rechecks SR19. On failed, A15 calls offsets.simulate(event=retry,recordId,attemptId,demoConfirmed=true) with a new key and current version, then refetches the latest attempt.

0.10.0: A09 uses SR26 recoveryCases for inconsistent-observation recovery. A13 input is BaselineInput: enter baselineKWh only for demo_fixed; Repository computes values and quality for demo_period_comparison. Show quality and assumed-baseline labels together (SR29). A14 filters candidates through units.list(filters.organizationId).

A07 may save only when Contract.activeRestrictionIds is empty and hasUnresolvedRecovery=false. Resolving an A09 recovery case does not release a successor restriction. Device demo events use bindingId fetched from Device (SR24/SR26).

Additional contracts for current version 0.21.0: Read IR01–106 in the [Re-review Correction Contracts](review-resolution-contracts.md). They take priority over older text on the same topic; follow IR72 for conflict precedence.

A13/A14 distinguish IR11 boundaryId (fixed options) from boundary (description). MRV supports on-screen previews of saved versions; file export is outside scope (IR15).

0.15.0: DD-A06 acceptance follows IR29/IR31. Repository stores completion time and rejects self-approval by any contributor.

Apply IR34 to job-list and jobs.list sorting. When URL sort is absent, use status:asc. Changing the selection discards cursor, keeps filters, and fetches page one of a new snapshot. Allow ascending/descending sorting by state, severity, or deadline.
