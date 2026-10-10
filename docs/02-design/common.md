---
document_id: DD-COMMON
version: 0.85.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# Common Detailed Design

Inputs: [Common Requirements](../01-requirements/common.md) and [PrepareDocument](../00-prepare/PrepareDocument.md). Also read the [Frontend Input and Output Contract](implementation-contracts.md) for exact inputs, outputs, errors, and demo timing, and the [Operation Catalog](operation-catalog.csv) for all logical operations (named processes). This document defines only browser screen data, mock services (simulated data processing), and display states. API paths, databases, authentication servers, and backend business processing are outside its scope.

The business goals in this document come from the BIZ items drawn from the [Original Company Requirements](../00-prepare/sources/company-requirements-original.txt) and the [Common Requirements](../01-requirements/common.md). Types, repositories (data access interfaces), caches (temporary storage), permission guards (access limits), and mock states are proposed frontend designs to meet these goals. When a real API is added, adapters will handle conversion to display data. This document does not define server authentication, databases, or communication contracts.

The features, screen fields, states, and exceptions in this design follow the original company requirements and their linked requirements. This document defines the processing and acceptance criteria for each FR (functional requirement). Reference mock screens are used only to guide the appearance of the shared UI.

**Implementation baseline for 0.22.0**: Read all chapters of the [Deterministic Contracts](deterministic-contracts.md) and strict-review-contracts.md, the authorization columns of the operation catalog, and the screen catalog together. Do not guess values, permissions, asynchronous behavior, or recovery during implementation. These are demo design proposals, not approval for production business use.

## 1. Structure and Responsibilities

As built (IR257; the Next.js 16 guides, IR175–IR178, IR199 onwards). `service/web` is an npm workspace with one Next.js App Router app per entry point (IR178) and the shared package `@ac/web`:

```text
service/web/
  customer/ partner/ technician/ admin/   one app each: routes, the role layout, proxy.ts, Server Actions
  shared/ (@ac/web)
    components/   AppShell, ui primitives (Tailwind CSS 4, Figma UI Guideline tokens), shared parts (JobBits, QrScan …)
    lib/          dal.ts (server-only data access), session, rest + routes.gen (Core API REST routes from the catalog),
                  pure mappers and validators per screen (Vitest), URL state, the Phase 1A demo stores
    screens/      shared screens: sign-in, notifications, preferences, demo, page unavailable
    bff/          Route Handlers: OIDC sign-in, callback and sign-out; /bff/ops for interactive reads; session
  e2e/            Playwright end-to-end tests against the local stack (IR252–IR256)
```

**Data in API mode (`DATA_SOURCE=api`).**
- Reads: a route's `page.tsx` is a Server Component. It reads through `lib/dal.ts`, which is `server-only`: the session comes from the app's signed cookie, and each operation goes to its Core API REST route. The page passes plain rows to Client Components. Mapping DTOs to view rows is done by pure functions in `shared/lib`.
- Writes: Server Actions in `actions.ts` next to the route. Each one checks its input, calls the operation with the row version (`X-Expected-Version`) and an Idempotency-Key, and calls `refresh()` so the route renders again. Field errors return to the form.
- Interactive reads: reads that follow a gesture, such as the technician's QR scan, go through the BFF Route Handler `/bff/ops/<operation>`.
- The browser never calls the Core API.

**Phase 1A demo (`DATA_SOURCE` unset).** The same routes render the demo components with in-browser fixture rows; nothing reaches a network. The shared demo stores (jobs, client users) live in the tab (`lib/demoStore`); their actions run in the tab and return refusals at once (IR309). The four apps of a tab on one origin see the same demo data, and switching roles keeps it; a reload or the reset on `/demo` brings back the seed (FR-X05).

**Navigation.** `next/link` and `next/navigation` are used directly. Filters, tabs and the selection live in the URL: `lib/urlState.ts` reads them through `useSyncExternalStore`, and `useUrlPatch` writes them, replacing the history entry. One router serves every app, so the earlier Navigation-interface proposal is not used.

