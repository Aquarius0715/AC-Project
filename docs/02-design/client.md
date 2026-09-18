---
document_id: DD-C
version: 0.21.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# Client Detailed Design

This document defines client (customer) screen features, fields, states, and errors (exceptions). It follows the original company requirements and their linked requirements. It defines the processing and acceptance criteria (what tests check) needed for each FR (functional requirement). Reference mock screens are used only to guide the shared UI appearance.

**Implementation baseline for 0.21.0**: Read all chapters of the [Deterministic Contracts](deterministic-contracts.md) and strict-review-contracts.md, the authorization columns of the operation catalog, and the screen catalog together. Do not guess values, permissions, asynchronous behavior, or recovery during implementation. These are demo design proposals, not approval for production business use.

## Inputs and Responsibilities

The primary source is the [Original Company Requirements (SRC-06)](../00-prepare/sources/company-requirements-original.txt). Screens, inputs, states, and acceptance criteria follow the requirements reorganized from this source. Inputs are the [Role Requirements](../01-requirements/client.md) and [Common Requirements](../01-requirements/common.md). Read the [Common Detailed Design](common.md) and [UIUX Specification](../03-uiux/UIUXSpecification.md) before designing.

This document designs frontend fields, displays, and mock behavior. Registration, assignment, payment receipt, restrictions, and audit on the screen only change state in shared mock memory (temporary example data). This does not request server implementation or database design.

Always validate route parameters (values in URLs) as untrusted input. “Service name” in the table means an operation in the shared Repository (data service). Rows with the same route describe different functions on one screen. Every row supports loading, empty, error, forbidden, and not-found states. Show retry only for recoverable errors. For forbidden and not-found, follow IR57 and do not show retry.

## Screen and Process Design

| Design ID / requirement | Route / main component | Read and action contracts | Input, processing, validation | Errors and prohibited actions |
|---|---|---|---|---|
| DD-C01 / FR-C01 | `/customer` / `Overview` | `units.list`, `telemetry.summary`, `alerts.list`, `summaries.get` | Filter by property and period. Store filters in the URL and summarize only permitted units | Distinguish missing data from 0. Show update time for old values |
| DD-C02 / FR-C02 | `/customer/properties` / `PropertyExplorer` | `properties.list`, `properties.save`, `properties.archive`, `spaces.list`, `spaces.save`, `spaces.archive`, `units.list` | Create and edit property types, names, and hierarchies in the user's organization. Reject cyclic parent space IDs and parents from other properties. Only HQ edits unit capabilities and manufacturer register data | Do not delete spaces containing units. Deleted URLs show not-found. Breadcrumbs return to parent levels |
| DD-C03 / FR-C03 | `/customer/units/:id` / `UnitControl` | `units.get`, `commands.create`, `commands.get` | Build temperature min/max/step, mode, and fan options from capabilities. Mutate after confirmation. Show room temperature separately from settings | Show rejection, expiry, and failure reasons. Recheck state before manual retry |
| DD-C04 / FR-C04 | `/customer/automations` / `AutomationEditor` | `automations.list`, `automations.save`, `automations.simulate`, `automations.fire`, `automations.nextRuns`, `units.list`, `units.get` | Require at least one weekday, start/end times, timezone, target units, and actions. Explicitly confirm overnight settings | Warn on overlapping conditions and show priorities. Reject triggered automation under restrictions and show the reason |
| DD-C05 / FR-C05 | `/customer/automations` / `AutomationEditor` | `automations.save`, `automations.simulate`, `automations.fire`, `consents.get`, `consents.update`, `units.list`, `units.get` | Change fields by condition type (discriminated union). Explain the purpose of location consent. The demo does not collect real location; users enter arrival/departure events | If consent is denied or location unavailable, use manual or scheduled control. Clearly label lifestyle-pattern inference as a demo |
| DD-C06 / FR-C06 | `/customer/energy` / `EnergyExplorer` | `energy.summary`, `baselines.list`, `units.list` | Require start<end and at most 366 days (provisional). Show currency, tariff version, comparison period, and data reliability | Show usable data coverage when data is missing. Do not treat estimated replacements as measurements |
| DD-C07 / FR-C07 | `/customer/air-quality` / `AirQuality` | `telemetry.series`, `units.get`, `commands.create`, `commands.get`, `units.list` | Select metric and period. Separately check ventilation capability before requesting ventilation | Show “Unsupported” when no sensor exists. Fan circulation is not outdoor-air ventilation |
| DD-C08 / FR-C08 | `/customer/alerts` / `AlertInbox` | `alerts.list`, `notifications.markRead`, `notifications.list`, `summaries.get` | Filter by severity or unread status. Keep notificationId separate from alertId | Do not show a healthy “No alerts” summary when fetching fails |
| DD-C09 / FR-C09 | `/customer/maintenance` / `MaintenanceRequest` | `jobs.list`, `jobs.create`, `jobs.get`, `jobs.cancel`, `jobs.addNote`, `reports.get`, `attachments.getContent`, `units.list` | Require unitId, type, symptom description (10–2000 characters), and future requested times (provisional). Requested times are not confirmed bookings | Prevent duplicate requests. Ask users to reselect unavailable times. Customers may cancel only requests with no assignee yet |
| DD-C10 / FR-C10 | `/customer/payments` / `BillingOverview` | `contracts.list`, `invoices.list` | Filter by contract ID and invoice state. Store amounts in minor currency units (such as yen or cents) | Deny data outside permitted scope. Do not present “No invoices yet” as overdue payment |
| DD-C11 / FR-C11 | `/customer/payments/:id` / `PaymentDemo` | `invoices.get`, `payments.simulate`, `notifications.preview`, `notifications.recipients` | Enter invoice ID, demo payment method, and confirmation. Provide no real card-number fields | Do not transfer real money. Do not show completion before processing ends. Reuse the same idempotency key on retry |
| DD-C12 / FR-C12 | `/customer/payments/:id` / `RestrictionNotice` | `restrictions.forInvoice`, `commands.get`, `inquiries.create`, `inquiries.list` | Restrictions are read-only. Provide inquiry and payment paths. Show target units and applied rule version | Hold processing when offline. Provide no customer forced-release feature |
| DD-C13 / FR-C13 | `/customer/energy/offsets` / `OffsetPreview` | `energy.summary`, `offsets.preview`, `offsets.simulate`, `offsets.list`, `units.list` | Enter requested quantity (>0), period, and demo confirmation. Issue no proof numbers for real trades or certification | Show actual CO2 reductions separately from retired credits. Prevent duplicate requests after failure |

