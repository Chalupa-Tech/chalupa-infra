#!/usr/bin/env bash
# Tests the "Auto-promote and detect version" step of
# .github/workflows/release-reusable.yml against throwaway git repos.
#
# The step's shell is extracted from the workflow (so the test runs what
# CI runs) and executed with a stub `gh`, a local bare repo as `origin`,
# and temp files for GITHUB_OUTPUT / GITHUB_STEP_SUMMARY.
#
# Usage: scripts/ci-tests/release-detect-version-test.sh
# Needs: bash, git, python3 with PyYAML. Set TMPDIR to a writable dir.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
WF="$ROOT/.github/workflows/release-reusable.yml"
WORK=$(mktemp -d "${TMPDIR:-/tmp}/release-test.XXXXXX")
trap 'rm -rf "$WORK"' EXIT

python3 - "$WF" > "$WORK/step.sh" <<'PY'
import sys, yaml
wf = yaml.safe_load(open(sys.argv[1]))
steps = [s for s in wf["jobs"]["release"]["steps"] if s.get("id") == "detect_version"]
if len(steps) != 1:
    sys.exit("expected exactly one step with id detect_version")
print(steps[0]["run"])
PY

mkdir -p "$WORK/bin"
cat > "$WORK/bin/gh" <<'EOF'
#!/usr/bin/env bash
echo "gh $*" >> "$GH_LOG"
case "$1 $2" in
  "pr list")
    if [[ "$*" == *"--head"* ]]; then echo "${GH_EXISTING_PR:-}"; fi ;;
  "pr create") echo "https://github.com/o/r/pull/42" ;;
esac
exit 0
EOF
chmod +x "$WORK/bin/gh"

PASS=0
FAIL=0
CASE_DIR=""

# new_case <name>: fresh repo with v0.13.0 tagged and an empty [Unreleased].
new_case() {
  CASE_DIR="$WORK/$1"
  mkdir -p "$CASE_DIR"
  git init -q --bare "$CASE_DIR/origin.git"
  git init -q -b main "$CASE_DIR/repo"
  cd "$CASE_DIR/repo"
  git config user.email t@example.com
  git config user.name test
  git remote add origin "$CASE_DIR/origin.git"
  changelog "" "## [0.13.0] - 2026-05-18
- old"
  git add -A
  git commit -qm "release: v0.13.0"
  git tag -a v0.13.0 -m v0.13.0
}

# changelog <unreleased body> <rest>
changelog() {
  printf '# Changelog\n\n## [Unreleased]\n%s\n\n%s\n' "$1" "$2" > CHANGELOG.md
}

commit() { git add -A; git commit -q --allow-empty -m "$1"; }

run_step() {
  : > "$CASE_DIR/out"; : > "$CASE_DIR/sum"; : > "$CASE_DIR/gh"
  set +e
  GITHUB_OUTPUT="$CASE_DIR/out" GITHUB_STEP_SUMMARY="$CASE_DIR/sum" \
    GH_LOG="$CASE_DIR/gh" REPO=o/r PATH="$WORK/bin:$PATH" \
    bash -e "$WORK/step.sh" > "$CASE_DIR/log" 2>&1
  RC=$?
  set -e
}

check() {
  local desc="$1"; shift
  if "$@"; then PASS=$((PASS + 1)); else
    FAIL=$((FAIL + 1)); echo "FAIL [$(basename "$CASE_DIR")] $desc"
    sed 's/^/    /' "$CASE_DIR/log"
  fi
}
has() { grep -qF -- "$2" "$CASE_DIR/$1"; }
lacks() { ! grep -qF -- "$2" "$CASE_DIR/$1"; }
rc_is() { [[ "$RC" == "$1" ]]; }

# 1. Untyped squash subject with [Unreleased] entries: warn, no release.
new_case untyped
changelog "### Added
- thing" "## [0.13.0] - 2026-05-18"
commit "session 176: strategy-research phase-3b (#90)"
run_step
check "exit 0" rc_is 0
check "skip" has out "skip=true"
check "warning annotation" has log "::warning title=Unreleased changes not released::"
check "summary says why" has sum "None of the 1 commit(s) since v0.13.0"

