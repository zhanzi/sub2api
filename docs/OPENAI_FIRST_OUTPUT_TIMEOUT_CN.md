# OpenAI 首有效输出超时

## 背景

`GATEWAY_OPENAI_RESPONSE_HEADER_TIMEOUT` 只限制等待 HTTP 响应头的时间。上游已经返回响应头后，如果 SSE 长时间只有 `response.created`、`response.in_progress`、空行或心跳，原实现仍可能等待十几分钟。

## 配置

- 配置键：`gateway.openai_first_output_timeout`
- 环境变量：`GATEWAY_OPENAI_FIRST_OUTPUT_TIMEOUT`
- 默认值：`300` 秒
- 有效范围：`30-1800` 秒；`0` 表示禁用

计时从收到上游响应头并进入 HTTP SSE 处理开始。现有 `openAIStreamDataStartsClientOutput` 语义认定的首个有效输出会停止计时；`response.failed` 和 `[DONE]` 也会结束等待，但不会记录为首 token。前导事件、空行、SSE 注释和心跳不会延长等待。

## 超时行为

- 关闭当前上游响应体，停止继续等待。
- 返回 HTTP `504`，错误码为 `upstream_first_output_timeout`。
- 不返回 `UpstreamFailoverError`，因此不会切换账号重放请求。
- 响应头超时同样按“上游结果未知”处理，返回 `504`，不跨账号重放。
- DNS 失败、连接拒绝、普通 EOF 等明确 transport 故障继续沿用原有 failover。

禁止超时后自动重放的原因是：请求可能已经被上游接收并产生计费或其他副作用，换账号重放可能造成重复执行。
