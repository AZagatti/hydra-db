import { createServer, IncomingMessage, Server, ServerResponse } from "node:http";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { homedir } from "node:os";
import { pathToFileURL } from "node:url";

const DEFAULT_PORT = 3100;
type Logger = Pick<Console, "error">;

// --- Types ---

interface CompleteRequest {
  systemPrompt: string;
  userMessage: string;
  maxTokens?: number;
  temperature?: number;
  model?: string;
}

interface CompleteResponse {
  text: string;
  usage: { inputTokens: number; outputTokens: number };
  provider: string;
  model: string;
  fallbackFrom?: string[];
}

interface ErrorResponse {
  error: string;
}

interface AuthJson {
  [provider: string]: { type: string; [key: string]: unknown };
}

export interface AuthLoadResult {
  path: string | null;
  credentials: AuthJson | null;
}

export interface PiAiDeps {
  complete: (...args: any[]) => Promise<any>;
  getModel: (...args: any[]) => unknown;
  getOAuthApiKey?: (...args: any[]) => Promise<any>;
}

type AuthSource = "env" | "oauth";

interface ResolvedApiKey {
  apiKey: string;
  source: AuthSource;
}

export interface SidecarRuntime {
  env: NodeJS.ProcessEnv;
  logger: Logger;
  authPath: string | null;
  authCredentials: AuthJson | null;
  providerChain: ProviderConfig[];
  loadPiAi: () => Promise<PiAiDeps>;
  piAi: PiAiDeps | null;
}

export interface SidecarOptions {
  auth?: AuthLoadResult;
  env?: NodeJS.ProcessEnv;
  loadPiAi?: () => Promise<PiAiDeps>;
  logger?: Logger;
  port?: number;
  providerChain?: ProviderConfig[];
}

// --- pi-ai lazy loading ---

async function defaultLoadPiAi(logger: Logger): Promise<PiAiDeps> {
  try {
    const piAi = await import("@mariozechner/pi-ai");
    if ("registerBuiltInApiProviders" in piAi) {
      (piAi as any).registerBuiltInApiProviders();
    }

    let getOAuthApiKey: PiAiDeps["getOAuthApiKey"];
    try {
      const oauth = await import("@mariozechner/pi-ai/oauth");
      getOAuthApiKey = oauth.getOAuthApiKey;
    } catch {
      // OAuth module may not be available in all versions.
    }

    logger.error("[sidecar] pi-ai loaded successfully");
    return {
      complete: piAi.complete,
      getModel: piAi.getModel,
      getOAuthApiKey,
    };
  } catch (err) {
    logger.error("[sidecar] Failed to load pi-ai:", err);
    throw err;
  }
}

async function ensurePiAi(runtime: SidecarRuntime): Promise<PiAiDeps> {
  if (runtime.piAi) {
    return runtime.piAi;
  }

  runtime.piAi = await runtime.loadPiAi();
  return runtime.piAi;
}

// --- Auth ---

// Auth search paths, checked in order.
export function getAuthPaths(env: NodeJS.ProcessEnv = process.env): string[] {
  return [
    env.PI_AI_AUTH_PATH,
    join(process.cwd(), "auth.json"),
    join(process.cwd(), "..", "..", "auth.json"),
    join(homedir(), ".pi-ai", "auth.json"),
    join(homedir(), "auth.json"),
  ].filter(Boolean) as string[];
}

export function loadAuthJson(
  env: NodeJS.ProcessEnv = process.env,
  logger: Logger = console
): AuthLoadResult {
  const explicitPath = env.PI_AI_AUTH_PATH;

  for (const p of getAuthPaths(env)) {
    try {
      const raw = readFileSync(p, "utf-8");
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === "object") {
        logger.error(`[sidecar] Loaded auth from ${p}`);
        return { path: p, credentials: parsed as AuthJson };
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      if (explicitPath && p === explicitPath) {
        throw new Error(`invalid PI_AI_AUTH_PATH ${p}: ${msg}`);
      }
      logger.error(`[sidecar] Skipping auth file ${p}: ${msg}`);
    }
  }

  return { path: null, credentials: null };
}

