"""Monthly evaluation: shared rules, scoped evidence, explicit user choices, no I/O."""

import hashlib
import json
from collections import Counter
from dataclasses import asdict, replace
from datetime import timedelta
from decimal import Decimal

from .engine import (
    HOT_THRESHOLD_C,
    START_MIN_SUPPORT,
    START_TOLERANCE_MINUTES,
    TEMPERATURE_MIN_DAYS,
    energy_decision,
    matching_weather,
    money,
    plan_cost,
    start_pattern,
    temperature_decision,
    usable_operations,
)
from .loader import InputError, energy_comparison, minute, timestamp
from .models import Scenario, Schedule, Weather, clock
from .monthly_data import Bundle, Customer, calendar_date, local_midnight

WEEKDAYS = "月火水木金土日"
PREPARATION_DAYS = 7


def signature(value: object) -> str:
    payload = json.dumps(
        value, ensure_ascii=False, sort_keys=True, default=str, separators=(",", ":")
    )
    return hashlib.sha256(payload.encode()).hexdigest()


def available_days(bundle: Bundle, customer: Customer) -> tuple:
    return tuple(
        day
        for day in customer.days
        if local_midnight(day.date + timedelta(days=1), customer.timezone)
        <= bundle.as_of
    )


def base_scenario(bundle: Bundle, customer: Customer, schedule: Schedule) -> Scenario:
    days = available_days(bundle, customer)
    operations = tuple(
        op
        for day in days
        if day.status == "complete"
        for _, op in day.events
        if customer.enrolled_at <= op.at < bundle.as_of
    )
    weather = []
    for day, row in bundle.sites[customer.site_id]["observations"].items():
        if row["issued_at"] > bundle.as_of:
            continue
        for index, celsius in enumerate(row["outdoor_c"]):
            at = local_midnight(day, customer.timezone) + timedelta(minutes=index * 30)
            if celsius is not None and at < bundle.as_of:
                weather.append(Weather(customer.device_id, at, celsius))
    return Scenario(
        customer.id,
        customer.description,
        customer.device_id,
        customer.timezone,
        bundle.as_of,
        max(local_midnight(bundle.start_date, customer.timezone), customer.enrolled_at),
        customer.enrolled_at,
        (),
        schedule,
        operations,
        customer.trips,
        tuple(weather),
    )


def result(
    kind: str,
    weekday: int | None,
    status: str,
    reason: str,
    values: dict,
    evidence: list,
    evidence_ids: list,
    basis: object,
    support: int = 0,
) -> dict:
    return dict(
        kind=kind,
        weekday=weekday,
        status=status,
        reason=reason,
        values=values,
        evidence=evidence,
        evidence_ids=sorted(evidence_ids),
        support_count=support,
        basis_signature=signature(basis),
        evidence_signature=signature(
            dict(
                kind=kind,
                weekday=weekday,
                values=values,
                basis_signature=signature(basis),
            )
        ),
        judgment_influence=[],
        estimate=None,
    )


def judgments_as_of(bundle: Bundle, customer: Customer) -> list[dict]:
    known = []
    for judgment in customer.judgments:
        if judgment["proposed_at"] > bundle.as_of:
            continue
        item = dict(judgment)
        if item["responded_at"] is not None and item["responded_at"] > bundle.as_of:
            item.update(response="no_response", responded_at=None, selected={})
        known.append(item)
    return known


