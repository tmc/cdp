package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerInspectTools registers inspection/reversing tools gated behind --enable-inspect.
func registerInspectTools(server *mcp.Server, s *mcpSession) {
	registerInspectIPCTools(server, s)
	registerInspectWalkTool(server, s)
	registerInspectFingerprintTool(server, s)
}

// --- Electron IPC sniffer tools ---

// startIPCLogJS is the JS that monkey-patches known Electron IPC bridges
// to log all outgoing and incoming messages.
const startIPCLogJS = `(() => {
	if (window.__ipcLog) return {already_active: true, count: window.__ipcLog.length};
	window.__ipcLog = [];
	const ts = () => new Date().toISOString();
	const push = (dir, ch, args) => {
		window.__ipcLog.push({timestamp: ts(), direction: dir, channel: ch, args: JSON.parse(JSON.stringify(args, (k,v) => {
			if (typeof v === 'function') return '[function]';
			if (v instanceof Error) return v.message;
			if (typeof v === 'object' && v !== null && v.constructor && v.constructor.name !== 'Object' && v.constructor.name !== 'Array')
				return '[' + v.constructor.name + ']';
			return v;
		}))});
	};

	// Patch contextBridge-exposed objects (electronBridge, vscode, etc.)
	const bridges = [
		{obj: window.electronBridge, name: 'electronBridge'},
		{obj: window.vscode, name: 'vscode'},
		{obj: window.electron, name: 'electron'},
	];
	let patched = 0;
	for (const {obj, name} of bridges) {
		if (!obj) continue;
		for (const key of Object.keys(obj)) {
			if (typeof obj[key] === 'function') {
				const orig = obj[key].bind(obj);
				obj[key] = function(...args) {
					push('out', name + '.' + key, args);
					return orig(...args);
				};
				patched++;
			}
		}
	}

	// Patch ipcRenderer if exposed (nodeIntegration or preload leak).
	const ipc = window.require && (() => { try { return window.require('electron').ipcRenderer } catch(e) { return null } })();
	if (ipc) {
		const origSend = ipc.send.bind(ipc);
		const origInvoke = ipc.invoke.bind(ipc);
		ipc.send = function(ch, ...args) { push('out', ch, args); return origSend(ch, ...args); };
		ipc.invoke = function(ch, ...args) { push('out', ch, args); return origInvoke(ch, ...args); };
		const origOn = ipc.on.bind(ipc);
		ipc.on = function(ch, fn) {
			return origOn(ch, (ev, ...args) => { push('in', ch, args); return fn(ev, ...args); });
		};
		patched++;
	}

	// Listen for postMessage-based IPC.
	window.addEventListener('message', (ev) => {
		if (ev.source === window) {
			push('in', 'window.postMessage', [ev.data]);
		}
	});

	return {started: true, patched: patched};
})()
`

// getIPCLogJSTmpl returns and optionally clears the captured IPC log.
const getIPCLogJSTmpl = `(() => {
	if (!window.__ipcLog) return {error: 'not started'};
	let entries = window.__ipcLog;
	const filter = %q;
	if (filter) {
		entries = entries.filter(e => e.channel.includes(filter));
	}
	if (%v) {
		window.__ipcLog = [];
	}
	return entries;
})()
`

type InspectIPCStartInput struct{}

type InspectIPCLogInput struct {
	Channel string `json:"channel,omitempty"` // filter by channel substring
	Clear   bool   `json:"clear,omitempty"`   // clear log after reading
	Limit   int    `json:"limit,omitempty"`   // max entries to return
}

func registerInspectIPCTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "inspect_ipc_start",
		Description: `Start capturing Electron IPC messages. Monkey-patches known bridges (electronBridge, vscode, electron) and postMessage listener. Call inspect_ipc_log to read captured messages.`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, input InspectIPCStartInput) (*mcp.CallToolResult, any, error) {
		var result any
		if err := chromedp.Run(s.activeCtx(), chromedp.Evaluate(startIPCLogJS, &result)); err != nil {
			return nil, nil, fmt.Errorf("inspect_ipc_start: %w", err)
		}
		data, _ := json.Marshal(result)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "inspect_ipc_log",
		Description: `Get captured IPC messages from inspect_ipc_start. Optional channel filter (substring match). Set clear=true to reset the log after reading.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input InspectIPCLogInput) (*mcp.CallToolResult, any, error) {
		js := fmt.Sprintf(getIPCLogJSTmpl, input.Channel, input.Clear)
		var result any
		if err := chromedp.Run(s.activeCtx(), chromedp.Evaluate(js, &result)); err != nil {
			return nil, nil, fmt.Errorf("inspect_ipc_log: %w", err)
		}

		// Apply limit if needed.
		if input.Limit > 0 {
			if arr, ok := result.([]any); ok && len(arr) > input.Limit {
				result = arr[len(arr)-input.Limit:]
			}
		}

		data, err := json.Marshal(result)
		if err != nil {
			return nil, nil, fmt.Errorf("inspect_ipc_log: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}

// --- Deep object walker ---

// walkObjectJS is the JS function that recursively walks an object,
// producing a typed tree with cycle detection and depth limiting.
const walkObjectJS = `(expr, maxDepth, maxKeys, sampleValues) => {
	const seen = new WeakSet();
	function walk(obj, depth) {
		if (obj === null) return {_type: 'null'};
		if (obj === undefined) return {_type: 'undefined'};
		const t = typeof obj;
		if (t === 'boolean' || t === 'number' || t === 'bigint') {
			const r = {_type: t};
			if (sampleValues) r._value = String(obj);
			return r;
		}
		if (t === 'string') {
			const r = {_type: 'string', _length: obj.length};
			if (sampleValues) r._value = obj.length > 100 ? obj.slice(0, 100) + '...' : obj;
			return r;
		}
		if (t === 'symbol') return {_type: 'symbol', _value: obj.toString()};
		if (t === 'function') return {_type: 'function', _length: obj.length, _name: obj.name || '(anonymous)'};
		if (t !== 'object') return {_type: t};

		// Object or array.
		if (seen.has(obj)) return {_type: 'circular'};
		seen.add(obj);

		if (Array.isArray(obj)) {
			const r = {_type: 'array', _length: obj.length};
			if (depth < maxDepth && obj.length > 0) {
				r._items = [];
				const n = Math.min(obj.length, maxKeys);
				for (let i = 0; i < n; i++) {
					r._items.push(walk(obj[i], depth + 1));
				}
				if (obj.length > n) r._truncated = obj.length - n;
			}
			return r;
		}

		const keys = Object.keys(obj);
		const r = {_type: 'object', _keys: keys.length};
		if (obj.constructor && obj.constructor.name !== 'Object') {
			r._class = obj.constructor.name;
		}
		if (depth >= maxDepth) return r;

		const n = Math.min(keys.length, maxKeys);
		for (let i = 0; i < n; i++) {
			try {
				r[keys[i]] = walk(obj[keys[i]], depth + 1);
			} catch(e) {
				r[keys[i]] = {_type: 'error', _value: e.message};
			}
		}
		if (keys.length > n) r._truncated = keys.length - n;
		return r;
	}
	try {
		const target = eval(expr);
		return walk(target, 0);
	} catch(e) {
		return {_type: 'error', _value: e.message};
	}
}
`

type InspectWalkInput struct {
	Expression   string `json:"expression"`             // JS expression to evaluate
	Depth        int    `json:"depth,omitempty"`         // max recursion depth (default 2)
	MaxKeys      int    `json:"max_keys,omitempty"`      // max keys per object (default 20)
	SampleValues bool   `json:"sample_values,omitempty"` // include primitive values
}

func registerInspectWalkTool(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "inspect_walk",
		Description: `Recursively explore a JS object's structure. Returns a typed tree with key counts, function arities, string lengths, and optional sampled values. Handles cycles via WeakSet. Use depth (default 2) and max_keys (default 20) to control output size.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input InspectWalkInput) (*mcp.CallToolResult, any, error) {
		depth := input.Depth
		if depth <= 0 {
			depth = 2
		}
		maxKeys := input.MaxKeys
		if maxKeys <= 0 {
			maxKeys = 20
		}

		js := fmt.Sprintf("(%s)(%q, %d, %d, %v)",
			walkObjectJS, input.Expression, depth, maxKeys, input.SampleValues)

		var result any
		if err := chromedp.Run(s.activeCtx(), chromedp.Evaluate(js, &result)); err != nil {
			return nil, nil, fmt.Errorf("inspect_walk: %w", err)
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, nil, fmt.Errorf("inspect_walk: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}

// --- App fingerprinting ---

// reverseAppJS runs a battery of detections to fingerprint the current app.
const reverseAppJS = `(() => {
	const r = {};

	// App identity.
	r.identity = {
		title: document.title,
		url: window.location.href,
		userAgent: navigator.userAgent,
	};
	const uaMatch = navigator.userAgent.match(/Electron\/(\S+)/);
	if (uaMatch) r.identity.electronVersion = uaMatch[1];
	const chromeMatch = navigator.userAgent.match(/Chrome\/(\S+)/);
	if (chromeMatch) r.identity.chromeVersion = chromeMatch[1];

	// Framework detection.
	r.frameworks = {};
	if (window.__REACT_DEVTOOLS_GLOBAL_HOOK__) r.frameworks.react = true;
	if (window.__REACT_DEVTOOLS_GLOBAL_HOOK__?.renderers?.size > 0) {
		const renderer = window.__REACT_DEVTOOLS_GLOBAL_HOOK__.renderers.values().next().value;
		if (renderer?.version) r.frameworks.reactVersion = renderer.version;
	}
	if (window.preactDevTools || window.__PREACT_DEVTOOLS__) r.frameworks.preact = true;
	if (window.__VUE__) r.frameworks.vue = true;
	if (window.__VUE_DEVTOOLS_GLOBAL_HOOK__) r.frameworks.vue = true;
	if (window.__SVELTE_HMR) r.frameworks.svelte = true;
	if (window.ng || document.querySelector('[ng-version]')) r.frameworks.angular = true;
	const ngEl = document.querySelector('[ng-version]');
	if (ngEl) r.frameworks.angularVersion = ngEl.getAttribute('ng-version');
	if (Object.keys(r.frameworks).length === 0) delete r.frameworks;

	// Bundler detection.
	r.bundler = {};
	if (typeof __webpack_require__ !== 'undefined') r.bundler.webpack = true;
	if (document.querySelector('script[type=importmap]')) r.bundler.importMap = true;
	const scripts = Array.from(document.querySelectorAll('script[src]')).map(s => s.src);
	if (scripts.some(s => s.includes('/@vite/') || s.includes('vite'))) r.bundler.vite = true;
	if (scripts.some(s => /chunk-[A-Za-z0-9]+\.js/.test(s))) r.bundler.chunked = true;
	if (Object.keys(r.bundler).length === 0) delete r.bundler;

	// Non-standard globals.
	const stdGlobals = new Set([
		'undefined','NaN','Infinity','eval','isFinite','isNaN','parseFloat','parseInt',
		'decodeURI','decodeURIComponent','encodeURI','encodeURIComponent',
		'Array','ArrayBuffer','BigInt','BigInt64Array','BigUint64Array','Boolean',
		'DataView','Date','Error','EvalError','Float32Array','Float64Array',
		'Function','Int8Array','Int16Array','Int32Array','JSON','Map','Math',
		'Number','Object','Promise','Proxy','RangeError','ReferenceError',
		'Reflect','RegExp','Set','SharedArrayBuffer','String','Symbol',
		'SyntaxError','TypeError','URIError','Uint8Array','Uint8ClampedArray',
		'Uint16Array','Uint32Array','WeakMap','WeakRef','WeakSet',
		'Atomics','console','crypto','fetch','performance','queueMicrotask',
		'setTimeout','clearTimeout','setInterval','clearInterval',
		'requestAnimationFrame','cancelAnimationFrame',
		'alert','blur','close','closed','confirm','focus','frames',
		'getComputedStyle','getSelection','history','innerHeight','innerWidth',
		'length','location','locationbar','matchMedia','menubar','moveBy','moveTo',
		'name','navigator','open','opener','outerHeight','outerWidth',
		'pageXOffset','pageYOffset','parent','personalbar','postMessage','print',
		'prompt','resizeBy','resizeTo','screen','screenLeft','screenTop',
		'screenX','screenY','scroll','scrollBy','scrollTo','scrollX','scrollY',
		'scrollbars','self','speechSynthesis','statusbar','stop','toolbar',
		'top','visualViewport','window','document','customElements','external',
		'frameElement','origin','clientInformation','event','offscreenBuffering',
		'styleMedia','defaultStatus','defaultstatus','isSecureContext','crossOriginIsolated',
		'scheduler','caches','cookieStore','onabort','onafterprint','onanimationend',
		'onanimationiteration','onanimationstart','onbeforeprint','onbeforeunload',
		'onblur','oncancel','oncanplay','oncanplaythrough','onchange','onclick',
		'onclose','oncontextmenu','oncuechange','ondblclick','ondrag','ondragend',
		'ondragenter','ondragleave','ondragover','ondragstart','ondrop',
		'ondurationchange','onemptied','onerror','onfocus','onformdata',
		'ongotpointercapture','onhashchange','oninput','oninvalid','onkeydown',
		'onkeypress','onkeyup','onlanguagechange','onload','onloadeddata',
		'onloadedmetadata','onloadstart','onlostpointercapture','onmessage',
		'onmessageerror','onmousedown','onmouseenter','onmouseleave','onmousemove',
		'onmouseout','onmouseover','onmouseup','onoffline','ononline','onpagehide',
		'onpageshow','onpause','onplay','onplaying','onpointercancel','onpointerdown',
		'onpointerenter','onpointerleave','onpointermove','onpointerout',
		'onpointerover','onpointerrawupdate','onpointerup','onpopstate',
		'onprogress','onratechange','onrejectionhandled','onreset','onresize',
		'onscroll','onsearch','onseeked','onseeking','onselect','onselectionchange',
		'onselectstart','onslotchange','onstalled','onstorage','onsubmit',
		'onsuspend','ontimeupdate','ontoggle','ontransitioncancel','ontransitionend',
		'ontransitionrun','ontransitionstart','onunhandledrejection','onunload',
		'onvolumechange','onwaiting','onwebkitanimationend','onwebkitanimationiteration',
		'onwebkitanimationstart','onwebkittransitionend','onwheel',
		'atob','btoa','structuredClone','requestIdleCallback','cancelIdleCallback',
		'createImageBitmap','find','getScreenDetails','showDirectoryPicker',
		'showOpenFilePicker','showSaveFilePicker','chrome','webkitRequestFileSystem',
		'webkitResolveLocalFileSystemURL','IndexedDB','webkitMediaStream',
		'WebKitMutationObserver','webkitRTCPeerConnection','webkitSpeechGrammar',
		'webkitSpeechGrammarList','webkitSpeechRecognition','webkitSpeechRecognitionError',
		'webkitSpeechRecognitionEvent','openDatabase','webkitRequestAnimationFrame',
		'webkitCancelAnimationFrame','getComputedStyle','reportError',
		'trustedTypes','navigation','launchQueue','documentPictureInPicture',
	]);
	const custom = Object.keys(window).filter(k => !stdGlobals.has(k) && !k.startsWith('on'));
	if (custom.length > 0) {
		r.globals = {};
		for (const k of custom.slice(0, 50)) {
			try {
				const v = window[k];
				const t = typeof v;
				if (t === 'function') r.globals[k] = {type: 'function', length: v.length};
				else if (t === 'object' && v !== null) r.globals[k] = {type: 'object', keys: Object.keys(v).length, class: v.constructor?.name};
				else r.globals[k] = {type: t};
			} catch(e) {
				r.globals[k] = {type: 'error', error: e.message};
			}
		}
		if (custom.length > 50) r.globals._truncated = custom.length - 50;
	}

	// Bridge API.
	const bridges = {};
	for (const name of ['electronBridge', 'vscode', 'electron']) {
		if (window[name]) {
			bridges[name] = {};
			for (const k of Object.keys(window[name])) {
				try {
					const v = window[name][k];
					bridges[name][k] = typeof v === 'function' ? {type: 'function', length: v.length} : {type: typeof v};
				} catch(e) {
					bridges[name][k] = {type: 'error'};
				}
			}
		}
	}
	if (Object.keys(bridges).length > 0) r.bridges = bridges;

	// Feature flags.
	const flags = {};
	if (window.__STATSIG__) flags.statsig = true;
	if (window.__UNLEASH__) flags.unleash = true;
	if (window.__LAUNCH_DARKLY__ || window.ldclient) flags.launchDarkly = true;
	if (window.__SPLIT__) flags.split = true;
	if (Object.keys(flags).length > 0) r.featureFlags = flags;

	// Error monitoring.
	const errors = {};
	if (window.__SENTRY__ || window.Sentry) errors.sentry = true;
	if (window.Bugsnag) errors.bugsnag = true;
	if (window.__datadog || window.DD_RUM) errors.datadog = true;
	if (window.newrelic || window.NREUM) errors.newrelic = true;
	if (Object.keys(errors).length > 0) r.errorMonitoring = errors;

	// Scripts summary.
	const allScripts = Array.from(document.querySelectorAll('script[src]'));
	r.scripts = {count: allScripts.length};
	const byDomain = {};
	for (const s of allScripts) {
		try {
			const u = new URL(s.src);
			byDomain[u.hostname] = (byDomain[u.hostname] || 0) + 1;
		} catch(e) {}
	}
	if (Object.keys(byDomain).length > 0) r.scripts.byDomain = byDomain;

	// CSP.
	const cspMeta = document.querySelector('meta[http-equiv="Content-Security-Policy"]');
	if (cspMeta) r.csp = cspMeta.content;

	return r;
})()
`

type InspectFingerprintInput struct{}

func registerInspectFingerprintTool(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "inspect_fingerprint",
		Description: `Automated app fingerprinting. Detects: identity (title, URL, Electron/Chrome versions), frameworks (React, Vue, Svelte, Angular), bundler (webpack, Vite), non-standard globals, bridge APIs (electronBridge, vscode), feature flags, error monitoring, script inventory, CSP.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input InspectFingerprintInput) (*mcp.CallToolResult, any, error) {
		var result any
		if err := chromedp.Run(s.activeCtx(), chromedp.Evaluate(reverseAppJS, &result)); err != nil {
			return nil, nil, fmt.Errorf("inspect_fingerprint: %w", err)
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, nil, fmt.Errorf("inspect_fingerprint: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}
