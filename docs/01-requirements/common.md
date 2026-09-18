---
document_id: REQ-COMMON
version: 0.21.0
status: draft
owner: design-agent
scope: frontend-demo-1A
---

# Common requirements

Parent document: [PrepareDocument](../00-prepare/PrepareDocument.md). Read this with the four role requirement documents. Sources are grouped as company original, production instructions, and added design details.

This document reorganizes the [original company requirements in English (SRC-06)](../00-prepare/sources/company-requirements-original.txt), the primary source. Trace information through company original → BIZ groups (categories of company requests) → FR (functional requirements) here → detailed design and acceptance criteria. Each feature separates company requests from design team additions. Screen fields, input limits, state changes, and priorities are frontend implementation proposals, not detailed company approvals. Reference mock screens guide visual design. Functional requirements and acceptance criteria make the original goals concrete using production instructions and added design details.

**0.21.0 implementation baseline**: Read all chapters of [deterministic contracts](../02-design/deterministic-contracts.md) and strict-review-contracts.md, the authorization column in the operation catalog, and the screen catalog. Do not guess numbers, permissions, asynchronous behavior, or recovery during implementation. These are demo design proposals, not production business approval.

## Common features

| ID | Priority | Status | Requirement | Acceptance criteria (AT-X number) |
|---|---|---|---|---|
| FR-X01 | P0 | Company original SRC-06 + added design details / BIZ-01, BIZ-02 | Sign in/out, reset passwords, select demo role and display language | Switch among four roles. Sign-out clears screen content and cache. Password reset uses generic wording that does not reveal whether the recipient exists. Language changes affect key screens, notifications, and date/time display. Authentication only simulates the demo experience |
| FR-X02 | P1 | Company original SRC-06 + added design details / BIZ-03 | Voice status checks, actions, and help in the chosen language, with equivalent text input | Temperature query/change demo works. Confirm target/details before changes and use the same permission checks as normal actions. Text input can complete actions when voice is unavailable or denied |
| FR-X03 | P0 | Company original SRC-06 + added design details / BIZ-09, BIZ-18 | Distinguish severity, measured/estimated/inspection results, data quality, and units | Urgent = red, caution = orange, normal = green, with text/icons as well as color. Show unknown for unmeasured/stale/disconnected data. Do not confuse ppm (volume ratio) with kgCO₂e (carbon dioxide equivalent) |
| FR-X04 | P0 | Added design details (supporting a company goal) / BIZ-04 | Limit viewing/actions by tenant, role, assigned scope, period, and capability | Menus, direct URLs, and service calls cannot retrieve out-of-scope data. Role switches never display the previous role's cached screen. State that APIs must check permissions again |
| FR-X05 | P0 | Company original SRC-06 + added design details / BIZ-05 | Clarify shared demo data, reset, and differences from real processing | Role switches in one tab show the same jobs/invoices/units. Reset returns to the same seed. No real card input, external sending, real device control, or real trades |
| FR-X06 | P0 | Company original SRC-06 + added design details / BIZ-05, BIZ-06, BIZ-12, BIZ-19 | Distinguish supported features and non-RTO treatment | Disable unsupported model actions with reasons. General-maintenance units can be monitored/controlled without contract billing. Do not label fan circulation as ventilation |
| FR-X07 | P0 | Company original SRC-06 + added design details / BIZ-08, BIZ-20, BIZ-22 | Provide notifications, history, and audit | Link to related alert/job/restriction screens. Reading a notification is separate from resolving an alert. Change records keep actor, time, target, reason, result, and correlation ID linking related records |

## Non-functional requirements

NFR-01–05/07 are quality proposals supporting the company's clickable UI goal (BIZ-05). NFR-06 is a production instruction (SRC-02) for future API integration. NFR-08 is a design proposal detailing multilingual support (BIZ-02). Numbers, test methods, and initial language below are not values specified in the company original.

