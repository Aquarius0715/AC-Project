"""Run with python3 -m automation.src.cli from the repository root."""

import argparse
import json
import sys
from dataclasses import asdict, replace
from pathlib import Path

from .engine import evaluate
from .loader import InputError, load, timestamp
from .monthly_data import calendar_date, load_bundle
from .monthly_engine import WEEKDAYS, evaluate_customer
from .presence_cli import presence_command

DATA_DIR = Path(__file__).resolve().parents[1] / "data"
LABELS = {
    "start": "曜日別の開始時刻",
    "temperature": "快適性・暑い日の温度",
    "precool": "節約・予冷",
    "history": "履歴の除外",
}
MONTHLY_DATA = DATA_DIR / "monthly" / "dataset.json"
STATUSES = {
    "proposed": "提案",
    "unchanged": "変更不要",
    "insufficient": "根拠不足",
    "suppressed": "再提案を抑制",
    "unavailable": "比較データなし",
    "rejected": "見送り",
}


def print_customer(report: dict) -> None:
    print(
        f"\n{report['customer_id']} / {report['description']}: {report['proposal_count']}件 / {report['status']}"
    )
    print(
        f"機器 {report['device_id']} / 地点 {report['site_id']} / {report['timezone']} / 料金 {report['tariff_id']}"
    )
    print(f"判定日時 {report['as_of']} / 観測状況 {report['coverage']}")
    print("現在の予定:")
    for schedule in report["current_schedules"]:
        print(
            f"  {WEEKDAYS[schedule['weekday']]}曜 {schedule['start']}〜{schedule['end']} {schedule['temperature_c']}℃"
        )
    if not report["current_schedules"]:
        print("  未設定")
    print("全曜日・温度・予冷の判定（見送りは提案件数に含みません）:")
    for item in report["decisions"]:
        scope = (
            f"{WEEKDAYS[item['weekday']]}曜"
            if item["weekday"] is not None
            else "共通条件"
        )
        print(
            f"  [{STATUSES[item['status']]}] {item['kind']} / {scope}: {item['reason']}"
        )
        if item["values"]:
            print(f"    内容: {item['values']} / 支持日数 {item['support_count']}")
        for detail in item["evidence"] + item["judgment_influence"]:
            print(f"    - {detail}")
        if item["estimate"]:
            print(f"    費用の仮予測: {item['estimate']}")
        elif item["status"] == "proposed":
            print("    この変更の費用: 候補別の予測がないため未見積（観測料金は下記）")
        print(f"    根拠署名: {item['evidence_signature']}")
    print("過去の提案と判定日時までに分かっている反応:")
    for judgment in report["judgments"]:
        print(
            f"  {judgment['id']} / {judgment['proposed_at']} / {judgment['response']} / 選択 {judgment['selected']}"
        )
        print(
            f"    内容 {judgment['values']} / 根拠 {judgment['reason']} / 根拠ID {judgment['evidence_ids']}"
        )
    if not report["judgments"]:
        print("  なし（反応履歴がないことは、手動操作の根拠を減らしません）")
    bill = report["billing"]
    print(
        f"観測電力からの従量料金: 合計 {bill['total_jpy'] if bill['total_jpy'] is not None else '欠損のため不明'}円 / 既知区間小計 {bill['known_subtotal_jpy']}円"
    )
    for day in bill["daily"]:
        print(
            f"  {day['date']} {day['operation_status']} / {day['usage']}: "
            f"{day['total_jpy'] if day['total_jpy'] is not None else '不明'}円（既知小計 {day['known_subtotal_jpy']}円）"
        )


