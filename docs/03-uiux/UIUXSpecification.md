---
document_id: UX-COMMON
version: 0.21.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# Common UIUX Specification

This document defines shared implementation, interaction, and display rules for all four roles. Wireframes (rough screen sketches) and screen layout diagrams are outside its scope. See the [detailed design](../02-design/common.md) for each screen's business logic. Libraries follow the proposed standards in DEC-02/03. Colors, fonts, and shapes follow the HTML/CSS obtained from the Loyalty page specified by the user. See the [reference design analysis](../00-prepare/reference-design-analysis.md) for evidence and adjusted values.

**Implementation basis for 0.21.0**: Read all chapters of the [deterministic contracts](../02-design/deterministic-contracts.md) and strict-review-contracts.md, the operation catalog's authorization column, and the screen catalog together. Do not guess numeric rules, permissions, asynchronous behavior, or recovery behavior during implementation. These are demo design proposals, not production business approvals.

## Company requests and shared UIUX rules

The primary source is the [company's original English requirements (SRC-06)](../00-prepare/sources/company-requirements-original.txt). Shared rules are based on the display and interaction goals in that source. Appearance follows the reference HTML/CSS specified by the document authors. Library choices, demo languages, and exact dimensions are design proposals based on the production policy.

| Source | Required experience | Shared rules and acceptance checks |
|---|---|---|
| Company original BIZ-01–03 | Start/end use, multiple languages, voice AI | Clearly label demo authentication. Use translation keys to switch display text for every role. Split voice interaction into input, interpretation, confirmation, and response; allow completion through text too. Offering en (English)/ms (Malay) initially, with English selected by default, is a proposal |
| Company original BIZ-04/07/08 | Visual dashboards, location and status awareness | Always show the selected property and unit. Link KPIs (key performance indicators) to the matching list. Give charts a period, unit, retrieval time, and an alternative table view |
| Company original BIZ-09–11/17 | Red/orange/green notifications, component conditions and causes | Use text and icons as well as color. Show connection status separately from severity. Show open windows and poor insulation as possible causes with supporting evidence; do not present estimates as confirmed causes |
| Company original BIZ-13–16/19 | Remote control, scheduling, location/price links, ventilation | Distinguish current and requested values, saving settings and simulated firing, and pending and acknowledged states. Show supported capabilities and reasons why an action is unsupported |
| Company original BIZ-18 | CO₂, dust, humidity, allergens | Show CO₂ concentration (ppm) and emissions (kgCO₂e) as separate concepts. For allergens, show the target, source, observation time, and availability. Do not turn missing measurements into 0 or "safe" |
| Company original BIZ-21/22 | Payment guidance and cooling restrictions | Explain the target, reason, deadline, and release status. Separate credit, debit, and payment instructions. Use selection controls in the demo; do not provide fields for real card numbers |
| Company original BIZ-23–26 | Energy-saving comparisons, MRV (measurement, reporting, verification), Scope 2, optional offsets | Keep the baseline, period, factors, boundaries, and data quality together. Show energy savings, estimated emission reductions, simulated retirement, and future market concepts separately. Leave offsets unselected by default |
| Production policy SRC-02 | Four roles, reference appearance, shared libraries, future API connections | Apply shared Component, Icon, token, and form rules. Access business data through asynchronous Repositories |
| Reference mock SRC-05 | Colors, fonts, spacing, shapes | Follow UX-04 onward and the REF values in the reference design analysis. Record readability adjustments as ADAPT |

Validate each screen's cases that add detail to the source (AT-*-SRC) together with the following shared rules. If a feature found in a mock is added as a mandatory company requirement, record its reason and impact as a design proposal.

## UX-01. UI libraries and sources

| Purpose | Standard/source | Project usage rules |
|---|---|---|
| Basic UI | [Official shadcn/ui docs](https://ui.shadcn.com/docs) | Copy and manage official components in shared/ui. Share buttons, Dialog, Sheet, Tabs, Select, Popover, and similar elements. Do not build separate versions of equivalent UI for each role |
| Icons | [Official Lucide React docs](https://lucide.dev/guide/react) | Use named imports from lucide-react. Default size: 16px; main actions: 20px; emphasis: 24px. Default stroke width: 1.75, matching the reference sidebar SVG. Set aria-hidden on decorative icons and always label icon-only buttons |
| Forms | [Official React Hook Form repository](https://github.com/react-hook-form/react-hook-form) | Use react-hook-form as the standard. See DEC-03 for the interpretation of "reactForms." Keep input state in the form; do not copy it into screen state |
| Schema validation | Zod + @hookform/resolvers | Separate input schemas from DTO (data transfer object) schemas. Keep required fields, formats, and conditional rules for each use case in its schema |
| Data reads/updates | [Official TanStack Query docs](https://tanstack.com/query/latest/docs/framework/react/overview) | Put asynchronous Repository requests, caching, and mutations in shared hooks |
| Lists/charts | TanStack Table / Recharts | Use for complex sorting/pagination and time-series charts. Use the shared Table for simple lists. Do not build custom replacements for charts |
| Date input | shadcn Calendar family | Manage dates separately from time and time zone. Use Intl for display formats and shared functions for conversion to UTC |
| Translation | i18next + react-i18next | Update en (English)/ms (Malay) keys together. English is the default. Notification templates and voice demo responses use the same dictionary system |
| Testing | Vitest, React Testing Library, Playwright, axe-core | Assign separate roles for logic tests, user interaction tests, E2E tests across screens, and automated accessibility checks |

Official sources were retrieved and checked on 2026-09-14. Official documentation was checked for shadcn, Lucide, and TanStack Query. The React Hook Form guide could not be retrieved, so its official repository was checked instead. The other libraries are project candidates; check compatibility, licenses, and maintenance again when implementation starts. Do not guess and pin version numbers.

There is no existing implementation. On setup, create one compatible set of dependencies and a lockfile, and record the selected versions and licenses. Do not add overlapping UI kits or icon sets. For exceptions, record the required feature, why the standard library is insufficient, cost, impact, and whether replacement is possible. Do not force library use where semantic HTML is enough.

## UX-02. Responsibilities of State, Effect, and Context

| State type | Storage | Duplicate management to avoid |
|---|---|---|
| Unit, job, invoice, and history data from Repositories | TanStack Query cache | Copying fetched results into useState/Context/Zustand |
| Search, sort, page, period, selected property | Validated URL search params | Two-way synchronization between URL and local state through Effects |
| Input values, dirty state, validation, submission state | React Hook Form | Managing each input again with useState or creating a separate error dictionary |
| Short-lived local state, such as whether a dialog is open | Component useState; useReducer if needed | Storing it in a screen-wide Context |
| Filtered results, totals, button availability | Pure functions computed during rendering | Saving derived results into separate state through Effects |
| Theme, locale, session, Repository references | Library Providers or small Contexts | Mixing frequently updated values, such as telemetry (sensor measurements, Measurement), into one large Context |
| Shared demo business data | Inside the mock Repository | Different seeds for each screen or using Context as a database |

Limit Effects to synchronization with external systems. Save in response to events through event handlers or mutations. Do not hide dependencies in a way that makes behavior unclear. React's official documentation also recommends avoiding unnecessary Effects. [Official React docs: You Might Not Need an Effect](https://react.dev/learn/you-might-not-need-an-effect)

Valid Effect uses include subscriptions and cleanup, media APIs, and synchronization with external DOM widgets. State the reason and provide cleanup together. Prevent duplicate registration when Strict Mode runs the Effect again during development. Normally use Query hooks to fetch data. Use `useMemo`/`useCallback` only when measurements show a need or stable references are required.

```tsx
// Example: keep filters in the URL, fetch through Query, and derive display values with pure functions.
const filters = parseUnitFilters(searchParams);
const unitsQuery = useUnits(filters);
const visibleUnits = selectVisibleUnits(unitsQuery.data?.items ?? [], filters);
// Do not add an Effect that copies visibleUnits into separate state.
```

Limit Context to information that needs broad distribution, such as session data. Use Providers where needed. Do not make rules such as "no Context," "zero useEffect," or "zero useState" goals in themselves. Reviews should check for a single source of state and a real need for external synchronization.

## UX-03. Form standards

- Use `useForm` and `zodResolver` as the default. Use register for native inputs and Controller only where controlled components need it. Use useFieldArray for array inputs.
- Use a shared Field component for labels, required/optional markers, help text, units, errors, and disabled reasons. A placeholder is not a substitute for a label.
- Match initial values to the schema. After asynchronous loading, reset only when the target ID changes or the user explicitly reloads. Background refresh must not overwrite unsaved (dirty) input.
- Validate first on blur or submission. After an error, validate again whenever the user edits the value. On submission, focus the first invalid field and provide links to fields from the error summary at the top.
- Prevent duplicate submissions while submitting and preserve input on failure. Map mock VALIDATION results to fieldErrors and CONFLICT results to conflict guidance.
- Allow incomplete inspection drafts to be saved. Submission schemas require all mandatory fields. Create a new report instead of directly editing a completed form.
- Check temperature, time, and money boundaries when converting strings to numbers. Do not treat an empty string as 0. Get allowed ranges from capability data or shared schemas.
- Ask whether to discard changes only when leaving with unsaved input. Do not add unnecessary confirmation to normal viewing navigation.

## UX-04. Design tokens based on the reference design

The single source for values is `src/shared/styles/tokens.css`. The following values are adopted for this version. `REF` means a value from the source site; `ADAPT` means an explicit adjustment for these requirements. See the [extraction evidence](../00-prepare/sources/reference-style-evidence.json).

| Semantic token | Adopted value | Basis/purpose |
|---|---|---|
| --color-primary / --color-primary-fg | #005BEA / #FFFFFF | REF main buttons, selected navigation |
| --color-primary-soft | #E6F0FF | REF icon frames, selectable-option backgrounds |
| --color-primary-hover | rgb(0 91 234 / .90) | REF hover:bg-primary/90. This blends with the background; do not replace it with a different fixed dark blue |
| --color-background / --color-surface / --color-surface-2 | #F8FBFF / #FFFFFF / #EDF6FF | REF page, card, and secondary-area backgrounds |
| --color-secondary / --color-secondary-fg | #EDF6FF / #0D2238 | REF secondary color. Do not use teal as a second brand color |
| --color-text / --color-text-muted | #0D2238 / rgb(13 34 56 / .72) | REF body and supporting text |
| --color-text-subtle-reference / --color-text-subtle | rgb(13 34 56 / .50) / rgb(13 34 56 / .72) | Keep the REF value, but use the darker ADAPT value for meaningful labels around 12px |
| --color-hero / --color-hero-fg | #0D2238 / #FFFFFF | REF main summary area. ADAPT supporting text to 72% white |
| --color-border / --color-input-border | #D6E4F5 / #71849A | REF decorative borders; ADAPT input borders for visibility |
| --color-success-accent / --color-success / --color-success-soft | #059669 / #166534 / #DCFCE7 | REF decoration/background; ADAPT text |
| --color-critical-accent / --color-critical / --color-critical-soft | #FF4757 / #B91C1C / #FFE5E9 | REF decoration/background; ADAPT text |
| --color-warning / --color-warning-soft | #9A3412 / #FFEDD5 | REF orange warning colors |
| --color-unknown / --color-unknown-soft | #475569 / #EDF6FF | ADAPT; do not use normal-status colors for unknown or missing data |
| --color-focus | #005BEA | REF 2px outline with 2px offset. Add a 3px/50% ring to buttons |
| --chart-series-1–4 | #005BEA / #0D2238 / #7C3AED / #9A3412 | ADAPT; series 1 and 2 use brand colors. Use line styles and legends as well as colors |

| Category | Token/value | Usage rules |
|---|---|---|
| Body font | --font-sans: "Plus Jakarta Sans", system-ui, sans-serif | REF. English and Malay both use Latin script, so no extra fallback font is needed. Set font-display:swap |
| ID/code font | --font-mono: "Geist Mono", ui-monospace, monospace | REF. Limit use to items such as contract IDs and device serial numbers |
| Optional display font | --font-display: "Bricolage Grotesque", var(--font-sans) | Defined as REF, but not the default h1 font for business screens |
| Font size | --text-xs:12px / sm:14px / base:16px / lg:18px / xl:20px / 2xl:24px / 3xl:30px / 5xl:48px / 6xl:60px | REF scale. Body text: 14–16px; important explanations: at least 12px. Use 5xl/6xl only for hero numbers |
| Line height | xs:16px / sm:20px / base:24px / 2xl:32px / 3xl:36px / hero:1 | REF. Use the same line heights for English and Malay |
| Weight/letter spacing | normal400 / medium500 / semibold600 / bold700; KPI numbers: -.01em | REF. Apply .12em letter spacing and uppercase to short labels in both English and Malay |
| Spacing | --space-1–8:4/8/12/16/20/24/28/32px; --space-12:48px | REF 4px steps. Inside cards: 16/20px; hero: 24px→28px |
| Corner radius | --radius-control:10px / --radius-card:14px / --radius-nav:16px / --radius-small:6px | REF. Keep the actual values rounded-lg=16 and rounded-xl=14 |
| Shadow | --shadow-card:0 14px 34px rgba(13,34,56,.055) | REF. May be combined with a 1px 70%-white ring or a 90%-opacity border |
| Button shadow | --shadow-action:0 10px 22px rgba(0,91,234,.18) / hover:0 14px 30px rgba(0,91,234,.24) | REF, for primary buttons only. Do not lift every card on hover |
| Motion | --duration-normal:200ms; --ease-out:cubic-bezier(0,0,.2,1) | REF hover: y=-2px; press: y=1px/scale=.985. Disable movement under reduced-motion |
| Content width | --content-max:1152px | REF max-w-6xl=72rem. The reference HTML has no margin-inline:auto; do not add it automatically |
| Sidebar | --sidebar-width:240px / --sidebar-width-icon:56px | REF inline values from the reference screen. Do not use the library defaults of 256/48px |
| Breakpoints | sm640 / md768 / lg1024 / xl1280px | REF. Navigation switches at xl, KPIs at md, secondary areas at lg |
| Stacking layers | --z-sidebar:10 / --z-header:30 / --z-modal:50 / --z-toast:60 | First two: REF; last two: ADAPT. Manage stacking and focus together |

Map shadcn's `--primary`/`--background`/`--card`/`--muted`/`--border` to these values. The reference page's .bg-background is white and .bg-bg is pale blue; do not merge them into one token. Keep raw color values only in this table and their source; screens must use semantic tokens.

Serve fonts locally after checking the official licenses. Do not permanently hotlink the reference site's hashed woff2 files. English and Malay both use Latin script, so no additional font design is needed. Dark mode and copying the brand logo are outside this standard.

## UX-05. Shared component contracts

| Shared component | Input summary (component-contracts.csv is authoritative for props; IR72 rank 5, IR102) | Rules |
|---|---|---|
| AppShell / RoleNavigation | role, allowed routes, display name, appNameKey | Check both navigation visibility and service access. Mark the current location with aria-current. Disable voice switching for roles without permission and show the reason (IR44). Use translation key app.name for the app name; always show a DEMO label in demo-only operation areas (IR60/IR74) |
| SessionExpiryDialog | session, now, extending, error | Show as role=alertdialog 120 seconds before Session expiry; initially focus "Extend." ShellContainer performs extension and sign-out (IR55) |
| ErrorBoundary | fallback, correlationId | Show uncaught rendering exceptions as a full-screen error instead of a blank screen (IR44) |
| KpiCard | label, value/null, denominator/null, unknownCount/null, asOf, href (list with the same conditions) | Share count KPIs across all four roles. null means "Cannot calculate"; 0 means "0." Do not replace failed retrieval with 0 (IR90) |
| MetricCard / TelemetryValue | value/null, unit, origin, quality, observedAt, isDemo | Show "— Not measured" for missing measurements. Do not hide demo/estimated labels or update times |
| StatusBadge | domain, status, labelKey | Use separate dictionaries for unit, connection, job, and invoice status names. Do not rely on color alone |
| DataTable | columns, rows, sort, pagination, rowAction | Support keyboard and mobile use. Paginate large tables. Do not rely only on clicking rows |
| TimeSeriesChart / EnergyChart | series, unit, quality, period | Do not join lines across missing measurements. Include a numeric table and legend. Clearly label units when using two axes |
| VoiceContainer / VoicePanel | Container: context, locale, routeJobId / Panel: intent, candidates, job candidates, reasons, confirmation state | VoiceContainer (feature hook) handles fetching/submission; VoicePanel only displays data and emits events (IR09/IR90) |
| CommandPanel | capability, observedState, pendingCommand, permission | Do not replace a pending state with a success toast. Show why an action is unsupported |
| ConfirmActionDialog | target, action, impact, reason, onConfirm | Share across restrictions, release, commissioning, firmware updates, voice setting changes, and similar actions. Initially focus the safe option |
| AsyncBoundary / EmptyState | status, messageKey, retryAction | Do not show persistent errors only as temporary toasts |
| Timeline / NotificationPanel / NotificationPreview | actor, time, action, result, correlationId | Label previews as "Preview — not sent." Distinguish read status from business completion |

UI primitives must not call business Repositories directly. Business components receive typed props and callbacks; feature hooks fetch data. Express role differences through permissions and displayed data instead of maintaining four copies of the same component.

## UX-06. Interaction, language, and accessibility

Charts and status cards must let users move from an overview to evidence and details. Keep the period, organization, and units clearly visible. Distinguish success, pending, and failure states, and give the next action with each error. Show an "Undo" button after a destructive action only when that action can actually be undone.

Validate English/Malay keys, plurals, and long translations. Mark ms dictionary text as an unverified implementation-agent draft; Business/UI/UX must review it before company acceptance (IR74). Fall back to en for missing ms keys; if both are missing, show the key string. Lint must check that en/ms key sets match (IR44). Use locale tags en-MY/ms-MY, amounts in the order "120.00 MYR," temperatures with one decimal place, and decimal half-up rounding (IR44). Format times with Intl.DateTimeFormat and money with Intl.NumberFormat; show the time zone for bookings. Changing language must not change measurement units or stored UTC values. For voice demos, confirm the transcript, target, and action, then process it as a normal Command.

Use WCAG 2.2 AA as the design target. Normal text must have at least 4.5:1 contrast; large text at least 3:1. Check that controls are recognizable and focus is visible. These are implementation test criteria, not a guarantee of compliance. [Official W3C quick reference](https://www.w3.org/WAI/WCAG22/quickref/)

The project's target touch area is 44×44px. Allow the main scenarios to be completed with a keyboard alone. When a Dialog closes, return focus to its trigger. Announce critical errors through suitable live regions; do not announce every telemetry (sensor measurement, Measurement) update. When refetching displayed data, use aria-busy and an "Updating" indicator; do not return to a skeleton (IR83). Also test at 200% browser zoom, 360px width, and with a screen reader.

## UX-07. UI review pass criteria

- Report library sources, adopted versions, and shared component locations. Do not add duplicate kits.
- Give forms, Query, URL, and local state clear responsibilities. Every Effect must have a reason to synchronize with an external system.
- Check that colors, fonts, sizes, and spacing use tokens, and that status colors are paired with text and icons.
- Check loading/empty/error/forbidden/offline/stale displays and preservation of input after submission failure.
- Test smartphones, keyboard use, English/Malay switching, missing data, long text, and voice alternatives. Clearly identify checks not yet run.
- Do not show success before device confirmation, imply real transactions, confuse CO₂ units, or suggest that marking an item read has resolved a fault.

## UX-08. Screen patterns based on the reference design and acceptance checks

Define reusable composition rules without creating wireframes. Each role's detailed design must name the pattern it uses.

| Pattern ID | Layout/dimension rules | Main use |
|---|---|---|
| UI-OVERVIEW | Title 24px→30px + description 14px; optional dark hero area; KPIs in 2→4 columns; cards below. Page padding x16/y24→x32/y32 at xl | Dashboards for each role |
| UI-LIST | Shared shell with heading/actions, search/filters, list in a white card, count/pagination, and links to details | Unit, job, invoice, and device lists |
| UI-DETAIL | Main information card and related history. At lg and above, use minmax(0,1fr)+320px with a 20px gap. Stack vertically on small screens | Unit, job, and invoice details |
| UI-FORM | White card with 14px labels/16px inputs, grouped into meaningful sections. Use 16px field gaps, primary/cancel buttons, and an error summary | Location, report, policy, and contract forms |
| UI-ANALYSIS | Shared KPIs, units/period, charts and numeric tables, evidence/data-quality cards | Power, air quality, and MRV analysis |
| UI-TIMELINE | White card; each row has py12px, event/time on the left, result badge on the right. Wrap long IDs | Audit logs and job history |

At widths of 1280px and above, fix desktop navigation to the left. Below that, use a top header and Sheet. While the drawer is open, stop background scrolling. Support Esc to close it; after selection, close it and focus the destination heading. Keep the same color and font systems across roles. Distinguish roles through menus, permissions, and key metrics.

Record readability and interaction adjustments for business screens as ADAPT. Test layout width, wrapping, and navigation switch boundaries at 360px, 768px, 1024px, 1279px, 1280px, and 1440px. On business screens with input, prioritize 44px touch areas instead of copying the reference page's 28px controls.

During acceptance, check token values, applied fonts, 14px card corners, 240/56px sidebar widths, breakpoints, and key components through the DOM and computed styles. Record implementation screenshots. Compare pixels against the reference page only if a separate rendering baseline for that page is available. Do not mark an unperformed comparison as passed.

## UX-09. Screen and Component implementation contracts (0.17.0)

The [screen catalog](screen-catalog.csv) defines Screen IDs, roles, FR/DD links, entry/exit paths, URL selections, tabs, primary/secondary Queries, states, and inputs/outputs for 48 routes. The [Component contracts](component-contracts.csv) define display responsibilities, Props, State, Event, dependencies, and Loading/Error/Empty behavior. Compose pages from shared components; change data through each Page's Repository operations.

Button flow: validate input → confirm (control/delete/restrict/pay/submit/accept) → write → versioned result → invalidate Query → redisplay. Cancelling confirmation causes zero writes. Do not add destructive-action confirmations to reads, selections, or previews. Disable the same action during submission; use D04 keys to prevent duplicates from other paths. Distinguish request acceptance from device acknowledgment. On failure, preserve input, correlation ID, and retry access. Show network offline, Device offline, connecting, error, and stale separately.

For shared routes, select schedule/event for C04/C05 and payment/restriction/inquiry for C11/C12 through the URL tab. Limit partial failures to the affected secondary Query panel. Disable controls if capability/restriction retrieval fails. Do not treat missing measurements, empty lists, and 404 as the same state. See deterministic contracts D01/D09/D10/D13 for the remaining navigation and input constraints.

0.9.0 correction contracts: Read the [strict review correction contracts](../02-design/strict-review-contracts.md) and [operation version contracts](../02-design/write-version-catalog.csv) together.

Approvals applied on 2026-09-16: Use SR17 for period presets. Show the offset retry button only for failed, and label the failed purchase/retirement stage (SR18). Follow SR19 for contract edit denial reasons and links to restriction cancellation/release.

Additional contracts for current version 0.21.0: Read IR01–106 in the [review resolution contracts](../02-design/review-resolution-contracts.md). They take priority over older text on the same issue; follow IR72 when rules conflict.

0.14.0: Under IR25, get the address before acceptance from the unit's installation property. After expiry, freeze only whether a report exists and its acceptance status.

IR26 provides common list/summary conditions for P01/T01. Show IR27 disabledReason for stopped items on C04/C05/A04. Follow IR28's create/update branches for required reasons in the model form.

0.15.0: P01/T01 severity follows IR30's unresolved alert categories. Do not present normal as a guarantee that the whole unit is healthy. Display self-approval denial as IR31 FORBIDDEN.

Disable P05/A06 accept/return buttons according to WorkReport.reviewAvailability and show the reason. IR31 also rejects direct calls.

0.16.0: IR33 covers A16's required audit Query, device candidate selection, URL restoration, and permission-aware related links. IR32 covers unitIds for P01/T01 lists and summaries.

0.18.0: IR50 covers KPI-to-list navigation and allowed URL keys; IR57 FORBIDDEN/NOT_FOUND displays; IR46 operation choices under restrictions; IR47 device connection displays; IR55 the session extension dialog; IR68 negative reductions. Device states (connecting/device-*) apply only to Screens that read units, devices, measurements, controls, restrictions, or summaries (IR74).

0.19.0: Use work-not-started for technician screens before the work window (IR76). Keep successfully loaded data during refetch and show aria-busy with "Updating" (IR83). Use KpiCard for count KPIs and VoiceContainer for voice fetching/submission. IR90 covers public screen state sets and empty notification lists; IR78 covers admin dashboard energy-saving forecasts; IR89 covers work-window end notices and the "Work window ended — reassignment required" display.

Job list sorting follows IR34. Allow selection of status (business order), severity, or deadline, and ascending/descending order. Default to status in ascending business order. Apply URL restoration, cursor reset, loading/error states, keyboard use, and aria-sort through the shared DataTable. Separate restriction-operation displays by the two permissions restriction.manage/override.