# 2. feat commit: promote through a PR, never push main.
new_case feat
changelog "### Added
- thing" "## [0.13.0] - 2026-05-18"
commit "feat(book): thing (#91)"
run_step
check "exit 0" rc_is 0
check "this run cuts nothing" has out "skip=true"
check "PR created" has gh "gh pr create -R o/r --title chore: promote Unreleased to v0.14.0"
check "auto-merge" has gh "--squash --auto --delete-branch"
check "branch pushed" bash -c "git --git-dir='$CASE_DIR/origin.git' rev-parse -q --verify refs/heads/ci/promote-v0.14.0 >/dev/null"
check "main not pushed" bash -c "! git --git-dir='$CASE_DIR/origin.git' rev-parse -q --verify refs/heads/main"
check "changelog promoted on branch" bash -c \
  "git --git-dir='$CASE_DIR/origin.git' show ci/promote-v0.14.0:CHANGELOG.md | grep -q '^## \[0.14.0\] - '"

# 3. Existing open promotion PR is reused, not duplicated.
new_case reuse
changelog "- fix" "## [0.13.0] - 2026-05-18"
commit "fix: thing (#92)"
GH_EXISTING_PR=https://github.com/o/r/pull/7 run_step
check "exit 0" rc_is 0
check "no second PR" lacks gh "gh pr create"
check "merges the existing PR" has gh "gh pr merge https://github.com/o/r/pull/7"

# 4. The squash-merged promotion commit releases its version.
new_case promoted
changelog "" "## [0.14.0] - 2026-11-16
- thing

## [0.13.0] - 2026-05-18"
commit "chore: promote Unreleased to v0.14.0 (#42)"
run_step
check "exit 0" rc_is 0
check "release" has out "skip=false"
check "version" has out "version=0.14.0"
check "no gh calls" lacks gh "gh pr"

# 5. Promotion subject without its changelog section fails loudly.
new_case promoted-missing
commit "chore: promote Unreleased to v0.14.0 (#42)"
run_step
check "exit 1" rc_is 1
check "error annotation" has log "::error::HEAD is 'chore: promote Unreleased to v0.14.0 (#42)'"

# 6. Tier 1 explicit version is unchanged.
new_case tier1
printf '# Changelog\n\n## [0.15.0] - 2026-11-16\n- x\n' > CHANGELOG.md
commit "release: v0.15.0"
run_step
check "exit 0" rc_is 0
check "release" has out "skip=false"
check "version" has out "version=0.15.0"

# 7. A merge after the promotion commit, before its tag: no double promotion.
new_case after-promotion
changelog "" "## [0.14.0] - 2026-11-16
- thing

## [0.13.0] - 2026-05-18"
commit "feat: thing (#91)"
commit "chore: promote Unreleased to v0.14.0 (#42)"
changelog "- later" "## [0.14.0] - 2026-11-16
- thing

## [0.13.0] - 2026-05-18"
commit "fix: later (#93)"
run_step
check "exit 0" rc_is 0
check "skip" has out "skip=true"
check "no PR" lacks gh "gh pr create"
check "says already promoted" has sum "v0.14.0 is promoted in CHANGELOG.md and waits for its tag"

# 7b. A stronger bump after a pending promotion: a feat after a promoted
# (untagged) fix must not open a second promotion for v0.14.0.
new_case stronger-after-promotion
changelog "" "## [0.13.1] - 2026-11-16
- fix

## [0.13.0] - 2026-05-18"
commit "fix: thing (#91)"
commit "chore: promote Unreleased to v0.13.1 (#42)"
changelog "- new" "## [0.13.1] - 2026-11-16
- fix

## [0.13.0] - 2026-05-18"
commit "feat: new (#94)"
run_step
check "exit 0" rc_is 0
check "skip" has out "skip=true"
check "no PR" lacks gh "gh pr create"
check "names the pending version" has sum "v0.13.1 is promoted in CHANGELOG.md and waits for its tag"

# 7c. No tags yet, CHANGELOG already has numbered sections, no promotion
# commit: the first Tier-2 release must still be promoted.
CASE_DIR="$WORK/tagless"; mkdir -p "$CASE_DIR"
git init -q --bare "$CASE_DIR/origin.git"
git init -q -b main "$CASE_DIR/repo"; cd "$CASE_DIR/repo"
git config user.email t@example.com; git config user.name test
git remote add origin "$CASE_DIR/origin.git"
changelog "- new" "## [0.1.0] - 2026-01-01
- old"
commit "feat: first (#1)"
run_step
check "exit 0" rc_is 0
check "promotion PR opened" has gh "gh pr create"
check "no deferral" lacks sum "waits for its tag"

# 8. No commits since the tag: notice only.
new_case nothing
run_step
check "exit 0" rc_is 0
check "skip" has out "skip=true"
check "notice, not warning" has log "::notice title=No release::No commits since v0.13.0."

echo "release-detect-version: $PASS passed, $FAIL failed"
[[ "$FAIL" == 0 ]]
