import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { describe, it } from "node:test";
import {
  applyAgentConfig,
  applyRestoreItems,
  applyWithRollback,
  buildRestorePlan,
  clearAppliedItems,
  createDefaultApplyExecutor,
  defaultApplyHandlerRegistry,
  createDefaultUndoExecutor,
  defaultUndoHandlerRegistry,
  formatApplySummary,
  formatRollbackSummary,
  getAppliedItems,
  getSuccessfulItems,
  noopUndoHandler,
  parseDryRunOutput,
  restorePreviousStateUndoHandler,
  rollbackAppliedItems,
  sortByDescendingOrder
} from "../src/restore.js";
import { captureCurrentState } from "../src/current-state.js";
import { writeSnapshot } from "../src/store.js";
import type {
  ApplyOptions,
  ApplySummary,
  RestoreExecutor,
  RestoreItem,
  RestoreItemStatus,
  RollbackOptions,
  RollbackSummary,
  UndoExecutor,
  UndoHandler,
  UndoHandlerRegistry
} from "../src/types.js";

async function makeRestoreSandbox(): Promise<{ projectPath: string; homeDir: string; storeDir: string }> {
  const root = await mkdtemp(path.join(tmpdir(), "gandalf-restore-"));
  const projectPath = path.join(root, "project");
  const homeDir = path.join(root, "home");
  const storeDir = path.join(root, "store");
  await mkdir(projectPath, { recursive: true });
  await mkdir(homeDir, { recursive: true });
  return { projectPath, homeDir, storeDir };
}

describe("content-backed Codex user restore planning", () => {
  it("restores a zero-filled Codex config byte-for-byte through targetHome", async () => {
    const { projectPath, homeDir, storeDir } = await makeRestoreSandbox();
    const configPath = path.join(homeDir, ".codex", "config.toml");
    const original = "model = \"gpt-5\"\napproval_policy = \"on-request\"\n";
    await mkdir(path.dirname(configPath), { recursive: true });
    await writeFile(configPath, original, "utf8");

    const state = await captureCurrentState({
      projectPath,
      homeDir,
      storeDir,
      agent: "codex",
      scope: "user",
      captureContent: true
    }, "baseline");
    await writeSnapshot(storeDir, state.snapshot, "codex");
    await writeFile(configPath, "", "utf8");

    const plan = await buildRestorePlan({
      sourceSnapshot: "baseline",
      projectPath,
      homeDir,
      storeDir,
      dryRun: true,
      agent: "codex",
      scope: "user"
    });
    const configItem = plan.items.find((item) => item.kind === "agent_config");

    assert.ok(configItem);
    assert.equal(configItem.action, "update");
    assert.equal(configItem.agent, "codex");
    assert.equal(configItem.sourcePath, "~/.codex/config.toml");
    assert.equal(plan.targetHome, homeDir);

    const parsed = parseDryRunOutput(JSON.stringify(plan));
    assert.deepEqual(parsed.errors, []);
    const executableConfigItem = parsed.items.find((item) => item.type === "agent_config");
    assert.equal(executableConfigItem?.dest, configPath);
    assert.equal(executableConfigItem?.targetContent, original);

    const summary = await applyRestoreItems(
      parsed.items,
      createDefaultApplyExecutor(defaultApplyHandlerRegistry()),
      { failFast: true }
    );

    assert.equal(summary.failed, 0);
    assert.equal(await readFile(configPath, "utf8"), original);
  });

  it("deletes a Codex user skill added after the baseline", async () => {
    const { projectPath, homeDir, storeDir } = await makeRestoreSandbox();
    await mkdir(path.join(homeDir, ".codex"), { recursive: true });
    await writeFile(path.join(homeDir, ".codex", "config.toml"), "model = \"gpt-5\"\n", "utf8");
    const state = await captureCurrentState({
      projectPath,
      homeDir,
      storeDir,
      agent: "codex",
      scope: "user",
      captureContent: true
    }, "baseline");
    await writeSnapshot(storeDir, state.snapshot, "codex");

    const skillFile = path.join(homeDir, ".codex", "skills", "unsafe", "SKILL.md");
    await mkdir(path.dirname(skillFile), { recursive: true });
    await writeFile(skillFile, "---\nname: unsafe\n---\n", "utf8");

    const plan = await buildRestorePlan({
      sourceSnapshot: "baseline",
      projectPath,
      homeDir,
      storeDir,
      dryRun: true,
      agent: "codex",
      scope: "user"
    });
    const skillItem = plan.items.find((item) => item.kind === "skill" && item.action === "delete");

    assert.ok(skillItem);
    assert.equal(skillItem.agent, "codex");
    assert.equal(skillItem.sourcePath, "~/.codex/skills/unsafe/SKILL.md");

    const parsed = parseDryRunOutput(JSON.stringify(plan));
    const summary = await applyRestoreItems(
      parsed.items,
      createDefaultApplyExecutor(defaultApplyHandlerRegistry()),
      { failFast: true }
    );

    assert.equal(summary.failed, 0);
    await assert.rejects(() => stat(skillFile), /ENOENT/);
  });

  it("marks Codex TOML MCP changes unsupported while config bytes carry the restore", async () => {
    const { projectPath, homeDir, storeDir } = await makeRestoreSandbox();
    const configPath = path.join(homeDir, ".codex", "config.toml");
    await mkdir(path.dirname(configPath), { recursive: true });
    await writeFile(configPath, [
      "model = \"gpt-5\"",
      "[mcp_servers.docs]",
      "command = \"docs-old\"",
      ""
    ].join("\n"), "utf8");
    const state = await captureCurrentState({
      projectPath,
      homeDir,
      storeDir,
      agent: "codex",
      scope: "user",
      captureContent: true
    }, "baseline");
    await writeSnapshot(storeDir, state.snapshot, "codex");

    await writeFile(configPath, [
      "model = \"gpt-5\"",
      "[mcp_servers.docs]",
      "command = \"docs-new\"",
      ""
    ].join("\n"), "utf8");

    const plan = await buildRestorePlan({
      sourceSnapshot: "baseline",
      projectPath,
      homeDir,
      storeDir,
      dryRun: true,
      agent: "codex",
      scope: "user"
    });

    assert.ok(plan.items.some((item) => item.kind === "agent_config" && item.action === "update"));
    assert.ok(
      plan.unsupportedItems.some(
        (item) =>
          item.kind === "mcp_server" &&
          item.agent === "codex" &&
          item.reason.includes("No supported restore action")
      )
    );
  });

  it("writes agent config content exactly without appending a newline", async () => {
    const { homeDir } = await makeRestoreSandbox();
    const configPath = path.join(homeDir, ".codex", "config.toml");
    await applyAgentConfig({
      itemId: "config",
      path: "~/.codex/config.toml",
      type: "agent_config",
      source: "~/.codex/config.toml",
      dest: configPath,
      action: "update",
      status: "pending",
      executionOrder: 1,
      rollbackState: null,
      targetContent: "model = \"gpt-5\"",
      canRollback: true
    });

    assert.equal(await readFile(configPath, "utf8"), "model = \"gpt-5\"");
  });
});

// ── Tests for per-item undo dispatch ──────────────────────────────

describe("noopUndoHandler — default no-op for non-reversible items", () => {
  it("does not throw when called with a valid item", async () => {
    const item: RestoreItem = {
      itemId: "test-1",
      path: "/tmp/test",
      type: "permission",
      source: "/tmp/test",
      dest: "/tmp/test",
      status: "applied",
      executionOrder: 1,
      rollbackState: null,
      canRollback: false,
      applyAt: "2026-05-12T12:00:00.000Z"
    };
    await assert.doesNotReject(noopUndoHandler(item));
  });

  it("does not throw even with canRollback=true (handler is always safe)", async () => {
    const item: RestoreItem = {
      itemId: "test-2",
      path: "/tmp/test",
      type: "mcp_server",
      source: "/tmp/test",
      dest: "/tmp/test",
      status: "applied",
      executionOrder: 1,
      rollbackState: { previousValue: "abc" },
      canRollback: true,
      applyAt: "2026-05-12T12:00:00.000Z"
    };
    await assert.doesNotReject(noopUndoHandler(item));
  });

  it("performs no side effects on the item", async () => {
    const item: RestoreItem = {
      itemId: "no-side-effects",
      path: "/tmp/test",
      type: "hook",
      source: "/tmp/test",
      dest: "/tmp/test",
      status: "applied",
      executionOrder: 1,
      rollbackState: { someData: "keep-me" },
      canRollback: false,
      applyAt: "2026-05-12T12:00:00.000Z"
    };
    const before = { ...item };
    await noopUndoHandler(item);
    // Item should be completely unchanged
    assert.equal(item.itemId, before.itemId);
    assert.equal(item.status, before.status);
    assert.equal(item.rollbackState, before.rollbackState);
  });
});

