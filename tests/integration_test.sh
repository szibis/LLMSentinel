#!/bin/bash

# Comprehensive integration tests for Claude Escalate Dashboard & Tools
# Tests all API endpoints, dashboard features, and tool management functionality

set -e

BASE_URL="http://localhost:8077"
TESTS_PASSED=0
TESTS_FAILED=0

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test helper function
assert_status() {
    local test_name=$1
    local expected=$2
    local actual=$3
    local response=$4

    if [ "$actual" -eq "$expected" ]; then
        echo -e "${GREEN}✓${NC} $test_name (HTTP $actual)"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗${NC} $test_name (Expected $expected, got $actual)"
        echo "  Response: $response"
        ((TESTS_FAILED++))
    fi
}

assert_contains() {
    local test_name=$1
    local search_string=$2
    local response=$3

    if echo "$response" | grep -q "$search_string"; then
        echo -e "${GREEN}✓${NC} $test_name"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗${NC} $test_name"
        echo "  Expected to find: $search_string"
        echo "  Response: $response"
        ((TESTS_FAILED++))
    fi
}

assert_json_key() {
    local test_name=$1
    local key=$2
    local response=$3

    if echo "$response" | jq -e "$key" > /dev/null 2>&1; then
        echo -e "${GREEN}✓${NC} $test_name"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗${NC} $test_name (Key not found: $key)"
        echo "  Response: $response"
        ((TESTS_FAILED++))
    fi
}

echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}Claude Escalate Integration Test Suite${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}\n"

# ============================================================================
# 1. DASHBOARD HTML TESTS
# ============================================================================
echo -e "${YELLOW}[1] Dashboard HTML Structure Tests${NC}"

response=$(curl -s "$BASE_URL/dashboard" -w "\n%{http_code}")
http_code=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

assert_status "GET /dashboard returns 200" 200 "$http_code" "$body"
assert_contains "Dashboard contains Configuration tab" 'Configuration' "$body"
assert_contains "Dashboard contains Tools tab" 'Tools' "$body"
assert_contains "Dashboard contains Feedback tab" 'Feedback' "$body"
assert_contains "Dashboard contains config-editor element" 'config-editor' "$body"
assert_contains "Dashboard contains config-hints panel" 'config-hints' "$body"
assert_contains "Dashboard has quick-jump buttons" 'quickJump' "$body"
assert_contains "Dashboard has highlightYAMLSyntax function" 'highlightYAMLSyntax' "$body"
assert_contains "Dashboard has getYAMLPath function" 'getYAMLPath' "$body"
assert_contains "Dashboard has loadTools function" 'loadTools' "$body"
echo

# ============================================================================
# 2. API CONFIG TESTS
# ============================================================================
echo -e "${YELLOW}[2] Configuration API Tests${NC}"

response=$(curl -s "$BASE_URL/api/config" -w "\n%{http_code}")
http_code=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

assert_status "GET /api/config returns 200" 200 "$http_code" "$body"
assert_json_key "Config response contains 'config' key" ".config" "$body"
assert_json_key "Config contains 'Gateway' section" ".config.Gateway" "$body"
assert_json_key "Config contains 'Optimizations' section" ".config.Optimizations" "$body"
assert_json_key "Config contains 'Security' section" ".config.Security" "$body"
echo

# ============================================================================
# 3. API CONFIG SPEC TESTS
# ============================================================================
echo -e "${YELLOW}[3] Configuration Specification Tests${NC}"

response=$(curl -s "$BASE_URL/api/config/spec" -w "\n%{http_code}")
http_code=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

