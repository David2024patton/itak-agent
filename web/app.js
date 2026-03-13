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
  chatPersona: null,
  personas: [],
  logs: [],
  logFilter: 'all',
  fsEvents: [],
  connected: false,
  ws: null,
  tasks: [],
  isThinking: false,
  liveEvents: [],
  canvasOpen: false,
  canvasContent: null,
  canvasTitle: 'Preview',
  canvasUrl: null,
  liveAgentsOpen: false,
  liveAgentAnimId: null,
  agentActivity: {},
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

async function fetchPersonas() {
  const data = await api('/v1/personas');
  if (data && data.personas) {
    state.personas = data.personas;
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

async function fetchTasks() {
  const data = await api('/v1/tasks');
  if (data && data.tasks) {
    state.tasks = data.tasks;
  }
  return data;
}

async function createTask(title, description) {
  const data = await api('/v1/tasks', {
    method: 'POST',
    body: JSON.stringify({ title, description }),
  });
  if (data) await fetchTasks();
  return data;
}

async function updateTaskStatus(id, newStatus, assignedAgent) {
  const task = state.tasks.find(t => t.id === id);
  if (!task) return;
  
  const data = await api(`/v1/tasks/${id}`, {
    method: 'PUT',
    body: JSON.stringify({
      title: task.title,
      description: task.description,
      status: newStatus,
      assigned_agent: assignedAgent || task.assigned_agent,
    }),
  });
  if (data) await fetchTasks();
  return data;
}

async function editTask(id, title, description, status, assignedAgent) {
  const data = await api(`/v1/tasks/${id}`, {
    method: 'PUT',
    body: JSON.stringify({
      title,
      description,
      status,
      assigned_agent: assignedAgent,
    }),
  });
  if (data) await fetchTasks();
  return data;
}

async function deleteTask(id) {
  const data = await api(`/v1/tasks/${id}`, {
    method: 'DELETE',
  });
  if (data) await fetchTasks();
  return data;
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

      const topicsOfInterest = [
        'agent.start', 'agent.tool_call', 'agent.complete', 
        'orchestrator.thinking', 'orchestrator.delegation', 'browser.navigate',
        'artifact.created'
      ];

      if (state.isThinking && topicsOfInterest.includes(evt.topic)) {
        let icon = '⚡';
        let text = evt.message || evt.topic;

        if (evt.topic === 'orchestrator.thinking') { icon = '🧠'; text = 'Orchestrator reasoning...'; }
        if (evt.topic === 'orchestrator.delegation') { icon = '✉️'; text = `Delegating task...`; if(evt.data && evt.data.agent) text = `Delegating to <b>${evt.data.agent}</b>`; }
        if (evt.topic === 'agent.start') { icon = '🚀'; text = `Starting <b>${evt.agent || 'agent'}</b>...`; }
        if (evt.topic === 'agent.tool_call') { 
          icon = '🔧'; 
          let toolName = evt.tool || evt.data?.tool || 'unknown tool';
          let toolArgs = evt.message ? `<div style="margin-top:4px; font-size:11px; color:var(--text-muted); padding:4px; background:rgba(0,0,0,0.2); border-radius:4px; white-space:pre-wrap; border-left: 2px solid var(--border);">${escapeHtml(evt.message)}</div>` : '';
          text = `<div>Using tool: <span style="color:var(--blue)">${toolName}</span></div>${toolArgs}`; 
        }
        if (evt.topic === 'agent.complete') { icon = '✅'; text = `<b>${evt.agent || 'Agent'}</b> completed task`; }
        if (evt.topic === 'browser.navigate') { icon = '🌐'; text = `Navigating browser...`; if(evt.data && evt.data.url) text = `Navigating to: <span style="color:var(--text-muted)">${evt.data.url}</span>`; }
        if (evt.topic === 'artifact.created') { 
          icon = '🖼'; 
          const artTitle = evt.data?.title || evt.message || 'Artifact';
          const artType = evt.data?.type || 'artifact';
          text = `<b>${artType}</b> created: <span style="color:var(--blue)">${escapeHtml(artTitle)}</span>`; 
        }

        state.liveEvents.push({ type: evt.topic, text, icon });
        if (state.page === 'chat') {
           const container = document.getElementById('live-events-container');
           if (container) {
             container.innerHTML = renderEventBoxes(state.liveEvents);
             const msgBox = document.getElementById('chat-messages');
             if (msgBox) msgBox.scrollTop = msgBox.scrollHeight;
           }
        } else if (state.page === 'tasks') {
           const tasksLiveContainer = document.getElementById('tasks-live-events');
           if (tasksLiveContainer) {
             tasksLiveContainer.innerHTML = renderEventBoxes(state.liveEvents);
           }
        }
      }

      // Auto-open Canvas when an artifact is created (regardless of isThinking state).
      if (evt.topic === 'artifact.created' && evt.data && evt.data.url) {
        const url = evt.data.url;
        const title = evt.data.title || evt.data.filename || 'Artifact';
        openCanvasUrl(url, title);
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
    personas: 'Agents',
    settings: 'Settings',
    tasks: 'Task Board',
    presentations: 'Presentations',
    database: 'Database',
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
    case 'personas': await renderPersonas(content); break;
    case 'settings': renderSettings(content); break;
    case 'tasks': await renderTasks(content); break;
    case 'presentations': await renderPresentations(content); break;
    case 'database': await renderDatabase(content); break;
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
    text.textContent = `${state.status.agents || 0} agent(s) available`; // Clarified from "running" to "available"
    text.style.color = 'var(--green)';
    if (state.status.version) ver.textContent = `v${state.status.version}`;
  } else {
    dot.style.background = 'var(--red)';
    text.textContent = 'Disconnected';
    text.style.color = 'var(--red)';
  }
}

// ── Sidebar Toggle ────────────────────────────────────────────────
function toggleSidebar() {
  const sidebar = document.getElementById('sidebar');
  if (sidebar) {
    sidebar.classList.toggle('collapsed');
  }
}

// ── Page: Chat ────────────────────────────────────────────────────
function renderEventBoxes(events) {
  if (!events || events.length === 0) return '';
  return `<div class="status-events" style="margin:8px 0; display:flex; flex-direction:column; gap:6px;">` +
    events.map(e => `
      <div class="status-box" style="display:flex; align-items:center; gap:8px; padding:6px 10px; background:var(--bg-card); border:1px solid var(--border); border-radius:var(--radius-sm); font-family:var(--mono); font-size:12px; color:var(--text-primary);">
        <span style="font-size:14px;">${e.icon}</span>
        <span>${e.text}</span>
      </div>
    `).join('') +
  `</div>`;
}

function renderChatMessages() {
  const msgBox = document.getElementById('chat-messages');
  if (!msgBox) return;

  const messages = state.chatMessages.map(m => `
    <div class="chat-msg ${m.role}">
      ${m.agent ? `<div class="msg-agent">${m.agent}</div>` : ''}
      ${m.events ? renderEventBoxes(m.events) : ''}
      <div class="msg-content">${formatContent(m.content)}</div>
    </div>
  `).join('');

  msgBox.innerHTML = messages + (
    state.isThinking ? `
      <div class="chat-msg assistant" id="thinking-block">
        <div class="msg-agent">agent is planning...</div>
        <div id="live-events-container">
          ${renderEventBoxes(state.liveEvents)}
        </div>
        <div class="spinner" style="margin-top:8px;"></div>
      </div>
    ` : ''
  );
  
  if (state.chatMessages.length === 0 && !state.isThinking) {
    msgBox.innerHTML = `
      <div class="empty-state">
        <div class="empty-icon">💬</div>
        <div class="empty-text">Send a message to get started</div>
      </div>
    `;
  }
  
  msgBox.scrollTop = msgBox.scrollHeight;
}

function renderChat(container) {
  if (!container) container = document.getElementById('page-content');

  // Build agent selector from focused agents.
  const agentOpts = state.agents.map(a =>
    `<option value="${a.name}" ${state.chatAgent === a.name ? 'selected' : ''}>${a.name}</option>`
  ).join('');

  // Also include personas that are focused agents (not orchestrator).
  const focusedPersonaOpts = state.personas.filter(p => !p.is_default).map(p =>
    `<option value="${p.name}" ${state.chatAgent === p.name ? 'selected' : ''}>${p.name}</option>`
  ).join('');

  const selectStyle = `
    padding:6px 10px;
    background:var(--bg-input);
    border:1px solid var(--border);
    border-radius:var(--radius-sm);
    color:var(--text-primary);
    font-family:var(--font);
    font-size:12px;
  `;

  container.innerHTML = `
    <div class="chat-split ${state.canvasOpen ? 'canvas-active' : ''}" id="chat-split">
      <div class="chat-pane">
        <div class="chat-container">
          <div style="display:flex;gap:8px;align-items:center;margin-bottom:8px;flex-wrap:wrap;">
            <span class="section-label" style="margin:0;">Agent:</span>
            <select id="chat-agent-select" onchange="state.chatAgent=this.value" style="${selectStyle}">
              <option value="">Orchestrator (auto-route)</option>
              ${agentOpts}
              ${focusedPersonaOpts}
            </select>
            <button class="btn" style="font-size:11px;padding:4px 10px;" onclick="navigate('personas')" title="Create a new focused agent">
              + New Agent
            </button>
            <button class="canvas-toggle ${state.liveAgentsOpen ? 'active' : ''}" onclick="toggleLiveAgents()" title="Toggle live agent topology view">
              📡 Live Agents
            </button>
            <button class="canvas-toggle ${state.canvasOpen ? 'active' : ''}" onclick="toggleCanvas()" title="Toggle Canvas preview">
              🖼 Canvas
            </button>
          </div>

          <div class="chat-messages" id="chat-messages">
            <!-- Rendered by renderChatMessages() -->
          </div>

          <div class="chat-input-area">
            <input type="text" id="chat-input" placeholder="Type a message... (Enter to send)" autofocus
              onkeydown="if(event.key==='Enter')handleChatSend()">
            <button class="btn btn-primary" onclick="handleChatSend()">Send</button>
          </div>
        </div>
      </div>

      <div class="canvas-pane" id="canvas-pane">
        <div class="canvas-header">
          <div class="canvas-header-left">
            <span class="canvas-header-icon">🖼</span>
            <span class="canvas-title" id="canvas-title">${escapeHtml(state.canvasTitle)}</span>
            ${state.canvasUrl ? `<span class="canvas-subtitle" title="${escapeHtml(state.canvasUrl)}">${escapeHtml(state.canvasUrl)}</span>` : ''}
          </div>
          <div class="canvas-header-actions">
            ${state.canvasUrl ? `<button class="canvas-btn" onclick="window.open('${state.canvasUrl}','_blank')">↗ Open</button>` : ''}
            <button class="canvas-btn" onclick="refreshCanvas()">↻ Refresh</button>
            <button class="canvas-close" onclick="closeCanvas()" title="Close canvas">×</button>
          </div>
        </div>
        <div class="canvas-body" id="canvas-body">
          ${state.canvasContent || state.canvasUrl ? 
            `<iframe id="canvas-iframe" ${state.canvasUrl ? `src="${state.canvasUrl}"` : ''} sandbox="allow-scripts allow-same-origin allow-popups"></iframe>` :
            `<div class="canvas-empty">
              <div class="canvas-empty-icon">🖼</div>
              <div class="canvas-empty-text">No preview yet</div>
              <div style="font-size:11px;color:var(--text-muted);max-width:200px;text-align:center;line-height:1.4;">Send a message that generates HTML, slides, or code and click "Preview in Canvas"</div>
            </div>`
          }
        </div>
        <div class="canvas-status">
          <span><span class="canvas-status-dot"></span>${state.canvasContent || state.canvasUrl ? 'Ready' : 'Idle'}</span>
          <span>${state.canvasUrl ? escapeHtml(state.canvasUrl) : state.canvasContent ? 'srcdoc' : 'no source'}</span>
        </div>
      </div>

      <div class="live-agents-pane ${state.liveAgentsOpen ? 'visible' : ''}" id="live-agents-pane">
        <div class="canvas-header">
          <div class="canvas-header-left">
            <span class="canvas-header-icon">📡</span>
            <span class="canvas-title">Live Agent Topology</span>
          </div>
          <div class="canvas-header-actions">
            <button class="canvas-btn" onclick="simulateAgentActivity()" title="Simulate delegation">⚡ Simulate</button>
            <button class="canvas-close" onclick="closeLiveAgents()" title="Close">×</button>
          </div>
        </div>
        <div class="canvas-body" style="position:relative;overflow:hidden;">
          <canvas id="agent-topology-canvas" style="width:100%;height:100%;display:block;"></canvas>
        </div>
      </div>
    </div>
  `;

  renderChatMessages();
  if (state.liveAgentsOpen) initAgentTopology();

  // If canvas has srcdoc content (not a URL), inject it after render.
  if (state.canvasOpen && state.canvasContent && !state.canvasUrl) {
    const iframe = document.getElementById('canvas-iframe');
    if (iframe) iframe.srcdoc = state.canvasContent;
  }
}

async function handleChatSend() {
  const input = document.getElementById('chat-input');
  if (!input) return;
  const msg = input.value.trim();
  if (!msg) return;
  input.value = '';

  const agent = state.chatAgent || '';
  state.chatMessages.push({ role: 'user', content: msg, agent: agent || null });
  
  state.isThinking = true;
  state.liveEvents = [];
  renderChatMessages();

  const result = await sendChat(msg, agent);
  
  state.isThinking = false;

  if (result) {
    state.chatMessages.push({
      role: 'assistant',
      content: result.response || result.error || 'No response',
      agent: agent || 'orchestrator',
      events: [...state.liveEvents]
    });
  } else {
    state.chatMessages.push({
      role: 'assistant',
      content: 'Failed to reach the agent. Check if the server is running.',
      agent: 'system',
    });
  }

  state.liveEvents = [];
  renderChatMessages();
}

// ── Canvas Controls ────────────────────────────────────────────────

// Open the canvas with raw HTML content (srcdoc mode).
function openCanvas(html, title) {
  state.canvasOpen = true;
  state.canvasContent = html;
  state.canvasTitle = title || 'Preview';
  state.canvasUrl = null;
  if (state.page === 'chat') renderChat();
}

// Open the canvas with a URL (src mode, for slides/reports).
function openCanvasUrl(url, title) {
  state.canvasOpen = true;
  state.canvasContent = null;
  state.canvasTitle = title || url.split('/').pop();
  state.canvasUrl = url;
  if (state.page === 'chat') renderChat();
}

// Close the canvas.
function closeCanvas() {
  state.canvasOpen = false;
  if (state.page === 'chat') renderChat();
}

// Toggle canvas visibility.
function toggleCanvas() {
  state.canvasOpen = !state.canvasOpen;
  if (state.page === 'chat') renderChat();
}

// Refresh the canvas iframe.
function refreshCanvas() {
  const iframe = document.getElementById('canvas-iframe');
  if (!iframe) return;
  if (state.canvasUrl) {
    iframe.src = state.canvasUrl;
  } else if (state.canvasContent) {
    iframe.srcdoc = state.canvasContent;
  }
}

// ── Live Agent Topology ──────────────────────────────────────────

function toggleLiveAgents() {
  state.liveAgentsOpen = !state.liveAgentsOpen;
  if (!state.liveAgentsOpen && state.liveAgentAnimId) {
    cancelAnimationFrame(state.liveAgentAnimId);
    state.liveAgentAnimId = null;
  }
  if (state.page === 'chat') renderChat();
}

function closeLiveAgents() {
  state.liveAgentsOpen = false;
  if (state.liveAgentAnimId) {
    cancelAnimationFrame(state.liveAgentAnimId);
    state.liveAgentAnimId = null;
  }
  if (state.page === 'chat') renderChat();
}

// Simulate a delegation event for demo purposes.
function simulateAgentActivity() {
  const focused = state.personas.filter(p => !p.is_default && !p.is_locked);
  if (focused.length === 0) return;
  const target = focused[Math.floor(Math.random() * focused.length)];
  const acts = state.agentActivity;

  // Create a delegation event
  acts[target.name] = {
    status: 'working',
    startedAt: Date.now(),
    task: 'Processing delegated task...',
    packets: [],
  };

  // After 3-6 seconds, mark as complete
  const duration = 3000 + Math.random() * 3000;
  setTimeout(() => {
    if (acts[target.name]) {
      acts[target.name].status = 'complete';
      acts[target.name].completedAt = Date.now();
      // Clear after 2s
      setTimeout(() => {
        if (acts[target.name] && acts[target.name].status === 'complete') {
          acts[target.name].status = 'idle';
        }
      }, 2000);
    }
  }, duration);
}

// Initialize and run the topology canvas renderer.
function initAgentTopology() {
  const canvas = document.getElementById('agent-topology-canvas');
  if (!canvas) return;

  const parent = canvas.parentElement;
  const dpr = window.devicePixelRatio || 1;

  // Cancel any previous loop
  if (state.liveAgentAnimId) {
    cancelAnimationFrame(state.liveAgentAnimId);
  }

  // Color palette
  const colors = {
    bg: '#0d1117',
    gridLine: 'rgba(48, 54, 61, 0.3)',
    orchRing: '#58a6ff',
    orchGlow: 'rgba(88, 166, 255, 0.25)',
    orchFill: '#161b22',
    agentRing: '#3fb950',
    agentGlow: 'rgba(63, 185, 80, 0.2)',
    agentFill: '#161b22',
    workingRing: '#d29922',
    workingGlow: 'rgba(210, 153, 34, 0.3)',
    completeRing: '#3fb950',
    errorRing: '#f85149',
    line: 'rgba(88, 166, 255, 0.15)',
    lineActive: 'rgba(88, 166, 255, 0.5)',
    packetColor: '#58a6ff',
    packetGlow: 'rgba(88, 166, 255, 0.6)',
    text: '#c9d1d9',
    textMuted: '#8b949e',
    textBright: '#f0f6fc',
    statusDot: '#3fb950',
  };

  function resize() {
    const w = parent.clientWidth;
    const h = parent.clientHeight;
    canvas.width = w * dpr;
    canvas.height = h * dpr;
    canvas.style.width = w + 'px';
    canvas.style.height = h + 'px';
  }
  resize();

  const ctx = canvas.getContext('2d');

  // Build agent node list
  function getNodes() {
    const orch = state.personas.find(p => p.is_default || p.is_locked);
    const focused = state.personas.filter(p => !p.is_default && !p.is_locked);
    return { orch, focused };
  }

  // Layout: orchestrator top-center, focused agents in an arc below
  function layoutNodes(w, h, orch, focused) {
    const nodes = [];
    const orchR = Math.min(w, h) * 0.06;
    const agentR = orchR * 0.65;
    const orchX = w / 2;
    const orchY = h * 0.2;

    if (orch) {
      nodes.push({
        x: orchX, y: orchY, r: orchR,
        name: orch.name || 'Orchestrator',
        role: 'Orchestrator',
        type: 'orchestrator',
        status: 'active',
      });
    }

    const count = focused.length;
    if (count === 0) return nodes;

    const arcRadius = Math.min(w * 0.35, h * 0.32);
    const arcCenter = { x: w / 2, y: orchY + arcRadius * 0.6 };
    const startAngle = Math.PI * 0.15;
    const endAngle = Math.PI * 0.85;
    const step = count > 1 ? (endAngle - startAngle) / (count - 1) : 0;

    focused.forEach((a, i) => {
      const angle = count > 1 ? startAngle + step * i : (startAngle + endAngle) / 2;
      const x = arcCenter.x + Math.cos(angle) * arcRadius;
      const y = arcCenter.y + Math.sin(angle) * arcRadius;
      const act = state.agentActivity[a.name] || {};

      nodes.push({
        x, y, r: agentR,
        name: a.name,
        role: a.role || 'Agent',
        type: 'focused',
        status: act.status || 'idle',
        task: act.task || '',
      });
    });

    return nodes;
  }

  // Animated packets: small glowing dots traveling along connection lines
  let packets = [];
  let lastPacketSpawn = 0;

  function spawnPacket(fromX, fromY, toX, toY, returning) {
    packets.push({
      sx: fromX, sy: fromY,
      tx: toX, ty: toY,
      progress: 0,
      speed: 0.008 + Math.random() * 0.004,
      returning: returning || false,
      size: 3 + Math.random() * 2,
    });
  }

  function updatePackets() {
    packets = packets.filter(p => {
      p.progress += p.speed;
      return p.progress < 1;
    });
  }

  // Draw grid background
  function drawGrid(w, h) {
    ctx.strokeStyle = colors.gridLine;
    ctx.lineWidth = 1;
    const gs = 30;
    for (let x = 0; x < w; x += gs) {
      ctx.beginPath();
      ctx.moveTo(x, 0);
      ctx.lineTo(x, h);
      ctx.stroke();
    }
    for (let y = 0; y < h; y += gs) {
      ctx.beginPath();
      ctx.moveTo(0, y);
      ctx.lineTo(w, y);
      ctx.stroke();
    }
  }

  // Draw a connection line between orchestrator and a focused agent
  function drawConnection(ox, oy, ax, ay, isActive, t) {
    ctx.beginPath();
    ctx.strokeStyle = isActive ? colors.lineActive : colors.line;
    ctx.lineWidth = isActive ? 2 : 1;
    ctx.moveTo(ox, oy);

    // Bezier curve for a smooth connection
    const midY = (oy + ay) / 2;
    ctx.quadraticCurveTo(ox, midY, ax, ay);
    ctx.stroke();

    // Active pulse on the line
    if (isActive) {
      ctx.shadowColor = colors.packetGlow;
      ctx.shadowBlur = 6;
      ctx.stroke();
      ctx.shadowBlur = 0;
    }
  }

  // Draw a node (circle with glow, icon, label)
  function drawNode(node, t) {
    const { x, y, r, name, role, type, status } = node;

    // Outer glow ring
    const pulseScale = 1 + Math.sin(t * 2) * 0.05;
    let ringColor = type === 'orchestrator' ? colors.orchRing : colors.agentRing;
    let glowColor = type === 'orchestrator' ? colors.orchGlow : colors.agentGlow;

    if (status === 'working') { ringColor = colors.workingRing; glowColor = colors.workingGlow; }
    if (status === 'complete') { ringColor = colors.completeRing; }
    if (status === 'error') { ringColor = colors.errorRing; }

    // Glow
    ctx.beginPath();
    ctx.arc(x, y, r * 1.4 * pulseScale, 0, Math.PI * 2);
    ctx.fillStyle = glowColor;
    ctx.fill();

    // Ring
    ctx.beginPath();
    ctx.arc(x, y, r, 0, Math.PI * 2);
    ctx.strokeStyle = ringColor;
    ctx.lineWidth = type === 'orchestrator' ? 3 : 2;
    ctx.stroke();

    // Fill
    ctx.beginPath();
    ctx.arc(x, y, r - 2, 0, Math.PI * 2);
    ctx.fillStyle = type === 'orchestrator' ? colors.orchFill : colors.agentFill;
    ctx.fill();

    // Icon
    ctx.font = `${r * 0.8}px sans-serif`;
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillStyle = colors.textBright;
    if (type === 'orchestrator') {
      ctx.fillText('\ud83e\udde0', x, y);
    } else {
      // Status-based icon
      const icons = { idle: '\u2b55', working: '\u26a1', complete: '\u2705', error: '\u274c' };
      ctx.fillText(icons[status] || '\u2b55', x, y);
    }

    // Label (name)
    ctx.font = `bold ${Math.max(11, r * 0.4)}px Inter, sans-serif`;
    ctx.textAlign = 'center';
    ctx.fillStyle = colors.textBright;
    ctx.fillText(name, x, y + r + 14);

    // Role subtitle
    ctx.font = `${Math.max(9, r * 0.3)}px Inter, sans-serif`;
    ctx.fillStyle = colors.textMuted;
    ctx.fillText(role, x, y + r + 26);

    // Working indicator - spinning arc
    if (status === 'working') {
      ctx.beginPath();
      const spinAngle = t * 3;
      ctx.arc(x, y, r + 4, spinAngle, spinAngle + Math.PI * 0.7);
      ctx.strokeStyle = colors.workingRing;
      ctx.lineWidth = 2.5;
      ctx.stroke();
    }
  }

  // Draw packets
  function drawPackets() {
    packets.forEach(p => {
      const x = p.sx + (p.tx - p.sx) * p.progress;
      // Bezier Y
      const midY = (p.sy + p.ty) / 2;
      const t2 = p.progress;
      const bY = (1-t2)*(1-t2)*p.sy + 2*(1-t2)*t2*midY + t2*t2*p.ty;
      const bX = (1-t2)*(1-t2)*p.sx + 2*(1-t2)*t2*p.sx + t2*t2*p.tx;

      ctx.beginPath();
      ctx.arc(bX, bY, p.size, 0, Math.PI * 2);
      ctx.fillStyle = p.returning ? colors.completeRing : colors.packetColor;
      ctx.fill();

      // Glow trail
      ctx.beginPath();
      ctx.arc(bX, bY, p.size * 2.5, 0, Math.PI * 2);
      ctx.fillStyle = p.returning ? 'rgba(63,185,80,0.2)' : colors.packetGlow;
      ctx.fill();
    });
  }

  // Title bar text
  function drawTitle(w) {
    ctx.font = 'bold 13px Inter, sans-serif';
    ctx.textAlign = 'left';
    ctx.fillStyle = colors.textMuted;
    ctx.fillText('LIVE AGENT TOPOLOGY', 16, 24);

    // Active count
    const activeCount = Object.values(state.agentActivity).filter(a => a.status === 'working').length;
    if (activeCount > 0) {
      ctx.fillStyle = colors.workingRing;
      ctx.fillText(`${activeCount} agent${activeCount > 1 ? 's' : ''} working`, 16, 40);
    }

    // Connection status dot
    ctx.beginPath();
    ctx.arc(w - 20, 20, 4, 0, Math.PI * 2);
    ctx.fillStyle = state.connected ? colors.statusDot : colors.errorRing;
    ctx.fill();
    ctx.font = '10px Inter, sans-serif';
    ctx.textAlign = 'right';
    ctx.fillStyle = colors.textMuted;
    ctx.fillText(state.connected ? 'Connected' : 'Offline', w - 28, 24);
  }

  // Main animation loop
  function animate() {
    if (!state.liveAgentsOpen) return;

    resize();
    const w = canvas.width / dpr;
    const h = canvas.height / dpr;
    const t = Date.now() / 1000;

    ctx.save();
    ctx.scale(dpr, dpr);

    // Clear + background
    ctx.fillStyle = colors.bg;
    ctx.fillRect(0, 0, w, h);

    // Subtle grid
    drawGrid(w, h);

    // Get layout
    const { orch, focused } = getNodes();
    const nodes = layoutNodes(w, h, orch, focused);
    const orchNode = nodes.find(n => n.type === 'orchestrator');

    // Draw connection lines from orchestrator to each agent
    if (orchNode) {
      nodes.filter(n => n.type === 'focused').forEach(n => {
        const isActive = n.status === 'working';
        drawConnection(orchNode.x, orchNode.y, n.x, n.y, isActive, t);

        // Spawn packets for active connections
        if (isActive && Math.random() < 0.03) {
          spawnPacket(orchNode.x, orchNode.y, n.x, n.y, false);
        }
        // Return packets for completed
        if (n.status === 'complete' && Math.random() < 0.05) {
          spawnPacket(n.x, n.y, orchNode.x, orchNode.y, true);
        }
      });
    }

    // Update and draw packets
    updatePackets();
    drawPackets();

    // Draw nodes on top
    nodes.forEach(n => drawNode(n, t));

    // Title overlay
    drawTitle(w);

    ctx.restore();

    state.liveAgentAnimId = requestAnimationFrame(animate);
  }

  animate();
}

// Open canvas from a code block index (used by inline Preview buttons).
// The code blocks are stored temporarily on the window during formatContent.
function openCanvasFromCodeBlock(index) {
  if (window.__canvasCodeBlocks && window.__canvasCodeBlocks[index]) {
    const block = window.__canvasCodeBlocks[index];
    openCanvas(block.code, block.label + ' preview');
  }
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

    <div class="settings-section" id="embed-settings">
      <h3>Embeddings</h3>
      <div class="setting-row">
        <div>
          <div class="setting-label">Active Provider</div>
          <div class="setting-desc" id="embed-status-text">Loading...</div>
        </div>
        <span class="badge" id="embed-status-badge">...</span>
      </div>

      <div class="setting-row" style="flex-direction:column;align-items:stretch;gap:12px;">
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;">
          <div>
            <label style="font-size:12px;color:var(--text-secondary);margin-bottom:4px;display:block;">Provider</label>
            <select id="embed-provider" class="form-control" onchange="embedProviderChanged()">
              <option value="local">Local (Torch/Ollama)</option>
              <option value="gemini">Gemini (Cloud)</option>
              <option value="openai">OpenAI-Compatible (Cloud)</option>
            </select>
          </div>
          <div>
            <label style="font-size:12px;color:var(--text-secondary);margin-bottom:4px;display:block;">Model</label>
            <select id="embed-model" class="form-control">
              <option value="">Loading models...</option>
            </select>
          </div>
        </div>

        <div id="embed-cloud-fields" style="display:none;">
          <div style="display:grid;grid-template-columns:2fr 1fr;gap:12px;">
            <div>
              <label style="font-size:12px;color:var(--text-secondary);margin-bottom:4px;display:block;">API Key</label>
              <input type="password" id="embed-api-key" class="form-control" placeholder="sk-... or AIza...">
            </div>
            <div>
              <label style="font-size:12px;color:var(--text-secondary);margin-bottom:4px;display:block;">API Base URL</label>
              <input type="text" id="embed-api-base" class="form-control" placeholder="https://api.openai.com">
            </div>
          </div>
        </div>

        <div style="display:flex;gap:8px;flex-wrap:wrap;">
          <button class="btn btn-primary" onclick="applyEmbedConfig()">Apply Config</button>
          <button class="btn" onclick="downloadEmbedModel()">Download Model</button>
          <button class="btn" onclick="testEmbedding()">Test Embedding</button>
        </div>

        <div id="embed-progress" style="display:none;">
          <div style="background:var(--bg-tertiary);border-radius:var(--radius-sm);overflow:hidden;height:8px;">
            <div id="embed-progress-bar" style="height:100%;background:var(--blue);transition:width 0.3s;width:0%;"></div>
          </div>
          <div id="embed-progress-text" style="font-size:11px;color:var(--text-secondary);margin-top:4px;">Downloading...</div>
        </div>

        <div id="embed-test-result" style="display:none;"></div>
      </div>
    </div>

    <div class="settings-section">
      <h3>Debug</h3>
      <div style="display:flex;gap:8px;">
        <button class="btn" onclick="refreshAll()">Refresh All Data</button>
        <button class="btn" onclick="navigator.clipboard.writeText(JSON.stringify(state.snapshot,null,2));alert('Copied!')">Copy Snapshot</button>
      </div>
    </div>
  `;

  // Load embedding status and models asynchronously.
  loadEmbedSettings();
}

// ── Embedding Settings Helpers ────────────────────────────────────
async function loadEmbedSettings() {
  // Load current status.
  const status = await api('/v1/embed/status');
  if (status) {
    const statusText = document.getElementById('embed-status-text');
    const statusBadge = document.getElementById('embed-status-badge');
    if (statusText) {
      statusText.textContent = `${status.provider} (${status.dimensions}d)`;
    }
    if (statusBadge) {
      statusBadge.textContent = status.active ? 'Active' : 'Inactive';
      statusBadge.className = `badge ${status.active ? 'badge-running' : ''}`;
    }
  }

  // Load model catalog.
  const models = await api('/v1/embed/models');
  if (models && models.models) {
    const select = document.getElementById('embed-model');
    if (select) {
      select.innerHTML = models.models.map(m =>
        `<option value="${m.name}" ${m.installed ? 'data-installed="true"' : ''}>` +
        `${m.name} (${m.dimensions}d, ${m.size_mb}MB)${m.installed ? ' [installed]' : ''}` +
        `</option>`
      ).join('');
    }
  }
}

function embedProviderChanged() {
  const provider = document.getElementById('embed-provider').value;
  const cloudFields = document.getElementById('embed-cloud-fields');
  if (cloudFields) {
    cloudFields.style.display = (provider === 'gemini' || provider === 'openai') ? 'block' : 'none';
  }
}

async function applyEmbedConfig() {
  const provider = document.getElementById('embed-provider').value;
  const model = document.getElementById('embed-model').value;
  const apiKey = document.getElementById('embed-api-key')?.value || '';
  const apiBase = document.getElementById('embed-api-base')?.value || '';

  const cfg = { provider, model };
  if (apiKey) cfg.api_key = apiKey;
  if (apiBase) cfg.api_base = apiBase;

  const result = await api('/v1/embed/config', {
    method: 'POST',
    body: JSON.stringify(cfg),
  });

  if (result && result.status === 'updated') {
    alert(`Embedding config updated: ${result.provider} (${result.dimensions}d)`);
    loadEmbedSettings();
  } else {
    alert('Failed to update config: ' + (result?.error || 'unknown error'));
  }
}

async function downloadEmbedModel() {
  const model = document.getElementById('embed-model').value;
  if (!model) return alert('Select a model first');

  const progressDiv = document.getElementById('embed-progress');
  const progressBar = document.getElementById('embed-progress-bar');
  const progressText = document.getElementById('embed-progress-text');
  if (progressDiv) progressDiv.style.display = 'block';

  try {
    const response = await fetch(`${API_BASE}/v1/embed/models/pull`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ model }),
    });

    if (response.headers.get('Content-Type')?.includes('text/event-stream')) {
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop();

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            try {
              const p = JSON.parse(line.slice(6));
              if (progressBar) progressBar.style.width = p.progress + '%';
              if (progressText) {
                if (p.status === 'downloading' && p.total > 0) {
                  const mb = (p.downloaded / 1024 / 1024).toFixed(1);
                  const totalMb = (p.total / 1024 / 1024).toFixed(1);
                  progressText.textContent = `Downloading: ${mb}MB / ${totalMb}MB (${p.progress}%)`;
                } else if (p.status === 'complete') {
                  progressText.textContent = 'Download complete!';
                  setTimeout(() => loadEmbedSettings(), 500);
                } else if (p.error) {
                  progressText.textContent = `Error: ${p.error}`;
                } else {
                  progressText.textContent = p.status;
                }
              }
            } catch {}
          }
        }
      }
    } else {
      const result = await response.json();
      if (result.status === 'already_installed') {
        alert(`${model} is already installed at ${result.path}`);
      } else if (result.error) {
        alert(`Download failed: ${result.error}`);
      } else {
        alert(`Download complete: ${result.status}`);
      }
    }
  } catch (err) {
    if (progressText) progressText.textContent = `Error: ${err.message}`;
  }
}

async function testEmbedding() {
  const resultDiv = document.getElementById('embed-test-result');
  if (!resultDiv) return;
  resultDiv.style.display = 'block';
  resultDiv.innerHTML = '<div class="card" style="font-family:var(--mono);font-size:12px;">Testing...</div>';

  const result = await api('/v1/embed/test', {
    method: 'POST',
    body: JSON.stringify({ text: 'The quick brown fox jumps over the lazy dog.' }),
  });

  if (result && result.dimensions) {
    resultDiv.innerHTML = `
      <div class="card" style="font-family:var(--mono);font-size:12px;">
        <div style="color:var(--green);margin-bottom:4px;">Embedding successful</div>
        <div>Provider: ${result.provider}</div>
        <div>Dimensions: ${result.dimensions}</div>
        <div>Preview: [${result.preview?.map(v => v.toFixed(4)).join(', ')}...]</div>
        <div style="color:var(--text-tertiary);margin-top:4px;">Text: "${result.text}"</div>
      </div>`;
  } else {
    resultDiv.innerHTML = `
      <div class="card" style="font-family:var(--mono);font-size:12px;color:var(--red);">
        Test failed: ${result?.error || 'No response'}
      </div>`;
  }
}

// ── Page: Tasks (Kanban) ──────────────────────────────────────────
async function renderTasks(container) {
  if (!container) container = document.getElementById('page-content');
  if (!state.tasks || state.tasks.length === 0) {
    await fetchTasks();
  }

  const columns = [
    { id: 'Todo', title: 'Todo' },
    { id: 'In Progress', title: 'In Progress' },
    { id: 'Review', title: 'Review' },
    { id: 'Done', title: 'Done' }
  ];

  container.innerHTML = `
    <div class="kanban-toolbar">
      <button class="btn btn-primary" onclick="openTaskModal()">+ New Task</button>
      <button class="btn" onclick="fetchTasks().then(() => renderTasks())">Refresh Tasks</button>
    </div>
    ${state.isThinking ? `
    <div class="card" style="margin-bottom:16px; border-left:4px solid var(--blue);">
      <div style="display:flex; align-items:center; gap:8px;">
        <div class="spinner" style="width:16px; height:16px; border-width:2px;"></div>
        <h4 style="margin:0; color:var(--text-primary);">Live Agent Activity</h4>
      </div>
      <div id="tasks-live-events" style="margin-top:8px;">
        ${renderEventBoxes(state.liveEvents)}
      </div>
    </div>
    ` : ''}
    <div class="kanban-board">
      ${columns.map(col => `
        <div class="kanban-column" data-status="${col.id}" ondragover="allowDrop(event)" ondrop="drop(event)" ondragleave="dragLeave(event)">
          <div class="kanban-column-header">
            <h3>${col.title}</h3>
            <span class="kanban-badge">${state.tasks.filter(t => t.status === col.id).length}</span>
          </div>
          <div class="kanban-column-body">
            ${state.tasks.filter(t => t.status === col.id).sort((a,b) => new Date(b.created_at) - new Date(a.created_at)).map(t => `
              <div class="kanban-card" draggable="true" ondragstart="dragStart(event, '${t.id}')" onclick="openEditTaskModal('${t.id}')">
                <div class="task-title">${escapeHtml(t.title)}</div>
                ${t.description ? `<div class="task-desc">${escapeHtml(t.description).substring(0, 60)}${t.description.length > 60 ? '...' : ''}</div>` : ''}
                <div class="task-footer">
                  ${t.assigned_agent ? `<span class="task-agent">🤖 ${escapeHtml(t.assigned_agent)}</span>` : '<span class="task-agent unassigned">Unassigned</span>'}
                  <span class="task-date">${new Date(t.updated_at || t.created_at).toLocaleDateString()}</span>
                </div>
              </div>
            `).join('')}
          </div>
        </div>
      `).join('')}
    </div>
  `;
}

function allowDrop(ev) {
  ev.preventDefault();
  const col = ev.currentTarget.closest('.kanban-column');
  if (col && !col.classList.contains('drag-over')) {
    col.classList.add('drag-over');
  }
}

function dragLeave(ev) {
  const col = ev.currentTarget.closest('.kanban-column');
  if (col) {
    col.classList.remove('drag-over');
  }
}

function dragStart(ev, id) {
  ev.dataTransfer.setData("text/plain", id);
}

async function drop(ev) {
  ev.preventDefault();
  
  // Remove drag-over styling from all columns
  document.querySelectorAll('.kanban-column').forEach(col => col.classList.remove('drag-over'));

  const id = ev.dataTransfer.getData("text/plain");
  const col = ev.currentTarget.closest('.kanban-column');
  if (!col || !id) return;
  
  const newStatus = col.dataset.status;
  const task = state.tasks.find(t => t.id === id);
  
  if (task && task.status !== newStatus) {
    // Optimistic update
    task.status = newStatus;
    renderTasks();
    
    // Sync to backend
    await updateTaskStatus(id, newStatus, task.assigned_agent);
    renderTasks();
  }
}

// ── Modals / Forms ────────────────────────────────────────────────
function openTaskModal() {
  document.getElementById('task-modal-title').textContent = 'New Task';
  document.getElementById('task-id').value = '';
  document.getElementById('task-title-input').value = '';
  document.getElementById('task-desc-input').value = '';
  document.getElementById('task-status-group').style.display = 'none';
  document.getElementById('task-agent-group').style.display = 'none';
  
  // Hide delete button for new tasks
  const deleteBtn = document.getElementById('task-delete-btn');
  if (deleteBtn) deleteBtn.remove();
  
  document.getElementById('task-modal').style.display = 'flex';
}

function openEditTaskModal(id) {
  const task = state.tasks.find(t => t.id === id);
  if (!task) return;
  
  document.getElementById('task-modal-title').textContent = 'Edit Task';
  document.getElementById('task-id').value = task.id;
  document.getElementById('task-title-input').value = task.title;
  document.getElementById('task-desc-input').value = task.description || '';
  
  document.getElementById('task-status-input').value = task.status;
  document.getElementById('task-status-group').style.display = 'block';
  
  document.getElementById('task-agent-input').value = task.assigned_agent || '';
  document.getElementById('task-agent-group').style.display = 'block';
  
  // Ensure delete button exists
  const footer = document.querySelector('.modal-footer');
  let deleteBtn = document.getElementById('task-delete-btn');
  if (!deleteBtn) {
    deleteBtn = document.createElement('button');
    deleteBtn.id = 'task-delete-btn';
    deleteBtn.className = 'btn';
    deleteBtn.style.marginRight = 'auto'; // push to the left
    deleteBtn.style.color = 'var(--red)';
    deleteBtn.style.borderColor = 'var(--red)';
    deleteBtn.textContent = 'Delete Task';
    footer.insertBefore(deleteBtn, footer.firstChild);
  }
  deleteBtn.onclick = async () => {
    if (confirm('Are you sure you want to delete this task?')) {
      await deleteTask(id);
      closeTaskModal();
      renderTasks();
    }
  };
  
  document.getElementById('task-modal').style.display = 'flex';
}

function closeTaskModal() {
  document.getElementById('task-modal').style.display = 'none';
}

async function saveTask() {
  const id = document.getElementById('task-id').value;
  const title = document.getElementById('task-title-input').value;
  const desc = document.getElementById('task-desc-input').value;
  
  if (!title) {
    alert('Title is required');
    return;
  }
  
  if (id) {
    // Edit existing
    const status = document.getElementById('task-status-input').value;
    const agent = document.getElementById('task-agent-input').value;
    await editTask(id, title, desc, status, agent);
  } else {
    // Create new
    await createTask(title, desc);
  }
  
  closeTaskModal();
  renderTasks();
}

// ── Page: Presentations ───────────────────────────────────────────
async function renderPresentations(container) {
  if (!container) container = document.getElementById('page-content');
  
  // In a real implementation this would fetch from an API
  // For now we'll just show the directory structure of generated assets
  
  container.innerHTML = `
    <div class="section-label">Generated Presentations</div>
    <div class="card" style="margin-bottom: 24px;">
      <p style="color: var(--text-muted); font-size: 14px; margin-bottom: 16px;">
        Presentations are generated by the <code>architect</code> or <code>operator</code> agents using the <code>slide_generate</code> and <code>report_generate</code> tools.
      </p>
      
      <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 16px;">
        <div style="border: 1px solid var(--border); border-radius: 8px; overflow: hidden;">
          <div style="background: var(--bg-card); padding: 12px 16px; border-bottom: 1px solid var(--border); font-weight: 500;">
            📊 Web Reports
          </div>
          <div style="padding: 16px; background: var(--bg-input);">
            <p style="color: var(--text-muted); font-size: 13px; margin-bottom: 16px;">
              Rich JSON-backed reports with Chart.js diagrams.
            </p>
            <a href="/reports/" target="_blank" class="btn btn-primary" style="text-decoration: none; display: inline-block;">Open Reports Directory ↗</a>
          </div>
        </div>
        
        <div style="border: 1px solid var(--border); border-radius: 8px; overflow: hidden;">
          <div style="background: var(--bg-card); padding: 12px 16px; border-bottom: 1px solid var(--border); font-weight: 500;">
            📽️ Slide Decks
          </div>
          <div style="padding: 16px; background: var(--bg-input);">
            <p style="color: var(--text-muted); font-size: 13px; margin-bottom: 16px;">
              Interactive HTML presentations powered by Reveal.js.
            </p>
            <a href="/slides/" target="_blank" class="btn btn-primary" style="text-decoration: none; display: inline-block;">Open Slides Directory ↗</a>
          </div>
        </div>
      </div>
    </div>
    
    <div class="section-label">Quick Actions</div>
    <div class="card-grid">
      <div class="agent-card" onclick="startChatWith(''); setTimeout(() => {document.getElementById('chat-input').value='/deep-research Artificial Intelligence Trends 2026'; document.getElementById('chat-input').focus();}, 100)">
        <div class="agent-card-head">
          <span class="agent-card-name">Deep Research</span>
          <span class="badge badge-research">swarm</span>
        </div>
        <div class="agent-card-desc">Start a multi-agent deep research swarm on a topic. Will automatically generate a presentation.</div>
      </div>
      
      <div class="agent-card" onclick="startChatWith('architect'); setTimeout(() => {document.getElementById('chat-input').value='Generate a presentation about the history of computing.'; document.getElementById('chat-input').focus();}, 100)">
        <div class="agent-card-head">
          <span class="agent-card-name">Draft Presentation</span>
          <span class="badge badge-dev">architect</span>
        </div>
        <div class="agent-card-desc">Ask the architect agent to generate a slide deck.</div>
      </div>
    </div>
  `;
}

// ── Helpers ────────────────────────────────────────────────────────
function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

// ── Structured Content Formatter ──────────────────────────────────
// Converts agent response text into rich HTML with code blocks,
// markdown formatting, and clickable artifact links.
function formatContent(text) {
  if (!text) return '';

  // Step 1: Extract code fences so they don't get mangled by markdown processing.
  const codeBlocks = [];
  // Also store raw (unescaped) HTML for Canvas preview.
  if (!window.__canvasCodeBlocks) window.__canvasCodeBlocks = [];
  text = text.replace(/```(\w*)\n([\s\S]*?)```/g, (match, lang, code) => {
    const id = `__CODE_BLOCK_${codeBlocks.length}__`;
    const label = lang || 'code';
    const rawCode = code.trim();
    codeBlocks.push({ id, label, code: escapeHtml(rawCode), raw: rawCode });
    // Store raw code for Canvas preview (only for HTML-like blocks).
    if (['html', 'htm', 'svg', 'xml'].includes(label.toLowerCase())) {
      window.__canvasCodeBlocks.push({ label, code: rawCode });
    }
    return id;
  });

  // Step 2: Detect slide/report file paths and make them clickable + preview.
  text = text.replace(/(Successfully generated (?:slide deck|report):\s*)(\S+\.html)/gi, (match, prefix, path) => {
    const filename = path.split(/[\\/]/).pop();
    let href = `/slides/${filename}`;
    if (path.includes('report')) href = `/reports/${filename}`;
    return `${escapeHtml(prefix)}<a href="${href}" target="_blank" style="color:var(--blue); text-decoration:underline;">${escapeHtml(filename)}</a> <button class="canvas-preview-btn" onclick="openCanvasUrl('${href}','${escapeHtml(filename)}')">\uD83D\uDDBC Preview in Canvas</button>`;
  });

  // Step 3: Escape remaining HTML.
  // But skip lines that are already processed (contain __CODE_BLOCK__).
  const lines = text.split('\n');
  const processed = lines.map(line => {
    if (line.includes('__CODE_BLOCK_')) return line;
    if (line.match(/<a href=/)) return line; // Skip already-linked lines.
    return escapeHtml(line);
  });
  text = processed.join('\n');

  // Step 4: Basic markdown to HTML.
  // Headings (### heading, ## heading, # heading)
  text = text.replace(/^### (.+)$/gm, '<h4 style="margin:8px 0 4px; font-size:14px; color:var(--text-primary);">$1</h4>');
  text = text.replace(/^## (.+)$/gm, '<h3 style="margin:10px 0 4px; font-size:15px; color:var(--text-primary);">$1</h3>');
  text = text.replace(/^# (.+)$/gm, '<h2 style="margin:12px 0 6px; font-size:17px; color:var(--text-primary);">$1</h2>');

  // Bold (**text**) and italic (*text*)
  text = text.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
  text = text.replace(/\*(.+?)\*/g, '<em>$1</em>');

  // Inline code (`code`)
  text = text.replace(/`([^`]+?)`/g, '<code style="background:rgba(0,0,0,0.3); padding:1px 5px; border-radius:3px; font-family:var(--mono); font-size:12px;">$1</code>');

  // Links [text](url)
  text = text.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" style="color:var(--blue); text-decoration:underline;">$1</a>');

  // Unordered lists (- item or * item)
  text = text.replace(/^[\-\*] (.+)$/gm, '<li style="margin-left:16px; list-style:disc;">$1</li>');
  // Wrap consecutive <li> into <ul>
  text = text.replace(/(<li[^>]*>.*?<\/li>\n?)+/g, (match) => {
    return `<ul style="margin:6px 0; padding-left:8px;">${match}</ul>`;
  });

  // Numbered lists (1. item)
  text = text.replace(/^(\d+)\. (.+)$/gm, '<li style="margin-left:16px; list-style:decimal;">$2</li>');

  // Horizontal rule (---)
  text = text.replace(/^---$/gm, '<hr style="border:none; border-top:1px solid var(--border); margin:12px 0;">');

  // Citations [Cite: url]
  text = text.replace(/\[Cite:\s*(https?:\/\/[^\]]+)\]/gi, (match, url) => {
    const short = url.replace(/https?:\/\/(www\.)?/, '').split('/')[0];
    return `<a href="${url}" target="_blank" style="font-size:11px; color:var(--text-muted); text-decoration:none; border-bottom:1px dotted var(--text-muted);" title="${escapeHtml(url)}">📎 ${escapeHtml(short)}</a>`;
  });

  // Newlines to <br> (but not inside block elements).
  text = text.replace(/\n/g, '<br>');
  // Clean up excessive <br> after block elements.
  text = text.replace(/(<\/h[2-4]>)<br>/g, '$1');
  text = text.replace(/(<\/ul>)<br>/g, '$1');
  text = text.replace(/(<\/li>)<br>/g, '$1');
  text = text.replace(/(<hr[^>]*>)<br>/g, '$1');

  // Step 5: Re-inject code blocks as styled <pre> elements.
  for (let i = 0; i < codeBlocks.length; i++) {
    const block = codeBlocks[i];
    const isHtml = ['html', 'htm', 'svg', 'xml'].includes(block.label.toLowerCase());
    const canvasIdx = isHtml ? window.__canvasCodeBlocks.length - codeBlocks.filter((b, j) => j <= i && ['html','htm','svg','xml'].includes(b.label.toLowerCase())).length : -1;
    const previewBtn = isHtml ? `<button class="canvas-preview-btn" onclick="openCanvasFromCodeBlock(${window.__canvasCodeBlocks.length - 1})" style="margin-left:auto;">\uD83D\uDDBC Preview</button>` : '';
    const rendered = `
      <div style="margin:8px 0; border:1px solid var(--border); border-radius:var(--radius-sm); overflow:hidden;">
        <div style="display:flex; justify-content:space-between; align-items:center; padding:4px 10px; background:rgba(0,0,0,0.3); font-size:11px; color:var(--text-muted); font-family:var(--mono); gap:6px;">
          <span>${escapeHtml(block.label)}</span>
          <div style="display:flex; gap:4px; align-items:center;">
            ${previewBtn}
            <button onclick="navigator.clipboard.writeText(this.closest('[style]').parentElement.querySelector('pre').textContent).then(()=>{this.textContent='Copied!';setTimeout(()=>this.textContent='Copy',1500)})" style="background:none; border:1px solid var(--border); color:var(--text-muted); padding:2px 8px; border-radius:3px; cursor:pointer; font-size:10px;">Copy</button>
          </div>
        </div>
        <pre style="margin:0; padding:10px; background:var(--bg-card); overflow-x:auto; font-size:12px; line-height:1.5; font-family:var(--mono);">${block.code}</pre>
      </div>`;
    text = text.replace(block.id, rendered);
  }

  return text;
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

// ── Page: Database ────────────────────────────────────────────────
state.dbSubTab = 'graph';

async function renderDatabase(container) {
  if (!container) container = document.getElementById('page-content');

  const tabs = [
    { id: 'graph', label: 'Graph', icon: '&#11052;' },
    { id: 'vector', label: 'Vector', icon: '&#10070;' },
    { id: 'tables', label: 'Tables', icon: '&#9638;' },
    { id: 'search', label: 'Search', icon: '&#128269;' },
    { id: 'errors', label: 'Errors', icon: '&#9888;' },
    { id: 'research', label: 'Research', icon: '&#127760;' },
  ];

  const tabBar = tabs.map(t => `
    <button class="log-filter ${state.dbSubTab === t.id ? 'active' : ''}"
      onclick="state.dbSubTab='${t.id}';renderDatabase();" style="gap:4px;">
      <span>${t.icon}</span> ${t.label}
    </button>
  `).join('');

  // Fetch stats
  let stats = null;
  try {
    stats = await api('/v1/graph/stats');
  } catch(e) {}

  let tabContent = '';

  switch (state.dbSubTab) {
    case 'graph':
      tabContent = `
        <div style="display:flex;gap:12px;margin-bottom:12px;">
          <div class="stat-card" style="flex:1;">
            <div class="stat-label">Nodes</div>
            <div class="stat-value">${stats?.node_count || 0}</div>
          </div>
          <div class="stat-card" style="flex:1;">
            <div class="stat-label">Edges</div>
            <div class="stat-value">${stats?.edge_count || 0}</div>
          </div>
          <div class="stat-card" style="flex:1;">
            <div class="stat-label">Vectors</div>
            <div class="stat-value">${stats?.vector_count || 0}</div>
          </div>
          <div class="stat-card" style="flex:1;">
            <div class="stat-label">Tables</div>
            <div class="stat-value">${stats?.table_count || 0}</div>
          </div>
          <div class="stat-card" style="flex:1;">
            <div class="stat-label">FTS Docs</div>
            <div class="stat-value">${stats?.fts_doc_count || 0}</div>
          </div>
        </div>
        <div style="display:flex;gap:8px;margin-bottom:12px;align-items:center;">
          <input type="file" id="zip-upload" accept=".zip" style="display:none">
          <button class="btn btn-primary" style="font-size:12px;padding:6px 14px;" onclick="document.getElementById('zip-upload').click()">
            &#128230; Upload ZIP Template
          </button>
          <span id="upload-status" style="font-size:11px;color:var(--text-muted);"></span>
        </div>
        <div style="border:1px solid var(--border);border-radius:var(--radius);overflow:hidden;height:calc(100vh - 260px);">
          <iframe id="graph-frame" src="/graph.html" style="width:100%;height:100%;border:none;"></iframe>
        </div>
      `;
      break;

    case 'vector':
      tabContent = `
        <div class="stat-grid" style="margin-bottom:16px;">
          <div class="stat-card">
            <div class="stat-label">Vectors Stored</div>
            <div class="stat-value">${stats?.vector_count || 0}</div>
            <div class="stat-sub">HNSW index</div>
          </div>
          <div class="stat-card">
            <div class="stat-label">DB Size</div>
            <div class="stat-value">${stats?.file_size_bytes ? formatSize(stats.file_size_bytes) : '-'}</div>
            <div class="stat-sub">On disk</div>
          </div>
        </div>
        <div class="card" style="padding:24px;text-align:center;">
          <div style="font-size:32px;margin-bottom:8px;">&#10070;</div>
          <div style="font-size:14px;color:var(--text-primary);font-weight:600;">Vector Search</div>
          <div style="font-size:12px;color:var(--text-muted);margin-top:4px;">Semantic similarity search via HNSW index.</div>
          <div style="font-size:11px;color:var(--text-muted);margin-top:8px;">Use the agent chat to query: "search for similar documents about X"</div>
        </div>
      `;
      break;

    case 'tables':
      tabContent = `
        <div class="stat-grid" style="margin-bottom:16px;">
          <div class="stat-card">
            <div class="stat-label">Tables</div>
            <div class="stat-value">${stats?.table_count || 0}</div>
            <div class="stat-sub">SQL-like engine</div>
          </div>
        </div>
        <div class="card" style="padding:24px;text-align:center;">
          <div style="font-size:32px;margin-bottom:8px;">&#9638;</div>
          <div style="font-size:14px;color:var(--text-primary);font-weight:600;">SQL Table Engine</div>
          <div style="font-size:12px;color:var(--text-muted);margin-top:4px;">Structured data with CREATE, INSERT, SELECT, UPDATE, DELETE, WHERE, LIKE.</div>
          <div style="font-size:11px;color:var(--text-muted);margin-top:8px;">Built on bbolt. Zero external dependencies.</div>
        </div>
      `;
      break;

    case 'search':
      tabContent = `
        <div class="stat-grid" style="margin-bottom:16px;">
          <div class="stat-card">
            <div class="stat-label">Indexed Documents</div>
            <div class="stat-value">${stats?.fts_doc_count || 0}</div>
            <div class="stat-sub">BM25 ranking</div>
          </div>
        </div>
        <div class="card" style="padding:24px;text-align:center;">
          <div style="font-size:32px;margin-bottom:8px;">&#128269;</div>
          <div style="font-size:14px;color:var(--text-primary);font-weight:600;">Full-Text Search</div>
          <div style="font-size:12px;color:var(--text-muted);margin-top:4px;">BM25-ranked keyword search with inverted index and stop word filtering.</div>
          <div style="font-size:11px;color:var(--text-muted);margin-top:8px;">Use the agent chat to search: "find documents containing X"</div>
        </div>
      `;
      break;

    case 'errors':
      tabContent = `
        <div class="card" style="margin-bottom:12px;">
          <h4 style="margin:0 0 8px 0;color:var(--text-primary);">Search Known Errors</h4>
          <div style="display:flex;gap:8px;">
            <input type="text" id="error-search-input" class="form-control" placeholder="Describe the error to search for solutions..." style="flex:1;">
            <button class="btn btn-primary" onclick="searchDebugErrors()">Search</button>
          </div>
        </div>
        <div id="error-results" style="display:flex;flex-direction:column;gap:8px;">
          <div class="card" style="text-align:center;padding:24px;">
            <div style="font-size:32px;margin-bottom:8px;">&#9888;</div>
            <div style="font-size:14px;color:var(--text-primary);font-weight:600;">Debug Memory</div>
            <div style="font-size:12px;color:var(--text-muted);margin-top:4px;">Search past errors and their solutions. Errors are stored automatically during agent operation.</div>
          </div>
        </div>
      `;
      break;

    case 'research':
      tabContent = `
        <div class="card" style="margin-bottom:12px;">
          <h4 style="margin:0 0 8px 0;color:var(--text-primary);">Search Research</h4>
          <div style="display:flex;gap:8px;">
            <input type="text" id="research-search-input" class="form-control" placeholder="Search past research by topic..." style="flex:1;">
            <button class="btn btn-primary" onclick="searchResearch()">Search</button>
          </div>
        </div>
        <div id="research-results" style="display:flex;flex-direction:column;gap:8px;">
          <div class="card" style="text-align:center;padding:24px;">
            <div style="font-size:32px;margin-bottom:8px;">&#127760;</div>
            <div style="font-size:14px;color:var(--text-primary);font-weight:600;">Web Research</div>
            <div style="font-size:12px;color:var(--text-muted);margin-top:4px;">Browse and search past website research. Data is stored per-website with domain grouping.</div>
          </div>
        </div>
      `;
      break;
  }

  container.innerHTML = `
    <div style="margin-bottom:12px;display:flex;gap:6px;align-items:center;">
      ${tabBar}
      <span style="margin-left:auto;font-size:10px;color:var(--text-muted);font-family:var(--mono);">v${stats?.version || '1.0.0'}</span>
    </div>
    ${tabContent}
  `;

  // Wire upload handler for graph tab
  if (state.dbSubTab === 'graph') {
    const upInput = document.getElementById('zip-upload');
    if (upInput) {
      upInput.addEventListener('change', async (ev) => {
        const file = ev.target.files[0];
        if (!file) return;
        const statusEl = document.getElementById('upload-status');
        if (statusEl) statusEl.textContent = `Uploading ${file.name}...`;

        const fd = new FormData();
        fd.append('file', file);

        try {
          const resp = await fetch('/v1/graph/ingest', { method: 'POST', body: fd });
          const result = await resp.json();
          if (result.status === 'ingested') {
            if (statusEl) statusEl.innerHTML = `<span style="color:var(--green)">Ingested ${result.template}: ${result.files} files, ${result.edges} edges</span>`;
            // Refresh iframe
            const frame = document.getElementById('graph-frame');
            if (frame) frame.src = frame.src;
            // Refresh stats after short delay
            setTimeout(() => renderDatabase(), 1000);
          } else {
            if (statusEl) statusEl.innerHTML = `<span style="color:var(--red)">Error: ${result.error || 'unknown'}</span>`;
          }
        } catch (err) {
          if (statusEl) statusEl.innerHTML = `<span style="color:var(--red)">Upload failed: ${err.message}</span>`;
        }
        upInput.value = '';
      });
    }
  }
}

// ── Debug/Research Search Helpers ─────────────────────────────────
async function searchDebugErrors() {
  const query = document.getElementById('error-search-input')?.value;
  if (!query) return;
  const resultsDiv = document.getElementById('error-results');
  if (!resultsDiv) return;
  resultsDiv.innerHTML = '<div class="card" style="text-align:center;padding:16px;color:var(--text-muted);">Searching...</div>';

  const data = await api('/v1/debug/search', {
    method: 'POST',
    body: JSON.stringify({ query, limit: 10 }),
  });

  if (!data || !data.results || data.results.length === 0) {
    resultsDiv.innerHTML = '<div class="card" style="text-align:center;padding:16px;color:var(--text-muted);">No matching errors found. ' + (data?.message || '') + '</div>';
    return;
  }

  resultsDiv.innerHTML = data.results.map(r => {
    const n = r.node || {};
    const isResolved = n.resolved === true;
    const fixInfo = r.fix ? `
      <div style="margin-top:8px;padding:8px;background:var(--bg-tertiary);border-radius:var(--radius-sm);border-left:3px solid var(--green);">
        <div style="font-size:11px;color:var(--green);font-weight:600;">FIX (by ${r.fix.agent || 'unknown'})</div>
        <div style="font-size:12px;color:var(--text-primary);margin-top:4px;">${r.fix.description || ''}</div>
        ${r.fix.code ? `<pre style="font-size:11px;margin-top:4px;background:var(--bg-secondary);padding:4px 8px;border-radius:4px;overflow-x:auto;">${r.fix.code}</pre>` : ''}
      </div>` : '';

    return `
      <div class="card" style="border-left:3px solid ${isResolved ? 'var(--green)' : 'var(--red)'};">
        <div style="display:flex;justify-content:space-between;align-items:center;">
          <span class="badge ${isResolved ? 'badge-running' : ''}" style="font-size:10px;">${isResolved ? 'Resolved' : 'Open'}</span>
          <span style="font-size:10px;color:var(--text-muted);">${n.error_type || 'unknown'} | ${(r.score * 100).toFixed(0)}% match</span>
        </div>
        <div style="font-size:12px;color:var(--text-primary);margin-top:6px;font-family:var(--mono);">${n.message || ''}</div>
        <div style="font-size:10px;color:var(--text-muted);margin-top:4px;">Source: ${n.source || '-'} | ${n.timestamp || ''}</div>
        ${fixInfo}
      </div>`;
  }).join('');
}

async function searchResearch() {
  const query = document.getElementById('research-search-input')?.value;
  if (!query) return;
  const resultsDiv = document.getElementById('research-results');
  if (!resultsDiv) return;
  resultsDiv.innerHTML = '<div class="card" style="text-align:center;padding:16px;color:var(--text-muted);">Searching...</div>';

  const data = await api('/v1/research/search', {
    method: 'POST',
    body: JSON.stringify({ query, limit: 10 }),
  });

  if (!data || !data.results || data.results.length === 0) {
    resultsDiv.innerHTML = '<div class="card" style="text-align:center;padding:16px;color:var(--text-muted);">No matching research found. ' + (data?.message || '') + '</div>';
    return;
  }

  resultsDiv.innerHTML = data.results.map(r => {
    const n = r.node || {};
    return `
      <div class="card">
        <div style="display:flex;justify-content:space-between;align-items:center;">
          <span style="font-size:13px;font-weight:600;color:var(--text-primary);">${n.title || 'Untitled'}</span>
          <span style="font-size:10px;color:var(--text-muted);">${(r.score * 100).toFixed(0)}% match</span>
        </div>
        <div style="display:flex;gap:8px;margin-top:4px;">
          <span class="badge" style="font-size:10px;">${n.domain || 'unknown'}</span>
          ${n.topic ? `<span class="badge" style="font-size:10px;">${n.topic}</span>` : ''}
        </div>
        <div style="font-size:12px;color:var(--text-secondary);margin-top:6px;">${n.findings || n.content?.substring(0, 200) || ''}</div>
        ${n.url ? `<a href="${n.url}" target="_blank" style="font-size:11px;color:var(--blue);margin-top:4px;display:block;text-decoration:none;">${n.url}</a>` : ''}
        <div style="font-size:10px;color:var(--text-muted);margin-top:4px;">${n.last_visited || ''}</div>
      </div>`;
  }).join('');
}

// ── Agent Management Page ─────────────────────────────────────────
async function renderPersonas(container) {
  if (!container) container = document.getElementById('page-content');
  await fetchPersonas();

  // Separate orchestrator from focused agents
  const orchestrator = state.personas.find(p => p.is_default || p.is_locked);
  const focusedAgents = state.personas.filter(p => !p.is_default && !p.is_locked);

  // ── Orchestrator card ──
  const orchCard = orchestrator ? `
    <div class="card" style="border-left:3px solid var(--blue);">
      <div style="display:flex;justify-content:space-between;align-items:center;">
        <div style="display:flex;align-items:center;gap:8px;">
          <span style="font-size:16px;font-weight:800;color:var(--text-primary);">🧠 Orchestrator</span>
          <span class="badge" style="font-size:9px;background:var(--blue);color:#fff;">CORE</span>
        </div>
        <button class="btn" style="font-size:11px;padding:4px 10px;" onclick="editOrchestrator()">Customize</button>
      </div>
      <div style="font-size:12px;color:var(--text-secondary);margin-top:8px;line-height:1.5;">${orchestrator.personality || 'No personality set'}</div>
      <div style="display:flex;gap:8px;margin-top:8px;flex-wrap:wrap;">
        <span style="font-size:10px;color:var(--text-muted);">Name: <strong style="color:var(--text-primary);">${orchestrator.name}</strong></span>
        <span style="font-size:10px;color:var(--text-muted);">Role: <strong style="color:var(--text-primary);">${orchestrator.role || 'Tech Lead'}</strong></span>
      </div>
      <div style="font-size:10px;color:var(--text-muted);margin-top:6px;padding:6px 8px;background:var(--bg-input);border-radius:var(--radius-sm);border:1px dashed var(--border);">
        🔒 Core delegation logic, system prompt, and max_delegations are locked to ensure stability.
      </div>
    </div>
  ` : `
    <div class="card" style="border-left:3px solid var(--yellow);text-align:center;padding:16px;">
      <span style="color:var(--text-muted);">Orchestrator not initialized. It will be created on first API call.</span>
    </div>
  `;

  // ── Focused agent cards ──
  const agentCards = focusedAgents.map(a => {
    const goalsHTML = (a.goals || []).map(g => `<span class="badge" style="font-size:10px;">${g}</span>`).join('');
    const toolsHTML = (a.tools || []).map(t => `<span class="badge" style="font-size:10px;background:var(--bg-input);">${t}</span>`).join('');
    return `
      <div class="card">
        <div style="display:flex;justify-content:space-between;align-items:center;">
          <div style="display:flex;align-items:center;gap:8px;">
            <span style="font-size:14px;font-weight:700;color:var(--text-primary);">${a.name}</span>
            <span class="badge" style="font-size:9px;">${a.role || 'Agent'}</span>
          </div>
          <div style="display:flex;gap:4px;">
            <button class="btn" style="font-size:11px;padding:3px 8px;" onclick="editAgent('${a.name}')">Edit</button>
            <button class="btn" style="font-size:11px;padding:3px 8px;color:var(--red);" onclick="deleteAgent('${a.name}')">Delete</button>
          </div>
        </div>
        <div style="font-size:12px;color:var(--text-secondary);margin-top:6px;line-height:1.4;">${a.personality || ''}</div>
        ${goalsHTML ? `<div style="margin-top:6px;display:flex;gap:4px;flex-wrap:wrap;"><span style="font-size:10px;color:var(--text-muted);margin-right:2px;">Goals:</span>${goalsHTML}</div>` : ''}
        ${toolsHTML ? `<div style="margin-top:4px;display:flex;gap:4px;flex-wrap:wrap;"><span style="font-size:10px;color:var(--text-muted);margin-right:2px;">Tools:</span>${toolsHTML}</div>` : ''}
        <div style="display:flex;gap:12px;margin-top:6px;">
          <span style="font-size:10px;color:var(--text-muted);">Autonomy: ${a.autonomy}/4</span>
          <span style="font-size:10px;color:var(--text-muted);">Max loops: ${a.max_loops || 10}</span>
        </div>
      </div>`;
  }).join('');

  container.innerHTML = `
    <div style="margin-bottom:16px;">
      <span class="section-label">Orchestrator</span>
      <p style="font-size:11px;color:var(--text-muted);margin:2px 0 8px 0;">The brain that routes tasks to focused agents. You can customize its personality and how it addresses you.</p>
      ${orchCard}
    </div>

    <div>
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;">
        <span class="section-label">Focused Agents (${focusedAgents.length})</span>
        <button class="btn btn-primary" style="font-size:12px;padding:6px 14px;" onclick="openAgentModal()">+ New Agent</button>
      </div>
      <p style="font-size:11px;color:var(--text-muted);margin:0 0 8px 0;">Specialized workers that receive delegated tasks. Each agent has a role, personality, tools, and autonomy level.</p>
      <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(360px,1fr));gap:10px;">
        ${agentCards || '<div style="color:var(--text-muted);text-align:center;padding:20px;">No focused agents yet. Click "+ New Agent" to create one.</div>'}
      </div>
    </div>

    <!-- Orchestrator Edit Modal -->
    <div id="orch-modal" style="display:none;position:fixed;inset:0;background:rgba(0,0,0,0.6);z-index:1000;align-items:center;justify-content:center;">
      <div style="background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);padding:20px;width:100%;max-width:460px;">
        <h3 style="margin:0 0 12px 0;font-size:16px;color:var(--text-primary);">🧠 Customize Orchestrator</h3>
        <div style="display:flex;flex-direction:column;gap:10px;">
          <div>
            <label style="font-size:11px;color:var(--text-muted);">Display Name (what the agent goes by)</label>
            <input type="text" id="orch-name" class="form-control" placeholder="e.g. Mike">
          </div>
          <div>
            <label style="font-size:11px;color:var(--text-muted);">Your Name (how the agent addresses you)</label>
            <input type="text" id="orch-user-name" class="form-control" placeholder="e.g. David">
          </div>
          <div>
            <label style="font-size:11px;color:var(--text-muted);">Personality / Attitude</label>
            <textarea id="orch-personality" class="form-control" rows="3" placeholder="Direct, professional, no fluff..."></textarea>
          </div>
          <div style="font-size:10px;color:var(--text-muted);padding:6px 8px;background:var(--bg-input);border-radius:var(--radius-sm);border:1px dashed var(--border);">
            🔒 Core delegation logic is locked and cannot be changed.
          </div>
        </div>
        <div style="display:flex;gap:8px;justify-content:flex-end;margin-top:14px;">
          <button class="btn btn-secondary" onclick="closeOrchModal()">Cancel</button>
          <button class="btn btn-primary" onclick="saveOrchestrator()">Save</button>
        </div>
      </div>
    </div>

    <!-- Agent Create/Edit Modal -->
    <div id="agent-modal" style="display:none;position:fixed;inset:0;background:rgba(0,0,0,0.6);z-index:1000;align-items:center;justify-content:center;">
      <div style="background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);padding:20px;width:100%;max-width:500px;">
        <h3 id="agent-modal-title" style="margin:0 0 12px 0;font-size:16px;color:var(--text-primary);">New Focused Agent</h3>
        <div style="display:flex;flex-direction:column;gap:10px;">
          <input type="hidden" id="agent-edit-original">
          <div>
            <label style="font-size:11px;color:var(--text-muted);">Name *</label>
            <input type="text" id="agent-name" class="form-control" placeholder="e.g. frontend_dev">
          </div>
          <div>
            <label style="font-size:11px;color:var(--text-muted);">Role *</label>
            <input type="text" id="agent-role" class="form-control" placeholder="e.g. Frontend Developer">
          </div>
          <div>
            <label style="font-size:11px;color:var(--text-muted);">Personality *</label>
            <textarea id="agent-personality" class="form-control" rows="3" placeholder="Describe how this agent behaves, communicates, and approaches tasks..."></textarea>
          </div>
          <div>
            <label style="font-size:11px;color:var(--text-muted);">Goals (comma separated, max 3)</label>
            <input type="text" id="agent-goals" class="form-control" placeholder="e.g. code_quality, performance, testing">
          </div>
          <div>
            <label style="font-size:11px;color:var(--text-muted);">Tools (comma separated)</label>
            <input type="text" id="agent-tools" class="form-control" placeholder="e.g. shell, file_read, file_write, grep">
          </div>
          <div style="display:flex;gap:12px;">
            <div style="flex:1;">
              <label style="font-size:11px;color:var(--text-muted);">Max Loops</label>
              <input type="number" id="agent-maxloops" class="form-control" value="10" min="1" max="50">
            </div>
            <div style="flex:1;">
              <label style="font-size:11px;color:var(--text-muted);">Autonomy (0=supervised, 4=autopilot)</label>
              <input type="number" id="agent-autonomy" class="form-control" value="2" min="0" max="4">
            </div>
          </div>
        </div>
        <div style="display:flex;gap:8px;justify-content:flex-end;margin-top:14px;">
          <button class="btn btn-secondary" onclick="closeAgentModal()">Cancel</button>
          <button class="btn btn-primary" onclick="saveFocusedAgent()">Save Agent</button>
        </div>
      </div>
    </div>
  `;
}

// ── Orchestrator edit ─────────────────────────────────────────────
function editOrchestrator() {
  const orch = state.personas.find(p => p.is_default || p.is_locked);
  const modal = document.getElementById('orch-modal');
  if (!modal) return;
  modal.style.display = 'flex';
  document.getElementById('orch-name').value = orch?.name || 'mike';
  document.getElementById('orch-user-name').value = orch?.role || '';
  document.getElementById('orch-personality').value = orch?.personality || '';
}

function closeOrchModal() {
  const modal = document.getElementById('orch-modal');
  if (modal) modal.style.display = 'none';
}

async function saveOrchestrator() {
  const orch = state.personas.find(p => p.is_default || p.is_locked);
  if (!orch) return;

  const body = {
    name: document.getElementById('orch-name').value.trim() || 'mike',
    role: document.getElementById('orch-user-name').value.trim(),
    personality: document.getElementById('orch-personality').value.trim(),
    is_default: true,
    is_locked: true,
  };

  // Use a special endpoint for orchestrator updates
  const res = await api(`/v1/personas/${orch.name}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  });

  closeOrchModal();
  await renderPersonas();
}

// ── Focused Agent CRUD ────────────────────────────────────────────
function openAgentModal(data) {
  const modal = document.getElementById('agent-modal');
  if (!modal) return;
  modal.style.display = 'flex';

  if (data) {
    document.getElementById('agent-modal-title').textContent = 'Edit Focused Agent';
    document.getElementById('agent-edit-original').value = data.name;
    document.getElementById('agent-name').value = data.name;
    document.getElementById('agent-name').disabled = true;
    document.getElementById('agent-role').value = data.role || '';
    document.getElementById('agent-personality').value = data.personality || '';
    document.getElementById('agent-goals').value = (data.goals || []).join(', ');
    document.getElementById('agent-tools').value = (data.tools || []).join(', ');
    document.getElementById('agent-maxloops').value = data.max_loops || 10;
    document.getElementById('agent-autonomy').value = data.autonomy || 2;
  } else {
    document.getElementById('agent-modal-title').textContent = 'New Focused Agent';
    document.getElementById('agent-edit-original').value = '';
    document.getElementById('agent-name').value = '';
    document.getElementById('agent-name').disabled = false;
    document.getElementById('agent-role').value = '';
    document.getElementById('agent-personality').value = '';
    document.getElementById('agent-goals').value = '';
    document.getElementById('agent-tools').value = 'shell, file_read, file_write';
    document.getElementById('agent-maxloops').value = 10;
    document.getElementById('agent-autonomy').value = 2;
  }
}

function closeAgentModal() {
  const modal = document.getElementById('agent-modal');
  if (modal) modal.style.display = 'none';
}

async function saveFocusedAgent() {
  const original = document.getElementById('agent-edit-original')?.value;
  const name = document.getElementById('agent-name').value.trim();
  const role = document.getElementById('agent-role').value.trim();
  const personality = document.getElementById('agent-personality').value.trim();
  const goals = document.getElementById('agent-goals').value.split(',').map(s => s.trim()).filter(Boolean);
  const tools = document.getElementById('agent-tools').value.split(',').map(s => s.trim()).filter(Boolean);
  const maxLoops = parseInt(document.getElementById('agent-maxloops').value) || 10;
  const autonomy = parseInt(document.getElementById('agent-autonomy').value) || 2;

  if (!name) { alert('Agent name is required'); return; }
  if (!role) { alert('Role is required'); return; }
  if (!personality) { alert('Personality is required'); return; }
  if (goals.length > 3) { alert('Max 3 goals allowed'); return; }

  const body = { name, role, personality, goals, tools, max_loops: maxLoops, autonomy };

  let res;
  if (original) {
    res = await api(`/v1/personas/${original}`, { method: 'PUT', body: JSON.stringify(body) });
  } else {
    res = await api('/v1/personas', { method: 'POST', body: JSON.stringify(body) });
  }

  if (res && !res.error) {
    closeAgentModal();
    await renderPersonas();
  } else {
    alert(res?.error || 'Failed to save agent');
  }
}

async function editAgent(name) {
  const agent = state.personas.find(p => p.name === name);
  if (agent) openAgentModal(agent);
}

async function deleteAgent(name) {
  if (!confirm(`Delete agent "${name}"? This cannot be undone.`)) return;
  const res = await api(`/v1/personas/${name}`, { method: 'DELETE' });
  if (res && !res.error) {
    await renderPersonas();
  } else {
    alert(res?.error || 'Failed to delete agent');
  }
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
  await Promise.all([fetchStatus(), fetchAgents(), fetchPersonas()]);
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
