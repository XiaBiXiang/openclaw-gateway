# OpenClaw Gateway

[English README](README.md)

OpenClaw Gateway 是一个面向混合推理场景的本地优先路由网关。

它位于客户端与模型提供方之间，对每个请求做路由判断，决定请求应当走本地模型还是云端模型，并向上层暴露一个统一的 OpenAI 兼容接口。

## 项目定位

这个项目关注的是一个非常具体的问题：

- 希望把敏感、离线、低复杂度请求优先留在本地执行
- 在本地模型能力不足时，自动升级到云端模型
- 对外只暴露一个稳定入口，避免上层应用频繁切换多套模型接口
- 为后续可视化治理、决策解释和策略管理预留控制面

当前版本不是一个完整的智能体平台，而是一个可运行、可扩展的网关内核。

## 当前能力

`v0.1.0` 的首版能力范围如下：

- 使用 Go 编写的单二进制 HTTP 网关
- 兼容 OpenAI 风格的 `POST /v1/chat/completions`
- 兼容 OpenAI 风格的 `POST /v1/responses`
- 支持将 `responses` 请求翻译为本地下游常见的 `chat/completions`
- 支持本地模型提供方和云端模型提供方
- 基于规则的路由决策
- 会话粘性路由，避免在本地和云端之间来回抖动
- 云端 `502/503/504` 的有限重试
- 将上游 HTML 错误规整为结构化 JSON 错误
- 提供路由决策检查接口，便于后续接入 dashboard

## 首版不做什么

以下内容不在首版目标内：

- 多智能体编排
- 流式输出
- 可视化工作流编排
- 完整的 Dashboard 前端
- 分布式会话存储

## 架构说明

首版架构保持简单：

- `client -> gateway -> local provider or cloud provider`
- Gateway 既是路由策略执行点，也是协议转换边界
- 未来如果增加 dashboard，它应当只承担控制面职责，不进入实时推理请求链路

更详细的设计说明见 [docs/architecture.md](docs/architecture.md)。

## 典型使用场景

这个项目适合以下类型的场景：

- OpenClaw 只希望对接一个统一模型入口
- 你本地已经有可运行的小模型，希望优先本地处理
- 你希望根据隐私级别、复杂度、上下文长度等信号自动切换本地和云端
- 你准备做端云协同产品，但暂时不想一开始就上复杂的控制平面

## 快速开始

### 1. 从源码运行

要求：

- Go `1.25` 或更高版本

启动步骤：

```bash
cp configs/config.example.json /tmp/openclaw-gateway.json
go run ./cmd/gateway -config /tmp/openclaw-gateway.json
```

默认健康检查：

```bash
curl -s http://127.0.0.1:8080/healthz
```

### 2. 构建本地二进制

```bash
go build -o bin/openclaw-gateway ./cmd/gateway
./bin/openclaw-gateway -config /path/to/config.json
```

### 3. 通过 Docker Compose 启动

```bash
cd deploy
cp .env.example .env
docker compose up --build
```

## 本地优先配置

仓库中提供了一个本地优先示例配置：

- [configs/config.local.example.json](configs/config.local.example.json)

这个示例的思路是：

- 本地模型走 `http://127.0.0.1:8000/v1`
- 网关本身监听 `http://127.0.0.1:18080`
- 默认优先本地
- 当请求复杂度变高、信心不足或上下文超限时，升级到云端

如果你要基于这个示例落地：

```bash
cp configs/config.local.example.json configs/config.local.json
```

然后按你的环境修改：

- `providers.local.base_url`
- `providers.local.model`
- `providers.cloud.base_url`
- `providers.cloud.model`
- `providers.cloud.api_key_env`

## 本地模型接入

如果你本地用的是 MLX 生态，仓库里已经准备了脚本：

- [scripts/run-mlx-server.sh](scripts/run-mlx-server.sh)
- [scripts/run-local.sh](scripts/run-local.sh)
- [scripts/run-hybrid-stack.sh](scripts/run-hybrid-stack.sh)

