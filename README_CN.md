# qwen2API 中文说明

默认主文档是英文版：[README.md](./README.md)。

## 项目简介

qwen2API 是自托管千问 Web 协议转换网关，当前主线为 `v2.0` Go 后端 + React WebUI；`v1.0` Python/FastAPI 仅作为历史版本说明。

主要能力：

- OpenAI / Anthropic / Gemini 兼容接口。
- 账号池、API Key 管理、运行设置、模型测试、图片测试、视频测试。
- Docker Hub 镜像、本地 Docker 构建、GitHub Actions 多架构打包。
- 环境变量注入 API Key、环境变量注入账号、keepalive 保活任务。

## Docker 路径

- Docker 容器内数据目录：`/app/data`
- Docker 容器内日志目录：`/app/logs`
- Docker 默认宿主机数据目录：当前目录 `./data`
- Docker 默认宿主机日志目录：当前目录 `./logs`
- 本地非 Docker 运行默认数据目录：当前项目下 `data`

## Docker Hub 部署

推荐方式是创建 `docker-compose.yml` 指向 Docker Hub 镜像，然后执行：

```bash
docker compose pull
docker compose up -d
docker compose logs -f qwen2api
```

完整 compose YAML、配置说明、开发指南、参与贡献和其他信息见英文主文档：[README.md](./README.md)。

## 安全说明

不要在 `.env.example`、README、提交记录或 Issue 中写入真实 `ADMIN_KEY`、Qwen token、Cookie、密码或下游 API Key。

## 特别鸣谢

- 特别鸣谢: [LinuxDo](https://linux.do/)
