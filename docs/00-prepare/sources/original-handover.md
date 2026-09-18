# AC Project — Codex implementation handover

## 1. Request for the next Codex

Build a clickable UI/UX for an AC IoT monitoring, remote control, and maintenance service, using the structure of https://aconland-mudah-milik.vercel.app/ as a reference.

Start with split AC units. Group users into three types: client, technician (internal/external contractor), and administrator/HQ. Provide a clear visual dashboard for each. The demo should support full flows for AC control, alert handling, maintenance requests, payments, and operating restrictions, as well as screen navigation.

First check the existing repository, AGENTS.md, startup instructions, and implementation. Prefer the existing technology stack and design system if present. This document does not provide access to the reference site's source code or an existing repository. Do not assume unverified implementations exist.

The “confirmed requirements” below are based on user requests. “Implementation proposals/provisional choices” help work begin; they are not already approved detailed specifications. Make reasonable assumptions for ordinary reversible UI decisions and record them in the deliverables.

## 2. Project goals and current status

### Goals

- Continuously monitor AC status and notify before or when faults occur to support maintenance.
- Manage comfort, air quality, and energy efficiency through remote control and automation.
- Link RTO (rent-to-own) contracts/payments to AC operating restrictions.
- Support general non-RTO maintenance, including scheduled, reactive, and preventive work.
- Visualize power-related CO₂ emissions and savings; later extend to Digital MRV and carbon offsets.

### Completed and pending work

- Original user requirements are organized into three roles.
- Public pages and demo screens of the reference site were researched and compared with requirements.
- No app implementation, repository changes, deployment, or real device connection occurred in this conversation.
- Reference research is based on retrieved page content and some publicly served code. Full click testing, detailed visual review, and production backend verification have not been done.

### Development phases

| Phase | Scope/deliverables |
|---|---|
| 1A: current starting scope | Clickable three-role UI/UX for split AC units, including demo data and state transitions |
| 1B: later implementation | System including database, APIs, authentication, payments, notifications, IoT devices, firmware, and real control |
| 2: future expansion | HVAC, central AC, ventilation equipment, and more brands/types |

The 1A/1B labels clarify implementation stages in this handover. The original user instruction was “phase 1: split units, first clickable UI/UX, then full solution” and “phase 2: HVAC.”

## 3. Reference site findings and differences

### References

- Home: https://aconland-mudah-milik.vercel.app/
- Client: https://aconland-mudah-milik.vercel.app/customer
- Administrator: https://aconland-mudah-milik.vercel.app/admin
- Contractor: https://aconland-mudah-milik.vercel.app/partner
- Technician: https://aconland-mudah-milik.vercel.app/technician
- RTO explanation: https://aconland-mudah-milik.vercel.app/how-rto-works
- Contractor explanation: https://aconland-mudah-milik.vercel.app/contractor
- Exchange: https://aconland-mudah-milik.vercel.app/exchange
- Investment: https://aconland-mudah-milik.vercel.app/investment
- AI roadmap: https://aconland-mudah-milik.vercel.app/future/ai
- API/SDK roadmap: https://aconland-mudah-milik.vercel.app/future/api-sdk

Checked: September 14, 2026. Public pages can change; recheck them during implementation.

### Main design difference

The reference centers on product selection → RTO application → review → contract → installation → payments/maintenance. This project centers on property → floor/room → installed AC → monitoring/control/maintenance, with contracts linked to units.

| Item | Found in reference site | Current implementation approach |
|---|---|---|
| Customer dashboard | Contracts, applications, next payment, warranty/maintenance renewals, points | Make room temperature, air quality, operation, power, and alerts the main information |
| Contractor/technician screens | Job intake, schedules, assignees, photos/reports, customer approval, pay, training | Add unit monitoring, alert evidence, preventive maintenance, and IoT maintenance |
| HQ screens | Reviews, installation, payments, stock, contractors, SLA, complaints, audit | Add monitoring of all units, control, restrictions, energy, and MRV |
| Red/orange/green | Mainly workflow states such as SLA | Also use for unit/air-quality states, managed separately from workflow states |
| GPS/geofencing | Technician site/work tracking | Add user away/home cooling control as a separate feature |
| Exchange | Points, statements, referrals, ratings, contract records | Do not equate it with carbon credits |
| AI/API/SDK | Listed as future features | Separately design needed voice control and IoT connections |
| IoT monitoring/remote control/MRV | Not found on checked pages | Design as new features, without claiming the reference has no implementation |

