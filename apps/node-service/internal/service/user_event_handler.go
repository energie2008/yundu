package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/airport-panel/config/events"
	"github.com/airport-panel/node-service/internal/repo"
	"github.com/google/uuid"
)

type UserEventHandler struct {
	deploymentSvc *DeploymentService
	runtimeRepo   *repo.RuntimeRepo
	credRepo      *repo.UserNodeCredentialRepo
	deltaSvc      *UserDeltaService
	logger        *slog.Logger
}

func NewUserEventHandler(
	deploymentSvc *DeploymentService,
	runtimeRepo *repo.RuntimeRepo,
	credRepo *repo.UserNodeCredentialRepo,
	deltaSvc *UserDeltaService,
	logger *slog.Logger,
) *UserEventHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &UserEventHandler{
		deploymentSvc: deploymentSvc,
		runtimeRepo:   runtimeRepo,
		credRepo:      credRepo,
		deltaSvc:      deltaSvc,
		logger:        logger.With("component", "user_event_handler"),
	}
}

func (h *UserEventHandler) HandleUserBanned(evt events.Event) {
	var payload events.UserEvent
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		h.logger.Error("HandleUserBanned: unmarshal payload failed", "error", err)
		return
	}
	userID, err := uuid.Parse(payload.UserID)
	if err != nil {
		h.logger.Error("HandleUserBanned: invalid user_id", "user_id", payload.UserID, "error", err)
		return
	}

	ctx := context.Background()
	h.logger.Info("user banned, pushing ban to all active nodes",
		"user_id", payload.UserID, "reason", payload.Reason, "operator", payload.Operator)

	userIdent := userID.String()
	h.deploymentSvc.PushUserBanToAllServers(ctx, []string{userIdent}, payload.Reason)
}

func (h *UserEventHandler) HandleUserUnbanned(evt events.Event) {
	var payload events.UserEvent
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		h.logger.Error("HandleUserUnbanned: unmarshal payload failed", "error", err)
		return
	}
	userID, err := uuid.Parse(payload.UserID)
	if err != nil {
		h.logger.Error("HandleUserUnbanned: invalid user_id", "user_id", payload.UserID, "error", err)
		return
	}

	ctx := context.Background()
	h.logger.Info("user unbanned, syncing via delta/full push",
		"user_id", payload.UserID, "operator", payload.Operator)

	h.pushUserAddToAllActive(ctx, userID)
}

func (h *UserEventHandler) HandleTrafficReset(evt events.Event) {
	var payload events.UserEvent
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		h.logger.Error("HandleTrafficReset: unmarshal payload failed", "error", err)
		return
	}

	ctx := context.Background()

	if payload.UserID == "*" {
		h.logger.Info("traffic reset for all users (monthly reset), triggering full config sync",
			"reason", payload.Reason, "operator", payload.Operator)
		h.triggerFullSyncForAllRuntimes(ctx)
		return
	}

	userID, err := uuid.Parse(payload.UserID)
	if err != nil {
		h.logger.Error("HandleTrafficReset: invalid user_id", "user_id", payload.UserID, "error", err)
		return
	}

	h.logger.Info("user traffic reset, syncing user to all active nodes",
		"user_id", payload.UserID, "operator", payload.Operator)

	h.pushUserAddToAllActive(ctx, userID)
}

func (h *UserEventHandler) triggerFullSyncForAllRuntimes(ctx context.Context) {
	runtimes, err := h.runtimeRepo.ListActiveRuntimeServers(ctx)
	if err != nil {
		h.logger.Error("triggerFullSyncForAllRuntimes: list active runtimes failed", "error", err)
		return
	}
	renewed := 0
	for _, rt := range runtimes {
		if _, err := h.deploymentSvc.GetRuntimeConfig(ctx, rt.RuntimeID, ""); err != nil {
			h.logger.Warn("triggerFullSyncForAllRuntimes: rebuild config failed",
				"server_code", rt.ServerCode, "runtime_id", rt.RuntimeID, "error", err)
		} else {
			renewed++
		}
	}
	h.logger.Info("triggerFullSyncForAllRuntimes: completed", "renewed", renewed, "total", len(runtimes))
}

