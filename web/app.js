/* ═══════════════════════════════════════════════════════════════════
   iTaK Agent Dashboard - Application
   Vanilla JS SPA: router + API client + 6 page renderers
   ═══════════════════════════════════════════════════════════════════ */

// ── State ─────────────────────────────────────────────────────────
const state = {
  page: 'chat',
  agents: [],
  status: null,
  snapshot: null,
  chatMessages: [],
  chatAgent: null,
  logs: [],
  logFilter: 'all',
  fsEvents: [],
  connected: false,
  ws: null,
};

// ── API Client ────────────────────────────────────────────────────
const API_BASE = window.location.origin;

async function api(path, opts = {}) {
  try {
    const res = await fetch(`${API_BASE}${path}`, {
      headers: { 'Content-Type': 'application/json' },
      ...opts,
    });
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
    return await res.json();
  } catch (err) {
    console.error(`API ${path}:`, err);
    return null;
  }
}

async function fetchStatus() {
  const data = await api('/v1/status');
  if (data) {
    state.status = data;
    state.connected = true;
    updateStatusIndicator();
  }
  return data;
}

async function fetchAgents() {
  const data = await api('/v1/agents');
  if (data && data.agents) {
    state.agents = data.agents;
  }
  return data;
}

async function fetchSnapshot() {
  const data = await api('/debug/snapshot');
  if (data) state.snapshot = data;
  return data;
}

async function sendChat(message, agentName) {
  if (agentName) {
    return await api(`/v1/agents/${agentName}/chat`, {
      method: 'POST',
      body: JSON.stringify({ message }),
    });
  }
  return await api('/v1/chat', {
    method: 'POST',
    body: JSON.stringify({ message }),
  });
}

async function fetchTokens() {
  return await api('/v1/tokens');
}

async function fetchDoctor() {
  return await api('/v1/doctor');
}

// ── SSE (live events) ───────────────────────────────────────────────
function connectWS() {
  const ws = new EventSource(`${location.protocol}//${location.host}/v1/events`);

  ws.onopen = () => {
    state.ws = ws;
    addLog('info', 'dashboard', 'Connected to event stream');
  };

  ws.onmessage = (e) => {
    try {
      const evt = JSON.parse(e.data);
      if (evt.type === 'connected') return;

      if (evt.topic === 'fs.activity' && evt.data) {
        state.fsEvents.unshift({
          action: evt.data.action,
          path: evt.data.path,
          size: evt.data.size || 0,
          time: new Date().toLocaleTimeString('en-US', { hour12: false })
        });
        if (state.fsEvents.length > 50) state.fsEvents.length = 50;
        if (state.page === 'overview') renderActivityFeed();
      } else {
        addLog(evt.level || 'info', evt.source || 'system', evt.message || JSON.stringify(evt));
      }
    } catch {
      addLog('info', 'event', e.data);
    }
  };

  ws.onerror = () => {
    if (state.ws != null) {
      state.ws = null;
      addLog('warn', 'dashboard', 'Event stream disconnected. Reconnecting...');
    }
  };
}

// ── Logging ───────────────────────────────────────────────────────
function addLog(level, source, message) {
  const entry = {
    time: new Date().toLocaleTimeString('en-US', { hour12: false }),
    level,
    source,
    message,
  };
  state.logs.unshift(entry);
  if (state.logs.length > 500) state.logs.length = 500;

  // Live-update if on logs page.
  if (state.page === 'logs') renderLogs();
}

// ── Router ────────────────────────────────────────────────────────
function navigate(page) {
  state.page = page;
  window.location.hash = page;

  // Update sidebar active state.
  document.querySelectorAll('.nav-item').forEach(item => {
    item.classList.toggle('active', item.dataset.page === page);
  });

  // Update header title.
  const titles = {
    chat: 'Chat',
    overview: 'Overview',
    analytics: 'Analytics',
    logs: 'Logs',
    agents: 'Agents',
    settings: 'Settings',
  };
  document.getElementById('page-title').textContent = titles[page] || page;

  renderPage();
}

async function renderPage() {
  const content = document.getElementById('page-content');
  switch (state.page) {
    case 'chat': renderChat(content); break;
    case 'overview': await renderOverview(content); break;
    case 'analytics': await renderAnalytics(content); break;
    case 'logs': renderLogs(content); break;
    case 'agents': await renderAgentsPage(content); break;
    case 'settings': renderSettings(content); break;
    default: renderChat(content);
  }
}