The contractor explanation stated that auto-assignment, GPS, and pay processing were clickable screens, not automated backend processing. The investment page also clearly said it was a UI/UX prototype. A screen does not prove production processing is complete.

### Reference features that are not required in this phase

Investor funding, referral agents/commissions, points, and campaigns are not required initially. Keep product catalogs, stock, new RTO application reviews, contractor pay, and training if already implemented; they do not need rebuilding from scratch to add monitoring. Do not remove existing features without permission.

## 4. Confirmed requirements: client

Goal: use AC comfortably and check status, electricity costs, air quality, maintenance, and payments.

| ID | Requirement |
|---|---|
| C01 | Graphical dashboard with operation, room temperature, humidity, air quality, power use, and alerts |
| C02 | Classify home/office and manage AC by area, floor, room, and space |
| C03 | Remotely control power, set temperature, supported modes, and fan speed |
| C04 | Weekday/time-slot schedules and cooling before arrival |
| C05 | Automation based on occupancy, location, routines, and weather; location requires consent |
| C06 | Show power/cost trends, comparisons with normal-operation baselines, and estimated savings |
| C07 | Show CO₂, temperature/humidity, dust, and ventilation/cleaning guidance |
| C08 | Show faults, cleaning/replacement due dates, and ventilation with red/orange/green and notifications |
| C09 | Request maintenance/repairs and view bookings, progress, results, and history |
| C10 | View RTO/other contracts, invoices, due dates, and payment receipt states |
| C11 | Receive payment guidance by WhatsApp/email and proceed to card payments or instructions |
| C12 | View nonpayment restriction schedules, reasons, details, and release conditions |
| C13 | View estimated CO₂ emissions/savings and optionally proceed to offsets |

## 5. Confirmed requirements: technicians (internal/external contractors)

Goal: understand assigned units and perform scheduled, reactive, and preventive maintenance.

| ID | Requirement |
|---|---|
| T01 | Dashboard of assigned units, alert severity, unresolved jobs, schedules, and progress |
| T02 | View installation location, brand, model, device configuration, installation date, and maintenance scope |
| T03 | Real-time monitoring of sensor readings, power, operation, and connectivity |
| T04 | Indoor unit: diagnostic support and inspection records for filters, evaporator coils, blower motors/fans, drain pipes/pans, outlets/louvers |
| T05 | Outdoor unit: diagnostic support and inspection records for condenser coils, compressors, fans/blades, refrigerant pipes |
| T06 | Electrical/control: diagnostic support and inspection records for thermostats, sensors, capacitors, contactors, wiring |
| T07 | View suspected faults and their supporting data/history |
| T08 | Intake, handling, and completion reports for scheduled/reactive/preventive maintenance |
| T09 | Record checklists, photos, readings, replacement parts, work details, and next actions |
| T10 | Remote diagnostics, setting changes, and test runs within permissions |
| T11 | IoT registration, unit linking, connection checks, calibration, and firmware updates |
| T12 | Notifications/checks for removal/tampering, power loss, and communication loss |

Internal technicians can view across their assigned scope. External contractors are limited to assigned customers, units, and work periods. Technicians must not freely change customer invoices or apply nonpayment restrictions.

## 6. Confirmed requirements: administrator/HQ

Goal: centrally manage customers, units, maintenance, contracts, revenue, energy, and environmental results.

