---
document_id: DEC-12
version: 0.21.0
status: accepted-for-demo
scope: frontend-demo-1A
---

# 1A adoption decisions and review responsibilities

Basis: user response on 2026-09-16. Document author names follow the corrected response.

Document authors and final decision makers for the 1A specification:

- Masaki Kitano
- Yuma Wakai

This assigns responsibility for documentation and 1A specifications. It does not mean company commercial approval, appointment of production technical owners, or deployment approval.

| Decision | Adopted content | Treatment of earlier findings |
|---|---|---|
| ① Current scope | Mock demo only. Real authentication, HTTP/API, database, and real device connections are out of scope | REV-004: 1A scope decision complete; production contracts are entry conditions for later work |
| ② Business rules | Adopt the following as 1A demo specifications, separate from commercial rule approval. Final decision makers are the document authors above | REV-031: demo adoption and decision ownership complete; company commercial decisions managed separately |
| ③ Timed actions | Use the demo clock. Before production design starts, Backend/IoT must decide who runs browser-independent end actions and failure recovery | REV-036: 1A decision complete; OPEN-11 remains a production design condition |
| ④ Independent review | An AI agent separate from the correction agent reviews the specification and rechecks fixes | REV-037: delegation approved; record execution/results in G1 to determine completion |
| Before deployment | People and external reviewers perform the final review | An AI G1 pass is not deployment approval |

## Adopted 1A demo specifications

- Four roles: customer, contractor, technician, and HQ.
- HQ reviews internal technician reports; the contractor's quality reviewer reviews external technician reports. Self-approval is forbidden. Hand over to HQ if no quality reviewer is available.
- Customers can view results and send inquiries. A final customer acceptance action is not included in this phase.
- English/Malay, with English as default. Demo currency MYR; initial timezone Asia/Kuala_Lumpur. Display invoices in their original currency without conversion.
- Give at least 24 hours' demo notice before nonpayment restrictions. After the deadline, HQ explicitly confirms application. Simulate grace periods, exceptions, and release after payment.
- Show synthetic fault/allergen data with evidence. Label unmeasured data as not measured; do not guarantee real detection or health safety.
- No real payments, notification sending, or credit trading; simulated actions and previews only.
- Adopt the current reference design, desktop/tablet/phone support, and detailed acceptance conditions in current documents.

Correction agents resolve technical conflicts, and independent reviewers recheck them. Do not require extra commercial approval for fixes within the adopted scope. Decisions changing scope or business responsibilities go back to the final decision makers above.

## DEC-13 Three strict-review decisions

On 2026-09-16, the user explicitly approved the recommended options for all three points. Adopt SR17 calendar-day periods, completed minutes, and UTC preservation on timezone change; SR18 stage-specific retries on the same offset record; and SR19 rejection of contract edits until cancellation/release is complete. These are business decisions for 1A mock documents, not independent review passage, completed implementation, or production transaction approval.

## DEC-14 Technical details of re-review fixes

SR22–29 were specified under the user's request to correct findings and repeat the review → correction cycle. Customers can retry only their own records; history uses the intersection with scope at event time; terminal restrictions have separate recovery records; operation categories are shared. AirPolicy uses existing notification settings as required inputs, without guessing new business values as defaults. Keep the fixed 100kWh demo comparison as modeled, distinct from measured quality. No extra real device behavior, production processing, or implementation is included.

## DEC-15 Technical details of re-review fixes (0.11.0)

Basis: the same day's user instructions to correct based on re-review results and repeat the review → correction cycle. These are corrections by the assigned AI, not new company approval or independent G1 approval. The 1A scope stays the same. Details cover the ban on changing the customer of an existing ID, shared simulated reminders, Repository assignment of notice timestamps, exclusive payment attempts, public projections, measured origin/boundaries, Raw normalization, Fact TTL, and view generations. See [IR01–18](../../02-design/review-resolution-contracts.md). Do not add MRV file exports. A long-running memory limit is deferred as a possible addition; retain the existing rule of keeping data until reset. Applicable requirements and acceptance criteria are in the resolution column of the [traceability table](../traceability.csv).

