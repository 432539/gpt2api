#!/usr/bin/env python3
"""
Convert Adobe browser-export cookie JSONL into the import format used by gpt2api.

Input format:
  One JSON object per line, each with a "cookie" field containing full browser
  cookies.

Output format:
  A JSON array of objects:
    [{"cookie": "ims_sid=...; aux_sid=...; ...", "name": "adobe_001"}]

The cookie field is normalized to match the working sample exactly:
  ims_sid; aux_sid; fg; relay; ftrset; filter-profile-map; filter-profile-map-permanent
"""

from __future__ import annotations

import argparse
import json
from collections import OrderedDict
from pathlib import Path


COOKIE_KEYS = [
    "ims_sid",
    "aux_sid",
    "fg",
    "relay",
    "ftrset",
    "filter-profile-map",
    "filter-profile-map-permanent",
]


def parse_cookie(raw: str) -> OrderedDict[str, str]:
    """Parse a Cookie header string, keeping the last value for duplicate keys."""
    pairs: OrderedDict[str, str] = OrderedDict()
    for part in raw.split(";"):
        part = part.strip()
        if not part or "=" not in part:
            continue
        key, value = part.split("=", 1)
        pairs[key.strip()] = value.strip()
    return pairs


def convert_file(src: Path, out: Path, name_prefix: str) -> tuple[int, list[tuple[int, list[str]]]]:
    items: list[dict[str, str]] = []
    missing_rows: list[tuple[int, list[str]]] = []

    for idx, line in enumerate(src.read_text(encoding="utf-8-sig").splitlines(), 1):
        line = line.strip()
        if not line:
            continue

        obj = json.loads(line)
        raw_cookie = obj.get("cookie") or obj.get("Cookie") or ""
        pairs = parse_cookie(raw_cookie)
        missing = [key for key in COOKIE_KEYS if not pairs.get(key)]
        if missing:
            missing_rows.append((idx, missing))

        cookie = "; ".join(f"{key}={pairs[key]}" for key in COOKIE_KEYS if pairs.get(key))
        items.append(
            {
                "cookie": cookie,
                "name": f"{name_prefix}_{idx:03d}",
            }
        )

    out.write_text(json.dumps(items, ensure_ascii=False, indent=2), encoding="utf-8")
    return len(items), missing_rows


def verify(out: Path) -> dict[str, object]:
    items = json.loads(out.read_text(encoding="utf-8-sig"))
    wrong_rows = []
    for idx, item in enumerate(items, 1):
        keys = [
            part.split("=", 1)[0].strip()
            for part in item["cookie"].split(";")
            if "=" in part
        ]
        if keys != COOKIE_KEYS:
            wrong_rows.append((idx, keys))

    text = out.read_text(encoding="utf-8")
    return {
        "count": len(items),
        "wrong_structure": len(wrong_rows),
        "has_opta": "Optanon" in text,
        "has_gds": "gds=" in text,
        "has_gas": "gas=" in text,
        "has_mbox": "mbox=" in text,
        "has_prod": "filter-profile-map-permanent_prod" in text,
        "has_escaped_amp": "\\u0026" in text,
        "first_keys": [
            part.split("=", 1)[0].strip()
            for part in items[0]["cookie"].split(";")
            if "=" in part
        ]
        if items
        else [],
    }


def main() -> int:
    default_src = Path("C:/Users/Administrator/Desktop/12321/50\u4e2aadobe 4k.txt")
    parser = argparse.ArgumentParser(description="Convert Adobe cookie JSONL to gpt2api import JSON.")
    parser.add_argument("src", nargs="?", type=Path, default=default_src, help="source .txt/.jsonl file")
    parser.add_argument("-o", "--out", type=Path, help="output .json file")
    parser.add_argument("--name-prefix", default="adobe", help="generated account name prefix")
    args = parser.parse_args()

    src = args.src
    out = args.out or src.with_name(src.stem + "_完全对齐.json")
    count, missing_rows = convert_file(src, out, args.name_prefix)
    report = verify(out)

    print(f"wrote={count}")
    print(f"out={out}")
    print(f"missing_rows={len(missing_rows)}")
    print(f"wrong_structure={report['wrong_structure']}")
    print(f"first_keys={report['first_keys']}")
    print(f"has_opta={report['has_opta']} has_gds={report['has_gds']} has_gas={report['has_gas']} has_mbox={report['has_mbox']} has_prod={report['has_prod']}")
    print(f"has_escaped_amp={report['has_escaped_amp']}")

    if missing_rows:
        print("missing_examples=" + json.dumps(missing_rows[:5], ensure_ascii=False))
        return 2
    if report["wrong_structure"]:
        return 3
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
