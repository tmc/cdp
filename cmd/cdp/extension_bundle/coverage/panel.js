// CDP Coverage Explorer — DevTools panel
(function () {
  "use strict";

  const $ = (sel) => document.querySelector(sel);
  const apiInput = $("#api-url");
  const btnRefresh = $("#btn-refresh");
  const btnMode = $("#btn-mode");
  const btnExport = $("#btn-export");
  const statusEl = $("#status");
  const timelineEl = $("#timeline");
  const detailEl = $("#detail");

  let snapshots = [];
  let selected = null;   // snapshot name
  let compareWith = null; // second snapshot name for compare mode
  let deltaMode = true;
  let pollTimer = null;

  // Restore saved API URL.
  chrome.storage?.local?.get("apiUrl", (r) => {
    if (r.apiUrl) apiInput.value = r.apiUrl;
  });
  apiInput.addEventListener("change", () => {
    chrome.storage?.local?.set({ apiUrl: apiInput.value });
  });

  function apiBase() {
    return apiInput.value.replace(/\/+$/, "");
  }

  async function apiFetch(path) {
    const resp = await fetch(apiBase() + path);
    if (!resp.ok) throw new Error(`${resp.status} ${resp.statusText}`);
    return resp;
  }

  // --- Data fetching ---

  async function fetchSnapshots() {
    try {
      const resp = await apiFetch("/api/coverage/snapshots");
      snapshots = await resp.json();
      renderTimeline();
      statusEl.textContent = `${snapshots.length} snapshot(s)`;
    } catch (e) {
      statusEl.textContent = e.message;
      snapshots = [];
      renderTimeline();
    }
  }

  async function fetchDetail(name) {
    try {
      const resp = await apiFetch(`/api/coverage/snapshot/${encodeURIComponent(name)}`);
      return await resp.json();
    } catch (e) {
      statusEl.textContent = e.message;
      return null;
    }
  }

  async function fetchDelta(before, after) {
    try {
      const resp = await apiFetch(
        `/api/coverage/delta?before=${encodeURIComponent(before)}&after=${encodeURIComponent(after)}`
      );
      return await resp.json();
    } catch (e) {
      statusEl.textContent = e.message;
      return null;
    }
  }

  // --- Rendering ---

  function pctColor(pct) {
    if (pct >= 60) return "var(--green)";
    if (pct >= 30) return "var(--yellow)";
    return "var(--red)";
  }

  function renderTimeline() {
    timelineEl.innerHTML = "";
    if (snapshots.length === 0) {
      timelineEl.innerHTML = '<div class="empty" style="width:100%">No snapshots</div>';
      return;
    }
    snapshots.forEach((s, i) => {
      const block = document.createElement("div");
      block.className = "ctx-block" + (s.name === selected ? " selected" : "");
      block.style.background = pctColor(s.coverage_percent);
      block.style.color = "#000";
      block.innerHTML = `<span class="name">${esc(s.name)}</span><span class="pct">${s.coverage_percent.toFixed(1)}%</span>`;
      block.addEventListener("click", (e) => {
        if (e.shiftKey && selected && selected !== s.name) {
          compareWith = s.name;
          showComparison(selected, compareWith);
        } else {
          selected = s.name;
          compareWith = null;
          selectSnapshot(s.name, i);
        }
        renderTimeline();
      });
      timelineEl.appendChild(block);
    });
  }

  async function selectSnapshot(name, idx) {
    if (deltaMode && idx > 0) {
      const prevName = snapshots[idx - 1].name;
      const delta = await fetchDelta(prevName, name);
      if (delta) {
        renderDelta(delta, prevName, name);
        return;
      }
    }
    const data = await fetchDetail(name);
    if (data) renderDetail(data, name);
  }

  async function showComparison(a, b) {
    const delta = await fetchDelta(a, b);
    if (delta) renderDelta(delta, a, b);
  }

  function renderDetail(summary, name) {
    const files = Object.values(summary).sort((a, b) => a.coverage_percent - b.coverage_percent);
    let totalLines = 0, coveredLines = 0;
    files.forEach((f) => { totalLines += f.total_lines; coveredLines += f.covered_lines; });
    const overallPct = totalLines > 0 ? (coveredLines / totalLines * 100) : 0;

    let html = `<h3>${esc(name)} &mdash; ${overallPct.toFixed(1)}% (${coveredLines}/${totalLines} lines, ${files.length} files)</h3>`;
    html += `<table><thead><tr><th>File</th><th>Covered</th><th>Total</th><th>%</th><th></th></tr></thead><tbody>`;
    files.forEach((f) => {
      const pct = f.coverage_percent.toFixed(1);
      html += `<tr>
        <td><a class="link" data-url="${esc(f.url)}">${esc(shortURL(f.url))}</a></td>
        <td class="num">${f.covered_lines}</td>
        <td class="num">${f.total_lines}</td>
        <td class="num">${pct}%</td>
        <td><span class="pct-bar" style="width:${Math.max(pct, 2)}px;background:${pctColor(f.coverage_percent)}"></span></td>
      </tr>`;
    });
    html += "</tbody></table>";
    detailEl.innerHTML = html;
    bindSourceLinks();
  }

  function renderDelta(delta, beforeName, afterName) {
    const scripts = Object.values(delta.scripts || {}).sort(
      (a, b) => Object.keys(b.newly_covered || {}).length - Object.keys(a.newly_covered || {}).length
    );
    let totalNew = 0;
    scripts.forEach((s) => { totalNew += Object.keys(s.newly_covered || {}).length; });

    let html = `<h3>Delta: ${esc(beforeName)} &rarr; ${esc(afterName)} &mdash; +${totalNew} new lines across ${scripts.length} files</h3>`;
    html += `<table><thead><tr><th>File</th><th>Before</th><th>After</th><th>Delta</th><th>New Lines</th></tr></thead><tbody>`;
    scripts.forEach((s) => {
      const newCount = Object.keys(s.newly_covered || {}).length;
      if (newCount === 0 && s.covered_before === s.covered_after) return;
      const diff = s.covered_after - s.covered_before;
      const cls = diff > 0 ? "delta-pos" : diff < 0 ? "delta-neg" : "";
      const beforePct = s.total_lines > 0 ? (s.covered_before / s.total_lines * 100).toFixed(1) : "0.0";
      const afterPct = s.total_lines > 0 ? (s.covered_after / s.total_lines * 100).toFixed(1) : "0.0";
      html += `<tr>
        <td><a class="link" data-url="${esc(s.url)}">${esc(shortURL(s.url))}</a></td>
        <td class="num">${beforePct}%</td>
        <td class="num">${afterPct}%</td>
        <td class="num ${cls}">${diff > 0 ? "+" : ""}${diff}</td>
        <td class="num">${newCount}</td>
      </tr>`;
    });
    html += "</tbody></table>";
    detailEl.innerHTML = html;
    bindSourceLinks();
  }

  function bindSourceLinks() {
    detailEl.querySelectorAll("a.link[data-url]").forEach((a) => {
      a.addEventListener("click", (e) => {
        e.preventDefault();
        const url = a.dataset.url;
        if (chrome.devtools?.panels) {
          chrome.devtools.panels.openResource(url, 0);
        }
      });
    });
  }

  // --- Export ---

  async function exportLcov() {
    if (!selected) { statusEl.textContent = "select a snapshot first"; return; }
    try {
      const resp = await apiFetch(`/api/coverage/lcov?name=${encodeURIComponent(selected)}`);
      const text = await resp.text();
      const blob = new Blob([text], { type: "text/plain" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${selected}.lcov`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      statusEl.textContent = e.message;
    }
  }

  // --- Utilities ---

  function shortURL(url) {
    try {
      const u = new URL(url);
      return u.pathname.split("/").slice(-2).join("/");
    } catch {
      return url.length > 60 ? "..." + url.slice(-57) : url;
    }
  }

  function esc(s) {
    const d = document.createElement("div");
    d.textContent = s;
    return d.innerHTML;
  }

  // --- Event wiring ---

  btnRefresh.addEventListener("click", fetchSnapshots);
  btnMode.addEventListener("click", () => {
    deltaMode = !deltaMode;
    btnMode.textContent = deltaMode ? "Delta" : "Cumulative";
    btnMode.classList.toggle("active", deltaMode);
    if (selected) {
      const idx = snapshots.findIndex((s) => s.name === selected);
      selectSnapshot(selected, idx >= 0 ? idx : 0);
    }
  });
  btnExport.addEventListener("click", exportLcov);

  // Auto-poll every 5 seconds.
  fetchSnapshots();
  pollTimer = setInterval(fetchSnapshots, 5000);
})();