| Requirement ID / Acceptance ID | Priority | Requirement and measurement | Basis/treatment |
|---|---|---|---|
| NFR-01 / AT-NFR01 | P0 | Complete main actions using only a keyboard. Show focus. Link labels and error messages. Do not rely only on color. Target WCAG 2.2 AA using automated checks and manual operation tests | Accessibility criteria are PROPOSED; do not claim verified compliance |
| NFR-02 / AT-NFR02 | P0 | Check main actions/navigation at 360/768/1024/1279/1280/1440 CSS pixels. Actions remain usable at 200% body text size. No page-level horizontal scrolling except data-table screens | Responsive targets are PROPOSED |
| NFR-03 / AT-NFR03 | P0 | Use fictional data only. Keep secrets out of logs, URLs, and storage. Confirm risky actions and test rejection/expired confirmation: if session expiry, permissions, or target version change during confirmation, do not submit; confirm again (IR74) | Does not replace production security implementation |
| NFR-04 / AT-NFR04 | P1 | Measure production build with 100 units and 1000 samples for the selected series on the same device/browser. Target immediate feedback within 200ms and demo list display within 2 seconds | Provisional development-environment targets. Measure intentional delays separately; not a network SLA |
| NFR-05 / AT-NFR05 | P0 | Show loading, empty, error, offline, forbidden, not-found, and stale states. During refetch, keep data and show “Updating” (IR83). Retries do not create duplicates. Preserve unsaved input | Check using each screen's failure fixtures |
| NFR-06 / AT-NFR06 | P0 | UI calls mock services through asynchronous interfaces. Separate appearance from data access for later replacement with real systems | Frontend only; API specifications, HTTP integration, database, and server design are out of scope |
| NFR-07 / AT-NFR07 | P0 | After implementation, run type checks, lint, build, route/permission unit tests, form component tests, and S01–S08 end-to-end tests. State reasons for anything not run | Implementation package scripts define the actual commands |
| NFR-08 / AT-NFR08 | P1 | Use translation keys and Intl for language, time, currency, and units. Store UTC and clearly show display timezone. Timezone changes must preserve the meaning of booked times | Demo languages: English (default) and Malay. Actual target market is unconfirmed |

