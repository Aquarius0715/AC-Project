# AC Project Frontend Development Documents

Version: 0.74.0 / Created: 2026-09-14 / Updated: 2026-10-08 / Status: Draft for review / Language: English

These documents cover a clickable frontend demo (1A) for monitoring, controlling, maintaining, and managing contracts for Split Unit AC. The 1A specification covers frontend design only; API specifications, HTTP contracts, databases, server processing, and production operations are not 1A inputs, and the frontend defines replaceable interfaces for them. The production backend and network are designed separately as PROPOSED targets on AWS (DEC-68, IR117) in the [backend architecture](02-design/backend-architecture.md) and [network architecture](02-design/network-architecture.md) (IR116). Application implementation is also outside the scope of this documentation work.

Document authors and final decision makers for the 1A specification: **Masaki Kitano and Yuma Wakai**. See [DEC-12](00-prepare/internal/decision-record-2026-09-16.md) for decisions on the mock demo scope and business rules. People and external reviewers perform the final check before deployment.

## Reading order

| Order | Document | Purpose |
|---|---|---|
| 1 | [PrepareDocument (for the company)](00-prepare/PrepareDocument.md) | Organize and analyze the company's original requirements and explain how to apply them to the frontend |
| 2 | [Common requirements](01-requirements/common.md) | Business boundaries, nonfunctional requirements, and permissions for all roles |
| 3 | Requirements by role: [Client](01-requirements/client.md) / [Contractor](01-requirements/contractor.md) / [Technician](01-requirements/technician.md) / [Administrator](01-requirements/admin.md) | Requirement IDs, priorities, and acceptance criteria |
| 4 | [Common detailed design](02-design/common.md) | Display data, replaceable interfaces, state transitions, and mock design |
| 5 | Detailed design by role: [Client](02-design/client.md) / [Contractor](02-design/contractor.md) / [Technician](02-design/technician.md) / [Administrator](02-design/admin.md) | Screens, input, processing, permissions, and error cases |
| 6 | [Common UIUX specification](03-uiux/UIUXSpecification.md) / [Screen catalog](03-uiux/screen-catalog.csv) / [Component contracts](03-uiux/component-contracts.csv) | Libraries, state management, tokens, accessibility, routes, tabs and URL state per screen |
| 7 | [Agentic SDLC](04-agentic-sdlc/README.md) | Agent responsibilities, gates, and handoff contracts |
| 8 | [Reference design analysis](00-prepare/reference-design-analysis.md) / [Frontend input/output contracts](02-design/implementation-contracts.md) / [Operation catalog](02-design/operation-catalog.csv) | Visual values from the source and detailed input/output definitions |
| 9 | [Verification plan](04-agentic-sdlc/verification.md) / [Traceability matrix](00-prepare/traceability.csv) | Check the links from requirements to design and tests |
| 10 | [Backend architecture](02-design/backend-architecture.md) / [Network architecture](02-design/network-architecture.md) | Production target (PROPOSED; AWS, Stripe and HQ company-network access per DEC-68 / IR117): BFF, Core API modules, data, IoT, integrations, zones and traffic flows; not Phase 1A acceptance inputs (IR116) |
| 11 | [Backend Go design](02-design/backend-go-design.md) / [Database design](02-design/database-design.md) / [db/schema.sql](02-design/db/schema.sql) / [Operation persistence map](02-design/operation-persistence-map.csv) | Production target implementation (PROPOSED): Go + Echo service layout, request pipeline, workers, tests; PostgreSQL schemas, constraints, RLS, partitions, migrations (IR118) |
| 12 | [Container design](02-design/container-design.md) / `compose.yaml` | Everything runs in Docker: images, Dockerfile standards, Compose profiles with local stand-ins for AWS services, ECS Fargate runtime (IR119) |

