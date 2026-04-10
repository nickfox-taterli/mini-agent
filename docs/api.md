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

## GET /api/conversations

用途: 列出所有会话 (不含消息), 按创建时间降序.

响应示例:

```json
{
  "conversations": [
    {
      "id": "conv-1713062400000-abc123",
      "title": "你好",
      "createdAt": 1713062400000
    }
  ]
}
```

## POST /api/conversations

用途: 创建新会话.

请求体:

```json
{
  "id": "conv-1713062400000-abc123",
  "title": "新对话",
  "createdAt": 1713062400000,
  "messages": []
}
```

字段约束:

- `id`: 必填, 前端生成的唯一标识
- `title`: 可选, 默认 "新对话"
- `createdAt`: 可选, Unix 毫秒时间戳
- `messages`: 可选, 初始消息列表

响应示例:

```json
{
  "conversation": {
    "id": "conv-1713062400000-abc123",
    "title": "新对话",
    "createdAt": 1713062400000,
    "messages": []
  }
}
```

## GET /api/conversations/:id

用途: 获取单个会话详情 (含全部消息).

响应示例:

```json
{
  "conversation": {
    "id": "conv-1713062400000-abc123",
    "title": "你好",
    "createdAt": 1713062400000,
    "messages": [
      {
        "role": "user",
        "content": "你好",
        "reasoning": "",
        "reasoningDone": false,
        "thinkingDuration": null
      },
      {
        "role": "assistant",
        "content": "你好! 有什么可以帮助你的?",
        "reasoning": "用户在打招呼...",
        "reasoningDone": true,
        "thinkingDuration": 1.23
      }
    ]
  }
}
```

## PUT /api/conversations/:id

用途: 更新会话 (标题 + 消息全量替换).

请求体:

```json
{
  "title": "新的对话标题",
  "messages": [
    {"role": "user", "content": "你好"},
    {"role": "assistant", "content": "你好!", "reasoningDone": true}
  ]
}
```

响应示例:

```json
{"ok": true}
```

## DELETE /api/conversations/:id

用途: 删除会话及其全部消息.

响应示例:

```json
{"ok": true}
```

## MCP: /api/mcp

用途: MCP Streamable HTTP 入口. 当前内置工具:

- `get_system_time`: 获取系统时间.
- `run_skill_bash`: 在 `backend/skills/<skill_name>` 中执行 Bash 命令,用于通用 skill 执行.
- `web_fetch`: 抓取网页文本内容.
- `convert_local_path_to_url`: 将前端目录下的本地磁盘路径转换为可下载 URL.

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

执行 Skill 脚本调用示例:

```json
{
  "jsonrpc": "2.0",
  "id": "run-skill-bash",
  "method": "tools/call",
  "params": {
    "name": "run_skill_bash",
    "arguments": {
      "skill_name": "minimax-xlsx",
      "command": "pwd"
    }
  }
}
```

路径转 URL 调用示例:

```json
{
  "jsonrpc": "2.0",
  "id": "convert-path",
  "method": "tools/call",
  "params": {
    "name": "convert_local_path_to_url",
    "arguments": {
      "local_path": "<project_root>/frontend/upload/2026/04/14/report.xlsx"
    }
  }
}
```
