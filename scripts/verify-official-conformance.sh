#!/usr/bin/env bash
# Run the pinned official PikaID cross-language conformance suite against the
# JSON-Lines adapter in cmd/conformance-adapter.
#
# The suite checkout is never written into this repository. It is cloned from
# the pinned commit into a scratch directory (or $PIKAID_SPECS_DIR when given)
# and removed on exit unless $PIKAID_CONFORMANCE_WORKDIR is set.
#
# Environment overrides:
#   PIKAID_SPECS_DIR        existing checkout to use instead of cloning
#   PIKAID_CONFORMANCE_WORKDIR  scratch directory to keep
#   PIKAID_CONFORMANCE_VENV     existing virtualenv to reuse
#   PIKAID_CONFORMANCE_REPORT_JSON  path for the automated-run JSON report
#   PYTHON                  Python interpreter (must be 3.11+)
#   GO                      Go toolchain
#
# Exit status is 0 only when every automated check passes, the pinned revision
# is exact, all 1,100 fixture rows are accounted for, and the default runner
# still reports outstanding manual audits with exit code 3.
set -euo pipefail

SPECS_REPO="${PIKAID_SPECS_REPO:-https://github.com/pikaid/pikaid-specs.git}"
SPECS_SHA="${PIKAID_SPECS_SHA:-059780cc0e5b113443812720d95c630649bf3aa3}"
PACKAGE_REVISION="1.0.0"
SPEC_REVISION="1.0.4"
EXPECTED_ROWS=1100
EXPECTED_NOT_APPLICABLE=2

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
GO="${GO:-go}"
WORK=""
CLEANUP_WORK=0
CLEANUP_SPECS=0

fail() {
  printf 'conformance: ERROR: %s\n' "$*" >&2
  exit 1
}

note() {
  printf 'conformance: %s\n' "$*" >&2
}

cleanup() {
  if [[ "$CLEANUP_WORK" == "1" && -n "$WORK" && -d "$WORK" ]]; then
    rm -rf "$WORK"
  fi
}
trap cleanup EXIT

# --- Python 3.11+ -----------------------------------------------------------

if [[ -n "${PYTHON:-}" ]]; then
  PY_CANDIDATES=("$PYTHON")
else
  PY_CANDIDATES=(python3.14 python3.13 python3.12 python3.11 python3)
fi
PY=""
for candidate in "${PY_CANDIDATES[@]}"; do
  if command -v "$candidate" >/dev/null 2>&1 &&
    "$candidate" -c 'import sys; raise SystemExit(0 if sys.version_info >= (3, 11) else 1)' >/dev/null 2>&1; then
    PY=$(command -v "$candidate")
    break
  fi
done
[[ -n "$PY" ]] || fail "Python 3.11 or newer is required (set PYTHON to an interpreter)"
note "python: $("$PY" --version 2>&1)"

command -v "$GO" >/dev/null 2>&1 || fail "Go toolchain not found (set GO)"
command -v node >/dev/null 2>&1 || fail "Node.js is required for verify_oracle_node.mjs"
note "node: $(node --version)"

# --- Scratch space ----------------------------------------------------------

if [[ -n "${PIKAID_CONFORMANCE_WORKDIR:-}" ]]; then
  WORK="$PIKAID_CONFORMANCE_WORKDIR"
  mkdir -p "$WORK"
else
  WORK=$(mktemp -d "${TMPDIR:-/tmp}/pikaid-conformance.XXXXXX")
  CLEANUP_WORK=1
fi
note "workdir: $WORK"

# --- Pinned suite checkout --------------------------------------------------

if [[ -n "${PIKAID_SPECS_DIR:-}" ]]; then
  SPECS="$PIKAID_SPECS_DIR"
  [[ -d "$SPECS/.git" ]] || fail "PIKAID_SPECS_DIR is not a git checkout: $SPECS"
