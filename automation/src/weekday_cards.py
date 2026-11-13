"""Presentation groups retain weekday proposal identities and explicit choices."""

import copy
import re
from datetime import date

from .loader import InputError, array, minute, obj, timestamp, number
from .monthly_engine import signature

WEEKDAYS = "月火水木金土日"


def same_start_choice(left, right):
    """Material change within one weekday, anchored to the shown/declined choice."""
    return (
        all(k in left and k in right for k in ("start", "end", "temperature_c"))
        and abs(minute(left["start"], "start") - minute(right["start"], "start")) <= 15
        and left["end"] == right["end"]
        and str(left["temperature_c"]) == str(right["temperature_c"])
    )


def validate_controls(raw, device_ids):
    ids = set()
    for record in array(raw.get("start_controls", []), "start_controls"):
        record = obj(record, "start_control")
        if (
            not isinstance(record.get("id"), str)
            or record["id"] in ids
            or not isinstance(record.get("device_id"), str)
            or record.get("device_id") not in device_ids
            or type(record.get("weekday")) is not int
            or record["weekday"] not in range(7)
            or record.get("action") not in ("lock", "unlock")
        ):
            raise InputError("開始固定のID・機器・曜日・設定が不正です")
        ids.add(record["id"])
        if timestamp(record.get("received_at"), "control.received_at") < timestamp(
            record.get("at"), "control.at"
        ):
            raise InputError("開始固定の受信時刻は設定時刻以降が必要です")

    ids = set()
    for response in array(raw.get("card_responses", []), "card_responses"):
        response = obj(response, "card_response")
        if (
            not isinstance(response.get("id"), str)
            or response["id"] in ids
            or not isinstance(response.get("card_id"), str)
        ):
            raise InputError("カード反応IDが不正または重複しています")
        ids.add(response["id"])
        if timestamp(response.get("responded_at"), "card.responded_at") <= timestamp(
            response.get("proposed_at"), "card.proposed_at"
        ):
            raise InputError("カードの反応は表示判定日時より後にしてください")
        members = array(response.get("members"), "card.members")
        if not members:
            raise InputError("カード反応には曜日別の参照内容が必要です")
        seen = set()
        for member in members:
            member = obj(member, "card.member")
            key = (member.get("device_id"), member.get("weekday"))
            if (
                not isinstance(member.get("device_id"), str)
                or member.get("device_id") not in device_ids
                or type(member.get("weekday")) is not int
                or member["weekday"] not in range(7)
                or key in seen
                or member.get("decision")
                not in ("approved", "declined", "excluded_once")
                or not isinstance(member.get("proposal_id"), str)
            ):
                raise InputError("カード反応の機器・曜日・選択・提案IDが不正です")
            seen.add(key)
            values = obj(member.get("values"), "card.values")
            if set(values) != {"start", "end", "temperature_c"}:
                raise InputError("開始カードの参照内容は開始・終了・温度が必要です")
            if minute(values["start"], "card.start") >= minute(
                values["end"], "card.end"
            ):
                raise InputError("カードの開始は終了より前が必要です")
            number(values["temperature_c"], "card.temperature_c", "16", "30")


def known_card_feedback(raw, at):
    """Flatten a card response only after it occurred; excluded_once is not a veto."""
    rows = []
    for response in raw.get("card_responses", []):
        if timestamp(response["responded_at"], "responded_at") > at:
            continue
        for member in response["members"]:
            rows.append(
                dict(
                    id=f"{response['id']}:{member['weekday']}",
                    origin="card_response",
                    card_id=response["card_id"],
                    proposed_at=response["proposed_at"],
                    responded_at=response["responded_at"],
                    device_id=member["device_id"],
                    kind="start",
                    weekday=member["weekday"],
                    proposal_id=member["proposal_id"],
                    values=copy.deepcopy(member["values"]),
                    response=member["decision"],
                    selected={},
                    evidence_signature=member.get("evidence_signature"),
                    evidence_ids=member.get("evidence_ids", []),
                )
            )
    return rows


def apply_fixed_weekdays(report, raw, at):
    latest = {}
    for row in sorted(
        raw.get("start_controls", []),
        key=lambda r: (
            timestamp(r["at"], "at"),
            timestamp(r["received_at"], "received_at"),
            r["id"],
        ),
    ):
        if (
            timestamp(row["at"], "at") <= at
            and timestamp(row["received_at"], "received_at") <= at
        ):
            latest[(row["device_id"], row["weekday"])] = row
    for device in report["devices"]:
        for item in device["decisions"]:
            record = latest.get((item["device_id"], item["weekday"]))
            if (
                item["kind"] in ("start", "start_review")
                and record
                and record["action"] == "lock"
            ):
                item.update(
                    status="fixed",
                    reason_code="start:fixed",
                    reason="ユーザーがこの機器・曜日の開始を固定しています。解除まで提案対象から外します。",
                )
                item["judgment_influence"].append(
                    f"{record['id']}: {record['at']}から曜日固定（受信{record['received_at']}）"
                )
        device["proposals"] = [
            d for d in device["decisions"] if d["status"] == "proposed"
        ]
        device["proposal_count"] = len(device["proposals"])
    report["proposal_count"] = sum(d["proposal_count"] for d in report["devices"])
    return list(latest.values())