| ID | Requirement |
|---|---|
| A01 | Visualize customer/unit counts, operation rate, faults, maintenance, unpaid bills, power, and estimated savings |
| A02 | Manage customers, organizations, properties, sites, areas, floors, rooms, and units |
| A03 | Manage three user groups and internal/external view/action permissions |
| A04 | Manage brands, models, capabilities, IoT, sensors, and firmware |
| A05 | Configure alert conditions, recipients, channels, and escalation |
| A06 | Manage maintenance plans, assignments, deadlines, work quality, and costs |
| A07 | Manage contract plans for RTO, general maintenance, energy savings, and environmental services |
| A08 | Manage invoices, payment integration, receipt confirmation, and reminders |
| A09 | Advance notices, temperature limits/stops, and release after payment under contract/application rules |
| A10 | Manage restriction permissions, grace periods, exceptions, manual release, and audit history |
| A11 | Control policies for occupancy, time-of-use tariffs, peak adjustment, and solar/battery integration |
| A12 | Air-quality thresholds, notifications, and control of supported ventilation equipment |
| A13 | Analyze savings against before/after installation data and baseline models |
| A14 | Digital MRV: measurements, calculation methods, emission factors, quality, verification history, reports |
| A15 | Manage credit purchases, retirement, and proof through external services; consider trading according to the scheme |
| A16 | Monitor/audit tampering, communication faults, unauthorized access, and remote control |

## 7. Common requirements and implementation boundaries

### Common requirements

- Sign in, sign out, and reset passwords.
- Choose a display language; notifications and AI responses also use it.
- Voice AI for status checks, actions, and questions, subject to permissions.
- Red = urgent action; orange = caution/action recommended; green = normal. Also use text/icons.
- Visual UI with charts, unit layouts, and status cards.
- Small, low-cost IoT devices for the local market. Consider in-unit installation, maintenance, communication, and removal detection.
- Aim to support multiple brands/types and manage capabilities by model.

### Required display/control distinctions

- Separate direct readings, estimates/diagnoses, and on-site inspection results. Do not treat unmeasured data as normal.
- Show communication loss, unregistered devices, and missing data as unknown/offline, not green.
- Treat indoor CO₂ concentration (ppm) and power-related CO₂ emissions (kgCO₂e) as different metrics/screens.
- The basic power-related emissions formula is energy used × emission factor. Confirm region, year, and calculation boundary for real use.
- Energy savings are the difference between baseline and actual use under matched conditions. Clearly label demo calculations as provisional.
- The original 10–20% or more savings rate is an expectation, not a guarantee.
- Do not promise that unselected sensors can detect tiny refrigerant leaks, allergens, or every component failure.
- Ventilation control requires supported equipment. Indoor fan circulation is not fresh air intake.
- Calculated emission savings do not automatically become tradable credits.
- Separate remote-control requested, sent, device response, and failed states. In production, changing a UI value alone is not success.
- Power/communication loss alone does not prove theft or removal. Keep events separate even in demos.
- This UI document does not decide real IoT parts, electrical work, or manufacturer warranty effects.

## 8. Proposed screen structure

The following is a proposal. Integrate with existing routes where present; do not replace them mechanically.

| Role | Proposed route | Main screens |
|---|---|---|
| Common | /login, /forgot-password | Authentication, demo role selection, password reset |
| Client | /customer | Status cards, AC list, warnings, power trends |
| Client | /customer/properties | Home/office, floors, rooms, spaces |
| Client | /customer/units/:id | Unit details, operation state, controls, history |
| Client | /customer/air-quality | CO₂, temperature/humidity, dust, ventilation guidance |
| Client | /customer/energy | Power, cost, baseline comparison, emissions |
| Client | /customer/automations | Schedules, occupancy, location, tariff integration |
| Client | /customer/maintenance | Maintenance requests, bookings, history |
| Client | /customer/payments | Contracts/invoices, payments, restriction guidance |
| Technician | /technician | Assigned units/jobs and priorities |
| Technician | /technician/units/:id | Sensors, alert evidence, component status |
| Technician | /technician/jobs/:id | Work, checklists, reports |
| Technician | /technician/devices | IoT registration, connection, calibration, updates |
| Administrator | /admin | Overall operation, faults, maintenance, payments, energy |
| Administrator | /admin/units | Cross-customer/location/unit list |
| Administrator | /admin/alerts | Alert intake, assignment, escalation |
| Administrator | /admin/jobs | Scheduled/reactive/preventive maintenance management |
| Administrator | /admin/billing | Contracts, invoices, receipts, reminders |
| Administrator | /admin/restrictions | Restriction notices, execution, release, history |
| Administrator | /admin/energy, /admin/mrv | Analysis, calculation conditions, reporting/verification |
| Administrator | /admin/offsets | Demo offset quotes, requests, and retirement records |
| Administrator | /admin/devices | Devices, connectivity, capabilities, updates |
| Administrator | /admin/settings, /admin/audit | Permissions, notifications, control conditions, audit |

