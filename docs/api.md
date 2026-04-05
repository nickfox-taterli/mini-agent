# API 协议

Base URL(本地默认): `http://127.0.0.1:18888`

## GET /api/health

用途: 健康检查.

响应示例:

```json
{
  "status": "ok",
  "host": "0.0.0.0",
  "port": 18888
}
```

## GET /api/backends

用途: 获取后端列表(含启用状态).

响应示例:

```json
{
  "backends": [
    {
      "id": "minimax-main",
      "type": "openai_compatible",
      "model": "MiniMax-M2.7",
      "enabled": true
    }
  ]
}
```

## POST /api/chat/stream

用途: 发起一轮流式对话.
说明: 后端会在对话过程中自动处理模型返回的 `tool_calls`, 并调用内置 MCP 工具后继续补全回答.

请求体:

```json
{
  "backend_id": "minimax-main",
  "messages": [
    {"role": "user", "content": "你好"}
  ]
}
```

字段约束:

- `backend_id`: 可选,留空时走默认启用后端
- `messages`: 必填,至少 1 条
- `role`: 仅允许 `system/user/assistant`
- `content`: 非空字符串

返回类型:

- `Content-Type: text/event-stream`

SSE 事件:

1. `event: reasoning`

```json
{"delta":"..."}
```

2. `event: content`

```json
{"delta":"..."}
```

3. `event: done`

```json
{"finish_reason":"stop","usage":{}}
```

4. `event: retrying`

```json
{
  "trace_id":"t...",
  "attempt":1,
  "max_attempts":4,
  "delay_seconds":2.5,
  "status_code":529,
  "upstream_request_id":"...",
  "cause":"overloaded_error",
  "retryable":true,
  "busy":true,
  "message":"..."
}
```

当上游返回 429/529/5xx 或网络抖动时, 后端会自动重试 (最多 4 次, 指数退避并支持 `Retry-After`). 每次重试前会发送此事件.
说明:
- `busy=true` 通常表示上游真实繁忙.
- `busy=false` 且 `retryable=false` 更可能是调用参数/鉴权/协议问题.
- `trace_id` 可用于后端日志定位.

5. `event: error`

```json
{"message":"..."}
```

## 错误语义

- 请求体不合法: HTTP `400`(普通 JSON 错误响应)
- 上游 429/529/5xx 或网络抖动: 后端自动重试 (最多 4 次), 每次重试前发送 SSE `retrying` 事件; 重试耗尽后发送 SSE `error` 事件
- 其他上游失败: SSE `error` 事件(HTTP 状态仍为 `200`)

## 后端日志

- 默认日志文件: `backend/logs/backend.log`
- 每次请求记录 `trace_id`, `status`, `busy`, `upstream_request_id`.

## MCP: /api/mcp

用途: MCP Streamable HTTP 入口. 当前内置工具:

- `get_system_time`: 获取系统时间.

前端调用示例(调用工具):

```json
{
  "jsonrpc": "2.0",
  "id": "call-time-tool",
  "method": "tools/call",
  "params": {
    "name": "get_system_time",
    "arguments": {}
  }
}
```

返回 `result.structuredContent` 示例:

```json
{
  "now_unix": 1713062400,
  "now_rfc3339": "2026-04-14T14:00:00+08:00",
  "now_local": "2026-04-14 14:00:00",
  "timezone_name": "Asia/Shanghai"
}
```
