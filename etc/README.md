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
| [`nano/microfun.nanorc`](nano/microfun.nanorc) | GNU nano | copy to `/usr/share/nano/`, `include` it from your nanorc |
| [`micro/microfun.yaml`](micro/microfun.yaml) | micro | copy to `~/.config/micro/syntax/` |
| [`zed/`](zed/) | Zed | install as a dev extension |

The Zed extension carries a full tree-sitter grammar
(`zed/grammars/microfun/grammar.js`) with highlight queries; the other two are
regex-based and colour keywords, builtins, literals, comments and operators.

All three still use the language's former name, `microfun`, in their
identifiers — the Zed grammar's id is tied to an external `tree-sitter-microfun`
repository, and renaming it means regenerating the grammar. The grammars
themselves are current: same keywords, same operators, same builtin list as
[`docs/LANGUAGE.md`](../docs/LANGUAGE.md).
