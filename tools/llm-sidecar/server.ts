import { createServer, IncomingMessage, ServerResponse } from "node:http";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { homedir } from "node:os";

const PORT = parseInt(process.env.LLM_SIDECAR_PORT || "3100", 10);

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
}

interface ErrorResponse {
  error: string;
}

// --- pi-ai lazy loading ---

let piAiLoaded = false;
let piComplete: any;
let piGetModel: any;
let piGetOAuthApiKey: any;

async function loadPiAi() {
  if (piAiLoaded) return;
  try {
    const piAi = await import("@mariozechner/pi-ai");
    if ("registerBuiltInApiProviders" in piAi) {
      (piAi as any).registerBuiltInApiProviders();
    }
    piComplete = piAi.complete;
    piGetModel = piAi.getModel;

    try {
      const oauth = await import("@mariozechner/pi-ai/oauth");
      piGetOAuthApiKey = oauth.getOAuthApiKey;
    } catch {
      // OAuth module may not be available in all versions
    }

    piAiLoaded = true;
    console.error("[sidecar] pi-ai loaded successfully");
  } catch (err) {
    console.error("[sidecar] Failed to load pi-ai:", err);
    throw err;
  }
}

// --- Auth ---

interface AuthJson {
  [provider: string]: { type: string; [key: string]: unknown };
}

// Auth search paths, checked in order.
function getAuthPaths(): string[] {
  return [
    process.env.PI_AI_AUTH_PATH,
    join(process.cwd(), "auth.json"),
    join(process.cwd(), "..", "..", "auth.json"),
    join(homedir(), ".pi-ai", "auth.json"),
    join(homedir(), "auth.json"),
  ].filter(Boolean) as string[];
}

function findAuthJsonPath(): string | null {
  for (const p of getAuthPaths()) {
    try {
      readFileSync(p);
      return p;
    } catch {
      // not found or unreadable
    }
  }
  return null;
}

function loadAuthJson(): AuthJson | null {
  for (const p of getAuthPaths()) {
    try {
      const raw = readFileSync(p, "utf-8");
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === "object") {
        console.error(`[sidecar] Loaded auth from ${p}`);
        return parsed;
      }
    } catch {
      /* skip invalid */
    }
  }
  return null;
}

let authCredentials: AuthJson | null = null;

async function resolveApiKey(provider: string): Promise<string | null> {
  // 1. Environment variable
  const envKey = process.env[`${provider.toUpperCase()}_API_KEY`];
  if (envKey) return envKey;

  // 2. OAuth via pi-ai (map provider name, e.g. openai → openai-codex)
  const oauthProvider = OAUTH_PROVIDER_MAP[provider] ?? provider;
  if (authCredentials && piGetOAuthApiKey && authCredentials[oauthProvider]) {
    try {
      const result = await piGetOAuthApiKey(oauthProvider, authCredentials);
      if (result) {
        authCredentials[oauthProvider] = result.newCredentials;
        // Persist refreshed credentials back to auth.json
        try {
          const authPath = findAuthJsonPath();
          if (authPath) {
            writeFileSync(authPath, JSON.stringify(authCredentials, null, 2));
          }
        } catch (err) {
          console.error("[sidecar] Failed to persist refreshed credentials:", err);
        }
        return result.apiKey;
      }
    } catch (err) {
      console.error(`[sidecar] OAuth refresh failed for ${oauthProvider}:`, err);
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

interface ProviderConfig {
  provider: string; // logical name (anthropic, openai)
  model: string;
}

const PROVIDER_CHAIN: ProviderConfig[] = [
  { provider: "anthropic", model: process.env.ANTHROPIC_MODEL || "claude-sonnet-4-20250514" },
  { provider: "openai", model: process.env.OPENAI_MODEL || "gpt-5.4" },
];

// --- Request handling ---

async function handleComplete(body: CompleteRequest): Promise<CompleteResponse> {
  await loadPiAi();

  const maxTokens = body.maxTokens ?? 1024;
  const temperature = body.temperature ?? 0.0;

  // Try each provider in the chain
  const errors: string[] = [];

  for (const config of PROVIDER_CHAIN) {
    const apiKey = await resolveApiKey(config.provider);
    if (!apiKey) {
      errors.push(`${config.provider}: no API key`);
      console.error(`[sidecar] ${config.provider}: no API key found`);
      continue;
    }

    try {
      const modelId = body.model || config.model;
      // Use the OAuth provider name for getModel (e.g. "openai-codex" not "openai")
      const piProvider = OAUTH_PROVIDER_MAP[config.provider] ?? config.provider;
      console.error(`[sidecar] Trying ${piProvider}:${modelId}`);
      const model = piGetModel(piProvider, modelId);

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
        apiKey,
        maxTokens,
      };
      // openai-codex does not support the temperature parameter
      if (piProvider !== "openai-codex") {
        options.temperature = temperature;
      }

      const response = await piComplete(model, context, options);

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

      return {
        text,
        usage: {
          inputTokens: response.usage?.input ?? 0,
          outputTokens: response.usage?.output ?? 0,
        },
      };
    } catch (err: any) {
      const msg = err?.message || String(err);
      errors.push(`${config.provider}: ${msg}`);
      console.error(`[sidecar] ${config.provider} failed: ${msg}`);
      continue;
    }
  }

  throw new Error(`All providers failed: ${errors.join("; ")}`);
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
      const result = await handleComplete(body);
      const elapsed = Date.now() - start;

      console.error(
        `[sidecar] complete: ${result.usage.inputTokens}in/${result.usage.outputTokens}out ${elapsed}ms`
      );

      sendJson(res, 200, result);
    } catch (err: any) {
      console.error(`[sidecar] error:`, err?.message || err);
      sendJson(res, 500, { error: err?.message || "Unknown error" } as ErrorResponse);
    }
    return;
  }

  sendJson(res, 404, { error: "Not found" });
});

// --- Startup ---

authCredentials = loadAuthJson();

server.listen(PORT, () => {
  console.error(`[sidecar] LLM sidecar running on http://localhost:${PORT}`);
  console.error(
    `[sidecar] Auth: ${authCredentials ? "loaded" : "not found (using env vars only)"}`
  );
  console.error(
    `[sidecar] Provider chain: ${PROVIDER_CHAIN.map((p) => `${p.provider}:${p.model}`).join(" → ")}`
  );
});