The four main document types are PrepareDocument, requirements, detailed design, and the UIUX specification. Requirements and design are split into four roles. Shared information is in separate files to avoid duplication. Agentic SDLC documents support execution; they are not a fifth product specification.

## Document conventions

- Sources are grouped as "company original SRC-06," "production policy SRC-02," "reference mock observations SRC-05/04," and "added design details." Content from the company's original text does not mean that its details have been approved.
- `PROPOSED`: A reversible design proposal chosen in these documents. Normal demo implementation may proceed. Record any changes in the decision log.
- `OPEN`: A business decision by a person or an external specification is still pending. Do not proceed with dependent production processing. Mock work may proceed with its assumptions stated clearly.
- `MUST` / `SHOULD` / `MAY`: Required / recommended / optional. Both P0 and P1 must be completed for 1A; implement P0 first. P2 covers production and future extensions.
- ID prefixes: `FR-C/P/T/A` = the four roles; `FR-X` = common; `NFR` = nonfunctional; `DD` = design; `UX` = common UI; `AT` = acceptance test; `DEC/OPEN` = decisions/open issues.
- Each role requirement row is one verification unit. Assign its acceptance criteria an `AT-*` ID based on the ID at the start of the row (for example, FR-C01 → AT-C01). S01–S08 are additional checks across roles.
- When specifications disagree on the same issue, follow the precedence table in [IR72](02-design/review-resolution-contracts.md#ir72-specification-priority--rev18-029). Report a document defect instead of choosing a rule during implementation.
- Precedence: latest production instructions confirmed in the original records → company original SRC-06 → requirements organized from that original → production policies whose original records are missing (such as the four roles; retained as the current basis where reversible, with company confirmation tracked in OPEN-10) → common contracts and role designs. Reference mocks are visual design references. Record affected IDs instead of silently overwriting conflicting content.
- A blank approver field means no approval has been given. The earlier original instructions for SRC-02 are not included; see their [confirmation status](00-prepare/sources/production-instructions.md). Implementation, test, and review agents must not grant business approval themselves.

## Change management

Keep the original requirement IDs. Update the PrepareDocument decision table, affected requirements and designs, traceability matrix, and acceptance criteria together in the same change. Do not reuse retired IDs. Record evidence using the [artifact templates](04-agentic-sdlc/templates/artifacts.md).

Current approver: not assigned / Implementation: not started / Application behavior checks: not run. Document consistency checks and application acceptance tests are separate activities.

0.5.0 (2026-09-15): Organized 26 items from the [company original](00-prepare/sources/company-requirements-original.txt) and revised the common and four-role requirements, detailed designs, and UIUX specification. The [company requirement map](00-prepare/company-requirement-map.csv), [requirement origins](00-prepare/requirement-origins.csv), and [traceability matrix](00-prepare/traceability.csv) show the original text, added design details, and acceptance criteria. Clarified the display of open windows, poor insulation, allergens, card categories, Scope 2, and the market integration concept. Preserved visual values from the reference HTML/CSS.

Each agent must read the original text and origin table first. Do not treat added design details as company-approved requirements. Use the detailed text, input contracts, and acceptance criteria together. Document structure checks and application behavior checks are separate. Application tests have not been run.

0.6.0 (2026-09-15): Defined input/output, read, commissioning, manual payment entry, and restriction release contracts in detail, bringing the total to 117 operations. Reworked acceptance criteria into concrete inputs and expected results. The traceability matrix covers 182 case bundles, including 10 additional R01 cases. Standardized the execution status schema, specification baseline, and reassessment after changes. The [production instruction confirmation status](00-prepare/sources/production-instructions.md) separates missing original instructions from design interpretations. Application tests have not been run.

0.7.0 (2026-09-16): Rewrote 147 rows of acceptance criteria (AT-N/E/B) across four roles as pairs of fixture values and observable expected results, and introduced subcase numbers ①②…. Added `automations.fire` (the write contract for firing automations) and `jobs.resumeHold/resumeRework`, bringing the total to 119 operations. Defined source labeling rules in PrepareDocument §2 and reassessed FR-T02 and FR-A07. Split precedence rules by whether original records exist, and registered the separate contractor role as OPEN-10. Added the canonical role identifier table (DDC-08 §5), centralized common agent definition rules (SDLC README §7), and documentation gate records (`04-agentic-sdlc/runs/`). Application tests have not been run.

0.8.0 (2026-09-16): Addressed 36 strict review findings. Detailed 1A authorization, trigger arbitration, restriction recovery, payment attempts, numbers, states, and safe display rules in the [deterministic contracts](02-design/deterministic-contracts.md). Added [canonical DTOs](02-design/service-contracts.ts), 131 operations, 47 screens, and Component/Query contracts. Fixed conflicts between acceptance criteria and the source documents. AT-FIX-001–036 are additional test plans, not application execution results.

The current English translation baseline is [TRANSLATION-EN-2026-09-17](04-agentic-sdlc/runs/TRANSLATION-EN-2026-09-17/spec-manifest.json), based on specification 0.21.0. DOC-0.21.0 retains its original review and hashes; its approval does not approve the translated file contents. See the [translation record](04-agentic-sdlc/runs/TRANSLATION-EN-2026-09-17/progress.md) for translation checks and review status. DOC-0.7.0–0.20.0 are historical records and must not be used as current implementation input (DOC-0.18.0 has no baseline; DOC-0.19.0 and DOC-0.20.0 failed independent G1; IR75). Run both `python3 docs/tools/validate_documents.py` and `python3 docs/tools/check_review_regressions.py` for verification (IR73). A self-review by the document editor is separate from G1 approval by another reviewer. Production connection is NOT READY until the D11 artifacts are finalized.

0.9.0 (2026-09-16): Added corrections for 21 findings from STRICT-DOC-0.8.0. The user approved three items on 2026-09-16: periods, offset failures, and contract editing under restrictions. G1 is pending; approval of an earlier version does not carry over. See the [correction contracts](02-design/strict-review-contracts.md).

0.10.0 (2026-09-16): Added eight design re-review findings to SR22–29 and fixed eight more findings from repeated checks. Kept approved SR17–19. Performed static document checks and scenario checks. Application implementation and behavior tests are not completion criteria for this work. Independent G1 approval is recorded separately.

0.11.0 (2026-09-16): Defined 17 items in the [review resolution contracts](02-design/review-resolution-contracts.md). Explicitly deferred one long-term memory capacity item as a possible additional requirement. Fixed six more findings during the self-review after the changes. Added the [24-case acceptance plan](04-agentic-sdlc/acceptance-resolution.csv) to the traceability matrix. Independent G1 is pending.

0.12.0 (2026-09-16): Fixed eight re-review findings. Clarified the full scope of restrictions and advance-notice recipients, acceptance retries, sorting of limited projections, reminder previews, accumulation at the start of each minute, saving measurements/inspections, and Fact evaluation. Added an [acceptance plan](04-agentic-sdlc/acceptance-convergence.csv).

0.13.0: Fixed five re-review findings about public job projections. The [acceptance plan](04-agentic-sdlc/acceptance-projection.csv) tracks list search/counts, history after expiry, summaries, and access expiry during pagination. Application tests have not been run.

0.14.0: Applied user decisions on installation property addresses and report status after expiry. Fixed job summary conditions, access to stop reasons, reason entry when saving models, and operation counts. Application tests have not been run.

0.15.0: Fixed independent review findings IRV-001–006. Aligned summary acceptance tests, address descriptions, and current handoff references. Clarified completion timestamps, equipment alert severity, and the rule against approving a report that the approver helped edit. See the current runs for decision evidence. Application tests have not been run.

0.16.0: Fixed job filters, the audit retrieval path, correlation ID search, and Page count descriptions after re-review. Applied the user's answers: two permission types, and business order as the default sort order. See the [gate record](04-agentic-sdlc/runs/DOC-0.16.0/gate-G1.yaml) for this version's G1 result. The earlier DOC-0.15.0 pass does not carry over to this version.

The 0.16.0 decision records are retained in [runs/DOC-0.16.0](04-agentic-sdlc/runs/DOC-0.16.0/review.md) as records of that version.

0.17.0 (2026-09-16): Applied an independent review by another AI (FRV-001–025: 2 CRITICAL, 8 MAJOR, 15 MINOR). Defined the release-request entry path and release idempotency, demo clock jumps and session lifetime, transport/network failure injection, customer-created job expiry, archived resource visibility, the population used for customer counts, 1h/24h presets, private fields in role projections, Sensor creation on Device registration, display formats, translation fallback, and rendering exceptions in [IR35–44](02-design/review-resolution-contracts.md). Added SCR-X-not-found and nine Component contracts (48 screens, 66 Components). Adopted design proposals are in [DEC-19–24](00-prepare/internal/review-decisions-017.json) (PROPOSED, reversible). See the [review report (13 items)](04-agentic-sdlc/runs/DOC-0.17.0/review.md), [decision table](04-agentic-sdlc/runs/DOC-0.17.0/traceability-matrix.csv), and [15-case acceptance plan](04-agentic-sdlc/acceptance-review-017.csv). The check after the changes was a self-review by the same agent that made them. Independent G1 remains pending, as stated in the [gate record](04-agentic-sdlc/runs/DOC-0.17.0/gate-G1.yaml). Application tests have not been run.

0.18.0 (2026-09-17): Applied strict review findings REV18-001–048 (2 BLOCKER, 4 CRITICAL, 24 MAJOR, 16 MINOR, 2 QUESTION). Defined the liveness simulator, operations under restrictions, derived equipment connection states, Offer expiry, technician viewing/work windows, KPI navigation and allowed URL keys, count definitions, lifestyle patterns, consent withdrawal, trigger paths, session extension, cancellation tables, FORBIDDEN/NOT_FOUND displays, notification lists, a single payment finalization path, demo-only operations, non-RTO displays, unassigned spaces, energy-saving baselines, contact hours, voice matching, Alert recovery, starting device operations, negative-value displays, demoSeed, holidays, change events, specification precedence, and validator mutation tests in [IR45–74](02-design/review-resolution-contracts.md). There are 137 operations, 48 screens, and 67 Components. Adopted design proposals are in [DEC-25–41](00-prepare/internal/review-decisions-018.json) (PROPOSED, reversible). See the [findings](04-agentic-sdlc/runs/DOC-0.18.0/review.md) and [48-case acceptance plan](04-agentic-sdlc/acceptance-review-018.csv). Work stopped during the corrections; no self-review or baseline creation was completed (the [gate record](04-agentic-sdlc/runs/DOC-0.18.0/gate-G1.yaml) is not_evaluated; IR75). Version 0.19.0 continued the unfinished work. Application tests have not been run.

0.19.0 (2026-09-17): Applied independent review findings REV19-001–042 for the interrupted DOC-0.18.0 working tree (1 BLOCKER, 3 CRITICAL, 16 MAJOR, 20 MINOR, 2 QUESTION; 037 was found in the self-review after changes, and 038–042 in a full check of acceptance preconditions requested by the user). Defined the handoff baseline and validator fixes, technician screens before the work window (work-not-started), simulator copy conditions, admin dashboard energy-saving forecasts (energyForecast), consistent session-extension rules, negative values on C06, regex-based detection of outdated wording, return routes after login, display during refetch, initial consent records, seed changes for KPI acceptance tests, responses to expired Offers, reason length limits, MRV display mappings, work-window end notices, demoSeed normalization, schedule updates on reassignment, interpretation of acceptance Given clauses and seed changes, and failure codes for starting technician work in [IR75–93](02-design/review-resolution-contracts.md). There are 137 operations, 48 screens, and 69 Components. Adopted design proposals are in [DEC-42–53](00-prepare/internal/review-decisions-019.json) (PROPOSED, reversible). The user settled the energy-saving forecast calculation (DEC-44) and work-window end behavior (DEC-50) on 2026-09-17. See the [review report](04-agentic-sdlc/runs/DOC-0.19.0/review.md), [decision table](04-agentic-sdlc/runs/DOC-0.19.0/traceability-matrix.csv), and [42-case acceptance plan](04-agentic-sdlc/acceptance-review-019.csv). The check after corrections was a self-review in the same session. The later independent G1 by another agent failed (12 MAJOR, 17 MINOR, 2 QUESTION), as shown in the [decision record](04-agentic-sdlc/runs/DOC-0.19.0/independent-g1/review.md). Version 0.20.0 addressed these findings. Application tests have not been run.

0.20.0 (2026-09-17): Applied independent G1 findings by another agent for DOC-0.19.0 (G1-001–031: 12 MAJOR, 17 MINOR, 2 QUESTION; result FAIL). Defined authorization column qualifiers and technician write conditions, business event notifications, restriction cancellation and releaseIntent, acceptance fixture invariants and input objects, demo data for allergen observations and possible-cause Alerts, air-condition guidance, the set of inspection components and submission validation, concrete common acceptance values for AT-X01–X07, and corrections to expected results in earlier acceptance plans in [IR94–102](02-design/review-resolution-contracts.md). There are 137 operations, 48 screens, and 69 Components. Adopted design proposals are in [DEC-54–59](00-prepare/internal/review-decisions-020.json) (PROPOSED, reversible). See the [review report](04-agentic-sdlc/runs/DOC-0.20.0/review.md), [decision table](04-agentic-sdlc/runs/DOC-0.20.0/traceability-matrix.csv), and [31-case acceptance plan](04-agentic-sdlc/acceptance-review-020.csv). Independent G1 reassessment was FAIL (1 MAJOR, 4 MINOR), as shown in the [decision record](04-agentic-sdlc/runs/DOC-0.20.0/independent-g1/review.md). Application tests have not been run.

0.21.0 (2026-09-17): Fixed independent G1 findings G120-001–005 for DOC-0.20.0. Defined the A12 continuous 60-second boundary, notification types and severity, allergen observation change events, and the scope version at creation for notification fixtures in [IR103–106](02-design/review-resolution-contracts.md). [DEC-60–61](00-prepare/internal/review-decisions-021.json) are reversible demo proposals. Added a [five-case acceptance plan](04-agentic-sdlc/acceptance-review-021.csv) and [correction report](04-agentic-sdlc/runs/DOC-0.21.0/review.md). See the [gate record](04-agentic-sdlc/runs/DOC-0.21.0/gate-G1.yaml) for the latest independent G1 result, which is separate from the self-review. Application implementation and behavior tests have not been run.

0.22.0 (2026-10-01): Reflected the Figma wireframes confirmed by the user as the final screen specification (IR107–IR112). Permissions became 38 Read/Write/action values; Access & roles covers HQ/contractor/technician only and client accounts moved to Customers & units › Users (FR-A17). Client locations are read-only except rename; alert policies are customer-owned and attached by units with an HQ default policy, and the Air quality policies page is merged into Alert policies (FR-A12). Client ventilation is a manual log. Added FR-C14–C18 (group control, customer alert policies, report export, confirm & rate, filter care), FR-P09–P10 (certificates, payouts; Overview dashboard and `/partner/jobs`), FR-T13–T15 (QR/check-in, parts & refrigerant & time on site, sign-off), FR-A17–A23 (client users, CSV import, warranty, firmware campaigns, contractor register, SLA, payouts), and FR-X08 (two-step verification). 189 operations, 49 screens, 82 requirements. Shared “Page unavailable” for 403/404; 1920×1080 layouts, centered modals, and search-select filters (UX-08a).

0.23.0 (2026-10-02): Maintenance scheduling across the four roles (IR113, DEC-63/64). Clients give 3 preferred times; HQ books one of them or proposes another time that the client accepts or declines (with new times); partner offers carry a fixed agreed visit time and contractors can propose another time through HQ; technicians accept new assignments (受領) or report they cannot make the time; every job shows its origin (Client request / Periodic plan). The Figma “(proposal)” screens were confirmed and integrated as normal spec (FR-C14–C18, FR-P09–P10, FR-T13–T15, FR-A17–A23, FR-X08 unchanged). 197 operations, 49 screens, 82 requirements. The clickable demo in `web/` implements the same flow with a shared mock store (`web/lib/jobs.ts`).

0.24.0 (2026-10-06): Closed the three gaps found by the use case diagrams (DEC-65, IR114). Client owners list users, invite members, and resend invites at `/customer/users` (new FR-C19 / DD-C19 / SCR-C19); clients add coordination notes to open jobs (`jobs.addNote`, Client 07o/07p); HQ classifies “Report a problem” follow-ups as rework or a new request (`jobs.classifyFollowUp`, Admin 06-17/06-18). 83 requirements, 50 screens, 197 operations.

0.25.0 (2026-10-07): Consistency check across the Figma Use Cases, User Flows and wireframes and these documents (DEC-66, IR115). Admin dashboard billing figures and the Energy analysis link need billing.read / energy.read (no *.manage names remain); default-policy rule switching in the customer app is owner-only (FR-C15); follow-up classification traces to FR-A06; tab contracts for SCR-T01/T04/A04/A06/A08/A15/A16/C08/C09/A02/P06 and the `orgId` key follow Figma. 83 requirements, 50 screens, 197 operations (unchanged).

0.26.0 (2026-10-07): Fixed the frontend stack to Next.js (App Router), which becomes the BFF in production, and added the production [backend architecture](02-design/backend-architecture.md) (modular monolith Core API for the 197 operations, IoT / scheduler / notification / export workers, data stores, events, integrations, NFR targets) and [network architecture](02-design/network-architecture.md) (zones, public endpoints, traffic flows, field connectivity, edge protection, certificates, DR) — DEC-67, IR116. Phase 1A scope and counts unchanged.

0.27.0 (2026-10-07): Production target decisions (DEC-68, IR117): AWS (ap-southeast-5 primary, ap-southeast-1 DR) with a component-to-service mapping, Stripe Checkout for payments, the HQ admin app only on `admin.<domain>` from the company network, retention periods, launch / design capacity (3,000 / 20,000 units), and customer office firewall requirements. AC interface, WhatsApp provider, payout rail and SIM provider remain open. Phase 1A scope and counts unchanged.

0.28.0 (2026-10-07): Backend implementation design in Go + Echo ([backend Go design](02-design/backend-go-design.md)) and database design ([database design](02-design/database-design.md), executable [db/schema.sql](02-design/db/schema.sql), 12 schemas, RLS, partitioned telemetry and audit) — DEC-69, IR118. The schema was applied to PostgreSQL 16 and the core request pipeline was compiled and tested against it. Phase 1A scope and counts unchanged.

0.29.0 (2026-10-07): Everything runs in Docker ([container design](02-design/container-design.md), DEC-70, IR119): `web/Dockerfile` (Next.js standalone, distroless, read-only), backend image design (Go 1.25, distroless static, healthcheck subcommand), repository-root `compose.yaml` with profiles demo / infra / schema / backend / full / obs / stripe and local stand-ins for AWS services. The demo and infra profiles were started and checked. Phase 1A scope and counts unchanged.

0.74.0 (2026-10-08): Event delivery between services through the outbox (IR183). Phase 1A unchanged.
0.73.0 (2026-10-08): Phase B step 1 — principals from identity-api (IR182). Phase 1A unchanged.
0.72.0 (2026-10-08): Phase B inventory of cross-domain database access and order of work (IR181). Phase 1A unchanged.
0.71.0 (2026-10-08): Core API split into five business-domain services and a gateway (IR180, phase A). Phase 1A unchanged.
0.70.0 (2026-10-08): Go code under service/api, web apps under service/web (IR179). Phase 1A unchanged.
0.69.0 (2026-10-08): Four web apps, one per entry point (customer-web, partner-web, technician-web, admin-web) with the shared package web-shared (IR178). Phase 1A unchanged.
0.68.0 (2026-10-08): Customer and admin screens rendered on the server; local HQ TOTP for end-to-end runs (IR177). Phase 1A unchanged.
0.67.0 (2026-10-08): Server-only DAL, token refresh in proxy, /notifications as Server Component + Server Action (IR176). Phase 1A unchanged.
0.66.0 (2026-10-08): Web on Next.js 16 with ESLint flat config, proxy.ts optimistic auth, server-only session, open-redirect fix (IR175). Phase 1A unchanged.
0.65.0 (2026-10-08): Go code in one module with the official server layout cmd/ + internal/, integration tests in test/integration (IR174). Phase 1A unchanged.
0.64.0 (2026-10-08): Core API on Echo v5 following the official Echo guide (IR173, DEC-71). Phase 1A unchanged.
0.63.0 (2026-10-08): Membership.displayName (IR172). Phase 1A unchanged.
0.62.0 (2026-10-08): Test database isolation and seeded job history (IR171). Phase 1A unchanged.
0.61.0 (2026-10-08): One unit scope for units, alerts, telemetry and devices; IR49(b) work-window gate (IR170). Phase 1A unchanged.
0.60.0 (2026-10-08): Unit read scope for external technicians and contractors (IR169). Phase 1A unchanged.
0.59.0 (2026-10-08): Shared demo clock `platform.demo_clock`, acknowledged settings in observedState, BFF token refresh, notifications and unit control on the API (IR168). Phase 1A unchanged.
0.58.0 (2026-10-08): `service/migrate` migration service replaces the psql `db-schema` job (IR167). Phase 1A unchanged.
0.57.0 (2026-10-08): Services move under `service/` (service/api, service/web) for the microservice layout (IR166). Phase 1A unchanged.

0.56.0 (2026-10-08): UnitDetail parts (capabilities, effective control policy, control availability, components, pending commands, location) and control blocking during recovery (IR165). Phase 1A unchanged.

0.55.0 (2026-10-08): Terminal-restriction recovery cases (SR26) in the backend (IR164). Phase 1A unchanged.

0.54.0 (2026-10-08): Result-type conformance sweep; jobs.acknowledgeAssignment returns Assignment (IR163). Phase 1A unchanged.

0.53.0 (2026-10-08): BFF action check with the seeded scenario; jobs.addNote returns JobNote (IR162). Phase 1A unchanged.

0.52.0 (2026-10-08): Demo business records seeded for the Core API (IR161). Phase 1A unchanged.

0.51.0 (2026-10-08): Result meta snapshotAt/eventCursor and the server clock for screens (IR160); job screens read jobs.list in api mode. Phase 1A unchanged.

0.50.0 (2026-10-08): Demo clock starts at fixture.clock; full-stack BFF sign-in and relay verified (IR159). Phase 1A unchanged.

0.49.0 (2026-10-08): Web BFF (OIDC sign-in, operation relay, session endpoint) and the first API-backed screen (IR158); Keycloak demo users carry seed IDs and tenant/membership/role claims. Phase 1A unchanged.

0.48.0 (2026-10-08): One business clock in the database: `platform.app_now()` and transaction-level `app.now` (IR157). Phase 1A unchanged.

0.47.0 (2026-10-08): Business-event notifications (jobs, restrictions, payments) fixed for the backend (IR156). Phase 1A unchanged.

0.46.0 (2026-10-08): Alert-policy notification evaluation in automation simulate/fire fixed for the backend (IR155). Phase 1A unchanged.

0.45.0 (2026-10-08): Demo operations (DEMO_OPS switch, clock, triggers) and demo sessions via the BFF fixed for the Core API (IR154); all 197 operations now have backend handlers. Phase 1A unchanged.

0.44.0 (2026-10-08): Voice intents and the monthly energy export fixed for the backend (IR153). Phase 1A unchanged.

0.43.0 (2026-10-08): Automation evaluation (simulate/fire, arbitration, tick replay) fixed for the backend (IR152); `control.evaluation_events` added. Phase 1A unchanged.

0.42.0 (2026-10-08): Customer automation rules (save, list, next runs) fixed for the backend (IR151); automation name length aligned to 1–120. Phase 1A unchanged.

0.41.0 (2026-10-08): Offset quotes, simulated records and retries fixed for the backend (IR150). Phase 1A unchanged.

0.40.0 (2026-10-08): MRV reports fixed for the backend (IR149); energy.summary uses the default emission factor and factor_missing (SR09). Phase 1A unchanged.

0.39.0 (2026-10-08): Energy summary comparison and the admin dashboard (actuals, IR78 forecast, billing visibility) fixed for the backend (IR148); partner KPI definitions aligned with D07. Phase 1A unchanged.

0.38.0 (2026-10-08): Emission factors, energy integration and baselines fixed for the backend (IR147). Phase 1A unchanged.

0.37.0 (2026-10-08): Role summaries fixed for the backend (IR146); UnitSummary.activeAlertCount follows the IR51 population. Phase 1A unchanged.

0.36.0 (2026-10-08): QR resolution, write results and password-reset preview fixed for the backend (IR145). Phase 1A unchanged.

0.35.0 (2026-10-08): Client users and two-step verification fixed for the backend (IR144): `identity.two_factor` table, Target kind `client_user` and template `invite`. Phase 1A unchanged.

0.34.0 (2026-10-08): Inquiries and audit search fixed for the backend (IR143). Phase 1A unchanged.

0.33.0 (2026-10-08): Notification inbox, recipients and preview, preferences and consents fixed for the backend (IR142). Phase 1A unchanged.

0.32.0 (2026-10-08): Restriction grace/exception, explicit and forced release, reconcile and retry fixed for the backend (IR141). Phase 1A unchanged.

0.31.0 (2026-10-08): Restriction schedule, execution, command results, release evaluation, cancellation and reads fixed for the backend (IR140). Phase 1A unchanged.

0.30.0 (2026-10-08): Alert-policy details fixed while implementing the backend (IR120): the six default rules with ruleKeys and limits, `DefaultPolicyInput` for HQ limit edits, field rules and client editor values, list filters and order, delete/default-rule-switch versions, and IR108 attachment errors; the client rule label becomes “Refrigerant low pressure”; automation-policy field rules. Telemetry and ventilation reads (IR121): Measurement IDs, series/summary targets and ranges, ventilation log/list scope, `monitoring.allergen_observations`. Job intake, notes, hold and list filters (IR122); offers, decisions and assignments (IR123); contractor/technician projections and frozen job history (IR124); technician on-site operations (IR125); work reports (IR126); follow-up, cost, access and warranty operations (IR127); time proposals and reschedules (IR128); attachments and sign-off (IR129); maintenance plans (IR130); contractor register, rate cards and SLA (IR131); memberships, eligibility, capacity and unavailability (IR132); certificates and parts catalog (IR133); filter care (IR134); contracts, invoices and reminders (IR135); payments (IR136); contractor payouts (IR137); unit commands (IR138); diagnostic test runs (IR139). Phase 1A unchanged.