**UI and forms — DEC-03, decided 2026-10-10.** The SRC-02 production condition “shared libraries, reactForms” has no archived original. DEC-03 first proposed shadcn/ui, Lucide, React Hook Form, Zod and TanStack Query. On 2026-10-10 the product owner decided to keep the build's approach, which follows the Next.js guides; the role designs and the UIUX specification (UX-01–UX-03) now describe it:
- UI: in-house components on Tailwind CSS 4 (`shared/components/ui.tsx`) that follow the Figma UI Guideline, with glyph icons.
- Forms: controlled inputs, pure validators per form in `shared/lib`, and the Core API's field errors from Server Actions.
- Translation: plain dictionaries chosen on the server, as in the Next.js internationalization guide (UX-01).

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

**Language — built for every business screen and the assistant (IR258–IR306).** FR-X01 asks that a language change reach key screens, notifications and dates.
- These show the saved language: the shell, a failed Server Action's toast, each route's loading and error states, Preferences, Demo controls, the notifications inbox and every customer screen (AT-X01-N) — overview, units & locations (with group control), automations, alerts (inbox and policies), maintenance (requests and Filter care), energy & cost with carbon offsets, air quality, contracts & payments with the invoice page, users and the unit screen — and the contractor overview, job list, offer / job page, quality review, schedule, team & capacity with its certifications, job history, payouts and the unit view — every contractor screen — and the technician overview with its QR scan, job workspace, unit screen, alert evidence, diagnostic control and devices with their events — every technician screen — and the HQ overview, maintenance jobs with their plans, contractors and SLA tabs, the alerts with their policies, customers & units with warranty & coverage, the device registry, the energy analysis, the MRV workspace, the offset demo, billing with contractor payouts, contracts, restrictions with their exception screen, access & roles, automation policies and the audit log — every business screen.
- The English text is the key of the Malay dictionary (`shared/lib/i18n-ms.ts`, a draft not yet reviewed). A text without an entry stays English. When one English text means two things, its key names the context before `::` ("tamper::Clear" is a state, "Clear" a button); English shows the text after it (IR287).
- Their times follow IR44: the user's language and display time zone, with the zone's abbreviation (`showTime` / `showClock`). Times on the days next to now show "today", "yesterday" or "tomorrow" instead of the date (`relativeTime`). Business days stay in Asia/Kuala_Lumpur, where they are cut, and the screens say so: the periods of the overview and the energy screens, the 7-day air-quality window, contract periods and invoice due dates (HQ billing and contracts too, with the payout pay dates), the contractor's timeline, team weeks, free hours and unavailable days, the technician's today and its timeline, a follow-up visit date, a unit's 7-day period, HQ's classify-by business day, the contractor register's dates, and a unit's installation and warranty dates (named when the display time zone is another one), and HQ's energy analysis, baseline, MRV report and offset quote periods, which HQ also types in Kuala Lumpur time (IR296–IR298). Booked and preferred times show as one span (`showSpan`), and the times users type — preferred and proposed times, an offer's expiry and access window, an extended work or access end, a plan's next date, an SLA target's start, a firmware campaign's start, a restriction's execute-after and a grace or exception end, a membership's valid period, the audit log's period days — are read in the display time zone (`zonedInstant`, NFR-08).
- The customer assistant speaks its own chosen language (IR306). It starts in the display language, and its header switches between English and Bahasa Melayu. A switch discards an unconfirmed change and keeps the typed text (D09). Its suggestions are sentences of the fixed grammar.
- Still English: the Phase 1A browser demo's fixture screens, including its simulated assistant.

**Tests.** Vitest covers the shared mappers and validators, the BFF session and the DAL (IR307), and checks the web's calls against the Core API contract (IR312). Playwright end-to-end tests run against the local stack. The Core API has its Go integration and unit tests.

