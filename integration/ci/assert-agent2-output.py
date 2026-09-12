#!/usr/bin/env python3

import json
import re
import sys


def fail(message):
    raise SystemExit(message)


def require_equal(actual, expected, field):
    if actual != expected:
        fail(f"unexpected {field}: got {actual!r}, want {expected!r}")


output = sys.stdin.read()


def item_payload(key):
    match = re.search(rf"{re.escape(key)}\s+\[s\|(\{{[^\n]*\}})\]", output)
    if match is None:
        fail(f"{key} did not return a supported string value: {output!r}")
    return json.loads(match.group(1))


packages = item_payload("packages.get")
require_equal(packages["schema_version"], 1, "package schema version")
require_equal(packages["backend"], "dnf", "package backend")
require_equal(
    packages["capabilities"],
    {
        "classification": {
            "security": "supported",
            "bugfix": "supported",
            "enhancement": "supported",
            "other": "supported",
        },
        "repository_attribution": "supported",
        "reboot_detection": "supported",
        "last_update": "supported",
        "metadata_age": "unsupported",
    },
    "package capabilities",
)
require_equal(
    packages["metadata"],
    {"refreshed_at": None, "age_seconds": None},
    "package metadata",
)
require_equal(
    packages["classification"],
    {"complete": True, "failed_categories": []},
    "package classification",
)
require_equal(packages["summary"]["repositories"], 1, "repository count")
require_equal(packages["summary"]["updates"], 1, "update count")
require_equal(
    packages["summary"]["update_types"],
    {"security": 0, "bugfix": 0, "enhancement": 0, "other": 1},
    "update type counts",
)
require_equal(
    packages["repositories"],
    [{"id": "test", "name": "Test repository", "update_count": 1}],
    "repositories",
)
require_equal(
    packages["updates"],
    [
        {
            "repository_id": "test",
            "name": "example",
            "epoch": "0",
            "version": "2.0",
            "release": "1",
            "arch": "x86_64",
            "type": "other",
            "full_version": "0:2.0-1",
            "identifier": "example-0:2.0-1.x86_64",
        }
    ],
    "updates",
)
require_equal(packages["summary"]["last_update"]["result"], "success", "last update")

advisories = item_payload("advisories.get")
require_equal(advisories["schema_version"], 1, "advisory schema version")
require_equal(advisories["collection"]["complete"], True, "advisory completeness")
require_equal(
    advisories["metadata"],
    {
        "details_complete": False,
        "cves_complete": False,
        "issue_dates_complete": False,
    },
    "advisory metadata",
)
require_equal(advisories["summary"]["advisories"], 1, "advisory count")
require_equal(advisories["summary"]["unique_cves"], 0, "unique CVE count")
require_equal(
    advisories["advisories"],
    [
        {
            "id": "TEST-2026:1",
            "type": "security",
            "severity": "important",
            "title": "",
            "issued_at": None,
            "updated_at": None,
            "cve_ids": [],
            "affected_update_nevras": ["example-0:2.0-1.x86_64"],
        }
    ],
    "advisories",
)