assert_status "GET /api/config/spec returns 200" 200 "$http_code" "$body"
assert_json_key "Spec contains 'Sections' key" ".spec.Sections" "$body"
assert_json_key "Spec contains 'gateway' section" ".spec.Sections.gateway" "$body"
assert_json_key "Spec contains 'optimizations' section" ".spec.Sections.optimizations" "$body"
assert_json_key "Spec contains 'security' section" ".spec.Sections.security" "$body"
assert_json_key "Spec contains 'metrics' section" ".spec.Sections.metrics" "$body"
assert_json_key "Spec contains 'thresholds' section" ".spec.Sections.thresholds" "$body"
assert_json_key "Spec contains 'models' section" ".spec.Sections.models" "$body"

# Check that sections have required fields
gateway_spec=$(echo "$body" | jq -r '.spec.Sections.gateway')
assert_contains "Gateway section has title" '"Title"' "$gateway_spec"
assert_contains "Gateway section has description" '"Description"' "$gateway_spec"
assert_contains "Gateway section has icon" '"Icon"' "$gateway_spec"
assert_contains "Gateway section has options" '"Options"' "$gateway_spec"
echo

# ============================================================================
# 4. API TOOLS LIST TESTS
# ============================================================================
echo -e "${YELLOW}[4] Tools List API Tests${NC}"

response=$(curl -s "$BASE_URL/api/tools" -w "\n%{http_code}")
http_code=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

assert_status "GET /api/tools returns 200" 200 "$http_code" "$body"
assert_json_key "Tools response contains 'tools' array" ".tools" "$body"

# Check if git is in the tools list (should be auto-detected in Docker)
tools_count=$(echo "$body" | jq '.tools | length')
echo -e "${BLUE}  Info: Found $tools_count configured tools${NC}"

if [ "$tools_count" -gt 0 ]; then
    first_tool=$(echo "$body" | jq '.tools[0]')
    assert_contains "Tool has 'name' field" '"name"' "$first_tool"
    assert_contains "Tool has 'type' field" '"type"' "$first_tool"
    assert_contains "Tool has 'path' field" '"path"' "$first_tool"
    assert_contains "Tool has 'health' field" '"health"' "$first_tool"
fi
echo

# ============================================================================
# 5. API TOOLS KNOWN TESTS
# ============================================================================
echo -e "${YELLOW}[5] Known Tools API Tests${NC}"

response=$(curl -s "$BASE_URL/api/tools/known" -w "\n%{http_code}")
http_code=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

assert_status "GET /api/tools/known returns 200" 200 "$http_code" "$body"
assert_json_key "Response contains 'success' key" ".success" "$body"
assert_json_key "Response contains 'tools' array" ".tools" "$body"

tools_array=$(echo "$body" | jq '.tools')
tools_count=$(echo "$tools_array" | jq 'length')
echo -e "${BLUE}  Info: Found $tools_count known tools${NC}"

# Check for known tool names
for tool_name in "git" "rtk" "scrapling"; do
    has_tool=$(echo "$tools_array" | jq "[.[] | select(.name == \"$tool_name\")] | length")
    if [ "$has_tool" -gt 0 ]; then
        echo -e "${GREEN}  ✓${NC} Known tool '$tool_name' found"
        ((TESTS_PASSED++))
    else
        echo -e "${YELLOW}  ℹ${NC} Known tool '$tool_name' not available"
    fi
done
echo

# ============================================================================
# 6. API TOOLS DISCOVER TESTS
# ============================================================================
echo -e "${YELLOW}[6] Tools Discovery API Tests${NC}"

response=$(curl -s "$BASE_URL/api/tools/discover" -w "\n%{http_code}")
http_code=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

assert_status "GET /api/tools/discover returns 200" 200 "$http_code" "$body"
assert_json_key "Discovery response contains 'success' key" ".success" "$body"
assert_json_key "Discovery response contains 'tools' object" ".tools" "$body"
echo

# ============================================================================
# 7. API TOOLS TYPES TESTS
# ============================================================================
echo -e "${YELLOW}[7] Tool Types API Tests${NC}"

response=$(curl -s "$BASE_URL/api/tools/types" -w "\n%{http_code}")
http_code=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

