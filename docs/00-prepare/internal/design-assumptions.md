---
document_id: PREP-INTERNAL
version: 0.21.0
status: working-notes
scope: frontend-only
updated: 2026-09-17
---

# Developer notes: assumptions and technical choices

[PrepareDocument](../PrepareDocument.md) is authoritative for explanations to the company. This file supports design and implementation agents; it is not a record of original company requirements or company approval. The latest company original is SRC-06; development scope instructions are SRC-02.

## Technical choices and demo assumptions


| ID | Status | Details/reason | Impact of a change |
|---|---|---|---|
| DEC-01 | PROPOSED | Contractors manage accepted work, own staff, and quality. Reassignment stays within HQ's delegated scope | All FR-P, S02/S08, MaintenanceJob |
| DEC-02 | PROPOSED | New SPA: React + TypeScript + Vite + React Router. Reconsider if SSR is needed | Shared design/build; check compatibility and pin dependency versions when implementation starts |
| DEC-03 | PROPOSED | Interpret “reactForms” as React Hook Form. Use Zod, shadcn/ui, Lucide, and TanStack Query | UIUX, forms, data boundary |
| DEC-04 | PROPOSED | Initial demo languages: English (en, default) and Malay (ms). Extra languages and rollout order are unconfirmed | Translation keys and AT-X01/X02 |
| DEC-05 | PROPOSED | Demo currency MYR; display timezone Asia/Kuala_Lumpur. Language/timezone can change. Currency is fixed demo MYR (invoices keep original currency) | Not a market decision; money/time formatting |
| DEC-06 | Production instruction (document authors) | Match customer Loyalty HTML/CSS: #005BEA primary, Plus Jakarta Sans, 14px cards. Remove the earlier separate color proposal | Evidence in [design analysis](../reference-design-analysis.md) and UIUX; mark accessibility adjustments as ADAPT |
| DEC-07 | PROPOSED | Shared in-memory demo Repository. Reload resets to seed; role switches in one tab preserve data | Demo instructions/tests; persistence and multi-tab sync are out of scope |
| DEC-08 | PROPOSED | Default voice behavior simulates transcription and responses; no real microphone needed | FR-X02, consent/refusal demo |
| DEC-09 | PROPOSED | Define inputs, business rules, outputs, and failures for 49 features. [Implementation contracts](../../02-design/implementation-contracts.md) and each DD govern demo times, limits, pre-acceptance projections, and rejection of overlapping confirmed schedules | Not production approval; changes update requirements, design, and AT-N/E/B with the same ID |
| DEC-10 | PROPOSED | Define 1A revisits, multiple resources, and conflicts. Preserve state on reassignment; manual payments start from invoices; release restrictions after all cause invoices are paid; do not restore previous power/temperature on release. Keep test runs and images in shared mocks | [DDC-08](../../02-design/implementation-contracts.md#ddc-08-multi-resource-revisit-and-cross-role-contracts), AT-*-R01, related FR/DD; not official company rules |
| DEC-11 | PROPOSED (reversible demo design under current correction instructions) | Define demo details for 36 findings: canonical DTOs, authorization, arbitration, recovery, numbers, screens, and checks | [Deterministic contracts](../../02-design/deterministic-contracts.md), operation/screen catalogs; excludes commercial approval and production contracts |

| DEC-19–24 | PROPOSED | Technical details from 0.17.0 independent FRV review (IR35–44): release triggers, clock jumps, job deadlines, customer counts, presets, projections | [Decision record](review-decisions-017.json); reversible, not company approval |
| DEC-25–41 | PROPOSED | Technical details from 0.18.0 strict REV18 review (IR45–74): heartbeat simulator, allowed actions under restrictions, derived connectivity, Offer expiry, view/work windows, counts, routines, consent withdrawal, session extensions, cancellation table, automatic savings baseline selection, contact hours, voice matching, negative values, non-working days, translation review, app name | [Decision record](review-decisions-018.json); reversible, not company approval |
| DEC-42–53 | PROPOSED (DEC-44/50 confirmed by user) | Technical details from 0.19.0 independent REV19 review (IR75–93): pre-work-window display, simulator copy rules, estimated savings, post-login return, refetch display, initial consent, expired Offer responses, reason lengths, work-window end warning, restriction Commands during device operations, pending audits, contact-hour input hints | [Decision record](review-decisions-019.json) |
| DEC-54–59 | PROPOSED | Technical details from 0.20.0 independent G1 findings G1-001–031 (IR94–102): technician write conditions, business event notifications, restriction cancellation, allergen observations/air-quality guidance, inspection components/submission validation, failures needed for common acceptance | [Decision record](review-decisions-020.json) |
| DEC-60–61 | PROPOSED | 0.21.0 independent G1 fixes: duration start, ventilation candidate eligibility, and business notification severity (IR103/104) | [Decision record](review-decisions-021.json) |
| DEC-12 | ACCEPTED FOR 1A | Mocks only. Adopt proposed business rules separately from commercial approval. Final decision makers: Masaki Kitano and Yuma Wakai. Independent review by another AI; final human/external review before deployment | [User decision record](decision-record-2026-09-16.md); do not guess missing production contracts |

PROPOSED items have no human business approval. DEC-06 is recorded in a summary of past production instructions. The original message is not archived; see [instruction verification status](../sources/production-instructions.md). Reversible 1A work can proceed using proposed specifications.


## Record of review coverage

HTML, linked CSS, and some public code for the reference Loyalty page were retrieved on 2026-09-14. Other screens rely on earlier handover research and were not rechecked to the same depth. Environment limits during the initial retrieval describe research conditions, not missing reference-app features. See [design analysis](../reference-design-analysis.md) and [extracted evidence](../sources/reference-style-evidence.json) for reference values.

## How to read links to the original

FR numbers are tracking IDs reorganized from the company original (SRC-06). Use the [requirement source table](../requirement-origins.csv) to check original locations and added design details. Assignment, report approval, payment confirmation, and detailed firmware/calibration/registration procedures include design details added to meet the company's monitoring and maintenance goals.

Screen meanings of red/orange/green, consent, data quality groups, state-transition names, and time values are frontend proposals supporting company goals. Do not turn questions or expected benefits in the original into verified capabilities or guaranteed results.

DEC-12 confirms adoption of the demo policies listed in that decision record. Earlier PROPOSED source labels remain because they do not mean commercial approval; demo adoption does not need to be confirmed again.
