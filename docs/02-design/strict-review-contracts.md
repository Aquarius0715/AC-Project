---
version: 0.21.0
scope: 1A frontend mock only
status: specified_pending_independent_review
---
# Strict Review Correction Contracts

Applies to review STRICT-DOC-0.8.0-2026-09-16, with issue IDs separate from old REV/IND. These correction contracts override older text on the same topic. Canonical types: [service-contracts.ts](service-contracts.ts); operation version requirements: [write-version-catalog.csv](write-version-catalog.csv). No production communication, authentication, payment, or IoT is added. SR17–19 were finalized on 2026-09-16 when the user accepted all three recommendations. This paragraph describes version 0.9.0 history. Judge current G1 from the baseline gate record named in the docs index, not old decisions.

## SR01 Conflict Detection Before Acceptance

JobOfferSummary.jobVersion is the source MaintenanceJob.version, not Offer.version or termsVersion. Pass it as options.expectedVersion to jobs.accept/decline. If changed, return CONFLICT, refetch/re-display terms, and reconfirm. Reject expired/out-of-scope offers under D01/D06.

## SR02 Write Version Contracts

write-version-catalog.csv lists every write and input branch. required means put the primary resource's fetched version in options.expectedVersion; omit means it is prohibited. Check all input_versions conditions for independent input versions. Do not require a nonexistent version for new resources. schedule checks selected Contract.expectedContractVersion; offset requests check saved quoteVersion; Invoice creation uses contractVersion. New reports conflict if Job already has a draft. consents.get returns a versioned Consent with granted=false from the first read (initial record source: IR84 demoSeed/Membership creation).

devices.addResponseNote uses DeviceEvent.version, not Device.version. calibrate/check/bind/updateFirmware use Device.version. payments.recordManual uses Invoice.version; confirm uses Payment.version. notifications.markRead uses Notification.version. Idempotent resends follow D04 and keep original input/version. Changed business decisions after rereading require a new key. read_source identifies version source; recheck all related state in the same transaction on update.

## SR03 Scope Types and Containment

Membership.scopes is ScopeRef[]. Empty means zero business-resource access, never all access. tenant contains all tenant resources; organization its resources; property its Space/Unit descendants; unit that Unit. Space hierarchy does not change Property ownership. Related job/device/report access follows their Unit; invoice/contract follows customer organization and all target Units; notification also requires own recipient. Return invoice/contract only if every target Unit is readable, not partial-scope details. Scope entries form a union; tenant/role/permission/owning organization/valid period/delegation/assignment conditions form an intersection. Multi-unit writes require all Units allowed, otherwise reject with zero side effects.

Only admin may hold tenant scope for its own tenant. client may hold only its customer organization or its properties/units. contractor may hold only its own contractor organization; derive resource reach from company Offers/accepted Jobs, never the whole customer organization. technician may hold property/unit only; external technicians additionally intersect company delegation and own Assignment. Apply required Assignment conditions to internal-technician writes too. No delegation-expiry exceptions except limited pre-acceptance Offer projections and own post-expiry JobHistorySnapshot. Empty scope returns neither.

members.save validates scope types, actual IDs, and organization relationships; invalid combinations return VALIDATION. Role allowlist is DDC-07. Role/permission/scope/validity changes increment scopeVersion and invalidate old contexts/caches. Recheck delegation, assignment, and qualification expiry on every read/action. Compute list total after authorization; individual out-of-scope targets return NOT_FOUND. Own session/preferences/logout work with empty scope.

## SR04 Screen Fetch Order and Candidates

Screen-catalog primary/secondary does not mean concurrent execution. Enable session→candidate list→selected ID→detail→dependent reads in order. No selection shows guidance; zero candidates shows empty. Invisible URL IDs stop at not-found; never replace with another candidate. Do not call sections whose API permissions are absent.

