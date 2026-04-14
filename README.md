# 🎬 VideoMax — 基于 Multi-Agent 的 AI 视频生成系统

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white" />
  <img src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=white" />
  <img src="https://img.shields.io/badge/Kafka-7.4-231F20?logo=apachekafka&logoColor=white" />
  <img src="https://img.shields.io/badge/MySQL-8.0-4479A1?logo=mysql&logoColor=white" />
  <img src="https://img.shields.io/badge/License-MIT-green" />
</p>

VideoMax 是一个全栈 AI 视频生成平台，核心采用 **5-Agent 多智能体协作系统（MAS）**，将用户的文字创意与参考图片自动转化为高质量视频。系统实现了 **CoT 推理、ReAct 工具调用、Reflection 自纠正** 等主流 Agent 设计范式，并通过 Kafka 消息队列实现异步任务编排。平台内置用户认证、历史记录管理和 OSS 持久化存储。

---

## ✨ 核心特性

- 🤖 **5-Agent 协作流水线** — Story / Character / Storyboard / Visual / Critic，Blackboard 模式共享上下文
- 🔄 **ReAct 推理循环** — Visual Agent 通过 Function Calling 动态调用工具，最多 5 轮 Thought→Action→Observation
- 🪞 **Reflection 自纠正** — Critic Agent 5 维结构化审计，不通过则携带反馈重试（最多 3 次）
- 👁️ **多模态 Vision** — Character Agent 利用 GPT-4o Vision 从参考图提取角色特征
- 🔌 **双 LLM 供应商** — 统一接口适配 OpenAI（Chat Completions）与豆包（Responses API）
- 🎥 **三视频供应商** — 工厂模式抽象字节 Seedance、可灵 Kling 与腾讯混元，支持多模型并存配置
- 📡 **SSE 实时推送** — Server-Sent Events 流式推送每个 Agent 的执行阶段与进度
- ⚡ **Kafka 异步编排** — 请求即返回 TaskID，后台消费者驱动 MAS 流水线 + 视频生成
- 🔐 **JWT 用户认证** — 注册/登录鉴权，bcrypt 密码哈希，受保护路由自动校验 Token
- 📋 **历史记录 & 统计** — 分页查询任务列表、按模型维度统计生成数据
- ☁️ **阿里云 OSS 存储** — 视频流式上传 OSS，支持自定义 CDN 域名（可选启用）

---

## 🏗️ 系统架构

```
┌──────────────────────────────────────────────────────────────────────┐
│                         Frontend (React 19 + Vite)                  │
│   Login/Register → Navbar → CreateForm → SSE ProgressView → History │
└────────────────────────────┬─────────────────────────────────────────┘
                             │ REST + SSE (JWT Bearer Token)
┌────────────────────────────▼─────────────────────────────────────────┐
│                         API Layer (Gin)                              │
│  POST /api/auth/register  │  POST /api/auth/login  (公开路由)        │
│  POST /api/video  │  GET /api/task/:id  │  SSE /api/events  (JWT)   │
│  GET /api/tasks  │  GET /api/stats                        (JWT)     │
└──────┬───────────────────────────────────────────────────────────────┘
       │ Kafka Produce                        ▲ Status Update
┌──────▼──────────────────────────────────────┴────────────────────────┐
│                     Kafka Message Queue                              │
└──────┬───────────────────────────────────────────────────────────────┘
       │ Kafka Consume
┌──────▼───────────────────────────────────────────────────────────────┐
│                  MAS Orchestrator (Blackboard Pattern)               │
│                                                                      │
│  ┌──────────┐   ┌──────────────┐   ┌──────────────┐                │
│  │  Story    │──▶│  Character   │──▶│  Storyboard  │                │
│  │  Agent    │   │  Agent       │   │  Agent       │                │
│  │  (CoT)    │   │  (Vision)    │   │  (Director)  │                │
│  └──────────┘   └──────────────┘   └──────┬───────┘                │
│                                           │                         │
│                                    ┌──────▼───────┐                 │
│                                    │  Visual Agent │◀──┐            │
│                                    │  (ReAct+Tool) │   │ Feedback   │
│                                    └──────┬───────┘   │            │
│                                           │           │            │
│                                    ┌──────▼───────┐   │            │
│                                    │ Critic Agent  │───┘            │
│                                    │ (Reflection)  │                │
│                                    └──────┬───────┘                │
│                                           │ APPROVED                │
└───────────────────────────────────────────┼─────────────────────────┘
                                            │
┌───────────────────────────────────────────▼─────────────────────────┐
│       Video Provider (Bytedance Seedance / Kling / Hunyuan)        │
│                  Submit → Poll Status → Get Video URL               │
│                         │ (oss.enabled=true)                        │
│                Stream Upload to Aliyun OSS / CDN                   │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 🧠 Multi-Agent 设计详解

### Agent 协作流程

| 阶段 | Agent | 推理范式 | 输入 | 输出 |
|------|-------|---------|------|------|
| 1 | **Story Agent** | Chain of Thought | 用户创意 + 参考图 | 2-3 句叙事梗概（起承转） |
| 2 | **Character Agent** | Vision Analysis | 故事线 + 参考图 | 角色锚点卡（外貌/服装/情绪） |
| 3 | **Storyboard Agent** | Director Pattern | 故事线 + 角色 | 分镜表（Shot 1-4，含时间轴） |
| 4 | **Visual Agent** | ReAct Loop | 分镜 + 角色 + 宽高比 | 统一英文视频 Prompt（≤1000 chars） |
| 5 | **Critic Agent** | Reflection | 全部产出 | APPROVED / REJECTED + 修改建议 |

### Blackboard 共享上下文

所有 Agent 通过 `MASContext` 黑板结构读写数据，实现松耦合协作：

```go
type MASContext struct {
    TaskID, UserIdea, Images, AspectRatio   // 输入
    Storyline                                // Story Agent →
    Characters                               // Character Agent →
    SceneList                                // Storyboard Agent →
    FinalPrompts                             // Visual Agent →
    ReviewFeedback, ReviewPassed             // Critic Agent →
}
```

### Visual ↔ Critic 质检循环

```
Visual Agent 生成 Prompt
        ↓
