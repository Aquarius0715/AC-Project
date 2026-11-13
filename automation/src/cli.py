"""Local CLI for editable JSON, daily replay and snapshot reports."""

import argparse
import sys
from pathlib import Path
from .dataset import DEFAULT_DATA, ROOT, load_dataset
from .generate import make_dataset
from .html_report import write_report
from .loader import InputError
from .replay import replay_dataset


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="編集可能な仮データと日別の承認型提案レポート"
    )
    sub = parser.add_subparsers(dest="command", required=True)
    gen = sub.add_parser("generate", help="顧客別の非圧縮JSONを生成")
    gen.add_argument("--output", type=Path, default=DEFAULT_DATA)
    gen.add_argument("--customers", type=int, default=100)
    gen.add_argument("--days", type=int, default=90)
    gen.add_argument("--seed", type=int, default=42)
    for name in ("replay", "snapshot"):
        run = sub.add_parser(name)
        run.add_argument("--data", type=Path, default=DEFAULT_DATA)
        run.add_argument("--customer")
        run.add_argument("--output", type=Path, default=ROOT / "results" / name)
    args = parser.parse_args(argv)
    try:
        if args.command == "generate":
            make_dataset(args.output, args.customers, args.days, args.seed)
            print(f"生成完了: {args.output}")
            return 0
        dataset = load_dataset(args.data)
        if args.customer and args.customer not in dataset.customers:
            raise InputError("顧客IDが見つかりません")
        result = replay_dataset(
            dataset,
            [args.customer] if args.customer else None,
            "daily" if args.command == "replay" else "snapshot",
        )
        output = write_report(result, args.output)
        print(
            f"{result['customer_count']}世帯 / {result['day_count']}回 / 新規 {result['new_proposal_count']}件 / {result['evaluation_seconds']}秒"
        )
        print(f"HTML: {output}")
        return 0
    except (InputError, ValueError, OSError, KeyError, TypeError) as exc:
        print(f"入力・実行エラー: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
