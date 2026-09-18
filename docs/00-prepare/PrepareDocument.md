---
document_id: PREP-001
version: 0.21.0
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

[DEC-17/18](internal/review-decisions-016.json) from the 0.16.0 re-review were confirmed by the user response. Restriction actions use two permissions, restriction.manage/override. Job lists support sorting, with ascending business order as the default. See IR34 for comparison order, UI, and acceptance criteria. The earlier G1 pass does not apply to the revised baseline.

0.21.0: Independent G1 findings G120-001–005 for DOC-0.20.0 are fixed in IR103–106. Reversible demo proposals for duration and notification severity are in [DEC-60–61](internal/review-decisions-021.json). The [acceptance plan](../04-agentic-sdlc/acceptance-review-021.csv) is included in traceability, and independent G1 is reassessed for the new baseline. See the [current gate](../04-agentic-sdlc/runs/DOC-0.21.0/gate-G1.yaml) for the result.