Use the reference site's sidebar, summary cards, lists, and detail layouts. Exact colors, fonts, spacing, and chart styles are not fixed here. Check real reference screens and existing code to match them. Make language, notifications, and voice controls accessible from a shared header or similar area.

## 9. Interactive demo scenarios for 1A

### S01: Control AC from a room

1. Select home/office → floor → room → AC.
2. Show current and set temperatures separately.
3. Change set temperature/power.
4. Show demo requested → success/failure and update action history.
5. For offline units, do not show success; offer retry or status checking.

### S02: From fault to maintenance completion

1. Trigger a demo alert such as a filter inspection recommendation.
2. Show the same alert to client and HQ.
3. HQ assigns a technician.
4. The technician checks evidence and records a checklist/work report.
5. After completion, update client maintenance history and HQ progress.
6. Separate work completion from alert resolution. Resolve the alert through a demo remeasurement or similar action.

### S03: Payments and operating restrictions

1. Create advance notice for an overdue contract.
2. Show the client the reason, deadline, restriction details, and payment link.
3. HQ confirms restriction start; show grace periods, exceptions, and cancellation.
4. The customer makes a demo payment; after receipt confirmation, proceed to release requested.
5. Separate restriction/release actions from device application; show pending when offline.
6. Record all actions in audit history.

Demo payments do not ask for real card data. WhatsApp/email use send previews/simulation without real external sending.

### S04: Air quality and ventilation

1. Show ventilation advice after a demo CO₂ rise.
2. If no ventilation equipment exists, give user guidance.
3. With supported equipment, show a demo ventilation request.
4. Manage thresholds as demo settings; do not display health/safety guarantees.

### S05: Power and emissions

1. Filter by period/unit and compare actual use with baseline.
2. Show estimated cost savings, emissions, reductions, and calculation conditions.
3. Show data quality when readings are missing.
4. Proceed to MRV report previews and demo offset records.

### S06: IoT faults

Reproduce communication loss and removal detection separately; notify HQ/technicians. Record recovery times and response history.

### S07: Language and voice

Switch key screens and notifications to the chosen language. Show voice flows for room-temperature queries and set-temperature changes. Allow text fallback when voice is unsupported or microphone access is denied. State in the implementation result whether real speech recognition or a clearly labeled simulation is used.

## 10. Proposed data and permission implementation

### Main data models

| Entity | Main information/relationships |
|---|---|
| User/Membership | Organization, role, internal/external type, assigned scope |
| Property/Space | Customer, home/office, area/floor/room hierarchy |
| ACUnit | Location, brand, model, type, installation date, capabilities |
| Device/Sensor | AC ID, serial, sensor type, last contact, firmware, calibration |
| Telemetry | Device, metric, time, value, unit, data quality |
| Command | Unit, actor, request, state, device response, failure reason |
| Alert | Unit, type, severity, evidence, detection/acknowledgment/resolution times |
| MaintenanceJob | Unit, type, assignee, deadline, state, report, photos |
| Contract/Invoice/Payment | Customer/unit, plan, payment deadline, receipt state |
| Restriction | Contract, notice, restriction details, exceptions, execution/release state |
| Automation | Conditions, target units, actions, enabled state |
| Notification/AuditEvent | Recipient, channel, delivery state / actor and changes |
| EnergyBaseline/EmissionFactor | Baseline model, region, period, factor, version |
| MRVReport/OffsetRecord | Calculation period, evidence, verification, credit/retirement proof |

### Permission policy

- Client: only units/contracts used by themselves or their organization.
- Internal technician: units/jobs in assigned scope; no billing/restriction permissions.
- External technician: assigned jobs, units, and periods only.
- HQ: overall management within managed organizations; separate permissions for high-impact actions.
- 1A demo role switching does not replace production authentication/authorization. Validate on the API side in 1B.

### State management examples

- Command: requested → sent → acknowledged, or failed/expired.
- Alert: open → acknowledged → resolved.
- Maintenance: requested → assigned → in_progress → completed (also consider cancellation/rework).
- Restriction: scheduled → requested → applied → release_requested → released (also show failures/cancellation).

