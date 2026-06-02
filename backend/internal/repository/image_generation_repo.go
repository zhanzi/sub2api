package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type imageGenerationRepository struct {
	sql sqlExecutor
}

func NewImageGenerationRepository(sqlDB *sql.DB) service.ImageGenerationRepository {
	return &imageGenerationRepository{sql: sqlDB}
}

func (r *imageGenerationRepository) CreateCompletedTask(ctx context.Context, task *service.ImageGenerationTask) error {
	if err := r.CreatePendingTask(ctx, task); err != nil {
		return err
	}
	return r.CompleteTask(ctx, task)
}

func (r *imageGenerationRepository) CreatePendingTask(ctx context.Context, task *service.ImageGenerationTask) error {
	if task == nil {
		return fmt.Errorf("image generation task is nil")
	}
	if len(task.RequestJSON) == 0 {
		task.RequestJSON = []byte("{}")
	}
	if !json.Valid(task.RequestJSON) {
		task.RequestJSON = []byte("{}")
	}

	query := `
		INSERT INTO image_generation_tasks
			(user_id, mode, status, model, prompt, request_json, error_message, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10)
		RETURNING id
	`
	if err := scanSingleRow(ctx, r.sql, query, []any{
		task.UserID, task.Mode, task.Status, task.Model, task.Prompt, string(task.RequestJSON),
		task.ErrorMessage, task.ExpiresAt, task.CreatedAt, task.UpdatedAt,
	}, &task.ID); err != nil {
		return err
	}
	return nil
}

func (r *imageGenerationRepository) CompleteTask(ctx context.Context, task *service.ImageGenerationTask) error {
	if task == nil || task.ID <= 0 {
		return fmt.Errorf("image generation task id is required")
	}
	if _, err := r.sql.ExecContext(ctx, `
		UPDATE image_generation_tasks
		SET status = $1, error_message = NULL, updated_at = $2
		WHERE id = $3 AND deleted_at IS NULL
	`, service.ImageGenerationStatusSucceeded, task.UpdatedAt, task.ID); err != nil {
		return err
	}

	for i := range task.Results {
		res := &task.Results[i]
		res.TaskID = task.ID
		if res.UserID == 0 {
			res.UserID = task.UserID
		}
		if res.CreatedAt.IsZero() {
			res.CreatedAt = task.CreatedAt
		}
		if err := scanSingleRow(ctx, r.sql, `
			INSERT INTO image_generation_results
				(task_id, user_id, result_index, mime_type, storage_path, size_bytes, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id
		`, []any{res.TaskID, res.UserID, res.Index, res.MimeType, res.StoragePath, res.SizeBytes, res.CreatedAt}, &res.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *imageGenerationRepository) MarkTaskFailed(ctx context.Context, taskID int64, errorMessage string, updatedAt time.Time) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE image_generation_tasks
		SET status = $1, error_message = $2, updated_at = $3
		WHERE id = $4 AND deleted_at IS NULL
	`, service.ImageGenerationStatusFailed, errorMessage, updatedAt, taskID)
	return err
}

func (r *imageGenerationRepository) ListTasksByUser(ctx context.Context, userID int64, params service.ImageGenerationListParams) ([]service.ImageGenerationTask, int64, error) {
	var total int64
	if err := scanSingleRow(ctx, r.sql, `
		SELECT COUNT(*)
		FROM image_generation_tasks
		WHERE user_id = $1 AND deleted_at IS NULL AND status <> $2
	`, []any{userID, service.ImageGenerationStatusExpired}, &total); err != nil {
		return nil, 0, err
	}

	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, user_id, mode, status, model, prompt, request_json, error_message, expires_at, created_at, updated_at, deleted_at
		FROM image_generation_tasks
		WHERE user_id = $1 AND deleted_at IS NULL AND status <> $2
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4
	`, userID, service.ImageGenerationStatusExpired, params.PageSize, (params.Page-1)*params.PageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	tasks, err := scanImageGenerationTasks(rows)
	if err != nil {
		return nil, 0, err
	}
	if err := r.attachResults(ctx, tasks); err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func (r *imageGenerationRepository) GetTaskByUser(ctx context.Context, userID, taskID int64) (*service.ImageGenerationTask, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, user_id, mode, status, model, prompt, request_json, error_message, expires_at, created_at, updated_at, deleted_at
		FROM image_generation_tasks
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, taskID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks, err := scanImageGenerationTasks(rows)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, service.ErrImageGenerationTaskNotFound
	}
	if err := r.attachResults(ctx, tasks); err != nil {
		return nil, err
	}
	return &tasks[0], nil
}

func (r *imageGenerationRepository) GetTaskByID(ctx context.Context, taskID int64) (*service.ImageGenerationTask, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, user_id, mode, status, model, prompt, request_json, error_message, expires_at, created_at, updated_at, deleted_at
		FROM image_generation_tasks
		WHERE id = $1 AND deleted_at IS NULL
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks, err := scanImageGenerationTasks(rows)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, service.ErrImageGenerationTaskNotFound
	}
	if err := r.attachResults(ctx, tasks); err != nil {
		return nil, err
	}
	return &tasks[0], nil
}

