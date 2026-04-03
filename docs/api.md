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

4. `event: error`

```json
{"message":"..."}
```

## 错误语义

- 请求体不合法: HTTP `400`(普通 JSON 错误响应)
- 流式过程中上游失败: SSE `error` 事件(HTTP 状态仍为 `200`)
