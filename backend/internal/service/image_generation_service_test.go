package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestImageGenerationService_SaveResultSetsExpiryAndListsByUser(t *testing.T) {
	repo := newMemoryImageGenerationRepo()
	dir := t.TempDir()
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	svc := NewImageGenerationService(repo, nil, nil, nil, nil, ImageGenerationStorageConfig{
		BaseDir:       dir,
		RetentionDays: 7,
		Now:           func() time.Time { return now },
	})

	task, err := svc.CreateCompletedTask(context.Background(), ImageGenerationCreateCompletedInput{
		UserID: 11,
		Mode:   ImageGenerationModeGeneration,
		Model:  "gpt-image-2",
		Prompt: "a quiet desk",
		Images: []ImageGenerationOutputImage{{
			MimeType: "image/png",
			Base64:   "iVBORw0KGgo=",
		}},
	})
	require.NoError(t, err)
	require.Equal(t, int64(11), task.UserID)
	require.Equal(t, ImageGenerationStatusSucceeded, task.Status)
	require.WithinDuration(t, now.AddDate(0, 0, 7), task.ExpiresAt, time.Second)
	require.Len(t, task.Results, 1)

	_, err = os.Stat(filepath.Join(dir, task.Results[0].StoragePath))
	require.NoError(t, err)

	items, _, err := svc.ListTasks(context.Background(), 11, ImageGenerationListParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, items, 1)

	otherItems, _, err := svc.ListTasks(context.Background(), 12, ImageGenerationListParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Empty(t, otherItems)
}

func TestImageGenerationService_CleanupExpiredImagesDeletesOnlyExpiredFiles(t *testing.T) {
	repo := newMemoryImageGenerationRepo()
	dir := t.TempDir()
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	svc := NewImageGenerationService(repo, nil, nil, nil, nil, ImageGenerationStorageConfig{
		BaseDir:       dir,
		RetentionDays: 1,
		Now:           func() time.Time { return now },
	})

	expired, err := svc.CreateCompletedTask(context.Background(), ImageGenerationCreateCompletedInput{
		UserID:    11,
		Mode:      ImageGenerationModeGeneration,
		Model:     "gpt-image-2",
		Prompt:    "expired",
		ExpiresAt: imageGenerationPtrTime(now.Add(-time.Hour)),
		Images: []ImageGenerationOutputImage{{
			MimeType: "image/png",
			Base64:   "iVBORw0KGgo=",
		}},
	})
	require.NoError(t, err)
	fresh, err := svc.CreateCompletedTask(context.Background(), ImageGenerationCreateCompletedInput{
		UserID:    11,
		Mode:      ImageGenerationModeGeneration,
		Model:     "gpt-image-2",
		Prompt:    "fresh",
		ExpiresAt: imageGenerationPtrTime(now.Add(time.Hour)),
		Images: []ImageGenerationOutputImage{{
			MimeType: "image/png",
			Base64:   "iVBORw0KGgo=",
		}},
	})
	require.NoError(t, err)

	result, err := svc.CleanupExpired(context.Background(), 50)
	require.NoError(t, err)
	require.Equal(t, 1, result.TasksCleaned)
	require.Equal(t, 1, result.FilesDeleted)

	_, err = os.Stat(filepath.Join(dir, expired.Results[0].StoragePath))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dir, fresh.Results[0].StoragePath))
	require.NoError(t, err)
}

func TestImageGenerationService_GetResultFileRequiresOwner(t *testing.T) {
	repo := newMemoryImageGenerationRepo()
	dir := t.TempDir()
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	svc := NewImageGenerationService(repo, nil, nil, nil, nil, ImageGenerationStorageConfig{
		BaseDir:       dir,
		RetentionDays: 7,
		Now:           func() time.Time { return now },
	})

	task, err := svc.CreateCompletedTask(context.Background(), ImageGenerationCreateCompletedInput{
		UserID: 11,
		Mode:   ImageGenerationModeGeneration,
		Model:  "gpt-image-2",
		Prompt: "source",
		Images: []ImageGenerationOutputImage{{
			MimeType: "image/png",
			Base64:   "iVBORw0KGgo=",
		}},
	})
	require.NoError(t, err)

	result, abs, err := svc.GetResultFile(context.Background(), 11, task.Results[0].ID)
	require.NoError(t, err)
	require.Equal(t, "image/png", result.MimeType)
	require.FileExists(t, abs)

	_, _, err = svc.GetResultFile(context.Background(), 12, task.Results[0].ID)
	require.ErrorIs(t, err, ErrImageGenerationTaskNotFound)
}

func TestImageGenerationService_PendingTaskIsListedAndCanComplete(t *testing.T) {
	repo := newMemoryImageGenerationRepo()
	dir := t.TempDir()
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	svc := NewImageGenerationService(repo, nil, nil, nil, nil, ImageGenerationStorageConfig{
		BaseDir:       dir,
		RetentionDays: 7,
		Now:           func() time.Time { return now },
	})

	pending, err := svc.CreatePendingTaskFromRequest(context.Background(), 11, ImageGenerationModeGeneration, []byte(`{"model":"gpt-image-2","prompt":"a tiny robot","size":"1024x1024"}`))
	require.NoError(t, err)
	require.Equal(t, ImageGenerationStatusPending, pending.Status)
	require.Empty(t, pending.Results)

	items, _, err := svc.ListTasks(context.Background(), 11, ImageGenerationListParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, ImageGenerationStatusPending, items[0].Status)

	completed, err := svc.CompletePendingTaskFromResponse(context.Background(), pending.ID, []byte(`{"data":[{"b64_json":"iVBORw0KGgo="}]}`))
	require.NoError(t, err)
	require.Equal(t, ImageGenerationStatusSucceeded, completed.Status)
	require.Len(t, completed.Results, 1)
	require.FileExists(t, filepath.Join(dir, completed.Results[0].StoragePath))
}

func imageGenerationPtrTime(t time.Time) *time.Time {
	return &t
}
