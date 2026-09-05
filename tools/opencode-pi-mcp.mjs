#!/usr/bin/env node

/**
 * A tiny MCP stdio bridge that exposes Pi as an OpenCode tool.
 *
 * The bridge deliberately starts a fresh, non-persistent Pi RPC session for
 * each call. This keeps the OpenCode session as the source of truth while
 * still letting Pi use its normal tools and credentials.
 */

import { spawn } from "node:child_process";
import { createInterface } from "node:readline";

const PROTOCOL_VERSION = "2024-11-05";
const SERVER_NAME = "opencode-pi-bridge";
const SERVER_VERSION = "1.0.0";
const TOOL_NAME = "pi_delegate";
const DEFAULT_TIMEOUT_MS = 10 * 60 * 1000;
const MAX_PROMPT_LENGTH = 100_000;
const MAX_MODEL_LENGTH = 200;
const PI_COMMAND = process.env.PI_BIN || "pi";

function send(message) {
  process.stdout.write(`${JSON.stringify(message)}\n`);
}

function result(id, value) {
  send({ jsonrpc: "2.0", id, result: value });
}

function error(id, code, message) {
  send({
    jsonrpc: "2.0",
    id,
    error: { code, message },
  });
}

function textResult(text, isError = false) {
  return {
    content: [{ type: "text", text }],
    ...(isError ? { isError: true } : {}),
  };
}

function validateArguments(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("arguments must be an object");
  }

  const prompt = value.prompt;
  if (typeof prompt !== "string" || prompt.trim().length === 0) {
    throw new Error("prompt must be a non-empty string");
  }
  if (prompt.length > MAX_PROMPT_LENGTH) {
    throw new Error(`prompt must be at most ${MAX_PROMPT_LENGTH} characters`);
  }

  const model = value.model;
  if (model !== undefined &&
      (typeof model !== "string" || model.length === 0 || model.length > MAX_MODEL_LENGTH)) {
    throw new Error(`model must be a non-empty string of at most ${MAX_MODEL_LENGTH} characters`);
  }

  const thinking = value.thinking;
  const thinkingLevels = new Set(["off", "minimal", "low", "medium", "high", "xhigh", "max"]);
  if (thinking !== undefined && !thinkingLevels.has(thinking)) {
    throw new Error("thinking must be one of: off, minimal, low, medium, high, xhigh, max");
  }

  const timeoutSeconds = value.timeoutSeconds;
  if (timeoutSeconds !== undefined &&
      (!Number.isInteger(timeoutSeconds) || timeoutSeconds < 1 || timeoutSeconds > 60 * 60)) {
    throw new Error("timeoutSeconds must be an integer between 1 and 3600");
  }

  return {
    prompt,
    model,
    thinking,
    timeoutMs: (timeoutSeconds ?? DEFAULT_TIMEOUT_MS / 1000) * 1000,
  };
}

function contentText(message) {
  if (!message || !Array.isArray(message.content)) return "";
  return message.content
    .filter((part) => part && part.type === "text" && typeof part.text === "string")
    .map((part) => part.text)
    .join("");
}

