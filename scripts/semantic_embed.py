#!/usr/bin/env python3
"""Semantic embedding bridge for Docket.

This initial version only validates stdin/stdout framing.
"""

from __future__ import annotations

import argparse
import json
import sys


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--model", required=True)
    args = parser.parse_args()

    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        json.dump(
            {
                "model": args.model,
                "errors": [
                    {
                        "code": "invalid_json",
                        "message": f"invalid JSON input: {exc.msg}",
                    }
                ],
            },
            sys.stdout,
        )
        sys.stdout.write("\n")
        return 1

    inputs = payload.get("inputs", [])
    results = [{"chunk_id": item.get("chunk_id", ""), "vector": []} for item in inputs]
    json.dump(
        {
            "model": args.model,
            "dimension": 0,
            "results": results,
            "errors": [],
        },
        sys.stdout,
    )
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
