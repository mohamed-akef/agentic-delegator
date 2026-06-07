#!/usr/bin/env bash
set -euo pipefail

: "${JOB_ID:?JOB_ID required}"
: "${REPO:?REPO required}"
: "${BASE_BRANCH:?BASE_BRANCH required}"
: "${WORK_BRANCH:?WORK_BRANCH required}"
: "${SPEC_TYPE:?SPEC_TYPE required}"
: "${SPEC_VALUE:?SPEC_VALUE required}"
MODEL_OVERRIDE="${MODEL_OVERRIDE:-}"

# Secrets are delivered via a read-only bind mount, not -e env vars. Fail fast
# if the mount is absent (e.g. an orchestrator too old to provide it).
SECRETS_DIR=/run/delegator-secrets
GH_TOKEN_FILE="$SECRETS_DIR/gh-token"
ANTHROPIC_KEY_FILE="$SECRETS_DIR/anthropic-key"
if ! { [ -r "$GH_TOKEN_FILE" ] && [ -r "$ANTHROPIC_KEY_FILE" ]; }; then
    echo "[delegator] missing secrets mount at $SECRETS_DIR (is the orchestrator new enough?)" >&2
    exit 3
fi

cd /workspace

# Configure git identity for the commits we'll make
git config --global user.email "agentic-delegator@local"
git config --global user.name "agentic-delegator"

# git auth via a transient GIT_ASKPASS that reads the token file on demand, so
# the token never enters the clone URL or .git/config. credential.helper=""
# disables any on-disk credential store. GIT_ASKPASS is inherited by the
# subsequent fetch / checkout / push.
cat > /tmp/askpass.sh <<EOF
#!/usr/bin/env bash
case "\$1" in
    *Username*) printf '%s' "x-access-token" ;;
    *Password*) printf '%s' "\$(cat "$GH_TOKEN_FILE")" ;;
esac
EOF
chmod 0700 /tmp/askpass.sh
export GIT_ASKPASS=/tmp/askpass.sh
git config --global credential.helper ""

echo "[delegator] cloning $REPO …"
git clone "https://github.com/${REPO}.git" repo
cd repo

# Authenticate gh from the token file — used by the agent's `gh pr create` and
# the safety-net `gh pr view` below. GH_TOKEN/GITHUB_TOKEN must be unset: when
# present they take precedence and bypass the stored credential. Fail fast — gh
# validates the token against api.github.com, and a 401 here would otherwise
# leave every later gh command silently unauthenticated.
unset GH_TOKEN GITHUB_TOKEN
if ! gh auth login --git-protocol https --hostname github.com --with-token < "$GH_TOKEN_FILE"; then
    echo "[delegator] gh auth login failed" >&2
    exit 4
fi

# Either continue an existing branch (fetch + checkout) OR create from base.
if git fetch origin "${WORK_BRANCH}" 2>/dev/null && git rev-parse --verify "origin/${WORK_BRANCH}" >/dev/null 2>&1; then
    echo "[delegator] continuing existing branch ${WORK_BRANCH}"
    git checkout -B "${WORK_BRANCH}" "origin/${WORK_BRANCH}"
else
    echo "[delegator] creating new branch ${WORK_BRANCH} from ${BASE_BRANCH}"
    git checkout "${BASE_BRANCH}"
    git checkout -b "${WORK_BRANCH}"
fi

# Per-repo config: .agentic-delegator.yml at the repo root (all keys optional).
#   model, max_turns, system_prompt_append, allowed_tools[], notification_webhook
CFG=".agentic-delegator.yml"
MODEL="${MODEL_OVERRIDE}"
MAX_TURNS=""
SYS_APPEND=""
ALLOWED_TOOLS=""
if [ -f "${CFG}" ]; then
    echo "[delegator] reading ${CFG}"
    [ -z "${MODEL}" ] && MODEL="$(yq -r '.model // ""' "${CFG}")"
    MAX_TURNS="$(yq -r '.max_turns // ""' "${CFG}")"
    SYS_APPEND="$(yq -r '.system_prompt_append // ""' "${CFG}")"
    ALLOWED_TOOLS="$(yq -r '(.allowed_tools // []) | join(",")' "${CFG}")"
    NOTIFY="$(yq -r '.notification_webhook // ""' "${CFG}")"
    # Hand the webhook URL back to the orchestrator, which fires it on completion.
    [ -n "${NOTIFY}" ] && printf '%s' "${NOTIFY}" > /workspace/.notification-webhook
fi

# Resolve the spec to a single string we feed to Claude.
case "${SPEC_TYPE}" in
    inline) SPEC_TEXT="${SPEC_VALUE}" ;;
    path)
        # Constrain SPEC_TYPE=path to within the cloned repo: reject absolute
        # paths, ".." traversal, and symlink escape — otherwise a job submitter
        # could read the mounted secrets (e.g. /run/delegator-secrets/gh-token)
        # straight into the prompt. cwd is the repo root here.
        repo_root="$(pwd -P)"
        resolved="$(realpath -m -- "${SPEC_VALUE}" 2>/dev/null || true)"
        case "${resolved}/" in
            "${repo_root}/"*) SPEC_TEXT="$(cat "${resolved}")" ;;
            *) echo "[delegator] SPEC_TYPE=path must resolve inside the repo: ${SPEC_VALUE}" >&2; exit 5 ;;
        esac
        ;;
    url)    SPEC_TEXT="$(curl -fsSL "${SPEC_VALUE}")" ;;
    *)      echo "unknown SPEC_TYPE: ${SPEC_TYPE}" >&2; exit 2 ;;
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

# Assemble claude flags from the per-repo config. The exact flag set may vary
# across Claude Code releases — adjust if the binary in the image rejects them.
CLAUDE_ARGS=(--dangerously-skip-permissions --print)
[ -n "${MODEL}" ]         && CLAUDE_ARGS+=(--model "${MODEL}")
[ -n "${MAX_TURNS}" ]     && CLAUDE_ARGS+=(--max-turns "${MAX_TURNS}")
[ -n "${ALLOWED_TOOLS}" ] && CLAUDE_ARGS+=(--allowedTools "${ALLOWED_TOOLS}")
[ -n "${SYS_APPEND}" ]    && CLAUDE_ARGS+=(--append-system-prompt "${SYS_APPEND}")

echo "[delegator] running claude…"
set +e
# Anthropic key is exported only into the claude process environment (gone from
# docker inspect); read from the mounted file at exec time.
ANTHROPIC_API_KEY="$(cat "$ANTHROPIC_KEY_FILE")" claude "${CLAUDE_ARGS[@]}" "${PROMPT}"
RC=$?
set -e

# As a safety net: if Claude didn't write .pr-url but a PR was opened, try to discover it.
if [ ! -f /workspace/.pr-url ]; then
    if pr_url=$(gh pr view "${WORK_BRANCH}" --json url --jq .url 2>/dev/null); then
        echo "${pr_url}" > /workspace/.pr-url
    fi
fi

exit ${RC}
