# 多 Agent 协同框架调研报告

> 调研时间：2025年7月
> 调研目标：评估主流多 Agent 协同框架，为 Hermes 项目的架构选型提供参考

---

## 目录

1. [OpenAI Swarm / Agents SDK](#1-openai-swarm--agents-sdk)
2. [AutoGen（Microsoft Agent Framework）](#2-autogenmicrosoft-agent-framework)
3. [CrewAI](#3-crewai)
4. [Semantic Kernel（Microsoft Agent Framework）](#4-semantic-kernelmicrosoft-agent-framework)
5. [TaskWeaver](#5-taskweaver)
6. [横向对比总结](#6-横向对比总结)

---

## 1. OpenAI Swarm / Agents SDK

### 简介

**Swarm** 是 OpenAI 推出的实验性多 Agent 编排框架，专注于**轻量级 Agent 协同与执行**。核心抽象只有两个原语：`Agent`（封装指令与工具）和 **Handoff**（ Agent 之间的交接机制）。目前已不再维护，官方推荐迁移至 **OpenAI Agents SDK**（生产级进化版本）。

**核心设计：**
- Agent 可随时选择将对话交接给另一个 Agent
- 完全基于 Chat Completions API，无状态设计
- 函数调用自动转换为 JSON Schema
- 支持流式响应（Streaming）

```python
from swarm import Swarm, Agent

client = Swarm()

def transfer_to_agent_b():
    return agent_b

agent_a = Agent(
    name="Agent A",
    instructions="You are a helpful agent.",
    functions=[transfer_to_agent_b],
)

agent_b = Agent(
    name="Agent B",
    instructions="Only speak in Haikus.",
)

response = client.run(
    agent=agent_a,
    messages=[{"role": "user", "content": "I want to talk to agent B."}],
)
```

### 优点

- **极简设计**：仅两个核心抽象，学习成本极低
- **高度可控**：执行流程透明，易于调试和测试
- **函数优先**：天然支持 Python 函数作为工具
- **无状态**：不保存中间状态，适合无状态场景

### 缺点

- **已停止维护**：Swarm 已废弃，需迁移到 Agents SDK
- **仅支持 OpenAI**：强绑定 OpenAI 模型
- **功能有限**：无内置记忆管理、无高级编排模式
- **实验性质**：不适合生产环境

### 适用场景

- 快速原型验证
- 教育学习（了解多 Agent 基本概念）
- 简单的 Agent 交接场景（如客服分流）

---

## 2. AutoGen（Microsoft Agent Framework）

### 简介

**AutoGen** 是微软研究院推出的多 Agent 对话框架，支持构建可自主运行或与人类协作的多 Agent 应用。目前处于**维护模式**，微软已停止新功能开发，推荐新用户迁移至 **Microsoft Agent Framework (MAF)**。

**核心特性：**
- 分层架构：Core API（底层消息传递、事件驱动）→ AgentChat API（高级编排）→ Extensions API（扩展生态）
- 支持多种对话模式：双 Agent 对话、群组聊天（Group Chat）
- 提供 AutoGen Studio（可视化无代码 GUI 搭建）
- 支持跨语言（Python + .NET）
- 集成 MCP（Model Context Protocol）服务器

```python
import asyncio
from autogen_agentchat.agents import AssistantAgent
from autogen_agentchat.tools import AgentTool
from autogen_ext.models.openai import OpenAIChatCompletionClient

async def main() -> None:
    model_client = OpenAIChatCompletionClient(model="gpt-4.1")

    math_agent = AssistantAgent(
        "math_expert",
        model_client=model_client,
        system_message="You are a math expert.",
    )
    math_agent_tool = AgentTool(math_agent, return_value_as_last_message=True)

    chemistry_agent = AssistantAgent(
        "chemistry_expert",
        model_client=model_client,
        system_message="You are a chemistry expert.",
    )
    chemistry_agent_tool = AgentTool(chemistry_agent, return_value_as_last_message=True)

    agent = AssistantAgent(
        "assistant",
        model_client=model_client,
        tools=[math_agent_tool, chemistry_agent_tool],
    )
    await Console(agent.run_stream(task="What is the integral of x^2?"))

asyncio.run(main())
```

### 优点

- **架构灵活**：分层设计，可在不同抽象层级使用
- **丰富的编排模式**：支持群组聊天、多轮对话、人类介入
- **可视化工具**：AutoGen Studio 支持无代码搭建
- **多模型支持**：OpenAI、Azure OpenAI 等
- **企业级继任者**：MAF 提供长期支持

### 缺点

- **维护模式**：原项目不再积极开发
- **学习曲线陡峭**：分层架构和配置较复杂
- **依赖较重**：安装包体积大，启动慢
- **文档混乱**：v0.2 与新版 API 差异大

### 适用场景

- 需要复杂对话模式的多 Agent 系统
- 研究和实验性质的多 Agent 编排
- 需要群组聊天和辩论模式的场景
- **新项目建议直接使用 Microsoft Agent Framework**

---

## 3. CrewAI

### 简介

**CrewAI** 是一个独立的、轻量级的多 Agent 自动化框架，**完全从零构建，不依赖 LangChain 或其他 Agent 框架**。是目前社区最活跃、增长最快的多 Agent 框架之一。

**两大核心概念：**
- **Crews（团队）**：一组具有自主决策能力的 AI Agent，通过角色协作完成任务
- **Flows（流程）**：生产级事件驱动的工作流，提供细粒度的执行控制

**关键特性：**
- YAML 配置驱动：Agent 角色、任务通过配置文件定义
- 角色扮演：每个 Agent 有 role、goal、backstory
- 任务编排：支持顺序执行、层级执行
- 内置工具生态：与 SerperDev、网页搜索等工具集成
- 企业平台：CrewAI AMP Suite 提供可观测性、安全和管理

```python
from crewai import Agent, Crew, Process, Task

researcher = Agent(
    role="Senior Data Researcher",
    goal="Uncover cutting-edge developments",
    backstory="You're a seasoned researcher...",
)

analyst = Agent(
    role="Reporting Analyst",
    goal="Create detailed reports based on research findings",
    backstory="You're a meticulous analyst...",
)

research_task = Task(
    description="Conduct research about AI agents",
    agent=researcher,
    expected_output="A list of key findings",
)

reporting_task = Task(
    description="Compile research into a report",
    agent=analyst,
    expected_output="A full report in markdown",
)

crew = Crew(
    agents=[researcher, analyst],
    tasks=[research_task, reporting_task],
    process=Process.sequential,
)
```

### 优点

- **独立轻量**：不依赖 LangChain，框架纯净
- **社区活跃**：增长最快，100,000+ 认证开发者
- **配置驱动**：YAML 配置 Agent 和任务，清晰易维护
- **Flows 支持**：生产级事件驱动编排
- **易于上手**：CLI 脚手架快速创建项目
- **企业支持**：AMP Suite 提供可观测性和管理

### 缺点

- **抽象层次较高**：定制底层行为不够灵活
- **性能开销**：Agent 间通信有一定延迟
- **调试困难**：Agent 自主决策导致不可预测
- **商业版收费**：高级功能需要 CrewAI Cloud

### 适用场景

- **内容生成和报告**：研究 + 写作的串行流程
- **自动化工作流**：需要多个专业角色协作
- **快速搭建多 Agent 原型**：脚手架快速启动
- **中小型团队**：需要快速落地 Agent 应用的场景

---

## 4. Semantic Kernel（Microsoft Agent Framework）

### 简介

**Semantic Kernel (SK)** 是微软推出的**模型无关的 AI 编排 SDK**，支持构建单个 AI Agent 和多 Agent 系统。现已演进为 **Microsoft Agent Framework (MAF)**，提供企业级支持。

**核心特性：**
- 多语言支持：Python、.NET（C#）、Java
- 模型无关：支持 OpenAI、Azure OpenAI、Hugging Face、NVIDIA、Ollama 等
- 插件生态：Native Code、Prompt Templates、OpenAPI、MCP
- 向量数据库集成：Azure AI Search、Elasticsearch、Chroma 等
- 多模态支持：文本、视觉、音频
- Process Framework：结构化业务流程建模
- 企业级：可观测性、安全、稳定 API

```python
import asyncio
from semantic_kernel.agents import ChatCompletionAgent
from semantic_kernel.connectors.ai.open_ai import AzureChatCompletion

# 创建单个 Agent
agent = ChatCompletionAgent(
    service=AzureChatCompletion(),
    name="SK-Assistant",
    instructions="You are a helpful assistant.",
)

# 多 Agent 系统 - 分诊模式
billing_agent = ChatCompletionAgent(
    service=AzureChatCompletion(),
    name="BillingAgent",
    instructions="Handle billing issues...",
)

refund_agent = ChatCompletionAgent(
    service=AzureChatCompletion(),
    name="RefundAgent",
    instructions="Assist with refund inquiries...",
)

triage_agent = ChatCompletionAgent(
    service=OpenAIChatCompletion(),
    name="TriageAgent",
    instructions="Evaluate requests and forward to specialized agents...",
    plugins=[billing_agent, refund_agent],
)

response = await triage_agent.get_response(messages="I was charged twice")
```

### 优点

- **多语言支持**（Python / .NET / Java），生态最广
- **模型无关**，可对接几乎所有主流 LLM
- **丰富的插件系统**：Native Code、OpenAPI、MCP
- **企业级设计**：可观测性、安全、稳定 API
- **Process Framework**：结构化业务流程建模
- **微软生态整合**：与 Azure 服务深度集成

### 缺点

- **学习曲线较陡**：概念多（Kernel、Plugin、Memory、Planning）
- **多 Agent 能力相对较新**：Agent 编排功能仍在迭代
- **文档分散**：SK、Agent Framework 之间文档过渡不够平滑
- **配置复杂**：对接不同模型和服务需要较多配置

### 适用场景

- **企业级应用**：需要稳定性、可观测性和安全性
- **多语言团队**：同时使用 Python 和 .NET 的组织
- **Azure 生态用户**：深度集成 Azure AI 服务
- **需要插件的复杂 Agent**：多数据源、多功能集成
- **新项目建议直接使用 Microsoft Agent Framework**

---

## 5. TaskWeaver

### 简介

**TaskWeaver** 是微软推出的**代码优先（Code-First）** 的 Agent 框架，专注于数据分析和复杂任务规划与执行。与一般 Agent 框架不同，TaskWeaver 不仅跟踪聊天历史，还**跟踪代码执行历史**（包括内存中的 DataFrame 等数据）。

**核心设计：**
- **Planner（规划器）**：将用户请求拆分为可执行的子任务
- **CodeInterpreter（代码解释器）**：执行生成的 Python 代码
- **Plugin（插件）**：封装自定义算法供编排
- **容器化执行**：默认在 Docker 容器中运行代码
- **状态化执行**：保留变量状态和中间结果

```python
# TaskWeaver 以库的形式引入
from taskweaver import TaskWeaver
import asyncio

# 或通过 CLI 启动
# python -m taskweaver -p ./project/
```

### 优点

- **代码优先**：天然适合数据分析和代码生成任务
- **状态化执行**：保留代码执行上下文和内存数据
- **复杂任务规划**：自动分解复杂请求为子任务
- **代码验证**：执行前检测代码问题
- **可视化支持**：Web UI 展示图表和生成的工件
- **插件丰富**：支持 SQL 查询、数据可视化、异常检测等

### 缺点

- **领域特定**：主要面向数据分析和代码执行场景
- **依赖 Docker**：默认需要 Docker 环境运行
- **社区规模较小**：相比 CrewAI 和 AutoGen 活跃度低
- **LLM 依赖**：规划质量取决于底层模型能力
- **通用性受限**：不适合纯对话或非代码生成场景

### 适用场景

- **数据分析自动化**：从数据库拉取数据 → 分析 → 可视化
- **报告生成**：自动生成含图表的分析报告
- **代码生成与验证**：需要安全执行代码的场景
- **数据科学工作流**：复杂数据处理和模型训练流程

---

## 6. 横向对比总结

| 维度 | OpenAI Swarm/Agents SDK | AutoGen / MAF | CrewAI | Semantic Kernel / MAF | TaskWeaver |
|------|------------------------|---------------|--------|----------------------|------------|
| **维护状态** | ❌ Swarm 已停，Agents SDK 活跃 | ⚠️ 维护模式，MAF 活跃 | ✅ 活跃 | ⚠️ 维护模式，MAF 活跃 | ✅ 活跃 |
| **学习曲线** | ⭐ 极低 | ⭐⭐⭐ 较高 | ⭐⭐ 中等 | ⭐⭐⭐ 较高 | ⭐⭐⭐ 较高 |
| **语言支持** | Python | Python + .NET | Python | Python + .NET + Java | Python |
| **模型绑定** | 仅 OpenAI | 多模型 | 多模型 | 多模型 | 多模型 |
| **核心抽象** | Agent + Handoff | AgentChat + GroupChat | Crew + Flow | Kernel + Plugin + Agent | Planner + CodeInterpreter |
| **代码执行** | ❌ | ✅ | ❌ | ❌ | ✅（容器化） |
| **可视化工具** | ❌ | AutoGen Studio | CrewAI AMP | ❌（有 Playground） | Web UI |
| **生产就绪** | ⚠️ 实验性 | ✅ | ✅ | ✅ | ⚠️ 偏向研究 |
| **社区规模** | ⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐ |
| **最佳场景** | 快速原型 | 复杂多 Agent 对话 | 角色协作自动化 | 企业级集成 | 数据分析/代码生成 |

### 选型建议

| 项目需求 | 推荐框架 |
|---------|---------|
| 快速原型验证 | OpenAI Agents SDK 或 CrewAI |
| 内容生成/报告撰写 | CrewAI |
| 复杂多 Agent 对话系统 | Microsoft Agent Framework（MAF） |
| 企业级应用（多语言、Azure 生态） | Microsoft Agent Framework（MAF） |
| 数据分析自动化 | TaskWeaver |
| 轻量级/学习入门 | OpenAI Agents SDK |
| 生产级多 Agent 编排 | CrewAI Flows 或 MAF |

### 对 Hermes 项目的参考意义

1. **CrewAI 的角色协作模式**：Role + Goal + Backstory 的设计值得借鉴，适合需要多个专业 Agent 协作的场景
2. **OpenAI Swarm 的 Handoff 机制**：轻量级 Agent 交接模式，适合简单的路由分发场景
3. **TaskWeaver 的代码优先理念**：如果 Hermes 涉及代码生成和执行，可参考其状态化执行设计
4. **Microsoft Agent Framework 的企业级设计**：可观测性、安全、稳定 API 是企业级应用的基石
5. **建议关注点**：多 Agent 通信协议（A2A、MCP）、状态管理、可观测性、插件扩展机制

---

*本报告由 ALPHA 自动生成，信息来源于各框架官方文档及 GitHub 仓库。*
