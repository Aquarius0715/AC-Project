"""A card's accepted weekday is pending until its effective schedule matches exactly."""

import copy

from .loader import timestamp
from .weekday_cards import same_start_choice


def card_approval_states(known, device_id, schedules):
    latest = {}
    for feedback in known:
        if (
            feedback.get("origin") != "card_response"
            or feedback["device_id"] != device_id
            or feedback["response"] != "approved"
        ):
            continue
        weekday = feedback["weekday"]
        previous = latest.get(weekday)
        if previous is None or timestamp(
            feedback["responded_at"], "responded_at"
        ) > timestamp(previous["responded_at"], "responded_at"):
            latest[weekday] = feedback
    states = []
    for weekday, feedback in sorted(latest.items()):
        if any(
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
        values = copy.deepcopy(feedback["values"])
        current = schedules.get(weekday)
        applied = current is not None and all(
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
                status="applied" if applied else "approved_pending",
            )
        )
    return states


def apply_card_approval(item, states):
    if item["kind"] != "start":
        return False
    state = next((s for s in states if s["weekday"] == item["weekday"]), None)
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
        else "承認した内容と、この判定日時に有効な予定が一致しています。反映済みです。",
    )
    item["judgment_influence"].append(
        f"{state['response_id']}: 承認した曜日別ID {state['proposal_id']} / {state['approved_at']}"
    )
    return True
