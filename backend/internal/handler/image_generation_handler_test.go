package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"testing"

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