// ── Theme ─────────────────────────────────────────────────────────
function toggleTheme() {
  const html = document.documentElement;
  const current = html.getAttribute('data-theme');
  const next = current === 'dark' ? 'light' : 'dark';
  html.setAttribute('data-theme', next);
  localStorage.setItem('itak-theme', next);
}

function loadTheme() {
  const saved = localStorage.getItem('itak-theme') || 'dark';
  document.documentElement.setAttribute('data-theme', saved);
}

// ── Status Indicator ──────────────────────────────────────────────
function updateStatusIndicator() {
  const dot = document.getElementById('status-dot');
  const text = document.getElementById('status-text');
  const ver = document.getElementById('version-label');

  if (state.connected && state.status) {
    dot.style.background = 'var(--green)';
    text.textContent = `${state.status.agents || 0} agent(s) running`;
    text.style.color = 'var(--green)';
    if (state.status.version) ver.textContent = `v${state.status.version}`;
  } else {
    dot.style.background = 'var(--red)';
    text.textContent = 'Disconnected';
    text.style.color = 'var(--red)';
  }
}

// ── Page: Chat ────────────────────────────────────────────────────
function renderChat(container) {
  if (!container) container = document.getElementById('page-content');

  // Build agent selector.
  const agentOpts = state.agents.map(a =>
    `<option value="${a.name}" ${state.chatAgent === a.name ? 'selected' : ''}>${a.name}</option>`
  ).join('');

  const messages = state.chatMessages.map(m => `
    <div class="chat-msg ${m.role}">
      ${m.agent ? `<div class="msg-agent">${m.agent}</div>` : ''}
      ${escapeHtml(m.content)}
    </div>
  `).join('');

  container.innerHTML = `
    <div class="chat-container">
      <div style="display:flex;gap:8px;align-items:center;margin-bottom:8px;">
        <span class="section-label" style="margin:0;">Agent:</span>
        <select id="chat-agent-select" onchange="state.chatAgent=this.value" style="
          padding:6px 10px;
          background:var(--bg-input);
          border:1px solid var(--border);
          border-radius:var(--radius-sm);
          color:var(--text-primary);
          font-family:var(--font);
          font-size:12px;
        ">
          <option value="">Orchestrator (auto-route)</option>
          ${agentOpts}
        </select>
      </div>

      <div class="chat-messages" id="chat-messages">
        ${messages || `
          <div class="empty-state">
            <div class="empty-icon">💬</div>
            <div class="empty-text">Send a message to get started</div>
          </div>
        `}
      </div>

      <div class="chat-input-area">
        <input type="text" id="chat-input" placeholder="Type a message... (Enter to send)" autofocus
          onkeydown="if(event.key==='Enter')handleChatSend()">
        <button class="btn btn-primary" onclick="handleChatSend()">Send</button>
      </div>
    </div>
  `;

  // Scroll to bottom.
  const msgBox = document.getElementById('chat-messages');
  if (msgBox) msgBox.scrollTop = msgBox.scrollHeight;
}

async function handleChatSend() {
  const input = document.getElementById('chat-input');
  if (!input) return;
  const msg = input.value.trim();
  if (!msg) return;
  input.value = '';

  const agent = state.chatAgent || '';
  state.chatMessages.push({ role: 'user', content: msg, agent: agent || null });
  renderChat();

  // Show spinner.
  const msgBox = document.getElementById('chat-messages');
  const spinner = document.createElement('div');
  spinner.className = 'chat-msg assistant';
  spinner.innerHTML = '<div class="spinner"></div>';
  msgBox.appendChild(spinner);
  msgBox.scrollTop = msgBox.scrollHeight;

  const result = await sendChat(msg, agent);
  spinner.remove();

  if (result) {
    state.chatMessages.push({
      role: 'assistant',
      content: result.response || result.error || 'No response',
      agent: agent || 'orchestrator',
    });
  } else {
    state.chatMessages.push({
      role: 'assistant',
      content: 'Failed to reach the agent. Check if the server is running.',
      agent: 'system',
    });
  }

  renderChat();
}