## Shared Implementation Steps

1. Check the session and permitted scope. Validate IDs and URL filters against schemas (data format rules).
2. Call mock services through Queries and receive display data.
3. Use React Hook Form and shared schemas for forms. The schemas also validate unit capabilities and periods.
4. Immediately before a mutation, check the target version, permissions, and current state. For major actions, show the target and reason in a confirmation view.
5. Change shared demo data through the Repository and emit an event with a correlation ID (tracking ID). Invalidate related Queries and fetch fresh data.
6. Keep a pending indicator visible while awaiting a response. Show success, denial, and failure separately. Keep form inputs when submission fails.

## Handoff to Testing

For each design ID (DD-C number), test the matching AT-C acceptance criteria, error cases in the table, and unauthorized direct calls. The [Verification Plan](../04-agentic-sdlc/verification.md) is the source of truth for test data and cross-role scenarios. If example limits such as character counts change, update the schema, document, and boundary tests together.

## Detailed Feature Specifications (0.6.0)

Keep form values in RHF (React Hook Form) and validate them with schemas. Read-only screens do not need form validation. Follow input/output contract DDC-03/09 for audit records, notifications, and shared error displays. Show read-only values from a single Query source. The [Implementation Contracts](implementation-contracts.md) define shared types, paging, time, and error handling; the following adds screen-specific conditions. Use the tokens (base colors, sizes, etc.) and patterns in [UIUX](../03-uiux/UIUXSpecification.md) UX-04/08 for appearance.

### DD-C01 Details

**Source mapping**: SRC-06 BIZ-04, BIZ-08 → FR-C01 → DD-C01. Source category: original company requirements SRC-06 + design additions. Design additions: summary scope and missing-data display. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-C01 / Main display pattern: **UI-OVERVIEW**. Service boundary: `units.list, telemetry.summary, alerts.list, summaries.get`.

**Initial view and prerequisites**: Available units are registered in the user's organization; the screen also opens with zero units. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| propertyId | ID/optional | Default: all properties in the user's organization | Target property |
| period | enum/required | today/7d/30d; default: today | Energy summary period |
| unitId | ID/optional | Only units in the selected property | Temperature/humidity target |
| summary | Read-only | total/online/offline/unknown/powerOn/powerOff/powerUnknown/alertCount (critical/warning only, IR51) and asOf | Cards and display timestamp |

