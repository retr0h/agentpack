#!/usr/bin/env bash
# End-to-end smoke test for agentpack add/del lifecycle.
# Run from the examples/sandbox/ directory:
#   cd examples/sandbox && bash test-lifecycle.sh
set -euo pipefail

AGENTPACK="go run ../../main.go"
PASS=0
FAIL=0

pass() { PASS=$((PASS + 1)); printf "  \033[32m✓\033[0m %s\n" "$1"; }
fail() { FAIL=$((FAIL + 1)); printf "  \033[31m✗\033[0m %s\n" "$1"; }

assert_file_exists() {
  if [[ -f "$1" ]]; then pass "$2"; else fail "$2 (missing: $1)"; fi
}

assert_file_missing() {
  if [[ ! -f "$1" ]]; then pass "$2"; else fail "$2 (still exists: $1)"; fi
}

assert_yaml_contains() {
  if grep -q "$1" agentpack-packages.yaml 2>/dev/null; then
    pass "$2"
  else
    fail "$2 (grep '$1' failed in yaml)"
  fi
}

assert_yaml_not_contains() {
  if ! grep -q "$1" agentpack-packages.yaml 2>/dev/null; then
    pass "$2"
  else
    fail "$2 (grep '$1' unexpectedly found in yaml)"
  fi
}

assert_lock_contains() {
  if grep -q "$1" agentpack.lock 2>/dev/null; then
    pass "$2"
  else
    fail "$2 (grep '$1' failed in lock)"
  fi
}

assert_lock_not_contains() {
  if ! grep -q "$1" agentpack.lock 2>/dev/null; then
    pass "$2"
  else
    fail "$2 (grep '$1' unexpectedly found in lock)"
  fi
}

assert_dir_exists() {
  if [[ -d "$1" ]]; then pass "$2"; else fail "$2 (missing dir: $1)"; fi
}

assert_dir_missing() {
  if [[ ! -d "$1" ]]; then pass "$2"; else fail "$2 (still exists: $1)"; fi
}

# ---------------------------------------------------------------------------
# Clean slate
# ---------------------------------------------------------------------------
rm -rf .claude .agents .codex .copilot .config agentpack-packages.yaml agentpack.lock
printf "\n\033[1m=== agentpack lifecycle smoke test ===\033[0m\n\n"

# ---------------------------------------------------------------------------
# Step 1: Add single skill to claude-code
# ---------------------------------------------------------------------------
printf "\033[1mStep 1: add @kubernetes-specialist → claude-code\033[0m\n"
$AGENTPACK add jeffallan/claude-skills@kubernetes-specialist --target claude-code 2>&1

assert_file_exists agentpack-packages.yaml "yaml created"
assert_file_exists agentpack.lock "lock created"
assert_yaml_contains "jeffallan/claude-skills" "yaml has package name"
assert_yaml_contains "kubernetes-specialist" "yaml has skill"
assert_yaml_contains "claude-code" "yaml has target"
assert_lock_contains "jeffallan/claude-skills" "lock has package name"
assert_lock_contains "kubernetes-specialist" "lock has skill"
assert_dir_exists .claude/skills/kubernetes-specialist "skill dir installed"

echo ""

# ---------------------------------------------------------------------------
# Step 2: Add second skill to different target (merge)
# ---------------------------------------------------------------------------
printf "\033[1mStep 2: add @react-expert → cursor (merge)\033[0m\n"
$AGENTPACK add jeffallan/claude-skills@react-expert --target cursor 2>&1

assert_yaml_contains "kubernetes-specialist" "yaml still has first skill"
assert_yaml_contains "react-expert" "yaml has second skill"
assert_yaml_contains "cursor" "yaml has cursor target"
assert_lock_contains "react-expert" "lock has second skill"
assert_dir_exists .agents/skills "cursor skill dir installed"

echo ""

# ---------------------------------------------------------------------------
# Step 3: Add a second package
# ---------------------------------------------------------------------------
printf "\033[1mStep 3: add microsoft/azure-skills@azure-kubernetes → claude-code\033[0m\n"
$AGENTPACK add microsoft/azure-skills@azure-kubernetes --target claude-code 2>&1

assert_yaml_contains "microsoft/azure-skills" "yaml has second package"
assert_lock_contains "microsoft/azure-skills" "lock has second package"
assert_dir_exists .claude/skills/azure-kubernetes "azure skill dir installed"

echo ""

# ---------------------------------------------------------------------------
# Step 4: List
# ---------------------------------------------------------------------------
printf "\033[1mStep 4: list\033[0m\n"
OUTPUT=$($AGENTPACK ls 2>&1)
echo "$OUTPUT"

if echo "$OUTPUT" | grep -q "jeffallan/claude-skills"; then
  pass "ls shows first package"
else
  fail "ls missing first package"
fi

if echo "$OUTPUT" | grep -q "microsoft/azure-skills"; then
  pass "ls shows second package"
else
  fail "ls missing second package"
fi

echo ""

# ---------------------------------------------------------------------------
# Step 5: Info
# ---------------------------------------------------------------------------
printf "\033[1mStep 5: info\033[0m\n"
INFO=$($AGENTPACK info jeffallan/claude-skills 2>&1)

if echo "$INFO" | grep -q "kubernetes-specialist"; then
  pass "info shows kubernetes-specialist"
else
  fail "info missing kubernetes-specialist"
fi

echo ""

# ---------------------------------------------------------------------------
# Step 6: Partial delete
# ---------------------------------------------------------------------------
printf "\033[1mStep 6: del @react-expert (partial)\033[0m\n"
$AGENTPACK del jeffallan/claude-skills@react-expert 2>&1

assert_yaml_contains "kubernetes-specialist" "yaml still has remaining skill"
assert_yaml_not_contains "react-expert" "yaml no longer has deleted skill"
assert_lock_not_contains "react-expert" "lock no longer has deleted skill"
assert_yaml_contains "jeffallan/claude-skills" "yaml still has package entry"

echo ""

# ---------------------------------------------------------------------------
# Step 7: Full delete of first package
# ---------------------------------------------------------------------------
printf "\033[1mStep 7: del jeffallan/claude-skills (full)\033[0m\n"
$AGENTPACK del jeffallan/claude-skills 2>&1

assert_yaml_not_contains "jeffallan/claude-skills" "yaml no longer has first package"
assert_yaml_contains "microsoft/azure-skills" "yaml still has second package"
assert_lock_not_contains "jeffallan/claude-skills" "lock no longer has first package"
assert_lock_contains "microsoft/azure-skills" "lock still has second package"

echo ""

# ---------------------------------------------------------------------------
# Step 8: Full delete of second package
# ---------------------------------------------------------------------------
printf "\033[1mStep 8: del microsoft/azure-skills (full)\033[0m\n"
$AGENTPACK del microsoft/azure-skills 2>&1

assert_yaml_not_contains "microsoft/azure-skills" "yaml empty of packages"
assert_lock_not_contains "microsoft/azure-skills" "lock empty of packages"

echo ""

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
TOTAL=$((PASS + FAIL))
printf "\n\033[1m=== Results: %d/%d passed ===" "$PASS" "$TOTAL"
if [[ $FAIL -gt 0 ]]; then
  printf " (\033[31m%d failed\033[0m)" "$FAIL"
fi
printf "\033[0m\n\n"

# Clean up
rm -rf .claude .agents .codex .copilot .config agentpack-packages.yaml agentpack.lock

exit "$FAIL"
