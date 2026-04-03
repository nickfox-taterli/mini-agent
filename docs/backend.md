# 后端开发指南

## 模块结构

- `main.go`: 启动入口
- `internal/config`: 配置结构与校验
- `internal/backend`: 后端抽象,管理器,OpenAI 兼容适配器
- `internal/server`: HTTP 路由和 SSE 输出
- `tests`: 集成测试

## 配置文件

文件: `backend/config.yaml`

关键字段:

- `server.host`
- `server.port`
- `backends[]`

`backends[]` 字段:

- `id`
- `type`
- `base_url`
- `api_key`
- `model`
- `temperature`
- `reasoning_split`
- `enabled`

## 多后端实现方式

后端适配器接口:

```go
type Adapter interface {
    ID() string
    StreamChat(ctx context.Context, req StreamRequest, emit EmitFunc) error
}
```

新增一个后端类型时需要做 3 件事:

1. 在 `internal/backend` 增加新的 adapter 实现.
2. 在 `Manager.NewManager` 的 `switch b.Type` 中注册.
3. 补充单元测试和至少 1 个流式集成测试场景.

## 流式分离规则

在 OpenAI 兼容适配器中,优先处理 `reasoning_details`.

如果上游把思考写进 `content`,则按 `<think>...</think>` 分离:

- 标签内推送 `reasoning`
- 标签外推送 `content`

## 测试要求

提交前最少执行:

```bash
cd backend
go test ./...
```

涉及流式解析改动时,必须更新以下测试:

- `internal/backend/openai_compatible_test.go`
- `tests/server_stream_test.go`
