# Event-Driven Grouping Requirements

## Core Objectives

### 1. Performance Optimization
- **Single Query Per Filter Group**: Execute database queries once per unique filter combination, not per session
- **Batch Template Rendering**: Render components once per group and broadcast to all sessions in that group
- **Memory Efficiency**: Minimize duplicate data structures and computations
- **Network Efficiency**: Reduce redundant SSE messages

### 2. Session Grouping Strategy

#### Filter-Based Grouping
```go
type FilterGroup struct {
    Hash        string                 // SHA256 of normalized filter state INCLUDING page number
    EntityType  string                 // e.g., "issues", "projects", "users"
    Filter      interface{}            // Generic filter (issues.ListIssuesFilter, etc.)
    OrderBy     string
    OrderDir    string
    Page        int                    // INCLUDED in group hash
    PageSize    int
    Sessions    map[string]*SSESession // sessionID -> SSE connection
    LastUpdate  time.Time
    DataCache   *GroupDataCache        // Cached query results (in-memory)
}
```

#### Group Lifecycle Management
- **Auto-creation**: Groups form when first session with unique filter joins
- **Auto-cleanup**: Groups dissolve when last session disconnects
- **Merge/Split**: Handle filter changes that move sessions between groups
- **TTL**: Remove inactive groups after configurable timeout

### 3. Event Processing Pipeline

#### Event Reception
```go
type EventProcessor struct {
    // Receive events from NATS with entity type prefixes
    // Route events to appropriate entity handlers
    // Determine which filter groups are affected by entity type
    // Queue updates for affected groups
}
```

#### Event Relevance Strategy
- **Entity-Prefixed Events**: Events use entity prefixes (e.g., "issues.created", "projects.updated", "users.deleted")
- **Entity-Based Subscription**: Subscribe to entity-specific event patterns
- **Granular Filtering**: On detail pages, filter to specific entity ID within entity type
- **Group Matching**: Check groups by entity type first, then filter relevance

#### Update Propagation
```go
type GroupUpdate struct {
    GroupHash   string
    EntityType  string                 // Entity type for this update
    UpdateType  string                 // "full_table", "full_page", "metadata_only"
    Data        interface{}            // Generic data payload
    AffectedIDs []string               // For tracking changes
}
```

### 4. Update Granularity

#### Update Strategies by Scope
- **Full Table Refresh**: Complete table re-render and replacement
- **Full Page Updates**: Entire page component updates (leveraging frontend morph)
- **Metadata Updates**: Pagination controls, counts, totals only
- **Frontend Morphing**: Let frontend handle DOM diffing and updates

#### Page Boundary Handling
```go
type PaginationUpdate struct {
    TotalCount    int
    CurrentPage   int
    TotalPages    int
    ItemsChanged  []string
    PageShift     bool // If items moved between pages
    ResetToFirst  bool // If filter change requires reset
}
```

### 5. Concurrency & Thread Safety

#### Group Registry
```go
type GroupRegistry struct {
    groups sync.Map // string -> *FilterGroup (key format: "entityType:hash")
    mutex  sync.RWMutex
}

// Group key format: "{entityType}:{filterHash}"
// Examples: "issues:abc123", "projects:def456", "users:ghi789"
```

#### Session Management
- **Atomic Operations**: Session join/leave operations must be atomic
- **Race Condition Prevention**: Handle concurrent filter changes
- **Graceful Degradation**: Continue serving other groups if one fails

### 6. Caching Strategy

#### In-Memory Caching
- **Group Data Cache**: Store query results in memory per group
- **TTL-Based Expiration**: Configurable cache lifetime
- **Memory Limits**: Automatic cleanup of least-recently-used groups
- **No External Dependencies**: Pure in-memory implementation

```go
type GroupDataCache struct {
    Data       interface{}
    CreatedAt  time.Time
    ExpiresAt  time.Time
    HitCount   int
}
```

### 7. Monitoring & Observability

#### Metrics to Track
- Group count and size distribution
- Update frequency per group
- Query execution time per filter combination
- Memory usage per group
- Session connection/disconnection rates
- Cache hit/miss ratios

#### Health Checks
- Detect and recover from stuck groups
- Monitor for memory leaks in group registry
- Alert on excessive group fragmentation

