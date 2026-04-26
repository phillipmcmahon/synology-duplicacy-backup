#!/bin/sh

set -eu

OWNER="phillipmcmahon"
PROJECT_NUMBER="1"
PROJECT_ID="PVT_kwHOABzcx84BUfOM"
STATUS_FIELD_ID="PVTSSF_lAHOABzcx84BUfOMzhBm_tw"
WORKFLOW_FIELD_ID="PVTSSF_lAHOABzcx84BUfOMzhBm_0o"
STATUS_IN_PROGRESS_OPTION_ID="47fc9ee4"
STATUS_DONE_OPTION_ID="98236657"
WORKFLOW_DONE_OPTION_ID="376027f9"
LIMIT="500"
FIX=0
INPUT_JSON=""

usage() {
    cat <<'EOF'
Usage: ./scripts/project-board-audit.sh [OPTIONS]

Check project board discipline for issue state, status labels, project Status,
and project Workflow.

Rules:
  - Closed issues must be Status=Done and Workflow=Done.
  - Closed issues must not keep status:* workflow labels.
  - Open non-epic issues must not be left in Workflow=Done.
  - status:in-progress must match Status=In Progress and Workflow=In Progress.
  - status:review must match Status=In Progress and Workflow=Review.

Options:
  --owner <login>        GitHub owner for the project (default: phillipmcmahon)
  --project <number>    GitHub project number (default: 1)
  --limit <count>       Maximum project items to fetch (default: 500)
  --fix                 Safely fix closed issue drift only
  --input-json <path>   Read normalized project items from a JSON fixture
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
        --project)
            [ "$#" -ge 2 ] || fail "--project requires a value"
            PROJECT_NUMBER="$2"
            shift 2
            ;;
        --limit)
            [ "$#" -ge 2 ] || fail "--limit requires a value"
            LIMIT="$2"
            shift 2
            ;;
        --fix)
            FIX=1
            shift
            ;;
        --input-json)
            [ "$#" -ge 2 ] || fail "--input-json requires a value"
            INPUT_JSON="$2"
            shift 2
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

require_command jq

