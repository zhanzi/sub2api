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

func TestImageGenerationService_ExtractsAsyncTaskWrappedOutputs(t *testing.T) {
	repo := newMemoryImageGenerationRepo()
	dir := t.TempDir()
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	svc := NewImageGenerationService(repo, nil, nil, nil, nil, ImageGenerationStorageConfig{
		BaseDir:       dir,
		RetentionDays: 7,
		Now:           func() time.Time { return now },
	})

	pending, err := svc.CreatePendingTaskFromRequest(context.Background(), 11, ImageGenerationModeGeneration, []byte(`{"model":"gpt-image-2","prompt":"a tiny robot"}`))
	require.NoError(t, err)

	completed, err := svc.CompletePendingTaskFromResponse(context.Background(), pending.ID, []byte(`{
		"id":"sync-gen-1",
		"object":"image.task",
		"status":"completed",
		"result":{"url":"data:image/webp;base64,aGVsbG8="},
		"images":[{"b64_json":"d29ybGQ="}]
	}`))
	require.NoError(t, err)
	require.Equal(t, ImageGenerationStatusSucceeded, completed.Status)
	require.Len(t, completed.Results, 2)
	mimeTypes := []string{completed.Results[0].MimeType, completed.Results[1].MimeType}
	require.Contains(t, mimeTypes, "image/webp")
	require.Contains(t, mimeTypes, "image/png")
	require.FileExists(t, filepath.Join(dir, completed.Results[0].StoragePath))
	require.FileExists(t, filepath.Join(dir, completed.Results[1].StoragePath))
}

func TestImageGenerationService_IgnoresRemoteHTTPImageURL(t *testing.T) {
	images, err := extractImageGenerationOutputs([]byte(`{"status":"completed","images":[{"url":"https://files.example.com/image.png"}]}`))
	require.NoError(t, err)
	require.Empty(t, images)
}

func TestImageGenerationService_BootstrapViewListsOnlyImageCapableUserKeys(t *testing.T) {
	repo := newMemoryImageGenerationRepo()
	groupID := int64(10)
	repo.apiKeys[1] = APIKey{
		ID:      1,
		UserID:  11,
		Key:     "sk-valid-image-key",
		Name:    "image key",
		GroupID: &groupID,
		Status:  StatusActive,
		Purpose: APIKeyPurposeUser,
		Group:   &Group{ID: groupID, Name: "OpenAI Image", Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true},
	}
	repo.apiKeys[2] = APIKey{
		ID:      2,
		UserID:  11,
		Key:     "sk-no-image-key",
		Name:    "no image",
		GroupID: &groupID,
		Status:  StatusActive,
		Purpose: APIKeyPurposeUser,
		Group:   &Group{ID: groupID, Name: "OpenAI Text", Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: false},
	}
	repo.apiKeys[3] = APIKey{
		ID:      3,
		UserID:  11,
		Key:     "sk-hidden-key",
		Name:    "hidden",
		GroupID: &groupID,
		Status:  StatusActive,
		Purpose: APIKeyPurposeImageGeneration,
		Group:   &Group{ID: groupID, Name: "OpenAI Image", Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true},
	}
	svc := NewImageGenerationService(repo, nil, nil, nil, nil, ImageGenerationStorageConfig{})

	view := imageGenerationBootstrapFromSettings(&ImageGenerationSettings{
		Enabled:        true,
		DefaultGroupID: groupID,
		DefaultModel:   "gpt-image-2",
		RetentionDays:  30,
	}, nil)
	keys, err := svc.selectableAPIKeys(context.Background(), 11)
	require.NoError(t, err)
	view.AvailableAPIKeys = keys

	require.Len(t, view.AvailableAPIKeys, 1)
	require.Equal(t, int64(1), view.AvailableAPIKeys[0].ID)
	require.Equal(t, ImageGenerationKeySelectionSystem, view.KeySelection)
}

func TestImageGenerationService_SavePreferenceRejectsInvalidAndClearsStaleSelection(t *testing.T) {
	repo := newMemoryImageGenerationRepo()
	groupID := int64(10)
	repo.apiKeys[1] = APIKey{
		ID:      1,
		UserID:  11,
		Key:     "sk-valid-image-key",
		Name:    "image key",
		GroupID: &groupID,
		Status:  StatusActive,
		Purpose: APIKeyPurposeUser,
		Group:   &Group{ID: groupID, Name: "OpenAI Image", Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: true},
	}
	repo.apiKeys[2] = APIKey{
		ID:      2,
		UserID:  11,
		Key:     "sk-no-image-key",
		Name:    "no image",
		GroupID: &groupID,
		Status:  StatusActive,
		Purpose: APIKeyPurposeUser,
		Group:   &Group{ID: groupID, Name: "OpenAI Text", Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: false},
	}
	svc := NewImageGenerationService(repo, nil, nil, nil, nil, ImageGenerationStorageConfig{})

	invalidID := int64(2)
	_, err := svc.SavePreference(context.Background(), 11, &invalidID)
	require.ErrorIs(t, err, ErrImageGenerationAPIKeyInvalid)

	validID := int64(1)
	repo.preferences[11] = validID
	repo.apiKeys[1] = APIKey{
		ID:      1,
		UserID:  11,
		Key:     "sk-valid-image-key",
		Name:    "image key",
		GroupID: &groupID,
		Status:  StatusActive,
		Purpose: APIKeyPurposeUser,
		Group:   &Group{ID: groupID, Name: "OpenAI Image", Platform: PlatformOpenAI, Status: StatusActive, AllowImageGeneration: false},
	}
	preferred, err := repo.GetPreferredAPIKeyID(context.Background(), 11)
	require.NoError(t, err)
	require.NotNil(t, preferred)

	_, _ = repo.GetSelectableAPIKey(context.Background(), 11, validID)
	_ = repo.SetPreferredAPIKeyID(context.Background(), 11, nil)
	preferred, err = repo.GetPreferredAPIKeyID(context.Background(), 11)
	require.NoError(t, err)
	require.Nil(t, preferred)
}

func imageGenerationPtrTime(t time.Time) *time.Time {
	return &t
}