### 8. Configuration & Tuning

#### Configurable Parameters
```go
type GroupingConfig struct {
    MaxGroupSize        int           // Max sessions per group
    GroupTTL           time.Duration // Cleanup inactive groups
    BatchWindow        time.Duration // Event batching window
    MaxCacheAge        time.Duration // Data cache TTL
    EnableCompression  bool          // SSE compression
    MaxMemoryUsage     int64         // Memory limit for caching
}
```

### 9. Fallback & Error Handling

#### Degradation Strategies
- **Individual Fallback**: If group update fails, fall back to per-session updates
- **Cache Fallback**: Serve stale data if fresh query fails
- **Circuit Breaker**: Temporarily disable grouping if error rate exceeds threshold

#### Error Recovery
- **Automatic Retry**: Retry failed group updates with exponential backoff
- **Session Migration**: Move sessions to new group if current group becomes unhealthy
- **State Reconciliation**: Periodic sync to ensure group state consistency

### 10. Implementation Phases

#### Phase 1: Basic Grouping
- Implement entity-agnostic filter-based session grouping (including page number in hash)
- Entity-prefixed group keys and event routing
- Single query per group for initial load
- Basic event propagation to entity-specific groups
- In-memory caching only

#### Phase 2: Smart Updates
- Entity-prefixed event relevance filtering
- Full table/page updates with frontend morphing
- Pagination-aware update logic
- Multi-entity support (issues, projects, users, etc.)

## Future Work

### Phase 3: Optimization (Future)
- Query result caching with TTL
- Event batching and deduplication
- Performance monitoring and tuning
- Memory management and cleanup

### Phase 4: Advanced Features (Future)
- Cross-group optimization
- Predictive caching
- Advanced error recovery
- Metrics and observability dashboard

## Design Decisions Summary

1. **Group Hash**: Include page number and entity type for granular grouping
2. **Event Relevance**: Entity-prefixed events with type-based routing and group checking
3. **Update Granularity**: Full table/page updates leveraging frontend morph capabilities
4. **Cache Strategy**: In-memory only with TTL and memory limits
5. **Entity Support**: Generic system supporting multiple entity types (issues, projects, users, etc.)

## Architecture Notes

- Groups are ephemeral and tied to active sessions
- Entity-agnostic design supports any paginated list view
- Entity-prefixed group keys enable efficient routing (`entityType:filterHash`)
- Frontend handles DOM morphing for efficient updates
- Entity-prefixed event filtering reduces unnecessary processing
- Page-specific grouping ensures users see consistent pagination state
- Memory-only caching keeps implementation simple and fast

## Middleware-Based Updates Connection

### Single Persistent `/updates` Endpoint
```go
// User opens ONE connection on app load
GET /api/updates

// Backend automatically detects page context via middleware
// Signals only needed for user-driven changes (filters, pagination, sorting)
type ContextAwareSignals struct {
    // Only user-controlled state, not page context
    Filter   interface{} `json:"filter"`    // User changes filters
    Page     int         `json:"page"`      // User changes page
    PageSize int         `json:"page_size"` // User changes page size
    OrderBy  string      `json:"order_by"`  // User changes sorting
    OrderDir string      `json:"order_dir"` // User changes sort direction
}
```

### Route-Level Middleware Implementation

#### Benefits
- **DRY**: Subscription logic defined once per route group
- **Explicit**: Clear declaration of what each route subscribes to
- **Maintainable**: Changes to route structure automatically update subscriptions
- **Type-safe**: Context values can be strongly typed
- **Component Rendering**: Middleware also defines how updates are rendered

