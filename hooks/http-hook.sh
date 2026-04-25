#!/bin/bash
# Minimal HTTP Hook - Calls the escalation service instead of bash logic
# All logic is in the Go service: detection, escalation, logging, stats
# This replaces: de-escalate-model.sh, auto-effort.sh, track-escalation-patterns.sh

set -o pipefail

SERVICE_URL="${ESCALATION_SERVICE_URL:-http://localhost:9000}"
TIMEOUT=5

# Read prompt from stdin
read -r PROMPT

# Call the service hook endpoint
RESPONSE=$(curl -s -m $TIMEOUT -X POST \
  -H "Content-Type: application/json" \
  -d "{\"prompt\":\"$PROMPT\"}" \
  "$SERVICE_URL/api/hook" 2>/dev/null)

# Return the response (continue, suppressOutput, etc.)
echo "$RESPONSE"