The stack is TypeScript (strict mode), React 19 and Next.js 16 App Router (DEC-67, DEC-71) with Tailwind CSS 4. Each app is its own container (container design) with its own session cookie (IR175).

## 2. Shared Routes and Screen States

| Route | Responsibility |
|---|---|
| /login | Select a fictional account and one of four roles. Show that this is not real authentication |
| /forgot-password | Check the email format, then show a message that does not reveal whether the email exists. Sending is a preview only |
| /settings/preferences | Language, display timezone, read-only demo currency MYR, consent settings (client only), monthly report email (client), and Security › two-step verification (demo, FR-X08, IR112) |
| /notifications | List notifications within the user's scope and manage read status. Keep this separate from business state |
| /demo | Switch scenarios, trigger failures, control the demo clock, and reset. Demo-only screen |
| /forbidden | Show the shared “Page unavailable” view (same component as `*`). Do not redirect automatically; show a link to the role home (IR57, IR112) |
| `*` (undefined routes) | Show the shared “Page unavailable” view (SCR-X-not-found), using the D01 not-found text. Do not redirect automatically; show a link to the role home (IR57) |

The shared header contains language selection, notifications, voice/text switching, and sign-out. Demo role switching has its own menu, separate from changing users in normal business use.

Use the following screen states.

- loading: Show a skeleton.
- empty: Show an explanation and the next action the user can take.
- error: Show a retry button.
- offline: Show the last update time and why actions are unavailable.
- stale: Note that the value is old.
- refreshing: Keep current data visible, show “Refreshing” and aria-busy, and do not return to a skeleton (IR83).
- work-not-started (before the work window): Show technician jobs as read-only, with the start time and disabled actions (IR76).
- Rendering exception: The shared ErrorBoundary shows a full-screen error with correlationId, rather than a blank screen (IR44).

Do not show an empty state to hide a lack of viewing permission. Follow IR57 for where to show FORBIDDEN and NOT_FOUND and how to navigate. Use similar wording for 404 (not found) and 403 (forbidden), so the message does not reveal whether the resource exists.

## 3. Frontend Screen and Mock Models

The following are not database table definitions. “Save” and “history” always mean keeping data in shared mock memory within the same browser tab.

Apply these common rules.

- `id: string` (identifier), `tenantId: string` (tenant identifier).
- All times use UTC in ISO 8601 format.
- Mutable data has `version: number` (version number).
- Demo IDs must also remain unchanged after creation.

The schema distinguishes null (no value) from “not registered.” Treat unknown enum values as UNAVAILABLE under D01.

