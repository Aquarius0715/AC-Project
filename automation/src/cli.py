"""Run with python3 -m automation.src.cli from the repository root."""

import argparse
import json
import sys
from dataclasses import asdict
from pathlib import Path

from .engine import evaluate
from .loader import InputError, load

DATA_DIR = Path(__file__).resolve().parents[1] / "data"
LABELS = {"start": "曜日別の開始時刻", "temperature": "快適性・暑い日の温度",
          "precool": "節約・予冷", "history": "履歴の除外"}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="仮データによるエアコン運転の提案デモ")
    selection = parser.add_mutually_exclusive_group(required=True)
    selection.add_argument("--list", action="store_true", help="ケース一覧")
    selection.add_argument("--case", help="ケース名（拡張子なし）")
    selection.add_argument("--all", action="store_true", help="全ケースを実行")
    selection.add_argument("--file", type=Path, help="別のローカルJSONを読み込む")
    parser.add_argument("--json", action="store_true", help="構造化JSONで出力")
    args = parser.parse_args(argv)
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
    reports = [{"case": path.stem, "description": scenario.description,
                "simulation": True, "decisions": [asdict(item) for item in evaluate(scenario)]}
               for path, scenario in zip(paths, scenarios)]
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