| Screen | Sources and dependencies |
| --- | --- |
| T04 | jobs.get→unitId→units.get. Build form after fetching capabilities, components, and inspection schema |
| A09 | contracts.list→select contract. Fetch invoices.list(contractId) and units.list(unitIds), check capabilities with units.get, and pass Contract.version to schedule |
| A14 | Select organizationId from organizations.list(kind=customer)→units.list(filters.organizationId), baselines.list, factors.list→select conditions→mrv.preview. After selecting reportId, use mrv.versions/mrv.get(reportVersion) |
| T10/A02 | Select Unit→diagnosticRuns.list→diagnosticRuns.get. HQ run history is read-only |
| T11/A04 | devices.list→devices.get→devices.events/calibrations/operations. Binding candidates: units.list→units.get. T11 jobId candidates: active assigned Jobs for the Unit from jobs.list |
| A07/A15 | customers.list→select customer→units.list. A15 quoteId comes only from offsets.preview response |
| C04/A11 | units.list→units.get→capability-based rule inputs |
| C06/C07/C09/C13/A06/A12/A13 | Use only authorized units.list candidates for target selection |
| A03 | organizations.list→select organization; members.list→edit membership. New userId options are only public demo userId/displayName from fixture actors, not production user search |

After writes, every screen applies the returned resource and invalidates related lists/summaries. SR14 governs updates during multi-page fetching.

## SR05 Logical Restrictions and Observations

UnitDetail.effectiveControlPolicy projects the logical policy Repository uses to decide control eligibility. Exclude billing amounts, cause invoices, and contract details. Schedule/notice/grace alone is unrestricted. From execute request use restricted/requested; after apply acknowledgement, applied; after release request, release_requested. Command failed/expired or disconnection alone does not lift logical restrictions. Release acknowledgement or confirmed not_required for that Unit makes it unrestricted. Partial multi-unit release clears only successful Units, separately from aggregate Restriction released. Show observedRestriction as last observation, pendingCommands as work in progress, and effectiveControlPolicy as current eligibility. UI and Repository return denial reasons from the same policy.

## SR06 Filters and KPI Navigation

query-catalog defines each operation allowlist. Unknown keys, invalid enums, empty statuses/connections return VALIDATION. Filters combine with AND; values within statuses/connections use OR. Do not combine status and statuses, or units.list status (connection alias) and connections. includeDescendants requires spaceId and defaults false. true includes Units in selected Space and all descendants; false includes direct members only. Inconsistent spaceId/propertyId returns VALIDATION.

jobs.customerId/propertyId joins Job.unitId→Unit→Property/customer. invoices.customerId is Contract.customerId; propertyId matches when at least one contract Unit belongs to that Property. alerts also joins through Unit. overdueOnly=true means now>dueAt and not completed/cancelled for jobs, or unpaid for invoices. false/omitted adds no filter. now=dueAt is not overdue. from/to uses half-open intervals on each query's time axis.

Partner business counts include only active IR23 JobSummary: scheduledCount uses statuses=[accepted,assigned], inProgress=[in_progress], active=[accepted,assigned,in_progress]. Admin jobCounts uses the relevant status. Connection-unknown KPI uses connections=[unknown,connecting,error]; online/offline use single values. powerOn/Off use powerState=on/off; all power states, including unknown, use SR27 effectivePowerState. Carry customer/property filters to destination URLs. Carry periods only for job/energy, not current-unit/alert/unpaid KPIs. C10 overdue maps to overdueOnly=true; all omits it.

## SR07 Inspection Inputs and Authors

WorkReportDraft takes InspectionItemInput/InspectionMeasurementInput. UI must not construct Entity, author, receipt time, or quality. Repository generates them from execution session/clock; unknown fields return VALIDATION. Generate InspectionItem.id on first save and preserve it across same report/componentKey edits and versions. Reject duplicate componentKey. Measurement.sensorId is the corresponding InspectionItem.id. Missing measurement componentKey in the same draft returns VALIDATION.