describe("restorePreviousStateUndoHandler — rollbackState-based undo", () => {
  it("is a no-op when rollbackState is null", async () => {
    const item: RestoreItem = {
      itemId: "no-state",
      path: "/tmp/test",
      type: "env_key",
      source: "/tmp/test",
      dest: "/tmp/test",
      status: "applied",
      executionOrder: 1,
      rollbackState: null,
      canRollback: true,
      applyAt: "2026-05-12T12:00:00.000Z"
    };
    await assert.doesNotReject(restorePreviousStateUndoHandler(item));
    // No side effects
    assert.equal(item.rollbackState, null);
  });

  it("is a no-op when rollbackState is undefined", async () => {
    const item: RestoreItem = {
      itemId: "no-state-2",
      path: "/tmp/test",
      type: "env_key",
      source: "/tmp/test",
      dest: "/tmp/test",
      status: "applied",
      executionOrder: 1,
      rollbackState: undefined as unknown as null,
      canRollback: true,
      applyAt: "2026-05-12T12:00:00.000Z"
    };
    await assert.doesNotReject(restorePreviousStateUndoHandler(item));
  });

  it("does not throw when rollbackState has data", async () => {
    const item: RestoreItem = {
      itemId: "has-state",
      path: "/tmp/test",
      type: "env_key",
      source: "/tmp/test",
      dest: "/tmp/test",
      status: "applied",
      executionOrder: 1,
      rollbackState: { previousValue: "MY_KEY=value" },
      canRollback: true,
      applyAt: "2026-05-12T12:00:00.000Z"
    };
    await assert.doesNotReject(restorePreviousStateUndoHandler(item));
    // rollbackState should still be intact (handler validates but doesn't mutate)
    assert.deepEqual(item.rollbackState, { previousValue: "MY_KEY=value" });
  });
});

describe("createDefaultUndoExecutor — type dispatch from registry", () => {
  function makeDispatchItem(
    itemId: string,
    type: string,
    overrides: Partial<RestoreItem> = {}
  ): RestoreItem {
    return {
      itemId,
      path: `~/.codex/${itemId}`,
      type,
      source: `~/.codex/${itemId}`,
      dest: `~/.codex/${itemId}`,
      status: "applied",
      executionOrder: 1,
      rollbackState: null,
      canRollback: true,
      applyAt: "2026-05-12T12:00:00.000Z",
      ...overrides
    };
  }

  it("dispatches to the correct handler based on item.type", async () => {
    const handledTypes: string[] = [];
    const registry: UndoHandlerRegistry = {
      mcp_server: async (item) => { handledTypes.push(`mcp:${item.itemId}`); },
      env_key: async (item) => { handledTypes.push(`env:${item.itemId}`); }
    };

    const executor = createDefaultUndoExecutor(registry);
    const items = [
      makeDispatchItem("mcp-1", "mcp_server"),
      makeDispatchItem("env-1", "env_key")
    ];

    for (const item of items) {
      await executor(item);
    }

    assert.deepEqual(handledTypes, ["mcp:mcp-1", "env:env-1"]);
  });

  it("defaults to no-op for unregistered types", async () => {
    const callCount = { count: 0 };
    const registry: UndoHandlerRegistry = {
      mcp_server: async () => { callCount.count++; }
    };

    const executor = createDefaultUndoExecutor(registry);
    const item = makeDispatchItem("unknown-1", "unknown_type");

    await executor(item);
    assert.equal(callCount.count, 0, "unregistered type should not call any handler");
  });

  it("skips execution when canRollback is false even if handler is registered", async () => {
    const callCount = { count: 0 };
    const registry: UndoHandlerRegistry = {
      mcp_server: async () => { callCount.count++; }
    };

    const executor = createDefaultUndoExecutor(registry);
    const item = makeDispatchItem("no-rollback", "mcp_server", { canRollback: false });

    await executor(item);
    assert.equal(callCount.count, 0, "canRollback=false should skip even registered handlers");
  });

  it("works with multiple items of the same type", async () => {
    const handled: string[] = [];
    const registry: UndoHandlerRegistry = {
      env_key: async (item) => { handled.push(item.itemId); }
    };

    const executor = createDefaultUndoExecutor(registry);
    const items = [
      makeDispatchItem("a", "env_key"),
      makeDispatchItem("b", "env_key"),
      makeDispatchItem("c", "env_key")
    ];

    for (const item of items) {
      await executor(item);
    }

    assert.deepEqual(handled, ["a", "b", "c"]);
  });

  it("returns an UndoExecutor compatible with rollbackAppliedItems", async () => {
    const handled: string[] = [];
    const registry: UndoHandlerRegistry = {
      env_key: async (item) => { handled.push(item.itemId); }
    };

    const executor = createDefaultUndoExecutor(registry);
    const items = [
      makeDispatchItem("env-1", "env_key", { executionOrder: 1 }),
      makeDispatchItem("env-2", "env_key", { executionOrder: 2 })
    ];

    const summary = await rollbackAppliedItems(items, executor);

    // Reverse order: env-2, env-1
    assert.deepEqual(handled, ["env-2", "env-1"]);
    assert.equal(summary.undone, 2);
    assert.equal(summary.failed, 0);
  });

  it("passes the full item to the handler including rollbackState", async () => {
    let capturedItem: RestoreItem | undefined;
    const registry: UndoHandlerRegistry = {
      mcp_server: async (item) => { capturedItem = item; }
    };

    const executor = createDefaultUndoExecutor(registry);
    const item = makeDispatchItem("full-access", "mcp_server", {
      rollbackState: { previousCommand: "npx serve" },
      executionOrder: 7,
      path: "/custom/mcp.json",
      source: "/snapshots/source",
      dest: "/custom/mcp.json"
    });

    await executor(item);

    assert.ok(capturedItem, "handler should receive the item");
    assert.equal(capturedItem!.itemId, "full-access");
    assert.equal(capturedItem!.executionOrder, 7);
    assert.deepEqual(capturedItem!.rollbackState, { previousCommand: "npx serve" });
  });
});

describe("defaultUndoHandlerRegistry — built-in type mappings", () => {
  it("contains entries for all known restore types", () => {
    const registry = defaultUndoHandlerRegistry();
    assert.ok(typeof registry.env_key === "function", "env_key should have a handler");
    assert.ok(typeof registry.mcp_server === "function", "mcp_server should have a handler");
    assert.ok(typeof registry.skill === "function", "skill should have a handler");
    assert.ok(typeof registry.agent_config === "function", "agent_config should have a handler");
  });

  it("maps all types to restorePreviousContentUndoHandler", () => {
    const registry = defaultUndoHandlerRegistry();
    // All registered types now use the content-restore undo handler
    const handlerTypes = ["agent_config", "agent_instruction", "mcp_server", "permission", "hook", "skill", "env_key", "env", "symlink"];
    for (const type of handlerTypes) {
      assert.equal(typeof registry[type], "function", `${type} should have a handler`);
    }
    // unsupported remains no-op
    assert.equal(registry.unsupported, noopUndoHandler);
  });

  it("returns noop for undefined/unknown type lookups", () => {
    const registry = defaultUndoHandlerRegistry();
    // Accessing unknown type returns undefined, which createDefaultUndoExecutor handles as no-op
    assert.equal((registry as Record<string, unknown>).unknown_type, undefined);
  });

  it("is compatible with createDefaultUndoExecutor and rollbackAppliedItems", async () => {
    const registry = defaultUndoHandlerRegistry();
    const executor = createDefaultUndoExecutor(registry);

    const items: RestoreItem[] = [
      {
        itemId: "env-1",
        path: "/tmp/env",
        type: "env_key",
        source: "/tmp/env",
        dest: "/tmp/env",
        status: "applied",
        executionOrder: 1,
        rollbackState: { previousValue: "OLD_KEY=old" },
        canRollback: true,
        applyAt: "2026-05-12T12:00:00.000Z"
      },
      {
        itemId: "hook-1",
        path: "/tmp/hook",
        type: "hook",
        source: "/tmp/hook",
        dest: "/tmp/hook",
        status: "applied",
        executionOrder: 2,
        rollbackState: null,
        canRollback: false,
        applyAt: "2026-05-12T12:00:00.000Z"
      }
    ];

    const summary = await rollbackAppliedItems(items, executor);

    // hook-1 is non-reversible (canRollback=false), skipped
    assert.equal(summary.undone, 1); // only env-1
    assert.equal(summary.skipped, 1); // hook-1 skipped as non-reversible
    assert.equal(summary.failed, 0);
  });
});

describe("createDefaultUndoExecutor — edge cases", () => {
  it("handles an empty registry without erroring", async () => {
    const executor = createDefaultUndoExecutor({});
    const item: RestoreItem = {
      itemId: "any",
      path: "/tmp/x",
      type: "anything",
      source: "/tmp/x",
      dest: "/tmp/x",
      status: "applied",
      executionOrder: 1,
      rollbackState: null,
      canRollback: true,
      applyAt: "2026-05-12T12:00:00.000Z"
    };
    await assert.doesNotReject(executor(item));
  });

  it("can be extended with custom handlers at runtime", async () => {
    const handled: string[] = [];
    const registry: UndoHandlerRegistry = {
      // Extend with a custom type
      custom_type: async (item) => { handled.push(item.itemId); }
    };

    const executor = createDefaultUndoExecutor(registry);
    await executor({
      itemId: "custom-1",
      path: "/tmp/c",
      type: "custom_type",
      source: "/tmp/c",
      dest: "/tmp/c",
      status: "applied",
      executionOrder: 1,
      rollbackState: null,
      canRollback: true,
      applyAt: "2026-05-12T12:00:00.000Z"
    });

    assert.deepEqual(handled, ["custom-1"]);
  });

  it("handler errors propagate for proper failure tracking in rollbackAppliedItems", async () => {
    const registry: UndoHandlerRegistry = {
      fragile: async (_item) => { throw new Error("handler crashed"); }
    };

    const executor = createDefaultUndoExecutor(registry);
    const items: RestoreItem[] = [
      {
        itemId: "fragile-1",
        path: "/tmp/x",
        type: "fragile",
        source: "/tmp/x",
        dest: "/tmp/x",
        status: "applied",
        executionOrder: 1,
        rollbackState: null,
        canRollback: true,
        applyAt: "2026-05-12T12:00:00.000Z"
      }
    ];

    const summary = await rollbackAppliedItems(items, executor);

    assert.equal(summary.failed, 1);
    assert.equal(summary.results[0].status, "failed");
    assert.match(summary.results[0].reason ?? "", /handler crashed/);
  });
});

