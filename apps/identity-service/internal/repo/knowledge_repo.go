package repo

import (
	"context"
	"fmt"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type KnowledgeRepo struct {
	pool *pgxpool.Pool
}

func NewKnowledgeRepo(pool *pgxpool.Pool) *KnowledgeRepo {
	return &KnowledgeRepo{pool: pool}
}

// ===== Category =====

const knowledgeCategoryColumns = "id, name, sort, created_at, updated_at"

func scanKnowledgeCategory(row pgx.Row, c *model.KnowledgeCategory) error {
	return row.Scan(&c.ID, &c.Name, &c.Sort, &c.CreatedAt, &c.UpdatedAt)
}

func (r *KnowledgeRepo) ListCategories(ctx context.Context) ([]*model.KnowledgeCategory, error) {
	query := "SELECT " + knowledgeCategoryColumns + " FROM knowledge_categories ORDER BY sort ASC, id ASC"
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*model.KnowledgeCategory
	for rows.Next() {
		c := &model.KnowledgeCategory{}
		if err := scanKnowledgeCategory(rows, c); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}

func (r *KnowledgeRepo) GetCategoryByID(ctx context.Context, id int64) (*model.KnowledgeCategory, error) {
	query := "SELECT " + knowledgeCategoryColumns + " FROM knowledge_categories WHERE id = $1"
	c := &model.KnowledgeCategory{}
	if err := scanKnowledgeCategory(r.pool.QueryRow(ctx, query, id), c); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (r *KnowledgeRepo) CreateCategory(ctx context.Context, c *model.KnowledgeCategory) error {
	query := "INSERT INTO knowledge_categories (name, sort) VALUES ($1, $2) RETURNING id, created_at, updated_at"
	return r.pool.QueryRow(ctx, query, c.Name, c.Sort).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *KnowledgeRepo) UpdateCategory(ctx context.Context, c *model.KnowledgeCategory) error {
	query := "UPDATE knowledge_categories SET name = $2, sort = $3, updated_at = now() WHERE id = $1"
	_, err := r.pool.Exec(ctx, query, c.ID, c.Name, c.Sort)
	return err
}

func (r *KnowledgeRepo) DeleteCategory(ctx context.Context, id int64) error {
	query := "DELETE FROM knowledge_categories WHERE id = $1"
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

// ===== Article =====

const knowledgeArticleColumns = "id, category, title, body, show, sort, created_at, updated_at"

func scanKnowledgeArticle(row pgx.Row, a *model.KnowledgeArticle) error {
	return row.Scan(&a.ID, &a.Category, &a.Title, &a.Body, &a.Show, &a.Sort, &a.CreatedAt, &a.UpdatedAt)
}

func (r *KnowledgeRepo) ListArticles(ctx context.Context, onlyShown bool) ([]*model.KnowledgeArticle, error) {
	query := "SELECT " + knowledgeArticleColumns + " FROM knowledge_articles"
	if onlyShown {
		query += " WHERE show = 1"
	}
	query += " ORDER BY sort DESC, id DESC"
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*model.KnowledgeArticle
	for rows.Next() {
		a := &model.KnowledgeArticle{}
		if err := scanKnowledgeArticle(rows, a); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

func (r *KnowledgeRepo) GetArticleByID(ctx context.Context, id int64) (*model.KnowledgeArticle, error) {
	query := "SELECT " + knowledgeArticleColumns + " FROM knowledge_articles WHERE id = $1"
	a := &model.KnowledgeArticle{}
	if err := scanKnowledgeArticle(r.pool.QueryRow(ctx, query, id), a); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

func (r *KnowledgeRepo) CreateArticle(ctx context.Context, a *model.KnowledgeArticle) error {
	query := `INSERT INTO knowledge_articles (category, title, body, show, sort)
		VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, query, a.Category, a.Title, a.Body, a.Show, a.Sort).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

func (r *KnowledgeRepo) UpdateArticle(ctx context.Context, a *model.KnowledgeArticle) error {
	query := `UPDATE knowledge_articles
		SET category = $2, title = $3, body = $4, show = $5, sort = $6, updated_at = now()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, a.ID, a.Category, a.Title, a.Body, a.Show, a.Sort)
	return err
}

func (r *KnowledgeRepo) UpdateArticleShow(ctx context.Context, id int64, show int) error {
	query := "UPDATE knowledge_articles SET show = $2, updated_at = now() WHERE id = $1"
	_, err := r.pool.Exec(ctx, query, id, show)
	return err
}

func (r *KnowledgeRepo) DeleteArticle(ctx context.Context, id int64) error {
	query := "DELETE FROM knowledge_articles WHERE id = $1"
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

// Helper for debug
func (r *KnowledgeRepo) Ping(ctx context.Context) error {
	var v int
	err := r.pool.QueryRow(ctx, "SELECT 1").Scan(&v)
	if err != nil {
		return fmt.Errorf("knowledge repo ping failed: %w", err)
	}
	return nil
}
