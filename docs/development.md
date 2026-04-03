# 开发与协作流程

## 本地启动

后端:

```bash
cd backend
go run .
```

前端:

```bash
cd frontend
npm install
npm run dev
```

默认地址:

- 前端: `http://127.0.0.1:18889`
- 后端: `http://127.0.0.1:18888`

## 常用验证命令

后端测试:

```bash
cd backend
go test ./...
```

前端构建:

```bash
cd frontend
npm run build
```

## 真实 API 联调

可以用 curl 直接验证流式事件:

```bash
curl -N -X POST http://127.0.0.1:18888/api/chat/stream \
  -H 'Content-Type: application/json' \
  -d '{"backend_id":"minimax-main","messages":[{"role":"user","content":"你好"}]}'
```

应能看到 `reasoning/content/done` 事件流.

## 提交前检查清单

1. 后端测试通过.
2. 前端构建通过.
3. 若改了接口,更新 `docs/api.md`.
4. 若改了架构,更新 `docs/architecture.md`.
5. 若新增后端类型,更新 `docs/backend.md`.

## 配置与安全说明

当前项目按需求使用明文 `api_key` 配置.

注意:

- 不要把真实密钥提交到公开仓库.
- 私有仓库也建议后续切换到环境变量方案.