**Steps**

1. Select property and period. Check operating state, room temperature, humidity, air quality, power, and alert counts. Open relevant unit lists or details from status cards.
2. Apply the following business rules to both reads and actions.
   - Do not average different rooms' temperatures into a representative temperature.
   - Show temperature and humidity for the selected unit or each room's measurement point.
   - Show total energy for the period and current power at the latest observation time.
3. Viewing does not change business state. Keep filters in the URL and restore them on Back.
4. Queries to update: `units / telemetry / alerts (only on subscribed events)`.

**Boundary cases and failures**: If one unit cannot be measured, count it as unknown rather than normal. Do not include another customer's unit in summaries even if its ID is supplied as a filter.

**Verification**: Check the traceability entries under AT-C01 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-C02 Details

**Source mapping**: SRC-06 BIZ-07 → FR-C02 → DD-C02. Source category: original company requirements SRC-06 + design additions. Design additions: hierarchy editing and deletion rules. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-C02 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `properties.list, properties.save, properties.archive, spaces.list, spaces.save, spaces.archive, units.list`.

**Initial view and prerequisites**: The user may edit properties owned by their organization. Only HQ may edit capabilities in the unit register. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| property.name | string/required | 1–120 characters after trim | Property name |
| property.kind | enum/required | home/office; initially unselected | Type |
| space.name | string/required | 1–120 characters | Floor/room name |
| space.kind | enum/required | area/floor/room/space | Hierarchy type |
| parentSpaceId | ID/null | null for root; same property; no cycles | Parent |
| expectedVersion | integer/required on update | Fetched version | Conflict detection |

**Steps**

1. Create a home or office property. Add needed areas, floors, and rooms. Select a space to view its units, then edit names or types.
2. Apply the following business rules to both reads and actions.
   - Each space belongs to only one parent.
   - Select a parent in the same property. A space cannot be its own parent or have a descendant as parent.
   - Do not archive or delete a space that still contains units or child spaces. First guide the user to move them.
3. Receive the created ID and version, then update the tree and breadcrumbs. Directly under a property, show units with no assigned space (IR62). For navigation from KPIs, show the unit-list section filtered by powerState/connections (IR50). Direct unit reassignment to the HQ register. Renaming does not change unit IDs.
4. Queries to update: `properties / spaces / units / customer summary`。

**Boundary cases and failures**: Reject empty or 121-character names, parent cycles, and parent IDs from other organizations. On CONFLICT, show the current version and keep inputs. Do not overwrite automatically.

**Verification**: Check the traceability entries under AT-C02 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-C03 Details

**Source mapping**: SRC-06 BIZ-13 → FR-C03 → DD-C03. Source category: original company requirements SRC-06 + design additions. Design additions: modes, fan levels, and acknowledgement states. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-C03 / Main display pattern: **UI-DETAIL**. Service boundary: `units.get, commands.create, commands.get`.

**Initial view and prerequisites**: Active Membership, permission to control the unit (control capability), online unit, and compliance with contract restrictions. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| power | boolean/on change | Initially confirmed value | Power |
| targetTemperature | number/on change | Matches capability.min/max/step | Set temperature °C |
| mode / fanLevel | enum/on change | Supported options only | Mode / fan level |
| unitVersion | integer/required | Version at fetch | Current-state check |
| idempotencyKey | UUID/required | Create on confirmed send; keep for retries of the same intent | Duplicate prevention |

**Steps**

1. View room temperature and current confirmed settings. Edit power, temperature, mode, or fan level. Confirm the change and follow request acceptance, sending, and device acknowledgement.
2. Apply the following business rules to both reads and actions.
   - Each confirmed submission is one Action. Change power, temperature, mode, and fan level separately.
   - Temperature must match the model's min/max/step; choose mode and fan level from listed options.
   - A power-OFF request does not set measured temperature to 0.
   - Disable new requests to a unit while a request for that unit is pending.
3. Create one Command and show the requested value separately. Update confirmed settings only after the device returns acknowledged. Keep the request and reason in history even on failure.
4. Queries to update: `commands / unit detail / telemetry summary / audit`。

**Boundary cases and failures**: For demo capability 16–30 degrees in 1-degree steps, reject 15, 31, and 24.5 degrees. Offline units, other customers' units, restriction violations, and late acknowledgements are not successes. Follow IR46 for actions under restrictions and IR47 for connection/power-signal rejection.

**Verification**: Check the traceability entries under AT-C03 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-C04 Details