// ── Page: Overview ────────────────────────────────────────────────
async function renderOverview(container) {
  if (!container) container = document.getElementById('page-content');
  container.innerHTML = '<div class="spinner" style="margin:40px auto;"></div>';

  await Promise.all([fetchStatus(), fetchSnapshot(), fetchAgents()]);
  if (state.page !== 'overview') return;

  const s = state.status || {};
  const snap = state.snapshot || {};
  const sys = snap.system || {};
  const mem = snap.memory || {};
  const tokens = snap.tokens || {};

  container.innerHTML = `
    <div class="stat-grid" style="margin-bottom:24px;">
      <div class="stat-card">
        <div class="stat-label">Agents</div>
        <div class="stat-value">${s.agents || 0}</div>
        <div class="stat-sub">Active agents</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Uptime</div>
        <div class="stat-value">${formatUptime(s.uptime_secs)}</div>
        <div class="stat-sub">${s.uptime || '-'}</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Memory</div>
        <div class="stat-value">${s.memory_mb || sys.heap_mb || '?'} MB</div>
        <div class="stat-sub">Heap allocated</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Goroutines</div>
        <div class="stat-value">${s.goroutines || sys.goroutines || '?'}</div>
        <div class="stat-sub">${s.go_version || ''}</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">CPUs</div>
        <div class="stat-value">${sys.cpus || '?'}</div>
        <div class="stat-sub">${s.os || ''} / ${s.arch || ''}</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Session</div>
        <div class="stat-value">${mem.session_messages || 0}</div>
        <div class="stat-sub">Messages in memory</div>
      </div>
    </div>

    <div class="section-label">Your Agents</div>
    <div class="card-grid">
      ${state.agents.map(a => agentCard(a)).join('')}
    </div>

    ${tokens.summary ? `
      <div class="section-label" style="margin-top:24px;">Tokens</div>
      <div class="card" style="font-family:var(--mono);font-size:12px;white-space:pre-wrap;">${escapeHtml(tokens.summary)}</div>
    ` : ''}

    <div class="section-label" style="margin-top:24px;">File System Activity</div>
    <div class="card" id="activity-feed">
      <!-- Injected by renderActivityFeed() -->
    </div>
  `;
  renderActivityFeed();
}

function renderActivityFeed() {
  const container = document.getElementById('activity-feed');
  if (!container) return;

  if (state.fsEvents.length === 0) {
    container.innerHTML = '<div class="empty-text" style="padding:20px;text-align:center;">No recent file activity</div>';
    return;
  }

  container.innerHTML = state.fsEvents.map(evt => {
    let icon = '📄';
    let color = 'var(--text-primary)';
    if (evt.action === 'read') { icon = '👁️'; color = 'var(--blue)'; }
    if (evt.action === 'write') { icon = '✏️'; color = 'var(--green)'; }
    if (evt.action === 'list') { icon = '📁'; color = 'var(--orange)'; }
    
    return `
      <div style="display:flex;gap:12px;padding:8px 0;border-bottom:1px solid var(--border);align-items:center;font-size:13px;font-family:var(--mono);">
        <span style="color:var(--text-muted);min-width:60px;">${evt.time}</span>
        <span style="color:${color};width:20px;text-align:center;">${icon}</span>
        <span style="color:${color};font-weight:600;min-width:50px;">${evt.action.toUpperCase()}</span>
        <span style="flex-grow:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${evt.path}">${escapeHtml(evt.path)}</span>
        ${evt.size ? `<span style="color:var(--text-muted);">${formatSize(evt.size)}</span>` : ''}
      </div>
    `;
  }).join('');
}

function formatSize(bytes) {
  if (bytes >= 1024 * 1024) return (bytes / 1024 / 1024).toFixed(1) + ' MB';
  if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return bytes + ' B';
}

// ── Page: Agents ──────────────────────────────────────────────────
async function renderAgentsPage(container) {
  if (!container) container = document.getElementById('page-content');
  container.innerHTML = '<div class="spinner" style="margin:40px auto;"></div>';

  await fetchAgents();
  if (state.page !== 'agents') return;

  container.innerHTML = `
    <div class="section-label">Active Agents (${state.agents.length})</div>
    <div class="card-grid">
      ${state.agents.map(a => agentCardFull(a)).join('')}
    </div>

    ${state.agents.length === 0 ? `
      <div class="empty-state">
        <div class="empty-icon">🤖</div>
        <div class="empty-text">No agents configured. Add agents to your itakagent.yaml.</div>
      </div>
    ` : ''}
  `;
}

function agentCard(a) {
  const badgeClass = getBadgeClass(a.role);
  return `
    <div class="agent-card" onclick="startChatWith('${a.name}')">
      <div class="agent-card-head">
        <span class="agent-card-name">${capitalize(a.name)}</span>
        <span class="badge ${badgeClass}">${a.role || 'general'}</span>
      </div>
      <div class="agent-card-desc">${a.personality || 'No description'}</div>
    </div>
  `;
}

