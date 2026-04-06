package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chromedp/chromedp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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

type ReverseAppInput struct{}

func registerReverseTools(server *mcp.Server, s *mcpSession) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "reverse_app",
		Description: `Automated app fingerprinting. Detects: identity (title, URL, Electron/Chrome versions), frameworks (React, Vue, Svelte, Angular), bundler (webpack, Vite), non-standard globals, bridge APIs (electronBridge, vscode), feature flags, error monitoring, script inventory, CSP.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ReverseAppInput) (*mcp.CallToolResult, any, error) {
		var result any
		if err := chromedp.Run(s.activeCtx(), chromedp.Evaluate(reverseAppJS, &result)); err != nil {
			return nil, nil, fmt.Errorf("reverse_app: %w", err)
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, nil, fmt.Errorf("reverse_app: marshal: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})
}
