---
document_id: DD-A
version: 0.30.0
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

**Implementation baseline for 0.22.0**: Read all chapters of the [Deterministic Contracts](deterministic-contracts.md) and strict-review-contracts.md, the authorization columns of the operation catalog, and the screen catalog together. Do not guess values, permissions, asynchronous behavior, or recovery during implementation. These are demo design proposals, not approval for production business use.

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
| DD-A01 / FR-A01 | `/admin` / `AdminOverview` | `admin.summary`, `customers.list`, `properties.list` | Specify organization and period. Show both target and unknown unit counts with operating rates | Never automatically classify unmeasured units as inactive or normal |
| DD-A02 / FR-A02 | `/admin/units` / `AssetRegistry` | `organizations.list`、`organizations.save`、`customers.list`、`customers.save`、`properties.save`、`spaces.save`、`units.save`、`units.archive`、`units.list`、`units.get`、`properties.list`、`spaces.list`、`capabilities.list`、`units.delete`、`commands.create`、`commands.get`、`diagnosticRuns.list`、`diagnosticRuns.get`、`properties.archive`、`spaces.archive`、`units.setAlertPolicies`、`policies.list` | Require a 1–120-character name. Enter parent ID, modelId, split type, and installation date. Derive tenantId from Session, not input (IR74) | Do not delete spaces in use; move related units first. Prefer archiving (taking out of use) to deletion |
| DD-A03 / FR-A03 | `/admin/settings/access` / `AccessManager` | `members.list`、`members.save`、`organizations.list` | Use identity.read/identity.write, membershipId, role (admin/contractor/technician only), scope, validFrom/Until, qualifications, and the 38-value permission matrix (IR107). Write implies Read | Demo policy forbids indefinite external assignments. Prevent users from granting themselves stronger permissions |
| DD-A04 / FR-A04 | `/admin/devices` / `DeviceRegistry` | `capabilities.list`、`capabilities.save`、`devices.list`、`devices.get`、`devices.register`、`devices.bind`、`devices.check`、`devices.calibrate`、`devices.updateFirmware`、`units.list`、`units.get`、`devices.calibrations`、`devices.operations`、`devices.events`、`audit.list` | Temperature requires min<=max and step>0. Specify allowed modes/fan levels and explicit ventilation support | Show the impact of capability changes incompatible with current Commands. Do not assume supported features |
| DD-A05 / FR-A05 | `/admin/alerts` / `AlertPolicyEditor` | `alerts.list`、`policies.save`、`notifications.preview`、`policies.list`、`policies.get`、`alerts.get`、`alerts.acknowledge`、`alerts.resolve`、`notifications.recipients`、`units.list`、`units.get`、`telemetry.series`、`policies.setDefaultRule`、`policies.delete`、`units.setAlertPolicies`、`customers.list`、`automations.simulate`、`automations.fire` | Policies belong to one customer (IR108); units attach them on unit edit. HQ default policy on every unit with per-customer rule on/off. Require upper/lower thresholds, positive duration, and recipients. Thresholds are fictional demo values | Marking a notification read does not resolve the Alert. External sending is preview only |
| DD-A06 / FR-A06 | `/admin/jobs` / `MaintenanceCoordinator` | `jobs.list`、`jobs.create`、`jobs.offer`、`jobs.assign`、`jobs.proposeSlot`、`jobs.withdrawProposal`、`jobs.resolvePartnerSlot`、`jobs.review`、`jobs.saveCost`、`jobs.hold`、`jobs.resumeHold`、`jobs.cancel`、`plans.save`、`plans.generateNext`、`jobs.get`、`reports.get`、`attachments.getContent`、`members.eligible`、`organizations.list`、`jobs.extendAccess`、`plans.list`、`plans.get`、`units.list`、`jobs.classifyFollowUp`、`jobs.events`、`customers.list`、`properties.list`、`members.list`、`units.get` | Enter maintenance type, target, and deadline. Choose internal or outsourced work. Enter nonnegative costs with currency. Contractors accept before assigning their staff | Re-offer to another contractor after decline. Judge completion separately from alert resolution. Do not send real contractor payments |
| DD-A07 / FR-A07 | `/admin/billing/contracts` / `ContractEditor` | `contracts.list`、`contracts.save`、`customers.list`、`units.list` | Enter contract type, customerId, unitIds, period, and price. Store restriction eligibility per contract | Do not apply RTO remote-stop restrictions to general maintenance contracts. Changes affecting finalized invoices require new versions |
| DD-A08 / FR-A08 | `/admin/billing` / `BillingManager` | `invoices.list`、`invoices.create`、`payments.confirm`、`notifications.preview`、`inquiries.list`、`inquiries.answer`、`payments.recordManual`、`contracts.list`、`invoices.get`、`notifications.recipients`, invoices.remind | Use billing.write permission, contract, amount, deadline, and payment reference. Manual confirmation requires a reason | Do not double-count a payment reference. Navigation alone must not confirm payment |
| DD-A09 / FR-A09 | `/admin/restrictions` / `RestrictionManager` | `restrictions.schedule`、`restrictions.execute`、`restrictions.release`、`commands.get`、`restrictions.list`、`restrictions.get`、`restrictions.retry`、`restrictions.reconcile`、`contracts.list`、`invoices.list`、`units.list`、`units.get` | Enter restriction.write permission, contract, units, reason, notice deadline, and restriction details. Recheck conditions immediately before execution | Reject paid, grace, exception, or unsupported-unit cases. Allowed unit actions during restrictions follow IR46. Failure/expiry remains unapplied |
| DD-A10 / FR-A10 | `/admin/restrictions/:id` / `RestrictionException` | `restrictions.defer`、`restrictions.exempt`、`restrictions.cancel`、`restrictions.override`、`audit.list`、`restrictions.get`、`restrictions.retry`、`restrictions.reconcile`, restrictions.list | Use override permission, reason, and expiry. When cancellation overlaps an execution request, check state and switch to release if needed | Manual release does not rewrite payment state. History cannot be deleted |
| DD-A11 / FR-A11 | `/admin/settings/automation` / `ControlPolicy` | `policies.save`、`automations.simulate`、`automations.fire`、`policies.list`、`policies.get`、`units.list`、`units.get` | Enter units, priority, trigger event, action, and stop conditions. Prioritize contract restrictions and safety capabilities | Suppress automatic execution when data is unavailable. Send no real commands to external power equipment |
| DD-A12 / FR-A12 | `/admin/alerts?tab=policies` (former `/admin/settings/air-quality` removed) / `AlertPolicyEditor` | `policies.save`、`policies.list`、`policies.get`、`automations.simulate`、`automations.fire`、`telemetry.series`、`notifications.recipients`、`units.list` | CO₂ (ppm) and PM2.5 (µg/m³) limits are customer-owned alert policies (IR108); fix ppm, µg/m³, °C, and % to their matching metrics | Do not guarantee health or safety. Air-quality policies notify only and never create Commands |
| DD-A13 / FR-A13 | `/admin/energy` / `EnergyAnalysis` | `energy.summary`、`baselines.list`、`baselines.save`、`units.list` | Enter baseline period, boundary, model version, and unit set. Validate period overlaps and missing data | Cannot calculate without a baseline. Do not guarantee reductions such as 10–20% or more |
| DD-A14 / FR-A14 | `/admin/mrv` / `MRVWorkspace` | `mrv.preview`、`mrv.saveDraft`、`mrv.recordReview`、`factors.list`、`factors.save`、`mrv.list`、`mrv.get`、`baselines.list`、`organizations.list`、`units.list`、`mrv.versions` | Require period, units, baseline version, factor version, and boundary. List evidence | Note missing data and estimates. Use “Demo review,” not “Externally verified” |
| DD-A15 / FR-A15 | `/admin/offsets` / `OffsetRegistry` | `offsets.preview`、`offsets.simulate`、`offsets.list`、`customers.list`、`units.list` | Quantity must be >0. Label scheme/provider “Not selected” and require the demo flag | Do not copy emission amounts into credit balances. No real trading or certificate issuance |
| DD-A16 / FR-A16 | `/admin/audit` / `AuditExplorer` | `audit.list`、`devices.events` | Use audit.read permission, period, actor, target, and event type. Mask confidential values | Show denied and successful actions separately. No deletion or changes through the screen. Demo records are not guaranteed tamper-proof |
| DD-A17 / FR-A17 | `/admin/units?customerId=&tab=users` / `ClientUserList` | `clientUsers.list`、`clientUsers.save`、`clientUsers.remove`、`clientUsers.resendInvite` | Email unique per customer; role owner/member; reason for removal | Last active owner cannot be demoted, disabled, or removed |
| DD-A18 / FR-A18 | `/admin/units` (Import CSV) / `UnitImportWizard` | `units.importPreview`、`units.importCommit`、`units.importUndo`、`customers.list` | UTF-8 CSV ≤ 1000 rows; 8 mapped columns; nothing written before import | Error rows skipped; undo only 24 h and before telemetry/jobs |
| DD-A19 / FR-A19 | `/admin/units?tab=warranty` / `WarrantyCoverage` | `units.coverage`、`jobs.recordWarrantyClaim`、`contracts.list` | Coverage = warranty or active contract; claim amount > 0 | Claims only for parts replaced under warranty |
| DD-A20 / FR-A20 | `/admin/devices?tab=firmware` / `FirmwareCampaigns` | `firmwareCampaigns.list`、`firmwareCampaigns.get`、`firmwareCampaigns.schedule`、`firmwareCampaigns.control`、`devices.list` | Signed version; start ≥ 24 h ahead; waves end at 100 %; auto-pause 1–50 % | Busy/offline/tampered devices skipped; failed devices keep the old version |
| DD-A21 / FR-A21 | `/admin/jobs?tab=contractors` / `ContractorRegister` | `contractors.list`、`contractors.save`、`contractors.setOfferStatus`、`rateCards.list`、`rateCards.save`、`certificates.list`、`certificates.verify` | Rate card from a future date; suspension reason 1–1000 | Suspension blocks new offers only |
| DD-A22 / FR-A22 | `/admin/jobs?tab=sla` / `SlaScorecard` | `sla.scorecard`、`sla.saveTargets` | Percentages 0–100; response 1–168 h; targets per plan | New targets apply only to new jobs |
| DD-A23 / FR-A23 | `/admin/billing?tab=payouts` / `ContractorPayouts` | `payouts.list`、`payouts.get`、`payouts.generate`、`payouts.transition`、`payouts.resolveQuery`、`rateCards.list` | billing.payment for every write; mark paid on or after the pay date | Approved/paid statements are never regenerated |

