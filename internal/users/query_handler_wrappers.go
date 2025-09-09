package users

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/shared"
)

// Individual query handler wrappers that implement shared.QueryHandler

// GetUserByIDHandler wraps the GetUserByID query
type GetUserByIDHandler struct {
	handler *UserQueryHandler
}

var _ shared.QueryHandler = (*GetUserByIDHandler)(nil)

func NewGetUserByIDHandler(handler *UserQueryHandler) *GetUserByIDHandler {
	return &GetUserByIDHandler{handler: handler}
}

func (h *GetUserByIDHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(GetUserByIDQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for GetUserByIDHandler")
	}
	return h.handler.HandleGetUserByID(ctx, q)
}

// GetUserByEmailHandler wraps the GetUserByEmail query
type GetUserByEmailHandler struct {
	handler *UserQueryHandler
}

var _ shared.QueryHandler = (*GetUserByEmailHandler)(nil)

func NewGetUserByEmailHandler(handler *UserQueryHandler) *GetUserByEmailHandler {
	return &GetUserByEmailHandler{handler: handler}
}

func (h *GetUserByEmailHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(GetUserByEmailQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for GetUserByEmailHandler")
	}
	return h.handler.HandleGetUserByEmail(ctx, q)
}

// GetUserByUsernameHandler wraps the GetUserByUsername query
type GetUserByUsernameHandler struct {
	handler *UserQueryHandler
}

var _ shared.QueryHandler = (*GetUserByUsernameHandler)(nil)

func NewGetUserByUsernameHandler(handler *UserQueryHandler) *GetUserByUsernameHandler {
	return &GetUserByUsernameHandler{handler: handler}
}

func (h *GetUserByUsernameHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(GetUserByUsernameQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for GetUserByUsernameHandler")
	}
	return h.handler.HandleGetUserByUsername(ctx, q)
}

// ListUsersHandler wraps the ListUsers query
type ListUsersHandler struct {
	handler *UserQueryHandler
}

var _ shared.QueryHandler = (*ListUsersHandler)(nil)

func NewListUsersHandler(handler *UserQueryHandler) *ListUsersHandler {
	return &ListUsersHandler{handler: handler}
}

func (h *ListUsersHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(ListUsersQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for ListUsersHandler")
	}
	return h.handler.HandleListUsers(ctx, q)
}

// SearchUsersHandler wraps the SearchUsers query
type SearchUsersHandler struct {
	handler *UserQueryHandler
}

var _ shared.QueryHandler = (*SearchUsersHandler)(nil)

func NewSearchUsersHandler(handler *UserQueryHandler) *SearchUsersHandler {
	return &SearchUsersHandler{handler: handler}
}

func (h *SearchUsersHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(SearchUsersQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for SearchUsersHandler")
	}
	return h.handler.HandleSearchUsers(ctx, q)
}