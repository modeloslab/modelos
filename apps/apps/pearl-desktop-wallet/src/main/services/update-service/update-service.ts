import { app, shell } from 'electron';
import { EventEmitter } from 'node:events';
import log from 'electron-log';
import {
  classifyUpdate,
  compareSemver,
  parseSemver,
  parseTaggedSemver,
  SemVer,
  UpdateSeverity,
} from './semver';

export interface UpdateStatus {
  severity: UpdateSeverity;
  localVersion: string;
  latestVersion: string | null;
  releaseUrl: string | null;
  releaseName: string | null;
  publishedAt: string | null;
  checkedAt: number | null;
  error: string | null;
}

export interface UpdateServiceOptions {
  owner: string;
  repo: string;
  tagPrefix: string;
  // Intervals are configurable mainly to keep tests fast; production values live in consts.
  initialDelayMs?: number;
  // Minimum wall-clock gap between automatic checks. Focus-triggered checks
  // within this window are ignored; manual "check now" actions can opt out.
  minIntervalMs?: number;
  requestTimeoutMs?: number;
}

interface GithubRelease {
  tag_name: string;
  name: string | null;
  html_url: string;
  draft: boolean;
  prerelease: boolean;
  published_at: string | null;
}

const DEFAULT_INITIAL_DELAY_MS = 5_000;
const DEFAULT_MIN_INTERVAL_MS = 10 * 60 * 1000; // 10 minutes
const DEFAULT_REQUEST_TIMEOUT_MS = 10_000;

class UpdateService extends EventEmitter {
  private readonly options: Required<UpdateServiceOptions>;
  private status: UpdateStatus;
  private initialTimer: NodeJS.Timeout | null = null;
  private inFlight: Promise<UpdateStatus> | null = null;
  // Wall-clock timestamp of the last *attempted* check (success or failure).
  // We throttle on attempt-time rather than success-time so a burst of focus
  // events during an outage doesn't hammer GitHub.
  private lastAttemptAt: number | null = null;

  constructor(options: UpdateServiceOptions) {
    super();
    this.options = {
      initialDelayMs: DEFAULT_INITIAL_DELAY_MS,
      minIntervalMs: DEFAULT_MIN_INTERVAL_MS,
      requestTimeoutMs: DEFAULT_REQUEST_TIMEOUT_MS,
      ...options,
    };
    this.status = {
      severity: 'none',
      localVersion: app.getVersion(),
      latestVersion: null,
      releaseUrl: null,
      releaseName: null,
      publishedAt: null,
      checkedAt: null,
      error: null,
    };
  }

  getStatus(): UpdateStatus {
    return this.status;
  }

  // Schedules a single delayed first check after process start. There is no
  // periodic poll anymore: subsequent checks are driven by window focus via
  // `notifyWindowFocus()`, subject to `minIntervalMs` throttling.
  start(): void {
    this.initialTimer = setTimeout(() => {
      void this.checkForUpdates().catch(() => {
        // already logged inside checkForUpdates
      });
    }, this.options.initialDelayMs);
  }

  stop(): void {
    if (this.initialTimer) clearTimeout(this.initialTimer);
    this.initialTimer = null;
  }

  // Called by the main process on `browser-window-focus`. Honours the
  // `minIntervalMs` throttle so rapid focus/blur cycles don't flood GitHub.
  notifyWindowFocus(): void {
    if (!this.isThrottleExpired()) return;
    void this.checkForUpdates().catch(() => {
      // already logged inside checkForUpdates
    });
  }

  // Returns true if the last attempt is older than `minIntervalMs` (or there
  // has never been an attempt).
  private isThrottleExpired(): boolean {
    if (this.lastAttemptAt === null) return true;
    return Date.now() - this.lastAttemptAt >= this.options.minIntervalMs;
  }

