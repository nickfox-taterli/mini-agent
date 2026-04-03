# 系统架构

## 目标

- 提供极简聊天体验,风格接近 ChatGPT.
- 支持流式输出.
- 同时展示"思考"和"回答".
- 后端从第一天开始支持多后端扩展.

## 当前技术栈

- 后端: Go 1.22+, Gin, YAML 配置
- 前端: Vue 3, Vite
- 上游模型: MiniMax OpenAI 兼容接口

## 运行拓扑

- 前端开发服务监听 `18889`
- 后端 API 服务监听 `18888`
- 前端通过 `VITE_API_BASE` 指向后端

## 数据流

1. 用户在前端输入文本.
2. 前端将完整对话历史 `messages[]` POST 到 `/api/chat/stream`.
3. 后端选择目标 backend adapter 并调用上游流式接口.
4. 后端把上游流转换成标准 SSE 事件并推给前端.
5. 前端按事件类型更新 UI.

## 流式事件模型

- `reasoning`: 思考内容增量
- `content`: 回答正文增量
- `done`: 本轮结束,包含 `finish_reason/usage`
- `error`: 错误信息

## 多后端扩展点

- 配置层: `backend/config.yaml` 的 `backends[]`
- 代码层: `backend/internal/backend/Adapter` 接口
- 调度层: `backend/internal/backend/Manager`

## 关于思考分离

当前后端兼容两种上游格式:

- `reasoning_details` 字段
- `content` 中的 `<think>...</think>` 标签

后端会统一分离后再发 SSE,前端不需要关心上游格式差异.
