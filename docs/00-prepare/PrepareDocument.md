---
document_id: PREP-001
version: 0.517.0
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
| BIZ-02 | Display in a chosen language | Language switching and translatable screens; initial languages defined separately | Initial English and Malay support is a design proposal; Japanese was added by DEC-99 |
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
| DEC-73 | A completed job releases its assignment at completion: the Assignment becomes completed, stops booking the technician and granting unit access, and the technician keeps the job as history | User request in this conversation, 2026-10-10 (“完了したジョブの割り当ては完了時に解除してください”); reflected in IR234 |
| DEC-74 | Every icon of the four web apps and the Figma screens comes from one icon library, Lucide (`lucide-react`, pinned), through one shared `Icon` component with semantic names. It is the one exception to DEC-03: glyphs and emoji render differently per platform and font, some as colour emoji, with mixed weights and sizes, and in-house glyphs cannot give one consistent style. Cost: one ISC-licensed dependency with no dependencies of its own; the icons are inline SVG and tree-shaken. Replacement stays possible by changing the one map | User request in this conversation, 2026-10-12 (“アイコンが、統一感がありません。絵文字？のようになってしまっている箇所があります。アイコンを全て同じアイコンライブラリからとってきて、統一感を出してください”); reflected from IR393 |
| DEC-75 | The UI is built from shadcn/ui components (style base-nova on Base UI, Tailwind CSS 4, Lucide), and established libraries do the logic where one exists; this supersedes DEC-03 (the 2026-10-10 in-house components, forms and dictionaries). Libraries: React Hook Form and Zod (forms, through shadcn Form/Field), Sonner (toasts), TanStack Table (tables), Recharts (charts), cmdk (command, combobox), react-day-picker with date-fns (date picker), input-otp (one-time code). Server Components and Server Actions stay for data, the Next.js way; TanStack Query only where a client component needs a cache, polling or optimistic updates; the translation library is decided in its own phase (the English-keyed dictionary is large). Phases: 1 foundation and the product components on it, 2 forms, 3 dialogs, menus and selects, 4 tables, charts and toasts, 5 screens on shadcn directly with a guard against hand-built UI, 6 Figma restyled, 7 translation. Cost: the components are copied into the repository (`shared/components/ui/`) and restyled to the UI Guideline; runtime dependencies are pinned exactly and the shadcn CLI is a development dependency; the screens keep their roles and names, so the E2E suite stays the check | User request in this conversation, 2026-10-12 (“UIですが、統一性を持たせるために、UIComponentを使いたいのですが、判断として妥当でしょうか”; then “shadcn/uiの導入を進めてください。できるだけ、chadcnのコンポーネントを使ってください。UI、ロジックはなるべく既存ライブラリを使いたいです”); reflected from IR400 |
| DEC-76 | A contractor manages its own company's accounts: it invites contractors and technicians by e-mail, changes or resends pending invites, and disables, enables and removes accounts, never its own and never the last active contractor; HQ keeps permissions, qualifications and access periods (FR-A03) | User request in this conversation, 2026-10-13 (“Contractorでもユーザー管理の画面や機能を作ってください。Contractorは同じ組織のContractorとTechnicianを管理することができます。”); reflected in IR416 |
| DEC-77 | A customer owner manages the customer's users in the customer app: invites members and owners, edits or resends pending invites, changes roles, disables, enables and removes users with a reason, never the own account and never the last active owner; HQ keeps password resets (supersedes the IR114 limits of DEC-65) | User request in this conversation, 2026-10-13 (“クライアント画面でユーザー管理ができるようにしてください”); reflected in IR417 |
| DEC-78 | E-mails are sent for real: in development through Mailpit, a local mail catcher with a web inbox (nothing leaves the machine), and in production through Amazon SES with the same notification worker | User request in this conversation, 2026-10-13 (“メール送信までやって見たいです” → option A “Aでいきましょうか”); reflected in IR418 |
| DEC-79 | Accounts are created where people are added: HQ adds anyone (a new person in Access & roles), a contractor adds its contractors and technicians, a customer owner adds owners and members; the sign-in account (Keycloak locally, Cognito in production) is opened at once and the person gets an e-mail to set their password. A technician a contractor adds gets an access end of at most a year, set by the contractor; HQ still verifies their certificates before they can be assigned. One account per person (an e-mail is used once). Supersedes the invites of DEC-76 / DEC-77 that only HQ could turn into accounts | User request in this conversation, 2026-10-24 (“TechnicianはContractor ,ADminどちらも作れるようにした方がいい”, “Client側でもClientを追加できるようにしましょうか”, option 1 “案１でお願いします”); reflected in IR461 |
| DEC-80 | When one schedule ends and another starts on the same unit in the same minute, the start wins if it would run: the earlier schedule's end is not sent, and its run is recorded as superseded. A start that would not run leaves the end | User decision in this conversation, 2026-10-30 (“全て推奨で修正してください”, the recommended option of IR326 item 4); reflected in IR504 |
| DEC-81 | A contractor or a customer owner adds at most 30 accounts to the organization in 24 hours (IR464 kept); HQ is not limited, and `ACCOUNTS_PER_ORG_PER_DAY` sets another value per environment | User decision in this conversation, 2026-10-30 (“全て推奨で修正してください”, business review question 3); reflected in IR507 |
| DEC-82 | Ending a contract early — its unpaid invoices, removing the units, releasing restrictions and the contractor's payout — is outside Phase 1A and is designed before production (OPEN-07) | User decision in this conversation, 2026-10-30 (“全て推奨で修正してください”, business review question 4); reflected in IR507 |
| DEC-83 | HQ's revoke in Access & roles also turns the person's sign-in off, and only HQ gives the access back: a contractor or customer owner cannot extend the access end, enable the account again or add the person as a new account; a later end set by HQ clears the revoke and turns the sign-in on, unless the company or customer disabled the account | User decision in this conversation, 2026-10-30 (“全て推奨で修正してください”, business review question 2); reflected in IR508 |
| DEC-84 | The completion criteria include starting from one HQ administrator in an empty environment: a bootstrap command creates a tenant's first administrator (the tenant and its HQ organization when missing, the sign-in account with the two-step set-up and the set-up e-mail) and is refused once the tenant has an active identity administrator; everything else is added in the apps | User decision in this conversation, 2026-10-30 (“全て推奨で修正してください”, business review question 1); reflected in IR509 |
| DEC-85 | HQ two-step verification is enforced by the identity provider: in production HQ signs in through its own Amazon Cognito user pool with MFA required; locally the Keycloak realm's HQ app flow asks for the password and the one-time code at every sign-in; the Core API takes HQ memberships only from the HQ app's tokens | User decision in this conversation, 2026-10-30 (“全て推奨で修正してください”, business review question 5); reflected in IR510 |
| DEC-86 | Real sensor nodes feed the development stack: the IoT bridge subscribes to the broker named in `.env.local` (`REAL_MQTT_URL` and its login) and publishes there nothing but the live unit's power commands (since DEC-89; nothing before). It records the nodes' room temperature, humidity and power draw, one reading per sensor and minute on the scenario clock, on a monitoring-only air conditioner ("Live sensors"; its customer since DEC-87: a client of its own; remote power since DEC-89). The broker's address and login stay out of the repository; production devices keep AWS IoT Core | User request in this conversation, 2026-10-30 (“このデータをシステムに取り込んでください”); reflected in IR524 |
| DEC-87 | (Superseded by DEC-92.) The real device belongs to a client of its own, not to customer-b (which keeps the mock fleet): "Live Sensor Customer" with the owner `live-sensors@example.com`, whose sign-in account is opened as HQ adds a customer owner, and who sets the password from the set-up e-mail (Mailpit in development) | User request in this conversation, 2026-10-30 (“リアルデバイスと繋げるのは、別ユーザーにしてください（クライアント”); reflected in IR525 |
| DEC-88 | (Superseded by DEC-89.) Each real sensor node is a device of its own on a unit of its own, because a unit has at most one active device: node_1 (room temperature and humidity) and node_2 (power), each with its own monitoring-only model, both in Live Sensor Customer's Sensor room | User request in this conversation, 2026-10-30 (“もう一つデバイスがあるはずなので、それも同じ組織に入れてください”); reflected in IR526 |
| DEC-89 | Both real nodes serve one Daikin air conditioner: they are one module on one unit ("Live · Daikin AC"). The app switches it on and off through node_1's infrared LED, and sending to the nodes' broker is on (`REAL_MQTT_COMMANDS=1`). Power on sends the firmware's COOL24 and power off sends OFF; the other settings wait for a firmware that takes them | User answers in this conversation, 2026-10-30 (“同じエアコン（1台）”, “有効にしてよい”, the firmware and “DAIKINです”); reflected in IR528 |
| DEC-90 | The UX loop's proposals of 2026-11-02 are taken as recommended. Build: (1) HQ Overview counts what waits for HQ regardless of the period; (3) on touch screens every control is at least 44 px; (4) the customer overview names an overdue invoice and the restriction it caused; (5) contractors list and remove their technicians' unavailable days, and same-name technicians show their e-mail; (6) a technician keeps the notices of a job whose assignment ended while its history snapshot is theirs; (7) a contractor's notice of an offer withdrawn before acceptance has no link; (8) a job history without a completion counts in the period by its delegation's end; (9) `telemetry.series` returns server-side averages for long windows; (10) a policy without a cooldown notifies once per open alert; (11) the partner's Return for rework lists the items to redo, as HQ's does; (12) the development simulator and bridge behave like real modules; (13) the audit log records denied writes, throttled, and then pending and failed results. Keep as built: (2) contractor capacity per preferred time stays out of Phase 1A | User decision in this conversation, 2026-11-02 (“すべて推奨どおりお願いします”); reflected from IR600 on |
| DEC-91 | node_2 reports the room too (temperature and humidity next to its power), so its temperature and humidity go to a sensor unit of their own, "Live · node_2 sensors", in the same Sensor room: a module has one sensor per metric, and node_1's readings stay the air conditioner's. Its model has no remote control. node_2's power stays on "Live · Daikin AC", and each unit's connection and AC offline follow its own nodes | User request in this conversation, 2026-11-04 (“node_2を取り込んでください”), and the answer “別ユニットを追加”; reflected in IR627 |
| DEC-92 | (In the shared test environment superseded by DEC-96.) The default client users split the demo from the real devices, so that a test deployment others open shows the same state from the seed alone: customer-a sees the demo data only, the mock fleet's simulated air conditioners included, and customer-b sees the real sensor nodes' live site, a property of Demo Customer B. The seed makes no customer or owner of its own for the live site (supersedes DEC-87). While the live site is seeded, customer-b keeps its access without the fixture's end date. The other default users stay as they are. In the development stack the existing site and fleet were moved with their history, and the empty "Live Sensor Customer" stays there | User requests in this conversation, 2026-11-04 (“customer-aがデモデータのみ、customer-bが実際のデバイスからのデータを取得するようにSeedで入れたいです。Clientだけです”, “テスト環境へデプロイして、他の人も同じ環境が見れるようにするのが目的です”), and the answers “customer-a に移す”, “seed から外し、dev DB には残す” and “customer-b だけ期限なしにする”; reflected in IR628 |
| DEC-93 | Continuous deployment of a shared test environment: pushing a tag `test<major>.<minor>.<patch>r<n>` (for example `test1.0.0r0`) on a commit of main whose CI passed deploys the whole stack to the lab server that runs the real nodes' broker. Others open it from the lab network over HTTP at the server's address (apps 3200–3203, Keycloak 8281, Mailpit 8225). The server pulls: a timer checks the tags every minute and builds the images there, because the server is out of GitHub's reach and the repository is public (no self-hosted runner, no registry token). The user runs the one-time set-up on the server. The data stays between deployments and is built again from the seed when the migrations changed. The app may switch the real air conditioner from there | User requests in this conversation, 2026-11-04 (“test1.0.0r0のようにタグを打ってPushすると、MQTTサーバーと同じサーバーにデプロイされるようにしたいです”), the server's facts the user gave, and the answers “私が実行する”, “IP とポート番号で HTTP”, “許す” and “スキーマが変わったときだけ作り直す”; reflected in IR629 |
| DEC-94 | The repository becomes private, and the shared test environment is deployed by the lab server as the repository's self-hosted runner (label `ac-test`): a test tag runs the workflow "Test deployment" there, which waits for the commit's CI and builds and starts the stack, so the build and deploy logs show on the Actions tab. This replaces DEC-93's timer that pulled the tags and the deployment statuses of IR630. A private repository keeps the runner to collaborators' code; its Actions minutes are limited on the free plan, and the runner's own minutes cost nothing | User request in this conversation, 2026-11-04 (“ビルドとかのログをActionsで見たいかも”), and the answer “リポジトリを非公開にして runner”; reflected in IR631 |
| DEC-95 | CI and E2E run on the lab server's self-hosted runner too, so the private repository's Actions minutes stay unused: each CI job in a container of its own, the E2E stack as a compose project of its own beside the server's services, and Playwright in its own image. E2E runs every night on main and by hand, no longer on each push and pull request. The test deployment no longer waits for CI on the runner: a tag deploys at once when its CI passed, else when that CI completes | User requests in this conversation, 2026-11-04 (“runnerで動かす形にしてください”, after the note that a private repository has 2,000 free Actions minutes a month and CI with E2E use about an hour a push; then “e2e、無くしちゃいましょうか” and the answer “手動＋毎晩 1 回”); reflected in IR632 |
| DEC-96 | The shared test environment holds the fixture's accounts and the real devices only. The seed loads the tenants, the organizations, the default users with their permissions, the customers with their user lists, the consents and the default alert policy, and none of the demo: no demo units, models, devices, readings, contracts, invoices, jobs, alerts, notifications, emission factor or mock fleet, and neither the technicians' scopes to the demo's units nor the demo setting that switches customer-a's offline rule off. The live site, the real air conditioner and node_2's sensor unit, belongs to Demo Customer A, so customer-a sees those two units. The scenario clock is the real time, and no access ends: an external technician's access, which must end (FR-A03), ends on 2099-12-31, and the qualifications hold as long. The local development stack and E2E keep the demo (DEC-92) | User request in this conversation, 2026-11-04 (“テスト環境のデータですが、モックデータは排除しましょうか。そのため、customer-aのみで本物のデバイスを接続しましょうか。なので２台のみになります”), and the answers “アカウント以外のデモを全部”, “実際の時刻” and “今のまま”; reflected in IR633 |
| DEC-97 | A device is registered with the MQTT node ID of its ESP32 (HQ, in the admin app's devices), and the IoT bridge takes readings and sends commands by the IDs it reads from the database every 30 seconds, instead of a table in the code. One device serves one air conditioner with one node; the serial keeps its own format, and the node ID is matched exactly, one device per ID. A node nobody registered is heard but not recorded. A new node needs no release, only the registration. The development seed follows the rule: node_1 is the air conditioner's device and node_2 the device of its own sensor unit, which also holds the air conditioner's supply power. | The user: the nodes of a new device did not come in when its serial was set to the MQTT ID, and asked to register the devices from Admin instead of the seed; answers “one device per air conditioner” and “the command node too”; reflected in IR635 |
| DEC-98 | The visit time is settled when a technician is assigned, not when HQ offers the job to a contractor. The offer carries every time the client agreed; the contractor picks one when it assigns a technician and may move to another open one when the technician cannot make it; the client is told the confirmed time when the contractor assigns (IR636). HQ's own technicians are assigned at one time as before. | User request 2026-11-07 (“メンテナンスのjobが任された場合、customer→admin→contractorの順に降りていくと思いますが、adminの時点で候補の日程を絞るのはおかしい。テクニシャンに渡すタイミングでメンテナンスの日程を絞るべきである。”), and the answers “選び直せる” and “割当時に通知する”; reflected in IR636 |
| DEC-99 | Japanese is a third display language beside English (default) and Malay. Every text the apps translate has a Japanese text, the voice assistant has a Japanese grammar (`温度 <room>`, `<room>を<integer>度に設定`, `ヘルプ`), and Preferences offers it under its own name (日本語). Dates and times use Japan's formats with the zone's abbreviation (MYT); the payout statement PDF stays English because its font has ASCII only. Like the Malay text, the Japanese text is a draft until Business / UI / UX review it. | IR639 |
| DEC-100 | Each app's sign-in page is a product page. It names the service (Client, Partner, Technician, HQ Admin), takes the e-mail address and continues to the identity provider's own pages for the password, the one-time code and a forgotten password, which the identity provider resets by e-mail. The demo-account button (“Continue as customer-a”) and the “not real authentication” notices are removed from the apps that have an identity service; the browser demo build keeps its demo continue. The identity provider's pages have an AC Project theme (one for Client, Partner and Technician, one for HQ Admin) in English, Bahasa Melayu and 日本語, and the language is chosen on the sign-in page. The Bahasa Melayu and 日本語 texts are drafts until Business / UI / UX review them. | User request 2026-11-08 (“各サービスのログイン画面について、デモで全て固定なので、よしなに製品版のログイン画面をそれぞれ作成してください”; answers: the identity provider's pages are branded, the demo entry is removed, one common theme plus HQ, language choice and a real reset); IR646 |
| DEC-101 | The AI assistant of the Client app is removed completely: the header button and panel, `voice.resolveIntent`, the microphone consent, the `voice` source of a command (an existing voice command becomes `ui`, and its history label “· by voice” goes), the assistant's texts and the Figma Client screens 09a–09g. FR-X02 stays as a number and is recorded as withdrawn; BIZ-03 (voice AI responses) is out of scope. The location consent stays, for location automations. Temperature is changed in Unit Control. | User request 2026-11-08 (answers: delete the past data, record FR-X02 as withdrawn); IR648 |
| DEC-102 | The shared test environment answers on more than one address: the lab network's and the Tailnet's. The server's environment file lists them in `PUBLIC_HOSTS`; a sign-in that starts on one address goes to Keycloak on that address and comes back to the app on it. Keycloak takes its host from the request, behind an nginx that answers only the listed hosts, so that a header anyone can send cannot choose where an e-mail link points. Unset, nothing changes (one fixed address, as before). | User request 2026-11-08 (“ローカルIPである10.100.1.17とTailnetのIPである100.99.169.116どちらも使えるようにしたい”; the Tailscale subnet route needed the Tailnet's administrator, so the code takes both addresses); IR651 |
| DEC-103 | HQ can delete what it registers, area by area. A registered thing that is in no use is deleted from its admin screen with a reason; one that is in use is refused with the reason and what to do first, never cascaded. Records that stand for what happened (contracts, invoices, payments, jobs, reports, certificates) are not deleted; they are cancelled or ended as before. Phase 1: devices (an Unbind step, and Delete of a device that is not bound, has no operation under way and is in no unfinished firmware campaign) and models (Delete of a model no unit uses and no firmware campaign names). | User request 2026-11-08 (“デバイスなど、登録できるものに関して、削除ができないです。CRUD全てできるようにしてください。基本はAdmin画面からだと思います”; answers: all master data, refuse with the reason, one pull request per area); IR652 |

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
| OPEN-07 | Ending a contract early: its unpaid invoices, removing the units, releasing restrictions, and the contractor's payout (DEC-82) | Outside Phase 1A; designed before production |

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
0.517.0: a membership names each scope once, as it names each permission once; a repeated scope no longer slips past the scope-version check (IR661). No operation changes. Phase 1A unchanged.
0.516.0: an emission factor too small for the six-decimal column is refused on its field instead of failing the database check (IR660). No operation changes. Phase 1A unchanged.
0.515.0: a device that rejects the start or the end Command of a diagnostic test run ends the run as `start_failed` / `end_failed` with the device's code (IR658). No operation changes. Phase 1A unchanged.
0.514.0: HQ deletes an account that was made by mistake and never acted, with its person and provider account: `members.delete` (225 operations, 246 routes; DEC-103 phase 3, IR657). Phase 1A unchanged.
0.513.0: HQ deletes a baseline and an emission factor no MRV report cites, and an MRV report no review touched: `baselines.delete`, `factors.delete`, `mrv.deleteDraft` (224 operations, 245 routes; DEC-103 phase 5, IR656). Phase 1A unchanged.
0.512.0: HQ deletes a maintenance plan that has made no job and a rate card that has not started: `plans.delete`, `rateCards.delete` (221 operations, 242 routes; DEC-103 phase 4, IR655). Phase 1A unchanged.
0.511.0: HQ deletes a customer or a contractor company registered by mistake: `customers.delete`, `contractors.delete`, `organizations.delete` (219 operations, 240 routes; DEC-103 phase 2, IR654). Phase 1A unchanged.
0.510.0: HQ deletes a firmware campaign that has ended: `firmwareCampaigns.delete` (216 operations, 237 routes; DEC-103 phase 1b, IR653). Phase 1A unchanged.
0.509.0: HQ deletes devices and models that are in no use, after an Unbind step: `devices.unbind`, `devices.delete`, `capabilities.delete` (215 operations, 236 routes; DEC-103, IR652). Phase 1A unchanged.
0.508.0: The shared test environment answers on several addresses (PUBLIC_HOSTS): a sign-in returns to the address it started on (DEC-102, IR651). Phase 1A unchanged.
0.507.0: The AI assistant is removed: the Client header button, `voice.resolveIntent` (212 operations, 233 routes), the microphone consent and the `voice` command source; FR-X02 is withdrawn (DEC-101, IR648). Phase 1A unchanged.
0.506.0: The sign-in pages are product pages: e-mail address, the identity provider's own pages for the password, the one-time code and a reset, a language choice, no demo account button (DEC-100, IR646). Phase 1A unchanged.
0.505.0: Japanese is a third display language, with a Japanese voice grammar (DEC-99, IR639). Phase 1A unchanged.
0.504.0: The visit time is settled when the contractor assigns a technician: an offer carries every time the client agreed (DEC-98, IR636). Phase 1A unchanged.
0.503.0: A device is registered with the MQTT node ID of its ESP32, and the IoT bridge reads the nodes from the database instead of the code (DEC-97, IR635). Phase 1A unchanged.
0.502.0: The shared test environment holds the fixture's accounts and the real devices at customer-a only, on the real clock (DEC-96, IR633). Phase 1A unchanged.
0.501.0: CI and E2E run on the lab server's runner too, in containers and beside its services (DEC-95, IR632). Phase 1A unchanged.
0.500.0: The test deployment runs on the lab server as the private repository's self-hosted runner, with its logs on the Actions tab (DEC-94, IR631). Phase 1A unchanged.
0.499.0: Each test deployment shows its progress under the repository's Deployments on GitHub (DEC-93, IR630). Phase 1A unchanged.
0.498.0: A test tag deploys the stack to the lab server for others to open (DEC-93, IR629). Phase 1A unchanged.
0.497.0: customer-a sees the demo data and the mock fleet, customer-b the real devices; the live site has no customer of its own (DEC-92, IR628). Phase 1A unchanged.
0.496.0: node_2's room temperature and humidity are recorded on a sensor unit of their own, "Live · node_2 sensors" (DEC-91, IR627). Phase 1A unchanged.
0.495.0: The four apps show no error codes, spec IDs, status codes or code names, and guards keep it so (IR626). Phase 1A unchanged.
0.494.0: The technician's Add part and Refrigerant refuse under the field concerned; pausing keeps Time on site open and the focus on its button (IR625). Phase 1A unchanged.
0.493.0: The partner schedule leaves former technicians and ended delegations out, and its assignment notes lose the Japanese term (IR624). Phase 1A unchanged.
0.492.0: The customer's alert policy editor focuses and marks the field to fix; the AC page words policies and counts the default rules that are on (IR623). Phase 1A unchanged.
0.491.0: Buttons keep the focus while their action runs; Access & roles focuses the field to fix, starts a new membership now and focuses the revoked row (IR622). Phase 1A unchanged.
0.490.0: The technician's check-in refuses under the field concerned, and the job's title takes the focus once the work has started (IR621). Phase 1A unchanged.
0.489.0: Table rows keep their buttons across renders, so a dialog gives the focus back to the row; the Users dialogs focus the field to fix (IR620). Phase 1A unchanged.
0.488.0: The customer's location consent card warns only when a withdrawal turned automations off (IR619). Phase 1A unchanged.
0.487.0: Icon and text actions are 44 px on touch screens too, and the phone crawl measures every button (IR618). Phase 1A unchanged.
0.486.0: Every filter chip and tab built from Plain is 44 px tall on touch screens (IR617). Phase 1A unchanged.
0.485.0: The technician's job tabs and device filters are touch-sized, and a device filter that hides the open device opens its first device, for HQ too (IR616). Phase 1A unchanged.
0.484.0: On touch screens the shared controls are at least 44 px tall, checked by the phone crawl (IR615). Phase 1A unchanged.
0.483.0: The job history's period counts a cancelled or unfinished job by when its delegation ended, and the row shows that time (IR614). Phase 1A unchanged.
0.482.0: In development the simulated fixture modules play their seeded connection and their heartbeats keep last seen current, as the real module's do; times of another day keep their date on the customer's unit page, and a device's long silence counts days (IR613). Phase 1A unchanged.
0.481.0: Contractors see their technicians' upcoming unavailable days and remove them from a chosen day on (`members.unavailability`, `members.clearUnavailability`; 212 operations, 233 routes); technicians of the same name show their e-mail (IR612). Phase 1A unchanged.
0.480.0: HQ's overview shows a "Waiting for HQ" tile of the requests to book, independent of the period (IR611). Phase 1A unchanged.
0.479.0: The customer overview shows a notice of an overdue invoice and the restriction it caused (IR610). Phase 1A unchanged.
0.478.0: The contractor's Return for rework lists the items that need rework, as HQ's does (IR609). Phase 1A unchanged.
0.477.0: The contractor's unit diagnosis charts read the whole day in hourly averages (IR608). Phase 1A unchanged.
0.476.0: The customer's air page and overview air card read whole windows in averages, and the air table is keyboard-scrollable (IR607). Phase 1A unchanged.
0.475.0: The technician's day and week charts read the whole period in averages (IR606). Phase 1A unchanged.
0.474.0: telemetry.series answers bucketMinutes (5 or 60) with the whole period in averages (IR605). Phase 1A unchanged.
0.473.0: A person's device command is audited pending and gets its result (success, or failed with the reason) under the same correlation ID (IR604). Phase 1A unchanged.
0.472.0: Writes refused for permission or scope leave a denied audit entry, at most one per actor and operation a minute (IR603). Phase 1A unchanged.
0.471.0: Notifications say whether their target opens for the reader (targetOpen), and a withdrawn offer's notice has no link (IR602). Phase 1A unchanged.
0.470.0: A technician reads the notices of a job whose history snapshot is theirs, through a new reference copy (IR601). Phase 1A unchanged.
0.469.0: The UX loop's proposals are decided (DEC-90); a policy without a cooldown notifies once per open alert (IR600). Phase 1A unchanged.
0.468.0: The audit explorer says that denied, failed and pending attempts are not recorded, and tells same-name actors apart (IR599). Phase 1A unchanged.
0.467.0: The technician's unit monitoring names the readings it did not load and dates the start of the 24 h axis (IR598). Phase 1A unchanged.
0.466.0: The partner's quality review uses the technician's words for items and results, and a refused return focuses the reason (IR597). Phase 1A unchanged.
0.465.0: The customer's demo offset quote explains its missing price (IR596). Phase 1A unchanged.
0.464.0: HQ restriction texts drop their spec IDs (IR595). Phase 1A unchanged.
0.463.0: The work report's next-action controls are labelled; a refused submit focuses the first item to fix; codes in parentheses leave partner and technician texts (IR594). Phase 1A unchanged.
0.462.0: A refused certificate submit focuses the first field to fix; the certificate and photo pickers show keyboard focus (IR593). Phase 1A unchanged.
0.461.0: The air quality table labels the hours before a cut-off series' oldest reading "Not loaded" (IR592). Phase 1A unchanged.
0.460.0: HQ customers & units texts carry no codes or IDs; "Remove permanently" is disabled for a unit in use (IR591). Phase 1A unchanged.
0.459.0: Technician texts carry no error codes or spec IDs, and a refused diagnostic review focuses the first field to fix (IR590). Phase 1A unchanged.
0.458.0: A cancelled job closes the contractor's unit access immediately (IR588); an empty 90-day history offers "Show all time" (IR589). Phase 1A unchanged.
0.457.0: Customer notes name no spec IDs and the demo report says nothing is downloaded or e-mailed (IR587). Phase 1A unchanged.
0.456.0: A failed baseline save focuses the first field to fix, focused elements stay below the sticky header in every app, and an empty period reads "Enter the start and the end" (IR586). Phase 1A unchanged.
0.455.0: The technician's device screen speaks in words (failures, refusals, the running operation) and its phone tables keep a gap between columns (IR585). Phase 1A unchanged.
0.454.0: Contractor texts name no error codes or spec IDs; an empty or filtered payouts page says why and offers "Clear the filters" (IR584). Phase 1A unchanged.
0.453.0: A restricted AC says the restriction is payment-related and links to its detail (IR583). Phase 1A unchanged.
0.452.0: Every HQ sidebar page names the permission a role lacks, as "To open <page>, your role needs <permission>." (IR582). Phase 1A unchanged.
0.451.0: The HQ overview and alerts pages name the permission a role lacks instead of failing with Try again (IR581). Phase 1A unchanged.
0.450.0: The notifications note names no requirement IDs in the four apps (IR580). Phase 1A unchanged.
0.449.0: The partner team screen sends account changes to Users and names no API (IR579). Phase 1A unchanged.
0.448.0: The customer's WhatsApp reminder preview is disabled with its reason when the account has no phone number, and a refused recipient reads in words (IR578). Phase 1A unchanged.
0.447.0: The HQ overview shows its rules in business words, its row links as View and its forecast warnings in words (IR577). Phase 1A unchanged.
0.446.0: Calibrate is a small button like the other device actions, in the technician and HQ apps (IR576). Phase 1A unchanged.
0.445.0: The partner overview lists only the technicians who can work, busiest first, eight per card with a link to the rest (IR575). Phase 1A unchanged.
0.444.0: A client's booked visit reads Scheduled, the word of its tab, instead of HQ's Offered / Accepted / Assigned (IR574). Phase 1A unchanged.
0.443.0: A field with an error is marked invalid, a failed new maintenance request focuses the field to fix, and the primary button keeps its contrast on hover (IR573). Phase 1A unchanged.
0.442.0: The HQ jobs page names the first three contractors by name and drops internal IDs from its texts (IR572). Phase 1A unchanged.
0.441.0: Page unavailable leads a signed-in user to their Overview and shows no design note (IR571). Phase 1A unchanged.
0.440.0: The technician's job page names its unit before the window, and its banners name the visit once and show no internal IDs (IR570). Phase 1A unchanged.
0.439.0: The contractor's offer page and job detail list the technicians who can take the visit first (IR569). Phase 1A unchanged.
0.438.0: Every page of the four apps starts with a "Skip to main content" link (IR568); the UX loop keeps its log and proposals locally in 05-document-review (ignored by git). Phase 1A unchanged.
0.437.0: The scheduler's Run wrapper, called only by a test, is removed; RunDomains with no domain list runs every domain (IR567). Phase 1A unchanged.
0.436.0: The unused copy of the component groups in the unit library is removed; techJob's is the one definition (IR566). Phase 1A unchanged.
0.435.0: The unused Kuala Lumpur helpers of the contractor job detail and the unit library are removed (IR565). Phase 1A unchanged.
0.434.0: The contractor job list's unused Kuala Lumpur slot text is removed; the remaining Kuala Lumpur times are their screens' rules (IR564). Phase 1A unchanged.
0.433.0: The documentation index lists the contracts and catalogs that bind the documents: service, deterministic, strict review and review resolution contracts, the write version and query catalogs, the API description and the demo fixture (IR563). Phase 1A unchanged.
0.432.0: Network architecture §6 notes that voice input, the QR scan and location automations need their feature allowed in the app's Permissions-Policy once they are built (IR562). Phase 1A unchanged.
0.431.0: The container design's IoT Core and SES stand-ins say what the bridge and Mailpit do as built, and the Dockerfile paragraph names the tools target (IR561). Phase 1A unchanged.
0.430.0: Backend Go design §8 and §9 say what is built: the per-domain database roles, the scans by hand, the planned acceptance, load and staging tests, and the Playwright suite (IR560). Phase 1A unchanged.
0.429.0: Backend Go design §4 matches the request pipeline as built (no tracing middleware yet, current Register and MountREST) and records read-your-writes on a reader replica as OPEN-GO-03 (IR559). Phase 1A unchanged.
0.428.0: Database design §2 lists identity.partner_users and no longer counts notify.deliveries, used since IR418, among the unused tables (IR558). Phase 1A unchanged.
0.427.0: Backend architecture §11 lists the identity provider's admin API and the SMTP mail as built; OPEN-BE-02 notes the development nodes (IR557). Phase 1A unchanged.
0.426.0: Backend architecture §14 says what CI runs and that the security scans run by hand; §15 gives the status of each migration step from the Phase 1A mock (IR556). Phase 1A unchanged.
0.425.0: Backend architecture §5 says how the caller context, change notification and files work as built: the user's own token, no SSE yet, uploads through Server Actions into the local blob store (IR555). Phase 1A unchanged.
0.424.0: Backend architecture §3 says which runtime components run today: the Core API's services, the workers that run or idle, the IoT bridge and what the cache holds (IR554). Phase 1A unchanged.
0.423.0: Backend architecture §4 says that its event column holds the design's names and points to §8 for the events as built (IR553). Phase 1A unchanged.
0.422.0: Backend architecture §8 lists the events the domain services exchange as built (the 19 contracts and the identity-accounts events) under the design's table of main flows (IR552). Phase 1A unchanged.
0.421.0: Network flow F20 and the container design's IDP row allow the identity provider's user administration calls (accounts, set-up e-mail, enable/disable, sign-out), not the sign-out alone (IR551). Phase 1A unchanged.
0.420.0: Container design §6 describes the GitHub Actions CI that runs (ci.yml's seven jobs, e2e.yml) above the planned production pipeline (IR550). Phase 1A unchanged.
0.419.0: `.env.local.example` drops two keys nothing reads and lists the optional overrides compose takes from `.env.local` (IR549). Phase 1A unchanged.
0.418.0: The README says how to connect the real sensor nodes and sign in as their client (IR548). Phase 1A unchanged.
0.417.0: The Go design's module tree lists the IoT tools, bootstrap and what the identity provider package does (IR547). Phase 1A unchanged.
0.416.0: Backend Go design §10 names the make targets that exist (no make up / down) and the images per Go service; container design §2 says the same of the images (IR546). Phase 1A unchanged.
0.415.0: The container design's variable table lists the IoT tools' broker logins, the live site's broker and commands, the mock history length and the seed switches (IR545). Phase 1A unchanged.
0.414.0: Air quality points the temperature to Unit Control only when the unit page sets the model's temperature; the live Daikin (power only) says it is set on the AC itself (IR544). Phase 1A unchanged.
0.413.0: Backend architecture §6 describes the account set-up: the set-up link, its 3-day validity, the resend and the daily cap (IR543). Phase 1A unchanged.
0.412.0: Backend architecture §10 lists the scheduled release of an assignment whose technician's access ends (IR439) and says that HQ re-offers an expired offer by hand (IR542). Phase 1A unchanged.
0.411.0: DEC-86 says that the IoT bridge publishes nothing to the nodes' broker but the live unit's power commands, since DEC-89 (IR541). Phase 1A unchanged.
0.410.0: The backend architecture describes the real sensor nodes' readings, commands and timing for the device team, with the firmware changes full control would need as OPEN (IR540). Phase 1A unchanged.
0.409.0: The IoT tools run from a distroless, non-root, read-only image without capabilities, like the services (IR539). Phase 1A unchanged.
0.408.0: The Core API services no longer read .env.local; each key reaches only the containers that use it (IR538). Phase 1A unchanged.
0.407.0: The voice assistant's change card offers no temperature change on a model without remote control (IR537). Phase 1A unchanged.
0.406.0: The client design's DD-C03 states the unit page's capability rules, changes and room readings (IR536). Phase 1A unchanged.
0.405.0: Tidy-ups: the bridge's one check for a recorded event, and DEC-86 points to DEC-89 for the live unit's remote power (IR535). Phase 1A unchanged.
0.404.0: The backend architecture's IoT design says how a module's connection state is reported (device events, never derived from lastSeenAt; IR534). Phase 1A unchanged.
0.403.0: The unit page's choice of remote controls per model is the tested helper controlLayout (IR533). Phase 1A unchanged.
0.402.0: The live module goes offline after 3 minutes without its nodes and back online when they report, through device events whose open fault the bridge reads from the new internal query devices.connectionFault (IR532). Phase 1A unchanged.
0.401.0: The live heartbeat gap is measured on the wall clock, so the time a sleeping machine missed the nodes counts toward AC offline (IR531). Phase 1A unchanged.
0.400.0: Figma's Unit Control row shows a power-only model (02o) and a model without remote control (02p); the web drops the review bar where the power is the only setting (IR530). Phase 1A unchanged.
0.399.0: The live readings feed the customer's alert policies through demo.trigger automation (alert policies only, no automation), the contract's heartbeat_gap, compressor_cycles and airflow_drop facts are taken, and a silent live module raises AC offline (IR529). Phase 1A unchanged.
0.398.0: The live Daikin air conditioner is one unit again and is switched on and off from the app through node_1's infrared LED (COOL24 / OFF); a state report with no settings keeps the observed state (DEC-89, IR528). Phase 1A unchanged.
0.397.0: The client's unit page lists each room unit's readings instead of its setting, and a monitoring-only model's temperature advice says it is set on the AC itself (IR527). Phase 1A unchanged.
0.396.0: Each real sensor node is a device of its own: node_1 (room sensor) and node_2 (power meter) are two monitoring-only units of Live Sensor Customer (DEC-88, IR526). Phase 1A unchanged.
0.395.0: The real sensor nodes report to a client of their own, "Live Sensor Customer", whose owner sets the password from the set-up e-mail (DEC-87, IR525). Phase 1A unchanged.
0.394.0: Real sensor nodes feed the development stack: the IoT bridge subscribes to their broker and records room temperature, humidity and power on a monitoring-only unit of customer-b, and the client's unit screen follows the model's capabilities (DEC-86, IR524). Phase 1A unchanged.
0.393.0: Figma's Use Cases note that HQ's revoke turns the sign-in off (UC-A11, IR523). Phase 1A unchanged.
0.392.0: Figma's frontend board names today's Vitest counts (IR522). Phase 1A unchanged.
0.391.0: Figma's System Architecture follows IR510 and IR511: HQ two-step required, two Cognito pools, the identity-accounts consumer, Keycloak 26.8 (IR521). Phase 1A unchanged.
0.390.0: AT-X08-B follows IR510: HQ's two-step verification is Required, with no dialog (IR520). Phase 1A unchanged.
0.389.0: The client's and the contractor's acceptance cases follow IR504 and IR506 (IR519). Phase 1A unchanged.
0.388.0: HQ's acceptance cases follow IR508 and IR509: AT-A03-R01 expects the revoke to turn the sign-in off, and AT-A03-R02 starts from one administrator (IR518). Phase 1A unchanged.
0.387.0: Figma gains the contractor's Users state 08-6, a user whose access HQ revoked (IR517). Phase 1A unchanged.
0.386.0: Figma's Access & roles revoke texts follow IR508: the sign-in goes off and only HQ gives the access back (IR516). Phase 1A unchanged.
0.385.0: The screen catalog notes that the HQ app shows two-step verification as Required (IR515). Phase 1A unchanged.
0.384.0: HQ's save of an access that ended by itself is no revoke: the end is compared at whole minutes, as the form sends it (IR514). Phase 1A unchanged.
0.383.0: The preferences end-to-end test and the common design follow IR510 (IR513). Phase 1A unchanged.
0.382.0: Figma and the use-case catalogue follow IR510: HQ's Preferences show two-step verification as Required, and the HQ-only setup screen 15-7 is removed (IR512). Phase 1A unchanged.
0.381.0: The sign-in accounts and sessions have their own event consumer, so a failing notification or audit event no longer delays turning an account off (IR511). Phase 1A unchanged.
0.380.0: HQ two-step verification is enforced: the local realm's HQ app flow asks for the password and the code every time, the Core API takes HQ memberships only from the HQ app's tokens, and production uses an HQ Cognito pool with MFA required (DEC-85, IR510). Phase 1A unchanged.
0.379.0: An empty environment starts from one HQ administrator: `migrate bootstrap-admin` creates a tenant's first administrator with the sign-in account and is refused once it has one (DEC-84, IR509). Phase 1A unchanged.
0.378.0: HQ's revoke turns the sign-in off, and only HQ gives the access back; contractors and customer owners cannot extend, enable or re-add a revoked person (DEC-83, IR508). Phase 1A unchanged.
0.377.0: The user's decisions of 2026-10-30 are recorded: back-to-back schedules (DEC-80), the account cap kept at 30 a day (DEC-81), ending a contract early outside Phase 1A (DEC-82, OPEN-07) (IR507). Phase 1A unchanged.
0.376.0: The contractor's job history reads its event counts with `jobs.list` (`JobSummary.eventSummary`), not one `jobs.events` call per job; hidden actions are not customer-visible; decision events are the organization's (IR506). Phase 1A unchanged.
0.375.0: `scripts/check.sh` stops on a failing document validator and runs its TypeScript check; the baseline is rewritten (IR505). Phase 1A unchanged.
0.374.0: Back-to-back schedules: a start of the same minute that would run wins over the end, recorded as superseded (IR504, IR326 item 4 decided). Phase 1A unchanged.
0.373.0: Go 1.26.10 and golang.org/x/net 0.62.0; `make vuln` checks the standard library of the module's own Go (IR503). Phase 1A unchanged.
0.372.0: Nine unused web exports are removed (IR502). Phase 1A unchanged.
0.371.0: Three pages that scrolled sideways at 360 px on the Linux runners fit; the phone crawl measures with a wide font (IR501). Phase 1A unchanged.
0.370.0: The phone crawl runs at 360 px, where HQ's overview no longer scrolls sideways (IR500). Phase 1A unchanged.
0.369.0: Sensor metrics, reminder recipients' roles and the default policy's name are in Malay too (IR499). Phase 1A unchanged.
0.368.0: Every page that reads the Core API has its loading and error state (IR498). Phase 1A unchanged.
0.367.0: Device, device-event and plan lists read their sensors, notes and generated jobs once per page (IR497). Phase 1A unchanged.
0.366.0: A page of members reads its permissions, scopes and grants with one query per table, not three per member (IR496). Phase 1A unchanged.
0.365.0: HQ's pages read every customer, property and unit they name or offer, not the first 100 (IR495). Phase 1A unchanged.
0.364.0: The partner's job history shows its latest 100 jobs and the schedule reads every open job (IR494). Phase 1A unchanged.
0.363.0: HQ's job refusals are in Malay too and name fields in words (IR493). Phase 1A unchanged.
0.362.0: A revoked access ends when HQ confirms, not when the page loaded (IR492). Phase 1A unchanged.
0.361.0: HQ's automation simulation and Fire take the Core API's current tick, not the page's load time (IR491). Phase 1A unchanged.
0.360.0: Every page is titled with its name in the display language (IR490). Phase 1A unchanged.
0.359.0: Every page declares the display language as its language (IR489). Phase 1A unchanged.
0.358.0: A payment reminder in the inbox names its amount (IR488). Phase 1A unchanged.
0.357.0: Every page at phone width in Malay is accessible and fits the screen, with a smoke crawl that checks it (IR487). Phase 1A unchanged.
0.356.0: A QR check-in with another unit's label says what to do (IR486). Phase 1A unchanged.
0.355.0: Single-row reads that dropped their errors return them (IR485). Phase 1A unchanged.
0.354.0: Every row loop of the service code checks the read's error, with a guard test (IR484). Phase 1A unchanged.
0.353.0: The design docs follow DEC-75 and the API-mode data path instead of DEC-03 and the Phase 1A Repository steps (IR483). Phase 1A unchanged.
0.352.0: A restriction unit waiting for a device operation is worded for HQ and the customer (IR482). Phase 1A unchanged.
0.351.0: The inbox details (status, release reason, firmware notice), the system's resolution reasons and the energy boundary are shown in the display language (IR481). Phase 1A unchanged.
0.350.0: Alert and observation evidence and the not-found screens are shown in the display language (IR480). Phase 1A unchanged.
0.349.0: The invitation e-mail queue from before the accounts were created with the users is removed; every user-list row has its account (IR479). Phase 1A unchanged.
0.348.0: Archiving a property or space and adding a unit or space to it at the same moment cannot both pass (IR478). Phase 1A unchanged.
0.347.0: A restriction becomes applied when two of its units answer at the same moment (IR477). Phase 1A unchanged.
0.346.0: A restriction ends when its cause invoices are paid at the same moment (IR476). Phase 1A unchanged.
0.345.0: Each design document's closing line says every IR applies, the newest included, instead of a range that fell behind (IR475). Phase 1A unchanged.
0.344.0: An invoice takes one payment at a time, so a second card attempt cannot be started while one is in flight (IR474). Phase 1A unchanged.
0.343.0: A demo clock jump or a paused scheduler leaves each unit in the state of its last schedule minute instead of the start of a window it passed (IR473). Phase 1A unchanged.
0.342.0: Someone always keeps the right to manage access, also when the last two administrators change each other at once or one moves its own start later (IR472). Phase 1A unchanged.
0.341.0: A customer's last owner and a contractor company's last contractor stay when two changes come at once (IR471). Phase 1A unchanged.
0.340.0: Restrictions scheduled at once on one unit from two contracts end as one (IR470). Phase 1A unchanged.
0.339.0: No two invoices of a contract cover the same moment (IR469). Phase 1A unchanged.
0.338.0: A new HQ account sets up its one-time-code device before its first sign-in, so HQ signs in with two steps (IR468). Phase 1A unchanged.
0.337.0: A registered device reads “Not bound yet — planned for <unit>” and the detail offers Bind for its first binding (IR467). Phase 1A unchanged.
0.336.0: The client's job history says “Report reviewed” without naming HQ (IR466). Phase 1A unchanged.
0.335.0: HQ's Add contractor creates a new contractor company with its profile, so the first contractor is added from the register (IR465). Phase 1A unchanged.
0.334.0: A contractor or customer owner adds at most 30 accounts to the organization in 24 hours (IR464). Phase 1A unchanged.
0.333.0: The common permission table and the backend actors describe adding users with an account (IR461). Phase 1A unchanged.
0.332.0: A person removed earlier can be added again with the same e-mail, and HQ's new membership for an existing person reaches their sign-in (IR462); the accounts people are added with carry their sign-in claims, checked end to end by signing in as a new technician (IR463). Phase 1A unchanged.
0.331.0: Accounts are created where people are added, with a set-up e-mail; contractors add technicians with an access end of at most a year; no set-up e-mail after the access end (DEC-79, IR461). FR/DD-P11, C19, A17 and A03 describe adding instead of inviting. Phase 1A unchanged.
0.330.0: Alerts opened at the same time end as one (IR460). Phase 1A unchanged.
0.329.0: A technician's new window refreezes their job history (IR459). Phase 1A unchanged.
0.328.0: Extending ended access refreezes the contractor's history (IR458). Phase 1A unchanged.
0.327.0: An offer is answered by the visit's start (IR457). Phase 1A unchanged.
0.326.0: A time that is over can neither be assigned nor offered (IR456). Phase 1A unchanged.
0.325.0: A proposal's reply-by ends by the time it proposes (IR455). Phase 1A unchanged.
0.324.0: The access window follows a moved visit (IR454). Phase 1A unchanged.
0.323.0: A command expires a few seconds after its window (IR453). Phase 1A unchanged.
0.322.0: An offer closed by a cancellation is no decline of the contractor (IR452). Phase 1A unchanged.
0.321.0: A plan's visits have no response target (IR451). Phase 1A unchanged.
0.320.0: A plan's next visit is not a follow-up for first-time fix (IR450). Phase 1A unchanged.
0.319.0: Time on site leaves out the periods the job was held or back with HQ (IR449). Phase 1A unchanged.
0.318.0: A pause still open ends with the report's submission (IR448). Phase 1A unchanged.
0.317.0: HQ records a manual payment after a failed attempt (IR447). Phase 1A unchanged.
0.316.0: The client's acceptance checks the proposal's hold again (IR446). Phase 1A unchanged.
0.315.0: A contractor's decline ends the client's proposals that hold it (IR445). Phase 1A unchanged.
0.314.0: A contractor's time change waiting for HQ ends with its offer (IR444). Phase 1A unchanged.
0.313.0: A contractor's time change ends with the client's proposal made from it (IR443). Phase 1A unchanged.
0.312.0: The client's job history leaves out HQ's and the contractor's own handling (IR442). Phase 1A unchanged.
0.311.0: Dependency upgrades: Node 26, PostgreSQL 18, TypeScript 6, Next.js 16.4.2 (IR441). Phase 1A unchanged.
0.310.0: Assignments released when a newly approved qualification no longer covers the visit (IR440). Phase 1A unchanged.
0.309.0: Assignments released when the technician's access ends (IR439, OPEN-BE-09). Phase 1A unchanged.
0.308.0: Problem-report notice targets the follow-up (IR438). Phase 1A unchanged.
0.307.0: Problem photos readable, jobs.getAttachment (IR437). Phase 1A unchanged.
0.306.0: certificates.getContent for HQ verification (IR436). Phase 1A unchanged.
0.305.0: Gateway-made request IDs only (IR435). Phase 1A unchanged.
0.304.0: Photos and signatures not stored by the browser (IR434). Phase 1A unchanged.
0.303.0: Assignments within the technician's access period (IR433). Phase 1A unchanged.
0.302.0: Booked technicians marked in the Users list (IR432). Phase 1A unchanged.
0.301.0: Internal query inputs validated (IR431). Phase 1A unchanged.
0.300.0: Local secrets refused outside local runs (IR430). Phase 1A unchanged.
0.299.0: Polling loops log a repeating failure once a minute (IR429). Phase 1A unchanged.
0.298.0: Platform table retention (IR428). Phase 1A unchanged.
0.297.0: Event consumers dead-letter a failing event (IR427). Phase 1A unchanged.
0.296.0: Invitation e-mails follow the invite (IR426). Phase 1A unchanged.
0.295.0: E-mail retries back off (IR425). Phase 1A unchanged.
0.294.0: E-mail headers: Date and encoded subject (IR424). Phase 1A unchanged.
0.293.0: Invitation resends limited to one per 5 minutes (IR423). Phase 1A unchanged.
0.292.0: Review fixes: booked technicians stay (IR421), accepted client e-mails stay (IR422). Phase 1A unchanged.
0.291.0: Review fixes: service start order and token keys (IR419), disabled accounts stay closed on HQ changes (IR420). Phase 1A unchanged.
0.290.0: E-mails are sent, through Mailpit in development (DEC-78, IR418). Phase 1A unchanged.
0.289.0: Customer owners manage their users, FR-C19 (DEC-77, IR417); automation cards and editor show a schedule's start and end apart. Phase 1A unchanged.
0.288.0: Contractors manage their own company's users, FR-P11 (DEC-76, IR416). Phase 1A unchanged.
0.287.0: The use case catalogue is in the repository (IR415). Phase 1A unchanged.
0.286.0: shadcn/ui phase 4, toasts, tables and charts on Sonner, TanStack Table and Recharts (IR403). Phase 1A unchanged.
0.285.0: shadcn/ui phase 3b, the search select, the last hand-made popups and the side panels (IR402). Phase 1A unchanged.
0.284.0: shadcn/ui phase 3a, dialogs, menus, tabs and the choice controls (IR401). Phase 1A unchanged.
0.283.0: shadcn/ui adopted (DEC-75, supersedes DEC-03), phase 1: foundation and the product components on it (IR400). Phase 1A unchanged.
0.282.0: Icons part 7, the last character icons, a guard test and the icon rules (IR399). Phase 1A unchanged.
0.281.0: Icons part 6, Figma uses the same Lucide icons (IR398). Phase 1A unchanged.
0.280.0: Icons part 5, the arrows in link and button texts (IR397). Phase 1A unchanged.
0.279.0: Icons part 4, partner, technician and HQ (IR396). Phase 1A unchanged.
0.278.0: Icons part 3, the customer app (IR395). Phase 1A unchanged.
0.277.0: Icons part 2 (IR394). Phase 1A unchanged.
0.276.0: One icon library, Lucide (DEC-74, IR393). Phase 1A unchanged.
0.275.0: Result types match the contract results (IR392). Phase 1A unchanged.
0.274.0: Input types match the contract inputs (IR391). Phase 1A unchanged.
0.273.0: Payment inputs split; XSS second pass (IR390). Phase 1A unchanged.
0.272.0: Upload pixel limits (IR389). Phase 1A unchanged.
0.271.0: Search terms matched as written (IR388). Phase 1A unchanged.
0.270.0: Checks round: open redirects, coverage, board counts (IR387). Phase 1A unchanged.
0.269.0: Sign out everywhere ends the identity provider's sessions too (IR386). Phase 1A unchanged.
0.268.0: Sign out everywhere in the Admin screens; refused sessions to sign-in (IR385). Phase 1A unchanged.
0.267.0: Sign out everywhere from Admin (IR384). Phase 1A unchanged.
0.266.0: Web containers on the session store; Permissions-Policy (IR383). Phase 1A unchanged.
0.265.0: Server-side session store with revocation at sign-out (IR382). Phase 1A unchanged.
0.264.0: Checks round: advisories, local stand-ins, technician scope kinds (IR381). Phase 1A unchanged.
0.263.0: Refusals over the write limit worded everywhere; Group control waits and resends (IR380). Phase 1A unchanged.
0.262.0: "Mark all as read" in one write (IR379). Phase 1A unchanged.
0.261.0: Coverage of owner-only refusals and the service configuration (IR378). Phase 1A unchanged.
0.260.0: Row-level security on the stored write responses (IR377). Phase 1A unchanged.
0.259.0: Row-level security on partitions (IR376). Phase 1A unchanged.
0.258.0: Write limit per membership (IR375). Phase 1A unchanged.
0.257.0: Cookie prefixes on https (IR374). Phase 1A unchanged.
0.256.0: Container hardening reviewed; local ports bound to this machine, capabilities dropped, images pinned (IR373). Phase 1A unchanged.
0.255.0: Server-side request forgery reviewed; dot-segment path parameters refused (IR372). Phase 1A unchanged.
0.254.0: Cross-site scripting reviewed; the gateway's API page pinned with integrity hashes and a policy (IR371). Phase 1A unchanged.
0.253.0: Rate limits reviewed; BFF input capped, sign-in lockout in the local realm (IR370). Phase 1A unchanged.
0.252.0: Secrets reviewed; env files out of images and git, masked logs (IR369). Phase 1A unchanged.
0.251.0: Uploads reviewed; file names, the photo header and CSV exports fixed (IR368). Phase 1A unchanged.
0.250.0: Authentication reviewed; access tokens only, no default cookie key on https (IR367). Phase 1A unchanged.
0.249.0: Authorization by target checked; three authorization gaps fixed (IR366). Phase 1A unchanged.
0.248.0: SQL built from caller input reviewed; the sort direction is checked where it is used (IR365). Phase 1A unchanged.
0.247.0: Dependency advisories scanned and fixed (IR364). Phase 1A unchanged.
0.246.0: The backend Go design matches the code (IR363). Phase 1A unchanged.
0.245.0: Errors are announced at once (IR362). Phase 1A unchanged.
0.244.0: Dialogs keep keyboard focus (IR361). Phase 1A unchanged.
0.243.0: The accessibility check covers dialogs (IR360). Phase 1A unchanged.
0.242.0: WCAG AA contrast and the axe-core check in the smoke crawl (IR359). Phase 1A unchanged.
0.241.0: The BFF refuses other sites' requests (IR358). Phase 1A unchanged.
0.240.0: Sign-out ends the identity provider's session (IR357). Phase 1A unchanged.
0.239.0: Several scheduler instances per domain (IR356). Phase 1A unchanged.
0.238.0: Security headers in the web apps (IR354) and stricter integration checks (IR355). Phase 1A unchanged.
0.237.0: Backend coverage round with two fixes (IR353). Phase 1A unchanged.
0.236.0: Photos link to inspection findings and the reviews name them (IR352). Phase 1A unchanged.
0.235.0: The inbox marks all as read and a failed test-run end can be retried (IR351). Phase 1A unchanged.
0.234.0: The model and factor editors mark and discard drafts (IR350). Phase 1A unchanged.
0.233.0: MRV reports attach the photos of accepted work reports as evidence (IR349). Phase 1A unchanged.
0.232.0: The HQ policy editors preview the notification and test a policy with demo data (IR348). Phase 1A unchanged.
0.231.0: HQ edits the default policy's rules, switches them per customer and copies them as customer policies (IR347). Phase 1A unchanged.
0.230.0: The HQ Policies tab has the scope, Show disabled and active hours (IR346). Phase 1A unchanged.
0.229.0: HQ creates and edits alert policies for a customer (IR345). Phase 1A unchanged.
0.228.0: Firmware campaigns record and show who scheduled them (IR344). Phase 1A unchanged.
0.227.0: The firmware campaign detail follows Figma 04-9 (IR343). Phase 1A unchanged.
0.226.0: The Use Cases page shows the clock-driven reminders and payout drafts (IR342). Phase 1A unchanged.
0.225.0: A billing scheduler generates the monthly payout drafts on the 1st (IR341). Phase 1A unchanged.
0.224.0: Warranty end reminders to HQ from the equipment scheduler (IR340). Phase 1A unchanged.
0.223.0: The firmware campaign runner on the equipment scheduler (IR339). Phase 1A unchanged.
0.222.0: The HQ Jobs follow-up row and its classify-by time; the monthly report e-mail recorded as a user decision (IR338). Phase 1A unchanged.
0.221.0: The technician's work window warning follows Figma; §10 lists it as shown by the app (IR337). Phase 1A unchanged.
0.220.0: Certificate expiry reminders built in the maintenance scheduler; the certificate notification target and type added (IR336). Phase 1A unchanged.
0.219.0: Screens, flows and use cases checked complete; the scheduler and the unused tables described as built (IR335). Phase 1A unchanged.
0.218.0: docker/ checked against the container design; the variables and stand-ins that wait for integrations are named (IR334). Phase 1A unchanged.
0.217.0: The BFF session cookie is sealed (AES-256-GCM); the container design follows compose.yaml; every design document reads IR01–IR333 (IR333). Phase 1A unchanged.
0.216.0: Every VALIDATION check of the integration suite names its field; two more misses fixed (IR332). Phase 1A unchanged.
0.215.0: VALIDATION checks name their field; audit.list names the missing end of its period (IR331). Phase 1A unchanged.
0.214.0: Coverage round 5 — import revalidation, diagnostics and the inbox; two checks that passed for another reason (IR330). Phase 1A unchanged.
0.213.0: The Figma architecture and use cases follow IR327; coverage round 4 (IR329). Phase 1A unchanged.
0.212.0: Coverage round 3 — tests for paths no test reached (IR328). Phase 1A unchanged.
0.211.0: An alert's resolution cites its evidence: the new read alerts.evidence lists the candidates, and the HQ and technician screens pick from them (IR327). Phase 1A unchanged.
0.210.0: Coverage round 2; open questions on back-to-back schedules and UTC plan recurrence (IR326). Phase 1A unchanged.
0.209.0: Coverage round — untested business rules of the Core API now tested (IR325). Phase 1A unchanged.
0.208.0: HQ offsets gain their filters; the HQ frames 10–25 are aligned (IR324). Phase 1A unchanged.
0.207.0: HQ baselines and automation policies follow Figma (counts, quality, footer, + Add fact) (IR323). Phase 1A unchanged.
0.206.0: HQ payouts gain filters, a statements table and a Generate drafts dialog; contracts show counts and eligibility (IR322). Phase 1A unchanged.
0.205.0: HQ access gains the organization filter and search-selects; billing filters by billing months; restrictions filter by cause invoice (IR321). Phase 1A unchanged.
0.204.0: The HQ audit explorer follows SCR-A16 and Figma (actor and target filters, the open entry in the URL, correlation trace, related records, device events only for a picked device) (IR320). Phase 1A unchanged.
0.203.0: The HQ Devices tab follows Figma 246:2 (state chips, serial order, tiles, bound unit, sensors with Calibrate →, histories); an active restriction shows without restriction.read (IR319). Phase 1A unchanged.
0.202.0: The HQ Alerts tab follows DD-A05 (URL filters and selection, Why we think this, Activity, Request maintenance, Open unit); the seed gives rows without createdAt the fixture's seedCreatedAt (IR91, IR318). Phase 1A unchanged.
0.201.0: The contractor and technician screens checked against Figma: the technician's job workspace shows the site's access and the unit links for an assigned job (IR317). Phase 1A unchanged.
0.200.0: The E2E suite passes on a fresh seed; the seed gains an overdue filter (demoSeed.filterCleanings) and four specs no longer depend on drifted data (IR316). Phase 1A unchanged.
0.199.0: The customer screens checked against Figma: the AC renamed on its screen, load causes not faults, repair requests for faults and offline ACs, the users page names its customer (IR315). Phase 1A unchanged.
0.198.0: The web's result types are checked against the contract; restriction commands named, ventilating rules kept, ended technician jobs handled (IR314). Phase 1A unchanged.
0.197.0: Every Core API call of the web is typed by the contract (IR313). Phase 1A unchanged.
0.196.0: The customer overview shows the next run again; the web's calls are checked against the contract (IR312). Phase 1A unchanged.
0.195.0: Every SQL statement of the Core API runs in a test; no other untyped parameter found (IR311). Phase 1A unchanged.
0.194.0: HQ can change a signed-in client user again (a membership update failed on a parameter type); release, forecast, campaign and internal query tests (IR310). Phase 1A unchanged.
0.193.0: Contractor quality, import, payout and alert evaluation tests; the demo job actions stay local (IR309). Phase 1A: a refused demo note is shown again.
0.192.0: Uploads are checked by their bytes; more input-rule, reschedule, partner proposal, report and reconcile tests (IR308). Phase 1A unchanged.
0.191.0: Tests for the BFF session, the DAL and every operation's input rules; two web fixes (inherited role names, coreAll past max) (IR307). Phase 1A unchanged.
0.190.0: The customer assistant reads voice.resolveIntent in its chosen language and labels its commands voice (IR306). Phase 1A unchanged.
0.189.0: Audit entries carry the actor's name; customers never see it (IR305). Phase 1A unchanged.
0.188.0: The HQ audit log in Malay; every business screen now follows the display language (IR304). Phase 1A unchanged.
0.187.0: HQ automation policies in Malay (IR303). Phase 1A unchanged.
0.186.0: HQ access & roles in Malay with the valid period in the display time zone; the override-only demo actor gets its own name (IR302). Phase 1A unchanged.
0.185.0: HQ restrictions and the exception screen in Malay; HQ types their deadlines in the display time zone (IR301). Phase 1A unchanged.
0.184.0: HQ contracts in Malay; contract periods stay Kuala Lumpur days (IR300). Phase 1A unchanged.
0.183.0: HQ billing in Malay; billing dates stay Kuala Lumpur days, inquiry times in the display time zone (IR299). Phase 1A unchanged.
0.182.0: The HQ offset demo in Malay; attempt and event times in the display time zone (IR298). Phase 1A unchanged.
0.181.0: The HQ MRV workspace in Malay; report periods stay Kuala Lumpur time, review times in the display time zone (IR297). Phase 1A unchanged.
0.180.0: The HQ energy analysis in Malay; its periods stay Kuala Lumpur time (IR296). Phase 1A unchanged.
0.179.0: Clickable table rows work by keyboard (IR294); the HQ device registry in Malay, a campaign start typed in the display time zone (IR295). Phase 1A unchanged.
0.178.0: HQ customers & units in Malay (IR293). Phase 1A unchanged.
0.177.0: The HQ alerts and alert policies in Malay; the HQ editor offers the seven metrics of DD-A05 (IR292). Phase 1A unchanged.
0.176.0: The HQ Plans, Contractors and SLA tabs in Malay; SLA breaches carry their minutes and early-cancelled jobs are no response miss (IR291). Phase 1A unchanged.
0.175.0: The HQ Jobs tab in Malay; jobs.list filters by type (IR290). Phase 1A unchanged.
0.174.0: The HQ overview in Malay (IR289). Phase 1A unchanged.
0.173.0: The technician's QR scan in Malay — every technician screen now follows the display language (IR288). Phase 1A unchanged.
0.172.0: The technician's devices and device events in Malay (IR287). Phase 1A unchanged.
0.171.0: The technician's diagnostic control in Malay (IR286). Phase 1A unchanged.
0.170.0: Sustained recovery resolves policy alerts in the demo evaluation, and a recurrence links to the alert before it (IR285). Phase 1A unchanged.
0.169.0: The technician's alert evidence as in Figma, in Malay; every alert carries the condition that raised it, and technicians acknowledge with alert.read (IR284). Phase 1A unchanged.
0.168.0: The technician's unit screen as in Figma, in Malay (IR283). Phase 1A unchanged.
0.167.0: The technician job workspace in Malay (IR282). Phase 1A unchanged.
0.166.0: The technician overview in Malay (IR281). Phase 1A unchanged.
0.165.0: The contractor's unit view as in Figma, in Malay (IR280). Phase 1A unchanged.
0.164.0: The contractor's payouts in Malay (IR279). Phase 1A unchanged.
0.163.0: The contractor's job history in Malay (IR278). Phase 1A unchanged.
0.162.0: The contractor's certifications as in Figma, in Malay; certificate files up to 10 MB reach the Core API (IR277). Phase 1A unchanged.
0.161.0: The contractor's team & capacity as in Figma, in Malay; the session names its organization (IR276). Phase 1A unchanged.
0.160.0: The contractor's schedule in Malay (IR275). Phase 1A unchanged.
0.159.0: The contractor's quality review in Malay (IR274). Phase 1A unchanged.
0.158.0: DEC-03 decided — the build's components, forms and dictionaries stand (IR273). Phase 1A unchanged.
0.157.0: The contractor's offer and job page in Malay (IR272). Phase 1A unchanged.
0.156.0: The contractor job list in Malay (IR271). Phase 1A unchanged.
0.155.0: The contractor overview in Malay (IR270). Phase 1A unchanged.
0.154.0: The failed-action toast and the route states in Malay (IR269). Phase 1A unchanged.
0.153.0: The last sign-in is recorded from the token's auth_time (IR268). Phase 1A unchanged.
0.152.0: The customer's contracts & payments and users in Malay; due dates stay Kuala Lumpur days; last sign-in not yet recorded (IR267). Phase 1A unchanged.
0.151.0: The customer's air quality in Malay and the display time zone; the hint for a unit outside a room points to HQ (IR266). Phase 1A unchanged.
0.150.0: The customer's energy & cost and carbon offsets in Malay; one default unit; rounding as written (IR265). Phase 1A unchanged.
0.149.0: The customer's automations in Malay; the schedule preview's end time fixed (IR264). Phase 1A unchanged.
0.148.0: Units & locations in Malay and the display time zone; language E2E specs restore in afterEach (IR263). Phase 1A unchanged.
0.147.0: The customer's maintenance in Malay and the display time zone; preferred times typed in that zone (IR262). Phase 1A unchanged.
0.146.0: The customer's alerts and alert policies in Malay and the display time zone (IR261). Phase 1A unchanged.
0.145.0: The customer overview in Malay and the display time zone (IR260). Phase 1A unchanged.
0.144.0: The customer's unit screen in Malay and the display time zone (AT-X01-N); translation library recorded with DEC-03 (IR259). Phase 1A unchanged.
0.143.0: Bahasa Melayu on the shell, Preferences, Demo controls and the inbox; other screens still English (IR258). Phase 1A demo language kept in the browser.
0.142.0: Frontend design follows the build; demo data per tab; DEC-03 and Malay left open (IR257). Phase 1A demo stores per tab.
0.141.0: End-to-end scenario across the four apps (IR256). Phase 1A unchanged.
0.140.0: Technician QR one-tap labels from the open jobs (IR254); more end-to-end specs (IR255). Phase 1A unchanged.
0.139.0: End-to-end tests of the four web apps (IR252); technician job notifications follow the assignment (IR253). Phase 1A unchanged.
0.138.0: Sign-in page on the server with the reason a sign-in came back (IR251). Phase 1A unchanged.
0.137.0: Demo controls on the Core API (IR249); Sign out ends the session in API mode (IR250). Phase 1A unchanged.
0.136.0: Two-step verification dialog with the setup QR and code boxes (IR247); the shared screens' frame renders per request (IR248). Phase 1A unchanged.
0.135.0: HQ Jobs tab requested-time period from the overview links (IR245); Preferences on the Core API with the client monthly report e-mail (IR246). Phase 1A unchanged.
0.134.0: HQ overview sections from admin.summary; SCR-A01 / DD-A01 filter lists and period keys (IR244). Phase 1A unchanged.
0.133.0: Customer alert policies on the Core API; Session.customerId (IR243). Phase 1A unchanged.
0.132.0: Shell user, organization and badges from the Core API; Session names (IR241); customer alert inbox (IR242). Phase 1A unchanged.
0.131.0: Customer overview on the Core API; DD-C01 / SCR-C01 reads (IR240). Phase 1A unchanged.
0.130.0: Filter cleaning reminders from the maintenance scheduler (IR239). Phase 1A unchanged.
0.129.0: Customer Filter care on the Core API; filterCare.getSettings (200 operations); run time restarts at 0 h after a cleaning (IR238). Phase 1A unchanged.
0.128.0: Customer maintenance (My requests) on the Core API; Assignment.technicianName for clients (IR237). Phase 1A unchanged.
0.127.0: HQ SLA tab on the Core API; SlaScorecard carries targets and plan types (IR236). Phase 1A unchanged.
0.126.0: HQ contractor register (Contractors tab) on the Core API (IR235). Phase 1A unchanged.
0.125.0: Completion releases the assignment (DEC-73, IR234). Phase 1A unchanged.
0.124.0: HQ maintenance plans (Plans tab) on the Core API, plan occurrences bookable, base-layer styles (IR233). Phase 1A unchanged.
0.123.0: HQ job report review, costs, New job and access extension on the Core API (IR232). Phase 1A unchanged.
0.122.0: HQ maintenance jobs (Jobs tab) on the server pattern (IR231). Phase 1A unchanged.
0.121.0: Technician unit QR scan from the Core API (IR230). Phase 1A unchanged.
0.120.0: Technician overview on the server pattern; uploaded files kept across container restarts (IR229). Phase 1A unchanged.
0.119.0: Contractor job history on the server pattern; notification previews take a 2000-character message (IR228). Phase 1A unchanged.
0.118.0: Contractor schedule & assignments on the server pattern; JobSummary access window (IR227). Phase 1A unchanged.
0.117.0: Contractor offer and job detail on the server pattern; offer projection fields; accept and decline take only their own inputs (IR226). Phase 1A unchanged.
0.116.0: Contractor job list on the server pattern; JobSummary.technicianMembershipId (IR225). Phase 1A unchanged.
0.115.0: Contractor overview on the server pattern; offer decline events fixed (IR224). Phase 1A unchanged.
0.114.0: The web apps call the REST routes; POST /v1/ops retired (IR223). Phase 1A unchanged.
0.113.0: REST routes for every Core API operation (rest_routes in the operation catalog, IR222). Phase 1A unchanged.
0.112.0: Technician job workspace on the server pattern; per-operation request body limits (IR221). Phase 1A unchanged.
0.111.0: Swagger built from handler annotations, served at /docs (IR220). Phase 1A unchanged.
0.110.0: Web unit tests and backend coverage tests (IR219). Phase 1A unchanged.
0.109.0: Contractor quality review and payouts on the server pattern; demoSeed.rateCards (IR218). Phase 1A unchanged.
0.108.0: Device operation lifecycle on the scheduler, demo device events with the tamper alert, technician devices on the server pattern (IR217). Phase 1A unchanged.
0.107.0: commands.list and Command.source; technician diagnostic control on the server pattern (IR216). Phase 1A unchanged.
0.106.0: Automation onlyIf conditions and lastRun with recorded skips (IR215). Phase 1A unchanged.
0.105.0: Customer automations on the server pattern; schedule firing job, consent withdrawal event, automations.delete and draft preview (IR214). Phase 1A unchanged.
0.104.0: Customer air quality on the server pattern; latest readings, ventilation CO₂ and telemetry normalization (IR213). Phase 1A unchanged.
0.103.0: Customer energy & cost, monthly report and carbon offsets on the server pattern (IR212). Phase 1A unchanged.
0.102.0: Customer units & locations with rename and group control on the server pattern (IR211). Phase 1A unchanged.
0.101.0: Customer contracts, payments and users on the server pattern; session.get returns clientRole (IR210). Phase 1A unchanged.
0.100.0: Admin customers and units part 2 (IR209): client users, warranty & coverage with claimable jobs, CSV import that creates properties and binds serials, units.save warranty end. Phase 1A unchanged.
0.99.0: Admin customers and units on the server pattern, part 1 (IR208): sibling and billing-name uniqueness, overnight alert windows, ac_offline in minutes. Phase 1A unchanged.
0.98.0: Admin automation policies on the server pattern (IR207). Phase 1A unchanged.
0.97.0: Admin access and roles on the server pattern (IR206). Phase 1A unchanged.
0.96.0: Admin restriction screens on the server pattern (IR205). Phase 1A unchanged.
0.95.0: Admin offset demo registry on the server pattern (IR204). Phase 1A unchanged.
0.94.0: Admin MRV workspace on the server pattern; evidence selection open (IR203). Phase 1A unchanged.
0.93.0: Admin energy analysis on the server pattern (IR202). Phase 1A unchanged.
0.92.0: Admin contracts/devices on the server pattern, audit before/after (IR201). Phase 1A unchanged.
0.91.0: Query catalog filters and sorts match the handlers (IR200). Phase 1A unchanged.
0.90.0: Admin audit/alerts/billing on the server pattern, invoice filters and numbering (IR199). Phase 1A unchanged.
0.89.0: Frontend board as built, page data sources (IR198). Phase 1A unchanged.
0.88.0: Architecture diagrams follow the domain services (IR197). Phase 1A unchanged.
0.87.0: Audit log owned by identity (IR196). Phase 1A unchanged.
0.86.0: Scheduler per domain (IR195). Phase 1A unchanged.
0.85.0: Database role per service, cluster test mode (IR194). Phase 1A unchanged.
0.84.0: Cross-domain adapters through owner queries (IR193). Phase 1A unchanged.
0.83.0: Query-backed identity Directory (IR192). Phase 1A unchanged.
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