#### Implementation
```go
// Enhanced middleware that defines both subscription and rendering
func SubscriptionMiddleware(entityType string, componentRenderer ComponentRenderer, idParam ...string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx := context.WithValue(r.Context(), "subscription_entity", entityType)
            ctx = context.WithValue(ctx, "component_renderer", componentRenderer)
            
            if len(idParam) > 0 {
                entityID := chi.URLParam(r, idParam[0])
                ctx = context.WithValue(ctx, "subscription_entity_id", entityID)
            }
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// Component renderer interface
type ComponentRenderer interface {
    RenderUpdate(data interface{}) templ.Component
}

// Route definitions with subscription + rendering metadata
func SetupRoutes(r chi.Router) {
    r.Route("/dashboard", func(r chi.Router) {
        r.Use(SubscriptionMiddleware("issues", &IssuesTableRenderer{}))
        r.Get("/", HandleDashboard)
    })
    
    r.Route("/projects", func(r chi.Router) {
        r.Use(SubscriptionMiddleware("projects", &ProjectsTableRenderer{}))
        r.Get("/", HandleProjects)
        r.Get("/new", HandleNewProject)
    })
    
    r.Route("/issue/{id}", func(r chi.Router) {
        r.Use(SubscriptionMiddleware("issue", &IssueDetailRenderer{}, "id"))
        r.Get("/", HandleIssueDetail)
    })
}

// Component renderers
type IssuesTableRenderer struct{}
func (r *IssuesTableRenderer) RenderUpdate(data interface{}) templ.Component {
    issuesData := data.(*IssuesUpdateData)
    return components.IssuesTable(issuesData.Issues, issuesData.ProjectMap, issuesData.UserMap)
}

type ProjectsTableRenderer struct{}
func (r *ProjectsTableRenderer) RenderUpdate(data interface{}) templ.Component {
    projectsData := data.(*ProjectsUpdateData)
    return components.ProjectsTable(projectsData.Projects)
}

type IssueDetailRenderer struct{}
func (r *IssueDetailRenderer) RenderUpdate(data interface{}) templ.Component {
    issueData := data.(*IssueUpdateData)
    return components.Issue(issueData.Issue, issueData.Projects)
}

// SSE handler uses context for both subscription and rendering
func HandleUpdatesSSE(w http.ResponseWriter, r *http.Request) {
    entityType := r.Context().Value("subscription_entity").(string)
    entityID, _ := r.Context().Value("subscription_entity_id").(string)
    renderer := r.Context().Value("component_renderer").(ComponentRenderer)
    
    // When event occurs, use renderer from context
    updateData := fetchUpdateData(entityType, entityID, userSignals)
    component := renderer.RenderUpdate(updateData)
    sse.PatchElementTempl(component)
}
```

## CQRS/Event Sourcing Integration Workflow

### Complete Event Flow from Command to UI Update