else
  SPECS="$WORK/pikaid-specs"
  rm -rf "$SPECS"
  git clone --quiet --no-tags "$SPECS_REPO" "$SPECS"
  CLEANUP_SPECS=1
fi

HEAD_SHA=$(git -C "$SPECS" rev-parse HEAD)
[[ "$HEAD_SHA" == "$SPECS_SHA" ]] || fail "specs checkout is $HEAD_SHA, expected pinned $SPECS_SHA"
if ! git -C "$SPECS" diff --quiet --ignore-submodules; then
  fail "specs checkout has uncommitted changes"
fi
note "specs revision: $HEAD_SHA"

CONFORMANCE="$SPECS/conformance"
for required in \
  manifest.json \
  SHA256SUMS \
  cases/valid.json \
  cases/invalid-string.json \
  cases/invalid-binary.json \
  cases/ordering.json \
  cases/generation.json \
  cases/properties.json \
  tools/verify_vectors.py \
  tools/run_adapter.py \
  tools/verify_oracle_node.mjs \
  tools/requirements.txt; do
  [[ -f "$CONFORMANCE/$required" ]] || fail "pinned suite is missing $required"
done

# --- Python environment -----------------------------------------------------

if [[ -n "${PIKAID_CONFORMANCE_VENV:-}" ]]; then
  VENV="$PIKAID_CONFORMANCE_VENV"
  [[ -x "$VENV/bin/python" ]] || fail "PIKAID_CONFORMANCE_VENV has no bin/python: $VENV"
else
  VENV="$WORK/venv"
  "$PY" -m venv "$VENV"
fi
"$VENV/bin/python" -m pip install --quiet --disable-pip-version-check \
  -r "$CONFORMANCE/tools/requirements.txt"
"$VENV/bin/python" -c 'import jsonschema' || fail "jsonschema is not importable in $VENV"
PY="$VENV/bin/python"

# --- Adapter build ----------------------------------------------------------

ADAPTER="$WORK/conformance-adapter"
"$GO" build -trimpath -o "$ADAPTER" "$ROOT/cmd/conformance-adapter"
note "adapter: $ADAPTER"

# --- Official suite self-check ---------------------------------------------

pushd "$CONFORMANCE" >/dev/null

"$PY" tools/verify_vectors.py \
  --expect-status OFFICIAL \
  --expect-package-revision "$PACKAGE_REVISION" \
  --expect-spec-revision "$SPEC_REVISION"

PYTHONDONTWRITEBYTECODE=1 "$PY" -m unittest discover -s tools/tests -v

node tools/verify_oracle_node.mjs

# --- Adapter conformance ----------------------------------------------------

AUTOMATED_REPORT="$WORK/adapter-report.json"
"$PY" tools/run_adapter.py --automated-only \
  --report-json "$AUTOMATED_REPORT" -- "$ADAPTER"

DEFAULT_REPORT="$WORK/adapter-report-default.json"
set +e
"$PY" tools/run_adapter.py \
  --report-json "$DEFAULT_REPORT" -- "$ADAPTER" >"$WORK/default-run.log" 2>&1
DEFAULT_EXIT=$?
set -e
cat "$WORK/default-run.log"
if [[ "$DEFAULT_EXIT" -ne 3 ]]; then
  fail "default runner exit = $DEFAULT_EXIT, expected 3 (manual audits outstanding)"
fi
note "default runner exit 3 confirmed: manual audits still outstanding"

# --- Negative control: the suite must reject a fixture-blind mock -----------
# A bridge that claims the mandatory capabilities and returns canned values
# would pass a vacuous suite. It must fail, which also proves the run is not a
# skip or an empty result.

NEGATIVE_STUB="$WORK/negative-control.py"
cat >"$NEGATIVE_STUB" <<'PY'
import json
import sys

CANONICAL = "0" * 26
BINARY = "00" * 17