// ── Tests for rollbackAppliedItems (dedicated section) ───────────────

describe("rollbackAppliedItems — reverse-iteration rollback execution", () => {
  function makeAppliedItem(
    itemId: string,
    overrides: Partial<RestoreItem> = {}
  ): RestoreItem {
    return {
      itemId,
      path: `~/.codex/${itemId}`,
      type: "env_key",
      source: `~/.codex/${itemId}`,
      dest: `~/.codex/${itemId}`,
      status: "applied",
      executionOrder: 1,
      rollbackState: null,
      canRollback: true,
      applyAt: "2026-05-12T12:00:00.000Z",
      ...overrides
    };
  }

  it("iterates items in reverse execution order (LIFO)", async () => {
    const undone: string[] = [];
    const executor: UndoExecutor = async (item) => { undone.push(item.itemId); };

    const items = [
      makeAppliedItem("first", { executionOrder: 1 }),
      makeAppliedItem("second", { executionOrder: 2 }),
      makeAppliedItem("third", { executionOrder: 3 })
    ];

    await rollbackAppliedItems(items, executor);

    assert.deepEqual(undone, ["third", "second", "first"]);
  });

  it("only processes items with status === 'applied'", async () => {
    const undone: string[] = [];
    const executor: UndoExecutor = async (item) => { undone.push(item.itemId); };

    const items = [
      makeAppliedItem("applied-1", { executionOrder: 1 }),
      makeAppliedItem("failed-1", { executionOrder: 2, status: "failed", canRollback: true }),
      makeAppliedItem("skipped-1", { executionOrder: 3, status: "skipped", canRollback: true }),
      makeAppliedItem("unsupported-1", { executionOrder: 4, status: "unsupported", canRollback: false }),
      makeAppliedItem("applied-2", { executionOrder: 5 })
    ];

    const summary = await rollbackAppliedItems(items, executor);

    // Only "applied" status items should be undone
    assert.deepEqual(undone, ["applied-2", "applied-1"]);
    assert.equal(summary.undone, 2);
    assert.equal(summary.skipped, 0); // skipped via status filter != summary.skipped
  });

  it("skips items with canRollback=false even when status is applied", async () => {
    const undone: string[] = [];
    const executor: UndoExecutor = async (item) => { undone.push(item.itemId); };

    const items = [
      makeAppliedItem("reversible-1", { executionOrder: 1, canRollback: true }),
      makeAppliedItem("non-reversible-1", { executionOrder: 2, canRollback: false }),
      makeAppliedItem("reversible-2", { executionOrder: 3, canRollback: true })
    ];

    const summary = await rollbackAppliedItems(items, executor);

    assert.deepEqual(undone, ["reversible-2", "reversible-1"]);
    assert.equal(summary.undone, 2);
    assert.equal(summary.skipped, 1);
    assert.equal(summary.results.filter((r) => r.status === "skipped").length, 1);
    assert.match(
      summary.results.find((r) => r.status === "skipped")?.reason ?? "",
      /does not support rollback/
    );
  });

  it("transitions item status from 'applied' to 'pending' after successful undo", async () => {
    const executor: UndoExecutor = async () => { /* no-op */ };
    const item = makeAppliedItem("status-transition", { rollbackState: { prev: "val" } });

    await rollbackAppliedItems([item], executor);

    assert.equal(item.status, "pending");
    assert.equal(item.rollbackState, null);
  });

  it("handles empty applied items array as no-op (empty registry edge case)", async () => {
    const executor: UndoExecutor = async () => { throw new Error("should not be called"); };

    const summary = await rollbackAppliedItems([], executor);

    assert.equal(summary.total, 0);
    assert.equal(summary.undone, 0);
    assert.equal(summary.skipped, 0);
    assert.equal(summary.failed, 0);
    assert.deepEqual(summary.results, []);
  });

  it("handles all-items-failed scenario: empty applied list means no-op rollback (all-failed edge case)", async () => {
    // Items that were in the plan but ALL failed — none are applied
    const items: RestoreItem[] = [
      makeAppliedItem("failed-1", { executionOrder: 1, status: "failed" }),
      makeAppliedItem("failed-2", { executionOrder: 2, status: "failed" })
    ];
    let callCount = 0;
    const executor: UndoExecutor = async () => { callCount++; };

    const summary = await rollbackAppliedItems(items, executor);

    // Only items with status === "applied" are processed — both are "failed"
    assert.equal(summary.total, 2); // total counts input length
    assert.equal(summary.undone, 0);
    assert.equal(callCount, 0, "undo executor should never be called");
  });

  it("records error on item and summary when undo executor throws", async () => {
    const executor: UndoExecutor = async () => { throw new Error("undo failed"); };
    const item = makeAppliedItem("throws-on-undo");

    const summary = await rollbackAppliedItems([item], executor);

    assert.equal(item.status, "applied"); // stays applied on failure
    assert.match(item.errorMessage ?? "", /Rollback failed: undo failed/);
    assert.equal(summary.failed, 1);
    assert.equal(summary.results[0].status, "failed");
    assert.match(summary.results[0].reason ?? "", /undo failed/);
  });

  it("returns summary with correct counts for mixed results", async () => {
    let callCount = 0;
    const executor: UndoExecutor = async (item) => {
      callCount++;
      if (item.itemId === "fragile") throw new Error("broke");
    };

    const items = [
      makeAppliedItem("good-1", { executionOrder: 1, canRollback: true }),
      makeAppliedItem("fragile", { executionOrder: 2, canRollback: true }),
      makeAppliedItem("non-reversible", { executionOrder: 3, canRollback: false }),
      makeAppliedItem("good-2", { executionOrder: 4, canRollback: true })
    ];

    const summary = await rollbackAppliedItems(items, executor);

    // Reverse order: good-2 (ok), non-reversible (skip), fragile (fail), good-1 (should run)
    assert.equal(summary.total, 4);
    assert.equal(summary.undone, 2);    // good-2 and good-1
    assert.equal(summary.skipped, 1);   // non-reversible
    assert.equal(summary.failed, 1);    // fragile
  });
});

// ── Tests for clearAppliedItems ─────────────────────────────────────

describe("clearAppliedItems — clears tracked applied-items list", () => {
  it("sets appliedItems to an empty array", () => {
    const summary: ApplySummary = {
      total: 3,
      successful: 3,
      failed: 0,
      skipped: 0,
      unsupported: 0,
      failures: [],
      statusRegistry: {},
      appliedItems: [
        { itemId: "a", path: "", type: "env", source: "", dest: "", status: "applied", executionOrder: 1, rollbackState: null, canRollback: true },
        { itemId: "b", path: "", type: "env", source: "", dest: "", status: "applied", executionOrder: 2, rollbackState: null, canRollback: true }
      ]
    };

    clearAppliedItems(summary);

    assert.deepEqual(summary.appliedItems, []);
  });

it("is idempotent when called on an already-empty list", () => {
    const summary: ApplySummary = {
      total: 0,
      successful: 0,
      failed: 0,
      skipped: 0,
      unsupported: 0,
      failures: [],
      statusRegistry: {},
      appliedItems: []
    };

    clearAppliedItems(summary);

    assert.deepEqual(summary.appliedItems, []);
  });
});

// ── Tests for applyRestoreItems — per-item error handling ────────────────

