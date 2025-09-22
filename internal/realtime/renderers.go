package realtime

// GroupUpdateData contains data needed for rendering group updates
type GroupUpdateData struct {
    EntityType string                 `json:"entity_type"`
    EntityID   *string                `json:"entity_id"`
    Filter     map[string]interface{} `json:"filter"`
    EventData  map[string]interface{} `json:"event_data"`
}
