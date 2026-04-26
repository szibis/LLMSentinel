# Documentation Audit Report

**Date**: 2026-04-25  
**Status**: AUDIT COMPLETE - READY FOR FIXES  

---

## Executive Summary

- **Total Markdown Files**: 41 (38 root + 3 docs/)
- **Mermaid Diagrams**: 12 (all in ARCHITECTURE_DIAGRAMS.md)
- **Critical Issues**: 6
- **Polish Issues**: 8
- **Dark Mode Issues**: 4+ Mermaid diagrams

---

## Critical Issues (MUST FIX)

### 1. Mermaid Color Styling (Dark Mode Incompatible)

**File**: `ARCHITECTURE_DIAGRAMS.md`  
**Problem**: Hard-coded light colors that are invisible in dark mode  
**Lines with Issues**: 101, 102, 179, 180, 181, 211

**Examples**:
```mermaid
# Line 101-102: Light colors
style Endpoint fill:#90EE90    # Light green (becomes invisible on dark background)
style Render fill:#87CEEB     # Light blue (becomes invisible on dark background)

# Line 179-181: More light colors
style Service fill:#FFE4B5    # Moccasin/peach
style DB fill:#E0F0FF        # Very light blue  
style Analytics fill:#E8F5E9  # Very light green
```

**Impact**: Diagrams unreadable in dark mode (which Claude Code uses by default)

**Fix Needed**: Use CSS variables or theme-aware colors:
```mermaid
%%{init: { 'theme': 'default' } }%%
graph LR
    Plugin["🔌 Plugin"]
    Service["🟢 Service"]
    
    style Plugin fill:var(--primary-color),color:var(--text-color)
    style Service fill:var(--success-color),color:var(--text-color)
```

OR use Mermaid's built-in `%%{init: { 'theme': 'dark' } }%%` support.

---

### 2. Incomplete Sections (TODO Placeholders)

**File**: `VALIDATION_PURE_BINARY.md` (lines 265, 278, 291)

**Missing Sections**:
- [ ] Phase 2: Monitor Mode (TODO)
- [ ] Phase 3: Report Mode (TODO)  
- [ ] Phase 4: Query Mode (TODO)

**Impact**: Users see unfinished documentation

---

### 3. Fragmented Documentation Structure

**Problem**: 41 markdown files scattered across root and docs/  

**Current State**:
```
/
├── README.md
├── ARCHITECTURE.md
├── ARCHITECTURE_DIAGRAMS.md
├── TOKEN_CAPTURE_ARCHITECTURE.md
├── VALIDATION_*.md (5 files)
├── FEEDBACK_LOOP_*.md (2 files)
├── PHASE_*.md (3 files)
├── QUICK_START.md
├── SERVICE_MODE.md
├── DASHBOARD.md
├── ... 20+ more
└── docs/
    ├── howit-works.md
    ├── claude-integration.md
    └── ... 3 more
```

**Problem**: Hard to navigate, users don't know where to start

**Fix Needed**: Reorganize to planned structure:
```
docs/
├── README.md (main index)
├── quick-start/
│   ├── 5-minute-setup.md
│   └── first-escalation.md
├── architecture/
│   ├── overview.md
│   ├── 3-phase-flow.md
│   ├── signal-detection.md
│   └── token-validation.md
├── integration/
│   ├── barista-statusline.md
│   ├── sentiment-detection.md
│   ├── budgets.md
│   └── api-reference.md
├── operations/
│   ├── deployment.md
│   ├── monitoring.md
│   └── troubleshooting.md
└── analytics/
    ├── dashboards.md
    ├── cost-analysis.md
    └── recommendations.md
```

---

## Polish Issues (SHOULD FIX)

### 1. README.md - Outdated Content

**Lines**: 38-80 (Documentation section)

**Current**:
```markdown
| Document | Purpose |
|----------|---------|
| **[QUICK_START.md](QUICK_START.md)** | ⚡ 5-minute setup guide |
| **[SERVICE_MODE.md](SERVICE_MODE.md)** | 🔄 HTTP service architecture & API |
... (many outdated links)
```

**Issue**: Links point to root-level files, not organized docs/  
**Fix**: Update to point to new docs/ structure

---

### 2. ARCHITECTURE.md - Incomplete Diagrams

**Lines**: 1-50

**Current**: Mentions "12 Mermaid diagrams" but doesn't explain each one

**Fix Needed**:
- Add brief description of each diagram's purpose
- Explain what to look for in each diagram
- Link to detailed sections

---

### 3. SERVICE_MODE.md - API Documentation Gaps

**Lines**: 150-200 (API Endpoints)

**Issue**: Missing new endpoints from recent work:
- `/api/signals/detect` — not documented
- `/api/decisions/make` — not documented
- `/api/decisions/learning` — not documented
- `/api/analytics/*` — not documented

**Fix Needed**: Add 6+ new endpoints to API reference

---

### 4. VALIDATION_INTEGRATION.md - Outdated References

**Lines**: 45, 120, 230

**Current**: References "Barista only" but system now supports multiple statusline sources

**Fix Needed**: Update to reflect multi-source architecture

---

### 5. QUICK_START.md - Missing Sentiment & Budgets

**Issue**: Doesn't mention new sentiment detection or budget features

**Fix Needed**: Add sections for:
- Setting up sentiment detection
- Configuring token budgets
- Understanding frustration escalation

---

### 6. TROUBLESHOOTING.md - Incomplete Coverage

**Lines**: 1-200