| Entity | Main fields and relationships |
|---|---|
| User / Membership | userId, organizationId, role(client/contractor/technician/admin), employment(internal/external/null), permissions[], scopes[], validFrom, validUntil. Do not store a single role directly on User |
| Organization / Customer | name, kind(customer/contractor/operator), status / organizationId, serviceProfile. Keep customer IDs separate from User IDs, and create customers only within the managed tenant |
| ContractorOrganization / Assignment | contractorOrgId, jobId, technicianMembershipId, delegatedScope, validFrom/Until, status. Keep the delegating tenant separate from the receiving company's ID |
| Property / Space | customerOrgId, kind(home/office), name / propertyId, parentSpaceId, kind(area/floor/room/space). Prevent cycles in the parent-child hierarchy |
| ACUnit / Capability | spaceId(null means no space assigned, IR62), connection/lastSeenAt(derived from Device, IR47), manufacturer, model, type(split), installedAt, serviceScope / temperature(min,max,step), modes[], fanLevels[], ventilation, control, sensors[] |
| Device / Sensor | unitId、serial、connection(online/offline/unknown/connecting/error)、lastSeenAt、firmwareVersion / deviceId、metric、unit、calibrationAt、staleAfterSeconds |
| Telemetry | unitId、sensorId、metric、value:number\|null、unit、observedAt、receivedAt、origin(measured/estimated/inspection)、quality(valid/missing/stale/suspect)、isDemo |
| Command | unitId, actorId, action:UnitAction or internal RestrictionAction, diagnosticRunId?, status, requestedAt, sentAt?, acknowledgedAt?, expiresAt, failureCode?, idempotencyKey(key to prevent duplicates), expectedVersion, correlationId |
| Alert | unitId, type, severity(critical/warning/normal), status, evidenceIds[], detectedAt, acknowledgedAt?, resolvedAt?, resolutionReason?. normal is also used in health summaries. Do not change an open alert to normal without a valid basis |
| MaintenanceJob | unitId、alertIds[]、type(periodic/reactive/preventive)、origin(client_request/periodic_plan)、status、contractorOrgId?、assignmentId?、requestedSlot、preferredSlots[]、slotProposal?、partnerSlotProposal?、scheduledSlot?、dueAt、reportVersion?、draftReportRef?、costs[] |
| WorkReport / InspectionItem / Attachment | jobId, authorId, version, items[], measurements[], replacementParts[], workText, nextAction, submittedAt? / componentGroup, componentKey, result, reason?, evidenceIds[] / jobId, reportId, blobId, name, mime, size, status(previewUrl is created only in the screen) |
| Contract / Invoice / Payment | customerOrgId、unitIds[]、planType、period、rulesVersion / contractId、amountMinor、currency、dueAt、status / invoiceId、amountMinor、method?、status、externalRef?、confirmedAt? |
| Restriction | contractId、causeInvoiceIds[]、unitIds[]、rulesVersion、noticeAt、executeAfter、reason、policy、state、applyCommandIds[]、releaseCommandIds[]、exception?、graceUntil? |
| Automation / Consent | unitIds[]、condition(discriminated union)、action、priority、timezone、enabled / purpose、granted、grantedAt?、revokedAt? |
| Notification / AuditEvent | recipientId、channel(inApp/email/whatsapp)、templateKey、params、readAt?、deliveryState(preview/simulated/failed) / actorId、action、targetId、before?、after?、reason?、result、correlationId、occurredAt |
| EnergyBaseline / EmissionFactor | unitIds[]、period、method、version、kWh、boundary / region、year、kgCO2ePerKWh、source、version、isDemo |
| MRVReport / OffsetRecord | period、baselineId、factorId、boundary、coverage、totals、evidenceIds[]、reviewHistory[]、status(draft/demo_reviewed) / amountKg、state(quoted/demo_requested/demo_purchased/demo_retired/failed)、demoCertificateRef?、isDemo |

