package recorder

// FetchCaptureScript is injected via Page.addScriptToEvaluateOnNewDocument
// to intercept fetch() calls to gRPC-Web streaming endpoints. It tees the
// ReadableStream so the page receives data normally while we capture chunks
// via structured console.log messages.
const FetchCaptureScript = `(function() {
  if (window.__cdpFetchCapture) return;
  window.__cdpFetchCapture = true;

  const origFetch = window.fetch;

  // Patterns that identify gRPC-Web streaming endpoints.
  const GRPC_PATTERNS = [
    'GenerateFreeFormStreamed',
    'LabsTailwindOrchestrationService',
    '/data/google.internal.labs',
    '$rpc',
  ];

  function isGRPCEndpoint(url) {
    for (const p of GRPC_PATTERNS) {
      if (url.includes(p)) return true;
    }
    return false;
  }

  window.fetch = async function(...args) {
    const input = args[0];
    const init = args[1] || {};
    const url = (typeof input === 'string') ? input : (input?.url || '');

    if (!isGRPCEndpoint(url)) {
      return origFetch.apply(this, args);
    }

    // Capture request details.
    let reqBody = '';
    if (init.body) {
      try {
        if (typeof init.body === 'string') {
          reqBody = init.body;
        } else if (init.body instanceof URLSearchParams) {
          reqBody = init.body.toString();
        } else if (init.body instanceof FormData) {
          reqBody = '[FormData]';
        } else if (init.body instanceof ArrayBuffer || ArrayBuffer.isView(init.body)) {
          reqBody = '[Binary ' + (init.body.byteLength || init.body.buffer?.byteLength || 0) + ' bytes]';
        }
      } catch(e) {
        reqBody = '[unreadable]';
      }
    }

    let reqHeaders = '';
    try {
      const h = init.headers || (input instanceof Request ? input.headers : null);
      if (h) {
        const pairs = [];
        if (h instanceof Headers) {
          h.forEach((v, k) => pairs.push(k + ': ' + v));
        } else if (typeof h === 'object') {
          for (const [k, v] of Object.entries(h)) pairs.push(k + ': ' + v);
        }
        reqHeaders = pairs.join('\\n');
      }
    } catch(e) {}

    console.log('CDP_GRPC:' + JSON.stringify({
      type: 'request',
      url: url,
      method: init.method || 'GET',
      headers: reqHeaders,
      body: reqBody,
    }));

    // Call original fetch.
    const resp = await origFetch.apply(this, args);

    // If no body or not a ReadableStream, return as-is.
    if (!resp.body || typeof resp.body.tee !== 'function') {
      return resp;
    }

    // Capture response headers for the complete event.
    let respContentType = '';
    let respHeaderPairs = [];
    try {
      resp.headers.forEach((v, k) => {
        respHeaderPairs.push(k + ': ' + v);
        if (k.toLowerCase() === 'content-type') respContentType = v;
      });
    } catch(e) {}

    // Tee the stream: one for the page, one for capture.
    const [s1, s2] = resp.body.tee();
    const reader = s2.getReader();
    const decoder = new TextDecoder();
    const status = resp.status;
    const statusText = resp.statusText || '';
    const method = init.method || 'GET';

    // Read capture stream in background.
    (async () => {
      const chunks = [];
      let idx = 0;
      try {
        while (true) {
          const {done, value} = await reader.read();
          if (done) break;
          const text = decoder.decode(value, {stream: true});
          chunks.push(text);
          console.log('CDP_GRPC:' + JSON.stringify({
            type: 'chunk',
            url: url,
            chunkIdx: idx++,
            chunk: text,
          }));
        }
      } catch(e) {
        // Stream error — emit what we have.
      }

      // Emit complete event with full concatenated response.
      console.log('CDP_GRPC:' + JSON.stringify({
        type: 'complete',
        url: url,
        method: method,
        headers: reqHeaders,
        respHeaders: respHeaderPairs.join('\\n'),
        respContentType: respContentType,
        body: reqBody,
        status: status,
        statusText: statusText,
        full: chunks.join(''),
      }));
    })();

    // Return a new Response with the untouched stream for the page.
    return new Response(s1, {
      status: resp.status,
      statusText: resp.statusText,
      headers: resp.headers,
    });
  };
})();`

// WebRTCCaptureScript is injected via Page.addScriptToEvaluateOnNewDocument
// to intercept RTCPeerConnection creation, DataChannel messages, and SDP
// offer/answer exchange. Captured via structured console.log messages.
const WebRTCCaptureScript = `(function() {
  if (window.__cdpWebRTCCapture) return;
  window.__cdpWebRTCCapture = true;

  const OrigPC = window.RTCPeerConnection || window.webkitRTCPeerConnection;
  if (!OrigPC) return;

  function serializePayload(raw) {
    if (typeof raw === 'string') return { data: raw, binary: false };
    if (raw instanceof ArrayBuffer) {
      const bytes = new Uint8Array(raw);
      let bin = '';
      for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
      return { data: btoa(bin), binary: true };
    }
    if (ArrayBuffer.isView(raw)) {
      const bytes = new Uint8Array(raw.buffer, raw.byteOffset, raw.byteLength);
      let bin = '';
      for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
      return { data: btoa(bin), binary: true };
    }
    if (raw instanceof Blob) return { data: '[Blob ' + raw.size + ' bytes]', binary: true };
    return { data: String(raw), binary: false };
  }

  function wrapDataChannel(dc, source) {
    // Capture inbound messages.
    dc.addEventListener('message', function(e) {
      const s = serializePayload(e.data);
      console.log('CDP_DC:' + JSON.stringify({
        type: 'message',
        label: dc.label || '',
        dir: 'incoming',
        data: s.data,
        binary: s.binary,
      }));
    });

    // Capture outbound send() calls.
    const origSend = dc.send.bind(dc);
    dc.send = function(payload) {
      const s = serializePayload(payload);
      console.log('CDP_DC:' + JSON.stringify({
        type: 'message',
        label: dc.label || '',
        dir: 'outgoing',
        data: s.data,
        binary: s.binary,
      }));
      return origSend(payload);
    };
  }

  function PatchedPC(config) {
    const pc = new OrigPC(config);

    // Wrap createDataChannel.
    const origCreateDC = pc.createDataChannel.bind(pc);
    pc.createDataChannel = function(label, opts) {
      const dc = origCreateDC(label, opts);
      wrapDataChannel(dc, 'outgoing');
      return dc;
    };

    // Capture remote data channels.
    pc.addEventListener('datachannel', function(e) {
      wrapDataChannel(e.channel, 'incoming');
    });

    // Capture SDP exchange.
    const origSLD = pc.setLocalDescription.bind(pc);
    pc.setLocalDescription = function(desc) {
      console.log('CDP_DC:' + JSON.stringify({
        type: 'sdp-local',
        sdpType: desc?.type || '',
        sdp: desc?.sdp || '',
      }));
      return origSLD(desc);
    };

    const origSRD = pc.setRemoteDescription.bind(pc);
    pc.setRemoteDescription = function(desc) {
      console.log('CDP_DC:' + JSON.stringify({
        type: 'sdp-remote',
        sdpType: desc?.type || '',
        sdp: desc?.sdp || '',
      }));
      return origSRD(desc);
    };

    return pc;
  }

  // Preserve static methods and prototype.
  PatchedPC.prototype = OrigPC.prototype;
  PatchedPC.generateCertificate = OrigPC.generateCertificate;

  window.RTCPeerConnection = PatchedPC;
  if (window.webkitRTCPeerConnection) {
    window.webkitRTCPeerConnection = PatchedPC;
  }
})();`