**Source mapping**: SRC-06 BIZ-14 → FR-C04 → DD-C04. Source category: original company requirements SRC-06 + design additions. Design additions: weekdays and time-overlap rules. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-C04 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `automations.list, automations.save, automations.simulate, automations.fire, automations.nextRuns, units.list, units.get`.

**Initial view and prerequisites**: Target units support schedule control, and a timezone can be fixed for display and storage. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| name | string/required | 1–120 characters | Rule name |
| unitIds | ID array/required | 1–50; within permitted scope | Targets |
| weekdays | integer array/required | ISO 1–7; no duplicates | Weekdays |
| startLocal / endLocal | HH:mm/required | Cannot be equal | Time range |
| endsNextDay | boolean/required | Default: false | Overnight |
| timezone | IANA string/required | Default: display setting; valid zone | Execution timezone |
| startAction / endAction | UnitAction/required | Supported by capabilities | Start/end actions |
| enabled | boolean/required | New: false; save may use true if explicitly switched ON in confirmation | Intent to enable |

**Steps**

1. Enter weekdays, operating hours, and settings. Check upcoming runs. Save or disable, and use the demo clock to check start/end events.
2. Apply the following business rules to both reads and actions.
   - Weekdays refer to the start day.
   - Treat an end time at or before start as next day only with the overnight flag.
   - Reject equal start/end times; do not infer 24-hour operation.
   - Require an end action; do not silently turn power OFF.
3. Save timezone, start/end actions, and enabled in Automation. Creation alone does not send a Command. The Repository evaluates demo-clock start/end triggers internally, rechecks owner permissions at trigger time, and creates Commands. Screens do not call automations.fire for these triggers (IR54).
4. Queries to update: `automations / next-run preview / audit`。

**Boundary cases and failures**: Reject zero weekdays, missing end actions, and ambiguous or nonexistent times (such as daylight-saving changes). After disabling a rule, its scheduled events must not create new requests.

**Verification**: Check the traceability entries under AT-C04 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-C05 Details

**Source mapping**: SRC-06 BIZ-14, BIZ-15, BIZ-17 → FR-C05 → DD-C05. Source category: original company requirements SRC-06 + design additions. Design additions: withdrawing consent and condition priority. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-C05 / Main display pattern: **UI-FORM**. Service boundary: `automations.save, automations.simulate, automations.fire, consents.get, consents.update, units.list, units.get`.

**Initial view and prerequisites**: Automation targets and condition types can be selected. Location conditions require purpose-specific consent. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| condition.type | enum/required | occupancy/location/pattern/weather | Condition type |
| condition (Condition type) | discriminated union/required | Occupancy, arrival/departure, scheduled time (pattern evaluation: IR52), weather comparison | Condition details |
| consentPurpose | enum/required for location | location_automation | Purpose |
| granted | boolean/required for location | Default: false | Consent |
| action | UnitAction/required | Within unit capabilities | Action |
| priority | integer/required | 0–100; default: 50 | Priority within a level |
| simulationEvent | synthetic event/optional | isDemo=true | Behavior check |

**Steps**

1. Select occupancy, arrival/departure, lifestyle pattern, or weather conditions. Check required consent and save actions. Use simulated events to check condition matches.
2. Apply the following business rules to both reads and actions.
   - Manage location consent only for location_automation; do not record consent to use the app itself (IR102).
   - Phase 1A uses only synthetic events for location data.
   - Lifestyle-pattern inference is a demo; do not collect real personal behavior histories.
   - Missing condition data does not count as a match.
3. Withdrawing consent disables related condition rules. Do not show already issued Commands as cancelled. Manual actions remain available within permissions.
4. Queries to update: `consents / automations / simulation results / audit`。

**Boundary cases and failures**: Location conditions cannot be enabled without consent. After consent withdrawal, an arrival event creates zero Commands. Show why evaluation was skipped if weather data is missing.

**Verification**: Check the traceability entries under AT-C05 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-C06 Details

**Source mapping**: SRC-06 BIZ-16, BIZ-23 → FR-C06 → DD-C06. Source category: original company requirements SRC-06 + design additions. Design additions: comparison periods, tariff versions, and data quality. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-C06 / Main display pattern: **UI-ANALYSIS**. Service boundary: `energy.summary, baselines.list, units.list`.