Omitted measurement id creates new; existing IDs edit only within that report. Other-report IDs return NOT_FOUND. New measurements have origin=inspection, receivedAt=now, unitId=Job.unitId, version=1; Repository assigns eventId/sequence. Derive quality under D07 number/unit/missing rules. Update authorId/observedAt to actor/save time only on changed items, retaining unchanged authors. Submitted versions are immutable. Photo references must be Attachments in the same report.

## SR08 History Reads

devices.calibrations/operations are Page reads requiring deviceId and Query. Besides the same authorized Device scope as devices.events, check event-time history scope under SR24. diagnosticRuns.list requires unitId; optional jobId adds AND. Technicians use own assignment scope; HQ may list/get with job.manage or control.execute. Permission to read a run does not grant technician diagnostic writes. Support empty/paging/failure/refetch for history; never overwrite past actions with only latest state.

## SR09 Calculation Evidence and Old Versions

EnergySummary returns complete calculation-time baselineSnapshot/factorSnapshot with baselineRef/factorRef. If no baseline is selected, baselineRef and baselineSnapshot are both null. Use the factor version chosen in MRVConditions; normal energy.summary uses fixture.defaultEmissionFactorId's version at read start. Reference/snapshot id/version must match. Missing factor makes both factor fields and emission values null, with factor_missing in qualityWarnings. Later master-data changes do not recalculate saved MRV summaries.

New/updated mrv.saveDraft saves current conditions, factors, baselines, and results as a new version. Keep old versions. mrv.versions pages all saved report versions by version; mrv.get(reportVersion) reads old versions, defaulting to latest if omitted. Old versions are read-only and cannot be edited/reviewed. recordReview creates a new version only when latest and both input/options versions match, leaving the old version unchanged. Review-history reportVersion is the reviewed version. Audit also records the newly generated version.

## SR10 Acceptance-Criteria Consistency

AT-A02-B expects archive CONFLICT for active dependencies but permits history-only archive. AT-A09-R01 becomes released only after every Unit has remove acknowledgement or not_required evidence such as no send. AT-C01-E distinguishes NOT_FOUND for other-customer individual reads/writes from successful empty scoped lists. Acceptance criteria without execution evidence remain not_run.

## SR11 URL Selection Values

Use screen-catalog.url_selection as the screen-specific key allowlist with common filter/sort/period keys. Restore C02 propertyId→spaceId, T11 deviceId/unitId/jobId, A14 reportId/reportVersion. reportVersion is a positive integer requiring reportId. Device/unit/job or property/space inconsistency returns VALIDATION. Decode IDs as opaque strings and authorize actual resources. URLs must not contain secrets, billing details, or unsaved form text. Remove unknown keys; show errors for invalid known values without choosing replacements. A15 quoteId must be an existing scoped quote; restored requests require the same session retaining quoteVersion. If reload has no Quote, require a new preview; do not derive versions/conditions from URL alone.

## SR12 Notifications Without Recipients

Notification still requires a real recipientMembershipId. With zero recipients, create no Notification and append one DeliveryFailure to source Alert.deliveryFailures. Deduplicate by alertId/eventId/policyId/reason; resends add none. deliveryState=failed, reason=no_recipient. Only HQ alert-policy managers see this array; other-role Alert projections use empty arrays. Include no individual recipient identifiers. No automatic redelivery in phase 1A. New business events reevaluate current recipients. Notification-only policy failures return the same ID in SR21 failureIds.

## SR13 Demo Qualifications

QualificationCode is demo_indoor/demo_outdoor/demo_electrical. Require every qualification matching ACUnit.serviceScope. JobOfferSummary.requiredQualifications returns that deduplicated set sorted by code. Job.type adds none. Initialize from fixture-contract qualificationRequirements and actors.qualifications. Grants use [validFrom,validUntil) and expire at revokedAt. members.eligible/assign includes only candidates whose qualifications remain valid for the entire scheduled work. Recheck qualifications on writes even during work; after expiry return FORBIDDEN with zero side effects. Keep assignment history. members.save has no qualification-grant UI. demo.trigger(qualification_revoked) sets membership/code revokedAt from clock and emits change. Arbitrary qualification strings return VALIDATION. Do not present these as official qualifications.

