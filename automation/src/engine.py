"""Explicit provisional rules. This module only receives normalized inputs."""

from collections import Counter
from datetime import timedelta
from decimal import Decimal, ROUND_HALF_UP

from .models import Decision, Operation, Plan, Scenario, Weather, clock

START_OCCURRENCES = 4
START_MIN_SUPPORT = 3
START_TOLERANCE_MINUTES = 15
HOT_THRESHOLD_C = Decimal("32")
TEMPERATURE_MIN_DAYS = 3
WEATHER_TOLERANCE = timedelta(minutes=30)
COMFORT_TOLERANCE_C = Decimal("0.5")


def exclusions(scenario: Scenario, operation: Operation) -> str | None:
    if operation.device_id != scenario.device_id:
        return "別機器"
    if not scenario.history_start <= operation.at < scenario.as_of:
        return "観測期間外"
    if operation.source != "web_manual":
        return "自動運転・付属リモコン"
    if operation.once:
        return "今回だけ"
    if any(period.start <= operation.at < period.end for period in scenario.trips):
        return "旅行期間"
    return None


def usable_operations(scenario: Scenario) -> tuple[Operation, ...]:
    return tuple(sorted((op for op in scenario.operations if exclusions(scenario, op) is None),
                        key=lambda op: op.at))


def start_decision(scenario: Scenario) -> Decision:
    schedule = scenario.schedule
    today = scenario.as_of.astimezone(scenario.timezone).date()
    dates = []
    # A partial day has not supplied a full opportunity to observe a manual start.
    day = today - timedelta(days=1)
    while len(dates) < START_OCCURRENCES:
        if day.weekday() == schedule.weekday:
            dates.append(day)
        day -= timedelta(days=1)
    first_on = {}
    for op in usable_operations(scenario):
        local = op.at.astimezone(scenario.timezone)
        if op.action == "on" and local.date() in dates:
            first_on.setdefault(local.date(), local.hour * 60 + local.minute)
    times = tuple(first_on.values())
    evidence = tuple(f"{day.isoformat()}: {clock(first_on[day]) if day in first_on else '有効な手動オンなし（欠損・除外含む）'}"
                     for day in sorted(dates))
    candidates = []
    # At most 4 samples × 1440 minutes. Prefer most supporting dates, then
    # smallest total deviation, then the earlier time for deterministic ties.
    for candidate in range(24 * 60):
        supported = [time for time in times if abs(time - candidate) <= START_TOLERANCE_MINUTES]
        candidates.append((len(supported), -sum(abs(time - candidate) for time in supported), -candidate))
    support, _, negative_candidate = max(candidates)
    if support < START_MIN_SUPPORT:
        return Decision("start", False,
                        "証拠不足: 直近4回の対象曜日のうち、候補時刻の前後15分に3回以上が必要です。", evidence)
    candidate = -negative_candidate
    if candidate == schedule.start:
        return Decision("start", False, "手動操作の傾向が現行の開始時刻と一致しています。", evidence)
    if candidate >= schedule.end:
        return Decision("start", False, "候補時刻が終了時刻以降のため、開始変更を見送ります。", evidence)
    return Decision("start", True,
                    f"直近4回中{support}回が{clock(candidate)}の前後15分です。学習有効化前のWeb手動操作も集計しました。",
                    evidence, (f"開始 {clock(schedule.start)} → {clock(candidate)}",
                               f"終了 {clock(schedule.end)} を維持（{'月火水木金土日'[schedule.weekday]}曜のみ）"))


def matching_weather(scenario: Scenario, operation: Operation) -> Weather | None:
    samples = [sample for sample in scenario.weather
               if sample.device_id == scenario.device_id and sample.at < scenario.as_of
               and abs(sample.at - operation.at) <= WEATHER_TOLERANCE]
    if not samples:
        return None
    closest = min(abs(sample.at - operation.at) for sample in samples)
    nearest = [sample for sample in samples if abs(sample.at - operation.at) == closest]
    # Equally near conflicting readings do not establish a weather association.
    if len({sample.outdoor_c for sample in nearest}) != 1:
        return None
    return nearest[0]


def temperature_decision(scenario: Scenario) -> Decision:
    per_day = {}
    incomplete = 0
    for op in usable_operations(scenario):
        if op.action != "temperature":
            continue
        # Only one (the last) adjustment per local day counts, so repeated clicks
        # or a subsequent reversal cannot manufacture repeated preferences.
        per_day[op.at.astimezone(scenario.timezone).date()] = (op, matching_weather(scenario, op))
    evidence = []
    hot_days = {}
    cool_targets = set()
    for day, (op, weather) in per_day.items():
        if op.before is None or op.after is None or weather is None:
            incomplete += 1
            continue
        evidence.append(f"{day}: 外気温{weather.outdoor_c}℃、手動 {op.before}℃ → {op.after}℃")
        if weather.outdoor_c >= HOT_THRESHOLD_C and op.before == scenario.schedule.temperature and op.after < op.before:
            hot_days.setdefault(op.after, []).append(day)
        elif weather.outdoor_c < HOT_THRESHOLD_C:
            cool_targets.add(op.after)
    if incomplete:
        evidence.append(f"気象との対応不明・温度欠損 {incomplete}件は根拠に算入しません")
    candidates = [(len(days), target) for target, days in hot_days.items()
                  if len(days) >= TEMPERATURE_MIN_DAYS and target not in cool_targets]
    if not candidates:
        return Decision("temperature", False,
                        "証拠不足: 外気温32℃以上で同じ温度への手動変更が異なる3日以上必要です。"
                        "外気温との対応不明、または涼しい日にも同じ変更がある場合は暑い日限定と判断しません。",
                        tuple(evidence))
    count, target = max(candidates)
    return Decision("temperature", True,
                    f"外気温32℃以上の{count}日で同じ手動変更を確認しました。快適性の提案で、節約を意味しません。",
                    tuple(evidence),
                    (f"暑い日（外気温32℃以上）に限り {scenario.schedule.temperature}℃ → {target}℃",
                     "通常日の温度・開始・終了時刻を維持"), str(target))


