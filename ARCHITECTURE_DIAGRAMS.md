# System Architecture - Mermaid Diagrams

## 1. Overall System Architecture

```mermaid
%%{init: { 'theme': 'auto' } }%%
graph TB
    Claude["🔵 Claude Code Session"]
    Hook["⚙️ Hook<br/>3-line bash<br/>localhost"]
    Service["🟢 Service<br/>Go Binary<br/>localhost:9000"]
    DB["💾 SQLite<br/>validation_metrics"]
    Settings["⚙️ settings.json"]
    Dashboard["📊 Dashboard<br/>http://localhost:9000"]
    Plugins["🔌 Statusline Plugins<br/>Barista/Custom"]

    Claude -->|"stdin: prompt"| Hook
    Hook -->|"POST /api/hook"| Service
    Service -->|"analyze<br/>estimate<br/>route"| Service
    Service -->|"Read/Write"| DB
    Service -->|"Update model"| Settings
    Service -->|"Serve UI"| Dashboard
    Service -->|"GET /api/statusline"| Plugins
    Plugins -->|"Display metrics"| Plugins
    Dashboard -->|"GET /api/validation/*"| Service
```

## 2. Pre-Response Flow (Hook Phase)

```mermaid
%%{init: { 'theme': 'auto' } }%%
sequenceDiagram
    participant User as User
    participant Hook as http-hook.sh
    participant Service as Service<br/>/api/hook
    participant DB as SQLite
    participant Settings as settings.json

    User->>Hook: Type: "What is ML?"
    Hook->>Hook: read -r PROMPT
    Hook->>Service: curl POST<br/>{"prompt": "..."}
    
    Service->>Service: Parse prompt
    Service->>Service: Detect effort: "low"
    Service->>Service: Estimate tokens: 500
    Service->>Service: Route model: "haiku"
    
    Service->>DB: CREATE validation_metric<br/>id=42, estimated=500<br/>effort="low"
    Service->>Settings: Update: "model": "haiku"
    
    Service-->>Hook: {"continue": true,<br/>"currentModel": "haiku",<br/>"validationId": 42}
    Hook-->>User: Return routing decision
    
    Note over User,Settings: Claude processes prompt...
```

## 3. Post-Response Flow (Validation Phase)

```mermaid
%%{init: { 'theme': 'auto' } }%%
sequenceDiagram
    participant Claude as Claude<br/>Processing
    participant Monitor as Monitor/Integration
    participant Service as Service<br/>/api/validate
    participant DB as SQLite
    participant Dashboard as Dashboard

    Claude->>Claude: Generate response
    Claude->>Claude: Count tokens: 493

    Monitor->>Monitor: Extract actual tokens
    Monitor->>Monitor: Input: 268, Output: 474<br/>Total: 493
    
    Monitor->>Service: POST /api/validate<br/>{"actual_total_tokens": 493}
    
    Service->>DB: LOOKUP validation_id=42
    Service->>Service: Compare:<br/>est=500 vs act=493
    Service->>Service: Calculate error:<br/>-1.4%
    
    Service->>DB: UPDATE validation_metric<br/>id=42, actual=493<br/>error=-1.4%, validated=true
    
    Service-->>Monitor: {"success": true}
    
    Dashboard->>Service: GET /api/validation/metrics
    Service-->>Dashboard: [metric#42, ...]
    Dashboard->>Dashboard: Render: Est 500 vs Act 493<br/>Error: -1.4% ✅
```

## 4. Statusline Plugin Integration

```mermaid
%%{init: { 'theme': 'auto' } }%%
graph LR
    Plugin["🔌 Statusline Plugin<br/>Barista/Custom"]
    Endpoint["GET /api/statusline"]
    Service["🟢 Service"]
    DB["💾 Database"]
    Render["📊 Display<br/>Model + Accuracy<br/>+ Savings"]

    Plugin -->|"HTTP Query"| Endpoint
    Endpoint -->|"In Service"| Service
    Service -->|"Query"| DB
    Service -->|"Return JSON"| Plugin
    Plugin -->|"Format & Display"| Render
```

## 5. Full Cycle: User Interaction to Validation

