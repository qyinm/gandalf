# Gate 2 Demo Recordings

This folder contains two VHS tapes for the Gate 2 customer journey.

Run both:

```sh
bun run demo:gate2:vhs
```

Or run one:

```sh
bun run demo:pain:vhs
bun run demo:fix:vhs
```

`pain.tape` shows the customer journey without Gandalf:

```sh
npx experimental-codex-harness install
codex
# then type: restore before prev agent setup
```

`fix.tape` shows the same journey with Gandalf:

```sh
cd __GANDALF_VISIBLE_PROJECT__
gandalf snapshot create --name before-harness --agent codex --scope user --project .
npx experimental-codex-harness install
gandalf diff before-harness current --agent codex --scope user --project .
gandalf restore --snapshot before-harness --dry-run --agent codex --scope user --project .
gandalf restore --snapshot before-harness --apply --experimental --agent codex --scope user --project .
```

The tapes use `__GANDALF_VISIBLE_PROJECT__` as a placeholder. The runner renders a temporary tape with that placeholder replaced by a short visible demo project path like `/Users/<you>/gandalf-demo`.

The runner creates that disposable demo project, with an isolated fake `HOME` at `<visible-project>/home`. It also creates a temporary `GANDALF_STORE`, `CODEX_HOME`, and wrapper executables for `npx`, `gandalf`, and `verify-rollback`. `codex` is the real Codex CLI and runs against the isolated `CODEX_HOME`. The runner deletes the visible demo project and its temporary root after VHS exits, so the real `~/.codex` directory is not targeted.

Generated media files are ignored by git:

- `demo/pain.gif`
- `demo/pain.mp4`
- `demo/fix.gif`
- `demo/fix.mp4`
