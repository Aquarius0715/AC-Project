"""Advance knowledge one day at a time; proposal episodes and responses are separate."""

import copy
from collections import Counter
from datetime import date, timedelta
from time import perf_counter

from .dataset import Dataset
from .loader import InputError, minute, timestamp
from .models import clock
from .monthly_data import local_midnight
from .monthly_engine import signature
from .notifications import NotificationPlanner
from .presence import PresenceIndex
from .presence_proposals import evaluate_household
from .weekday_cards import (
    apply_fixed_weekdays,
    build_cards,
    known_card_feedback,
    same_start_choice,
    validate_response_references,
)

NEW_EVENTS = {"first", "changed", "reappeared"}
START_DISPLAY_TOLERANCE_MINUTES = 15


def stabilize_start(item, previous, tolerance_minutes=START_DISPLAY_TOLERANCE_MINUTES):
    """Keep the episode's first displayed value, never the previous day's candidate."""
    if item["kind"] != "start" or item["status"] != "proposed":
        return
    candidate = copy.deepcopy(item["values"])
    item["candidate_values"] = candidate
    item["candidate_reason"] = item["reason"]
    reference = previous["values"] if previous else candidate
    delta = minute(candidate["start"], "start") - minute(reference["start"], "start")
    retained = bool(
        previous
        and abs(delta) <= tolerance_minutes
        and {k: v for k, v in candidate.items() if k != "start"}
        == {k: v for k, v in reference.items() if k != "start"}
        and previous.get("current_start") == item.get("current_start")
    )
    item["stabilization"] = dict(
        reference_start=reference["start"],
        candidate_start=candidate["start"],
        delta_minutes=delta,
        tolerance_minutes=tolerance_minutes,
        retained=retained,
    )
    if retained:
        item["values"] = copy.deepcopy(reference)
        item["proposal_id"] = previous["proposal_id"]
        item["first_displayed_at"] = previous["first_displayed_at"]
        if candidate != reference:
            item["reason"] = (
                f"毎日の計算候補は{candidate['start']}（支持{item['support_count']}日）。"
                f"初回表示{reference['start']}との差が{tolerance_minutes}分以内のため同じ提案を継続。"
                f"表示・承認対象は{reference['start']}開始、終了{reference['end']}、"
                f"設定{reference['temperature_c']}℃を維持します。"
            )


def proposal_key(item):
    return (
        item["device_id"],
        item["kind"],
        item["weekday"],
        item["values"].get("base_temperature_c"),
        item.get("target_date") if item["kind"] == "precool" else None,
    )


def content(item):
    return signature(
        dict(values=item["values"], current_start=item.get("current_start"))
    )


def known_feedback(raw, at):
    known = []
    for original in raw["feedback"]:
        proposed = timestamp(original["proposed_at"], "feedback.proposed_at")
        responded = (
            timestamp(original["responded_at"], "feedback.responded_at")
            if original.get("responded_at")
            else None
        )
        if responded and responded < proposed:
            raise InputError("反応日時は提案日時以降が必要です")
        if original["response"] not in (
            "approved",
            "edited",
            "declined",
            "no_response",
        ):
            raise InputError("不明な反応です")
        if proposed > at:
            continue
        row = copy.deepcopy(original)
        if responded is None or responded > at:
            row.update(response="no_response", responded_at=None, selected={})
        known.append(row)
    return known + known_card_feedback(raw, at)


