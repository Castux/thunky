# Vendored third-party assets

Both libraries used to load from a CDN. That made the "self-contained site"
claim false, added two third-party requests to every page load, and meant a
blocked or unreachable CDN rendered a dead shell rather than a degraded page —
`site.js` and `playground.js` both throw at top level without them. They are
0.2 MB against the 3.87 MB wasm binary the site already ships, so vendoring
costs nothing that matters.

| File | Version | Licence | Upstream |
|------|---------|---------|----------|
| `codemirror.min.js`, `codemirror.min.css` | 5.65.16 | MIT | [codemirror/codemirror5](https://github.com/codemirror/codemirror5) |
| `marked.min.js` | 12.0.2 | MIT | [markedjs/marked](https://github.com/markedjs/marked) |

CodeMirror 5 rather than 6 deliberately: 5 is a single global-script drop-in,
which suits a site with no build step and dozens of editor instances per page.

To update, fetch the new versions and re-run the site build:

```sh
V=5.65.16
curl -o web/vendor/codemirror.min.js  https://cdnjs.cloudflare.com/ajax/libs/codemirror/$V/codemirror.min.js
curl -o web/vendor/codemirror.min.css https://cdnjs.cloudflare.com/ajax/libs/codemirror/$V/codemirror.min.css
curl -o web/vendor/marked.min.js      https://cdn.jsdelivr.net/npm/marked@12.0.2/marked.min.js
```