**Initial view and prerequisites**: Period data exists for the organization's units. Actual data remains viewable without a configured baseline or tariff. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| from / to | ISO datetime/required | from<to; maximum 366 days | Summary interval |
| unitIds | ID array/required | Own organization; remove duplicates | Unit set |
| baselineId | ID/optional | Active version with matching boundary | Baseline |
| tariffVersion | Read-only | Included with calculation result | Fictional unit-price version |
| coverage / totals | Read-only | kWh/amountMinor/difference; null allowed | Comparison |

**Steps**

1. Select period and units. Check energy and estimated cost. Expand baseline conditions, then check comparisons and data quality.
2. Apply the following business rules to both reads and actions.
   - Periods are at most 366 days.
   - Aggregate over [from, to), including start and excluding end. Show tariff and emission-factor versions used.
   - Do not calculate differences when baseline and actual results cover different units or calculation boundaries.
   - Round money to the currency's decimal places only at the end.
3. Changing conditions updates the URL and Query key. This screen only displays data; it does not change contract tariffs or emission factors.
4. Queries to update: `energy (only when search conditions change)`.

**Boundary cases and failures**: A 100kWh baseline, 80kWh actual use, and 0.5MYR unit price produce 10MYR savings. Do not show a reduction rate for baseline 0. For actual use 120kWh, the DTO has reduction -20kWh and reduction rate -20; display “Increase 20.0 kWh” and “Increase 20.0%” (IR68/IR80). Show missing data with coverage.

**Verification**: Check the traceability entries under AT-C06 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-C07 Details

**Air Quality Display and Processing, Including Allergens (BIZ-18)**

Add allergenObservation to telemetry.series air-quality display data. availability is available/not_measured/unsupported. Show substance, value, unit, observedAt, and sourceLabel only when data is available. For available, always show evidence and time; a number also requires a unit. Treat incomplete information as unknown. Do not derive allergen quantity from PM2.5. Show CO2 in ppm; show electricity-related emissions separately in kgCO2e. For units without ventilation capability, show guidance only.

Verification: AT-C07-SRC. Check display switching with not-measured, unsupported, and synthetic-observation fixtures. Do not show “0” or “Safe” for unmeasured data. A number without a unit is unknown.

**Source mapping**: SRC-06 BIZ-18, BIZ-19 → FR-C07 → DD-C07. Source category: original company requirements SRC-06 + design additions. Design additions: checking ventilation capability and displaying missing data. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-C07 / Main display pattern: **UI-ANALYSIS**. Service boundary: `telemetry.series, units.get, commands.create, commands.get, units.list`.

**Initial view and prerequisites**: Air-quality sensor availability and ventilation capability can be fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| spaceId / unitId | ID/one required | Valid target. Pass unitIds or spaceId to telemetry.series | Measurement location |
| metric | enum/required | co2/pm25/temperature/humidity | Display metric |
| period | enum/required | 1h/24h/7d/custom; default: 24h. 1h/24h use rolling [to-60 minutes,to)/[to-1440 minutes,to) windows; 7d uses SR17 calendar days; to is a UTC minute boundary (IR41) | Time series |
| value / quality / observedAt | Read-only | Distinguish null from 0 | Measurement evidence |
| ventilationAction | UnitAction/optional | Only with ventilation capability | Ventilation request |

**Steps**

1. Select a room or unit. Check CO2, PM2.5, temperature, humidity, and their observation times. Read ventilation/cleaning guidance; only when ventilation capability exists, request it through confirmation.
2. Apply the following business rules to both reads and actions.
   - Show CO2 in ppm, PM2.5 in µg/m³, temperature in degrees, and humidity in %, as separate series.
   - Humidity 0 is a measured zero, not missing data. null means missing.
   - Follow the IR99 table for ventilation/cleaning guidance (co2≥1000ppm, pm25≥35µg/m³, insufficient data). IR98 defines the allergen observation source.
   - Without ventilation capability, show only manual guidance.
3. Viewing does not change business state. Ventilation requests create normal Command history, but acknowledgement alone does not imply reduced indoor CO2.
4. Queries to update: `telemetry / commands (only on ventilation requests) / audit`.

**Boundary cases and failures**: PM2.5 values remain displayable when CO2 data is missing. Do not create ventilation Commands for units with only fan capability.

**Verification**: Check the traceability entries under AT-C07 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-C08 Details

**Display and Processing for Load Alerts from Open Windows or Poor Insulation (BIZ-17)**