describe("applyRestoreItems — per-item error handling", () => {
  function makePendingItem(
    itemId: string,
    overrides: Partial<RestoreItem> = {}
  ): RestoreItem {
    return {
      itemId,
      path: "/tmp/test",
      type: "env_key",
      source: "/tmp/test",
      dest: "/tmp/test",
      status: "pending" as const,
      executionOrder: 1,
      rollbackState: null,
      canRollback: true,
      ...overrides
    };
  }

  function successExecutor(): RestoreExecutor {
    return async (_item: RestoreItem) => { /* no-op */ };
  }

  function failOnceExecutor(failItemId: string): RestoreExecutor {
    return async (item: RestoreItem) => {
      if (item.itemId === failItemId) {
        throw new Error(`simulated failure for ${item.itemId}`);
      }
    };
  }

  it("applies all items successfully when no errors occur", async () => {
    const items = [
      makePendingItem("item-1", { executionOrder: 1 }),
      makePendingItem("item-2", { executionOrder: 2 }),
      makePendingItem("item-3", { executionOrder: 3 })
    ];

    const summary = await applyRestoreItems(items, successExecutor(), { failFast: false });

    assert.equal(summary.total, 3);
    assert.equal(summary.successful, 3);
    assert.equal(summary.failed, 0);
    assert.equal(summary.skipped, 0);
    assert.equal(summary.unsupported, 0);
    assert.equal(summary.appliedItems.length, 3);
    assert.deepEqual(
      summary.appliedItems.map((i) => i.itemId),
      ["item-1", "item-2", "item-3"]
    );
    assert.equal(items[0].status, "applied");
    assert.equal(items[1].status, "applied");
    assert.equal(items[2].status, "applied");
  });

  it("continues processing after a single item failure (default best-effort)", async () => {
    const items = [
      makePendingItem("ok-1", { executionOrder: 1 }),
      makePendingItem("fail-2", { executionOrder: 2 }),
      makePendingItem("ok-3", { executionOrder: 3 })
    ];

    const summary = await applyRestoreItems(items, failOnceExecutor("fail-2"), { failFast: false });

    // ok-1 applied, fail-2 failed, ok-3 still applied (best-effort)
    assert.equal(summary.total, 3);
    assert.equal(summary.successful, 2);
    assert.equal(summary.failed, 1);
    assert.equal(items[0].status, "applied");
    assert.equal(items[1].status, "failed");
    assert.equal(items[2].status, "applied");
    assert.ok(items[1].errorMessage, "failed item should have errorMessage");
    assert.match(items[1].errorMessage!, /simulated failure for fail-2/);
  });

  it("stops immediately on first failure when failFast is true", async () => {
    const items = [
      makePendingItem("ok-1", { executionOrder: 1 }),
      makePendingItem("fail-2", { executionOrder: 2 }),
      makePendingItem("never-reached", { executionOrder: 3 })
    ];

    const summary = await applyRestoreItems(items, failOnceExecutor("fail-2"), { failFast: true });

    // ok-1 applied, fail-2 failed, never-reached should be skipped
    assert.equal(summary.successful, 1);
    assert.equal(summary.failed, 1);
    assert.equal(summary.skipped, 1);
    assert.equal(items[0].status, "applied");
    assert.equal(items[1].status, "failed");
    assert.equal(items[2].status, "skipped");
    assert.equal(items[2].skipReason, "Execution stopped before this item");
  });

  it("captures failure details in summary.failures array", async () => {
    const items = [
      makePendingItem("fail-a", { executionOrder: 1 }),
      makePendingItem("fail-b", { executionOrder: 2 }),
      makePendingItem("ok-c", { executionOrder: 3 })
    ];

    // Fail both fail-a and fail-b
    let callCount = 0;
    const executor: RestoreExecutor = async (item) => {
      callCount++;
      if (item.itemId.startsWith("fail-")) {
        throw new Error(`${item.itemId} exploded`);
      }
    };

    const summary = await applyRestoreItems(items, executor, { failFast: false });

    assert.equal(summary.failed, 2);
    assert.equal(summary.failures.length, 2);
    assert.equal(summary.failures[0].itemId, "fail-a");
    assert.match(summary.failures[0].reason, /fail-a exploded/);
    assert.equal(summary.failures[1].itemId, "fail-b");
    assert.match(summary.failures[1].reason, /fail-b exploded/);
  });

  it("respects unsupported items without executing them", async () => {
    let executedCount = 0;
    const executor: RestoreExecutor = async () => { executedCount++; };

    const items = [
      makePendingItem("supported-1", { executionOrder: 1 }),
      makePendingItem("unsupported-1", { executionOrder: 2, status: "unsupported", canRollback: false }),
      makePendingItem("supported-2", { executionOrder: 3 })
    ];

    const summary = await applyRestoreItems(items, executor, { failFast: false });

    assert.equal(summary.successful, 2);
    assert.equal(summary.unsupported, 1);
    assert.equal(executedCount, 2); // only supported items executed
  });

  it("respects pre-skipped items without executing them", async () => {
    let executedCount = 0;
    const executor: RestoreExecutor = async () => { executedCount++; };

    const items = [
      makePendingItem("skipped-1", { executionOrder: 1, status: "skipped", canRollback: false }),
      makePendingItem("normal-1", { executionOrder: 2 })
    ];

    const summary = await applyRestoreItems(items, executor, { failFast: false });

    assert.equal(summary.successful, 1);
    assert.equal(summary.skipped, 1);
    assert.equal(executedCount, 1);
  });

  it("returns empty appliedItems when all items fail", async () => {
    const executor: RestoreExecutor = async () => { throw new Error("always fail"); };
    const items = [
      makePendingItem("all-fail-1", { executionOrder: 1 }),
      makePendingItem("all-fail-2", { executionOrder: 2 })
    ];

    const summary = await applyRestoreItems(items, executor, { failFast: false });

    assert.equal(summary.successful, 0);
    assert.equal(summary.failed, 2);
    assert.deepEqual(summary.appliedItems, []);
  });

  it("sets applyAt timestamp on successfully applied items", async () => {
    const items = [
      makePendingItem("ts-1", { executionOrder: 1 }),
      makePendingItem("ts-2", { executionOrder: 2 })
    ];

    const summary = await applyRestoreItems(items, successExecutor(), { failFast: false });

    assert.equal(summary.successful, 2);
    assert.ok(items[0].applyAt, "item should have applyAt timestamp");
    assert.ok(items[1].applyAt, "item should have applyAt timestamp");
    assert.doesNotThrow(() => new Date(items[0].applyAt!).toISOString());
  });

  it("handles executor throwing non-Error values gracefully", async () => {
    const executor: RestoreExecutor = async () => { throw "string error"; };
    const items = [makePendingItem("string-error", { executionOrder: 1 })];

    const summary = await applyRestoreItems(items, executor, { failFast: false });

    assert.equal(summary.failed, 1);
    assert.equal(items[0].errorMessage, "string error");
  });
});

describe("applyRestoreItems — failed/skipped status tracking", () => {
  function makePendingItem(
    itemId: string,
    overrides: Partial<RestoreItem> = {}
  ): RestoreItem {
    return {
      itemId,
      path: "/tmp/test",
      type: "env_key",
      source: "/tmp/test",
      dest: "/tmp/test",
      status: "pending" as const,
      executionOrder: 1,
      rollbackState: null,
      canRollback: true,
      ...overrides
    };
  }

  it("records status 'failed' and errorMessage when executor throws", async () => {
    const executor: RestoreExecutor = async () => {
      throw new Error("Permission denied");
    };
    const item = makePendingItem("fail-me");

    await applyRestoreItems([item], executor, { failFast: false });

    assert.equal(item.status, "failed");
    assert.equal(item.errorMessage, "Permission denied");
  });

  it("records status 'skipped' with reason when failFast stops early", async () => {
    let callCount = 0;
    const executor: RestoreExecutor = async (item) => {
      callCount++;
      if (item.itemId === "fail-2") throw new Error("second fails");
    };

    const items = [
      makePendingItem("item-1", { executionOrder: 1 }),
      makePendingItem("fail-2", { executionOrder: 2 }),
      makePendingItem("item-3", { executionOrder: 3 })
    ];

    const summary = await applyRestoreItems(items, executor, { failFast: true });

    // item-1 applied, fail-2 failed, item-3 skipped
    assert.equal(items[0].status, "applied");
    assert.equal(items[1].status, "failed");
    assert.equal(items[2].status, "skipped");
    assert.equal(items[2].skipReason, "Execution stopped before this item");
    assert.equal(summary.successful, 1);
    assert.equal(summary.failed, 1);
    assert.equal(summary.skipped, 1);
  });

  it("does not mark non-pending items as skipped during failFast stop", async () => {
    const executor: RestoreExecutor = async (_item) => {
      throw new Error("all fail");
    };

    const items = [
      makePendingItem("fail-1", { executionOrder: 1 }),
      // Item-2 starts with non-pending status; should not be overwritten
      makePendingItem("skip-2", { executionOrder: 2, status: "skipped" as const, skipReason: "User opted out" }),
      makePendingItem("unsup-3", { executionOrder: 3, status: "unsupported" as const, skipReason: "Not supported" })
    ];

    const summary = await applyRestoreItems(items, executor, { failFast: true });

    // fail-1 gets failed
    assert.equal(items[0].status, "failed");
    // skip-2 preserves its status and reason
    assert.equal(items[1].status, "skipped");
    assert.equal(items[1].skipReason, "User opted out");
    // unsup-3 preserves its status
    assert.equal(items[2].status, "unsupported");
    assert.equal(items[2].skipReason, "Not supported");
  });

  it("sets applyAt timestamp on applied items", async () => {
    const executor: RestoreExecutor = async () => {};
    const item = makePendingItem("ts-check");

    await applyRestoreItems([item], executor, { failFast: false });

    assert.equal(item.status, "applied");
    assert.ok(item.applyAt, "applyAt should be set");
    assert.match(item.applyAt!, /\d{4}-\d{2}-\d{2}T/);
  });

  it("preserves 'failed' status when executor throws and failFast is false", async () => {
    const executor: RestoreExecutor = async () => {
      throw new Error("generic failure");
    };
    const items = [
      makePendingItem("fail-a", { executionOrder: 1 }),
      makePendingItem("ok-b", { executionOrder: 2 })
    ];

    const summary = await applyRestoreItems(items, executor, { failFast: false });

    assert.equal(items[0].status, "failed");
    assert.equal(items[0].errorMessage, "generic failure");
    // ok-b is also executed (failFast=false) and its executor also throws
    assert.equal(items[1].status, "failed");
    assert.equal(items[1].errorMessage, "generic failure");
    assert.equal(summary.failed, 2);
  });

  it("tracks skipped counter for items pre-marked as skipped", async () => {
    const executor: RestoreExecutor = async () => { throw new Error("should not call"); };
    const item = makePendingItem("already-skipped", {
      status: "skipped" as const,
      skipReason: "Pre-emptively skipped",
      executionOrder: 1
    });

    const summary = await applyRestoreItems([item], executor, { failFast: false });

    // The executor should not be called — item is skipped before try
    assert.equal(item.status, "skipped");
    assert.equal(summary.skipped, 1);
    assert.equal(summary.failed, 0);
  });

  it("tracks unsupported counter for items pre-marked as unsupported", async () => {
    const executor: RestoreExecutor = async () => { throw new Error("should not call"); };
    const item = makePendingItem("already-unsup", {
      status: "unsupported" as const,
      skipReason: "Type not supported yet",
      executionOrder: 1
    });

    const summary = await applyRestoreItems([item], executor, { failFast: false });

    assert.equal(item.status, "unsupported");
    assert.equal(summary.unsupported, 1);
    assert.equal(summary.failed, 0);
  });
});

// ── Tests for statusRegistry — per-item mutable state tracking ──────

