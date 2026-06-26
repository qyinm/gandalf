# Gandalf CLI

`@qxinm/gandalf` is the public npm package for Gandalf's CLI.

The package name uses `qxinm` because that is the npm publishing account. The source repository, issue tracker, and homepage remain `qyinm/gandalf`.

Gandalf's current Gate 2 path is narrow: save, diff, and restore user-global Codex setup under `~/.codex/` after a setup experiment goes wrong.

```bash
bun install -g @qxinm/gandalf
gandalf snapshot create --name baseline --agent codex --scope user --project .
gandalf diff baseline current --agent codex --scope user --project .
gandalf restore --snapshot baseline --dry-run --agent codex --scope user --project .
gandalf restore --snapshot baseline --apply --experimental --agent codex --scope user --project .
```