def apply_judgments(item: dict, current: dict | None, known: list[dict]) -> dict:
    matching = [
        j
        for j in known
        if j["kind"] == item["kind"]
        and j["weekday"] == item["weekday"]
        and j["values"].get("base_temperature_c")
        == item["values"].get("base_temperature_c")
    ]
    explicit = [j for j in matching if j["response"] in ("approved", "edited")]
    latest_explicit_at = None
    if explicit:
        latest = max(explicit, key=lambda j: (j["responded_at"], j["id"]))
        latest_explicit_at = latest["responded_at"]
        selected = dict(latest["values"])
        selected.update(latest["selected"] if latest["response"] == "edited" else {})
        # These are user-selected values, not additional votes in operation history.
        item["judgment_influence"].append(
            f"{latest['id']}: {'修正して承認' if latest['response'] == 'edited' else '承認'}した値を明示的な希望として優先: {selected}"
        )
        if item["kind"] in ("start", "add_schedule", "temperature"):
            item["values"].update(selected)
            item["status"] = (
                "unchanged"
                if current is not None
                and all(str(current.get(k)) == str(v) for k, v in selected.items())
                else "proposed"
            )
            item["reason"] = (
                "過去に明示的に選んだ値に基づく提案です。"
                if item["status"] == "proposed"
                else "現行予定が過去に明示的に選んだ値と一致しています。"
            )
            if item["kind"] in ("start", "add_schedule") and minute(
                item["values"]["start"], "selected.start"
            ) >= minute(item["values"]["end"], "selected.end"):
                item.update(
                    status="rejected",
                    reason="明示的な選択値の開始が終了以降になるため見送り。",
                )
        else:
            item["judgment_influence"].append(
                "予冷の承認履歴があっても、今回の費用と快適性の条件を満たすことが必要です"
            )
    item["evidence_signature"] = signature(
        dict(
            kind=item["kind"],
            weekday=item["weekday"],
            values=item["values"],
            basis_signature=item["basis_signature"],
        )
    )
    for judgment in matching:
        if judgment["response"] == "no_response":
            item["judgment_influence"].append(
                f"{judgment['id']}: 無反応は意思不明。支持・拒否に数えません"
            )
        elif judgment["response"] == "declined":
            if (
                latest_explicit_at is not None
                and judgment["responded_at"] <= latest_explicit_at
            ):
                item["judgment_influence"].append(
                    f"{judgment['id']}: 後の明示的な承認を優先"
                )
                continue
            if (
                judgment["evidence_signature"] == item["evidence_signature"]
                and item["status"] == "proposed"
            ):
                item["status"] = "suppressed"
                item["reason"] = "同じ根拠で見送られた提案のため再提案を抑制しました。"
                item["judgment_influence"].append(f"{judgment['id']}: 根拠署名が一致")
            else:
                item["judgment_influence"].append(
                    f"{judgment['id']}: 根拠が変わったため現在のデータで再評価（以前の見送りを支持票にはしません）"
                )
    if not matching:
        item["judgment_influence"].append(
            "対応する判断履歴なし。手動操作などの根拠だけで判定"
        )
    return item


