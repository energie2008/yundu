package model

import (
	"time"

	"github.com/google/uuid"
)

type NotificationPreferences struct {
	NotifyExpiry      bool `json:"notify_expiry"`
	NotifyTraffic     bool `json:"notify_traffic"`
	NotifyTicketReply bool `json:"notify_ticket_reply"`
}

type WithdrawStatus int

const (
	WithdrawStatusPending  WithdrawStatus = 0
	WithdrawStatusPaid     WithdrawStatus = 1
	WithdrawStatusRejected WithdrawStatus = 2
)

type Withdraw struct {
	ID        uuid.UUID      `json:"id" db:"id"`
	UserID    uuid.UUID      `json:"user_id" db:"user_id"`
	Amount    float64        `json:"amount" db:"amount"`
	Method    string         `json:"method" db:"method"`
	Account   string         `json:"account" db:"account"`
	RealName  *string        `json:"real_name,omitempty" db:"real_name"`
	Status    WithdrawStatus `json:"status" db:"status"`
	Remark    *string        `json:"remark,omitempty" db:"remark"`
	HandledBy *uuid.UUID     `json:"handled_by,omitempty" db:"handled_by"`
	HandledAt *time.Time     `json:"handled_at,omitempty" db:"handled_at"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at"`
}

type CreateWithdrawRequest struct {
	Amount   float64 `json:"amount" binding:"required,min=1"`
	Method   string  `json:"method" binding:"required,oneof=alipay usdt paypal"`
	Account  string  `json:"account" binding:"required"`
	RealName string  `json:"real_name,omitempty"`
}

type WithdrawResponse struct {
	ID        uuid.UUID      `json:"id"`
	UserID    uuid.UUID      `json:"user_id"`
	Amount    float64        `json:"amount"`
	Method    string         `json:"method"`
	Account   string         `json:"account"`
	RealName  *string        `json:"real_name,omitempty"`
	Status    WithdrawStatus `json:"status"`
	Remark    *string        `json:"remark,omitempty"`
	HandledBy *uuid.UUID     `json:"handled_by,omitempty"`
	HandledAt *time.Time     `json:"handled_at,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type CommissionSummary struct {
	AvailableBalance  float64 `json:"available_balance"`
	TotalEarned       float64 `json:"total_earned"`
	PendingSettlement float64 `json:"pending_settlement"`
	InvitedCount      int     `json:"invited_count"`
	WithdrawnTotal    float64 `json:"withdrawn_total"`
	Rate              int     `json:"rate"`
	MinWithdraw       float64 `json:"min_withdraw"`
	WithdrawEnabled   bool    `json:"withdraw_enabled"`
}

func NewWithdrawResponse(w *Withdraw) WithdrawResponse {
	return WithdrawResponse{
		ID:        w.ID,
		UserID:    w.UserID,
		Amount:    w.Amount,
		Method:    w.Method,
		Account:   w.Account,
		RealName:  w.RealName,
		Status:    w.Status,
		Remark:    w.Remark,
		HandledBy: w.HandledBy,
		HandledAt: w.HandledAt,
		CreatedAt: w.CreatedAt,
		UpdatedAt: w.UpdatedAt,
	}
}
