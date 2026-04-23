# Philosophy

## Why Hydra exists

Most backend frameworks were designed for a world where humans make requests and servers respond. That model is fundamentally misaligned with how AI-native software works.

In an agent-native system, the primary actors are not humans clicking buttons. They are autonomous agents executing multi-step plans, calling tools, reading and writing memory, collaborating with other agents, and making decisions that cascade across time. A traditional REST API framework -- no matter how well-designed -- is the wrong abstraction for this. It would be like using a form handler to build an operating system.

Hydra exists because the gap between "web framework" and "agent runtime" is not a feature difference. It is an architectural difference. Hydra is not Express with agent features bolted on. It is a new category of infrastructure.

## The organism metaphor

The name is not cosmetic. Hydra's architecture mirrors the biological metaphor:

- **Heads are specialized subsystems.** Each head does one thing well. The gateway routes. The agent runtime executes. The memory plane stores. The policy engine enforces. No head tries to do another head's job.
- **The body is shared.** All heads share identity, tracing, configuration, event bus, and health. This is what makes Hydra one organism rather than six microservices that happen to talk to each other.
- **Heads can evolve independently.** You can rewrite the memory plane without touching the gateway. You can add a new adapter without modifying the execution engine. The organism adapts.
- **The organism survives head loss.** A failing head degrades the system but does not kill it. The body keeps running. Other heads keep serving.

This metaphor imposes discipline. When you are tempted to put logic in the wrong place, ask: "which head does this belong to?" When you are tempted to share mutable state across heads, ask: "does this belong in the body?" The architecture has opinions.

## What makes Hydra different

### vs. Web frameworks (Express, FastAPI, Gin, Echo)

Web frameworks handle HTTP requests. They route, parse, validate, and respond. Hydra does this too, but it treats HTTP as just one input channel among many. A Slack message, a CLI command, a webhook, a scheduled event -- all are first-class inputs. The gateway normalizes everything into the same Envelope, and the rest of the system never knows (or cares) where the request came from.

More importantly, web frameworks assume the request-response cycle is the unit of work. Hydra assumes the unit of work is an agent execution -- which may span seconds, minutes, or days, may involve subagents, may call tools, may read and write memory, and may be interrupted and resumed. This is a fundamentally different execution model.

### vs. Orchestration platforms (Temporal, Inngest, BullMQ)

Orchestration platforms manage workflows. They excel at durable execution, retries, and task graphs. Hydra has an execution plane that does all of this, but it wraps it in an agent-native context: workflows are not abstract DAGs, they are agent execution plans with identity, policy, memory, and audit trailing built in.

In a pure orchestration platform, a workflow is a function. In Hydra, a workflow is something an agent does -- with permissions, budget limits, memory context, and full observability. The execution plane serves the agent, not the other way around.

### vs. Agent frameworks (LangChain, CrewAI, AutoGen)

Agent frameworks manage LLM interactions. They provide chain-of-thought, tool calling, prompt templates, and model routing. Hydra does not compete with these. Hydra sits below them.

An agent framework decides *how an agent thinks*. Hydra decides *how an agent operates* -- what it is allowed to do, what resources it can access, what happens when it fails, who is responsible for its actions, and what gets remembered. Hydra provides the execution environment, the policy boundaries, the memory substrate, and the audit trail. You could run LangChain inside Hydra's agent runtime head.

The key difference: agent frameworks trust the prompt. Hydra does not.

### vs. Microservice architectures

Microservices distribute a system across processes and networks. Hydra starts as a modular monolith -- one binary, one process, zero network overhead between components. This is deliberate. The cost of microservices (serialization, network calls, distributed state, deployment complexity) is only justified at scale. Hydra's `Head` interface defines clean boundaries so that when scale demands it, any head can be extracted into a separate service. But you don't pay that cost until you need to.

## Core beliefs

These are the architectural convictions that shape every decision in Hydra:

**Backend-enforced policy, not prompt-only policy.**
Relying on LLM prompts for safety is like relying on a note on the door for building security. Prompts can be ignored, circumvented, or misunderstood by the model. Hydra enforces permissions, budgets, tool access, and audit logging at the infrastructure layer. The agent cannot bypass policy because policy is enforced before and after every action, regardless of what the agent "decides" to do.

**Agent-native, not chatbot-native.**
Chatbots respond to messages. Agents execute plans. Hydra is designed for the latter: autonomous, multi-step, tool-using, memory-aware execution that may run for seconds or days. Chat is just one adapter.

**Structured context, not prompt sludge.**
Feeding 50KB of unstructured context into a prompt is not an architecture. Hydra uses typed data structures -- Identity, Envelope, AgentContext, Memory -- that can be validated, filtered, audited, and versioned. Agents receive structured context, not text dumps.

**Memory-aware, but memory-skeptical.**
Agents need memory to be useful across sessions. But memory is dangerous: it can be poisoned, it can become stale, it can contain contradictions. Hydra stores memory with confidence scores, timestamps, and type tags. Retrieval is deliberate, not automatic. The system is designed to question what it remembers.

**Observability is not optional.**
If you cannot trace an agent's actions from request to response, you cannot debug it, audit it, or trust it. Hydra traces every action with correlation IDs, spans, and audit entries. Observability is built into the body, not bolted on as an afterthought.

**Human override always exists.**
No matter how autonomous the system becomes, a human must always be able to intercept, inspect, and override any agent action. This is not a feature. It is a requirement.

**Graceful degradation.**
The organism survives head loss. If the memory plane goes down, agents still execute (without memory). If an adapter fails, other channels keep working. The body keeps the organism alive even when heads are damaged.

**The architecture should earn its complexity.**
Every abstraction in Hydra exists because it solves a real problem. The head/body split is not over-engineering -- it enforces separation of concerns. The Envelope is not ceremony -- it normalizes heterogeneous inputs into a common shape. The policy engine is not overhead -- it is the difference between a system you can trust and one you cannot.

## The stack context

Hydra exists inside a broader architecture:

- **Yell** defines the interface layer. It is a declarative UI/expression surface designed for AI-native interface generation.
- **Hydra** is the runtime backbone. It receives, routes, orchestrates, executes, enforces, and remembers.
- **TardigradeDB** is the persistent memory engine. It stores and retrieves memory in a form suited for agent use.

The slogan captures the relationship:

> *"Yell draws the interface, Hydra operates the organism, TardigradeDB remembers the trauma."*

Each layer is independent but designed to compose. Hydra can run without Yell or TardigradeDB. But when all three are present, the system becomes a complete agent-native platform: interface, execution, and memory.

## What Hydra is not

- Hydra is not an LLM framework. It does not manage prompts, tokens, or model routing.
- Hydra is not a database. It delegates persistence to providers like TardigradeDB.
- Hydra is not an agent SDK. It is the runtime environment where agents operate.
- Hydra is not a chatbot builder. Chat is one channel, not the architecture.
- Hydra is not a microservice chassis. It is a modular monolith that can become distributed when needed.

Hydra is the operating backend for agent-native software. It is the infrastructure layer between "an agent wants to do something" and "something was done, correctly, safely, observably."
