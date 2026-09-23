---
title: Quickstart
description: Install the cdp tools and confirm they can drive a browser, in about five minutes.
icon: rocket
---

# Quickstart

## Install

The tools need Go 1.26 or later and a Chromium-based browser — Chrome,
Chromium, Brave, Edge, Opera, or Vivaldi. They launch a browser you already
have rather than downloading one.

```bash
go install github.com/tmc/cdp/cmd/cdp@latest
go install github.com/tmc/cdp/cmd/churl@latest
go install github.com/tmc/cdp/cmd/cdpscript@latest
```

These land in `$(go env GOPATH)/bin`, usually `~/go/bin`. Make sure that is on
your `PATH`.

## Confirm a browser is found

```bash
cdp -list-browsers
```

```
Brave	/Applications/Brave Browser.app/Contents/MacOS/Brave Browser		Running
Brave	/Applications/Brave Browser.app/Contents/MacOS/Brave Browser	151.1.93.129	Installed
Chrome Canary	/Applications/Google Chrome Canary.app/…/Google Chrome Canary	canary	Installed
```

The columns are name, path, version, and whether that browser is running now.
Your list will differ. If nothing is found, or you want a specific browser,
pass `-chrome-path` to any command or set `CHROME_PATH` in the environment.

## Extract something from a page

This runs headless, so no window appears:

```bash
cdp -headless -url 'data:text/html,<h1>Hi</h1>' -extract h1
```

```
"{\"text\":\"Hi\"}"
```

The result is a JSON string. `-extract-mode` selects what you get back —
`text`, `html`, `attr:name`, or `count`.

## Render a page as Markdown

```bash
cdp -headless -url 'data:text/html,<h1>Title</h1><p>Body <b>text</b>.</p>' -render body
```

```
# Title

Body **text**.
```

## Confirm JavaScript actually runs

This is what separates these tools from `curl`. Serve a page whose content is
built by JavaScript after load:

```bash
mkdir -p /tmp/cdp-demo/api && cd /tmp/cdp-demo
echo '{"count":3}' > api/items
cat > index.html <<'HTML'
<!doctype html><title>Demo</title><h1>Demo site</h1>
<script>fetch('/api/items').then(r=>r.json()).then(d=>{
  document.body.insertAdjacentHTML('beforeend','<div id=done>'+d.count+'</div>')})</script>
HTML
python3 -m http.server 8099 &
```

Now fetch it:

```bash
churl http://localhost:8099/
```

```
<html><head><title>Demo</title></head><body><h1>Demo site</h1>
<script>fetch('/api/items').then(r=>r.json()).then(d=>{
  document.body.insertAdjacentHTML('beforeend','<div id=done>'+d.count+'</div>')})</script>
<div id="done">3</div></body></html>
```

The `<div id="done">3</div>` does not exist in the served HTML — the page's own
JavaScript fetched it and inserted it. `curl http://localhost:8099/` does not
show it. Keep this server running if you want to follow the
[capture guide](/docs/capturing-traffic); otherwise stop it with
`pkill -f 'http.server 8099'`.

You are now in `/tmp/cdp-demo`, so anything you capture or screenshot next
lands there. `cd -` to go back.

## Take a screenshot

```bash
cdp -headless -url 'data:text/html,<h1>Hi</h1>' -screenshot 'full shot.png'
```

```
Screenshot saved to: shot.png (3119 bytes)
```

The byte count will differ on your machine; fonts and device pixel ratio change
the rendering.

## Next steps

- [Tutorial: your first script](/docs/tutorial-first-script) — turn these one-off
  commands into something repeatable that runs as a test.
- [How cdp works](/docs/how-cdp-works) — why there are several commands and when to
  reach for each.
- [Troubleshooting](/docs/troubleshooting) — if any command above did not behave as
  shown.