assert_status "GET /api/tools/types returns 200" 200 "$http_code" "$body"
assert_json_key "Types response is array" "." "$body"

# Check for expected tool types
for type in "cli" "mcp" "rest" "database" "binary"; do
    has_type=$(echo "$body" | jq "[.[] | select(.type == \"$type\")] | length")
    if [ "$has_type" -gt 0 ]; then
        echo -e "${GREEN}  ✓${NC} Tool type '$type' available"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}  ✗${NC} Tool type '$type' missing"
        ((TESTS_FAILED++))
    fi
done
echo

# ============================================================================
# 8. TOOL ADDITION TESTS
# ============================================================================
echo -e "${YELLOW}[8] Tool Management Tests${NC}"

# Test adding a new tool
new_tool_response=$(curl -s -X POST "$BASE_URL/api/tools/add" \
    -H "Content-Type: application/json" \
    -d '{
        "name": "test_tool",
        "type": "cli",
        "path": "/usr/bin/test",
        "settings": {}
    }' -w "\n%{http_code}")

http_code=$(echo "$new_tool_response" | tail -1)
body=$(echo "$new_tool_response" | sed '$d')

if [ "$http_code" -eq 200 ] || [ "$http_code" -eq 201 ]; then
    echo -e "${GREEN}✓${NC} POST /api/tools/add succeeds (HTTP $http_code)"
    ((TESTS_PASSED++))
else
    echo -e "${YELLOW}ℹ${NC} POST /api/tools/add returns HTTP $http_code (may be expected if validation fails)"
fi
echo

# ============================================================================
# 9. METRICS ENDPOINT TESTS
# ============================================================================
echo -e "${YELLOW}[9] Metrics API Tests${NC}"

response=$(curl -s "$BASE_URL/api/metrics" -w "\n%{http_code}")
http_code=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

assert_status "GET /api/metrics returns 200" 200 "$http_code" "$body"
assert_json_key "Metrics response is valid JSON" "." "$body"
echo

# ============================================================================
# 10. HEALTH CHECK TESTS
# ============================================================================
echo -e "${YELLOW}[10] Health Check Tests${NC}"

response=$(curl -s "$BASE_URL/health" -w "\n%{http_code}")
http_code=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

assert_status "GET /health returns 200" 200 "$http_code" "$body"
assert_json_key "Health response contains 'status' key" ".status" "$body"
echo

# ============================================================================
# 11. JAVASCRIPT FUNCTION TESTS
# ============================================================================
echo -e "${YELLOW}[11] Dashboard JavaScript Functions${NC}"

response=$(curl -s "$BASE_URL/dashboard")

assert_contains "JavaScript function: switchTab" "function switchTab" "$response"
assert_contains "JavaScript function: loadConfig" "function loadConfig" "$response"
assert_contains "JavaScript function: saveConfig" "function saveConfig" "$response"
assert_contains "JavaScript function: highlightYAMLSyntax" "function highlightYAMLSyntax" "$response"
assert_contains "JavaScript function: getYAMLPath" "function getYAMLPath" "$response"
assert_contains "JavaScript function: getNestedConfigHints" "function getNestedConfigHints" "$response"
assert_contains "JavaScript function: quickJump" "function quickJump" "$response"
assert_contains "JavaScript function: loadTools" "function loadTools" "$response"
assert_contains "JavaScript function: loadKnownTools" "function loadKnownTools" "$response"
assert_contains "JavaScript function: addTool" "function addTool" "$response"
assert_contains "JavaScript function: validateToolForm" "function validateToolForm" "$response"
echo

# ============================================================================
# 12. HTML ELEMENT TESTS
# ============================================================================
echo -e "${YELLOW}[12] Dashboard HTML Elements${NC}"

response=$(curl -s "$BASE_URL/dashboard")

