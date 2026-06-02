package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ImageGenerationHandler struct {
	service       *service.ImageGenerationService
	openAIGateway *OpenAIGatewayHandler
}

func NewImageGenerationHandler(svc *service.ImageGenerationService, openAIGateway *OpenAIGatewayHandler) *ImageGenerationHandler {
	return &ImageGenerationHandler{service: svc, openAIGateway: openAIGateway}
}

func (h *ImageGenerationHandler) Bootstrap(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}
	settings, err := h.service.BootstrapView(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *ImageGenerationHandler) SavePreference(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}
	var req struct {
		KeySelection string `json:"key_selection"`
		APIKeyID     *int64 `json:"api_key_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		response.BadRequest(c, "invalid request body")
		return
	}
	var selected *int64
	if req.KeySelection == service.ImageGenerationKeySelectionUserKey {
		selected = req.APIKeyID
		if selected == nil || *selected <= 0 {
			response.ErrorFrom(c, service.ErrImageGenerationAPIKeyInvalid)
			return
		}
	}
	settings, err := h.service.SavePreference(c.Request.Context(), subject.UserID, selected)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *ImageGenerationHandler) ListTasks(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}
	page, pageSize := response.ParsePagination(c)
	tasks, total, err := h.service.ListTasks(c.Request.Context(), subject.UserID, service.ImageGenerationListParams{Page: page, PageSize: pageSize})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, imageGenerationTasksToDTO(tasks), total, page, pageSize)
}

func (h *ImageGenerationHandler) GetTask(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		response.BadRequest(c, "invalid task id")
		return
	}
	task, err := h.service.GetTask(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, imageGenerationTaskToDTO(*task))
}

func (h *ImageGenerationHandler) Generate(c *gin.Context) {
	h.proxyImages(c, service.ImageGenerationModeGeneration, "/v1/images/generations")
}

func (h *ImageGenerationHandler) Edit(c *gin.Context) {
	h.proxyImages(c, service.ImageGenerationModeEdit, "/v1/images/edits")
}

func (h *ImageGenerationHandler) DeleteTask(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		response.BadRequest(c, "invalid task id")
		return
	}
	if err := h.service.DeleteTask(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

func (h *ImageGenerationHandler) File(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	result, abs, err := h.service.GetResultFile(c.Request.Context(), subject.UserID, id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.Header("Content-Type", result.MimeType)
	c.File(abs)
}

func (h *ImageGenerationHandler) proxyImages(c *gin.Context, mode, path string) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.BadRequest(c, "failed to read request body")
		return
	}
	contentType := c.GetHeader("Content-Type")
	apiKeyID, body, contentType, err := extractImageGenerationAPIKeyID(contentType, body)
	if err != nil {
		response.BadRequest(c, "invalid api key selection")
		return
	}
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	apiKey, err := h.service.ResolveAPIKeyForRequest(c.Request.Context(), subject.UserID, apiKeyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	recordBody := body
	if !strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data") {
		body = ensureImageGenerationB64Response(body)
		recordBody = body
	} else {
		sourceID := parseImageGenerationSourceResultID(contentType, body)
		if sourceID > 0 && mode == service.ImageGenerationModeEdit && !imageGenerationMultipartHasFile(contentType, body, "image") {
			result, abs, err := h.service.GetResultFile(c.Request.Context(), subject.UserID, sourceID)
			if err != nil {
				response.ErrorFrom(c, err)
				return
			}
			data, err := os.ReadFile(abs)
			if err != nil {
				response.BadRequest(c, "failed to read source image")
				return
			}
			source := imageGenerationSourceFile{
				FieldName: "image",
				FileName:  "history-" + strconv.FormatInt(result.ID, 10) + imageExtForMime(result.MimeType),
				MimeType:  result.MimeType,
				Data:      data,
			}
			contentType, body, err = injectImageGenerationSourceFile(contentType, body, source)
			if err != nil {
				response.BadRequest(c, "failed to attach source image")
				return
			}
			c.Request.Header.Set("Content-Type", contentType)
		}
		recordBody = summarizeImageGenerationMultipart(c.GetHeader("Content-Type"), body)
	}
	pendingTask, err := h.service.CreatePendingTaskFromRequest(c.Request.Context(), subject.UserID, mode, recordBody)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	headers := c.Request.Header.Clone()
	role := c.GetString(string(middleware2.ContextKeyUserRole))
	go h.runImageGenerationTask(pendingTask.ID, apiKey, subject, role, headers, body, path)
	response.Success(c, imageGenerationTaskToDTO(*pendingTask))
}

func (h *ImageGenerationHandler) runImageGenerationTask(taskID int64, apiKey *service.APIKey, subject middleware2.AuthSubject, role string, headers http.Header, body []byte, path string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	rec := httptest.NewRecorder()
	gc, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req = req.WithContext(ctx)
	req.Header = headers.Clone()
	gc.Request = req
	gc.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	gc.Set(string(middleware2.ContextKeyUser), subject)
	gc.Set(string(middleware2.ContextKeyUserRole), role)

	h.openAIGateway.Images(gc)
	if rec.Code < 200 || rec.Code >= 300 {
		_ = h.service.MarkTaskFailed(context.Background(), taskID, fmt.Errorf("%s", strings.TrimSpace(rec.Body.String())))
		return
	}
	if _, err := h.service.CompletePendingTaskFromResponse(context.Background(), taskID, rec.Body.Bytes()); err != nil {
		_ = h.service.MarkTaskFailed(context.Background(), taskID, err)
	}
}

func ensureImageGenerationB64Response(body []byte) []byte {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	if _, ok := obj["response_format"]; !ok {
		obj["response_format"] = "b64_json"
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}

func extractImageGenerationAPIKeyID(contentType string, body []byte) (*int64, []byte, string, error) {
	if strings.HasPrefix(strings.ToLower(contentType), "multipart/form-data") {
		return extractImageGenerationAPIKeyIDFromMultipart(contentType, body)
	}
	var obj map[string]any
	if len(body) == 0 {
		return nil, body, contentType, nil
	}
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, body, contentType, nil
	}
	apiKeyID := parseJSONInt64(obj["api_key_id"])
	delete(obj, "api_key_id")
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, body, contentType, err
	}
	if apiKeyID <= 0 {
		return nil, out, contentType, nil
	}
	return &apiKeyID, out, contentType, nil
}

func extractImageGenerationAPIKeyIDFromMultipart(contentType string, body []byte) (*int64, []byte, string, error) {
	req := httptest.NewRequest(http.MethodPost, "/rewrite", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	if err := req.ParseMultipartForm(64 << 20); err != nil {
		return nil, body, contentType, err
	}
	defer func(form *multipart.Form) {
		if form != nil {
			_ = form.RemoveAll()
		}
	}(req.MultipartForm)

	apiKeyID, _ := strconv.ParseInt(strings.TrimSpace(req.FormValue("api_key_id")), 10, 64)
	var out bytes.Buffer
	writer := multipart.NewWriter(&out)
	for key, values := range req.MultipartForm.Value {
		if key == "api_key_id" {
			continue
		}
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				_ = writer.Close()
				return nil, nil, "", err
			}
		}
	}
	for key, files := range req.MultipartForm.File {
		for _, header := range files {
			file, err := header.Open()
			if err != nil {
				_ = writer.Close()
				return nil, nil, "", err
			}
			partHeader := textproto.MIMEHeader{}
			for headerKey, values := range header.Header {
				partHeader[headerKey] = append([]string(nil), values...)
			}
			if partHeader.Get("Content-Disposition") == "" {
				partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeMultipartQuote(key), escapeMultipartQuote(header.Filename)))
			}
			part, err := writer.CreatePart(partHeader)
			if err != nil {
				_ = file.Close()
				_ = writer.Close()
				return nil, nil, "", err
			}
			if _, err := io.Copy(part, file); err != nil {
				_ = file.Close()
				_ = writer.Close()
				return nil, nil, "", err
			}
			_ = file.Close()
		}
	}
	if err := writer.Close(); err != nil {
		return nil, nil, "", err
	}
	if apiKeyID <= 0 {
		return nil, out.Bytes(), writer.FormDataContentType(), nil
	}
	return &apiKeyID, out.Bytes(), writer.FormDataContentType(), nil
}

func parseJSONInt64(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		id, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return id
	default:
		return 0
	}
}

func summarizeImageGenerationMultipart(contentType string, body []byte) []byte {
	req := httptest.NewRequest(http.MethodPost, "/summary", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	if err := req.ParseMultipartForm(64 << 20); err != nil {
		return []byte("{}")
	}
	defer func(form *multipart.Form) {
		if form != nil {
			_ = form.RemoveAll()
		}
	}(req.MultipartForm)

	obj := map[string]any{"response_format": "b64_json"}
	for _, key := range []string{"model", "prompt", "size", "quality", "n"} {
		if value := strings.TrimSpace(req.FormValue(key)); value != "" {
			obj[key] = value
		}
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return []byte("{}")
	}
	return out
}

type imageGenerationSourceFile struct {
	FieldName string
	FileName  string
	MimeType  string
	Data      []byte
}

func parseImageGenerationSourceResultID(contentType string, body []byte) int64 {
	req := httptest.NewRequest(http.MethodPost, "/summary", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	if err := req.ParseMultipartForm(64 << 20); err != nil {
		return 0
	}
	defer func(form *multipart.Form) {
		if form != nil {
			_ = form.RemoveAll()
		}
	}(req.MultipartForm)
	id, _ := strconv.ParseInt(strings.TrimSpace(req.FormValue("source_result_id")), 10, 64)
	return id
}

func imageGenerationMultipartHasFile(contentType string, body []byte, field string) bool {
	req := httptest.NewRequest(http.MethodPost, "/summary", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	if err := req.ParseMultipartForm(64 << 20); err != nil {
		return false
	}
	defer func(form *multipart.Form) {
		if form != nil {
			_ = form.RemoveAll()
		}
	}(req.MultipartForm)
	return len(req.MultipartForm.File[field]) > 0
}

func injectImageGenerationSourceFile(contentType string, body []byte, source imageGenerationSourceFile) (string, []byte, error) {
	if strings.TrimSpace(source.FieldName) == "" {
		source.FieldName = "image"
	}
	if strings.TrimSpace(source.FileName) == "" {
		source.FileName = "source.png"
	}
	if strings.TrimSpace(source.MimeType) == "" {
		source.MimeType = "application/octet-stream"
	}
	if len(source.Data) == 0 {
		return "", nil, fmt.Errorf("source image is empty")
	}

	req := httptest.NewRequest(http.MethodPost, "/rewrite", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	if err := req.ParseMultipartForm(64 << 20); err != nil {
		return "", nil, err
	}
	defer func(form *multipart.Form) {
		if form != nil {
			_ = form.RemoveAll()
		}
	}(req.MultipartForm)

	var out bytes.Buffer
	writer := multipart.NewWriter(&out)
	for key, values := range req.MultipartForm.Value {
		if key == "source_result_id" {
			continue
		}
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				_ = writer.Close()
				return "", nil, err
			}
		}
	}
	for key, files := range req.MultipartForm.File {
		if key == source.FieldName {
			continue
		}
		for _, header := range files {
			file, err := header.Open()
			if err != nil {
				_ = writer.Close()
				return "", nil, err
			}
			part, err := writer.CreateFormFile(key, header.Filename)
			if err != nil {
				_ = file.Close()
				_ = writer.Close()
				return "", nil, err
			}
			if _, err := io.Copy(part, file); err != nil {
				_ = file.Close()
				_ = writer.Close()
				return "", nil, err
			}
			_ = file.Close()
		}
	}
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeMultipartQuote(source.FieldName), escapeMultipartQuote(source.FileName)))
	partHeader.Set("Content-Type", source.MimeType)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		_ = writer.Close()
		return "", nil, err
	}
	if _, err := part.Write(source.Data); err != nil {
		_ = writer.Close()
		return "", nil, err
	}
	if err := writer.Close(); err != nil {
		return "", nil, err
	}
	return writer.FormDataContentType(), out.Bytes(), nil
}

func escapeMultipartQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

type imageGenerationTaskDTO struct {
	ID        int64                      `json:"id"`
	Mode      string                     `json:"mode"`
	Status    string                     `json:"status"`
	Model     string                     `json:"model"`
	Prompt    string                     `json:"prompt"`
	Size      string                     `json:"size"`
	Error     *string                    `json:"error_message,omitempty"`
	ExpiresAt string                     `json:"expires_at"`
	CreatedAt string                     `json:"created_at"`
	Results   []imageGenerationResultDTO `json:"results"`
}

type imageGenerationResultDTO struct {
	ID        int64  `json:"id"`
	Index     int    `json:"index"`
	MimeType  string `json:"mime_type"`
	SizeBytes int64  `json:"size_bytes"`
	URL       string `json:"url"`
}

func imageGenerationTasksToDTO(tasks []service.ImageGenerationTask) []imageGenerationTaskDTO {
	out := make([]imageGenerationTaskDTO, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, imageGenerationTaskToDTO(task))
	}
	return out
}

func imageGenerationTaskToDTO(task service.ImageGenerationTask) imageGenerationTaskDTO {
	results := make([]imageGenerationResultDTO, 0, len(task.Results))
	for _, res := range task.Results {
		results = append(results, imageGenerationResultDTO{
			ID:        res.ID,
			Index:     res.Index,
			MimeType:  res.MimeType,
			SizeBytes: res.SizeBytes,
			URL:       "/api/v1/image-generation/files/" + strconv.FormatInt(res.ID, 10),
		})
	}
	return imageGenerationTaskDTO{
		ID:        task.ID,
		Mode:      task.Mode,
		Status:    task.Status,
		Model:     task.Model,
		Prompt:    task.Prompt,
		Size:      imageGenerationTaskSize(task.RequestJSON),
		Error:     task.ErrorMessage,
		ExpiresAt: task.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
		CreatedAt: task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Results:   results,
	}
}

func imageGenerationTaskSize(requestJSON []byte) string {
	var obj struct {
		Size string `json:"size"`
	}
	if err := json.Unmarshal(requestJSON, &obj); err != nil {
		return ""
	}
	return obj.Size
}

func imageExtForMime(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}
