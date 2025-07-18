package issues

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/shared"
)

// Issue query handlers

type IssueGetByIDHandler struct {
	handler *IssueQueryHandler
}

var _ shared.QueryHandler = (*IssueGetByIDHandler)(nil)

func NewIssueGetByIDHandler(handler *IssueQueryHandler) *IssueGetByIDHandler {
	return &IssueGetByIDHandler{handler: handler}
}

func (h *IssueGetByIDHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(GetIssueByIDQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for IssueGetByIDHandler")
	}
	return h.handler.HandleGetIssueByID(ctx, q)
}

type IssueListHandler struct {
	handler *IssueQueryHandler
}

var _ shared.QueryHandler = (*IssueListHandler)(nil)

func NewIssueListHandler(handler *IssueQueryHandler) *IssueListHandler {
	return &IssueListHandler{handler: handler}
}

func (h *IssueListHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(ListIssuesQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for IssueListHandler")
	}
	return h.handler.HandleListIssues(ctx, q)
}

type GetIssueByKeyHandler struct {
	handler *IssueQueryHandler
}

var _ shared.QueryHandler = (*GetIssueByKeyHandler)(nil)

func NewGetIssueByKeyHandler(handler *IssueQueryHandler) *GetIssueByKeyHandler {
	return &GetIssueByKeyHandler{handler: handler}
}

func (h *GetIssueByKeyHandler) Handle(ctx context.Context, query interface{}) (interface{}, error) {
	q, ok := query.(GetIssueByKeyQuery)
	if !ok {
		return nil, fmt.Errorf("invalid query type for GetIssueByKeyHandler")
	}
	return h.handler.HandleGetIssueByKey(ctx, q)
}
