package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/Silo-Server/silo-server/internal/adminjob"
	"github.com/Silo-Server/silo-server/internal/models"
)

// ListAdminTaskJobs uses repository pagination without exposing legacy HTTP DTOs.
func (h *AdminJobsHandler) ListAdminTaskJobs(ctx context.Context, kind string, before time.Time, id string, limit int) ([]*models.AdminJob, error) {
	repo, ok := h.repo.(interface {
		ListPage(context.Context, string, time.Time, string, int) ([]*models.AdminJob, error)
	})
	if !ok {
		return nil, fmt.Errorf("paged admin jobs unavailable")
	}
	return repo.ListPage(ctx, kind, before, id, limit)
}
func (h *AdminJobsHandler) GetAdminTaskJob(ctx context.Context, id string) (*models.AdminJob, error) {
	return h.repo.GetByID(ctx, id)
}
func (h *AdminJobsHandler) AdminTaskJobDownload(ctx context.Context, job *models.AdminJob) (string, *time.Time) {
	if h.store == nil || job.Status != adminjob.StatusCompleted || job.ArtifactBucket == "" || job.ArtifactKey == "" {
		return "", nil
	}
	expiry := time.Now().UTC().Add(adminJobDownloadExpiry)
	url, err := h.store.PresignGetURL(ctx, job.ArtifactBucket, job.ArtifactKey, adminJobDownloadExpiry)
	if err != nil {
		return "", nil
	}
	return url, &expiry
}