Add causeCode (window_open/insulation_loss/unknown), evidenceKind (demo_observation/inferred/inspection), evidenceText, and observedAt to Alert from alerts.list. causeCode and evidenceKind are required; use unknown if evidence is unavailable. Do not hard-code specific claims such as “Electricity cost doubles.” Label inference “Suspected” and inspected findings “Inspection record.” From notification details, allow navigation to the same unitId or its maintenance request screen.

Verification: AT-C08-SRC. Prepare three fixtures: suspected open window, inspection record of poor insulation, and no evidence. Check that wording, evidence, and time differ. Marking read does not resolve the alert itself.

**Source mapping**: SRC-06 BIZ-08, BIZ-09, BIZ-17 → FR-C08 → DD-C08. Source category: original company requirements SRC-06 + design additions. Design addition: keeping read status separate from alert resolution. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-C08 / Main display pattern: **UI-LIST**. Service boundary: `alerts.list, notifications.markRead, notifications.list, summaries.get`.

**Initial view and prerequisites**: Notifications and alerts within the customer's permitted scope can be fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| severity | enum/optional | critical/warning/all; default: all (all means omit filters.severity, IR74) | Filter |
| unreadOnly | boolean/required | Default: false | Unread |
| notificationId / alertId | Read-only | Separate IDs; notifications may have no linked alert | Reference |
| readAt | datetime/null | null when unread | Read state |
| alertCount | Read-only | summaries.get(kind=customer).counts.alertCount (unresolved critical/warning, IR51). Show separately from unread count (IR102) | Unresolved alert count |

**Steps**

1. Filter by severity or unread status. Open notification text, then unit evidence or maintenance requests. Finally, mark only the notification as read.
2. Apply the following business rules to both reads and actions.
   - critical and warning indicate response priority.
   - normal is a health notice, not an unresolved problem.
   - Use type to distinguish cleaning/replacement reminders from problem alerts.
3. Save read time on Notification. Do not change Alert.status. Restore filters and page position when returning from another screen.
4. Queries to update: `notifications / unread count`。

**Boundary cases and failures**: Do not show zero alerts after a fetch failure. If the referenced unit is unavailable, show only “Unavailable” without details.

**Verification**: Check the traceability entries under AT-C08 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-C09 Details

**Source mapping**: SRC-06 BIZ-12 → FR-C09 → DD-C09. Source category: original company requirements SRC-06 + design additions. Design additions: booking, cancellation, and job progress. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-C09 / Main display pattern: **UI-LIST / UI-FORM / UI-DETAIL**. Service boundary: `jobs.list, jobs.create, jobs.get, jobs.cancel, jobs.addNote, reports.get, attachments.getContent, units.list`.

**Initial view and prerequisites**: The requested unit is active and belongs to the user's organization. An RTO contract is not required to request maintenance. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| unitId | ID/required | Own unit; not archived | Target |
| type | enum/required | periodic/reactive/preventive | Maintenance type |
| symptom | string/required | 10–2000 characters | Symptoms |
| requestedStart / requestedEnd | ISO datetime/required | Future times; start<end | Requested slot |
| contactWindow | string/optional | 0–200 characters. '@' or 7+ consecutive digits returns VALIDATION; visibility follows IR64. Show “Use HH:mm for times (example: Weekdays 09:00-18:00)” (IR90) | Contact hours |
| dueAt | Read-only | Customer cannot enter it. Repository saves the requested slot's end time (IR38) | Job deadline |
| cancelReason / note | string/required for action | 1–1000 / 1–2000 characters | Cancellation / adjustment request |

**Steps**

1. Enter unit, maintenance type, symptoms, and preferred dates. Confirm and receive a request number. From the list, view confirmed schedules, progress, and past reports.
2. Apply the following business rules to both reads and actions.
   - Requested times are not confirmed bookings.
   - Customers may cancel only requested jobs (no assignee yet).
   - From assigned onward, customers may send adjustment notes but cannot directly change schedules or assignees.
3. HQ, contractors, and technicians share the same jobId. Cancellation before assignment records cancelled and the reason. Publish submitted reports to customers after quality review.
4. Queries to update: `jobs / job events / admin summary / notifications`。

**Boundary cases and failures**: Reject 9- or 2001-character symptom descriptions, past requested times, and missing units. Keep symptom text after communication failure. Duplicate submission must not create two jobs.

**Verification**: Check the traceability entries under AT-C09 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-C10 Details

**Source mapping**: SRC-06 BIZ-21 → FR-C10 → DD-C10. Source category: original company requirements SRC-06 + design additions. Design additions: contract lists and invoice states. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-C10 / Main display pattern: **UI-LIST / UI-DETAIL**. Service boundary: `contracts.list, invoices.list`.

