---
document_id: REQ-C
version: 0.22.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# Client requirements

**0.27.0 implementation baseline**: Read all chapters of [deterministic contracts](../02-design/deterministic-contracts.md) and strict-review-contracts.md, the authorization column in the operation catalog, and the screen catalog. Do not guess numbers, permissions, asynchronous behavior, or recovery during implementation. These are demo design proposals, not production business approval.

## Purpose and assumptions

This document is based on the [original company requests in English (SRC-06)](../00-prepare/sources/company-requirements-original.txt). Company requests are first grouped into BIZ items, then into this document's FR (functional requirements), and finally into detailed designs and acceptance criteria. This provides end-to-end traceability.

Each feature separates company requests from added design details. Screen fields, allowed inputs, state transitions, and priorities are frontend implementation proposals, including details the company has not yet reviewed. Reference mock screens guide appearance only. Requirements and acceptance criteria make the original goals more concrete through production policies and design additions.

This document covers only what users can view, enter, and do in the frontend. Registration, billing, receipts, device actions, and notifications are simulated by mocks. Real server processing, storage, and authentication are out of scope.

The goal is to help users comfortably use their organization’s units and understand status, costs, maintenance, and payments.

Required reading: [PrepareDocument](../00-prepare/PrepareDocument.md) and [common requirements](common.md). All common authentication, language, voice, permission, notification, and non-functional requirements apply.

P0 means the core foundational flow. P1 is also required for completion in phase 1A. Each row is verified under its `AT-C` number. This document separates coverage of company requests from proposed screen design and provisional values.

## Functional requirements and acceptance criteria

| Requirement ID | Priority | Status/basis | Requirement | Acceptance criteria |
|---|---|---|---|---|
| FR-C01 | P0 | Company original SRC-06 + added design details / BIZ-04, BIZ-08 | Monitoring dashboard | Show operation, room temperature, humidity, air quality, power, and alerts using the same unit’s latest values. For missing data, show unknown and its time. |
| FR-C02 | P0 | Company original SRC-06 + added design details / BIZ-07 | Location hierarchy (read-only) | View own properties, floors, rooms, and spaces read-only and navigate property→floor→room→unit; clearly show units with no space. Clients may only rename locations and units; HQ creates, moves, and deletes the structure (FR-A02). |
| FR-C03 | P0 | Company original SRC-06 + added design details / BIZ-13 | Remote control | Change power, set temperature, supported modes, and fan speed. Show requested→sent→response or failure. Unsupported/offline devices do not succeed. |
| FR-C04 | P1 | Company original SRC-06 + added design details / BIZ-14 | Schedules/pre-arrival cooling | Create/edit/disable weekday and time slots. View next execution and test results. |
| FR-C05 | P1 | Company original SRC-06 + added design details / BIZ-14, BIZ-15, BIZ-17 | Automation with consent | Choose demo occupancy/location/routine/weather conditions. Withdrawing location consent stops location rules. |
| FR-C06 | P1 | Company original SRC-06 + added design details / BIZ-16, BIZ-23 | Power/cost comparison | Show kWh, estimated cost, baseline differences, and conditions by period/unit. Do not calculate savings percentage with a zero baseline. |
| FR-C07 | P1 | Company original SRC-06 + added design details / BIZ-18, BIZ-19 | Air quality | Show CO₂ ppm, temperature/humidity, dust, units, values, and sources. Show ventilation guidance and let the user log a manual ventilation; no device command is sent. |
| FR-C08 | P0 | Company original SRC-06 + added design details / BIZ-08, BIZ-09, BIZ-17 | Fault/inspection notifications | Show urgent/caution/normal clearly with text/icons. Link to unit/request screens. Reading a notification does not clear the alert. |
| FR-C09 | P0 | Company original SRC-06 + added design details / BIZ-12 | Maintenance requests/bookings/history | Submit unit, symptoms, and 3 preferred times; receive request number. Accept or decline times proposed by HQ; ask for another time for periodic visits. See whether a job is a client request or a periodic plan visit. View contractor/technician progress and completion reports in the same job. |
| FR-C10 | P0 | Company original SRC-06 + added design details / BIZ-21 | Contracts/invoices | Show contract/unit links, deadlines, unpaid/processing/paid states. Display non-RTO contracts correctly rather than leaving the screen empty. |
| FR-C11 | P0 | Company original SRC-06 + added design details / BIZ-22 | Payment guidance/demo | Preview WhatsApp/email guidance and proceed to test payments. No real card numbers. Reproduce success/failure/processing. |
| FR-C12 | P0 | Company original SRC-06 + added design details / BIZ-21 | Restriction explanation | View start time, reason, scope, and release conditions. After payment, show release pending until device response. |
| FR-C13 | P1 | Company original SRC-06 + added design details / BIZ-23, BIZ-24, BIZ-25, BIZ-26 | Emissions/offset access | Show estimated CO₂ emissions (kgCO₂e) and calculation conditions. Only interested users proceed to demo offset requests/records. |
| FR-C14 | P1 | Figma-confirmed screen specification 2026-10-01 (02m/02n) / BIZ-13 | Group control | The customer owner selects several ACs in one room or area and sends one setting change; each AC gets its own command, offline/restricted ACs are skipped and shown before sending. |
| FR-C15 | P0 | Figma-confirmed screen specification 2026-10-01 (02e/02l/06e–06g) / BIZ-08, BIZ-17, BIZ-18 | Customer alert policies | View the HQ default policy (the owner switches each rule on/off for the account, IR115); create, edit, and delete own alert policies; attach/detach them on each AC. |
| FR-C16 | P1 | Figma-confirmed screen specification 2026-10-01 (04g) / BIZ-16, BIZ-23 | Monthly energy report export | Export a monthly energy/cost report as PDF or CSV and opt in to a monthly email copy. |
| FR-C17 | P1 | Figma-confirmed screen specification 2026-10-01 (07g–07i) / BIZ-12 | Confirm, rate, and report a problem | After HQ accepts the report, confirm the job and rate it; report a problem that creates a follow-up request. |
| FR-C18 | P1 | Figma-confirmed screen specification 2026-10-01 (07j) / BIZ-18, BIZ-12 | Filter care | Show filter run time since cleaning per AC with reminders; mark self-cleaning or request cleaning. |
| FR-C19 | P1 | Figma-confirmed screen specification 2026-10-06 (11a–11c) / BIZ-04 | Customer users (owner) | The customer owner lists the customer's client users, invites members and resends pending invites. Role changes, disabling, password resets, and removal stay with HQ (FR-A17). Members do not see the page. |

## Business boundaries and dependencies

The [common permission matrix](common.md) is the sole authority for who can do what. Separate viewing from changing permissions. Recheck immediately before service calls. If capabilities, work periods, or contract conditions change, do not reuse old screen permissions.

Real device control, external notifications, payments, and API authentication belong to phase 1B. Phase 1A simulates actions, including rejection, failure, and missing data as well as success.

## Completion criteria