def schedule_results(
    bundle: Bundle,
    customer: Customer,
    known: list[dict],
    confirmed_on_ids: frozenset[str] | None = None,
) -> list[dict]:
    schedules = {plan.weekday: plan for plan in customer.schedules}
    days = {day.date: day for day in available_days(bundle, customer)}
    results = []
    base = base_scenario(bundle, customer, Schedule(0, 0, 1439, Decimal(24)))
    if confirmed_on_ids is not None:
        permitted = {
            op
            for day in days.values()
            for event_id, op in day.events
            if op.action != "on"
            or op.source != "web_manual"
            or event_id in confirmed_on_ids
        }
        base = replace(
            base, operations=tuple(op for op in base.operations if op in permitted)
        )
    for weekday in range(7):
        current = schedules.get(weekday)
        scenario = replace(
            base, schedule=current or Schedule(weekday, 0, 1439, Decimal(24))
        )
        candidate, support, votes = start_pattern(scenario)
        allowed_dates = {day for day, _ in votes}
        eligible = usable_operations(scenario)
        ids = [
            event_id
            for day in days.values()
            if day.date in allowed_dates
            for event_id, op in day.events
            if op in eligible and op.action == "on"
        ]
        evidence = [
            f"{day}: {clock(time) if time is not None else '有効なWeb手動オンなし'} / "
            f"{days[day].status + ':' + days[day].usage if day in days else '観測なし'}"
            for day, time in votes
        ]
        basis = [
            dict(
                date=str(day),
                status=days[day].status if day in days else "missing",
                usage=days[day].usage if day in days else "unknown",
                events=[
                    (event_id, asdict(op))
                    for event_id, op in days[day].events
                    if op in eligible and op.action == "on"
                ]
                if day in days
                else [],
            )
            for day, _ in votes
        ]
        known_days = sum(
            day in days and days[day].status == "complete" for day, _ in votes
        )
        values = dict(start=clock(candidate)) if support >= START_MIN_SUPPORT else {}
        if current is not None:
            status = (
                "proposed"
                if support >= START_MIN_SUPPORT
                and candidate != current.start
                and candidate < current.end
                else "unchanged"
            )
            if support < START_MIN_SUPPORT and not (
                support == 0 and known_days >= START_MIN_SUPPORT
            ):
                status = "insufficient"
            reason = (
                f"直近4回中{support}回が{clock(candidate)}±15分。終了{clock(current.end)}を維持。"
                if status == "proposed"
                else "現行予定と開始の傾向が一致、または変更の必要を示す手動操作がありません。"
                if status == "unchanged"
                else "曜日別の有効な手動オンが不足。直近4回中3回の根拠が必要です。"
            )
            if support >= START_MIN_SUPPORT and candidate >= current.end:
                status = "rejected"
                reason = "開始候補が現行の終了以降になるため見送り。"
            manual_days = sum(time is not None for _, time in votes)
            if status == "insufficient" and manual_days >= START_MIN_SUPPORT:
                reason = (
                    f"手動オンの履歴は{manual_days}日ありますが、開始時刻が分散しています。"
                    f"候補±15分の支持は{support}日で、直近4回中3回の条件を満たしません。"
                )
            current_values = dict(
                start=clock(current.start),
                end=clock(current.end),
                temperature_c=str(current.temperature),
            )
            values.update(
                end=clock(current.end), temperature_c=str(current.temperature)
            )
            item = result(
                "start", weekday, status, reason, values, evidence, ids, basis, support
            )
        else:
            # Infer a complete initial schedule only when start, last manual off,
            # and the on-event's temperature have three common supporting dates.
            last_off = {}
            first_on = {}
            for op in eligible:
                day = op.at.astimezone(customer.timezone).date()
                if day not in allowed_dates:
                    continue
                if op.action == "off":
                    last_off[day] = replace(op, action="on")
                elif op.action == "on":
                    first_on.setdefault(day, op)
            end, end_support, _ = start_pattern(
                replace(scenario, operations=tuple(last_off.values()))
            )
            joint = [
                day
                for day, time in votes
                if time is not None
                and abs(time - candidate) <= START_TOLERANCE_MINUTES
                and day in last_off
                and last_off[day].at > first_on[day].at
                and abs(
                    last_off[day].at.astimezone(customer.timezone).hour * 60
                    + last_off[day].at.astimezone(customer.timezone).minute
                    - end
                )
                <= START_TOLERANCE_MINUTES
            ]
            temps = Counter(
                first_on[day].after for day in joint if first_on[day].after is not None
            )
            temp, temp_support = (
                max(temps.items(), key=lambda pair: (pair[1], pair[0]))
                if temps
                else (None, 0)
            )
            ready = (
                min(support, end_support, len(joint), temp_support) >= START_MIN_SUPPORT
                and candidate < end
            )
            unused = sum(
                day in days and days[day].usage == "unused" for day, _ in votes
            )
            status = (
                "proposed"
                if ready
                else "unchanged"
                if unused >= START_MIN_SUPPORT
                else "insufficient"
            )
            values = (
                dict(start=clock(candidate), end=clock(end), temperature_c=str(temp))
                if ready
                else {}
            )
            reason = (
                f"同じ{temp_support}日以上で手動オン・最終オフ・温度が対応。初回の曜日別予定を提案。"
                if ready
                else "未利用が確認された曜日のため予定追加は不要です。"
                if unused >= START_MIN_SUPPORT
                else "初回予定には開始・終了・温度の共通する3日以上の根拠が必要です。"
            )
            off_ids = [
                event_id
                for day in days.values()
                if day.date in allowed_dates
                for event_id, op in day.events
                if op in eligible and op.action == "off"
            ]
            basis.append(dict(off=[asdict(op) for op in last_off.values()]))
            evidence.append(
                f"開始支持{support}日 / 終了支持{end_support}日 / 共通{len(joint)}日 / 温度支持{temp_support}日"
            )
            item = result(
                "add_schedule",
                weekday,
                status,
                reason,
                values,
                evidence,
                ids + off_ids,
                basis,
                support,
            )
            current_values = None
        results.append(apply_judgments(item, current_values, known))
    return results