```go
// 1. Command Handler executes CQRS/ES operations
func (h *IssueCommandHandler) HandleUpdateIssue(cmd *PatchIssueCommand) error {
    // Generate domain event (no aggregate loading/modification)
    event := &IssuePatchedEvent{
        AggregateID: cmd.IssueID,
        Version:     h.getNextVersion(cmd.IssueID),
        Changes:     cmd.Updates,
        UpdatedAt:   time.Now(),
        EventType:   "IssuePatched",
    }
    
    // 1. Store event in event store
    err := h.eventStore.StoreEvent(event)
    if err != nil {
        return err
    }
    
    // 2. Apply to projection store (read models)
    err = h.projectionStore.ApplyEvent(event)
    if err != nil {
        return err
    }
    
    // 3. Publish to event bus (NATS)
    return h.eventBus.PublishEvent(event)
}

// 2. Domain Event (stored + published)
type IssuePatchedEvent struct {
    AggregateID string                 `json:"aggregate_id"` // Issue ID
    Version     int                    `json:"version"`      // Event version
    Changes     map[string]interface{} `json:"changes"`      // What changed
    UpdatedAt   time.Time             `json:"updated_at"`
    EventType   string                `json:"event_type"`   // "IssuePatched"
}

// 3. Projection Handler (subscribes to domain events)
func (h *IssueProjectionHandler) HandleIssuePatched(event *IssuePatchedEvent) error {
    // This already happened in step 2 above, but could also be async
    return h.projectionStore.UpdateIssue(event.AggregateID, event.Changes)
}

// 4. UI Event Handler (subscribes to domain events from event bus)
func (h *UIEventHandler) HandleIssuePatched(event *IssuePatchedEvent) error {
    // Convert domain event to UI-specific event
    uiEvent := UIUpdateEvent{
        EventType:  "issues.patched",
        EntityID:   event.AggregateID,
        EntityType: "issues",
        Timestamp:  time.Now(),
        Data:       event.Changes,
    }
    
    // Publish to separate UI events topic for SSE handlers
    return h.publisher.Publish("events", uiEvent)
}

// 5. SSE Handler receives UI event and routes to groups
func HandleUpdatesSSE(...) {
    // Subscribe to "events" topic (UI events, not domain events)
    sub, err := nc.Subscribe("events", func(msg *nats.Msg) {
        var uiEvent UIUpdateEvent
        json.Unmarshal(msg.Data, &uiEvent)
        
        // Route based on entity type
        switch uiEvent.EntityType {
        case "issues":
            updateIssuesGroups(uiEvent)
        case "projects":
            updateProjectsGroups(uiEvent)
        }
    })
}

// 5. Cross-Entity Update Routing - Issue 123 Example
func routeUIEvent(event UIUpdateEvent) {
    switch event.EntityType {
    case "issues":
        // Route to both dashboard groups AND issue detail groups
        updateIssuesTableGroups(event)  // Dashboard sessions
        updateIssueDetailGroups(event)  // Issue detail sessions
    case "projects":
        updateProjectsGroups(event)
    }
}

// Update dashboard sessions showing issue tables
func updateIssuesTableGroups(event UIUpdateEvent) {
    // Find all groups subscribed to "issues" (dashboard pages)
    for groupKey, group := range groupRegistry.GetByEntityType("issues") {
        // Check if this issue would appear in this group's filtered view
        if group.Filter.WouldInclude(event.EntityID) {
            // Fetch fresh paginated data for this group's filter/page
            updateData := fetchIssuesForGroup(group)
            
            // Render full issues table
            component := group.Renderer.RenderUpdate(updateData) // IssuesTableRenderer
            
            // Send to all dashboard sessions in this group
            for sessionID, session := range group.Sessions {
                session.SSE.PatchElementTempl(component)
            }
        }
    }
}

// Update issue detail sessions for specific issue
func updateIssueDetailGroups(event UIUpdateEvent) {
    // Find groups subscribed to this specific issue
    issueGroupKey := fmt.Sprintf("issue:%s", event.EntityID)
    
    if group, exists := groupRegistry.GetByKey(issueGroupKey); exists {
        // Fetch fresh issue data
        issue := fetchIssueByID(event.EntityID)
        projects := fetchAllProjects() // For dropdown
        
        updateData := &IssueUpdateData{
            Issue:    issue,
            Projects: projects,
        }
        
        // Render issue detail component
        component := group.Renderer.RenderUpdate(updateData) // IssueDetailRenderer
        
        // Send to all sessions viewing this specific issue
        for sessionID, session := range group.Sessions {
            session.SSE.PatchElementTempl(component)
        }
    }
}
```

### Event Types and Routing

```go
// Domain Events (from CQRS/ES)
type DomainEvent interface {
    AggregateID() string
    EventType() string
}

// UI Events (for real-time updates)
type UIUpdateEvent struct {
    EventType  string      `json:"event_type"`  // "issues.patched", "projects.created"
    EntityType string      `json:"entity_type"` // "issues", "projects", "users"
    EntityID   string      `json:"entity_id"`   // Specific entity ID
    Data       interface{} `json:"data"`        // Event payload
    Timestamp  time.Time   `json:"timestamp"`
}

// Event Type Mapping
// Domain Event -> UI Event
// IssueCreated -> "issues.created"
// IssuePatched -> "issues.patched" 
// IssueDeleted -> "issues.deleted"
// ProjectCreated -> "projects.created"
```

## Detailed Example: Issue 123 Update Flow

### Scenario: User patches Issue 123 (changes priority from "Low" to "High")

#### Active Sessions:
- **Session A**: `/dashboard` with filter `{project_id: "proj1"}` - Issue 123 is visible on page 1
- **Session B**: `/dashboard` with filter `{status: "open"}` - Issue 123 is visible on page 2  
- **Session C**: `/dashboard` with filter `{assignee: "user456"}` - Issue 123 NOT visible (different assignee)
- **Session D**: `/issue/123` - viewing Issue 123 detail page
- **Session E**: `/issue/456` - viewing different issue detail page

