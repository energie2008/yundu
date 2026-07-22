package service

import (
	"context"
	"errors"

	"github.com/airport-panel/identity-service/internal/model"
	"github.com/airport-panel/identity-service/internal/repo"
)

type KnowledgeService struct {
	repo *repo.KnowledgeRepo
}

func NewKnowledgeService(r *repo.KnowledgeRepo) *KnowledgeService {
	return &KnowledgeService{repo: r}
}

// ListCategories returns all categories sorted by sort asc.
func (s *KnowledgeService) ListCategories(ctx context.Context) ([]*model.KnowledgeCategory, error) {
	return s.repo.ListCategories(ctx)
}

// ListArticles returns articles. If onlyShown=true, only show=1 articles are returned (for user side).
func (s *KnowledgeService) ListArticles(ctx context.Context, onlyShown bool) ([]*model.KnowledgeArticle, error) {
	return s.repo.ListArticles(ctx, onlyShown)
}

// SaveCategory creates or updates a category. If id>0, update; otherwise create.
func (s *KnowledgeService) SaveCategory(ctx context.Context, id int64, name string, sort int) (*model.KnowledgeCategory, error) {
	if name == "" {
		return nil, errors.New("category name required")
	}
	c := &model.KnowledgeCategory{ID: id, Name: name, Sort: sort}
	if id > 0 {
		if err := s.repo.UpdateCategory(ctx, c); err != nil {
			return nil, err
		}
		// refresh timestamps
		fresh, err := s.repo.GetCategoryByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if fresh != nil {
			return fresh, nil
		}
	} else {
		if err := s.repo.CreateCategory(ctx, c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// SaveArticle creates or updates an article. categoryID is resolved to category name.
func (s *KnowledgeService) SaveArticle(ctx context.Context, id int64, categoryID int64, category string, title string, body string, show int, sort int) (*model.KnowledgeArticle, error) {
	if title == "" {
		return nil, errors.New("title required")
	}
	if body == "" {
		return nil, errors.New("body required")
	}
	// Resolve category name from categoryID if provided
	if category == "" && categoryID > 0 {
		c, err := s.repo.GetCategoryByID(ctx, categoryID)
		if err != nil {
			return nil, err
		}
		if c != nil {
			category = c.Name
		}
	}
	if show == 0 {
		show = 1
	}
	a := &model.KnowledgeArticle{ID: id, Category: category, Title: title, Body: body, Show: show, Sort: sort}
	if id > 0 {
		if err := s.repo.UpdateArticle(ctx, a); err != nil {
			return nil, err
		}
	} else {
		if err := s.repo.CreateArticle(ctx, a); err != nil {
			return nil, err
		}
	}
	return a, nil
}

// DeleteCategoryOrArticle deletes a category (if type=="category") or article (otherwise).
func (s *KnowledgeService) DeleteCategoryOrArticle(ctx context.Context, id int64, typ string) error {
	if typ == "category" {
		return s.repo.DeleteCategory(ctx, id)
	}
	return s.repo.DeleteArticle(ctx, id)
}

// UpdateShow toggles article show flag.
func (s *KnowledgeService) UpdateShow(ctx context.Context, id int64, show int) error {
	return s.repo.UpdateArticleShow(ctx, id, show)
}