## SR14 Updates During Paging

The first read creates an immutable authorized-set snapshot; cursor binds generation/tenant/membership/scopeVersion/query/snapshotVersion/offset. Later pages keep the same set, total, values, and initial meta.snapshotAt/eventCursor despite business updates. Keep snapshots until reset. Recheck session/expiry/authorization on every page; never return expired-scope snapshots. Condition changes return VALIDATION. Reset/nonexistent snapshots return CONFLICT and restart page one. MRV old-version lists use unique version keys; others use existing deterministic query tie-breaks.

While fetching multi-page charts, defer subscription invalidation and show the completed snapshot. Combine notices into one “Updates available” indicator and refetch at most once after completion. Updates during that refetch leave a badge until the next explicit refresh. Avoid endless automatic refetch that prevents rendering. Scope/role changes immediately cancel/discard as exceptions. Mid-fetch errors must not show an incomplete chart as normal.

## SR15 A15 Permissions

A15 view/preview/simulate requires offset.manage. mrv.manage alone does not grant access. MRV and offset permissions are independent; align operation catalog and screen guards.

## SR16 Normative Document Scope

Implementation inputs include every deterministic-contracts chapter (D01–D16), every chapter here and in review-resolution-contracts, types, and all CSVs. Old guidance to stop at D11 is invalid. SR17–19 also apply as fixed contracts.

## SR17 Period Presets

User-approved 2026-09-16. today is the half-open range from 00:00 today in display timezone through completed minutes now. Floor demo clock to a UTC minute for to. 7d/30d include today as calendar days; from is 00:00 six/twenty-nine days earlier in display timezone. Use IANA calendar arithmetic, not fixed 168/720-hour subtraction. If day start does not exist, use its first valid time; if ambiguous, use the earlier time. Zone changes preserve selected UTC from/to and change labels only. Reselecting a preset recalculates with current zone/clock. Live updates do not shift the period; explicit refresh reapplies the selected preset. Custom ranges stay fixed on refresh.

If from=to in the day's first minute, show “No completed measurement interval” and do not call summaries. Zero records must not appear as zero consumption. Save timezone/range in URL and reuse saved UTC ranges on revisit. Example: Asia/Kuala_Lumpur, now=2026-09-14T01:00:45Z gives today=[2026-09-13T16:00:00Z,2026-09-14T01:00:00Z), 7d from=2026-09-07T16:00:00Z, 30d from=2026-08-15T16:00:00Z. Exclude the extra 45 seconds. Do not treat DST calendar days as fixed 24 hours.

## SR18 Offset Failure and Retry

User-approved 2026-09-16. Keep attempts in the same OffsetRecord; Repository generates each attempt ID. stage=purchase/retirement; status=pending/succeeded/failed. A request creates one purchase attempt and state=demo_requested. Successful purchase_confirm generates purchaseRef once, creates a retirement attempt, and sets demo_purchased. Successful retire creates retirementRef/demoCertificateRef, sets demo_retired, and currentAttemptId=null. Only the current pending attempt accepts success/failure events. On failed, retain that attempt as currentAttemptId; successful retry switches to a new ID. pending completedAt=null; terminal completedAt uses processing clock.

| Current state | Event | Next state and side effects |
| --- | --- | --- |
| demo_requested | purchase_confirm | Purchase succeeds; retirement pending; demo_purchased |
| demo_requested | fail | Purchase fails; failed; previousState=demo_requested |
| demo_purchased | retire | Retirement succeeds; demo_retired |
| demo_purchased | fail | Retirement fails; failed; previousState=demo_purchased; keep purchaseRef |
| failed / purchase failure | retry | New purchase attempt; demo_requested; old attempt remains failed |
| failed / retirement failure | retry | New retirement attempt; demo_purchased; purchased reference unchanged |

