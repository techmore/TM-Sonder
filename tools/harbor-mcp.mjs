#!/usr/bin/env node

/**
 * Harbor MCP bridge: safe control of the Harbor torrent client
 * (co.hapy.harbor) from OpenCode.
 *
 * Harbor has no API. This bridge edits
 * ~/Library/Application Support/Harbor/downloads.json with Harbor stopped
 * (backup first), then relaunches. Reads are lock-free.
 *
 * Tools:
 *   harbor_status   counts by status + downloading list + disk
 *   harbor_queue    add magnet(s) as queued entries (dedupe by fingerprint)
 *   harbor_pause    set seeding/downloading entries to paused (filter optional)
 *   harbor_move     dry-run or live pass of bin/harbor-mover.py (movies only)
 *   harbor_cleanup  drop entries whose staging source is gone
 */

import { execFileSync, spawnSync } from "node:child_process";
import { createInterface } from "node:readline";
import { randomUUID } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const PROTOCOL_VERSION = "2024-11-05";
const SERVER_NAME = "harbor-mcp";
const SERVER_VERSION = "1.0.0";

const HOME = os.homedir();
const HARBOR_DIR = path.join(HOME, "Library/Application Support/Harbor");
const STATE_FILE = path.join(HARBOR_DIR, "downloads.json");
const STAGING = path.join(HOME, "Downloads/Torrents");
const PLEX_MOVIES = "/Users/seandolbec/NAS/plex/movies";
const MOVER = "/Users/seandolbec/Projects/TM-Sonder/bin/harbor-mover.py";

const TRACKERS =
  "&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337" +
  "&tr=udp%3A%2F%2Fopen.stealth.si%3A80%2Fannounce" +
  "&tr=udp%3A%2F%2Ftracker.torrent.eu.org%3A451%2Fannounce" +
  "&tr=udp%3A%2F%2Ftracker.bittor.pw%3A1337%2Fannounce" +
  "&tr=udp%3A%2F%2Fpublic.popcorn-tracker.org%3A6969%2Fannounce" +
  "&tr=udp%3A%2F%2Ftracker.dler.org%3A6969%2Fannounce" +
  "&tr=udp%3A%2F%2Fexodus.desync.com%3A6969" +
  "&tr=udp%3A%2F%2Fopen.demonii.com%3A1337%2Fannounce" +
  "&tr=udp%3A%2F%2Fglotorrents.pw%3A6969%2Fannounce" +
  "&tr=udp%3A%2F%2Ftracker.coppersurfer.tk%3A6969";

function send(message) {
  process.stdout.write(`${JSON.stringify(message)}\n`);
}
function result(id, value) {
  send({ jsonrpc: "2.0", id, result: value });
}
function error(id, code, message) {
  send({ jsonrpc: "2.0", id, error: { code, message } });
}
function textResult(text, isError = false) {
  return {
    content: [{ type: "text", text }],
    isError,
  };
}

function loadState() {
  return JSON.parse(fs.readFileSync(STATE_FILE, "utf8"));
}
function cfNow() {
  return Date.now() / 1000 - 978307200; // CFAbsoluteTime
}
function harborRunning() {
  const r = spawnSync("pgrep", ["-f", "MacOS/Harbor"]);
  return r.status === 0;
}
function stopHarbor() {
  // SIGTERM the app; aria2 child is handled separately on relaunch.
  spawnSync("pkill", ["-f", "MacOS/Harbor"]);
  for (let i = 0; i < 10; i++) {
    if (!harborRunning()) return true;
    Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 500);
  }
  return !harborRunning();
}
function startHarbor() {
  spawnSync("pkill", ["-f", "aria2-next"]);
  spawnSync("open", ["-a", "Harbor"]);
}
function backup() {
  const dst = `/tmp/downloads.json.harbor-mcp-${Date.now()}.bak`;
  fs.copyFileSync(STATE_FILE, dst);
  return dst;
}
function magnetFor(hash, name) {
  return (
    `magnet:?xt=urn:btih:${hash}&dn=${encodeURIComponent(name)}${TRACKERS}`
  );
}
function makeEntry(hash, name) {
  const now = cfNow();
  return {
    sourceURL: magnetFor(hash, name),
    progress: 0,
    shouldSeedAfterDownload: true,
    updatedAt: now,
    activityEvents: [{ kind: "added", id: randomUUID().toUpperCase(), timestamp: now }],
    id: randomUUID().toUpperCase(),
    requestHeaders: [],
    completionNotificationDelivered: false,
    backend: "aria2",
    uploadLimitOverride: { inherit: {} },
    destinationFolderPath: STAGING,
    status: "queued",
    wasSuspendedForNetworkBinding: false,
    torrentPayloadPaths: [],
    expectedBytes: 0,
    uploadedBytes: 0,
    requiresMediaRecoveryReset: false,
    downloadsTorrentPiecesSequentially: false,
    removeOriginalTorrentAfterImport: false,
    downloadLimitOverride: { inherit: {} },
    bytesWritten: 0,
    metadataName: name,
    createdAt: now,
    torrentFingerprint: hash.toLowerCase(),
    sourceKind: "magnetLink",
  };
}
function diskStats() {
  try {
    const df = execFileSync("df", ["-h", "/System/Volumes/Data"]).toString().trim().split("\n").pop();
    const du = execFileSync("du", ["-sh", STAGING]).toString().trim();
    return { df, du };
  } catch (e) {
    return { error: String(e.message || e) };
  }
}