### 启动 MLX 本地模型服务

```bash
./scripts/run-mlx-server.sh mlx-community/Qwen3-4B-Instruct-2507-4bit
```

这个参数既可以是 Hugging Face repo id，也可以是本地模型目录。

### 启动 Gateway

```bash
export OPENAI_API_KEY=replace-me
./scripts/run-local.sh
```

### 一键启动本地模型加网关

```bash
MLX_MODEL=mlx-community/Qwen3-4B-Instruct-2507-4bit ./scripts/run-hybrid-stack.sh
```

这条命令会：

- 如果本地模型服务尚未运行，则先拉起 MLX OpenAI 兼容服务
- 等待本地模型端口就绪
- 再启动 Gateway

## OpenClaw 接入

如果你希望让 OpenClaw 只连接一个统一入口，可以把 Gateway 暴露为 OpenClaw 的自定义 provider。

OpenClaw 侧的关键思路是：

- OpenClaw 只请求 Gateway
- Gateway 决定当前请求走本地还是云端
- 上游云端密钥尽量放在 Gateway 环境变量中管理，而不是散落在客户端配置里

一个概念上的 OpenClaw provider 条目如下：

```json
{
  "my-local-router": {
    "baseUrl": "http://127.0.0.1:18080/v1",
    "apiKey": "not-required-unless-you-enable-auth",
    "api": "openai-responses",
    "models": [
      {
        "id": "auto",
        "name": "Auto Route",
        "reasoning": true,
        "input": ["text"],
        "contextWindow": 128000,
        "maxTokens": 32768
      }
    ]
  }
}
```

如果你的 OpenClaw 配置里已经存了云端密钥，也可以让脚本自动读取：

```bash
export OPENCLAW_PROVIDER_ID=openai
./scripts/run-local.sh
```

这时 `run-local.sh` 会：

- 读取当前网关配置中的 `providers.cloud.api_key_env`
- 去 `~/.openclaw/openclaw.json` 中找到对应 provider 的 `apiKey`
- 自动导出环境变量后再启动 Gateway

更详细的接入说明见 [docs/openclaw.md](docs/openclaw.md)。

## 目录结构

```text
cmd/gateway            进程入口与生命周期管理
internal/config        配置加载、校验与默认值
internal/policy        纯路由规则
internal/router        粘性会话与路由决策编排
internal/providers     上游 provider 抽象与适配器
internal/session       内存态会话存储
internal/server        HTTP 接口、请求解析与协议转换
internal/telemetry     结构化日志
configs                示例配置
docs                   架构、安装与路线图文档
scripts                本地模型与网关启动脚本
```

## 路由信号

当前规则引擎主要消费以下信号：

- `privacy_level` 为 `high` 或 `sensitive` 时强制本地
- `offline=true` 且配置允许时强制本地
- `complexity` 超过阈值时走云端
- `local_confidence` 低于阈值时走云端
- `estimated_tokens` 超过本地上下文能力时走云端
- `session_id` 用于会话粘性和云端停留时间控制

## 相关文档

- 安装说明: [docs/install.md](docs/install.md)
- OpenClaw 接入: [docs/openclaw.md](docs/openclaw.md)
- 架构说明: [docs/architecture.md](docs/architecture.md)
- 演示页面: [docs/presentation.html](docs/presentation.html)
- 路线图: [docs/roadmap.md](docs/roadmap.md)

## 开源发布相关

仓库已经包含基础开源发布层：

- [LICENSE](LICENSE)
- [CONTRIBUTING.md](CONTRIBUTING.md)
- [SECURITY.md](SECURITY.md)
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- `.github` 下的 issue、PR、CI 和 release workflow

## 后续方向

当前版本已经覆盖了最小可用的网关层。后续可以继续往三个方向演进：

1. 增强路由决策，加入成本、时延、设备状态和用户策略
2. 补 dashboard，把当前的决策解释能力变成可观测控制面
3. 做更完整的本地安装分发和版本发布体系
