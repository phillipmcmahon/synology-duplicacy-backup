#!/bin/sh

set -eu

OWNER="phillipmcmahon"
REPO="synology-duplicacy-backup"
PROJECT_NUMBER="1"
PROJECT_ID="PVT_kwHOABzcx84BUfOM"
STATUS_FIELD_ID="PVTSSF_lAHOABzcx84BUfOMzhBm_tw"
WORKFLOW_FIELD_ID="PVTSSF_lAHOABzcx84BUfOMzhBm_0o"
STATUS_TODO_OPTION_ID="f75ad846"
STATUS_IN_PROGRESS_OPTION_ID="47fc9ee4"
STATUS_DONE_OPTION_ID="98236657"
WORKFLOW_READY_OPTION_ID="10cbb837"
WORKFLOW_IN_PROGRESS_OPTION_ID="1f1c0582"
WORKFLOW_REVIEW_OPTION_ID="86795c9d"
WORKFLOW_DONE_OPTION_ID="376027f9"
ISSUE=""
STAGE=""
CLOSE_ISSUE=0
REOPEN_ISSUE=0

usage() {
    cat <<'EOF'
Usage: ./scripts/project-transition.sh --issue <number> --stage <stage> [OPTIONS]

Move one issue through the project board workflow and keep status labels aligned.

Stages:
  ready          Status=Todo, Workflow=Ready, remove status labels
  in-progress    Status=In Progress, Workflow=In Progress, status:in-progress
  review         Status=Done, Workflow=Review, status:review
  done           Status=Done, Workflow=Done, remove status labels

Options:
  --owner <login>        GitHub owner (default: phillipmcmahon)
  --repo <name>          GitHub repo name (default: synology-duplicacy-backup)
  --project <number>    GitHub project number (default: 1)
  --issue <number>      Issue number to transition
  --stage <stage>       ready, in-progress, review, or done
  --close               Close the issue after setting stage=done
  --reopen              Reopen the issue before applying a non-done stage
  --help                Show this help text
EOF
}

fail() {
    echo "Error: $*" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --owner)
            [ "$#" -ge 2 ] || fail "--owner requires a value"
            OWNER="$2"
            shift 2
            ;;
        --repo)
            [ "$#" -ge 2 ] || fail "--repo requires a value"
            REPO="$2"
            shift 2
            ;;
        --project)
            [ "$#" -ge 2 ] || fail "--project requires a value"
            PROJECT_NUMBER="$2"
            shift 2
            ;;
        --issue)
            [ "$#" -ge 2 ] || fail "--issue requires a value"
            ISSUE="$2"
            shift 2
            ;;
        --stage)
            [ "$#" -ge 2 ] || fail "--stage requires a value"
            STAGE="$2"
            shift 2
            ;;
        --close)
            CLOSE_ISSUE=1
            shift
            ;;
        --reopen)
            REOPEN_ISSUE=1
            shift
            ;;
        --help)
            usage
            exit 0
            ;;
        *)
            fail "unknown option: $1"
            ;;
    esac
done

[ -n "$ISSUE" ] || fail "--issue is required"
[ -n "$STAGE" ] || fail "--stage is required"
[ "$CLOSE_ISSUE" -eq 0 ] || [ "$STAGE" = "done" ] || fail "--close can only be used with --stage done"

require_command gh
require_command jq

case "$STAGE" in
    ready)
        STATUS_OPTION="$STATUS_TODO_OPTION_ID"
        WORKFLOW_OPTION="$WORKFLOW_READY_OPTION_ID"
        ADD_LABEL=""
        ;;
    in-progress)
        STATUS_OPTION="$STATUS_IN_PROGRESS_OPTION_ID"
        WORKFLOW_OPTION="$WORKFLOW_IN_PROGRESS_OPTION_ID"
        ADD_LABEL="status:in-progress"
        ;;
    review)
        STATUS_OPTION="$STATUS_DONE_OPTION_ID"
        WORKFLOW_OPTION="$WORKFLOW_REVIEW_OPTION_ID"
        ADD_LABEL="status:review"
        ;;
    done)
        STATUS_OPTION="$STATUS_DONE_OPTION_ID"
        WORKFLOW_OPTION="$WORKFLOW_DONE_OPTION_ID"
        ADD_LABEL=""
        ;;
    *)
        fail "--stage must be ready, in-progress, review, or done"
        ;;
esac

ISSUE_URL="https://github.com/$OWNER/$REPO/issues/$ISSUE"
ITEM_ID="$(gh api graphql \
    -f query='query($owner:String!, $repo:String!, $number:Int!) { repository(owner:$owner, name:$repo) { issue(number:$number) { projectItems(first:20) { nodes { id project { id } } } } } }' \
    -f owner="$OWNER" \
    -f repo="$REPO" \
    -F number="$ISSUE" \
    | jq -r --arg project "$PROJECT_ID" '.data.repository.issue.projectItems.nodes[] | select(.project.id == $project) | .id' \
    | head -n 1)"

if [ -z "$ITEM_ID" ]; then
    ITEM_ID="$(gh project item-add "$PROJECT_NUMBER" --owner "$OWNER" --url "$ISSUE_URL" --format json --jq .id)"
fi

gh api graphql \
    -f query='mutation($project:ID!, $item:ID!, $statusField:ID!, $status:String!, $workflowField:ID!, $workflow:String!) { setStatus: updateProjectV2ItemFieldValue(input:{projectId:$project,itemId:$item,fieldId:$statusField,value:{singleSelectOptionId:$status}}) { projectV2Item { id } } setWorkflow: updateProjectV2ItemFieldValue(input:{projectId:$project,itemId:$item,fieldId:$workflowField,value:{singleSelectOptionId:$workflow}}) { projectV2Item { id } } }' \
    -f project="$PROJECT_ID" \
    -f item="$ITEM_ID" \
    -f statusField="$STATUS_FIELD_ID" \
    -f status="$STATUS_OPTION" \
    -f workflowField="$WORKFLOW_FIELD_ID" \
    -f workflow="$WORKFLOW_OPTION" >/dev/null

for label in status:in-progress status:review; do
    if gh issue view "$ISSUE" --repo "$OWNER/$REPO" --json labels --jq '.labels[].name' | grep -Fx "$label" >/dev/null 2>&1; then
        gh issue edit "$ISSUE" --repo "$OWNER/$REPO" --remove-label "$label" >/dev/null
    fi
done

if [ -n "$ADD_LABEL" ]; then
    gh issue edit "$ISSUE" --repo "$OWNER/$REPO" --add-label "$ADD_LABEL" >/dev/null
fi

if [ "$REOPEN_ISSUE" -eq 1 ]; then
    gh issue reopen "$ISSUE" --repo "$OWNER/$REPO" >/dev/null
fi

if [ "$CLOSE_ISSUE" -eq 1 ]; then
    gh issue close "$ISSUE" --repo "$OWNER/$REPO" >/dev/null
fi

echo "Issue #$ISSUE moved to $STAGE"
