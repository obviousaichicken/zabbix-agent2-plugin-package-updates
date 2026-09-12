#!/usr/bin/env python3

import json
import re
import sys


def fail(message):
    raise SystemExit(message)


def require(condition, message):
    if not condition:
        fail(message)


def item_payload(output, key):
    match = re.search(rf"{re.escape(key)}\s+\[s\|(\{{[^\n]*\}})\]", output)
    if match is None:
        fail(f"{key} did not return a supported string value: {output!r}")
    return json.loads(match.group(1))


if len(sys.argv) != 2 or sys.argv[1] not in {"apt", "dnf"}:
    fail("usage: assert-installed-output.py apt|dnf")

backend = sys.argv[1]
output = sys.stdin.read()

if backend == "apt":
    payload = item_payload(output, "packages.get")
    require(payload["schema_version"] == 1, "unexpected package schema version")
    require(payload["backend"] == "apt", "packages.get did not use APT")
    require(payload["collection"]["complete"] is True, "collection is incomplete")
    require(
        payload["classification"]
        == {"complete": True, "failed_categories": []},
        "classification is incomplete",
    )
    require(isinstance(payload["repositories"], list), "repositories is not a list")
    require(isinstance(payload["updates"], list), "updates is not a list")
    require(
        payload["summary"]["repositories"] == len(payload["repositories"]),
        "repository count does not match payload",
    )
    require(
        payload["summary"]["updates"] == len(payload["updates"]),
        "update count does not match payload",
    )
    counts = payload["summary"]["update_types"]
    require(counts["bugfix"] == 0, "APT unexpectedly classified a bugfix")
    require(counts["enhancement"] == 0, "APT unexpectedly classified an enhancement")
    require(
        counts["security"] + counts["other"] == payload["summary"]["updates"],
        "APT update classifications do not match the update count",
    )
    require(payload["metadata"]["refreshed_at"] is not None, "refresh time is missing")
    require(payload["metadata"]["age_seconds"] >= 0, "metadata age is negative")
else:
    payload = item_payload(output, "advisories.get")
    require(payload["schema_version"] == 1, "unexpected advisory schema version")
    require(payload["collection"]["complete"] is True, "advisory collection is incomplete")
    require(isinstance(payload["advisories"], list), "advisories is not a list")