const TOOLS = [
  {
    name: "harbor_status",
    description: "Harbor queue counts by status, top downloading items, and local disk/staging usage. Read-only.",
    inputSchema: { type: "object", properties: {}, additionalProperties: false },
  },
  {
    name: "harbor_queue",
    description: "Queue magnet(s) into Harbor as paused-at-first queued entries. Dedupes by infohash. Restarts Harbor to pick them up.",
    inputSchema: {
      type: "object",
      properties: {
        items: {
          type: "array",
          description: "Entries to queue",
          items: {
            type: "object",
            properties: {
              hash: { type: "string", description: "40-char infohash hex" },
              name: { type: "string", description: "Display name, e.g. 'Title (Year) 1080p BluRay x264'" },
            },
            required: ["hash", "name"],
          },
        },
      },
      required: ["items"],
    },
  },
  {
    name: "harbor_pause",
    description: "Set matching entries to paused (stops seeding/downloading). Filter by status and/or name substring. Default pauses seeding entries.",
    inputSchema: {
      type: "object",
      properties: {
        status: { type: "string", description: "Only touch this status", default: "seeding" },
        match: { type: "string", description: "Only entries whose metadataName includes this (case-insensitive)" },
      },
      additionalProperties: false,
    },
  },
  {
    name: "harbor_move",
    description: "Run bin/harbor-mover.py once (movies staging -> Plex). Dry-run by default; live=true actually moves with size verification.",
    inputSchema: {
      type: "object",
      properties: {
        live: { type: "boolean", description: "Actually move files (default false = dry run)" },
      },
      additionalProperties: false,
    },
  },
  {
    name: "harbor_cleanup",
    description: "Drop Harbor entries whose staging source is gone (already moved to Plex). Restarts Harbor.",
    inputSchema: { type: "object", properties: {}, additionalProperties: false },
  },
];

function handleStatus(id) {
  const d = loadState();
  const counts = {};
  for (const x of d) counts[x.status] = (counts[x.status] || 0) + 1;
  const dl = d
    .filter((x) => x.status === "downloading")
    .sort((a, b) => (b.progress || 0) - (a.progress || 0))
    .slice(0, 10)
    .map((x) => `${(x.progress || 0).toFixed(2)} ${x.metadataName}`);
  result(id, textResult(JSON.stringify({ total: d.length, counts, downloading: dl, disk: diskStats(), harborRunning: harborRunning() }, null, 2)));
}

function handleQueue(id, args) {
  const items = (args && args.items) || [];
  if (!items.length) return error(id, -32602, "items[] required");
  for (const it of items) {
    if (!/^[0-9a-fA-F]{40}$/.test(it.hash || "")) return error(id, -32602, `bad infohash: ${it.hash}`);
    if (!it.name) return error(id, -32602, "name required for each item");
  }
  if (!stopHarbor()) return error(id, -32000, "could not stop Harbor; aborting (state untouched)");
  const bak = backup();
  try {
    const d = loadState();
    const fps = new Set(d.map((x) => (x.torrentFingerprint || "").toLowerCase()));
    let added = 0, skipped = 0;
    for (const it of items) {
      if (fps.has(it.hash.toLowerCase())) { skipped++; continue; }
      d.push(makeEntry(it.hash, it.name));
      fps.add(it.hash.toLowerCase());
      added++;
    }
    fs.writeFileSync(STATE_FILE, JSON.stringify(d, null, 2));
    startHarbor();
    result(id, textResult(`queued ${added}, skipped-dupes ${skipped}, total ${d.length} (backup ${bak})`));
  } catch (e) {
    try { startHarbor(); } catch {}
    error(id, -32000, `queue failed: ${e.message} (backup ${bak})`);
  }
}