These state names are proposals. Align with existing models if present.

## 11. Technical policy and provisional choices

- No technology stack is specified. Prefer an existing repository's stack.
- If a new choice is needed: TypeScript + React/Next.js and compatible UI/chart libraries are proposed, not confirmed specifications.
- 1A uses a shared demo store so role switches show the same unit/invoice/job states.
- Allow demo data reset. Implementation chooses reload persistence and documents it in README.
- Separate UI from data access/action services for later API adapter replacement.
- IoT protocols, cloud, database, payment provider, notification provider, and carbon platform are unselected. Do not make production contracts or connections without authorization.
- Do not claim complete support for every language. Use extensible translation keys. English is proposed as the main initial demo language; record Japanese/Malay as candidates. The initial language set is unconfirmed.
- Even if using the reference site's RM display, treat it as a configurable demo value, not a target country/currency decision.
- Use fictional customers, addresses, and readings; do not directly copy apparent personal data from the reference site.

## 12. Proposed implementation order

1. Check repository, execution steps, existing features, and reference site; identify reusable parts.
2. Implement shared layout, three-role navigation, translation keys, and shared demo store.
3. Implement property/room/unit lists, unit details, and control panel.
4. Implement alert → assignment → work → completion across roles.
5. Implement contract/payment → notice → restriction → release demo.
6. Implement air-quality, energy-saving, automation, and IoT management screens.
7. Connect voice flows, MRV/offset demos, notifications, and audits.
8. Check main scenarios, role visibility, mobile layouts, errors, and empty states.
9. Prepare README, startup steps, examples, mock scope, and later tasks for handover.

Even with many screens, menus with broken links are not completion. Clearly state the scope of future features whose details are unimplemented.

## 13. 1A completion criteria

- [ ] All three roles are accessible with different information/actions.
- [ ] Internal/external technician types and assigned scope appear in the demo.
- [ ] Users can follow property → floor → room → unit to details.
- [ ] Set/room temperatures and requested/applied actions are distinct.
- [ ] S01–S07 flows work; simulations are clearly labeled.
- [ ] Unit/job/invoice states agree across customer, technician, and HQ views.
- [ ] Red/orange/green also use text/icons; missing data/communication loss are not normal.
- [ ] General-maintenance units without RTO contracts can be viewed/controlled.
- [ ] Power, air quality, and emissions have distinct metrics, units, and measured/estimated/demo labels.
- [ ] Unsupported model actions are disabled with reasons.
- [ ] Text input works when voice is unsupported or microphone access is denied.
- [ ] Main actions work at common smartphone widths.
- [ ] No real payments, notification sending, AC stops, or credit purchases occur.
- [ ] Report main-flow checks and required existing-project build/check results.
- [ ] README covers startup, demo switching, reset, implemented/unimplemented features, and mock scope.

## 14. Decisions before production implementation

The UI phase can use provisional values, but these must be settled before real device connections or commercial operation.

| Topic | Open decisions |
|---|---|
| Target market | Country, currency, language, electricity rates, applicable contract/notification rules |
| AC scope | Initial brands/models, connection methods, available monitoring/control |
| Hardware | Sensors, power, communication, installation, cost, removal detection |
| Fault diagnosis | Metrics, sampling intervals, thresholds, validation data, false-positive handling |
| Restrictions | Methods, grace periods, exceptions, notice conditions, recovery/communication-loss behavior |
| Payments/notifications | Payment provider, Webhooks, real email/WhatsApp/push connections |
| Automation | Sources for occupancy, location, weather, rates, solar/battery data |
| MRV/offsets | Baseline models, emission factors, verification, schemes, external platforms |
| Operations | Tenant boundaries, retention, audit, recovery, firmware update methods |
| Delivery | Extend an existing site or build new; hosting/public access scope |

## 15. What Codex's final report should include

- Implemented screens and user flows.
- Startup/check instructions and a preview if available.
- Scenarios checked and results.
- Demo/unimplemented/real-connected distinctions.
- Adopted assumptions and next decisions.
- API/data/IoT connection points to hand over to 1B.

Do not interpret this document as approval for production integrations, billing, device control, or public-access changes beyond its scope.