- Meet all FR-C and applicable FR-X/NFR requirements. Do not relabel unfinished work as out of scope to claim completion.
- Map acceptance criteria to screens, services, and error handling in the [detailed design](../02-design/client.md).
- Verify every AT-C under the [verification plan](../04-agentic-sdlc/verification.md) and retain applicable scenario evidence.
- Return undecided business questions to PrepareDocument's OPEN list and report provisional mock decisions.

## Feature use cases and business rules (0.6.0)

The table above is an index. The following sections explain entry conditions, steps, results, and acceptance criteria for each requirement.

Numbers ①②… in acceptance cells identify observations within that cell. Match independent Given conditions to Then results by meaning; do not confuse multiple assertions with case IDs. D01's cause-based priority table determines one failure code.

Use the fixed fixtures named in the [verification plan](../04-agentic-sdlc/verification.md).

Detailed thresholds and operating rules absent from the company original are phase 1A proposals under DEC-09, not confirmed production rules.

### FR-C01 Monitoring dashboard

- **Company request basis**: SRC-06 BIZ-04, BIZ-08 — Clear dashboards for customers, internal/external technicians, and administrators/HQ; real-time fault notifications before or when faults occur.
- **Added design details**: Aggregation scope and missing-data display.

- **Entry conditions**: Own organization has available registered units. The screen is also accessible with zero units.
- **Main flow**: Select property/period → check operation, room temperature, humidity, air quality, power, and alert count → open unit list/details from status cards.
- **Business rule BR-C01**: Do not average different rooms into a representative room temperature. Show readings per selected unit/room. Energy is the period total; current power uses the latest observation time.
- **Resulting business state**: Viewing changes no business state. Save filters in the URL and restore them on Back.
- **Boundaries/prohibitions**: A unit with missing data counts as unknown, not normal. Filters specifying another customer's unit must not include it in totals.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C01-N | customer-a; property-home-a contains unit-online-rto (power ON, room 28.0°C, humidity 60%, last observation 00:59:30Z) and unit-non-rto (power OFF). Both connection=online; power observations/latest power Measurements meet fixture.powerClassification fresh conditions. One open alert. When: Open `/customer?propertyId=property-home-a&period=today`, follow running card to list, then Back | ① “Running 1 / Stopped 1 / Unknown 0,” one alert. With no unit selected, temperature says “Select a unit,” without an average ② Power card asOf=00:59:30Z ③ Destination `/customer/properties?propertyId=property-home-a&powerState=on` (IR50), without period; Back restores original period too ④ Zero Repository writes |
| AT-C01-E | ① simulator=false; latest sensor-online-power value=null, quality=missing ② Open `/customer?unitId=unit-other-customer` ③ units.list UNAVAILABLE | ① “Running 0 / Stopped 1 / Unknown 1,” no addition to normal count ② NOT_FOUND/unavailable target; return neither other-customer data nor successful zero totals ③ KPI error/retry; do not show zero or previous values as normal |
| AT-C01-B | ① room-1=28.0°C, room-2=24.0°C, no unit selected ② Select unitId for room-1's unit ③ Change period=7d | ① No representative temperature (26.0°C absent) ② Room 28.0°C and observation time ③ Energy integrated over seven-day [from,to); current power keeps latest observation time |