def money(amount: Decimal) -> str:
    return f"{amount.quantize(Decimal('0.01'), rounding=ROUND_HALF_UP):.2f}円"


def plan_cost(scenario: Scenario, plan: Plan) -> Decimal | None:
    comparison = scenario.energy
    if (comparison is None or any(kwh is None for kwh in plan.energy)
            or any(slot.price is None for slot in comparison.slots)):
        return None
    return sum((slot.price * kwh for slot, kwh in zip(comparison.slots, plan.energy)
                if kwh is not None and slot.price is not None), Decimal(0))


def energy_decision(scenario: Scenario, temperature: Decision | None) -> Decision:
    comparison = scenario.energy
    if comparison is None:
        return Decision("precool", False, "比較期間・時間帯別の料金/消費電力量・快適性データが不足しています。")
    evidence = [f"同じ比較期間: {comparison.slots[0].start.isoformat()} 〜 {comparison.slots[-1].end.isoformat()}",
                f"必要時刻 {comparison.required_at.isoformat()} の予測室温を"
                f"{comparison.comfort_target_c}℃ ±{COMFORT_TOLERANCE_C}℃で比較（仮モデル）"]
    costs = {}
    comfortable = {}
    for plan in comparison.plans:
        cost = plan_cost(scenario, plan)
        costs[plan.name] = cost
        comfortable[plan.name] = (plan.indoor_c is not None and
                                 abs(plan.indoor_c - comparison.comfort_target_c) <= COMFORT_TOLERANCE_C)
        equations = " + ".join(f"[{slot.start.astimezone(scenario.timezone):%H:%M}〜"
                               f"{slot.end.astimezone(scenario.timezone):%H:%M}] "
                               f"{kwh if kwh is not None else '不明'}kWh×{slot.price if slot.price is not None else '不明'}円/kWh"
                               for slot, kwh in zip(comparison.slots, plan.energy))
        evidence.append(f"{plan.name}: {equations} = {money(cost) if cost is not None else '総額不明（消費電力量・料金欠損）'}"
                        f"、必要時刻の室温 {str(plan.indoor_c) + '℃' if plan.indoor_c is not None else '不明'}")
    usual = next(plan for plan in comparison.plans if plan.kind == "usual")
    if temperature is not None and temperature.proposed:
        for plan in comparison.plans:
            if plan.kind == "comfort" and costs[plan.name] is not None and costs[usual.name] is not None:
                difference = costs[plan.name] - costs[usual.name]
                evidence.append(f"快適性案 {plan.name} の普段比: {money(difference)}（費用の見積もり。節約提案とは別）")
        if Decimal(temperature.target_temperature_c) != comparison.comfort_target_c:
            return Decision("precool", False,
                            "快適性案の温度と料金比較の快適性目標が一致しないため見送ります。", tuple(evidence))
    references = [plan for plan in comparison.plans if plan.kind in ("usual", "comfort")
                  and costs[plan.name] is not None and comfortable[plan.name]]
    reference = min(references, key=lambda plan: costs[plan.name]) if references else None
    candidates = []
    for plan in comparison.plans:
        if plan.kind != "precool":
            continue
        if costs[plan.name] is None or plan.indoor_c is None:
            evidence.append(f"{plan.name}: 料金・消費電力量・快適性のいずれかが不足のため見送り")
        elif not comfortable[plan.name]:
            evidence.append(f"{plan.name}: 快適性を保てないため節約候補から除外")
        elif reference is None:
            evidence.append(f"{plan.name}: 同等の快適性の比較元の総額が不明のため見送り")
        elif costs[plan.name] >= costs[reference.name]:
            evidence.append(f"{plan.name}: {reference.name}より総額が下がらないため見送り")
        else:
            candidates.append(plan)
    if not candidates:
        return Decision("precool", False, "同等の快適性を満たし、比較期間の総額が下がる予冷案がありません。", tuple(evidence))
    winner = min(candidates, key=lambda plan: costs[plan.name])
    savings = costs[reference.name] - costs[winner.name]
    return Decision("precool", True,
                    f"仮データ上で同等の快適性を満たし、{reference.name}比で{money(savings)}下がります。",
                    tuple(evidence), (f"運転候補: {winner.name}",
                                      f"比較期間の総額 {money(costs[reference.name])} → {money(costs[winner.name])}"))


def evaluate(scenario: Scenario) -> tuple[Decision, ...]:
    decisions = [start_decision(scenario)] if "start" in scenario.checks else []
    temperature = temperature_decision(scenario) if "temperature" in scenario.checks else None
    if temperature is not None:
        decisions.append(temperature)
    if "precool" in scenario.checks:
        decisions.append(energy_decision(scenario, temperature))
    excluded = Counter(reason for op in scenario.operations if (reason := exclusions(scenario, op)))
    if excluded:
        decisions.append(Decision("history", False, "普段の傾向から除外: " +
                                  "、".join(f"{key} {count}件" for key, count in sorted(excluded.items()))))
    return tuple(decisions)