def apply_feedback(report, raw, at):
    known = known_feedback(raw, at)
    for device in report["devices"]:
        schedules = {s["weekday"]: s for s in device["current_schedules"]}
        for item in device["decisions"]:
            if item["kind"] == "start_review" and item["status"] == "merged":
                start = next(
                    (
                        d
                        for d in device["decisions"]
                        if d["kind"] == "start" and d["weekday"] == item["weekday"]
                    ),
                    None,
                )
                if start and start["status"] != "proposed":
                    if start["status"] == "suppressed":
                        item.update(
                            status="suppressed",
                            reason="同じ根拠を含む開始変更が見送り済みのため抑制しました。",
                        )
                    else:
                        item.update(
                            status="proposed",
                            reason_code="repeated_absence",
                            reason=f"直近4回中{item['support_count']}回、{item['current_start']}時点で不在でした。開始時刻を見直しますか？",
                        )
            matching = [
                f
                for f in known
                if f["device_id"] == item["device_id"]
                and f["kind"] == item["kind"]
                and f["weekday"] == item["weekday"]
            ]
            explicit = [f for f in matching if f["response"] in ("approved", "edited")]
            explicit = [
                f
                for f in explicit
                if not (
                    f.get("origin") == "card_response"
                    and item["kind"] == "start"
                    and not same_start_choice(item["values"], f["values"])
                )
            ]
            if explicit:
                chosen = max(explicit, key=lambda f: f["responded_at"])
                values = dict(chosen["values"], **chosen.get("selected", {}))
                item["judgment_influence"].append(
                    f"{chosen['id']}: {chosen['response']} / 選択値 {values}"
                )
                if item["kind"] in ("start", "add_schedule", "temperature"):
                    current = schedules.get(item["weekday"])
                    if current and all(
                        str(current.get(k)) == str(v)
                        for k, v in values.items()
                        if k in ("start", "end", "temperature_c")
                    ):
                        item.update(
                            status="approved",
                            reason="承認・修正して承認した内容が現在の予定に反映されています。",
                        )
                    elif item["status"] == "proposed" and values != item["values"]:
                        if values == item.get("candidate_values"):
                            # A supported explicit edit takes precedence over smoothing.
                            item["values"] = copy.deepcopy(values)
                            item["reason"] = item["candidate_reason"]
                            item["stabilization"].update(
                                retained=False, override="explicit_choice"
                            )
                            item.pop("proposal_id", None)
                            item.pop("first_displayed_at", None)
                        else:
                            # Do not invent power/comfort predictions for edited values.
                            item.update(
                                status="insufficient",
                                reason="明示的な選択値を優先しますが、その値の運転根拠・候補予測が不足しています。",
                                requested_values=values,
                            )
                elif (
                    item["kind"] == "start_review"
                    and chosen["evidence_signature"] == item["evidence_signature"]
                    and item["status"] == "proposed"
                ):
                    item.update(
                        status="approved",
                        reason="同じ根拠の見直しは承認済みです。予定は入力の有効な版を参照します。",
                    )
            for feedback in matching:
                if feedback["response"] == "no_response":
                    item["judgment_influence"].append(
                        f"{feedback['id']}: 無反応は意思不明"
                    )
                if feedback["response"] == "excluded_once":
                    item["judgment_influence"].append(
                        f"{feedback['id']}: 今回の承認だけ対象外。固定・拒否はしません。"
                    )
                if feedback["response"] == "declined":
                    item["judgment_influence"].append(
                        f"{feedback['id']}: 見送りの参照内容 {feedback['values']}"
                    )
                    if (
                        explicit
                        and max(f["responded_at"] for f in explicit)
                        >= feedback["responded_at"]
                    ):
                        continue
                    if (
                        same_start_choice(feedback["values"], item["values"])
                        if item["kind"] == "start"
                        else feedback["evidence_signature"]
                        == item["evidence_signature"]
                    ) and item["status"] == "proposed":
                        item.update(
                            status="suppressed",
                            reason="この曜日・候補は見送り済みです。支持日の追加やカード構成の変化だけでは再提示しません。"
                            if item["kind"] == "start"
                            else "同じ根拠で見送り済みのため再提案を抑制しました。",
                        )
                    else:
                        item["judgment_influence"].append(
                            f"{feedback['id']}: 同じ根拠ではないため現在の情報で再評価"
                        )
            item["reason_code"] = (
                f"{item['kind']}:{item['status']}" if matching else item["reason_code"]
            )
        device["proposals"] = [
            i for i in device["decisions"] if i["status"] == "proposed"
        ]
        device["proposal_count"] = len(device["proposals"])
    report["proposal_count"] = sum(d["proposal_count"] for d in report["devices"])
    return known