# Test for specific HTML elements
assert_contains "Config editor textarea exists" 'id="config-editor"' "$response"
assert_contains "Config highlight pre exists" 'id="config-highlight"' "$response"
assert_contains "Config hints panel exists" 'id="config-hints"' "$response"
assert_contains "Tools list table exists" 'id="tools-list-table"' "$response"
assert_contains "Available tools div exists" 'id="available-tools"' "$response"
assert_contains "Tool type selector exists" 'id="tool-type"' "$response"
assert_contains "Tool name input exists" 'id="tool-name"' "$response"
assert_contains "Tool path input exists" 'id="tool-path"' "$response"
assert_contains "Metrics grid exists" 'id="metrics-grid"' "$response"
echo

# ============================================================================
# 13. CONFIGURATION ENDPOINTS TESTS
# ============================================================================
echo -e "${YELLOW}[13] Configuration Management Tests${NC}"

# Test config reload
response=$(curl -s "$BASE_URL/api/config/reload" -w "\n%{http_code}")
http_code=$(echo "$response" | tail -1)
body=$(echo "$response" | sed '$d')

assert_status "GET /api/config/reload returns 200" 200 "$http_code" "$body"
assert_json_key "Reload response contains 'success' key" ".success" "$body"
echo

# ============================================================================
# 14. YAML SYNTAX TESTS
# ============================================================================
echo -e "${YELLOW}[14] YAML Configuration Format Tests${NC}"

response=$(curl -s "$BASE_URL/api/config")
config=$(echo "$response" | jq '.config')

# Check for YAML-serializable config structure
assert_contains "Config has Gateway section" 'Gateway' "$(echo "$config" | jq keys)"
assert_contains "Config has Optimizations section" 'Optimizations' "$(echo "$config" | jq keys)"
assert_contains "Config has Security section" 'Security' "$(echo "$config" | jq keys)"

# Verify Gateway config structure
gateway=$(echo "$config" | jq '.Gateway')
assert_contains "Gateway has 'Port' field" 'Port' "$(echo "$gateway" | jq keys)"
assert_contains "Gateway has 'Host' field" 'Host' "$(echo "$gateway" | jq keys)"
echo

# ============================================================================
# 15. DOCKER DEPLOYMENT TESTS
# ============================================================================
echo -e "${YELLOW}[15] Docker Deployment Tests${NC}"

# Check if we're running in Docker
if [ -f "/.dockerenv" ]; then
    echo -e "${GREEN}✓${NC} Running inside Docker container"
    ((TESTS_PASSED++))
else
    # Check service availability from host
    response=$(curl -s -m 2 "$BASE_URL/health" 2>/dev/null)
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓${NC} Service accessible from host"
        ((TESTS_PASSED++))
    else
        echo -e "${YELLOW}ℹ${NC} Docker container test skipped"
    fi
fi

# Check network binding
if netstat -tuln 2>/dev/null | grep -q ':8077.*LISTEN'; then
    echo -e "${GREEN}✓${NC} Port 8077 is listening"
    ((TESTS_PASSED++))
elif lsof -i :8077 2>/dev/null | grep -q LISTEN; then
    echo -e "${GREEN}✓${NC} Port 8077 is listening"
    ((TESTS_PASSED++))
else
    echo -e "${YELLOW}ℹ${NC} Port check requires netstat or lsof"
fi
echo

# ============================================================================
# SUMMARY
# ============================================================================
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}Test Summary${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}Passed:${NC} $TESTS_PASSED"
echo -e "${RED}Failed:${NC} $TESTS_FAILED"
TOTAL=$((TESTS_PASSED + TESTS_FAILED))
echo -e "${BLUE}Total:${NC} $TOTAL"

if [ "$TESTS_FAILED" -eq 0 ]; then
    echo -e "\n${GREEN}All tests passed! ✓${NC}\n"
    exit 0
else
    echo -e "\n${RED}Some tests failed!${NC}\n"
    exit 1
fi
