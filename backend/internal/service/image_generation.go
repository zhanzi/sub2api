package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrImageGenerationDisabled      = infraImageGenerationError("IMAGE_GENERATION_DISABLED", "image generation page is disabled")
	ErrImageGenerationConfigInvalid = infraImageGenerationError("IMAGE_GENERATION_CONFIG_INVALID", "image generation page is not configured correctly")
	ErrImageGenerationTaskNotFound  = infraImageGenerationError("IMAGE_GENERATION_TASK_NOT_FOUND", "image generation task not found")
	ErrImageGenerationNoOutput      = infraImageGenerationError("IMAGE_GENERATION_NO_OUTPUT", "image generation response did not contain images")
	ErrImageGenerationAPIKeyInvalid = infraImageGenerationError("IMAGE_GENERATION_API_KEY_INVALID", "image generation api key is not available")
)

const (
	ImageGenerationModeGeneration = "generation"
	ImageGenerationModeEdit       = "edit"

	ImageGenerationStatusPending   = "pending"
	ImageGenerationStatusSucceeded = "succeeded"
	ImageGenerationStatusFailed    = "failed"
	ImageGenerationStatusDeleted   = "deleted"
	ImageGenerationStatusExpired   = "expired"

	ImageGenerationKeySelectionSystem  = "system"
	ImageGenerationKeySelectionUserKey = "user_key"
)

type ImageGenerationTask struct {
	ID           int64
	UserID       int64
	Mode         string
	Status       string
	Model        string
	Prompt       string
	RequestJSON  []byte
	ErrorMessage *string
	ExpiresAt    time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
	Results      []ImageGenerationResult
}

type ImageGenerationResult struct {
	ID          int64
	TaskID      int64
	UserID      int64
	Index       int
	MimeType    string
	StoragePath string
	SizeBytes   int64
	CreatedAt   time.Time
	DeletedAt   *time.Time
}

type ImageGenerationOutputImage struct {
	MimeType string
	Base64   string
}

type ImageGenerationCreateCompletedInput struct {
	UserID      int64
	Mode        string
	Model       string
	Prompt      string
	RequestJSON []byte
	ExpiresAt   *time.Time
	Images      []ImageGenerationOutputImage
}

type ImageGenerationCreatePendingInput struct {
	UserID      int64
	Mode        string
	Model       string
	Prompt      string
	RequestJSON []byte
	ExpiresAt   *time.Time
}

type ImageGenerationListParams struct {
	Page     int
	PageSize int
}

type ImageGenerationCleanupResult struct {
	TasksCleaned int
	FilesDeleted int
	Errors       []string
}

type ImageGenerationStorageConfig struct {
	BaseDir       string
	RetentionDays int
	Now           func() time.Time
}

type ImageGenerationSettings struct {
	Enabled        bool   `json:"enabled"`
	DefaultGroupID int64  `json:"default_group_id"`
	DefaultModel   string `json:"default_model"`
	RetentionDays  int    `json:"retention_days"`
}

type ImageGenerationBootstrap struct {
	Enabled          bool                              `json:"enabled"`
	DefaultGroupID   int64                             `json:"default_group_id"`
	DefaultModel     string                            `json:"default_model"`
	RetentionDays    int                               `json:"retention_days"`
	KeySelection     string                            `json:"key_selection"`
	SelectedAPIKeyID *int64                            `json:"selected_api_key_id,omitempty"`
	AvailableAPIKeys []ImageGenerationSelectableAPIKey `json:"available_api_keys"`
}