func (r *imageGenerationRepository) SoftDeleteTask(ctx context.Context, userID, taskID int64, deletedAt time.Time) (*service.ImageGenerationTask, error) {
	task, err := r.GetTaskByUser(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}
	res, err := r.sql.ExecContext(ctx, `
		UPDATE image_generation_tasks
		SET status = $1, deleted_at = $2, updated_at = $2
		WHERE id = $3 AND user_id = $4 AND deleted_at IS NULL
	`, service.ImageGenerationStatusDeleted, deletedAt, taskID, userID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, service.ErrImageGenerationTaskNotFound
	}
	_, _ = r.sql.ExecContext(ctx, `
		UPDATE image_generation_results SET deleted_at = $1 WHERE task_id = $2 AND deleted_at IS NULL
	`, deletedAt, taskID)
	task.Status = service.ImageGenerationStatusDeleted
	task.DeletedAt = &deletedAt
	task.UpdatedAt = deletedAt
	return task, nil
}

func (r *imageGenerationRepository) ListExpiredTasks(ctx context.Context, now time.Time, limit int) ([]service.ImageGenerationTask, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT id, user_id, mode, status, model, prompt, request_json, error_message, expires_at, created_at, updated_at, deleted_at
		FROM image_generation_tasks
		WHERE deleted_at IS NULL AND status <> $1 AND expires_at <= $2
		ORDER BY expires_at ASC, id ASC
		LIMIT $3
	`, service.ImageGenerationStatusExpired, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks, err := scanImageGenerationTasks(rows)
	if err != nil {
		return nil, err
	}
	if err := r.attachResults(ctx, tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *imageGenerationRepository) MarkTaskExpired(ctx context.Context, taskID int64, deletedAt time.Time, errorMessage *string) error {
	_, err := r.sql.ExecContext(ctx, `
		UPDATE image_generation_tasks
		SET status = $1, deleted_at = $2, updated_at = $2, error_message = $3
		WHERE id = $4
	`, service.ImageGenerationStatusExpired, deletedAt, errorMessage, taskID)
	if err == nil {
		_, _ = r.sql.ExecContext(ctx, `
			UPDATE image_generation_results SET deleted_at = $1 WHERE task_id = $2 AND deleted_at IS NULL
		`, deletedAt, taskID)
	}
	return err
}

func (r *imageGenerationRepository) GetOrCreatePurposeAPIKey(ctx context.Context, input service.ImageGenerationPurposeKeyInput) (*service.APIKey, error) {
	if input.UserID <= 0 || input.GroupID <= 0 || input.Key == "" {
		return nil, fmt.Errorf("invalid image generation api key input")
	}
	row := &service.APIKey{}
	var groupID int64
	err := scanSingleRow(ctx, r.sql, `
		SELECT id, user_id, key, name, group_id, status, purpose, created_at, updated_at
		FROM api_keys
		WHERE user_id = $1 AND purpose = $2 AND deleted_at IS NULL
		ORDER BY id ASC
		LIMIT 1
	`, []any{input.UserID, service.APIKeyPurposeImageGeneration},
		&row.ID, &row.UserID, &row.Key, &row.Name, &groupID, &row.Status, &row.Purpose, &row.CreatedAt, &row.UpdatedAt)
	if err == nil {
		if groupID != input.GroupID || row.Status != service.StatusActive {
			if _, updateErr := r.sql.ExecContext(ctx, `
				UPDATE api_keys
				SET group_id = $1, status = $2, updated_at = NOW()
				WHERE id = $3
			`, input.GroupID, service.StatusActive, row.ID); updateErr != nil {
				return nil, updateErr
			}
			groupID = input.GroupID
			row.Status = service.StatusActive
			row.UpdatedAt = time.Now()
		}
		row.GroupID = &groupID
		return row, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	err = scanSingleRow(ctx, r.sql, `
		INSERT INTO api_keys (user_id, key, name, group_id, status, purpose, quota, quota_used, rate_limit_5h, rate_limit_1d, rate_limit_7d, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, 0, 0, 0, 0, 0, NOW(), NOW())
		RETURNING id, user_id, key, name, group_id, status, purpose, created_at, updated_at
	`, []any{input.UserID, input.Key, input.Name, input.GroupID, service.StatusActive, service.APIKeyPurposeImageGeneration},
		&row.ID, &row.UserID, &row.Key, &row.Name, &groupID, &row.Status, &row.Purpose, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		return nil, err
	}
	row.GroupID = &groupID
	return row, nil
}

func (r *imageGenerationRepository) GetResultByUser(ctx context.Context, userID, resultID int64) (*service.ImageGenerationResult, error) {
	res := &service.ImageGenerationResult{}
	err := scanSingleRow(ctx, r.sql, `
		SELECT id, task_id, user_id, result_index, mime_type, storage_path, size_bytes, created_at, deleted_at
		FROM image_generation_results
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, []any{resultID, userID}, &res.ID, &res.TaskID, &res.UserID, &res.Index, &res.MimeType, &res.StoragePath, &res.SizeBytes, &res.CreatedAt, &res.DeletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrImageGenerationTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (r *imageGenerationRepository) ListSelectableAPIKeys(ctx context.Context, userID int64) ([]service.APIKey, error) {
	rows, err := r.sql.QueryContext(ctx, `
		SELECT
			k.id, k.user_id, k.key, k.name, k.group_id, k.status, COALESCE(k.purpose, $2), k.quota, k.quota_used, k.expires_at,
			k.created_at, k.updated_at,
			g.id, g.name, g.platform, g.status, g.allow_image_generation
		FROM api_keys k
		JOIN groups g ON g.id = k.group_id
		WHERE k.user_id = $1
			AND k.deleted_at IS NULL
			AND (k.purpose IS NULL OR k.purpose = $2)
			AND k.status = $3
			AND (k.expires_at IS NULL OR k.expires_at > NOW())
			AND g.platform = $4
			AND g.status = $3
			AND g.allow_image_generation = TRUE
		ORDER BY k.name ASC, k.id ASC
	`, userID, service.APIKeyPurposeUser, service.StatusActive, service.PlatformOpenAI)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSelectableImageAPIKeys(rows)
}

func (r *imageGenerationRepository) GetSelectableAPIKey(ctx context.Context, userID, apiKeyID int64) (*service.APIKey, error) {
	var key service.APIKey
	var groupID int64
	var group service.Group
	err := scanSingleRow(ctx, r.sql, `
		SELECT
			k.id, k.user_id, k.key, k.name, k.group_id, k.status, COALESCE(k.purpose, $3), k.quota, k.quota_used, k.expires_at,
			k.created_at, k.updated_at,
			g.id, g.name, g.platform, g.status, g.allow_image_generation
		FROM api_keys k
		JOIN groups g ON g.id = k.group_id
		WHERE k.user_id = $1
			AND k.id = $2
			AND k.deleted_at IS NULL
			AND (k.purpose IS NULL OR k.purpose = $3)
			AND k.status = $4
			AND (k.expires_at IS NULL OR k.expires_at > NOW())
			AND g.platform = $5
			AND g.status = $4
			AND g.allow_image_generation = TRUE
	`, []any{userID, apiKeyID, service.APIKeyPurposeUser, service.StatusActive, service.PlatformOpenAI},
		&key.ID, &key.UserID, &key.Key, &key.Name, &groupID, &key.Status, &key.Purpose,
		&key.Quota, &key.QuotaUsed, &key.ExpiresAt, &key.CreatedAt, &key.UpdatedAt,
		&group.ID, &group.Name, &group.Platform, &group.Status, &group.AllowImageGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrImageGenerationAPIKeyInvalid
	}
	if err != nil {
		return nil, err
	}
	key.GroupID = &groupID
	group.Hydrated = true
	key.Group = &group
	return &key, nil
}

func (r *imageGenerationRepository) GetPreferredAPIKeyID(ctx context.Context, userID int64) (*int64, error) {
	var id sql.NullInt64
	err := scanSingleRow(ctx, r.sql, `
		SELECT selected_api_key_id
		FROM image_generation_preferences
		WHERE user_id = $1
	`, []any{userID}, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !id.Valid || id.Int64 <= 0 {
		return nil, nil
	}
	return &id.Int64, nil
}

func (r *imageGenerationRepository) SetPreferredAPIKeyID(ctx context.Context, userID int64, apiKeyID *int64) error {
	if apiKeyID == nil || *apiKeyID <= 0 {
		_, err := r.sql.ExecContext(ctx, `DELETE FROM image_generation_preferences WHERE user_id = $1`, userID)
		return err
	}
	_, err := r.sql.ExecContext(ctx, `
		INSERT INTO image_generation_preferences (user_id, selected_api_key_id, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE
		SET selected_api_key_id = EXCLUDED.selected_api_key_id,
			updated_at = NOW()
	`, userID, *apiKeyID)
	return err
}

func scanImageGenerationTasks(rows *sql.Rows) ([]service.ImageGenerationTask, error) {
	var tasks []service.ImageGenerationTask
	for rows.Next() {
		var task service.ImageGenerationTask
		if err := rows.Scan(&task.ID, &task.UserID, &task.Mode, &task.Status, &task.Model, &task.Prompt, &task.RequestJSON, &task.ErrorMessage, &task.ExpiresAt, &task.CreatedAt, &task.UpdatedAt, &task.DeletedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

type sqlScanner interface {
	Scan(dest ...any) error
}

func scanSelectableImageAPIKeys(rows *sql.Rows) ([]service.APIKey, error) {
	var keys []service.APIKey
	for rows.Next() {
		key, err := scanSelectableImageAPIKeyRow(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *key)
	}
	return keys, rows.Err()
}

func scanSelectableImageAPIKeyRow(row sqlScanner) (*service.APIKey, error) {
	var key service.APIKey
	var groupID int64
	var group service.Group
	if err := row.Scan(
		&key.ID, &key.UserID, &key.Key, &key.Name, &groupID, &key.Status, &key.Purpose,
		&key.Quota, &key.QuotaUsed, &key.ExpiresAt, &key.CreatedAt, &key.UpdatedAt,
		&group.ID, &group.Name, &group.Platform, &group.Status, &group.AllowImageGeneration,
	); err != nil {
		return nil, err
	}
	key.GroupID = &groupID
	group.Hydrated = true
	key.Group = &group
	return &key, nil
}

func (r *imageGenerationRepository) attachResults(ctx context.Context, tasks []service.ImageGenerationTask) error {
	for i := range tasks {
		rows, err := r.sql.QueryContext(ctx, `
			SELECT id, task_id, user_id, result_index, mime_type, storage_path, size_bytes, created_at, deleted_at
			FROM image_generation_results
			WHERE task_id = $1 AND deleted_at IS NULL
			ORDER BY result_index ASC, id ASC
		`, tasks[i].ID)
		if err != nil {
			return err
		}
		var results []service.ImageGenerationResult
		for rows.Next() {
			var res service.ImageGenerationResult
			if err := rows.Scan(&res.ID, &res.TaskID, &res.UserID, &res.Index, &res.MimeType, &res.StoragePath, &res.SizeBytes, &res.CreatedAt, &res.DeletedAt); err != nil {
				_ = rows.Close()
				return err
			}
			results = append(results, res)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		tasks[i].Results = results
	}
	return nil
}
