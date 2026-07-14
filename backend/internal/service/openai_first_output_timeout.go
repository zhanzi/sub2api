package service

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIFirstOutputTimeoutCode    = "upstream_first_output_timeout"
	openAIResponseHeaderTimeoutCode = "upstream_response_header_timeout"
	openAIMaxPendingPreambleBytes   = 1 << 20
)

func (s *OpenAIGatewayService) openAIFirstOutputTimeout() time.Duration {
	if s == nil || s.cfg == nil || s.cfg.Gateway.OpenAIFirstOutputTimeout <= 0 {
		return 0
	}
	return time.Duration(s.cfg.Gateway.OpenAIFirstOutputTimeout) * time.Second
}

func openAIStreamLineStopsFirstOutputTimer(line string) bool {
	data, ok := extractOpenAISSEDataLine(line)
	if !ok {
		return false
	}
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return false
	}
	if trimmed == "[DONE]" {
		return true
	}
	eventType := strings.TrimSpace(gjson.Get(trimmed, "type").String())
	if eventType == "response.failed" {
		return true
	}
	return openAIStreamDataStartsClientOutput(trimmed, eventType)
}

func writeOpenAITimeoutResponse(c *gin.Context, code, message string) {
	if c == nil {
		return
	}
	headers := c.Writer.Header()
	for _, name := range []string{
		"Cache-Control",
		"Connection",
		"Content-Encoding",
		"Content-Length",
		"Transfer-Encoding",
		"X-Accel-Buffering",
	} {
		headers.Del(name)
	}
	setOpsUpstreamError(c, http.StatusGatewayTimeout, message, "")
	MarkOpsStreamError(c, code, message, http.StatusGatewayTimeout)
	MarkResponseCommitted(c)
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.JSON(http.StatusGatewayTimeout, gin.H{
		"error": gin.H{
			"type":    "server_error",
			"code":    code,
			"message": message,
		},
	})
}

func (s *OpenAIGatewayService) handleOpenAIFirstOutputTimeout(
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
) error {
	timeout := s.openAIFirstOutputTimeout()
	message := "OpenAI upstream timed out before producing output"
	if timeout > 0 {
		message = fmt.Sprintf("OpenAI upstream did not produce output within %s", timeout)
	}
	if c != nil {
		event := OpsUpstreamErrorEvent{
			Platform:           PlatformOpenAI,
			UpstreamStatusCode: http.StatusGatewayTimeout,
			UpstreamRequestID:  strings.TrimSpace(upstreamRequestID),
			Passthrough:        passthrough,
			Kind:               "timeout",
			Message:            message,
		}
		if account != nil {
			event.Platform = account.Platform
			event.AccountID = account.ID
			event.AccountName = account.Name
		}
		appendOpsUpstreamError(c, event)
	}
	writeOpenAITimeoutResponse(c, openAIFirstOutputTimeoutCode, message)
	return fmt.Errorf("%s: %s", openAIFirstOutputTimeoutCode, message)
}