Continuing DEC-15 fixes (0.12.0): Apply [CV-001–008](../../04-agentic-sdlc/acceptance-convergence.csv) under the user's repeated correction instructions. Keep existing 1A scope and business approvals, and resolve inconsistencies in read scope, retries, types, times, and input-saving paths.


## DEC-16 Installation address and report summary after expiry

The user explicitly said the address belongs to the AC installation location, and the post-expiry summary should be “Report present/absent” and “Accepted/not accepted.” Withdraw the proposal for HQ to re-enter the region when outsourcing. IR25 includes the source, public type, and freeze conditions. IR26–28 fix technical inconsistencies in existing requirements; they are not new business approval. Independent G1 and app tests are assessed separately.

0.15.0: The user's “Please proceed” authorized independent review by another AI. Independent findings IRV-001–006 are reflected in IR29–31, existing AT cases, address explanations, and current manifest guidance. Completion timestamps, unresolved unit alert categories, and the ban on self-approval by any contributor clarify existing requirements; they add no commercial rules or features.

## DEC-19–24 Technical details from independent review (FRV) (0.17.0)

In response to another AI's independent findings FRV-001–025, the correction agent adopted six items in [review-decisions-017.json](review-decisions-017.json) as PROPOSED: automatic release requests and idempotent `restrictions.release` (IR35), no session lifetime consumed by demo-clock jumps (IR36), customer-created job dueAt equal to requested slot end (IR38), customer count population (IR40), 1h/24h rolling windows (IR41), and hidden fields in role projections (IR42). All clarify the existing 1A scope; none means new company approval, independent G1 passage, or completed implementation.

## DEC-25–41 Technical details from strict review (REV18) (0.18.0)

On 2026-09-17, under the user's instruction to repeat review → correction until every result is OK, the correction agent adopted 17 items in [review-decisions-018.json](review-decisions-018.json) as PROPOSED. These clarify existing 1A scope, not new company approval, independent G1 passage, or completed implementation. They do not change scope or business responsibilities. Changes update requirements, designs, and acceptance criteria with the same IDs.

## DEC-17/18 User answers to re-review questions

The user chose two restriction permissions and requested sorting for job lists with business order as default. See the [decision record](review-decisions-016.json). Adopt the originally proposed business order and keep existing severity/deadline sorting. This decides the 1A specification; it does not mean completed implementation, independent G1 passage, or deployment approval.

## DEC-54–59 Technical details from independent G1 (DOC-0.19.0) findings (0.20.0)

Status: PROPOSED (reversible). Proposer: independent review and correction AI in this conversation. Adopted to fix findings G1-001–031 from another agent's independent G1 ([runs/DOC-0.19.0/independent-g1](../../04-agentic-sdlc/runs/DOC-0.19.0/independent-g1/review.md), FAIL). See [review-decisions-020.json](review-decisions-020.json) and [correction contracts IR94–102](../../02-design/review-resolution-contracts.md#ir94-authorization-column-qualifiers-and-technician-write-conditions--g1-001--g1-026--g1-030). They cover technician write conditions, business event notifications, restriction cancellation, allergen observations/air-quality guidance, inspection components/submission validation, and failures needed for common acceptance. These are not company approval or final commercial decisions. Product Owner / Business / Security / UI/UX / IoT / QA review them before company acceptance.

## DEC-42–53 Technical details from independent review (REV19) (0.19.0)

Status: PROPOSED (reversible), except DEC-44 (estimated savings calculation, option A) and DEC-50 (work-window end handling), confirmed by the user on 2026-09-17: “For 1, option A is fine” and “For 2, the current proposal is fine.” Proposer: independent review and correction AI in this conversation. This is not company approval or a final commercial decision. See [review-decisions-019.json](review-decisions-019.json) and [correction contracts IR75–93](../../02-design/review-resolution-contracts.md#ir75-current-baseline-and-treatment-of-the-0180-records--rev19-001). They cover pre-work-window displays, simulator copy rules, admin dashboard savings estimates, post-login returns, refetch displays, initial consent, expired Offer responses, reason lengths, work-window end warnings, restriction Commands during device operations, pending audits, and contact-hour input hints. Product Owner / Business / Security / UI/UX / IoT review them before company acceptance.