func (h *UserEventHandler) HandlePlanChanged(evt events.Event) {
	var payload struct {
		UserID   string `json:"user_id"`
		PlanID   string `json:"plan_id,omitempty"`
		Bytes    int64  `json:"bytes,omitempty"`
		Operator string `json:"operator,omitempty"`
	}
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		h.logger.Error("HandlePlanChanged: unmarshal payload failed", "error", err)
		return
	}
	userID, err := uuid.Parse(payload.UserID)
	if err != nil {
		h.logger.Error("HandlePlanChanged: invalid user_id", "user_id", payload.UserID, "error", err)
		return
	}

	ctx := context.Background()
	h.logger.Info("user plan changed, syncing to all active nodes",
		"user_id", payload.UserID, "operator", payload.Operator)

	h.pushUserAddToAllActive(ctx, userID)
}

func (h *UserEventHandler) HandleTokenRevoked(evt events.Event) {
	var payload events.TokenEvent
	if err := json.Unmarshal(evt.Data, &payload); err != nil {
		h.logger.Error("HandleTokenRevoked: unmarshal payload failed", "error", err)
		return
	}
	userID, err := uuid.Parse(payload.UserID)
	if err != nil {
		h.logger.Error("HandleTokenRevoked: invalid user_id", "user_id", payload.UserID, "error", err)
		return
	}

	ctx := context.Background()
	h.logger.Info("user token revoked, pushing ban to kick user",
		"user_id", payload.UserID, "reason", payload.Reason)

	userIdent := userID.String()
	h.deploymentSvc.PushUserBanToAllServers(ctx, []string{userIdent}, "token_revoked")
}

func (h *UserEventHandler) pushUserAddToAllActive(ctx context.Context, userID uuid.UUID) {
	cred, err := h.credRepo.GetByUserID(ctx, userID)
	if err != nil {
		h.logger.Error("pushUserAddToAllActive: get user cred failed", "user_id", userID, "error", err)
		return
	}

	runtimes, err := h.runtimeRepo.ListActiveRuntimeServers(ctx)
	if err != nil {
		h.logger.Error("pushUserAddToAllActive: list active runtimes failed", "error", err)
		return
	}
	if len(runtimes) == 0 {
		h.logger.Warn("pushUserAddToAllActive: no active runtimes found")
		return
	}

	if cred == nil {
		h.logger.Info("pushUserAddToAllActive: user has no active cred (may be banned/inactive), skipping add",
			"user_id", userID)
		return
	}

	if h.deltaSvc != nil {
		extra := make(map[string]string)
		if cred.SpeedLimitMbps > 0 {
			extra["speed_limit_mbps"] = fmt.Sprintf("%d", cred.SpeedLimitMbps)
		}
		if cred.DeviceLimit > 0 {
			extra["device_limit"] = fmt.Sprintf("%d", cred.DeviceLimit)
		}
		adds := []UserChangeEntry{
			{
				Email:    cred.Email,
				UUID:     cred.CredentialValue,
				Level:    0,
				Password: cred.CredentialValue,
				Extra:    extra,
			},
		}
		configVersion := time.Now().UnixMilli()
		successCount := 0
		for _, rt := range runtimes {
			if err := h.deltaSvc.OnUsersChanged(ctx, rt.ServerCode, rt.RuntimeType, adds, nil, configVersion); err != nil {
				h.logger.Debug("pushUserAddToAllActive: delta push failed, will rely on heartbeat fallback",
					"server_code", rt.ServerCode, "error", err)
			} else {
				successCount++
			}
		}
		h.logger.Info("pushUserAddToAllActive: delta sync completed",
			"user_id", userID, "email", cred.Email, "success", successCount, "total", len(runtimes))
		return
	}

	h.logger.Info("pushUserAddToAllActive: delta service not available, using ban-push as fallback (agent will full-sync)",
		"user_id", userID)
	h.deploymentSvc.PushUserBanToAllServers(ctx, []string{userID.String()}, "user_updated")
}