type ImageGenerationSelectableAPIKey struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	MaskedKey string `json:"masked_key"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
}

type ImageGenerationPurposeKeyInput struct {
	UserID  int64
	GroupID int64
	Key     string
	Name    string
}

type ImageGenerationRepository interface {
	CreateCompletedTask(ctx context.Context, task *ImageGenerationTask) error
	CreatePendingTask(ctx context.Context, task *ImageGenerationTask) error
	CompleteTask(ctx context.Context, task *ImageGenerationTask) error
	MarkTaskFailed(ctx context.Context, taskID int64, errorMessage string, updatedAt time.Time) error
	ListTasksByUser(ctx context.Context, userID int64, params ImageGenerationListParams) ([]ImageGenerationTask, int64, error)
	GetTaskByID(ctx context.Context, taskID int64) (*ImageGenerationTask, error)
	GetTaskByUser(ctx context.Context, userID, taskID int64) (*ImageGenerationTask, error)
	SoftDeleteTask(ctx context.Context, userID, taskID int64, deletedAt time.Time) (*ImageGenerationTask, error)
	ListExpiredTasks(ctx context.Context, now time.Time, limit int) ([]ImageGenerationTask, error)
	MarkTaskExpired(ctx context.Context, taskID int64, deletedAt time.Time, errorMessage *string) error
	GetOrCreatePurposeAPIKey(ctx context.Context, input ImageGenerationPurposeKeyInput) (*APIKey, error)
	GetResultByUser(ctx context.Context, userID, resultID int64) (*ImageGenerationResult, error)
	ListSelectableAPIKeys(ctx context.Context, userID int64) ([]APIKey, error)
	GetSelectableAPIKey(ctx context.Context, userID, apiKeyID int64) (*APIKey, error)
	GetPreferredAPIKeyID(ctx context.Context, userID int64) (*int64, error)
	SetPreferredAPIKeyID(ctx context.Context, userID int64, apiKeyID *int64) error
}

type ImageGenerationService struct {
	repo           ImageGenerationRepository
	settingService *SettingService
	apiKeyService  *APIKeyService
	groupRepo      GroupRepository
	cfg            ImageGenerationStorageConfig
}

func NewImageGenerationService(
	repo ImageGenerationRepository,
	settingService *SettingService,
	apiKeyService *APIKeyService,
	_ *OpenAIGatewayService,
	groupRepo GroupRepository,
	cfg ImageGenerationStorageConfig,
) *ImageGenerationService {
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = 30
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if strings.TrimSpace(cfg.BaseDir) == "" {
		cfg.BaseDir = filepath.Join(".", "data", "image-generation")
	}
	return &ImageGenerationService{repo: repo, settingService: settingService, apiKeyService: apiKeyService, groupRepo: groupRepo, cfg: cfg}
}

func (s *ImageGenerationService) Bootstrap(ctx context.Context, userID int64) (*ImageGenerationSettings, *APIKey, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !settings.Enabled {
		return settings, nil, ErrImageGenerationDisabled
	}
	if settings.DefaultGroupID <= 0 || s.groupRepo == nil {
		return settings, nil, ErrImageGenerationConfigInvalid
	}
	group, err := s.groupRepo.GetByID(ctx, settings.DefaultGroupID)
	if err != nil || group == nil || group.Platform != PlatformOpenAI || !GroupAllowsImageGeneration(group) {
		return settings, nil, ErrImageGenerationConfigInvalid
	}
	if settings.DefaultModel == "" {
		settings.DefaultModel = "gpt-image-2"
	}
	key, err := s.ensurePurposeKey(ctx, userID, settings.DefaultGroupID)
	if err != nil {
		return settings, nil, err
	}
	if s.apiKeyService != nil {
		full, err := s.apiKeyService.GetByKey(ctx, key.Key)
		if err == nil && full != nil {
			key = full
		}
	}
	return settings, key, nil
}

func (s *ImageGenerationService) BootstrapView(ctx context.Context, userID int64) (*ImageGenerationBootstrap, error) {
	settings, _, err := s.Bootstrap(ctx, userID)
	if err != nil {
		return nil, err
	}
	keys, err := s.selectableAPIKeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := imageGenerationBootstrapFromSettings(settings, keys)
	preferredID, err := s.repo.GetPreferredAPIKeyID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if preferredID == nil {
		return out, nil
	}
	key, err := s.repo.GetSelectableAPIKey(ctx, userID, *preferredID)
	if err != nil || !isSelectableImageGenerationAPIKey(key) {
		_ = s.repo.SetPreferredAPIKeyID(ctx, userID, nil)
		return out, nil
	}
	out.KeySelection = ImageGenerationKeySelectionUserKey
	out.SelectedAPIKeyID = &key.ID
	return out, nil
}

func (s *ImageGenerationService) SavePreference(ctx context.Context, userID int64, apiKeyID *int64) (*ImageGenerationBootstrap, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("image generation service not ready")
	}
	if apiKeyID != nil {
		key, err := s.repo.GetSelectableAPIKey(ctx, userID, *apiKeyID)
		if err != nil || !isSelectableImageGenerationAPIKey(key) {
			return nil, ErrImageGenerationAPIKeyInvalid
		}
	}
	if err := s.repo.SetPreferredAPIKeyID(ctx, userID, apiKeyID); err != nil {
		return nil, err
	}
	return s.BootstrapView(ctx, userID)
}

func (s *ImageGenerationService) ResolveAPIKeyForRequest(ctx context.Context, userID int64, apiKeyID *int64) (*APIKey, error) {
	if apiKeyID == nil || *apiKeyID <= 0 {
		_, key, err := s.Bootstrap(ctx, userID)
		return key, err
	}
	key, err := s.repo.GetSelectableAPIKey(ctx, userID, *apiKeyID)
	if err != nil || !isSelectableImageGenerationAPIKey(key) {
		return nil, ErrImageGenerationAPIKeyInvalid
	}
	return key, nil
}

func (s *ImageGenerationService) GetSettings(ctx context.Context) (*ImageGenerationSettings, error) {
	if s == nil || s.settingService == nil || s.settingService.settingRepo == nil {
		return &ImageGenerationSettings{DefaultModel: "gpt-image-2", RetentionDays: s.retentionDays()}, nil
	}
	vals, err := s.settingService.settingRepo.GetMultiple(ctx, []string{
		SettingKeyImageGenerationEnabled,
		SettingKeyImageGenerationDefaultGroupID,
		SettingKeyImageGenerationDefaultModel,
		SettingKeyImageGenerationRetentionDays,
	})
	if err != nil {
		return nil, err
	}
	groupID, _ := parseInt64Setting(vals[SettingKeyImageGenerationDefaultGroupID])
	retention := parsePositiveIntSetting(vals[SettingKeyImageGenerationRetentionDays], 30)
	if retention <= 0 {
		retention = 30
	}
	model := strings.TrimSpace(vals[SettingKeyImageGenerationDefaultModel])
	if model == "" {
		model = "gpt-image-2"
	}
	return &ImageGenerationSettings{
		Enabled:        vals[SettingKeyImageGenerationEnabled] == "true",
		DefaultGroupID: groupID,
		DefaultModel:   model,
		RetentionDays:  retention,
	}, nil
}

func (s *ImageGenerationService) CreateCompletedTaskFromResponse(ctx context.Context, userID int64, mode string, requestJSON []byte, responseBody []byte) (*ImageGenerationTask, error) {
	images, err := extractImageGenerationOutputs(responseBody)
	if err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return nil, ErrImageGenerationNoOutput
	}
	var req struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	_ = json.Unmarshal(requestJSON, &req)
	settings, _ := s.GetSettings(ctx)
	retention := s.retentionDays()
	if settings != nil && settings.RetentionDays > 0 {
		retention = settings.RetentionDays
	}
	expiresAt := s.now().AddDate(0, 0, retention)
	return s.CreateCompletedTask(ctx, ImageGenerationCreateCompletedInput{
		UserID:      userID,
		Mode:        mode,
		Model:       firstNonEmpty(req.Model, settingsDefaultModel(settings)),
		Prompt:      req.Prompt,
		RequestJSON: requestJSON,
		ExpiresAt:   &expiresAt,
		Images:      images,
	})
}

func (s *ImageGenerationService) CreatePendingTaskFromRequest(ctx context.Context, userID int64, mode string, requestJSON []byte) (*ImageGenerationTask, error) {
	var req struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	_ = json.Unmarshal(requestJSON, &req)
	settings, _ := s.GetSettings(ctx)
	retention := s.retentionDays()
	if settings != nil && settings.RetentionDays > 0 {
		retention = settings.RetentionDays
	}
	expiresAt := s.now().AddDate(0, 0, retention)
	return s.CreatePendingTask(ctx, ImageGenerationCreatePendingInput{
		UserID:      userID,
		Mode:        mode,
		Model:       firstNonEmpty(req.Model, settingsDefaultModel(settings)),
		Prompt:      req.Prompt,
		RequestJSON: requestJSON,
		ExpiresAt:   &expiresAt,
	})
}

func (s *ImageGenerationService) CompletePendingTaskFromResponse(ctx context.Context, taskID int64, responseBody []byte) (*ImageGenerationTask, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("image generation service not ready")
	}
	task, err := s.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	images, err := extractImageGenerationOutputs(responseBody)
	if err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return nil, ErrImageGenerationNoOutput
	}
	results := make([]ImageGenerationResult, 0, len(images))
	now := s.now()
	for i, img := range images {
		result, err := s.writeOutputImage(task.UserID, now, i, img)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	task.Status = ImageGenerationStatusSucceeded
	task.ErrorMessage = nil
	task.UpdatedAt = now
	task.Results = results
	if err := s.repo.CompleteTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *ImageGenerationService) GetResultFile(ctx context.Context, userID, resultID int64) (*ImageGenerationResult, string, error) {
	if s == nil || s.repo == nil {
		return nil, "", fmt.Errorf("image generation service not ready")
	}
	result, err := s.repo.GetResultByUser(ctx, userID, resultID)
	if err != nil {
		return nil, "", err
	}
	abs, err := s.safeStoragePath(result.StoragePath)
	if err != nil {
		return nil, "", err
	}
	return result, abs, nil
}

func (s *ImageGenerationService) CreatePendingTask(ctx context.Context, input ImageGenerationCreatePendingInput) (*ImageGenerationTask, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("image generation service not ready")
	}
	if input.UserID <= 0 {
		return nil, fmt.Errorf("user_id is required")
	}
	now := s.now()
	expiresAt := now.AddDate(0, 0, s.cfg.RetentionDays)
	if input.ExpiresAt != nil {
		expiresAt = *input.ExpiresAt
	}
	task := &ImageGenerationTask{
		UserID:      input.UserID,
		Mode:        normalizeImageGenerationMode(input.Mode),
		Status:      ImageGenerationStatusPending,
		Model:       strings.TrimSpace(input.Model),
		Prompt:      input.Prompt,
		RequestJSON: input.RequestJSON,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.CreatePendingTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *ImageGenerationService) CreateCompletedTask(ctx context.Context, input ImageGenerationCreateCompletedInput) (*ImageGenerationTask, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("image generation service not ready")
	}
	if input.UserID <= 0 {
		return nil, fmt.Errorf("user_id is required")
	}
	if len(input.Images) == 0 {
		return nil, fmt.Errorf("at least one image is required")
	}

	now := s.now()
	expiresAt := now.AddDate(0, 0, s.cfg.RetentionDays)
	if input.ExpiresAt != nil {
		expiresAt = *input.ExpiresAt
	}
	task := &ImageGenerationTask{
		UserID:      input.UserID,
		Mode:        normalizeImageGenerationMode(input.Mode),
		Status:      ImageGenerationStatusSucceeded,
		Model:       strings.TrimSpace(input.Model),
		Prompt:      input.Prompt,
		RequestJSON: input.RequestJSON,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	results := make([]ImageGenerationResult, 0, len(input.Images))
	for i, img := range input.Images {
		result, err := s.writeOutputImage(input.UserID, now, i, img)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	task.Results = results

	if err := s.repo.CreateCompletedTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *ImageGenerationService) MarkTaskFailed(ctx context.Context, taskID int64, err error) error {
	if s == nil || s.repo == nil || taskID <= 0 {
		return nil
	}
	msg := "图片生成失败"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		msg = err.Error()
	}
	return s.repo.MarkTaskFailed(ctx, taskID, msg, s.now())
}

func (s *ImageGenerationService) ListTasks(ctx context.Context, userID int64, params ImageGenerationListParams) ([]ImageGenerationTask, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, fmt.Errorf("image generation service not ready")
	}
	return s.repo.ListTasksByUser(ctx, userID, normalizeImageGenerationListParams(params))
}

func (s *ImageGenerationService) GetTask(ctx context.Context, userID, taskID int64) (*ImageGenerationTask, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("image generation service not ready")
	}
	return s.repo.GetTaskByUser(ctx, userID, taskID)
}

func (s *ImageGenerationService) DeleteTask(ctx context.Context, userID, taskID int64) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("image generation service not ready")
	}
	task, err := s.repo.SoftDeleteTask(ctx, userID, taskID, s.now())
	if err != nil {
		return err
	}
	result := s.deleteTaskFiles(task)
	if len(result.Errors) > 0 {
		return errors.New(strings.Join(result.Errors, "; "))
	}
	return nil
}

func (s *ImageGenerationService) CleanupExpired(ctx context.Context, limit int) (*ImageGenerationCleanupResult, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("image generation service not ready")
	}
	if limit <= 0 {
		limit = 100
	}
	now := s.now()
	tasks, err := s.repo.ListExpiredTasks(ctx, now, limit)
	if err != nil {
		return nil, err
	}
	out := &ImageGenerationCleanupResult{}
	for i := range tasks {
		task := tasks[i]
		fileResult := s.deleteTaskFiles(&task)
		out.FilesDeleted += fileResult.FilesDeleted
		out.Errors = append(out.Errors, fileResult.Errors...)
		var msg *string
		if len(fileResult.Errors) > 0 {
			joined := strings.Join(fileResult.Errors, "; ")
			msg = &joined
		}
		if err := s.repo.MarkTaskExpired(ctx, task.ID, now, msg); err != nil {
			out.Errors = append(out.Errors, err.Error())
			continue
		}
		out.TasksCleaned++
	}
	return out, nil
}

func (s *ImageGenerationService) writeOutputImage(userID int64, now time.Time, index int, img ImageGenerationOutputImage) (ImageGenerationResult, error) {
	mimeType := normalizeImageMimeType(img.MimeType)
	ext := imageExtensionForMimeType(mimeType)
	data, err := base64.StdEncoding.DecodeString(stripDataURLPrefix(img.Base64))
	if err != nil {
		return ImageGenerationResult{}, fmt.Errorf("decode generated image: %w", err)
	}
	name, err := imageGenerationRandomHex(12)
	if err != nil {
		return ImageGenerationResult{}, err
	}
	rel := filepath.ToSlash(filepath.Join(
		fmt.Sprintf("user-%d", userID),
		now.UTC().Format("2006/01/02"),
		fmt.Sprintf("%s-%d%s", name, index, ext),
	))
	abs := filepath.Join(s.cfg.BaseDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return ImageGenerationResult{}, fmt.Errorf("create image directory: %w", err)
	}
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		return ImageGenerationResult{}, fmt.Errorf("write generated image: %w", err)
	}
	return ImageGenerationResult{
		UserID:      userID,
		Index:       index,
		MimeType:    mimeType,
		StoragePath: rel,
		SizeBytes:   int64(len(data)),
		CreatedAt:   now,
	}, nil
}

func (s *ImageGenerationService) deleteTaskFiles(task *ImageGenerationTask) ImageGenerationCleanupResult {
	out := ImageGenerationCleanupResult{}
	if task == nil {
		return out
	}
	for _, result := range task.Results {
		if strings.TrimSpace(result.StoragePath) == "" {
			continue
		}
		abs, err := s.safeStoragePath(result.StoragePath)
		if err != nil {
			out.Errors = append(out.Errors, err.Error())
			continue
		}
		if err := os.Remove(abs); err != nil {
			if os.IsNotExist(err) {
				out.FilesDeleted++
				continue
			}
			out.Errors = append(out.Errors, err.Error())
			continue
		}
		out.FilesDeleted++
	}
	return out
}

func (s *ImageGenerationService) safeStoragePath(rel string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", fmt.Errorf("invalid image storage path")
	}
	base, err := filepath.Abs(s.cfg.BaseDir)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(filepath.Join(base, cleaned))
	if err != nil {
		return "", err
	}
	if abs != base && !strings.HasPrefix(abs, base+string(filepath.Separator)) {
		return "", fmt.Errorf("image storage path escapes base directory")
	}
	return abs, nil
}

func (s *ImageGenerationService) now() time.Time {
	return s.cfg.Now().UTC()
}

func normalizeImageGenerationMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ImageGenerationModeEdit:
		return ImageGenerationModeEdit
	default:
		return ImageGenerationModeGeneration
	}
}

func normalizeImageGenerationListParams(params ImageGenerationListParams) ImageGenerationListParams {
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	return params
}

func normalizeImageMimeType(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/jpg":
		return "image/jpeg"
	case "image/webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func imageExtensionForMimeType(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

func stripDataURLPrefix(value string) string {
	if idx := strings.Index(value, ","); strings.HasPrefix(value, "data:") && idx >= 0 {
		return value[idx+1:]
	}
	return value
}

func imageGenerationRandomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func infraImageGenerationError(reason, message string) error {
	switch reason {
	case "IMAGE_GENERATION_DISABLED":
		return infraerrors.Forbidden(reason, message)
	case "IMAGE_GENERATION_TASK_NOT_FOUND":
		return infraerrors.NotFound(reason, message)
	default:
		return infraerrors.BadRequest(reason, message)
	}
}

func (s *ImageGenerationService) selectableAPIKeys(ctx context.Context, userID int64) ([]ImageGenerationSelectableAPIKey, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("image generation service not ready")
	}
	keys, err := s.repo.ListSelectableAPIKeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ImageGenerationSelectableAPIKey, 0, len(keys))
	for i := range keys {
		key := &keys[i]
		if !isSelectableImageGenerationAPIKey(key) {
			continue
		}
		groupID := int64(0)
		groupName := ""
		if key.GroupID != nil {
			groupID = *key.GroupID
		}
		if key.Group != nil {
			groupName = key.Group.Name
		}
		out = append(out, ImageGenerationSelectableAPIKey{
			ID:        key.ID,
			Name:      key.Name,
			MaskedKey: maskAPIKeyForImageGeneration(key.Key),
			GroupID:   groupID,
			GroupName: groupName,
		})
	}
	return out, nil
}

func imageGenerationBootstrapFromSettings(settings *ImageGenerationSettings, keys []ImageGenerationSelectableAPIKey) *ImageGenerationBootstrap {
	if settings == nil {
		settings = &ImageGenerationSettings{DefaultModel: "gpt-image-2", RetentionDays: 30}
	}
	return &ImageGenerationBootstrap{
		Enabled:          settings.Enabled,
		DefaultGroupID:   settings.DefaultGroupID,
		DefaultModel:     settingsDefaultModel(settings),
		RetentionDays:    settings.RetentionDays,
		KeySelection:     ImageGenerationKeySelectionSystem,
		AvailableAPIKeys: keys,
	}
}

func isSelectableImageGenerationAPIKey(key *APIKey) bool {
	if key == nil || key.UserID <= 0 || key.ID <= 0 {
		return false
	}
	if strings.TrimSpace(key.Purpose) != "" && key.Purpose != APIKeyPurposeUser {
		return false
	}
	if key.Status != StatusActive && key.Status != StatusAPIKeyActive {
		return false
	}
	if key.IsExpired() || key.IsQuotaExhausted() {
		return false
	}
	if key.GroupID == nil || *key.GroupID <= 0 || key.Group == nil {
		return false
	}
	return key.Group.Status == StatusActive && key.Group.Platform == PlatformOpenAI && key.Group.AllowImageGeneration
}

func maskAPIKeyForImageGeneration(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if len(key) <= 10 {
		return key[:min(len(key), 4)] + "..."
	}
	return key[:6] + "..." + key[len(key)-4:]
}

func (s *ImageGenerationService) ensurePurposeKey(ctx context.Context, userID, groupID int64) (*APIKey, error) {
	if s == nil || s.repo == nil || s.apiKeyService == nil {
		return nil, fmt.Errorf("image generation service not ready")
	}
	key, err := s.apiKeyService.GenerateKey()
	if err != nil {
		return nil, err
	}
	return s.repo.GetOrCreatePurposeAPIKey(ctx, ImageGenerationPurposeKeyInput{
		UserID:  userID,
		GroupID: groupID,
		Key:     key,
		Name:    "系统图片生成页",
	})
}

func (s *ImageGenerationService) retentionDays() int {
	if s == nil || s.cfg.RetentionDays <= 0 {
		return 30
	}
	return s.cfg.RetentionDays
}

func settingsDefaultModel(settings *ImageGenerationSettings) string {
	if settings == nil || strings.TrimSpace(settings.DefaultModel) == "" {
		return "gpt-image-2"
	}
	return strings.TrimSpace(settings.DefaultModel)
}

func parseInt64Setting(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	var v int64
	_, err := fmt.Sscan(raw, &v)
	return v, err
}

func parsePositiveIntSetting(raw string, fallback int) int {
	var v int
	if _, err := fmt.Sscan(strings.TrimSpace(raw), &v); err != nil || v <= 0 {
		return fallback
	}
	return v
}

func extractImageGenerationOutputs(body []byte) ([]ImageGenerationOutputImage, error) {
	var payload struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	out := make([]ImageGenerationOutputImage, 0, len(payload.Data))
	for _, item := range payload.Data {
		if strings.TrimSpace(item.B64JSON) == "" {
			continue
		}
		out = append(out, ImageGenerationOutputImage{MimeType: "image/png", Base64: item.B64JSON})
	}
	return out, nil
}

type memoryImageGenerationRepo struct {
	mu          sync.Mutex
	nextID      int64
	nextRID     int64
	tasks       map[int64]ImageGenerationTask
	apiKeys     map[int64]APIKey
	preferences map[int64]int64
}

func newMemoryImageGenerationRepo() *memoryImageGenerationRepo {
	return &memoryImageGenerationRepo{
		nextID:      1,
		nextRID:     1,
		tasks:       map[int64]ImageGenerationTask{},
		apiKeys:     map[int64]APIKey{},
		preferences: map[int64]int64{},
	}
}

func (r *memoryImageGenerationRepo) CreateCompletedTask(_ context.Context, task *ImageGenerationTask) error {
	if err := r.CreatePendingTask(context.Background(), task); err != nil {
		return err
	}
	return r.CompleteTask(context.Background(), task)
}

func (r *memoryImageGenerationRepo) CreatePendingTask(_ context.Context, task *ImageGenerationTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := cloneImageGenerationTask(*task)
	cp.ID = r.nextID
	r.nextID++
	*task = cloneImageGenerationTask(cp)
	r.tasks[cp.ID] = cp
	return nil
}

func (r *memoryImageGenerationRepo) CompleteTask(_ context.Context, task *ImageGenerationTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := cloneImageGenerationTask(*task)
	if cp.ID <= 0 {
		return fmt.Errorf("image generation task not found")
	}
	cp.Status = ImageGenerationStatusSucceeded
	for i := range cp.Results {
		cp.Results[i].ID = r.nextRID
		cp.Results[i].TaskID = cp.ID
		r.nextRID++
	}
	*task = cloneImageGenerationTask(cp)
	r.tasks[cp.ID] = cp
	return nil
}

func (r *memoryImageGenerationRepo) MarkTaskFailed(_ context.Context, taskID int64, errorMessage string, updatedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok {
		return ErrImageGenerationTaskNotFound
	}
	task.Status = ImageGenerationStatusFailed
	task.ErrorMessage = &errorMessage
	task.UpdatedAt = updatedAt
	r.tasks[taskID] = task
	return nil
}

func (r *memoryImageGenerationRepo) ListTasksByUser(_ context.Context, userID int64, params ImageGenerationListParams) ([]ImageGenerationTask, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var items []ImageGenerationTask
	for _, task := range r.tasks {
		if task.UserID == userID && task.DeletedAt == nil && task.Status != ImageGenerationStatusExpired {
			items = append(items, cloneImageGenerationTask(task))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID > items[j].ID })
	total := int64(len(items))
	start := (params.Page - 1) * params.PageSize
	if start >= len(items) {
		return []ImageGenerationTask{}, total, nil
	}
	end := start + params.PageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], total, nil
}

func (r *memoryImageGenerationRepo) GetTaskByUser(_ context.Context, userID, taskID int64) (*ImageGenerationTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok || task.UserID != userID || task.DeletedAt != nil {
		return nil, fmt.Errorf("image generation task not found")
	}
	cp := cloneImageGenerationTask(task)
	return &cp, nil
}

func (r *memoryImageGenerationRepo) GetTaskByID(_ context.Context, taskID int64) (*ImageGenerationTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok || task.DeletedAt != nil {
		return nil, ErrImageGenerationTaskNotFound
	}
	cp := cloneImageGenerationTask(task)
	return &cp, nil
}

func (r *memoryImageGenerationRepo) SoftDeleteTask(_ context.Context, userID, taskID int64, deletedAt time.Time) (*ImageGenerationTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok || task.UserID != userID || task.DeletedAt != nil {
		return nil, fmt.Errorf("image generation task not found")
	}
	task.Status = ImageGenerationStatusDeleted
	task.DeletedAt = &deletedAt
	task.UpdatedAt = deletedAt
	r.tasks[taskID] = task
	cp := cloneImageGenerationTask(task)
	return &cp, nil
}

func (r *memoryImageGenerationRepo) ListExpiredTasks(_ context.Context, now time.Time, limit int) ([]ImageGenerationTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var items []ImageGenerationTask
	for _, task := range r.tasks {
		if task.DeletedAt == nil && task.Status != ImageGenerationStatusExpired && !task.ExpiresAt.After(now) {
			items = append(items, cloneImageGenerationTask(task))
			if len(items) >= limit {
				break
			}
		}
	}
	return items, nil
}

func (r *memoryImageGenerationRepo) MarkTaskExpired(_ context.Context, taskID int64, deletedAt time.Time, errorMessage *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok {
		return fmt.Errorf("image generation task not found")
	}
	task.Status = ImageGenerationStatusExpired
	task.DeletedAt = &deletedAt
	task.UpdatedAt = deletedAt
	task.ErrorMessage = errorMessage
	r.tasks[taskID] = task
	return nil
}

func (r *memoryImageGenerationRepo) GetOrCreatePurposeAPIKey(_ context.Context, input ImageGenerationPurposeKeyInput) (*APIKey, error) {
	groupID := input.GroupID
	return &APIKey{
		ID:      1,
		UserID:  input.UserID,
		Key:     input.Key,
		Name:    input.Name,
		GroupID: &groupID,
		Status:  StatusActive,
		Purpose: APIKeyPurposeImageGeneration,
	}, nil
}

func (r *memoryImageGenerationRepo) GetResultByUser(_ context.Context, userID, resultID int64) (*ImageGenerationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, task := range r.tasks {
		if task.UserID != userID || task.DeletedAt != nil {
			continue
		}
		for _, result := range task.Results {
			if result.ID == resultID && result.DeletedAt == nil {
				cp := result
				return &cp, nil
			}
		}
	}
	return nil, ErrImageGenerationTaskNotFound
}

func (r *memoryImageGenerationRepo) ListSelectableAPIKeys(_ context.Context, userID int64) ([]APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []APIKey
	for _, key := range r.apiKeys {
		if key.UserID == userID && isSelectableImageGenerationAPIKey(&key) {
			out = append(out, cloneAPIKeyForImageGeneration(key))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].ID < out[j].ID
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (r *memoryImageGenerationRepo) GetSelectableAPIKey(_ context.Context, userID, apiKeyID int64) (*APIKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key, ok := r.apiKeys[apiKeyID]
	if !ok || key.UserID != userID || !isSelectableImageGenerationAPIKey(&key) {
		return nil, ErrImageGenerationAPIKeyInvalid
	}
	cp := cloneAPIKeyForImageGeneration(key)
	return &cp, nil
}

func (r *memoryImageGenerationRepo) GetPreferredAPIKeyID(_ context.Context, userID int64) (*int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.preferences[userID]
	if !ok || id <= 0 {
		return nil, nil
	}
	return &id, nil
}

func (r *memoryImageGenerationRepo) SetPreferredAPIKeyID(_ context.Context, userID int64, apiKeyID *int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if apiKeyID == nil || *apiKeyID <= 0 {
		delete(r.preferences, userID)
		return nil
	}
	r.preferences[userID] = *apiKeyID
	return nil
}

func cloneImageGenerationTask(task ImageGenerationTask) ImageGenerationTask {
	if len(task.RequestJSON) > 0 {
		task.RequestJSON = append([]byte(nil), task.RequestJSON...)
	}
	if len(task.Results) > 0 {
		task.Results = append([]ImageGenerationResult(nil), task.Results...)
	}
	return task
}

func cloneAPIKeyForImageGeneration(key APIKey) APIKey {
	if key.Group != nil {
		group := *key.Group
		key.Group = &group
	}
	return key
}
