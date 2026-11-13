"""Record hypothetical notifications; no transport or persistence side effects."""

import copy
from datetime import date, timedelta

from .feedback_scope import feedback_matches
from .weekday_cards import same_start_choice


def review_key(member):
    return (
        member["device_id"],
        member["kind"],
        None if member["kind"] == "precool" else member["weekday"],
        member["values"].get("base_temperature_c"),
        member.get("target_date"),
    )


def same_review(left, right):
    if left["kind"] == "start":
        return same_start_choice(left["values"], right["values"])
    return left["values"] == right["values"] and left.get("current_start") == right.get(
        "current_start"
    )


class NotificationPlanner:
    def __init__(self, presentation="grouped-weekly"):
        self.presentation = presentation
        self.sent_weeks = set()
        self.anchors = {}
        self.pending = {}

    def evaluate(self, at, cards, events, known):
        week = str(at.date() - timedelta(days=at.weekday()))
        active = {}
        for card in cards:
            for member in card["members"]:
                target = dict(copy.deepcopy(member), card_id=card["card_id"])
                # Distinguish each one-off precooling execution date.
                event = next(
                    e
                    for e in events
                    if e["proposal_id"] == member["proposal_id"]
                    and e["event"] != "ended"
                )
                target["target_date"] = event.get("target_date")
                target["current_start"] = event.get("current_start")
                if target["kind"] == "precool" and (
                    not target["target_date"]
                    or date.fromisoformat(target["target_date"]) < at.date()
                ):
                    continue
                active[review_key(target)] = target
        dropped = [
            dict(
                value,
                drop_reason="実行対象日が過ぎた予冷案のため通知待ちから除外しました"
                if value.get("target_date")
                and date.fromisoformat(value["target_date"]) < at.date()
                else "提案終了・承認・固定・見送りなどで有効でなくなりました",
            )
            for key, value in self.pending.items()
            if key not in active
        ]
        self.pending = {k: v for k, v in self.pending.items() if k in active}
        for key, member in active.items():
            anchor = self.anchors.get(key)
            acknowledged = any(
                feedback_matches(member, f)
                and f["response"] in ("approved", "edited", "declined", "excluded_once")
                and (
                    same_start_choice(
                        member["values"], dict(f["values"], **f.get("selected", {}))
                    )
                    if member["kind"] == "start"
                    else True
                    if member["kind"] == "precool"
                    else member["values"] == dict(f["values"], **f.get("selected", {}))
                )
                for f in known
            )
            novel = not acknowledged and (
                anchor is None or not same_review(member, anchor)
            )
            if self.presentation == "individual-daily":
                novel = any(
                    e["proposal_id"] == member["proposal_id"] and e["is_new"]
                    for e in events
                )
            if novel:
                waiting = self.pending.get(key, {}).get(
                    "first_waiting_at", at.isoformat()
                )
                self.pending[key] = dict(member, first_waiting_at=waiting)
            else:
                self.pending.pop(key, None)
        available = (
            self.presentation == "individual-daily" or week not in self.sent_weeks
        )
        scheduled = bool(self.pending) and available
        targets = list(self.pending.values()) if scheduled else []
        if scheduled:
            self.sent_weeks.add(week)
            for target in targets:
                self.anchors[review_key(target)] = copy.deepcopy(target)
            self.pending = {}
        return dict(
            scheduled=scheduled,
            scheduled_at=at.isoformat() if scheduled else None,
            week_start=week,
            reason="新しく判断する案をまとめて通知予定"
            if scheduled
            else "今週は通知予定済み。未通知の有効な案を翌週へ保留"
            if self.pending
            else "新しく確認すべき有効な案なし（継続・無反応だけでは催促しません）",
            targets=targets,
            pending=list(self.pending.values()),
            dropped=dropped,
        )