Retry requires new idempotencyKey, latest OffsetRecord.version, and failed currentAttemptId. Do not create a new quote, request, or purchase. Success/failure events also require attemptId/eventId. Old attempts, wrong stages, or transitions from terminal states return CONFLICT with zero side effects. Late success while failed must not change it to success. Repeating record/eventId with identical content returns stored results after authorization; different content returns CONFLICT. New events check version. Same-key retries follow D04. Duplicate retry with another key conflicts on old version or nonfailed state. Keep all attempts/event history. previousState records the actual immediately preceding state. UI shows “Retry failed stage” only in failed and identifies purchase/retirement. These are simulated trades with no real payment.

## SR19 Contract Editing During Restrictions

User-approved 2026-09-16. Contract.activeRestrictionIds derives current active restriction IDs at read time, empty for new contracts. Derive from restriction state, not contract version; restriction subscriptions invalidate contracts.list. A07 disables save when this set is nonempty or hasUnresolvedRecovery=true, offering restriction-detail navigation only to authorized users. Repository rechecks on save. contracts.save(id present) returns CONFLICT if any same-contract Restriction is scheduled/requested/applied/release_requested. Reject all edits, not only units/eligibility/rulesVersion. Grace/exemption and Command failure/expiry still count as active; change no Contract/Restriction/Command. Cancel notices, use D03 reconcile/release for applied/unknown results, then refetch contract after all related Restrictions are cancelled/released and edit with new key/latest contract version. Contract changes do not automatically cancel/release restrictions.

schedule checks expectedContractVersion against current Contract.version and saves it as Restriction.contractVersion. execute/apply retry checks saved contract version, units, eligibility, and rulesVersion; mismatch returns CONFLICT requiring confirmation. release/reconcile uses saved policy/rulesVersion so release is not blocked. contracts.save and schedule check each other's conditions in the same transaction's current state. Schedule first blocks edit; edit first blocks old-version schedule. New contract creation is outside this existing-contract edit restriction.

## SR20 Recovery Events

restored requires DeviceRecovery; other DeviceEvents have recovery=null. axis=connection/power/tamper. sourceEventId must be the current unresolved fault for the same device/axis or return CONFLICT. Connection recovery changes only connection to online; power recovery only power to on; tamper recovery only observed tamper to clear. Other axes stay unchanged. sequence increases per device/binding/axis; after history deduplication, old sequences do not affect state. Same eventId with different content returns CONFLICT. Set restoredAt only on the referenced fault. Observed tamper recovery alone does not resolve Alert; use existing human resolution.

## SR21 Notification-Only Rules and Control Conflicts

Evaluate alert and all air_quality notifications per policy/unit. alert/notify_only have no control candidate; only notify_and_ventilate adds ventilation candidates (SR25). Control remains at most one Command per tick/unit. Notification-only targets return control results suppressed/no_control_action and notifications selected/created. High-priority notification-only rules do not suppress lower control rules.

simulate returns notifications:NotificationDecision[] without saving Command/Alert/Notification. fire returns notifications:NotificationOutcome[]. Sort by unitId/policyId ascending; no target policies means empty. Suppress unmatched conditions as not_due, poor data as quality, disabled rules as disabled, and expired owners as owner_forbidden. Also suppress while cooldown is active. Zero recipients returns failed/no_recipient plus SR12 Alert failure records; valid recipients return created and notificationIds. Generate under D08 recipient/channel/cooldown rules. Uniquely create/reuse each policy event's source Alert. eventId retries keep all results without regeneration. Command failure does not undo notification success; notification failure does not undo control success.

## SR22 Offset Authorization by Branch — REREV-001