function agentCardFull(a) {
  const badgeClass = getBadgeClass(a.role);
  const tools = (a.tools || []).slice(0, 10).map(t =>
    `<span class="tool-tag">${t}</span>`
  ).join('');
  const more = (a.tools || []).length > 10 ? `<span class="tool-tag">+${a.tools.length - 10} more</span>` : '';

  return `
    <div class="agent-card" onclick="startChatWith('${a.name}')">
      <div class="agent-card-head">
        <span class="agent-card-name">${capitalize(a.name)}</span>
        <span class="badge ${badgeClass}">${a.role || 'general'}</span>
      </div>
      <div class="agent-card-desc" style="margin-bottom:4px;">${a.personality || 'No description'}</div>
      <div style="font-size:11px;color:var(--text-muted);font-family:var(--mono);">
        Model: ${a.model || 'default'} | Max loops: ${a.max_loops || '?'}
      </div>
      <div class="tool-tags">${tools}${more}</div>
    </div>
  `;
}

function startChatWith(name) {
  state.chatAgent = name;
  navigate('chat');
}

function getBadgeClass(role) {
  if (!role) return 'badge-general';
  const r = role.toLowerCase();
  if (r.includes('code') || r.includes('dev')) return 'badge-dev';
  if (r.includes('research') || r.includes('scout')) return 'badge-research';
  if (r.includes('ops') || r.includes('operator')) return 'badge-ops';
  if (r.includes('doctor') || r.includes('health')) return 'badge-health';
  return 'badge-general';
}

// ── Page: Analytics ───────────────────────────────────────────────
async function renderAnalytics(container) {
  if (!container) container = document.getElementById('page-content');
  container.innerHTML = '<div class="spinner" style="margin:40px auto;"></div>';

  const [tokenData, doctorData] = await Promise.all([fetchTokens(), fetchDoctor()]);
  await fetchSnapshot();
  if (state.page !== 'analytics') return;

  const snap = state.snapshot || {};
  const tokens = snap.tokens || {};
  const tasks = snap.tasks || {};

  container.innerHTML = `
    <div class="stat-grid" style="margin-bottom:24px;">
      <div class="stat-card">
        <div class="stat-label">Token Usage</div>
        <div class="stat-value">${tokenData ? formatNumber(tokenData.total_tokens || 0) : '-'}</div>
        <div class="stat-sub">Total tokens consumed</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Input Tokens</div>
        <div class="stat-value">${tokenData ? formatNumber(tokenData.input_tokens || 0) : '-'}</div>
        <div class="stat-sub">Prompt tokens</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Output Tokens</div>
        <div class="stat-value">${tokenData ? formatNumber(tokenData.output_tokens || 0) : '-'}</div>
        <div class="stat-sub">Completion tokens</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Est. Cost</div>
        <div class="stat-value">$${tokenData ? (tokenData.estimated_cost || 0).toFixed(4) : '0'}</div>
        <div class="stat-sub">USD estimate</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Active Tasks</div>
        <div class="stat-value">${tasks.active || 0}</div>
        <div class="stat-sub">${tasks.archived || 0} archived</div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Doctor</div>
        <div class="stat-value">${doctorData ? (doctorData.healing ? '🔧 Healing' : '✅ Healthy') : '?'}</div>
        <div class="stat-sub">${doctorData ? (doctorData.fix_count || 0) + ' fixes in memory' : ''}</div>
      </div>
    </div>

    ${tokens.summary ? `
      <div class="section-label">Token Summary</div>
      <div class="card" style="font-family:var(--mono);font-size:12px;white-space:pre-wrap;">${escapeHtml(tokens.summary)}</div>
    ` : ''}
  `;
}

// ── Page: Logs ────────────────────────────────────────────────────
function renderLogs(container) {
  if (!container) container = document.getElementById('page-content');

  const filters = ['all', 'info', 'warn', 'error', 'debug'];
  const filteredLogs = state.logFilter === 'all'
    ? state.logs
    : state.logs.filter(l => l.level === state.logFilter);

  container.innerHTML = `
    <div class="log-container">
      <div class="log-toolbar">
        ${filters.map(f => `
          <button class="log-filter ${state.logFilter === f ? 'active' : ''}"
            onclick="state.logFilter='${f}';renderLogs();">${f.toUpperCase()}</button>
        `).join('')}
        <button class="btn" style="margin-left:auto;font-size:12px;" onclick="state.logs=[];renderLogs();">Clear</button>
      </div>
      <div class="log-entries" id="log-entries">
        ${filteredLogs.length === 0 ? `
          <div class="empty-state">
            <div class="empty-icon">📋</div>
            <div class="empty-text">No logs yet. Events will appear here in real-time.</div>
          </div>
        ` : filteredLogs.map(l => `
          <div class="log-entry">
            <span class="log-time">${l.time}</span>
            <span class="log-level ${l.level}">${l.level.toUpperCase()}</span>
            <span class="log-source">${l.source}</span>
            <span class="log-message">${escapeHtml(l.message)}</span>
          </div>
        `).join('')}
      </div>
    </div>
  `;
}

