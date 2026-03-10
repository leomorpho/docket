#!/usr/bin/env python3
"""Semantic embedding bridge for Docket."""

from __future__ import annotations

import argparse
import json
import sys


def emit(payload: dict) -> None:
    json.dump(payload, sys.stdout)
    sys.stdout.write("\n")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--model", required=True)
    args = parser.parse_args()

    try:
        payload = json.load(sys.stdin)
    except json.JSONDecodeError as exc:
        emit(
            {
                "model": args.model,
                "errors": [
                    {
                        "code": "invalid_json",
                        "message": f"invalid JSON input: {exc.msg}",
                    }
                ],
            },
        )
        return 1

    inputs = payload.get("inputs", [])
    texts = [item.get("text", "") for item in inputs]

    try:
        from sentence_transformers import SentenceTransformer
    except Exception as exc:  # pragma: no cover - exercised via subprocess tests
        emit(
            {
                "model": args.model,
                "errors": [{"code": "import_error", "message": str(exc)}],
            }
        )
        return 1

    try:
        model = SentenceTransformer(args.model)
        vectors = model.encode(texts)
    except Exception as exc:  # pragma: no cover - exercised via subprocess tests
        emit(
            {
                "model": args.model,
                "errors": [{"code": "model_load_error", "message": str(exc)}],
            }
        )
        return 1

    results = []
    dimension = 0
    for item, vector in zip(inputs, vectors):
        values = [float(value) for value in vector]
        if values:
            dimension = len(values)
        results.append({"chunk_id": item.get("chunk_id", ""), "vector": values})

    emit(
        {
            "model": args.model,
            "dimension": dimension,
            "results": results,
            "errors": [],
        }
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