Clients may only request for their own customer and retry failed own records. Admin with offset.manage may use all events within managed scope. Client purchase_confirm/retire/fail returns FORBIDDEN; other-customer records return NOT_FOUND. Retry requires SR18 current failed attempt, latest record version, and new key; it does not finalize purchase/retirement. HQ still explicitly simulates confirmation events afterward. C13 retry buttons cover only own records; A15 only managed scope.

## SR23 T12 Alert References — REREV-002

DeviceEvent.alertIds is the authorized set of Alerts generated from that event. Link event and Alert creation in one transition. Zero gives []; multiple IDs display ascending. Flow: devices.events→select event→alerts.get for each alertId→select Alert→alerts.acknowledge(alertId, options.expectedVersion=Alert.version). Do not reuse DeviceEvent.version. Only open Alerts allow acknowledgement; acknowledged/resolved shows current state without an acknowledge-again button. Post-display conflict returns CONFLICT/refetch; expiry/deletion returns NOT_FOUND and stops that action. Response notes still use DeviceEvent version. Recovery-event alertIds inherit authorized source-fault links without auto-resolution.

## SR24 Event-Time Scope for Device History — REREV-003

Require DeviceHistoryScope on DeviceOperation/DeviceEvent/CalibrationRecord. bindingId, unitIdAtOccurrence, customerOrgIdAtOccurrence are immutable creation-time values. devices.get/list returns Device.bindingId, null before binding. DemoControls uses fetched current bindingId in event inputs. bind saves DeviceBinding, assigns new Device.bindingId, and closes old binding.unboundAt. Pre-binding events/operations save targetUnitId/customer with bindingId=null. Calibration requires binding. Reject rebinding if source/destination Unit has active Restriction, unresolved recoveryCases, or active Command/run/operation; do not change an operation's event-time ownership midway.

All history reads require current Device read permission AND current read permission for event-time Unit/customer. Technicians also require active assignment to the event-time Unit. Permission changes and relocations never rewrite history ownership. IR02 forbids moving the same Unit to another customer. If a Device is rebound to another customer's other Unit, compare customerOrgIdAtOccurrence too. Authorize rows before total/sort/Page; disclose no denied row IDs/counts/authors. HQ tenant scope may read managed history.

Project devices.get calibrationRefs/activeOperation under the same rules. Also check DeviceEvent.alertIds against each Alert's current scope. addResponseNote checks current Device and event-time scope; out-of-scope returns NOT_FOUND. Recovery targets only unresolved sourceEventId of the current binding. restored for old-binding events returns CONFLICT. DemoTrigger.device requires bindingId for an existing binding of the same Device; another Device's binding returns VALIDATION (null before binding). Events for noncurrent bindings only save history, changing no current state/Alert/Notification. Scheduled future events recheck binding at delivery. Old-binding restored is not current recovery and returns CONFLICT. New binding starts connection/power observations unknown, lastSeenAt=null; tamper must be clear before bind. Old heartbeats/observations must not make the new Unit online. Compare sequences per device/binding/axis; old-binding records do not affect current state.

## SR25 Independent Notification and Ventilation Results — REREV-004

Notification candidates are alert and all air_quality. Conditions are enabled, owner's policy permission/scope, valid metric quality, and threshold/duration. Ventilation capability, control restrictions, busy Commands, and arbitration results do not determine notification eligibility. Only notify_and_ventilate creates an extra ventilate candidate, arbitrated under D02 capabilities/restrictions/online/exclusion/priority. Notification settings/recipients follow SR28/D08.

If notification matches but ventilation is unsupported, notifications=created and control results=suppressed/invalid_capability; busy gives suppressed/busy, restricted gives suppressed/restricted. A notify_only Unit without any control candidate uses no_control_action. If another control rule wins, results shows that rule requested while air_quality notification independently shows created. If all candidates fail, use the first exclusion reason in candidate-ID order; if no candidates exist, use no_match. Return results per notification policy. simulate checks enabled/threshold/quality/recipients/channels/cooldown on the same snapshot: eligible=selected, cooldown etc.=suppressed, no recipients=failed/no_recipient. fire.created corresponds to simulate.selected. simulate saves no Alerts, failure evidence, Notifications, or Commands. fire reevaluates, so changed conditions after simulate may change results.