Critic Agent 5 维审计:
  ① 角色一致性  ② 动作连贯性  ③ 镜头与风格
  ④ 参数合规性  ⑤ 整体效率（≤1000 chars）
        ↓
  APPROVED → 提交视频生成
  REJECTED → 携带具体修改建议 → Visual Agent 重试（最多 3 次）
```

### ReAct 工具调用（Visual Agent）

Visual Agent 在构建 Prompt 时，可通过 Function Calling 动态查询平台最佳实践：

```
Thought: "我需要了解字节 Seedance 的分辨率规格"
Action:  search_best_practices(provider="bytedance", category="resolution")
Observation: "1920x1080 (16:9) 或 1080x1920 (9:16)，支持最高 4K"
Thought: "现在我可以按规格构建 Prompt 了"
→ 输出最终 Prompt
```

---

## 🛠️ 技术栈

### 后端
| 组件 | 技术 | 版本 |
|------|------|------|
| 语言 | Go | 1.26 |
| Web 框架 | Gin | 1.12 |
| ORM | GORM | 1.31 |
| 数据库 | MySQL | 8.0 |
| 消息队列 | Apache Kafka | 7.4 |
| 日志 | Uber Zap + Lumberjack | 1.27 |
| LLM SDK | go-openai | 1.41 |
| Kafka SDK | IBM Sarama | 1.47 |
| JWT | golang-jwt/jwt | v5 |
| 密码哈希 | golang.org/x/crypto bcrypt | — |
| OSS SDK | aliyun-oss-go-sdk | — |

### 前端
| 组件 | 技术 | 版本 |
|------|------|------|
| 框架 | React | 19 |
| 构建工具 | Vite | 8.0 |
| 路由 | React Router | v7 |
| 样式 | TailwindCSS | 4.2 |
| 图标 | Lucide React | 1.8 |

### 基础设施
| 组件 | 技术 |
|------|------|
| 容器编排 | Docker Compose |
| 数据库 | MySQL 8.0 |
| 消息队列 | Confluent Kafka 7.4 + Zookeeper |

---

## 📁 项目结构

```
videoMax/
├── cmd/api/main.go                    # 入口：初始化流程
├── internal/
│   ├── api/
│   │   ├── router.go                  # 路由注册（7 个端点，含公开与受保护）
│   │   ├── handler/
│   │   │   ├── video_handler.go       # 创建任务 + 查询状态
│   │   │   ├── sse_handler.go         # SSE 实时事件推送
│   │   │   ├── auth_handler.go        # 用户注册 + 登录（JWT）
│   │   │   └── history_handler.go     # 历史记录列表 + 使用统计
│   │   └── middleware/
│   │       ├── cors.go                # CORS 中间件
│   │       └── auth.go                # JWT 鉴权中间件
│   ├── domain/
│   │   ├── entity/
│   │   │   ├── task_entity.go         # Task 实体（9 种状态）
│   │   │   └── user_entity.go         # User 实体（UUID + bcrypt 密码哈希）
│   │   └── dto/
│   │       ├── video_req_res.go       # 视频请求/响应 DTO
│   │       └── auth_req_res.go        # 注册/登录请求/响应 DTO
│   ├── mas/                           # ★ 多智能体系统核心
│   │   ├── protocol/agent_interface.go # Agent 统一接口
│   │   ├── orchestrator.go            # 编排器（流水线 + 质检循环）
│   │   ├── event.go                   # SSE EventEmitter（发布/订阅）
│   │   └── agents/                    # 5 个 Agent 实现
│   │       ├── story_agent.go         # CoT 推理
│   │       ├── character_agent.go     # Vision 图像分析
│   │       ├── storyboard_agent.go    # 导演分镜
│   │       ├── visual_agent.go        # ReAct + Function Calling
│   │       └── critic_agent.go        # Reflection 质检
│   ├── tools/
│   │   ├── ai_tool_interface.go       # AITool 接口（Name/Desc/Schema/Execute）
│   │   └── preset_search.go           # 平台最佳实践查询工具
│   ├── queue/
│   │   ├── producer.go                # Kafka 生产者
│   │   └── consumer.go                # Kafka 消费者（驱动 MAS）
│   ├── repository/
│   │   ├── task_repo.go               # TaskRepository 接口（含 ListByUserID / GetUserStats）
│   │   ├── task_repo_mysql.go         # MySQL 实现
│   │   ├── user_repo.go               # UserRepository 接口
│   │   └── user_repo_mysql.go         # MySQL 实现
│   └── video/
│       ├── provider_interface.go      # VideoProvider 接口
│       ├── factory.go                 # 供应商工厂（多模型并存）
│       ├── bytedance_client.go        # 字节 Seedance 客户端
│       ├── kling_client.go            # 可灵 Kling 客户端
│       └── hunyuan_client.go          # 腾讯混元 客户端
├── pkg/
│   ├── config/parser.go               # YAML 配置解析
│   ├── kafka/conn.go                  # Kafka 连接工具
│   ├── llmclient/
│   │   ├── llm_interface.go           # LLMClient 统一接口
│   │   ├── openai_client.go           # OpenAI / 兼容 API
│   │   └── doubao_client.go           # 豆包 Responses API
│   ├── oss/uploader.go                # 阿里云 OSS 流式上传
│   └── logger/zap_logger.go           # 结构化日志
├── frontend/                          # React SPA
│   └── src/
│       ├── components/
│       │   ├── auth/LoginForm.jsx     # 登录页
│       │   ├── auth/RegisterForm.jsx  # 注册页
│       │   ├── history/HistoryPage.jsx # 历史记录页
│       │   ├── CreateForm.jsx         # 视频创建表单
│       │   └── ProgressView.jsx       # SSE 进度视图
│       └── services/
│           ├── api.js                 # Axios 实例（自动注入 Bearer Token）
│           ├── authService.js         # 登录/注册/本地存储封装
│           └── historyService.js      # 历史记录 API 调用
├── configs/
│   ├── config.example.yaml            # 配置模板
│   └── config.yaml                    # 实际配置（gitignore）
└── docker-compose.yml                 # MySQL + Kafka + Zookeeper
```

---

## 🚀 快速开始

### 前置条件

- Go 1.26+
- Node.js 18+ & pnpm
- Docker & Docker Compose

### 1. 启动基础设施

```bash
docker-compose up -d
```

等待 MySQL 和 Kafka 就绪（约 15-30 秒）。

### 2. 配置

```bash
cp configs/config.example.yaml configs/config.yaml
```

编辑 `configs/config.yaml`，填入你的 API Key：

```yaml
llm:
  provider: "doubao"                     # 或 "openai"
  api_key: "your-api-key"
  model: "your-model-endpoint-id"

