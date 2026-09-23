// webrtc_helper.js — RTCPeerConnection monitoring for cdpscripttest.
//
// Injected via Page.addScriptToEvaluateOnNewDocument before navigation.
// All globals are namespaced with __cdpst_rtc_ to avoid page collisions.
(function() {
  'use strict';

  // Guard against double-injection.
  if (window.__cdpst_rtc_initialized) return;
  window.__cdpst_rtc_initialized = true;

  // Incrementing counter for stable peer connection IDs.
  var nextID = 0;

  // Map<int, RTCPeerConnection> — all tracked connections.
  var connections = new Map();
  window.__cdpst_rtc_connections = connections;

  // Array of {id, type, state, timestamp} event records.
  var events = [];
  window.__cdpst_rtc_events = events;

  // Map<int, {local: string|null, remote: string|null}> — captured SDP.
  var sdpMap = new Map();
  window.__cdpst_rtc_sdp = sdpMap;

  // Map<int, {local: Array, remote: Array}> — captured ICE candidates.
  var iceMap = new Map();
  window.__cdpst_rtc_ice = iceMap;

  // Map<int, Array> — cached getStats() results per connection.
  var statsMap = new Map();
  window.__cdpst_rtc_stats = statsMap;

  // Map<int, Map<string, RTCDataChannel>> — tracked data channels per connection.
  var datachannels = new Map();
  window.__cdpst_rtc_datachannels = datachannels;

  // Map<int, number> — interval IDs for stats polling per connection.
  var pollIntervals = new Map();

  // Save the original constructor.
  var OriginalRTCPeerConnection = window.RTCPeerConnection;

  // logEvent records a state change event for the given connection ID.
  function logEvent(id, type, state) {
    events.push({
      id: id,
      type: type,
      state: state,
      timestamp: Date.now()
    });
  }

  // startStatsPolling begins polling getStats() every 1000ms for a connection.
  function startStatsPolling(id, pc) {
    if (pollIntervals.has(id)) return;
    var interval = setInterval(function() {
      // Stop if the connection is gone or closed.
      if (!connections.has(id) || pc.connectionState === 'closed') {
        clearInterval(interval);
        pollIntervals.delete(id);
        return;
      }
      pc.getStats().then(function(report) {
        var result = [];
        report.forEach(function(stat) {
          result.push(stat);
        });
        statsMap.set(id, result);
      }).catch(function() {
        // Connection may have been destroyed; stop polling.
        clearInterval(interval);
        pollIntervals.delete(id);
      });
    }, 1000);
    pollIntervals.set(id, interval);
  }

  // Wrapped RTCPeerConnection constructor.
  window.RTCPeerConnection = function(config, constraints) {
    var pc;
    if (constraints !== undefined) {
      pc = new OriginalRTCPeerConnection(config, constraints);
    } else if (config !== undefined) {
      pc = new OriginalRTCPeerConnection(config);
    } else {
      pc = new OriginalRTCPeerConnection();
    }

    var id = nextID++;
    pc.__cdpst_rtc_id = id;
    connections.set(id, pc);
    sdpMap.set(id, {local: null, remote: null});
    iceMap.set(id, {local: [], remote: []});

    // Track state change events.
    pc.addEventListener('connectionstatechange', function() {
      logEvent(id, 'connectionstatechange', pc.connectionState);
      if (pc.connectionState === 'closed') {
        // Stop polling when connection closes.
        if (pollIntervals.has(id)) {
          clearInterval(pollIntervals.get(id));
          pollIntervals.delete(id);
        }
      }
    });
    pc.addEventListener('iceconnectionstatechange', function() {
      logEvent(id, 'iceconnectionstatechange', pc.iceConnectionState);
    });
    pc.addEventListener('icegatheringstatechange', function() {
      logEvent(id, 'icegatheringstatechange', pc.iceGatheringState);
    });
    pc.addEventListener('signalingstatechange', function() {
      logEvent(id, 'signalingstatechange', pc.signalingState);
    });

    // Wrap setLocalDescription to capture SDP.
    var origSetLocal = pc.setLocalDescription.bind(pc);
    pc.setLocalDescription = function(desc) {
      if (desc && desc.sdp) {
        var entry = sdpMap.get(id);
        if (entry) entry.local = desc.sdp;
      }
      return origSetLocal.apply(this, arguments);
    };

    // Wrap setRemoteDescription to capture SDP.
    var origSetRemote = pc.setRemoteDescription.bind(pc);
    pc.setRemoteDescription = function(desc) {
      if (desc && desc.sdp) {
        var entry = sdpMap.get(id);
        if (entry) entry.remote = desc.sdp;
      }
      return origSetRemote.apply(this, arguments);
    };

    // Wrap addIceCandidate to capture remote ICE candidates.
    var origAddIce = pc.addIceCandidate.bind(pc);
    pc.addIceCandidate = function(candidate) {
      if (candidate && candidate.candidate) {
        var entry = iceMap.get(id);
        if (entry) entry.remote.push(candidate.candidate);
      }
      return origAddIce.apply(this, arguments);
    };

    // Capture local ICE candidates via onicecandidate.
    pc.addEventListener('icecandidate', function(evt) {
      if (evt.candidate && evt.candidate.candidate) {
        var entry = iceMap.get(id);
        if (entry) entry.local.push(evt.candidate.candidate);
      }
    });

    // Track data channels.
    datachannels.set(id, new Map());

    // Wrap createDataChannel to track outgoing channels.
    var origCreateDC = pc.createDataChannel.bind(pc);
    pc.createDataChannel = function(label) {
      var dc = origCreateDC.apply(this, arguments);
      datachannels.get(id).set(dc.label, dc);
      return dc;
    };

    // Track incoming data channels via ondatachannel event.
    pc.addEventListener('datachannel', function(evt) {
      if (evt.channel) {
        datachannels.get(id).set(evt.channel.label, evt.channel);
      }
    });

    // Begin stats polling for this connection.
    startStatsPolling(id, pc);

    return pc;
  };

  // Preserve prototype chain so instanceof checks work.
  window.RTCPeerConnection.prototype = OriginalRTCPeerConnection.prototype;

  // Copy static properties (generateCertificate, etc.).
  Object.keys(OriginalRTCPeerConnection).forEach(function(key) {
    window.RTCPeerConnection[key] = OriginalRTCPeerConnection[key];
  });

  // --- Accessor functions ---

  // Returns array of {id, connectionState, iceConnectionState, signalingState}.
  window.__cdpst_rtc_getPeers = function() {
    var result = [];
    connections.forEach(function(pc, id) {
      result.push({
        id: id,
        connectionState: pc.connectionState,
        iceConnectionState: pc.iceConnectionState,
        signalingState: pc.signalingState
      });
    });
    return result;
  };

  // Returns connectionState string for the given ID.
  window.__cdpst_rtc_getState = function(id) {
    var pc = connections.get(id);
    if (!pc) return null;
    return pc.connectionState;
  };

  // Returns cached stats array (synchronous) for the given ID.
  window.__cdpst_rtc_getStats = function(id) {
    return statsMap.get(id) || [];
  };

  // Returns filtered video RTP stats for the given ID and direction.
  // direction: 'inbound' or 'outbound'
  window.__cdpst_rtc_getStatsVideo = function(id, direction) {
    var stats = statsMap.get(id) || [];
    var rtpType = direction === 'inbound' ? 'inbound-rtp' : 'outbound-rtp';
    var result = [];
    for (var i = 0; i < stats.length; i++) {
      var s = stats[i];
      if (s.type === rtpType && s.kind === 'video') {
        result.push(s);
      }
    }
    return result;
  };

  // Returns filtered audio RTP stats for the given ID and direction.
  // direction: 'inbound' or 'outbound'
  window.__cdpst_rtc_getStatsAudio = function(id, direction) {
    var stats = statsMap.get(id) || [];
    var rtpType = direction === 'inbound' ? 'inbound-rtp' : 'outbound-rtp';
    var result = [];
    for (var i = 0; i < stats.length; i++) {
      var s = stats[i];
      if (s.type === rtpType && s.kind === 'audio') {
        result.push(s);
      }
    }
    return result;
  };

  // Returns transport and candidate-pair stats for the given ID.
  window.__cdpst_rtc_getStatsTransport = function(id) {
    var stats = statsMap.get(id) || [];
    var result = [];
    for (var i = 0; i < stats.length; i++) {
      var s = stats[i];
      if (s.type === 'transport' || s.type === 'candidate-pair') {
        result.push(s);
      }
    }
    return result;
  };

  // Returns {local, remote} SDP strings for the given ID.
  window.__cdpst_rtc_getSDP = function(id) {
    return sdpMap.get(id) || {local: null, remote: null};
  };

  // Returns {local: [...], remote: [...]} ICE candidates for the given ID.
  window.__cdpst_rtc_getICE = function(id) {
    return iceMap.get(id) || {local: [], remote: []};
  };

  // Returns events array filtered by connection ID.
  window.__cdpst_rtc_getEvents = function(id) {
    if (id === undefined || id === null) return events;
    var result = [];
    for (var i = 0; i < events.length; i++) {
      if (events[i].id === id) result.push(events[i]);
    }
    return result;
  };

  // Returns array of {direction, kind, enabled, readyState} for all tracks.
  window.__cdpst_rtc_getTracks = function(id) {
    var pc = connections.get(id);
    if (!pc) return [];
    var result = [];
    // Senders (outgoing tracks).
    var senders = pc.getSenders();
    for (var i = 0; i < senders.length; i++) {
      var track = senders[i].track;
      if (track) {
        result.push({
          direction: 'send',
          kind: track.kind,
          enabled: track.enabled,
          readyState: track.readyState
        });
      }
    }
    // Receivers (incoming tracks).
    var receivers = pc.getReceivers();
    for (var j = 0; j < receivers.length; j++) {
      var rtrack = receivers[j].track;
      if (rtrack) {
        result.push({
          direction: 'recv',
          kind: rtrack.kind,
          enabled: rtrack.enabled,
          readyState: rtrack.readyState
        });
      }
    }
    return result;
  };

  // Returns a Promise that resolves when the connection reaches targetState,
  // or rejects on timeout. timeoutMs defaults to 30000.
  // Uses both event listeners and polling to avoid missed events under load.
  window.__cdpst_rtc_waitState = function(id, targetState, timeoutMs) {
    if (!timeoutMs) timeoutMs = 30000;
    var pc = connections.get(id);
    if (!pc) return Promise.reject(new Error('no connection with id ' + id));
    // Already in target state.
    if (pc.connectionState === targetState) return Promise.resolve(targetState);
    return new Promise(function(resolve, reject) {
      var settled = false;
      function done(err) {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        clearInterval(poller);
        pc.removeEventListener('connectionstatechange', handler);
        if (err) { reject(err); } else { resolve(targetState); }
      }
      var timer = setTimeout(function() {
        done(new Error('timeout waiting for state ' + targetState + ', current: ' + pc.connectionState));
      }, timeoutMs);
      function handler() {
        if (pc.connectionState === targetState) { done(null); }
      }
      pc.addEventListener('connectionstatechange', handler);
      // Poll every 200ms as fallback for missed events.
      var poller = setInterval(function() {
        if (pc.connectionState === targetState) { done(null); }
      }, 200);
    });
  };

  // --- getDisplayMedia mock (not auto-activated) ---

  // Replaces navigator.mediaDevices.getDisplayMedia with a canvas.captureStream().
  // Call this before the page invokes getDisplayMedia.
  window.__cdpst_rtc_mockScreenShare = function(width, height, fps) {
    width = width || 1920;
    height = height || 1080;
    fps = fps || 30;

    var canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    var ctx = canvas.getContext('2d');

    // Draw a test pattern: colored quadrants with timestamp.
    function drawFrame() {
      var w = canvas.width;
      var h = canvas.height;
      var hw = w / 2;
      var hh = h / 2;
      ctx.fillStyle = '#ff0000';
      ctx.fillRect(0, 0, hw, hh);
      ctx.fillStyle = '#00ff00';
      ctx.fillRect(hw, 0, hw, hh);
      ctx.fillStyle = '#0000ff';
      ctx.fillRect(0, hh, hw, hh);
      ctx.fillStyle = '#ffff00';
      ctx.fillRect(hw, hh, hw, hh);
      ctx.fillStyle = '#ffffff';
      ctx.font = Math.round(h / 10) + 'px monospace';
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillText(new Date().toISOString(), w / 2, h / 2);
      requestAnimationFrame(drawFrame);
    }
    drawFrame();

    var stream = canvas.captureStream(fps);

    navigator.mediaDevices.getDisplayMedia = function() {
      return Promise.resolve(stream);
    };

    return {width: width, height: height, fps: fps};
  };

})();