describe("applyRestoreItems — statusRegistry tracks per-item state (Sub-AC 2b)", () => {
  function makePendingItem(
    itemId: string,
    overrides: Partial<RestoreItem> = {}
  ): RestoreItem {
    return {
      itemId,
      path: "/tmp/test",
      type: "env_key",
      source: "/tmp/test",
      dest: "/tmp/test",
      status: "pending" as const,
      executionOrder: 1,
      rollbackState: null,
      canRollback: true,
      ...overrides
    };
  }

  function successExecutor(): RestoreExecutor {
    return async (_item: RestoreItem) => { /* no-op */ };
  }

  function failExecutor(failItemId?: string): RestoreExecutor {
    return async (item: RestoreItem) => {
      if (!failItemId || item.itemId === failItemId) {
        throw new Error(`simulated failure for ${item.itemId}`);
      }
    };
  }

  it("maps all itemIds to 'applied' when all items succeed", async () => {
    const items = [
      makePendingItem("a", { executionOrder: 1 }),
      makePendingItem("b", { executionOrder: 2 }),
      makePendingItem("c", { executionOrder: 3 })
    ];

    const summary = await applyRestoreItems(items, successExecutor(), { failFast: false });

    assert.deepEqual(summary.statusRegistry, {
      a: "applied",
      b: "applied",
      c: "applied"
    });
  });

  it("maps failed items to 'failed' after executor throws", async () => {
    const items = [
      makePendingItem("ok", { executionOrder: 1 }),
      makePendingItem("fragile", { executionOrder: 2 }),
      makePendingItem("also-ok", { executionOrder: 3 })
    ];

    const summary = await applyRestoreItems(items, failExecutor("fragile"), { failFast: false });

    assert.equal(summary.statusRegistry["ok"], "applied");
    assert.equal(summary.statusRegistry["fragile"], "failed");
    assert.equal(summary.statusRegistry["also-ok"], "applied");
  });

  it("maps pre-skipped items to 'skipped' in the registry", async () => {
    const items = [
      makePendingItem("a", { executionOrder: 1, status: "skipped", canRollback: false }),
      makePendingItem("b", { executionOrder: 2 })
    ];

    const summary = await applyRestoreItems(items, successExecutor(), { failFast: false });

    assert.equal(summary.statusRegistry["a"], "skipped");
    assert.equal(summary.statusRegistry["b"], "applied");
  });

  it("maps pre-unsupported items to 'unsupported' in the registry", async () => {
    const items = [
      makePendingItem("x", { executionOrder: 1 }),
      makePendingItem("unsup", { executionOrder: 2, status: "unsupported", canRollback: false })
    ];

    const summary = await applyRestoreItems(items, successExecutor(), { failFast: false });

    assert.equal(summary.statusRegistry["x"], "applied");
    assert.equal(summary.statusRegistry["unsup"], "unsupported");
  });

  it("marks skipped items in registry when failFast stops early", async () => {
    const items = [
      makePendingItem("first", { executionOrder: 1 }),
      makePendingItem("fail-on-me", { executionOrder: 2 }),
      makePendingItem("post-fail-1", { executionOrder: 3 }),
      makePendingItem("post-fail-2", { executionOrder: 4 })
    ];

    const summary = await applyRestoreItems(items, failExecutor("fail-on-me"), { failFast: true });

    assert.equal(summary.statusRegistry["first"], "applied");
    assert.equal(summary.statusRegistry["fail-on-me"], "failed");
    assert.equal(summary.statusRegistry["post-fail-1"], "skipped");
    assert.equal(summary.statusRegistry["post-fail-2"], "skipped");
  });

  it("keeps non-pending items unchanged in registry when abort/failFast runs counting pass", async () => {
    const items = [
      makePendingItem("fail-1", { executionOrder: 1 }),
      makePendingItem("already-skip", { executionOrder: 2, status: "skipped", skipReason: "User opted out" }),
      makePendingItem("already-unsup", { executionOrder: 3, status: "unsupported", skipReason: "Not supported" })
    ];

    const summary = await applyRestoreItems(items, failExecutor("fail-1"), { failFast: true });

    // fail-1 failed (the first item tried — visited in main loop)
    assert.equal(summary.statusRegistry["fail-1"], "failed");
    // Items after the failure in execution order but before the failure
    // point in the sorted array — only main-loop-visited items get
    // recorded. "already-skip" was never visited (failFast broke before
    // reaching it) and the post-loop counting pass only handles pending
    // and unsupported → no registry entry for it.
    assert.equal(summary.statusRegistry["already-skip"], undefined);
    // already-unsup gets recorded in the post-loop counting pass
    // (explicit else-if branch for unsupported)
    assert.equal(summary.statusRegistry["already-unsup"], "unsupported");

    // Items themselves preserve their original status
    assert.equal(items[1].status, "skipped");
    assert.equal(items[1].skipReason, "User opted out");
    assert.equal(items[2].status, "unsupported");
    assert.equal(items[2].skipReason, "Not supported");
  });

  it("registry size equals item count after full execution", async () => {
    const items = [
      makePendingItem("a", { executionOrder: 1 }),
      makePendingItem("b", { executionOrder: 2 }),
      makePendingItem("c", { executionOrder: 3 })
    ];

    const summary = await applyRestoreItems(items, successExecutor(), { failFast: false });

    assert.equal(Object.keys(summary.statusRegistry).length, 3);
  });

  it("registry contains entries even when all items fail", async () => {
    const items = [
      makePendingItem("all-1", { executionOrder: 1 }),
      makePendingItem("all-2", { executionOrder: 2 }),
      makePendingItem("all-3", { executionOrder: 3 })
    ];

    const summary = await applyRestoreItems(items, failExecutor(), { failFast: false });

    assert.equal(summary.statusRegistry["all-1"], "failed");
    assert.equal(summary.statusRegistry["all-2"], "failed");
    assert.equal(summary.statusRegistry["all-3"], "failed");
    assert.equal(Object.keys(summary.statusRegistry).length, 3);
  });

  it("registry is not stale after early abort via failFast", async () => {
    const items = [
      makePendingItem("first", { executionOrder: 1 }),
      makePendingItem("dead-on-arrival", { executionOrder: 2 }),
      makePendingItem("never-reached", { executionOrder: 3 })
    ];

    const summary = await applyRestoreItems(items, failExecutor("dead-on-arrival"), { failFast: true });

    // Registry should have 3 entries: applied, failed, skipped
    assert.equal(Object.keys(summary.statusRegistry).length, 3);
    // All three statuses must be correct
    const values = Object.values(summary.statusRegistry);
    assert.ok(values.includes("applied"));
    assert.ok(values.includes("failed"));
    assert.ok(values.includes("skipped"));
  });
});

// ── Tests for applyWithRollback orchestration ───────────────────────