video:
  providers:
    - name: "doubao-seedance-1-0-pro-250528"
      provider: "bytedance"
      api_key: "your-ark-api-key"
    - name: "kling-v1-6"
      provider: "kling"
      api_key: "your-access-key:your-secret-key"
    - name: "hunyuan-video"
      provider: "hunyuan"
      api_key: "your-secret-id:your-secret-key"

jwt:
  secret: "change-this-to-a-strong-random-secret"
  expire_days: 7

oss:
  enabled: false                         # 启用后视频上传至阿里云 OSS
  endpoint: "oss-cn-hangzhou.aliyuncs.com"
  access_key_id: ""
  access_key_secret: ""
  bucket: ""
  base_url: ""                           # 可选：自定义 CDN 域名
```

<details>
<summary>📋 支持的供应商配置</summary>

**LLM 供应商：**
| 供应商 | provider | api_key | model | base_url |
|--------|----------|---------|-------|----------|
| OpenAI | `openai` | OpenAI API Key | `gpt-4o` | 留空（默认） |
| 豆包 | `doubao` | 火山引擎 ARK Key | 推理接入点 ID | 留空（默认） |

**视频供应商（可同时配置多个）：**
| 供应商 | provider | api_key | name（模型标识） |
|--------|----------|---------|-----------------|
| 字节 Seedance | `bytedance` | ARK API Key | `doubao-seedance-1-0-pro-250528` 等 |
| 可灵 Kling | `kling` | `access_key:secret_key` | `kling-v1-6` / `kling-v2-6` / `kling-v3` |
| 腾讯混元 | `hunyuan` | `secret_id:secret_key` | `hunyuan-video` 等 |

</details>

### 3. 启动后端

```bash
go run cmd/api/main.go
```

服务默认监听 `http://localhost:8080`。