for line in sys.stdin:
    try:
        request = json.loads(line)
    except Exception:
        continue
    request_id = request.get("request_id")
    operation = request.get("operation")
    if operation == "hello":
        value = {
            "protocol_version": 1,
            "capabilities": ["generate_string", "text_to_binary", "binary_to_text"],
        }
    elif operation == "generate_string":
        value = CANONICAL
    elif operation == "text_to_binary":
        value = BINARY
    elif operation == "binary_to_text":
        value = CANONICAL
    else:
        print(json.dumps({
            "request_id": request_id,
            "ok": False,
            "error": {"category": "unsupported_operation", "message": "negative control"},
        }), flush=True)
        continue
    print(json.dumps({"request_id": request_id, "ok": True, "value": value}), flush=True)
PY

set +e
"$PY" tools/run_adapter.py --automated-only \
  --report-json "$WORK/negative-report.json" -- "$PY" "$NEGATIVE_STUB" \
  >"$WORK/negative-control.log" 2>&1
NEGATIVE_EXIT=$?
set -e
if [[ "$NEGATIVE_EXIT" -eq 0 ]]; then
  fail "negative control adapter was accepted; the suite is not discriminating"
fi
note "negative control rejected (exit $NEGATIVE_EXIT)"

popd >/dev/null

# --- Report assertions ------------------------------------------------------

"$PY" - "$AUTOMATED_REPORT" "$DEFAULT_REPORT" <<'PY'
import json
import sys

EXPECTED_ROWS = 1100
EXPECTED_NOT_APPLICABLE = 2
AUDITS = {"P-GENERATE-RNG-SOURCE", "P-GENERATE-RNG-FAILURE"}


def load(path):
    with open(path, encoding="utf-8") as handle:
        return json.load(handle)


def check(report, path, status):
    assert report["status"] == status, (path, report["status"], status)
    assert report["automated_passed"] is True, path
    assert report["errors"] == [], (path, report["errors"])
    assert report["fatal"] is None, (path, report["fatal"])
    counts = report["counts"]
    total = sum(item["passed"] + item["failed"] + item["not_applicable"] for item in counts.values())
    assert total == EXPECTED_ROWS, (path, total)
    failed = sum(item["failed"] for item in counts.values())
    not_applicable = sum(item["not_applicable"] for item in counts.values())
    assert failed == 0, (path, failed)
    assert not_applicable == EXPECTED_NOT_APPLICABLE, (path, not_applicable)
    audits = {audit["id"] for audit in report["audits"]}
    assert audits == AUDITS, (path, audits)
    assert all(audit["status"] == "MANUAL_EVIDENCE_REQUIRED" for audit in report["audits"]), path
    freshness = report["generation_freshness"]
    assert freshness["distinct"] >= freshness["minimum_required"], (path, freshness)
    return total, not_applicable


automated = load(sys.argv[1])
check(automated, sys.argv[1], "AUTOMATED_PASS_AUDIT_NOT_PERFORMED")
assert automated["exit_code"] == 0
assert automated["automated_only"] is True
assert automated["outstanding_audits"] == ["P-GENERATE-RNG-SOURCE", "P-GENERATE-RNG-FAILURE"]

default = load(sys.argv[2])
check(default, sys.argv[2], "AUTOMATED_PASS_AUDIT_REQUIRED")
assert default["exit_code"] == 3
assert default["automated_only"] is False

print(f"conformance: report assertions passed ({EXPECTED_ROWS} rows, "
      f"{EXPECTED_NOT_APPLICABLE} audited not-applicable)")
PY

if [[ -n "${PIKAID_CONFORMANCE_REPORT_JSON:-}" ]]; then
  mkdir -p "$(dirname "$PIKAID_CONFORMANCE_REPORT_JSON")"
  cp "$AUTOMATED_REPORT" "$PIKAID_CONFORMANCE_REPORT_JSON"
  note "report: $PIKAID_CONFORMANCE_REPORT_JSON"
fi

note "PASS: pinned suite, adapter, and manual-audit gate verified"
