package shared

import (
	"context"
	"fmt"
)

// QueryBus handles query routing and execution
type QueryBus struct {
	handlers map[string]QueryHandler
}

// QueryHandler interface for all query handlers
type QueryHandler interface {
	Handle(ctx context.Context, query interface{}) (interface{}, error)
}

// Query interface for all queries
// All queries must implement GetQueryType() string
// Example: func (q GetIssueByIDQuery) GetQueryType() string { return "GetIssueByID" }
type Query interface {
	GetQueryType() string
}

// NewQueryBus creates a new query bus
func NewQueryBus() *QueryBus {
	return &QueryBus{
		handlers: make(map[string]QueryHandler),
	}
}

// RegisterHandler registers a query handler for a specific query type
func (b *QueryBus) RegisterHandler(queryType string, handler QueryHandler) {
	b.handlers[queryType] = handler
}

// Execute dispatches a query to its appropriate handler
func (b *QueryBus) Execute(ctx context.Context, query interface{}) (interface{}, error) {
	queryType := getQueryType(query)

	handler, exists := b.handlers[queryType]
	if !exists {
		return nil, fmt.Errorf("no handler registered for query type: %s", queryType)
	}

	return handler.Handle(ctx, query)
}

// getQueryType returns the type name of a query for routing
func getQueryType(query interface{}) string {
	if q, ok := query.(Query); ok {
		return q.GetQueryType()
	}
	return ""
}