describe("applyWithRollback — apply-then-rollback orchestration", () => {
  function makePendingItem(
    itemId: string,
    overrides: Partial<RestoreItem> = {}
  ): RestoreItem {
    return {
      itemId,
      path: "test-path",
      type: "env_key",
      source: "test-source",
      dest: "test-dest",
      status: "pending" as const,
      executionOrder: 1,
      rollbackState: null,
      canRollback: true,
      ...overrides
    };
  }

  it("returns only applySummary when rollback is not requested", async () => {
    const executor: RestoreExecutor = async () => {};
    const items = [makePendingItem("item-1")];
    const undoExecutor: UndoExecutor = async () => {};

    const result = await applyWithRollback(items, executor, {
      failFast: false,
      rollback: false,
      undoExecutor
    });

    assert.ok(result.applySummary);
    assert.equal(result.rollbackSummary, undefined);
    assert.equal(result.applySummary.successful, 1);
  });

  it("runs rollback when rollback=true and items were applied", async () => {
    const appliedIds: string[] = [];
    const executor: RestoreExecutor = async (item) => {
      appliedIds.push(item.itemId);
    };
    const undoneIds: string[] = [];
    const undoExecutor: UndoExecutor = async (item) => {
      undoneIds.push(item.itemId);
    };

    const items = [
      makePendingItem("item-1", { executionOrder: 1 }),
      makePendingItem("item-2", { executionOrder: 2 }),
      makePendingItem("item-3", { executionOrder: 3 })
    ];

    const result = await applyWithRollback(items, executor, {
      failFast: false,
      rollback: true,
      undoExecutor
    });

    // Items applied in order
    assert.deepEqual(appliedIds, ["item-1", "item-2", "item-3"]);
    // Items undone in reverse order (LIFO)
    assert.deepEqual(undoneIds, ["item-3", "item-2", "item-1"]);
    assert.equal(result.applySummary.successful, 3);
    assert.ok(result.rollbackSummary);
    assert.equal(result.rollbackSummary!.undone, 3);
  });

  it("clears applied items list after rollback completes", async () => {
    const executor: RestoreExecutor = async () => {};
    const undoExecutor: UndoExecutor = async () => {};

    const items = [
      makePendingItem("item-1", { executionOrder: 1 }),
      makePendingItem("item-2", { executionOrder: 2 })
    ];

    const result = await applyWithRollback(items, executor, {
      failFast: false,
      rollback: true,
      undoExecutor
    });

    // After rollback, the applied items list should be cleared
    assert.equal(result.applySummary.appliedItems.length, 0);
  });

  it("returns rollbackSummary with undo results", async () => {
    const executor: RestoreExecutor = async () => {};
    const undoExecutor: UndoExecutor = async () => {};

    const items = [makePendingItem("only-one")];

    const result = await applyWithRollback(items, executor, {
      failFast: false,
      rollback: true,
      undoExecutor
    });

    assert.ok(result.rollbackSummary);
    assert.equal(result.rollbackSummary!.undone, 1);
    assert.equal(result.rollbackSummary!.failed, 0);
    assert.equal(result.rollbackSummary!.results[0].itemId, "only-one");
    assert.equal(result.rollbackSummary!.results[0].status, "undone");
  });

  it("skips rollback when rollback=true but no items were applied (all failed)", async () => {
    const executor: RestoreExecutor = async (_item) => {
      throw new Error("always fail");
    };
    const undoCallCount = { count: 0 };
    const undoExecutor: UndoExecutor = async () => { undoCallCount.count++; };

    const items = [makePendingItem("fail-1")];

    const result = await applyWithRollback(items, executor, {
      failFast: false,
      rollback: true,
      undoExecutor
    });

    assert.equal(result.applySummary.failed, 1);
    assert.equal(result.applySummary.successful, 0);
    // No applied items → rollback should not execute
    assert.equal(result.rollbackSummary, undefined);
    assert.equal(undoCallCount.count, 0);
  });

  it("handles partial rollback failure: one undo fails, others continue (no cascading errors)", async () => {
    const executor: RestoreExecutor = async () => {};
    let callCount = 0;
    const undoExecutor: UndoExecutor = async (item) => {
      callCount++;
      if (item.itemId === "fragile") throw new Error("rollback failed for fragile");
    };

    const items = [
      makePendingItem("item-1", { executionOrder: 1, canRollback: true }),
      makePendingItem("fragile", { executionOrder: 2, canRollback: true }),
      makePendingItem("item-3", { executionOrder: 3, canRollback: true })
    ];

    const result = await applyWithRollback(items, executor, {
      failFast: false,
      rollback: true,
      undoExecutor
    });

    // All 3 items were applied
    assert.equal(result.applySummary.successful, 3);
    // Rollback runs in reverse: item-3 (ok), fragile (fails), item-1 (should still run)
    assert.ok(result.rollbackSummary, "rollback should execute");
    assert.equal(result.rollbackSummary!.undone, 2, "item-3 and item-1 undone");
    assert.equal(result.rollbackSummary!.failed, 1, "fragile failed");
    // The fragile failure does NOT prevent item-1 from being undone
    assert.equal(callCount, 3, "all 3 items were visited by undo executor");
    // appliedItems cleared after rollback
    assert.equal(result.applySummary.appliedItems.length, 0);
  });

  it("handles failFast + rollback: stops apply early then rollbacks what was applied", async () => {
    let callCount = 0;
    const executor: RestoreExecutor = async (item) => {
      callCount++;
      if (item.itemId === "fail-on-2") throw new Error("second fails");
    };
    const undoneIds: string[] = [];
    const undoExecutor: UndoExecutor = async (item) => {
      undoneIds.push(item.itemId);
    };

    const items = [
      makePendingItem("item-1", { executionOrder: 1 }),
      makePendingItem("fail-on-2", { executionOrder: 2 }),
      makePendingItem("item-3", { executionOrder: 3 })
    ];

    const result = await applyWithRollback(items, executor, {
      failFast: true,
      rollback: true,
      undoExecutor
    });

    // item-1 applied, fail-on-2 failed, item-3 skipped
    assert.equal(result.applySummary.successful, 1);
    assert.equal(result.applySummary.failed, 1);
    // Only item-1 should be undone (reverse of applied items)
    assert.deepEqual(undoneIds, ["item-1"]);
    assert.ok(result.rollbackSummary);
    assert.equal(result.rollbackSummary!.undone, 1);
  });
});

// ── Tests for formatApplySummary ────────────────────────────────────

describe("formatApplySummary — human-readable apply result rendering", () => {
  it("renders all-zero summary correctly", () => {
    const summary: ApplySummary = {
      total: 0, successful: 0, failed: 0, skipped: 0, unsupported: 0,
      failures: [], statusRegistry: {}, appliedItems: []
    };

    const output = formatApplySummary(summary);

    assert.match(output, /Successful: 0/);
    assert.match(output, /Failed:     0/);
    assert.match(output, /Skipped:    0/);
    assert.match(output, /Unsupported: 0/);
    assert.match(output, /Total:      0/);
  });

  it("renders mixed counts correctly", () => {
    const summary: ApplySummary = {
      total: 10, successful: 7, failed: 2, skipped: 1, unsupported: 0,
      failures: [
        { itemId: "fail-1", reason: "File not found" },
        { itemId: "fail-2", reason: "Permission denied" }
      ],
      appliedItems: [],
      statusRegistry: {}
    };

    const output = formatApplySummary(summary);

    assert.match(output, /Successful: 7/);
    assert.match(output, /Failed:     2/);
    assert.match(output, /Skipped:    1/);
    assert.match(output, /Unsupported: 0/);
    assert.match(output, /Total:      10/);
    assert.match(output, /\[fail-1\] File not found/);
    assert.match(output, /\[fail-2\] Permission denied/);
  });

  it("omits failures section when there are no failures", () => {
    const summary: ApplySummary = {
      total: 3, successful: 3, failed: 0, skipped: 0, unsupported: 0,
      failures: [], statusRegistry: {}, appliedItems: []
    };

    const output = formatApplySummary(summary);
    assert.doesNotMatch(output, /Failures/);
  });

  it("starts with 'Restore apply results'", () => {
    const summary: ApplySummary = {
      total: 0, successful: 0, failed: 0, skipped: 0, unsupported: 0,
      failures: [], statusRegistry: {}, appliedItems: []
    };

    const output = formatApplySummary(summary);
    assert.ok(output.startsWith("Restore apply results"));
  });
});

// ── Tests for formatRollbackSummary ─────────────────────────────────

describe("formatRollbackSummary — human-readable rollback result rendering", () => {
  it("renders all-zero summary correctly", () => {
    const summary: RollbackSummary = {
      total: 0, undone: 0, skipped: 0, failed: 0, results: []
    };

    const output = formatRollbackSummary(summary);

    assert.match(output, /Undone:  0/);
    assert.match(output, /Skipped: 0/);
    assert.match(output, /Failed:  0/);
    assert.match(output, /Total:   0/);
  });

  it("renders mixed counts with failure details", () => {
    const summary: RollbackSummary = {
      total: 5, undone: 3, skipped: 1, failed: 1,
      results: [
        { itemId: "good-1", status: "undone" },
        { itemId: "good-2", status: "undone" },
        { itemId: "non-rev", status: "skipped", reason: "Item does not support rollback" },
        { itemId: "fail-1", status: "failed", reason: "Rollback failed: permission error" },
        { itemId: "good-3", status: "undone" }
      ]
    };

    const output = formatRollbackSummary(summary);

    assert.match(output, /Undone:  3/);
    assert.match(output, /Skipped: 1/);
    assert.match(output, /Failed:  1/);
    assert.match(output, /Total:   5/);
    assert.match(output, /\[fail-1\] Rollback failed: permission error/);
  });

  it("renders skipped items in a dedicated section", () => {
    const summary: RollbackSummary = {
      total: 2, undone: 1, skipped: 1, failed: 0,
      results: [
        { itemId: "good-1", status: "undone" },
        { itemId: "non-rev", status: "skipped", reason: "No rollback support" }
      ]
    };

    const output = formatRollbackSummary(summary);

    assert.match(output, /Skipped \(non-reversible\):/);
    assert.match(output, /\[non-rev\] No rollback support/);
  });

  it("omits failures section when there are no failures", () => {
    const summary: RollbackSummary = {
      total: 2, undone: 2, skipped: 0, failed: 0,
      results: [
        { itemId: "a", status: "undone" },
        { itemId: "b", status: "undone" }
      ]
    };

    const output = formatRollbackSummary(summary);
    assert.doesNotMatch(output, /Failures/);
  });

  it("omits skipped section when there are no skipped items", () => {
    const summary: RollbackSummary = {
      total: 1, undone: 1, skipped: 0, failed: 0,
      results: [{ itemId: "a", status: "undone" }]
    };

    const output = formatRollbackSummary(summary);
    assert.doesNotMatch(output, /Skipped \(non-reversible\)/);
  });

  it("starts with 'Rollback complete.'", () => {
    const summary: RollbackSummary = {
      total: 0, undone: 0, skipped: 0, failed: 0, results: []
    };
    const output = formatRollbackSummary(summary);
    assert.ok(output.startsWith("Rollback complete."));
  });
});

// ── Tests for applyRestoreItems fail-fast behavior (AC 3) ────────────