def day_context(prepared, snapshot, target, at):
    source = prepared.dataset.customers[prepared.household_id]
    devices = []
    for device, parsed in zip(source["devices"], snapshot.monthly.customers):
        day = next(d for d in parsed.days if d.date == target)
        visible = prepared.availability[(parsed.device_id, str(target))] <= at
        prices = snapshot.monthly.tariffs[parsed.tariff_id]
        intervals = [
            dict(
                time=f"{i // 2:02d}:{i % 2 * 30:02d}",
                kwh=str(kwh) if visible and kwh is not None else None,
                jpy_per_kwh=str(price) if price is not None else None,
                jpy=str(kwh * price)
                if visible and kwh is not None and price is not None
                else None,
            )
            for i, (kwh, price) in enumerate(zip(day.kwh, prices))
        ]
        versions = [
            v
            for v in prepared.versions[parsed.device_id]
            if v["recorded_at"] <= at
            and v["valid_from"] <= local_midnight(target, parsed.timezone)
        ]
        old = versions[-1]["schedules"] if versions else ()
        operations = [
            copy.deepcopy(o)
            for d in device["days"]
            if d["date"] == str(target)
            for o in d["operations"]
            if timestamp(o["at"], "at") <= at
            and timestamp(o["received_at"], "received_at") <= at
        ]
        for operation in operations:
            if (
                operation.get("power")
                and timestamp(operation["power"]["received_at"], "power.received_at")
                > at
            ):
                operation["power"] = None
                operation["power_availability"] = "判定時点では未受信"
        weather = snapshot.monthly.sites[parsed.site_id]["observations"].get(target)
        devices.append(
            dict(
                device_id=parsed.device_id,
                operations=operations,
                intervals=intervals,
                weather=weather,
                observed_schedules=[
                    dict(
                        weekday=s.weekday,
                        start=clock(s.start),
                        end=clock(s.end),
                        temperature_c=str(s.temperature),
                    )
                    for s in old
                ],
            )
        )
    observations = [
        o
        for o in source["presence"]
        if timestamp(o["observed_at"], "observed_at")
        .astimezone(snapshot.monthly.customers[0].timezone)
        .date()
        == target
        and timestamp(o["observed_at"], "observed_at") <= at
        and timestamp(o["received_at"], "received_at") <= at
    ]
    return dict(devices=devices, presence=observations)


