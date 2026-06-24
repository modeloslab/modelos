/**
 * Inference relay service — runs a small HTTP server inside the Electron main
 * process so miners can fetch prompts and deliver results directly to the wallet.
 *
 * Flow:
 *   1. User submits a prompt on InferencePage.
 *   2. Relay stores the prompt keyed by BLAKE3 hash.
 *   3. Wallet builds inference_tx with result_address = http://<ip>:<port>
 *   4. Miner fetches  GET  /prompt/:hash  → plaintext prompt
 *   5. Miner POSTs    POST /result/:hash  → { result: "..." }
 *   6. Relay fires ipcMain event → renderer updates the chat.
 *
 * Port: INFERENCE_RELAY_PORT (default 44211).
 * Bind address: 0.0.0.0 (all interfaces) so miners can reach it.
 */

import * as http from 'http';
import * as crypto from 'crypto';
import { ipcMain, BrowserWindow } from 'electron';
import * as os from 'os';

export const INFERENCE_RELAY_PORT = 44211;

interface StoredJob {
  prompt: string;
  promptHash: string;
  result?: string;
  createdAt: number;
}

export class InferenceRelayService {
  private server: http.Server | null = null;
  private jobs = new Map<string, StoredJob>(); // hash → job
  private win: BrowserWindow | null = null;

  /** Attach the BrowserWindow so we can push results to the renderer. */
  setWindow(win: BrowserWindow): void {
    this.win = win;
  }

  /** Start the HTTP relay server. Safe to call multiple times. */
  start(): void {
    if (this.server) return;

    this.server = http.createServer((req, res) => {
      res.setHeader('Access-Control-Allow-Origin', '*');
      res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
      res.setHeader('Access-Control-Allow-Headers', 'Content-Type');

      if (req.method === 'OPTIONS') {
        res.writeHead(204);
        res.end();
        return;
      }

      const url = new URL(req.url ?? '/', `http://localhost:${INFERENCE_RELAY_PORT}`);
      const parts = url.pathname.split('/').filter(Boolean);

      // GET /prompt/:hash — miner fetches the plaintext prompt
      if (req.method === 'GET' && parts[0] === 'prompt' && parts[1]) {
        const hash = parts[1].toLowerCase();
        const job = this.jobs.get(hash);
        if (!job) {
          res.writeHead(404, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({ error: 'prompt not found' }));
          return;
        }
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ prompt: job.prompt, hash }));
        return;
      }

      // POST /result/:hash — winning miner delivers the inference result
      if (req.method === 'POST' && parts[0] === 'result' && parts[1]) {
        const hash = parts[1].toLowerCase();
        let body = '';
        req.on('data', chunk => { body += chunk; });
        req.on('end', () => {
          try {
            const payload = JSON.parse(body) as {
              result?: string;
              text?: string;
              result_hash?: string; // hex SHA3-256 of result_text
            };
            const resultText = payload.result ?? payload.text ?? '';

            const job = this.jobs.get(hash);
            if (!job) {
              res.writeHead(404, { 'Content-Type': 'application/json' });
              res.end(JSON.stringify({ error: 'unknown prompt hash' }));
              return;
            }

            // Verify SHA3-256(resultText) == claimed result_hash.
            // This catches a miner delivering text A but intending to commit
            // to the hash of text B on-chain — the wallet will refuse payment
            // if the hashes don't match, so this is an early warning.
            if (payload.result_hash) {
              const actualHash = crypto.createHash('sha3-256').update(resultText, 'utf8').digest('hex');
              if (actualHash !== payload.result_hash.toLowerCase()) {
                res.writeHead(400, { 'Content-Type': 'application/json' });
                res.end(JSON.stringify({ error: 'hash_mismatch', detail: 'SHA3-256(result) does not match result_hash' }));
                return;
              }
            }

            job.result = resultText;

            // Push result to renderer immediately — no polling needed.
            this.win?.webContents.send('inference-result', {
              promptHash: hash,
              result: resultText,
              status: 'fulfilled',
            });

            res.writeHead(200, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ accepted: true }));
          } catch {
            res.writeHead(400, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ error: 'invalid JSON' }));
          }
        });
        return;
      }

      // GET /health
      if (req.method === 'GET' && parts[0] === 'health') {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ status: 'ok', jobs: this.jobs.size }));
        return;
      }

      res.writeHead(404, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'not found' }));
    });

    this.server.listen(INFERENCE_RELAY_PORT, '0.0.0.0', () => {
      console.log(`[inference-relay] listening on :${INFERENCE_RELAY_PORT}`);
    });

    this.server.on('error', (err: NodeJS.ErrnoException) => {
      if (err.code === 'EADDRINUSE') {
        console.warn(`[inference-relay] port ${INFERENCE_RELAY_PORT} in use — relay unavailable`);
      } else {
        console.error('[inference-relay] server error:', err);
      }
    });

    // Expire jobs older than 30 minutes to avoid unbounded memory growth.
    setInterval(() => this._expireOldJobs(), 5 * 60 * 1000);
  }

  stop(): void {
    this.server?.close();
    this.server = null;
  }

  /**
   * Store a prompt and return its hash.
   * Hash = first 32 hex chars of SHA3-256 (Node built-in; BLAKE3 not available in renderer).
   */
  storePrompt(prompt: string): string {
    const hash = crypto.createHash('sha3-256').update(prompt, 'utf8').digest('hex');
    this.jobs.set(hash, { prompt, promptHash: hash, createdAt: Date.now() });
    return hash;
  }

  /** Return stored result if the miner has already delivered it. */
  getResult(promptHash: string): { result: string; status: 'fulfilled' } | null {
    const job = this.jobs.get(promptHash.toLowerCase());
    if (job?.result !== undefined) {
      return { result: job.result, status: 'fulfilled' };
    }
    return null;
  }

  /**
   * Build the relay URL to embed as result_address in the inference_tx.
   * Prefers the machine's external/LAN IP; can be overridden via env var
   * MODELOS_INFERENCE_RELAY_HOST for users with static public IPs.
   */
  buildRelayUrl(): string {
    const host = process.env.MODELOS_INFERENCE_RELAY_HOST ?? this._getLanIp();
    return `http://${host}:${INFERENCE_RELAY_PORT}`;
  }

  private _getLanIp(): string {
    const interfaces = os.networkInterfaces();
    for (const iface of Object.values(interfaces)) {
      for (const addr of iface ?? []) {
        if (addr.family === 'IPv4' && !addr.internal) {
          return addr.address;
        }
      }
    }
    return '127.0.0.1';
  }

  private _expireOldJobs(): void {
    const cutoff = Date.now() - 30 * 60 * 1000;
    for (const [hash, job] of this.jobs.entries()) {
      if (job.createdAt < cutoff) this.jobs.delete(hash);
    }
  }
}

export const inferenceRelay = new InferenceRelayService();
