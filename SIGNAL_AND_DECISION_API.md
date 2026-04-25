# Signal Detection & Decision Engine API

## Overview

Two new modules enable **early reaction** to user signals and **data-driven optimization decisions**:

1. **Signal Detector** (`internal/signals/detector.go`) — Analyzes user text for success/failure/effort signals
2. **Decision Engine** (`internal/decisions/engine.go`) — Makes optimization decisions based on tokens + signals

Both are **pure Go binary logic** — no shell scripts.

---

## Part 1: Signal Detection

### What Signals Are Detected

**Success Signals** (User is happy):
```
"Perfect!"
"Works great!"
"Thank you"
"That fixed it"
"Solved"
"Exactly"
"Excellent"
```
**Action**: Can cascade down to cheaper model

**Failure Signals** (User is unhappy):
```
"Didn't work"
"Still broken"
"That's wrong"
"Going in circles"
"Try again"
"Incomplete"
"Missing something"
```
**Action**: Should escalate to better model

**Effort Signals** (Task difficulty):
```
"Complex" / "Difficult" → Upgrade effort (low→medium, medium→high)
"Simple" / "Quick" → Downgrade effort
"Multiple steps" → Increase token estimate
```
**Action**: Adjust effort level and model routing

**Escalation Commands** (Explicit):
```
"/escalate"
"/escalate to opus"
"/escalate to sonnet"
```
**Action**: Immediate escalation

### Using Signal Detector

```go
// In service code
detector := signals.NewDetector()

// Detect signals in user text
sig := detector.DetectSignal("Perfect! That's exactly what I needed")

// Returns:
// Signal{
//   Type:       SignalSuccess,
//   Text:       "Perfect! That's exactly what I needed",
//   Confidence: 0.95,  // 95% confident this is success
//   Pattern:    "perfect",
// }

// Use signal for early reactions (before token data available)
if sig.Type == signals.SignalSuccess && sig.Confidence > 0.80 {
    fmt.Println("User is happy! Can cascade down to cheaper model")
}
```

### Confidence Scores

Each pattern has a confidence score (0.0-1.0):

```
Perfect!              → 0.95 (very strong signal)
Works great           → 0.93 (strong signal)
Thank you            → 0.85 (moderate signal)
Simple task          → 0.80 (weaker signal)
Going in circles     → 0.88 (strong failure signal)
```

---

## Part 2: Decision Engine

### Making Decisions

```go
// In service code
engine := decisions.NewEngine()

// Combine validation metrics + signal
decision := engine.MakeDecision(validation, signal)

// Returns:
// Decision{
//   Action:           "cascade",
//   NextModel:        "sonnet",
//   NextEffort:       "low",
//   Reason:           "User satisfied + model was over-provisioned",
//   Confidence:       0.95,
//   CascadeAvailable: true,
// }
```

### Decision Rules (Priority Order)

**RULE 1: Explicit Escalation** (Highest Priority)
```
IF user said "/escalate"
THEN escalate to Opus immediately
     Confidence: signal.Confidence (usually 0.98+)
```

**RULE 2: Success Signal + Within Thresholds**
```
IF user said "Perfect!" AND signal.Confidence > 0.80
THEN cascade down (Opus→Sonnet, Sonnet→Haiku)
     Reason: "User satisfied + model was over-provisioned"
     Confidence: signal.Confidence (usually 0.90+)
```

**RULE 3: Failure Signal**
```
IF user said "Didn't work" AND signal.Confidence > 0.80
THEN escalate up (Haiku→Sonnet, Sonnet→Opus)
     Reason: "User unsatisfied, current model insufficient"
     Confidence: signal.Confidence (usually 0.87+)
```

**RULE 4: Token-Based Accuracy** (Only if validated)
```
IF actual_tokens exceeded estimate by > 15%
THEN escalate (model was under-provisioned)

IF actual_tokens under estimate by > 15%
THEN cascade (model was over-provisioned)

IF within ±15% range
THEN stay (model choice was correct)
```

**RULE 5: Effort Signals**
```
IF user said "Complex" OR "Multiple steps"
THEN escalate effort level + model

IF user said "Simple" OR "Quick"
THEN cascade effort level + model
```

---

## Part 3: API Endpoints (New)

### Endpoint 1: Detect Signal in Text
```
POST /api/signals/detect
```

**Request**:
```json
{
  "text": "Perfect! That's exactly what I needed"
}
```

**Response**:
```json
{
  "type": "success",
  "confidence": 0.95,
  "pattern": "perfect",
  "recommendation": {
    "action": "can_cascade",
    "reason": "User indicates high satisfaction"
  }
}
```

### Endpoint 2: Make Decision (Tokens + Signal)
```
POST /api/decisions/make
```

**Request**:
```json
{
  "validation_id": 42,
  "signal_text": "Perfect!"
}
```

**Response**:
```json
{
  "action": "cascade",
  "next_model": "sonnet",
  "next_effort": "low",
  "reason": "User satisfied + model was over-provisioned",
  "confidence": 0.95,
  "cascade_available": true,
  "estimated_savings": 0.003
}
```

### Endpoint 3: Get Learning from History
```
GET /api/decisions/learning
```

