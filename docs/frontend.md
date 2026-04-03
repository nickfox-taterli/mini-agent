# 前端开发指南

## 模块结构

- `src/App.vue`: 页面逻辑与 UI
- `src/style.css`: 样式
- `src/main.js`: 入口

## 页面行为

- 中部消息区
- 底部输入框和发送按钮
- 用户消息在右侧,助手消息在左侧
- 助手消息分为"思考"和"回答"两段

## 流式协议处理

前端使用 `fetch + ReadableStream` 处理 SSE:

1. POST `/api/chat/stream`
2. 按 `\n\n` 分块
3. 解析 `event:` 和 `data:`
4. 根据事件类型更新 `assistant.reasoning` 或 `assistant.content`

事件映射:

- `reasoning` -> 追加到思考区域
- `content` -> 追加到回答区域
- `done` -> 结束本轮
- `error` -> 在回答区追加错误提示

## 本地开发

```bash
cd frontend
npm install
npm run dev
```

## 构建检查

```bash
cd frontend
npm run build
```