project_query='
query($owner:String!, $number:Int!, $cursor:String) {
  user(login:$owner) {
    projectV2(number:$number) {
      items(first:100, after:$cursor) {
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          id
          content {
            __typename
            ... on Issue {
              number
              title
              state
              labels(first:50) {
                nodes {
                  name
                }
              }
            }
          }
          fieldValues(first:30) {
            nodes {
              ... on ProjectV2ItemFieldSingleSelectValue {
                name
                field {
                  ... on ProjectV2SingleSelectField {
                    name
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
'

fetch_items() {
    require_command gh

    tmp="$(mktemp)"
    page="$(mktemp)"
    printf '{"items":[]}\n' >"$tmp"
    cursor=""
    fetched=0

    while [ "$fetched" -lt "$LIMIT" ]; do
        if [ -n "$cursor" ]; then
            gh api graphql -f query="$project_query" -f owner="$OWNER" -F number="$PROJECT_NUMBER" -f cursor="$cursor" >"$page"
        else
            gh api graphql -f query="$project_query" -f owner="$OWNER" -F number="$PROJECT_NUMBER" >"$page"
        fi

        jq '
          {items: [
            .data.user.projectV2.items.nodes[]
            | select(.content.__typename == "Issue")
            | {
                id,
                number: .content.number,
                title: .content.title,
                state: .content.state,
                labels: [.content.labels.nodes[].name],
                status: ((.fieldValues.nodes[]? | select(.field.name == "Status") | .name) // ""),
                workflow: ((.fieldValues.nodes[]? | select(.field.name == "Workflow") | .name) // "")
              }
          ]}
        ' "$page" >"$page.normalized"

        jq -s '{items: (.[0].items + .[1].items)}' "$tmp" "$page.normalized" >"$tmp.next"
        mv "$tmp.next" "$tmp"

        page_count="$(jq '.items | length' "$page.normalized")"
        fetched=$((fetched + page_count))
        has_next="$(jq -r '.data.user.projectV2.items.pageInfo.hasNextPage' "$page")"
        [ "$has_next" = "true" ] || break
        cursor="$(jq -r '.data.user.projectV2.items.pageInfo.endCursor' "$page")"
    done

    cat "$tmp"
    rm -f "$tmp" "$page" "$page.normalized"
}

audit_items() {
    jq -r '
      def has_label($name): (.labels // []) | index($name) != null;
      def status_labels: [.labels[]? | select(startswith("status:"))];
      .items[]
      | . as $item
      | (
          if $item.state == "CLOSED" and (($item.status // "") != "Done" or ($item.workflow // "") != "Done") then
            ["closed-not-done", $item.number, $item.title, ("Status=" + (($item.status // "") | tostring) + ", Workflow=" + (($item.workflow // "") | tostring))]
          else empty end
        ),
        (
          if $item.state == "CLOSED" and ((status_labels | length) > 0) then
            ["closed-has-status-label", $item.number, $item.title, ("labels=" + (status_labels | join(",")))]
          else empty end
        ),
        (
          if $item.state == "OPEN" and (has_label("epic") | not) and (($item.workflow // "") == "Done") then
            ["open-non-epic-done", $item.number, $item.title, ("Status=" + (($item.status // "") | tostring) + ", Workflow=" + (($item.workflow // "") | tostring))]
          else empty end
        ),
        (
          if $item.state == "OPEN" and has_label("status:in-progress") and (($item.status // "") != "In Progress" or ($item.workflow // "") != "In Progress") then
            ["label-field-mismatch", $item.number, $item.title, "status:in-progress requires Status=In Progress and Workflow=In Progress"]
          else empty end
        ),
        (
          if $item.state == "OPEN" and has_label("status:review") and (($item.status // "") != "In Progress" or ($item.workflow // "") != "Review") then
            ["label-field-mismatch", $item.number, $item.title, "status:review requires Status=In Progress and Workflow=Review"]
          else empty end
        )
      | @tsv
    '
}

fix_closed_drift() {
    require_command gh

    jq -r '
      .items[]
      | select(.state == "CLOSED")
      | select((.status // "") != "Done" or (.workflow // "") != "Done" or ((.labels // []) | any(startswith("status:"))))
      | [.id, .number, ((.labels // []) | map(select(startswith("status:"))) | join(","))]
      | @tsv
    ' "$1" | while IFS="$(printf '\t')" read -r item_id number labels; do
        [ -n "$item_id" ] || continue
        gh api graphql \
            -f query='mutation($project:ID!, $item:ID!, $statusField:ID!, $status:String!, $workflowField:ID!, $workflow:String!) { setStatus: updateProjectV2ItemFieldValue(input:{projectId:$project,itemId:$item,fieldId:$statusField,value:{singleSelectOptionId:$status}}) { projectV2Item { id } } setWorkflow: updateProjectV2ItemFieldValue(input:{projectId:$project,itemId:$item,fieldId:$workflowField,value:{singleSelectOptionId:$workflow}}) { projectV2Item { id } } }' \
            -f project="$PROJECT_ID" \
            -f item="$item_id" \
            -f statusField="$STATUS_FIELD_ID" \
            -f status="$STATUS_DONE_OPTION_ID" \
            -f workflowField="$WORKFLOW_FIELD_ID" \
            -f workflow="$WORKFLOW_DONE_OPTION_ID" >/dev/null

        if [ -n "$labels" ]; then
            old_ifs="$IFS"
            IFS=","
            for label in $labels; do
                [ -n "$label" ] || continue
                gh issue edit "$number" --remove-label "$label" >/dev/null
            done
            IFS="$old_ifs"
        fi
    done
}

items_file="$(mktemp)"
violations_file="$(mktemp)"
cleanup() {
    rm -f "$items_file" "$violations_file"
}
trap cleanup EXIT INT TERM

if [ -n "$INPUT_JSON" ]; then
    [ -f "$INPUT_JSON" ] || fail "input JSON not found: $INPUT_JSON"
    cp "$INPUT_JSON" "$items_file"
else
    fetch_items >"$items_file"
fi

if [ "$FIX" -eq 1 ]; then
    [ -z "$INPUT_JSON" ] || fail "--fix cannot be used with --input-json"
    fix_closed_drift "$items_file"
    fetch_items >"$items_file"
fi

audit_items <"$items_file" >"$violations_file"

if [ -s "$violations_file" ]; then
    echo "Project board audit failed:" >&2
    column -t -s "$(printf '\t')" "$violations_file" >&2 2>/dev/null || cat "$violations_file" >&2
    exit 1
fi

echo "Project board audit passed"