**Initial view and prerequisites**: The organization has zero or more contracts. Viewing does not require payment actions. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| contractId | ID/optional | Own organization only | Contract selection |
| status | enum/optional | all/unpaid/processing/paid/overdue (UI values: map overdue to overdueOnly=true; all means omit status) | Invoice filter |
| amountMinor / currency | Read-only | Integer with currency | Amount |
| dueAt / paidAt | Read-only | UTC → display zone | Due/payment date |
| planType / unitIds | Read-only | rto/general/energy/environment | Applicable contract |

**Steps**

1. Select a contract. Check units, period, and plan. Check invoice amount, due date, and payment state, then open invoice details.
2. Apply the following business rules to both reads and actions.
   - Contract expiry does not itself mean a unit is unavailable.
   - Determine invoice overdue status from the deadline and unpaid balance. Payment processing is not paid.
3. For non-RTO general/energy/environment contracts, show contract type, period, and invoices. This screen is read-only. Only when there are zero contracts, show “No contract — general maintenance” with a path back to monitoring.
4. Queries to update: `contracts / invoices (read-only)`.

**Boundary cases and failures**: Customer A cannot fetch customer B's invoiceId. Do not show “Start payment” for paid invoices.

**Verification**: Check the traceability entries under AT-C10 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-C11 Details

**Card Type and Payment Instruction Selection (BIZ-22)**

The screen's paymentChoice is demo_credit_card/demo_debit_card/demo_instructions. Pass only the two card choices as method to payments.simulate(event=initiate). For demo_instructions, pass event=instructions with only invoiceId/demoConfirmed; do not create Payment or change Invoice method/state. Credit and debit cards use the same state flow; keep the selected type in confirmation and results. Viewing instructions alone does not set paid. notifications.preview shows invoice ID, contact channel, and selected payment method. Provide no real card or bank information fields.

Verification: AT-C11-SRC. Reproduce processing, success, and failure for each card type. Check that history records the selected type. Opening payment instructions alone leaves the invoice unpaid.

**Source mapping**: SRC-06 BIZ-22 → FR-C11 → DD-C11. Source category: original company requirements SRC-06 + design additions. Design additions: payment states and notification previews. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-C11 / Main display pattern: **UI-DETAIL / UI-FORM**. Service boundary: `invoices.get, payments.simulate, notifications.preview, notifications.recipients`.

**Initial view and prerequisites**: An unpaid invoice exists, its current amount is known, and the user can confirm this is a demo payment. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| invoiceId | ID/required | Unpaid invoice | Target |
| paymentChoice | UI enum/required | demo_credit_card/demo_debit_card/demo_instructions | Map cards to initiate.method; instructions to the instructions event |
| channel | enum/for preview | email/whatsapp | Instructions |
| demoConfirmed | boolean/required | Default: false | Confirm simulation |
| outcome | enum/demo controls | processing/confirmed/failed | Test event, separate from normal form |

**Steps**

1. Check the notification preview. Choose a demo card or payment instructions. Start demo payment and observe processing, failure, and payment-confirmation events.
2. Apply the following business rules to both reads and actions.
   - Do not request card number, CVV, or expiry date.
   - Screen navigation alone does not confirm payment.
   - Do not accept a new payment intent during processing.
3. Create Payment; mark Invoice paid only after a confirmation event. Show result and reference ID in history. A restriction-release request may start, but do not show released before acknowledgement.
4. Queries to update: `payments / invoices / restrictions / notifications / audit`。

**Boundary cases and failures**: Double clicks or confirmation with the same reference must not double-count payment. On payment failure, keep the unpaid state and a retry path.

**Verification**: Check the traceability entries under AT-C11 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-C12 Details

**Source mapping**: SRC-06 BIZ-21 → FR-C12 → DD-C12. Source category: original company requirements SRC-06 + design additions. Design additions: advance notice, grace periods, and pending release. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-C12 / Main display pattern: **UI-DETAIL**. Service boundary: `restrictions.forInvoice, commands.get, inquiries.create, inquiries.list`.

**Initial view and prerequisites**: Restriction information relates to the user's contracts. No restriction is a valid empty state. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| invoiceId | route ID/required | Within invoice read scope | Find related restrictions |
| restrictionId / rulesVersion | Read-only | Check invoice relationship | Basis |
| noticeAt / executeAfter / graceUntil | Read-only | With display timezone | Schedule |
| perUnitStates | Read-only array | By apply/release Command | Application status |
| message | string/required for inquiry | 1–2000 characters | Adjustment request |