def replay_customer(
    dataset: Dataset,
    household_id: str,
    mode="daily",
    start_tolerance_minutes=START_DISPLAY_TOLERANCE_MINUTES,
    presentation="grouped-weekly",
):
    if start_tolerance_minutes not in (0, START_DISPLAY_TOLERANCE_MINUTES):
        raise InputError(
            "開始提案の継続幅は0分（従来比較）または15分を指定してください"
        )
    if presentation not in ("grouped-weekly", "individual-daily"):
        raise InputError("表示・通知方式が不正です")
    if mode == "snapshot" and dataset.customers[household_id].get("card_responses"):
        # A card response references a historical displayed ID. Validate it by
        # replaying its original display, then expose just the final decision.
        final = replay_customer(
            dataset, household_id, "daily", start_tolerance_minutes, presentation
        )
        row = final["days"][-1]
        new = [e for e in row["events"] if e["is_new"]]
        final.update(
            mode="snapshot",
            days=[row],
            new_proposal_count=len(new),
            new_by_kind=dict(Counter(e["kind"] for e in new)),
            first_proposal_at=row["decision_at"] if new else None,
            notification_count=int(row["notification"]["scheduled"]),
            notification_target_count=len(row["notification"]["targets"]),
        )
        for total, daily in (
            ("eligible_start_day_count", "eligible_start_count"),
            ("start_weekday_day_count", "start_weekday_count"),
            ("card_day_count", "card_count"),
            ("start_card_day_count", "start_card_count"),
        ):
            final[total] = row[daily]
        return final
    prepared = dataset.prepare(household_id)
    raw = dataset.customers[household_id]
    start = date.fromisoformat(dataset.config["start_date"])
    end = date.fromisoformat(dataset.config["end_date"])
    targets = (
        [end - timedelta(days=1)]
        if mode == "snapshot"
        else [start + timedelta(days=i) for i in range((end - start).days)]
    )
    # Historical queries use received_at <= query. Reusing completed-day caches
    # is safe: later received observations cannot alter a past query's inputs.
    index = PresenceIndex(prepared.full.households[0], prepared.full.monthly.as_of)
    active = {}
    seen = set()
    last_content = {}
    serial = 0
    rows = []
    ledger = {}
    planner = NotificationPlanner(presentation)
    previous_at = local_midnight(start, prepared.full.monthly.customers[0].timezone)
    for target in targets:
        at = local_midnight(
            target + timedelta(days=1), prepared.full.monthly.customers[0].timezone
        ) + timedelta(hours=12)
        snapshot = prepared.snapshot(at)
        validate_response_references(raw, at, ledger)
        report = evaluate_household(
            snapshot,
            snapshot.households[0],
            include_billing=False,
            presence_index=index,
        )
        # Feedback must inspect the same values that the ID and HTML refer to.
        for device in report["devices"]:
            for item in device["decisions"]:
                stabilize_start(
                    item, active.get(proposal_key(item)), start_tolerance_minutes
                )
        eligible_start_count = sum(
            i["kind"] == "start" and i["status"] == "proposed"
            for d in report["devices"]
            for i in d["decisions"]
        )
        known = apply_feedback(report, raw, at)
        controls = apply_fixed_weekdays(report, raw, at)
        for device in report["devices"]:
            for item in device["decisions"]:
                if item["kind"] == "precool" and item.get("estimate"):
                    item["target_date"] = item["estimate"]["required_at"][:10]
        next_active = {}
        events = []
        for device in report["devices"]:
            for item in device["proposals"]:
                key = proposal_key(item)
                value = content(item)
                previous = active.get(key)
                state = (
                    "continued"
                    if previous and previous["content"] == value
                    else "changed"
                    if previous
                    else ("reappeared" if last_content[key] == value else "changed")
                    if key in seen
                    else "first"
                )
                if state in NEW_EVENTS:
                    serial += 1
                proposal_id = (
                    previous["proposal_id"]
                    if state == "continued"
                    else f"{household_id}-P{serial:04d}"
                )
                item["proposal_id"] = proposal_id
                first_displayed_at = (
                    previous["first_displayed_at"]
                    if state == "continued"
                    else at.isoformat()
                )
                item["first_displayed_at"] = first_displayed_at
                next_active[key] = dict(
                    content=value,
                    proposal_id=proposal_id,
                    kind=item["kind"],
                    weekday=item["weekday"],
                    device_id=item["device_id"],
                    values=item["values"],
                    current_start=item.get("current_start"),
                    first_displayed_at=first_displayed_at,
                    target_date=item.get("target_date"),
                )
                events.append(
                    dict(
                        next_active[key],
                        event=state,
                        is_new=state in NEW_EVENTS,
                        evidence_ids=item["evidence_ids"],
                        evidence_signature=item["evidence_signature"],
                        support_count=item["support_count"],
                        **{
                            k: copy.deepcopy(item[k])
                            for k in ("candidate_values", "stabilization")
                            if k in item
                        },
                    )
                )
                seen.add(key)
                last_content[key] = value
        for key, previous in active.items():
            if key not in next_active:
                match = next(
                    (
                        d
                        for dev in report["devices"]
                        for d in dev["decisions"]
                        if proposal_key(d) == key
                    ),
                    None,
                )
                events.append(
                    dict(
                        previous,
                        event="ended",
                        is_new=False,
                        reason=match["reason"]
                        if match
                        else "条件を満たす提案がなくなりました",
                    )
                )
        responses = [
            f
            for f in known
            if f.get("responded_at")
            and previous_at < timestamp(f["responded_at"], "responded_at") <= at
        ]
        canonical_cards = build_cards(report, household_id)
        cards = (
            canonical_cards
            if presentation == "grouped-weekly"
            else build_cards(report, household_id, grouped=False)
        )
        for card in canonical_cards:
            ledger[(at, card["card_id"])] = copy.deepcopy(card["members"])
        notification = planner.evaluate(at, cards, events, known)
        for device in report["devices"]:
            device.pop("billing", None)
            device.pop(
                "proposals", None
            )  # Status=proposed in decisions already contains these exact objects.
            device.pop("confirmed_manual_on_ids", None)
            exclusions = device.pop("power_evidence_exclusions", [])
            device["power_exclusion_counts"] = dict(
                Counter(e["reason"] for e in exclusions)
            )
            for item in device["decisions"]:
                item.pop(
                    "presence_review", None
                )  # The standalone merged decision contains the same evidence.
        rows.append(
            dict(
                target_date=str(target),
                decision_at=at.isoformat(),
                active_count=report["proposal_count"],
                new_count=sum(e["is_new"] for e in events),
                events=events,
                responses=responses,
                devices=report["devices"],
                context=day_context(prepared, snapshot, target, at),
                cards=cards,
                card_count=len(cards),
                start_card_count=sum(c["kind"] == "start" for c in cards),
                eligible_start_count=eligible_start_count,
                start_weekday_count=sum(
                    len(c["members"]) for c in cards if c["kind"] == "start"
                ),
                start_controls=controls,
                approval_targets=[
                    dict(
                        device_id=f["device_id"],
                        weekday=f["weekday"],
                        proposal_id=f.get("proposal_id"),
                        values=dict(f["values"], **f.get("selected", {})),
                    )
                    for f in responses
                    if f["response"] in ("approved", "edited")
                ],
                notification=notification,
            )
        )
        active = next_active
        previous_at = at
    new = [
        dict(e, decision_at=r["decision_at"])
        for r in rows
        for e in r["events"]
        if e["is_new"]
    ]
    last_reasons = Counter(
        d["reason_code"]
        for dev in rows[-1]["devices"]
        for d in dev["decisions"]
        if d["status"] not in ("proposed", "merged")
    )
    return dict(
        household_id=household_id,
        start_tolerance_minutes=start_tolerance_minutes,
        presentation=presentation,
        installation=raw["installation"],
        residents=raw["residents"],
        mode=mode,
        new_proposal_count=len(new),
        eligible_start_day_count=sum(r["eligible_start_count"] for r in rows),
        start_weekday_day_count=sum(r["start_weekday_count"] for r in rows),
        card_day_count=sum(r["card_count"] for r in rows),
        start_card_day_count=sum(r["start_card_count"] for r in rows),
        notification_count=sum(r["notification"]["scheduled"] for r in rows),
        notification_target_count=sum(len(r["notification"]["targets"]) for r in rows),
        new_by_kind=dict(Counter(e["kind"] for e in new)),
        first_proposal_at=new[0]["decision_at"] if new else None,
        last_reason_counts=dict(last_reasons),
        input_path=str(dataset.path / "customers" / f"{household_id}.json"),
        days=rows,
    )