```mermaid
%%{init: { 'theme': 'auto' } }%%
timeline
    title Complete Escalation & Validation Cycle
    
    section PRE-RESPONSE
    00:00 : User types prompt: "What is ML?"
    00:01 : Hook reads stdin
    00:02 : Hook POSTs to /api/hook
    00:05 : Service analyzes prompt
    00:10 : Service detects: low effort
    00:15 : Service estimates: 500 tokens
    00:20 : Service creates validation record (estimate)
    00:25 : Service updates settings.json
    00:30 : Service returns routing decision
    00:35 : Claude Code receives response
    
    section CLAUDE PROCESSING
    00:36 : Claude loads prompt
    01:00 : Claude generates response
    01:50 : Claude counts tokens: 493
    02:00 : Generation complete
    
    section POST-RESPONSE
    02:01 : Monitor/Integration extracts actual tokens
    02:02 : Monitor POSTs to /api/validate
    02:05 : Service receives actual metrics
    02:06 : Service matches estimate to actual
    02:07 : Service calculates error: -1.4%
    02:08 : Service updates validation record (actual)
    02:09 : Database now has complete record
    
    section DASHBOARD
    02:10 : Dashboard refreshes (2s poll)
    02:11 : Dashboard queries /api/validation/metrics
    02:12 : Dashboard displays: Est 500 vs Act 493
    02:13 : Dashboard shows accuracy: 98.6% ✅
    02:14 : Real-time metrics updated
```

## 6. Data Model: Validation Metric

```mermaid
%%{init: { 'theme': 'auto' } }%%
graph TD
    A["ValidationMetric (SQLite Record)"]
    
    A --> B["Identification"]
    B --> B1["ID: 42"]
    B --> B2["Timestamp: 2026-04-25T15:52:00Z"]
    
    A --> C["User Input"]
    C --> C1["Prompt: 'What is ML?'"]
    C --> C2["Task Type: general"]
    
    A --> D["Hook Estimates<br/>Phase 1"]
    D --> D1["DetectedEffort: low"]
    D --> D2["RoutedModel: haiku"]
    D --> D3["EstimatedInputTokens: 27"]
    D --> D4["EstimatedOutputTokens: 500"]
    D --> D5["EstimatedTotalTokens: 527"]
    D --> D6["EstimatedCost: $0.005"]
    
    A --> E["Actual Values<br/>Phase 2"]
    E --> E1["ActualInputTokens: 25"]
    E --> E2["ActualOutputTokens: 474"]
    E --> E3["ActualTotalTokens: 499"]
    E --> E4["ActualCost: $0.00499"]
    
    A --> F["Calculated Errors"]
    F --> F1["TokenError: -5.3%"]
    F --> F2["CostError: -2.0%"]
    F --> F3["Validated: true"]
```

## 7. API Endpoint Architecture

```mermaid
%%{init: { 'theme': 'auto' } }%%
graph TB
    HTTP["HTTP Server<br/>localhost:9000"]
    
    HTTP --> H1["POST /api/hook<br/>Pre-Response"]
    H1 --> H1A["Parse prompt"]
    H1A --> H1B["Estimate tokens"]
    H1B --> H1C["Detect effort"]
    H1C --> H1D["Create validation<br/>record estimate"]
    
    HTTP --> H2["POST /api/validate<br/>Post-Response"]
    H2 --> H2A["Receive actual<br/>metrics"]
    H2A --> H2B["Look up estimate"]
    H2B --> H2C["Compare &<br/>calculate error"]
    H2C --> H2D["Update validation<br/>record"]
    
    HTTP --> H3["GET /api/statusline<br/>Plugin Query"]
    H3 --> H3A["Query database"]
    H3A --> H3B["Calculate stats"]
    H3B --> H3C["Return JSON"]
    
    HTTP --> H4["GET /api/validation/*<br/>Dashboard"]
    H4 --> H4A["Return metrics"]
    H4A --> H4B["Return stats"]
```

## 8. Information Flow: Hook to Dashboard

```mermaid
%%{init: { 'theme': 'auto' } }%%
graph LR
    A["User Input:<br/>Prompt"] -->|"Hook<br/>Analyzes"| B["Estimate:<br/>500 tokens<br/>low effort"]
    
    B -->|"Service<br/>Creates"| C["Validation<br/>Record #42<br/>estimate_only"]
    
    C -->|"Database<br/>Stores"| D["SQLite"]
    
    D -->|"Claude<br/>Processes"| E["Claude<br/>Generation"]
    
    E -->|"Actual:<br/>493 tokens"| F["Monitor<br/>Extracts"]
    
    F -->|"Service<br/>Updates"| G["Validation<br/>Record #42<br/>estimate+actual"]
    
    G -->|"Database<br/>Stores"| D
    
    D -->|"Dashboard<br/>Queries"| H["Real-time<br/>Display<br/>Est vs Act"]
    
    H -->|"Show<br/>Accuracy"| I["Metrics:<br/>-1.4% error<br/>98.6% accuracy"]
```