def weekday_label(weekdays):
    if weekdays == [0, 1, 2, 3, 4]:
        return "平日すべて"
    if weekdays == [5, 6]:
        return "土日まとめて"
    return "・".join(WEEKDAYS[w] for w in weekdays if w is not None)


def build_cards(report, household_id, grouped=True):
    cards = {}
    for device in report["devices"]:
        plans = {s["weekday"]: s for s in device["current_schedules"]}
        for item in device["decisions"]:
            if item["status"] != "proposed":
                continue
            start = grouped and item["kind"] == "start"
            segment = (
                "weekday"
                if item["weekday"] is not None and item["weekday"] < 5
                else "weekend"
            )
            key = (
                (item["device_id"], segment, item["values"]["start"])
                if start
                else (item["proposal_id"],)
            )
            card = cards.setdefault(
                key,
                dict(
                    card_id=f"{household_id}-C{signature(key)[:12]}",
                    device_id=item["device_id"],
                    kind=item["kind"],
                    segment=segment if start else "individual",
                    members=[],
                ),
            )
            member = {
                k: copy.deepcopy(item[k])
                for k in (
                    "proposal_id",
                    "weekday",
                    "values",
                    "evidence_ids",
                    "evidence_signature",
                    "support_count",
                    "first_displayed_at",
                    "candidate_values",
                    "target_date",
                )
                if k in item
            }
            member.update(
                device_id=item["device_id"],
                kind=item["kind"],
                current=plans.get(item["weekday"]),
                evidence_dates=sorted(
                    {e[:10] for e in item["evidence"] if _date_prefix(e)}
                ),
                support_dates=support_dates(item),
            )
            card["members"].append(member)
    for card in cards.values():
        card["members"].sort(key=lambda m: -1 if m["weekday"] is None else m["weekday"])
        card["weekdays"] = [m["weekday"] for m in card["members"]]
        card["label"] = weekday_label(card["weekdays"])
        card["title"] = (
            f"{card['label']}：{card['members'][0]['values']['start']}開始へ変更"
            if card["kind"] == "start"
            else f"{card['label']} "
            + {
                "start_review": "開始時刻の見直し",
                "add_schedule": "初回予定",
                "temperature": "暑い日の温度",
                "precool": "予冷",
            }.get(card["kind"], card["kind"])
        )
    return sorted(
        cards.values(), key=lambda c: (c["device_id"], c["segment"], c["title"])
    )


def _date_prefix(text):
    try:
        date.fromisoformat(text[:10])
        return True
    except ValueError:
        return False


def support_dates(item):
    if item["kind"] != "start":
        return []
    candidate = item.get("candidate_values", item["values"])["start"]
    dates = set()
    for text in item["evidence"]:
        vote = re.fullmatch(r"(\d{4}-\d{2}-\d{2}): (\d{2}:\d{2}) / complete:used", text)
        if vote and abs(minute(vote[2], "vote") - minute(candidate, "candidate")) <= 15:
            dates.add(vote[1])
    return sorted(dates)


def validate_response_references(raw, at, ledger):
    for response in raw.get("card_responses", []):
        if timestamp(response["responded_at"], "responded_at") > at:
            continue
        displayed = ledger.get(
            (timestamp(response["proposed_at"], "proposed_at"), response["card_id"])
        )
        if displayed is None:
            raise InputError(f"{response['id']}: 参照した日時の提案カードがありません")
        for member in response["members"]:
            shown = next(
                (m for m in displayed if m["proposal_id"] == member["proposal_id"]),
                None,
            )
            if shown is None or any(
                shown[k] != member[k] for k in ("device_id", "weekday", "values")
            ):
                raise InputError(
                    f"{response['id']}: 曜日別IDと表示・承認対象が一致しません"
                )
            if member.get("evidence_signature") != shown["evidence_signature"]:
                raise InputError(f"{response['id']}: 表示時の根拠署名と一致しません")
            if (
                member.get("evidence_ids", shown["evidence_ids"])
                != shown["evidence_ids"]
            ):
                raise InputError(f"{response['id']}: 表示時の根拠IDと一致しません")