## SR26 Contradictory Terminal-Restriction Observations and Recovery — REREV-005

If a released/cancelled Restriction is observed again with a new device sequence, keep its terminal state. Add a pending recoveryCase on the original Restriction with unit, observed restrictionId/rulesVersion, successorRestrictionId, and evidence event. If an unresolved case for the same unit/observedRestrictionId/observedRulesVersion exists, update its latest observation and keep the old observation in audit history. A resolved case requires a new case; different old IDs on one Unit use separate cases. Create one reconciliation_required Alert. Do not add active restrictions; keep the one-per-Unit limit. Return UnitDetail.controlAvailability=blocked/reconciliation_required; ordinary manual/diagnostic/automatic control returns CONFLICT (automation: suppressed/reconciliation_required). effectiveControlPolicy keeps the successor logical policy, separate from observed old policy.

HQ with restriction.manage selects the old Restriction and calls existing restrictions.reconcile(restrictionId,unitIds) with new key/latest old-Restriction version. Besides normal authorization/exclusion, reread observations no older than 30 seconds. If observation matches old ID/rulesVersion, device is online, and no unfinished Command/run/operation exists, create remove_restriction for that old ID, set case=removing, and append Command ID to case.commandIds. If observation is null or valid successor ID, create zero Commands and set resolved. Another unknown ID returns CONFLICT; old observation TIMEOUT; offline OFFLINE, keeping pending. Never generate successor removal from an old case.

Resolve a case only after timely remove acknowledgement and confirmed removal of that old ID. Check IDs again during acknowledgement; if current observation is successor ID, preserve successor and resolve only the old case. failed/expired returns case to pending; explicit reconcile creates a new Command. Contradictory observations cancel unsent successor applies; sent applies wait for final result/30-second expiry, with no new Commands while waiting. Keep scheduled successors scheduled. For requested/applied successors, set this Unit not_applied and aggregate requested, requiring explicit retry(apply). Keep release_requested; after old-case resolution, normal reconcile determines not_required/release. Revalidate latest notice conditions before reapplication. Case resolution changes no stored Invoice/Contract fields. Only when all Unit cases resolve, return controlAvailability=available, while keeping successor effectiveControlPolicy restrictions. Confirmed successor observations update its D03 evidence; refetch before explicit apply retry to avoid duplicates.

Unresolved Unit cases block new schedule, edits to Contracts containing that Unit, and Unit archive with CONFLICT. Contract.hasUnresolvedRecovery derives true when any target Unit has unresolved cases; subscriptions invalidate contracts.list on case changes. Nonmanagement screens must not receive old-customer/restriction IDs. A07 also disables save for this flag. A09 shows recoveryCases through existing get/list and offers old-record reconcile→Command state→requery. Control screens without permission see only the reason. Keep case/Command history except on reset.

## SR27 Shared Power-State Classification — REREV-006

Use UnitSummary.effectivePowerState for both KPI and units.list(powerState). With read snapshotAt as now, map observedState.power boolean to on/off only when connection=online, observation is not future and at most 120 seconds old, and latest power Measurement is origin=measured, quality=valid, nonnull value, within D07 sensor stale limit. Otherwise unknown, including missing required power sensor/measurement. Choose candidates by observedAt desc, sequence desc, id asc; never fall back to older valid values. Do not convert energy kW readings directly to ON/OFF.

Customer/admin count powerOn/powerOff with the same classification. Connection unknown remains a separate axis. total=powerOn+powerOff+powerUnknown. Summary.counts/AdminSummary.powerUnknown means unknown operation; unknown means connection unknown/connecting/error. C01/A01 operating/stopped/unknown cards use powerOn/powerOff/powerUnknown. KPI destinations filter by this classification; paging uses SR14 snapshots. Show update times so later business updates may explain changed counts in new snapshots. Pending Commands do not overwrite observations; show requested values separately.