function handlePause(id, args) {
  const status = (args && args.status) || "seeding";
  const match = ((args && args.match) || "").toLowerCase();
  if (!stopHarbor()) return error(id, -32000, "could not stop Harbor; aborting");
  const bak = backup();
  try {
    const d = loadState();
    let n = 0;
    for (const x of d) {
      if (x.status !== status) continue;
      if (match && !(x.metadataName || "").toLowerCase().includes(match)) continue;
      x.status = "paused";
      x.updatedAt = cfNow();
      n++;
    }
    fs.writeFileSync(STATE_FILE, JSON.stringify(d, null, 2));
    startHarbor();
    result(id, textResult(`paused ${n} (status=${status}${match ? ` match=${match}` : ""}) (backup ${bak})`));
  } catch (e) {
    try { startHarbor(); } catch {}
    error(id, -32000, `pause failed: ${e.message} (backup ${bak})`);
  }
}

function handleMove(id, args) {
  const live = !!(args && args.live);
  try {
    const env = { ...process.env };
    if (live) env.DRYRUN = "0";
    const out = execFileSync("python3", [MOVER], { env, timeout: 110000 }).toString();
    const tail = out.trim().split("\n").slice(-4).join("\n");
    result(id, textResult(`${live ? "LIVE" : "dry-run"} mover done:\n${tail}`));
  } catch (e) {
    error(id, -32000, `mover failed/timeout: ${(e.stdout || "").toString().slice(-500) || e.message}`);
  }
}

function handleCleanup(id) {
  if (!stopHarbor()) return error(id, -32000, "could not stop Harbor; aborting");
  const bak = backup();
  try {
    const d = loadState();
    const kept = [];
    let removed = 0;
    for (const x of d) {
      const src = path.join(STAGING, x.metadataName || "");
      if (!fs.existsSync(src)) { removed++; continue; }
      kept.push(x);
    }
    fs.writeFileSync(STATE_FILE, JSON.stringify(kept, null, 2));
    startHarbor();
    result(id, textResult(`removed ${removed} gone-source entries, kept ${kept.length} (backup ${bak})`));
  } catch (e) {
    try { startHarbor(); } catch {}
    error(id, -32000, `cleanup failed: ${e.message} (backup ${bak})`);
  }
}

const rl = createInterface({ input: process.stdin, terminal: false });
let initialized = false;
rl.on("line", (line) => {
  if (!line.trim()) return;
  let msg;
  try { msg = JSON.parse(line); } catch { return; }
  if (msg.method === "initialize") {
    initialized = true;
    result(msg.id, {
      protocolVersion: PROTOCOL_VERSION,
      serverInfo: { name: SERVER_NAME, version: SERVER_VERSION },
      capabilities: { tools: {} },
    });
    return;
  }
  if (msg.method === "notifications/initialized") return;
  if (msg.method === "tools/list") {
    result(msg.id, { tools: TOOLS });
    return;
  }
  if (msg.method === "tools/call") {
    if (!initialized) return error(msg.id, -32002, "not initialized");
    const { name, arguments: args } = msg.params || {};
    try {
      if (name === "harbor_status") return handleStatus(msg.id);
      if (name === "harbor_queue") return handleQueue(msg.id, args);
      if (name === "harbor_pause") return handlePause(msg.id, args);
      if (name === "harbor_move") return handleMove(msg.id, args);
      if (name === "harbor_cleanup") return handleCleanup(msg.id, args);
      return error(msg.id, -32601, `unknown tool: ${name}`);
    } catch (e) {
      return error(msg.id, -32000, e.message);
    }
    return;
  }
  if (msg.id !== undefined) error(msg.id, -32601, `unknown method: ${msg.method}`);
});