**Steps**

1. Check advance notice, reason, target units, and scheduled date. Review grace periods, exceptions, and release conditions. Proceed to payment or request an adjustment.
2. Apply the following business rules to both reads and actions.
   - Show scheduled restriction, apply request, per-unit result, release request, and released separately.
   - For partial application, show each unit's state. A summary badge must not hide details.
3. This screen does not change restrictions; it only passes relevant IDs to payment or inquiry actions. Inquiries are accepted in the demo app and are not sent externally.
4. Queries to update: `inquiries / inquiry events (only on submission)`.

**Boundary cases and failures**: Do not show released if offline units still have unknown application results. Confirmed non-application under D03 is not_required. Customers cannot override restrictions by changing URLs or calling services directly.

**Verification**: Check the traceability entries under AT-C12 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-C13 Details

**Display and Processing for Future Carbon-Market Integration (BIZ-26)**

Add marketConcept to offsets.preview results. stage is always future_concept, providerLabel is “Not selected,” verificationStatus is “unverified,” and ledgerStatus is “not_connected.” Display energy savings, estimated emission reductions, and demo purchase/retirement records separately. Provide no market prices, real token balances, or trade-execution buttons. Explain that integration partners and verification conditions are undecided. The UI only displays asynchronous Repository results, allowing a future data adapter to be added later.

Verification: AT-C13-SRC. Without selecting an offset, no request is created. Opening the market-concept screen generates no balance, real proof, or trade result, and displays a state distinct from simulated demo retirement.

**Source mapping**: SRC-06 BIZ-23, BIZ-24, BIZ-25, BIZ-26 → FR-C13 → DD-C13. Source category: original company requirements SRC-06 + design additions. Design additions: demo requests, retirement states, and market-concept display. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-C13 / Main display pattern: **UI-ANALYSIS / UI-FORM**. Service boundary: `energy.summary, offsets.preview, offsets.simulate, offsets.list, units.list`.

**Initial view and prerequisites**: The organization has a period eligible for calculation. Offsets are optional. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| period / unitIds | Required | Same as calculation conditions | Target |
| amountKg | number/required | >0, maximum 100000, 3 decimal places (provisional) | Requested kgCO₂e |
| quoteId / quoteVersion | ID/required on request | Fetched simulated quote | Confirmation target |
| demoConfirmed | boolean/required | Default: false | Confirm this is not a real purchase |

**Steps**

1. Check estimated emissions, reductions, and conditions. Enter quantity and get a demo quote. Confirm, submit a demo request, and view the simulated record.
2. Apply the following business rules to both reads and actions.
   - Do not create tradable balances from electricity savings.
   - Quantity must be positive. Missing emission-factor data prevents emissions calculation.
   - A request does not mean a real external purchase.
3. Save OffsetRecord as demo_requested. A quote alone does not create a request. In the proof field, show a reference starting with “DEMO-” or issue none.
4. Queries to update: `offsets / audit`。

**Boundary cases and failures**: Reject zero/negative quantities, expired quotes, and another customer's calculation results. Use distinct wording for simulated demo retirement and real external certification.

**Verification**: Check the traceability entries under AT-C13 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

Convert condition forms to the Condition type's discriminated union. occupancy is {type,occupied}, location is {type,event}, pattern is {type,localTime}, weather is {type,metric:"temperature",operator,value}, tariff is {type,operator,value,unit:"MYR_per_kWh"}, peak is {type,active}, and solar/battery is {type,operator,value,unit:"kW"}. Do not send an extra params wrapper. Use weather_temperature for weather Fact.metric; do not confuse it with the room-temperature Fact temperature.

0.9.0 correction contracts: Read the [Strict Review Correction Contracts](strict-review-contracts.md) and [Per-Operation Version Contract](write-version-catalog.csv) together.

2026-09-16 approved updates: C01/C06 period boundaries follow SR17. C13 retry follows SR18 like A15; get the current version and attemptId through offsets.list.

Additional contracts for current version 0.21.0: Read IR01–106 in the [Re-review Correction Contracts](review-resolution-contracts.md). They take priority over older text on the same topic; follow IR72 for conflict precedence.

Apply IR34 to job-list and jobs.list sorting. When URL sort is absent, use status:asc. Changing the selection discards cursor, keeps filters, and fetches page one of a new snapshot. Allow ascending/descending sorting by state, severity, or deadline.