## SR28 AirPolicy Notification Settings — REREV-007

air_quality requires the same severity/channels/cooldownMinutes/escalateAfterMinutes inputs as alert. severity=warning/critical; channels is a nonempty unique array of inApp/email/whatsapp; both times are integers from 1–1440. New forms leave them unselected and disable save until complete. Existing-policy edits use fetched values. Neither UI nor Repository may invent required defaults.

A12 shares A05 notification settings. Choose recipientMembershipIds from the intersection of Memberships eligible for all Units/selected channels. Channel changes refetch candidates and require correction of no-longer-eligible selections. Recheck on save. On threshold match, generate configured-severity Alert/Notifications, one per recipient/channel. If only some channels lack recipients, create permitted-channel notifications and combine failures into one DeliveryFailure per policy/event. NotificationOutcome returns failed/no_recipient, successful notificationIds, and failureIds. UI says “Some notifications failed” when some succeeded, otherwise “Notification failed.” simulate also returns failed/no_recipient but saves nothing. Zero-recipient failure does not advance successful cooldown time; update policy/unit/severity time only if at least one notification was created. Keep D08 severity-increase/escalation rules. email/whatsapp are simulated previews; inApp is simulated. Cooldown ends when elapsed time since last notification creation reaches configured minutes. First notification is allowed. Escalation occurs once only under D08 unacknowledged conditions, simulating configured channels to HQ alert.policy.manage holders. No-recipient failure follows SR12.

## SR29 Saving and Comparing Baseline Quality — REREV-008

baselines.save accepts only BaselineInput, not user-supplied quality/metadata. demo_period_comparison also forbids baselineKWh input. At save, Repository calculates value, validSlots, expectedSlots, and coverage from same-snapshot D07 slots with measured origin and matching boundary under IR08/IR11. unitIds must be nonempty/unique; period must be a positive UTC-minute range up to 366 days. expectedSlots>0; coverage=validSlots/expectedSlots. Zero valid slots gives baselineKWh=null. Save quality.kind=measured and sourceSnapshot={generation,eventCursor,snapshotAt}. Late data does not recalculate saved versions. Explicit save with latest version recalculates into a new version.

demo_fixed is a demo model with user-entered finite nonnegative baselineKWh, assumptions, and source. Save quality.kind=modeled; coverage/slot counts/sourceSnapshot=null. Never disguise it as measured coverage of 100%. The 100 kWh versus 80 kWh example uses this fixed model. Label comparisons “Comparison with assumed baseline,” not measured/guaranteed reductions.

Comparison requires matching unit set/boundary/minute count, actual coverage=1, and either measured baseline coverage=1 or explicitly stated demo assumptions with nonnull modeled value. Otherwise differences/rates/cost savings/emission reductions are null; actual results may display. A baseline of 0 gives a null rate. Modeled baselines always add modeled_baseline to qualityWarnings. Copy quality into MRV.summary.baselineSnapshot and freeze old versions. Incomplete measured baselines set MRV.incomplete=true and prevent demo_reviewed. Reports with modeled baselines are demo assumptions, not verified measured MRV.

Creating/changing a recovery case increments owning Restriction.version and target Unit.version. Contract.activeRestrictionIds/hasUnresolvedRecovery are derived projections and do not change contract version. Emit contracts ChangeEvent with version=null and derived field names in changedFields to invalidate. Do not discard version=null subscription events through version comparison; recheck current scope and refetch. case.commandIds matches Commands; never reuse acknowledgements for other cases/generations.

Additional contracts for current version 0.21.0: Read IR01–106 in the [Re-review Correction Contracts](review-resolution-contracts.md). They take priority over older text on the same topic; follow IR72 for conflict precedence.
