# Hem CLI

`@qxinm/hem` is the public npm package for Hem's CLI.

The package name uses `qxinm` because that is the npm publishing account. The source repository, issue tracker, and homepage remain `qyinm/hem`.

Hem's current Gate 2 path is narrow: save, diff, and restore user-global Codex setup under `~/.codex/` after a setup experiment goes wrong.

```bash
bun install -g @qxinm/hem
hem snapshot create --name baseline --agent codex --scope user --project .
hem diff baseline current --agent codex --scope user --project .
hem restore --snapshot baseline --dry-run --agent codex --scope user --project .
hem restore --snapshot baseline --apply --experimental --agent codex --scope user --project .
```