### 4. 启动前端

```bash
cd frontend
pnpm install
pnpm dev
```

前端默认运行在 `http://localhost:5173`，API 请求自动代理到后端。

### 5. 使用

1. 打开浏览器访问 `http://localhost:5173`
2. 注册账号或登录（邮箱 + 密码）
3. 输入视频创意描述（支持中文）
4. 可选：上传参考图片（拖拽或点击）
5. 选择画面比例（16:9 / 9:16 / 1:1 等）及视频模型
6. 点击提交，实时查看 5 个 Agent 的执行进度
7. 视频生成完成后，在线预览或下载
8. 通过顶部导航"历史记录"查看所有生成任务及统计数据

---

## 📡 API 接口

### 认证接口（公开）

| 方法 | 路径 | 说明 | 请求格式 |
|------|------|------|----------|
| `POST` | `/api/auth/register` | 用户注册 | `application/json` |
| `POST` | `/api/auth/login` | 用户登录 | `application/json` |

### 业务接口（需 JWT）

| 方法 | 路径 | 说明 | 请求格式 |
|------|------|------|----------|
| `POST` | `/api/video` | 创建视频生成任务 | `multipart/form-data` |
| `GET` | `/api/task/:id` | 查询任务状态 | — |
| `GET` | `/api/events/:taskId` | SSE 实时事件流 | — |
| `GET` | `/api/tasks` | 当前用户任务历史（分页） | `?page=1&page_size=10` |
| `GET` | `/api/stats` | 当前用户使用统计 | — |

### 注册

```bash
curl -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","email":"alice@example.com","password":"secret123"}'
```

**响应：**
```json
{
  "code": 0,
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user_id": "...",
  "username": "alice",
  "email": "alice@example.com",
  "msg": "注册成功"
}
```

### 创建任务

```bash
curl -X POST http://localhost:8080/api/video \
  -H "Authorization: Bearer <your-token>" \
  -F "idea=一个女孩在咖啡馆遇见老朋友，从惊讶到拥抱" \
  -F "aspect_ratio=16:9" \
  -F "images=@reference.jpg"
```

**响应：**
```json
{
  "task_id": "a55fad7c-06e1-4ba2-83e8-f60667eb5ea8"
}
```

### 查询状态

```bash
curl http://localhost:8080/api/task/a55fad7c-06e1-4ba2-83e8-f60667eb5ea8
```

**响应：**
```json
{
  "id": "a55fad7c-06e1-4ba2-83e8-f60667eb5ea8",
  "status": "success",
  "video_url": "https://..."
}
```

### 任务状态机

```
pending → phase_story → phase_char → phase_board → phase_visual → phase_review → generating → success
                                                                                            → failed
```

---

## ⚙️ 设计模式

| 模式 | 应用场景 |
|------|---------|
| **Blackboard Pattern** | MASContext 作为共享黑板，Agent 间松耦合协作 |
| **Chain of Thought** | Story Agent 3 步强制推理链 |
| **ReAct** | Visual Agent 推理 + 工具调用循环 |
| **Reflection** | Critic Agent 审计 + 反馈驱动重试 |
| **Factory Pattern** | LLM 客户端 / 视频供应商的创建 |
| **Strategy Pattern** | 可插拔的 LLM 与视频供应商实现 |
| **Repository Pattern** | 数据访问层接口与实现分离 |
| **Observer (Pub/Sub)** | SSE EventEmitter 事件发布/订阅 |
| **Middleware Chain** | JWT 鉴权中间件 + CORS 中间件 |

---

## 📄 License

MIT
