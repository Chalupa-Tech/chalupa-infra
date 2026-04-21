#!/usr/bin/env bash
# phase-56a AI5 mixed-PR validation: does path-filter return true
# when the PR contains both a reviewable file and an exempt file?
# Expected: HAS_REVIEWABLE=true (the code file's presence makes
# the PR reviewable even though CHANGELOG.md is in the diff too).
set -euo pipefail

# Pad to > 20 lines so size gate doesn't fire first.
echo "phase-56 mixed test"
echo "line 1"
echo "line 2"
echo "line 3"
echo "line 4"
echo "line 5"
echo "line 6"
echo "line 7"
echo "line 8"
echo "line 9"
echo "line 10"
echo "line 11"
echo "line 12"
echo "line 13"
echo "line 14"
echo "line 15"
echo "line 16"
echo "line 17"
echo "line 18"
echo "line 19"
echo "line 20"
