---
document_id: DD-COMMON
version: 0.21.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# Common Detailed Design

Inputs: [Common Requirements](../01-requirements/common.md) and [PrepareDocument](../00-prepare/PrepareDocument.md). Also read the [Frontend Input and Output Contract](implementation-contracts.md) for exact inputs, outputs, errors, and demo timing, and the [Operation Catalog](operation-catalog.csv) for all logical operations (named processes). This document defines only browser screen data, mock services (simulated data processing), and display states. API paths, databases, authentication servers, and backend business processing are outside its scope.

The business goals in this document come from the BIZ items drawn from the [Original Company Requirements](../00-prepare/sources/company-requirements-original.txt) and the [Common Requirements](../01-requirements/common.md). Types, repositories (data access interfaces), caches (temporary storage), permission guards (access limits), and mock states are proposed frontend designs to meet these goals. When a real API is added, adapters will handle conversion to display data. This document does not define server authentication, databases, or communication contracts.

The features, screen fields, states, and exceptions in this design follow the original company requirements and their linked requirements. This document defines the processing and acceptance criteria for each FR (functional requirement). Reference mock screens are used only to guide the appearance of the shared UI.

**Implementation baseline for 0.21.0**: Read all chapters of the [Deterministic Contracts](deterministic-contracts.md) and strict-review-contracts.md, the authorization columns of the operation catalog, and the screen catalog together. Do not guess values, permissions, asynchronous behavior, or recovery during implementation. These are demo design proposals, not approval for production business use.

## 1. Structure and Responsibilities

```text
src/
  app/                router, providers, composition-root, route-guards
  features/           units, jobs, billing, restrictions, energy, devices...
    <feature>/        pages, components, queries, forms, schemas
  domain/             entities, policies, transitions, repository-contracts
  infrastructure/
    mock/             repository, fixtures, scenario-clock, event-bus
    adapters/         replacement point for external data (interface only in this phase)
  shared/
    ui/               shadcn-based UI primitives
    components/       shared business displays such as StatusBadge and MetricCard
    styles/           tokens.css
    config/           demo-settings, locale-settings
    i18n/             en, ms
tests/                unit, component, contract, e2e
```

Data dependencies run in this order: `page → feature hook → Repository interface → injected adapter`. The domain layer (business rules) depends on neither React, HTTP, nor mocks. Pages must not use fetch, mock seeds (initial data), or localStorage directly. Only the composition-root selects the adapter.

Screen navigation follows the same approach. Pages and feature hooks must not call React Router APIs (`useNavigate`, `useParams`, `useLocation`, etc.) directly. Instead, they use a small Navigation interface under `shared` (for example, `navigateTo(routeKey, params)` and `getParam(name)`). Only the composition-root knows its React Router implementation. As with the Repository pattern, this separates the interface from its implementation, so a future router change affects only the Navigation interface implementation.

The proposed stack (PROPOSED) is TypeScript (strict mode), React, Vite, and React Router. The app will be an SPA (single-page application) because server-side HTML rendering (SSR) is not required. Direct URL access (deep links) will require an SPA fallback setting on the future hosting service. At implementation start, check library version compatibility and fix versions in a lockfile. App startup commands do not exist yet, so this document does not claim that the app has been tested.

## 2. Shared Routes and Screen States

| Route | Responsibility |
|---|---|
| /login | Select a fictional account and one of four roles. Show that this is not real authentication |
| /forgot-password | Check the email format, then show a message that does not reveal whether the email exists. Sending is a preview only |
| /settings/preferences | Language, display timezone, read-only demo currency MYR, and consent settings |
| /notifications | List notifications within the user's scope and manage read status. Keep this separate from business state |
| /demo | Switch scenarios, trigger failures, control the demo clock, and reset. Demo-only screen |
| /forbidden | Show access denial. Do not redirect automatically; show a link to the role home (IR57) |
| `*` (undefined routes) | Show page not found (SCR-X-not-found), using the D01 not-found text. Do not redirect automatically; show a link to the role home (IR57) |

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
| MaintenanceJob | unitId、alertIds[]、type(periodic/reactive/preventive)、status、contractorOrgId?、assignmentId?、requestedSlot、scheduledSlot?、dueAt、reportVersion?、draftReportRef?、costs[] |
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

Separate layers in this order: `page → feature hook → Repository interface → mock adapter`. Repository here means a group of asynchronous services called by the frontend, not a database access layer. This phase builds only the interface and a mock adapter using shared memory.

```ts
interface CommandRepository {
  create(context: DemoViewContext, input: CreateCommandInput,
      options: DemoWriteOptions): Promise<ServiceResult<Command>>;
  get(context: DemoViewContext, commandId: string,
      signal?: AbortSignal): Promise<ServiceResult<Command>>;
}
```

`DemoViewContext` describes the selected fictional user, role, and visible scope. `DemoWriteOptions` carries a key to prevent duplicate demo actions and the version before the change. These do not define production authentication or server permissions. See the [Frontend Input and Output Contract](implementation-contracts.md) for the fields.

The [Operation Catalog](operation-catalog.csv) lists the 137 local service operations needed by the screens, with their inputs, return values, and screens that use them. It does not define URLs, HTTP methods, database tables, or server transactions.

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
| requested | HQ assigns an internal staff member | assigned |
| requested | HQ offers the job to an external contractor | offered |
| offered | The selected contractor accepts or declines within the valid period | accepted / requested (keep the decline in history) |
| offered | offerExpiresAt is reached without a response (IR48) | requested (keep the Offer as expired) |
| accepted | The contractor assigns an active technician from its own company | assigned |
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

API paths, HTTP methods, databases, server authentication and authorization, real payments, real notifications, and real device control are outside this document's design scope. Their open status does not prevent completion of these frontend documents.

0.9.0 correction contracts: Read the [Strict Review Correction Contracts](strict-review-contracts.md) and [Per-Operation Version Contract](write-version-catalog.csv) together.

Additional contracts for current version 0.21.0: Read IR01–106 in the [Re-review Correction Contracts](review-resolution-contracts.md). They take priority over older text on the same topic; follow IR72 for conflict precedence.