def replay_dataset(
    dataset,
    customers=None,
    mode="daily",
    start_tolerance_minutes=START_DISPLAY_TOLERANCE_MINUTES,
    presentation="grouped-weekly",
):
    started = perf_counter()
    reports = []
    for hid in customers or dataset.customers:
        reports.append(
            replay_customer(dataset, hid, mode, start_tolerance_minutes, presentation)
        )
        print(
            f"評価済み: {hid} / 新規 {reports[-1]['new_proposal_count']}件", flush=True
        )
    reasons = Counter(
        k
        for r in reports
        for day in r["days"]
        for dev in day["devices"]
        for d in dev["decisions"]
        if d["status"] not in ("proposed", "merged")
        for k in [d["reason_code"]]
    )
    return dict(
        customer_count=len(reports),
        day_count=len(reports[0]["days"]),
        mode=mode,
        presentation=presentation,
        start_tolerance_minutes=start_tolerance_minutes,
        new_proposal_count=sum(r["new_proposal_count"] for r in reports),
        **{
            k: sum(r[k] for r in reports)
            for k in (
                "eligible_start_day_count",
                "start_weekday_day_count",
                "card_day_count",
                "start_card_day_count",
                "notification_count",
                "notification_target_count",
            )
        },
        new_by_kind=dict(sum((Counter(r["new_by_kind"]) for r in reports), Counter())),
        decision_day_reason_counts=dict(reasons),
        evaluation_seconds=round(perf_counter() - started, 3),
        customers=reports,
    )
