# Independent Supporting Review of State Transitions and Authorization

- Scope: DOC-0.14.0 / frontend-demo-1A
- Reviewed baseline: `f8e6cdd8387f1e623f69616518f41108e55ae9cd5f7ec4b223be5776238f4096`
- Review date: 2026-09-16
- Reviewer: Independent supporting AI `/root/state_review`
- Method: Read-only comparison of specifications. Existing self-review pass records were not used as a basis for the assessment.
- Assessment: **0 new findings** and **0 unresolved findings** in the assigned scope below.
- Application runtime validation: **not_run**. This assesses document implementability and consistency; it does not approve implementation, real devices, external APIs, payments, or production suitability.

## Scope Checked

The review focused on `deterministic-contracts.md`, `strict-review-contracts.md`, `review-resolution-contracts.md`, `service-contracts.ts`, `implementation-contracts.md`, `operation-catalog.csv`, and `write-version-catalog.csv` in `docs/02-design/`. It compared relevant admin/client/technician requirements and detailed designs with applicable conditions in `acceptance-fixes.csv`, `acceptance-strict-review.csv`, `acceptance-rereview.csv`, `acceptance-resolution.csv`, and `acceptance-convergence.csv`.

The main focus was time-limited control, Device binding, Restriction, synchronization of payment confirmation with restriction evaluation, and equipment operation classifications for summaries. Job projections, address disclosure, report summaries, and the overall G1 decision are assigned to the main review. The zero findings in this supporting review must not be reused as a standalone approval of all specifications. The review assumes the user-approved 1A demo scope, the policy to obtain addresses from the installation property, and the policy to show only report availability and acceptance status after expiry.

## Specific Scenarios and Assessment Basis

| Scenario checked | Expected result in the documents and basis | Assessment |
| --- | --- | --- |
| Command response arrives just before, exactly at, or after expiry | D04 allows success only when `receivedAt < expiresAt`. Expiry takes priority at equality, and a late response does not change expired back to acknowledged. AT-FIX-028 defines the boundary. | Consistent |
| During a diagnostic run, the user leaves the screen, switches roles, loses an assignment, or loses communication | The diagnostic-run contract in implementation-contracts continues on the Repository clock and reauthorizes the original Membership at the end. Blocked termination is recorded as end_blocked/end_failed; the run is not marked stopped without a response. D05 equipment exclusivity and reserved Command slots for diagnostic runs are also consistent. | Consistent |
| FW/check success arrives after the 60-second deadline | D05 gives failure priority at equality and does not update firmwareVersion on late success. Retrying uses a new operationId and key. | Consistent |
| A Device is moved to another Unit and an old-binding event arrives late | D05/SR24 ends and retains the old binding, creates a new sensorId, and sets the observation to unknown and lastSeenAt=null. Old-binding events affect history only, not current state, Alerts, or notifications. Recovery of an old binding returns CONFLICT. IR21 specifies discarding retained facts. | Consistent |
| After Device rebinding, the current customer reads the old customer's history | SR24 requires the intersection of current Device read permission and current read permission for the Unit/customer at the time of the event. Counts and pagination are calculated after authorization. IR02 prohibits changing the customer of an existing Unit. | Consistent |
| Full payment arrives when restriction application is undelivered, sent with an unknown result, or confirmed | D03 distinguishes not_required based on non-delivery evidence, reconcile for a sent command with an unknown result, and remove for an applied restriction. SR05 separates logical restrictions from observations and does not set the aggregate state to released until evidence is available for all units. | Consistent |
| An old ID of a terminal Restriction is observed again with a new sequence; a successor restriction also exists | SR26 does not revert the terminal state and blocks control through a recovery case. It removes only the old ID and preserves the successor's logical policy. With multiple cases, blocking continues until all are resolved. AT-REREV-005/AT-CYCLE-007 define the check conditions. | Consistent |
| Editing a contract with an active restriction or unresolved recovery case races with schedule | SR19/SR26 requires revalidation within the same transition and rejects edits while a restriction or recovery case is active. If an earlier edit succeeds, schedule with the old expectedContractVersion is rejected. | Consistent |
| Override-only HQ starts release and recovers from an unknown result | IR03 defines list/get/override through a limited projection and reconcile/retry release after release intent. Canonical types, operation permissions, and the version catalog support this path without retrieving invoice details. | Consistent |
| Payments for the same Invoice start concurrently with different keys; a manual payment also races | IR06 limits initiated/processing attempts to one. It checks the Invoice version and rejects manual payment when a nonterminal attempt exists. D04 and write-version-catalog distinguish the initial Invoice version from subsequent Payment versions. | Consistent |
| One of several invoices causing a restriction is paid, then all are paid | The manual-payment/multiple-invoice contract in implementation-contracts puts Payment/Invoice/audit/notification/restriction evaluation in one transition. Paying one invoice does not release the restriction. When all are paid, scheduled becomes cancelled and requested/applied becomes release_requested. processing is not treated as paid. | Consistent |
| Retrieve operation KPIs and lists with old observations, missing data, or unknown communication | SR27 uses effectivePowerState as the common classification and requires a fresh power-state observation and the latest measured/valid power. SR06 links to filters using the same classification. Communication unknown is separate from operation unknown. | Consistent |
| An asynchronous response belongs to an old view generation, or a page is retrieved after permission expires | D07/SR14/IR17/IR24 defines revalidation of generation/viewEpoch/scope and current permissions, discarding old callbacks, and invalidating snapshots when visibility shrinks. Accepted business processing is separate from cancelling a view. | Consistent |

## Handover and Re-review Conditions

These are the results of the initial supporting review against a fixed baseline. The corrections for IRV-001–003 shared by root (summary acceptance criteria, address wording, and old handover links) are not treated as verified in this record. The main reviewer checks the correction diff to decide approval of a new baseline and overall G1. Re-review this assigned scope if time-limited control, binding, restriction, payment, or authorization contracts, or their related types, operations, or version conditions, change.
