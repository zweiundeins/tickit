package notifications

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/shared"
)

type GetNotificationsHandler struct {
	handler *QueryHandler
}

var _ shared.QueryHandler = (*GetNotificationsHandler)(nil)

func NewGetNotificationsHandler(handler *QueryHandler) *GetNotificationsHandler {
	return &GetNotificationsHandler{
		handler: handler,
	}
}

func (h *GetNotificationsHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(GetNotificationsQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for GetNotificationsHandler")
	}
	return h.handler.HandleGetNotifications(ctx, q)
}

type GetUnreadCountHandler struct {
	handler *QueryHandler
}

var _ shared.QueryHandler = (*GetUnreadCountHandler)(nil)

func NewGetUnreadCountHandler(handler *QueryHandler) *GetUnreadCountHandler {
	return &GetUnreadCountHandler{
		handler: handler,
	}
}

func (h *GetUnreadCountHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(GetUnreadCountQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for GetUnreadCountHandler")
	}
	return h.handler.HandleGetUnreadCount(ctx, q)
}

type GetNotificationByUIDHandler struct {
	handler *QueryHandler
}

var _ shared.QueryHandler = (*GetNotificationByUIDHandler)(nil)

func NewGetNotificationByUIDHandler(handler *QueryHandler) *GetNotificationByUIDHandler {
	return &GetNotificationByUIDHandler{
		handler: handler,
	}
}

func (h *GetNotificationByUIDHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(GetNotificationByUIDQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for GetNotificationByUIDHandler")
	}
	return h.handler.HandleGetNotificationByUID(ctx, q)
}