  // `force: true` bypasses the throttle. Used by the renderer's manual
  // "Check now" action so the user isn't silently ignored.
  async checkForUpdates(options: { force?: boolean } = {}): Promise<UpdateStatus> {
    if (this.inFlight) return this.inFlight;
    if (!options.force && !this.isThrottleExpired()) return this.status;
    this.lastAttemptAt = Date.now();
    this.inFlight = this.performCheck()
      .catch(err => {
        const message = err instanceof Error ? err.message : String(err);
        log.warn('[UpdateService] Update check failed:', message);
        const next: UpdateStatus = {
          ...this.status,
          checkedAt: Date.now(),
          error: message,
        };
        this.setStatus(next);
        return next;
      })
      .finally(() => {
        this.inFlight = null;
      }) as Promise<UpdateStatus>;
    return this.inFlight;
  }

  async openReleasePage(): Promise<void> {
    const url = this.status.releaseUrl ?? this.defaultReleasesUrl();
    await shell.openExternal(url);
  }

  private defaultReleasesUrl(): string {
    return `https://github.com/${this.options.owner}/${this.options.repo}/releases`;
  }

  private async performCheck(): Promise<UpdateStatus> {
    const localVersion = app.getVersion();
    const local = parseSemver(localVersion);
    if (!local) {
      throw new Error(`Unable to parse local app version "${localVersion}"`);
    }

    const releases = await this.fetchReleases();
    const candidate = this.pickLatestWalletRelease(releases);

    if (!candidate) {
      const next: UpdateStatus = {
        severity: 'none',
        localVersion,
        latestVersion: null,
        releaseUrl: null,
        releaseName: null,
        publishedAt: null,
        checkedAt: Date.now(),
        error: null,
      };
      this.setStatus(next);
      return next;
    }

    const { release, version } = candidate;
    const severity = classifyUpdate(local, version);
    const next: UpdateStatus = {
      severity,
      localVersion,
      latestVersion: version.raw,
      releaseUrl: release.html_url,
      releaseName: release.name,
      publishedAt: release.published_at,
      checkedAt: Date.now(),
      error: null,
    };
    this.setStatus(next);
    return next;
  }

  private setStatus(next: UpdateStatus): void {
    const changed =
      next.severity !== this.status.severity ||
      next.latestVersion !== this.status.latestVersion ||
      next.error !== this.status.error ||
      next.releaseUrl !== this.status.releaseUrl;

    this.status = next;
    if (changed) {
      this.emit('status-changed', next);
    }
  }

  private async fetchReleases(): Promise<GithubRelease[]> {
    const { owner, repo, requestTimeoutMs } = this.options;
    const url = `https://api.github.com/repos/${owner}/${repo}/releases?per_page=30`;
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), requestTimeoutMs);
    try {
      const response = await fetch(url, {
        headers: {
          Accept: 'application/vnd.github+json',
          'X-GitHub-Api-Version': '2022-11-28',
          'User-Agent': `modelos-desktop-wallet/${app.getVersion()}`,
        },
        signal: controller.signal,
      });
      if (!response.ok) {
        throw new Error(`GitHub releases API returned ${response.status}`);
      }
      const data = (await response.json()) as GithubRelease[];
      return data;
    } finally {
      clearTimeout(timeout);
    }
  }

  private pickLatestWalletRelease(
    releases: GithubRelease[],
  ): { release: GithubRelease; version: SemVer } | null {
    let best: { release: GithubRelease; version: SemVer } | null = null;
    for (const release of releases) {
      if (release.draft || release.prerelease) continue;
      const version = parseTaggedSemver(release.tag_name, this.options.tagPrefix);
      if (!version) continue;
      // Skip prereleases surfaced only via tag (e.g. "modelos-wallet-v0.1.0-rc1")
      if (version.prerelease) continue;
      if (!best || compareSemver(version, best.version) > 0) {
        best = { release, version };
      }
    }
    return best;
  }
}

export { UpdateService };
