---
title: churl
description: Fetch a URL through a real browser so JavaScript runs, then print the rendered HTML, text, or JSON — with an optional HAR alongside.
icon: download
---

# churl

`churl` is shaped like `curl`, but every request runs in a real browser. That
is the whole point: a page that renders its content from JavaScript comes back
rendered, not as the empty shell `curl` would print.

```bash
churl https://example.com
```

```html
<html lang="en"><head><title>Example Domain</title>…
```

The default output is the rendered HTML — the DOM after scripts ran, not the
bytes the server sent.

## Output formats

```bash
churl --output-format=text https://example.com
churl --output-format=json https://example.com
churl --output-format=pdf -o page.pdf https://example.com
churl --har capture.har https://example.com     # any format, plus a HAR
churl -o page.html https://example.com
```

`json` returns `url`, `title`, and `content`. `text` is the extracted text, and
it includes the contents of `<style>` elements, so a page's CSS appears as a
line of text near the top. Pipe it through your own filter if that
matters.

`pdf` prints the rendered page through Chrome's own print pipeline, so it is
the page as the browser would print it — backgrounds included, JavaScript
already run. It is binary, so churl refuses to write it to a terminal; pass
`-o` or redirect:

```bash
churl --output-format=pdf https://example.com > page.pdf
```

The waiting flags matter more here than for other formats, because whatever has
not rendered when the print happens is simply absent from the file:

```bash
churl --output-format=pdf --wait-for '#report-ready' -o report.pdf https://app.example.com
```

`--har` works alongside any format.

### PDF print settings

Print geometry decides whether a generated document is usable or has to be
reprinted by hand, so all of it is reachable — but through one flag rather than
ten:

```bash
churl --output-format=pdf --pdf 'page=a4,margin=0.75' -o page.pdf URL
churl --output-format=pdf --pdf 'page=210mmx297mm,margin=20mm 15mm,outline' -o doc.pdf URL
churl --output-format=pdf --pdf 'page=tabloid,landscape,scale=0.8' -o wide.pdf URL
```

| Setting | Meaning |
| --- | --- |
| `page=` | `letter` (default), `legal`, `tabloid`, `ledger`, `a0`–`a6`, or `WxH` |
| `margin=` | 1, 2, or 4 space-separated lengths, in CSS order (default 0.4in) |
| `scale=` | render scale (default 1) |
| `ranges=` | print only these one-based pages, e.g. `ranges=1-5 8` |
| `landscape` | landscape orientation |
| `outline` | PDF bookmarks from the headings; implies `tagged` |
| `tagged` | tagged (accessible) PDF |
| `css-page-size` | honour `@page` size from the document's own CSS |

Lengths accept `in`, `mm`, `cm`, and `px` suffixes and are inches when
unsuffixed, so `a4` and `210mmx297mm` describe the same page. Values may not
contain commas — that is why `margin` and `ranges` take space-separated lists.

`outline` is the one worth reaching for when printing a docs tree: it turns the
heading structure into PDF bookmarks. It only works alongside `tagged`, because
Chrome derives the outline from the tag tree, so asking for `outline` turns
tagging on for you.

A document that declares `@page` size in its own CSS overrides `page=` and
`landscape`, even without `css-page-size`. This is quiet — the flag simply has
no effect — so check the document's CSS before concluding the flag is broken.

### Headers, footers, and page numbers

`--pdf-header` and `--pdf-footer` stay separate flags, because the templates are
HTML and would not survive a comma-separated list. Chrome substitutes any
element carrying the classes `date`, `title`, `url`, `pageNumber`, or
`totalPages`:

```bash
churl --output-format=pdf --pdf 'margin=1' \
  --pdf-footer '<div style="font-size:9px;width:100%;text-align:center">page <span class="pageNumber"></span> of <span class="totalPages"></span></div>' \
  -o report.pdf https://example.com
```

The catch: the template renders *inside* the margin, so a footer with the
default 0.4in margin is clipped or invisible. Give the edge room — `margin=1` is
a safe starting point — and set the font size explicitly, because the template
does not inherit the page's styles.

### Images in the PDF

There is no image-quality or resampling setting. `Page.printToPDF` has none,
and `deviceScaleFactor` does not affect its output — the same page printed at
scale factor 1, 2, and 3 produces byte-identical PDFs.

Images are embedded at their **source** resolution, not at the size they are
displayed. The same page with a 1600px-wide source image produced a 24 KB PDF
where a 200px source produced 2.5 KB, both shown at the same width. If you need
sharper images in the PDF, load sharper images.

## Waiting for the page to settle

A rendered page is only worth reading once it has stopped changing. `churl`
waits for network idle by default, and takes two more explicit signals:

```bash
churl --wait-for '#app-root' https://app.example.com
churl --stable-timeout 60 https://slow.example.com
```

`--wait-for-challenge` is on by default and waits out anti-bot interstitials
such as Cloudflare's.

## Proxies

Full coverage is on its own page — including the `<-loopback>` behavior that
makes `localhost` URLs fail as soon as you set `--proxy`.

→ [churl behind a proxy](/docs/churl-proxy)

## Blocking, scripts, and WebSockets

`churl` can drop requests inside the browser (`--block-ads`,
`--block-tracking`, `--block-domain`, `--block-regex`), inject JavaScript
before or after load (`--script-before`, `--script-after`, and their
`--script-file-*` forms), and watch WebSocket traffic (`--ws-enabled`,
`--ws-send`, `--ws-wait-for`). These are documented flag-by-flag in `go doc
./cmd/churl`; no page here has verified their workflows end to end.

## What does not work

The wget-style recursion and mirroring flags — `-r`, `-m`, `-l`, `-np`, `-P`
and the rest — are parsed and ignored. `churl` exits 0 and writes nothing. Use
it for single-URL fetches and drive a crawl from a shell loop.
See [Known issues](/docs/known-issues).

## Exit status

- `0` success
- `2` usage error, such as an unknown flag
- `1` everything else, including every proxy and navigation failure

There is no finer failure classification; do not branch on a specific code
beyond these three. Verified against this tree: a proxy that refuses the
connection (`churl --proxy http://127.0.0.1:9 https://example.com`) exits `1`,
and an unknown flag exits `2`.

## When to use something else

- You want the traffic, not the page → [`chrome-to-har`](/docs/capturing-traffic).
- You want to click through something first → [`cdp`](/docs/cdp).
- You want it to run the same way tomorrow → [`cdpscript`](/docs/scripting).

## Next steps

- [churl behind a proxy](/docs/churl-proxy) — proxy, authentication, and bypass.
- [Capture network traffic](/docs/capturing-traffic) — when the HAR is the goal.
- [Command reference](/docs/commands) — `go doc ./cmd/churl` for every flag.