async function resolveApiKey(
  config: ProviderConfig,
  runtime: SidecarRuntime
): Promise<ResolvedApiKey | null> {
  // 1. Environment variable
  const envKey = runtime.env[`${config.provider.toUpperCase()}_API_KEY`];
  if (envKey) {
    return { apiKey: envKey, source: "env" };
  }

  // 2. OAuth via pi-ai when matching credentials are available.
  const authProvider = config.authProvider ?? config.provider;
  const piAi = await ensurePiAi(runtime);
  if (
    runtime.authCredentials &&
    piAi.getOAuthApiKey &&
    runtime.authCredentials[authProvider]
  ) {
    try {
      const result = await piAi.getOAuthApiKey(authProvider, runtime.authCredentials);
      if (result) {
        runtime.authCredentials = result.newCredentials;
        // Persist refreshed credentials back to auth.json
        if (runtime.authPath) {
          try {
            writeFileSync(runtime.authPath, JSON.stringify(runtime.authCredentials, null, 2));
          } catch (err) {
            runtime.logger.error("[sidecar] Failed to persist refreshed credentials:", err);
          }
        }
        return { apiKey: result.apiKey, source: "oauth" };
      }
    } catch (err) {
      runtime.logger.error(`[sidecar] OAuth refresh failed for ${authProvider}:`, err);
    }
  }

  return null;
}

// --- Provider config ---

// pi-ai uses "openai-codex" as the provider name for OpenAI OAuth,
// but "openai" for API key auth. This maps the logical provider to
// the pi-ai provider name used for OAuth and getModel().
const OAUTH_PROVIDER_MAP: Record<string, string> = {
  anthropic: "anthropic",
  openai: "openai-codex",
};

export interface ProviderConfig {
  provider: string; // logical name (anthropic, openai)
  authProvider?: string;
  modelProvider?: string;
  model: string;
}

export function getProviderChain(env: NodeJS.ProcessEnv = process.env): ProviderConfig[] {
  return [
    {
      provider: "anthropic",
      authProvider: "anthropic",
      modelProvider: "anthropic",
      model: env.ANTHROPIC_MODEL || "claude-sonnet-4-20250514",
    },
    {
      provider: "openai",
      authProvider: "openai-codex",
      modelProvider: "openai",
      model: env.OPENAI_MODEL || "gpt-5.4",
    },
  ];
}

// --- Request handling ---

async function handleComplete(
  body: CompleteRequest,
  runtime: SidecarRuntime
): Promise<CompleteResponse> {
  const maxTokens = body.maxTokens ?? 1024;
  const temperature = body.temperature ?? 0.0;
  const fallbackFrom: string[] = [];

  for (const config of runtime.providerChain) {
    const resolved = await resolveApiKey(config, runtime);
    if (!resolved) {
      fallbackFrom.push(`${config.provider}:not-configured`);
      runtime.logger.error(`[sidecar] ${config.provider}: no API key found`);
      continue;
    }

    try {
      const piAi = await ensurePiAi(runtime);
      const modelId = body.model || config.model;
      const modelProvider =
        resolved.source === "oauth"
          ? (config.authProvider ?? config.provider)
          : (config.modelProvider ?? config.provider);

      runtime.logger.error(`[sidecar] Trying ${modelProvider}:${modelId}`);
      const model = piAi.getModel(modelProvider, modelId);

      const context = {
        systemPrompt: body.systemPrompt,
        messages: [
          {
            role: "user" as const,
            content: body.userMessage,
            timestamp: Date.now(),
          },
        ],
      };

      const options: Record<string, unknown> = {
        apiKey: resolved.apiKey,
        maxTokens,
      };
      // openai-codex does not support the temperature parameter.
      if (modelProvider !== "openai-codex") {
        options.temperature = temperature;
      }

      const response = await piAi.complete(model, context, options);

      if (response.errorMessage) {
        throw new Error(response.errorMessage);
      }

      // Extract text from response
      let text = "";
      if (response.content) {
        for (const block of response.content) {
          if (block.type === "text" && block.text) {
            text += block.text;
          }
        }
      }

      const result: CompleteResponse = {
        text,
        usage: {
          inputTokens: response.usage?.input ?? 0,
          outputTokens: response.usage?.output ?? 0,
        },
        provider: config.provider,
        model: modelId,
      };

      if (fallbackFrom.length > 0) {
        result.fallbackFrom = fallbackFrom;
      }

      return result;
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      throw new Error(`provider ${config.provider} failed: ${msg}`);
    }
  }

  throw new Error("no configured providers available");
}

