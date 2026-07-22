package model

import "time"

// KnowledgeCategory 知识库分类
type KnowledgeCategory struct {
	ID        int64     `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Sort      int       `json:"sort" db:"sort"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// KnowledgeArticle 知识库文章
type KnowledgeArticle struct {
	ID        int64     `json:"id" db:"id"`
	Category  string    `json:"category" db:"category"`
	Title     string    `json:"title" db:"title"`
	Body      string    `json:"body" db:"body"`
	Show      int       `json:"show" db:"show"`
	Sort      int       `json:"sort" db:"sort"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// KnowledgeArticleWithCategory 文章带分类名（管理端列表用）
type KnowledgeArticleWithCategory struct {
	KnowledgeArticle
	CategoryName string `json:"category_name"`
}

// SaveKnowledgeRequest 保存分类或文章的请求（XBoard 兼容：type=category 走分类，否则文章）
type SaveKnowledgeRequest struct {
	ID         int64  `json:"id,omitempty"`
	Type       string `json:"type,omitempty"`        // "category" 表示保存分类
	Name       string `json:"name,omitempty"`        // 分类名
	Sort       int    `json:"sort,omitempty"`        // 排序
	Category   string `json:"category,omitempty"`    // 文章分类标识（分类名或id字符串）
	CategoryID int64  `json:"category_id,omitempty"` // 文章分类ID（YunDu 风格）
	Title      string `json:"title,omitempty"`       // 文章标题
	Body       string `json:"body,omitempty"`        // 文章内容
	Show       int    `json:"show,omitempty"`        // 1=显示 0=隐藏
}

// DropKnowledgeRequest 删除请求
type DropKnowledgeRequest struct {
	ID   int64  `json:"id"`
	Type string `json:"type,omitempty"` // "category" 表示删除分类
}

// ShowKnowledgeRequest 切换显示请求
type ShowKnowledgeRequest struct {
	ID   int64 `json:"id"`
	Show int   `json:"show"`
}
