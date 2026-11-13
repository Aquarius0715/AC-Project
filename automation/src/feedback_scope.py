"""Resolve one-off execution dates from causal references, never decision weekdays."""

from datetime import date

from .loader import InputError, timestamp


def target_date(value):
    if not isinstance(value, str):
        raise InputError("feedback.target_dateはYYYY-MM-DDで指定してください")
    try:
        parsed = date.fromisoformat(value)
    except ValueError as exc:
        raise InputError("feedback.target_dateはYYYY-MM-DDで指定してください") from exc
    if str(parsed) != value:
        raise InputError("feedback.target_dateはYYYY-MM-DDで指定してください")
    return value


def resolve_precool_date(feedback, history=()):
    """Return (execution date, provenance); ambiguous/unknown references stay unscoped."""
    explicit = feedback.get("target_date")
    if explicit is not None:
        explicit = target_date(explicit)
    candidates = {}
    for reference in feedback.get("evidence_ids", []):
        if isinstance(reference, str) and reference.startswith("forecast:"):
            try:
                day = target_date(reference.rsplit(":", 1)[-1])
            except InputError:
                continue
            candidates.setdefault(day, []).append("forecast参照")
    for record in history:
        if record["device_id"] != feedback["device_id"] or record["at"] > timestamp(
            feedback["proposed_at"], "proposed_at"
        ):
            continue
        sources = []
        if feedback.get("proposal_id") == record["proposal_id"]:
            sources.append("提案ID")
        if feedback.get("evidence_signature") == record["evidence_signature"]:
            sources.append("根拠署名")
        if sources:
            candidates.setdefault(record["target_date"], []).extend(sources)
    if explicit:
        if any(day != explicit for day in candidates):
            raise InputError(
                "feedback.target_dateと過去の予冷参照の実行対象日が一致しません"
            )
        return explicit, "明示した実行対象日"
    if len(candidates) == 1:
        day = next(iter(candidates))
        return day, "・".join(sorted(set(candidates[day])))
    return None, "参照が曖昧" if candidates else "実行対象日を特定できません"


def feedback_matches(item, feedback):
    if item["device_id"] != feedback["device_id"] or item["kind"] != feedback["kind"]:
        return False
    if item["kind"] == "precool":
        day, _ = resolve_precool_date(feedback)
        return day is not None and day == item.get("target_date")
    return item["weekday"] == feedback["weekday"]