The table above summarizes shared data. For the added fields for “possible causes,” “allergens,” “payment methods,” “Scope 2,” and “market concept,” read [Display Model Additions](implementation-contracts.md#ddc-source-additions-display-models-for-company-requests) and the relevant DD (detailed design). Invoice display data derives paymentMethod/paymentStatus from Payment records; do not store the same information twice.

Telemetry's `isDemo` is separate from whether data is measured or estimated. Even demo “measured” values must be clearly marked as fictional. Use only fictional contacts and photos. Store images in the shared mock Blob store, and reference them from Attachment using blobId. Revoke screen-created object URLs when leaving a screen, switching roles, or signing out. Check permission before recreating them for display. Keep submitted Blobs on sign-out and release them on reset. See input/output contract DDC-08 for details.

## 4. Frontend Data Service Boundaries

As built (IR257): in API mode the server-side DAL (§1) calls each operation at its Core API REST route and gets the data or a DomainError. The Phase 1A demo renders fixture rows and the shared demo stores. The Repository interface below is the Phase 1A proposal; its contract still holds — ServiceResult, DomainError, ignoring late responses.

Phase 1A proposal: separate layers in this order: `page → feature hook → Repository interface → mock adapter`. Repository here means a group of asynchronous services called by the frontend, not a database access layer. This phase builds only the interface and a mock adapter using shared memory.

```ts
interface CommandRepository {
  create(context: DemoViewContext, input: CreateCommandInput,
      options: DemoWriteOptions): Promise<ServiceResult<Command>>;
  get(context: DemoViewContext, commandId: string,
      signal?: AbortSignal): Promise<ServiceResult<Command>>;
}
```

`DemoViewContext` describes the selected fictional user, role, and visible scope. `DemoWriteOptions` carries a key to prevent duplicate demo actions and the version before the change. These do not define production authentication or server permissions. See the [Frontend Input and Output Contract](implementation-contracts.md) for the fields.

The [Operation Catalog](operation-catalog.csv) lists the 200 local service operations needed by the screens, with their inputs, return values, and screens that use them. Its `rest_routes` column gives each operation's Core API REST route (IR222); database tables and transactions are in the [database design](database-design.md).

Successful mock operations return ServiceResult<T>; failures reject with DomainError. Pending processing is shown through Command.status or similar fields in the success DTO. Do not create a custom pending Promise response type.

The screen uses the returned DomainError to decide what to display, whether to keep inputs, and whether to reload. Use the operation ID and view generation to detect and ignore old responses that arrive late. Never render values from before a role switch afterward.

When a real API becomes available, keep the interface used by screens and use a new adapter to convert external responses to display data. Until the external contract is fixed, adapter replacement alone is not guaranteed to be enough. Real API specifications, authentication methods, and communication contracts are outside this phase.

## 5. State Transitions and Consistency

### Commands

| Current state | Event and guard (condition) | Next state / display |
|---|---|---|
| Not created | Check permissions, capabilities, contract restrictions, and online status before sending | requested / Show “Request accepted.” Do not change confirmed values yet |
| requested | A delivery event occurs | sent / Show “Waiting for device response” |
| requested / sent | An explicit failure occurs, or no response arrives before the deadline | failed / expired. Do not copy requested values to confirmed values |
| sent | A matching success response arrives before the deadline | acknowledged. Update only settings confirmed by the response |
| failed / expired | The user checks the state again and retries | Create a new request. Keep the old Command in history |

Record responses received after the deadline as late events in history. Do not silently change the state to success. Check the actual state again and display it separately. Reject new requests while offline. If the connection is lost after sending, wait for the response until the deadline.

### Maintenance Jobs (Separate Contractor Handling)

| Current state | Authorized person or event | Next state |
|---|---|---|
| requested | HQ assigns an internal staff member at one of the client's preferred times (IR113) | assigned |
| requested | HQ offers the job to an external contractor with a fixed agreed visit slot (IR113) | offered |
| requested | None of the preferred times fits: HQ proposes another time (`jobs.proposeSlot`) | requested (UI “Time proposed” until the client answers, IR113) |
| requested (proposal pending) | The client accepts / declines with new preferred times / does not answer by replyBy | assigned or offered automatically with the held partner / requested (next round) / requested (expired, HQ calls the client) |
| offered | The contractor proposes another time (`jobs.proposePartnerSlot`); HQ sends it to the client or keeps the agreed slot | offered (visitSlot updated only after the client accepts, IR113) |
| offered | The selected contractor accepts or declines within the valid period | accepted / requested (keep the decline in history) |
| offered | offerExpiresAt is reached without a response (IR48) | requested (keep the Offer as expired) |
| accepted | The contractor assigns an active technician from its own company at the offer's visit slot | assigned |
| assigned | The assigned technician accepts the assignment (受領) or reports it cannot make the time (`jobs.acknowledgeAssignment`) | assigned (acknowledgement accepted / cant_make; coordinator reassigns or HQ proposes a new time, IR113) |
| assigned | The assigned technician starts work within the valid period | in_progress |
| assigned / in_progress | HQ (internal work) or the receiving contractor (outsourced work) changes the assignee with a reason | Keep the state. Invalidate the old assignment and create a new one. Keep the original author on work-in-progress records |
| in_progress | The assignee submits the required report | submitted |
| submitted | The contractor's quality reviewer (outsourced work) or HQ (internal work) reviews it | completed / rework_requested |
| rework_requested | The assignee starts rework (`jobs.resumeRework`) | in_progress (keep the previous report version) |
| requested | The client who requested maintenance or HQ cancels with a reason (IR56) | cancelled |
| offered / accepted / assigned | HQ cancels with a reason | cancelled. Invalidate related assignments too |
| in_progress / submitted | HQ pauses work with a reason | on_hold. Do not automatically complete or cancel it. Direct cancellation from either state returns CONFLICT (IR56) |
| on_hold | HQ checks the current situation and resumes (`jobs.resumeHold`, reason required) or ends the work | in_progress / cancelled (reason and incomplete-work record required) |

Additional work after completed is a separate request with a new jobId. After reassignment, the new assignee creates a new report version from the current draft and continues work. Do not change old versions or the original author of each inspection. Authors cannot approve their own outsourced reports, including by switching Membership while keeping the same userId. If the contractor has no quality reviewer, escalate to HQ. Formal final customer approval remains undecided in OPEN-01, so phase 1A supports only viewing results and making inquiries.

Alert moves through open → acknowledged → resolved. Direct open → resolved is also allowed for sustained recovery under D08 (only Alerts with policyId≠null, IR66) or authorized manual resolution. Follow IR66 to identify the same event and link earlier Alerts. If the same problem returns after resolved, create a new Alert linked by previousAlertId. Resolution requires a new measurement that meets the conditions, or confirmation with a reason by HQ or an authorized diagnostician. A Job reaching completed alone must not automatically resolve an Alert.

### Payments and Restrictions

Payment moves through initiated → processing → confirmed/failed. Offset moves through quoted → demo_requested → demo_purchased → demo_retired. On failure, set failed and record the previous state. Reject retirement before purchase and duplicate retirement. Invoice moves through unpaid → processing → paid (when payment is confirmed). On failure, return to unpaid. Derive overdue status from dueAt and the unpaid amount. Installments and refunds are outside phase 1A and will be defined in phase 1B.

| Current state | Event and guard (condition) | Next state / notes |
|---|---|---|
| Not created | The contract permits restrictions; payment is unpaid; permission, reason, and prior notice exist | scheduled |
| scheduled | The deadline arrives; recheck that there is no grace period or exception and payment is still unpaid | requested; create an apply Command for each unit |
| scheduled | All cause invoices have confirmed payment, or cancellation, grace, or an exception applies | cancelled / scheduled (change the date or pause application, with a reason) |
| requested | Target units acknowledge application | applied if all succeed. If some have not responded, keep requested and show each unit's state |
| requested | Offline, failure, or expiry | Keep requested, pendingReason, and command results. Do not mark success or resend automatically |
| requested / applied | All cause invoices have confirmed payment; grace or an exception is set; forced release or cancellation occurs (IR96); or an authorized person explicitly calls restrictions.release (IR35) | release_requested. In the same transition, evaluate each unit for release under D03 and create a remove Command for online units with applied restrictions. restrictions.release is idempotent in release_requested |
| release_requested | Evidence confirms release or definite non-application for every target (D03) | released |
| release_requested | Offline or failure | Show pending. Recheck or explicitly retry |

When cancelling from requested onward, follow the IR96 state table for release; do not assume nothing has been applied. If grace or an exception is added after applied, create a release request when needed. After release is requested, a late apply-success response must not return the state to applied. Reconcile each unit's observed values with the release request again. An override does not change payment records.

Keep Command and Restriction, Job and Alert, and Invoice and Payment as separate records. Update related records on events, but do not merge their states. The screen must clearly explain partial success, pending status, and actual device application separately.

### Devices and Automation

A firmware update Operation moves through queued → running → succeeded/failed. Check online status, capabilities, and the target version. Update firmwareVersion only on succeeded. Keep calibration history with the reference value, unit, time, and operator. If control and an update overlap, the demo policy rejects control actions during the update.

Automation follows this priority order.

1. Device capabilities and the result of applying restrictions and exceptions.
2. HQ policy.
3. Customer rules.

Within one level, a higher priority number wins; ties use ascending ID order (DEC-09). Do not trigger automation when condition data is unavailable or consent has been withdrawn. Every path ultimately passes through the same Command policy, so voice actions and automation cannot bypass restrictions.

## 6. Queries and Demo State

Query keys combine `[repositoryInstanceId, generation, viewEpoch, tenantId, membershipId, scopeVersion, resource, id, normalizedFilters, normalizedSort, cursor, limit]`. On role switch or sign-out, cancel old requests and subscriptions and clear the cache before showing the next screen. Update scopeVersion when permissions change.

Only one mock Repository exists per browser tab. It holds normalized Maps and an event sequence; do not copy these into screen useState. On an update event, invalidate and refetch only the read-operation Queries listed in IR71. For example, report-completion scenario S02 updates the job and history, but waits for a separate event before resolving an Alert.

The demo clock runs from the seed time at real-time speed. At each minute boundary, the IR45 liveness simulator generates synthetic measurements and heartbeat signals. `demo.advanceClock` jumps forward without consuming session lifetime (IR36). Reloading the screen restores the initial demo seed (DEC-07). Sign-out clears viewed caches and unsaved photos but keeps shared fictional business data. Reset returns the demo clock, delayed work, subscriptions, image URLs, caches, and business data to their initial state. Use generation numbers to ignore late events from before reset.

Make clocks, ID generation, and success/failure results replaceable. Avoid tests that depend on random values or real time. The /demo screen must explicitly trigger failures, connection loss, device removal, payments, and responses. fixture-contract.json's demoSeed is the source of truth for initial business data; IR69 defines test overlays. Provide the following fictional test data.

- Tenants: 2.
- Clients: 2.
- Contractors: 2.
- Internal technicians: 1; external technicians: 2.
- HQ: Memberships for both normal permissions and restriction-operation permissions.

## 7. Calculations, Units, and Data Quality

- Energy [kWh] is power [kW] accumulated over time. For demo calculations from time-series data, define sample intervals and rules for excluding missing data. Clearly state any conversion from W (watts).
- Estimated cost is energy × a fictional unit price. If rates vary by time, add the costs for each interval. State when the demo excludes taxes or fixed charges.
- Emissions [kgCO₂e] are kWh × factor [kgCO₂e/kWh]. The factor's region, year, version, source, and calculation boundary are required. Do not present a fictional factor as an official real value.
- Reduction is baseline under matching comparison conditions − actual result. Reduction rate is difference ÷ baseline × 100; it is undefined when the baseline is 0. Show negative reductions as increases.
- coverage is valid sample count ÷ expected sample count. If the expected count is 0, show “Not calculated.” Note quality issues in summaries and reports. Do not fill missing data with 0.
- Indoor CO₂ concentration [ppm] is not used in the emissions calculation above. Expected energy savings are not guaranteed values.

## 8. Interfaces Kept for a Future API

- TypeScript types for screen inputs and return values, and asynchronous interfaces.
- Replaceable adapters that convert external data to display data.
- UI loading/error/empty/pending/confirmed states, and rules for discarding views on role changes.
- Frontend verification using simulated responses.

API paths, HTTP methods, databases, server authentication and authorization, real payments, real notifications, and real device control are outside this document's design scope. Their open status does not prevent completion of these frontend documents. The production target is designed separately (PROPOSED, AWS per DEC-68 / IR117) in the [backend architecture](backend-architecture.md) and [network architecture](network-architecture.md) (IR116); those documents do not change Phase 1A.

0.9.0 correction contracts: Read the [Strict Review Correction Contracts](strict-review-contracts.md) and [Per-Operation Version Contract](write-version-catalog.csv) together.

Additional contracts for current version 0.85.0: Read IR01–IR312 in the [Re-review Correction Contracts](review-resolution-contracts.md). They take priority over older text on the same topic; follow IR72 for conflict precedence.
