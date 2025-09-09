package users

import (
	"context"
	"fmt"

	"github.com/mbolli/tickit/internal/shared"
)

// Individual command handler wrappers that implement shared.CommandHandler

// RegisterUserHandler wraps the RegisterUser command
type RegisterUserHandler struct {
	handler *UserCommandHandler
}

var _ shared.CommandHandler = (*RegisterUserHandler)(nil)

func NewRegisterUserHandler(handler *UserCommandHandler) *RegisterUserHandler {
	return &RegisterUserHandler{handler: handler}
}

func (h *RegisterUserHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(RegisterUserCommand)
	if !ok {
		return fmt.Errorf("invalid command type for RegisterUserHandler")
	}
	return h.handler.HandleRegisterUser(ctx, cmd)
}

// VerifyEmailHandler wraps the VerifyEmail command
type VerifyEmailHandler struct {
	handler *UserCommandHandler
}

var _ shared.CommandHandler = (*VerifyEmailHandler)(nil)

func NewVerifyEmailHandler(handler *UserCommandHandler) *VerifyEmailHandler {
	return &VerifyEmailHandler{handler: handler}
}

func (h *VerifyEmailHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(VerifyEmailCommand)
	if !ok {
		return fmt.Errorf("invalid command type for VerifyEmailHandler")
	}
	return h.handler.HandleVerifyEmail(ctx, cmd)
}

// LoginUserHandler wraps the LoginUser command
type LoginUserHandler struct {
	handler *UserCommandHandler
}

var _ shared.CommandHandler = (*LoginUserHandler)(nil)

func NewLoginUserHandler(handler *UserCommandHandler) *LoginUserHandler {
	return &LoginUserHandler{handler: handler}
}

func (h *LoginUserHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(LoginUserCommand)
	if !ok {
		return fmt.Errorf("invalid command type for LoginUserHandler")
	}
	return h.handler.HandleLoginUser(ctx, cmd)
}

// ForgotPasswordHandler wraps the ForgotPassword command
type ForgotPasswordHandler struct {
	handler *UserCommandHandler
}

var _ shared.CommandHandler = (*ForgotPasswordHandler)(nil)

func NewForgotPasswordHandler(handler *UserCommandHandler) *ForgotPasswordHandler {
	return &ForgotPasswordHandler{handler: handler}
}

func (h *ForgotPasswordHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(ForgotPasswordCommand)
	if !ok {
		return fmt.Errorf("invalid command type for ForgotPasswordHandler")
	}
	return h.handler.HandleForgotPassword(ctx, cmd)
}

// ResetPasswordHandler wraps the ResetPassword command
type ResetPasswordHandler struct {
	handler *UserCommandHandler
}

var _ shared.CommandHandler = (*ResetPasswordHandler)(nil)

func NewResetPasswordHandler(handler *UserCommandHandler) *ResetPasswordHandler {
	return &ResetPasswordHandler{handler: handler}
}

func (h *ResetPasswordHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(ResetPasswordCommand)
	if !ok {
		return fmt.Errorf("invalid command type for ResetPasswordHandler")
	}
	return h.handler.HandleResetPassword(ctx, cmd)
}

// ChangePasswordHandler wraps the ChangePassword command
type ChangePasswordHandler struct {
	handler *UserCommandHandler
}

var _ shared.CommandHandler = (*ChangePasswordHandler)(nil)

func NewChangePasswordHandler(handler *UserCommandHandler) *ChangePasswordHandler {
	return &ChangePasswordHandler{handler: handler}
}

func (h *ChangePasswordHandler) Handle(ctx context.Context, command interface{}) error {
	cmd, ok := command.(ChangePasswordCommand)
	if !ok {
		return fmt.Errorf("invalid command type for ChangePasswordHandler")
	}
	return h.handler.HandleChangePassword(ctx, cmd)
}
