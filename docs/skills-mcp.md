# Skill 与 MCP 扩展指南

本文档用于沉淀当前项目在 `Skill` 与 `MCP` 方面的实现约定,方便后续快速新增能力并保持一致性.

## 目标

- 让模型可感知本地 Skill,并在合适场景调用.
- 通过 `/api/mcp` 暴露本地工具能力,支持模型函数调用回路.
- 保持最小改动路径,新增能力时不破坏现有流式语义.

## 当前实现总览

- Skill 扫描入口: `backend/internal/skills/catalog.go`
- MCP 服务入口: `backend/internal/mcpserver/time_server.go`
- 工具执行入口(函数调用回路): `backend/internal/mcpserver/tool_runtime.go`
- HTTP 路由挂载: `backend/internal/server/server.go`
- 技能目录根路径: `backend/skills`

---

## 一,如何新增 Skill

### 1) 新建 Skill 目录

在 `backend/skills` 下新增目录,例如:

```bash
backend/skills/my-skill/
```

目录内至少包含:

```bash
backend/skills/my-skill/SKILL.md
```

### 2) 编写 SKILL.md 元信息

`catalog.go` 会读取 `SKILL.md` 中的 `name:` 和 `description:` 并注入系统提示词.

建议模板:

```md
---
name: my-skill
description: 用于 xxx 场景,支持 xxx 操作.
---

# My Skill

这里写技能使用说明,输入输出约束,以及推荐命令.
```

注意事项:

- `name:` 为空时会回退到目录名.
- `description:` 建议一句话说明能力边界.
- 只要目录下有 `SKILL.md`,启动时就会被自动发现.

### 3) 如需可执行逻辑,放到 Skill 目录内

推荐结构:

```bash
backend/skills/my-skill/
  SKILL.md
  scripts/
  references/
  templates/
```

模型可通过 `run_skill_bash` 在该目录中执行命令. 运行时会自动注入:

- `SKILL_DIR`: 当前 skill 绝对路径.
- `FRONTEND_UPLOAD_DIR`: 前端上传目录绝对路径 (按日期分区, 如 `frontend/upload/2026/04/14/`).
- `FRONTEND_UPLOAD_URL_BASE`: 上传目录的 HTTP URL 前缀 (如 `http://127.0.0.1:18889/upload/2026/04/14`).
- `FRONTEND_TMP_DIR`: 已弃用, 保留向后兼容.

### 4) 最小验证

启动后调用 `POST /api/chat/stream`,检查上游请求中的系统提示词是否包含新 Skill 信息. 也可直接跑后端测试确保主流程正常.

---

## 二,如何新增 MCP 工具

### 1) 在 mcpserver 中定义输入输出结构

建议在 `backend/internal/mcpserver` 新建或复用文件,定义:

- `xxxInput`
- `xxxOutput`
- `xxx` 实际处理函数
- 可复用的 `xxxLocal` 或纯逻辑函数(便于测试和复用)

参考现有实现:

- `get_system_time`
- `run_skill_bash`

### 2) 在 HTTP MCP Server 注册工具

在 `NewHTTPHandler()` 里追加:

```go
mcp.AddTool(srv, &mcp.Tool{
    Name:        "your_tool_name",
    Description: "Tool description.",
}, yourToolFunc)
```

位置: `backend/internal/mcpserver/time_server.go`.

### 3) 在函数调用回路中注册执行分支

模型通过 tool call 触发时,会走 `ExecuteToolByJSON()`.
新增工具后,必须在 `backend/internal/mcpserver/tool_runtime.go` 的 `switch name` 中补一个 `case`,负责:

- 解析 `rawArgs`.
- 调用本地逻辑.
- 返回 `map[string]any` 结构化结果.

如果只在 `NewHTTPHandler()` 注册,但未在 `ExecuteToolByJSON()` 增加分支,模型函数调用链路会报 `unsupported tool`.

### 4) 安全边界建议

新增工具时默认遵循以下规则:

- 对路径参数做 `filepath.Clean` 和路径穿越拦截.
- 禁止绝对路径写入(除非明确设计为受控白名单).
- 对外部命令执行设置超时和最大时长上限.
- 输出中保留必要的调试字段,避免泄露敏感信息.

---

## 三,新增 Skill 与 MCP 的推荐组合方式

推荐模式:

1. Skill 负责方法论和目录内脚本组织.
2. MCP 提供受控执行入口,例如 `run_skill_bash`.
3. 文件产物统一写入 `frontend/tmp`,由前端下载或展示.

好处:

- Skill 易扩展.
- MCP 易治理.
- 保持模型调用路径统一.

---

## 四,测试与回归清单

每次新增 Skill 或 MCP,至少执行:

```bash
cd backend && go test ./...
cd frontend && npm run build
```

重点关注:

- MCP API 测试: `backend/tests/server_mcp_test.go`
- 函数调用回路: `backend/tests/server_tool_call_test.go`
- 流式语义不变: `reasoning/content/done/error`

如新增接口契约,同步更新:

- `docs/api.md`

---

## 五,快速检查清单

新增 Skill:

- 已创建 `backend/skills/<name>/SKILL.md`.
- `name/description` 可被扫描.
- 如有脚本,可在 skill 目录独立运行.

新增 MCP:

- 已在 `NewHTTPHandler()` 注册工具.
- 已在 `ExecuteToolByJSON()` 增加 `case`.
- 参数校验,路径校验,超时控制已完成.
- 已补充测试或复用现有测试模式验证.

---

## 六,常见问题

Q: 新 Skill 不生效?

- 检查是否存在 `SKILL.md`.
- 检查 `name:`/`description:` 格式是否正确.
- 检查后端是否已重启.

Q: MCP 工具可在 `/api/mcp` 调用,但模型里调用失败?

- 通常是 `tool_runtime.go` 未注册对应 `case`.

Q: 生成文件找不到?

- 检查是否写入 `frontend/tmp`.
- 检查调用时 `file_name` 是否为相对路径.