// ── Page: Settings ────────────────────────────────────────────────
function renderSettings(container) {
  if (!container) container = document.getElementById('page-content');
  const isDark = document.documentElement.getAttribute('data-theme') === 'dark';

  container.innerHTML = `
    <div class="settings-section">
      <h3>Appearance</h3>
      <div class="setting-row">
        <div>
          <div class="setting-label">Dark Mode</div>
          <div class="setting-desc">Toggle between dark and light themes</div>
        </div>
        <label class="toggle">
          <input type="checkbox" ${isDark ? 'checked' : ''} onchange="toggleTheme();renderSettings();">
          <span class="toggle-slider"></span>
        </label>
      </div>
    </div>

    <div class="settings-section">
      <h3>Connection</h3>
      <div class="setting-row">
        <div>
          <div class="setting-label">API Endpoint</div>
          <div class="setting-desc">${API_BASE}</div>
        </div>
        <span class="badge ${state.connected ? 'badge-running' : ''}">${state.connected ? 'Connected' : 'Disconnected'}</span>
      </div>
      <div class="setting-row">
        <div>
          <div class="setting-label">WebSocket</div>
          <div class="setting-desc">Live event streaming</div>
        </div>
        <span class="badge ${state.ws ? 'badge-running' : ''}">${state.ws ? 'Connected' : 'Disconnected'}</span>
      </div>
    </div>

    <div class="settings-section">
      <h3>System Info</h3>
      <div class="card" style="font-family:var(--mono);font-size:12px;white-space:pre-wrap;">${
        state.status
          ? JSON.stringify(state.status, null, 2)
          : 'Loading...'
      }</div>
    </div>

    <div class="settings-section">
      <h3>Debug</h3>
      <div style="display:flex;gap:8px;">
        <button class="btn" onclick="refreshAll()">Refresh All Data</button>
        <button class="btn" onclick="navigator.clipboard.writeText(JSON.stringify(state.snapshot,null,2));alert('Copied!')">Copy Snapshot</button>
      </div>
    </div>
  `;
}

// ── Helpers ────────────────────────────────────────────────────────
function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function capitalize(s) {
  return s ? s.charAt(0).toUpperCase() + s.slice(1) : '';
}

function formatUptime(secs) {
  if (!secs) return '-';
  if (secs < 60) return `${Math.round(secs)}s`;
  if (secs < 3600) return `${Math.round(secs / 60)}m`;
  return `${Math.round(secs / 3600)}h ${Math.round((secs % 3600) / 60)}m`;
}

function formatNumber(n) {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K';
  return String(n);
}

async function refreshAll() {
  await Promise.all([fetchStatus(), fetchSnapshot(), fetchAgents()]);
  renderPage();
}

// ── Keyboard Shortcuts ────────────────────────────────────────────
document.addEventListener('keydown', (e) => {
  if (e.ctrlKey && e.key === 'k') {
    e.preventDefault();
    navigate('agents');
  }
  if (e.ctrlKey && e.key === 'n') {
    e.preventDefault();
    navigate('chat');
    setTimeout(() => document.getElementById('chat-input')?.focus(), 100);
  }
});

// ── Init ──────────────────────────────────────────────────────────
(async function init() {
  loadTheme();

  // Determine page from hash.
  const hash = window.location.hash.replace('#', '') || 'chat';
  state.page = hash;

  // Fetch initial data.
  await Promise.all([fetchStatus(), fetchAgents()]);
  addLog('info', 'dashboard', 'Dashboard loaded');

  // Navigate to correct page.
  navigate(state.page);

  // Try WebSocket connection.
  try { connectWS(); } catch (e) { console.warn('WS unavailable:', e); }

  // Periodic refresh every 30 seconds.
  setInterval(async () => {
    await fetchStatus();
    if (state.page === 'overview') renderOverview();
  }, 30000);
})();
