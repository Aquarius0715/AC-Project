import type { ReportHistorySummary, JobOfferSummary, OperationContracts, RuleInput } from '../../../02-design/service-contracts';
const noReport: ReportHistorySummary = {hasReport:false, acceptance:'not_accepted'};
const unaccepted: ReportHistorySummary = {hasReport:true, acceptance:'not_accepted'};
const accepted: ReportHistorySummary = {hasReport:true, acceptance:'accepted'};
// @ts-expect-error An absent report cannot be accepted.
const invalidState: ReportHistorySummary = {hasReport:false, acceptance:'accepted'};
// @ts-expect-error Free report text must not be returned after access expiry.
const privateReport: ReportHistorySummary = 'Site address and private contact';
const absentAddress: JobOfferSummary['siteAddress'] = null;
// @ts-expect-error Disclosure fields are not accepted in the constrained summary.
const extraPrivateField: ReportHistorySummary = {hasReport:true, acceptance:'accepted', body:'address'};
type CapabilityInput = OperationContracts['capabilities.save']['input'];
declare const baseCapability: Omit<CapabilityInput,'changeReason'>;
const createWithoutReason: CapabilityInput = baseCapability;
// @ts-expect-error Disabled reason is repository output, not rule input.
const rule: RuleInput = {name:'demo',unitIds:['unit-a'],timezone:'UTC',enabled:false,priority:50,disabledReason:'capability_changed'};

import type { ReviewAvailability, MaintenanceJob, ReportRevisionProvenance } from '../../../02-design/service-contracts';
const blockedReview: ReviewAvailability = {allowed:false, reason:'self_authored'};
const allowedReview: ReviewAvailability = {allowed:true, reason:null};
// @ts-expect-error A self-authored report cannot be advertised as reviewable.
const contradictoryReview: ReviewAvailability = {allowed:true, reason:'self_authored'};
const unfinishedJob: MaintenanceJob['completedAt'] = null;
const provenance: ReportRevisionProvenance = {reportId:'report-a', reportVersion:2, contributorUserIds:['user-t1','user-t2']};