## Shared Implementation Steps

1. Check the session and assigned scope; validate IDs and URL filters against schemas.
2. Call mock services through Queries and receive display data.
3. Forms use controlled inputs with a pure validator per form in `shared/lib`, including capability and period validation; the Core API checks again (DEC-03, decided 2026-10-10).
4. Immediately before a change, check the target version, permissions, and current state. Confirm the target and reason before major actions.
5. Change shared demo state through the Repository and emit an event with a correlation ID. Invalidate related Queries and fetch the latest state.
6. Keep the display visible while awaiting a response. Show success, denial, and failure separately. Keep form inputs after submission failure.

## Handoff to Testing

For each DD-A number, check matching AT-A acceptance criteria, the error cases above, and unauthorized direct calls. The [Verification Plan](../04-agentic-sdlc/verification.md) is the source of truth for test data and cross-role scenarios. If examples such as character limits change, update schemas, documents, and boundary tests together.

## Detailed Feature Specifications (0.6.0)

Keep form values in the form component and validate them with its pure validator (DEC-03). Read-only screens need no form validation. Follow input/output contract DDC-03/09 for audit, notifications, and shared errors. Display read-only values from a single Query source. The [Implementation Contracts](implementation-contracts.md) define shared types, paging, time, and errors; the following adds individual conditions. Follow [UIUX](../03-uiux/UIUXSpecification.md) UX-04/08 tokens and patterns for appearance.

### DD-A01 Details

**Source mapping**: SRC-06 BIZ-04, BIZ-08 → FR-A01 → DD-A01. Source category: original company requirements SRC-06 + design additions. Design additions: overall summaries and navigation to responsible staff. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A01 / Main display pattern: **UI-OVERVIEW**. Service boundary: `admin.summary, customers.list, properties.list`.

**Initial view and prerequisites**: The HQ Membership may view summaries for the target tenant. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| customerId / propertyId | ID/optional | Within the managed tenant | Filter targets |
| customerCount | Read-only | Count where both Customer.status and Organization.status are active (IR40) | Customer count |
| from / to | datetime/required | From today/7d/30d/custom presets (SR17); maximum 366 days (IR74) | Period |
| counts / rates | Read-only | Numerator/denominator/unknownCount/asOf | Operating state |
| total / powerOn / powerOff / powerUnknown | Read-only | total=powerOn+powerOff+powerUnknown; archived units excluded (SR27, IR39) | Power-state axis |
| online / offline / unknown | Read-only | unknown = connection unknown/connecting/error; separate axis from powerUnknown (SR27) | Connection axis |
| alertCount | Read-only | open/acknowledged critical/warning only (IR51) | Unresolved alerts |
| jobCounts | Read-only Record<JobStatus, number> | All ten statuses in the period, including zeros | Maintenance by status |
| overdueInvoiceCount / amountsByCurrency | Read-only / array or null | Separate by currency; null with billingVisibility=forbidden (D-KPI) | Unpaid amount |
| energySummary | Read-only | Actual period results and quality. Reduction fields are null (comparison in A13) | Energy-saving results |
| energyForecast | Read-only | IR78: forecast prorated from a demo_fixed baseline matching the target unit set. For null show “No target units / No baseline / Cannot calculate” | Expected reduction |

**Steps**

