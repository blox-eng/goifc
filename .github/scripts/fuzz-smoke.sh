#!/usr/bin/env bash
# Run one coverage-guided fuzz target and decide whether a non-zero exit is a
# finding or infra noise.
#
# `go test -fuzz` fails for two very different reasons. A real crasher writes
# the offending input under testdata/fuzz/ and prints "Failing input written
# to". Separately, when -fuzztime elapses the coordinator cancels its context
# while workers may still be mid-RPC; the run then reports "context deadline
# exceeded", writes no crasher, and means nothing at all about the code. That
# second one turned main red on 128e466 while the identical tree was green on
# the pull request an hour earlier.
#
# ci.yml already states the policy — "a fuzz OOM or timeout is infra noise and
# must not block a merge". This script is what makes the policy true, and it is
# narrow on purpose: only the known shutdown race is forgiven. A crasher, a
# failing seed, or a build error still fails the job.
#
# Usage: fuzz-smoke.sh <package> <FuzzTarget> [fuzztime]
set -uo pipefail

if [[ $# -lt 2 ]]; then
	echo "usage: $0 <package> <FuzzTarget> [fuzztime]" >&2
	exit 2
fi

pkg=$1
target=$2
fuzztime=${3:-60s}

log=$(mktemp)
trap 'rm -f "$log"' EXIT

go test "$pkg" -run "$target" -fuzz "$target" -fuzztime="$fuzztime" 2>&1 | tee "$log"
status=${PIPESTATUS[0]}

if [[ $status -eq 0 ]]; then
	exit 0
fi

# Check for a crasher BEFORE the deadline race: a target can find a crasher and
# then also trip the shutdown race on its way out, and the crasher is the one
# that matters.
if grep -q 'Failing input written to' "$log"; then
	echo "::error::${target} found a crasher. The input is in the fuzz-crashers artifact — commit it to testdata/fuzz/ as a regression case, then fix it."
	exit 1
fi

if grep -q 'context deadline exceeded' "$log"; then
	echo "::warning::${target} hit the -fuzztime shutdown race (golang/go#48504). No crasher was written, so this is not a finding."
	exit 0
fi

echo "::error::${target} failed for a reason that is neither a crasher nor the known shutdown race. Read the log above — do not assume it is flaky."
exit "$status"