Assess each AT-NFR using the measurement method above and the [verification plan observation points](../04-agentic-sdlc/verification.md#11-common-and-nonfunctional-at-observation-points).

## Permission matrix

“In scope” requires matching tenantId plus customer membership, delegated job, assigned scope, and valid period. Even HQ does not have unrestricted access to every tenant.

| Action | Client | Contractor | Technician (internal/external) | HQ |
|---|---|---|---|---|
| View units/locations | Own use scope | Information needed for accepted jobs | Internal: assigned scope / external: assigned units and period | Managed scope |
| Create/edit properties/locations | Own organization | No | No | Managed scope |
| Normal AC control | Within scope/capabilities | No (do not grant commands.create/voice.resolveIntent to contractors) | Diagnostic permission and valid target/period | control.execute permission |
| Maintenance requests | Own units | Coordinate accepted jobs | Handle assigned jobs | Create/assign within managed scope |
| Contractor acceptance/own assignment | No | Accepted jobs, own company, valid qualifications/period | No | Outsource or assign internally |
| Inspection/work reports | View results | View, quality review, return for rework | Create for assigned jobs | View/quality review |
| View invoices/payment demo | Own contracts | No | No | Manage with billing.manage |
| Restrictions/grace/release | View reasons | No | No | restriction.manage; override requires separate permission |
| IoT registration/calibration/firmware updates | View status | View accepted units' status | device.maintain within assigned scope | device.manage permission |
| MRV/audit | Own estimates/history | History needed for jobs only | History of assigned actions | mrv.manage/audit.read permissions |
| Change users/roles | No | View own assignment candidates only | No | identity.manage permission |

Use IR46 for allowed AC actions under restrictions, and IR47 for rejection based on connectivity/power signals. Do not disclose details, including existence, of inaccessible data. Show reasons for blocked actions only when viewing the data itself is allowed. Contractors do not automatically gain technician control permissions. If one manager has several roles, switch Membership so the active role is clear.

## Canonical terms

Short terms in this text refer to canonical names: unit = ACUnit (AC itself, `unitId`); device = Device (IoT device inside the AC, `deviceId`); job = MaintenanceJob (customer maintenance request, `jobId`); outsourcing offer = Offer (HQ request to a contractor, `offerId`); assignment = Assignment; inquiry = Inquiry (customer question to HQ about billing/restrictions); notification = Notification; alert = Alert. “Request” refers to Job; HQ requests to contractors are called Offer. DDC-08 §5 governs role identifiers.

## Business invariants

- Room/set temperature and requested/device-confirmed values are distinct. Completion requires a device response event, even in the demo.
- Work completion and alert resolution are separate. Resolution requires remeasurement or a recorded confirmation action.
- Making a payment and confirming receipt are separate. Release requires confirmed payment followed by a release request and either an applied device's release response or evidence of confirmed non-delivery/non-application (D03).
- Restrictions, diagnostics, firmware updates, and voice setting changes share capability, permission, and scope checks.
- Do not replace missing indoor CO₂, dust, or power-related emissions with zero. Do not guarantee savings percentages or issue emissions as credits.
- Demo thresholds and contract rules are provisional, not formal medical, legal, or electrical-work decisions.

## Detailed business conditions for common features (0.6.0)

### FR-X01 Sessions and language

- **Company request basis**: SRC-06 BIZ-01, BIZ-02 — Sign in/out, reset passwords, and choose a display language.
- **Added design details**: Four-role demo, initial languages (English default, Malay available), reset wording.

Demo login selects from predefined fictional accounts. The demo clock measures the 30-minute session life, but `demo.advanceClock` jumps do not consume it (IR36). Show an extension prompt 120 seconds before expiry; choosing extension adds 30 minutes from that moment (IR55). Login supports four roles and internal/external technician Memberships without real email/password input. Unauthenticated access to a protected route redirects to `/login?returnTo=…`. Keep only allowed relative return routes and ignore invalid values (IR82). Sign-out/role switches clear old screen content, Queries, image previews, and permission-dependent drafts.

The password reset demo checks only email format and returns the same completion message/send preview whether registered or not. Language changes affect English/Malay display but not stored UTC, units, or IDs. AT-X01 covers `demoSession.signIn/signOut/switchMembership`, `auth.previewPasswordReset`, and `preferences.update` (locale): direct protected-route access, Back after logout, stale responses during role switches, and no existence disclosure for valid-format unregistered demo emails.

### FR-X02 Voice and questions

- **Company request basis**: SRC-06 BIZ-03 — Voice AI responses.
- **Added design details**: Extend responses to temperature queries, device actions, and questions; voice demo/text fallback; confirmation before changes. Voice device changes are not explicitly stated in the company original.

Supported intents are temperature query, set-temperature change, and help (read operation instructions; do not create an FR-C12 Inquiry). Hide the panel for roles without voice/text action permission, such as contractors (IR44). When room names match multiple candidates, show extra context such as container/floor to narrow the target. If still indistinguishable, switch to text input for target selection. Never guess and execute. For unknown intents, show “Action unavailable” and return to text input. Before sending a voice change as a Command, show target, previous value, and new value. No business writes occur before confirmation.

AT-X02 checks duplicate room names (including identical container/floor context requiring text input), unauthorized rooms, unsupported temperatures, microphone refusal, and cancelled confirmation. Withdrawing location or microphone consent discards the pending request; it does not undo earlier device responses. No real voice service is connected.

### FR-X03 Status and quality

- **Company request basis**: SRC-06 BIZ-09, BIZ-18 — Red/orange/green signals and notifications; CO₂, dust, humidity, allergen information, and cleaning/ventilation guidance.
- **Added design details**: Color meanings, units, data quality, and non-color cues.

Display severity, connectivity, operation, workflow progress, and data quality as separate dimensions. An offline device may show its last-known power state, but must not claim it is currently running. Never replace missing readings with zero. Show value, update time, and quality together. AT-X03 checks simultaneous display of disconnected, previously powered ON, and unresolved warning for the same unit, and separate metrics for ppm and kgCO₂e.

### FR-X04 Scope and action permissions

- **Company request basis**: SRC-06 BIZ-04 — Visual dashboards for customers, internal/external technicians, and administrators/HQ.
- **Added design details**: Visibility limits by role, assignment, period, and capability.

An action requires all of the following: valid session, matching tenant, allowed role/permission, matching target scope, valid period, and current state/capability conditions. The same conditions govern projections. Client, contractor, and external technician boundaries differ. Recheck permissions on every action, not only when showing menus.

AT-X04 verifies rejection for seven cases: another tenant, another customer in the same tenant, another contractor, an unqualified technician, exactly at expiry, an old open screen after permission changes, and the same person switching Membership to approve their own work. This tests frontend demo display/action control. Production authorization design/implementation is out of scope.

### FR-X05 Demo and shared data

- **Company request basis**: SRC-06 BIZ-05 — Start with split AC units and clickable UI/UX, then expand to the full solution.
- **Added design details**: Shared mocks, reset, and separation from real processing.

Use a fixed seed. Four roles in one tab read the same business IDs. Role switching does not reset business data. Only page reload or explicit reset restores initial data. Delayed events from before reset cannot enter the new seed. Visually separate the state-control panel from business screens and always label it “Demo.” AT-X05 checks `demo.reset`, `demo.trigger`, and `demoSession.switchMembership` for repeatable seed data, cross-role consistency, discarded old delayed events, and zero real payment/notification/IoT/trading connections.

### FR-X06 Capabilities and contract types

- **Company request basis**: SRC-06 BIZ-05, BIZ-06, BIZ-12, BIZ-19 — Start with split units and clickable UI/UX before the full solution. Expand to HVAC in Phase 2 and to more brands, split/central/cassette types. Support scheduled/reactive/preventive and non-RTO general maintenance. Improve air quality with fresh air intake when CO₂ rises.
- **Added design details**: Model capabilities and screen enablement rules.

Tie Capability to a model version. Unverified capabilities are not supported. General-maintenance units can be controlled without RTO billing. Restriction eligibility comes from the contract's restrictionEligible field, not the “RTO” name. AT-X06 covers five configurations: no temperature support, different modes, fan-only, general maintenance, and restriction-ineligible RTO.

### FR-X07 Notifications and audit

- **Company request basis**: SRC-06 BIZ-08, BIZ-20, BIZ-22 — Detect faults before/when they occur and notify promptly; small low-cost in-unit devices/firmware with removal/theft protection and notifications; WhatsApp/email links to card payments/instructions.
- **Added design details**: Read status, related-screen links, and action history.

Choose recipients from the event and current business scope. Publish only quality-reviewed reports to customers; hide internal rework notes. Audit records keep the actor, role, target, reason, result, version, and correlation ID as they were at the time. AT-X07 checks that reading a notification leaves the alert unresolved, report visibility changes after acceptance, and sign-out does not change historical actors.

### Concrete common acceptance criteria

The table below governs Given/When/Then for AT-X01–X07 (IR101). Unstated setup uses demoSeed per IR92, clock 2026-09-14T01:00:00.000Z, simulator=false (IR97 item 4).

| ID | Given / When | Then |
|---|---|---|
| AT-X01-N | Open `/customer/units/unit-online-rto` unauthenticated → demoSession.signIn as customer-a → preferences.update(locale=ms, timezone=Asia/Kuala_Lumpur) | ① Redirect to `/login?returnTo=%2Fcustomer%2Funits%2Funit-online-rto` ② Show returnTo after signIn (IR82) ③ Malay display; stored UTC, units, and IDs unchanged |
| AT-X01-E | ① Browser Back after customer-a signs out ② During demo.trigger(transport, operation=units.list, outcome=DELAY, delayMs=3000, retryAfterSeconds=null, remainingCalls=1), switchMembership(hq-operator) ③ auth.previewPasswordReset for a@example.com and b@example.com ④ demoEmail=not-an-email ⑤ signIn with returnTo=https://example.com/x | ① /login, no old screen values ② Discard delayed response from old viewEpoch; no customer list on HQ screen (IR17) ③ Both return `{messageKey:auth.reset_generic, deliveryState:preview}` ④ VALIDATION ⑤ Ignore returnTo; go to role home (IR82) |
| AT-X01-B | In customer-a Session: ① Reach expiresAt−120 seconds ② Without extending, demo.trigger(session_expired) ③ Change locale to ms while an unconfirmed voice change is shown | ① Extension prompt (IR55) ② Discard screen/Query/unsaved draft; /login (D09) ③ Discard unconfirmed intent, retain input text (D09) |
| AT-X02-N | customer-a: “set Bedroom to 24 degrees” → confirm | ① voice.resolveIntent returns change (unitId=unit-online-rto, celsius=24, before=26). Same-name Space in property-home-b is out of scope and contributes zero matches (IR65) ② Zero Commands before confirmation ③ Confirmation creates one commands.create (no jobId, expectedUnitVersion=7, IR09) |
| AT-X02-E | ① customer-a: “set Bedroom to 31 degrees” → confirm ② customer-b: “set Living room to 24 degrees” ③ demo.trigger(microphone_denied) ④ Cancel change confirmation | ① commands.create returns VALIDATION (Capability.temperature 16–30; model choices at D01 priority 7), zero Commands ② Out-of-scope Space gives zero matches, unsupported, zero Commands ③ Switch to text input, zero Commands (D09) ④ Zero writes (IR09) |
| AT-X02-B | `acceptancePatches["AT-X02-B"]` (move unit-non-rto to the same “Home A > 1F > Bedroom” as room-1, displayName “Bedroom AC”). customer-a: ① “temperature Bedroom” ② Select unit-non-rto from candidates ③ Send selectedUnitId=unit-limited | ① Two candidates; identical pathLabel, so also show unitId and require text selection (IR101) ② temperature (unitId=unit-non-rto) ③ NOT_FOUND (IR65) |
| AT-X03-N | For unit-online-rto: demo.trigger(device, deviceId=device-online-rto, bindingId=binding-online-rto, kind=communication_lost, sequence=2) | ① Show separate simultaneous labels: “Offline,” “Last known power: ON (2026-09-14T00:59:30Z),” and “Unresolved warning (alert-window-a)”; do not claim running ② Show CO₂ (ppm) and emissions (kgCO₂e) as separate metrics/units |
| AT-X03-E | For unit-online-rto via demo.trigger(telemetry): ① humidity value=null ② power unit=W, value=1000 ③ Advance to 2026-09-14T01:02:31Z (181 seconds after latest temperature observedAt) | ① “Not measured,” not zero ② suspect (unit_mismatch), exclude from aggregation/control (IR12) ③ stale, with last value/time/quality (D07) |
| AT-X03-B | For unit-online-rto via demo.trigger(telemetry): ① humidity=0 ② co2=10000 ③ co2=10001 | ① “0%” ② valid ③ suspect (outside D07 range) |
| AT-X04-N | ① customer-a opens unit-online-rto ② contractor-a opens job-contractor-a ③ tech-external-a opens job-contractor-a ④ hq-operator opens /admin | Each screen shows success |
| AT-X04-E | ① customer-a opens `/customer/units/unit-tenant-b` (other tenant) ② customer-a opens `/customer/units/unit-other-customer` (same tenant, other customer) ③ contractor-b calls jobs.get(job-contractor-a) (other contractor) ④ While tech-external-a views job-contractor-a, demo.trigger(qualification_revoked, demo_indoor) → start from the open screen ⑤ With `acceptancePatches["AT-X04-E.5"]`, tech-internal-a starts job-t07 → saveDraft using `shared:report-draft-all-normal` → submit → switchMembership to the same user's hq-self-approver → accept via jobs.review ⑥ After tech-external-a starts job-contractor-a, advance to assignment-contractor-a.validUntil, exactly 2026-09-20T00:00:00.000Z → saveDraft | ①② not-found while keeping URL (IR57) ③ NOT_FOUND ④ jobs.start from open screen returns FORBIDDEN (qualification rechecked each action, SR03/D01 priority 4), zero business changes ⑤ FORBIDDEN (self-approval, D01 priority 4), Job remains submitted ⑥ FORBIDDEN (errors.assignment_ended; half-open work window, IR94) |
| AT-X04-B | ① hq-operator uses members.save to change tech-external-a.validUntil to 2026-10-15 (scopeVersion=2); Repository test calls commands.create(jobId=job-contractor-a) with old Context (scopeVersion=1) ② Instead, remove control.diagnose from permissions (scopeVersion=2), then commands.create with old Context ③ Advance to customer-a Membership.validUntil (2026-10-01T00:00:00.000Z), then units.list | ① CONFLICT (errors.scope_changed), zero Commands ② FORBIDDEN (authorization first, D01 priority 4) ③ UNAUTHENTICATED (errors.membership_inactive), /login (IR101) |
| AT-X05-N | customer-a sends set_temperature 25 to unit-online-rto → switchMembership(hq-operator) and open the same unit | Same commandId and Unit.version; role switch does not reset business data |
| AT-X05-E | ① While response is delayed by demo.trigger(transport, operation=commands.create, outcome=DELAY, delayMs=3000, retryAfterSeconds=null, remainingCalls=1), demo.reset ② Reload page | ① Discard old-generation response; do not display it (IR17). Zero Commands in new generation ② Return to seed, Session=null, /login, locale=en (D09) |
| AT-X05-B | ① demo.reset twice in a row ② Record external network requests during AT-X05-N | ① Second reset has the same seed IDs/versions ② Zero requests to external payment/notification/IoT/trading origins |
| AT-X06-N | customer-a opens unit-online-rto control panel (ventilation-demo v3) | Temperature 16–30, modes cool/dry/fan, fan speeds low/mid/high, ventilation enabled |
| AT-X06-E | With `acceptancePatches["AT-X06-B.1"]`, customer-a directly calls commands.create(set_temperature 25) for unit-non-rto, bypassing UI | VALIDATION (model choices at D01 priority 7), zero Commands |
| AT-X06-B | ① `acceptancePatches["AT-X06-B.1"]` (no temperature support) ② `["AT-X06-B.2"]` (cool only) ③ `["AT-X06-B.3"]` (fan only) ④ Seed unit-non-rto (general maintenance under contract-general-a) ⑤ `["AT-X06-B.5"]` (RTO contract contract-rto-x06 with restrictionEligible=false); hq-restriction-manager calls restrictions.schedule | ① Temperature control disabled with reason ② Only cool mode ③ Only fan mode; temperature disabled ④ Controls work without RTO billing ⑤ VALIDATION (fieldErrors.contractId, errors.restriction_ineligible, IR101), zero Restrictions |
| AT-X07-N | customer-a marks notif-alert-window-a as read | Save readAt; alert-window-a remains open; summaries.get alertCount remains 1 (IR51) |
| AT-X07-E | Internal job from AT-C09-N: after tech-internal-a submits, customer-a calls jobs.get → hq-operator returns via jobs.review with reason → tech-internal-a resubmits → hq-operator accepts → customer-a calls jobs.get | Before acceptance, reportRefs empty and no body. After acceptance, show only accepted version, without return reason (IR42) |
| AT-X07-B | customer-a calls commands.create → signOut → hq-operator calls audit.list | Audit actorId, actorRoleAtTime=client, correlationId, and result=success remain unchanged after sign-out |

See [frontend input/output contracts](../02-design/implementation-contracts.md) for detailed interactions. Assess non-functional requirements with numerical targets and the measured environment; adding a library alone does not meet them.

## Implementation conditions defined in 0.8.0

This version resolves ambiguity in existing FRs under DEC-11; it adds no feature scope. Use this document's permission matrix for Actor/scope, the operation catalog for required permissions, each DD/canonical DTO for Trigger/Input/Processing/Output/success conditions, and deterministic contracts D01–D13 for Failure/Validation/Error handling.

FR-X01/X02 require D09's 30-minute session and fixed en/ms grammar; FR-X03/X05 use D05/D07 connectivity/quality/subscriptions; FR-X04 uses D01/D06/D12 authorization/projections; FR-X07 uses D08 recipients/notification links. NFR01/02/04 use D10 measurement environment, counts, maximums, and accessibility acceptance; NFR03/05 use D04/D08 retries/safe display; NFR06 uses D11 production separation; NFR07 uses verification plans/evidence; NFR08 uses D09 fixed display currency. Do not add new User CRUD, real payments, real voice, or real IoT control.

0.9.0 correction contracts: Read [strict review correction contracts](../02-design/strict-review-contracts.md) and [operation version contracts](../02-design/write-version-catalog.csv) together.

Additional current 0.21.0 contracts: Read [re-review correction contracts](../02-design/review-resolution-contracts.md) IR01–106. They override older text on the same issues; use IR72 for conflict priority.
