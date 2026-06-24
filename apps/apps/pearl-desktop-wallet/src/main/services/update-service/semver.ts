export interface SemVer {
  major: number;
  minor: number;
  patch: number;
  prerelease: string | null;
  raw: string;
}

// Parses versions such as "0.1.2" or "v0.1.2" or "0.1.2-rc1". Returns null if invalid.
export function parseSemver(input: string): SemVer | null {
  if (!input) return null;
  const trimmed = input.trim().replace(/^v/i, '');
  const match = /^(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$/.exec(trimmed);
  if (!match) return null;
  const [, major, minor, patch, prerelease] = match;
  return {
    major: Number(major),
    minor: Number(minor),
    patch: Number(patch),
    prerelease: prerelease ?? null,
    raw: trimmed,
  };
}

// Strips a known tag prefix (e.g. "modelos-wallet-v") before parsing.
export function parseTaggedSemver(tag: string, prefix: string): SemVer | null {
  if (!tag) return null;
  const stripped = tag.startsWith(prefix) ? tag.slice(prefix.length) : tag;
  return parseSemver(stripped);
}

// Returns a negative number if a < b, 0 if equal, positive if a > b.
// Uses standard semver precedence: a release is greater than any prerelease of the same X.Y.Z.
export function compareSemver(a: SemVer, b: SemVer): number {
  if (a.major !== b.major) return a.major - b.major;
  if (a.minor !== b.minor) return a.minor - b.minor;
  if (a.patch !== b.patch) return a.patch - b.patch;
  if (a.prerelease === b.prerelease) return 0;
  if (a.prerelease === null) return 1;
  if (b.prerelease === null) return -1;
  return a.prerelease < b.prerelease ? -1 : 1;
}

export type UpdateSeverity = 'none' | 'patch' | 'minor' | 'major';

// Decides how critical the gap between local and remote is.
// A remote version that is equal or older than local produces 'none'.
export function classifyUpdate(local: SemVer, remote: SemVer): UpdateSeverity {
  if (compareSemver(remote, local) <= 0) return 'none';
  if (remote.major > local.major) return 'major';
  if (remote.minor > local.minor) return 'minor';
  return 'patch';
}
