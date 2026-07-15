# OpenAI 原生 Responses 首有效输出超时

## 背景

上游从 `v0.1.156` 起提供 native HTTP Responses 首输出总预算，可同时限制等待 HTTP 响应头和首个语义 SSE 事件的时间。本 fork 采用上游实现，并保留“不跨账号重放”的计费安全策略。

## 配置

- 配置键：`gateway.openai_first_output_timeout_seconds`
- 环境变量：`GATEWAY_OPENAI_FIRST_OUTPUT_TIMEOUT_SECONDS`
- 默认值：`0`，即禁用
- 有效范围：`30-600` 秒；`0` 表示禁用
- high/xhigh/max 可通过 `gateway.openai_high_effort_first_output_timeout_seconds` 或 `GATEWAY_OPENAI_HIGH_EFFORT_FIRST_OUTPUT_TIMEOUT_SECONDS` 单独设置，范围为 `30-1800` 秒；`0` 表示沿用普通值

计时从 native Responses 转发开始。等待上游响应头和等待首个语义 SSE 输出共用同一份总预算，不会串联叠加。前导事件、空行、SSE 注释和心跳不会延长等待。首输出前的单次尝试暂存上限为 8 MiB，避免向客户端暴露不完整 SSE 事件。

该功能只覆盖 native HTTP Responses 流式请求，不覆盖 passthrough、WebSocket 和非流式 Responses。Compose 的 `.env` 只参与变量替换，部署文件的 `environment` 必须映射新变量；旧变量 `GATEWAY_OPENAI_FIRST_OUTPUT_TIMEOUT` 不再生效。

## 超时行为

- 关闭当前上游响应体，停止继续等待。
- 返回 HTTP `504`，错误类型为 `first_output_timeout`。
- 内部仍使用上游的 `UpstreamFailoverError` 传递响应，但设置 `NextAccountAction: NextAccountStop`，因此不会切换账号重放。
- 响应头等待和首个语义输出等待都使用相同策略。

禁止超时后自动重放的原因是：请求可能已经被上游接收并产生计费或其他副作用，换账号重放可能造成重复执行。
