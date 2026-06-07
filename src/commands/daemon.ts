import { hasFlag, json, valueAfter } from "../cli-shared.js";
import { formatSnapError } from "../errors.js";
import {
  DaemonStartError,
  readDaemonStatus,
  runDaemonWorker,
  startDaemon,
  stopDaemon
} from "../daemon.js";
import type { DaemonStatus } from "../types.js";
import type { Command, CommandContext } from "./index.js";

export const daemonCommand: Command = {
  name: "daemon",
  description: "Run the local setup timeline daemon",

  async execute(ctx: CommandContext): Promise<number> {
    const { args, options } = ctx;
    const sub = args[1] ?? "status";

    if (sub === "start") {
      let result: Awaited<ReturnType<typeof startDaemon>>;
      try {
        result = await startDaemon(options, {
          intervalMs: numberAfter(args, "--interval-ms"),
          debounceMs: numberAfter(args, "--debounce-ms")
        });
      } catch (error) {
        if (error instanceof DaemonStartError) {
          if (hasFlag(args, "--json")) {
            process.stdout.write(json({ started: false, status: error.status, error: error.code }));
          }
          process.stderr.write(formatSnapError({
            code: error.code,
            problem: error.problem,
            cause: error.cause,
            fix: error.fix
          }));
          return 1;
        }
        throw error;
      }
      process.stdout.write(hasFlag(args, "--json")
        ? json({ started: result.started, status: result.status })
        : renderStatus(result.status, headlineForStart(result)));
      return result.reason === "stale-live" ? 1 : 0;
    }

    if (sub === "status") {
      const result = await readDaemonStatus(options);
      if (hasFlag(args, "--json")) {
        process.stdout.write(json(result.ok ? result.status : { ...result.status, error: result.error }));
        return result.ok ? 0 : 1;
      }
      process.stdout.write(renderStatus(result.status, result.ok ? undefined : `Status error: ${result.error}`));
      return result.ok ? 0 : 1;
    }

    if (sub === "stop") {
      const result = await stopDaemon(options);
      process.stdout.write(hasFlag(args, "--json")
        ? json({ stopped: result.stopped, status: result.status })
        : renderStatus(result.status, result.stopped ? "Stopped hem daemon." : "Hem daemon was not safely running."));
      return 0;
    }

    if (sub === "worker") {
      const runId = valueAfter(args, "--run-id");
      if (!runId) {
        process.stderr.write(
          formatSnapError({
            code: "HEM_DAEMON_RUN_ID_REQUIRED",
            problem: "Daemon worker requires --run-id.",
            cause: "`daemon worker` was called without an identity token.",
            fix: "Run `hem daemon start`; worker is an internal command."
          })
        );
        return 1;
      }
      const identityToken = valueAfter(args, "--identity-token");
      if (!identityToken) {
        process.stderr.write(
          formatSnapError({
            code: "HEM_DAEMON_IDENTITY_REQUIRED",
            problem: "Daemon worker requires --identity-token.",
            cause: "`daemon worker` was called without a worker identity token.",
            fix: "Run `hem daemon start`; worker is an internal command."
          })
        );
        return 1;
      }
      await runDaemonWorker(options, runId, {
        intervalMs: numberAfter(args, "--interval-ms"),
        debounceMs: numberAfter(args, "--debounce-ms"),
        identityToken
      });
      return 0;
    }

    process.stderr.write(
      formatSnapError({
        code: "HEM_UNKNOWN_SUBCOMMAND",
        problem: `Unknown daemon subcommand: "${sub}".`,
        cause: "`daemon` was called with an unrecognized subcommand.",
        fix: "Use `start`, `status`, or `stop`. Run `hem --help` for details."
      })
    );
    return 1;
  }
};

function numberAfter(args: string[], flag: string): number | undefined {
  const raw = valueAfter(args, flag);
  if (!raw) return undefined;
  const parsed = Number.parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}

function renderStatus(status: DaemonStatus, headline?: string): string {
  const lines = [
    ...(headline ? [headline, ""] : []),
    `running: ${status.running ? "yes" : "no"}`,
    `stale: ${status.stale ? "yes" : "no"}`,
    `stale reason: ${status.staleReason ?? "-"}`,
    `pid: ${status.pid ?? "-"}`,
    `pid alive: ${status.pidAlive ? "yes" : "no"}`,
    `identity verified: ${status.identityVerified ? "yes" : "no"}`,
    `identity error: ${status.identityError ?? "-"}`,
    `run id: ${status.runId ?? "-"}`,
    `last heartbeat: ${status.lastHeartbeatAt ?? "-"}`,
    `last event: ${status.lastEventAt ?? "-"}`,
    `watched paths: ${status.watchedPaths.length}`,
    `store: ${status.storeDir}`
  ];

  if (status.errors.length > 0) {
    lines.push("errors:");
    for (const error of status.errors) {
      lines.push(`  ${error}`);
    }
  }

  return `${lines.join("\n")}\n`;
}

function headlineForStart(result: Awaited<ReturnType<typeof startDaemon>>): string {
  if (result.reason === "started") return "Started hem daemon.";
  if (result.reason === "already-running") return "Hem daemon already running.";
  return "Hem daemon is stale but its pid is still alive; not starting another daemon.";
}

export default daemonCommand;