Design: [DD-C01](../02-design/client.md#dd-c01-details). Assess parent AT-C01 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-C02 Location hierarchy (read-only with rename)

- **Company request basis**: SRC-06 BIZ-07 — Manage homes/offices by area, floor, room, and space.
- **Added design details**: Figma-confirmed 2026-10-01 (Client 02a–02d): the structure is read-only for clients; HQ manages it in Customers & units (FR-A02).

- **Entry conditions**: Client Membership with access to the customer's properties (owner or member).
- **Main flow**: Select property → expand floors/areas → select a room to list every AC in it → open an AC (Unit Control) or rename the location (✎ / Rename).
- **Business rule BR-C02**: Clients cannot create, move, archive, or delete properties, floors, rooms, spaces, or units, and cannot place unassigned units into rooms (HQ does this). The only structural write is `locations.rename` for a property, space, or unit: name 1–120 characters, unique among siblings (IR109).
- **Resulting business state**: Rename increments the target version; HQ and technicians see the new name; IDs, parents, and unit membership never change.
- **Boundaries/prohibitions**: Reject empty or ≥121-character names and duplicate sibling names (VALIDATION, input kept). Rename from an old version returns CONFLICT and shows the current name. Other-customer IDs return NOT_FOUND.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C02-N | customer-a; seed property-home-a/property-office-a. When: select Home A → room “Bedroom” → rename “Home A” to “Home A — Tan family” | ① Tree shows two properties; Bedroom lists Bedroom AC and Bedroom AC #2 ② One `locations.rename` call; Property version+1, child unit IDs unchanged ③ No add/move/delete controls; unassigned units show “placed by HQ” guidance ④ Breadcrumb shows the new name |
| AT-C02-E | ① Empty/121-character name ② Rename room “Bedroom” to sibling name “Living room” ③ Display version=1, another action makes version=2, then save ④ Direct properties.save / spaces.save / spaces.archive with a client session | ① VALIDATION, fieldErrors.name, zero saves ② VALIDATION (duplicate among siblings), input kept ③ CONFLICT, current name shown, input kept ④ FORBIDDEN, zero changes |
| AT-C02-B | ① Rename a unit to a 120-character name ② Rename an unassigned unit ③ Open a space archived by HQ by direct URL | ① Success ② Success; it stays unassigned ③ not-found |

Design: [DD-C02](../02-design/client.md#dd-c02-details). Assess parent AT-C02 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-C03 Remote control

- **Company request basis**: SRC-06 BIZ-13 — Check temperature with smart thermostats and change settings remotely.
- **Added design details**: Modes, fan speeds, and response states.

- **Entry conditions**: Active Membership, target `control` capability, online unit, and within contract restrictions.
- **Main flow**: View room temperature/current confirmed settings → edit power/temperature/mode/fan speed → confirm target/change → follow request accepted, sent, and device response.
- **Business rule BR-C03**: Each confirmed submission is one Action. Change power, temperature, mode, and fan separately. Temperature follows model minimum/maximum/step; modes/fan speeds use fixed choices. Power OFF does not set measured temperature to zero. No second request to the same unit while one is pending.
- **Resulting business state**: Create one Command. Show requested and current settings separately. Update settings only after acknowledged. Keep failed requests/reasons in history.
- **Alert policies on this AC (Figma 02e/02l)**: Unit Control shows the policies attached to the AC: the HQ default policy (always attached, cannot be detached) and the customer's own policies with Detach. “+ Attach policy” lists only the customer's policies (`units.setAlertPolicies`, IR108); detaching removes the policy from this AC only. Rename the AC with ✎ (`locations.rename`).
- **Boundaries/prohibitions**: For a 16–30°C model with 1°C steps, reject 15°C, 31°C, and 24.5°C. Offline, other-customer, restriction-violating, or late-response operations are not success. Use IR46 for allowed restricted actions and IR47 for connection rejection.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C03-N | customer-a, unit-online-rto (16–30°C/1°C, modes cool/dry/fan, fan low/mid/high, confirmed 26°C, room 28°C, version=7). When: Enter 24 → confirm → send → /demo sent→acknowledged | ① One requested Command; separate requested 24°C/confirmed 26°C ② sent shows “Waiting for device response” ③ Only after acknowledged: confirmed 24°C, room remains 28°C ④ Same commandId history requested→sent→acknowledged |
| AT-C03-E | ① Temperature 15/31/24.5 ② unit-offline-rto ③ unit-other-customer ④ Request 22 on unit-limited (restriction minimum 24°C) ⑤ acknowledged at request+30 seconds+1ms | ① VALIDATION, zero Commands ② OFFLINE, zero Commands ③ NOT_FOUND ④ FORBIDDEN (restriction violation), zero Commands ⑤ expired, confirmed remains 26°C; late response recorded as after expiry |
| AT-C03-B | ① power=false ② Send 16 then 30 ③ Send mode=dry then fan=high (acknowledge each before next) ④ Send 25 while requested | ① action=set_power; measured 28°C unchanged ② One accepted each, min/max ③ One each; one submission=one Action ④ Send disabled; direct call CONFLICT |

Design: [DD-C03](../02-design/client.md#dd-c03-details). Assess parent AT-C03 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-C04 Schedules and cooling before arrival

- **Company request basis**: SRC-06 BIZ-14 — Scheduled operation, pre-arrival cooling, and automatic stop when empty.
- **Added design details**: Weekday inputs and overlapping-time rules.

- **Entry conditions**: Unit supports scheduled control; display/storage timezone defined.
- **Main flow**: Enter weekdays/time window/settings → check next execution → save/disable → advance demo clock to start/end events.
- **Business rule BR-C04**: Weekdays refer to the start day. An end before or at the start is interpreted as next day only with the overnight setting; equal start/end is still an error, not 24-hour operation. Require an explicit end action; never assume power OFF.
- **Resulting business state**: Save Automation timezone, start/end actions, and enabled. Creating it sends no immediate Command. Reauthorize at execution time.
- **Boundaries/prohibitions**: Reject no weekdays, missing end action, and ambiguous/nonexistent daylight-saving times. Events after disabling create no requests.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C04-N | unit-online-rto, timezone=Asia/Kuala_Lumpur. When: Save weekdays=[1,3], 18:00–22:00, start=set_temperature 25°C, end=set_power OFF, enabled=true → advance to Monday 18:00 → 22:00 | ① One Automation, next=2026-09-14 18:00+08:00 ② Zero Commands on save ③ One set_temperature 25 Command at 18:00, audited reauthorization ④ One set_power false Command at 22:00 |
| AT-C04-E | ① weekdays=[] ② Missing endAction ③ Seed clock 2026-09-14T01:00Z; save weekly America/New_York Sunday 02:30 (nonexistent 2027-03-14) / Sunday 01:30 (ambiguous 2026-11-01), D09 366-day validation/IR102 ④ Start event after enabled=false | ①② VALIDATION, zero saves ③ VALIDATION (nonexistent/ambiguous time) ④ Zero Commands |
| AT-C04-B | ① Monday 23:00–Tuesday 01:00, endsNextDay=true ② Same times with endsNextDay=false ③ start=end=10:00 | ① Save succeeds, end shown as next day 01:00 ② VALIDATION (end<=start) ③ VALIDATION, not 24-hour operation |

Design: [DD-C04](../02-design/client.md#dd-c04-details). Assess parent AT-C04 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-C05 Automation with consent

- **Company request basis**: SRC-06 BIZ-14, BIZ-15, BIZ-17 — Schedules, pre-arrival cooling, automatic stop when empty; GPS-based reduction while away/resume on return; adapt to routines/weather and identify load from open windows/poor insulation.
- **Added design details**: Consent withdrawal and priority among overlapping conditions.

- **Entry conditions**: Select automation targets/condition types. Location conditions require purpose-specific consent.
- **Main flow**: Choose occupancy, arrival/departure, routine, or weather condition → check consent → save action → trigger test events to check matching/nonmatching conditions.
- **Business rule BR-C05**: Manage location consent only for location_automation; do not record consent merely to use the app (IR102). Phase 1A uses predefined location events only. Routines are demo estimates; collect no real personal behavior history. Missing condition data never counts as a match.
- **Resulting business state**: Withdrawal disables rules with enabled=false, disabledReason=consent_revoked. Regranting does not automatically enable them (IR53). Do not label existing Commands cancelled. Authorized manual actions remain available.
- **Boundaries/prohibitions**: Cannot enable location conditions without consent. Arrival after withdrawal creates no Command. Missing weather data skips with a reason.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C05-N | Seed consent granted=false; unit-online-rto. When: Grant location_automation → save condition=location arrival, action=set_temperature 24°C → /demo arrival → withdraw consent → arrival | ① Consent granted=true, grantedAt saved ② Automation enabled=true ③ First arrival: one Command ④ After withdrawal, disabled rule, second arrival zero Commands, existing Command unchanged |
| AT-C05-E | unit-online-rto: ① Save enabled=true location condition without consent ② Arrival after withdrawal ③ Weather value=null | ① VALIDATION, zero saves ② Zero Commands ③ skipReason=missing_data, zero Commands |
| AT-C05-B | Evaluate same arrival event for unit-online-rto location rule with ① granted=false ② Consent granted ③ Consent withdrawn ④ Missing condition data | ①③④ Zero Commands with reasons ② One Command |

Design: [DD-C05](../02-design/client.md#dd-c05-details). Assess parent AT-C05 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-C06 Power and electricity cost comparisons

- **Company request basis**: SRC-06 BIZ-16, BIZ-23 — Pre-cool at low tariffs, avoid peak prices, integrate solar/batteries; visualize power/cost and compare with normal operation; expect 10–20% or more waste reduction.
- **Added design details**: Comparison periods, tariff versions, and quality display.

- **Entry conditions**: Period data for own units. Actual use remains viewable without baseline/tariff data.
- **Main flow**: Select period/units → view energy/estimated cost → expand baseline conditions → check comparison/data quality.
- **Business rule BR-C06**: Maximum period 366 days, half-open [from,to). Show tariff/emission-factor versions. Do not calculate differences for mismatched unit sets/calculation boundaries. Round money only at the end to currency precision.
- **Resulting business state**: Changed conditions update URL/Query key. Display only; do not change contract tariffs or emission factors.
- **Boundaries/prohibitions**: Baseline 100kWh, actual 80kWh, price 0.5MYR gives savings 10MYR. Baseline 0 means no savings percentage (“Cannot calculate”). Actual 120 means DTO percentage −20, displayed “Increase 20.0%” and “Increase 20.0 kWh” (IR68/IR80). Show coverage for missing data.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C06-N | `acceptancePatches["AT-C06-N"]` (IR92: unit-online-rto actual 80kWh, demo_fixed baseline 100kWh, price 0.5MYR/kWh, custom [2026-09-14T00:00Z, 01:00Z)). When: Display → expand baseline | ① 80.0kWh, estimated 40.00MYR, tariffVersion ② Savings 20.0kWh, 20.0%, 10.00MYR ③ coverage=100% ④ URL has `from`, `to`, `unitIds` |
| AT-C06-E | Query separate cases: .1 Same as AT-C06-N (baseline 100/actual 80) .2 `acceptancePatches["AT-C06-E.2"]` (baseline 0) .3 `["AT-C06-E.3"]` (unit-online-rto/unit-non-rto, baseline 100/actual 120) .4 `["AT-C06-E.4"]` (five missing slots 00:10–00:15Z) | 100/80 gives savings 10MYR. Baseline 0 gives percentage null (“Cannot calculate”); actual 120 gives savingPercentage=-20, “Increase 20.0%” (IR80). Missing slots lower coverage, but valid values remain visible; do not reject the whole screen. |
| AT-C06-B | ① 366 days ② 366 days+1ms ③ Same unit set/calculation boundary ④ Different unit set | ① Success ② VALIDATION ③ Show difference ④ Difference null, reason “Calculation boundary mismatch” |

Design: [DD-C06](../02-design/client.md#dd-c06-details). Assess parent AT-C06 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-C07 Air quality

**Requirement made concrete from the original — Air quality including allergens (BIZ-18)**

Alongside CO₂, dust, and humidity, show whether allergen data is available and its source. Missing data does not mean no allergens.

**Additional acceptance AT-C07-SRC**: Check three fixtures (IR98): not measured (unit-limited, no observation row), unsupported (unit-non-rto), and predefined observation data (unit-online-rto), plus missing-unit `acceptancePatches["AT-C07-SRC.4"]`. Missing data must not show zero or safe. Numeric values without units show unknown.

- **Company request basis**: SRC-06 BIZ-18, BIZ-19 — Monitor CO₂, dust, humidity, allergens; give cleaning/ventilation advice; improve air quality with fresh air when CO₂ rises.
- **Added design details**: Missing-data display and a manual ventilation log (Figma 05a–05c, 2026-10-01).

- **Entry conditions**: Sensor availability and ventilation capabilities can be retrieved.
- **Main flow**: Select room/metric → check value/unit/quality → view ventilation/cleaning advice → after opening a window or running a fan, “Log ventilation” (method, duration).
- **Business rule BR-C07**: 1h/24h presets roll back from current UTC minute boundary; 7d uses SR17 calendar days (IR41). Show separate CO₂ (ppm), PM2.5 (µg/m³), temperature (°C), and humidity (%) series. Humidity 0 is a measured zero; only null is missing. Ventilation is always a manual action logged by the user (IR110); the screen never sends a device command.
- **Resulting business state**: Viewing makes no changes. “Log ventilation” creates one VentilationLog (room, method, duration, CO₂ at logging, user, time) and zero Commands; a log does not imply CO₂ fell.
- **Boundaries/prohibitions**: PM2.5 can display even with missing CO₂. Duration outside 1–240 minutes is VALIDATION. The log is not sent to HQ or to the AC.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C07-N | room-1: CO₂=1000ppm, PM2.5=12µg/m³, temperature 28°C, humidity 60%. When: Display → Log ventilation (window opened, 15 min) → save | ① Four separate series with units ② “Ventilation recommended” (co2≥1000ppm, IR99) ③ One VentilationLog with co2AtLog=1000 ppm, zero Commands ④ CO₂ remains 1000ppm; no assumed reduction |
| AT-C07-E | Display unit-online-rto CO₂=null, PM2.5=12µg/m³ / Log ventilation with duration 0 or 241 min | CO₂ not measured; PM2.5 12µg/m³; “No current advice” (IR99). Duration VALIDATION, input kept, zero logs. |
| AT-C07-B | unit-online-rto: ① humidity=0 ② humidity=null ③ View unit-non-rto without a fresh-air function ④ CO₂=999ppm, PM2.5=12 ⑤ CO₂=1000ppm ⑥ PM2.5=35µg/m³ ⑦ Both CO₂/PM2.5 null | ① “0%” ② “Not measured” ③ Manual ventilation guidance; Log ventilation still available ④ “No current advice” ⑤ “Ventilation recommended” ⑥ “Filter cleaning/inspection recommended” ⑦ “Not enough data for advice” (IR99) |

Design: [DD-C07](../02-design/client.md#dd-c07-details). Assess parent AT-C07 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-C08 Fault and inspection notifications

**Requirement made concrete from the original — Load notifications for open windows/poor insulation (BIZ-17)**

Notify about open windows or poor insulation as possible causes of increased load. Show target unit, time, supporting data, and links to checking/maintenance.

**Additional acceptance AT-C08-SRC**: Check three fixtures in `acceptancePatches["AT-C08-SRC"]` (IR98: alert-window-a, alert-insulation-a, alert-unknown-a and notifications to customer-a). Each has different wording/evidence/time. Reading the notification does not clear the alert.

- **Company request basis**: SRC-06 BIZ-08, BIZ-09, BIZ-17 — Real-time fault notifications before/at occurrence; red/orange/green signals; adapt to routines/weather and identify load from open windows/poor insulation.
- **Added design details**: Separate read status from alert resolution.

- **Entry conditions**: Notifications/alerts in the customer's scope are retrievable.
- **Main flow**: Filter by severity/unread → open notification → navigate to unit evidence/maintenance request → mark only notification read.
- **Business rule BR-C08**: critical/warning determine response priority. normal is a health summary, not an unresolved alert. Distinguish cleaning/replacement due notices from fault notices by type.
- **Resulting business state**: Save read time on Notification; leave Alert.status unchanged. Back restores filters/page position.
- **Boundaries/prohibitions**: Fetch failure must not show zero alerts. If target unit becomes unavailable, opening the link shows only “Unavailable,” without details; exclude that notification from refetched lists (IR58).

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C08-N | `acceptancePatches["AT-C08-N"]`: customer-a unread critical1 (added), warning1 (seed notif-alert-window-a, Alert causeCode=window_open), normal1 (added); seed restriction notice read. When: unreadOnly=true → open critical → unit → Back → mark read | ① Three items, critical first ② Destination URL unit-online-rto ③ Back restores unreadOnly ④ readAt saved, Alert.status remains open |
| AT-C08-E | ① notifications.list UNAVAILABLE ② Keep displayed notification to unit-online-rto; hq-operator members.save changes customer-a scopes to [{kind:property,id:property-office-a}], then open link | ① Error, not zero alerts ② “Unavailable,” no unit name/values |
| AT-C08-B | Add/open one notification per subcase to customer-a, target=unit-online-rto (IR92 item 4): ① critical ② warning ③ normal (source Alerts same severity) ④ type=cleaning_due ⑤ type=fault | ①② Source Alerts count in Summary.counts.alertCount (IR51) ③ normal source Alert excluded (health summary) ④⑤ Distinct icons/labels |

Design: [DD-C08](../02-design/client.md#dd-c08-details). Assess parent AT-C08 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-C09 Maintenance requests, bookings, and history

- **Company request basis**: SRC-06 BIZ-12 — Scheduled, reactive, and preventive maintenance, including general non-RTO maintenance.
- **Added design details**: Booking, cancellation, and job progress.

- **Entry conditions**: Target is an active own-organization unit; RTO status does not affect request eligibility.
- **Main flow**: Enter unit/type/symptoms/3 preferred times → confirm → receive request number → (if HQ proposes another time) accept or decline with reason and new preferred times → check confirmed schedule/progress/report history in list. Every job shows its origin (Client request / Periodic plan).
- **Business rule BR-C09**: Preferred times are not confirmed bookings. HQ books one of them; any other time (proposed by HQ or by a service partner through HQ) is booked only after the client accepts it; declining keeps the request open (IR113). Periodic plan visits are booked by HQ; the client may request another time with 3 preferred times up to 48 h before the visit. Customers cannot set dueAt; deadline is requested slot end (IR38). Customers can cancel only requested jobs. After assigned, they may send coordination notes but cannot directly change schedule/assignee.
- **Resulting business state**: HQ/contractor/technician use the same jobId. Cancellation before assignment records cancelled and reason. Publish submitted reports to customers after quality review.
- **Boundaries/prohibitions**: Reject symptoms ≤9 or ≥2001 characters, past or same-day preferred times, fewer than 3 or duplicate preferred times, or missing unit. Preserve symptoms on communication failure. Duplicate requests do not create duplicate jobs.
- **Coordination notes (Figma 07b → 07o → 07p, IR114)**: On any own job that is not completed or cancelled, “+ Add note” opens a dialog (`jobs.addNote`, visibility=customer, 1–2000 characters). A note never changes the visit time, assignee, or status; HQ (and the contractor of a delegated job) is notified, and the assigned technician sees it in the job history.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C09-N | unit-online-rto. When: Submit type=reactive, 20-character symptoms, requested 2026-09-15 10:00–12:00 (Asia/Kuala_Lumpur, 02:00Z–04:00Z) with alternativeSlots 2026-09-16 14:00–16:00 and 2026-09-17 09:00–11:00 → hq-operator assigns tech-internal-a the 1st slot → advance to 2026-09-15T02:00Z → tech-internal-a starts → saveDraft with `acceptancePatches["shared:report-draft-all-normal"]` → submit → hq-operator accepts internal job | ① jobId returned, requested, slot labeled requested ② Confirmed schedule after assigned ③ Report body hidden before acceptance ④ Report viewable at completed, same jobId |
| AT-C09-E | ① Symptoms 9/2001 characters ② Past slot ③ Missing unitId ④ jobs.create UNAVAILABLE ⑤ Submit same idempotencyKey twice | ①②③ VALIDATION, zero jobs ④ Error, symptoms retained ⑤ One job |
| AT-C09-B | ① Cancel requested job-internal-a ② Cancel assigned job-contractor-a ③ Coordination note on job-contractor-a ④ Retrieve report submitted in AT-C09-N before/after acceptance ⑤ HQ proposes 2026-10-01 10:00–12:00 holding contractor-a on a requested job whose 3 preferred times do not fit; customer-a accepts ⑥ Same proposal; customer-a declines (not_home) with 3 new preferred times ⑦ Only 2 preferred times, or 3 with a duplicate ⑧ hq-operator assigns a slot that is neither preferred nor an accepted proposal (IR113) | ① cancelled, cancelReason saved ② CONFLICT, unchanged state ③ One JobNote visibility=customer ④ Before FORBIDDEN, after retrievable ⑤ Proposal accepted; job offered to contractor-a with visitSlot 2026-10-01 10:00–12:00; one notification to contractor-a ⑥ Job requested, preferenceRound=2, preferredSlots replaced, proposal declined; one notification to hq-operator ⑦ VALIDATION, zero jobs ⑧ VALIDATION errors.slot_not_agreed, zero side effects |

Design: [DD-C09](../02-design/client.md#dd-c09-details). Assess parent AT-C09 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-C10 Contracts and invoices

- **Company request basis**: SRC-06 BIZ-21 — Let administrators reduce/stop cooling for unpaid RTO or similar contracts.
- **Added design details**: Contract lists and invoice states.

- **Entry conditions**: Zero or more own-organization contracts. Viewing requires no payment action.
- **Main flow**: Select contract → check units/period/plan → view amount/deadline/receipt state → invoice details.
- **Business rule BR-C10**: Contract expiry does not mean unit unavailability. Invoice overdue status depends on due date and unpaid balance. Processing is not paid.
- **Resulting business state**: Read-only. Only customers with zero contracts see “No contract / General maintenance” with a monitoring link. Show type/period/invoices for general/energy/environment contracts (IR61).
- **Boundaries/prohibitions**: Customer A cannot retrieve customer B's invoiceId. Paid invoices have no start-payment button.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C10-N | contract-rto-a (unit-online-rto, planType=rto), invoice-overdue-a 120.00MYR, dueAt=2026-09-10, unpaid. When: Select contract → invoice details; also open as customer-b with zero contracts | ① Contract/unit link ② 120.00 MYR, deadline, unpaid/overdue ③ Navigate to invoice details ④ customer-b sees “No contract / General maintenance” and monitoring link |
| AT-C10-E | ① customer-a opens cust-b invoice-general-b from `acceptancePatches["AT-C10-E.1"]` ② Open invoice-overdue-a after hq-operator records full payment using payments.recordManual | ① NOT_FOUND ② No start-payment button |
| AT-C10-B | invoice-overdue-a: ① Just after reset set clock to dueAt (2026-09-10T00:00Z, IR36) ② dueAt+1ms ③ customer-a payments.simulate(initiate→processing) ④ After recordManual ⑤ unit-online-rto after patching contract-rto-a.endAt to 2026-09-13T00:00Z | ① Not overdue ② Overdue ③ Processing, not paid ④ paid ⑤ Monitoring/control available |

Design: [DD-C10](../02-design/client.md#dd-c10-details). Assess parent AT-C10 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-C11 Payment guidance and demo payments

**Requirement made concrete from the original — Card type and guidance choices (BIZ-22)**

From WhatsApp/email guidance, choose demo credit card, demo debit card, or payment instructions.

**Additional acceptance AT-C11-SRC**: Reproduce processing/success/failure for each card type and verify matching history types. Viewing instructions alone leaves the invoice unpaid.

- **Company request basis**: SRC-06 BIZ-22 — WhatsApp/email links to card payments or payment instructions.
- **Added design details**: Payment states and notification previews.

- **Entry conditions**: Unpaid invoice, known current amount, and clear confirmation that payment is a test.
- **Main flow**: Check notification preview → choose demo card/instructions → start demo payment → observe processing/failure/confirmed receipt events.
- **Business rule BR-C11**: No card number, CVV, or expiry inputs. Screen navigation does not confirm payment. Reject another payment while processing.
- **Resulting business state**: Create Payment; set Invoice paid after receipt-confirmation event. Show result/reference ID in history. Allow restriction release requests, but do not show released before response.
- **Boundaries/prohibitions**: Double-clicks or repeated confirmation for the same reference do not double-count. On failure, restore unpaid state and keep retry access.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C11-N | invoice-overdue-a unpaid. When: Email preview → select demo_credit_card → initiate → processing → confirm | ① Preview invoiceId/channel/method ② Payment initiated→processing, Invoice processing ③ After confirm: Invoice paid, Payment confirmed, reference ID ④ Related Restriction automatically becomes release_requested on payment confirmation alone (IR35); applied units get remove Commands. No released label |
| AT-C11-E | ① Double-click initiate ② Confirm same reference twice ③ Trigger fail | ① One Payment ② One confirmed Payment, Invoice paid ③ Invoice unpaid, method retained, retry link |
| AT-C11-B | ① Before start ② Restart during processing ③ After confirmed | ① No card number/CVV/expiry fields ② Button disabled, direct call CONFLICT ③ paid |

Design: [DD-C11](../02-design/client.md#dd-c11-details). Assess parent AT-C11 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-C12 Operating restriction explanations

- **Company request basis**: SRC-06 BIZ-21 — Let administrators reduce/stop cooling for unpaid RTO or similar contracts.
- **Added design details**: Advance notice, grace periods, and pending release.

- **Entry conditions**: Restriction information for own contract; no restrictions is a valid empty state.
- **Main flow**: Check notice/reason/units/scheduled date → view grace/exception/release conditions → proceed to payment or coordination inquiry.
- **Business rule BR-C12**: Separately show scheduled restriction, application requested, per-unit application, release requested, and released. For partial application, show each unit's state, not only an aggregate badge.
- **Resulting business state**: Do not change restrictions on this screen. Pass related IDs to payment/inquiry screens. Inquiries are received only as in-app demos; no external sending.
- **Boundaries/prohibitions**: Do not show overall released while offline units have unknown application results. D03 not_required is allowed with zero-delivery or confirmed-nonapplication evidence. Customers cannot override through direct URLs/services.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C12-N | Scheduled Restriction created by hq-restriction-manager using restrictions.schedule(contract-rto-a, causeInvoiceIds=[invoice-overdue-a], unitIds=[unit-offline-rto,unit-online-rto], policy=power_off, executeAfter=2026-09-15T01:00Z). When: Display → send inquiry | ① Notice/reason/two units/executeAfter ② Release condition: all cause invoices paid ③ Inquiry received, zero external sends ④ Restriction unchanged |
| AT-C12-E | ① After AT-C11-N payment confirmation makes restriction-limited-a release_requested, demo.trigger(device communication_lost) on unit-limited ② customer-a directly calls restrictions.override | ① release_requested; under 30 seconds after remove Command creation, perUnit.releaseState=requested (“Waiting for release response (disconnected)”); at/after 30 seconds, failed (IR102). No released label ② FORBIDDEN, unchanged state |
| AT-C12-B | Create states through normal actions (IR97 item 3): ① AT-C12-N Restriction scheduled ② Advance to executeAfter, execute → requested ③ Seed restriction-limited-a applied ④ After AT-C11-N confirmation: restriction-limited-a release_requested ⑤ Its remove Command sent→acknowledged → released ⑥ For ②, only unit-online-rto responds | ①–⑤ Distinct labels ⑥ Aggregate requested; per-unit applied/pending |

**Additional acceptance AT-C12-R01 (revisit, conflict, cross-role)**

| Acceptance ID | Given / When | Then |
|---|---|---|
| AT-C12-R01 | Send inquiry → HQ replies → customer visits another screen then returns to same invoice | inquiries.list redisplays own answer body/answered. Other customers are denied. |

Design: [DD-C12](../02-design/client.md#dd-c12-details). Assess parent AT-C12 using all N/E/B/R01 and applicable SRC cases in traceability.

### FR-C13 Emissions and offset access

**Requirement made concrete from the original — Future carbon market integration display (BIZ-26)**

Show optional offsets separately from future tokenization/digital-asset and market integration concepts. Displaying the market concept is a design proposal for this phase.

**Additional acceptance AT-C13-SRC**: No request is created without selecting an offset amount. Opening the market concept creates no balance, real certificate, or trade result; show it as distinct from demo retirement records.

- **Company request basis**: SRC-06 BIZ-23, BIZ-24, BIZ-25, BIZ-26 — Visualize power/cost against normal operation; expect 10–20% or more waste reduction; optional carbon offset/exchange platform; future baseline comparisons, measurement/reporting/verification, corporate reporting with regional factors, and credit creation; tokenize savings, distributed ledgers, micro-offsets, market trading, and real-time carbon market APIs.
- **Added design details**: Demo requests, retirement states, and market concept display.

- **Entry conditions**: A period with calculable emissions for own organization. Offset requests are optional.
- **Main flow**: Check estimated emissions/savings/conditions → enter desired amount → view demo quote → confirm demo request → view demo record.
- **Business rule BR-C13**: Energy savings do not create tradable balances. Amount must be positive. Missing factors prevent emissions calculation. A request is not a real external purchase.
- **Resulting business state**: Save OffsetRecord as `demo_requested`. Viewing a quote creates no request record. Proof field shows a reference prefixed “DEMO-” or not yet issued.
- **Boundaries/prohibitions**: Reject zero/negative amounts, expired quotes, and other-customer calculations. Use different labels for demo retirement and real external certification.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C13-N | `acceptancePatches["AT-C13-N"]`: actual 40kgCO₂e, savings 10kgCO₂e (factor-demo-2026, IR92). When: Quote purpose=Demo offset, period=[2026-09-14T00:00Z, 01:00Z), unitIds=[unit-online-rto], amountKg=1 → request with demoConfirmed=true → open record | ① OffsetQuote quoted, expiresAt=now+15 minutes ② Quote alone: zero OffsetRecords ③ Request: one demo_requested ④ Proof “Not issued,” marketConcept=future_concept |
| AT-C13-E | ① amountKg=0/-1 ② Request expired quote ③ Use customer-b calculation ④ Open demo retirement record | ① VALIDATION ② CONFLICT (expired) ③ NOT_FOUND ④ “Demo retirement,” no “Certified” wording |
| AT-C13-B | ① Show 20kWh savings ② Quote only ③ After request ④ No factor | ① No balance field ② Zero records ③ One record ④ Emissions “Cannot calculate,” request disabled |

Design: [DD-C13](../02-design/client.md#dd-c13-details). Assess parent AT-C13 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-C14 Group control (owner)

- **Company request basis**: SRC-06 BIZ-13 — Control ACs remotely; Figma-confirmed 02m/02n (2026-10-01) replaces the earlier “no room-wide bulk control” rule.
- **Entry conditions**: Client owner (`clientRole=owner`) with control.execute; the selection is limited to one space level (one room or area).
- **Main flow**: In Units & locations select a room/area → switch to Group control → select ACs (or “Select all online”) → choose power/temperature/mode/fan → Review & send shows each AC's change and skips → Send to N ACs.
- **Business rule BR-C14**: Each selected AC gets its own `commands.create` with the same FR-C03 validation and confirmation; there is no batch command. Offline ACs and ACs whose restriction would clamp the value are listed as Skipped or with the clamped value before sending. Members see Single AC only.
- **Resulting business state**: One Command per sent AC; each card shows Sending → Confirmed/Failed independently; failed ACs can be retried one by one.
- **Boundaries/prohibitions**: Selections across rooms/areas, member sessions, and zero online ACs are rejected before sending. Automations (FR-C04) remain the way to schedule changes.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C14-N | customer-a owner, Office A › Open office (8 ACs, 1 offline). When: select 4 incl. the offline one → set On, 24 °C, Cool, Fan Auto → review → send | ① Review lists 3 “Will send” and 1 “Skipped (offline)” ② Three Commands, one per AC ③ Each card shows its own Confirmed/Failed |
| AT-C14-E | ① Member session opens Group control ② Select ACs from two rooms ③ One AC fails | ① Not offered; direct sends per AC follow FR-C03 authorization ② VALIDATION before sending ③ Only that card shows Failed with Retry |
| AT-C14-B | ① Select all online (7) ② Lobby AC under restriction minimum 24 °C with 22 °C requested | ① Seven Commands ② Review shows “Set to 24 °C (restriction)” |

Design: [DD-C14](../02-design/client.md#dd-c14-details). Assess parent AT-C14 using all N/E/B cases in traceability.

### FR-C15 Customer alert policies

- **Company request basis**: SRC-06 BIZ-08, BIZ-17, BIZ-18 — Fault and air-quality notifications; Figma-confirmed 06e–06g/02e/02l (2026-10-01).
- **Entry conditions**: Client Membership of the customer; editing own policies requires owner or member of that customer (policies belong to the customer, not the user). Switching default-policy rules requires clientRole=owner (IR115); members see the toggles read-only.
- **Main flow**: Alerts › Alert policies → view the default policy (HQ limits) and, as owner, switch rules on/off → create or edit an own policy (what to watch, condition, recovery, “only if” hours, severity, channels) → attach it on each AC's page.
- **Business rule BR-C15**: Every AC carries the HQ default policy (6 rules: ventilation CO₂, dust/PM2.5, refrigerant leak, compressor short-cycling, clogged filter, AC offline). Clients cannot edit its limits; the customer owner can switch each rule on/off for all their ACs (`policies.setDefaultRule`, owner only — a member call is FORBIDDEN and nothing changes, IR115). Own policies (`kind=alert`, `customerId` = own customer) carry one condition; units carry policies (`units.setAlertPolicies`), never the reverse. Editing changes the policy on every attached AC. Delete detaches it from all ACs first after confirmation; the default policy cannot be deleted. Recovery must be on the correct side of the threshold; units are fixed per metric (ppm/µg/m³ never mixed). WhatsApp is offered only when allowed in Preferences. Missing/stale readings never trigger.
- **Resulting business state**: Policy version+1 on save; DefaultRuleSetting per customer and rule; ACUnit.alertPolicyIds updated on attach/detach. Alerts remain view-only for clients (no Resolve).
- **Boundaries/prohibitions**: Other customers' policies and the HQ template limits are not editable. Attaching another customer's policy returns NOT_FOUND.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C15-N | customer-a owner. When: create “Stuffy office” CO₂ ≥ 1200 ppm 15 min, recover < 1000, Mon–Fri 08:00–19:00, Warning, in-app → attach to Workstations AC 1 and 2 → switch default rule “AC offline” off | ① Policy version=1, customerId=customer-a ② Both ACs list it under “Alert policies on this AC” ③ DefaultRuleSetting enabled=false for customer-a; other customers unchanged |
| AT-C15-E | ① Recovery 1300 ppm for ≥ 1200 ② Delete the default policy ③ Attach customer-b's policy to Bedroom AC ④ Member session switches default rule “AC offline” | ① VALIDATION, nothing saved ② VALIDATION (default cannot be deleted) ③ NOT_FOUND ④ Toggles are read-only with “Only the account owner can change this”; a direct `policies.setDefaultRule` call is FORBIDDEN and DefaultRuleSetting is unchanged |
| AT-C15-B | ① Delete “Bedroom too hot” attached to 2 ACs ② Missing CO₂ readings for 20 min | ① Confirmation names both ACs; policy detached then deleted ② No alert; “not measured” shown |

Design: [DD-C15](../02-design/client.md#dd-c15-details). Assess parent AT-C15 using all N/E/B cases in traceability.

### FR-C16 Monthly energy report export

- **Company request basis**: SRC-06 BIZ-16, BIZ-23 — Energy/cost visibility; Figma-confirmed 04g (2026-10-01).
- **Entry conditions**: Client with access to the selected properties.
- **Main flow**: Energy & cost › Export → month, locations, sections (energy & cost by AC, comparison with last month, CO₂ and offsets, alerts & maintenance), format PDF/CSV → Download; optionally tick “Email this report to me on the 1st of every month”.
- **Business rule BR-C16**: Figures use the same tariff, estimation labels, and MYR amounts as the screen; estimated values stay marked. Only completed months can be exported. The monthly email uses the Preferences email and can be turned off there (`Preferences.monthlyReportEmail`).
- **Resulting business state**: `energy.exportReport` returns a demo file; nothing else changes.
- **Boundaries/prohibitions**: No sections or an unfinished month is VALIDATION; locations outside scope are NOT_FOUND.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C16-N | customer-a, September 2026, all locations, all sections, PDF | ① One ReportFile (application/pdf, isDemo=true) ② Estimated values marked as estimated |
| AT-C16-E | ① Zero sections ② Current month | ① ② VALIDATION, no file |
| AT-C16-B | Tick monthly email then save | Preferences.monthlyReportEmail=true; unticking in Preferences turns it off |

Design: [DD-C16](../02-design/client.md#dd-c16-details). Assess parent AT-C16 using all N/E/B cases in traceability.

### FR-C17 Confirm, rate, and report a problem after completion

- **Company request basis**: SRC-06 BIZ-12 — Maintenance requests and history; Figma-confirmed 07g–07i (2026-10-01).
- **Entry conditions**: Job of the customer in status completed (HQ or contractor accepted the report).
- **Main flow**: Completed job banner (also on Overview for 7 days) → Confirm & rate (1–5 ★ required, tags, optional comment) → job closed for the client; or Report a problem (reason, details, photos, preferred visit) → follow-up request “Under HQ review”.
- **Business rule BR-C17**: The rating can be changed for 7 days (`jobs.rate`); unconfirmed jobs are auto-confirmed after 7 days without a rating. Ratings ≤ 2 ★ ask what went wrong and offer Report a problem. Ratings are visible to HQ and the service company only, never to other customers, and feed contractor KPIs/SLA (FR-A21, FR-A22). Report a problem creates a requested follow-up job linked by `followUpOfJobId`; HQ classifies it within one business day as rework (free) or a new request (`jobs.classifyFollowUp`).
- **Resulting business state**: MaintenanceJob.rating and customerConfirmedAt are set; a follow-up job appears in My requests as “Under HQ review” until HQ classifies it in Admin 06-17/06-18 (FR-A06, IR114); afterwards it shows “Rework (free)” or “New request”.
- **Boundaries/prohibitions**: Jobs not yet completed cannot be rated or reported (CONFLICT). Details outside 10–2000 characters and more than 5 photos are VALIDATION.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C17-N | customer-a, completed job-c02. When: rate 4 ★ with tags “On time”, “Clean work”, “Fixed the problem” and a comment | ① job-c02 rating.stars=4, customerConfirmedAt set ② Admin contractor KPIs include the rating |
| AT-C17-E | ① Rate an in-progress job ② Report a problem with 9-character details ③ Change the rating 8 days later | ① CONFLICT ② VALIDATION, input kept ③ CONFLICT (window closed) |
| AT-C17-B | ① Rate 2 ★ ② Report a problem on job-c02 | ① “What went wrong?” and Report a problem offered ② New requested job with followUpOfJobId=job-c02, followUpClass=pending |

Design: [DD-C17](../02-design/client.md#dd-c17-details). Assess parent AT-C17 using all N/E/B cases in traceability.

### FR-C18 Filter care

- **Company request basis**: SRC-06 BIZ-18, BIZ-12 — Cleaning advice and maintenance; Figma-confirmed 07j (2026-10-01).
- **Entry conditions**: Client with access to the ACs; Maintenance › Filter care tab.
- **Main flow**: View run time since the last cleaning per AC (or per area summary) with status OK / Due soon / Overdue → Request cleaning (opens a prefilled maintenance request) or Mark cleaned (self cleaning) → owners edit reminders.
- **Business rule BR-C18**: Run time counts hours the AC was running (from telemetry). Reminders fire at the model default 250 h (owner may set 50–2000 h) or every N days (7–180, default 30) when run time is unknown, to the location owners (or all users) by app and email. A reminder is an Alert of type maintenance and therefore a cleaning_due notification. A technician cleaning recorded in a job resets the counter like Mark cleaned; Mark cleaned is shown to the technician.
- **Resulting business state**: FilterCareStatus reset with lastCleanedBy=customer; FilterCareSettings saved per customer.
- **Boundaries/prohibitions**: Offline ACs show run time as unknown, never 0.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C18-N | customer-a: Meeting room AC 268 h, Bedroom AC 212 h (threshold 250 h). When: Mark cleaned on Bedroom AC | ① Meeting room AC Overdue, Bedroom AC Due soon ② Bedroom AC resets to 0 h, lastCleanedBy=customer |
| AT-C18-E | ① Threshold 20 h ② Member edits reminders | ① VALIDATION ② FORBIDDEN |
| AT-C18-B | ① Offline AC ② Request cleaning on Meeting room AC | ① Status unknown, run time “—” ② New maintenance request form prefilled with the unit |

Design: [DD-C18](../02-design/client.md#dd-c18-details). Assess parent AT-C18 using all N/E/B cases in traceability.

### FR-C19 Customer users (owner)

- **Company request basis**: SRC-06 BIZ-04 — Visual dashboards for customers with role-appropriate access; Figma-confirmed 11a–11c (2026-10-06, DEC-65) add the customer-app screen promised by FR-A17 (“a client owner can also invite members from the customer app”).
- **Entry conditions**: Client Membership with clientRole=owner of the customer; sidebar Users (`/customer/users`). Members see no Users item; opening the URL directly shows the shared Page unavailable view (FORBIDDEN, IR112).
- **Main flow**: Users → list of the customer's client users (name, e-mail, role, status, last sign-in) → + Invite member (e-mail; role fixed to Member) → Send invite → the row appears as “Invite pending” with an invitation preview → Resend invite on a pending row (preview).
- **Business rule BR-C19**: Owners use `clientUsers.list`, `clientUsers.save` without id and with clientRole=member only, and `clientUsers.resendInvite` for invited users (IR114). E-mail is unique per customer and compared case-insensitively (IR111). Owners cannot invite owners, change roles, disable sign-in, reset passwords, or remove users; HQ does that in Customers & units › Users (FR-A17). Invitations and resends are previews only (FR-X05).
- **Resulting business state**: One ClientUser with status=invited, clientRole=member, and invitedByMembershipId = the owner's Membership; HQ sees the same row in Customers & units › Users.
- **Boundaries/prohibitions**: Duplicate e-mail → VALIDATION with the input kept. clientRole=owner, a save with id, or `clientUsers.remove` from a client session → FORBIDDEN. Other customers' users are never listed.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-C19-N | customer-a owner Tan Wei (tan.wei@example.com) and member Mei Tan. When: open Users → invite lim.ka@example.com → Resend invite on the new row | ① List shows Tan Wei (Owner, Active) and Mei Tan (Member, Active) ② One ClientUser lim.ka@example.com status=invited, clientRole=member, invitedBy = Tan Wei's Membership ③ Resend returns a NotificationPreview only; zero real sends |
| AT-C19-E | ① Invite Mei.Tan@example.com ② Member Mei Tan opens /customer/users ③ Owner calls clientUsers.save with clientRole=owner ④ Owner calls clientUsers.remove for Mei Tan | ① VALIDATION (duplicate, case-insensitive), input kept ② Page unavailable (FORBIDDEN); the sidebar has no Users item ③ FORBIDDEN, zero changes ④ FORBIDDEN, zero changes |
| AT-C19-B | ① Invite with an empty or malformed e-mail ② Resend invite for an active user | ① VALIDATION, zero saves ② VALIDATION (only invited users can be re-invited), zero previews |

Design: [DD-C19](../02-design/client.md#dd-c19-details). Assess parent AT-C19 using all N/E/B cases in traceability.


0.9.0 correction contracts: Read [strict review correction contracts](../02-design/strict-review-contracts.md) and [operation version contracts](../02-design/write-version-catalog.csv) together.

Approval applied 2026-09-16: FR-C01/C06 today/7d/30d use display-timezone calendar days and completed minutes (SR17). FR-C13 retries failed demo offsets only on the same record and failed stage (SR18).

Additional current 0.27.0 contracts: Read [re-review correction contracts](../02-design/review-resolution-contracts.md) IR01–117. They override older text on the same issues; use IR72 for conflict priority.

Job lists support ascending/descending sorting by status (business order), severity, and deadline. Default: status in business order (IR34). Sort all results before pagination; language changes do not change order. Also use AT-REV16-005 for acceptance.