function runPi({ prompt, model, thinking, timeoutMs }) {
  return new Promise((resolve, reject) => {
    const args = ["--mode", "rpc", "--no-session", "--approve"];
    if (model) args.push("--model", model);
    if (thinking) args.push("--thinking", thinking);

    const child = spawn(PI_COMMAND, args, {
      cwd: process.cwd(),
      env: process.env,
      stdio: ["pipe", "pipe", "pipe"],
    });

    let output = "";
    let settled = false;
    let childClosed = false;
    let timer;
    let stderr = "";

    const finish = (callback, value) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      callback(value);
    };

    const stop = () => {
      if (!childClosed) child.kill("SIGTERM");
      setTimeout(() => {
        if (!childClosed) child.kill("SIGKILL");
      }, 2_000).unref();
    };

    const lines = createInterface({ input: child.stdout });
    lines.on("line", (line) => {
      if (!line.trim()) return;

      let message;
      try {
        message = JSON.parse(line);
      } catch {
        // Pi's RPC stream is JSONL. Ignore an accidental non-JSON diagnostic;
        // stderr is included if the process ultimately fails.
        return;
      }

      if (message.type === "response" && message.command === "prompt" && !message.success) {
        finish(reject, new Error("Pi rejected the prompt"));
        stop();
        return;
      }

      if (message.type === "message_update" &&
          message.assistantMessageEvent?.type === "text_delta" &&
          typeof message.assistantMessageEvent.delta === "string") {
        output += message.assistantMessageEvent.delta;
        return;
      }

      if (message.type === "agent_end") {
        const lastMessage = Array.isArray(message.messages)
          ? [...message.messages].reverse().find((item) => item?.role === "assistant")
          : undefined;
        const finalText = output || contentText(lastMessage);
        finish(resolve, finalText || "Pi completed without a text response.");
        stop();
        return;
      }

      if (message.type === "session.error") {
        finish(reject, new Error(message.error?.message || "Pi session failed"));
        stop();
      }
    });

    child.stderr.on("data", (chunk) => {
      stderr += chunk.toString();
    });

    child.on("error", (cause) => {
      finish(reject, new Error(`Unable to start Pi: ${cause.message}`));
    });

    child.on("close", (code, signal) => {
      childClosed = true;
      if (settled) return;
      const detail = stderr.trim() ? `: ${stderr.trim().slice(-2_000)}` : "";
      finish(reject, new Error(`Pi exited before completing (code=${code}, signal=${signal})${detail}`));
    });

    timer = setTimeout(() => {
      stop();
      finish(reject, new Error(`Pi timed out after ${Math.round(timeoutMs / 1000)} seconds`));
    }, timeoutMs);

    // Keep Pi's RPC stdin open until the agent settles. Closing it immediately
    // can make Pi exit before its asynchronous model response is delivered.
    child.stdin.write(JSON.stringify({
      id: "opencode-pi-prompt",
      type: "prompt",
      message: prompt,
    }) + "\n");
  });
}

function handle(request) {
  const { id, method, params = {} } = request;

  if (method === "initialize") {
    result(id, {
      protocolVersion: PROTOCOL_VERSION,
      capabilities: { tools: {} },
      serverInfo: { name: SERVER_NAME, version: SERVER_VERSION },
    });
    return;
  }

  if (method === "notifications/initialized" || method === "ping") {
    if (id !== undefined) result(id, {});
    return;
  }

  if (method === "tools/list") {
    result(id, {
      tools: [{
        name: TOOL_NAME,
        description: "Run a coding task in Pi. Pi runs in the current OpenCode project and may read or modify files.",
        inputSchema: {
          type: "object",
          properties: {
            prompt: {
              type: "string",
              description: "The coding or review task to give Pi.",
            },
            model: {
              type: "string",
              description: "Optional Pi model pattern, for example openai/gpt-5 or anthropic/claude-sonnet.",
            },
            thinking: {
              type: "string",
              enum: ["off", "minimal", "low", "medium", "high", "xhigh", "max"],
              description: "Optional Pi thinking level.",
            },
            timeoutSeconds: {
              type: "integer",
              minimum: 1,
              maximum: 3600,
              description: "Optional maximum run time (default: 600 seconds).",
            },
          },
          required: ["prompt"],
          additionalProperties: false,
        },
      }],
    });
    return;
  }

  if (method === "tools/call") {
    if (params.name !== TOOL_NAME) {
      result(id, textResult(`Unknown tool: ${params.name}`, true));
      return;
    }

    let args;
    try {
      args = validateArguments(params.arguments);
    } catch (cause) {
      result(id, textResult(cause instanceof Error ? cause.message : String(cause), true));
      return;
    }

    runPi(args)
      .then((text) => result(id, textResult(text)))
      .catch((cause) => result(id, textResult(cause instanceof Error ? cause.message : String(cause), true)));
    return;
  }

  if (id !== undefined) error(id, -32601, `Method not found: ${method}`);
}

const input = createInterface({ input: process.stdin });
input.on("line", (line) => {
  if (!line.trim()) return;
  try {
    handle(JSON.parse(line));
  } catch (cause) {
    error(null, -32700, cause instanceof Error ? cause.message : String(cause));
  }
});
