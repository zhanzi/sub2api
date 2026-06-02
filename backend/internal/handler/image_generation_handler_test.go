package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInjectImageGenerationSourceFileUsesHistoryResult(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("prompt", "make it warmer"))
	require.NoError(t, writer.WriteField("source_result_id", "42"))
	require.NoError(t, writer.Close())

	source := imageGenerationSourceFile{
		FieldName: "image",
		FileName:  "history-42.png",
		MimeType:  "image/png",
		Data:      []byte("png-bytes"),
	}
	contentType, rewritten, err := injectImageGenerationSourceFile(writer.FormDataContentType(), body.Bytes(), source)
	require.NoError(t, err)
	require.Contains(t, contentType, "multipart/form-data")

	req, err := http.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(rewritten))
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentType)
	require.NoError(t, req.ParseMultipartForm(1024))
	require.Equal(t, "make it warmer", req.FormValue("prompt"))
	require.Equal(t, "", req.FormValue("source_result_id"))

	files := req.MultipartForm.File["image"]
	require.Len(t, files, 1)
	require.Equal(t, "history-42.png", files[0].Filename)
	file, err := files[0].Open()
	require.NoError(t, err)
	defer file.Close()
	data := new(bytes.Buffer)
	_, err = data.ReadFrom(file)
	require.NoError(t, err)
	require.Equal(t, []byte("png-bytes"), data.Bytes())
}

func TestExtractImageGenerationAPIKeyIDStripsJSONField(t *testing.T) {
	apiKeyID, rewritten, _, err := extractImageGenerationAPIKeyID("application/json", []byte(`{"model":"gpt-image-2","prompt":"hello","api_key_id":42}`))
	require.NoError(t, err)
	require.NotNil(t, apiKeyID)
	require.Equal(t, int64(42), *apiKeyID)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rewritten, &payload))
	require.Equal(t, "gpt-image-2", payload["model"])
	require.NotContains(t, payload, "api_key_id")
}

func TestExtractImageGenerationAPIKeyIDStripsMultipartField(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("api_key_id", "42"))
	require.NoError(t, writer.WriteField("prompt", "hello"))
	part, err := writer.CreateFormFile("image", "source.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("png"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	apiKeyID, rewritten, contentType, err := extractImageGenerationAPIKeyID(writer.FormDataContentType(), body.Bytes())
	require.NoError(t, err)
	require.NotNil(t, apiKeyID)
	require.Equal(t, int64(42), *apiKeyID)

	req, err := http.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(rewritten))
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentType)
	require.NoError(t, req.ParseMultipartForm(1024))
	require.Equal(t, "hello", req.FormValue("prompt"))
	require.Empty(t, req.FormValue("api_key_id"))
	require.Len(t, req.MultipartForm.File["image"], 1)
}

func TestSetImageGenerationGatewayContextCarriesSubscription(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	apiKey := &service.APIKey{ID: 7}
	subject := middleware2.AuthSubject{UserID: 42, Concurrency: 3}
	subscription := &service.UserSubscription{ID: 99, UserID: 42, GroupID: 5}

	setImageGenerationGatewayContext(c, apiKey, subject, "user", subscription)

	gotAPIKey, ok := middleware2.GetAPIKeyFromContext(c)
	require.True(t, ok)
	require.Same(t, apiKey, gotAPIKey)
	gotSub, ok := middleware2.GetSubscriptionFromContext(c)
	require.True(t, ok)
	require.Same(t, subscription, gotSub)
	gotSubject, ok := middleware2.GetAuthSubjectFromContext(c)
	require.True(t, ok)
	require.Equal(t, subject, gotSubject)
	require.Equal(t, "user", c.GetString(string(middleware2.ContextKeyUserRole)))
}