def monthly_command(args: argparse.Namespace) -> int:
    bundle = load_bundle(args.monthly_data or MONTHLY_DATA)
    if args.as_of:
        bundle = replace(bundle, as_of=timestamp(args.as_of, "--as-of"))
    customers = [
        customer
        for customer in bundle.customers
        if not args.customer or customer.id == args.customer
    ]
    if not customers:
        raise InputError("不明な顧客IDです。--customersで確認してください")
    reports = [evaluate_customer(bundle, customer) for customer in customers]
    if args.customers:
        summaries = [
            dict(
                customer_id=r["customer_id"],
                description=r["description"],
                proposal_count=r["proposal_count"],
                status=r["status"],
                proposals=[
                    dict(kind=p["kind"], weekday=p["weekday"], values=p["values"])
                    for p in r["proposals"]
                ],
                observed_total_jpy=r["billing"]["total_jpy"],
            )
            for r in reports
        ]
        if args.json:
            print(json.dumps(summaries, ensure_ascii=False, indent=2))
        else:
            print(
                f"仮データ・仮ルール / {len(summaries)}顧客の月次評価。金額・室温は仮の値です。"
            )
            for report in summaries:
                proposals = (
                    "、".join(
                        f"{p['kind']}:{WEEKDAYS[p['weekday']] if p['weekday'] is not None else '暑い日'}"
                        for p in report["proposals"]
                    )
                    or "なし"
                )
                print(
                    f"{report['customer_id']}  {report['proposal_count']}件  {report['status']} / {proposals} / {report['description']}"
                )
        return 0
    report = reports[0]
    if args.day:
        day_date = calendar_date(args.day, "--day")
        day = next((day for day in customers[0].days if day.date == day_date), None)
        charges = next(
            (day for day in report["billing"]["daily"] if day["date"] == args.day), None
        )
        if day is None or charges is None:
            raise InputError(
                "--day: 判定日時以前の、記録のある完了日を指定してください"
            )
        site = bundle.sites[customers[0].site_id]
        weather = site["observations"].get(day_date)
        weather = weather if weather and weather["issued_at"] <= bundle.as_of else None
        report["day_detail"] = dict(
            charges,
            operations=[
                dict(asdict(op), id=key, at=op.at.isoformat())
                for key, op in day.events
                if op.at < bundle.as_of
            ],
            weather_kind="observation",
            condition=weather["condition"] if weather else None,
            outdoor_c=[str(c) if c is not None else None for c in weather["outdoor_c"]]
            if weather
            else None,
        )
    if args.json:
        print(json.dumps(report, ensure_ascii=False, indent=2, default=str))
    else:
        print("仮データ・仮ルールの月次評価。快適性と費用は仮の見積もりです。")
        print_customer(report)
        if args.day:
            detail = report["day_detail"]
            print(f"\n{args.day} の区間料金と操作（天気は履歴の観測相当値）:")
            for op in detail["operations"]:
                print(f"  操作: {op}")
            for index, interval in enumerate(detail["intervals"]):
                outdoor = detail["outdoor_c"][index] if detail["outdoor_c"] else None
                print(
                    f"  {interval['start']}  {interval['kwh']} kWh × {interval['jpy_per_kwh']} 円/kWh = {interval['jpy']} 円 / 外気温 {outdoor}℃ {detail['condition']}"
                )
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="仮データによるエアコン運転の提案デモ")
    selection = parser.add_mutually_exclusive_group(required=True)
    selection.add_argument("--list", action="store_true", help="ケース一覧")
    selection.add_argument("--case", help="ケース名（拡張子なし）")
    selection.add_argument("--all", action="store_true", help="全ケースを実行")
    selection.add_argument("--file", type=Path, help="別のローカルJSONを読み込む")
    selection.add_argument(
        "--customers", action="store_true", help="全顧客の月次評価・件数一覧"
    )
    selection.add_argument("--customer", help="顧客IDを指定して月次評価の詳細を表示")
    selection.add_argument(
        "--households", action="store_true", help="スマホ由来の在宅デモを全世帯評価"
    )
    selection.add_argument("--household", help="在宅デモの世帯IDを指定して確認")
    parser.add_argument(
        "--presence-data", type=Path, help="在宅デモのJSONまたはJSON.gz"
    )
    parser.add_argument(
        "--monthly-data", type=Path, help="月次データの別のローカルJSON"
    )
    parser.add_argument(
        "--day", help="個別顧客の日付別の操作・天気・48区間料金を表示 (YYYY-MM-DD)"
    )
    parser.add_argument(
        "--as-of", help="月次評価の判定日時を変更 (UTCオフセット付きISO 8601)"
    )
    parser.add_argument("--json", action="store_true", help="構造化JSONで出力")
    args = parser.parse_args(argv)
    if args.day and not (args.customer or args.household):
        parser.error("--dayは--customerまたは--householdと一緒に指定してください")
    if args.monthly_data and not (args.customers or args.customer):
        parser.error("--monthly-dataは従来の月次評価で指定してください")
    if args.presence_data and not (args.households or args.household):
        parser.error("--presence-dataは在宅デモで指定してください")
    if args.as_of and not (
        args.customers or args.customer or args.households or args.household
    ):
        parser.error("--as-ofは月次評価または在宅デモで指定してください")
    if args.households or args.household:
        try:
            return presence_command(args)
        except InputError as exc:
            print(f"入力エラー: {exc}", file=sys.stderr)
            return 2
    if args.customers or args.customer:
        try:
            return monthly_command(args)
        except InputError as exc:
            print(f"入力エラー: {exc}", file=sys.stderr)
            return 2
    paths = sorted(DATA_DIR.glob("*.json"))
    if args.case:
        paths = [path for path in paths if path.stem == args.case]
        if not paths:
            parser.error("不明なケースです。--listで確認してください")
    elif args.file:
        paths = [args.file]
    try:
        scenarios = [load(path) for path in paths]
    except InputError as exc:
        print(f"入力エラー: {exc}", file=sys.stderr)
        return 2
    if args.list:
        for path, scenario in zip(paths, scenarios):
            print(f"{path.stem}: {scenario.description}")
        return 0
    reports = [
        {
            "case": path.stem,
            "description": scenario.description,
            "simulation": True,
            "decisions": [asdict(item) for item in evaluate(scenario)],
        }
        for path, scenario in zip(paths, scenarios)
    ]
    if args.json:
        print(json.dumps(reports, ensure_ascii=False, indent=2))
    else:
        print("仮データ・仮ルールによる提案です。金額・快適性は仮の見積もりです。")
        for report in reports:
            print(f"\n[{report['case']}] {report['description']}")
            for item in report["decisions"]:
                state = "提案" if item["proposed"] else "見送り・参考"
                print(f"  {LABELS[item['check']]} / {state}: {item['reason']}")
                for detail in (*item["changes"], *item["evidence"]):
                    print(f"    - {detail}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
