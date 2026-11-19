"""Keep the first effective application of each approval, separate from today's plan."""

import copy

from .loader import timestamp
from .models import clock
from .weekday_cards import same_start_choice


def first_application(values, weekday, approved_at, versions, at):
    eligible = sorted(
        (v for v in versions if v["recorded_at"] <= at and v["valid_from"] <= at),
        key=lambda v: v["valid_from"],
    )
    for index, version in enumerate(eligible):
        begin = max(approved_at, version["valid_from"])
        end = eligible[index + 1]["valid_from"] if index + 1 < len(eligible) else None
        if begin > at or (end is not None and begin >= end):
            continue
        plan = next((s for s in version["schedules"] if s.weekday == weekday), None)
        if (
            plan is not None
            and dict(
                start=clock(plan.start),
                end=clock(plan.end),
                temperature_c=str(plan.temperature),
            )
            == values
        ):
            return begin.isoformat()
    return None


def card_approval_states(known, device_id, schedules, *, versions, at):
    states = []
    for feedback in known:
        if (
            feedback.get("origin") != "card_response"
            or feedback["device_id"] != device_id
            or feedback["response"] != "approved"
        ):
            continue
        approved_at = timestamp(feedback["responded_at"], "responded_at")
        if approved_at > at:
            continue
        weekday = feedback["weekday"]
        values = copy.deepcopy(feedback["values"])
        applied_at = first_application(values, weekday, approved_at, versions, at)
        if applied_at is None and any(
            f["device_id"] == device_id
            and f["kind"] == "start"
            and f["weekday"] == weekday
            and f["response"] == "declined"
            and same_start_choice(f["values"], feedback["values"])
            and timestamp(f["responded_at"], "responded_at")
            > timestamp(feedback["responded_at"], "responded_at")
            for f in known
        ):
            continue
        current = schedules.get(weekday)
        current_matches = current is not None and all(
            str(current.get(k)) == str(v) for k, v in values.items()
        )
        states.append(
            dict(
                weekday=weekday,
                device_id=device_id,
                proposal_id=feedback["proposal_id"],
                response_id=feedback["id"],
                displayed_at=feedback["proposed_at"],
                approved_at=feedback["responded_at"],
                values=values,
                current_schedule=copy.deepcopy(current),
                status="applied" if applied_at is not None else "approved_pending",
                applied_at=applied_at,
                current_matches=current_matches,
                superseded=applied_at is not None and not current_matches,
            )
        )
    return sorted(
        states,
        key=lambda s: (
            s["weekday"],
            timestamp(s["approved_at"], "approved_at"),
            s["response_id"],
        ),
    )


def apply_card_approval(item, states):
    if item["kind"] != "start":
        return False
    matching = [s for s in states if s["weekday"] == item["weekday"]]
    state = (
        max(matching, key=lambda s: timestamp(s["approved_at"], "approved_at"))
        if matching
        else None
    )
    if state is not None and state["superseded"]:
        return False
    if state is None or (
        item["status"] == "proposed"
        and not same_start_choice(item["values"], state["values"])
    ):
        return False
    item.update(
        status=state["status"],
        values=copy.deepcopy(state["values"]),
        proposal_id=state["proposal_id"],
        approval=copy.deepcopy(state),
        reason="承認済み・反映待ちです。同じ変更を承認カード・通知候補から外しています。"
        if state["status"] == "approved_pending"
        else f"承認した内容は{state['applied_at']}に有効な予定へ反映済みです。",
    )
    item["judgment_influence"].append(
        f"{state['response_id']}: 承認した曜日別ID {state['proposal_id']} / {state['approved_at']}"
    )
    return True