def temperature_results(
    bundle: Bundle, customer: Customer, known: list[dict]
) -> list[dict]:
    results = []
    groups = sorted({plan.temperature for plan in customer.schedules})
    for base in groups:
        weekdays = {
            plan.weekday for plan in customer.schedules if plan.temperature == base
        }
        scenario = base_scenario(
            bundle, customer, Schedule(min(weekdays), 0, 1439, base)
        )
        scenario = replace(
            scenario,
            operations=tuple(
                op
                for op in scenario.operations
                if op.at.astimezone(customer.timezone).weekday() in weekdays
            ),
        )
        outcome = temperature_decision(scenario)
        eligible = usable_operations(scenario)
        ops = [op for op in eligible if op.action == "temperature"]
        ids = [
            event_id
            for day in available_days(bundle, customer)
            for event_id, op in day.events
            if op in ops
        ]
        # Only weather near the actual manual adjustments is relevant evidence.
        basis = [
            dict(
                operation=asdict(op),
                weather=asdict(sample)
                if (sample := matching_weather(scenario, op))
                else None,
            )
            for op in ops
        ]
        last_ops = {op.at.astimezone(customer.timezone).date(): op for op in ops}
        measured = [
            op
            for op in last_ops.values()
            if op.before is not None
            and op.after is not None
            and matching_weather(scenario, op) is not None
        ]
        status = (
            "proposed"
            if outcome.proposed
            else "rejected"
            if len(measured) >= TEMPERATURE_MIN_DAYS
            else "insufficient"
            if ops
            else "unchanged"
        )
        values = dict(base_temperature_c=str(base), condition="outdoor_c>=32")
        if outcome.target_temperature_c:
            values["temperature_c"] = outcome.target_temperature_c
        evidence = list(outcome.evidence)
        evidence.append(
            "適用する予定の曜日: "
            + "・".join(WEEKDAYS[weekday] for weekday in sorted(weekdays))
        )
        support = len(
            {
                op.at.astimezone(customer.timezone).date()
                for op in last_ops.values()
                if (sample := matching_weather(scenario, op)) is not None
                and sample.outdoor_c >= HOT_THRESHOLD_C
                and op.before == base
                and str(op.after) == outcome.target_temperature_c
            }
        )
        item = result(
            "temperature",
            None,
            status,
            outcome.reason if ops else "暑い日の温度変更を示す手動操作がありません。",
            values,
            evidence,
            ids,
            dict(weekdays=sorted(weekdays), operations=basis),
            support,
        )
        item["applicable_weekdays"] = sorted(weekdays)
        current_values = dict(
            base_temperature_c=str(base),
            condition="outdoor_c>=32",
            temperature_c=str(base),
        )
        results.append(apply_judgments(item, current_values, known))
    return results


