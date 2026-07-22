package service

import (
	"context"
	"encoding/json"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/repo"
	"github.com/google/uuid"
)

type AuditService struct {
	auditRepo *repo.AuditRepo
}

func NewAuditService(auditRepo *repo.AuditRepo) *AuditService {
	return &AuditService{auditRepo: auditRepo}
}

func (s *AuditService) Write(ctx context.Context, actorType model.ActorType, actorID *uuid.UUID, actorDisplay *string, action, resourceType string, resourceID *uuid.UUID, requestID, ipAddress, userAgent string, before, after interface{}, metadata map[string]interface{}) error {
	log := &model.AuditLog{
		ID:           uuid.New(),
		ActorType:    actorType,
		ActorID:      actorID,
		ActorDisplay: actorDisplay,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Metadata:     json.RawMessage(`{}`),
	}
	if requestID != "" {
		log.RequestID = &requestID
	}
	if ipAddress != "" {
		log.IPAddress = &ipAddress
	}
	if userAgent != "" {
		log.UserAgent = &userAgent
	}
	if before != nil {
		b, err := json.Marshal(before)
		if err == nil {
			log.BeforeJSON = b
		}
	}
	if after != nil {
		b, err := json.Marshal(after)
		if err == nil {
			log.AfterJSON = b
		}
	}
	if metadata != nil {
		b, err := json.Marshal(metadata)
		if err == nil {
			log.Metadata = b
		}
	}
	return s.auditRepo.Create(ctx, log)
}

func (s *AuditService) ListAuditLogs(ctx context.Context, page, pageSize int, actorType, actorID, resourceType, resourceID, action string) ([]*model.AuditLog, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.auditRepo.List(ctx, page, pageSize, actorType, actorID, resourceType, resourceID, action)
}
