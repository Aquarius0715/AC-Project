---
document_id: PREP-001
version: 0.82.0
status: review-draft
audience: business-stakeholders
scope: frontend-only
updated: 2026-09-17
---

# AC Project Company Requirements and Frontend Design Preparation

**PrepareDocument | For company review | September 15, 2026**

## 1. Purpose and assumptions

This document organizes and analyzes the company's original requirements. It prepares for turning them into frontend displays, inputs, actions, states, and acceptance criteria. It separates company requests, the document authors' production instructions, and added design details.

### 1.1 Requirements provided by the company

The company plans a service centered on AC monitoring, remote control, and maintenance. It also covers energy savings, air quality, payment management for RTO (a service that transfers ownership after installment payments), carbon emissions, and offsets. Customers, internal and external technicians, and administrators/HQ each need clear visual screens.

The original text places split AC units in Phase 1: first build a clickable UI/UX, then develop the complete solution, including devices and firmware. HVAC (heating, ventilation, and air conditioning) and support for more models and brands are later plans. The original text includes explicit feature requests, questions about how to achieve them, and expected benefits.

The source is the [original company requirements in English](sources/company-requirements-original.txt) provided for this project. The original is stored unchanged.

### 1.2 Reference mock app

The [Aconland Mudah Milik customer Loyalty page](https://aconland-mudah-milik.vercel.app/customer/loyalty) is a design reference for colors, fonts, cards, and navigation. Required features, information, and action flows come from company requirements and user tasks.

### 1.3 Current scope

**The deliverables are frontend design documents only.** They define displayed information, input fields, action flows, state changes, shared design, and demo data handling. They retain the principle of separating screens from data access for future API connections.

The development stages are as follows. **1A** is the clickable frontend demo within the original Phase 1 (split AC units). **1B** is the complete solution in the same Phase 1, including devices, firmware, and production API connections. **Phase 2** extends to HVAC. These documents cover only 1A.

Real device control, device and sensor selection, firmware, real payments and notifications, carbon trading, and API, database, and server design are out of scope. Payment receipts, restrictions, notifications, and device responses on screen use synthetic demo data. The existing reference mock and the new frontend demo are separate products.

## 2. How requirements and sources are organized

The original text is organized by users, monitoring and maintenance, control and automation, payments, energy and environment, and future expansion. Repeated requests are combined. Items with an unconfirmed implementation method remain unconfirmed; they are not presented as working capabilities or guaranteed values.

| Source label | Meaning | Treatment here |
|---|---|---|
| **Company original** | English requests provided by the company | The basis for requirements; not proof of feasibility or detailed approval |
| **Production instructions (from the document authors)** | Instructions such as four roles, frontend only, and matching the reference design | Separate from the original company requirements |
| **Reference mock observations** | Findings from public screens, HTML/CSS, or previous research records | Design references for colors, fonts, and shapes |
| **Added design details (design proposal)** | Workflows, screen details, demo values, and technical choices | Proposals to meet company requests, separate from explicit original requests |

Requirements start from the original text and expand into screen fields, actions, states, exceptions, and acceptance criteria. Every original request can be traced to its coverage, whether or not a reference design is used.

“Added design details” and “design proposal” mean ideas added here to make company requests concrete. Source labels follow these rules. **“Company original SRC-06 + added design details”** means the original directly names the feature, target, or action, while only screen details or rules are added (for example, component inspections, remote control, and contracts and payment restrictions). **“Added design details (supporting a company goal)”** means the original gives only a goal, while the authors add the screen, role, or management feature (for example, access management, work report forms, technician test runs, and grace periods or exceptions). **“Production instructions SRC-02 + added design details”** means the requirement comes from an instruction absent from the original, such as using four roles. If requirements based on the same BIZ have different labels, the original_phrases column in the [requirement source table](requirement-origins.csv) shows whether the original mentions the feature directly. The four roles are client (customer), contractor, technician (internal or external), and administrator (HQ). Later documents use the same groups.

## 3. Organized company requirements

The BIZ numbers below are assigned by this document, not by the company. Every row comes from the **company original (SRC-06)**. “Treatment in this frontend” describes how the original is organized and proposed for the frontend.

### 3.1 Users, targets, and monitoring

| Group ID | Company request | Treatment in this frontend | Notes |
|---|---|---|---|
| BIZ-01 | Sign in, sign out, and reset passwords | Entry, exit, and reset screens with demo actions | FR-X01 defines screens, actions, and acceptance criteria |
| BIZ-02 | Display in a chosen language | Language switching and translatable screens; initial languages defined separately | Initial English and Malay support is a design proposal |
| BIZ-03 | Voice AI responses | Demo voice input and responses, with text fallback; temperature queries and device control are design proposals | The original does not explicitly request device changes through voice. Real voice integration is out of scope |
| BIZ-04 | Visual dashboards for customers, internal/external technicians, and administrators/HQ | Show each role's metrics, states, and next actions | A separate contractor role is the document authors' production instruction |
| BIZ-05 | Start with split AC units and clickable UI/UX, then build the full solution | Design screens, actions, and simulated states for split units | Current deliverables are frontend documents |
| BIZ-06 | HVAC in Phase 2, with support for more brands, split units, central AC, and cassette units | Distinguish device types, models, and supported features | Proving support for every brand is out of scope |
| BIZ-07 | Manage homes/offices by area, floor, room, and space | Select units from a location hierarchy and check their status | Registration and editing steps are added design details |
| BIZ-08 | Detect faults before or when they occur and notify in real time | Show readings, alerts, update times, and next actions using synthetic data | Detectable conditions and accuracy are unconfirmed |
| BIZ-09 | Red, orange, and green signals and notifications | Show severity using colors, text, and icons | The exact meaning of each color is a design proposal |
| BIZ-10 | Check indoor, outdoor, electrical, and control components | Component status, evidence, and inspection records; components listed below | Inspection input and approval flows are added design details |

### 3.2 Maintenance, control, and air quality

| Group ID | Company request | Treatment in this frontend | Notes |
|---|---|---|---|
| BIZ-11 | Early detection of vibration, high temperature, low refrigerant, tiny leaks, and clogged filters | Demo suspected faults, supporting data, and maintenance guidance | Detection of tiny leaks and similar conditions needs verification |
| BIZ-12 | Scheduled, reactive, and preventive maintenance, including general maintenance outside RTO | View requests, work progress, and results across roles | Intake, assignment, and quality review responsibilities are added design details |
| BIZ-13 | Check temperature with smart thermostats and change settings remotely | Separate room and set temperatures; demo change requests and responses | Detailed controls such as mode and fan speed depend on model capabilities |
| BIZ-14 | Scheduling, cooling before arrival, and automatic stop when empty | Set weekdays, time slots, and occupancy conditions; simulate triggers | Time limits and conflict priorities are design proposals |
| BIZ-15 | Reduce cooling when away and resume on return using GPS | Demo automation from away/home events | Location consent and withdrawal are added design details |
| BIZ-16 | Pre-cool at low rates, avoid peak rates, and connect solar/battery systems | Simulate tariff, peak, solar/battery conditions and results | Real external data and device connections are out of scope |
| BIZ-17 | Adapt operation to routines and weather; notify about load from open windows or poor insulation | Define automation conditions and abnormal load guidance | C08/T07/A05 show open windows and poor insulation as possible causes with evidence |
| BIZ-18 | Monitor CO₂, dust, humidity, and allergens; advise cleaning and ventilation | Distinguish metrics, units, values, quality, and guidance | C07/A12 separate allergen data availability, evidence, and not measured |
| BIZ-19 | Improve air quality with fresh air intake when CO₂ rises | Demo requests for supported ventilation equipment; otherwise guide the user | Distinguish fan circulation from fresh air intake; no health or safety guarantee |
| BIZ-20 | Small, low-cost devices inside AC units, firmware, and removal/theft protection and alerts | Demo device details, connectivity, updates, and removal detection | Size, cost, and detection methods are unconfirmed; registration and calibration are design proposals |

### 3.3 Payments, energy savings, and environmental value

| Group ID | Company request | Treatment in this frontend | Notes |
|---|---|---|---|
| BIZ-21 | Let administrators reduce or stop cooling for unpaid RTO and similar bills | Demo payment states, restriction reasons, schedules, application, and release | Notice, grace periods, exceptions, and release after payment are added design details |
| BIZ-22 | WhatsApp/email links to card payments and payment instructions | Notification previews, demo credit/debit cards, and payment guidance | C11/A08 distinguish demo credit, debit, and payment instructions |
| BIZ-23 | Visualize power use and cost; compare with normal use; expect 10–20% or more waste reduction | Show baseline, actual use, estimated cost, savings, and comparison conditions | Original percentages are expectations, not guarantees or acceptance criteria |
| BIZ-24 | Optional use of a carbon offset/exchange platform | Links to emissions, optional requests, and demo purchase/retirement records | Schemes, platforms, and real transactions are unconfirmed |
| BIZ-25 | Baseline comparison, measurement/reporting/verification, regional emission factors, corporate emissions reporting, and future credit creation | Preview calculation conditions, quality, evidence, and reports | A14 shows Scope 2 period, organization, regional factors, and calculation boundary |
| BIZ-26 | Tokenize savings, distributed ledgers, micro-offsets, trading, and real-time carbon market APIs | Keep as a future concept, separate from current demo offset records | C13/A15 propose showing future integration concepts; real trading and connection methods are out of scope |

### 3.4 Components named in the original

| Category | Components in the company original | Frontend treatment |
|---|---|---|
| Indoor unit | Air filters, evaporator coils, blower motors/fans, drain pipes/pans, outlets/louvers | Screens for component status, inspections, and evidence |
| Outdoor unit | Condenser coils, compressors, fans/blades, refrigerant pipes | Suspected faults, readings, and inspection records |
| Electrical/control | Thermostats, possible smart thermostats/sensors, capacitors/contactors, wiring | Temperature checks/settings and electrical/control inspection records |

These components are the targets of status and inspection screens. The proposal distinguishes measurements, diagnostic results, and inspection results.

### 3.5 From company text to requirements and design

Using the original English text as the primary source, BIZ-01–26 are expanded into common and four-role requirements, screen fields, actions, states, exceptions, and acceptance criteria. The [company request map](company-requirement-map.csv) traces source locations to requirements. The [requirement sources and added design details](requirement-origins.csv) traces requirements back to original text and additions. The reference mock guides appearance.

Open-window/poor-insulation alerts, allergen data, credit/debit distinctions, and Scope 2 reporting are explicit in current requirements and detailed designs. Tokenization and market integration remain future company concepts; current screens propose explaining the concepts and the lack of a connection. Real trading and market API specifications are out of scope.

## 4. Users and production instructions

| User | Main intended use | Source | Notes |
|---|---|---|---|
| Client | Check unit status, temperature, cost, air quality, maintenance, and payments | Company original | Individual booking and confirmation steps include design proposals |
| Technician (internal/external) | Monitor assigned units, check faults, inspect, and record work | Company original | The original distinguishes internal/external technicians. Assignment and time-based access limits are design proposals |
| Administrator/HQ | Manage overall status, maintenance, payments, operating restrictions, energy, and environmental data | Company original | Grace periods, exceptions, release, and audit details are proposed workflow additions |
| Contractor | Manage accepted jobs, own technicians, schedules, and quality reviews | Production instructions (document authors) + design proposal | The original does not define a separate fourth role. Production instructions separate contractor managers from field technicians |

Four roles separate company-level job management from field work. The proposed flow is HQ → technician for internal jobs, and HQ → contractor → technician for outsourced jobs. This is not a detailed workflow approved by the company in the original text.

## 5. Role of the design reference

Colors, fonts, spacing, and shapes from the reference page's HTML/CSS guide shared UI design. The [reference design analysis](reference-design-analysis.md) records what was retrieved and the visual values. Screen layouts aim to clearly present information and actions defined from company requirements.

## 6. How requirements apply to this frontend

### 6.1 Screens and actions

| Area | What this phase shows | Main source |
|---|---|---|
| Common use/dashboards | Sign in/out, language, voice entry, role summaries | Company original + four-role production instruction |
| Units/locations/monitoring | Home/office, locations, hierarchy, units, values, units of measure, update times, alerts | Company original |
| Control/automation | Temperature changes, schedules, occupancy/arrival/weather conditions and results | Company original + proposed input/state details |
| Maintenance | Cross-role demo of requests, arrangements, assigned work, reports, and results | Company original + proposed intake/assignment/quality review |
| Payments/restrictions | Payment guidance, simulated payments, restriction reasons, application/release states | Company original + proposed notice/exception/release flows |
| Air quality/energy | Ventilation guidance, power, cost, baseline comparisons, estimated emissions | Company original + proposed calculation conditions/quality displays |
| IoT/environmental reporting | Demo connections, removal detection, device responses, MRV, and optional offsets | Company original + proposed screen actions |

### 6.2 Applying the design

As instructed by the document authors, the design follows the retrieved Loyalty page HTML/CSS. The primary color is blue (#005BEA), the background light blue (#F8FBFF), and body text dark navy (#0D2238). Plus Jakarta Sans is the base font for Latin letters and numbers. White cards and the arrangement of summaries, details, and history are shared.

Small text and controls are adjusted for readability and usability in work screens. Reference and adjusted values are recorded separately. This is a **production instruction to match the reference mock**, not a brand requirement in the company original. See the [design analysis](reference-design-analysis.md) and [UIUX specification](../03-uiux/UIUXSpecification.md).

### 6.3 Design assumptions separate from company requirements

| Decision ID | Assumption/policy | Source/status |
|---|---|---|
| DEC-01 | Contractors accept jobs, assign their own staff, and review quality | Design proposal; splitting into four roles is a production instruction |
| DEC-04 | Initial demo languages are English (default) and Malay | Design proposal, not company approval to limit multilingual support to two languages |
| DEC-05 | Use MYR and Kuala Lumpur time in the demo | Design proposal, not a confirmed target market |
| DEC-06 | Base the design on the reference Loyalty page | Production instruction from document authors |
| DEC-07 | Share the same data across role switches; reset on reload | Frontend design proposal |
| DEC-08 | Simulate voice interactions | Design proposal suited to this frontend scope |
| DEC-09 | Specific input limits, response timeouts, schedule conflicts, visibility, and other details | Design proposal, not official company operating rules |
| DEC-62 | The Figma wireframes (file VOeKPrid46kOf24ktEfe8r) are the final screen specification as of 2026-10-01, including screens marked “(proposal)” | User decision in this conversation, 2026-10-01; reflected in IR107–IR112 and FR-C14–C18, FR-P09–P10, FR-T13–T15, FR-A17–A23, FR-X08 |
| DEC-63 | Maintenance scheduling: clients give 3 preferred times; HQ books one of them or proposes another time; any time the client did not choose is booked only after the client accepts it; partners receive a fixed agreed time and may propose another time via HQ; technicians accept (受領) new assignments; every role sees whether a job is a client request or a periodic plan visit | User request in this conversation, 2026-10-02; reflected in IR113, FR-C09, FR-A06, FR-P02, FR-P03, FR-T08 and Figma Client 07k–07n, Admin 06-11–06-16, Contractor 02-22–02-24, Technician 02-33/02-34 |
| DEC-64 | The Figma screens previously marked “(proposal)” are confirmed and integrated as normal specification with entry points on the base screens | User decision in this conversation, 2026-10-02 (“提案されているUI、画面については全てOK”) |
| DEC-65 | The three gaps found by the use case diagrams get screens and rules: client owners invite members from the customer app (FR-C19), clients add coordination notes to jobs, and HQ classifies follow-up requests | User request in this conversation, 2026-10-06 (“３つのギャップは作成してください”); reflected in IR114, FR-C19, FR-C09, FR-C17, FR-A06, FR-A17 and Figma Client 07o/07p/11a–11c, Admin 06-17/06-18 |
| DEC-66 | Resolve the 2026-10-07 consistency check: dashboard billing/energy visibility uses billing.read/energy.read; default-policy rule switching in the customer app is owner-only; all other mismatches follow the IR115 resolution | User request in this conversation, 2026-10-07 (“2. 自然なやつに揃えて 3. Client owner限定 それ以外も全て修正”); reflected in IR115, FR-A01, FR-C15, FR-A05, SCR-T01/T04/A15/A16/P06 and the Figma wireframes, User Flows and Use Cases pages |
| DEC-67 | The frontend stack is Next.js (App Router), also used as the BFF in production; the production backend is a modular monolith with separate IoT / async workers and is designed cloud-agnostically together with the network | User request in this conversation, 2026-10-07 (“バックエンドやネットワーク、も設計して”; answers: cloud-agnostic logical design, modular monolith + workers, Figma + docs, option A Next.js); reflected in IR116, common design §1/§8, backend-architecture.md, network-architecture.md and Figma System Architecture boards 02–04 |
| DEC-68 | Production target: AWS hosting, Stripe for payments, HQ admin app only from the company network; retention, capacity and customer office firewall requirements are set by the design team; AC interface and SIM provider stay open | User request in this conversation, 2026-10-07 (“クラウド事業者はひとまずAWS…決済サービスなどはStripe…HQ画面へのアクセスは社内ネットワークに限定します…”); reflected in IR117, backend-architecture.md §1/§3a/§6/§7/§11/§13/§16 and network-architecture.md §2a/§3/§4/§5a/§10 |
| DEC-69 | The backend is implemented in Go with the Echo framework; the database is Aurora PostgreSQL with one schema per module | User request in this conversation, 2026-10-07 (“バックエンドの言語はGoとします。GoEchoを使用してください”); reflected in IR118, backend-go-design.md, database-design.md, db/schema.sql and Figma System Architecture board 06 |
| DEC-70 | Everything runs in Docker: all application components are Docker images in every environment (Compose locally and in CI, ECS Fargate in staging and production); production databases, queues and IoT stay AWS managed services | User request in this conversation, 2026-10-07 (“全てDockerで動かす前提です”; answer: apps all in Docker, production DB etc. AWS managed); reflected in IR119, container-design.md, compose.yaml, web/Dockerfile, docker/ and Figma System Architecture board 07 |
| DEC-71 | Backend and web follow the frameworks' official documentation and current major versions: Core API on Echo v5 structured as in the Echo guide (central HTTPErrorHandler, echo/v5 middleware, route groups, StartConfig graceful shutdown, slog); web on the current Next.js App Router conventions (IR174+) | User request in this conversation, 2026-10-08 (“バックエンドの構成ですが、GoEchoのドキュメントに沿って作成してください。Web側も同様に公式ドキュメントに沿って作成してください”); reflected in IR173, backend-go-design.md §1/§4 |
| DEC-72 | Core API split into business-domain microservices (identity, equipment, maintenance, billing, energy) behind a gateway; Go under service/api, web under service/web | User request in this conversation, 2026-10-08 (“APIもマイクロサービスに分けてください / APIの中にWebがあるのはおかしいです”; choices: api/ and web/ trees, split by business domain); reflected in IR179, IR180 |

**DEC-10 (design proposal)**: Preserve work status on reassignment. Release restrictions only after all cause invoices fixed at notice time are paid. Release does not automatically power units on or restore previous set temperatures. Manual payment recording, test-run end confirmation, and viewing saved reports are specified for the 1A demo. This does not mean company approval of commercial rules. See [input/output contract DDC-08](../02-design/implementation-contracts.md#ddc-08-multi-resource-revisit-and-cross-role-contracts).

Development methods and libraries (including DEC-02/03) are in [developer notes](internal/design-assumptions.md), separate from original company requests.

## 7. Questions and design considerations

### 7.1 Questions when defining the frontend

| Tracking ID | Topic | Current treatment |
|---|---|---|
| OPEN-01 | Work verification responsibilities of contractors, technicians, HQ, and customers | Demo acceptance/assignment/quality review as proposals, separate from official responsibilities |
| OPEN-02 | Initial languages, market, currency, and payment restriction notices | Use configurable demo values; do not finalize multilingual support or commercial conditions |
| OPEN-07 | Brand assets, readability, and display devices | Follow the reference design and state usability adjustments |
| OPEN-08 | Open-window/poor-insulation and allergen displays | Dedicated displays and acceptance criteria in C07/C08/T07/A05/A12; business review of wording remains |
| OPEN-09 | Credit/debit distinctions, Scope 2 reports, and tokenization/market trading displays | Card types, Scope 2, and market concepts defined in C11/A08/A14/C13/A15; real trading conditions remain future work |
| OPEN-10 | Contractor as a separate fourth role | The company original names three user types: customer, technician (internal/third party), and administrator. Four roles are proposed under production instructions (original message not archived). Affects FR-P01–08, the contractor column in the permission matrix, and S08. If merged, move acceptance, assignment, and quality review to external technicians |

### 7.2 Questions for the future full solution

| Tracking ID | Topic related to the original | Boundary of this phase |
|---|---|---|
| OPEN-03 | Sensors, detection accuracy, model capabilities, size, cost, installation, and firmware | Device design is out of scope; frontend uses synthetic values and supported/unsupported labels |
| OPEN-04 | Real authentication, data access scope, external integrations | Server/API/database design is out of scope; only screen switching and visibility are defined |
| OPEN-05 | Payment, notification, location, weather, and tariff service integrations | No real sending or payment; show previews and demo actions |
| OPEN-06 | Emission factors, MRV, verification, credit issuance/trading schemes, and partners | No certification of real calculations, issuance, or trading; show conditions and simulated records |

Energy waste reduction of 10–20% or more is an expected benefit. Distinguish original concepts and questions from proven capabilities for tiny refrigerant leaks, allergen detection, health safety, credit issuance from savings, and tokenization. Indoor CO₂ concentration and power-related CO₂ emissions are separate metrics.

Screen design can proceed with clear assumptions even while future technical and commercial conditions remain open. Their confirmation is not a prerequisite for frontend documentation.

## 8. Relationship to later documents

Based on this document, role requirements define what users can see and do; detailed designs define how fields, actions, states, and exceptions work; and the shared UIUX specification defines presentation and common implementation rules.

- [Common requirements](../01-requirements/common.md), plus [client](../01-requirements/client.md), [contractor](../01-requirements/contractor.md), [technician](../01-requirements/technician.md), and [administrator](../01-requirements/admin.md)
- [Common detailed design](../02-design/common.md) and role designs ([index](../README.md))
- [Shared UIUX specification](../03-uiux/UIUXSpecification.md)

Role FR numbers are project tracking IDs. Their content reorganizes the original company text and separates production instructions from added design details. Detailed designs map to these requirements. The UIUX specification separates company-based interaction/display rules from reference-mock appearance rules.

## 9. Sources and review coverage

| Source ID | Material | Role |
|---|---|---|
| SRC-06 | [Original company requirements](sources/company-requirements-original.txt), received 2026-09-15 | Primary source for company requests; basis of BIZ-01–26 |
| SRC-02 | [Production instruction record and verification status](sources/production-instructions.md) | Summary of earlier instructions; original messages not archived. Four roles and similar instructions remain the current production baseline, but the original record is not verified |
| SRC-01 | [Existing handover](sources/original-handover.md) | Secondary source for earlier reference-mock research; not primary evidence of company requirements |
| SRC-04 | Reference-site research in that handover | Earlier reference research, not a source of product feature requirements |
| SRC-05 | [Loyalty page](https://aconland-mudah-milik.vercel.app/customer/loyalty) and [design analysis](reference-design-analysis.md) | Direct HTML/CSS review on 2026-09-14; record of visual reference values |
| SRC-03 | Initial development environment check | Internal information; not evidence of company requirements or reference-mock features |

This document is for reviewing requirements and proposed coverage. It does not indicate company approval of detailed specifications, completed development, real device connections, or third-party verification.

## 10. Decision owners and deadlines

The 1A demo specification was adopted through the user response on 2026-09-16 ([DEC-12](internal/decision-record-2026-09-16.md)). The document authors and final 1A decision makers are Masaki Kitano and Yuma Wakai. This is separate from commercial approval. DEC-11 covers detailed design refinement. AI must not treat the open items below as commercially final. “Before company acceptance” means just before approving the demo as business requirements. “Before production design starts” means before 1B API/device connection design starts. “Who Should Decide” below names commercial/production responsibilities. The two final 1A decision makers are already named above. Production technical owners must be named before production design starts.

| ID | Who Should Decide | Deadline/gate | Related requirements | Adopted 1A proposal |
|---|---|---|---|---|
| OPEN-01 | Product Owner / Business | Before company acceptance | FR-P03/P05/T09/A06 | D06 assignment, quality review, and HQ handover |
| OPEN-02 | Product Owner / UI/UX / Business | Before company acceptance | FR-X01/C11/A09 | en/ms, MYR display, D03/D09 demo conditions |
| OPEN-03 | IoT | Before production design starts | FR-T11/T12/A04 | D05/D07 simulated connections and synthetic values |
| OPEN-04 | Backend / Security / IoT | Before production design starts | FR-X04, NFR03/05/06 | D11; production API/database/authentication NOT DEFINED |
| OPEN-05 | Backend / Business / Security | Before production design starts | FR-C05/C11, FR-X07 | No real sending, charging, or location collection |
| OPEN-06 | Business / Product Owner | Before production design starts | FR-C06/C13/A13/A14/A15 | D07 fixed provisional factors and simulated records |
| OPEN-07 | UI/UX / Product Owner | Before company acceptance | NFR01/02/04 | D10 acceptance environment and UIUX tokens |
| OPEN-08 | IoT / Product Owner | Before company acceptance | FR-C07/C08/T07/A05/A12 | Show evidence/not measured; no capability guarantee |
| OPEN-09 | Business / Product Owner | Before company acceptance | FR-C11/C13/A08/A14/A15 | No card data, Scope 2 demo, future market not connected |
| OPEN-10 | Product Owner / Business | Before company acceptance | FR-P01–08, FR-X04 | Keep reversible four-role proposal |
| OPEN-11 | Backend / IoT | Before production design starts | FR-T10/C04/A09 | 1A clock only; production owner of end actions and failure recovery NOT DEFINED |

Undefined production APIs and pending company approval do not justify guessing 1A behavior. The production connection implementation gate is NOT READY and is managed separately from the independent 1A G1 gate.

For 1A, DEC-12 completes demo adoption decisions for OPEN-01/02/07/08/09/10. Their commercial review and production topics OPEN-03–06/11 remain open but do not count as unresolved 1A defects. Independent AI review is G1. Final review by people and external reviewers before deployment is a separate required step.

For 0.11.0 technical fixes, see [DEC-15](internal/decision-record-2026-09-16.md#dec-15-technical-details-of-re-review-fixes-0110). They refine the existing 1A scope and have no separate approval.

Reversible design proposals adopted in 0.20.0 after another agent's independent G1 review of 0.19.0 (G1-001–031, FAIL) are recorded as PROPOSED in [DEC-54–59](internal/review-decisions-020.json). Product Owner / Business / Security / UI/UX / IoT / QA must review them before company acceptance (IR94–102).

Reversible proposals adopted in the 0.19.0 independent review (REV19-001–042) are recorded as PROPOSED in [DEC-42–53](internal/review-decisions-019.json). Product Owner / Business / Security / UI/UX / IoT must review them before company acceptance (IR75–93). The admin dashboard's estimated savings calculation (DEC-44, option A) and handling at the end of work windows (DEC-50) were confirmed as 1A specifications by the user response on 2026-09-17, separate from company commercial approval.

Reversible proposals adopted in the 0.18.0 strict review (REV18-001–048) are recorded as PROPOSED in [DEC-25–41](internal/review-decisions-018.json). Product Owner / Business / Security / UI/UX / IoT must review them before company acceptance (IR45–74).

Reversible proposals adopted in the 0.17.0 independent review (FRV) are recorded as PROPOSED in [DEC-19–24](internal/review-decisions-017.json). Product Owner / Business / Security / UI/UX must review them before company acceptance (IR35–44).

[DEC-17/18](internal/review-decisions-016.json) from the 0.16.0 re-review were confirmed by the user response. Restriction actions use two permissions, restriction.write/override. Job lists support sorting, with ascending business order as the default. See IR34 for comparison order, UI, and acceptance criteria. The earlier G1 pass does not apply to the revised baseline.

0.21.0: Independent G1 findings G120-001–005 for DOC-0.20.0 are fixed in IR103–106. Reversible demo proposals for duration and notification severity are in [DEC-60–61](internal/review-decisions-021.json). The [acceptance plan](../04-agentic-sdlc/acceptance-review-021.csv) is included in traceability, and independent G1 is reassessed for the new baseline. See the [current gate](../04-agentic-sdlc/runs/DOC-0.21.0/gate-G1.yaml) for the result.

0.22.0: The user confirmed the Figma wireframes as the final specification (DEC-62). Client structure is read-only except rename, alert policies are customer-owned and attached by units, air-quality limits are alert policies, permissions are 38 Read/Write/action values, and 18 Figma features were added as requirements (see README 0.22.0 and IR107–IR112).

0.23.0: Maintenance scheduling across the four roles (DEC-63, IR113) and confirmation of the former Figma proposal screens (DEC-64). No requirement IDs were added; FR-C09, FR-A06, FR-P02, FR-P03 and FR-T08 and their acceptance criteria were extended.

0.24.0: Use case diagrams (Figma page “Use Cases”, 126 use cases) found three spec gaps; they now have screens and rules (DEC-65, IR114). New requirement FR-C19 Customer users (owner) with SCR-C19 `/customer/users`; coordination notes (Client 07o/07p) and follow-up classification (Admin 06-17/06-18) use existing operations.
0.25.0: Consistency check of use cases, user flows, screens and documents (DEC-66, IR115). Dashboard visibility uses billing.read/energy.read, default-policy rule switching is owner-only for clients, follow-up classification traces to FR-A06, and tab/URL contracts follow Figma. No new requirement IDs.
0.26.0: Frontend stack fixed to Next.js (App Router) and production backend / network designed at a logical, cloud-agnostic level (DEC-67, IR116). Phase 1A scope unchanged.
0.27.0: Production target on AWS with Stripe payments and HQ access limited to the company network; retention, capacity and customer office firewall requirements defined (DEC-68, IR117). Phase 1A unchanged.
0.28.0: Backend implementation design in Go + Echo and database design with an executable PostgreSQL schema (DEC-69, IR118). Phase 1A unchanged.
0.29.0: Everything runs in Docker — image catalogue, Dockerfile standards, Compose stack with local stand-ins for AWS services, ECS Fargate runtime (DEC-70, IR119). Phase 1A unchanged.
0.82.0: Notifications by events, identity membership queries (IR191). Phase 1A unchanged.
0.81.0: Read models by API composition (IR190). Phase 1A unchanged.
0.80.0: Energy reference copies, principal timezone (IR189). Phase 1A unchanged.
0.79.0: Notify reference copies via generic change capture (IR188). Phase 1A unchanged.
0.78.0: Diagnostic-run scope from the assignment projection (IR187). Phase 1A unchanged.
0.77.0: Unit scope without maintenance reads (IR186). Phase 1A unchanged.
0.76.0: No cross-domain writes (IR185). Phase 1A unchanged.
0.75.0: Restriction → equipment events (IR184). Phase 1A unchanged.
0.74.0: Outbox event delivery (IR183). Phase 1A unchanged.
0.73.0: Principals from identity-api (IR182). Phase 1A unchanged.
0.72.0: Phase B inventory (IR181). Phase 1A unchanged.
0.71.0: Domain microservices + gateway (IR180). Phase 1A unchanged.
0.70.0: service/api and service/web (IR179). Phase 1A unchanged.
0.69.0: One web app per entry point (IR178). Phase 1A unchanged.
0.68.0: Server-rendered screens, local HQ TOTP (IR177). Phase 1A unchanged.
0.67.0: Server-side DAL pilot (IR176). Phase 1A unchanged.
0.66.0: Next.js 16 step 1 (IR175). Phase 1A unchanged.
0.65.0: Go server layout cmd/ + internal/ (IR174). Phase 1A unchanged.
0.64.0: Echo v5 per the official guide (IR173, DEC-71). Phase 1A unchanged.
0.63.0: Membership.displayName (IR172). Phase 1A unchanged.
0.62.0: Test database isolation (IR171). Phase 1A unchanged.
0.61.0: One unit scope for every module (IR170). Phase 1A unchanged.
0.60.0: Unit read scope for external technicians and contractors (IR169). Phase 1A unchanged.
0.59.0: Shared demo clock, ack → observedState, BFF refresh (IR168). Phase 1A unchanged.
0.58.0: Migration service `service/migrate` (IR167). Phase 1A unchanged.
0.57.0: Service directory layout under `service/` (IR166). Phase 1A unchanged.

0.56.0: UnitDetail parts and control blocking during recovery (IR165). Phase 1A unchanged.

0.55.0: Terminal-restriction recovery cases (IR164). Phase 1A unchanged.

0.54.0: Result-type conformance sweep (IR163). Phase 1A unchanged.

0.53.0: BFF action check; jobs.addNote result (IR162). Phase 1A unchanged.

0.52.0: Demo business records seeded for the Core API (IR161). Phase 1A unchanged.

0.51.0: Result meta and the server clock for screens (IR160). Phase 1A unchanged.

0.50.0: Demo clock start and full-stack BFF check (IR159). Phase 1A unchanged.

0.49.0: Web BFF and the first API-backed screen (IR158). Phase 1A unchanged.

0.48.0: One business clock in the database (IR157). Phase 1A unchanged.

0.47.0: Business-event notifications for the backend (IR156). Phase 1A unchanged.

0.46.0: Alert-policy notification evaluation for the backend (IR155). Phase 1A unchanged.

0.45.0: Demo operations and demo sessions in the Core API (IR154). Phase 1A unchanged.

0.44.0: Voice intents and the energy export for the backend (IR153). Phase 1A unchanged.

0.43.0: Automation evaluation for the backend (IR152). Phase 1A unchanged.

0.42.0: Customer automation rules for the backend (IR151). Phase 1A unchanged.

0.41.0: Offset quotes and simulated records for the backend (IR150). Phase 1A unchanged.

0.40.0: MRV reports for the backend (IR149). Phase 1A unchanged.

0.39.0: Energy summary and admin dashboard for the backend (IR148). Phase 1A unchanged.

0.38.0: Emission factors, energy integration and baselines for the backend (IR147). Phase 1A unchanged.

0.37.0: Role summaries for the backend (IR146). Phase 1A unchanged.

0.36.0: QR resolution, write results and password-reset preview for the backend (IR145). Phase 1A unchanged.

0.35.0: Client users and two-step verification for the backend (IR144). Phase 1A unchanged.

0.34.0: Inquiries and audit search for the backend (IR143). Phase 1A unchanged.

0.33.0: Notification inbox, recipients and preview, preferences and consents for the backend (IR142). Phase 1A unchanged.

0.32.0: Restriction grace/exception, explicit and forced release, reconcile and retry for the backend (IR141). Phase 1A unchanged.

0.31.0: Restriction schedule, execution, command results, release evaluation, cancellation and reads for the backend (IR140). Phase 1A unchanged.

0.30.0: Alert-policy details for the backend — default rules, HQ limit edits, field and list rules (IR120); telemetry and ventilation reads (IR121); job intake, notes, hold and list filters (IR122); offers, decisions and assignments (IR123); contractor/technician projections (IR124); technician on-site operations (IR125); work reports (IR126); follow-up, cost, access and warranty operations (IR127); time proposals and reschedules (IR128); attachments and sign-off (IR129); maintenance plans (IR130); contractor register, rate cards and SLA (IR131); memberships, eligibility, capacity and unavailability (IR132); certificates and parts catalog (IR133); filter care (IR134); contracts, invoices and reminders (IR135); payments (IR136); contractor payouts (IR137); unit commands (IR138); diagnostic test runs (IR139). Phase 1A unchanged.