def precool_results(
    bundle: Bundle, customer: Customer, heat: list[dict], known: list[dict]
) -> list[dict]:
    results = []
    if not customer.predictions:
        return [
            result(
                "precool",
                None,
                "unavailable",
                "今後の候補別の消費電力量・室温予測が未提供です。",
                {},
                [],
                [],
                None,
            )
        ]
    for raw in customer.predictions:
        day_text = raw.get("date")
        day = calendar_date(day_text, "prediction.date")
        weekday = day.weekday()
        forecast = bundle.sites[customer.site_id]["forecast"].get(day)
        generated = timestamp(raw.get("generated_at"), "prediction.generated_at")
        evidence = [
            f"予測の対象日: {day} / 地点: {customer.site_id} / 履歴の観測kWhとは別の将来予測"
        ]
        basis = dict(prediction=raw, forecast=forecast)
        if (
            forecast is None
            or forecast["issued_at"] > bundle.as_of
            or generated > bundle.as_of
        ):
            results.append(
                result(
                    "precool",
                    weekday,
                    "unavailable",
                    "判定日時までに利用可能な気象予報・運転予測がありません。",
                    {},
                    evidence,
                    [],
                    basis,
                )
            )
            continue
        start = local_midnight(day, customer.timezone) + timedelta(
            minutes=minute(raw.get("start"), "prediction.start")
        )
        end = local_midnight(day, customer.timezone) + timedelta(
            minutes=minute(raw.get("end"), "prediction.end")
        )
        required = local_midnight(day, customer.timezone) + timedelta(
            minutes=minute(raw.get("required_at"), "prediction.required_at")
        )
        basis["tariff"] = bundle.tariffs[customer.tariff_id][
            start.hour * 2 + start.minute // 30 : end.hour * 2 + end.minute // 30
        ]
        if start < bundle.as_of:
            results.append(
                result(
                    "precool",
                    weekday,
                    "unavailable",
                    "過去の候補予測は今後の運転提案に利用しません。",
                    {},
                    evidence,
                    [],
                    basis,
                )
            )
            continue
        if signature(forecast) != raw.get("weather_signature"):
            results.append(
                result(
                    "precool",
                    weekday,
                    "unavailable",
                    "気象予報と候補予測の前提が一致しません。予測の再作成が必要です。",
                    {},
                    evidence,
                    [],
                    basis,
                )
            )
            continue
        if any(
            celsius is None
            for celsius in forecast["outdoor_c"][
                start.hour * 2 + start.minute // 30 : end.hour * 2 + end.minute // 30
            ]
        ):
            results.append(
                result(
                    "precool",
                    weekday,
                    "unavailable",
                    "比較期間の予報気温が欠損しています。",
                    {},
                    evidence,
                    [],
                    basis,
                )
            )
            continue
        slots = []
        cursor = start
        while cursor < end:
            index = cursor.hour * 2 + cursor.minute // 30
            price = bundle.tariffs[customer.tariff_id][index]
            slots.append(
                dict(
                    start=cursor.isoformat(),
                    end=(cursor + timedelta(minutes=30)).isoformat(),
                    jpy_per_kwh=price,
                )
            )
            cursor += timedelta(minutes=30)
        comparison = energy_comparison(
            dict(
                start=start.isoformat(),
                end=end.isoformat(),
                required_at=required.isoformat(),
                comfort_target_c=raw.get("target_c"),
                slots=slots,
                plans=raw.get("plans"),
            )
        )
        if comparison is None:
            raise InputError("prediction: 予測が必要です")
        target = comparison.comfort_target_c
        selected_heat = [
            item
            for item in heat
            if item["status"] == "proposed" and weekday in item["applicable_weekdays"]
        ]
        if selected_heat:
            index = required.hour * 2 + required.minute // 30
            if forecast["outdoor_c"][index] is None:
                results.append(
                    result(
                        "precool",
                        weekday,
                        "unavailable",
                        "必要時刻の予報気温が欠損しています。",
                        {},
                        evidence,
                        [],
                        basis,
                    )
                )
                continue
            if forecast["outdoor_c"][index] >= HOT_THRESHOLD_C and any(
                Decimal(item["values"]["temperature_c"]) != target
                for item in selected_heat
            ):
                results.append(
                    result(
                        "precool",
                        weekday,
                        "unavailable",
                        "暑い日の明示的な温度希望と候補の快適性目標が一致しません。",
                        {},
                        evidence,
                        [],
                        basis,
                    )
                )
                continue
        explicit = [
            j
            for j in known
            if j["kind"] == "precool"
            and j["weekday"] == weekday
            and j["response"] in ("approved", "edited")
        ]
        if explicit:
            latest = max(explicit, key=lambda j: (j["responded_at"], j["id"]))
            selected = dict(latest["values"], **latest["selected"])
            choice = selected["plan"]
            selected_raw = next(
                (
                    plan
                    for plan in raw["plans"]
                    if plan["name"] == choice and plan["kind"] == "precool"
                ),
                None,
            )
            if selected_raw is None or any(
                str(selected_raw["controls"].get(key)) != str(value)
                for key, value in selected.items()
                if key != "plan"
            ):
                item = result(
                    "precool",
                    weekday,
                    "unavailable",
                    "明示的に選ばれた予冷案の今回の予測がありません。",
                    {},
                    evidence,
                    [],
                    basis,
                )
                results.append(apply_judgments(item, None, known))
                continue
            comparison = replace(
                comparison,
                plans=tuple(
                    plan
                    for plan in comparison.plans
                    if plan.kind != "precool" or plan.name == choice
                ),
            )
        schedule = next(
            (plan for plan in customer.schedules if plan.weekday == weekday),
            Schedule(weekday, 0, 1439, target),
        )
        if any(plan.weekday == weekday for plan in customer.schedules):
            usual_raw = next(plan for plan in raw["plans"] if plan["kind"] == "usual")
            current_controls = dict(
                start=clock(schedule.start),
                end=clock(schedule.end),
                temperature_c=str(schedule.temperature),
            )
            if any(
                str(usual_raw["controls"][key]) != value
                for key, value in current_controls.items()
            ):
                results.append(
                    result(
                        "precool",
                        weekday,
                        "unavailable",
                        "現行予定と通常運転予測の条件が一致しません。候補の再予測が必要です。",
                        {},
                        evidence,
                        [],
                        basis,
                    )
                )
                continue
        scenario = replace(base_scenario(bundle, customer, schedule), energy=comparison)
        outcome = energy_decision(scenario, None)
        costs = {}
        for plan in comparison.plans:
            cost = plan_cost(scenario, plan)
            costs[plan.name] = None if cost is None else str(cost)
        winner = next(
            (plan for plan in raw["plans"] if plan["name"] == outcome.target_plan), None
        )
        values = (
            dict(
                plan=outcome.target_plan,
                **{key: str(value) for key, value in winner["controls"].items()},
            )
            if winner
            else {}
        )
        item = result(
            "precool",
            weekday,
            "proposed" if outcome.proposed else "rejected",
            outcome.reason,
            values,
            evidence + list(outcome.evidence),
            [f"forecast:{customer.site_id}:{day}"],
            basis,
        )
        item["estimate"] = dict(
            basis="synthetic_prediction",
            target_c=str(target),
            required_at=required.isoformat(),
            costs_jpy=costs,
            note="現行予定を基準とする独立した候補比較。他の予定変更と合成した節約額ではありません。",
        )
        usual = next(plan for plan in comparison.plans if plan.kind == "usual")
        for plan in comparison.plans:
            if (
                plan.kind == "comfort"
                and costs[plan.name] is not None
                and costs[usual.name] is not None
            ):
                item["evidence"].append(
                    f"快適性案 {plan.name} の普段比 {money(Decimal(costs[plan.name]) - Decimal(costs[usual.name]))}（節約案と別）"
                )
        results.append(apply_judgments(item, None, known))
    return results