1. Filter by customer, site, and period. Check customer counts, units, operation, alerts, maintenance, billing, and energy. Open lists with the same conditions from KPI values.
2. Apply these business rules to reads and actions. Operating rate is the powerOn share of units with known current state. Always show total units and unknown units together. Customer count follows IR40 (Customers where both Customer and Organization are active). Determine overdue billing from unpaid amounts.
3. This screen is read-only. Keep periods, scope, and definitions consistent between the dashboard and lists opened from it.
4. Queries to update: `admin summary (on related events)`.
5. Layout (top to bottom), all from the single `admin.summary` result (the filter bar's Customer and Property options come from `customers.list` and `properties.list`, hidden without asset.read, IR244); no other Query is called, so the screen has no recent-alert list, job list, per-customer table, hourly chart, restriction count, or CSV export. (a) Filter bar: Customer, Property, Period (Today / Last 7 days / Last 30 days / Custom, SR17), the `asOf` time with display timezone and period range, and an explicit Refresh. (b) Eight KpiCards: active customers, target units (ON/OFF/unknown under the value), operation rate (ON / known, with the unknown count), unresolved alerts, energy used (energySummary kWh, cost, coverage), overdue billing (amount per currency and overdueInvoiceCount), maintenance jobs in the period, and connection (online, with offline and unknown/connecting/error). (c) Energy-saving forecast card (step 6). (d) Power-state card and connection card, each with a stacked bar and one row per class. (e) Jobs-by-status card listing all `jobCounts` keys (each status opens the Jobs tab stage with the scope and the period as `from` / `to`, IR245) and a billing card listing one row per currency.
6. Forecast card (IR78): label “Forecast (prorated assumed baseline, demo)”; forecastSavedKWh and forecastSavingPercentage using “Expected reduction / Expected increase / No change 0.0” with absolute values; predictedBaselineKWh with baselineRef, method, and baseline kWh; predictedActualKWh with actualKWhOnValidSlots; coverage as validUnitMinutes / expectedUnitMinutes; and qualityWarnings as returned, one badge each. Null values show “No target equipment” (no_units), “Baseline not set” (baseline_unavailable), or “Cannot calculate”. State that it is a forecast, not a measured saving; actual results stay in the energy-used KPI.
7. Drill-down links (IR50, SR06): units and operation → `/admin/units?customerId&propertyId&powerState=on|off|unknown`; connection → `connections=online|offline` or `connections=connecting,error,unknown`; alerts → `/admin/alerts`; overdue billing → `/admin/billing?overdueOnly=true`. These current-state links carry the customer/property filters but not the period. Jobs → `/admin/jobs` with `statuses` and the period. Back restores this URL including the period (D13).
8. Permission-scoped sections (IR115): without billing.read, `billingVisibility=forbidden` and `amountsByCurrency=null`; the billing KPI and card show “Not permitted”, never 0, and the billing link is hidden. The forecast is shown to every dashboard.read holder, but the `/admin/energy` link appears only for energy.read holders (the Energy analysis screen itself needs energy.read; baselines need energy.write).
9. States: initial/loading shows a skeleton only on first load or when the URL conditions change; refetch keeps values with an “Updating” indicator and aria-busy (IR83); a failed refetch keeps values and adds a stale banner with last success time and Retry; an initial failure shows an error message and Retry with no KPI values; zero target units is a successful empty result (counts 0, rates and energy “Cannot calculate”, forecast “No target equipment”); in the first minute of today (from=to) show “No completed measurement interval” and do not call `admin.summary` (SR17). Wireframes: Figma “Admin — Wireframes”, row “Overview — /admin (FR-A01)”.

**Boundary cases and failures**: Do not silently include unknown units in the operating-rate denominator. Do not sum different currencies; display them separately. A rate with zero data is not calculable.

**Verification**: Check the traceability entries under AT-A01 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A02 Details

**Source mapping**: SRC-06 BIZ-07 → FR-A02 → DD-A02. Source category: original company requirements SRC-06 + design additions. Design additions: registration, editing, and archiving (taking out of use). Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A02 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `organizations.list, organizations.save, customers.list, customers.save, properties.save, spaces.save, units.save, units.archive, units.list, units.get, properties.list, spaces.list, capabilities.list, units.delete, commands.create, commands.get, diagnosticRuns.list, diagnosticRuns.get, properties.archive, spaces.archive, units.setAlertPolicies, policies.list`.

**Initial view and prerequisites**: Permission to edit the managed organization register; customer/property/unit relationships can be traced correctly. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| organization.name / customer.name | string/required | 1–120 characters | Customer register |
| property.kind / name | enum and string/required | home/office, 1–120 characters | Property |
| space.parentSpaceId / kind | ID and enum | Same property; no parent-child cycles | Hierarchy |
| unit.modelId / spaceId | modelId: ID/required; spaceId: ID/null | Valid model. spaceId is in the same property; null means no assigned space (IR62) | Unit |
| unit.type / installedAt | enum and date/type required; installedAt nullable | split; no future completed installation dates; null means “Not registered” (IR44) | Type / installation date |
| unit.warrantyEndsAt | instant/optional | null = no warranty; not before installedAt; omitted on save keeps the stored end (IR209) | Warranty & coverage (DD-A19) |
| serviceScope | enum array/required | Target inspection groups | Maintenance scope |
| changeReason | string/required for relocation and similar changes | 1–1000 characters | Change reason |
| property.address / accessInstructions | string/optional | 0–500 / 0–1000 characters; fictional values only | Address is siteAddress under IR25 even before acceptance of the company's Offer. Entry instructions are visible only after acceptance within the valid period |

**Steps**

1. Create the customer organization, then the property and space hierarchy. Register a split unit's model and location. Then search, edit, relocate, or archive.
2. Apply these business rules to reads and actions. On unit registration, check consistency of customerOrgId, spaceId, and modelId. Units linked to contracts, jobs, or IoT cannot be physically deleted. Moving units between tenants is outside phase 1A.
3. Keep immutable IDs, versions, before/after values, and change reasons. Relocation updates the current location and preserves the original in history.
4. Queries to update: `organizations / customers / properties / spaces / units / audit`。

**Screens (Figma Admin 02-1…02-16, IR111)**: (1) Customer list `/admin/units`: search (name, ID, property), Status/Contract filters, KPI tiles (customers, properties, units, needs attention), rows with status, operation, alerts, contract badges; inactive customers only with Status: All (IR40); “+ New customer” modal (`customers.save`). (2) Customer header (KPIs: units, operation, open alerts, overdue billing, restriction link) with tabs Units & locations, Users (DD-A17), Alert policies (customer-filtered policy list linking to SCR-A05), Warranty & coverage (DD-A19). (3) Location tree with “+ Add property/floor/area/room” modals (`properties.save`/`spaces.save`), ⋯ Rename and Delete on every location (`properties.archive`/`spaces.archive`; disabled with CONFLICT while it still contains locations or units). (4) Selecting a room lists its units; unit rows open unit edit `?unitId=`. (5) Unit edit: name, location select (saving asks a change reason in a modal), model, installed at, IoT binding, service scope, “Alert policies on this unit” card (default policy always attached; customer policies with Edit → SCR-A05 and Detach; “+ Attach policy” modal lists only that customer’s policies → `units.setAlertPolicies`), links to command panel, diagnostic runs, device events, audit; Delete unit (CONFLICT while in use). Archived units open read-only (IR39). URL keys customerId, locationId (property or space), unitId, tab, powerState, connections, search.

**Boundary cases and failures**: Reject another customer's room, hierarchy cycles, nonexistent modelId, or deletion of units in use. Do not register new units for inactive customers. Names are unique among non-archived siblings after trimming, ignoring case (a property per customer, a space per parent, a unit per space): CONFLICT `error.duplicateSiblingName`; a second customer organization with the same billing name is CONFLICT `error.duplicateName` (IR208). “Delete unit” takes the unit out of use (`units.archive`, ID and history kept); `units.delete` only removes a unit registered by mistake that never had contracts, jobs or IoT. Archiving (`properties.archive`, `spaces.archive`, `units.archive`) takes a reason of 1–1000 characters; archiving a property or space that still holds non-archived units or child spaces is CONFLICT, and archived properties and spaces cannot receive new spaces or units (NOT_FOUND).

**Verification**: Check the traceability entries under AT-A02 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A03 Details

**Source mapping**: SRC-06 BIZ-04 → FR-A03 → DD-A03. Source category: design additions supporting company goals. Design additions: permissions, memberships, and assignment periods. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A03 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `members.list, members.save, organizations.list`.

**Initial view and prerequisites**: identity.write permission; the target's current role, scope, and valid period are fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

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
2. Apply these business rules to reads and actions. Keep role and permissions separate. Admin status does not automatically grant restriction.write or override. External technicians must not have unit access without an end date. Increasing one's own permissions requires another HQ permission administrator to perform the change.
3. Update Membership version and scopeVersion. Discard caches from old sessions; evaluate subsequent mutations using new permissions.
4. Queries to update: `members / session scope / all affected query caches / audit`。
5. Language and time (IR302): texts, roles and matrix resources follow the display language; permission codes stay codes. The valid period is typed and shown in the display time zone, and the form names it. A revoke ends the access at now.

**Boundary cases and failures**: Reject other-tenant scopes, external access without an end date, validFrom at or after validUntil, and adding override to oneself. Reject saves from old screens after permissions expire.

**Permission matrix (IR107, Figma Admin 03-1…03-7)**: Rows per resource with READ / WRITE / ACTIONS columns (see FR-A03 BR-A03); a header shows “Permissions · n of 38”. Turning Write on locks Read on. The role select only pre-checks defaults. The organization filter and “+ Add scope” use search-selects (first 20, type to search). Client memberships are excluded from `members.list` results on this screen (role filter Admin/Contractor/Technician) and from the role options. Revoke access opens a centered modal (valid until = now, reason). Granting `identity.write` or `restriction.override` to one’s own membership is rejected (FORBIDDEN) with Save disabled.

**Verification**: Check the traceability entries under AT-A03 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A04 Details

**Source mapping**: SRC-06 BIZ-06, BIZ-20 → FR-A04 → DD-A04. Source category: original company requirements SRC-06 + design additions. Design additions: model capabilities and IoT register editing. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A04 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `capabilities.list, capabilities.save, devices.list, devices.get, devices.register, devices.bind, devices.check, devices.calibrate, devices.updateFirmware, units.list, units.get, devices.calibrations, devices.operations, devices.events, audit.list`.

**Initial view and prerequisites**: device.write permission (IR74). Capability values are managed as a demo register. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

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
5. Tabs: `tab=models` (capability list and editor), `tab=firmware` (firmware campaigns, DD-A20) and `tab=devices` (device list; `deviceId` opens the detail with binding, sensors, operations, and calibrations).
6. Version history (capability): shown only to holders of audit.read. On explicit open, call `audit.list` with `targetId=<capabilityId>` and list each saved version as previousVersion→nextVersion, actor, time, reason, and maskedBefore/maskedAfter. No separate capability-version read operation exists; the current version comes from `capabilities.list`.
7. Device events: for the selected `deviceId`, call `devices.events` (communication_lost / power_lost / tamper / restored / operation_failed with recovery) as a secondary Query after `devices.get` succeeds.

**Boundary cases and failures**: Reject min>max, step<=0, or enabled mode control with no modes. Do not include unsupported firmware versions as candidates. Capability rules: manufacturer + model unique (case-insensitive, CONFLICT); temperature min < max and 0 < step ≤ max − min; modes, fan levels and ventilation levels are unique values of their enums; modeControl, fanControl or a temperature range require control = true, and modeControl / fanControl need at least one mode / fan level; ventilation levels only when ventilation = true; one sensor per metric, with the metric's fixed unit (temperature °C, humidity %, co2 ppm, pm25 µg/m³, power kW, vibration mm/s, refrigerant_pressure kPa, compressor_cycles cycles/h, airflow_drop %, heartbeat_gap min), staleAfterSeconds 10–86400, and boundaryId only on power. Saving an existing model writes the next version, marks the previous one not current, and moves every unit of the model to the new capability version.

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

Scope: FR-A05 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `alerts.list, policies.save, notifications.preview, policies.list, policies.get, alerts.get, alerts.acknowledge, alerts.resolve, notifications.recipients, units.list, units.get, telemetry.series, policies.setDefaultRule, policies.delete, units.setAlertPolicies, customers.list, automations.simulate, automations.fire`.

**Initial view and prerequisites**: alert.policy.write and recipient read permissions; metric units and target units are fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| customerId / metric | ID and enum/required | Owner customer (fixed after creation); metric within supported capabilities | Owner and metric |
| operator / threshold | enum and number/required | gt/gte/lt/lte comparisons; finite number | Trigger condition |
| durationSeconds | integer/required | 1–86400 | Duration (seconds) |
| recoveryThreshold | number/required | Hysteresis matching comparison direction | Recovery threshold |
| severity | enum/required | normal (“Info”)/warning/critical | Severity |
| activeWindow | object/optional | weekdays + startLocal/endLocal in the policy timezone | Only if … |
| recipientMembershipIds / channels | array/required | 1–20 recipients who can read the customer's alerts; 1–3 channels including inApp (IR120) | Recipients / channels |
| escalateAfterMinutes / cooldownMinutes | integer/required | 1–1440 / 1–1440 | Escalation delay / duplicate suppression |
| name | Required | 1–120 trimmed characters | IR07 shared inputs (alert policies carry no unitIds input, IR108) |
| timezone / enabled / priority | Required | IANA name / boolean / integer 0–100. New UI shows Preferences.timezone / false / 50 | IR07 shared inputs |

**Steps**

1. Choose the owner customer, metric, comparison, and duration. Specify recipients, channels, and escalation delay. After saving, combine conditions to check trigger and recovery behavior.
2. Apply these business rules to reads and actions. Units are fixed by metric. Do not use missing or stale data to judge normal thresholds; treat them as connection/data-quality notices. Suppress repeats with cooldown. Record severity changes as new notification reasons.
3. Save Policy version. On trigger, create an Alert and Notification preview. Keep notification read status separate from Alert acknowledgement.
4. Queries to update: `policies / alerts / notifications / admin summary / audit`。
5. Tabs: `tab=alerts` (default; alert list with status/severity/cause/unit filters and the `alertId` detail) and `tab=policies` (policy list and the `policyId` editor). The detail shows cause and evidence using the BIZ-17 wording above, the target unit, and notification activity; reading a notification never changes the Alert.
6. Resolve (IR66): policy-free Alerts resolve only manually by an alert.resolve holder with a 1–1000 character reason and at least one evidence ID. Evidence candidates are the Alert's own `evidenceIds` (from `alerts.get`) and remeasurements from `telemetry.series` for the same unit and metric observed after `detectedAt`, fetched only when the Resolve dialog opens.
7. Policy editor order: basics (name, priority, timezone) → targets and metric → condition and recovery → severity → recipients/channels, cooldown, escalation → `notifications.preview` per recipient → demo test (synthetic reading and clock advance, simulator off) → save.
8. Scope entry (IR108): the Policies tab filters by `customerId` → `propertyId` → `unitId` (URL keys, search-selects with the first 20 options and server search; `policies.list` filters, customer candidates from `customers.list`, unit candidates from `units.list`). The list is grouped “Default · on every unit” first, then one group per customer (owner). A unitId filter (from unit edit) lists only the policies attached to that unit. A new policy first asks for the owner customer; the owner is fixed after creation. The editor’s “Owner & units” section shows the attached units read-only with a link to each unit edit — units are never assigned from this screen.
9. Default policy (`kind=default_alert`, policy-default): rule list (6 rules, IR120), the selected rule’s condition editor (HQ template; saving affects all units), and “On / off per customer — this rule” with a customer search-select; toggles call `policies.setDefaultRule` with a reason. “Copy as a <customer> policy →” opens an unsaved alert policy prefilled from the rule. The default policy cannot be deleted or detached.
10. Air-quality limits (FR-A12/DD-A12) are alert policies with metric co2 (ppm) or pm25 (µg/m³). Metric choices: temperature, humidity, CO₂, PM2.5, refrigerant pressure, vibration, power. “Only if …” sets activeWindow (weekdays and local hours in the policy timezone). Delete policy (`policies.delete`) detaches from all units after confirmation.
11. Alerts tab actions: Acknowledge, Resolve (step 6), Request maintenance (opens New job in SCR-A06 prefilled with unit and alert), Open unit → (SCR-A02 unit edit).

**Boundary cases and failures**: Reject zero recipients, zero duration, and recovery thresholds inconsistent with comparison direction. Check just-before, exact-threshold, and duration boundaries.

**Verification**: Check the traceability entries under AT-A05 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A06 Details

**Source mapping**: SRC-06 BIZ-12 → FR-A06 → DD-A06. Source category: original company requirements SRC-06 + design additions. Design additions: request intake, delegation, and quality review. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A06 / Main display pattern: **UI-LIST / UI-DETAIL / UI-FORM**. Service boundary: `jobs.list, jobs.create, jobs.offer, jobs.assign, jobs.proposeSlot, jobs.withdrawProposal, jobs.resolvePartnerSlot, jobs.review, jobs.saveCost, jobs.hold, jobs.resumeHold, jobs.cancel, plans.save, plans.generateNext, jobs.get, reports.get, attachments.getContent, members.eligible, organizations.list, jobs.extendAccess, plans.list, plans.get, units.list, jobs.classifyFollowUp, jobs.events, customers.list, properties.list, members.list, units.get`.

**Initial view and prerequisites**: job.write permission; target units and internal/contractor options are fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| unitId / type | Required | Target split unit; periodic/reactive/preventive | Request details |
| dueAt | datetime/optional | At least requestedEnd. HQ only; defaults to requestedEnd (IR38) | Deadline |
| deliveryMode | enum/required | internal/contractor | Delivery type |
| bookedSlot | Slot/required for assign/offer | An agreed slot only: one of the client's preferred times, an accepted proposal, or the plan occurrence (IR113 `errors.slot_not_agreed`); read-only (locked) in the booking modal | Visit time |
| proposal | slot + hold (HQ technician or contractor) + message 1–1000 + replyBy (≤7 days) | Only when no pending proposal; slot must not be a preferred time (IR113) | Propose another time |
| assigneeId / contractorOrgId | ID/conditionally required | Match delivery type | Contractor / assignee |
| recurrence | structure/optional for periodic work | monthly, interval 1–12 months, next date; each explicit action generates only the next occurrence and updates next date (D16) | Recurring plan |
| costLines | array/optional | kind=estimate/actual, amountMinor>=0, currency, description | Costs |
| reviewDecision / reason | Conditionally required | accept/return; hold and cancellation also require reasons. Cancellable states follow IR56 | Quality decision / exception reason |

**Steps**

1. Register maintenance type, unit, and deadline. Assign internally or offer to a contractor. Check schedule and progress, review quality, and record actual costs.
2. Apply these business rules to reads and actions. HQ confirms internal assignees and schedules. Contractors accept before assigning their own staff. Recurring plans show the next generation date; allow only one job per plan occurrence.
3. Link Job, Assignment, Offer, cost lines, and review history to the same jobId. Judge work completion separately from Alert resolution.
4. Queries to update: `jobs / plans / offers / assignments / costs / notifications / audit`。
5. Tabs: `tab=jobs` (default), `tab=plans`, `tab=contractors` (DD-A21) and `tab=sla` (DD-A22). Both share the scope filter `customerId` → `propertyId` → `unitId` (URL keys; `jobs.list` / `plans.list` filters).
6. Jobs tab: show per-status counts for the scoped result set (plus a derived **Time proposed** stage = pending SlotProposal, IR113), an Origin filter (Client request / Periodic plan), filters for type, delivery (internal/contractor via `organizationId`), assignee (`membershipId`) and `overdueOnly`, and sort (IR34). Each row shows job, unit, customer, type, status, and the IR89 overdue badge. The `jobId` detail shows a status stepper (internal delivery skips offered/accepted), requested window, due, scheduled slot, symptom, delivery (assignee or offer with access window and `jobs.extendAccess`), the submitted report with Return for rework / Accept (`jobs.review`, self-approval rejected), and cost lines with estimate/actual totals per currency. Hold and Cancel follow IR56; where cancellation is not allowed, disable it and show the reason.
7. Client request triage (IR113): a `requested` job shows the client's preferred times (round N) with availability per time (qualified HQ technicians, contractors, travel) and **Use this time** (enabled only when it fits) → the booking modal with the time locked (`jobs.assign` or `jobs.offer` with `visitSlot`). If none fits: **Propose another time…** (`jobs.proposeSlot`: one option, capacity held, message, reply deadline) → “waiting for the client” card with Withdraw / Edit / Remind. A declined proposal shows the reason and the client's new preferred times. When the contractor proposes another time, the job detail shows the agreed vs proposed slot with **Send to client for approval** / **Keep the agreed time** (`jobs.resolvePartnerSlot`) / Offer to another contractor. The delivery card shows the technician's acknowledgement (awaiting / accepted / can't make it, with reason) and, for can't make it, Reassign… or Propose another time….
8. New job: step 1 `jobs.create` (unit, type, symptom, the customer's preferred times asked by phone — 1st as requested window plus up to 2 alternatives, due defaulting to requested end, contact window); step 2 either `jobs.assign` with candidates from `members.eligible` for the chosen slot, or `jobs.offer` to a contractor organization. “Save as requested” stops after step 1.
9. Plans tab: `plans.list` → `plans.get` for `planId`; edit recurrence (monthly, 1–12 months) and next due with `plans.save`; list generated occurrences with their jobs; “Generate job” calls `plans.generateNext` once for the next occurrence (a repeat for the same date returns CONFLICT).

**Jobs tab from the Core API (IR231)**: the scope (customer → property → unit from `customers.list`, `properties.list`, `units.list`) and the filters (type, origin, delivery — internal is the HQ organization, otherwise a contractor via `organizationId` — assignee via `membershipId`, overdue only, sort) live in the URL and go to `jobs.list` as its filters — type too, and without a stage tile every status but cancelled, so the total counts what the list shows (IR290); the ten stage tiles are one `jobs.list` total each within the scope (Time proposed = `proposalPending`), a tile filters the list. A requested-time period from the HQ overview (`from` / `to` in the URL, IR245) narrows the list and the tiles to jobs whose requested slot starts in it, the same rule as `admin.summary` jobCounts, and a chip names it and clears it. Rows name the unit and customer (the unit's organization), type, origin, status and what is next. The detail (`jobs.get`, `jobs.events`, `units.get`) shows the stepper, the four facts, the follow-up card, the client's preferred times with the HQ technicians free for each (`members.eligible`; past times disabled) and **Use this time** → the booking dialog with the time locked (internal technician from the eligible ones, or a contractor offer with the default offer and access windows and the terms version), **Propose another time…** (date and times, the held HQ technician or contractor, message, reply within 24 / 48 / 72 h but not after the time), the waiting card with Withdraw, the delivery card with the technician's acknowledgement and Reassign… for an internal job the technician cannot make, the contractor's time change with Send to client for approval (reply within 48 h) / Keep the agreed time, Hold… / Resume… / Cancel… with a reason (Cancel disabled with the IR56 reason), and the history. The report card (`reports.get`, photos through `attachments.getContent`, **Open report →** for the whole report) with Return for rework… / Accept report (`jobs.review`; a contractor's job only by escalation with a reason), the cost lines with totals per currency (`jobs.saveCost`), **+ New job** (step 1 `jobs.create`, step 2 the booking dialog on its preferred times) and **Extend access…** (`jobs.extendAccess`) follow IR232. The Plans tab (`plans.list` / `plans.get`, `plans.save`, `plans.generateNext`, occurrences with their job status) follows IR233, and a plan's job is booked at its occurrence. Contractors and SLA show illustrative data until they are connected.

**Boundary cases and failures**: Check re-offering after decline, overlapping confirmed schedules, overdue jobs, and returns after failed quality review. Summarize estimate and actual costs separately by currency, without conversion.

**Follow-up requests (FR-C17)**: A client “Report a problem” creates a requested job with followUpOfJobId and followUpClass=pending, highlighted in the Jobs tab; HQ classifies it within one business day as rework (free, linked to the original job) or a new request (`jobs.classifyFollowUp`, reason required). The list row shows “↩ Follow-up of <jobId>” with the classify-by time; the detail (06-17) shows the client's report, the original job and its rating; Classify… opens a centered dialog (06-18) with Rework (free) / New request and a reason (1–1000). Classification is set once (second call CONFLICT, IR114); the job stays requested and is booked under IR113 (06-11). Tabs: Jobs, Plans, Contractors (DD-A21), SLA by customer (DD-A22).

**Verification**: Check the traceability entries under AT-A06 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A07 Details

**Source mapping**: SRC-06 BIZ-21 → FR-A07 → DD-A07. Source category: original company requirements SRC-06 + design additions. Design additions: plan and contract editing. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A07 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `contracts.list, contracts.save, customers.list, units.list`.

**Initial view and prerequisites**: contract.write permission; customer and linked units are in the same tenant. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

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
5. List: scope filter `customerId` → `unitId` and plan-type chips (`kind`), all as URL keys and `contracts.list` filters. Each row shows contract ID, plan type, customer, unit count, price, period, and restriction eligibility with rules version.
6. Editor (`contractId`) and New contract (no `id`): customer, unit checklist limited to that customer's active units (a unit already on another contract is labelled, not hidden), plan type, period, price and currency, restriction eligibility and rules version (enabled only for `rto`; disabled with the reason for other plans), and a “what saving does” summary (new version, issued invoices keep their version). When `activeRestrictionIds` is non-empty or `hasUnresolvedRecovery` is true, disable save and show the SR19 reason.
7. Language and dates (IR300): texts and plan types follow the display language. Contract periods are Kuala Lumpur business days. The list shows them in the user's language, the date inputs take Kuala Lumpur days, and a hint says so.

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

**Initial view and prerequisites**: billing.write permission; contracts, invoices, and simulated payment targets can be matched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

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
5. Tabs: `tab=invoices` (default), `tab=inquiries` and `tab=payouts` (contractor payouts, DD-A23). The Invoices tab has the scope filter `customerId` → `propertyId` → `contractId` (URL keys; `invoices.list` filters) and `overdueOnly=true` from the dashboard drill-down (step 7 of DD-A01, SR06) as a removable chip, status chips, and totals per currency for the scoped result set (outstanding, overdue, processing, paid in period) without conversion.
6. Invoice detail (`invoiceId`): amount, contract and contract version, period, due, payment method and payment status as read-only values (“Not selected” when null). Enable `payments.confirm` only for an existing processing Payment and `payments.recordManual` only when the invoice has no Payment; otherwise disable with the reason. Reminder: choose recipient (`notifications.recipients`) and channel, show `notifications.preview`, then `invoices.remind` with a reason, only for overdue unpaid invoices. Show related restriction IDs from `InvoiceDetail.restrictionIds` as links to the Restrictions screen; restriction state and units are not read here. Show linked inquiries via `inquiries.list(invoiceId)`.
7. Record manual payment: confirmed amount must equal the full invoice amount and currency, payment reference 1–128 characters unique within the tenant, reason 1–1000 characters; summarise the effect (invoice paid, method stays null, related restrictions move per step 3, resubmitting the same reference returns the same result).
8. Create invoice: contract and contract version, period within the contract, amount in the contract currency, future due date (`invoices.create`). Inquiries tab: `inquiries.list` with state filter; the `inquiryId` detail shows the message and linked invoice/restriction IDs and sends a 1–2000 character reply with `inquiries.answer`; a reply never changes invoices or restrictions.
9. Language and dates (IR299): texts follow the display language. Billing periods, due dates, billing months and pay dates are Kuala Lumpur business days in the user's language. The screen names them when the display time zone is another one, and HQ types new invoice dates as Kuala Lumpur days. Inquiry times are in the display time zone. The reminder preview is the customer's message (`previewText`) in this screen's language.

**Boundary cases and failures**: Reject amount/currency mismatches, reusing a reference on another invoice, and rebilling paid invoices. Recheck and stop reminders attempted immediately after payment is confirmed.

**Verification**: Check the traceability entries under AT-A08 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A09 Details

**Source mapping**: SRC-06 BIZ-21 → FR-A09 → DD-A09. Source category: original company requirements SRC-06 + design additions. Design additions: advance notices, execution confirmation, and device acknowledgements. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A09 / Main display pattern: **UI-LIST / UI-DETAIL / UI-FORM**. Service boundary: `restrictions.schedule, restrictions.execute, restrictions.release, commands.get, restrictions.list, restrictions.get, restrictions.retry, restrictions.reconcile, contracts.list, invoices.list, units.list, units.get`.

**Initial view and prerequisites**: restriction.write permission, an eligible RTO contract, unpaid status, and confirmed target-unit capabilities. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

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
5. List: scope filter `contractId` / `invoiceId` and state (URL keys; `restrictions.list` filters) with per-state counts of the scoped result set. Each row shows ID, state, contract, customer, policy, unit count, and progress (applied/released units, pending offline units).
6. Detail (`restrictionId`): lifecycle stepper, policy, noticeAt, executeAfter, grace/exception, the customer-visible reason (IR42), cause invoices with paid state (`invoices.list`), and per-unit apply/release/observed state (`units.get`, `commands.get`). Enable Execute only for scheduled restrictions at or after executeAfter with rules-version confirmation; Retry/Reconcile only for units that are pending, failed or waiting for reconciliation (SR26); explicit Request release only when payment, grace/exception or override allows it (IR35), otherwise disabled with the reason. Link to the SCR-A10 exception screen.
7. Schedule restriction: pick an eligible RTO contract (`contracts.list`); cause invoices are fixed to all its overdue unpaid invoices; units without the needed capability or with an active restriction are shown disabled with the reason; policy (temperature limit with a setpoint inside the unit range, or power off); executeAfter at least 24 hours ahead; customer-visible reason; show how many clients receive the notice and reject when none can view every target (IR05).
8. Language and time (IR301): texts and state codes follow the display language. Notice, execute-after, grace and exception, release-intent and observed times are instants in the display time zone. HQ types execute-after in that zone, and the dialog names it.

**Boundary cases and failures**: If all cause invoices are paid immediately before execution, set cancelled and create no apply requests. Grace, exceptions, missing notice, and unsupported devices also prevent application. If some units are offline, do not set the whole restriction applied; show per-unit pending states. Failed release stays release_requested; late apply acknowledgements must not return it to applied.

**Verification**: Check the traceability entries under AT-A09 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A10 Details

**Source mapping**: SRC-06 BIZ-21 → FR-A10 → DD-A10. Source category: design additions supporting company goals. Design additions: grace periods, exceptions, and audit steps. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A10 / Main display pattern: **UI-DETAIL / UI-FORM / UI-TIMELINE**. Service boundary: `restrictions.defer, restrictions.exempt, restrictions.cancel, restrictions.override, audit.list, restrictions.get, restrictions.retry, restrictions.reconcile, restrictions.list`.

**Initial view and prerequisites**: Grace/exception actions require restriction.write; manual release requires restriction.override. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

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
5. Screen: breadcrumb back to SCR-A09, a summary (policy, units, cause invoices, grace/exception, the viewer's restriction permissions), four action cards (grace/defer, exception/exempt, cancel, override release) that each state the resulting transition for the current state before save (IR96/IR35), the form for the chosen action (future `until` for defer/exempt, reason 1–1000), an impact summary (release still needs per-unit evidence; the invoice stays unpaid), and an `audit.list` timeline shown only with audit.read. Override is shown only to restriction.override holders; override-only users see the release projection (IR03).
6. Language and time (IR301): texts follow the display language. The defer/exempt `until` is typed in the display time zone, and the form names it. The audit timeline, the active period and the release intent are shown in that zone.

**Boundary cases and failures**: Reject HQ users without override permission, empty reasons, and past grace dates. Cancellation from requested must not assume application is impossible.

**Verification**: Check the traceability entries under AT-A10 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A11 Details

**Source mapping**: SRC-06 BIZ-14, BIZ-16, BIZ-17 → FR-A11 → DD-A11. Source category: original company requirements SRC-06 + design additions. Design additions: condition settings and simulation. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A11 / Main display pattern: **UI-FORM**. Service boundary: `policies.save, automations.simulate, automations.fire, policies.list, policies.get, units.list, units.get`.

**Initial view and prerequisites**: automation.policy.write permission; target units and control capabilities are fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

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
5. Screen: scope filter `customerId` → `propertyId` → `unitId` (URL keys; `policies.list` with kind=automation), list grouped by the customer of the target units (policies spanning customers under “Across customers”), and the `policyId` editor loaded from `policies.get` (AT-A11-R01). The editor has basics (name, priority, timezone, enabled), target units, a When sentence for the chosen condition type, a Then sentence for the UnitAction with the capability range of the targets, and an explanation of the tier order.
6. Simulate: enter synthetic facts per unit (value, unit, observedAt, quality) and call `automations.simulate`; show one row per unit with the selected rule or the suppression reason (`DecisionReason`, e.g. missing_data). Fire (demo) calls `automations.fire` with a one-time key and reports the created command IDs.

**Boundary cases and failures**: Check HQ priority over customer rules, deterministic results for ties, and no automatic execution when solar data is unavailable.

**Verification**: Check the traceability entries under AT-A11 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A12 Details

**Air Quality Display and Processing, Including Allergens (BIZ-18)**

Add allergenObservation to air-quality display data returned by `telemetry.series`. availability is one of the following.

- available
- not_measured
- unsupported

Show substance, value, unit, observedAt, and sourceLabel only when data is available. available requires evidence and time; a numeric value also requires a unit. Treat incomplete information as unknown. Do not derive allergen amounts from PM2.5. Show CO₂ in ppm and electricity-related emissions separately in kgCO₂e.

Verification: AT-A12-SRC. Switch among not-measured, unsupported, and synthetic-observation fixtures. Unmeasured data must not show 0 or “Safe.” A number without a unit must show unknown.

**Source mapping**: SRC-06 BIZ-18, BIZ-19 → FR-A12 → DD-A12. Source category: original company requirements SRC-06 + Figma-confirmed screen specification (Admin 05-7, 2026-10-01): the separate Air quality policies page is merged into Alert policies (SCR-A05).

Scope: FR-A12 / Main display pattern: **UI-FORM / UI-ANALYSIS**. Service boundary: `policies.save, policies.list, policies.get, automations.simulate, automations.fire, telemetry.series, notifications.recipients, units.list`.

**Initial view and prerequisites**: alert.policy.read/write; the Policies tab of SCR-A05 with an air-quality policy selected (e.g. `policyId=policy-co2-a`). Display in this order: validate route/conditions → check session scope → fetch the required Queries.

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| metric | enum/required | co2 (ppm) or pm25 (µg/m³); unit fixed | Air-quality metric |
| threshold / recoveryThreshold | number/required | Recovery on the correct side of the threshold | Trigger/recovery |
| durationSeconds | integer/required | 1–86400 | Duration |
| activeWindow | object/optional | e.g. Mon–Fri 08:00–19:00 | Only if … |
| severity / channels / cooldownMinutes / escalateAfterMinutes / recipientMembershipIds | as DD-A05 | SR28 | Notification settings |

**Steps**

1. Same editor as DD-A05 (IR108). The default policy carries the HQ ventilation rules (CO₂ ≥ 1000 ppm, PM2.5 ≥ 35 µg/m³); customer policies add their own limits.
2. Current readings: for an attached unit, `telemetry.series` shows the latest valid co2/pm25/humidity with units, the allergenObservation availability/source (IR98), and IR99 guidance; unmeasured values are “not measured”, never 0 or safe.
3. Test with demo data / Simulate (`automations.simulate`, `automations.fire` for the demo) shows the duration rule (no alert before the full duration) and the notifications; no control decision or Command is produced. Notification text advises ventilating and logging it in the client Air quality screen (FR-C07).
4. Queries to update: `policies / alerts / notifications / audit`.

**Boundary cases and failures**: Do not mix ppm and µg/m³ thresholds. Do not use unmeasured data to judge normality or recovery. The old route `/admin/settings/air-quality` shows Page unavailable.

**Verification**: Check the traceability entries under AT-A12 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-A13 Details

**Source mapping**: SRC-06 BIZ-23, BIZ-25 → FR-A13 → DD-A13. Source category: original company requirements SRC-06 + design additions. Design additions: baseline versions and calculation conditions. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-A13 / Main display pattern: **UI-ANALYSIS / UI-FORM**. Service boundary: `energy.summary, baselines.list, baselines.save, units.list`.

**Initial view and prerequisites**: energy.write permission, period data within managed scope, and the ability to enter baseline-model evidence. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

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
5. Tabs: `tab=analysis` (default) and `tab=baselines`. Analysis: choose customer (URL `customerId`), units from `units.list`, period, and baseline; show `energy.summary` actual, baseline, difference in kWh and %, cost, and emissions with the IR68 wording, the “not adjusted” label, calculation conditions (boundary, baseline snapshot, factor snapshot, tariff version, coverage) and quality warnings. When unit sets or boundaries differ, show the warning and no difference.
6. Baselines: scope filter `customerId` → `propertyId` → `unitId` and period (URL keys; `baselines.list` filters; the period filter is `[from, to)` on a baseline's period start, labelled “Period starts from (Kuala Lumpur)” / “Period starts before”), the list, and the selected `baselineId`; editing saves a new version with `baselines.save`. baselineKWh is entered only for demo_fixed. Explain that existing MRV reports keep the version they reference.
7. Language and time (IR296): texts follow the display language. The analysis and baseline periods are Kuala Lumpur business time in every display time zone; they are typed in it, and the labels and the conditions name it.

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

**Initial view and prerequisites**: mrv.write permission; period, units, baseline version, factor version, and boundary are selected. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

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
5. Tabs: `tab=reports` (default) and `tab=factors`. Reports: filters `organizationId`, `unitId`, period and status (URL keys; `mrv.list` filters). The `reportId` detail shows a version selector (`mrv.versions`, `reportVersion`; old versions read-only), results (electricity kWh, Scope 2 kgCO₂e, energy vs baseline and emissions vs baseline as separate figures using IR68 wording), the stored condition snapshots (organization, sites/units, period, boundary, baseline and factor with region/year/value/source/version), evidence, and demo review history with the review action for the latest version only.
6. New report: conditions from `organizations.list` → `units.list`, `baselines.list`, `factors.list`, boundary; `mrv.preview` shows a “Not saved” preview with “Demo — unverified”; missing factor/region/boundary or zero coverage shows “Calculation incomplete” without zero-filling; “Save draft” calls `mrv.saveDraft` with the selected evidence.
7. Factors: `factors.list` and a form (region, year, kgCO₂e/kWh with the fixed unit, source stating demo) saved with `factors.save` as a new version; existing reports keep the factor version they reference.
8. Language and time (IR297): texts follow the display language. Report periods are Kuala Lumpur business time in every display time zone. They are typed in it, and the labels and the condition snapshot name it. The list's period filter is `[from, to)` on a report's period start, labelled “Period starts from (Kuala Lumpur)” / “Period starts before”. Demo review times are in the display time zone.

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

**Initial view and prerequisites**: offset.write permission; the screen states that transactions are simulated. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

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
5. Tabs (IR115, Figma Admin 13-1…13-5): `tab=records` (default; offset records with the `recordId` detail, quote → request, proof, failed → retry) and `tab=market` (read-only carbon-market concept; no orders, no prices).
6. Language and time (IR298): texts follow the display language. States, audit events and the market concept are worded, and unknown codes stay as codes. Attempt, event and quote expiry times are in the display time zone. A quote's period is Kuala Lumpur business time, and HQ types it in that time.

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
5. Tabs (IR115, Figma Admin 14-1…14-5): `tab=log` (default; audit search with `correlationId`, actor, target, result and period in the URL) and `tab=devices` (device event history for the `deviceId`; notes are added from Devices & models with device.write, not here).

**Boundary cases and failures**: Search correlation IDs within the authorized set. Other-tenant and nonexistent correlation IDs return the same successful empty result. Reject delete-equivalent calls and reversed periods. Role switching must not change historical actors to different people.

**Verification**: Check the traceability entries under AT-A16 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

## HQ Paths for Shared Operations (FR-X04/X06)

Show CommandPanel at /admin/units?unitId=:id; users with control.execute use commands.create/get. Require a 1–1000-character reason. Acknowledgement/resolution at /admin/alerts?alertId=:id requires alert.resolve; registration, binding, connection checks, calibration, and FW updates at /admin/devices?deviceId=:id require device.write. Share inputs and states with DD-C03/DD-T11 and deterministic contracts D01/D05. Without permission, allow target viewing only and show why actions are disabled.

DD-A08: On invoice selection, fetch invoices.get and select paymentId/version for confirmation from InvoiceDetail.paymentRefs. payments.confirm uses the Payment version; recordManual uses the Invoice version. Do not invent IDs when no Payment exists.

Convert condition forms to the Condition type's discriminated union. occupancy is {type,occupied}, location is {type,event}, pattern is {type,localTime}, weather is {type,metric:"temperature",operator,value}, tariff is {type,operator,value,unit:"MYR_per_kWh"}, peak is {type,active}, and solar/battery is {type,operator,value,unit:"kW"}. Do not send an extra params wrapper. Use weather_temperature for weather Fact.metric; do not confuse it with the room-temperature Fact temperature.

### DD-A17 Details

**Source mapping**: SRC-06 BIZ-04 → FR-A17 → DD-A17. Source category: Figma-confirmed screen specification (Admin 02-15, 2026-10-01).

Scope: FR-A17 / Main display pattern: **UI-LIST**. Service boundary: `clientUsers.list, clientUsers.save, clientUsers.remove, clientUsers.resendInvite`.

**Initial view and prerequisites**: asset.read; customer selected; tab `users`.

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| email | string/required | Valid address, unique per customer | Invitation |
| clientRole | enum/required | owner/member; default member | Role |
| status | enum | invited/active/disabled | Account state |
| reason | string/required for remove | 1–1000 characters | Audit |

**Steps**

1. Table: user (name, email), role, status (“Invite pending · invited <date> by <actor>”), last sign-in, notification channels, ⋯ menu.
2. “+ Invite user” (centered modal) → `clientUsers.save` without id. Menu: change role / disable (`clientUsers.save` with id), resend invite (`clientUsers.resendInvite`, preview only), reset password (`auth.previewPasswordReset`, generic message), remove (`clientUsers.remove`).
3. Queries to update: `client users / audit`.

**Boundary cases and failures**: Last active owner → CONFLICT; duplicate email → VALIDATION. No permission editor for client users.

**Verification**: Check the traceability entries under AT-A17 (N/E/B).

**Customer app (IR114)**: Client owners list users, invite members, and resend invites from `/customer/users` (DD-C19); role changes, disabling, password resets, and removal remain on this tab.

### DD-A18 Details

**Source mapping**: SRC-06 BIZ-07 → FR-A18 → DD-A18. Source category: Figma-confirmed screen specification (Admin 02-17/02-18, 2026-10-01).

Scope: FR-A18 / Main display pattern: **UI-FORM**. Service boundary: `units.importPreview, units.importCommit, units.importUndo, customers.list`.

**Initial view and prerequisites**: asset.write; “Import CSV” on the customer list opens a two-step centered modal.

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| customerId | ID/required | Active customer (search-select) | Owner of created rows |
| file | CSV/required | UTF-8, ≤ 1000 rows | Source |
| mapping | Record/required | property, unit_name, model_code required; floor, room, serial, installed_on, warranty_end optional | Column mapping |

**Steps**

1. Step 1: customer, file (row/column count shown), template download, column mapping → Validate (`units.importPreview`, writes nothing).
2. Step 2: counts ready/warning/error and a row table (row, location/unit, model · serial, result message); Download error report; ← Back; “Import n valid rows” (`units.importCommit`).
3. After import show the result with “Undo (24 h)” (`units.importUndo`, reason).
4. Queries to update: `properties / spaces / units / audit`.

**Boundary cases and failures**: Expired or changed preview → CONFLICT (validate again). Undo after telemetry or jobs on a created unit → CONFLICT. Row results: `error` for a missing property or unit name, an unknown model code, an invalid date or a future installed_on, a serial already bound to another unit, or a unit name already used in the same location; a serial that is not a registered device, is bound to a unit, has an unresolved tamper or appears twice in the file; `warning` when the row creates a property (kind office, no address yet), floor or room that does not exist yet (an existing location of any kind with that name under the parent is reused); otherwise `ready`. Dates are Kuala Lumpur calendar dates. Commit binds the device of a row's serial to the created unit, and undo ends those bindings (IR209). The preview is kept for 30 minutes (`expiresAt`) in a preview store and writes no business data; commit requires the same customer and an unexpired preview, imports ready and warning rows in one transaction, and records one audit entry. Undo archives every created unit, space and property of the import.

**Verification**: Check the traceability entries under AT-A18 (N/E/B).

### DD-A19 Details

**Source mapping**: SRC-06 BIZ-12, BIZ-21 → FR-A19 → DD-A19. Source category: Figma-confirmed screen specification (Admin 02-19, 2026-10-01).

Scope: FR-A19 / Main display pattern: **UI-LIST**. Service boundary: `units.coverage, jobs.recordWarrantyClaim, contracts.list`.

**Initial view and prerequisites**: asset.read or contract.read; tab `warranty`.

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| customerId / coverage / expiringWithinDays | filters | All / All / 90 | Scope |
| partLabel / amountMinor / currency / reason | required for a claim | amount > 0 | Warranty claim |

**Steps**

1. KPI tiles (under warranty, ends ≤ 90 days, out of warranty with no contract, maintenance contract) and the unit table sorted by coverage end.
2. Actions: Renewal offer → SCR-A07 New contract prefilled with customer and unit; Open contract → SCR-A07; Export CSV (client-side, demo).
3. “Warranty on jobs” lists claimable jobs — completed while the unit's warranty ran (warrantyEndsAt not before completedAt), with replaced parts in the accepted work report and no filed claim yet (IR209); the claim form is prefilled with those parts and the job's actual cost lines; Mark claim filed → `jobs.recordWarrantyClaim`.

**Boundary cases and failures**: Claim outside warranty → VALIDATION.

**Verification**: Check the traceability entries under AT-A19 (N/E/B).

### DD-A20 Details

**Source mapping**: SRC-06 BIZ-06, BIZ-20 → FR-A20 → DD-A20. Source category: Figma-confirmed screen specification (Admin 04-9/04-10, 2026-10-01).

Scope: FR-A20 / Main display pattern: **UI-LIST / UI-DETAIL**. Service boundary: `firmwareCampaigns.list, firmwareCampaigns.get, firmwareCampaigns.schedule, firmwareCampaigns.control, devices.list`.

**Initial view and prerequisites**: device.read; tab `firmware`; URL `campaignId`.

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| modelId / targetVersion | required | Signed version from Capability.firmwareCandidates | Target |
| deviceIds | ID[]/required | Devices of the model (excluding open tamper) | Scope |
| waves | array/required | Ascending percents ending at 100 (default 5 devices → 20 % → 100 %) | Rollout |
| window | local times/required | e.g. 01:00–05:00 device local time | Install window |
| autoPauseFailurePercent / startAt | required | 1–50 % / ≥ 24 h ahead; startAt is typed in the user’s display time zone and sent as an instant (NFR-08, IR295) | Safety |

**Steps**

1. List of campaigns with state badges; detail with version/checksum, window, auto-pause, created by, wave progress bars, result counts, and a failure/skip table with Retry.
2. Pause / resume / abort (reason) / retry_device → `firmwareCampaigns.control`; + New campaign (centered modal) → `firmwareCampaigns.schedule`.
3. Customer notices are created 24 h before start (templateKey device_operation preview).

**Boundary cases and failures**: Devices busy with a test run, firmware job, or tamper are skipped with the reason; failed devices keep the old version. Control transitions: pause from scheduled or running; resume from paused back to running (or to scheduled while now < startAt); abort (reason 1–1000) from scheduled, running or paused; aborted and completed are terminal; retry_device only for a failed or skipped device while the campaign is running or paused; any other combination is CONFLICT. Schedule rejects unknown devices, devices of another model, devices with open tamper and devices already on the target version (VALIDATION); devices are assigned to waves by cumulative percent in input order; the checksum is the signed artifact digest (`sha256:…`).

**Verification**: Check the traceability entries under AT-A20 (N/E/B).

### DD-A21 Details

**Source mapping**: SRC-06 BIZ-12 → FR-A21 → DD-A21. Source category: Figma-confirmed screen specification (Admin 06-9, 2026-10-01).

Scope: FR-A21 / Main display pattern: **UI-LIST / UI-DETAIL**. Service boundary: `contractors.list, contractors.save, contractors.setOfferStatus, rateCards.list, rateCards.save, certificates.list, certificates.verify`.

**Initial view and prerequisites**: job.read; tab `contractors`; URL `contractorId`.

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| registrationNo / serviceAreas / contactEmail / insuranceValidUntil | required except insurance | At least one service area | Company profile |
| rate card lines | array/required | workType, amount (MYR), effectiveFrom in the future | Pricing |
| reason | string/required for suspension | 1–1000 | Audit |

**Steps**

1. KPI tiles (offer acceptance, arrival in window, report accepted first time, customer rating with count, rework rate; 90 days) for the selected contractor.
2. Contractor list (status, region, technician count); detail cards: registration, service areas, delegation period, contact, insurance; technicians & certificates with “Verify uploads (n)” (`certificates.verify` approve/reject); rate card table with version and Edit rate card (`rateCards.save`); Suspend offers (`contractors.setOfferStatus`).
3. Queries to update: `contractors / rate cards / certificates / members`.

**Boundary cases and failures**: Offers to a suspended contractor are rejected by `jobs.offer` (CONFLICT); the Jobs tab does not propose it. Contractor organizations without a profile are listed and get a profile from the register; KPI tiles name their measure (targets are per plan in the SLA tab). As built: IR235.

**Verification**: Check the traceability entries under AT-A21 (N/E/B).

### DD-A22 Details

**Source mapping**: SRC-06 BIZ-12 → FR-A22 → DD-A22. Source category: Figma-confirmed screen specification (Admin 06-10, 2026-10-01).

Scope: FR-A22 / Main display pattern: **UI-ANALYSIS**. Service boundary: `sla.scorecard, sla.saveTargets`.

**Initial view and prerequisites**: job.read; tab `sla`.

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| period / contractorOrgId | filters | Last 90 days / all | Scope |
| responseHours / arrivalInWindowPercent / firstTimeFixPercent | required on edit | 1–168 / 0–100 / 0–100 per plan type | Targets |

**Steps**

1. KPI tiles with targets; Customers table (jobs, response, arrival, first-time fix, rating, overdue, status); Recent breaches with Open job → (SCR-A06 job detail); Export CSV (client-side).
2. Edit SLA targets → `sla.saveTargets` (effective for jobs created afterwards).

**Boundary cases and failures**: Metrics without data show “—”, never 0 %. The scorecard returns the targets in effect and scheduled per plan type and each customer’s plan type, so the tiles show targets and the edit dialog its current values (IR236). A breach carries its response minutes (`tookMinutes`, `limitMinutes`), so the tab words it in the user’s language; a job cancelled before its response was due counts neither as within the target nor as a miss (IR291).

**Verification**: Check the traceability entries under AT-A22 (N/E/B).

### DD-A23 Details

**Source mapping**: SRC-06 BIZ-12, BIZ-21 → FR-A23 → DD-A23. Source category: Figma-confirmed screen specification (Admin 08-8, 2026-10-01).

Scope: FR-A23 / Main display pattern: **UI-LIST / UI-DETAIL**. Service boundary: `payouts.list, payouts.get, payouts.generate, payouts.transition, payouts.resolveQuery, rateCards.list`.

**Initial view and prerequisites**: billing.read; tab `payouts`; URL `statementId`.

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| period / contractorOrgId / status | filters | Current month | Scope |
| reply / adjustmentMinor | required/optional | 1–2000; signed minor units | Answer a question |

**Steps**

1. Statements table (contractor, statement, queries, MYR, status); detail with gross, deductions, net, lines, and questions.
2. Reply / Add adjustment → `payouts.resolveQuery`; Approve / Mark paid (unlocks on the pay date) → `payouts.transition`; Generate drafts → `payouts.generate`. All need billing.payment.
3. Queries to update: `payouts / audit`.

**Boundary cases and failures**: Mark paid before the pay date or on a draft → CONFLICT.

**Verification**: Check the traceability entries under AT-A23 (N/E/B).

0.9.0 correction contracts: Read the [Strict Review Correction Contracts](strict-review-contracts.md) and [Per-Operation Version Contract](write-version-catalog.csv) together.

2026-09-16 approved updates: A07 disables saving contracts with active restrictions and shows the reason; Repository rechecks SR19. On failed, A15 calls offsets.simulate(event=retry,recordId,attemptId,demoConfirmed=true) with a new key and current version, then refetches the latest attempt.

0.10.0: A09 uses SR26 recoveryCases for inconsistent-observation recovery. A13 input is BaselineInput: enter baselineKWh only for demo_fixed; Repository computes values and quality for demo_period_comparison. Show quality and assumed-baseline labels together (SR29). A14 filters candidates through units.list(filters.organizationId).

A07 may save only when Contract.activeRestrictionIds is empty and hasUnresolvedRecovery=false. Resolving an A09 recovery case does not release a successor restriction. Device demo events use bindingId fetched from Device (SR24/SR26).

Additional contracts for current version 0.30.0: Read IR01–139 in the [Re-review Correction Contracts](review-resolution-contracts.md). They take priority over older text on the same topic; follow IR72 for conflict precedence.

A13/A14 distinguish IR11 boundaryId (fixed options) from boundary (description). MRV supports on-screen previews of saved versions; file export is outside scope (IR15).

0.15.0: DD-A06 acceptance follows IR29/IR31. Repository stores completion time and rejects self-approval by any contributor.

Apply IR34 to job-list and jobs.list sorting. When URL sort is absent, use status:asc. Changing the selection discards cursor, keeps filters, and fetches page one of a new snapshot. Allow ascending/descending sorting by state, severity, or deadline.