**Response**:
```json
{
  "low_effort": {
    "count": 23,
    "avg_token_error": -2.1,
    "success_rate": 94,
    "best_model": "haiku",
    "model_counts": {
      "haiku": 23,
      "sonnet": 0,
      "opus": 0
    }
  },
  "medium_effort": {
    "count": 18,
    "avg_token_error": 1.8,
    "success_rate": 87,
    "best_model": "sonnet",
    "model_counts": {
      "haiku": 2,
      "sonnet": 14,
      "opus": 2
    }
  },
  "high_effort": {
    "count": 5,
    "avg_token_error": 4.2,
    "success_rate": 91,
    "best_model": "opus",
    "model_counts": {
      "haiku": 0,
      "sonnet": 1,
      "opus": 4
    }
  }
}
```

---

## Part 4: Integration with Service

### Service Integration Code (Pseudocode)

```go
package service

import (
    "github.com/szibis/claude-escalate/internal/signals"
    "github.com/szibis/claude-escalate/internal/decisions"
)

type Service struct {
    db       *store.Store
    detector *signals.Detector
    engine   *decisions.Engine
}

func (s *Service) handleSignalDetect(w http.ResponseWriter, r *http.Request) {
    // Parse request: text to analyze
    var req struct{ Text string }
    json.NewDecoder(r.Body).Decode(&req)
    
    // Detect signal
    sig := s.detector.DetectSignal(req.Text)
    
    // Return signal with recommendation
    json.NewEncoder(w).Encode(sig)
}

func (s *Service) handleMakeDecision(w http.ResponseWriter, r *http.Request) {
    // Parse request: validation_id + signal_text
    var req struct {
        ValidationID int
        SignalText   string
    }
    json.NewDecoder(r.Body).Decode(&req)
    
    // Get validation record
    validations, _ := s.db.GetValidationMetrics(100)
    var validation store.ValidationMetric
    for _, v := range validations {
        if v.ID == req.ValidationID {
            validation = v
            break
        }
    }
    
    // Detect signal
    sig := s.detector.DetectSignal(req.SignalText)
    
    // Make decision
    decision := s.engine.MakeDecision(validation, sig)
    
    // Return decision
    json.NewEncoder(w).Encode(decision)
}

func (s *Service) handleGetLearning(w http.ResponseWriter, r *http.Request) {
    // Get all validations
    validations, _ := s.db.GetValidationMetrics(1000)
    
    // Calculate learning
    learning := s.engine.CalculateLearning(validations)
    
    // Return statistics
    json.NewEncoder(w).Encode(learning)
}
```

---

## Part 5: Real-World Example Flow

### Scenario: User Says "Perfect!"

```
T0: USER TYPES: "Perfect! That works great."
    ↓
    Hook → /api/hook
    Response: model=haiku, validation_id=42
    ↓
    DB: Create validation_metric #42 {
      prompt: "...",
      estimated_tokens: 450,
      routed_model: "haiku"
    }

T2sec: CLAUDE RESPONDS (generation complete)
       Tokens: 444
       ↓
       IMMEDIATE: POST to /api/signals/detect
       Text: "Perfect! That works great."
       Response: {type: success, confidence: 0.95}
       
       ACTION: Instantly show user
       "✅ You're happy! Excellent result."
       (Don't wait for token validation)

T3sec: TOKEN CAPTURE: POST /api/validate {actual_total_tokens: 444}
       DB: Update validation_metric #42 {
         actual_tokens: 444,
         token_error: -1.3%
       }
       ↓
       POST to /api/decisions/make
       validation_id: 42
       signal_text: "Perfect!"
       
       Response: {
         action: "stay",
         reason: "User satisfied, already on cheapest model"
       }
       
       DB: Store decision for learning
       
T3.5sec: DASHBOARD UPDATES
        Shows: ✅ Perfect match (estimate 450 vs actual 444)
               ✅ User satisfied
               ✅ Model choice was correct
               ✅ Consider haiku for future low-effort tasks
```

---

## Part 6: Early vs Late Decisions

### Early Decision (No Token Data Yet)
```
T2sec: User says "Perfect!"
       Signal detected: success, confidence 0.95
       
       Decision: "Cascade available"
       Reasoning: User happy, no need to wait for token validation
       Confidence: 0.95 (from signal alone)
       
       Action: IMMEDIATE cascade suggestion
```

### Late Decision (With Token Data)
```
T3sec: Token validation complete
       Estimate: 450 tokens
       Actual: 444 tokens
       Error: -1.3%
       
       Combined with signal + token data:
       - User said "Perfect!" (0.95 confidence)
       - Tokens accurate (-1.3% error)
       - Model was right choice
       
       Decision: "Stay on haiku"
       Confidence: 0.98 (combined signal + token accuracy)
       
       Action: Store as successful validation for learning
```

---

## Part 7: Thresholds (Configurable)

```go
engine := decisions.NewEngine()

// Customize thresholds
engine.TokenErrorThreshold = 20.0    // Allow ±20% instead of ±15%
engine.SuccessSignalThreshold = 0.85 // Require 85% confidence, not 80%
engine.FailureSignalThreshold = 0.75 // Accept 75% confidence for failures
```

---

## Summary

**Signal Detection**:
- 40+ detectable patterns across 6 signal types
- Confidence scoring (0.0-1.0 per pattern)
- Early detection before token data available

**Decision Engine**:
- Priority-based rules (escalation > success > failure > tokens > effort)
- Token-aware (can validate model choices)
- Learning-capable (track task-type accuracy)

**Integration**:
- 3 new API endpoints
- Pure Go logic (no scripts)
- Works with existing validation system

**Result**: **React to user satisfaction in 2 seconds, validate with token data in 3 seconds.**
