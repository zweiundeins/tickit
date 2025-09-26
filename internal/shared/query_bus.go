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

// RegisterQueryHandler registers both a typed handler and its factory in one call
func RegisterQueryHandler[T Query](b *QueryBus, queryType string, handlerFunc func(context.Context, T) (interface{}, error)) {
	// Create a wrapper that implements QueryHandler
	wrapper := &typedQueryHandlerWrapper[T]{
		handlerFunc: handlerFunc,
	}
	b.handlers[queryType] = wrapper
}

// typedQueryHandlerWrapper wraps a typed handler function to implement QueryHandler
type typedQueryHandlerWrapper[T any] struct {
	handlerFunc func(context.Context, T) (interface{}, error)
}

func (w *typedQueryHandlerWrapper[T]) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	// Try pointer type first
	var q T
	if ptr, ok := query.(*T); ok {
		q = *ptr
	} else if direct, ok := query.(T); ok {
		q = direct
	} else {
		return nil, fmt.Errorf("invalid query type: expected %T, got %T", q, query)
	}

	return w.handlerFunc(ctx, q)
}

// getQueryType returns the type name of a query for routing
func getQueryType(query interface{}) string {
	if q, ok := query.(Query); ok {
		return q.GetQueryType()
	}
	return ""
}
