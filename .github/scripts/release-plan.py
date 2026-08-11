#!/usr/bin/env python3

"""Resolve and validate the Roaminal release plan for GitHub Actions."""

import functools
import json
import os
import re
import subprocess
import sys


def fail(message):
    print(f"release plan error: {message}", file=sys.stderr)
    raise SystemExit(1)


if len(sys.argv) != 3:
    fail("usage: release-plan.py <project-root> <chart|container>")

root = os.path.abspath(sys.argv[1])
artifact_kind = sys.argv[2]
if artifact_kind not in {"chart", "container"}:
    fail(f"unsupported artifact kind: {artifact_kind}")

tag_name = os.environ["TAG_NAME"]
forgekit = os.environ["FORGEKIT_BIN"]
output_path = os.environ["GITHUB_OUTPUT"]


def command(*args, check=True):
    result = subprocess.run(args, cwd=root, text=True, capture_output=True)
    if check and result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip()
        fail(f"{' '.join(args)} failed: {detail}")
    return result


def semver(value):
    match = re.fullmatch(
        r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
        r"(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?",
        str(value),
    )
    if not match:
        fail(f"invalid SemVer: {value}")
    prerelease = match.group(4)
    identifiers = [] if prerelease is None else prerelease.split(".")
    for identifier in identifiers:
        if identifier.isdigit() and len(identifier) > 1 and identifier.startswith("0"):
            fail(f"numeric prerelease identifier has a leading zero: {value}")
    return (int(match.group(1)), int(match.group(2)), int(match.group(3))), identifiers


def compare(left, right):
    left_core, left_pre = semver(left)
    right_core, right_pre = semver(right)
    if left_core != right_core:
        return (left_core > right_core) - (left_core < right_core)
    if not left_pre and not right_pre:
        return 0
    if not left_pre:
        return 1
    if not right_pre:
        return -1
    for left_id, right_id in zip(left_pre, right_pre):
        if left_id == right_id:
            continue
        left_num, right_num = left_id.isdigit(), right_id.isdigit()
        if left_num and right_num:
            return (int(left_id) > int(right_id)) - (int(left_id) < int(right_id))
        if left_num != right_num:
            return -1 if left_num else 1
        return (left_id > right_id) - (left_id < right_id)
    return (len(left_pre) > len(right_pre)) - (len(left_pre) < len(right_pre))


tag_match = re.fullmatch(
    r"roaminal-v(?P<version>(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\."
    r"(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?)",
    tag_name,
)
if not tag_match:
    fail("tag must match roaminal-v<semver> without build metadata")
tag_version = tag_match.group("version")
semver(tag_version)

forgekit_json = command(
    forgekit,
    "--project-root",
    root,
    "--output",
    "json",
    "version",
    "get",
    "roaminal",
).stdout
try:
    payload = json.loads(forgekit_json)
    app = payload.get("data", payload).get("app")
except (TypeError, json.JSONDecodeError) as exc:
    fail(f"ForgeKit returned invalid JSON: {exc}")
if not isinstance(app, dict):
    fail("ForgeKit output did not contain app metadata")
if (app.get("name"), app.get("type"), app.get("path")) != ("roaminal", "chart", "chart"):
    fail("ForgeKit app metadata must be roaminal/chart/chart")

linked = app.get("linked") or []
runtime_targets = [item for item in linked if item.get("name") == "roaminal-runtime"]
if len(linked) != 1 or len(runtime_targets) != 1:
    fail("Chart must contain exactly one linked roaminal-runtime target")
runtime = runtime_targets[0]
if (runtime.get("type"), runtime.get("path")) != ("container", "container"):
    fail("roaminal-runtime must be a container at path container")
runtime_version = str(runtime.get("value", ""))
semver(runtime_version)

yaml_script = (
    "require 'yaml'; require 'json'; "
    "chart=YAML.safe_load(File.read(ARGV[0]), aliases: true); "
    "values=YAML.safe_load(File.read(ARGV[1]), aliases: true); "
    "puts JSON.generate({'chart'=>chart, 'values'=>values})"
)
yaml_result = subprocess.run(
    [
        "ruby",
        "-e",
        yaml_script,
        os.path.join(root, "chart/Chart.yaml"),
        os.path.join(root, "chart/values.yaml"),
    ],
    cwd=root,
    text=True,
    capture_output=True,
)
if yaml_result.returncode != 0:
    fail(f"unable to parse Chart metadata: {yaml_result.stderr.strip()}")
try:
    metadata = json.loads(yaml_result.stdout)
    chart, values = metadata["chart"], metadata["values"]
except (KeyError, TypeError, json.JSONDecodeError) as exc:
    fail(f"Chart metadata is invalid: {exc}")
if chart.get("name") != "roaminal" or str(chart.get("version")) != tag_version:
    fail("tag version must equal chart/Chart.yaml version")
if str(chart.get("appVersion")) != runtime_version:
    fail("Chart appVersion must equal linked runtime version")
if str(values.get("image", {}).get("tag")) != runtime_version:
    fail("values.yaml image.tag must equal linked runtime version")
image = values.get("image", {})
if f"{image.get('registry')}/{image.get('repository')}" != "ghcr.io/ben-wangz/roaminal":
    fail("Chart default image must be ghcr.io/ben-wangz/roaminal")

head = command("git", "rev-parse", "HEAD").stdout.strip()
tag_commit = command("git", "rev-list", "-n", "1", f"{tag_name}^{{}}").stdout.strip()
if tag_commit != head:
    fail("tag must resolve to the checked-out commit")
main_ref = command("git", "rev-parse", "refs/remotes/origin/main").stdout.strip()
ancestor = command("git", "merge-base", "--is-ancestor", head, main_ref, check=False)
if ancestor.returncode != 0:
    fail("tag commit must be an ancestor of origin/main")

tag_candidates = command(
    "git", "tag", "--merged", head, "--list", "roaminal-v*"
).stdout.splitlines()
tag_candidates = [candidate for candidate in tag_candidates if candidate != tag_name]
for candidate in tag_candidates:
    if not re.fullmatch(r"roaminal-v.+", candidate):
        fail(f"invalid existing release tag: {candidate}")
tag_candidates.sort(key=functools.cmp_to_key(lambda a, b: compare(a[10:], b[10:])))
previous_tag = tag_candidates[-1] if tag_candidates else ""
previous_runtime = ""
if previous_tag:
    previous_runtime = command(
        "git", "show", f"{previous_tag}:container/VERSION"
    ).stdout.strip()
    semver(previous_runtime)
    if compare(runtime_version, previous_runtime) < 0:
        fail("runtime version must not move backwards from the previous release")

should_run = artifact_kind == "chart" or not previous_tag or runtime_version != previous_runtime


def write_output(name, value):
    with open(output_path, "a", encoding="utf-8") as handle:
        handle.write(f"{name}={value}\n")


outputs = {
    "should_run": "true" if should_run else "false",
    "app_name": "roaminal",
    "tag_version": tag_version,
    "chart_dir": "chart",
    "runtime_name": "roaminal-runtime",
    "runtime_dir": "container",
    "runtime_version": runtime_version,
    "commit_sha": head,
    "previous_tag": previous_tag,
    "image_ref": f"ghcr.io/ben-wangz/roaminal:{runtime_version}",
}
for name, value in outputs.items():
    write_output(name, value)
print(f"release plan: {artifact_kind} tag={tag_name} runtime={runtime_version} should_run={should_run}")