## 9. Component Interaction Matrix

```mermaid
%%{init: { 'theme': 'auto' } }%%
graph TB
    subgraph Binary["Go Binary<br/>(escalation-manager)"]
        Service["Service Mode<br/>:9000"]
        Monitor["Monitor Mode<br/>Background"]
        Dashboard["Dashboard UI<br/>Static Files"]
    end
    
    subgraph Local["Local System"]
        Hook["Hook<br/>3-line bash"]
        DB["SQLite<br/>Database"]
        Settings["settings.json"]
    end
    
    subgraph External["External"]
        Claude["Claude Code"]
        Plugin["Statusline<br/>Plugins"]
    end
    
    Claude -->|"UserPromptSubmit"| Hook
    Hook -->|"HTTP POST"| Service
    Service -->|"Read/Write"| DB
    Service -->|"Update"| Settings
    Service -->|"Serve"| Dashboard
    Service -->|"Query"| DB
    Plugin -->|"HTTP GET"| Service
```

## 10. Deployment Architecture

```mermaid
%%{init: { 'theme': 'auto' } }%%
graph TD
    Start["1. Copy binary<br/>~/.local/bin/escalation-manager"]
    
    Start --> S1["2. Create hook<br/>~/.claude/hooks/http-hook.sh<br/>3 lines"]
    
    S1 --> S2["3. Start service<br/>escalation-manager<br/>service --port 9000"]
    
    S2 --> S3{{"4. Choose Integration"}}
    
    S3 -->|"Option A"| O1["Monitor Mode<br/>escalation-manager<br/>monitor --port 9000"]
    
    S3 -->|"Option B"| O2["Barista Module<br/>~/.claude/barista/<br/>modules/...sh"]
    
    S3 -->|"Option C"| O3["Custom Integration<br/>POST to /api/validate"]
    
    O1 --> Done["✅ Full validation<br/>system active"]
    O2 --> Done
    O3 --> Done
    
    Done --> Live["Live validation:<br/>Estimate vs Actual"]
```

## 11. Data Flow: Hook Analysis

```mermaid
%%{init: { 'theme': 'auto' } }%%
graph TB
    Prompt["User Prompt:<br/>'What is ML?'"]
    
    Prompt --> P1["Effort Detection<br/>Keywords: 'what is'"]
    P1 --> P1A["Result: LOW effort"]
    
    Prompt --> P2["Command Detection<br/>Check: /escalate?"]
    P2 --> P2A["Result: None"]
    
    Prompt --> P3["Signal Detection<br/>Check: success signals?"]
    P3 --> P3A["Result: None"]
    
    Prompt --> P4["Token Estimation<br/>27 chars / 4 per token"]
    P4 --> P4A["Est Input: 7 tokens"]
    
    P1A --> Decision["Model Routing<br/>Decision"]
    P2A --> Decision
    P3A --> Decision
    P4A --> Decision
    
    Decision --> Output["Route: HAIKU<br/>Effort: LOW<br/>Est: 500 tokens<br/>ValidationID: 42"]
```

## 12. Validation Statistics Calculation

```mermaid
%%{init: { 'theme': 'auto' } }%%
graph TB
    Records["All ValidationMetric<br/>Records in DB<br/>42 total"]
    
    Records --> Loop["For each record:<br/>✓ Has estimate?<br/>✓ Has actual?<br/>✓ Validated?"]
    
    Loop --> Calc["Calculate:<br/>• token_error = (actual-est)/est<br/>• cost_error = (actual-est)/est<br/>• accuracy = 100 - avg_error"]
    
    Calc --> Stats["Aggregated Stats"]
    
    Stats --> S1["Token Stats"]
    S1 --> S1A["Estimated Total: 12,340"]
    S1 --> S1B["Actual Total: 11,920"]
    S1 --> S1C["Tokens Saved: 420"]
    S1 --> S1D["Savings %: 3.4%"]
    
    Stats --> S2["Accuracy Stats"]
    S2 --> S2A["Avg Token Error: -3.2%"]
    S2 --> S2B["Avg Cost Error: -2.1%"]
    S2 --> S2C["Accuracy Score: 96.8%"]
    
    Stats --> S3["Cost Stats"]
    S3 --> S3A["Est Cost: $0.1234"]
    S3 --> S3B["Actual Cost: $0.1192"]
    S3 --> S3C["Cost Saved: $0.0042"]
    
    S1D --> Display["Display on Dashboard<br/>& Statusline"]
    S2C --> Display
    S3C --> Display
```