def billing(bundle: Bundle, customer: Customer, include_intervals: bool = True) -> dict:
    days = []
    for day in available_days(bundle, customer):
        intervals = []
        costs = []
        for index, kwh in enumerate(day.kwh):
            at = local_midnight(day.date, customer.timezone) + timedelta(
                minutes=index * 30
            )
            price = bundle.tariffs[customer.tariff_id][index]
            cost = None if kwh is None or price is None else kwh * price
            costs.append(cost)
            if include_intervals:
                intervals.append(
                    dict(
                        start=at.isoformat(),
                        kwh=None if kwh is None else str(kwh),
                        jpy_per_kwh=None if price is None else str(price),
                        jpy=None if cost is None else str(cost),
                    )
                )
        complete = all(cost is not None for cost in costs)
        subtotal = sum((cost for cost in costs if cost is not None), Decimal(0))
        days.append(
            dict(
                date=str(day.date),
                operation_status=day.status,
                usage=day.usage,
                total_jpy=str(subtotal) if complete else None,
                known_subtotal_jpy=str(subtotal),
                intervals=intervals,
            )
        )
    total = sum((Decimal(day["known_subtotal_jpy"]) for day in days), Decimal(0))
    return dict(
        basis="synthetic_observation",
        daily=days,
        known_subtotal_jpy=str(total),
        total_jpy=str(total)
        if days and all(day["total_jpy"] is not None for day in days)
        else None,
    )


