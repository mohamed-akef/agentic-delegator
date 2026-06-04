#!/usr/bin/env bash
set -euo pipefail

: "${JOB_ID:?JOB_ID required}"
: "${REPO:?REPO required}"
: "${BASE_BRANCH:?BASE_BRANCH required}"
: "${WORK_BRANCH:?WORK_BRANCH required}"
: "${GH_TOKEN:?GH_TOKEN required}"
: "${ANTHROPIC_API_KEY:?ANTHROPIC_API_KEY required}"
: "${SPEC_TYPE:?SPEC_TYPE required}"
: "${SPEC_VALUE:?SPEC_VALUE required}"

cd /workspace

# Configure git identity for the commits we'll make
git config --global user.email "agentic-delegator@local"
git config --global user.name "agentic-delegator"

echo "[delegator] cloning $REPO …"
git clone "https://x-access-token:${GH_TOKEN}@github.com/${REPO}.git" repo
cd repo

# Either continue an existing branch (fetch + checkout) OR create from base.
if git fetch origin "${WORK_BRANCH}" 2>/dev/null && git rev-parse --verify "origin/${WORK_BRANCH}" >/dev/null 2>&1; then
    echo "[delegator] continuing existing branch ${WORK_BRANCH}"
    git checkout -B "${WORK_BRANCH}" "origin/${WORK_BRANCH}"
else
    echo "[delegator] creating new branch ${WORK_BRANCH} from ${BASE_BRANCH}"
    git checkout "${BASE_BRANCH}"
    git checkout -b "${WORK_BRANCH}"
fi

# Resolve the spec to a single string we feed to Claude.
case "${SPEC_TYPE}" in
    inline) SPEC_TEXT="${SPEC_VALUE}" ;;
    path)   SPEC_TEXT="$(cat "${SPEC_VALUE}")" ;;
    url)    SPEC_TEXT="$(curl -fsSL "${SPEC_VALUE}")" ;;
    *)      echo "unknown SPEC_TYPE: ${SPEC_TYPE}"; exit 2 ;;
esac

PROMPT="$(cat <<EOF
You are agentic-delegator. Implement the following spec on the current git working tree.

Spec:
${SPEC_TEXT}

When done:
1. Stage and commit your changes with a descriptive message.
2. Push the branch '${WORK_BRANCH}' to origin.
3. Open a pull request with 'gh pr create --base ${BASE_BRANCH} --head ${WORK_BRANCH}'.
4. Write the resulting PR URL to /workspace/.pr-url so the orchestrator can pick it up.
EOF
)"

# Run Claude Code in non-interactive mode. The exact flag set may vary across
# Claude Code releases — adjust if the binary in the image rejects them.
echo "[delegator] running claude…"
GH_TOKEN="${GH_TOKEN}" claude --dangerously-skip-permissions --print "${PROMPT}"
RC=$?

# As a safety net: if Claude didn't write .pr-url but a PR was opened, try to discover it.
if [ ! -f /workspace/.pr-url ]; then
    if pr_url=$(gh pr view "${WORK_BRANCH}" --json url --jq .url 2>/dev/null); then
        echo "${pr_url}" > /workspace/.pr-url
    fi
fi

exit ${RC}
