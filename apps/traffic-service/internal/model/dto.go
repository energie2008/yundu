package model

import (
	"time"

	"github.com/google/uuid"
)

type PaginationResponse struct {
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int         `json:"total"`
	Items    interface{} `json:"items"`
}

type TrafficReportItem struct {
	// UserID 可选：旧版 node-agent 直接上报 user_id。
	// 当 Credential 非空且能反查到用户时，会被反查到的 user_id 覆盖。
	// 新版 agent 仅上报 Credential（用户 UUID 或 email），UserID 留空。
	UserID        uuid.UUID `json:"user_id,omitempty"`
	NodeID        *uuid.UUID `json:"node_id"`
	// Credential 可选：node-agent 上报的 per-user UUID 或 email。
	// 当提供该字段时，traffic-service 会通过 users 表反查 user_id。
	Credential    string    `json:"credential,omitempty"`
	UploadBytes   int64     `json:"upload_bytes"`
	DownloadBytes int64     `json:"download_bytes"`
	Timestamp     time.Time `json:"timestamp"`
}

type TrafficReportRequest struct {
	ServerCode string              `json:"server_code" binding:"required"`
	Reports    []TrafficReportItem `json:"reports" binding:"required"`
}

type DailyTrafficItem struct {
	Date          string `json:"date"`
	UploadBytes   int64  `json:"upload_bytes"`
	DownloadBytes int64  `json:"download_bytes"`
	TotalBytes    int64  `json:"total_bytes"`
}

type UserTrafficResponse struct {
	UserID         uuid.UUID          `json:"user_id"`
	StartDate      string             `json:"start_date"`
	EndDate        string             `json:"end_date"`
	TotalUpload    int64              `json:"total_upload"`
	TotalDownload  int64              `json:"total_download"`
	TotalBytes     int64              `json:"total_bytes"`
	DailyBreakdown []DailyTrafficItem `json:"daily_breakdown"`
	Quota          QuotaCheckResult   `json:"quota"`
}

type NodeTrafficItem struct {
	NodeID        uuid.UUID `json:"node_id"`
	UploadBytes   int64     `json:"upload_bytes"`
	DownloadBytes int64     `json:"download_bytes"`
	TotalBytes    int64     `json:"total_bytes"`
}

type OverviewResponse struct {
	TodayUpload    int64             `json:"today_upload"`
	TodayDownload  int64             `json:"today_download"`
	TodayTotal     int64             `json:"today_total"`
	OnlineCount    int64             `json:"online_count"`
	TopNodes       []NodeTrafficItem `json:"top_nodes"`
}

type SessionResponse struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	NodeID     *uuid.UUID `json:"node_id,omitempty"`
	ClientIP   *string    `json:"client_ip,omitempty"`
	ClientType *string    `json:"client_type,omitempty"`
	ConnectedAt string    `json:"connected_at"`
	LastSeenAt  string    `json:"last_seen_at"`
}

func NewSessionResponse(s *OnlineSession) SessionResponse {
	return SessionResponse{
		ID:          s.ID,
		UserID:      s.UserID,
		NodeID:      s.NodeID,
		ClientIP:    s.ClientIP,
		ClientType:  s.ClientType,
		ConnectedAt: s.ConnectedAt.Format("2006-01-02T15:04:05Z07:00"),
		LastSeenAt:  s.LastSeenAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
