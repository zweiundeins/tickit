package bus

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/cqrs/queries"
)

// QueryBus handles query routing and execution
type QueryBus struct {
	handlers map[string]QueryHandler
}

// QueryHandler interface for all query handlers
type QueryHandler interface {
	Handle(ctx context.Context, query interface{}) (interface{}, error)
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

// getQueryType returns the type name of a query
func getQueryType(query interface{}) string {
	switch query.(type) {
	case queries.GetNotificationsQuery:
		return "GetNotifications"
	case queries.GetUnreadCountQuery:
		return "GetUnreadCount"
	case queries.GetNotificationByUIDQuery:
		return "GetNotificationByUID"
	case queries.GetIssueByIDQuery:
		return "GetIssueByID"
	case queries.ListIssuesQuery:
		return "ListIssues"
	default:
		return fmt.Sprintf("%T", query)
	}
}

// Concrete query handlers

type GetNotificationsHandler struct {
	handler *queries.QueryHandler
}

func NewGetNotificationsHandler(handler *queries.QueryHandler) *GetNotificationsHandler {
	return &GetNotificationsHandler{
		handler: handler,
	}
}

func (h *GetNotificationsHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(queries.GetNotificationsQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for GetNotificationsHandler")
	}
	return h.handler.HandleGetNotifications(ctx, q)
}

type GetUnreadCountHandler struct {
	handler *queries.QueryHandler
}

func NewGetUnreadCountHandler(handler *queries.QueryHandler) *GetUnreadCountHandler {
	return &GetUnreadCountHandler{
		handler: handler,
	}
}

func (h *GetUnreadCountHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(queries.GetUnreadCountQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for GetUnreadCountHandler")
	}
	return h.handler.HandleGetUnreadCount(ctx, q)
}

type GetNotificationByUIDHandler struct {
	handler *queries.QueryHandler
}

func NewGetNotificationByUIDHandler(handler *queries.QueryHandler) *GetNotificationByUIDHandler {
	return &GetNotificationByUIDHandler{
		handler: handler,
	}
}

func (h *GetNotificationByUIDHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(queries.GetNotificationByUIDQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for GetNotificationByUIDHandler")
	}
	return h.handler.HandleGetNotificationByUID(ctx, q)
}

// Issue query handlers

type IssueGetByIDHandler struct {
	handler *queries.IssueQueryHandler
}

func NewIssueGetByIDHandler(handler *queries.IssueQueryHandler) *IssueGetByIDHandler {
	return &IssueGetByIDHandler{handler: handler}
}

func (h *IssueGetByIDHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(queries.GetIssueByIDQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for IssueGetByIDHandler")
	}
	return h.handler.HandleGetIssueByID(ctx, q)
}

type IssueListHandler struct {
	handler *queries.IssueQueryHandler
}

func NewIssueListHandler(handler *queries.IssueQueryHandler) *IssueListHandler {
	return &IssueListHandler{handler: handler}
}

func (h *IssueListHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(queries.ListIssuesQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for IssueListHandler")
	}
	return h.handler.HandleListIssues(ctx, q)
}