def evaluate_customer(
    bundle: Bundle,
    customer: Customer,
    *,
    confirmed_on_ids: frozenset[str] | None = None,
    include_intervals: bool = True,
) -> dict:
    known = judgments_as_of(bundle, customer)
    decisions = schedule_results(bundle, customer, known, confirmed_on_ids)
    heat = temperature_results(bundle, customer, known)
    decisions.extend(heat)
    decisions.extend(precool_results(bundle, customer, heat, known))
    completed = available_days(bundle, customer)
    enrolled_days = (
        bundle.as_of.astimezone(customer.timezone).date()
        - customer.enrolled_at.astimezone(customer.timezone).date()
    ).days
    proposals = [item for item in decisions if item["status"] == "proposed"]
    if proposals:
        status = "提案あり"
    elif enrolled_days < PREPARATION_DAYS:
        status = "提案を準備中"
    elif any(item["status"] == "insufficient" for item in decisions):
        status = "履歴不足で一部判定できません"
    elif any(item["status"] == "suppressed" for item in decisions):
        status = "同じ根拠の再提案を抑制"
    else:
        status = "変更の必要を示す根拠なし"
    return dict(
        customer_id=customer.id,
        description=customer.description,
        device_id=customer.device_id,
        site_id=customer.site_id,
        timezone=str(customer.timezone),
        tariff_id=customer.tariff_id,
        as_of=bundle.as_of.isoformat(),
        simulation=True,
        status=status,
        proposal_count=len(proposals),
        current_schedules=[
            dict(
                weekday=plan.weekday,
                start=clock(plan.start),
                end=clock(plan.end),
                temperature_c=str(plan.temperature),
            )
            for plan in customer.schedules
        ],
        proposals=proposals,
        decisions=decisions,
        coverage=dict(
            days=len(completed),
            operation_status=dict(Counter(day.status for day in completed)),
            usage=dict(Counter(day.usage for day in completed)),
        ),
        judgments=[
            dict(
                j,
                proposed_at=j["proposed_at"].isoformat(),
                responded_at=j["responded_at"].isoformat()
                if j["responded_at"]
                else None,
            )
            for j in known
        ],
        billing=billing(bundle, customer, include_intervals),
    )


def evaluate_bundle(bundle: Bundle) -> list[dict]:
    return [evaluate_customer(bundle, customer) for customer in bundle.customers]
