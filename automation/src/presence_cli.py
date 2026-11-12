"""CLI presentation for household review and timed batch evaluation."""

import json
from dataclasses import asdict, replace
from pathlib import Path
from time import perf_counter

from .loader import InputError, timestamp
from .monthly_data import calendar_date
from .presence_data import load_presence
from .presence_proposals import evaluate_cohort, evaluate_household

REPRESENTATIVES = (
    Path(__file__).resolve().parents[1]
    / "data"
    / "presence"
    / "representatives.json.gz"
)
WEEKDAYS = "月火水木金土日"


def print_household(report: dict) -> None:
    print(
        f"\n世帯 {report['household_id']} / 設置地点 {report['installation_site_id']} / 天気地点 {report['weather_site_id']}"
    )
    print(
        f"住人 {report['residents']} / 機器 {report['device_ids']} / 提案 {report['proposal_count']}件"
    )
    for device in report["devices"]:
        print(f"  機器 {device['device_id']}: 現在の予定 {device['current_schedules']}")
        for item in device["decisions"]:
            weekday = (
                WEEKDAYS[item["weekday"]] + "曜"
                if item["weekday"] is not None
                else "共通"
            )
            print(f"    {item['kind']} {weekday} / {item['status']}: {item['reason']}")
            if item["status"] == "proposed":
                print(
                    f"      提案内容 {item['values'] or '開始時刻の見直しを相談（具体時刻は未提示）'} / 承認が必要・未適用"
                )
                print(
                    f"      費用 {item['estimate'] or '未見積。帰宅時刻から節約額を推定しません'}"
                )
            if item["kind"] == "start_review" or item["status"] == "proposed":
                for line in item["evidence"]:
                    print(f"      - {line}")
                for row in item.get("comparisons", []):
                    print(f"        {row['date']} 住人別: {row['residents']}")
        print(
            f"    手動オンの電源確認: 有効{len(device['confirmed_manual_on_ids'])}件 / 除外理由 {dict_count(device['power_evidence_exclusions'])}"
        )
        bill = device["billing"]
        print(
            f"    観測電力による従量料金: 合計 {bill['total_jpy']}円 / 既知小計 {bill['known_subtotal_jpy']}円"
        )


def dict_count(rows: list[dict]) -> dict:
    from collections import Counter

    return dict(Counter(row["reason"] for row in rows))


def presence_command(args) -> int:
    start = perf_counter()
    data = load_presence(args.presence_data or REPRESENTATIVES)
    if args.as_of:
        data = replace(
            data, monthly=replace(data.monthly, as_of=timestamp(args.as_of, "--as-of"))
        )
    load_seconds = perf_counter() - start
    if args.households:
        report = evaluate_cohort(data)
        report.update(
            load_seconds=round(load_seconds, 6),
            total_seconds=round(perf_counter() - start, 6),
        )
        if args.json:
            print(json.dumps(report, ensure_ascii=False, indent=2, default=str))
        else:
            print(
                "仮データによる承認型の提案。位置取得・実機制御・予定適用は行いません。"
            )
            print(
                f"{report['household_count']}世帯 / {report['device_count']}機器 / {report['days']}日 / 提案 {report['proposal_count']}件"
            )
            print(
                f"世帯別件数 {report['household_counts']} / 種類別 {report['proposal_counts_by_kind']}"
            )
            print(
                f"見送り・変更不要の内訳（機器別・曜日別の判定数）: {report['nonproposal_reason_counts']}"
            )
            print(
                f"読込 {report['load_seconds']:.3f}秒 / 評価 {report['evaluation_seconds']:.3f}秒 / 合計 {report['total_seconds']:.3f}秒"
            )
            for household in report["households"]:
                print(
                    f"{household['household_id']}: {household['proposal_count']}件 / {household['device_ids']}"
                )
        return 0
    household = next(
        (house for house in data.households if house.id == args.household), None
    )
    if household is None:
        raise InputError("不明な世帯IDです。--householdsで確認してください")
    report = evaluate_household(data, household, include_intervals=bool(args.day))
    if args.day:
        day = calendar_date(args.day, "--day")
        details = []
        for device in report["devices"]:
            customer = next(
                c for c in data.monthly.customers if c.device_id == device["device_id"]
            )
            row = next((d for d in customer.days if d.date == day), None)
            bill = next(
                (d for d in device["billing"]["daily"] if d["date"] == args.day), None
            )
            if row is None or bill is None:
                raise InputError(
                    "--day: 判定日時以前の完了した記録日を指定してください"
                )
            details.append(
                dict(
                    device_id=device["device_id"],
                    operations=[
                        dict(id=event_id, **asdict(op))
                        for event_id, op in row.events
                        if op.at < data.monthly.as_of
                    ],
                    power_evidence=[
                        asdict(p)
                        for p in data.power.get(device["device_id"], ())
                        if p.event_id in {key for key, _ in row.events}
                        and p.received_at <= data.monthly.as_of
                    ],
                    charges=bill,
                    weather=data.monthly.sites[customer.site_id]["observations"].get(
                        day
                    ),
                )
            )
        report["day_detail"] = dict(
            date=str(day),
            devices=details,
            phone_observations=[
                asdict(o)
                for o in household.observations
                if o.at.astimezone(
                    data.monthly.sites[
                        data.installations[household.installation_site_id][
                            "weather_site_id"
                        ]
                    ]["timezone"]
                ).date()
                == day
                and o.at <= data.monthly.as_of
                and o.received_at <= data.monthly.as_of
            ],
        )
    if args.json:
        print(json.dumps(report, ensure_ascii=False, indent=2, default=str))
    else:
        print("仮のスマホ情報から世帯の在宅を判定し、承認が必要な提案を表示します。")
        print_household(report)
        if args.day:
            print(
                json.dumps(
                    report["day_detail"], ensure_ascii=False, indent=2, default=str
                )
            )
    return 0