**Missing Sections**:
- Sentiment detection not triggering
- Budget limits not enforced
- Mermaid diagrams not rendering (dark mode issue)

**Fix Needed**: Add troubleshooting for all new features

---

### 7. No "Getting Started with Budgets" Guide

**Issue**: Users don't know how to set up token budgets

**File to Create**: `docs/quick-start/budgets-setup.md`

**Should Cover**:
- What is a budget?
- Why limit tokens?
- Daily vs monthly budgets
- Hard vs soft limits
- Example configurations

---

### 8. No "Sentiment Detection Explained" Guide

**Issue**: Users don't understand how sentiment is detected

**File to Create**: `docs/architecture/sentiment-detection.md`

**Should Cover**:
- What sentiments are detected?
- How does detection work?
- What happens when frustration is detected?
- Examples of sentiment patterns

---

## Mermaid Rendering Issues

### Diagrams to Test

**File**: `ARCHITECTURE_DIAGRAMS.md`

| # | Section | Type | Color Issue | Renders? |
|---|---------|------|-------------|----------|
| 1 | Overall System | graph TB | No | ? |
| 2 | Pre-Response Flow | sequenceDiagram | No | ? |
| 3 | Post-Response Flow | sequenceDiagram | No | ? |
| 4 | Statusline Plugin | graph LR | **YES** (#90EE90, #87CEEB) | ? |
| 5 | Full Cycle | timeline | No | ? |
| 6 | Data Model | graph TD | **YES** (#FFE4B5, #E0F0FF, #E8F5E9) | ? |
| 7 | API Endpoint | graph TB | **YES** | ? |
| 9 | Interaction Matrix | table/graph | No | ? |
| 10 | Deployment | graph TD | No | ? |
| 12 | Validation Stats | graph LR | No | ? |

**Status**: Need to verify which diagrams actually render in light/dark mode

---

## Dark Mode Support Strategy

### Current Problem
Mermaid diagrams use hard-coded colors like:
- `fill:#90EE90` (light green) — invisible on dark background
- `fill:#87CEEB` (light blue) — invisible on dark background

### Solution: Mermaid Theme Support

**Option 1: Use Mermaid Init Config** (Recommended)
```mermaid
%%{init: { 'theme': 'dark', 'primaryColor':'#2563eb', 'primaryTextColor':'#fff' } }%%
graph LR
    Service["🟢 Service"]
    style Service fill:#2563eb,stroke:#1e40af,color:#fff
```

**Option 2: Use CSS Variables** (Best for flexibility)
```css
/* In CSS or Mermaid theme */
:root {
  --primary: #2563eb;
  --success: #10b981;
  --warning: #f59e0b;
  --danger: #ef4444;
  --text: #1f2937;
  --bg: #ffffff;
}

/* Dark mode */
@media (prefers-color-scheme: dark) {
  :root {
    --text: #f3f4f6;
    --bg: #1f2937;
  }
}
```

**Option 3: Detect User Preference** (Most elegant)
```mermaid
%%{init: { 'theme': auto } }%%
```
Mermaid auto-detects light/dark mode preference.

### Implementation Plan

1. **For Existing Diagrams**: 
   - Add `%%{init: { 'theme': 'auto' } }%%` to each mermaid block
   - Test in both light/dark mode
   - Remove hard-coded `style` fill colors (use Mermaid defaults)

2. **For New Diagrams**:
   - Always use `%%{init: { 'theme': 'auto' } }%%` at top
   - Avoid hard-coded colors
   - Use emoji + text for contrast, not color alone

3. **Fallback**:
   - If 'auto' doesn't work, provide light/dark versions separately:
   ```
   ## Diagram (Light Mode)
   [light version]
   
   ## Diagram (Dark Mode)  
   [dark version]
   ```

---

## Summary: What Needs Fixing

### CRITICAL (Before Execution)
- [ ] Remove hard-coded colors from 4 Mermaid diagrams
- [ ] Add Mermaid dark mode support (`%%{init: { 'theme': 'auto' } }%%`)
- [ ] Complete VALIDATION_PURE_BINARY.md TODO sections
- [ ] Test all 12 Mermaid diagrams in dark/light mode

### HIGH PRIORITY (During Execution)
- [ ] Reorganize docs from 41 scattered files → organized docs/ structure
- [ ] Update README.md to link to new docs/ location
- [ ] Update SERVICE_MODE.md with 6 new API endpoints
- [ ] Update VALIDATION_INTEGRATION.md for multi-source statusline
- [ ] Update QUICK_START.md with sentiment + budget sections

### MEDIUM PRIORITY (Nice to Have)
- [ ] Create `docs/quick-start/budgets-setup.md`
- [ ] Create `docs/architecture/sentiment-detection.md`
- [ ] Update TROUBLESHOOTING.md for new features
- [ ] Add "Common Questions" section to README

---

## Testing Checklist

Before finalizing docs:

- [ ] All Mermaid diagrams render in light mode
- [ ] All Mermaid diagrams render in dark mode (Claude Code default)
- [ ] All links in README.md work and point to correct locations
- [ ] No broken cross-references between docs
- [ ] All API endpoints documented (20+ total)
- [ ] All new features mentioned (sentiment, budgets, multi-source)
- [ ] Quick-start guides complete and tested
- [ ] No TODO or FIXME remaining in docs

---

## Recommendation

**Before execution**: Fix all CRITICAL items (color + rendering + TODOs)  
**During execution**: Implement documentation reorganization + updates  
**After execution**: Run full verification checklist

Total estimated fix time: **2-3 hours**