// --- HTTP server ---

function readBody(req: IncomingMessage): Promise<string> {
  return new Promise((resolve, reject) => {
    let size = 0;
    const maxSize = 1024 * 1024; // 1 MB
    const chunks: Buffer[] = [];
    req.on("data", (chunk) => {
      size += chunk.length;
      if (size > maxSize) {
        req.destroy();
        reject(new Error("Request body too large"));
        return;
      }
      chunks.push(chunk);
    });
    req.on("end", () => resolve(Buffer.concat(chunks).toString()));
    req.on("error", reject);
  });
}

function sendJson(res: ServerResponse, status: number, data: unknown) {
  const body = JSON.stringify(data);
  res.writeHead(status, {
    "Content-Type": "application/json",
    "Content-Length": Buffer.byteLength(body),
  });
  res.end(body);
}

export function createSidecarServer(options: SidecarOptions = {}): {
  runtime: SidecarRuntime;
  server: Server;
} {
  const env = options.env ?? process.env;
  const logger = options.logger ?? console;
  const auth = options.auth ?? loadAuthJson(env, logger);

  const runtime: SidecarRuntime = {
    env,
    logger,
    authPath: auth.path,
    authCredentials: auth.credentials,
    providerChain: options.providerChain ?? getProviderChain(env),
    loadPiAi: options.loadPiAi ?? (() => defaultLoadPiAi(logger)),
    piAi: null,
  };

  const server = createServer(async (req, res) => {
    const url = req.url || "/";
    const method = req.method || "GET";

    if (url === "/health" && (method === "GET" || method === "POST")) {
      sendJson(res, 200, { status: "ok" });
      return;
    }

    if (url === "/complete" && method === "POST") {
      try {
        const raw = await readBody(req);

        let body: CompleteRequest;
        try {
          body = JSON.parse(raw);
        } catch {
          sendJson(res, 400, { error: "Invalid JSON" } as ErrorResponse);
          return;
        }

        if (!body.systemPrompt || !body.userMessage) {
          sendJson(res, 400, {
            error: "systemPrompt and userMessage are required",
          } as ErrorResponse);
          return;
        }

        const start = Date.now();
        const result = await handleComplete(body, runtime);
        const elapsed = Date.now() - start;

        logger.error(
          `[sidecar] complete: ${result.provider}:${result.model} ${result.usage.inputTokens}in/${result.usage.outputTokens}out ${elapsed}ms`
        );

        sendJson(res, 200, result);
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Unknown error";
        logger.error("[sidecar] error:", msg);
        sendJson(res, 500, { error: msg } as ErrorResponse);
      }
      return;
    }

    sendJson(res, 404, { error: "Not found" });
  });

  return { runtime, server };
}

export function startSidecar(options: SidecarOptions = {}): Server {
  const { runtime, server } = createSidecarServer(options);
  const port = options.port ?? parseInt(runtime.env.LLM_SIDECAR_PORT || `${DEFAULT_PORT}`, 10);

  server.listen(port, () => {
    runtime.logger.error(`[sidecar] LLM sidecar running on http://localhost:${port}`);
    runtime.logger.error(
      `[sidecar] Auth: ${runtime.authCredentials ? "loaded" : "not found (using env vars only)"}`
    );
    runtime.logger.error(
      `[sidecar] Provider chain: ${runtime.providerChain
        .map((p) => `${p.provider}:${p.model}`)
        .join(" -> ")}`
    );
  });

  return server;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  startSidecar();
}