#### Group Structure:
```go
// Dashboard groups (by filter + page)
"issues:hash_proj1_page1" -> {Sessions: [A], Renderer: IssuesTableRenderer}
"issues:hash_open_page2"  -> {Sessions: [B], Renderer: IssuesTableRenderer}  
"issues:hash_user456_page1" -> {Sessions: [C], Renderer: IssuesTableRenderer}

// Issue detail groups (by specific issue ID)
"issue:123" -> {Sessions: [D], Renderer: IssueDetailRenderer}
"issue:456" -> {Sessions: [E], Renderer: IssueDetailRenderer}
```

#### Update Flow:

**1. Command Execution**
```go
PATCH /api/issues/123 {priority: "High"}
→ Route handler collects patch parameters from signals
→ issueService.PatchIssue(issueID, updates)
→ Triggers PatchIssueCommand on command bus
→ IssueCommandHandler.HandleUpdateIssue()
→ eventStore.StoreEvent(IssuePatched{IssueID: "123", Changes: {priority: "High"}})
→ projectionStore.ApplyEvent(IssuePatched{...})
→ eventBus.PublishEvent(IssuePatched{...})
```

**2. Event Processing**
```go
IssueEventHandler receives IssuePatched
→ Publishes UIUpdateEvent{
    EventType: "issues.patched",
    EntityType: "issues", 
    EntityID: "123",
    Data: {priority: "High"}
}
```

**3. Cross-Entity Routing**
```go
routeUIEvent() receives UIUpdateEvent
→ Calls updateIssuesTableGroups(event) // For dashboard sessions
→ Calls updateIssueDetailGroups(event) // For issue detail sessions
```

**4. Dashboard Updates**
```go
updateIssuesTableGroups() processes each "issues" group:

Group "issues:hash_proj1_page1":
→ group.Filter.WouldInclude("123") = TRUE (Issue 123 in project proj1)
→ fetchIssuesForGroup() gets fresh page 1 data with updated priority
→ IssuesTableRenderer.RenderUpdate() renders full table
→ Send to Session A ✅

Group "issues:hash_open_page2":  
→ group.Filter.WouldInclude("123") = TRUE (Issue 123 is still open)
→ fetchIssuesForGroup() gets fresh page 2 data with updated priority
→ IssuesTableRenderer.RenderUpdate() renders full table
→ Send to Session B ✅

Group "issues:hash_user456_page1":
→ group.Filter.WouldInclude("123") = FALSE (Issue 123 not assigned to user456)
→ No update sent to Session C ❌
```

**5. Issue Detail Updates**
```go
updateIssueDetailGroups() looks for specific issue group:

Group "issue:123":
→ Found! Session D is viewing Issue 123
→ fetchIssueByID("123") gets fresh issue data with updated priority
→ IssueDetailRenderer.RenderUpdate() renders issue detail component
→ Send to Session D ✅

Group "issue:456":
→ Different issue, no update needed for Session E ❌
```

#### Final Result:
- **Session A** (dashboard proj1): Gets updated table showing Issue 123 with "High" priority
- **Session B** (dashboard open): Gets updated table showing Issue 123 with "High" priority  
- **Session C** (dashboard user456): No update (Issue 123 not in their filter)
- **Session D** (issue/123 detail): Gets updated issue detail showing "High" priority
- **Session E** (issue/456 detail): No update (different issue)

### Integration Points

1. **Domain Event Publishers**: Aggregate repositories publish domain events
2. **Event Handlers**: Convert domain events to UI events
3. **NATS Topics**: 
   - Domain events: `domain.{aggregate}.{event}` 
   - UI events: `events` (single topic for all UI updates)
4. **SSE Subscribers**: Listen to `events` topic and route to groups
5. **Group Filters**: Determine which sessions care about specific events
6. **Cross-Entity Routing**: Single event updates multiple entity types (table + detail views)

### Frontend Usage
```go
// Pages just call /api/updates - middleware handles the rest
// Dashboard
data-on-load="@get('/api/updates')"
data-signals__ifmissing="{filter: {}, page: 1, page_size: 25}"

// Projects  
data-on-load="@get('/api/updates')"
data-signals__ifmissing="{filter: {}, page: 1, page_size: 10}"

// Issue detail
data-on-load="@get('/api/updates')"
// Middleware extracts issue ID from route param automatically
```
