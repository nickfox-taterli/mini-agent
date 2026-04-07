# 项目笔记

## 2026-04-14: 下载 URL 生成策略收敛到 MCP 工具

背景:

- 仅靠 system prompt 约束模型拼接下载 URL,在真实对话里会出现端口或路径猜测偏差.

决策:

- Skill 产物先返回本地磁盘路径.
- 新增 MCP 工具 `convert_local_path_to_url`,由工具执行路径到 URL 的受控转换.
- Prompt 只要求模型调用该工具,不再让模型自行拼接 URL.

收益:

- 消除 host/port/path 猜测行为.
- URL 生成逻辑可测试,可回归.
- 后续端口或路由变化时,只需改后端映射逻辑.
