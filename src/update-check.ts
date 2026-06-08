import { createRequire } from "node:module";
import { get } from "node:https";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";

const require = createRequire(import.meta.url);
const packageJson = require("../../package.json") as { name: string; version: string };

const CHECK_INTERVAL_MS = 24 * 60 * 60 * 1000;
const REQUEST_TIMEOUT_MS = 800;
const REGISTRY_URL = `https://registry.npmjs.org/${encodeURIComponent(packageJson.name)}/latest`;

interface UpdateCheckCache {
  checkedAt: number;
  latestVersion?: string;
}

interface RegistryLatest {
  version?: string;
}

export interface UpdateNotice {
  currentVersion: string;
  latestVersion: string;
  message: string;
}

interface UpdateCheckOptions {
  args: string[];
  homeDir: string;
  env?: NodeJS.ProcessEnv;
  stderrIsTty?: boolean;
  now?: number;
  fetchLatestVersion?: () => Promise<string | undefined>;
}

export async function maybePrintUpdateNotice(options: UpdateCheckOptions): Promise<void> {
  if (!shouldCheckForUpdates(options)) return;

  const notice = await checkForUpdate(options).catch(() => undefined);
  if (!notice) return;

  process.stderr.write(`${notice.message}\n`);
}

export async function checkForUpdate(options: UpdateCheckOptions): Promise<UpdateNotice | undefined> {
  const now = options.now ?? Date.now();
  const cachePath = updateCachePath(options.homeDir);
  const cached = await readUpdateCache(cachePath);

  if (cached && now - cached.checkedAt < CHECK_INTERVAL_MS) {
    return noticeForLatestVersion(cached.latestVersion);
  }

  const latestVersion = await (options.fetchLatestVersion ?? fetchLatestVersion)();
  await writeUpdateCache(cachePath, { checkedAt: now, latestVersion });
  return noticeForLatestVersion(latestVersion);
}

export function noticeForLatestVersion(latestVersion: string | undefined): UpdateNotice | undefined {
  if (!latestVersion || !isNewerVersion(latestVersion, packageJson.version)) return undefined;

  return {
    currentVersion: packageJson.version,
    latestVersion,
    message: `✨ hem ${latestVersion} is available. Update with: npm install -g ${packageJson.name}`
  };
}

export function shouldCheckForUpdates(options: UpdateCheckOptions): boolean {
  const env = options.env ?? process.env;
  if (env.HEM_UPDATE_CHECK === "0" || env.NO_UPDATE_NOTIFIER === "1") return false;
  if (env.CI === "true" || env.CI === "1") return false;
  if (options.stderrIsTty === false) return false;
  if (options.args.includes("--json")) return false;
  return true;
}

function isNewerVersion(candidate: string, current: string): boolean {
  const left = parseVersion(candidate);
  const right = parseVersion(current);
  if (!left || !right) return false;

  for (let index = 0; index < 3; index += 1) {
    if (left[index] > right[index]) return true;
    if (left[index] < right[index]) return false;
  }
  return false;
}

function parseVersion(version: string): [number, number, number] | undefined {
  const match = version.match(/^(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$/);
  if (!match) return undefined;
  return [Number(match[1]), Number(match[2]), Number(match[3])];
}

async function fetchLatestVersion(): Promise<string | undefined> {
  return await new Promise((resolve) => {
    const request = get(REGISTRY_URL, { timeout: REQUEST_TIMEOUT_MS }, (response) => {
      if (response.statusCode !== 200) {
        response.resume();
        resolve(undefined);
        return;
      }

      let body = "";
      response.setEncoding("utf8");
      response.on("data", (chunk) => {
        body += chunk;
      });
      response.on("end", () => {
        try {
          resolve((JSON.parse(body) as RegistryLatest).version);
        } catch {
          resolve(undefined);
        }
      });
    });

    request.on("timeout", () => {
      request.destroy();
      resolve(undefined);
    });
    request.on("error", () => {
      resolve(undefined);
    });
  });
}

async function readUpdateCache(cachePath: string): Promise<UpdateCheckCache | undefined> {
  try {
    return JSON.parse(await readFile(cachePath, "utf8")) as UpdateCheckCache;
  } catch {
    return undefined;
  }
}

async function writeUpdateCache(cachePath: string, cache: UpdateCheckCache): Promise<void> {
  try {
    await mkdir(path.dirname(cachePath), { recursive: true, mode: 0o700 });
    await writeFile(cachePath, `${JSON.stringify(cache)}\n`, { mode: 0o600 });
  } catch {
    // Update checks are advisory only.
  }
}

function updateCachePath(homeDir: string): string {
  return path.join(homeDir, ".hem", "update-check.json");
}
