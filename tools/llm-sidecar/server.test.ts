import assert from "node:assert/strict";
import { once } from "node:events";
import { AddressInfo } from "node:net";
import test from "node:test";

import { createSidecarServer, type SidecarOptions } from "./server";

const logger = {
  error() {
    // Silence test logs.
  },
};

async function startTestSidecar(options: SidecarOptions = {}) {
  const { server } = createSidecarServer({ logger, ...options });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");

  const address = server.address() as AddressInfo;
  return {
    baseUrl: `http://127.0.0.1:${address.port}`,
    async close() {
      server.close();
      await once(server, "close");
    },
  };
}

test("returns 400 for invalid JSON", async () => {
  const { baseUrl, close } = await startTestSidecar();

  try {
    const res = await fetch(`${baseUrl}/complete`, {
      method: "POST",
      body: "{invalid",
    });

    assert.equal(res.status, 400);
    assert.deepEqual(await res.json(), { error: "Invalid JSON" });
  } finally {
    await close();
  }
});

test("uses openai model registry for OPENAI_API_KEY requests", async () => {
  const modelCalls: string[] = [];
  const { baseUrl, close } = await startTestSidecar({
    auth: {
      path: null,
      credentials: {
        "openai-codex": { type: "oauth" },
      },
    },
    env: {
      OPENAI_API_KEY: "sk-openai",
    } as NodeJS.ProcessEnv,
    loadPiAi: async () => ({
      getModel(provider, model) {
        modelCalls.push(`${provider}:${model}`);
        return { provider, model };
      },
      async complete() {
        return {
          content: [{ type: "text", text: "ok" }],
          usage: { input: 8, output: 2 },
        };
      },
    }),
  });

  try {
    const res = await fetch(`${baseUrl}/complete`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        systemPrompt: "You are helpful.",
        userMessage: "Say hi.",
      }),
    });

    assert.equal(res.status, 200);
    assert.deepEqual(await res.json(), {
      text: "ok",
      usage: { inputTokens: 8, outputTokens: 2 },
      provider: "openai",
      model: "gpt-5.4",
      fallbackFrom: ["anthropic:not-configured"],
    });
    assert.deepEqual(modelCalls, ["openai:gpt-5.4"]);
  } finally {
    await close();
  }
});

test("fails fast on configured provider runtime errors", async () => {
  const calls: string[] = [];
  const { baseUrl, close } = await startTestSidecar({
    env: {
      ANTHROPIC_API_KEY: "anthropic-key",
      OPENAI_API_KEY: "openai-key",
    } as NodeJS.ProcessEnv,
    loadPiAi: async () => ({
      getModel(provider, model) {
        calls.push(`model:${provider}:${model}`);
        return { provider, model };
      },
      async complete(model: unknown) {
        const resolvedModel = model as { provider: string };
        calls.push(`complete:${resolvedModel.provider}`);
        if (resolvedModel.provider === "anthropic") {
          throw new Error("provider down");
        }

        return {
          content: [{ type: "text", text: "unexpected fallback" }],
          usage: { input: 1, output: 1 },
        };
      },
    }),
  });

  try {
    const res = await fetch(`${baseUrl}/complete`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        systemPrompt: "You are helpful.",
        userMessage: "Say hi.",
      }),
    });

    assert.equal(res.status, 500);
    assert.deepEqual(await res.json(), {
      error: "provider anthropic failed: provider down",
    });
    assert.deepEqual(calls, [
      "model:anthropic:claude-sonnet-4-20250514",
      "complete:anthropic",
    ]);
  } finally {
    await close();
  }
});
