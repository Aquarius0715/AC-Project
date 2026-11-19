"""Approval-only review suggestions with independently verified start evidence."""

from collections import Counter
from datetime import timedelta
from time import perf_counter

from .engine import START_MIN_SUPPORT, START_TOLERANCE_MINUTES
from .models import clock
from .monthly_data import Customer, local_midnight
from .monthly_engine import evaluate_customer, result, signature
from .presence import PresenceIndex
from .presence_data import Household, PresenceBundle

POWER_WINDOW = timedelta(minutes=5)


def confirmed_starts(
    data: PresenceBundle, customer: Customer
) -> tuple[frozenset[str], list[dict]]:
    samples = {
        sample.event_id: sample for sample in data.power.get(customer.device_id, ())
    }
    confirmed = set()
    excluded = []
    for day in customer.days:
        if day.status != "complete":
            continue
        for event_id, op in day.events:
            if (
                op.action != "on"
                or op.source != "web_manual"
                or op.at >= data.monthly.as_of
            ):
                continue
            sample = samples.get(event_id)
            if (
                sample is None
                or sample.before is None
                or sample.after is None
                or sample.received_at > data.monthly.as_of
                or op.at - sample.before_at > POWER_WINDOW
                or sample.after_at - op.at > POWER_WINDOW
            ):
                reason = "power_state_unknown"
            elif any(
                other_id != event_id
                and other.action in ("on", "off")
                and sample.before_at <= other.at <= sample.after_at
                for other_id, other in day.events
            ):
                reason = "intervening_power_operation"
            elif sample.before is False and sample.after is True:
                confirmed.add(event_id)
                continue
            else:
                reason = (
                    "already_running" if sample.before is True else "on_not_confirmed"
                )
            excluded.append(
                dict(event_id=event_id, at=op.at.isoformat(), reason=reason)
            )
    return frozenset(confirmed), excluded


def review_weekday(
    data: PresenceBundle,
    household: Household,
    customer: Customer,
    schedule,
    index: PresenceIndex,
) -> dict:
    monthly = data.monthly
    day = monthly.as_of.astimezone(customer.timezone).date() - timedelta(days=1)
    dates = []
    while len(dates) < 4:
        if day.weekday() == schedule.weekday:
            dates.append(day)
        day -= timedelta(days=1)
    days = {day.date: day for day in customer.days}
    comparisons = []
    for day in sorted(dates):
        historical = schedule
        versions = data.schedule_versions.get(customer.device_id, ())
        if versions:
            eligible = [
                v
                for v in versions
                if v["valid_from"] <= local_midnight(day, customer.timezone)
            ]
            historical = (
                next(
                    (
                        s
                        for s in eligible[-1]["schedules"]
                        if s.weekday == schedule.weekday
                    ),
                    None,
                )
                if eligible
                else None
            )
        start_minutes = historical.start if historical else schedule.start
        at = local_midnight(day, customer.timezone) + timedelta(minutes=start_minutes)
        end = local_midnight(day + timedelta(days=1), customer.timezone)
        record = days.get(day)
        exclusion = None
        if historical is None:
            exclusion = "no_schedule"
        elif record is None or record.status != "complete":
            exclusion = "operation_record_missing"
        elif historical.start != schedule.start:
            exclusion = "schedule_changed"
        elif any(
            period.start < end and period.end > local_midnight(day, customer.timezone)
            for period in customer.trips
        ):
            exclusion = "travel"
        elif any(op.once and op.at < monthly.as_of for _, op in record.events):
            exclusion = "one_off"
        presence = index.at(at)
        if exclusion is None and presence["state"] == "unknown":
            exclusion = "presence_unknown"
        arrival = (
            index.first_home_after(at, end)
            if not exclusion and presence["state"] == "away"
            else None
        )
        comparisons.append(
            dict(
                date=str(day),
                scheduled_at=at.isoformat(),
                state=presence["state"],
                residents=presence["residents"],
                excluded_reason=exclusion,
                confirmed_return=arrival,
            )
        )
    absent = [
        row
        for row in comparisons
        if row["excluded_reason"] is None and row["state"] == "away"
    ]
    home = [
        row
        for row in comparisons
        if row["excluded_reason"] is None and row["state"] == "home"
    ]
    unknown = [
        row for row in comparisons if row["excluded_reason"] == "presence_unknown"
    ]
    exceptions = [
        row
        for row in comparisons
        if row["excluded_reason"] is not None and row not in unknown
    ]
    proposed = len(absent) >= START_MIN_SUPPORT
    code = (
        "repeated_absence"
        if proposed
        else "presence_unknown"
        if unknown
        else "exception_excluded"
        if exceptions
        else "someone_home"
        if home
        else "absence_not_repeated"
    )
    reason = (
        f"直近4回中{len(absent)}回、{clock(schedule.start)}の開始時点で世帯全体が不在でした。開始時刻を見直しますか？"
        if proposed
        else f"不在の支持は{len(absent)}/4日。3日以上が必要です（在宅{len(home)}日、不明{len(unknown)}日、例外等{len(exceptions)}日）。"
    )
    evidence = []
    for row in comparisons:
        arrival = (
            row["confirmed_return"]["confirmed_at"]
            if row["confirmed_return"]
            else "確認なし"
        )
        evidence.append(
            f"{row['date']}: {row['scheduled_at'][11:16]}時点 {row['state']} / 帰宅確認 {arrival} / 除外 {row['excluded_reason'] or 'なし'}"
        )
    evidence.append(
        "帰宅前の予冷・遠隔運転もあり得るため、帰宅時刻だけで遅らせません。具体時刻・節約額は別の根拠がなければ表示しません。"
    )
    item = result(
        "start_review",
        schedule.weekday,
        "proposed"
        if proposed
        else "insufficient"
        if unknown or exceptions
        else "unchanged",
        reason,
        {},
        evidence,
        [e for row in comparisons for r in row["residents"] for e in r["evidence_ids"]],
        comparisons,
        len(absent),
    )
    item.update(
        reason_code=code,
        current_start=clock(schedule.start),
        comparison_days=len(comparisons),
        valid_days=len(absent) + len(home),
        absent_dates=[row["date"] for row in absent],
        excluded_dates=[
            dict(date=row["date"], reason=row["excluded_reason"])
            for row in comparisons
            if row["excluded_reason"]
        ],
        comparisons=comparisons,
        household_id=household.id,
        installation_site_id=household.installation_site_id,
        device_id=customer.device_id,
        requires_approval=True,
        applied=False,
    )
    return item


