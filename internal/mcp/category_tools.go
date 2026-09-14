package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kubera/internal/domain"
)

func (h *Handler) registerCategoryTools(s *mcp.Server) {
	mcp.AddTool[ListCategoriesIn, any](s, &mcp.Tool{
		Name:        "list_categories",
		Description: "List categories, active by default; set include_archived to also list archived ones.",
	}, logTool(h, "list_categories", h.ListCategories))

	mcp.AddTool[CreateCategoryIn, any](s, &mcp.Tool{
		Name: "create_category",
		Description: "Create a user-defined category with a unique name among active categories. " +
			"A short description of what belongs in the category helps AI clients classify transactions.",
	}, logTool(h, "create_category", h.CreateCategory))

	mcp.AddTool[UpdateCategoryIn, any](s, &mcp.Tool{
		Name:        "update_category",
		Description: "Update a category's name and/or description. Historical transactions keep their association. Provide at least one field; send description as an empty string to clear it.",
	}, logTool(h, "update_category", h.UpdateCategory))

	mcp.AddTool[ArchiveCategoryIn, any](s, &mcp.Tool{
		Name:        "archive_category",
		Description: "Archive a category. Archived categories cannot be assigned to new or updated transactions. Idempotent.",
	}, logTool(h, "archive_category", h.ArchiveCategory))
}

type ListCategoriesIn struct {
	IncludeArchived bool `json:"include_archived,omitempty"`
}

func (h *Handler) ListCategories(ctx context.Context, req *mcp.CallToolRequest, in ListCategoriesIn) (*mcp.CallToolResult, any, error) {
	cats, err := h.Svc.ListCategories(ctx, in.IncludeArchived)
	if err != nil {
		return toolErr(err, h.Log)
	}
	out := make([]CategoryJSON, 0, len(cats))
	for _, c := range cats {
		out = append(out, toCatJSON(c))
	}
	return toolOK(map[string]any{"categories": out})
}

type CreateCategoryIn struct {
	Name        string `json:"name" jsonschema:"user-visible category name, max 100 characters"`
	Description string `json:"description,omitempty" jsonschema:"optional description of what kinds of transactions belong in this category, max 500 characters"`
}

func (h *Handler) CreateCategory(ctx context.Context, req *mcp.CallToolRequest, in CreateCategoryIn) (*mcp.CallToolResult, any, error) {
	c, err := h.Svc.CreateCategory(ctx, in.Name, in.Description)
	if err != nil {
		return toolErr(err, h.Log)
	}
	return toolOK(map[string]any{"status": "category_created", "category": toCatJSON(c)})
}

type UpdateCategoryIn struct {
	CategoryID  string  `json:"category_id"`
	Name        *string `json:"name,omitempty" jsonschema:"new category name; omit to leave unchanged"`
	Description *string `json:"description,omitempty" jsonschema:"new description of what belongs in this category; omit to leave unchanged, empty string to clear"`
}

func (h *Handler) UpdateCategory(ctx context.Context, req *mcp.CallToolRequest, in UpdateCategoryIn) (*mcp.CallToolResult, any, error) {
	c, err := h.Svc.UpdateCategory(ctx, domain.CategoryID(in.CategoryID), in.Name, in.Description)
	if err != nil {
		return toolErr(err, h.Log)
	}
	return toolOK(map[string]any{"status": "category_updated", "category": toCatJSON(c)})
}

type ArchiveCategoryIn struct {
	CategoryID string `json:"category_id"`
}

func (h *Handler) ArchiveCategory(ctx context.Context, req *mcp.CallToolRequest, in ArchiveCategoryIn) (*mcp.CallToolResult, any, error) {
	c, err := h.Svc.ArchiveCategory(ctx, domain.CategoryID(in.CategoryID))
	if err != nil {
		return toolErr(err, h.Log)
	}
	return toolOK(map[string]any{"status": "category_archived", "category": toCatJSON(c)})
}