describe("applyRestoreItems — failFast behavior", () => {
  function makePendingItem(
    itemId: string,
    overrides: Partial<RestoreItem> = {}
  ): RestoreItem {
    return {
      itemId,
      path: `~/.codex/${itemId}`,
      type: "env_key",
      source: `~/.codex/${itemId}`,
      dest: `~/.codex/${itemId}`,
      status: "pending",
      executionOrder: 1,
      rollbackState: null,
      canRollback: true,
      applyAt: undefined,
      errorMessage: undefined,
      skipReason: undefined,
      ...overrides
    };
  }

  it("default (failFast=false) continues processing remaining items after a failure", async () => {
    const callOrder: string[] = [];
    const executor: RestoreExecutor = async (item) => {
      callOrder.push(item.itemId);
      if (item.itemId === "fail-2") throw new Error("item-2 failed");
    };

    const items = [
      makePendingItem("item-1", { executionOrder: 1 }),
      makePendingItem("fail-2", { executionOrder: 2 }),
      makePendingItem("item-3", { executionOrder: 3 })
    ];

    const summary = await applyRestoreItems(items, executor, { failFast: false });

    // All items were visited by the executor
    assert.deepEqual(callOrder, ["item-1", "fail-2", "item-3"]);
    assert.equal(summary.successful, 2);
    assert.equal(summary.failed, 1);
    assert.equal(summary.skipped, 0);
    assert.equal(summary.appliedItems.length, 2);
  });

  it("failFast=true stops execution immediately on the first failure", async () => {
    const callOrder: string[] = [];
    const executor: RestoreExecutor = async (item) => {
      callOrder.push(item.itemId);
      if (item.itemId === "fail-2") throw new Error("item-2 failed");
    };

    const items = [
      makePendingItem("item-1", { executionOrder: 1 }),
      makePendingItem("fail-2", { executionOrder: 2 }),
      makePendingItem("item-3", { executionOrder: 3 }),
      makePendingItem("item-4", { executionOrder: 4 })
    ];

    const summary = await applyRestoreItems(items, executor, { failFast: true });

    // Executor only called for item-1 and fail-2 — item-3, item-4 not visited
    assert.deepEqual(callOrder, ["item-1", "fail-2"]);
    assert.equal(summary.successful, 1);
    assert.equal(summary.failed, 1);
    // Remaining 2 items should be counted as skipped
    assert.equal(summary.skipped, 2);
    assert.equal(summary.appliedItems.length, 1);
    assert.equal(summary.total, 4);
  });

  it("failFast=true marks remaining pending items as 'skipped' with skipReason", async () => {
    const executor: RestoreExecutor = async (item) => {
      if (item.itemId === "fail-1") throw new Error("intentional fail");
    };

    const items = [
      makePendingItem("item-1", { executionOrder: 1 }),
      makePendingItem("fail-1", { executionOrder: 2 }),
      makePendingItem("post-fail-1", { executionOrder: 3 }),
      makePendingItem("post-fail-2", { executionOrder: 4 })
    ];

    await applyRestoreItems(items, executor, { failFast: true });

    // Items before failure preserved their status
    assert.equal(items[0].status, "applied");
    assert.equal(items[1].status, "failed");
    // Remaining items marked as skipped
    assert.equal(items[2].status, "skipped");
    assert.equal(items[3].status, "skipped");
    // skipReason set for remaining items
    assert.equal(items[2].skipReason, "Execution stopped before this item");
    assert.equal(items[3].skipReason, "Execution stopped before this item");
  });

  it("failFast=true with all items succeeding produces normal full summary", async () => {
    const executor: RestoreExecutor = async () => {};

    const items = [
      makePendingItem("item-1", { executionOrder: 1 }),
      makePendingItem("item-2", { executionOrder: 2 }),
      makePendingItem("item-3", { executionOrder: 3 })
    ];

    const summary = await applyRestoreItems(items, executor, { failFast: true });

    assert.equal(summary.successful, 3);
    assert.equal(summary.failed, 0);
    assert.equal(summary.skipped, 0);
    assert.equal(summary.total, 3);
    assert.equal(summary.appliedItems.length, 3);
  });

  it("failFast=true stops on first item when that item fails", async () => {
    const callOrder: string[] = [];
    const executor: RestoreExecutor = async (item) => {
      callOrder.push(item.itemId);
      if (item.itemId === "fail-first") throw new Error("first item fails");
    };

    const items = [
      makePendingItem("fail-first", { executionOrder: 1 }),
      makePendingItem("would-run", { executionOrder: 2 })
    ];

    const summary = await applyRestoreItems(items, executor, { failFast: true });

    assert.deepEqual(callOrder, ["fail-first"]);
    assert.equal(summary.successful, 0);
    assert.equal(summary.failed, 1);
    assert.equal(summary.skipped, 1);
    assert.equal(summary.appliedItems.length, 0);
  });

  it("failFast=true records the failure reason in summary.failures", async () => {
    const executor: RestoreExecutor = async () => {
      throw new Error("disk full");
    };

    const items = [makePendingItem("disk-error", { executionOrder: 1 })];

    const summary = await applyRestoreItems(items, executor, { failFast: true });

    assert.equal(summary.failures.length, 1);
    assert.equal(summary.failures[0].itemId, "disk-error");
    assert.match(summary.failures[0].reason, /disk full/);
  });

  it("failFast=true does not mark non-pending items as extra skipped", async () => {
    const executor: RestoreExecutor = async (item) => {
      if (item.itemId === "fail-1") throw new Error("fail");
    };

    const items = [
      makePendingItem("item-1", { executionOrder: 1 }),
      makePendingItem("fail-1", { executionOrder: 2 }),
      // An item already marked unsupported before the loop
      makePendingItem("unsupported-1", { executionOrder: 3, status: "unsupported", canRollback: false }),
      makePendingItem("post-fail", { executionOrder: 4 })
    ];

    const summary = await applyRestoreItems(items, executor, { failFast: true });

    // item-1 applied, fail-1 failed, unsupported-1 stays unsupported, post-fail skipped
    assert.equal(summary.successful, 1);
    assert.equal(summary.failed, 1);
    assert.equal(summary.unsupported, 1);
    assert.equal(summary.skipped, 1);
    // Unsupported item should not have been marked skipped
    assert.equal(items[2].status, "unsupported");
    assert.equal(items[3].status, "skipped");
  });
});

// ── Tests for getSuccessfulItems — registry read interface (Sub-AC 2.3a) ──────

describe("getSuccessfulItems — registry read interface for rollback", () => {
  function makeItem(
    itemId: string,
    overrides: Partial<RestoreItem> = {}
  ): RestoreItem {
    return {
      itemId,
      path: `/tmp/${itemId}`,
      type: "env_key",
      source: `/tmp/${itemId}`,
      dest: `/tmp/${itemId}`,
      status: "pending",
      executionOrder: 1,
      rollbackState: null,
      canRollback: true,
      applyAt: undefined,
      errorMessage: undefined,
      skipReason: undefined,
      ...overrides
    };
  }

  it("returns all items when all are marked 'applied' in the registry", () => {
    const items = [
      makeItem("a", { executionOrder: 1 }),
      makeItem("b", { executionOrder: 2 }),
      makeItem("c", { executionOrder: 3 })
    ];
    const registry: Record<string, RestoreItemStatus> = {
      a: "applied",
      b: "applied",
      c: "applied"
    };

    const result = getSuccessfulItems(items, registry);

    assert.equal(result.length, 3);
    assert.deepEqual(
      result.map((i) => i.itemId),
      ["a", "b", "c"]
    );
  });

  it("returns only items with status 'applied', excluding other statuses", () => {
    const items = [
      makeItem("applied-1", { executionOrder: 1 }),
      makeItem("failed-1", { executionOrder: 2 }),
      makeItem("skipped-1", { executionOrder: 3 }),
      makeItem("unsupported-1", { executionOrder: 4 }),
      makeItem("pending-1", { executionOrder: 5 }),
      makeItem("applied-2", { executionOrder: 6 })
    ];
    const registry: Record<string, RestoreItemStatus> = {
      "applied-1": "applied",
      "failed-1": "failed",
      "skipped-1": "skipped",
      "unsupported-1": "unsupported",
      "pending-1": "pending",
      "applied-2": "applied"
    };

    const result = getSuccessfulItems(items, registry);

    assert.equal(result.length, 2);
    assert.deepEqual(
      result.map((i) => i.itemId),
      ["applied-1", "applied-2"]
    );
  });

  it("returns empty array when no items are marked 'applied'", () => {
    const items = [
      makeItem("fail-1", { executionOrder: 1 }),
      makeItem("skip-1", { executionOrder: 2 })
    ];
    const registry: Record<string, RestoreItemStatus> = {
      "fail-1": "failed",
      "skip-1": "skipped"
    };

    const result = getSuccessfulItems(items, registry);

    assert.deepEqual(result, []);
  });

  it("returns items sorted by executionOrder regardless of input order", () => {
    const items = [
      makeItem("c", { executionOrder: 3 }),
      makeItem("a", { executionOrder: 1 }),
      makeItem("b", { executionOrder: 2 })
    ];
    const registry: Record<string, RestoreItemStatus> = {
      a: "applied",
      b: "applied",
      c: "applied"
    };

    const result = getSuccessfulItems(items, registry);

    assert.deepEqual(
      result.map((i) => i.itemId),
      ["a", "b", "c"]
    );
    assert.deepEqual(
      result.map((i) => i.executionOrder),
      [1, 2, 3]
    );
  });

  it("returns empty array for empty items input", () => {
    const result = getSuccessfulItems([], {});

    assert.deepEqual(result, []);
  });

  it("returns empty array when registry is empty", () => {
    const items = [makeItem("a", { executionOrder: 1 })];
    const result = getSuccessfulItems(items, {});

    assert.deepEqual(result, []);
  });

  it("excludes items whose registry status differs from the item's own status field", () => {
    // The registry is the authoritative source — not the item.status field
    const items = [
      makeItem("a", { executionOrder: 1, status: "pending" }),
      makeItem("b", { executionOrder: 2, status: "applied" })
    ];
    const registry: Record<string, RestoreItemStatus> = {
      a: "applied",   // registry says applied (even though item.status is pending)
      b: "failed"     // registry says failed (even though item.status is applied)
    };

    const result = getSuccessfulItems(items, registry);

    // a is included (registry says "applied"), b is excluded (registry says "failed")
    assert.equal(result.length, 1);
    assert.equal(result[0].itemId, "a");
  });

  it("items that exist in the input but have no registry entry are excluded", () => {
    const items = [
      makeItem("has-entry", { executionOrder: 1 }),
      makeItem("no-entry", { executionOrder: 2 })
    ];
    const registry: Record<string, RestoreItemStatus> = {
      "has-entry": "applied"
    };

    const result = getSuccessfulItems(items, registry);

    assert.equal(result.length, 1);
    assert.equal(result[0].itemId, "has-entry");
  });

  it("returns items in execution order when execution orders are non-sequential", () => {
    const items = [
      makeItem("x", { executionOrder: 10 }),
      makeItem("y", { executionOrder: 5 }),
      makeItem("z", { executionOrder: 1 })
    ];
    const registry: Record<string, RestoreItemStatus> = {
      x: "applied",
      y: "applied",
      z: "applied"
    };

    const result = getSuccessfulItems(items, registry);

    assert.deepEqual(
      result.map((i) => i.itemId),
      ["z", "y", "x"]
    );
  });

  it("preserves the full RestoreItem objects (not just itemIds)", () => {
    const items = [
      makeItem("full-1", {
        executionOrder: 1,
        type: "mcp_server",
        path: "/custom/path",
        canRollback: true,
        rollbackState: { previousValue: "old" }
      })
    ];
    const registry: Record<string, RestoreItemStatus> = {
      "full-1": "applied"
    };

    const result = getSuccessfulItems(items, registry);

    assert.equal(result.length, 1);
    assert.equal(result[0].itemId, "full-1");
    assert.equal(result[0].type, "mcp_server");
    assert.equal(result[0].path, "/custom/path");
    assert.equal(result[0].canRollback, true);
    assert.deepEqual(result[0].rollbackState, { previousValue: "old" });
  });
});