def evaluate_household(
    data: PresenceBundle,
    household: Household,
    *,
    include_intervals: bool = False,
    include_billing: bool = True,
    presence_index: PresenceIndex | None = None,
) -> dict:
    index = presence_index or PresenceIndex(household, data.monthly.as_of)
    devices = []
    customers = {customer.device_id: customer for customer in data.monthly.customers}
    for device_id in household.device_ids:
        customer = customers[device_id]
        confirmed_ids, excluded = confirmed_starts(data, customer)
        report = evaluate_customer(
            data.monthly,
            customer,
            confirmed_on_ids=confirmed_ids,
            include_intervals=include_intervals,
            include_billing=include_billing,
        )
        # A prior choice remains visible, but a concrete new start in this demo
        # additionally needs three actual off→on events near that chosen time.
        for item in report["decisions"]:
            if (
                item["kind"] not in ("start", "add_schedule")
                or item["status"] != "proposed"
            ):
                continue
            target = item["values"]["start"]
            target_minutes = int(target[:2]) * 60 + int(target[3:])
            support = {
                day.date
                for day in customer.days
                for event_id, op in day.events
                if event_id in confirmed_ids
                and event_id in item["evidence_ids"]
                and not op.once
                and op.at.astimezone(customer.timezone).weekday() == item["weekday"]
                and abs(
                    op.at.astimezone(customer.timezone).hour * 60
                    + op.at.astimezone(customer.timezone).minute
                    - target_minutes
                )
                <= START_TOLERANCE_MINUTES
                and not any(
                    period.start <= op.at < period.end for period in customer.trips
                )
            }
            if len(support) < START_MIN_SUPPORT:
                item.update(
                    status="insufficient",
                    reason="具体時刻を支えるオフ→オンの確認が3日未満です。",
                    values={},
                )
        for schedule in customer.schedules:
            review = review_weekday(data, household, customer, schedule, index)
            same = next(
                (
                    item
                    for item in report["decisions"]
                    if item["kind"] == "start"
                    and item["weekday"] == schedule.weekday
                    and item["status"] == "proposed"
                ),
                None,
            )
            if review["status"] == "proposed" and same is not None:
                same["presence_review"] = review
                same["evidence"].extend(review["evidence"])
                same["evidence_signature"] = signature(
                    dict(
                        manual=same["evidence_signature"],
                        presence=review["evidence_signature"],
                    )
                )
                review.update(
                    status="merged",
                    reason_code="merged_with_manual_start",
                    reason="同じ曜日の手動操作による開始提案へ在宅の根拠を統合しました。",
                )
            report["decisions"].append(review)
        for item in report["decisions"]:
            item.update(device_id=device_id, requires_approval=True, applied=False)
            item.setdefault("reason_code", f"{item['kind']}:{item['status']}")
        report["proposals"] = [
            item for item in report["decisions"] if item["status"] == "proposed"
        ]
        report["proposal_count"] = len(report["proposals"])
        report["power_evidence_exclusions"] = excluded
        report["confirmed_manual_on_ids"] = sorted(confirmed_ids)
        report["status"] = (
            "提案あり"
            if report["proposals"]
            else "見送り・変更不要（理由は各判定を参照）"
        )
        devices.append(report)
    return dict(
        household_id=household.id,
        installation_site_id=household.installation_site_id,
        weather_site_id=data.installations[household.installation_site_id][
            "weather_site_id"
        ],
        residents=list(household.residents),
        device_ids=list(household.device_ids),
        proposal_count=sum(device["proposal_count"] for device in devices),
        devices=devices,
        requires_approval=True,
        applied=False,
        as_of=data.monthly.as_of.isoformat(),
    )


def evaluate_cohort(data: PresenceBundle) -> dict:
    start = perf_counter()
    reports = [evaluate_household(data, household) for household in data.households]
    reasons = Counter(
        item["reason_code"]
        for report in reports
        for device in report["devices"]
        for item in device["decisions"]
        if item["status"] not in ("proposed", "merged")
    )
    proposals = Counter(
        item["kind"]
        for report in reports
        for device in report["devices"]
        for item in device["proposals"]
    )
    counts = Counter(
        "zero"
        if report["proposal_count"] == 0
        else "one"
        if report["proposal_count"] == 1
        else "multiple"
        for report in reports
    )
    return dict(
        household_count=len(reports),
        device_count=sum(len(h.device_ids) for h in data.households),
        days=(data.monthly.end_date - data.monthly.start_date).days,
        proposal_count=sum(report["proposal_count"] for report in reports),
        proposal_counts_by_kind=dict(sorted(proposals.items())),
        household_counts=dict(counts),
        nonproposal_reason_counts=dict(sorted(reasons.items())),
        evaluation_seconds=round(perf_counter() - start, 6),
        households=reports,
    )
