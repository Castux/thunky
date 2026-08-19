# etc — editor support and repository tooling

Things that are not the compiler and not the site, but are worth keeping.

## `check-docs.py`

Runs every runnable fenced code block in `docs/` exactly the way the site's Run
buttons do, and fails if any of them does not compile and run. It is the reason
a broken snippet cannot reach the published documentation.

```sh
go build -o thunky .
python etc/check-docs.py [--verbose]
```

Runs in CI on every push (`.github/workflows/test.yml`). Its sibling,
`web/check-examples.mjs`, does the same job for the playground's example
catalogue.

## Editor support

Syntax highlighting for three editors, all recognising `.th` and `.þ`:

| Path | Editor | Install |
|------|--------|---------|
| [`nano/thunky.nanorc`](nano/thunky.nanorc) | GNU nano | copy to `/usr/share/nano/`, `include` it from your nanorc |
| [`micro/thunky.yaml`](micro/thunky.yaml) | micro | copy to `~/.config/micro/syntax/` |
| [`zed/`](zed/) | Zed | install as a dev extension |

The nano and micro files are regex-based and colour keywords, builtins, literals,
comments and operators. The Zed extension is a real tree-sitter integration, and
it is split across two repositories:

- [`zed/`](zed/) here holds the extension — `extension.toml` plus
  `languages/thunky/{config.toml,highlights.scm}`.
- The grammar itself lives in
  [`Castux/tree-sitter-thunky`](https://github.com/Castux/tree-sitter-thunky),
  because Zed only consumes a grammar it can clone at a pinned commit.

Zed clones that repo at the `commit` in `[grammars.thunky]` and compiles
`src/parser.c` to wasm itself, so **after every grammar change you must
regenerate and commit `src/` there, push, and bump the hash here** — otherwise
Zed keeps building the old parser. The highlight queries Zed applies are
`zed/languages/thunky/highlights.scm` in this repo, not the grammar repo's
`queries/highlights.scm`; the two are identical today and should be kept that
way.

All three grammars are current: same keywords, same operators, same builtin list
as [`docs/LANGUAGE.md`](../docs/LANGUAGE.md).