// ── Tests for getAppliedItems — rollback eligibility filter (Sub-AC 6a) ─────

describe("getAppliedItems — rollback eligibility filter", () => {
  function makeItem(
    itemId: string,
    overrides: Partial<RestoreItem> = {}
  ): RestoreItem {
    return {
      itemId,
      path: `/tmp/${itemId}`,
      type: "env_key",
      source: `/tmp/${itemId}`,
      dest: `/tmp/${itemId}`,
      status: "pending",
      executionOrder: 1,
      rollbackState: null,
      canRollback: true,
      applyAt: undefined,
      errorMessage: undefined,
      skipReason: undefined,
      ...overrides
    };
  }

  it("returns all items when all have status 'applied'", () => {
    const items = [
      makeItem("a", { executionOrder: 1, status: "applied" }),
      makeItem("b", { executionOrder: 2, status: "applied" }),
      makeItem("c", { executionOrder: 3, status: "applied" })
    ];

    const result = getAppliedItems(items);

    assert.equal(result.length, 3);
    assert.deepEqual(
      result.map((i) => i.itemId),
      ["a", "b", "c"]
    );
  });

  it("returns only items with status 'applied', excluding other statuses", () => {
    const items = [
      makeItem("applied-1", { executionOrder: 1, status: "applied" }),
      makeItem("failed-1", { executionOrder: 2, status: "failed" }),
      makeItem("skipped-1", { executionOrder: 3, status: "skipped" }),
      makeItem("unsupported-1", { executionOrder: 4, status: "unsupported" }),
      makeItem("pending-1", { executionOrder: 5, status: "pending" }),
      makeItem("applied-2", { executionOrder: 6, status: "applied" })
    ];

    const result = getAppliedItems(items);

    assert.equal(result.length, 2);
    assert.deepEqual(
      result.map((i) => i.itemId),
      ["applied-1", "applied-2"]
    );
  });

  it("returns empty array when no items have status 'applied'", () => {
    const items = [
      makeItem("fail-1", { executionOrder: 1, status: "failed" }),
      makeItem("skip-1", { executionOrder: 2, status: "skipped" }),
      makeItem("unsup-1", { executionOrder: 3, status: "unsupported" }),
      makeItem("pend-1", { executionOrder: 4, status: "pending" })
    ];

    const result = getAppliedItems(items);

    assert.deepEqual(result, []);
  });

  it("returns items sorted by executionOrder regardless of input order", () => {
    const items = [
      makeItem("c", { executionOrder: 3, status: "applied" }),
      makeItem("a", { executionOrder: 1, status: "applied" }),
      makeItem("b", { executionOrder: 2, status: "applied" })
    ];

    const result = getAppliedItems(items);

    assert.deepEqual(
      result.map((i) => i.itemId),
      ["a", "b", "c"]
    );
    assert.deepEqual(
      result.map((i) => i.executionOrder),
      [1, 2, 3]
    );
  });

  it("returns empty array for empty input", () => {
    const result = getAppliedItems([]);

    assert.deepEqual(result, []);
  });

  it("returns empty array when input has non-applied statuses only", () => {
    const items = [
      makeItem("x", { executionOrder: 10, status: "failed" }),
      makeItem("y", { executionOrder: 5, status: "skipped" })
    ];

    const result = getAppliedItems(items);

    assert.equal(result.length, 0);
  });

  it("preserves full RestoreItem objects in the result", () => {
    const items = [
      makeItem("full-1", {
        executionOrder: 1,
        status: "applied",
        type: "mcp_server",
        path: "/custom/mcp.json",
        canRollback: true,
        rollbackState: { previousValue: "old" },
        applyAt: "2026-05-12T12:00:00.000Z"
      })
    ];

    const result = getAppliedItems(items);

    assert.equal(result.length, 1);
    assert.equal(result[0].itemId, "full-1");
    assert.equal(result[0].type, "mcp_server");
    assert.equal(result[0].path, "/custom/mcp.json");
    assert.equal(result[0].canRollback, true);
    assert.equal(result[0].applyAt, "2026-05-12T12:00:00.000Z");
    assert.deepEqual(result[0].rollbackState, { previousValue: "old" });
  });

  it("handles mixed applied/non-applied with non-sequential execution orders", () => {
    const items = [
      makeItem("z", { executionOrder: 10, status: "applied" }),
      makeItem("y", { executionOrder: 5, status: "failed" }),
      makeItem("x", { executionOrder: 1, status: "applied" })
    ];

    const result = getAppliedItems(items);

    assert.equal(result.length, 2);
    assert.deepEqual(
      result.map((i) => i.itemId),
      ["x", "z"]
    );
    assert.deepEqual(
      result.map((i) => i.executionOrder),
      [1, 10]
    );
  });

  it("status filter is based on the item's own status field (not an external registry)", () => {
    // getAppliedItems reads item.status directly — no external registry needed
    const items = [
      makeItem("a", { executionOrder: 1, status: "applied" }),
      makeItem("b", { executionOrder: 2, status: "failed" })
    ];

    const result = getAppliedItems(items);

    assert.equal(result.length, 1);
    assert.equal(result[0].itemId, "a");
  });
});

// ── Tests for sortByDescendingOrder — execution order reverse sort (Sub-AC 6b) ──

describe("sortByDescendingOrder — reverse execution order sort for rollback", () => {
  function makeItem(
    itemId: string,
    overrides: Partial<RestoreItem> = {}
  ): RestoreItem {
    return {
      itemId,
      path: `/tmp/${itemId}`,
      type: "env_key",
      source: `/tmp/${itemId}`,
      dest: `/tmp/${itemId}`,
      status: "applied",
      executionOrder: 1,
      rollbackState: null,
      canRollback: true,
      applyAt: undefined,
      ...overrides
    };
  }

  it("returns items sorted by descending executionOrder (highest first)", () => {
    const items = [
      makeItem("mid", { executionOrder: 5 }),
      makeItem("high", { executionOrder: 10 }),
      makeItem("low", { executionOrder: 1 })
    ];

    const result = sortByDescendingOrder(items);

    assert.equal(result.length, 3);
    assert.deepEqual(
      result.map((i) => ({ itemId: i.itemId, executionOrder: i.executionOrder })),
      [
        { itemId: "high", executionOrder: 10 },
        { itemId: "mid", executionOrder: 5 },
        { itemId: "low", executionOrder: 1 }
      ]
    );
  });

  it("does not mutate the original array", () => {
    const items = [
      makeItem("a", { executionOrder: 1 }),
      makeItem("b", { executionOrder: 3 }),
      makeItem("c", { executionOrder: 2 })
    ];
    const originalOrder = items.map((i) => i.executionOrder);

    sortByDescendingOrder(items);

    assert.deepEqual(
      items.map((i) => i.executionOrder),
      originalOrder
    );
  });

  it("preserves items with equal executionOrder (stable relative to sort)", () => {
    const items = [
      makeItem("first", { executionOrder: 3 }),
      makeItem("second", { executionOrder: 1 }),
      makeItem("third", { executionOrder: 3 })
    ];

    const result = sortByDescendingOrder(items);

    assert.equal(result.length, 3);
    // All executionOrder=3 items come before executionOrder=1
    assert.equal(result[0].executionOrder, 3);
    assert.equal(result[1].executionOrder, 3);
    assert.equal(result[2].executionOrder, 1);
  });

  it("handles an empty array", () => {
    const result = sortByDescendingOrder([]);
    assert.deepEqual(result, []);
  });

  it("handles a single item", () => {
    const items = [makeItem("only", { executionOrder: 42 })];
    const result = sortByDescendingOrder(items);
    assert.equal(result.length, 1);
    assert.equal(result[0].itemId, "only");
    assert.equal(result[0].executionOrder, 42);
  });

  it("preserves all item fields through the sort", () => {
    const items = [
      makeItem("b", { executionOrder: 2, type: "mcp_server", path: "/custom/mcp.json", canRollback: false, rollbackState: { prev: "x" } }),
      makeItem("a", { executionOrder: 1, type: "env_key", path: "/tmp/a", canRollback: true })
    ];

    const result = sortByDescendingOrder(items);

    assert.equal(result.length, 2);
    // highest executionOrder first
    assert.equal(result[0].itemId, "b");
    assert.equal(result[0].type, "mcp_server");
    assert.equal(result[0].path, "/custom/mcp.json");
    assert.equal(result[0].canRollback, false);
    assert.deepEqual(result[0].rollbackState, { prev: "x" });
    // second item intact
    assert.equal(result[1].itemId, "a");
    assert.equal(result[1].executionOrder, 1);
  });
});
