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
  sessionId: null,
  personas: [],
  logs: [],
  logFilter: 'all',
  fsEvents: [],
  connected: false,
  ws: null,
  tasks: [],
  taskSearch: '',
  taskFilterPriority: -1,
  taskFilterLabel: '',
  taskFilterAgent: '',
  taskSort: 'priority',
  projects: [],
  selectedProjectId: '',
  taskViewMode: 'board',
  pendingApprovals: 0,
  isThinking: false,
  liveEvents: [],
  canvasOpen: false,
  canvasContent: null,
  canvasTitle: 'Preview',
  canvasUrl: null,
  projectFiles: [],
  liveAgentsOpen: false,
  liveAgentAnimId: null,
  agentActivity: {},
  agentCallCounts: {},
  // Agency context for chat.
  activeAgencyId: 0,
  activeSubAccountId: 0,
  activeAgencyName: '',
  activeSubAccountName: '',
  allAgencies: [],
};

// ── API Client ────────────────────────────────────────────────────
const API_BASE = window.location.origin;

async function api(path, opts = {}) {
  try {
    const { signal, ...restOpts } = opts;
    const res = await fetch(`${API_BASE}${path}`, {
      headers: { 'Content-Type': 'application/json' },
      ...restOpts,
      ...(signal ? { signal } : {}),
    });
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
    return await res.json();
  } catch (err) {
    if (err.name === 'AbortError') throw err; // Let AbortError propagate to caller
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
    state.agentDivisions = data.divisions || [];
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

async function sendChat(message, agentName, attachments) {
  const body = { message };
  if (state.sessionId) body.session_id = state.sessionId;
  body.channel = 'web';
  // Attach agency context if active.
  if (state.activeAgencyId) body.agency_id = state.activeAgencyId;
  if (state.activeSubAccountId) body.subaccount_id = state.activeSubAccountId;
  // Attach images/files if present.
  if (attachments && attachments.length > 0) {
    body.attachments = attachments.map(a => ({
      filename: a.name || 'image.png',
      mime_type: a.type || 'image/png',
      data: a.dataUrl,
    }));
  }

  // Use AbortController for a 120s timeout on chat requests.
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), 120000);

  const chatOpts = {
    method: 'POST',
    body: JSON.stringify(body),
    signal: controller.signal,
  };

  try {
    let result;
    if (agentName) {
      result = await api(`/v1/agents/${agentName}/chat`, chatOpts);
    } else {
      result = await api('/v1/chat', chatOpts);
    }
    clearTimeout(timeoutId);
    // Track session from server response.
    if (result && result.session_id) {
      state.sessionId = result.session_id;
    }
    return result;
  } catch (err) {
    clearTimeout(timeoutId);
    if (err.name === 'AbortError') {
      return { error: 'Request timed out after 2 minutes. The agent may still be processing.' };
    }
    return null;
  }
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

async function createTask(title, description, priority, labels, dueDate, projectId, agent, agentConfig, autoExec) {
  const body = { title, description };
  if (priority !== undefined) body.priority = parseInt(priority);
  if (labels && labels.length) body.labels = labels;
  if (dueDate) body.due_date = new Date(dueDate).toISOString();
  if (projectId) {
    body.project_id = projectId;
  } else if (state.selectedProjectId) {
    body.project_id = state.selectedProjectId;
  }
  if (agent) body.assigned_agent = agent;
  if (agentConfig) body.agent_config = agentConfig;
  if (autoExec) body.auto_execute = true;
  const data = await api('/v1/tasks', {
    method: 'POST',
    body: JSON.stringify(body),
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
      priority: task.priority || 0,
      labels: task.labels || [],
      due_date: task.due_date || null,
      sub_items: task.sub_items || [],
      project_id: task.project_id || '',
      blocked_by: task.blocked_by || [],
      blocks: task.blocks || [],
      recur_pattern: task.recur_pattern || '',
      assigned_agent: assignedAgent || task.assigned_agent,
    }),
  });
  if (data) await fetchTasks();
  return data;
}

async function editTask(id, title, description, status, assignedAgent, priority, labels, dueDate, subItems) {
  const body = {
    title,
    description,
    status,
    assigned_agent: assignedAgent,
    priority: parseInt(priority) || 0,
    labels: labels || [],
    sub_items: subItems || [],
    project_id: state.selectedProjectId || '',
    blocked_by: [],
    blocks: [],
  };
  if (dueDate) body.due_date = new Date(dueDate).toISOString();
  const data = await api(`/v1/tasks/${id}`, {
    method: 'PUT',
    body: JSON.stringify(body),
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
          // Auto-load project preview when pipeline generates files.
          setTimeout(() => loadProjectPreview(), 1500);
        }

        state.liveEvents.push({ type: evt.topic, text, icon, timestamp: new Date().toISOString() });
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

      // Update agentActivity for Live Agent Topology (always, not just when isThinking).
      if (evt.topic === 'agent.start' && evt.agent) {
        state.agentActivity[evt.agent] = {
          status: 'working',
          startedAt: Date.now(),
          task: evt.message || evt.data?.task || 'Processing...',
        };
        // Increment call count for this agent.
        state.agentCallCounts[evt.agent] = (state.agentCallCounts[evt.agent] || 0) + 1;
      }
      if (evt.topic === 'agent.tool_call' && evt.agent) {
        if (state.agentActivity[evt.agent]) {
          state.agentActivity[evt.agent].task = evt.data?.tool || evt.message || 'Using tool...';
        }
      }
      if (evt.topic === 'agent.complete' && evt.agent) {
        if (state.agentActivity[evt.agent]) {
          state.agentActivity[evt.agent].status = 'complete';
          state.agentActivity[evt.agent].completedAt = Date.now();
          // After 5 seconds, reset to idle (default color) but keep in sessionActiveAgents.
          setTimeout(() => {
            if (state.agentActivity[evt.agent] && state.agentActivity[evt.agent].status === 'complete') {
              state.agentActivity[evt.agent].status = 'idle';
            }
          }, 5000);
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
// ── Help Tips ─────────────────────────────────────────────────────
const helpTips = {
  chat:          'Send messages to your AI agents. Pick a specific agent or let the Orchestrator auto-route to the best one. Use "+ New Session" for a fresh conversation.',
  overview:      'System dashboard showing agent health, active sessions, memory usage, and real-time events at a glance.',
  analytics:     'Performance metrics: response times, token usage, tool calls, and agent activity trends over time.',
  logs:          'Live system log stream. Filter by level or agent to debug issues and track agent behavior.',
  agents:        'View and manage all registered agents. See their status, tools, and recent activity.',
  sessions:      'Browse all chat sessions. Click any session to review its full conversation history.',
  personas:      'Create focused agents with custom roles, goals, and tool access. Assign them to specific departments.',
  settings:      'Configure system-wide options: LLM provider, API keys, memory settings, and agent defaults.',
  tasks:         'Kanban task board. Create, assign, and track tasks across To Do, In Progress, and Done columns.',
  presentations: 'View and manage AI-generated slide decks and presentations.',
  database:      'Browse the agent memory database. Explore stored conversations, facts, and entities.',
  agency:        'Create and manage agency profiles. An agency groups agents, credentials, and contacts for a specific business.',
  credentials:   'Store API keys and login credentials that agents can use to connect to external services (CRMs, email, etc.).',
  automations:   'Schedule recurring agent tasks. Set cron-based triggers to run agents on a timed schedule.',
  marketplace:   'Install pre-built agent templates, tools, and plugins from the community marketplace.',
  projects:      'Organize work into projects. Each project groups related tasks, sessions, and agents.',
  contacts:      'Manage business contacts. Store client info that agents can reference during conversations.',
  pipelines:     'Visual sales/lead pipeline. Drag contacts through stages from Lead to Closed to track deals.',
  calendar:      'Calendar view of scheduled automations, tasks, and agent activity.',
  reports:       'Generate and view reports on agent performance, task completion, and business metrics.',
  knowledge:     'Manage the knowledge base. Upload documents and data that agents can search and reference.',
  workflows:     'Build multi-step automation workflows. Chain agent actions with triggers, conditions, and branching.',
  agencyDashboard: 'Agency-level metrics at a glance: contacts added, deals closed, revenue, and task completion rates.',
  agencyConversations: 'Unified inbox for all client communications across SMS, email, and chat channels.',
  agencyPayments: 'Manage invoices, track payments, and set up recurring billing for agency clients.',
  agencySites: 'Build and manage landing pages and sales funnels for client campaigns.',
  agencySocial: 'Schedule and manage social media posts across all connected platforms.',
  agencyReputation: 'Monitor reviews, request feedback, and manage client reputation across platforms.',
  agencyMemberships: 'Manage client portals, membership access levels, and course delivery.',
  agencyMedia: 'Centralized file storage for agency assets: images, documents, and brand materials.',
  agencyPhone: 'Manage phone numbers, view call logs, send SMS, and configure IVR for agency clients.',
  agencyBrands: 'Create and manage brand boards with colors, logos, fonts, and style guidelines.',
  voiceAI: 'Configure voice agents for inbound/outbound calls with AI-powered conversation handling.',
  conversationAI: 'Set up auto-reply chatbots and AI conversation flows across channels.',
  contentAI: 'Generate blog posts, emails, social content, and ad copy using AI models.',
  agentTemplates: 'Browse and deploy pre-built agent templates for common business workflows.'
};

// Generate a help-icon element with tooltip.
function helpIcon(tip, extraClass) {
  const cls = extraClass ? `help-icon ${extraClass}` : 'help-icon';
  return `<span class="${cls}">?<span class="help-tooltip">${tip}</span></span>`;
}

// Generate a section header with inline help icon.
function sectionHelp(label, tip) {
  return `<span class="section-help">${label} ${helpIcon(tip)}</span>`;
}

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
    sessions: 'Sessions',
    personas: 'Agents',
    settings: 'Settings',
    tasks: 'Task Board',
    presentations: 'Presentations',
    database: 'Database',
    agency: 'Agency',
    credentials: 'Credentials',
    automations: 'Automations',
    marketplace: 'Marketplace',
    projects: 'Projects',
    contacts: 'Contacts',
    pipelines: 'Pipelines',
    calendar: 'Calendar',
    reports: 'Reports',
    knowledge: 'Knowledge Base',
    workflows: 'Workflows',
    agencyDashboard: 'Agency Dashboard',
    agencyConversations: 'Conversations',
    agencyPayments: 'Payments',
    agencySites: 'Sites & Funnels',
    agencySocial: 'Social Planner',
    agencyReputation: 'Reputation',
    agencyMemberships: 'Memberships',
    agencyMedia: 'Media Library',
    agencyPhone: 'Phone System',
    agencyBrands: 'Brand Boards',
    voiceAI: 'Voice AI',
    conversationAI: 'Conversation AI',
    contentAI: 'Content AI',
    agentTemplates: 'Agent Templates',
  };
  const titleEl = document.getElementById('page-title');
  const titleText = titles[page] || page;
  const tip = helpTips[page] || '';
  titleEl.innerHTML = titleText + (tip ? ' ' + helpIcon(tip, 'page-help-icon') : '');

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
    case 'sessions': await renderSessionsPage(content); break;
    case 'personas': await renderPersonas(content); break;
    case 'settings': renderSettings(content); break;
    case 'tasks': await renderTasks(content); break;
    case 'presentations': await renderPresentations(content); break;
    case 'database': await renderDatabase(content); break;
    case 'agency': await renderAgencyPage(content); break;
    case 'credentials': await renderCredentialsPage(content); break;
    case 'automations': await renderAutomationsPage(content); break;
    case 'marketplace': await renderMarketplacePage(content); break;
    case 'projects': await renderProjectsPage(content); break;
    case 'contacts': await renderContactsPage(content); break;
    case 'pipelines': await renderPipelinesPage(content); break;
    case 'calendar': await renderCalendarPage(content); break;
    case 'reports': await renderReportsPage(content); break;
    case 'knowledge': await renderKnowledgePage(content); break;
    case 'workflows': await renderWorkflowsPage(content); break;
    case 'agencyDashboard': await renderAgencyDashboardPage(content); break;
    case 'agencyConversations': await renderAgencyConversationsPage(content); break;
    case 'agencyPayments': await renderAgencyPaymentsPage(content); break;
    case 'agencySites': await renderAgencySitesPage(content); break;
    case 'agencySocial': await renderAgencySocialPage(content); break;
    case 'agencyReputation': await renderAgencyReputationPage(content); break;
    case 'agencyMemberships': await renderAgencyMembershipsPage(content); break;
    case 'agencyMedia': await renderAgencyMediaPage(content); break;
    case 'agencyPhone': await renderAgencyPhonePage(content); break;
    case 'agencyBrands': await renderAgencyBrandsPage(content); break;
    case 'voiceAI': await renderVoiceAIPage(content); break;
    case 'conversationAI': await renderConversationAIPage(content); break;
    case 'contentAI': await renderContentAIPage(content); break;
    case 'agentTemplates': await renderAgentTemplatesPage(content); break;
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
    text.textContent = 'Connected';
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

// ── Collapsible nav groups (localStorage persistence) ──────────────
function toggleNavGroup(labelEl) {
  const group = labelEl.closest('.nav-group');
  if (!group) return;
  group.classList.toggle('collapsed');

  // Persist state -- use the text content of the label span as the key.
  const name = labelEl.querySelector('span') ? labelEl.querySelector('span').textContent.trim() : '';
  if (name) {
    const key = 'nav-collapsed';
    let collapsed = {};
    try { collapsed = JSON.parse(localStorage.getItem(key) || '{}'); } catch(e) {}
    collapsed[name] = group.classList.contains('collapsed');
    localStorage.setItem(key, JSON.stringify(collapsed));
  }
}

// Restore nav group collapse states on load.
// Default: all collapsed for a cleaner look. Only expand if user explicitly opened them.
(function restoreNavGroups() {
  let saved = {};
  try { saved = JSON.parse(localStorage.getItem('nav-collapsed') || '{}'); } catch(e) {}
  document.querySelectorAll('.nav-group').forEach(group => {
    const label = group.querySelector('.nav-group-label span');
    if (!label) return;
    const name = label.textContent.trim();
    // Default to collapsed. Only expand if explicitly saved as false (user opened it).
    if (saved[name] === false) {
      group.classList.remove('collapsed');
    } else {
      group.classList.add('collapsed');
    }
  });

  // Auto-expand the group containing the active page so user can see where they are.
  const hash = window.location.hash.replace('#', '') || 'chat';
  const activeItem = document.querySelector(`.nav-item[data-page="${hash}"]`);
  if (activeItem) {
    const parentGroup = activeItem.closest('.nav-group');
    if (parentGroup) parentGroup.classList.remove('collapsed');
  }
})();

// ── Page: Chat ────────────────────────────────────────────────────
function renderEventBoxes(events) {
  if (!events || events.length === 0) return '';
  return `<div class="status-events" style="margin:8px 0; display:flex; flex-direction:column; gap:6px;">` +
    events.map(e => {
      const ts = e.timestamp ? new Date(e.timestamp).toLocaleTimeString([], {hour:'2-digit',minute:'2-digit',second:'2-digit'}) : '';
      return `
      <div class="status-box" style="display:flex; align-items:center; gap:8px; padding:6px 10px; background:var(--bg-card); border:1px solid var(--border); border-radius:var(--radius-sm); font-family:var(--mono); font-size:12px; color:var(--text-primary);">
        <span style="font-size:14px;">${e.icon}</span>
        <span style="flex:1;">${e.text}</span>
        ${ts ? `<span style="font-size:9px;color:var(--text-muted);white-space:nowrap;">${ts}</span>` : ''}
      </div>`;
    }).join('') +
  `</div>`;
}

function renderChatMessages() {
  const msgBox = document.getElementById('chat-messages');
  if (!msgBox) return;

  const messages = state.chatMessages.map(m => {
    const ts = new Date(m.timestamp || Date.now()).toLocaleTimeString([], {hour:'2-digit',minute:'2-digit'});
    const tsColor = m.role === 'user' ? '#1a1a1a' : 'var(--text-muted)';

    // Render attached images inline.
    const attachmentHtml = (m.attachments && m.attachments.length > 0)
      ? `<div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:8px;">${m.attachments.map(a =>
          `<img src="${a.dataUrl || a.data}" alt="${escapeHtml(a.name || a.filename || 'attachment')}" style="max-width:240px;max-height:180px;border-radius:8px;border:1px solid var(--border);cursor:pointer;object-fit:cover;" onclick="window.open(this.src,'_blank')">`
        ).join('')}</div>`
      : '';

    if (m.role === 'user') {
      return `
      <div class="chat-msg user">
        <div style="display:flex;justify-content:space-between;align-items:center;">
          <div></div>
          <span style="font-size:10px;color:${tsColor};font-family:var(--mono);opacity:0.85;">${ts}</span>
        </div>
        ${attachmentHtml}
        <div class="msg-content">${formatContent(m.content)}</div>
      </div>`;
    }

    // Assistant message - polished card template.
    const agentName = m.agent || 'orchestrator';
    const agentLabel = agentName.charAt(0).toUpperCase() + agentName.slice(1);
    const content = formatContent(m.content);
    return `
    <div class="chat-msg assistant">
      <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px;">
        <div style="width:28px;height:28px;border-radius:50%;background:linear-gradient(135deg,#e85d04,#f48c06);display:flex;align-items:center;justify-content:center;font-size:14px;flex-shrink:0;">🤖</div>
        <div style="display:flex;flex-direction:column;flex:1;">
          <span style="font-size:12px;font-weight:600;color:var(--accent);text-transform:uppercase;letter-spacing:.3px;">${agentLabel}</span>
        </div>
        <span style="font-size:10px;color:var(--text-muted);font-family:var(--mono);opacity:0.85;">${ts}</span>
      </div>
      ${m.events ? renderEventBoxes(m.events) : ''}
      ${attachmentHtml}
      <div class="msg-content" style="padding:10px 12px;background:rgba(0,0,0,0.15);border-radius:8px;border-left:3px solid var(--accent);line-height:1.6;font-size:13px;">${content}</div>
    </div>`;
  }).join('');

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

  const panelOpen = state.rightPanelOpen || false;
  const activeTab = state.rightPanelTab || 'canvas';

  container.innerHTML = `
    <div class="chat-split ${panelOpen ? 'canvas-active' : ''}" id="chat-split">
      <div class="chat-pane">
        <div class="chat-container">
          <div style="display:flex;gap:8px;align-items:center;margin-bottom:8px;flex-wrap:wrap;">
            <span class="section-label" style="margin:0;">Agent: ${helpIcon('Pick a specific agent to handle your message, or leave on Orchestrator to let the system auto-route to the best agent.')}</span>
            <select id="chat-agent-select" onchange="state.chatAgent=this.value" style="${selectStyle}">
              <option value="">Orchestrator (auto-route)</option>
              ${agentOpts}
              ${focusedPersonaOpts}
            </select>
            <span class="section-label" style="margin:0;">Context: ${helpIcon('Set the business context for the conversation. Agents will use this agency\'s contacts, credentials, and knowledge.')}</span>
            <select id="chat-context-select" onchange="setAgencyContext(this.value)" style="${selectStyle}">
              ${buildAgencyDropdown()}
            </select>
            <button class="btn" style="font-size:11px;padding:4px 10px;" onclick="navigate('personas')" title="Create a new focused agent">
              + New Agent
            </button>
            <button class="btn btn-primary" style="font-size:11px;padding:4px 10px;" onclick="createNewSession()" title="Start a fresh chat session">
              + New Session
            </button>
            <div style="display:flex;gap:2px;margin-left:auto;">
              <button class="canvas-toggle ${panelOpen && activeTab === 'agents' ? 'active' : ''}" onclick="openRightPanel('agents')" title="Live Agent Topology">
                📡 Live Agents
              </button>
              <button class="canvas-toggle ${panelOpen && activeTab === 'canvas' ? 'active' : ''}" onclick="openRightPanel('canvas')" title="Canvas Preview">
                🖼 Canvas
              </button>
              <button class="canvas-toggle ${panelOpen && activeTab === 'code' ? 'active' : ''}" onclick="openRightPanel('code')" title="Project Files">
                📝 Code
              </button>
              <button class="canvas-toggle ${panelOpen && activeTab === 'browser' ? 'active' : ''}" onclick="openRightPanel('browser')" title="Agent Browser View">
                🌐 Browser
              </button>
            </div>
          </div>

          <div class="chat-messages" id="chat-messages">
            <!-- Rendered by renderChatMessages() -->
          </div>

          <!-- Image attachment preview strip -->
          <div id="attachment-preview" style="display:none;padding:6px 8px;background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius-sm);margin-bottom:4px;">
            <div style="display:flex;align-items:center;gap:4px;margin-bottom:4px;">
              <span style="font-size:11px;color:var(--text-muted);font-family:var(--mono);">Attachments</span>
              <button onclick="clearAttachments()" style="margin-left:auto;background:none;border:none;color:var(--red);cursor:pointer;font-size:11px;font-family:var(--mono);">Clear all</button>
            </div>
            <div id="attachment-thumbs" style="display:flex;gap:6px;flex-wrap:wrap;"></div>
          </div>
          <div class="chat-input-area">
            <input type="file" id="chat-file-input" accept="image/*,.webp" multiple style="display:none;" onchange="handleFileAttach(this.files)">
            <button class="btn" onclick="document.getElementById('chat-file-input').click()" title="Attach image" style="padding:6px 10px;font-size:16px;line-height:1;flex-shrink:0;">📎</button>
            <input type="text" id="chat-input" placeholder="Type a message... (Enter to send, Ctrl+V to paste image)" autofocus
              onkeydown="if(event.key==='Enter')handleChatSend()">
            <button class="btn btn-primary" onclick="handleChatSend()">Send</button>
          </div>
        </div>
      </div>

      <div class="right-panel ${panelOpen ? 'visible' : ''}" id="right-panel">
        <!-- Tab bar -->
        <div class="right-panel-tabs">
          <button class="rp-tab ${activeTab === 'canvas' ? 'active' : ''}" onclick="switchRightTab('canvas')">🖼 Canvas</button>
          <button class="rp-tab ${activeTab === 'agents' ? 'active' : ''}" onclick="switchRightTab('agents')">📡 Agents</button>
          <button class="rp-tab ${activeTab === 'browser' ? 'active' : ''}" onclick="switchRightTab('browser')">🌐 Browser</button>
          <button class="right-panel-close" onclick="closeRightPanel()" title="Close panel">×</button>
        </div>

        <!-- Canvas tab -->
        <div class="rp-content" style="display:${activeTab === 'canvas' ? 'flex' : 'none'};" id="rp-canvas">
          <div class="canvas-header">
            <div class="canvas-header-left">
              <span class="canvas-header-icon">🖼</span>
              <span class="canvas-title" id="canvas-title">${escapeHtml(state.canvasTitle)}</span>
              ${state.canvasUrl ? `<span class="canvas-subtitle" title="${escapeHtml(state.canvasUrl)}">${escapeHtml(state.canvasUrl)}</span>` : ''}
            </div>
            <div class="canvas-header-actions">
              ${state.canvasUrl ? `<button class="canvas-btn" onclick="window.open('${state.canvasUrl}','_blank')">↗ Open</button>` : ''}
              <button class="canvas-btn" onclick="refreshCanvas()">↻ Refresh</button>
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
        </div>

        <!-- Code tab (project file browser) -->
        <div class="rp-content" style="display:${activeTab === 'code' ? 'flex' : 'none'};flex-direction:column;" id="rp-code">
          <div class="canvas-header">
            <div class="canvas-header-left">
              <button class="canvas-btn" id="code-back-btn" onclick="codeViewerBack()" style="display:none;margin-right:6px;">← Back</button>
              <span class="canvas-header-icon">📝</span>
              <span class="canvas-title" id="code-file-name">Project Files</span>
            </div>
            <div class="canvas-header-actions">
              <button class="canvas-btn" onclick="loadProjectFiles()">↻ Refresh</button>
              <button class="canvas-btn" onclick="loadProjectPreview()">▶ Preview</button>
            </div>
          </div>
          <div class="canvas-body" style="flex-direction:column;overflow-y:auto;padding:0;">
            <div id="code-file-list" style="padding:4px;"></div>
            <div id="code-viewer" style="display:none;flex:1;overflow:auto;">
              <pre style="margin:0;padding:12px;font-size:12px;font-family:'JetBrains Mono',monospace;white-space:pre-wrap;word-break:break-all;color:var(--text);background:var(--bg-main);min-height:100%;"></pre>
            </div>
          </div>
        </div>

        <!-- Live Agents tab -->
        <div class="rp-content" style="display:${activeTab === 'agents' ? 'flex' : 'none'};" id="rp-agents">
          <div class="canvas-header">
            <div class="canvas-header-left">
              <span class="canvas-header-icon">📡</span>
              <span class="canvas-title">Live Agent Topology</span>
            </div>
            <div class="canvas-header-actions">
              <button class="canvas-btn" onclick="simulateAgentActivity()" title="Simulate delegation">⚡ Simulate</button>
            </div>
          </div>
          <div class="canvas-body" style="position:relative;overflow:hidden;">
            <canvas id="agent-topology-canvas" style="width:100%;height:100%;display:block;"></canvas>
          </div>
        </div>

        <!-- Browser tab -->
        <div class="rp-content" style="display:${activeTab === 'browser' ? 'flex' : 'none'};" id="rp-browser">
          <div class="canvas-header">
            <div class="canvas-header-left">
              <span class="canvas-header-icon">🌐</span>
              <span class="canvas-title">Agent Browser</span>
            </div>
            <div class="canvas-header-actions">
              <button class="canvas-btn" onclick="refreshBrowserView()">↻ Refresh</button>
            </div>
          </div>
          <div class="canvas-body" id="browser-body" style="position:relative;">
            <!-- Agent badge overlay -->
            <div id="browser-agent-badge" style="
              position:absolute;top:8px;right:8px;z-index:10;
              background:rgba(13,17,23,0.85);backdrop-filter:blur(8px);
              border:1px solid var(--border);border-radius:var(--radius-sm);
              padding:4px 10px;display:flex;align-items:center;gap:6px;
              font-size:11px;color:var(--text-secondary);
              pointer-events:none;
            ">
              <span style="width:6px;height:6px;border-radius:50%;background:var(--green);display:inline-block;"></span>
              <span id="browser-active-agent">Orchestrator</span>
            </div>
            <iframe id="browser-iframe" src="about:blank" style="width:100%;height:100%;border:none;border-radius:0 0 var(--radius) var(--radius);"
              sandbox="allow-scripts allow-same-origin allow-popups allow-forms"></iframe>
          </div>
        </div>
      </div>
    </div>
  `;

  renderChatMessages();
  if (panelOpen && activeTab === 'agents') initAgentTopology();

  // If canvas has srcdoc content (not a URL), inject it after render.
  if (panelOpen && activeTab === 'canvas' && state.canvasContent && !state.canvasUrl) {
    const iframe = document.getElementById('canvas-iframe');
    if (iframe) iframe.srcdoc = state.canvasContent;
  }
}

// ── Right Panel Management ────────────────────────────────────────
function openRightPanel(tab) {
  // If clicking the same tab that's active, close the panel.
  if (state.rightPanelOpen && state.rightPanelTab === tab) {
    closeRightPanel();
    return;
  }
  state.rightPanelOpen = true;
  state.rightPanelTab = tab;
  // Keep canvas state in sync.
  state.canvasOpen = (tab === 'canvas');
  state.liveAgentsOpen = (tab === 'agents');
  renderChat();
}

function switchRightTab(tab) {
  state.rightPanelTab = tab;
  state.canvasOpen = (tab === 'canvas');
  state.liveAgentsOpen = (tab === 'agents');
  renderChat();
  // Auto-load project files when switching to Code tab.
  if (tab === 'code') loadProjectFiles();
}

function closeRightPanel() {
  state.rightPanelOpen = false;
  state.canvasOpen = false;
  state.liveAgentsOpen = false;
  if (state.liveAgentAnimId) {
    cancelAnimationFrame(state.liveAgentAnimId);
    state.liveAgentAnimId = null;
  }
  renderChat();
}

function refreshBrowserView() {
  const iframe = document.getElementById('browser-iframe');
  if (iframe && iframe.src !== 'about:blank') {
    iframe.src = iframe.src; // reload
  }
}

async function handleChatSend() {
  const input = document.getElementById('chat-input');
  if (!input) return;
  const msg = input.value.trim();
  const hasAttachments = state.pendingAttachments && state.pendingAttachments.length > 0;
  if (!msg && !hasAttachments) return;
  input.value = '';

  const agent = state.chatAgent || '';
  const attachments = state.pendingAttachments ? [...state.pendingAttachments] : [];
  state.chatMessages.push({
    role: 'user',
    content: msg || (attachments.length > 0 ? `[${attachments.length} image(s) attached]` : ''),
    agent: agent || null,
    attachments: attachments,
    timestamp: new Date().toISOString()
  });
  clearAttachments();
  
  state.isThinking = true;
  state.liveEvents = [];
  renderChatMessages();

  const result = await sendChat(msg, agent, attachments);
  
  state.isThinking = false;

  if (result) {
    state.chatMessages.push({
      role: 'assistant',
      content: result.response || result.error || 'No response',
      agent: agent || 'orchestrator',
      events: [...state.liveEvents],
      timestamp: new Date().toISOString()
    });
  } else {
    state.chatMessages.push({
      role: 'assistant',
      content: 'Failed to reach the agent. Check if the server is running.',
      agent: 'system',
      timestamp: new Date().toISOString()
    });
  }

  state.liveEvents = [];
  renderChatMessages();
}

// ── Image Attachment Helpers ──────────────────────────────────────

// Initialize state for pending attachments.
if (!state.pendingAttachments) state.pendingAttachments = [];

// Handle file input change (from paperclip button).
function handleFileAttach(files) {
  if (!files || files.length === 0) return;
  for (const file of files) {
    if (!file.type.startsWith('image/')) continue;
    const reader = new FileReader();
    reader.onload = (e) => {
      state.pendingAttachments.push({
        name: file.name,
        type: file.type,
        dataUrl: e.target.result,
      });
      renderAttachmentPreview();
    };
    reader.readAsDataURL(file);
  }
  // Reset file input so the same file can be re-selected.
  const fileInput = document.getElementById('chat-file-input');
  if (fileInput) fileInput.value = '';
}

// Handle clipboard paste on the chat input.
document.addEventListener('paste', function(e) {
  // Only intercept if the chat page is active.
  if (state.page !== 'chat') return;

  const items = e.clipboardData?.items;
  if (!items) return;

  for (const item of items) {
    if (item.type.startsWith('image/')) {
      e.preventDefault();
      const blob = item.getAsFile();
      if (!blob) continue;
      const reader = new FileReader();
      reader.onload = (ev) => {
        const ext = blob.type.split('/')[1] || 'png';
        state.pendingAttachments.push({
          name: `pasted-image.${ext}`,
          type: blob.type,
          dataUrl: ev.target.result,
        });
        renderAttachmentPreview();
      };
      reader.readAsDataURL(blob);
    }
  }
});

// Handle drag and drop on the chat messages area.
document.addEventListener('dragover', function(e) {
  if (state.page !== 'chat') return;
  e.preventDefault();
  e.dataTransfer.dropEffect = 'copy';
});
document.addEventListener('drop', function(e) {
  if (state.page !== 'chat') return;
  e.preventDefault();
  const files = e.dataTransfer?.files;
  if (files) handleFileAttach(files);
});

function renderAttachmentPreview() {
  const strip = document.getElementById('attachment-preview');
  const thumbs = document.getElementById('attachment-thumbs');
  if (!strip || !thumbs) return;

  if (state.pendingAttachments.length === 0) {
    strip.style.display = 'none';
    return;
  }
  strip.style.display = 'block';

  thumbs.innerHTML = state.pendingAttachments.map((a, i) => `
    <div style="position:relative;display:inline-block;">
      <img src="${a.dataUrl}" alt="${escapeHtml(a.name)}" style="width:64px;height:64px;object-fit:cover;border-radius:6px;border:1px solid var(--border);">
      <button onclick="removeAttachment(${i})" style="
        position:absolute;top:-4px;right:-4px;
        width:18px;height:18px;border-radius:50%;
        background:var(--red);color:white;border:none;
        font-size:11px;cursor:pointer;display:flex;
        align-items:center;justify-content:center;line-height:1;
      ">x</button>
      <div style="font-size:9px;color:var(--text-muted);text-align:center;margin-top:2px;max-width:64px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${escapeHtml(a.name)}</div>
    </div>
  `).join('');
}

function removeAttachment(index) {
  state.pendingAttachments.splice(index, 1);
  renderAttachmentPreview();
}

function clearAttachments() {
  state.pendingAttachments = [];
  const strip = document.getElementById('attachment-preview');
  if (strip) strip.style.display = 'none';
  const thumbs = document.getElementById('attachment-thumbs');
  if (thumbs) thumbs.innerHTML = '';
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
  state.canvasContent = null;
  state.canvasTitle = title || url.split('/').pop();
  state.canvasUrl = url;
  openRightPanel('canvas');
}

// Close the canvas.
function closeCanvas() {
  closeRightPanel();
}

// Toggle canvas visibility.
function toggleCanvas() {
  openRightPanel('canvas');
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

// ── Project Preview (auto-load from pipeline output) ─────────────

// Check if project files exist and auto-load the preview.
async function loadProjectPreview() {
  try {
    const resp = await fetch('/v1/projects/latest/files');
    if (!resp.ok) return;
    const data = await resp.json();
    if (!data.files || data.files.length === 0) return;

    // Check if there's an index.html to preview.
    const hasIndex = data.files.some(f => f.name === 'index.html');
    if (hasIndex) {
      openCanvasUrl('/v1/projects/latest/preview', 'Site Preview');
      state.projectFiles = data.files;
    }
  } catch (e) {
    console.debug('Project preview check failed:', e);
  }
}

// Load the project file list for the Code tab.
async function loadProjectFiles() {
  try {
    const resp = await fetch('/v1/projects/latest/files');
    if (!resp.ok) return;
    const data = await resp.json();
    state.projectFiles = data.files || [];
    renderCodeTab();
  } catch (e) {
    console.debug('Project files load failed:', e);
  }
}

// Render the Code tab file list.
function renderCodeTab() {
  const container = document.getElementById('code-file-list');
  if (!container) return;

  const files = state.projectFiles || [];
  if (files.length === 0) {
    container.innerHTML = `
      <div style="padding:24px;text-align:center;color:var(--text-muted);">
        <div style="font-size:24px;margin-bottom:8px;">📂</div>
        <div>No project files yet</div>
        <div style="font-size:11px;margin-top:4px;">Ask the agent to build a website</div>
      </div>`;
    return;
  }

  // Group files by extension for icons.
  const extIcon = (name) => {
    const ext = name.split('.').pop().toLowerCase();
    switch (ext) {
      case 'html': case 'htm': return '📄';
      case 'css': return '🎨';
      case 'js': return '⚡';
      case 'json': return '📋';
      case 'svg': return '🖼';
      case 'png': case 'jpg': case 'jpeg': case 'webp': return '🖼';
      default: return '📄';
    }
  };

  const formatSize = (bytes) => {
    if (bytes < 1024) return bytes + ' B';
    return (bytes / 1024).toFixed(1) + ' KB';
  };

  container.innerHTML = files.map(f => `
    <div class="code-file-item" onclick="viewProjectFile('${escapeHtml(f.path)}')" title="${escapeHtml(f.path)}">
      <span class="code-file-icon">${extIcon(f.name)}</span>
      <span class="code-file-name">${escapeHtml(f.name)}</span>
      <span class="code-file-size">${formatSize(f.size)}</span>
    </div>`).join('');
}

// View a specific project file's content.
async function viewProjectFile(filePath) {
  try {
    const resp = await fetch('/v1/projects/latest/preview/' + filePath);
    if (!resp.ok) return;
    const content = await resp.text();

    const viewer = document.getElementById('code-viewer');
    const fileList = document.getElementById('code-file-list');
    const backBtn = document.getElementById('code-back-btn');
    const fileName = document.getElementById('code-file-name');

    if (viewer) {
      viewer.style.display = 'block';
      viewer.querySelector('pre').textContent = content;
    }
    if (fileList) fileList.style.display = 'none';
    if (backBtn) backBtn.style.display = 'inline-flex';
    if (fileName) fileName.textContent = filePath;
  } catch (e) {
    console.debug('File view failed:', e);
  }
}

// Go back from file viewer to file list.
function codeViewerBack() {
  const viewer = document.getElementById('code-viewer');
  const fileList = document.getElementById('code-file-list');
  const backBtn = document.getElementById('code-back-btn');

  if (viewer) viewer.style.display = 'none';
  if (fileList) fileList.style.display = 'block';
  if (backBtn) backBtn.style.display = 'none';
}

// ── Live Agent Topology ──────────────────────────────────────────

function toggleLiveAgents() {
  openRightPanel('agents');
}

function closeLiveAgents() {
  closeRightPanel();
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
  // Ensure agents are loaded for topology nodes.
  if (!state.agents || state.agents.length === 0) {
    fetchAgents().then(() => initAgentTopology());
    return;
  }
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
  // Track which agents were activated during this session on state (globally accessible).
  // Once an agent appears, it stays for the duration of the session.
  if (!state.sessionActiveAgents) state.sessionActiveAgents = new Set();

  // Build agent node list: only orchestrator initially, agents appear when called.
  function getNodes() {
    const allAgents = state.agents || [];

    // Accumulate: any agent that has ever been "working" this session stays visible.
    allAgents.forEach(a => {
      const act = state.agentActivity[a.name];
      if (act && (act.status === 'working' || act.status === 'complete')) {
        state.sessionActiveAgents.add(a.name);
      }
    });

    // Only show agents that have been activated this session.
    const visibleAgents = allAgents.filter(a => state.sessionActiveAgents.has(a.name));

    return {
      orch: { name: 'mike', role: 'Orchestrator' },
      focused: visibleAgents.map(a => ({ name: a.name, role: a.role || a.name }))
    };
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
      // Rotate the brain emoji slowly.
      ctx.save();
      ctx.translate(x, y);
      ctx.rotate(t * 0.5); // slow clockwise spin
      ctx.fillText('\ud83e\udde0', 0, 0);
      ctx.restore();
    } else {
      // Status-based icon
      const icons = { idle: '\u2b55', working: '\u26a1', complete: '\u2705', error: '\u274c' };
      ctx.fillText(icons[status] || '\u2b55', x, y);
    }

    // Call count badge (top-right of circle for non-orchestrator agents).
    const callCount = state.agentCallCounts[name] || 0;
    if (callCount > 0 && type !== 'orchestrator') {
      const badgeR = Math.max(8, r * 0.32);
      const bx = x + r * 0.7;
      const by = y - r * 0.7;
      // Badge background.
      ctx.beginPath();
      ctx.arc(bx, by, badgeR, 0, Math.PI * 2);
      ctx.fillStyle = '#e85d04';
      ctx.fill();
      ctx.strokeStyle = colors.bg;
      ctx.lineWidth = 1.5;
      ctx.stroke();
      // Badge text.
      ctx.font = `bold ${Math.max(8, badgeR * 1.1)}px Inter, sans-serif`;
      ctx.textAlign = 'center';
      ctx.textBaseline = 'middle';
      ctx.fillStyle = '#fff';
      ctx.fillText(callCount > 99 ? '99+' : String(callCount), bx, by);
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
    } else {
      const sessionCount = state.sessionActiveAgents ? state.sessionActiveAgents.size : 0;
      ctx.fillStyle = colors.textMuted;
      ctx.font = '11px Inter, sans-serif';
      ctx.fillText(sessionCount > 0 ? `${sessionCount} agent${sessionCount > 1 ? 's' : ''} used this session` : 'Waiting for task...', 16, 40);
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

  // Store agents data and divisions for filter.
  const allAgents = state.agents || [];
  const divisions = state.agentDivisions || [];

  // State for filters (stored on window so event handlers can access).
  if (!window._agentFilter) window._agentFilter = { search: '', division: 'all', source: 'all' };

  renderAgentCatalog(container, allAgents, divisions);
}

function renderAgentCatalog(container, allAgents, divisions) {
  const f = window._agentFilter;

// -- Agent Usage Leaderboard --
function buildLeaderboard(allAgents) {
  const counts = state.agentCallCounts || {};
  const entries = allAgents
    .map(a => ({ name: a.name, role: a.role || 'Agent', source: a.source, count: counts[a.name] || 0 }))
    .filter(e => e.count > 0)
    .sort((a, b) => b.count - a.count);

  if (entries.length === 0) return '';

  const totalCalls = entries.reduce((s, e) => s + e.count, 0);
  const maxCount = entries[0].count;

  return `
    <div style="margin-bottom:24px;padding:16px;background:var(--card-bg);border:1px solid var(--border);border-radius:var(--radius);">
      <div style="display:flex;align-items:center;gap:8px;margin-bottom:12px;">
        <span style="font-size:18px;">🏆</span>
        <span style="font-size:14px;font-weight:600;color:var(--text);text-transform:uppercase;letter-spacing:.5px;">Agent Leaderboard</span>
        <span style="font-size:11px;color:var(--text-muted);margin-left:auto;">${totalCalls} total calls</span>
      </div>
      <div style="display:flex;flex-direction:column;gap:8px;">
        ${entries.slice(0, 10).map((e, i) => {
          const pct = Math.round((e.count / totalCalls) * 100);
          const barW = Math.round((e.count / maxCount) * 100);
          const medal = i === 0 ? '🥇' : i === 1 ? '🥈' : i === 2 ? '🥉' : `#${i+1}`;
          const srcColor = e.source === 'core' ? 'var(--success)' : e.source === 'focus' ? 'var(--accent)' : '#8b5cf6';
          return `
            <div style="display:flex;align-items:center;gap:10px;">
              <span style="width:28px;text-align:center;font-size:${i < 3 ? '16px' : '11px'};color:var(--text-muted);">${medal}</span>
              <span style="width:100px;font-size:12px;font-weight:600;color:var(--text);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">${e.name}</span>
              <div style="flex:1;height:20px;background:var(--bg-secondary);border-radius:10px;overflow:hidden;position:relative;">
                <div style="width:${barW}%;height:100%;background:linear-gradient(90deg,#e85d04,#f48c06);border-radius:10px;transition:width .3s;"></div>
                <span style="position:absolute;right:8px;top:2px;font-size:10px;color:var(--text);font-weight:600;">${pct}%</span>
              </div>
              <span style="width:40px;text-align:right;font-size:13px;font-weight:700;color:#e85d04;">${e.count}x</span>
            </div>`;
        }).join('')}
      </div>
    </div>`;
}


  // Apply filters.
  let filtered = allAgents.filter(a => {
    const matchSearch = !f.search || 
      (a.name || '').toLowerCase().includes(f.search) ||
      (a.role || '').toLowerCase().includes(f.search) ||
      (a.personality || '').toLowerCase().includes(f.search);
    const matchDiv = f.division === 'all' || (a.division || '').toLowerCase() === f.division.toLowerCase();
    const matchSrc = f.source === 'all' || (a.source || '') === f.source;
    return matchSearch && matchDiv && matchSrc;
  });

  // Group by division.
  const groups = {};
  filtered.forEach(a => {
    const div = a.division || (a.source === 'core' ? 'Core' : 'General');
    if (!groups[div]) groups[div] = [];
    groups[div].push(a);
  });

  // Division counts (unfiltered, for tab badges).
  const divCounts = {};
  allAgents.forEach(a => {
    const div = a.division || (a.source === 'core' ? 'Core' : 'General');
    divCounts[div] = (divCounts[div] || 0) + 1;
  });

  // Source counts.
  const srcCounts = { core: 0, focus: 0, agency: 0 };
  allAgents.forEach(a => { if (srcCounts[a.source] !== undefined) srcCounts[a.source]++; });

  const divIcons = {
    'Core': '⚡', 'Engineering': '💻', 'Design': '🎨', 'Marketing': '📢',
    'Sales': '💼', 'Paid Media': '💰', 'Product': '📊', 'Project Management': '🎬',
    'Testing': '🧪', 'Support': '🛟', 'Specialized': '🎯', 'Game Development': '🎮', 'General': '🤖'
  };

  const allDivs = ['all', 'Core', ...divisions.filter(d => d !== 'Core')];

  container.innerHTML = `
    <div style="display:flex;align-items:center;gap:12px;margin-bottom:16px;flex-wrap:wrap;">
      <div class="section-label" style="margin:0;">Agent Catalog (${filtered.length}/${allAgents.length})</div>
      <input id="agent-search" type="text" placeholder="Search agents..." value="${f.search}"
        oninput="window._agentFilter.search=this.value.toLowerCase();renderAgentCatalog(document.getElementById('page-content'), state.agents, state.agentDivisions || [])"
        style="flex:1;min-width:200px;max-width:400px;padding:6px 12px;border-radius:6px;border:1px solid var(--border);background:var(--card-bg);color:var(--text);font-size:13px;outline:none;" />
      <div style="display:flex;gap:4px;">
        ${['all','core','focus','agency'].map(s => `
          <button onclick="window._agentFilter.source='${s}';renderAgentCatalog(document.getElementById('page-content'), state.agents, state.agentDivisions || [])"
            style="padding:4px 10px;border-radius:4px;border:1px solid ${f.source===s?'var(--accent)':'var(--border)'};background:${f.source===s?'var(--accent)':'transparent'};color:${f.source===s?'#fff':'var(--text-muted)'};font-size:11px;cursor:pointer;text-transform:uppercase;">
            ${s === 'all' ? 'All' : s} ${s !== 'all' ? '('+srcCounts[s]+')' : ''}
          </button>
        `).join('')}
      </div>
    </div>

    <div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:20px;padding-bottom:12px;border-bottom:1px solid var(--border);">
      ${allDivs.map(d => {
        const label = d === 'all' ? 'All Divisions' : d;
        const icon = divIcons[d] || '🤖';
        const count = d === 'all' ? allAgents.length : (divCounts[d] || 0);
        const active = f.division === d || (f.division === 'all' && d === 'all');
        return `
          <button onclick="window._agentFilter.division='${d}';renderAgentCatalog(document.getElementById('page-content'), state.agents, state.agentDivisions || [])"
            style="padding:5px 12px;border-radius:20px;border:1px solid ${active?'var(--accent)':'var(--border)'};background:${active?'var(--accent)':'transparent'};color:${active?'#fff':'var(--text-muted)'};font-size:11px;cursor:pointer;white-space:nowrap;transition:all .15s;">
            ${d === 'all' ? '🌐' : icon} ${label} <span style="opacity:.7">(${count})</span>
          </button>
        `;
      }).join('')}
    </div>

    ${buildLeaderboard(allAgents)}

    ${filtered.length === 0 ? `
      <div class="empty-state">
        <div class="empty-icon">🔍</div>
        <div class="empty-text">No agents match your filters. Try broadening your search.</div>
      </div>
    ` : Object.entries(groups).map(([div, agents]) => `
      <div style="margin-bottom:28px;">
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:12px;">
          <span style="font-size:18px;">${divIcons[div] || '🤖'}</span>
          <span style="font-size:14px;font-weight:600;color:var(--text);text-transform:uppercase;letter-spacing:.5px;">${div}</span>
          <span style="font-size:11px;color:var(--text-muted);background:var(--card-bg);padding:2px 8px;border-radius:10px;">${agents.length}</span>
        </div>
        <div class="card-grid">
          ${agents.map(a => {
            const srcColor = a.source === 'core' ? 'var(--success)' : a.source === 'focus' ? 'var(--accent)' : '#8b5cf6';
            // Truncate description to keep cards uniform.
            const rawDesc = a.personality || a.description || '';
            const desc = rawDesc.length > 120 ? rawDesc.substring(0, 120) + '...' : rawDesc;
            return `
              <div class="agent-card" onclick="openAgentDetail('${a.name}')" style="cursor:pointer;border-left:3px solid ${srcColor};height:180px;display:flex;flex-direction:column;overflow:hidden;transition:transform .15s,box-shadow .15s;" onmouseenter="this.style.transform='translateY(-2px)';this.style.boxShadow='0 4px 12px rgba(0,0,0,.2)'" onmouseleave="this.style.transform='';this.style.boxShadow=''">
                <div class="agent-card-head" style="display:flex;align-items:center;gap:6px;">
                  <span class="agent-card-name">${capitalize(a.name)}</span>
                  <span class="badge" style="font-size:9px;padding:1px 6px;background:${srcColor};color:#fff;">${a.source || 'agent'}</span>
                  ${(state.agentCallCounts[a.name] || 0) > 0 ? `<span style="font-size:9px;padding:1px 6px;background:#e85d04;color:#fff;border-radius:10px;font-weight:600;margin-left:2px;" title="Times called">${state.agentCallCounts[a.name]}x</span>` : ''}
                </div>
                <div style="font-size:11px;color:var(--accent);margin-bottom:4px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">${a.role || ''}</div>
                <div class="agent-card-desc" style="font-size:11px;flex:1;overflow:hidden;line-height:1.4;color:var(--text-muted);">${desc}</div>
                <div class="tool-tags" style="margin-top:auto;padding-top:6px;">
                  ${(a.tools || []).slice(0, 4).map(t => '<span class="tool-tag">'+t+'</span>').join('')}
                  ${(a.tools || []).length > 4 ? '<span class="tool-tag">+'+(a.tools.length - 4)+'</span>' : ''}
                </div>
              </div>
            `;
          }).join('')}
        </div>
      </div>
    `).join('')}
  `;
}

function agentCard(a) {
  const badgeClass = getBadgeClass(a.role);
  const rawDesc = a.personality || a.description || 'No description';
  const desc = rawDesc.length > 100 ? rawDesc.substring(0, 100) + '...' : rawDesc;
  return `
    <div class="agent-card" onclick="startChatWith('${a.name}')" style="height:140px;display:flex;flex-direction:column;overflow:hidden;">
      <div class="agent-card-head">
        <span class="agent-card-name">${capitalize(a.name)}</span>
        <span class="badge ${badgeClass}">${a.role || 'general'}</span>
      </div>
      <div class="agent-card-desc" style="flex:1;overflow:hidden;line-height:1.4;">${desc}</div>
    </div>
  `;
}

function agentCardFull(a) {
  const badgeClass = getBadgeClass(a.role);
  const tools = (a.tools || []).slice(0, 5).map(t =>
    `<span class="tool-tag">${t}</span>`
  ).join('');
  const more = (a.tools || []).length > 5 ? `<span class="tool-tag">+${a.tools.length - 5}</span>` : '';
  const rawDesc = a.personality || a.description || 'No description';
  const desc = rawDesc.length > 120 ? rawDesc.substring(0, 120) + '...' : rawDesc;

  return `
    <div class="agent-card" onclick="startChatWith('${a.name}')" style="height:180px;display:flex;flex-direction:column;overflow:hidden;">
      <div class="agent-card-head">
        <span class="agent-card-name">${capitalize(a.name)}</span>
        <span class="badge ${badgeClass}">${a.role || 'general'}</span>
      </div>
      <div class="agent-card-desc" style="flex:1;overflow:hidden;line-height:1.4;margin-bottom:4px;">${desc}</div>
      <div style="font-size:11px;color:var(--text-muted);font-family:var(--mono);">
        Model: ${a.model || 'default'} | Max loops: ${a.max_loops || '?'}
      </div>
      <div class="tool-tags" style="margin-top:auto;padding-top:4px;">${tools}${more}</div>
    </div>
  `;
}

function startChatWith(name) {
  state.chatAgent = name;
  navigate('chat');
}

// ── Agent Detail Panel ────────────────────────────────────────────
function openAgentDetail(name) {
  const a = (state.agents || []).find(x => x.name === name);
  if (!a) return;

  const srcColor = a.source === 'core' ? 'var(--success)' : a.source === 'focus' ? 'var(--accent)' : '#8b5cf6';
  const isCore = a.source === 'core';
  const fullDesc = a.personality || a.description || 'No description available.';
  const allTools = a.tools || [];
  const allGoals = a.goals || [];
  const calls = state.agentCallCounts[a.name] || 0;

  // Autonomy level label mapping.
  const autonomyLabels = {
    0: 'Supervised (asks before every action)',
    1: 'Cautious (asks before destructive actions)',
    2: 'Balanced (default, acts independently on safe ops)',
    3: 'Autonomous (acts independently, reports after)',
    4: 'Autopilot (fully autonomous, minimal reporting)',
  };
  const autonomyLevel = a.autonomy !== undefined ? a.autonomy : 2;
  const autonomyLabel = autonomyLabels[autonomyLevel] || `Level ${autonomyLevel}`;
  const autonomyColor = autonomyLevel <= 1 ? '#22c55e' : autonomyLevel <= 2 ? 'var(--accent)' : autonomyLevel <= 3 ? '#f59e0b' : '#ef4444';

  const container = document.getElementById('page-content');
  container.innerHTML = `
    <div style="max-width:900px;margin:0 auto;">
      <!-- Back button -->
      <button onclick="navigate('agents')" style="display:flex;align-items:center;gap:6px;background:none;border:none;color:var(--text-muted);cursor:pointer;font-size:13px;margin-bottom:16px;padding:0;font-family:var(--font);" onmouseenter="this.style.color='var(--accent)'" onmouseleave="this.style.color='var(--text-muted)'">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="15 18 9 12 15 6"/></svg>
        Back to Agents
      </button>

      <!-- Agent Header -->
      <div style="background:var(--card-bg);border:1px solid var(--border);border-radius:var(--radius);padding:24px;border-left:4px solid ${srcColor};margin-bottom:20px;">
        <div style="display:flex;align-items:center;gap:12px;margin-bottom:16px;">
          <div style="width:48px;height:48px;border-radius:12px;background:linear-gradient(135deg,${srcColor},rgba(0,0,0,0.3));display:flex;align-items:center;justify-content:center;font-size:24px;flex-shrink:0;">
            ${a.source === 'core' ? '\u2699\ufe0f' : '\ud83e\udd16'}
          </div>
          <div style="flex:1;">
            <div style="font-size:20px;font-weight:700;color:var(--text-primary);">${capitalize(a.name)}</div>
            <div style="font-size:12px;color:var(--accent);margin-top:2px;">${a.role || 'Agent'}</div>
          </div>
          <span class="badge" style="font-size:10px;padding:3px 10px;background:${srcColor};color:#fff;border-radius:12px;">${a.source || 'agent'}</span>
          ${calls > 0 ? `<span style="font-size:11px;padding:3px 10px;background:#e85d04;color:#fff;border-radius:12px;font-weight:600;">${calls}x called</span>` : ''}
        </div>

        <!-- Action Buttons -->
        <div style="display:flex;gap:8px;flex-wrap:wrap;">
          <button class="btn btn-primary" onclick="startChatWith('${a.name}')" style="font-size:13px;">
            \ud83d\udcac Chat with ${capitalize(a.name)}
          </button>
          ${!isCore ? `
            <button class="btn" onclick="editAgent('${a.name}')" style="font-size:13px;">
              \u270f\ufe0f Edit
            </button>
            <button class="btn" id="agent-delete-btn" onclick="confirmDeleteAgent('${a.name}')" style="font-size:13px;color:var(--red);border-color:var(--red);">
              \ud83d\uddd1\ufe0f Delete
            </button>
          ` : ''}
        </div>
        <div id="agent-delete-confirm" style="display:none;margin-top:12px;padding:12px;background:rgba(255,50,50,0.08);border:1px solid var(--red);border-radius:var(--radius);">
          <div style="font-size:13px;color:var(--red);font-weight:600;margin-bottom:8px;">Are you sure you want to delete "${capitalize(a.name)}"?</div>
          <div style="font-size:11px;color:var(--text-muted);margin-bottom:8px;">This action cannot be undone. You can reinstall from the Marketplace later.</div>
          <div style="display:flex;gap:8px;">
            <button class="btn" onclick="deleteAgentConfirmed('${a.name}')" style="background:var(--red);color:#fff;border:none;font-size:12px;">Yes, Delete</button>
            <button class="btn" onclick="document.getElementById('agent-delete-confirm').style.display='none'" style="font-size:12px;">Cancel</button>
          </div>
        </div>
      </div>

      <!-- Configuration Overview -->
      <div style="background:var(--card-bg);border:1px solid var(--border);border-radius:var(--radius);padding:20px;margin-bottom:20px;">
        <div style="font-size:13px;font-weight:600;color:var(--text-primary);margin-bottom:16px;text-transform:uppercase;letter-spacing:.5px;">\u2699\ufe0f Configuration</div>
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;">
          <div style="padding:12px;background:var(--bg-input);border-radius:8px;">
            <div style="font-size:10px;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;margin-bottom:4px;">Model</div>
            <div style="font-size:13px;color:var(--text-primary);font-family:var(--mono);">${a.model || 'default (orchestrator)'}</div>
          </div>
          <div style="padding:12px;background:var(--bg-input);border-radius:8px;">
            <div style="font-size:10px;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;margin-bottom:4px;">Max Loops</div>
            <div style="font-size:13px;color:var(--text-primary);font-family:var(--mono);">${a.max_loops || 'N/A'}</div>
          </div>
          <div style="padding:12px;background:var(--bg-input);border-radius:8px;">
            <div style="font-size:10px;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;margin-bottom:4px;">Division</div>
            <div style="font-size:13px;color:var(--text-primary);">${a.division || 'General'}</div>
          </div>
          <div style="padding:12px;background:var(--bg-input);border-radius:8px;">
            <div style="font-size:10px;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;margin-bottom:4px;">Source</div>
            <div style="font-size:13px;color:var(--text-primary);">${a.source || 'custom'}</div>
          </div>
          <div style="padding:12px;background:var(--bg-input);border-radius:8px;grid-column:span 2;">
            <div style="font-size:10px;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;margin-bottom:4px;">Autonomy Level</div>
            <div style="display:flex;align-items:center;gap:8px;">
              <div style="width:28px;height:28px;border-radius:50%;background:${autonomyColor};display:flex;align-items:center;justify-content:center;font-size:14px;font-weight:700;color:#fff;">${autonomyLevel}</div>
              <div style="font-size:13px;color:var(--text-primary);">${autonomyLabel}</div>
            </div>
          </div>
        </div>
      </div>

      <!-- Personality / System Prompt -->
      <div style="background:var(--card-bg);border:1px solid var(--border);border-radius:var(--radius);padding:20px;margin-bottom:20px;">
        <div style="font-size:13px;font-weight:600;color:var(--text-primary);margin-bottom:12px;text-transform:uppercase;letter-spacing:.5px;">\ud83e\udde0 Personality / System Prompt</div>
        <div style="font-size:12px;color:var(--text-secondary);line-height:1.8;white-space:pre-wrap;background:var(--bg-input);padding:16px;border-radius:8px;border-left:3px solid ${srcColor};max-height:300px;overflow-y:auto;">${escapeHtml(fullDesc)}</div>
      </div>

      <!-- Goals -->
      ${allGoals.length > 0 ? `
      <div style="background:var(--card-bg);border:1px solid var(--border);border-radius:var(--radius);padding:20px;margin-bottom:20px;">
        <div style="font-size:13px;font-weight:600;color:var(--text-primary);margin-bottom:12px;text-transform:uppercase;letter-spacing:.5px;">\ud83c\udfaf Goals (${allGoals.length})</div>
        <div style="display:flex;flex-wrap:wrap;gap:8px;">
          ${allGoals.map(g => `<span style="font-size:11px;padding:6px 14px;background:linear-gradient(135deg,rgba(34,197,94,0.12),rgba(34,197,94,0.04));border:1px solid rgba(34,197,94,0.3);color:#22c55e;border-radius:20px;font-weight:500;">${g.replace(/_/g,' ')}</span>`).join('')}
        </div>
      </div>
      ` : ''}

      <!-- Tools -->
      <div style="background:var(--card-bg);border:1px solid var(--border);border-radius:var(--radius);padding:20px;margin-bottom:20px;">
        <div style="font-size:13px;font-weight:600;color:var(--text-primary);margin-bottom:12px;text-transform:uppercase;letter-spacing:.5px;">\ud83d\udee0\ufe0f Tools (${allTools.length})</div>
        ${allTools.length > 0 ? `
          <div style="display:flex;flex-wrap:wrap;gap:6px;">
            ${allTools.map(t => `<span class="tool-tag" style="font-size:11px;padding:4px 10px;">${t}</span>`).join('')}
          </div>
        ` : '<div style="font-size:12px;color:var(--text-muted);">No tools configured</div>'}
      </div>

      <!-- Knowledge Base -->
      <div style="background:var(--card-bg);border:1px solid var(--border);border-radius:var(--radius);padding:20px;margin-bottom:20px;">
        <div style="font-size:13px;font-weight:600;color:var(--text-primary);margin-bottom:12px;text-transform:uppercase;letter-spacing:.5px;">\ud83d\udcda Knowledge Base</div>
        <div style="font-size:11px;color:var(--text-muted);margin-bottom:12px;">Upload documents or files that only this agent can reference.</div>

        <div style="border:2px dashed var(--border);border-radius:var(--radius);padding:20px;text-align:center;cursor:pointer;transition:all .2s;margin-bottom:12px;"
             ondragover="event.preventDefault();this.style.borderColor='var(--accent)';this.style.background='rgba(232,93,4,0.05)'"
             ondragleave="this.style.borderColor='var(--border)';this.style.background='transparent'"
             ondrop="handleAgentKnowledgeDrop(event,'${a.name}')"
             onclick="document.getElementById('agent-knowledge-input').click()">
          <div style="font-size:20px;margin-bottom:4px;">\ud83d\udcc1</div>
          <div style="font-size:12px;color:var(--text-muted);">Drop files here or click to browse</div>
          <div style="font-size:10px;color:var(--text-muted);margin-top:2px;">PDF, TXT, MD, JSON, CSV, Images (max 32MB)</div>
        </div>
        <input type="file" id="agent-knowledge-input" accept=".pdf,.txt,.md,.json,.csv,.doc,.docx,.png,.jpg,.jpeg,.gif,.svg,.webp" multiple style="display:none;"
          onchange="handleAgentKnowledgeUpload(this.files,'${a.name}')">
        <div id="agent-knowledge-list" style="display:flex;flex-direction:column;gap:4px;"></div>
      </div>

      <!-- Link & YouTube Scraping -->
      <div style="background:var(--card-bg);border:1px solid var(--border);border-radius:var(--radius);padding:20px;margin-bottom:20px;">
        <div style="font-size:13px;font-weight:600;color:var(--text-primary);margin-bottom:12px;text-transform:uppercase;letter-spacing:.5px;">\ud83c\udf10 Add Knowledge from URL</div>
        <div style="font-size:11px;color:var(--text-muted);margin-bottom:12px;">Paste a web page URL or YouTube link to scrape content and add to this agent's knowledge.</div>

        <div style="display:flex;gap:8px;margin-bottom:8px;">
          <input type="text" id="agent-url-input" placeholder="https://example.com or YouTube URL..."
            style="flex:1;padding:8px 12px;border-radius:6px;border:1px solid var(--border);background:var(--bg-input);color:var(--text-primary);font-size:13px;outline:none;">
          <button class="btn btn-primary" onclick="scrapeUrlForAgent('${a.name}')" style="flex-shrink:0;font-size:12px;">
            \ud83d\udd0d Scrape
          </button>
        </div>
        <div id="agent-scrape-status" style="font-size:11px;color:var(--text-muted);font-family:var(--mono);"></div>
      </div>
    </div>
  `;
}

function confirmDeleteAgent(name) {
  const confirm = document.getElementById('agent-delete-confirm');
  if (confirm) confirm.style.display = 'block';
}

async function deleteAgentConfirmed(name) {
  const res = await api('/v1/personas/' + name, { method: 'DELETE' });
  if (res && !res.error) {
    navigate('agents');
  } else {
    alert(res?.error || 'Failed to delete agent');
  }
}

// Knowledge upload handler.
async function handleAgentKnowledgeUpload(files, agentName) {
  if (!files || files.length === 0) return;
  const list = document.getElementById('agent-knowledge-list');
  for (const file of files) {
    const reader = new FileReader();
    reader.onload = async (e) => {
      const entry = document.createElement('div');
      entry.style.cssText = 'display:flex;align-items:center;gap:8px;padding:6px 10px;background:var(--bg-input);border-radius:6px;font-size:12px;color:var(--text-primary);';
      entry.innerHTML = `\ud83d\udcc4 ${file.name} <span style="font-size:10px;color:var(--text-muted);margin-left:auto;">${(file.size / 1024).toFixed(1)}KB</span> <span style="color:var(--success);font-size:10px;">Uploaded</span>`;
      if (list) list.appendChild(entry);

      // POST to agent knowledge endpoint (backend handles storage).
      await api('/v1/agents/' + agentName + '/knowledge', {
        method: 'POST',
        body: JSON.stringify({
          filename: file.name,
          content: e.target.result,
          mime_type: file.type,
        }),
      });
    };
    reader.readAsDataURL(file);
  }
}

function handleAgentKnowledgeDrop(event, agentName) {
  event.preventDefault();
  event.currentTarget.style.borderColor = 'var(--border)';
  event.currentTarget.style.background = 'transparent';
  const files = event.dataTransfer?.files;
  if (files) handleAgentKnowledgeUpload(files, agentName);
}

// URL/YouTube scraping handler.
async function scrapeUrlForAgent(agentName) {
  const input = document.getElementById('agent-url-input');
  const status = document.getElementById('agent-scrape-status');
  if (!input || !input.value.trim()) return;

  const url = input.value.trim();
  if (status) status.textContent = 'Scraping ' + url + '...';

  const isYouTube = url.includes('youtube.com') || url.includes('youtu.be');

  const res = await api('/v1/agents/' + agentName + '/knowledge/scrape', {
    method: 'POST',
    body: JSON.stringify({ url, type: isYouTube ? 'youtube' : 'webpage' }),
  });

  if (res && !res.error) {
    if (status) {
      status.innerHTML = `<span style="color:var(--success);">\u2713 ${isYouTube ? 'Transcript' : 'Page content'} scraped and added to knowledge base.</span>`;
    }
    input.value = '';
  } else {
    if (status) {
      status.innerHTML = `<span style="color:var(--red);">\u2717 ${res?.error || 'Failed to scrape URL'}</span>`;
    }
  }
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
const priorityColors = { 0: 'var(--text-muted)', 1: 'var(--info)', 2: 'var(--warning)', 3: 'var(--red)' };
const priorityNames = { 0: 'Low', 1: 'Medium', 2: 'High', 3: 'Urgent' };
const priorityIcons = { 0: '◇', 1: '◆', 2: '▲', 3: '🔥' };
const labelColors = ['#e85d04','#2196F3','#4CAF50','#9C27B0','#FF9800','#00BCD4','#E91E63','#607D8B'];

function timeAgo(dateStr) {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 60) return `${mins}m`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h`;
  const days = Math.floor(hrs / 24);
  return `${days}d`;
}

function isOverdue(task) {
  return task.due_date && new Date(task.due_date) < Date.now() && task.status !== 'Done';
}

async function fetchProjects() {
  const data = await api('/v1/projects');
  if (data && data.projects) state.projects = data.projects;
}

async function renderTasks(container) {
  if (!container) container = document.getElementById('page-content');
  if (!state.tasks || state.tasks.length === 0) {
    await fetchTasks();
  }
  if (!state.projects || state.projects.length === 0) {
    await fetchProjects();
  }

  const columns = [
    { id: 'Todo', title: 'Todo', icon: '📋' },
    { id: 'In Progress', title: 'In Progress', icon: '⚡' },
    { id: 'Waiting', title: 'Waiting', icon: '⏳' },
    { id: 'Blocked', title: 'Blocked', icon: '🚫' },
    { id: 'Review', title: 'Review', icon: '👁' },
    { id: 'Paused', title: 'Paused', icon: '⏸' },
    { id: 'Escalated', title: 'Escalated', icon: '🚨' },
    { id: 'Failed', title: 'Failed', icon: '💀' },
    { id: 'Done', title: 'Done', icon: '✅' }
  ];

  // Stats bar
  const total = state.tasks.length;
  const done = state.tasks.filter(t => t.status === 'Done').length;
  const overdue = state.tasks.filter(t => isOverdue(t)).length;
  const urgent = state.tasks.filter(t => (t.priority || 0) >= 3 && t.status !== 'Done').length;

  // Collect unique labels + agents for filter chips
  const allLabels = [...new Set(state.tasks.flatMap(t => t.labels || []))];
  const allAgents = [...new Set(state.tasks.map(t => t.assigned_agent).filter(Boolean))];

  // Apply project filter first
  let filtered = state.tasks;
  if (state.selectedProjectId) {
    filtered = filtered.filter(t => t.project_id === state.selectedProjectId);
  }

  // Apply filters
  // (filtered already declared above from project filter)
  if (state.taskSearch) {
    const q = state.taskSearch.toLowerCase();
    filtered = filtered.filter(t =>
      t.title.toLowerCase().includes(q) ||
      (t.description || '').toLowerCase().includes(q)
    );
  }
  if (state.taskFilterPriority >= 0) {
    filtered = filtered.filter(t => (t.priority || 0) === state.taskFilterPriority);
  }
  if (state.taskFilterLabel) {
    filtered = filtered.filter(t => (t.labels || []).includes(state.taskFilterLabel));
  }
  if (state.taskFilterAgent) {
    filtered = filtered.filter(t => t.assigned_agent === state.taskFilterAgent);
  }

  container.innerHTML = `
    ${state.projects.length > 0 ? `
    <div class="project-tabs" style="display:flex;gap:4px;margin-bottom:10px;border-bottom:1px solid var(--border);padding-bottom:8px;flex-wrap:wrap;align-items:center;">
      <button class="btn ${!state.selectedProjectId ? 'btn-primary' : ''}" style="font-size:11px;padding:4px 12px;border-radius:var(--radius-sm) var(--radius-sm) 0 0;" onclick="state.selectedProjectId='';renderTasks()">📊 All Tasks</button>
      ${(() => {
        const activeProjects = state.projects.filter(p => p.status === 'active');
        const grouped = {};
        activeProjects.forEach(p => {
          const group = p.agency_name || 'General';
          if (!grouped[group]) grouped[group] = [];
          grouped[group].push(p);
        });
        return Object.entries(grouped).map(([agency, projects]) => `
          ${Object.keys(grouped).length > 1 ? `<span style="font-size:9px;color:var(--text-muted);font-weight:600;margin-left:6px;">${escapeHtml(agency)}:</span>` : ''}
          ${projects.map(p => `
            <button class="btn ${state.selectedProjectId === String(p.id) ? 'btn-primary' : ''}" style="font-size:11px;padding:4px 12px;border-radius:var(--radius-sm) var(--radius-sm) 0 0;position:relative;" onclick="state.selectedProjectId='${p.id}';renderTasks()">
              📁 ${escapeHtml(p.name)}
              <span style="font-size:8px;padding:1px 4px;border-radius:3px;margin-left:4px;background:${p.auto_mode ? 'var(--green)' : 'var(--text-muted)'};color:#000;font-weight:700;">${p.auto_mode ? 'AI' : 'Manual'}</span>
            </button>
          `).join('')}
        `).join('');
      })()}
    </div>
    ` : ''}
    <div class="kanban-toolbar" style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-bottom:8px;">
      <button class="btn btn-primary" onclick="openTaskModal()" title="Shortcut: N">+ New Task</button>
      <button class="btn" onclick="fetchTasks().then(() => renderTasks())" title="Shortcut: R">Refresh</button>
      <div style="display:flex;gap:2px;margin-right:4px;border:1px solid var(--border);border-radius:var(--radius-sm);overflow:hidden;">
        <button class="btn ${state.taskViewMode==='board'?'btn-primary':''}" onclick="state.taskViewMode='board';renderTasks()" title="Shortcut: V" style="padding:4px 10px;font-size:11px;border-radius:0;">Board</button>
        <button class="btn ${state.taskViewMode==='table'?'btn-primary':''}" onclick="state.taskViewMode='table';renderTasks()" title="Shortcut: V" style="padding:4px 10px;font-size:11px;border-radius:0;">Table</button>
      </div>
      ${state.selectedProjectId ? `
        <button class="btn" onclick="toggleProjectAutoMode('${state.selectedProjectId}')" title="Toggle AI/Manual mode" style="font-size:11px;padding:4px 10px;">
          ${(state.projects.find(p => String(p.id) === state.selectedProjectId) || {}).auto_mode ? '🤖 AI Mode' : '👤 Manual'}
        </button>
      ` : ''}
      <button class="btn" onclick="openApprovalPanel()" title="Pending approvals" style="font-size:11px;padding:4px 10px;position:relative;">
        Approvals${state.pendingApprovals > 0 ? ` <span style="background:var(--red);color:#fff;font-size:9px;padding:1px 5px;border-radius:8px;margin-left:3px;">${state.pendingApprovals}</span>` : ''}
      </button>
      <div style="flex:1;min-width:180px;max-width:320px;position:relative;">
        <input id="task-search" type="text" placeholder="Search tasks... (F)" value="${escapeHtml(state.taskSearch)}" oninput="state.taskSearch=this.value;renderTasks()" style="width:100%;padding:5px 10px 5px 28px;background:var(--bg-input);border:1px solid var(--border);border-radius:var(--radius-sm);color:var(--text-primary);font-size:12px;">
        <span style="position:absolute;left:8px;top:50%;transform:translateY(-50%);font-size:12px;color:var(--text-muted);">🔍</span>
      </div>
      <select onchange="state.taskSort=this.value;renderTasks()" style="padding:4px 8px;background:var(--bg-input);border:1px solid var(--border);border-radius:var(--radius-sm);color:var(--text-primary);font-size:11px;cursor:pointer;">
        <option value="priority" ${state.taskSort==='priority'?'selected':''}>Sort: Priority</option>
        <option value="date" ${state.taskSort==='date'?'selected':''}>Sort: Date</option>
        <option value="due" ${state.taskSort==='due'?'selected':''}>Sort: Due Date</option>
        <option value="agent" ${state.taskSort==='agent'?'selected':''}>Sort: Agent</option>
      </select>
      <div style="margin-left:auto;display:flex;gap:10px;font-size:11px;color:var(--text-muted);">
        <span>${total} total</span>
        <span style="color:var(--green)">${done} done</span>
        ${overdue > 0 ? `<span style="color:var(--red)">⚠ ${overdue} overdue</span>` : ''}
        ${urgent > 0 ? `<span style="color:var(--red)">🔥 ${urgent} urgent</span>` : ''}
      </div>
    </div>
    <div class="kanban-filters" style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:10px;align-items:center;">
      <span style="font-size:10px;color:var(--text-muted);font-weight:600;">Priority:</span>
      <button class="btn ${state.taskFilterPriority < 0 ? 'btn-primary' : ''}" style="font-size:10px;padding:2px 8px;" onclick="state.taskFilterPriority=-1;renderTasks()">All</button>
      ${[0,1,2,3].map(p => `<button class="btn ${state.taskFilterPriority===p?'btn-primary':''}" style="font-size:10px;padding:2px 8px;border-left:3px solid ${priorityColors[p]};" onclick="state.taskFilterPriority=${p};renderTasks()">${priorityIcons[p]} ${priorityNames[p]}</button>`).join('')}
      ${allLabels.length ? '<span style="font-size:10px;color:var(--text-muted);font-weight:600;margin-left:6px;">Label:</span>' : ''}
      ${allLabels.map((l,i) => `<button class="btn ${state.taskFilterLabel===l?'btn-primary':''}" style="font-size:10px;padding:2px 8px;" onclick="state.taskFilterLabel=state.taskFilterLabel==='${escapeHtml(l)}'?'':'${escapeHtml(l)}';renderTasks()">${escapeHtml(l)}</button>`).join('')}
      ${allAgents.length ? '<span style="font-size:10px;color:var(--text-muted);font-weight:600;margin-left:6px;">Agent:</span>' : ''}
      ${allAgents.map(a => `<button class="btn ${state.taskFilterAgent===a?'btn-primary':''}" style="font-size:10px;padding:2px 8px;" onclick="state.taskFilterAgent=state.taskFilterAgent==='${escapeHtml(a)}'?'':'${escapeHtml(a)}';renderTasks()">🤖 ${escapeHtml(a)}</button>`).join('')}
      ${(state.taskSearch || state.taskFilterPriority >= 0 || state.taskFilterLabel || state.taskFilterAgent) ? `<button class="btn" style="font-size:10px;padding:2px 8px;color:var(--red);margin-left:6px;" onclick="state.taskSearch='';state.taskFilterPriority=-1;state.taskFilterLabel='';state.taskFilterAgent='';renderTasks()">Clear Filters</button>` : ''}
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
    ${state.taskViewMode === 'table' ? renderTaskTable(filtered, columns) : renderKanbanBoard(filtered, columns)}
  `;
}

function sortFilteredTasks(filtered) {
  return [...filtered].sort((a,b) => {
    if (state.taskSort === 'priority') {
      const pa = a.priority || 0, pb = b.priority || 0;
      if (pb !== pa) return pb - pa;
      return new Date(b.created_at) - new Date(a.created_at);
    } else if (state.taskSort === 'date') {
      return new Date(b.created_at) - new Date(a.created_at);
    } else if (state.taskSort === 'due') {
      const da = a.due_date ? new Date(a.due_date).getTime() : Infinity;
      const db = b.due_date ? new Date(b.due_date).getTime() : Infinity;
      return da - db;
    } else if (state.taskSort === 'agent') {
      return (a.assigned_agent || 'zzz').localeCompare(b.assigned_agent || 'zzz');
    }
    return 0;
  });
}

function renderTaskTable(filtered, columns) {
  const sorted = sortFilteredTasks(filtered);
  return `
    <div style="overflow-x:auto;">
    <table style="width:100%;border-collapse:collapse;font-size:12px;">
      <thead>
        <tr style="border-bottom:2px solid var(--border);text-align:left;">
          <th style="padding:8px 6px;color:var(--text-muted);font-weight:600;">Priority</th>
          <th style="padding:8px 6px;color:var(--text-muted);font-weight:600;">Title</th>
          <th style="padding:8px 6px;color:var(--text-muted);font-weight:600;">Status</th>
          <th style="padding:8px 6px;color:var(--text-muted);font-weight:600;">Agent</th>
          <th style="padding:8px 6px;color:var(--text-muted);font-weight:600;">Labels</th>
          <th style="padding:8px 6px;color:var(--text-muted);font-weight:600;">Due</th>
          <th style="padding:8px 6px;color:var(--text-muted);font-weight:600;">Age</th>
          <th style="padding:8px 6px;color:var(--text-muted);font-weight:600;"></th>
        </tr>
      </thead>
      <tbody>
        ${sorted.map(t => {
          const p = t.priority || 0;
          const over = isOverdue(t);
          const blockerCount = (t.blocked_by || []).length;
          return `
          <tr style="border-bottom:1px solid var(--border);cursor:pointer;" onmouseover="this.style.background='var(--bg-hover)'" onmouseout="this.style.background=''" onclick="openTaskDetail('${t.id}')">
            <td style="padding:6px;color:${priorityColors[p]};">${priorityIcons[p]} ${priorityNames[p]}</td>
            <td style="padding:6px;color:var(--text-primary);font-weight:500;">
              ${blockerCount > 0 ? `<span title="Blocked by ${blockerCount} task(s)" style="color:var(--red);margin-right:4px;">⛓${blockerCount}</span>` : ''}
              ${escapeHtml(t.title)}
              ${(t.comments || []).length > 0 ? `<span style="color:var(--text-muted);font-size:10px;margin-left:4px;">💬${t.comments.length}</span>` : ''}
            </td>
            <td style="padding:6px;"><span style="padding:2px 8px;border-radius:9px;font-size:10px;background:var(--bg-input);color:var(--text-primary);">${t.status}</span></td>
            <td style="padding:6px;color:var(--text-muted);">${t.assigned_agent ? '🤖 '+escapeHtml(t.assigned_agent) : '<span style="opacity:.4">Unassigned</span>'}</td>
            <td style="padding:6px;">${(t.labels||[]).map((l,i) => `<span style="display:inline-block;padding:1px 5px;border-radius:9px;font-size:9px;background:${labelColors[i%labelColors.length]}22;color:${labelColors[i%labelColors.length]};margin-right:2px;">${escapeHtml(l)}</span>`).join('')}</td>
            <td style="padding:6px;color:${over?'var(--red)':'var(--text-muted)'};font-size:11px;">${t.due_date ? new Date(t.due_date).toLocaleDateString('en-US',{month:'short',day:'numeric'}) : ''}</td>
            <td style="padding:6px;color:var(--text-muted);font-size:11px;">${timeAgo(t.created_at)}</td>
            <td style="padding:6px;"><button class="btn" style="font-size:10px;padding:2px 6px;" onclick="event.stopPropagation();openEditTaskModal('${t.id}')">Edit</button></td>
          </tr>`;
        }).join('')}
      </tbody>
    </table>
    </div>
    ${sorted.length === 0 ? '<div style="padding:24px;text-align:center;color:var(--text-muted);">No tasks match the current filters</div>' : ''}
  `;
}

function renderKanbanBoard(filtered, columns) {
  return `
    <div class="kanban-board">
      ${columns.map(col => {
        const colTasks = filtered.filter(t => t.status === col.id).sort((a,b) => {
          if (state.taskSort === 'priority') {
            const pa = a.priority || 0, pb = b.priority || 0;
            if (pb !== pa) return pb - pa;
            return new Date(b.created_at) - new Date(a.created_at);
          } else if (state.taskSort === 'date') {
            return new Date(b.created_at) - new Date(a.created_at);
          } else if (state.taskSort === 'due') {
            const da = a.due_date ? new Date(a.due_date).getTime() : Infinity;
            const db = b.due_date ? new Date(b.due_date).getTime() : Infinity;
            return da - db;
          } else if (state.taskSort === 'agent') {
            return (a.assigned_agent || 'zzz').localeCompare(b.assigned_agent || 'zzz');
          }
          return 0;
        });
        return `
        <div class="kanban-column" data-status="${col.id}" ondragover="allowDrop(event)" ondrop="drop(event)" ondragleave="dragLeave(event)">
          <div class="kanban-column-header">
            <h3>${col.icon} ${col.title}</h3>
            <span class="kanban-badge">${colTasks.length}</span>
          </div>
          <div class="kanban-column-body">
            ${colTasks.map(t => {
              const p = t.priority || 0;
              const borderColor = priorityColors[p];
              const overdueClass = isOverdue(t) ? 'border:1px solid var(--red);' : '';
              const subDone = (t.sub_items || []).filter(s => s.done).length;
              const subTotal = (t.sub_items || []).length;
              const blockerCount = (t.blocked_by || []).length;
              const labels = (t.labels || []).map((l,i) => 
                `<span style="display:inline-block;padding:1px 6px;border-radius:9px;font-size:9px;font-weight:600;background:${labelColors[i % labelColors.length]}22;color:${labelColors[i % labelColors.length]};border:1px solid ${labelColors[i % labelColors.length]}44;">${escapeHtml(l)}</span>`
              ).join('');
              const dueBadge = t.due_date ? (() => {
                const d = new Date(t.due_date);
                const over = isOverdue(t);
                const color = over ? 'var(--red)' : 'var(--text-muted)';
                return `<span style="font-size:10px;color:${color};" title="Due: ${d.toLocaleDateString()}">${over ? '⚠ ' : '📅 '}${d.toLocaleDateString('en-US', {month:'short',day:'numeric'})}</span>`;
              })() : '';

              return `
              <div class="kanban-card" draggable="true" ondragstart="dragStart(event, '${t.id}')" onclick="openTaskDetail('${t.id}')" style="border-left:3px solid ${borderColor};${overdueClass}">
                <div style="display:flex;align-items:center;gap:4px;margin-bottom:4px;">
                  <span style="font-size:11px;color:${borderColor};" title="${priorityNames[p]} priority">${priorityIcons[p]}</span>
                  <div class="task-title" style="flex:1;">${escapeHtml(t.title)}</div>
                  ${blockerCount > 0 ? `<span title="Blocked by ${blockerCount} task(s)" style="font-size:10px;color:var(--red);">⛓${blockerCount}</span>` : ''}
                  ${(t.comments||[]).length > 0 ? `<span style="font-size:10px;color:var(--text-muted);">💬${t.comments.length}</span>` : ''}
                </div>
                ${labels ? `<div style="display:flex;gap:3px;flex-wrap:wrap;margin-bottom:4px;">${labels}</div>` : ''}
                ${t.description ? `<div class="task-desc">${escapeHtml(t.description).substring(0, 80)}${t.description.length > 80 ? '...' : ''}</div>` : ''}
                ${subTotal > 0 ? `
                  <div style="margin:4px 0;">
                    <div style="display:flex;align-items:center;gap:4px;font-size:10px;color:var(--text-muted);margin-bottom:2px;">
                      <span>☑ ${subDone}/${subTotal}</span>
                    </div>
                    <div style="height:3px;background:var(--border);border-radius:2px;overflow:hidden;">
                      <div style="height:100%;width:${subTotal > 0 ? (subDone/subTotal*100) : 0}%;background:var(--green);border-radius:2px;transition:width .3s;"></div>
                    </div>
                  </div>
                ` : ''}
                <div class="task-footer">
                  ${t.assigned_agent ? `<span class="task-agent">${escapeHtml(t.assigned_agent)}</span>` : '<span class="task-agent unassigned">Unassigned</span>'}
                  <div style="display:flex;gap:6px;align-items:center;">
                    ${t.confidence_score ? `<span style="font-size:10px;font-weight:700;padding:1px 5px;border-radius:8px;background:${t.confidence_score >= 80 ? 'var(--green)' : t.confidence_score >= 50 ? '#f59e0b' : 'var(--red)'}22;color:${t.confidence_score >= 80 ? 'var(--green)' : t.confidence_score >= 50 ? '#f59e0b' : 'var(--red)'};" title="Confidence: ${t.confidence_score}%">${t.confidence_score}%</span>` : ''}
                    ${t.parent_task_id ? `<span style="font-size:10px;color:var(--accent);cursor:pointer;" onclick="event.stopPropagation();openTaskDetail('${t.parent_task_id}')" title="Child of parent task">↑Parent</span>` : ''}
                    ${(t.child_task_ids||[]).length > 0 ? `<span style="font-size:10px;color:var(--accent);" title="${t.child_task_ids.length} child task(s)">↓${t.child_task_ids.length}</span>` : ''}
                    ${(t.attachments||[]).length > 0 ? `<span style="font-size:10px;color:var(--text-muted);" title="${t.attachments.length} file(s)">📎${t.attachments.length}</span>` : ''}
                    ${t.review ? `<span style="font-size:10px;color:${t.review.passed ? 'var(--green)' : 'var(--red)'};" title="Review: ${t.review.passed ? 'Passed' : 'Failed'}">${t.review.passed ? '✅' : '❌'}</span>` : ''}
                    ${dueBadge}
                    <span class="task-date" title="Created ${new Date(t.created_at).toLocaleString()}">${timeAgo(t.created_at)}</span>
                    ${t.status === 'Todo' ? `<button onclick="event.stopPropagation();executeTask('${t.id}')" style="background:var(--green);color:#000;border:none;border-radius:3px;padding:2px 8px;font-size:10px;font-weight:700;cursor:pointer;" title="Run this task with AI">Run</button>` : ''}
                    ${t.status === 'Waiting' ? `<button onclick="event.stopPropagation();wakeTask('${t.id}')" style="background:var(--accent);color:#fff;border:none;border-radius:3px;padding:2px 8px;font-size:10px;font-weight:700;cursor:pointer;" title="Wake this task manually">Wake</button>` : ''}
                  </div>
                </div>
              </div>
            `}).join('')}
          </div>
        </div>
      `}).join('')}
    </div>
  `;
}

// ── Task Detail Slide-Out Panel ───────────────────────────────────
function openTaskDetail(taskId) {
  const t = state.tasks.find(x => x.id === taskId);
  if (!t) return;

  // Remove existing panel if any
  closeTaskDetail();

  const p = t.priority || 0;
  const over = isOverdue(t);
  const subDone = (t.sub_items || []).filter(s => s.done).length;
  const subTotal = (t.sub_items || []).length;
  const blockedBy = (t.blocked_by || []).map(bid => {
    const bt = state.tasks.find(x => x.id === bid);
    return bt ? `<span style="color:var(--red);cursor:pointer;" onclick="openTaskDetail('${bid}')" title="${escapeHtml(bt.title)}">&#x26D3; ${escapeHtml(bt.title)}</span>` : bid;
  }).join('<br>');
  const blocking = (t.blocks || []).map(bid => {
    const bt = state.tasks.find(x => x.id === bid);
    return bt ? `<span style="color:var(--warning);cursor:pointer;" onclick="openTaskDetail('${bid}')" title="${escapeHtml(bt.title)}">&#x2192; ${escapeHtml(bt.title)}</span>` : bid;
  }).join('<br>');

  const panel = document.createElement('div');
  panel.id = 'task-detail-panel';
  panel.style.cssText = 'position:fixed;top:0;right:0;width:480px;max-width:90vw;height:100vh;background:var(--bg-card);border-left:1px solid var(--border);box-shadow:-4px 0 24px rgba(0,0,0,.3);z-index:9998;overflow-y:auto;display:flex;flex-direction:column;animation:slideIn .2s ease;';
  
  panel.innerHTML = `
    <style>@keyframes slideIn{from{transform:translateX(100%)}to{transform:translateX(0)}}</style>
    <div style="padding:16px;border-bottom:1px solid var(--border);display:flex;align-items:center;gap:8px;">
      <span style="font-size:14px;color:${priorityColors[p]};">${priorityIcons[p]}</span>
      <h3 style="flex:1;margin:0;color:var(--text-primary);font-size:15px;">${escapeHtml(t.title)}</h3>
      <button class="btn" style="font-size:11px;padding:3px 8px;" onclick="openEditTaskModal('${t.id}');closeTaskDetail()">Edit</button>
      <button class="btn btn-icon" onclick="closeTaskDetail()" style="font-size:16px;padding:4px;">✕</button>
    </div>

    <div style="padding:16px;flex:1;overflow-y:auto;">
      <!-- Metadata -->
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-bottom:16px;font-size:12px;">
        <div><span style="color:var(--text-muted);">Status</span><br><span style="padding:2px 8px;border-radius:9px;background:var(--bg-input);color:var(--text-primary);font-size:11px;">${t.status}</span></div>
        <div><span style="color:var(--text-muted);">Priority</span><br><span style="color:${priorityColors[p]};">${priorityIcons[p]} ${priorityNames[p]}</span></div>
        <div><span style="color:var(--text-muted);">Agent</span><br>${t.assigned_agent ? '&#x1F916; '+escapeHtml(t.assigned_agent) : '<span style="opacity:.4">Unassigned</span>'}</div>
        <div><span style="color:var(--text-muted);">Due</span><br>${t.due_date ? `<span style="color:${over?'var(--red)':'var(--text-primary)'};">${new Date(t.due_date).toLocaleDateString()}</span>` : '<span style="opacity:.4">None</span>'}</div>
        ${t.recur_pattern ? `<div><span style="color:var(--text-muted);">Recurring</span><br>&#x1F501; ${escapeHtml(t.recur_pattern)}</div>` : ''}
      </div>

      <!-- Labels -->
      ${(t.labels||[]).length > 0 ? `<div style="margin-bottom:12px;">${(t.labels||[]).map((l,i) => `<span style="display:inline-block;padding:2px 8px;border-radius:9px;font-size:10px;font-weight:600;background:${labelColors[i%labelColors.length]}22;color:${labelColors[i%labelColors.length]};border:1px solid ${labelColors[i%labelColors.length]}44;margin-right:4px;">${escapeHtml(l)}</span>`).join('')}</div>` : ''}

      <!-- Description -->
      ${t.description ? `<div style="margin-bottom:16px;padding:10px;background:var(--bg-input);border-radius:var(--radius-sm);font-size:12px;color:var(--text-primary);line-height:1.5;">${escapeHtml(t.description)}</div>` : ''}

      <!-- Sub-items -->
      ${subTotal > 0 ? `
        <div style="margin-bottom:16px;">
          <h4 style="margin:0 0 8px;font-size:12px;color:var(--text-muted);">Checklist (${subDone}/${subTotal})</h4>
          <div style="height:4px;background:var(--border);border-radius:2px;margin-bottom:8px;overflow:hidden;">
            <div style="height:100%;width:${(subDone/subTotal*100)}%;background:var(--green);border-radius:2px;"></div>
          </div>
          ${(t.sub_items||[]).map(s => `<div style="display:flex;align-items:center;gap:6px;padding:3px 0;font-size:12px;color:var(--text-primary);"><span style="color:${s.done?'var(--green)':'var(--text-muted)'};">${s.done?'&#x2611;':'&#x2610;'}</span>${escapeHtml(s.title)}</div>`).join('')}
        </div>
      ` : ''}

      <!-- Dependencies -->
      ${(t.blocked_by||[]).length > 0 || (t.blocks||[]).length > 0 ? `
        <div style="margin-bottom:16px;">
          <h4 style="margin:0 0 8px;font-size:12px;color:var(--text-muted);">Dependencies</h4>
          ${blockedBy ? `<div style="font-size:12px;margin-bottom:4px;"><strong style="color:var(--red);">Blocked by:</strong><br>${blockedBy}</div>` : ''}
          ${blocking ? `<div style="font-size:12px;"><strong style="color:var(--warning);">Blocks:</strong><br>${blocking}</div>` : ''}
        </div>
      ` : ''}

      <!-- Auto-Review Results -->
      ${t.review ? `
        <div style="margin-bottom:16px;padding:10px;border-radius:var(--radius);border:1px solid ${t.review.passed ? 'var(--green)' : 'var(--red)'};background:${t.review.passed ? 'rgba(0,200,0,0.05)' : 'rgba(200,0,0,0.05)'};">
          <h4 style="margin:0 0 6px;font-size:12px;display:flex;align-items:center;gap:6px;">
            ${t.review.passed ? '<span style="color:var(--green);">Review Passed</span>' : '<span style="color:var(--red);">Review Failed</span>'}
            <span style="font-size:10px;color:var(--text-muted);font-weight:normal;">${t.review.review_at ? new Date(t.review.review_at).toLocaleString() : ''}</span>
          </h4>
          ${(t.review.issues || []).length > 0 ? `
            <ul style="margin:4px 0 0;padding-left:16px;font-size:11px;color:var(--text-primary);">
              ${t.review.issues.map(issue => `<li style="margin-bottom:2px;">${escapeHtml(issue)}</li>`).join('')}
            </ul>
          ` : ''}
          ${(t.review.files || []).length > 0 ? `
            <div style="font-size:10px;color:var(--text-muted);margin-top:4px;">Files: ${t.review.files.map(f => escapeHtml(f)).join(', ')}</div>
          ` : ''}
        </div>
      ` : ''}

      <!-- Progress Bar -->
      ${t.progress > 0 || t.status === 'In Progress' ? `
        <div style="margin-bottom:16px;">
          <div style="display:flex;justify-content:space-between;margin-bottom:4px;">
            <span style="font-size:11px;color:var(--text-muted);">Progress</span>
            <span style="font-size:11px;font-weight:600;color:var(--text-primary);">${t.progress || 0}%</span>
          </div>
          <div style="height:6px;background:var(--border);border-radius:3px;overflow:hidden;">
            <div style="height:100%;width:${t.progress || 0}%;background:var(--green);border-radius:3px;transition:width .5s;"></div>
          </div>
        </div>
      ` : ''}

      <!-- Agent Config Summary -->
      ${t.agent_config ? `
        <div style="margin-bottom:16px;padding:10px;background:var(--bg-input);border-radius:var(--radius);border:1px solid var(--border);">
          <h4 style="margin:0 0 8px;font-size:12px;color:var(--text-muted);">AI Configuration</h4>
          <div style="display:grid;grid-template-columns:1fr 1fr;gap:6px;font-size:11px;">
            ${t.agent_config.autonomy_level ? `<div><span style="color:var(--text-muted);">Autonomy:</span> <span style="color:var(--text-primary);font-weight:600;">${t.agent_config.autonomy_level}</span></div>` : ''}
            ${t.agent_config.max_retries ? `<div><span style="color:var(--text-muted);">Retries:</span> ${t.retry_count || 0}/${t.agent_config.max_retries}</div>` : ''}
            ${t.agent_config.max_tokens ? `<div><span style="color:var(--text-muted);">Tokens:</span> <span style="${t.tokens_used >= t.agent_config.max_tokens ? 'color:var(--red);font-weight:700' : ''}">${t.tokens_used || 0}/${t.agent_config.max_tokens}</span></div>` : ''}
            ${t.agent_config.ttl_seconds ? `<div><span style="color:var(--text-muted);">TTL:</span> ${t.wall_clock_start ? Math.round((Date.now() - new Date(t.wall_clock_start).getTime()) / 1000) + 's / ' : ''}${t.agent_config.ttl_seconds}s</div>` : ''}
            ${t.agent_config.confidence_threshold ? `<div><span style="color:var(--text-muted);">Confidence Threshold:</span> ${t.agent_config.confidence_threshold}%</div>` : ''}
            ${t.agent_config.sandbox_required ? `<div><span style="color:var(--accent);">🔒 Sandbox Required</span></div>` : ''}
          </div>
          ${t.agent_config.system_prompt ? `<div style="margin-top:6px;font-size:10px;color:var(--text-muted);"><em>${escapeHtml(t.agent_config.system_prompt).substring(0,100)}${t.agent_config.system_prompt.length > 100 ? '...' : ''}</em></div>` : ''}
          ${(t.agent_config.allowed_tools||[]).length ? `<div style="margin-top:4px;font-size:10px;color:var(--text-muted);">Tools: ${t.agent_config.allowed_tools.join(', ')}</div>` : ''}
          ${(t.agent_config.allowed_hosts||[]).length ? `<div style="margin-top:4px;font-size:10px;color:var(--text-muted);">Allowed Hosts: ${t.agent_config.allowed_hosts.join(', ')}</div>` : ''}
        </div>
      ` : ''}

      <!-- Confidence Score -->
      ${t.confidence_score ? `
        <div style="margin-bottom:16px;padding:10px;border-radius:var(--radius);border:1px solid ${t.confidence_score >= 80 ? 'var(--green)' : t.confidence_score >= 50 ? '#f59e0b' : 'var(--red)'};background:${t.confidence_score >= 80 ? 'rgba(0,200,0,0.05)' : t.confidence_score >= 50 ? 'rgba(245,158,11,0.05)' : 'rgba(200,0,0,0.05)'};">
          <div style="display:flex;align-items:center;gap:8px;">
            <span style="font-size:24px;font-weight:800;color:${t.confidence_score >= 80 ? 'var(--green)' : t.confidence_score >= 50 ? '#f59e0b' : 'var(--red)'};">${t.confidence_score}%</span>
            <span style="font-size:11px;color:var(--text-muted);">Agent Confidence</span>
          </div>
        </div>
      ` : ''}

      <!-- Child Tasks Tree -->
      ${(t.child_task_ids||[]).length > 0 ? `
        <div style="margin-bottom:16px;">
          <h4 style="margin:0 0 8px;font-size:12px;color:var(--text-muted);">Child Tasks (${t.child_task_ids.length})</h4>
          <div style="display:flex;flex-direction:column;gap:4px;">
            ${t.child_task_ids.map(cid => {
              const ct = state.tasks.find(x => x.id === cid);
              if (!ct) return `<div style="font-size:11px;color:var(--text-muted);">${cid} (not found)</div>`;
              const statusColor = ct.status === 'Done' ? 'var(--green)' : ct.status === 'Failed' ? 'var(--red)' : ct.status === 'In Progress' ? 'var(--accent)' : 'var(--text-muted)';
              return `<div style="display:flex;align-items:center;gap:6px;padding:6px 8px;background:var(--bg-input);border-radius:var(--radius-sm);cursor:pointer;font-size:11px;" onclick="openTaskDetail('${cid}')">
                <span style="width:8px;height:8px;border-radius:50%;background:${statusColor};flex-shrink:0;"></span>
                <span style="flex:1;color:var(--text-primary);">${escapeHtml(ct.title)}</span>
                <span style="color:var(--text-muted);font-size:10px;">${ct.status}</span>
              </div>`;
            }).join('')}
          </div>
        </div>
      ` : ''}

      <!-- Parent Link -->
      ${t.parent_task_id ? (() => {
        const pt = state.tasks.find(x => x.id === t.parent_task_id);
        return `<div style="margin-bottom:16px;padding:8px;background:var(--bg-input);border-radius:var(--radius);border:1px solid var(--border);font-size:11px;cursor:pointer;" onclick="openTaskDetail('${t.parent_task_id}')">
          <span style="color:var(--text-muted);">↑ Parent Task:</span> <span style="color:var(--accent);font-weight:600;">${pt ? escapeHtml(pt.title) : t.parent_task_id}</span>
        </div>`;
      })() : ''}

      <!-- Reasoning Scratchpad -->
      ${(t.reasoning||[]).length > 0 ? `
        <details style="margin-bottom:16px;border:1px solid var(--border);border-radius:var(--radius);overflow:hidden;">
          <summary style="cursor:pointer;padding:8px 12px;background:var(--bg-input);font-size:12px;font-weight:600;color:var(--text-muted);user-select:none;">
            Chain of Thought (${t.reasoning.length} steps)
          </summary>
          <div style="padding:8px 12px;max-height:300px;overflow-y:auto;">
            ${t.reasoning.map(r => `
              <div style="margin-bottom:8px;padding:6px;background:var(--bg-card);border-radius:var(--radius-sm);font-size:11px;">
                <div style="display:flex;justify-content:space-between;margin-bottom:2px;">
                  <span style="color:var(--text-primary);font-weight:600;">${escapeHtml(r.step)}</span>
                  <span style="color:var(--text-muted);font-size:10px;">${new Date(r.timestamp).toLocaleTimeString()}</span>
                </div>
                ${r.tool_call ? `<div style="color:var(--blue);font-family:monospace;font-size:10px;">$ ${escapeHtml(r.tool_call)}</div>` : ''}
                ${r.result ? `<div style="color:var(--text-muted);font-size:10px;margin-top:2px;">${escapeHtml(r.result).substring(0,200)}${r.result.length > 200 ? '...' : ''}</div>` : ''}
              </div>
            `).join('')}
          </div>
        </details>
      ` : ''}

      <!-- Execute Button (for Todo tasks) -->
      ${t.status === 'Todo' ? `
        <div style="margin-bottom:16px;">
          <button onclick="executeTask('${t.id}')" style="width:100%;padding:10px;background:var(--green);color:#000;border:none;border-radius:var(--radius);font-size:13px;font-weight:700;cursor:pointer;">
            Run with AI
          </button>
        </div>
      ` : ''}

      <!-- Activity History -->
      <div style="margin-bottom:16px;">
        <h4 style="margin:0 0 8px;font-size:12px;color:var(--text-muted);">Activity</h4>
        <div style="border-left:2px solid var(--border);padding-left:12px;">
          ${(t.history||[]).slice().reverse().map(h => `
            <div style="margin-bottom:8px;position:relative;">
              <div style="position:absolute;left:-17px;top:3px;width:6px;height:6px;border-radius:50%;background:var(--blue);"></div>
              <div style="font-size:11px;color:var(--text-primary);">${escapeHtml(h.action)}</div>
              <div style="font-size:10px;color:var(--text-muted);">${new Date(h.timestamp).toLocaleString()}${h.worker_agent ? ' by '+escapeHtml(h.worker_agent) : ''}</div>
            </div>
          `).join('')}
        </div>
      </div>

      <!-- Comments -->
      <div>
        <h4 style="margin:0 0 8px;font-size:12px;color:var(--text-muted);">Comments (${(t.comments||[]).length})</h4>
        ${(t.comments||[]).map(c => `
          <div style="margin-bottom:10px;padding:8px;background:var(--bg-input);border-radius:var(--radius-sm);">
            <div style="display:flex;justify-content:space-between;margin-bottom:4px;">
              <span style="font-size:11px;font-weight:600;color:var(--text-primary);">${escapeHtml(c.author)}</span>
              <span style="font-size:10px;color:var(--text-muted);">${timeAgo(c.created_at)}</span>
            </div>
            <div style="font-size:12px;color:var(--text-primary);line-height:1.4;">${escapeHtml(c.text)}</div>
          </div>
        `).join('')}
        <div style="display:flex;gap:6px;margin-top:8px;">
          <input id="task-comment-input" type="text" placeholder="Add a comment..." style="flex:1;padding:6px 10px;background:var(--bg-input);border:1px solid var(--border);border-radius:var(--radius-sm);color:var(--text-primary);font-size:12px;" onkeydown="if(event.key==='Enter')addTaskComment('${t.id}')">
          <button class="btn btn-primary" style="font-size:11px;padding:4px 12px;" onclick="addTaskComment('${t.id}')">Post</button>
        </div>
      </div>
    </div>
  `;

  document.body.appendChild(panel);
}

function closeTaskDetail() {
  const panel = document.getElementById('task-detail-panel');
  if (panel) panel.remove();
}

async function addTaskComment(taskId) {
  const input = document.getElementById('task-comment-input');
  if (!input || !input.value.trim()) return;
  const text = input.value.trim();

  const data = await api(`/v1/tasks/${taskId}/comments`, {
    method: 'POST',
    body: JSON.stringify({ author: 'User', text }),
  });

  if (data) {
    // Refresh task data and re-open detail
    await fetchTasks();
    openTaskDetail(taskId);
    renderTasks();
  }
}

// ── Toggle AI/Manual mode for a project ──────────────────────────
async function toggleProjectAutoMode(projectId) {
  const project = state.projects.find(p => String(p.id) === projectId);
  if (!project) return;

  const newMode = !project.auto_mode;
  try {
    await fetch(`${API}/v1/projects/${projectId}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ auto_mode: newMode })
    });
    project.auto_mode = newMode;
    renderTasks();
  } catch (e) {
    console.error('Failed to toggle auto mode:', e);
  }
}

// ── Fetch pending approval count ──────────────────────────────────
async function fetchApprovals() {
  try {
    const res = await fetch(`${API}/v1/approvals`);
    const data = await res.json();
    state.pendingApprovals = data.count || 0;
    return data.approvals || [];
  } catch (e) {
    state.pendingApprovals = 0;
    return [];
  }
}

// ── Approval Panel ────────────────────────────────────────────────
async function openApprovalPanel() {
  const approvals = await fetchApprovals();

  // Remove existing panel if any
  const existing = document.getElementById('approval-panel');
  if (existing) { existing.remove(); return; }

  const panel = document.createElement('div');
  panel.id = 'approval-panel';
  panel.style.cssText = `
    position:fixed;top:0;right:0;width:380px;height:100vh;z-index:9000;
    background:var(--bg-secondary);border-left:2px solid var(--border);
    box-shadow:-4px 0 20px rgba(0,0,0,0.5);overflow-y:auto;padding:16px;
    animation:slideInRight 0.2s ease;
  `;

  panel.innerHTML = `
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:16px;">
      <h3 style="margin:0;color:var(--text-primary);">Pending Approvals</h3>
      <button onclick="document.getElementById('approval-panel').remove()" style="background:none;border:none;color:var(--text-muted);font-size:18px;cursor:pointer;">&times;</button>
    </div>
    ${approvals.length === 0 ? `
      <div style="text-align:center;padding:40px 0;color:var(--text-muted);">
        <div style="font-size:32px;margin-bottom:8px;">✅</div>
        <p>No pending approvals</p>
      </div>
    ` : approvals.map(a => `
      <div style="background:var(--bg-card);border:1px solid var(--border);border-left:3px solid ${a.status === 'pending' ? 'var(--orange)' : 'var(--green)'};border-radius:var(--radius);padding:12px;margin-bottom:8px;">
        <div style="font-weight:600;color:var(--text-primary);font-size:13px;margin-bottom:4px;">${escapeHtml(a.description)}</div>
        <div style="font-size:11px;color:var(--text-muted);margin-bottom:8px;">
          Action: <span style="color:var(--orange);font-weight:600;">${escapeHtml(a.action)}</span>
          &middot; Requested by: ${escapeHtml(a.requester)}
          &middot; ${new Date(a.created_at).toLocaleString()}
        </div>
        ${a.status === 'pending' ? `
          <div style="display:flex;gap:6px;">
            <button class="btn btn-primary" style="font-size:11px;padding:4px 12px;" onclick="handleApprovalAction('${a.id}', 'approve')">Approve</button>
            <button class="btn" style="font-size:11px;padding:4px 12px;color:var(--red);" onclick="handleApprovalAction('${a.id}', 'reject')">Reject</button>
          </div>
        ` : `
          <span style="font-size:11px;color:var(--green);">Resolved: ${a.status}</span>
        `}
      </div>
    `).join('')}
  `;

  document.body.appendChild(panel);
}

// ── Handle approve/reject action ──────────────────────────────────
async function handleApprovalAction(id, action) {
  try {
    await fetch(`${API}/v1/approvals/${id}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action })
    });
    // Refresh the panel
    document.getElementById('approval-panel')?.remove();
    openApprovalPanel();
    fetchApprovals().then(() => {
      if (state.currentPage === 'tasks') renderTasks();
    });
  } catch (e) {
    console.error('Approval action failed:', e);
  }
}

// Poll for approvals every 30 seconds
setInterval(() => {
  fetchApprovals().then(() => {
    // Update badge if on tasks page
    const badge = document.querySelector('#approval-badge');
    if (badge) badge.textContent = state.pendingApprovals || '';
  });
}, 30000);

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
// Staged files for upload after task creation
window._stagedFiles = [];

function openTaskModal() {
  document.getElementById('task-modal-title').textContent = 'New Task';
  document.getElementById('task-id').value = '';
  document.getElementById('task-title-input').value = '';
  document.getElementById('task-desc-input').value = '';
  document.getElementById('task-status-group').style.display = 'none';
  
  // Reset fields
  const priEl = document.getElementById('task-priority-input');
  if (priEl) priEl.value = '0';
  const lblEl = document.getElementById('task-labels-input');
  if (lblEl) lblEl.value = '';
  const dueEl = document.getElementById('task-due-input');
  if (dueEl) dueEl.value = '';
  const autoEl = document.getElementById('task-autoexec-input');
  if (autoEl) autoEl.checked = false;
  
  // Clear staged files
  window._stagedFiles = [];
  const fileList = document.getElementById('task-file-list');
  if (fileList) fileList.innerHTML = '';
  
  // Populate dropdowns
  populateModalDropdowns();
  
  // Show all field groups
  document.getElementById('task-priority-group').style.display = 'block';
  document.getElementById('task-labels-group').style.display = 'block';
  document.getElementById('task-due-group').style.display = 'block';
  const siGroup = document.getElementById('task-subitems-group');
  if (siGroup) siGroup.style.display = 'none';
  
  // Hide delete button for new tasks
  const deleteBtn = document.getElementById('task-delete-btn');
  if (deleteBtn) deleteBtn.remove();
  
  document.getElementById('task-modal').style.display = 'flex';
}

function populateModalDropdowns() {
  // Populate project dropdown
  const projSelect = document.getElementById('task-project-input');
  if (projSelect) {
    projSelect.innerHTML = '<option value="">No Project</option>';
    (state.projects || []).filter(p => p.status === 'active').forEach(p => {
      const opt = document.createElement('option');
      opt.value = p.id;
      opt.textContent = p.name;
      if (state.selectedProjectId && String(p.id) === state.selectedProjectId) opt.selected = true;
      projSelect.appendChild(opt);
    });
  }
  
  // Populate agent dropdown
  const agentSelect = document.getElementById('task-agent-input');
  if (agentSelect) {
    agentSelect.innerHTML = '<option value="">Auto (Orchestrator decides)</option>';
    (state.agents || []).forEach(a => {
      const opt = document.createElement('option');
      opt.value = a.name;
      opt.textContent = `${a.name} - ${a.role || ''}`;
      agentSelect.appendChild(opt);
    });
  }
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
  
  // New fields
  const priEl = document.getElementById('task-priority-input');
  if (priEl) priEl.value = (task.priority || 0).toString();
  const lblEl = document.getElementById('task-labels-input');
  if (lblEl) lblEl.value = (task.labels || []).join(', ');
  const dueEl = document.getElementById('task-due-input');
  if (dueEl && task.due_date) {
    dueEl.value = new Date(task.due_date).toISOString().split('T')[0];
  } else if (dueEl) {
    dueEl.value = '';
  }
  document.getElementById('task-priority-group').style.display = 'block';
  document.getElementById('task-labels-group').style.display = 'block';
  document.getElementById('task-due-group').style.display = 'block';

  // Sub-items
  const siGroup = document.getElementById('task-subitems-group');
  if (siGroup) {
    siGroup.style.display = 'block';
    const siContainer = document.getElementById('task-subitems-list');
    if (siContainer) {
      siContainer.innerHTML = (task.sub_items || []).map((si, i) => `
        <div style="display:flex;align-items:center;gap:6px;margin-bottom:4px;">
          <input type="checkbox" ${si.done ? 'checked' : ''} onchange="toggleSubItem(${i})" style="cursor:pointer;">
          <input type="text" value="${escapeHtml(si.title)}" onchange="updateSubItemTitle(${i}, this.value)" style="flex:1;padding:4px 8px;background:var(--bg-input);border:1px solid var(--border);border-radius:var(--radius-sm);color:var(--text-primary);font-size:12px;${si.done ? 'text-decoration:line-through;opacity:.6;' : ''}">
          <button class="btn" style="padding:2px 6px;font-size:10px;color:var(--red);" onclick="removeSubItem(${i})">×</button>
        </div>
      `).join('');
    }
  }
  
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

// Sub-item helpers (temporary state stored on window for modal)
window._editSubItems = [];

function toggleSubItem(idx) {
  const id = document.getElementById('task-id').value;
  const task = state.tasks.find(t => t.id === id);
  if (task && task.sub_items && task.sub_items[idx]) {
    task.sub_items[idx].done = !task.sub_items[idx].done;
    openEditTaskModal(id); // refresh
  }
}

function updateSubItemTitle(idx, newTitle) {
  const id = document.getElementById('task-id').value;
  const task = state.tasks.find(t => t.id === id);
  if (task && task.sub_items && task.sub_items[idx]) {
    task.sub_items[idx].title = newTitle;
  }
}

function removeSubItem(idx) {
  const id = document.getElementById('task-id').value;
  const task = state.tasks.find(t => t.id === id);
  if (task && task.sub_items) {
    task.sub_items.splice(idx, 1);
    openEditTaskModal(id); // refresh
  }
}

function addSubItem() {
  const id = document.getElementById('task-id').value;
  const task = state.tasks.find(t => t.id === id);
  if (task) {
    if (!task.sub_items) task.sub_items = [];
    task.sub_items.push({ id: `si_${Date.now()}`, title: '', done: false });
    openEditTaskModal(id); // refresh
  }
}

function closeTaskModal() {
  document.getElementById('task-modal').style.display = 'none';
}

async function saveTask(returnId = false) {
  const id = document.getElementById('task-id').value;
  const title = document.getElementById('task-title-input').value;
  const desc = document.getElementById('task-desc-input').value;
  const priority = document.getElementById('task-priority-input')?.value || '0';
  const labelsRaw = document.getElementById('task-labels-input')?.value || '';
  const labels = labelsRaw.split(',').map(l => l.trim()).filter(Boolean);
  const dueDate = document.getElementById('task-due-input')?.value || '';
  const projectId = document.getElementById('task-project-input')?.value || '';
  const agent = document.getElementById('task-agent-input')?.value || '';
  const autoExec = document.getElementById('task-autoexec-input')?.checked || false;
  
  // Collect AI execution settings
  const sysPrompt = document.getElementById('task-systemprompt')?.value || '';
  const outputSchema = document.getElementById('task-outputschema')?.value || '';
  const autonomy = document.getElementById('task-autonomy')?.value || 'supervised';
  const maxRetries = parseInt(document.getElementById('task-maxretries')?.value) || 3;
  const maxTokens = parseInt(document.getElementById('task-maxtokens')?.value) || 0;
  const ttl = parseInt(document.getElementById('task-ttl')?.value) || 0;
  const toolsRaw = document.getElementById('task-allowedtools')?.value || '';
  const allowedTools = toolsRaw.split(',').map(t => t.trim()).filter(Boolean);
  const refsRaw = document.getElementById('task-contextrefs')?.value || '';
  const contextRefs = refsRaw.split(',').map(r => r.trim()).filter(Boolean);
  
  // Build agent_config if any AI setting was provided
  let agentConfig = null;
  if (sysPrompt || outputSchema || allowedTools.length || contextRefs.length || maxTokens || ttl) {
    agentConfig = {
      system_prompt: sysPrompt,
      output_schema: outputSchema,
      autonomy_level: autonomy,
      max_retries: maxRetries,
      max_tokens: maxTokens,
      ttl_seconds: ttl,
      allowed_tools: allowedTools.length ? allowedTools : undefined,
      context_refs: contextRefs.length ? contextRefs : undefined,
    };
  }
  
  if (!title) {
    alert('Title is required');
    return null;
  }
  
  let taskId = id;
  if (id) {
    // Edit existing
    const status = document.getElementById('task-status-input').value;
    const task = state.tasks.find(t => t.id === id);
    const subItems = task?.sub_items || [];
    await editTask(id, title, desc, status, agent, priority, labels, dueDate, subItems, projectId);
  } else {
    // Create new with project_id and agent_config
    const result = await createTask(title, desc, priority, labels, dueDate, projectId, agent, agentConfig, autoExec);
    if (result && result.id) taskId = result.id;
  }
  
  // Upload any staged files
  if (taskId && window._stagedFiles.length > 0) {
    await uploadTaskFiles(taskId, window._stagedFiles);
    window._stagedFiles = [];
  }
  
  // Auto-execute if checkbox was checked
  if (!id && autoExec && taskId) {
    setTimeout(() => executeTask(taskId), 200);
  }
  
  if (!returnId) {
    closeTaskModal();
    renderTasks();
  }
  return taskId;
}

// Save and immediately dispatch to the orchestrator
async function saveAndRunTask() {
  const taskId = await saveTask(true);
  if (!taskId) return;
  
  closeTaskModal();
  renderTasks();
  await executeTask(taskId);
}

// Dispatch a task to the orchestrator
async function executeTask(taskId) {
  try {
    const res = await fetch(`${API}/v1/tasks/${taskId}/execute`, { method: 'POST' });
    const data = await res.json();
    if (res.ok) {
      // Refresh tasks to show In Progress status
      setTimeout(() => { fetchTasks().then(() => renderTasks()); }, 500);
    } else {
      console.error('Execute failed:', data.error);
    }
  } catch (e) {
    console.error('Execute error:', e);
  }
}

async function wakeTask(taskId) {
  try {
    const res = await fetch(`${API}/v1/tasks/${taskId}/wake`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ webhook_id: 'manual_wake' })
    });
    const data = await res.json();
    if (res.ok) {
      setTimeout(() => { fetchTasks().then(() => renderTasks()); }, 500);
    } else {
      console.error('Wake failed:', data.error);
    }
  } catch (e) {
    console.error('Wake error:', e);
  }
}

// File handling for task attachments
function handleFileDrop(ev) {
  ev.preventDefault();
  const dropzone = document.getElementById('task-file-dropzone');
  if (dropzone) {
    dropzone.style.borderColor = 'var(--border)';
    dropzone.style.background = 'transparent';
  }
  const files = ev.dataTransfer?.files;
  if (files) handleFileSelect(files);
}

function handleFileSelect(files) {
  for (const file of files) {
    window._stagedFiles.push(file);
  }
  renderStagedFiles();
}

function removeStagedFile(index) {
  window._stagedFiles.splice(index, 1);
  renderStagedFiles();
}

function renderStagedFiles() {
  const container = document.getElementById('task-file-list');
  if (!container) return;
  container.innerHTML = window._stagedFiles.map((f, i) => {
    const icon = f.type.startsWith('image/') ? '🖼' : f.type.includes('pdf') ? '📄' : '📎';
    const size = f.size < 1024 ? f.size + 'B' : (f.size / 1024).toFixed(1) + 'KB';
    return `<div style="display:flex;align-items:center;gap:6px;padding:4px 8px;background:var(--bg-input);border:1px solid var(--border);border-radius:var(--radius-sm);margin-bottom:4px;font-size:11px;">
      <span>${icon}</span>
      <span style="flex:1;color:var(--text-primary);">${escapeHtml(f.name)}</span>
      <span style="color:var(--text-muted);">${size}</span>
      <button type="button" onclick="removeStagedFile(${i})" style="background:none;border:none;color:var(--red);cursor:pointer;font-size:14px;">x</button>
    </div>`;
  }).join('');
}

async function uploadTaskFiles(taskId, files) {
  for (const file of files) {
    const formData = new FormData();
    formData.append('file', file);
    try {
      await fetch(`${API}/v1/tasks/${taskId}/attachments`, {
        method: 'POST',
        body: formData
      });
    } catch (e) {
      console.error('File upload failed:', e);
    }
  }
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

function helpIcon(tip) {
  return `<span style="display:inline-flex;align-items:center;justify-content:center;width:18px;height:18px;border-radius:50%;border:1.5px solid var(--text-muted);color:var(--text-muted);font-size:11px;font-weight:700;cursor:help;margin-left:6px;position:relative;flex-shrink:0;line-height:1;" title="${escapeHtml(tip)}">?</span>`;
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

  // Smart paragraph splitting for plain-text LLM responses.
  // If the output is one long block without markdown structure, split on sentence boundaries.
  const hasStructure = /<(br|li|h[2-4]|ul|ol|hr)/.test(text);
  if (!hasStructure && text.length > 200) {
    // Split on sentence boundaries (period followed by space and capital letter).
    text = text.replace(/\.\s+([A-Z])/g, '.<br><br>$1');
    // Split on semicolons followed by space.
    text = text.replace(/;\s+/g, ';<br>');
  }

  // Make URLs in plain text clickable.
  text = text.replace(/(https?:\/\/[^\s<]+)/g, (url) => {
    if (url.match(/<\/a>/)) return url; // already a link
    const short = url.replace(/https?:\/\/(www\.)?/, '').split('/')[0];
    return `<a href="${url}" target="_blank" style="color:var(--blue);text-decoration:underline;word-break:break-all;">🔗 ${short}</a>`;
  });

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
        <div style="display:flex;gap:8px;margin-bottom:12px;align-items:center;flex-wrap:wrap;">
          <input type="file" id="db-upload" multiple
            accept=".zip,.rar,.7z,.tar,.tar.gz,.tgz,.tar.bz2,.gz,.bz2,.pdf,.txt,.md,.rst,.docx,.doc,.csv,.tsv,.json,.yaml,.yml,.xml,.toml,.html,.htm,.css,.js,.jsx,.ts,.tsx,.py,.go,.rs,.java,.c,.cpp,.rb,.sh,.bat,.ps1,.env,.svg,.png,.jpg,.jpeg,.gif,.webp,.mp3,.wav,.mp4"
            style="display:none">
          <button class="btn btn-primary" style="font-size:12px;padding:6px 14px;" onclick="document.getElementById('db-upload').click()">
            &#128230; Upload Files
          </button>
          <span style="font-size:10px;color:var(--text-muted);">Archives: zip, rar, 7z, tar.gz | Docs: pdf, txt, md, docx, csv, json, yaml, html, code files</span>
        </div>
        <div id="upload-progress-area" style="display:none;margin-bottom:12px;">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px;">
            <span id="upload-filename" style="font-size:11px;color:var(--text);font-weight:500;"></span>
            <span id="upload-percent" style="font-size:11px;color:var(--accent);font-family:var(--mono);"></span>
          </div>
          <div style="width:100%;height:6px;background:var(--surface);border-radius:3px;overflow:hidden;">
            <div id="upload-bar" style="height:100%;width:0%;background:linear-gradient(90deg,var(--accent),#3fb950);border-radius:3px;transition:width .15s;"></div>
          </div>
          <div id="upload-detail" style="font-size:10px;color:var(--text-muted);margin-top:2px;"></div>
        </div>
        <div id="upload-status" style="font-size:11px;color:var(--text-muted);margin-bottom:8px;"></div>
        <div style="border:1px solid var(--border);border-radius:var(--radius);overflow:hidden;height:calc(100vh - 300px);">
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
    const upInput = document.getElementById('db-upload');
    if (upInput) {
      upInput.addEventListener('change', async (ev) => {
        const files = Array.from(ev.target.files);
        if (!files.length) return;

        const progressArea = document.getElementById('upload-progress-area');
        const filenameEl = document.getElementById('upload-filename');
        const percentEl = document.getElementById('upload-percent');
        const barEl = document.getElementById('upload-bar');
        const detailEl = document.getElementById('upload-detail');
        const statusEl = document.getElementById('upload-status');

        let totalIngested = 0;

        for (let i = 0; i < files.length; i++) {
          const file = files[i];
          if (progressArea) progressArea.style.display = 'block';
          if (filenameEl) filenameEl.textContent = `Uploading ${file.name} (${i+1}/${files.length})`;
          if (percentEl) percentEl.textContent = '0%';
          if (barEl) barEl.style.width = '0%';
          if (detailEl) detailEl.textContent = `0 / ${formatSize(file.size)}`;

          const fd = new FormData();
          fd.append('file', file);

          try {
            const result = await new Promise((resolve, reject) => {
              const xhr = new XMLHttpRequest();
              xhr.open('POST', '/v1/graph/ingest');

              xhr.upload.addEventListener('progress', (e) => {
                if (e.lengthComputable) {
                  const pct = Math.round((e.loaded / e.total) * 100);
                  if (barEl) barEl.style.width = pct + '%';
                  if (percentEl) percentEl.textContent = pct + '%';
                  if (detailEl) detailEl.textContent = `${formatSize(e.loaded)} / ${formatSize(e.total)}`;
                }
              });

              xhr.addEventListener('load', () => {
                try {
                  resolve(JSON.parse(xhr.responseText));
                } catch (e) {
                  reject(new Error('Invalid response'));
                }
              });

              xhr.addEventListener('error', () => reject(new Error('Upload failed')));
              xhr.send(fd);
            });

            if (result.status === 'ingested') {
              totalIngested += result.files || 1;
              if (barEl) { barEl.style.width = '100%'; barEl.style.background = 'var(--success)'; }
              if (statusEl) statusEl.innerHTML = `<span style="color:var(--green)">Ingested ${result.template}: ${result.files || 1} files across 4 databases</span>`;
            } else {
              if (statusEl) statusEl.innerHTML = `<span style="color:var(--red)">Error: ${result.error || 'unknown'}</span>`;
            }
          } catch (err) {
            if (statusEl) statusEl.innerHTML = `<span style="color:var(--red)">Upload failed: ${err.message}</span>`;
          }
        }

        // Refresh after all uploads
        if (totalIngested > 0) {
          const frame = document.getElementById('graph-frame');
          if (frame) frame.src = frame.src;
          setTimeout(() => renderDatabase(), 1000);
        }

        // Hide progress after brief delay
        setTimeout(() => {
          if (progressArea) progressArea.style.display = 'none';
          if (barEl) { barEl.style.width = '0%'; barEl.style.background = 'linear-gradient(90deg,var(--accent),#3fb950)'; }
        }, 2000);

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

  // Separate system agents from focused agents
  const systemAgents = state.personas.filter(p => p.is_default || p.is_locked);
  const focusedAgents = state.personas.filter(p => !p.is_default && !p.is_locked);
  const orchestrator = systemAgents.find(p => p.name === 'mike') || systemAgents[0];
  const embedAgent = systemAgents.find(p => p.name === 'embed');

  // System agent icons
  const agentIcon = (name) => {
    if (name === 'mike') return '🧠';
    if (name === 'embed') return '🗄️';
    return '🤖';
  };

  // ── System Agents cards ──
  const systemCards = systemAgents.map(sa => `
    <div class="card" style="border-left:3px solid ${sa.name === 'mike' ? 'var(--blue)' : 'var(--green)'};">
      <div style="display:flex;justify-content:space-between;align-items:center;">
        <div style="display:flex;align-items:center;gap:8px;">
          <span style="font-size:16px;font-weight:800;color:var(--text-primary);">${agentIcon(sa.name)} ${sa.name === 'mike' ? 'Orchestrator' : 'Embed Agent'}</span>
          <span class="badge" style="font-size:9px;background:${sa.name === 'mike' ? 'var(--blue)' : 'var(--green)'};color:#fff;">SYSTEM</span>
          <span class="badge" style="font-size:9px;background:var(--bg-input);color:var(--text-muted);">🔒 LOCKED</span>
        </div>
        <button class="btn" style="font-size:11px;padding:4px 10px;" onclick="${sa.name === 'mike' ? 'editOrchestrator()' : ''}">Customize</button>
      </div>
      <div style="font-size:12px;color:var(--text-secondary);margin-top:8px;line-height:1.5;">${sa.personality || 'No personality set'}</div>
      <div style="display:flex;gap:8px;margin-top:8px;flex-wrap:wrap;">
        <span style="font-size:10px;color:var(--text-muted);">Name: <div class="agency-sub-name">${escapeHtml(sa.name)}</div></span>
        <span style="font-size:10px;color:var(--text-muted);">Role: <strong style="color:var(--text-primary);">${sa.role || (sa.name === 'mike' ? 'Tech Lead' : 'Knowledge Agent')}</strong></span>
      </div>
      <div style="font-size:10px;color:var(--text-muted);margin-top:6px;padding:6px 8px;background:var(--bg-input);border-radius:var(--radius-sm);border:1px dashed var(--border);">
        🔒 ${sa.name === 'mike' ? 'Core delegation logic, system prompt, and max_delegations are locked.' : 'Embedding pipeline and knowledge persistence are locked.'}
      </div>
    </div>
  `).join('');

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
        <div style="margin-top:8px;padding:6px 8px;background:var(--bg-input);border-radius:var(--radius-sm);border:1px solid var(--border);display:flex;align-items:center;gap:6px;">
          <span style="font-size:10px;color:var(--text-muted);">Model:</span>
          <span class="badge" style="font-size:10px;background:var(--bg-sidebar);">Use Global</span>
        </div>
      </div>`;
  }).join('');

  container.innerHTML = `
    <!-- Global Model Config -->
    <div style="margin-bottom:16px;">
      <span class="section-label">Model Configuration</span>
      <p style="font-size:11px;color:var(--text-muted);margin:2px 0 8px 0;">Set the default LLM for all agents. Individual agents can override this.</p>
      <div class="card" style="border-left:3px solid var(--accent);">
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:10px;">
          <span style="font-size:14px;font-weight:700;color:var(--text-primary);">⚙️ Global Model</span>
        </div>
        <div style="display:flex;gap:10px;flex-wrap:wrap;">
          <div style="flex:1;min-width:120px;">
            <label style="font-size:10px;color:var(--text-muted);display:block;margin-bottom:3px;">Model Type</label>
            <select id="global-model-type" class="form-control" style="font-size:12px;" onchange="onModelTypeChange(this.value)">
              <option value="api">API Provider</option>
              <option value="torch">Local (Torch / SafeTensors)</option>
              <option value="ollama">Ollama (GGUF)</option>
            </select>
          </div>
          <div style="flex:2;min-width:160px;" id="model-provider-group">
            <label style="font-size:10px;color:var(--text-muted);display:block;margin-bottom:3px;">Provider</label>
            <select id="global-provider" class="form-control" style="font-size:12px;" onchange="onProviderChange(this.value)">
              <option value="">Select provider...</option>
            </select>
          </div>
          <div style="flex:2;min-width:150px;">
            <label style="font-size:10px;color:var(--text-muted);display:block;margin-bottom:3px;">API Key</label>
            <input type="password" id="global-api-key" class="form-control" style="font-size:12px;" placeholder="sk-...">
          </div>
        </div>
        <div style="display:flex;gap:10px;margin-top:8px;align-items:flex-end;">
          <div style="flex:3;min-width:200px;">
            <label style="font-size:10px;color:var(--text-muted);display:block;margin-bottom:3px;">Model</label>
            <select id="global-model-select" class="form-control" style="font-size:12px;">
              <option value="">Enter API key then Load Models</option>
            </select>
          </div>
          <button class="btn btn-primary" style="font-size:11px;padding:6px 14px;white-space:nowrap;height:32px;" onclick="loadModels()">Load Models</button>
          <button class="btn" style="font-size:11px;padding:6px 14px;white-space:nowrap;height:32px;" onclick="saveGlobalModel()">Save</button>
        </div>
      </div>
    </div>

    <!-- System Agents -->
    <div style="margin-bottom:16px;">
      <span class="section-label">System Agents</span>
      <p style="font-size:11px;color:var(--text-muted);margin:2px 0 8px 0;">Core agents that cannot be deleted. The orchestrator routes tasks, the embed agent persists knowledge.</p>
      <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(360px,1fr));gap:10px;">
        ${systemCards}
      </div>
    </div>

    <!-- Focused Agents -->
    <div style="margin-bottom:16px;">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;">
        <span class="section-label">Focused Agents (${focusedAgents.length})</span>
        <button class="btn btn-primary" style="font-size:12px;padding:6px 14px;" onclick="openAgentModal()">+ New Agent</button>
      </div>
      <p style="font-size:11px;color:var(--text-muted);margin:0 0 8px 0;">Specialized workers that receive delegated tasks. Each agent has a role, personality, tools, and autonomy level.</p>
      <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(360px,1fr));gap:10px;">
        ${agentCards || '<div style="color:var(--text-muted);text-align:center;padding:20px;">No focused agents yet. Click "+ New Agent" to create one.</div>'}
      </div>
    </div>

    <!-- Doctor Status -->
    <div style="margin-bottom:16px;">
      <span class="section-label">Doctor (Self-Healing)</span>
      <div class="card" style="border-left:3px solid var(--yellow);">
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px;">
          <span style="font-size:14px;font-weight:700;color:var(--text-primary);">🏥 Doctor</span>
          <span class="badge" style="font-size:9px;background:var(--green);color:#fff;">ACTIVE</span>
        </div>
        <div style="font-size:12px;color:var(--text-secondary);line-height:1.5;">
          Auto-heals errors, runs linters, manages golden snapshots, and audits the database for stale or contradictory data.
        </div>
        <div style="display:flex;gap:16px;margin-top:8px;flex-wrap:wrap;">
          <span style="font-size:10px;color:var(--text-muted);">Heartbeat: <strong style="color:var(--text-primary);">30 min</strong></span>
          <span style="font-size:10px;color:var(--text-muted);">DB Audit: <strong style="color:var(--green);">Healthy</strong></span>
          <span style="font-size:10px;color:var(--text-muted);">Max Fix Attempts: <strong style="color:var(--text-primary);">3/session</strong></span>
        </div>
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

// ── Model Management ──────────────────────────────────────────────
async function loadModels() {
  const provider = document.getElementById('global-provider')?.value;
  const apiKey = document.getElementById('global-api-key')?.value;
  const modelSelect = document.getElementById('global-model-select');
  if (!modelSelect) return;

  modelSelect.innerHTML = '<option value="">Loading...</option>';

  const body = { provider: provider || '', api_key: apiKey || '' };
  const res = await api('/v1/models/list', { method: 'POST', body: JSON.stringify(body) });

  if (res && res.models) {
    modelSelect.innerHTML = res.models.map(m =>
      `<option value="${m.id}">${m.id}${m.owned_by ? ' (' + m.owned_by + ')' : ''}</option>`
    ).join('');
    if (res.models.length === 0) {
      modelSelect.innerHTML = '<option value="">No models found</option>';
    }
  } else {
    modelSelect.innerHTML = '<option value="">Failed to load models</option>';
  }
}

async function saveGlobalModel() {
  const modelType = document.getElementById('global-model-type')?.value || 'api';
  const provider = document.getElementById('global-provider')?.value || '';
  const apiKey = document.getElementById('global-api-key')?.value || '';
  const model = document.getElementById('global-model-select')?.value || '';

  const body = {
    model_type: modelType,
    provider: provider,
    model: model,
    api_key: apiKey,
    api_base: '',
  };

  if (modelType === 'ollama') {
    body.ollama_model = model;
    body.ollama_endpoint = 'http://localhost:11434';
  }

  const res = await api('/v1/models/global', { method: 'PUT', body: JSON.stringify(body) });
  if (res && !res.error) {
    addLog('info', 'model', 'Global model config saved: ' + model);
  } else {
    alert(res?.error || 'Failed to save model config');
  }
}

function onModelTypeChange(type) {
  const providerGroup = document.getElementById('model-provider-group');
  if (providerGroup) {
    providerGroup.style.display = type === 'api' ? '' : 'none';
  }
  if (type === 'ollama') {
    const sel = document.getElementById('global-provider');
    if (sel) sel.value = 'ollama';
  }
}

async function onProviderChange(slug) {
  // No-op for now, could auto-populate API base
}

// Load providers into dropdown when agents page renders
async function loadProviderCatalog() {
  const sel = document.getElementById('global-provider');
  if (!sel) return;

  const res = await api('/v1/models/providers');
  if (res && res.providers) {
    sel.innerHTML = '<option value="">Select provider...</option>' +
      res.providers.filter(p => p.compatible && p.api_base).map(p =>
        `<option value="${p.slug}">${p.name} (${p.category})</option>`
      ).join('');
  }
}

// Auto-load provider catalog when the agents page opens
const origRenderPersonas = renderPersonas;
renderPersonas = async function(container) {
  await origRenderPersonas(container);
  await loadProviderCatalog();
};

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
  await Promise.all([fetchStatus(), fetchAgents(), fetchPersonas(), fetchAllAgencies()]);
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


// ── Workflow Builder v2 (n8n-style visual canvas) ───────────────────

const WORKFLOW_NODE_TYPES = [
  { type: 'prompt',       icon: '💬', label: 'Prompt',       color: '#6366f1', group: 'Core',     tip: 'Send a text instruction to start a workflow or feed data to the next node' },
  { type: 'agent',        icon: '🤖', label: 'Agent',        color: '#22c55e', group: 'Core',     tip: 'Run a focused agent (researcher, coder, etc.) with a specific task' },
  { type: 'webhook',      icon: '🔗', label: 'Webhook',      color: '#f59e0b', group: 'Triggers', tip: 'Listen for incoming HTTP requests to trigger this workflow' },
  { type: 'api_call',     icon: '🌐', label: 'API Call',     color: '#3b82f6', group: 'Actions', tip: 'Make an HTTP request to an external API and pass the response forward' },
  { type: 'websocket',    icon: '⚡', label: 'WebSocket',    color: '#a855f7', group: 'Actions', tip: 'Open a real-time WebSocket connection to stream data' },
  { type: 'condition',    icon: '🔀', label: 'Condition',    color: '#ef4444', group: 'Logic',   tip: 'Branch the flow based on a true/false condition (has green and red outputs)' },
  { type: 'transform',    icon: '🔄', label: 'Transform',    color: '#14b8a6', group: 'Logic',   tip: 'Reshape or format data using a template before passing it on' },
  { type: 'delay',        icon: '⏱️', label: 'Delay',        color: '#78716c', group: 'Logic',   tip: 'Pause the workflow for a set number of seconds before continuing' },
  { type: 'loop',         icon: '🔁', label: 'Loop',         color: '#0ea5e9', group: 'Logic',   tip: 'Repeat the next nodes once for each item in an array' },
  { type: 'merge',        icon: '🔃', label: 'Merge',        color: '#8b5cf6', group: 'Logic',   tip: 'Wait for multiple inputs to arrive, then combine them into one' },
  { type: 'code',         icon: '📝', label: 'Code',         color: '#06b6d4', group: 'Actions', tip: 'Run custom JavaScript code to process data however you want' },
  { type: 'error_handler',icon: '🛡️', label: 'Error Handler',color: '#dc2626', group: 'Logic',   tip: 'Catch errors from upstream nodes and handle them gracefully' },
  { type: 'schedule',     icon: '📅', label: 'Schedule',     color: '#d97706', group: 'Triggers', tip: 'Trigger the workflow on a cron schedule (e.g. every hour, daily)' },
  { type: 'db_query',     icon: '🗄️', label: 'DB Query',     color: '#059669', group: 'Actions', tip: 'Run a database query (SELECT, INSERT, UPDATE) and pass results forward' },
  { type: 'email',        icon: '📧', label: 'Email',        color: '#e11d48', group: 'Actions', tip: 'Send an email with configurable to, subject, and body fields' },
  { type: 'notification', icon: '🔔', label: 'Notification', color: '#7c3aed', group: 'Actions', tip: 'Send a notification to Discord, Slack, or a webhook channel' },
  { type: 'human',        icon: '👤', label: 'Approval',     color: '#f97316', group: 'Logic',   tip: 'Pause the workflow until a human approves or rejects the request' },
  { type: 'note',         icon: '📌', label: 'Note',         color: '#fbbf24', group: 'Core',     tip: 'A sticky note on the canvas for documentation (does not execute)' },
];

// Workflow templates for quick start.
const WORKFLOW_TEMPLATES = [
  { name: 'Social Media Monitor', desc: 'Watch feeds, analyze sentiment, generate report', nodes: [
    {id:'t1',type:'schedule',label:'Every Hour',x:80,y:150,config:{cron:'0 * * * *'}},
    {id:'t2',type:'api_call',label:'Fetch Feed',x:300,y:150,config:{method:'GET',url:'https://api.example.com/feed'}},
    {id:'t3',type:'agent',label:'Analyze',x:520,y:150,config:{agent_name:'researcher',task:'Analyze sentiment'}},
    {id:'t4',type:'condition',label:'Negative?',x:740,y:150,config:{field:'sentiment',operator:'==',value:'negative'}},
    {id:'t5',type:'notification',label:'Alert',x:960,y:80,config:{channel:'discord'}},
    {id:'t6',type:'db_query',label:'Store',x:960,y:220,config:{query:'INSERT INTO reports'}},
  ], edges: [{id:'e1',source:'t1',target:'t2'},{id:'e2',source:'t2',target:'t3'},{id:'e3',source:'t3',target:'t4'},{id:'e4',source:'t4',target:'t5'},{id:'e5',source:'t4',target:'t6'}]},
  { name: 'Research Pipeline', desc: 'Search, summarize, and email results', nodes: [
    {id:'t1',type:'prompt',label:'Research Query',x:80,y:150,config:{prompt:'Research {{topic}}'}},
    {id:'t2',type:'agent',label:'Web Search',x:300,y:150,config:{agent_name:'researcher',task:'Search the web'}},
    {id:'t3',type:'transform',label:'Format',x:520,y:150,config:{template:'## Results\n{{results}}'}},
    {id:'t4',type:'email',label:'Send Report',x:740,y:150,config:{to:'team@company.com',subject:'Research: {{topic}}'}},
  ], edges: [{id:'e1',source:'t1',target:'t2'},{id:'e2',source:'t2',target:'t3'},{id:'e3',source:'t3',target:'t4'}]},
  { name: 'Data ETL', desc: 'Extract, transform, and load data', nodes: [
    {id:'t1',type:'webhook',label:'Receive Data',x:80,y:150,config:{method:'POST'}},
    {id:'t2',type:'code',label:'Parse JSON',x:300,y:150,config:{language:'javascript',code:'return JSON.parse(input)'}},
    {id:'t3',type:'loop',label:'Each Record',x:520,y:150,config:{array_field:'records'}},
    {id:'t4',type:'transform',label:'Map Fields',x:740,y:150,config:{template:'{{item.name}}: {{item.value}}'}},
    {id:'t5',type:'db_query',label:'Insert',x:960,y:150,config:{query:'INSERT INTO processed'}},
  ], edges: [{id:'e1',source:'t1',target:'t2'},{id:'e2',source:'t2',target:'t3'},{id:'e3',source:'t3',target:'t4'},{id:'e4',source:'t4',target:'t5'}]},
  { name: 'Approval Flow', desc: 'Request, review, approve, and notify', nodes: [
    {id:'t1',type:'webhook',label:'Submit Request',x:80,y:150,config:{method:'POST'}},
    {id:'t2',type:'human',label:'Manager Review',x:300,y:150,config:{approver:'manager',message:'Please review this request'}},
    {id:'t3',type:'condition',label:'Approved?',x:520,y:150,config:{field:'approved',operator:'==',value:'true'}},
    {id:'t4',type:'email',label:'Confirm',x:740,y:80,config:{to:'{{requester}}',subject:'Approved!'}},
    {id:'t5',type:'email',label:'Reject',x:740,y:220,config:{to:'{{requester}}',subject:'Rejected'}},
  ], edges: [{id:'e1',source:'t1',target:'t2'},{id:'e2',source:'t2',target:'t3'},{id:'e3',source:'t3',target:'t4'},{id:'e4',source:'t3',target:'t5'}]},
  { name: 'My First Workflow', desc: 'Beginner: give an instruction, agent does work, get notified', nodes: [
    {id:'t1',type:'prompt',label:'Your Instruction',x:120,y:150,config:{prompt:'Write a summary of today\'s tech news'}},
    {id:'t2',type:'agent',label:'Researcher',x:380,y:150,config:{agent_name:'researcher',task:'Execute the instruction'}},
    {id:'t3',type:'notification',label:'Notify Me',x:640,y:150,config:{channel:'discord',message:'Done! Check your results.'}},
  ], edges: [{id:'e1',source:'t1',target:'t2'},{id:'e2',source:'t2',target:'t3'}]},
];

// Editor state v3.
let wfEditorState = {
  workflowId: null,
  workflowName: '',
  nodes: [],
  edges: [],
  dragging: null,
  connecting: null,
  wirePreview: null,
  selectedNode: null,
  selectedNodes: new Set(),   // multi-select
  clipboard: [],              // copy/paste buffer
  panOffset: { x: 0, y: 0 },
  panning: false,
  panStart: { x: 0, y: 0 },
  zoom: 1,
  executionHistory: [],
  undoStack: [],
  snapToGrid: true,           // snap nodes to 20px grid
  rubberBand: null,            // { x1, y1, x2, y2 } selection rectangle
  paletteSearch: '',           // filter palette items
  stickyNotes: [],             // canvas annotations
  workflowVars: {},            // global variables
  nodeOutputs: {},             // last run outputs per node
  executingNodes: new Set(),   // currently executing (for animation)
};

// ── Workflow List Page ──────────────────────────────────────────────

async function renderWorkflowsPage(container) {
  let workflows = [];
  try {
    const r = await fetch('/v1/workflows');
    const d = await r.json();
    workflows = d.pipelines || [];
  } catch (e) { console.error(e); }

  const statusColors = { active: '#22c55e', paused: '#f59e0b', draft: '#6b7280' };

  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);display:flex;align-items:center;">Agent Workflows (${workflows.length})${helpIcon('Build visual node-based workflows to orchestrate agents with drag-and-drop. Connect prompts, APIs, conditions, and actions.')}</h3>
        <button class="btn-primary" onclick="showCreateWorkflowModal()">+ New Workflow</button>
      </div>

      <!-- Templates -->
      <div style="margin-bottom:1.5rem;">
        <div style="font-size:12px;font-weight:600;color:var(--text-muted);margin-bottom:8px;text-transform:uppercase;letter-spacing:0.5px;">Quick Start Templates</div>
        <div style="display:flex;gap:10px;overflow-x:auto;padding-bottom:6px;">
          ${WORKFLOW_TEMPLATES.map((t, i) => `
            <div style="min-width:200px;padding:12px;background:var(--bg-card);border:1px solid var(--border);border-radius:8px;cursor:pointer;transition:border-color 0.2s;"
                 onmouseenter="this.style.borderColor='var(--accent)'" onmouseleave="this.style.borderColor='var(--border)'"
                 onclick="createFromTemplate(${i})">
              <div style="font-weight:600;font-size:13px;color:var(--text-primary);margin-bottom:4px;">${escapeHtml(t.name)}</div>
              <div style="font-size:11px;color:var(--text-muted);line-height:1.4;">${escapeHtml(t.desc)}</div>
              <div style="margin-top:6px;font-size:10px;color:var(--accent);">${t.nodes.length} nodes &bull; ${t.edges.length} connections</div>
            </div>
          `).join('')}
        </div>
      </div>

      ${workflows.length === 0 ? '<div class="empty-state">No workflows yet. Create one or use a template above.</div>' :
        `<div style="display:flex;flex-direction:column;gap:4px;">${workflows.map(w => {
          const nc = (w.nodes || []).length;
          const ec = (w.edges || []).length;
          const sc = statusColors[w.status] || '#6b7280';
          return `
          <div style="display:flex;align-items:center;gap:10px;padding:8px 12px;background:var(--bg-card);border:1px solid var(--border);border-left:3px solid ${sc};border-radius:6px;cursor:pointer;transition:border-color 0.15s;"
               onclick="openWorkflowEditor(${w.id})"
               onmouseenter="this.style.borderColor='var(--accent)'" onmouseleave="this.style.borderColor='var(--border)';this.style.borderLeftColor='${sc}'">
            <div style="flex:1;min-width:0;">
              <span style="font-weight:600;font-size:13px;color:var(--text-primary);">${escapeHtml(w.name)}</span>
              ${w.description ? `<span style="font-size:11px;color:var(--text-muted);margin-left:8px;">${escapeHtml(w.description.substring(0, 50))}</span>` : ''}
            </div>
            <span style="font-size:10px;color:var(--text-muted);">${nc}n ${ec}c</span>
            <span style="padding:2px 6px;border-radius:3px;font-size:9px;font-weight:600;background:${sc};color:#fff;">${(w.status || 'draft').toUpperCase()}</span>
            <button class="btn-sm" onclick="event.stopPropagation();openWorkflowEditor(${w.id})" style="font-size:10px;padding:2px 8px;">Edit</button>
            <button class="btn-sm btn-danger" onclick="event.stopPropagation();deleteWorkflow(${w.id})" style="font-size:10px;padding:2px 6px;">Del</button>
          </div>`;
        }).join('')}</div>`}
    </div>
    <div id="wf-create-modal" class="modal-overlay" style="display:none;"></div>
  `;
}

function showCreateWorkflowModal() {
  const m = document.getElementById('wf-create-modal');
  if (!m) return;
  m.style.display = 'flex';
  m.innerHTML = `
    <div class="modal-card" style="max-width:420px;width:90%;">
      <h3 style="margin:0 0 1rem;color:var(--text-primary);">New Workflow</h3>
      <div class="form-group"><label>Name *</label><input id="wf-name" class="input" placeholder="My Agent Workflow"></div>
      <div class="form-group"><label>Description</label><input id="wf-desc" class="input" placeholder="What this workflow does"></div>
      <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
        <button class="btn-sm" onclick="document.getElementById('wf-create-modal').style.display='none'">Cancel</button>
        <button class="btn-primary" onclick="createWorkflowAndOpen()">Create & Open</button>
      </div>
    </div>
  `;
}

async function createWorkflowAndOpen() {
  const name = document.getElementById('wf-name')?.value;
  if (!name) { alert('Name required'); return; }
  const r = await fetch('/v1/workflows', {
    method: 'POST', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({
      name,
      description: document.getElementById('wf-desc')?.value || '',
      status: 'draft',
      nodes: [{ id: '_start', type: 'prompt', label: 'Start Prompt', x: 200, y: 200, config: { prompt: '' } }],
      edges: [],
    })
  });
  const d = await r.json();
  document.getElementById('wf-create-modal').style.display = 'none';
  if (d.id) openWorkflowEditor(d.id);
}

async function createFromTemplate(idx) {
  const t = WORKFLOW_TEMPLATES[idx];
  if (!t) return;
  const r = await fetch('/v1/workflows', {
    method: 'POST', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({
      name: t.name, description: t.desc, status: 'draft',
      nodes: JSON.parse(JSON.stringify(t.nodes)),
      edges: JSON.parse(JSON.stringify(t.edges)),
    })
  });
  const d = await r.json();
  if (d.id) openWorkflowEditor(d.id);
}

async function deleteWorkflow(id) {
  if (!confirm('Delete this workflow?')) return;
  await fetch('/v1/workflows/' + id, { method: 'DELETE' });
  renderWorkflowsPage(document.getElementById('page-content'));
}

// ── Workflow Canvas Editor v2 ───────────────────────────────────────

async function openWorkflowEditor(id) {
  const r = await fetch('/v1/workflows/' + id);
  const wf = await r.json();
  wfEditorState.workflowId = id;
  wfEditorState.workflowName = wf.name || 'Untitled';
  wfEditorState.nodes = wf.nodes || [];
  wfEditorState.edges = wf.edges || [];
  wfEditorState.selectedNode = null;
  wfEditorState.panOffset = { x: 0, y: 0 };
  wfEditorState.zoom = 1;
  wfEditorState.wirePreview = null;
  renderWorkflowCanvas();
}

function renderWorkflowCanvas() {
  const container = document.getElementById('page-content');
  if (!container) return;

  // Group nodes by category for palette.
  const groups = {};
  WORKFLOW_NODE_TYPES.forEach(nt => {
    if (!groups[nt.group]) groups[nt.group] = [];
    groups[nt.group].push(nt);
  });
  const groupOrder = ['Core', 'Triggers', 'Actions', 'Logic'];
  const search = wfEditorState.paletteSearch.toLowerCase();

  container.innerHTML = `
    <div style="display:flex;height:calc(100vh - 60px);overflow:hidden;" id="wf-editor-root" tabindex="0">
      <!-- Left: Grouped Node Palette with Search -->
      <div id="wf-palette" style="width:180px;background:var(--bg-card);border-right:1px solid var(--border);padding:10px;overflow-y:auto;flex-shrink:0;">
        <input id="wf-palette-search" class="input" placeholder="Search nodes..." value="${escapeHtml(wfEditorState.paletteSearch)}"
               oninput="wfEditorState.paletteSearch=this.value;renderWorkflowCanvas()"
               style="margin-bottom:8px;font-size:11px;padding:5px 8px;">
        ${groupOrder.map(g => {
          const items = (groups[g] || []).filter(nt => !search || nt.label.toLowerCase().includes(search) || nt.type.includes(search));
          if (items.length === 0) return '';
          return `
          <div style="font-weight:600;font-size:10px;color:var(--text-muted);margin:8px 0 4px;text-transform:uppercase;letter-spacing:0.6px;">${g}</div>
          ${items.map(nt => `
            <div class="wf-palette-item" onmousedown="startPaletteDrag(event,'${nt.type}')"
                 title="${nt.tip || nt.label}"
                 style="display:flex;align-items:center;gap:6px;padding:5px 8px;margin-bottom:2px;border-radius:6px;cursor:grab;
                 background:rgba(255,255,255,0.02);border:1px solid transparent;transition:all 0.15s;font-size:11px;"
                 onmouseenter="this.style.borderColor='${nt.color}';this.style.background='rgba(255,255,255,0.06)'"
                 onmouseleave="this.style.borderColor='transparent';this.style.background='rgba(255,255,255,0.02)'">
              <span style="font-size:13px;width:18px;text-align:center;">${nt.icon}</span>
              <span style="color:var(--text-primary);font-weight:500;">${nt.label}</span>
            </div>
          `).join('')}`;
        }).join('')}
      </div>

      <!-- Center: Canvas -->
      <div style="flex:1;display:flex;flex-direction:column;overflow:hidden;">
        <!-- Toolbar Row 1 -->
        <div style="display:flex;align-items:center;gap:6px;padding:5px 10px;background:var(--bg-card);border-bottom:1px solid var(--border);flex-shrink:0;flex-wrap:wrap;">
          <button class="btn-sm" onclick="renderWorkflowsPage(document.getElementById('page-content'))">← Back</button>
          <span style="font-weight:600;color:var(--text-primary);font-size:13px;">${escapeHtml(wfEditorState.workflowName)}</span>
          <span style="font-size:10px;color:var(--text-muted);">${wfEditorState.nodes.length}n ${wfEditorState.edges.length}c</span>
          <div style="flex:1;"></div>
          <!-- Snap toggle -->
          <button class="btn-sm" onclick="toggleSnap()" style="${wfEditorState.snapToGrid ? 'background:var(--accent);color:#fff;' : ''}" title="Snap to Grid">
            🧲 ${wfEditorState.snapToGrid ? 'On' : 'Off'}
          </button>
          <!-- Auto Layout -->
          <button class="btn-sm" onclick="autoLayoutNodes()" title="Auto Layout">📐</button>
          <!-- Variables -->
          <button class="btn-sm" onclick="showVariablesPanel()" title="Variables">📋 Vars</button>
          <!-- Import/Export -->
          <button class="btn-sm" onclick="exportWorkflow()" title="Export JSON">📤</button>
          <button class="btn-sm" onclick="importWorkflow()" title="Import JSON">📥</button>
          <!-- Zoom controls -->
          <div style="display:flex;align-items:center;gap:2px;background:rgba(255,255,255,0.05);border-radius:6px;padding:2px;">
            <button class="btn-sm" onclick="wfZoom(-0.1)" style="padding:2px 6px;font-size:13px;">−</button>
            <span id="wf-zoom-label" style="font-size:10px;color:var(--text-muted);padding:0 3px;min-width:30px;text-align:center;">${Math.round(wfEditorState.zoom * 100)}%</span>
            <button class="btn-sm" onclick="wfZoom(0.1)" style="padding:2px 6px;font-size:13px;">+</button>
            <button class="btn-sm" onclick="wfZoomReset()" style="padding:2px 5px;font-size:10px;">Fit</button>
          </div>
          <button class="btn-sm" onclick="saveWorkflow()" style="background:var(--info);color:#fff;">💾</button>
          <button class="btn-sm" onclick="executeWorkflowAnimated()" style="background:var(--success);color:#fff;">▶ Run</button>
        </div>

        <!-- Canvas area with dot grid -->
        <div id="wf-canvas-wrap" style="flex:1;position:relative;overflow:hidden;background:#0d1117;cursor:default;"
             onmousedown="canvasMouseDown(event)" onmousemove="canvasMouseMove(event)" onmouseup="canvasMouseUp(event)"
             onclick="canvasClick(event)" onwheel="canvasWheel(event)" oncontextmenu="event.preventDefault();">
          <!-- Dot grid pattern -->
          <svg style="position:absolute;top:0;left:0;width:100%;height:100%;pointer-events:none;z-index:0;">
            <defs><pattern id="wf-dots" x="0" y="0" width="20" height="20" patternUnits="userSpaceOnUse">
              <circle cx="10" cy="10" r="1" fill="rgba(255,255,255,0.08)"/>
            </pattern></defs>
            <rect width="100%" height="100%" fill="url(#wf-dots)"/>
          </svg>
          <svg id="wf-svg" style="position:absolute;top:0;left:0;width:100%;height:100%;pointer-events:none;z-index:1;"></svg>
          <div id="wf-nodes-layer" style="position:absolute;top:0;left:0;width:100%;height:100%;z-index:2;"></div>
          <!-- Rubber band selection rectangle -->
          <div id="wf-rubber-band" style="position:absolute;border:1px dashed rgba(99,102,241,0.7);background:rgba(99,102,241,0.1);display:none;z-index:8;pointer-events:none;"></div>
          <!-- Minimap -->
          <div id="wf-minimap" style="position:absolute;bottom:8px;right:8px;width:130px;height:90px;background:rgba(0,0,0,0.6);border:1px solid rgba(255,255,255,0.15);border-radius:6px;z-index:10;overflow:hidden;pointer-events:none;"></div>
        </div>
      </div>

      <!-- Right: Config Panel -->
      <div id="wf-config-panel" style="width:260px;background:var(--bg-card);border-left:1px solid var(--border);padding:12px;overflow-y:auto;flex-shrink:0;display:${wfEditorState.selectedNode ? 'block' : 'none'};">
      </div>
    </div>
    <!-- Hidden file input for import -->
    <input id="wf-import-input" type="file" accept=".json" style="display:none;" onchange="handleWorkflowImport(event)">
    <!-- Variables modal -->
    <div id="wf-vars-modal" class="modal-overlay" style="display:none;"></div>
  `;

  // Keyboard shortcuts.
  document.addEventListener('keydown', wfKeyHandler);

  renderCanvasNodes();
  renderCanvasEdges();
  renderMinimap();
  if (wfEditorState.selectedNode) renderNodeConfig(wfEditorState.selectedNode);
}

function renderCanvasNodes() {
  const layer = document.getElementById('wf-nodes-layer');
  if (!layer) return;
  const ox = wfEditorState.panOffset.x;
  const oy = wfEditorState.panOffset.y;
  const z = wfEditorState.zoom;

  layer.style.transform = `scale(${z})`;
  layer.style.transformOrigin = '0 0';

  // Empty canvas guide for beginners.
  if (wfEditorState.nodes.length === 0) {
    layer.innerHTML = `
      <div style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);text-align:center;color:var(--text-muted);pointer-events:none;">
        <div style="font-size:40px;margin-bottom:12px;opacity:0.4;">🧩</div>
        <div style="font-size:15px;font-weight:600;margin-bottom:6px;">Start building your workflow</div>
        <div style="font-size:12px;line-height:1.5;">Drag a node from the palette on the left,<br>or click the canvas to place one.</div>
      </div>`;
    return;
  }

  layer.innerHTML = wfEditorState.nodes.map(n => {
    const nt = WORKFLOW_NODE_TYPES.find(t => t.type === n.type) || { icon: '?', label: n.type, color: '#666' };
    const selected = wfEditorState.selectedNode === n.id || wfEditorState.selectedNodes.has(n.id);
    const executing = wfEditorState.executingNodes.has(n.id);
    const output = wfEditorState.nodeOutputs[n.id];
    const isNote = n.type === 'note';
    const isCondition = n.type === 'condition';

    let glow = 'box-shadow:0 4px 12px rgba(0,0,0,0.4);';
    if (executing) glow = `box-shadow:0 0 0 3px #22c55e,0 0 30px rgba(34,197,94,0.5);animation:wf-pulse 0.8s ease-in-out infinite;`;
    else if (selected) glow = `box-shadow:0 0 0 2px ${nt.color},0 0 20px ${nt.color}40,0 4px 12px rgba(0,0,0,0.4);`;

    // Note type renders as a sticky note.
    if (isNote) {
      return `
        <div class="wf-node wf-note" data-id="${n.id}"
             onmousedown="nodeMouseDown(event,'${n.id}')" oncontextmenu="nodeContextMenu(event,'${n.id}')"
             style="position:absolute;left:${n.x + ox/z}px;top:${n.y + oy/z}px;width:180px;
                    background:#fef3c7;border:2px solid ${selected ? '#f59e0b' : '#fbbf24'};border-radius:4px;
                    cursor:grab;user-select:none;z-index:3;${glow}transform:rotate(-1deg);">
          <div style="padding:8px 10px;font-size:11px;color:#78350f;white-space:pre-wrap;min-height:30px;">${escapeHtml(n.config?.text || n.label)}</div>
        </div>`;
    }

    return `
      <div class="wf-node" data-id="${n.id}"
           onmousedown="nodeMouseDown(event,'${n.id}')" oncontextmenu="nodeContextMenu(event,'${n.id}')"
           style="position:absolute;left:${n.x + ox/z}px;top:${n.y + oy/z}px;width:160px;
                  background:#1a1f2e;border:2px solid ${selected ? '#fff' : nt.color};border-radius:8px;
                  cursor:grab;user-select:none;z-index:3;${glow}transition:box-shadow 0.2s;">
        <div style="display:flex;align-items:center;gap:6px;padding:7px 10px;border-bottom:1px solid rgba(255,255,255,0.06);">
          <span style="font-size:13px;">${nt.icon}</span>
          <span style="font-size:11px;font-weight:600;color:#e5e7eb;flex:1;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${escapeHtml(n.label)}</span>
        </div>
        <div style="padding:5px 10px;font-size:10px;color:#9ca3af;">
          ${getNodePreview(n)}
        </div>
        ${output ? `<div style="padding:3px 10px 5px;font-size:9px;color:#22c55e;border-top:1px solid rgba(34,197,94,0.2);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${escapeHtml(JSON.stringify(output).substring(0,200))}">✓ ${typeof output === 'string' ? escapeHtml(output.substring(0,40)) : 'Output ready'}</div>` : ''}
        <!-- Input port (left) -->
        <div class="wf-port wf-port-in" data-node="${n.id}" data-port="in"
             onmousedown="portMouseDown(event,'${n.id}','in')"
             style="position:absolute;left:-7px;top:50%;transform:translateY(-50%);width:14px;height:14px;
                    background:${nt.color};border:2px solid #0d1117;border-radius:50%;cursor:crosshair;z-index:5;
                    transition:transform 0.15s;"></div>
        ${isCondition ? `
        <!-- Condition: TRUE output port -->
        <div class="wf-port wf-port-out" data-node="${n.id}" data-port="out_true"
             onmousedown="portMouseDown(event,'${n.id}','out_true')"
             style="position:absolute;right:-7px;top:30%;transform:translateY(-50%);width:14px;height:14px;
                    background:#22c55e;border:2px solid #0d1117;border-radius:50%;cursor:crosshair;z-index:5;
                    transition:transform 0.15s;" title="True"></div>
        <span style="position:absolute;right:12px;top:30%;transform:translateY(-50%);font-size:8px;color:#22c55e;font-weight:700;">T</span>
        <!-- Condition: FALSE output port -->
        <div class="wf-port wf-port-out" data-node="${n.id}" data-port="out_false"
             onmousedown="portMouseDown(event,'${n.id}','out_false')"
             style="position:absolute;right:-7px;top:70%;transform:translateY(-50%);width:14px;height:14px;
                    background:#ef4444;border:2px solid #0d1117;border-radius:50%;cursor:crosshair;z-index:5;
                    transition:transform 0.15s;" title="False"></div>
        <span style="position:absolute;right:12px;top:70%;transform:translateY(-50%);font-size:8px;color:#ef4444;font-weight:700;">F</span>
        ` : `
        <!-- Standard output port (right) -->
        <div class="wf-port wf-port-out" data-node="${n.id}" data-port="out"
             onmousedown="portMouseDown(event,'${n.id}','out')"
             style="position:absolute;right:-7px;top:50%;transform:translateY(-50%);width:14px;height:14px;
                    background:${nt.color};border:2px solid #0d1117;border-radius:50%;cursor:crosshair;z-index:5;
                    transition:transform 0.15s;"></div>
        `}
      </div>`;
  }).join('');
}

function getNodePreview(node) {
  const c = node.config || {};
  switch (node.type) {
    case 'prompt':    return c.prompt ? escapeHtml(c.prompt.substring(0, 40)) + '...' : 'No prompt set';
    case 'agent':     return c.agent_name ? 'Agent: ' + escapeHtml(c.agent_name) : 'No agent selected';
    case 'webhook':   return c.url ? escapeHtml(c.url.substring(0, 30)) : 'No URL';
    case 'api_call':  return c.method ? c.method + ' ' + (c.url || '').substring(0, 20) : 'No request';
    case 'websocket': return c.url ? 'ws: ' + escapeHtml(c.url.substring(0, 25)) : 'No URL';
    case 'condition': return c.field ? c.field + ' ' + (c.operator || '==') + ' ' + (c.value || '') : 'No condition';
    case 'transform': return c.template ? escapeHtml(c.template.substring(0, 30)) : 'No template';
    case 'delay':     return c.seconds ? c.seconds + ' seconds' : 'No delay';
    case 'loop':      return c.array_field ? 'Each: ' + escapeHtml(c.array_field) : 'No array field';
    case 'merge':     return 'Merges ' + (c.mode || 'append');
    case 'code':      return c.language ? c.language + ' script' : 'No code';
    case 'error_handler': return c.on_error || 'continue';
    case 'schedule':  return c.cron || 'No schedule';
    case 'db_query':  return c.query ? escapeHtml(c.query.substring(0, 30)) : 'No query';
    case 'email':     return c.to ? 'To: ' + escapeHtml(c.to.substring(0, 25)) : 'No recipient';
    case 'notification': return c.channel || 'No channel';
    case 'human':     return c.approver ? 'Approver: ' + escapeHtml(c.approver) : 'No approver';
    case 'note':      return c.text ? escapeHtml(c.text.substring(0, 40)) : 'Empty note';
    default:          return node.type;
  }
}

function renderCanvasEdges() {
  const svg = document.getElementById('wf-svg');
  if (!svg) return;
  const ox = wfEditorState.panOffset.x;
  const oy = wfEditorState.panOffset.y;
  const z = wfEditorState.zoom;
  const nodeMap = {};
  wfEditorState.nodes.forEach(n => { nodeMap[n.id] = n; });

  svg.style.transform = `scale(${z})`;
  svg.style.transformOrigin = '0 0';

  let paths = '';

  // Render established edges.
  wfEditorState.edges.forEach(e => {
    const src = nodeMap[e.source];
    const tgt = nodeMap[e.target];
    if (!src || !tgt) return;

    const x1 = src.x + ox/z + 160;
    // Conditional port Y positions: out_true=30%, out_false=70%, default=50%
    const srcH = 70; // estimated node height
    let y1pct = 0.5;
    if (e.sourcePort === 'out_true') y1pct = 0.3;
    else if (e.sourcePort === 'out_false') y1pct = 0.7;
    const y1 = src.y + oy/z + srcH * y1pct;
    const x2 = tgt.x + ox/z;
    const y2 = tgt.y + oy/z + 35;
    const dx = Math.max(Math.abs(x2 - x1) * 0.5, 50);
    let srcColor = WORKFLOW_NODE_TYPES.find(t => t.type === src.type)?.color || '#666';
    if (e.sourcePort === 'out_true') srcColor = '#22c55e';
    else if (e.sourcePort === 'out_false') srcColor = '#ef4444';

    paths += `<path d="M${x1},${y1} C${x1+dx},${y1} ${x2-dx},${y2} ${x2},${y2}"
               stroke="${srcColor}" stroke-width="2.5" fill="none" stroke-opacity="0.6"
               style="pointer-events:stroke;cursor:pointer;"
               onclick="deleteEdge('${e.id}')"/>`;
    // Arrow head.
    paths += `<circle cx="${x2}" cy="${y2}" r="4" fill="${srcColor}" fill-opacity="0.6"/>`;
  });

  // Live wire preview (dashed line following cursor during drag).
  if (wfEditorState.wirePreview) {
    const wp = wfEditorState.wirePreview;
    const dx = Math.max(Math.abs(wp.x2 - wp.x1) * 0.5, 50);
    paths += `<path d="M${wp.x1},${wp.y1} C${wp.x1+dx},${wp.y1} ${wp.x2-dx},${wp.y2} ${wp.x2},${wp.y2}"
               stroke="#fff" stroke-width="2" fill="none" stroke-opacity="0.5"
               stroke-dasharray="6,4"/>`;
  }

  svg.innerHTML = paths;
}

// ── Canvas interaction v2 (live wire dragging) ──────────────────────

let paletteDragType = null;

function startPaletteDrag(event, type) {
  paletteDragType = type;
}

// Port mousedown starts wire dragging from output or input.
function portMouseDown(event, nodeId, portType) {
  event.stopPropagation();
  event.preventDefault();
  const node = wfEditorState.nodes.find(n => n.id === nodeId);
  if (!node) return;
  const z = wfEditorState.zoom;
  const ox = wfEditorState.panOffset.x;
  const oy = wfEditorState.panOffset.y;
  const nodeH = 70;

  if (portType.startsWith('out')) {
    let y1pct = 0.5;
    if (portType === 'out_true') y1pct = 0.3;
    else if (portType === 'out_false') y1pct = 0.7;
    const x1 = node.x + ox/z + 160;
    const y1 = node.y + oy/z + nodeH * y1pct;
    wfEditorState.connecting = { sourceNodeId: nodeId, portType };
    wfEditorState.wirePreview = { x1, y1, x2: x1, y2: y1 };
    event.target.style.transform = 'translateY(-50%) scale(1.5)';
  } else if (portType === 'in') {
    const x1 = node.x + ox/z;
    const y1 = node.y + oy/z + 35;
    wfEditorState.connecting = { sourceNodeId: nodeId, portType: 'in' };
    wfEditorState.wirePreview = { x1, y1, x2: x1, y2: y1 };
    event.target.style.transform = 'translateY(-50%) scale(1.5)';
  }
}

function canvasClick(event) {
  if (event.target.closest('.wf-node') || event.target.closest('.wf-port')) return;
  // If we have a palette type selected, place a node.
  if (paletteDragType) {
    const canvas = document.getElementById('wf-canvas-wrap');
    const rect = canvas.getBoundingClientRect();
    const z = wfEditorState.zoom;
    let x = (event.clientX - rect.left) / z - wfEditorState.panOffset.x / z;
    let y = (event.clientY - rect.top) / z - wfEditorState.panOffset.y / z;
    if (wfEditorState.snapToGrid) { x = Math.round(x / 20) * 20; y = Math.round(y / 20) * 20; }
    const nt = WORKFLOW_NODE_TYPES.find(t => t.type === paletteDragType);
    const newNode = {
      id: 'n_' + Date.now(),
      type: paletteDragType,
      label: nt ? nt.label : paletteDragType,
      x: Math.round(x),
      y: Math.round(y),
      config: {},
    };
    wfEditorState.nodes.push(newNode);
    paletteDragType = null;
    renderCanvasNodes();
    renderCanvasEdges();
    renderMinimap();
    return;
  }
  // Deselect node(s) if clicking empty space.
  wfEditorState.selectedNode = null;
  wfEditorState.selectedNodes.clear();
  wfEditorState.connecting = null;
  wfEditorState.wirePreview = null;
  const panel = document.getElementById('wf-config-panel');
  if (panel) panel.style.display = 'none';
  renderCanvasNodes();
  renderCanvasEdges();
}

function nodeMouseDown(event, nodeId) {
  event.stopPropagation();
  if (event.target.closest('.wf-port')) return;

  // Ctrl+click for multi-select.
  if (event.ctrlKey || event.metaKey) {
    if (wfEditorState.selectedNodes.has(nodeId)) {
      wfEditorState.selectedNodes.delete(nodeId);
    } else {
      wfEditorState.selectedNodes.add(nodeId);
    }
    renderCanvasNodes();
    return;
  }

  wfEditorState.selectedNode = nodeId;
  wfEditorState.selectedNodes.clear();
  wfEditorState.selectedNodes.add(nodeId);
  renderNodeConfig(nodeId);
  const panel = document.getElementById('wf-config-panel');
  if (panel) panel.style.display = 'block';

  const node = wfEditorState.nodes.find(n => n.id === nodeId);
  if (!node) return;
  const canvas = document.getElementById('wf-canvas-wrap');
  const rect = canvas.getBoundingClientRect();
  const z = wfEditorState.zoom;
  wfEditorState.dragging = {
    nodeId,
    offsetX: (event.clientX - rect.left) / z - node.x - wfEditorState.panOffset.x / z,
    offsetY: (event.clientY - rect.top) / z - node.y - wfEditorState.panOffset.y / z,
  };
  renderCanvasNodes();
}

function canvasMouseDown(event) {
  if (event.target.closest('.wf-node') || event.target.closest('.wf-port')) return;
  // Shift+click starts rubber band selection.
  if (event.shiftKey) {
    const canvas = document.getElementById('wf-canvas-wrap');
    const rect = canvas.getBoundingClientRect();
    const x = event.clientX - rect.left;
    const y = event.clientY - rect.top;
    wfEditorState.rubberBand = { x1: x, y1: y, x2: x, y2: y };
    return;
  }
  wfEditorState.panning = true;
  wfEditorState.panStart = { x: event.clientX - wfEditorState.panOffset.x, y: event.clientY - wfEditorState.panOffset.y };
}

function canvasMouseMove(event) {
  const canvas = document.getElementById('wf-canvas-wrap');
  if (!canvas) return;
  const rect = canvas.getBoundingClientRect();
  const z = wfEditorState.zoom;

  // Rubber band selection.
  if (wfEditorState.rubberBand) {
    const rb = wfEditorState.rubberBand;
    rb.x2 = event.clientX - rect.left;
    rb.y2 = event.clientY - rect.top;
    const rbEl = document.getElementById('wf-rubber-band');
    if (rbEl) {
      rbEl.style.display = 'block';
      rbEl.style.left = Math.min(rb.x1, rb.x2) + 'px';
      rbEl.style.top = Math.min(rb.y1, rb.y2) + 'px';
      rbEl.style.width = Math.abs(rb.x2 - rb.x1) + 'px';
      rbEl.style.height = Math.abs(rb.y2 - rb.y1) + 'px';
    }
    return;
  }

  // Live wire preview.
  if (wfEditorState.connecting && wfEditorState.wirePreview) {
    const mx = (event.clientX - rect.left) / z;
    const my = (event.clientY - rect.top) / z;
    wfEditorState.wirePreview.x2 = mx;
    wfEditorState.wirePreview.y2 = my;
    renderCanvasEdges();
    return;
  }

  if (wfEditorState.dragging) {
    const node = wfEditorState.nodes.find(n => n.id === wfEditorState.dragging.nodeId);
    if (node) {
      let nx = (event.clientX - rect.left) / z - wfEditorState.dragging.offsetX - wfEditorState.panOffset.x / z;
      let ny = (event.clientY - rect.top) / z - wfEditorState.dragging.offsetY - wfEditorState.panOffset.y / z;
      if (wfEditorState.snapToGrid) { nx = Math.round(nx / 20) * 20; ny = Math.round(ny / 20) * 20; }
      // Move all selected nodes together.
      const dx = nx - node.x;
      const dy = ny - node.y;
      if (wfEditorState.selectedNodes.size > 1) {
        wfEditorState.selectedNodes.forEach(nid => {
          const sn = wfEditorState.nodes.find(nn => nn.id === nid);
          if (sn) { sn.x += dx; sn.y += dy; }
        });
      } else {
        node.x = nx;
        node.y = ny;
      }
      renderCanvasNodes();
      renderCanvasEdges();
    }
  } else if (wfEditorState.panning) {
    wfEditorState.panOffset.x = event.clientX - wfEditorState.panStart.x;
    wfEditorState.panOffset.y = event.clientY - wfEditorState.panStart.y;
    renderCanvasNodes();
    renderCanvasEdges();
  }
}

function canvasMouseUp(event) {
  // Rubber band: select nodes inside rectangle.
  if (wfEditorState.rubberBand) {
    const rb = wfEditorState.rubberBand;
    const canvas = document.getElementById('wf-canvas-wrap');
    const rect = canvas.getBoundingClientRect();
    const z = wfEditorState.zoom;
    const ox = wfEditorState.panOffset.x;
    const oy = wfEditorState.panOffset.y;
    const minX = Math.min(rb.x1, rb.x2) / z - ox / z;
    const maxX = Math.max(rb.x1, rb.x2) / z - ox / z;
    const minY = Math.min(rb.y1, rb.y2) / z - oy / z;
    const maxY = Math.max(rb.y1, rb.y2) / z - oy / z;
    wfEditorState.selectedNodes.clear();
    wfEditorState.nodes.forEach(n => {
      if (n.x >= minX && n.x + 160 <= maxX && n.y >= minY && n.y + 70 <= maxY) {
        wfEditorState.selectedNodes.add(n.id);
      }
    });
    wfEditorState.rubberBand = null;
    const rbEl = document.getElementById('wf-rubber-band');
    if (rbEl) rbEl.style.display = 'none';
    renderCanvasNodes();
    return;
  }

  // Wire drop: check if we dropped on a port.
  if (wfEditorState.connecting && wfEditorState.wirePreview) {
    const target = event.target.closest('.wf-port');
    if (target) {
      const targetNodeId = target.dataset.node;
      const targetPort = target.dataset.port;
      const srcId = wfEditorState.connecting.sourceNodeId;
      const srcPort = wfEditorState.connecting.portType;

      if (srcId !== targetNodeId && (srcPort.startsWith('out') ? targetPort === 'in' : targetPort.startsWith('out'))) {
        const source = srcPort.startsWith('out') ? srcId : targetNodeId;
        const tgt = srcPort.startsWith('out') ? targetNodeId : srcId;
        const sPort = srcPort.startsWith('out') ? srcPort : targetPort;
        const exists = wfEditorState.edges.some(e => e.source === source && e.target === tgt && e.sourcePort === sPort);
        if (!exists) {
          wfEditorState.edges.push({ id: 'e_' + Date.now(), source, target: tgt, sourcePort: sPort });
        }
      }
    }
    wfEditorState.connecting = null;
    wfEditorState.wirePreview = null;
    renderCanvasEdges();
    renderMinimap();
  }
  if (wfEditorState.dragging) renderMinimap();
  wfEditorState.dragging = null;
  wfEditorState.panning = false;
}

function deleteEdge(edgeId) {
  wfEditorState.edges = wfEditorState.edges.filter(e => e.id !== edgeId);
  renderCanvasEdges();
  renderMinimap();
}

// ── Zoom ────────────────────────────────────────────────────────────

function wfZoom(delta) {
  wfEditorState.zoom = Math.max(0.25, Math.min(2, wfEditorState.zoom + delta));
  const label = document.getElementById('wf-zoom-label');
  if (label) label.textContent = Math.round(wfEditorState.zoom * 100) + '%';
  renderCanvasNodes();
  renderCanvasEdges();
}

function wfZoomReset() {
  if (wfEditorState.nodes.length === 0) { wfEditorState.zoom = 1; wfZoom(0); return; }
  const xs = wfEditorState.nodes.map(n => n.x);
  const ys = wfEditorState.nodes.map(n => n.y);
  const minX = Math.min(...xs), maxX = Math.max(...xs) + 160;
  const minY = Math.min(...ys), maxY = Math.max(...ys) + 70;
  const canvas = document.getElementById('wf-canvas-wrap');
  if (!canvas) return;
  const cw = canvas.clientWidth, ch = canvas.clientHeight;
  const zx = cw / (maxX - minX + 100), zy = ch / (maxY - minY + 100);
  wfEditorState.zoom = Math.max(0.25, Math.min(1.5, Math.min(zx, zy)));
  wfEditorState.panOffset.x = (-minX + 50) * wfEditorState.zoom;
  wfEditorState.panOffset.y = (-minY + 50) * wfEditorState.zoom;
  const label = document.getElementById('wf-zoom-label');
  if (label) label.textContent = Math.round(wfEditorState.zoom * 100) + '%';
  renderCanvasNodes();
  renderCanvasEdges();
  renderMinimap();
}

function canvasWheel(event) {
  event.preventDefault();
  wfZoom(event.deltaY > 0 ? -0.05 : 0.05);
}

// ── Minimap ─────────────────────────────────────────────────────────

function renderMinimap() {
  const mm = document.getElementById('wf-minimap');
  if (!mm || wfEditorState.nodes.length === 0) { if (mm) mm.innerHTML = ''; return; }
  const xs = wfEditorState.nodes.map(n => n.x);
  const ys = wfEditorState.nodes.map(n => n.y);
  const minX = Math.min(...xs) - 20, maxX = Math.max(...xs) + 180;
  const minY = Math.min(...ys) - 20, maxY = Math.max(...ys) + 90;
  const w = maxX - minX || 1, h = maxY - minY || 1;
  const scale = Math.min(120 / w, 80 / h);
  mm.innerHTML = wfEditorState.nodes.map(n => {
    const nt = WORKFLOW_NODE_TYPES.find(t => t.type === n.type) || { color: '#666' };
    return `<div style="position:absolute;left:${(n.x-minX)*scale+5}px;top:${(n.y-minY)*scale+5}px;width:${Math.max(8,160*scale)}px;height:${Math.max(3,50*scale)}px;background:${nt.color};border-radius:2px;opacity:0.7;"></div>`;
  }).join('');
}

// ── Snap to Grid ────────────────────────────────────────────────────

function toggleSnap() {
  wfEditorState.snapToGrid = !wfEditorState.snapToGrid;
  if (wfEditorState.snapToGrid) {
    wfEditorState.nodes.forEach(n => { n.x = Math.round(n.x/20)*20; n.y = Math.round(n.y/20)*20; });
    renderCanvasNodes();
    renderCanvasEdges();
  }
  renderWorkflowCanvas();
}

// ── Auto Layout (DAG left-to-right) ────────────────────────────────

function autoLayoutNodes() {
  const nodes = wfEditorState.nodes;
  const edges = wfEditorState.edges;
  if (nodes.length === 0) return;

  // Build adjacency and in-degree.
  const adj = {};
  const inDeg = {};
  nodes.forEach(n => { adj[n.id] = []; inDeg[n.id] = 0; });
  edges.forEach(e => {
    if (adj[e.source]) adj[e.source].push(e.target);
    if (inDeg[e.target] !== undefined) inDeg[e.target]++;
  });

  // Topological sort layers using BFS (Kahn's algorithm).
  const layers = [];
  let queue = nodes.filter(n => inDeg[n.id] === 0).map(n => n.id);
  const visited = new Set();
  while (queue.length > 0) {
    layers.push([...queue]);
    queue.forEach(id => visited.add(id));
    const next = [];
    queue.forEach(id => {
      (adj[id] || []).forEach(tid => {
        inDeg[tid]--;
        if (inDeg[tid] === 0 && !visited.has(tid)) next.push(tid);
      });
    });
    queue = next;
  }
  // Add unvisited nodes as a last layer.
  const unvisited = nodes.filter(n => !visited.has(n.id)).map(n => n.id);
  if (unvisited.length) layers.push(unvisited);

  // Position nodes left-to-right, centered vertically.
  const xGap = 220, yGap = 100, startX = 80, startY = 80;
  layers.forEach((layer, li) => {
    layer.forEach((nid, ni) => {
      const n = nodes.find(nn => nn.id === nid);
      if (n) {
        n.x = startX + li * xGap;
        n.y = startY + ni * yGap;
        if (wfEditorState.snapToGrid) { n.x = Math.round(n.x/20)*20; n.y = Math.round(n.y/20)*20; }
      }
    });
  });

  renderCanvasNodes();
  renderCanvasEdges();
  renderMinimap();
  wfZoomReset();
}

// ── Import / Export ─────────────────────────────────────────────────

function exportWorkflow() {
  const data = {
    name: wfEditorState.workflowName,
    nodes: wfEditorState.nodes,
    edges: wfEditorState.edges,
    variables: wfEditorState.workflowVars,
    exportedAt: new Date().toISOString(),
  };
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = (wfEditorState.workflowName || 'workflow').replace(/\s+/g, '_') + '.json';
  a.click();
  URL.revokeObjectURL(url);
}

function importWorkflow() {
  document.getElementById('wf-import-input')?.click();
}

function handleWorkflowImport(event) {
  const file = event.target.files[0];
  if (!file) return;
  const reader = new FileReader();
  reader.onload = (e) => {
    try {
      const data = JSON.parse(e.target.result);
      if (data.nodes) {
        // Generate new IDs to avoid collisions.
        const idMap = {};
        data.nodes.forEach(n => {
          const newId = 'n_' + Date.now() + '_' + Math.random().toString(36).substr(2, 4);
          idMap[n.id] = newId;
          n.id = newId;
        });
        (data.edges || []).forEach(edge => {
          edge.id = 'e_' + Date.now() + '_' + Math.random().toString(36).substr(2, 4);
          edge.source = idMap[edge.source] || edge.source;
          edge.target = idMap[edge.target] || edge.target;
        });
        wfEditorState.nodes.push(...data.nodes);
        wfEditorState.edges.push(...(data.edges || []));
        if (data.variables) Object.assign(wfEditorState.workflowVars, data.variables);
        renderCanvasNodes();
        renderCanvasEdges();
        renderMinimap();
      }
    } catch (err) { alert('Invalid JSON: ' + err.message); }
  };
  reader.readAsText(file);
  event.target.value = '';
}

// ── Variables Panel ─────────────────────────────────────────────────

function showVariablesPanel() {
  const modal = document.getElementById('wf-vars-modal');
  if (!modal) return;
  const vars = wfEditorState.workflowVars;
  const keys = Object.keys(vars);
  modal.style.display = 'flex';
  modal.innerHTML = `
    <div class="modal-content" style="max-width:400px;">
      <h4 style="margin:0 0 12px;color:var(--text-primary);">Workflow Variables</h4>
      <div style="font-size:11px;color:var(--text-muted);margin-bottom:10px;">Define global variables that any node can reference using {{variable_name}} syntax.</div>
      <div id="wf-vars-list">
        ${keys.map(k => `
          <div style="display:flex;gap:6px;margin-bottom:6px;align-items:center;">
            <input class="input" value="${escapeHtml(k)}" style="flex:1;font-size:11px;" onchange="renameWfVar('${escapeHtml(k)}',this.value)">
            <input class="input" value="${escapeHtml(vars[k])}" style="flex:2;font-size:11px;" onchange="wfEditorState.workflowVars['${escapeHtml(k)}']=this.value">
            <button class="btn-sm btn-danger" onclick="delete wfEditorState.workflowVars['${escapeHtml(k)}'];showVariablesPanel()">×</button>
          </div>
        `).join('')}
      </div>
      <div style="display:flex;gap:6px;margin-top:8px;">
        <button class="btn-sm" onclick="addWfVar()">+ Add Variable</button>
        <div style="flex:1;"></div>
        <button class="btn-sm btn-primary" onclick="document.getElementById('wf-vars-modal').style.display='none'">Close</button>
      </div>
    </div>
  `;
}

function addWfVar() {
  const key = 'var_' + Object.keys(wfEditorState.workflowVars).length;
  wfEditorState.workflowVars[key] = '';
  showVariablesPanel();
}

function renameWfVar(oldKey, newKey) {
  if (oldKey === newKey) return;
  wfEditorState.workflowVars[newKey] = wfEditorState.workflowVars[oldKey];
  delete wfEditorState.workflowVars[oldKey];
}

// ── Context Menu (Right-click) ──────────────────────────────────────

function nodeContextMenu(event, nodeId) {
  event.preventDefault();
  event.stopPropagation();
  // Remove existing context menu.
  document.querySelectorAll('.wf-context-menu').forEach(el => el.remove());

  const menu = document.createElement('div');
  menu.className = 'wf-context-menu';
  menu.style.cssText = `position:fixed;left:${event.clientX}px;top:${event.clientY}px;background:#1e293b;border:1px solid var(--border);border-radius:6px;padding:4px 0;z-index:1000;min-width:140px;box-shadow:0 8px 24px rgba(0,0,0,0.5);`;
  menu.innerHTML = `
    <div onclick="testSingleNode('${nodeId}');this.parentElement.remove()" style="padding:6px 12px;font-size:11px;color:var(--text-primary);cursor:pointer;display:flex;align-items:center;gap:6px;" onmouseenter="this.style.background='rgba(255,255,255,0.08)'" onmouseleave="this.style.background='transparent'">🧪 Test Node</div>
    <div onclick="duplicateNode('${nodeId}');this.parentElement.remove()" style="padding:6px 12px;font-size:11px;color:var(--text-primary);cursor:pointer;display:flex;align-items:center;gap:6px;" onmouseenter="this.style.background='rgba(255,255,255,0.08)'" onmouseleave="this.style.background='transparent'">📋 Duplicate</div>
    <div onclick="wfCopySingle('${nodeId}');this.parentElement.remove()" style="padding:6px 12px;font-size:11px;color:var(--text-primary);cursor:pointer;display:flex;align-items:center;gap:6px;" onmouseenter="this.style.background='rgba(255,255,255,0.08)'" onmouseleave="this.style.background='transparent'">📄 Copy</div>
    <div style="border-top:1px solid var(--border);margin:2px 0;"></div>
    <div onclick="deleteNodeDirect('${nodeId}');this.parentElement.remove()" style="padding:6px 12px;font-size:11px;color:#ef4444;cursor:pointer;display:flex;align-items:center;gap:6px;" onmouseenter="this.style.background='rgba(255,255,255,0.08)'" onmouseleave="this.style.background='transparent'">🗑️ Delete</div>
  `;
  document.body.appendChild(menu);
  const close = () => { menu.remove(); document.removeEventListener('click', close); };
  setTimeout(() => document.addEventListener('click', close), 100);
}

function duplicateNode(nodeId) {
  const orig = wfEditorState.nodes.find(n => n.id === nodeId);
  if (!orig) return;
  const dup = JSON.parse(JSON.stringify(orig));
  dup.id = 'n_' + Date.now();
  dup.x += 30;
  dup.y += 30;
  dup.label = orig.label + ' (copy)';
  wfEditorState.nodes.push(dup);
  renderCanvasNodes();
  renderCanvasEdges();
  renderMinimap();
}

function deleteNodeDirect(nodeId) {
  wfEditorState.nodes = wfEditorState.nodes.filter(n => n.id !== nodeId);
  wfEditorState.edges = wfEditorState.edges.filter(e => e.source !== nodeId && e.target !== nodeId);
  if (wfEditorState.selectedNode === nodeId) wfEditorState.selectedNode = null;
  wfEditorState.selectedNodes.delete(nodeId);
  renderCanvasNodes();
  renderCanvasEdges();
  renderMinimap();
}

function testSingleNode(nodeId) {
  const node = wfEditorState.nodes.find(n => n.id === nodeId);
  if (!node) return;
  wfEditorState.executingNodes.add(nodeId);
  renderCanvasNodes();
  setTimeout(() => {
    wfEditorState.executingNodes.delete(nodeId);
    wfEditorState.nodeOutputs[nodeId] = `Test OK at ${new Date().toLocaleTimeString()}`;
    renderCanvasNodes();
  }, 1500);
}

function wfCopySingle(nodeId) {
  const node = wfEditorState.nodes.find(n => n.id === nodeId);
  if (node) wfEditorState.clipboard = [JSON.parse(JSON.stringify(node))];
}

// ── Copy / Paste ────────────────────────────────────────────────────

function wfCopy() {
  const selected = [...wfEditorState.selectedNodes];
  if (selected.length === 0 && wfEditorState.selectedNode) selected.push(wfEditorState.selectedNode);
  wfEditorState.clipboard = selected.map(id => {
    const n = wfEditorState.nodes.find(nn => nn.id === id);
    return n ? JSON.parse(JSON.stringify(n)) : null;
  }).filter(Boolean);
}

function wfPaste() {
  if (wfEditorState.clipboard.length === 0) return;
  const idMap = {};
  const pasted = wfEditorState.clipboard.map(n => {
    const dup = JSON.parse(JSON.stringify(n));
    const newId = 'n_' + Date.now() + '_' + Math.random().toString(36).substr(2, 4);
    idMap[n.id] = newId;
    dup.id = newId;
    dup.x += 40;
    dup.y += 40;
    return dup;
  });
  // Copy edges between pasted nodes.
  const clipIds = new Set(wfEditorState.clipboard.map(n => n.id));
  wfEditorState.edges.filter(e => clipIds.has(e.source) && clipIds.has(e.target)).forEach(e => {
    wfEditorState.edges.push({
      id: 'e_' + Date.now() + '_' + Math.random().toString(36).substr(2, 4),
      source: idMap[e.source], target: idMap[e.target], sourcePort: e.sourcePort,
    });
  });
  wfEditorState.nodes.push(...pasted);
  wfEditorState.selectedNodes.clear();
  pasted.forEach(n => wfEditorState.selectedNodes.add(n.id));
  renderCanvasNodes();
  renderCanvasEdges();
  renderMinimap();
}

// ── Execution animation ─────────────────────────────────────────────

async function executeWorkflowAnimated() {
  // Build topological order.
  const nodes = wfEditorState.nodes;
  const edges = wfEditorState.edges;
  const adj = {};
  const inDeg = {};
  nodes.forEach(n => { adj[n.id] = []; inDeg[n.id] = 0; });
  edges.forEach(e => {
    if (adj[e.source]) adj[e.source].push(e.target);
    if (inDeg[e.target] !== undefined) inDeg[e.target]++;
  });
  const order = [];
  let queue = nodes.filter(n => inDeg[n.id] === 0).map(n => n.id);
  const visited = new Set();
  while (queue.length > 0) {
    order.push(...queue);
    queue.forEach(id => visited.add(id));
    const next = [];
    queue.forEach(id => {
      (adj[id] || []).forEach(tid => {
        inDeg[tid]--;
        if (inDeg[tid] === 0 && !visited.has(tid)) next.push(tid);
      });
    });
    queue = next;
  }
  // Add unvisited.
  nodes.forEach(n => { if (!visited.has(n.id)) order.push(n.id); });

  wfEditorState.nodeOutputs = {};
  wfEditorState.executingNodes.clear();

  // Also fire the backend execute.
  fetch('/v1/workflows/' + wfEditorState.workflowId + '/execute', { method: 'POST' }).catch(() => {});

  // Animate each node in order.
  for (const nid of order) {
    wfEditorState.executingNodes.add(nid);
    renderCanvasNodes();
    await new Promise(r => setTimeout(r, 500));
    wfEditorState.executingNodes.delete(nid);
    wfEditorState.nodeOutputs[nid] = `OK`;
    renderCanvasNodes();
  }

  // Record in execution history.
  wfEditorState.executionHistory.push({
    timestamp: new Date().toISOString(),
    nodeCount: order.length,
    status: 'completed',
  });
}

// ── Keyboard shortcuts v3 ───────────────────────────────────────────

function wfKeyHandler(event) {
  if (!document.getElementById('wf-editor-root')) {
    document.removeEventListener('keydown', wfKeyHandler);
    return;
  }
  if (event.target.tagName === 'INPUT' || event.target.tagName === 'TEXTAREA' || event.target.tagName === 'SELECT') return;

  if (event.key === 'Delete' || event.key === 'Backspace') {
    // Delete all selected nodes.
    const toDelete = new Set(wfEditorState.selectedNodes);
    if (wfEditorState.selectedNode) toDelete.add(wfEditorState.selectedNode);
    if (toDelete.size > 0) {
      wfEditorState.nodes = wfEditorState.nodes.filter(n => !toDelete.has(n.id));
      wfEditorState.edges = wfEditorState.edges.filter(e => !toDelete.has(e.source) && !toDelete.has(e.target));
      wfEditorState.selectedNode = null;
      wfEditorState.selectedNodes.clear();
      const panel = document.getElementById('wf-config-panel');
      if (panel) panel.style.display = 'none';
      renderCanvasNodes();
      renderCanvasEdges();
      renderMinimap();
      event.preventDefault();
    }
  } else if (event.key === 's' && event.ctrlKey) {
    event.preventDefault();
    saveWorkflow();
  } else if (event.key === 'c' && event.ctrlKey) {
    event.preventDefault();
    wfCopy();
  } else if (event.key === 'v' && event.ctrlKey) {
    event.preventDefault();
    wfPaste();
  } else if (event.key === 'a' && event.ctrlKey) {
    event.preventDefault();
    wfEditorState.selectedNodes.clear();
    wfEditorState.nodes.forEach(n => wfEditorState.selectedNodes.add(n.id));
    renderCanvasNodes();
  } else if (event.key === 'Escape') {
    wfEditorState.selectedNode = null;
    wfEditorState.selectedNodes.clear();
    wfEditorState.connecting = null;
    wfEditorState.wirePreview = null;
    paletteDragType = null;
    const panel = document.getElementById('wf-config-panel');
    if (panel) panel.style.display = 'none';
    renderCanvasNodes();
    renderCanvasEdges();
    document.querySelectorAll('.wf-context-menu').forEach(el => el.remove());
  }
}

// ── Node Config Panel ───────────────────────────────────────────────

function renderNodeConfig(nodeId) {
  const panel = document.getElementById('wf-config-panel');
  if (!panel) return;
  const node = wfEditorState.nodes.find(n => n.id === nodeId);
  if (!node) return;
  const cfg = node.config || {};
  const nt = WORKFLOW_NODE_TYPES.find(t => t.type === node.type) || { icon: '?', label: node.type, color: '#666' };

  let fields = '';
  switch (node.type) {
    case 'prompt':
      fields = `
        <div class="form-group"><label>Prompt / Instruction</label>
          <textarea id="nc-prompt" class="input" rows="5" placeholder="Tell the agent what to do...">${escapeHtml(cfg.prompt || '')}</textarea>
        </div>
        <div class="form-group"><label>Variables (comma-separated)</label>
          <input id="nc-variables" class="input" value="${escapeHtml(cfg.variables || '')}" placeholder="{{input}}, {{result}}">
        </div>`;
      break;
    case 'agent':
      const agentOpts = (state.agents || []).map(a =>
        `<option value="${escapeHtml(a.name)}" ${cfg.agent_name === a.name ? 'selected' : ''}>${escapeHtml(a.name)}</option>`
      ).join('');
      fields = `
        <div class="form-group"><label>Agent</label>
          <select id="nc-agent" class="input"><option value="">Select agent...</option>${agentOpts}</select>
        </div>
        <div class="form-group"><label>Task</label>
          <textarea id="nc-task" class="input" rows="3" placeholder="Task instructions for this agent">${escapeHtml(cfg.task || '')}</textarea>
        </div>`;
      break;
    case 'webhook':
      fields = `
        <div class="form-group"><label>URL</label><input id="nc-url" class="input" value="${escapeHtml(cfg.url || '')}" placeholder="https://..."></div>
        <div class="form-group"><label>Method</label>
          <select id="nc-method" class="input">
            ${['GET','POST','PUT','DELETE'].map(m => `<option ${cfg.method===m?'selected':''}>${m}</option>`).join('')}
          </select>
        </div>
        <div class="form-group"><label>Headers (JSON)</label>
          <textarea id="nc-headers" class="input" rows="2" placeholder='{"Authorization":"Bearer ..."}'>${escapeHtml(cfg.headers || '')}</textarea>
        </div>
        <div class="form-group"><label>Body</label>
          <textarea id="nc-body" class="input" rows="3" placeholder="Request body">${escapeHtml(cfg.body || '')}</textarea>
        </div>`;
      break;
    case 'api_call':
      fields = `
        <div class="form-group"><label>URL</label><input id="nc-url" class="input" value="${escapeHtml(cfg.url || '')}" placeholder="https://api.example.com/..."></div>
        <div class="form-group"><label>Method</label>
          <select id="nc-method" class="input">
            ${['GET','POST','PUT','PATCH','DELETE'].map(m => `<option ${cfg.method===m?'selected':''}>${m}</option>`).join('')}
          </select>
        </div>
        <div class="form-group"><label>Headers (JSON)</label>
          <textarea id="nc-headers" class="input" rows="2">${escapeHtml(cfg.headers || '')}</textarea>
        </div>
        <div class="form-group"><label>Body</label>
          <textarea id="nc-body" class="input" rows="3">${escapeHtml(cfg.body || '')}</textarea>
        </div>
        <div class="form-group"><label>Auth Type</label>
          <select id="nc-auth" class="input">
            <option ${cfg.auth==='none'?'selected':''}>none</option>
            <option ${cfg.auth==='bearer'?'selected':''}>bearer</option>
            <option ${cfg.auth==='basic'?'selected':''}>basic</option>
          </select>
        </div>`;
      break;
    case 'websocket':
      fields = `
        <div class="form-group"><label>WebSocket URL</label><input id="nc-url" class="input" value="${escapeHtml(cfg.url || '')}" placeholder="wss://..."></div>
        <div class="form-group"><label>Message to Send</label>
          <textarea id="nc-message" class="input" rows="3">${escapeHtml(cfg.message || '')}</textarea>
        </div>
        <div class="form-group"><label>Listen Events (comma-separated)</label>
          <input id="nc-events" class="input" value="${escapeHtml(cfg.listen_events || '')}" placeholder="message, close">
        </div>`;
      break;
    case 'condition':
      fields = `
        <div class="form-group"><label>Field</label><input id="nc-field" class="input" value="${escapeHtml(cfg.field || '')}" placeholder="result.status"></div>
        <div class="form-group"><label>Operator</label>
          <select id="nc-operator" class="input">
            ${['==','!=','>','<','>=','<=','contains','starts_with'].map(op => `<option ${cfg.operator===op?'selected':''}>${op}</option>`).join('')}
          </select>
        </div>
        <div class="form-group"><label>Value</label><input id="nc-value" class="input" value="${escapeHtml(cfg.value || '')}" placeholder="200"></div>`;
      break;
    case 'transform':
      fields = `
        <div class="form-group"><label>Transform Template</label>
          <textarea id="nc-template" class="input" rows="4" placeholder="Map input to output format...">${escapeHtml(cfg.template || '')}</textarea>
        </div>`;
      break;
    case 'delay':
      fields = `
        <div class="form-group"><label>Delay (seconds)</label>
          <input id="nc-seconds" class="input" type="number" value="${cfg.seconds || 0}" min="0">
        </div>`;
      break;
    case 'loop':
      fields = `
        <div class="form-group"><label>Array Field</label>
          <input id="nc-array-field" class="input" value="${escapeHtml(cfg.array_field || '')}" placeholder="items">
        </div>
        <div class="form-group"><label>Max Iterations</label>
          <input id="nc-max-iter" class="input" type="number" value="${cfg.max_iterations || 100}" min="1">
        </div>`;
      break;
    case 'merge':
      fields = `
        <div class="form-group"><label>Merge Mode</label>
          <select id="nc-mode" class="input">
            ${['append','combine','wait_all'].map(m => `<option ${cfg.mode===m?'selected':''}>${m}</option>`).join('')}
          </select>
        </div>`;
      break;
    case 'code':
      fields = `
        <div class="form-group"><label>Language</label>
          <select id="nc-language" class="input">
            ${['javascript','python','shell'].map(l => `<option ${cfg.language===l?'selected':''}>${l}</option>`).join('')}
          </select>
        </div>
        <div class="form-group"><label>Code</label>
          <textarea id="nc-code" class="input" rows="6" style="font-family:monospace;font-size:11px;" placeholder="// your code here...">${escapeHtml(cfg.code || '')}</textarea>
        </div>`;
      break;
    case 'error_handler':
      fields = `
        <div class="form-group"><label>On Error</label>
          <select id="nc-on-error" class="input">
            ${['continue','stop','retry'].map(o => `<option ${cfg.on_error===o?'selected':''}>${o}</option>`).join('')}
          </select>
        </div>
        <div class="form-group"><label>Max Retries</label>
          <input id="nc-max-retries" class="input" type="number" value="${cfg.max_retries || 3}" min="0">
        </div>`;
      break;
    case 'schedule':
      fields = `
        <div class="form-group"><label>Cron Expression</label>
          <input id="nc-cron" class="input" value="${escapeHtml(cfg.cron || '')}" placeholder="0 * * * * (every hour)">
        </div>
        <div style="font-size:10px;color:var(--text-muted);margin-top:4px;">min hour day month weekday</div>`;
      break;
    case 'db_query':
      fields = `
        <div class="form-group"><label>Database</label>
          <select id="nc-database" class="input">
            ${['graph','sqlite','neo4j'].map(d => `<option ${cfg.database===d?'selected':''}>${d}</option>`).join('')}
          </select>
        </div>
        <div class="form-group"><label>Query</label>
          <textarea id="nc-query" class="input" rows="4" style="font-family:monospace;font-size:11px;" placeholder="SELECT * FROM ...">${escapeHtml(cfg.query || '')}</textarea>
        </div>`;
      break;
    case 'email':
      fields = `
        <div class="form-group"><label>To</label>
          <input id="nc-to" class="input" value="${escapeHtml(cfg.to || '')}" placeholder="user@example.com">
        </div>
        <div class="form-group"><label>Subject</label>
          <input id="nc-subject" class="input" value="${escapeHtml(cfg.subject || '')}" placeholder="Report: {{title}}">
        </div>
        <div class="form-group"><label>Body</label>
          <textarea id="nc-email-body" class="input" rows="4" placeholder="Email content...">${escapeHtml(cfg.body || '')}</textarea>
        </div>`;
      break;
    case 'notification':
      fields = `
        <div class="form-group"><label>Channel</label>
          <select id="nc-channel" class="input">
            ${['discord','slack','webhook','system'].map(c => `<option ${cfg.channel===c?'selected':''}>${c}</option>`).join('')}
          </select>
        </div>
        <div class="form-group"><label>Message</label>
          <textarea id="nc-notif-msg" class="input" rows="3" placeholder="Alert: {{event}}">${escapeHtml(cfg.message || '')}</textarea>
        </div>
        <div class="form-group"><label>Webhook URL (if webhook)</label>
          <input id="nc-notif-url" class="input" value="${escapeHtml(cfg.webhook_url || '')}" placeholder="https://...">
        </div>`;
      break;
    case 'human':
      fields = `
        <div class="form-group"><label>Approver</label>
          <input id="nc-approver" class="input" value="${escapeHtml(cfg.approver || '')}" placeholder="manager">
        </div>
        <div class="form-group"><label>Approval Message</label>
          <textarea id="nc-approval-msg" class="input" rows="3" placeholder="Please review...">${escapeHtml(cfg.message || '')}</textarea>
        </div>`;
      break;
    case 'note':
      fields = `
        <div class="form-group"><label>Note Text</label>
          <textarea id="nc-note-text" class="input" rows="5" placeholder="Add a note...">${escapeHtml(cfg.text || '')}</textarea>
        </div>`;
      break;
  }

  panel.innerHTML = `
    <div style="display:flex;align-items:center;gap:6px;margin-bottom:12px;">
      <span style="font-size:18px;">${nt.icon}</span>
      <span style="font-weight:600;font-size:13px;color:var(--text-primary);">${nt.label} Node</span>
    </div>
    <div class="form-group"><label>Label</label>
      <input id="nc-label" class="input" value="${escapeHtml(node.label)}" onchange="updateNodeLabel('${nodeId}',this.value)">
    </div>
    ${fields}
    <div style="display:flex;gap:6px;margin-top:12px;">
      <button class="btn-sm" onclick="applyNodeConfig('${nodeId}')" style="background:var(--info);color:#fff;flex:1;">Apply</button>
      <button class="btn-sm btn-danger" onclick="deleteNode('${nodeId}')">Delete</button>
    </div>
  `;
}

function updateNodeLabel(nodeId, newLabel) {
  const node = wfEditorState.nodes.find(n => n.id === nodeId);
  if (node) { node.label = newLabel; renderCanvasNodes(); }
}

function applyNodeConfig(nodeId) {
  const node = wfEditorState.nodes.find(n => n.id === nodeId);
  if (!node) return;
  const cfg = {};
  switch (node.type) {
    case 'prompt':
      cfg.prompt = document.getElementById('nc-prompt')?.value || '';
      cfg.variables = document.getElementById('nc-variables')?.value || '';
      break;
    case 'agent':
      cfg.agent_name = document.getElementById('nc-agent')?.value || '';
      cfg.task = document.getElementById('nc-task')?.value || '';
      break;
    case 'webhook':
    case 'api_call':
      cfg.url = document.getElementById('nc-url')?.value || '';
      cfg.method = document.getElementById('nc-method')?.value || 'GET';
      cfg.headers = document.getElementById('nc-headers')?.value || '';
      cfg.body = document.getElementById('nc-body')?.value || '';
      if (node.type === 'api_call') cfg.auth = document.getElementById('nc-auth')?.value || 'none';
      break;
    case 'websocket':
      cfg.url = document.getElementById('nc-url')?.value || '';
      cfg.message = document.getElementById('nc-message')?.value || '';
      cfg.listen_events = document.getElementById('nc-events')?.value || '';
      break;
    case 'condition':
      cfg.field = document.getElementById('nc-field')?.value || '';
      cfg.operator = document.getElementById('nc-operator')?.value || '==';
      cfg.value = document.getElementById('nc-value')?.value || '';
      break;
    case 'transform':
      cfg.template = document.getElementById('nc-template')?.value || '';
      break;
    case 'delay':
      cfg.seconds = parseInt(document.getElementById('nc-seconds')?.value) || 0;
      break;
    case 'loop':
      cfg.array_field = document.getElementById('nc-array-field')?.value || '';
      cfg.max_iterations = parseInt(document.getElementById('nc-max-iter')?.value) || 100;
      break;
    case 'merge':
      cfg.mode = document.getElementById('nc-mode')?.value || 'append';
      break;
    case 'code':
      cfg.language = document.getElementById('nc-language')?.value || 'javascript';
      cfg.code = document.getElementById('nc-code')?.value || '';
      break;
    case 'error_handler':
      cfg.on_error = document.getElementById('nc-on-error')?.value || 'continue';
      cfg.max_retries = parseInt(document.getElementById('nc-max-retries')?.value) || 3;
      break;
    case 'schedule':
      cfg.cron = document.getElementById('nc-cron')?.value || '';
      break;
    case 'db_query':
      cfg.database = document.getElementById('nc-database')?.value || 'graph';
      cfg.query = document.getElementById('nc-query')?.value || '';
      break;
    case 'email':
      cfg.to = document.getElementById('nc-to')?.value || '';
      cfg.subject = document.getElementById('nc-subject')?.value || '';
      cfg.body = document.getElementById('nc-email-body')?.value || '';
      break;
    case 'notification':
      cfg.channel = document.getElementById('nc-channel')?.value || 'discord';
      cfg.message = document.getElementById('nc-notif-msg')?.value || '';
      cfg.webhook_url = document.getElementById('nc-notif-url')?.value || '';
      break;
    case 'human':
      cfg.approver = document.getElementById('nc-approver')?.value || '';
      cfg.message = document.getElementById('nc-approval-msg')?.value || '';
      break;
    case 'note':
      cfg.text = document.getElementById('nc-note-text')?.value || '';
      break;
  }
  node.config = cfg;
  renderCanvasNodes();
}

function deleteNode(nodeId) {
  if (!confirm('Delete this node and its connections?')) return;
  wfEditorState.nodes = wfEditorState.nodes.filter(n => n.id !== nodeId);
  wfEditorState.edges = wfEditorState.edges.filter(e => e.source !== nodeId && e.target !== nodeId);
  wfEditorState.selectedNode = null;
  const panel = document.getElementById('wf-config-panel');
  if (panel) panel.style.display = 'none';
  renderCanvasNodes();
  renderCanvasEdges();
}

// ── Save / Execute ──────────────────────────────────────────────────

async function saveWorkflow() {
  if (!wfEditorState.workflowId) return;
  await fetch('/v1/workflows/' + wfEditorState.workflowId, {
    method: 'PUT', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({
      nodes: wfEditorState.nodes,
      edges: wfEditorState.edges,
    })
  });
  // Brief flash feedback.
  const btn = document.querySelector('[onclick="saveWorkflow()"]');
  if (btn) { const orig = btn.textContent; btn.textContent = '✓ Saved'; setTimeout(() => { btn.textContent = orig; }, 1500); }
}

async function executeWorkflow() {
  if (!wfEditorState.workflowId) return;
  // Save first.
  await saveWorkflow();
  try {
    const r = await fetch('/v1/workflows/' + wfEditorState.workflowId + '/execute', { method: 'POST' });
    const d = await r.json();
    alert('Workflow executing: ' + (d.nodes_total || 0) + ' nodes queued.\n\nExecution order:\n' +
      (d.execution_order || []).map(s => s.step + '. ' + s.label + ' (' + s.type + ')').join('\n'));
  } catch (e) {
    alert('Execute failed: ' + e.message);
  }
}

// ── Knowledge Page (agency-scoped) ──────────────────────────────────

async function renderKnowledgePage(container) {
  const agencyId = state.activeAgencyId || 0;
  const agencyName = state.activeAgencyName || 'All Agencies';

  let entries = [];
  try {
    const url = agencyId ? `/v1/knowledge?agency_id=${agencyId}` : '/v1/knowledge';
    const r = await fetch(url);
    const d = await r.json();
    entries = d.entries || [];
  } catch (e) { console.error(e); }

  const typeIcons = { fact: '💡', document: '📄', faq: '❓', note: '📝' };

  container.innerHTML = `
    <div class="page-section">
      <!-- Agency scope indicator -->
      <div style="display:flex;align-items:center;gap:8px;padding:10px 14px;background:${agencyId ? 'rgba(59,130,246,0.1)' : 'rgba(255,255,255,0.03)'};border:1px solid ${agencyId ? 'var(--info)' : 'var(--border)'};border-radius:var(--radius);margin-bottom:1rem;">
        <span style="font-size:14px;">${agencyId ? '🏢' : '🌐'}</span>
        <span style="font-size:12px;color:var(--text-primary);font-weight:500;">Scope: ${escapeHtml(agencyName)}</span>
        ${agencyId ? `<span style="font-size:10px;color:var(--text-muted);">Showing knowledge for this agency only</span>` :
          `<span style="font-size:10px;color:var(--text-muted);">Showing all knowledge. Select an agency in Chat to filter.</span>`}
      </div>

      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);display:flex;align-items:center;">Knowledge Base (${entries.length})${helpIcon('Ingest repos, files, and datasets. Content is indexed across Graph, Vector, Table, and Full-Text engines for maximum recall.')}</h3>
        <button class="btn-primary" onclick="showCreateKnowledgeModal()">+ Add Entry</button>
      </div>
      ${entries.length === 0 ? '<div class="empty-state">No knowledge entries yet. Add facts, documents, FAQs, or notes to build your agency knowledge base.</div>' :
        `<div style="display:flex;flex-direction:column;gap:6px;">
          ${entries.map(e => `
            <div style="padding:10px 14px;background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);display:flex;align-items:center;gap:10px;">
              <span style="font-size:18px;">${typeIcons[e.type] || '📝'}</span>
              <div style="flex:1;min-width:0;">
                <div style="font-weight:600;font-size:13px;color:var(--text-primary);">${escapeHtml(e.title)}</div>
                ${e.content ? `<div style="font-size:11px;color:var(--text-secondary);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">${escapeHtml(e.content.substring(0, 120))}</div>` : ''}
                <div style="font-size:10px;color:var(--text-muted);margin-top:2px;">
                  ${e.source ? 'Source: ' + escapeHtml(e.source) + ' | ' : ''}${e.tags ? 'Tags: ' + escapeHtml(e.tags) + ' | ' : ''}${e.created_at ? new Date(e.created_at).toLocaleDateString() : ''}
                </div>
              </div>
              <span class="mp-badge" style="font-size:10px;">${(e.type || 'note').toUpperCase()}</span>
              <button class="btn-sm btn-danger" onclick="deleteKnowledgeEntry(${e.id})" style="font-size:10px;padding:2px 6px;">Del</button>
            </div>
          `).join('')}
        </div>`}
    </div>
    <div id="knowledge-modal" class="modal-overlay" style="display:none;"></div>
  `;
}

function showCreateKnowledgeModal() {
  const modal = document.getElementById('knowledge-modal');
  if (!modal) return;
  modal.style.display = 'flex';

  modal.innerHTML = `
    <div class="modal-card" style="max-width:520px;width:90%;">
      <h3 style="margin:0 0 1rem;color:var(--text-primary);">Add Knowledge Entry</h3>
      <div class="form-group"><label>Title *</label><input id="kb-title" class="input" placeholder="Entry title"></div>
      <div class="form-group"><label>Type</label>
        <select id="kb-type" class="input">
          <option value="note">📝 Note</option>
          <option value="fact">💡 Fact</option>
          <option value="document">📄 Document</option>
          <option value="faq">❓ FAQ</option>
        </select>
      </div>
      <div class="form-group"><label>Content</label>
        <textarea id="kb-content" class="input" rows="4" placeholder="Knowledge content..."></textarea>
      </div>
      <div class="form-group"><label>Source (optional)</label><input id="kb-source" class="input" placeholder="URL, file path, or manual"></div>
      <div class="form-group"><label>Tags (comma-separated)</label><input id="kb-tags" class="input" placeholder="marketing, social, seo"></div>
      <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
        <button class="btn-sm" onclick="document.getElementById('knowledge-modal').style.display='none'">Cancel</button>
        <button class="btn-primary" onclick="submitCreateKnowledge()">Add</button>
      </div>
    </div>
  `;
}

async function submitCreateKnowledge() {
  const title = document.getElementById('kb-title')?.value;
  if (!title) { alert('Title required'); return; }
  await fetch('/v1/knowledge', {
    method: 'POST', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({
      title,
      type: document.getElementById('kb-type')?.value || 'note',
      content: document.getElementById('kb-content')?.value || '',
      source: document.getElementById('kb-source')?.value || '',
      tags: document.getElementById('kb-tags')?.value || '',
      agency_id: String(state.activeAgencyId || 0),
      project_id: String(state.activeSubAccountId || 0),
    })
  });
  document.getElementById('knowledge-modal').style.display = 'none';
  renderKnowledgePage(document.getElementById('page-content'));
}

async function deleteKnowledgeEntry(id) {
  if (!confirm('Delete this knowledge entry?')) return;
  await fetch('/v1/knowledge/' + id, { method: 'DELETE' });
  renderKnowledgePage(document.getElementById('page-content'));
}

// ── Calendar Page ───────────────────────────────────────────────────

async function renderCalendarPage(container) {
  // Fetch automations and pipelines to populate the calendar.
  let automations = [], pipelines = [];
  try {
    const [aRes, pRes] = await Promise.all([
      fetch('/v1/automations').then(r => r.json()).catch(() => ({})),
      fetch('/v1/pipelines').then(r => r.json()).catch(() => ({})),
    ]);
    automations = aRes.automations || [];
    pipelines = pRes.pipelines || [];
  } catch (e) { console.error(e); }

  // Build a 7-day calendar from today.
  const days = [];
  const now = new Date();
  for (let i = 0; i < 7; i++) {
    const d = new Date(now);
    d.setDate(now.getDate() + i);
    days.push({
      label: d.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' }),
      dateStr: d.toISOString().split('T')[0],
      isToday: i === 0,
      events: []
    });
  }

  // Map automations to days based on next_run.
  for (const auto of automations) {
    if (!auto.enabled || !auto.next_run) continue;
    const runDate = auto.next_run.split('T')[0];
    const runTime = auto.next_run.split('T')[1]?.substring(0, 5) || '';
    for (const day of days) {
      if (day.dateStr === runDate) {
        day.events.push({ type: 'automation', name: auto.name, time: runTime, agent: auto.agent, id: auto.id });
      }
    }
  }

  // Map pipelines with schedules.
  for (const pl of pipelines) {
    if (pl.status !== 'active' || !pl.schedule) continue;
    // For interval-based pipelines, show on today.
    days[0].events.push({ type: 'pipeline', name: pl.name, time: pl.schedule, steps: (pl.steps || []).length });
  }

  container.innerHTML = `
    <div class="page-section">
      <h3 style="margin:0 0 1rem;color:var(--text-primary);display:flex;align-items:center;">Automation Calendar${helpIcon('Schedule and visualize automated tasks on a calendar. Drag events to reschedule.')}</h3>
      <div style="display:grid;grid-template-columns:repeat(7,1fr);gap:8px;min-height:400px;">
        ${days.map(day => `
          <div style="background:var(--bg-card);border:1px solid ${day.isToday ? 'var(--accent)' : 'var(--border)'};border-radius:var(--radius);padding:10px;display:flex;flex-direction:column;">
            <div style="font-size:12px;font-weight:600;color:${day.isToday ? 'var(--accent)' : 'var(--text-primary)'};margin-bottom:8px;text-align:center;">
              ${day.label}${day.isToday ? ' (Today)' : ''}
            </div>
            <div style="display:flex;flex-direction:column;gap:4px;flex:1;">
              ${day.events.length === 0 ? '<div style="font-size:10px;color:var(--text-muted);text-align:center;padding:20px 0;">No events</div>' :
                day.events.map(ev => `
                  <div style="font-size:10px;padding:6px;border-radius:var(--radius-sm);background:${ev.type === 'automation' ? 'rgba(59,130,246,0.15)' : 'rgba(16,185,129,0.15)'};border-left:2px solid ${ev.type === 'automation' ? 'var(--info)' : 'var(--success)'};">
                    <div style="font-weight:600;color:var(--text-primary);">${escapeHtml(ev.name)}</div>
                    <div style="color:var(--text-muted);">${ev.time}${ev.agent ? ' - ' + ev.agent : ''}${ev.steps ? ' (' + ev.steps + ' steps)' : ''}</div>
                  </div>
                `).join('')}
            </div>
          </div>
        `).join('')}
      </div>
    </div>
  `;
}

// ── Reports Page ────────────────────────────────────────────────────

const REPORT_TYPES = [
  { value: 'social_media', label: 'Social Media Summary', icon: '📱' },
  { value: 'email_activity', label: 'Email Activity', icon: '📧' },
  { value: 'engagement', label: 'Client Engagement', icon: '📈' },
  { value: 'custom', label: 'Custom Report', icon: '📝' },
];

async function renderReportsPage(container) {
  let templates = [], generated = [];
  try {
    const [tRes, gRes] = await Promise.all([
      fetch('/v1/reports/templates').then(r => r.json()).catch(() => ({})),
      fetch('/v1/reports/generated').then(r => r.json()).catch(() => ({})),
    ]);
    templates = tRes.templates || [];
    generated = gRes.reports || [];
  } catch (e) { console.error(e); }

  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);display:flex;align-items:center;">Report Templates (${templates.length})${helpIcon('Create reusable report templates that agents generate on schedule or on demand.')}</h3>
        <button class="btn-primary" onclick="showCreateReportModal()">+ New Template</button>
      </div>
      ${templates.length === 0 ? '<div class="empty-state">No report templates yet. Create one to automate recurring reports.</div>' :
        `<div class="cards-grid">${templates.map(t => {
          const rt = REPORT_TYPES.find(r => r.value === t.type) || { icon: '📝', label: t.type };
          return `
          <div class="agency-card">
            <div class="agency-card-header">
              <div class="agency-card-info">
                <div class="agency-card-name">${rt.icon} ${escapeHtml(t.name)}</div>
                ${t.description ? `<div class="agency-card-niche">${escapeHtml(t.description)}</div>` : ''}
                <div style="font-size:11px;color:var(--text-muted);margin-top:2px;">Runs: ${t.run_count || 0}${t.schedule ? ' | Schedule: ' + escapeHtml(t.schedule) : ' | Manual'}</div>
              </div>
              <span class="mp-badge" style="font-size:10px;">${rt.label}</span>
            </div>
            <div class="agency-card-actions" style="margin-top:8px;">
              <button class="btn-sm btn-danger" onclick="deleteReportTemplate(${t.id})">Delete</button>
            </div>
          </div>`;
        }).join('')}</div>`}

      ${generated.length > 0 ? `
        <h3 style="margin:1.5rem 0 1rem;color:var(--text-primary);">Generated Reports (${generated.length})</h3>
        <div style="display:flex;flex-direction:column;gap:6px;">
          ${generated.map(r => `
            <div style="padding:10px;background:var(--bg-card);border:1px solid var(--border);border-radius:var(--radius);display:flex;justify-content:space-between;align-items:center;">
              <div>
                <span style="font-weight:500;color:var(--text-primary);">${escapeHtml(r.name)}</span>
                <span style="font-size:11px;color:var(--text-muted);margin-left:8px;">${r.generated_at ? new Date(r.generated_at).toLocaleString() : ''}</span>
              </div>
              <button class="btn-sm" onclick="alert(decodeURIComponent(escape(atob('${btoa(unescape(encodeURIComponent(r.content || 'No content')))}') || 'Empty'))">View</button>
            </div>
          `).join('')}
        </div>` : ''}
    </div>
    <div id="report-modal" class="modal-overlay" style="display:none;"></div>
  `;
}

function showCreateReportModal() {
  const modal = document.getElementById('report-modal');
  if (!modal) return;
  modal.style.display = 'flex';
  const typeOpts = REPORT_TYPES.map(r => '<option value="' + r.value + '">' + r.icon + ' ' + r.label + '</option>').join('');
  const agentOpts = state.agents.map(a => '<option value="' + escapeHtml(a.name) + '">' + escapeHtml(a.name) + '</option>').join('');

  modal.innerHTML = `
    <div class="modal-card" style="max-width:520px;width:90%;">
      <h3 style="margin:0 0 1rem;color:var(--text-primary);">New Report Template</h3>
      <div class="form-group"><label>Name *</label><input id="rpt-name" class="input" placeholder="Weekly Social Summary"></div>
      <div class="form-group"><label>Type</label>
        <select id="rpt-type" class="input">${typeOpts}</select>
      </div>
      <div class="form-group"><label>Description</label><input id="rpt-desc" class="input" placeholder="What does this report cover?"></div>
      <div class="form-group"><label>Agent</label>
        <select id="rpt-agent" class="input"><option value="">Auto (orchestrator)</option>${agentOpts}</select>
      </div>
      <div class="form-group"><label>Generation Prompt</label>
        <textarea id="rpt-prompt" class="input" rows="3" placeholder="Generate a weekly summary of all social media posts, engagement metrics, and recommendations..."></textarea>
      </div>
      <div class="form-group"><label>Schedule (optional)</label>
        <input id="rpt-schedule" class="input" placeholder="e.g. 0 9 * * 1 (every Monday at 9am)">
      </div>
      <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
        <button class="btn-sm" onclick="document.getElementById('report-modal').style.display='none'">Cancel</button>
        <button class="btn-primary" onclick="submitCreateReport()">Create</button>
      </div>
    </div>
  `;
}

async function submitCreateReport() {
  const name = document.getElementById('rpt-name')?.value;
  if (!name) { alert('Report name required'); return; }
  await fetch('/v1/reports/templates', {
    method: 'POST', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({
      name, type: document.getElementById('rpt-type')?.value || 'custom',
      description: document.getElementById('rpt-desc')?.value || '',
      agent: document.getElementById('rpt-agent')?.value || '',
      prompt: document.getElementById('rpt-prompt')?.value || '',
      schedule: document.getElementById('rpt-schedule')?.value || '',
      agency_id: String(state.activeAgencyId || 0),
      project_id: String(state.activeSubAccountId || 0),
    })
  });
  document.getElementById('report-modal').style.display = 'none';
  renderReportsPage(document.getElementById('page-content'));
}

async function deleteReportTemplate(id) {
  if (!confirm('Delete this report template?')) return;
  await fetch('/v1/reports/templates/' + id, { method: 'DELETE' });
  renderReportsPage(document.getElementById('page-content'));
}

// ── Contacts (CRM) Page ─────────────────────────────────────────────

async function renderContactsPage(container) {
  let contacts = [];
  const agencyFilter = state.activeAgencyId ? '?agency_id=' + state.activeAgencyId : '';
  try {
    const r = await fetch('/v1/contacts' + agencyFilter);
    const d = await r.json();
    contacts = d.contacts || [];
  } catch (e) { console.error(e); }

  const statusColors = { lead: '#f59e0b', active: 'var(--success)', inactive: 'var(--text-muted)' };

  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);display:flex;align-items:center;">Contacts (${contacts.length})${state.activeAgencyName ? ' - ' + escapeHtml(state.activeAgencyName) : ''}${helpIcon('Manage contacts associated with your agencies. Import, export, and organize client data.')}</h3>
        <button class="btn-primary" onclick="showCreateContactModal()">+ Add Contact</button>
      </div>
      ${contacts.length === 0 ? '<div class="empty-state">No contacts yet. Add your first lead or client.</div>' : `
        <table class="data-table" style="width:100%;border-collapse:collapse;">
          <thead><tr style="border-bottom:1px solid var(--border);">
            <th style="text-align:left;padding:8px;color:var(--text-muted);font-size:11px;">NAME</th>
            <th style="text-align:left;padding:8px;color:var(--text-muted);font-size:11px;">EMAIL</th>
            <th style="text-align:left;padding:8px;color:var(--text-muted);font-size:11px;">COMPANY</th>
            <th style="text-align:left;padding:8px;color:var(--text-muted);font-size:11px;">STATUS</th>
            <th style="text-align:right;padding:8px;color:var(--text-muted);font-size:11px;">ACTIONS</th>
          </tr></thead>
          <tbody>
            ${contacts.map(c => `<tr style="border-bottom:1px solid var(--border-subtle);">
              <td style="padding:8px;color:var(--text-primary);font-weight:500;">${escapeHtml(c.name)}</td>
              <td style="padding:8px;color:var(--text-secondary);">${escapeHtml(c.email || '-')}</td>
              <td style="padding:8px;color:var(--text-secondary);">${escapeHtml(c.company || '-')}</td>
              <td style="padding:8px;"><span class="mp-badge" style="background:${statusColors[c.status] || 'var(--border)'};color:#000;font-size:10px;">${(c.status || 'lead').toUpperCase()}</span></td>
              <td style="padding:8px;text-align:right;"><button class="btn-sm btn-danger" onclick="deleteContact(${c.id})">Del</button></td>
            </tr>`).join('')}
          </tbody>
        </table>`}
    </div>
    <div id="contact-modal" class="modal-overlay" style="display:none;"></div>
  `;
}

function showCreateContactModal() {
  const modal = document.getElementById('contact-modal');
  if (!modal) return;
  modal.style.display = 'flex';
  modal.innerHTML = `
    <div class="modal-card" style="max-width:480px;width:90%;">
      <h3 style="margin:0 0 1rem;color:var(--text-primary);">New Contact</h3>
      <div class="form-group"><label>Name *</label><input id="ct-name" class="input" placeholder="Full name"></div>
      <div class="form-group"><label>Email</label><input id="ct-email" class="input" placeholder="email@example.com"></div>
      <div class="form-group"><label>Phone</label><input id="ct-phone" class="input" placeholder="+1 555 0123"></div>
      <div class="form-group"><label>Company</label><input id="ct-company" class="input" placeholder="Company name"></div>
      <div class="form-group"><label>Tags</label><input id="ct-tags" class="input" placeholder="vip, enterprise"></div>
      <div class="form-group"><label>Source</label><input id="ct-source" class="input" placeholder="website, referral, social"></div>
      <div class="form-group"><label>Status</label>
        <select id="ct-status" class="input"><option value="lead">Lead</option><option value="active">Active</option><option value="inactive">Inactive</option></select>
      </div>
      <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
        <button class="btn-sm" onclick="document.getElementById('contact-modal').style.display='none'">Cancel</button>
        <button class="btn-primary" onclick="submitCreateContact()">Create</button>
      </div>
    </div>
  `;
}

async function submitCreateContact() {
  const name = document.getElementById('ct-name')?.value;
  if (!name) { alert('Name required'); return; }
  await fetch('/v1/contacts', {
    method: 'POST', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({
      name, email: document.getElementById('ct-email')?.value || '',
      phone: document.getElementById('ct-phone')?.value || '',
      company: document.getElementById('ct-company')?.value || '',
      tags: document.getElementById('ct-tags')?.value || '',
      source: document.getElementById('ct-source')?.value || '',
      status: document.getElementById('ct-status')?.value || 'lead',
      agency_id: String(state.activeAgencyId || 0),
      subaccount_id: String(state.activeSubAccountId || 0),
    })
  });
  document.getElementById('contact-modal').style.display = 'none';
  renderContactsPage(document.getElementById('page-content'));
}

async function deleteContact(id) {
  if (!confirm('Delete this contact?')) return;
  await fetch('/v1/contacts/' + id, { method: 'DELETE' });
  renderContactsPage(document.getElementById('page-content'));
}

// ── Pipelines Page ──────────────────────────────────────────────────

const PIPELINE_ACTIONS = [
  { value: 'run_agent', label: 'Run Agent', icon: '🤖' },
  { value: 'post_social', label: 'Post to Social', icon: '📱' },
  { value: 'check_email', label: 'Check Email', icon: '📧' },
  { value: 'generate_report', label: 'Generate Report', icon: '📊' },
  { value: 'webhook', label: 'Webhook', icon: '🔗' },
  { value: 'wait', label: 'Wait/Delay', icon: '⏳' },
];

async function renderPipelinesPage(container) {
  let pipelines = [];
  try {
    const r = await fetch('/v1/pipelines');
    const d = await r.json();
    pipelines = d.pipelines || [];
  } catch (e) { console.error(e); }

  const statusColors = { active: 'var(--success)', paused: 'var(--warning)', draft: 'var(--text-muted)' };

  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);display:flex;align-items:center;">Pipelines (${pipelines.length})${helpIcon('Linear step-by-step automations. Chain agent actions, emails, webhooks, and delays in sequence.')}</h3>
        <button class="btn-primary" onclick="showCreatePipelineModal()">+ New Pipeline</button>
      </div>
      ${pipelines.length === 0 ? '<div class="empty-state">No pipelines yet. Create one to chain actions like posting, emailing, and reporting.</div>' :
        `<div style="display:flex;flex-direction:column;gap:4px;">${pipelines.map(p => {
          const sc = statusColors[p.status] || 'var(--border)';
          const steps = p.steps || [];
          const stepBadges = steps.map(s => {
            const a = PIPELINE_ACTIONS.find(a => a.value === s.action) || { icon: '⚙', label: s.action };
            return `<span style="font-size:9px;padding:1px 4px;background:rgba(255,255,255,0.06);border-radius:3px;">${a.icon}</span>`;
          }).join('<span style="color:var(--text-muted);font-size:8px;margin:0 1px;">→</span>');
          return `
          <div style="display:flex;align-items:center;gap:10px;padding:8px 12px;background:var(--bg-card);border:1px solid var(--border);border-left:3px solid ${sc};border-radius:6px;cursor:pointer;transition:border-color 0.15s;"
               onmouseenter="this.style.borderColor='var(--accent)'" onmouseleave="this.style.borderColor='var(--border)'">
            <div style="flex:1;min-width:0;display:flex;align-items:center;gap:8px;">
              <span style="font-weight:600;font-size:13px;color:var(--text-primary);">${escapeHtml(p.name)}</span>
              ${steps.length ? `<span style="display:flex;align-items:center;gap:1px;">${stepBadges}</span>` : ''}
              ${p.description ? `<span style="font-size:11px;color:var(--text-muted);">${escapeHtml(p.description.substring(0, 40))}</span>` : ''}
            </div>
            <span style="font-size:10px;color:var(--text-muted);">${steps.length}s ${p.run_count || 0}r</span>
            <span style="padding:2px 6px;border-radius:3px;font-size:9px;font-weight:600;background:${sc};color:#fff;">${(p.status || 'draft').toUpperCase()}</span>
            <button class="btn-sm" onclick="event.stopPropagation();togglePipelineStatus(${p.id}, '${p.status}')" style="font-size:10px;padding:2px 8px;">${p.status === 'active' ? 'Pause' : 'Start'}</button>
            <button class="btn-sm btn-danger" onclick="event.stopPropagation();deletePipeline(${p.id})" style="font-size:10px;padding:2px 6px;">Del</button>
          </div>`;
        }).join('')}</div>`}
    </div>
    <div id="pipeline-modal" class="modal-overlay" style="display:none;"></div>
  `;
}

// Track pipeline steps while building in the modal.
let pipelineStepsTemp = [];

function showCreatePipelineModal() {
  pipelineStepsTemp = [{ order: 1, action: 'run_agent', agent: '', prompt: '', delay_min: 0 }];
  const modal = document.getElementById('pipeline-modal');
  if (!modal) return;
  modal.style.display = 'flex';
  renderPipelineModal();
}

function renderPipelineModal() {
  const modal = document.getElementById('pipeline-modal');
  if (!modal) return;

  const actionOpts = PIPELINE_ACTIONS.map(a => '<option value="' + a.value + '">' + a.icon + ' ' + a.label + '</option>').join('');
  const stepsHTML = pipelineStepsTemp.map((s, i) => `
    <div style="display:flex;gap:6px;align-items:center;padding:8px;background:var(--bg-body);border-radius:var(--radius-sm);border:1px solid var(--border-subtle);">
      <span style="font-weight:600;color:var(--text-muted);min-width:20px;">${i + 1}.</span>
      <select class="input" style="flex:1;font-size:12px;" onchange="pipelineStepsTemp[${i}].action=this.value">${PIPELINE_ACTIONS.map(a => '<option value="' + a.value + '"' + (s.action === a.value ? ' selected' : '') + '>' + a.icon + ' ' + a.label + '</option>').join('')}</select>
      <input class="input" style="flex:2;font-size:12px;" placeholder="Prompt / config" value="${escapeHtml(s.prompt || '')}" onchange="pipelineStepsTemp[${i}].prompt=this.value">
      <button class="btn-sm btn-danger" onclick="pipelineStepsTemp.splice(${i},1);renderPipelineModal();" style="font-size:10px;">X</button>
    </div>
  `).join('');

  modal.innerHTML = `
    <div class="modal-card" style="max-width:580px;width:90%;">
      <h3 style="margin:0 0 1rem;color:var(--text-primary);">New Pipeline</h3>
      <div class="form-group"><label>Name *</label><input id="pl-name" class="input" placeholder="Pipeline name"></div>
      <div class="form-group"><label>Description</label><input id="pl-desc" class="input" placeholder="What does this pipeline do?"></div>
      <div class="form-group"><label>Steps</label>
        <div style="display:flex;flex-direction:column;gap:6px;">${stepsHTML}</div>
        <button class="btn-sm" style="margin-top:6px;" onclick="pipelineStepsTemp.push({order:pipelineStepsTemp.length+1,action:'run_agent',prompt:''});renderPipelineModal();">+ Add Step</button>
      </div>
      <div class="form-group"><label>Schedule</label>
        <select id="pl-schedule-type" class="input"><option value="manual">Manual</option><option value="every">Interval</option><option value="cron">Cron</option></select>
      </div>
      <div class="form-group"><input id="pl-schedule" class="input" placeholder="e.g. 1h, 0 9 * * 1-5"></div>
      <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
        <button class="btn-sm" onclick="document.getElementById('pipeline-modal').style.display='none'">Cancel</button>
        <button class="btn-primary" onclick="submitCreatePipeline()">Create</button>
      </div>
    </div>
  `;
}

async function submitCreatePipeline() {
  const name = document.getElementById('pl-name')?.value;
  if (!name) { alert('Pipeline name required'); return; }
  await fetch('/v1/pipelines', {
    method: 'POST', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({
      name, description: document.getElementById('pl-desc')?.value || '',
      steps: pipelineStepsTemp,
      schedule_type: document.getElementById('pl-schedule-type')?.value || 'manual',
      schedule: document.getElementById('pl-schedule')?.value || '',
      agency_id: String(state.activeAgencyId || 0),
      subaccount_id: String(state.activeSubAccountId || 0),
      status: 'draft'
    })
  });
  document.getElementById('pipeline-modal').style.display = 'none';
  renderPipelinesPage(document.getElementById('page-content'));
}

async function togglePipelineStatus(id, current) {
  const next = current === 'active' ? 'paused' : 'active';
  await fetch('/v1/pipelines/' + id, {
    method: 'PUT', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({ status: next })
  });
  renderPipelinesPage(document.getElementById('page-content'));
}

async function deletePipeline(id) {
  if (!confirm('Delete this pipeline?')) return;
  await fetch('/v1/pipelines/' + id, { method: 'DELETE' });
  renderPipelinesPage(document.getElementById('page-content'));
}

// ── Projects Page ───────────────────────────────────────────────────

async function renderProjectsPage(container) {
  let projects = [];
  try {
    const r = await fetch('/v1/projects');
    const d = await r.json();
    projects = d.projects || [];
  } catch (e) { console.error('Failed to fetch projects:', e); }

  const columns = [
    { key: 'active', label: 'Active', color: 'var(--success)' },
    { key: 'paused', label: 'Paused', color: 'var(--warning)' },
    { key: 'archived', label: 'Archived', color: 'var(--text-muted)' },
  ];

  function renderProjectCard(p) {
    return `
      <div class="kanban-card" draggable="true" ondragstart="event.dataTransfer.setData('text/plain','${p.id}')" data-id="${p.id}"
           style="background:var(--bg-card);border:1px solid var(--border-subtle);border-left:3px solid ${columns.find(c => c.key === p.status)?.color || 'var(--border)'};border-radius:var(--radius);padding:10px;margin-bottom:8px;cursor:grab;">
        <div style="font-weight:600;color:var(--text-primary);font-size:13px;">${escapeHtml(p.name)}</div>
        ${p.description ? `<div style="font-size:11px;color:var(--text-secondary);margin-top:2px;">${escapeHtml(p.description.substring(0, 60))}${p.description.length > 60 ? '...' : ''}</div>` : ''}
        ${p.agency_name ? `<div style="font-size:10px;color:var(--text-muted);margin-top:4px;">📁 ${escapeHtml(p.agency_name)}${p.subaccount_name ? ' / ' + escapeHtml(p.subaccount_name) : ''}</div>` : ''}
        ${(p.assigned_agents && p.assigned_agents.length && p.assigned_agents[0]) ? `
          <div style="margin-top:6px;display:flex;flex-wrap:wrap;gap:3px;">
            ${p.assigned_agents.map(a => `<span class="mp-badge" style="background:var(--info);font-size:9px;">${escapeHtml(a)}</span>`).join('')}
          </div>` : ''}
        <div style="display:flex;gap:4px;margin-top:6px;">
          <button class="btn-sm btn-danger" onclick="deleteProject(${p.id})" style="font-size:10px;padding:2px 6px;">Del</button>
        </div>
      </div>`;
  }

  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);display:flex;align-items:center;">Projects Board (${projects.length})${helpIcon('Kanban board for managing agent projects. Track tasks through stages from backlog to done.')}</h3>
        <button class="btn-primary" onclick="showCreateProjectModal()">+ New Project</button>
      </div>
      <div style="display:grid;grid-template-columns:repeat(3,1fr);gap:12px;min-height:400px;">
        ${columns.map(col => {
          const colProjects = projects.filter(p => (p.status || 'active') === col.key);
          return `
          <div class="kanban-column" data-status="${col.key}"
               ondragover="event.preventDefault();this.style.background='rgba(255,255,255,0.04)'"
               ondragleave="this.style.background=''"
               ondrop="kanbanDrop(event,'${col.key}');this.style.background=''"
               style="background:var(--bg-body);border:1px solid var(--border);border-radius:var(--radius);padding:12px;display:flex;flex-direction:column;">
            <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:10px;padding-bottom:8px;border-bottom:2px solid ${col.color};">
              <span style="font-weight:600;font-size:12px;color:var(--text-primary);">${col.label}</span>
              <span style="font-size:11px;color:var(--text-muted);">${colProjects.length}</span>
            </div>
            <div style="flex:1;overflow-y:auto;">
              ${colProjects.length === 0 ? '<div style="font-size:11px;color:var(--text-muted);text-align:center;padding:30px 0;">Drop projects here</div>' :
                colProjects.map(p => renderProjectCard(p)).join('')}
            </div>
          </div>`;
        }).join('')}
      </div>
    </div>
    <div id="project-modal" class="modal-overlay" style="display:none;"></div>
  `;
}

async function kanbanDrop(event, newStatus) {
  event.preventDefault();
  const projectId = event.dataTransfer.getData('text/plain');
  if (!projectId) return;
  await fetch('/v1/projects/' + projectId, {
    method: 'PUT', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({ status: newStatus })
  });
  renderProjectsPage(document.getElementById('page-content'));
}

function showCreateProjectModal() {
  const modal = document.getElementById('project-modal');
  if (!modal) return;
  modal.style.display = 'flex';

  // Build agency/sub-account options.
  let agencyOpts = '<option value="0" data-name="">None</option>';
  for (const ag of state.allAgencies) {
    agencyOpts += '<option value="' + ag.id + '" data-name="' + escapeHtml(ag.name) + '">' + escapeHtml(ag.name) + '</option>';
  }

  // Build agents multi-select.
  const agentChecks = state.agents.map(a =>
    '<label style="display:flex;align-items:center;gap:4px;font-size:12px;color:var(--text-secondary);"><input type="checkbox" class="proj-agent-cb" value="' + escapeHtml(a.name) + '"> ' + escapeHtml(a.name) + '</label>'
  ).join('');

  modal.innerHTML = `
    <div class="modal-card" style="max-width:520px;width:90%;">
      <h3 style="margin:0 0 1rem;color:var(--text-primary);">New Project</h3>
      <div class="form-group"><label>Name *</label><input id="proj-name" class="input" placeholder="Project name"></div>
      <div class="form-group"><label>Description</label><input id="proj-desc" class="input" placeholder="Brief description"></div>
      <div class="form-group"><label>Agency</label>
        <select id="proj-agency" class="input" onchange="updateSubAccountOpts(this.value)">
          ${agencyOpts}
        </select>
      </div>
      <div class="form-group"><label>Sub-Account</label>
        <select id="proj-subaccount" class="input"><option value="0">None</option></select>
      </div>
      <div class="form-group"><label>Assigned Agents</label>
        <div id="proj-agents-list" style="display:flex;flex-wrap:wrap;gap:6px;max-height:100px;overflow-y:auto;">
          ${agentChecks || '<span style="font-size:12px;color:var(--text-muted);">No agents loaded</span>'}
        </div>
      </div>
      <div class="form-group"><label>Skills (comma-separated)</label><input id="proj-skills" class="input" placeholder="e.g. facebook-poster, email-checker"></div>
      <div class="form-group"><label>Status</label>
        <select id="proj-status" class="input">
          <option value="active">Active</option>
          <option value="paused">Paused</option>
        </select>
      </div>
      <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
        <button class="btn-sm" onclick="document.getElementById('project-modal').style.display='none'">Cancel</button>
        <button class="btn-primary" onclick="submitCreateProject()">Create</button>
      </div>
    </div>
  `;
}

function updateSubAccountOpts(agencyId) {
  const sel = document.getElementById('proj-subaccount');
  if (!sel) return;
  sel.innerHTML = '<option value="0">None</option>';
  const ag = state.allAgencies.find(a => String(a.id) === String(agencyId));
  if (ag && ag.subaccounts) {
    for (const sa of ag.subaccounts) {
      sel.innerHTML += '<option value="' + sa.id + '" data-name="' + escapeHtml(sa.name) + '">' + escapeHtml(sa.name) + '</option>';
    }
  }
}

async function submitCreateProject() {
  const name = document.getElementById('proj-name')?.value;
  if (!name) { alert('Project name is required'); return; }

  const agencySel = document.getElementById('proj-agency');
  const subSel = document.getElementById('proj-subaccount');
  const agencyId = parseInt(agencySel?.value) || 0;
  const subId = parseInt(subSel?.value) || 0;
  const agencyName = agencySel?.selectedOptions[0]?.dataset.name || '';
  const subName = subSel?.selectedOptions[0]?.dataset.name || '';

  const agents = [...document.querySelectorAll('.proj-agent-cb:checked')].map(cb => cb.value);
  const skills = (document.getElementById('proj-skills')?.value || '').split(',').map(s => s.trim()).filter(Boolean);

  await fetch('/v1/projects', {
    method: 'POST', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({
      name, description: document.getElementById('proj-desc')?.value || '',
      agency_id: agencyId, subaccount_id: subId,
      agency_name: agencyName, subaccount_name: subName,
      assigned_agents: agents, assigned_skills: skills,
      status: document.getElementById('proj-status')?.value || 'active'
    })
  });
  document.getElementById('project-modal').style.display = 'none';
  renderProjectsPage(document.getElementById('page-content'));
}

async function toggleProjectStatus(id, current) {
  const next = current === 'active' ? 'paused' : 'active';
  await fetch('/v1/projects/' + id, {
    method: 'PUT', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({ status: next })
  });
  renderProjectsPage(document.getElementById('page-content'));
}

async function deleteProject(id) {
  if (!confirm('Delete this project?')) return;
  await fetch('/v1/projects/' + id, { method: 'DELETE' });
  renderProjectsPage(document.getElementById('page-content'));
}

// ── Agency Context Helpers ──────────────────────────────────────────

// Fetch all agencies (with sub-accounts) for the chat context dropdown.
async function fetchAllAgencies() {
  try {
    const r = await fetch('/v1/agency');
    const d = await r.json();
    const agencies = d.agencies || [];
    // Fetch sub-accounts for each agency.
    for (const ag of agencies) {
      try {
        const sr = await fetch('/v1/agency/' + ag.id + '/accounts');
        const sd = await sr.json();
        ag.subaccounts = sd.accounts || [];
      } catch (_) { ag.subaccounts = []; }
    }
    state.allAgencies = agencies;
    // Load saved active context.
    try {
      const ar = await fetch('/v1/agency/active');
      const ad = await ar.json();
      state.activeAgencyId = ad.active_agency_id || 0;
      state.activeSubAccountId = ad.active_subaccount_id || 0;
      state.activeAgencyName = ad.agency_name || '';
      state.activeSubAccountName = ad.subaccount_name || '';
    } catch (_) {}
  } catch (e) { console.error('Failed to fetch agencies:', e); }
}

// Set the active agency/sub-account context.
async function setAgencyContext(value) {
  // value format: "agency:{id}" or "sub:{agencyId}:{subId}" or "" for none.
  let agencyId = 0, subId = 0;
  if (value.startsWith('agency:')) {
    agencyId = parseInt(value.split(':')[1]) || 0;
  } else if (value.startsWith('sub:')) {
    const parts = value.split(':');
    agencyId = parseInt(parts[1]) || 0;
    subId = parseInt(parts[2]) || 0;
  }
  try {
    const r = await fetch('/v1/agency/active', {
      method: 'PUT', headers: {'Content-Type':'application/json'},
      body: JSON.stringify({ agency_id: agencyId, subaccount_id: subId })
    });
    const d = await r.json();
    state.activeAgencyId = d.active_agency_id || 0;
    state.activeSubAccountId = d.active_subaccount_id || 0;
  } catch (e) { console.error('setAgencyContext failed:', e); }
  // Resolve names from local data.
  state.activeAgencyName = '';
  state.activeSubAccountName = '';
  for (const ag of state.allAgencies) {
    if (ag.id === state.activeAgencyId) {
      state.activeAgencyName = ag.name;
      for (const sa of (ag.subaccounts || [])) {
        if (sa.id === state.activeSubAccountId) {
          state.activeSubAccountName = sa.name;
          break;
        }
      }
      break;
    }
  }
  // Re-render chat to update the dropdown display.
  if (state.page === 'chat') renderChat();
}

// Build agency context dropdown options HTML.
function buildAgencyDropdown() {
  let opts = '<option value="">No context</option>';
  for (const ag of state.allAgencies) {
    const agVal = 'agency:' + ag.id;
    const agSel = (state.activeAgencyId === ag.id && state.activeSubAccountId === 0) ? 'selected' : '';
    opts += '<option value="' + agVal + '" ' + agSel + '>' + escapeHtml(ag.name) + '</option>';
    for (const sa of (ag.subaccounts || [])) {
      const saVal = 'sub:' + ag.id + ':' + sa.id;
      const saSel = (state.activeAgencyId === ag.id && state.activeSubAccountId === sa.id) ? 'selected' : '';
      opts += '<option value="' + saVal + '" ' + saSel + '>  └ ' + escapeHtml(sa.name) + '</option>';
    }
  }
  return opts;
}

// ── Agency Page ─────────────────────────────────────────────────────
async function renderAgencyPage(container) {
  let agencies = [];
  try {
    const r = await fetch('/v1/agency');
    const d = await r.json();
    agencies = d.agencies || [];
  } catch (e) { console.error('Failed to fetch agencies:', e); }

  // Fetch dashboard stats.
  let projCount = 0, contactCount = 0, autoCount = 0, pipeCount = 0;
  try {
    const [pj, ct, au, pl] = await Promise.all([
      fetch('/v1/projects').then(r => r.json()).catch(() => ({})),
      fetch('/v1/contacts').then(r => r.json()).catch(() => ({})),
      fetch('/v1/automations').then(r => r.json()).catch(() => ({})),
      fetch('/v1/pipelines').then(r => r.json()).catch(() => ({})),
    ]);
    projCount = pj.count || 0;
    contactCount = ct.count || 0;
    autoCount = au.count || 0;
    pipeCount = pl.count || 0;
  } catch (_) {}

  container.innerHTML = `
    <div class="page-section">
      <!-- Dashboard stats bar -->
      <div style="display:grid;grid-template-columns:repeat(4,1fr);gap:.75rem;margin-bottom:1.5rem;">
        <div class="agency-card" style="text-align:center;cursor:pointer;" onclick="navigate('projects')">
          <div style="font-size:24px;font-weight:700;color:var(--accent);">${projCount}</div>
          <div style="font-size:11px;color:var(--text-muted);margin-top:2px;">Projects</div>
        </div>
        <div class="agency-card" style="text-align:center;cursor:pointer;" onclick="navigate('contacts')">
          <div style="font-size:24px;font-weight:700;color:var(--info);">${contactCount}</div>
          <div style="font-size:11px;color:var(--text-muted);margin-top:2px;">Contacts</div>
        </div>
        <div class="agency-card" style="text-align:center;cursor:pointer;" onclick="navigate('automations')">
          <div style="font-size:24px;font-weight:700;color:var(--warning);">${autoCount}</div>
          <div style="font-size:11px;color:var(--text-muted);margin-top:2px;">Automations</div>
        </div>
        <div class="agency-card" style="text-align:center;cursor:pointer;" onclick="navigate('pipelines')">
          <div style="font-size:24px;font-weight:700;color:var(--success);">${pipeCount}</div>
          <div style="font-size:11px;color:var(--text-muted);margin-top:2px;">Pipelines</div>
        </div>
      </div>

      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);display:flex;align-items:center;">Your Agencies (${agencies.length})${helpIcon('Create and manage business agencies. Each agency has its own agents, contacts, pipelines, and settings.')}</h3>
        <button class="btn-primary" onclick="showCreateAgencyModal()">+ New Agency</button>
      </div>
      
        ${agencies.length === 0 ? '<div class="empty-state">No agencies yet. Create one to get started.</div>' :
          agencies.map(ag => `
            <div class="agency-card" id="agency-card-${ag.id}">
              <div class="agency-card-header">
                <div class="agency-card-info">
                  <div class="agency-card-name">${escapeHtml(ag.name)}</div>
                  ${ag.industry ? `<div class="agency-card-niche">${escapeHtml(ag.industry)}</div>` : ''}
                  ${ag.domain ? `<a class="agency-card-url" href="${ag.domain.startsWith('http') ? ag.domain : 'https://' + ag.domain}" target="_blank">${escapeHtml(ag.domain)}</a>` : ''}
                  <div class="agency-card-colors">
                    ${ag.primary_color ? `<span style="background:${ag.primary_color}" title="Primary"></span>` : ''}
                    ${ag.secondary_color ? `<span style="background:${ag.secondary_color}" title="Secondary"></span>` : ''}
                    ${ag.accent_color ? `<span style="background:${ag.accent_color}" title="Accent"></span>` : ''}
                  </div>
                </div>
                <div class="agency-card-id">ID: ${ag.id}</div>
              </div>
              <div class="agency-card-actions">
                <button class="btn-sm" onclick="toggleSubAccounts(${ag.id})">Sub-Accounts</button>
                <button class="btn-sm" onclick="scrapeForAgency(${ag.id})">Scrape Website</button>
                <button class="btn-sm btn-danger" onclick="deleteAgency(${ag.id})">Delete</button>
              </div>
              <div class="agency-subs" id="subs-${ag.id}" style="display:none;"></div>
            </div>
          `).join('')}
      </div>
    </div>
    <div id="agency-modal" class="modal-overlay" style="display:none;"></div>
    
  `;
}

async function showCreateAgencyModal() {
  const modal = document.getElementById('agency-modal');
  if (!modal) return;
  modal.style.display = 'flex';
  modal.innerHTML = `
    <div class="modal-card" style="max-width:500px;width:90%;">
      <h3 style="margin:0 0 1rem;color:var(--text-primary);">Create Agency</h3>
      <div class="form-group"><label>Name *</label><input id="ag-name" class="input" placeholder="Agency name"></div>
      <div class="form-group"><label>Industry</label><input id="ag-industry" class="input" placeholder="e.g. Marketing, Real Estate"></div>
      <div class="form-group"><label>Domain</label><input id="ag-domain" class="input" placeholder="e.g. myagency.com"></div>
      <div class="form-group"><label>Tagline</label><input id="ag-tagline" class="input" placeholder="Your agency tagline"></div>
      <div style="display:flex;gap:1rem;">
        <div class="form-group" style="flex:1;"><label>Primary Color</label><input id="ag-primary" type="color" value="#f97316" style="width:100%;height:36px;border:none;cursor:pointer;"></div>
        <div class="form-group" style="flex:1;"><label>Secondary Color</label><input id="ag-secondary" type="color" value="#0ea5e9" style="width:100%;height:36px;border:none;cursor:pointer;"></div>
        <div class="form-group" style="flex:1;"><label>Accent Color</label><input id="ag-accent" type="color" value="#22c55e" style="width:100%;height:36px;border:none;cursor:pointer;"></div>
      </div>
      <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
        <button class="btn-sm" onclick="document.getElementById('agency-modal').style.display='none'">Cancel</button>
        <button class="btn-primary" onclick="submitCreateAgency()">Create</button>
      </div>
    </div>
  `;
}

async function submitCreateAgency() {
  const body = {
    name: document.getElementById('ag-name').value,
    industry: document.getElementById('ag-industry').value,
    domain: document.getElementById('ag-domain').value,
    tagline: document.getElementById('ag-tagline').value,
    primary_color: document.getElementById('ag-primary').value,
    secondary_color: document.getElementById('ag-secondary').value,
    accent_color: document.getElementById('ag-accent').value,
  };
  if (!body.name) return alert('Name is required');
  await fetch('/v1/agency', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body) });
  document.getElementById('agency-modal').style.display = 'none';
  renderAgencyPage(document.getElementById('page-content'));
}

async function toggleSubAccounts(agencyId) {
  const panel = document.getElementById('subs-' + agencyId);
  if (!panel) return;
  if (panel.style.display !== 'none' && panel.innerHTML !== '') { panel.style.display = 'none'; return; }
  let accounts = [];
  try {
    const r = await fetch(`/v1/agency/${agencyId}/accounts`);
    const d = await r.json();
    accounts = d.accounts || [];
  } catch (e) { console.error(e); }
  panel.style.display = 'block';
  panel.innerHTML = `
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:1rem;">
      <span class="agency-subs-title">Sub-Accounts (${accounts.length})</span>
      <button class="btn-sm btn-primary" onclick="createSubAccount(${agencyId})">+ Add Business</button>
    </div>
    ${accounts.length === 0 ? '<div class="empty-state">No sub-accounts yet.</div>' :
      accounts.map(sa => `
        <div class="agency-sub-card">
          <div class="agency-sub-name">${escapeHtml(sa.name)}</div>
          ${sa.website ? `<span style="color:var(--text-muted);margin-left:1rem;">${sa.website}</span>` : ''}
          ${sa.industry ? `<span class="badge" style="margin-left:.5rem;">${sa.industry}</span>` : ''}
        </div>
      `).join('')}
  `;
}

async function createSubAccount(agencyId) {
  // Show inline form instead of prompt() which can be blocked by some browsers.
  const panel = document.getElementById('subs-' + agencyId);
  if (!panel) return;
  panel.style.display = 'block';
  panel.innerHTML = `
    <div class="agency-subs-header">
      <span class="agency-subs-title">Add New Business</span>
    </div>
    <div style="display:flex;flex-direction:column;gap:8px;padding:8px 0;">
      <input id="sub-name-${agencyId}" class="input" placeholder="Business name *" autofocus>
      <input id="sub-website-${agencyId}" class="input" placeholder="Website URL (optional)">
      <input id="sub-industry-${agencyId}" class="input" placeholder="Industry (optional)">
      <div style="display:flex;gap:6px;margin-top:4px;">
        <button class="btn-sm btn-primary" onclick="submitSubAccount(${agencyId})">Create</button>
        <button class="btn-sm" onclick="toggleSubAccounts(${agencyId}); toggleSubAccounts(${agencyId});">Cancel</button>
      </div>
    </div>
  `;
  setTimeout(() => document.getElementById('sub-name-' + agencyId)?.focus(), 50);
}

async function submitSubAccount(agencyId) {
  const name = document.getElementById('sub-name-' + agencyId)?.value;
  const website = document.getElementById('sub-website-' + agencyId)?.value || '';
  const industry = document.getElementById('sub-industry-' + agencyId)?.value || '';
  if (!name) { alert('Business name is required'); return; }
  await fetch('/v1/agency/' + agencyId + '/accounts', {
    method: 'POST', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({ name, website, industry })
  });
  // Force-refresh: hide first so toggle re-fetches
  const panel = document.getElementById('subs-' + agencyId);
  if (panel) { panel.style.display = 'none'; panel.innerHTML = ''; }
  toggleSubAccounts(agencyId);
}

async function deleteSubAccount(agencyId, accountId) {
  if (!confirm('Delete this sub-account?')) return;
  await fetch('/v1/agency/' + agencyId + '/accounts/' + accountId, { method: 'DELETE' });
  const panel = document.getElementById('subs-' + agencyId);
  if (panel) { panel.style.display = 'none'; panel.innerHTML = ''; }
  toggleSubAccounts(agencyId);
}

async function scrapeForAgency(agencyId) {
  const url = prompt('Enter website URL to scrape for knowledge:');
  if (!url) return;
  const r = await fetch(`/v1/agency/${agencyId}/scrape`, {
    method: 'POST', headers: {'Content-Type':'application/json'},
    body: JSON.stringify({ url })
  });
  const d = await r.json();
  alert(`Scraped ${d.chars || 0} characters from ${url}`);
}

async function deleteAgency(id) {
  if (!confirm('Delete this agency?')) return;
  await fetch(`/v1/agency/${id}`, { method: 'DELETE' });
  renderAgencyPage(document.getElementById('page-content'));
}

// ── Credentials Page ────────────────────────────────────────────────
async function renderCredentialsPage(container) {
  let creds = [];
  try {
    const r = await fetch('/v1/credentials');
    const d = await r.json();
    creds = d.credentials || [];
  } catch (e) { console.error('Failed to fetch credentials:', e); }

  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);display:flex;align-items:center;">Credential Vault (${creds.length})${helpIcon('Securely store API keys, tokens, and passwords. Credentials are encrypted and available to agents and automations.')}</h3>
        <button class="btn-primary" onclick="showCreateCredentialModal()">+ Add Credential</button>
      </div>
      <div style="color:var(--text-muted);font-size:.85rem;margin-bottom:1rem;">
        Encrypted with AES-256-GCM. Values are masked until revealed.
      </div>
      <table class="data-table" style="width:100%;border-collapse:collapse;">
        <thead>
          <tr style="border-bottom:1px solid var(--border);">
            <th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Name</th>
            <th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Type</th>
            <th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Provider</th>
            <th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Scope</th>
            <th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Value</th>
            <th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Actions</th>
          </tr>
        </thead>
        <tbody>
          ${creds.length === 0 ? '<tr><td colspan="6" class="empty-state" style="padding:2rem;text-align:center;">No credentials stored yet.</td></tr>' :
            creds.map(c => `
              <tr style="border-bottom:1px solid var(--border-light);">
                <td style="padding:.5rem;color:var(--text-primary);font-weight:500;">${c.name}</td>
                <td style="padding:.5rem;"><span class="badge">${c.type}</span></td>
                <td style="padding:.5rem;color:var(--text-secondary);">${c.provider || '-'}</td>
                <td style="padding:.5rem;"><span class="badge" style="background:${c.scope === 'global' ? 'var(--accent)' : 'var(--info)'};color:#000;">${c.scope}${c.scope_id ? ':' + c.scope_id : ''}</span></td>
                <td style="padding:.5rem;color:var(--text-muted);font-family:monospace;" id="cred-val-${c.id}">••••••••</td>
                <td style="padding:.5rem;display:flex;gap:.25rem;">
                  <button class="btn-sm" onclick="revealCredential(${c.id})">Reveal</button>
                  <button class="btn-sm btn-danger" onclick="deleteCredential(${c.id})">Delete</button>
                </td>
              </tr>
            `).join('')}
        </tbody>
      </table>
    </div>
    <div id="cred-modal" class="modal-overlay" style="display:none;"></div>
  `;
}

async function showCreateCredentialModal() {
  const modal = document.getElementById('cred-modal');
  if (!modal) return;
  modal.style.display = 'flex';
  modal.innerHTML = `
    <div class="modal-card" style="max-width:450px;width:90%;">
      <h3 style="margin:0 0 1rem;color:var(--text-primary);">Add Credential</h3>
      <div class="form-group"><label>Name *</label><input id="cred-name" class="input" placeholder="e.g. GHL API Key"></div>
      <div class="form-group"><label>Value *</label><input id="cred-value" class="input" type="password" placeholder="API key or password"></div>
      <div class="form-group"><label>Type</label>
        <select id="cred-type" class="input">
          <option value="api_key">API Key</option><option value="password">Password</option>
          <option value="oauth">OAuth Token</option><option value="token">Bearer Token</option>
          <option value="custom">Custom</option>
        </select>
      </div>
      <div class="form-group"><label>Provider</label><input id="cred-provider" class="input" placeholder="e.g. GoHighLevel, OpenAI"></div>
      <div class="form-group"><label>Scope</label>
        <select id="cred-scope" class="input">
          <option value="global">Global</option><option value="agency">Agency</option><option value="subaccount">Sub-Account</option>
        </select>
      </div>
      <div class="form-group"><label>Description</label><input id="cred-desc" class="input" placeholder="Optional description"></div>
      <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
        <button class="btn-sm" onclick="document.getElementById('cred-modal').style.display='none'">Cancel</button>
        <button class="btn-primary" onclick="submitCreateCredential()">Save</button>
      </div>
    </div>
  `;
}

async function submitCreateCredential() {
  const body = {
    name: document.getElementById('cred-name').value,
    value: document.getElementById('cred-value').value,
    type: document.getElementById('cred-type').value,
    provider: document.getElementById('cred-provider').value,
    scope: document.getElementById('cred-scope').value,
    description: document.getElementById('cred-desc').value,
  };
  if (!body.name || !body.value) return alert('Name and value are required');
  await fetch('/v1/credentials', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body) });
  document.getElementById('cred-modal').style.display = 'none';
  renderCredentialsPage(document.getElementById('page-content'));
}

async function revealCredential(id) {
  try {
    const r = await fetch(`/v1/credentials/${id}/reveal`);
    const d = await r.json();
    const el = document.getElementById(`cred-val-${id}`);
    if (el) el.textContent = d.value || '(empty)';
    setTimeout(() => { if (el) el.textContent = '••••••••'; }, 10000); // Auto-hide after 10s.
  } catch (e) { alert('Failed to reveal credential'); }
}

async function deleteCredential(id) {
  if (!confirm('Delete this credential?')) return;
  await fetch(`/v1/credentials/${id}`, { method: 'DELETE' });
  renderCredentialsPage(document.getElementById('page-content'));
}

// ── Automations Page ────────────────────────────────────────────────
async function renderAutomationsPage(container) {
  let jobs = [];
  try {
    const r = await fetch('/v1/automations');
    const d = await r.json();
    jobs = d.automations || [];
  } catch (e) { console.error('Failed to fetch automations:', e); }

  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);display:flex;align-items:center;">Scheduled Automations (${jobs.length})${helpIcon('Set up recurring tasks with cron expressions. Automations run agents on a schedule automatically.')}</h3>
        <button class="btn-primary" onclick="showCreateAutomationModal()">+ New Automation</button>
      </div>
      <div id="automations-grid" class="cards-grid">
        ${jobs.length === 0 ? '<div class="empty-state">No automations yet. Create one to schedule agent work.</div>' :
          jobs.map(j => `
            <div class="stat-card" style="border-left:4px solid ${j.enabled ? 'var(--success)' : 'var(--text-muted)'};">
              <div style="display:flex;justify-content:space-between;align-items:center;">
                <h4 style="margin:0;color:var(--text-primary);">${j.name}</h4>
                <span class="badge" style="background:${j.enabled ? 'var(--success)' : 'var(--text-muted)'};color:#fff;padding:2px 8px;border-radius:4px;font-size:.7rem;">${j.enabled ? 'ACTIVE' : 'DISABLED'}</span>
              </div>
              <div style="color:var(--text-secondary);font-size:.85rem;margin-top:.25rem;">
                Agent: <strong>${j.agent}</strong> | Type: <span class="badge">${j.schedule_type || j.type}</span>
              </div>
              <div style="color:var(--text-muted);font-size:.8rem;margin-top:.25rem;font-family:monospace;">
                Schedule: ${j.schedule || 'N/A'}
              </div>
              <div style="color:var(--text-muted);font-size:.8rem;margin-top:.25rem;">
                Prompt: "${(j.prompt || '').substring(0, 80)}${(j.prompt || '').length > 80 ? '...' : ''}"
              </div>
              <div style="display:flex;gap:.5rem;margin-top:.75rem;flex-wrap:wrap;">
                <div style="font-size:.75rem;color:var(--text-muted);">
                  Runs: ${j.run_count || 0} | Last: ${j.last_run ? new Date(j.last_run).toLocaleString() : 'never'}
                </div>
              </div>
              <div style="display:flex;gap:.5rem;margin-top:.5rem;">
                <button class="btn-sm btn-primary" onclick="triggerAutomation('${j.id}')">Trigger Now</button>
                <button class="btn-sm" onclick="viewAutomationHistory('${j.id}')">History</button>
                <button class="btn-sm btn-danger" onclick="deleteAutomation('${j.id}')">Delete</button>
              </div>
            </div>
          `).join('')}
      </div>
    </div>
    <div id="auto-modal" class="modal-overlay" style="display:none;"></div>
    <div id="auto-history" class="page-section" style="margin-top:1rem;display:none;"></div>
  `;
}

async function showCreateAutomationModal() {
  const modal = document.getElementById('auto-modal');
  if (!modal) return;

  // Get available agents for dropdown.
  let agentNames = [];
  try {
    const r = await fetch('/v1/agents');
    const d = await r.json();
    agentNames = (d.agents || []).map(a => a.name);
  } catch (e) { agentNames = ['mike']; }

  modal.style.display = 'flex';
  modal.innerHTML = `
    <div class="modal-card" style="max-width:500px;width:90%;">
      <h3 style="margin:0 0 1rem;color:var(--text-primary);">Create Automation</h3>
      <div class="form-group"><label>Name *</label><input id="auto-name" class="input" placeholder="e.g. Daily Social Media Post"></div>
      <div class="form-group"><label>Agent *</label>
        <select id="auto-agent" class="input">
          ${agentNames.map(n => `<option value="${n}">${n}</option>`).join('')}
        </select>
      </div>
      <div class="form-group"><label>Prompt *</label><textarea id="auto-prompt" class="input" rows="3" placeholder="What should the agent do?"></textarea></div>
      <div class="form-group"><label>Schedule Type</label>
        <select id="auto-stype" class="input" onchange="updateScheduleHelp()">
          <option value="every">Interval (every X)</option>
          <option value="cron">Cron Expression</option>
          <option value="at">One-shot (at time)</option>
        </select>
      </div>
      <div class="form-group"><label>Schedule *</label><input id="auto-schedule" class="input" placeholder="e.g. 30m, 1h, 0 9 * * 1-5"></div>
      <div id="schedule-help" style="color:var(--text-muted);font-size:.8rem;margin-bottom:.5rem;">Examples: 30m, 1h, 24h</div>
      <div class="form-group"><label>Execution Mode</label>
        <select id="auto-mode" class="input">
          <option value="main">Main Session (shares context)</option>
          <option value="isolated">Isolated (fresh session)</option>
        </select>
      </div>
      <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
        <button class="btn-sm" onclick="document.getElementById('auto-modal').style.display='none'">Cancel</button>
        <button class="btn-primary" onclick="submitCreateAutomation()">Create</button>
      </div>
    </div>
  `;
}

function updateScheduleHelp() {
  const type = document.getElementById('auto-stype').value;
  const help = document.getElementById('schedule-help');
  if (!help) return;
  const hints = {
    every: 'Examples: 30m, 1h, 24h',
    cron: 'Examples: 0 9 * * 1-5 (weekdays 9am), */30 * * * * (every 30min)',
    at: 'ISO format: 2026-03-14T09:00:00-04:00'
  };
  help.textContent = hints[type] || '';
}

async function submitCreateAutomation() {
  const body = {
    name: document.getElementById('auto-name').value,
    agent: document.getElementById('auto-agent').value,
    prompt: document.getElementById('auto-prompt').value,
    schedule_type: document.getElementById('auto-stype').value,
    schedule: document.getElementById('auto-schedule').value,
    execution_mode: document.getElementById('auto-mode').value,
    type: 'cron',
  };
  if (!body.name || !body.agent || !body.prompt || !body.schedule) return alert('Name, agent, prompt, and schedule are required');
  await fetch('/v1/automations', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify(body) });
  document.getElementById('auto-modal').style.display = 'none';
  renderAutomationsPage(document.getElementById('page-content'));
}

async function triggerAutomation(id) {
  const r = await fetch(`/v1/automations/${id}/trigger`, { method: 'POST' });
  const d = await r.json();
  alert(d.status === 'triggered' ? 'Automation triggered!' : 'Failed to trigger');
  renderAutomationsPage(document.getElementById('page-content'));
}

async function viewAutomationHistory(id) {
  const panel = document.getElementById('auto-history');
  if (!panel) return;
  let records = [];
  try {
    const r = await fetch(`/v1/automations/${id}/history`);
    const d = await r.json();
    records = d.history || [];
  } catch (e) { console.error(e); }
  panel.style.display = 'block';
  panel.innerHTML = `
    <h3 style="color:var(--text-primary);margin-bottom:.5rem;">Run History (${records.length})</h3>
    ${records.length === 0 ? '<div class="empty-state">No runs yet.</div>' :
      records.map(r => `
        <div class="stat-card" style="margin-bottom:.25rem;padding:.5rem;">
          <span style="color:var(--text-primary);">${new Date(r.run_at).toLocaleString()}</span>
          <span class="badge" style="margin-left:.5rem;background:${r.status === 'success' ? 'var(--success)' : 'var(--error)'};color:#fff;">${r.status}</span>
        </div>
      `).join('')}
  `;
}

async function deleteAutomation(id) {
  if (!confirm('Delete this automation?')) return;
  await fetch(`/v1/automations/${id}`, { method: 'DELETE' });
  renderAutomationsPage(document.getElementById('page-content'));
}

// ── Page: Sessions ─────────────────────────────────────────────────
async function renderSessionsPage(container) {
  if (!container) container = document.getElementById('page-content');
  container.innerHTML = '<div class="spinner" style="margin:40px auto;"></div>';

  let sessions = [];
  try {
    const res = await fetch('/v1/sessions');
    const data = await res.json();
    sessions = data.sessions || [];
  } catch (e) {
    console.error('Sessions fetch error:', e);
  }

  if (state.page !== 'sessions') return;

  const channelColors = {
    web: '#f97316', discord: '#5865F2', whatsapp: '#25D366',
    telegram: '#26A5E4', api: '#8b5cf6'
  };
  const channelIcons = {
    web: '🌐', discord: '💬', whatsapp: '📱', telegram: '✈️', api: '⚡'
  };

  container.innerHTML = `
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:20px;">
      <div class="section-label" style="margin:0;">Chat Sessions (${sessions.length})</div>
      <button class="btn-primary" onclick="createNewSession()" style="padding:6px 16px;border-radius:6px;border:none;background:var(--accent);color:#fff;cursor:pointer;font-size:13px;">
        + New Session
      </button>
    </div>

    ${sessions.length === 0 ? `
      <div class="empty-state">
        <div class="empty-icon">💬</div>
        <div class="empty-text">No chat sessions yet. Start a conversation to create one.</div>
      </div>
    ` : `
      <div class="card-grid">
        ${sessions.map(s => {
          const ch = s.channel || 'web';
          const statusLabel = s.status === 'active' ? '● ACTIVE' : s.status || 'archived';
          const statusColor = s.status === 'active' ? 'var(--success)' : 'var(--text-muted)';
          const timeAgo = formatTimeAgo(s.timestamp);
          return `
            <div class="agent-card" onclick="resumeSession(${s.id})" style="cursor:pointer;border-left:3px solid ${channelColors[ch] || '#666'};">
              <div class="agent-card-head" style="align-items:flex-start;">
                <div style="flex:1;">
                  <span class="agent-card-name" style="font-size:14px;">${s.title || 'Untitled Session'}</span>
                  <div style="display:flex;gap:6px;margin-top:4px;flex-wrap:wrap;">
                    <span class="badge" style="background:${channelColors[ch] || '#666'};color:#fff;font-size:9px;padding:2px 6px;">
                      ${channelIcons[ch] || '🌐'} ${ch}
                    </span>
                    <span style="font-size:10px;color:${statusColor};font-weight:600;">${statusLabel}</span>
                  </div>
                </div>
                <span style="font-size:10px;color:var(--text-muted);white-space:nowrap;">${timeAgo}</span>
              </div>
              <div class="agent-card-desc" style="margin:6px 0;font-size:12px;max-height:48px;overflow:hidden;">
                ${s.summary || 'No summary available'}
              </div>
              <div style="display:flex;align-items:center;justify-content:space-between;margin-top:8px;">
                <div style="display:flex;gap:4px;flex-wrap:wrap;">
                  ${(s.agents_used || []).slice(0, 3).map(a => `
                    <span class="tool-tag" style="font-size:9px;">${a}</span>
                  `).join('')}
                </div>
                <div style="display:flex;align-items:center;gap:12px;">
                  <span style="font-size:11px;color:var(--text-muted);">${s.message_count || 0} msgs</span>
                  <button onclick="event.stopPropagation();compactSession(${s.id})" title="Compact context" style="background:none;border:none;cursor:pointer;font-size:14px;padding:0;">🗜️</button>
                  <button onclick="event.stopPropagation();deleteSession(${s.id})" title="Delete session" style="background:none;border:none;cursor:pointer;font-size:14px;padding:0;">🗑️</button>
                </div>
              </div>
            </div>
          `;
        }).join('')}
      </div>
    `}
  `;
}

function formatTimeAgo(timestamp) {
  if (!timestamp) return '';
  const now = new Date();
  const then = new Date(timestamp);
  const diffMs = now - then;
  const diffMins = Math.floor(diffMs / 60000);
  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHrs = Math.floor(diffMins / 60);
  if (diffHrs < 24) return `${diffHrs}h ago`;
  const diffDays = Math.floor(diffHrs / 24);
  if (diffDays < 7) return `${diffDays}d ago`;
  return then.toLocaleDateString();
}

async function createNewSession() {
  try {
    const res = await fetch('/v1/sessions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ channel: 'web', title: 'New Session' })
    });
    const data = await res.json();
    state.sessionId = data.session_id;
    // Clear chat and topology for the fresh session.
    state.chatMessages = [];
    state.liveEvents = [];
    state.agentActivity = {};
    state.sessionActiveAgents = new Set();
    state.isThinking = false;
    navigate('chat');
  } catch (e) {
    console.error('Create session error:', e);
  }
}

async function resumeSession(id) {
  state.sessionId = id;
  // Clear current state for the resumed session.
  state.chatMessages = [];
  state.liveEvents = [];
  state.agentActivity = {};
  state.sessionActiveAgents = new Set();
  state.isThinking = false;

  // Load old messages from the API.
  try {
    const res = await fetch(`/v1/sessions/${id}/messages`);
    if (res.ok) {
      const data = await res.json();
      const msgs = data.messages || [];
      // Map backend LogMessage format to frontend chatMessages format.
      state.chatMessages = msgs
        .filter(m => m.role === 'user' || m.role === 'assistant')
        .map(m => ({
          role: m.role,
          content: m.content,
          agent: m.agent || (m.role === 'assistant' ? 'orchestrator' : null),
          timestamp: m.timestamp || new Date().toISOString(),
        }));
    }
  } catch (e) {
    console.error('Failed to load session messages:', e);
  }

  navigate('chat');
}

async function compactSession(id) {
  if (!confirm('Compact this session? This will summarize older messages to save context.')) return;
  try {
    await fetch(`/v1/sessions/${id}/compact`, { method: 'POST' });
    renderSessionsPage(document.getElementById('page-content'));
  } catch (e) {
    console.error('Compact error:', e);
  }
}

async function deleteSession(id) {
  if (!confirm('Archive this session?')) return;
  try {
    await fetch(`/v1/sessions/${id}`, { method: 'DELETE' });
    renderSessionsPage(document.getElementById('page-content'));
  } catch (e) {
    console.error('Delete session error:', e);
  }
}

// ── Page: Marketplace ─────────────────────────────────────────────

const marketplaceState = {
  items: [],
  filter: 'all',
  search: '',
  loading: false,
};

async function fetchMarketplaceCatalog() {
  const data = await api('/v1/marketplace/catalog');
  if (data && data.items) {
    marketplaceState.items = data.items;
  }
  return data;
}

async function renderMarketplacePage(container) {
  if (!container) container = document.getElementById('page-content');
  marketplaceState.loading = true;

  container.innerHTML = `
    <div class="marketplace-wrapper">
      <div class="marketplace-loading">
        <div class="spinner"></div>
        <span>Loading marketplace catalog...</span>
      </div>
    </div>
  `;

  await fetchMarketplaceCatalog();
  marketplaceState.loading = false;
  renderMarketplaceContent(container);
}

function renderMarketplaceContent(container) {
  if (!container) container = document.getElementById('page-content');

  const items = marketplaceState.items;
  const filter = marketplaceState.filter;
  const search = marketplaceState.search.toLowerCase();

  // Filter items.
  let filtered = items;
  if (filter !== 'all') {
    filtered = filtered.filter(item => item.type === filter);
  }
  if (search) {
    filtered = filtered.filter(item =>
      item.display_name.toLowerCase().includes(search) ||
      item.description.toLowerCase().includes(search) ||
      (item.tags || []).some(t => t.toLowerCase().includes(search))
    );
  }

  // Count by type.
  const counts = { all: items.length, skill: 0, agent: 0, plugin: 0, tool: 0 };
  items.forEach(item => { counts[item.type] = (counts[item.type] || 0) + 1; });

  const installedCount = items.filter(i => i.installed).length;

  const filterBtn = (type, label, icon) => {
    const active = filter === type ? 'active' : '';
    const count = counts[type] || 0;
    return `<button class="mp-filter-btn ${active}" onclick="setMarketplaceFilter('${type}')">
      ${icon} ${label} <span class="mp-filter-count">${count}</span>
    </button>`;
  };

  const cards = filtered.map(item => {
    const typeColors = { skill: 'var(--blue)', agent: 'var(--green)', plugin: 'var(--accent)', tool: '#a78bfa' };
    const typeColor = typeColors[item.type] || 'var(--accent)';
    const typeBadge = item.type.charAt(0).toUpperCase() + item.type.slice(1);
    const installedBadge = item.installed
      ? `<span class="mp-badge mp-badge-installed">Installed</span>`
      : '';
    const coreBadge = item.is_core
      ? `<span class="mp-badge mp-badge-core">Core</span>`
      : '';

    // Dependency info for agents.
    const depSkills = item.requires_skills || [];
    const depTools = item.requires_tools || [];
    const depBadge = (item.type === 'agent' && (depSkills.length > 0 || depTools.length > 0))
      ? `<span class="mp-dep-badge" title="Requires: ${depSkills.length} skills, ${depTools.length} tools">\u26d3 ${depSkills.length + depTools.length} deps</span>`
      : '';

    // For tool type, show the individual tool names.
    const toolsList = (item.type === 'tool' && item.tools && item.tools.length > 0)
      ? `<div class="mp-tools-list">${item.tools.slice(0, 5).map(t => `<code class="mp-tool-name">${t}</code>`).join('')}${item.tools.length > 5 ? `<span class="mp-tool-more">+${item.tools.length - 5} more</span>` : ''}</div>`
      : '';

    let actionBtn;
    if (item.is_core) {
      actionBtn = `<button class="btn mp-btn-active" disabled>Core</button>`;
    } else if (item.installed) {
      actionBtn = item.type === 'skill'
        ? `<button class="btn mp-btn-uninstall" onclick="marketplaceUninstall('${item.name}', '${item.type}')">Uninstall</button>`
        : `<button class="btn mp-btn-active" disabled>Active</button>`;
    } else if (item.type === 'plugin') {
      actionBtn = `<button class="btn mp-btn-config" onclick="marketplaceInstall('${item.name}', '${item.type}')">Configure</button>`;
    } else if (item.type === 'tool') {
      actionBtn = `<button class="btn mp-btn-active" disabled>Built-in</button>`;
    } else if (item.type === 'agent' && depSkills.length > 0) {
      actionBtn = `<button class="btn btn-primary mp-btn-install" onclick="marketplaceInstallWithDeps('${item.name}', '${item.type}')">Install</button>`;
    } else {
      actionBtn = `<button class="btn btn-primary mp-btn-install" onclick="marketplaceInstall('${item.name}', '${item.type}')">Install</button>`;
    }

    const tagPills = (item.tags || []).slice(0, 3).map(t =>
      `<span class="mp-tag">${t}</span>`
    ).join('');

    return `
      <div class="mp-card ${item.installed ? 'mp-card-installed' : ''} ${item.is_core ? 'mp-card-core' : ''}" id="mp-card-${item.name}">
        <div class="mp-card-header">
          <div class="mp-card-icon">${item.icon || '\ud83d\udce6'}</div>
          <div class="mp-card-meta">
            <h3 class="mp-card-title">${escapeHtml(item.display_name)}</h3>
            <div class="mp-card-badges">
              <span class="mp-badge" style="background:${typeColor}">${typeBadge}</span>
              ${coreBadge}
              ${installedBadge}
              ${depBadge}
            </div>
          </div>
        </div>
        <p class="mp-card-desc">${escapeHtml(item.description)}</p>
        ${toolsList}
        <div class="mp-card-footer">
          <div class="mp-card-info">
            <span class="mp-card-author">${escapeHtml(item.author)}</span>
            <span class="mp-card-version">v${item.version}</span>
            ${tagPills}
          </div>
          ${actionBtn}
        </div>
      </div>
    `;
  }).join('');

  container.innerHTML = `
    <div class="marketplace-wrapper">
      <div class="mp-header">
        <div class="mp-header-left">
          <div class="mp-stats">
            <span class="mp-stat">${items.length} Available</span>
            <span class="mp-stat-sep">|</span>
            <span class="mp-stat mp-stat-installed">${installedCount} Installed</span>
          </div>
        </div>
        <div class="mp-search-box">
          <svg class="mp-search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
          <input type="text" class="mp-search-input" id="mp-search" placeholder="Search skills, agents, tools, plugins..."
            value="${escapeHtml(marketplaceState.search)}"
            oninput="setMarketplaceSearch(this.value)">
        </div>
      </div>

      <div class="mp-filters">
        ${filterBtn('all', 'All', '\ud83d\udce6')}
        ${filterBtn('skill', 'Skills', '\ud83e\udde0')}
        ${filterBtn('agent', 'Agents', '\ud83e\udd16')}
        ${filterBtn('tool', 'Tools', '\ud83d\udee0\ufe0f')}
        ${filterBtn('plugin', 'Plugins', '\ud83d\udd0c')}
      </div>

      ${filtered.length === 0 ? `
        <div class="empty-state">
          <div class="empty-icon">🔍</div>
          <div class="empty-text">No items match your search</div>
        </div>
      ` : `
        <div class="mp-grid">
          ${cards}
        </div>
      `}
    </div>
  `;
}

function setMarketplaceFilter(type) {
  marketplaceState.filter = type;
  renderMarketplaceContent();
}

function setMarketplaceSearch(value) {
  marketplaceState.search = value;
  renderMarketplaceContent();
  // Restore focus to search input after re-render.
  const searchInput = document.getElementById('mp-search');
  if (searchInput) {
    searchInput.focus();
    searchInput.setSelectionRange(value.length, value.length);
  }
}

async function marketplaceInstall(name, type) {
  const card = document.getElementById(`mp-card-${name}`);
  if (card) {
    const btn = card.querySelector('.mp-btn-install, .mp-btn-config');
    if (btn) {
      btn.disabled = true;
      btn.textContent = 'Installing...';
    }
  }

  const result = await api('/v1/marketplace/install', {
    method: 'POST',
    body: JSON.stringify({ name, type }),
  });

  if (result && result.status === 'installed') {
    await fetchMarketplaceCatalog();
    renderMarketplaceContent();
  } else if (result && result.message) {
    alert(result.message);
    if (card) {
      const btn = card.querySelector('button[disabled]');
      if (btn) { btn.disabled = false; btn.textContent = type === 'plugin' ? 'Configure' : 'Install'; }
    }
  } else {
    const errorMsg = result && result.error ? result.error : 'Install failed';
    alert(errorMsg);
    if (card) {
      const btn = card.querySelector('button[disabled]');
      if (btn) { btn.disabled = false; btn.textContent = type === 'plugin' ? 'Configure' : 'Install'; }
    }
  }
}

async function marketplaceInstallWithDeps(name, type) {
  // Fetch dependency preview first.
  const deps = await api(`/v1/marketplace/dependencies?name=${encodeURIComponent(name)}&type=${encodeURIComponent(type)}`);

  if (deps && deps.missing_skills && deps.missing_skills.length > 0) {
    const skillList = deps.missing_skills.join(', ');
    const confirmed = confirm(
      `Installing "${name}" will also install ${deps.missing_skills.length} required skill(s):\n\n${skillList}\n\nContinue?`
    );
    if (!confirmed) return;
  }

  await marketplaceInstall(name, type);
}

async function marketplaceUninstall(name, type) {
  // Check if this item is core.
  const item = marketplaceState.items.find(i => i.name === name && i.type === type);
  if (item && item.is_core) {
    alert('Cannot uninstall core items. They are required for the system to function.');
    return;
  }

  if (!confirm(`Uninstall "${name}"? This will remove the skill files.`)) return;

  const result = await api('/v1/marketplace/uninstall', {
    method: 'POST',
    body: JSON.stringify({ name, type }),
  });

  if (result && result.status === 'uninstalled') {
    await fetchMarketplaceCatalog();
    renderMarketplaceContent();
  } else {
    const errorMsg = result && result.error ? result.error : 'Uninstall failed';
    alert(errorMsg);
  }
}

// ── Command Palette (Ctrl+K) ──────────────────────────────────────
function openCommandPalette() {
  let overlay = document.getElementById('cmd-palette-overlay');
  if (!overlay) {
    overlay = document.createElement('div');
    overlay.id = 'cmd-palette-overlay';
    overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,.55);z-index:9999;display:flex;justify-content:center;padding-top:15vh;';
    overlay.onclick = (e) => { if (e.target === overlay) closeCommandPalette(); };

    const palette = document.createElement('div');
    palette.id = 'cmd-palette';
    palette.style.cssText = 'width:520px;max-height:420px;background:var(--bg-card);border:1px solid var(--border);border-radius:12px;box-shadow:0 16px 48px rgba(0,0,0,.4);overflow:hidden;display:flex;flex-direction:column;';

    palette.innerHTML = `
      <div style="padding:12px 16px;border-bottom:1px solid var(--border);">
        <input id="cmd-palette-input" type="text" placeholder="Type a command or page name..." autofocus style="width:100%;padding:8px 12px;background:var(--bg-input);border:1px solid var(--border);border-radius:var(--radius-sm);color:var(--text-primary);font-size:14px;outline:none;">
      </div>
      <div id="cmd-palette-results" style="flex:1;overflow-y:auto;padding:8px;"></div>
      <div style="padding:6px 16px;border-top:1px solid var(--border);display:flex;gap:12px;font-size:10px;color:var(--text-muted);">
        <span><kbd style="padding:1px 4px;background:var(--bg-input);border:1px solid var(--border);border-radius:3px;font-size:9px;">↑↓</kbd> Navigate</span>
        <span><kbd style="padding:1px 4px;background:var(--bg-input);border:1px solid var(--border);border-radius:3px;font-size:9px;">Enter</kbd> Select</span>
        <span><kbd style="padding:1px 4px;background:var(--bg-input);border:1px solid var(--border);border-radius:3px;font-size:9px;">Esc</kbd> Close</span>
      </div>
    `;
    overlay.appendChild(palette);
    document.body.appendChild(overlay);
  } else {
    overlay.style.display = 'flex';
  }

  const input = document.getElementById('cmd-palette-input');
  input.value = '';
  input.focus();
  window._cmdSelected = 0;
  renderPaletteResults('');

  input.oninput = () => {
    window._cmdSelected = 0;
    renderPaletteResults(input.value);
  };
  input.onkeydown = (e) => {
    const items = document.querySelectorAll('.cmd-item');
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      window._cmdSelected = Math.min(window._cmdSelected + 1, items.length - 1);
      highlightPaletteItem(items);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      window._cmdSelected = Math.max(window._cmdSelected - 1, 0);
      highlightPaletteItem(items);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      if (items[window._cmdSelected]) items[window._cmdSelected].click();
    } else if (e.key === 'Escape') {
      closeCommandPalette();
    }
  };
}

function closeCommandPalette() {
  const overlay = document.getElementById('cmd-palette-overlay');
  if (overlay) overlay.style.display = 'none';
}

function highlightPaletteItem(items) {
  items.forEach((el, i) => {
    el.style.background = i === window._cmdSelected ? 'var(--bg-hover)' : 'transparent';
  });
}

function renderPaletteResults(query) {
  const container = document.getElementById('cmd-palette-results');
  if (!container) return;

  // Build command list from sidebar nav items
  const commands = [];
  document.querySelectorAll('.nav-item').forEach(el => {
    const page = el.getAttribute('onclick')?.match(/navigate\('([^']+)'\)/)?.[1];
    const label = el.textContent.trim();
    if (page && label) {
      commands.push({ label, action: () => { navigate(page); closeCommandPalette(); }, icon: label.split(' ')[0] || '>' });
    }
  });

  // Task-specific actions
  commands.push(
    { label: 'New Task', action: () => { navigate('tasks'); openTaskModal(); closeCommandPalette(); }, icon: '+' },
    { label: 'Refresh Tasks', action: () => { fetchTasks().then(() => renderTasks()); closeCommandPalette(); }, icon: '↻' },
    { label: 'Clear Task Filters', action: () => { state.taskSearch='';state.taskFilterPriority=-1;state.taskFilterLabel='';state.taskFilterAgent='';if(state.page==='tasks') renderTasks(); closeCommandPalette(); }, icon: '✕' }
  );

  const q = query.toLowerCase();
  const filtered = q ? commands.filter(c => c.label.toLowerCase().includes(q)) : commands;

  container.innerHTML = filtered.map((c, i) => `
    <div class="cmd-item" style="display:flex;align-items:center;gap:10px;padding:8px 12px;border-radius:6px;cursor:pointer;color:var(--text-primary);font-size:13px;${i === window._cmdSelected ? 'background:var(--bg-hover);' : ''}" onmouseenter="window._cmdSelected=${i};highlightPaletteItem(document.querySelectorAll('.cmd-item'))" onclick="(${c.action.toString()})()">
      <span style="width:20px;text-align:center;font-size:14px;">${c.icon}</span>
      <span>${escapeHtml(c.label)}</span>
    </div>
  `).join('') || '<div style="padding:16px;text-align:center;color:var(--text-muted);font-size:13px;">No results</div>';
}

// ── Global Keyboard Shortcuts ─────────────────────────────────────
document.addEventListener('keydown', (e) => {
  // Ignore when typing in inputs/textareas (except Escape and Ctrl+K)
  const tag = document.activeElement?.tagName;
  const isInput = tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT';

  // Ctrl+K / Cmd+K = command palette (always)
  if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
    e.preventDefault();
    const overlay = document.getElementById('cmd-palette-overlay');
    if (overlay && overlay.style.display === 'flex') {
      closeCommandPalette();
    } else {
      openCommandPalette();
    }
    return;
  }

  // Escape = close detail panel, modals, palette
  if (e.key === 'Escape') {
    const detailPanel = document.getElementById('task-detail-panel');
    if (detailPanel) {
      closeTaskDetail();
      return;
    }
    const palette = document.getElementById('cmd-palette-overlay');
    if (palette && palette.style.display === 'flex') {
      closeCommandPalette();
      return;
    }
    const taskModal = document.getElementById('task-modal');
    if (taskModal && taskModal.style.display === 'flex') {
      closeTaskModal();
      return;
    }
  }

  // Skip remaining shortcuts when typing in inputs
  if (isInput) return;

  // Task page shortcuts
  if (state.page === 'tasks') {
    if (e.key === 'n' || e.key === 'N') {
      e.preventDefault();
      openTaskModal();
    } else if (e.key === 'f' || e.key === 'F') {
      e.preventDefault();
      const searchEl = document.getElementById('task-search');
      if (searchEl) searchEl.focus();
    } else if (e.key === 'r' || e.key === 'R') {
      e.preventDefault();
      fetchTasks().then(() => renderTasks());
    } else if (e.key === 'v' || e.key === 'V') {
      e.preventDefault();
      state.taskViewMode = state.taskViewMode === 'board' ? 'table' : 'board';
      renderTasks();
    }
  }
});

// ── Agency Dashboard Page ──────────────────────────────────────────
async function renderAgencyDashboardPage(container) {
  let contactCount = 0, taskCount = 0, agentCount = 0, connData = {};
  try {
    const [contactsRes, tasksRes, agentsRes, connRes] = await Promise.all([
      fetch('/v1/contacts').then(r => r.json()).catch(() => ({})),
      fetch('/v1/tasks').then(r => r.json()).catch(() => ({})),
      fetch('/v1/agents').then(r => r.json()).catch(() => ({})),
      fetch('/v1/connectors').then(r => r.json()).catch(() => ({})),
    ]);
    contactCount = (contactsRes.contacts || []).length;
    taskCount = (tasksRes.tasks || []).length;
    agentCount = (agentsRes.agents || []).length;
    connData = connRes || {};
  } catch (e) { console.error('Dashboard fetch error:', e); }

  const activeTasks = (state.tasks || []).filter(t => t.status === 'In Progress').length;
  const completedTasks = (state.tasks || []).filter(t => t.status === 'Done').length;

  // Build connector status summary
  const cats = connData.connectors || {};
  const catNames = Object.keys(cats);
  const activeConns = connData.active || {};
  const integrationCards = [
    {cat:'phone', label:'Phone/SMS', icon:'📱', provider:activeConns.phone||'none', setup:'twilio'},
    {cat:'payments', label:'Payments', icon:'💳', provider:activeConns.payments||'none', setup:'stripe'},
    {cat:'crm', label:'CRM', icon:'📊', provider:activeConns.crm||'none', setup:'ghl'},
  ];

  container.innerHTML = `
    <div class="page-section">
      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:1rem;margin-bottom:1.5rem;">
        <div class="stat-card" style="border-left:4px solid var(--blue);">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;">Total Contacts</div>
          <div style="font-size:2rem;font-weight:700;color:var(--text-primary);margin:4px 0;">${contactCount}</div>
          <div style="font-size:.75rem;color:var(--text-muted);">Across all agencies</div>
        </div>
        <div class="stat-card" style="border-left:4px solid var(--green);">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;">Active Tasks</div>
          <div style="font-size:2rem;font-weight:700;color:var(--text-primary);margin:4px 0;">${activeTasks}</div>
          <div style="font-size:.75rem;color:var(--text-muted);">${completedTasks} completed</div>
        </div>
        <div class="stat-card" style="border-left:4px solid var(--accent);">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;">Agents Online</div>
          <div style="font-size:2rem;font-weight:700;color:var(--text-primary);margin:4px 0;">${agentCount}</div>
          <div style="font-size:.75rem;color:var(--text-muted);">Ready for work</div>
        </div>
        <div class="stat-card" style="border-left:4px solid #a855f7;">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;">Integrations</div>
          <div style="font-size:2rem;font-weight:700;color:var(--text-primary);margin:4px 0;">${catNames.length}</div>
          <div style="font-size:.75rem;color:var(--text-muted);">Categories active</div>
        </div>
      </div>

      <!-- Integration Status -->
      <h3 style="color:var(--text-primary);margin-bottom:.75rem;">Integration Status</h3>
      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(250px,1fr));gap:1rem;margin-bottom:1.5rem;">
        ${integrationCards.map(ic => {
          const connected = ic.provider !== 'none';
          return `
            <div class="stat-card" style="display:flex;align-items:center;gap:12px;">
              <div style="font-size:1.8rem;">${ic.icon}</div>
              <div style="flex:1;">
                <div style="font-weight:600;color:var(--text-primary);">${ic.label}</div>
                <div style="font-size:.8rem;color:${connected?'var(--green)':'var(--text-muted)'};">${connected ? ic.provider.charAt(0).toUpperCase()+ic.provider.slice(1)+' connected' : 'Not connected'}</div>
              </div>
              ${connected ?
                `<button class="btn" style="font-size:.75rem;padding:4px 10px;" onclick="showConnectorSwitch('${ic.cat}')">Switch</button>` :
                `<button class="btn-primary" style="font-size:.75rem;padding:5px 12px;" onclick="showConnectorSetup('${ic.cat}','${ic.setup}')">Connect</button>`}
            </div>`;
        }).join('')}
      </div>

      <div style="display:flex;gap:.5rem;flex-wrap:wrap;margin-bottom:1.5rem;">
        <button class="btn-primary" onclick="navigate('contacts')">+ New Contact</button>
        <button class="btn-primary" onclick="navigate('pipelines')">+ New Deal</button>
        <button class="btn-primary" onclick="navigate('agencyConversations')">Open Inbox</button>
        <button class="btn" onclick="navigate('agencyPayments')">View Payments</button>
        <button class="btn" onclick="navigate('agencyPhone')">Phone System</button>
      </div>

      <h3 style="color:var(--text-primary);margin-bottom:.75rem;">Recent Activity</h3>
      <div class="stat-card" style="max-height:300px;overflow-y:auto;">
        ${state.logs.length > 0 ? state.logs.slice(0, 15).map(l => `
          <div style="display:flex;align-items:center;gap:8px;padding:6px 0;border-bottom:1px solid var(--border-light);font-size:.85rem;">
            <span style="color:var(--text-muted);font-family:var(--mono);font-size:.75rem;white-space:nowrap;">${l.time}</span>
            <span style="color:${l.level === 'error' ? 'var(--red)' : l.level === 'warn' ? 'var(--yellow)' : 'var(--text-secondary)'};">${escapeHtml(l.message)}</span>
          </div>
        `).join('') : '<div class="empty-state">No recent activity. Start by adding contacts or connecting integrations.</div>'}
      </div>
    </div>
  `;
}

// ── Agency Conversations Page ──────────────────────────────────────
async function renderAgencyConversationsPage(container) {
  let contacts = [], connectors = {};
  try {
    const [cRes, connRes] = await Promise.all([
      fetch('/v1/contacts').then(r => r.json()).catch(() => ({})),
      fetch('/v1/connectors').then(r => r.json()).catch(() => ({})),
    ]);
    contacts = (cRes.contacts || []).slice(0, 30);
    connectors = connRes.connectors || {};
  } catch (e) { console.error('Conversations fetch error:', e); }

  const hasPhone = !!(connectors.phone && connectors.phone.length > 0);
  const hasCrm = !!(connectors.crm && connectors.crm.length > 0);

  container.innerHTML = `
    <div class="page-section" style="display:grid;grid-template-columns:280px 1fr;gap:0;height:calc(100vh - 120px);overflow:hidden;">
      <div style="border-right:1px solid var(--border);overflow-y:auto;background:var(--bg-card);">
        <div style="padding:12px;border-bottom:1px solid var(--border);">
          <input type="text" placeholder="Search conversations..." style="width:100%;padding:8px 12px;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--bg-input);color:var(--text-primary);font-size:.85rem;" oninput="filterConvList(this.value)">
        </div>
        <div id="conversation-list">
          ${contacts.length === 0 ?
            '<div style="padding:2rem;text-align:center;color:var(--text-muted);font-size:.85rem;">No contacts yet. <a href="#" onclick="navigate(\'contacts\');return false;" style="color:var(--blue);">Add a contact</a> to start.</div>' :
            contacts.map((c, i) => `
              <div class="conv-item" data-idx="${i}" style="display:flex;align-items:center;gap:10px;padding:12px;cursor:pointer;border-bottom:1px solid var(--border-light);transition:background .15s;${i === 0 ? 'background:rgba(59,130,246,.08);' : ''}" onclick="pickConv(${i})" onmouseover="this.style.background='rgba(59,130,246,.05)'" onmouseout="this.style.background='${i===0?'rgba(59,130,246,.08)':'transparent'}'">
                <div style="width:36px;height:36px;border-radius:50%;background:linear-gradient(135deg,var(--blue),var(--accent));display:flex;align-items:center;justify-content:center;color:#fff;font-weight:600;font-size:.85rem;flex-shrink:0;">${(c.name || c.first_name || 'U')[0].toUpperCase()}</div>
                <div style="flex:1;min-width:0;">
                  <div style="font-weight:500;color:var(--text-primary);font-size:.85rem;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">${escapeHtml(c.name || c.first_name || 'Unknown')} ${escapeHtml(c.last_name || '')}</div>
                  <div style="font-size:.75rem;color:var(--text-muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">${escapeHtml(c.email || c.phone || 'No contact info')}</div>
                </div>
              </div>
            `).join('')}
        </div>
      </div>
      <div style="display:flex;flex-direction:column;background:var(--bg-main);">
        <div style="padding:12px 16px;border-bottom:1px solid var(--border);display:flex;align-items:center;gap:10px;">
          <div style="width:32px;height:32px;border-radius:50%;background:linear-gradient(135deg,var(--blue),var(--accent));display:flex;align-items:center;justify-content:center;color:#fff;font-weight:600;font-size:.8rem;" id="conv-avatar">${contacts.length > 0 ? (contacts[0].name || contacts[0].first_name || 'U')[0].toUpperCase() : '?'}</div>
          <div>
            <div style="font-weight:600;color:var(--text-primary);font-size:.9rem;" id="conv-name">${contacts.length > 0 ? escapeHtml(contacts[0].name || contacts[0].first_name || 'Select a contact') : 'No contacts'}</div>
            <div style="font-size:.75rem;color:var(--text-muted);" id="conv-info">${contacts.length > 0 ? escapeHtml(contacts[0].email || contacts[0].phone || '') : ''}</div>
          </div>
          <div style="margin-left:auto;display:flex;gap:6px;">
            <button class="btn" style="font-size:.75rem;padding:4px 10px;" onclick="convSendSms()">SMS</button>
            <button class="btn" style="font-size:.75rem;padding:4px 10px;">Email</button>
          </div>
        </div>
        <div style="flex:1;overflow-y:auto;padding:16px;" id="conv-msgs">
          <div class="empty-state"><div style="font-size:2rem;margin-bottom:8px;">💬</div><div style="color:var(--text-muted);">Select a contact to view or start a conversation</div></div>
        </div>
        <div style="padding:12px;border-top:1px solid var(--border);display:flex;gap:8px;">
          <input type="text" id="conv-input" placeholder="Type a message..." style="flex:1;padding:10px 14px;border:1px solid var(--border);border-radius:var(--radius);background:var(--bg-input);color:var(--text-primary);font-size:.85rem;" onkeydown="if(event.key==='Enter')convSendMsg()">
          <button class="btn-primary" style="padding:10px 20px;" onclick="convSendMsg()">Send</button>
        </div>
      </div>
    </div>
  `;
  // Store contacts for conversation picking
  window._convContacts = contacts;
}
function filterConvList(q) { document.querySelectorAll('.conv-item').forEach(el => { el.style.display = el.textContent.toLowerCase().includes(q.toLowerCase()) ? '' : 'none'; }); }
function pickConv(idx) {
  const c = (window._convContacts || [])[idx];
  if (!c) return;
  document.querySelectorAll('.conv-item').forEach(el => el.style.background = 'transparent');
  const el = document.querySelector(`.conv-item[data-idx="${idx}"]`);
  if (el) el.style.background = 'rgba(59,130,246,.08)';
  const avatar = document.getElementById('conv-avatar');
  const name = document.getElementById('conv-name');
  const info = document.getElementById('conv-info');
  if (avatar) avatar.textContent = (c.name || c.first_name || 'U')[0].toUpperCase();
  if (name) name.textContent = (c.name || c.first_name || 'Unknown') + ' ' + (c.last_name || '');
  if (info) info.textContent = c.email || c.phone || '';
  window._convSelectedContact = c;
  const m = document.getElementById('conv-msgs');
  if (m) m.innerHTML = '<div style="text-align:center;color:var(--text-muted);padding:2rem;font-size:.85rem;">No messages yet. Send a message to start this conversation.</div>';
}
function convSendSms() {
  const c = window._convSelectedContact || (window._convContacts || [])[0];
  if (c && c.phone) showSendSmsModal('');
}
async function convSendMsg() {
  const input = document.getElementById('conv-input');
  const body = input ? input.value.trim() : '';
  if (!body) return;
  const c = window._convSelectedContact || (window._convContacts || [])[0];
  if (!c) return;
  // Try sending via phone connector if they have a phone number
  if (c.phone) {
    try {
      const r = await fetch('/v1/connector/phone/send-sms', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({to:c.phone,body})});
      const d = await r.json();
      if (d.error) { showToast('SMS failed: ' + d.error, 'error'); return; }
      input.value = '';
      const msgs = document.getElementById('conv-msgs');
      if (msgs) {
        if (msgs.querySelector('.empty-state')) msgs.innerHTML = '';
        msgs.innerHTML += `<div style="display:flex;justify-content:flex-end;margin-bottom:8px;"><div style="background:var(--blue);color:#fff;padding:8px 14px;border-radius:16px 16px 4px 16px;max-width:70%;font-size:.85rem;">${escapeHtml(body)}</div></div>`;
        msgs.scrollTop = msgs.scrollHeight;
      }
      showToast('Message sent!', 'success');
    } catch(e) { showToast('Send failed: ' + e.message, 'error'); }
  } else {
    showToast('Contact has no phone number', 'error');
  }
}

// ── Agency Payments Page ────────────────────────────────────────────
async function renderAgencyPaymentsPage(container) {
  let invoices = [], customers = [], balance = null, connName = 'none', connErr = '';
  try {
    const [invRes, custRes, balRes, connRes] = await Promise.all([
      fetch('/v1/connector/payments/list-invoices', {method:'POST',headers:{'Content-Type':'application/json'},body:'{"limit":"20"}'}).then(r=>r.json()).catch(()=>({})),
      fetch('/v1/connector/payments/list-customers', {method:'POST',headers:{'Content-Type':'application/json'},body:'{"limit":"10"}'}).then(r=>r.json()).catch(()=>({})),
      fetch('/v1/connector/payments/get-balance', {method:'POST',headers:{'Content-Type':'application/json'},body:'{}'}).then(r=>r.json()).catch(()=>({})),
      fetch('/v1/connectors').then(r=>r.json()).catch(()=>({})),
    ]);
    invoices = invRes.data?.data || [];
    customers = custRes.data?.data || [];
    balance = balRes.data || null;
    connName = invRes.connector || 'none';
    if (invRes.error) connErr = invRes.error;
  } catch(e) { connErr = e.message; }

  const paid = invoices.filter(i => i.status === 'paid').reduce((s,i) => s + (i.amount_paid||0), 0) / 100;
  const pending = invoices.filter(i => i.status === 'open' || i.status === 'draft').reduce((s,i) => s + (i.amount_due||0), 0) / 100;
  const overdue = invoices.filter(i => i.status === 'uncollectible' || (i.status === 'open' && i.due_date && i.due_date * 1000 < Date.now())).reduce((s,i) => s + (i.amount_due||0), 0) / 100;

  container.innerHTML = `
    <div class="page-section">
      ${connName !== 'none' && !connErr ? `
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:1rem;padding:8px 14px;background:rgba(34,197,94,.08);border:1px solid rgba(34,197,94,.2);border-radius:var(--radius-sm);font-size:.85rem;">
          <span style="color:var(--green);font-weight:600;">Connected:</span> ${escapeHtml(connName)}
          <button class="btn" style="margin-left:auto;font-size:.75rem;padding:3px 10px;" onclick="showConnectorSwitch('payments')">Switch</button>
        </div>` : `
        <div style="display:flex;flex-wrap:wrap;align-items:center;gap:8px;margin-bottom:1rem;padding:10px 14px;background:rgba(239,68,68,.08);border:1px solid rgba(239,68,68,.2);border-radius:var(--radius-sm);font-size:.85rem;">
          <span style="color:var(--red);font-weight:600;">Not Connected</span>
          <span style="color:var(--text-muted);">${connErr ? escapeHtml(connErr) : 'Choose a payment gateway:'}</span>
          <div style="display:flex;gap:4px;margin-left:auto;flex-wrap:wrap;">
            <button class="btn-primary" style="font-size:.7rem;padding:4px 10px;" onclick="showConnectorSetup('payments','stripe')">Stripe</button>
            <button class="btn" style="font-size:.7rem;padding:4px 10px;" onclick="showConnectorSetup('payments','paypal')">PayPal</button>
            <button class="btn" style="font-size:.7rem;padding:4px 10px;" onclick="showConnectorSetup('payments','square')">Square</button>
            <button class="btn" style="font-size:.7rem;padding:4px 10px;" onclick="showConnectorSetup('payments','gocardless')">GoCardless</button>
            <button class="btn" style="font-size:.7rem;padding:4px 10px;" onclick="showConnectorSetup('payments','authorizenet')">Authorize.net</button>
            <button class="btn" style="font-size:.7rem;padding:4px 10px;" onclick="showConnectorSetup('payments','zelle')">Zelle</button>
          </div>
        </div>`}

      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);">Invoices & Payments${helpIcon('Track client payments, create invoices, and manage recurring billing.')}</h3>
        <button class="btn-primary" onclick="showInvModal()">+ New Invoice</button>
      </div>
      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:1rem;margin-bottom:1.5rem;">
        <div class="stat-card" style="border-left:4px solid var(--green);"><div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Paid</div><div style="font-size:1.5rem;font-weight:700;color:var(--green);">$${paid.toFixed(2)}</div></div>
        <div class="stat-card" style="border-left:4px solid var(--yellow);"><div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Pending</div><div style="font-size:1.5rem;font-weight:700;color:var(--yellow);">$${pending.toFixed(2)}</div></div>
        <div class="stat-card" style="border-left:4px solid var(--red);"><div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Overdue</div><div style="font-size:1.5rem;font-weight:700;color:var(--red);">$${overdue.toFixed(2)}</div></div>
        <div class="stat-card" style="border-left:4px solid var(--blue);"><div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Customers</div><div style="font-size:1.5rem;font-weight:700;color:var(--text-primary);">${customers.length}</div></div>
      </div>
      <table class="data-table" style="width:100%;border-collapse:collapse;">
        <thead><tr style="border-bottom:1px solid var(--border);"><th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Invoice #</th><th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Client</th><th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Amount</th><th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Status</th><th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Due Date</th></tr></thead>
        <tbody>
          ${invoices.length === 0 ? '<tr><td colspan="5" class="empty-state" style="padding:2rem;text-align:center;">No invoices yet. Connect Stripe and create your first invoice.</td></tr>' :
            invoices.slice(0,20).map(inv => `
              <tr style="border-bottom:1px solid var(--border-light);">
                <td style="padding:.5rem;font-family:var(--mono);color:var(--text-primary);">${escapeHtml(inv.number || inv.id || '-')}</td>
                <td style="padding:.5rem;color:var(--text-secondary);">${escapeHtml(inv.customer_name || inv.customer_email || inv.customer || '-')}</td>
                <td style="padding:.5rem;color:var(--text-primary);font-weight:500;">$${((inv.amount_due||0)/100).toFixed(2)}</td>
                <td style="padding:.5rem;"><span class="badge" style="background:${inv.status==='paid'?'rgba(34,197,94,.15);color:var(--green)':inv.status==='open'?'rgba(234,179,8,.15);color:var(--yellow)':'rgba(239,68,68,.15);color:var(--red)'};">${inv.status||'draft'}</span></td>
                <td style="padding:.5rem;color:var(--text-muted);font-size:.85rem;">${inv.due_date ? new Date(inv.due_date*1000).toLocaleDateString() : '-'}</td>
              </tr>`).join('')}
        </tbody>
      </table>
    </div>
    <div id="inv-modal" class="modal-overlay" style="display:none;"></div>
  `;
}
function showInvModal() {
  const m = document.getElementById('inv-modal'); if (!m) return; m.style.display = 'flex';
  m.innerHTML = `<div class="modal-card" style="max-width:450px;width:90%;"><h3 style="margin:0 0 1rem;color:var(--text-primary);">Create Invoice</h3>
    <div class="form-group"><label>Customer ID *</label><input class="input" id="inv-customer" placeholder="cus_... (Stripe customer ID)"></div>
    <div class="form-group"><label>Description</label><input class="input" id="inv-desc" placeholder="Invoice details"></div>
    <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
      <button class="btn" onclick="document.getElementById('inv-modal').style.display='none'">Cancel</button>
      <button class="btn-primary" onclick="doCreateInvoice()">Create</button>
    </div></div>`;
}
async function doCreateInvoice() {
  const customer = document.getElementById('inv-customer').value;
  if (!customer) return alert('Customer ID is required');
  try {
    const r = await fetch('/v1/connector/payments/create-invoice', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({customer,description:document.getElementById('inv-desc').value})});
    const d = await r.json();
    if (d.error) { alert('Error: ' + d.error); return; }
    document.getElementById('inv-modal').style.display='none';
    showToast('Invoice created!', 'success');
    renderAgencyPaymentsPage(document.getElementById('main-content') || document.querySelector('.content'));
  } catch(e) { alert('Failed: ' + e.message); }
}

// ── Agency Sites & Funnels Page ────────────────────────────────────
async function renderAgencySitesPage(container) {
  const templates = [
    { icon: '🚀', name: 'Lead Capture Funnel', desc: 'Opt-in page, thank you page, email sequence trigger' },
    { icon: '📋', name: 'Survey Funnel', desc: 'Multi-step survey with conditional branching' },
    { icon: '💰', name: 'Sales Page', desc: 'Product showcase with checkout integration' },
    { icon: '📅', name: 'Booking Funnel', desc: 'Calendar booking with automated reminders' },
  ];
  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);">Sites & Funnels${helpIcon('Build landing pages and sales funnels for client campaigns.')}</h3>
        <div style="display:flex;gap:.5rem;"><button class="btn-primary">+ New Funnel</button><button class="btn">+ New Website</button></div>
      </div>
      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:1rem;">
        ${templates.map(t => `
          <div class="stat-card" style="cursor:pointer;transition:transform .15s,box-shadow .15s;" onmouseover="this.style.transform='translateY(-2px)';this.style.boxShadow='0 8px 24px rgba(0,0,0,.15)'" onmouseout="this.style.transform='';this.style.boxShadow=''">
            <div style="font-size:2rem;margin-bottom:8px;">${t.icon}</div>
            <h4 style="margin:0;color:var(--text-primary);">${t.name}</h4>
            <div style="color:var(--text-muted);font-size:.85rem;margin-top:4px;">${t.desc}</div>
            <button class="btn-primary" style="margin-top:12px;font-size:.8rem;">Use Template</button>
          </div>
        `).join('')}
      </div>
    </div>
  `;
}

// ── Agency Social Planner Page ─────────────────────────────────────
async function renderAgencySocialPage(container) {
  const now = new Date(), y = now.getFullYear(), mo = now.getMonth();
  const months = ['January','February','March','April','May','June','July','August','September','October','November','December'];
  const days = ['Sun','Mon','Tue','Wed','Thu','Fri','Sat'];
  const first = new Date(y, mo, 1).getDay(), dim = new Date(y, mo + 1, 0).getDate();
  let cells = '';
  for (let i = 0; i < first; i++) cells += '<div style="padding:8px;"></div>';
  for (let d = 1; d <= dim; d++) {
    const today = d === now.getDate();
    cells += `<div style="padding:8px;border:1px solid var(--border-light);border-radius:var(--radius-sm);min-height:60px;cursor:pointer;transition:background .15s;${today?'background:rgba(59,130,246,.08);border-color:var(--blue);':''}" onmouseover="this.style.background='rgba(59,130,246,.05)'" onmouseout="this.style.background='${today?'rgba(59,130,246,.08)':'transparent'}'"><div style="font-size:.8rem;font-weight:${today?'700':'400'};color:${today?'var(--blue)':'var(--text-primary)'};">${d}</div></div>`;
  }
  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);">Social Planner${helpIcon('Schedule and manage social media posts across platforms.')}</h3>
        <button class="btn-primary">+ Schedule Post</button>
      </div>
      <div style="display:flex;gap:.5rem;margin-bottom:1rem;flex-wrap:wrap;">
        <span class="badge" style="padding:6px 12px;font-size:.8rem;background:rgba(59,130,246,.15);color:var(--blue);">Facebook</span>
        <span class="badge" style="padding:6px 12px;font-size:.8rem;background:rgba(219,39,119,.15);color:#db2777;">Instagram</span>
        <span class="badge" style="padding:6px 12px;font-size:.8rem;background:rgba(0,172,237,.15);color:#00acee;">Twitter/X</span>
        <span class="badge" style="padding:6px 12px;font-size:.8rem;background:rgba(10,102,194,.15);color:#0a66c2;">LinkedIn</span>
        <button class="btn" style="font-size:.75rem;padding:4px 10px;">+ Connect</button>
      </div>
      <div style="margin-bottom:.5rem;display:flex;align-items:center;justify-content:space-between;">
        <h4 style="margin:0;color:var(--text-primary);">${months[mo]} ${y}</h4>
      </div>
      <div style="display:grid;grid-template-columns:repeat(7,1fr);gap:2px;margin-bottom:4px;">
        ${days.map(d => '<div style="padding:4px 8px;text-align:center;font-size:.75rem;font-weight:600;color:var(--text-muted);text-transform:uppercase;">'+d+'</div>').join('')}
      </div>
      <div style="display:grid;grid-template-columns:repeat(7,1fr);gap:2px;">${cells}</div>
    </div>
  `;
}

// ── Agency Reputation Page ─────────────────────────────────────────
async function renderAgencyReputationPage(container) {
  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);">Reputation Management${helpIcon('Monitor reviews and manage online reputation.')}</h3>
        <button class="btn-primary">+ Request Review</button>
      </div>
      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:1rem;margin-bottom:1.5rem;">
        <div class="stat-card" style="text-align:center;border-left:4px solid var(--green);"><div style="font-size:2.5rem;font-weight:700;color:var(--text-primary);">0.0</div><div style="font-size:.8rem;color:var(--text-muted);">Average Rating</div></div>
        <div class="stat-card" style="text-align:center;border-left:4px solid var(--blue);"><div style="font-size:2.5rem;font-weight:700;color:var(--text-primary);">0</div><div style="font-size:.8rem;color:var(--text-muted);">Total Reviews</div></div>
        <div class="stat-card" style="text-align:center;border-left:4px solid var(--yellow);"><div style="font-size:2.5rem;font-weight:700;color:var(--text-primary);">0</div><div style="font-size:.8rem;color:var(--text-muted);">Pending Requests</div></div>
        <div class="stat-card" style="text-align:center;border-left:4px solid var(--accent);"><div style="font-size:2.5rem;font-weight:700;color:var(--text-primary);">0%</div><div style="font-size:.8rem;color:var(--text-muted);">Response Rate</div></div>
      </div>
      <h4 style="color:var(--text-primary);margin-bottom:.75rem;">Connected Platforms</h4>
      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:1rem;margin-bottom:1.5rem;">
        <div class="stat-card" style="display:flex;align-items:center;gap:12px;cursor:pointer;"><div style="font-size:1.5rem;">🔍</div><div><div style="font-weight:600;color:var(--text-primary);">Google</div><div style="font-size:.8rem;color:var(--text-muted);">Not connected</div></div></div>
        <div class="stat-card" style="display:flex;align-items:center;gap:12px;cursor:pointer;"><div style="font-size:1.5rem;">📘</div><div><div style="font-weight:600;color:var(--text-primary);">Facebook</div><div style="font-size:.8rem;color:var(--text-muted);">Not connected</div></div></div>
        <div class="stat-card" style="display:flex;align-items:center;gap:12px;cursor:pointer;"><div style="font-size:1.5rem;">⭐</div><div><div style="font-weight:600;color:var(--text-primary);">Yelp</div><div style="font-size:.8rem;color:var(--text-muted);">Not connected</div></div></div>
      </div>
      <h4 style="color:var(--text-primary);margin-bottom:.75rem;">Recent Reviews</h4>
      <div class="stat-card"><div class="empty-state" style="padding:2rem;text-align:center;">No reviews yet. Connect a platform or send review requests.</div></div>
    </div>
  `;
}

// ── Agency Memberships Page ────────────────────────────────────────
async function renderAgencyMembershipsPage(container) {
  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);">Memberships & Client Portal${helpIcon('Manage client portals, membership tiers, and course delivery.')}</h3>
        <div style="display:flex;gap:.5rem;"><button class="btn-primary">+ New Membership</button><button class="btn">+ New Course</button></div>
      </div>
      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:1rem;margin-bottom:1.5rem;">
        <div class="stat-card" style="border-left:4px solid var(--blue);"><div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Active Members</div><div style="font-size:1.8rem;font-weight:700;color:var(--text-primary);">0</div></div>
        <div class="stat-card" style="border-left:4px solid var(--green);"><div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Courses</div><div style="font-size:1.8rem;font-weight:700;color:var(--text-primary);">0</div></div>
        <div class="stat-card" style="border-left:4px solid var(--accent);"><div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Revenue</div><div style="font-size:1.8rem;font-weight:700;color:var(--text-primary);">$0</div></div>
      </div>
      <h4 style="color:var(--text-primary);margin-bottom:.75rem;">Membership Tiers</h4>
      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:1rem;">
        <div class="stat-card" style="border-top:4px solid var(--text-muted);"><h4 style="margin:0;color:var(--text-primary);">Free Tier</h4><div style="font-size:.85rem;color:var(--text-muted);margin-top:4px;">Basic access, community forum</div><div style="font-size:1.5rem;font-weight:700;color:var(--text-primary);margin:12px 0;">$0/mo</div><div style="font-size:.8rem;color:var(--text-muted);">0 members</div></div>
        <div class="stat-card" style="border-top:4px solid var(--blue);"><h4 style="margin:0;color:var(--text-primary);">Pro Tier</h4><div style="font-size:.85rem;color:var(--text-muted);margin-top:4px;">Full courses, priority support</div><div style="font-size:1.5rem;font-weight:700;color:var(--text-primary);margin:12px 0;">$49/mo</div><div style="font-size:.8rem;color:var(--text-muted);">0 members</div></div>
        <div class="stat-card" style="border-top:4px solid var(--accent);"><h4 style="margin:0;color:var(--text-primary);">Enterprise</h4><div style="font-size:.85rem;color:var(--text-muted);margin-top:4px;">Custom solutions, white-label</div><div style="font-size:1.5rem;font-weight:700;color:var(--text-primary);margin:12px 0;">Custom</div><div style="font-size:.8rem;color:var(--text-muted);">0 members</div></div>
      </div>
    </div>
  `;
}

// ── Agency Media Library Page ──────────────────────────────────────
async function renderAgencyMediaPage(container) {
  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);">Media Library${helpIcon('Centralized file storage for agency assets.')}</h3>
        <div style="display:flex;gap:.5rem;">
          <button class="btn-primary" onclick="document.getElementById('media-up').click()">+ Upload Files</button>
          <button class="btn">+ New Folder</button>
        </div>
        <input type="file" id="media-up" multiple style="display:none;" onchange="alert(this.files.length + ' file(s) selected. Upload API coming soon.')">
      </div>
      <div style="display:flex;gap:.5rem;margin-bottom:1rem;flex-wrap:wrap;align-items:center;">
        <input type="text" placeholder="Search files..." style="padding:8px 12px;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--bg-input);color:var(--text-primary);font-size:.85rem;min-width:200px;">
        <select style="padding:8px 12px;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--bg-input);color:var(--text-primary);font-size:.85rem;"><option>All Types</option><option>Images</option><option>Documents</option><option>Videos</option></select>
      </div>
      <div style="border:2px dashed var(--border);border-radius:var(--radius);padding:3rem;text-align:center;margin-bottom:1.5rem;cursor:pointer;transition:all .2s;" ondragover="event.preventDefault();this.style.borderColor='var(--blue)';this.style.background='rgba(59,130,246,.05)'" ondragleave="this.style.borderColor='var(--border)';this.style.background='transparent'" onclick="document.getElementById('media-up').click()">
        <div style="font-size:2.5rem;margin-bottom:8px;">📁</div>
        <div style="color:var(--text-primary);font-weight:500;">Drop files here or click to browse</div>
        <div style="color:var(--text-muted);font-size:.8rem;margin-top:4px;">Supports images, documents, videos, and audio</div>
      </div>
      <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(160px,1fr));gap:1rem;">
        <div class="empty-state" style="grid-column:1/-1;padding:2rem;text-align:center;">No files uploaded yet.</div>
      </div>
    </div>
  `;
}

// ── Agency Phone System Page ───────────────────────────────────────
async function renderAgencyPhonePage(container) {
  // Fetch live data from connector
  let numbers = [], calls = [], messages = [], connectorName = 'none';
  let connErr = '';
  try {
    const [numRes, callRes, msgRes, connRes] = await Promise.all([
      fetch('/v1/connector/phone/list-numbers', {method:'POST',headers:{'Content-Type':'application/json'},body:'{}'}).then(r=>r.json()).catch(()=>({})),
      fetch('/v1/connector/phone/call-log', {method:'POST',headers:{'Content-Type':'application/json'},body:'{"limit":"20"}'}).then(r=>r.json()).catch(()=>({})),
      fetch('/v1/connector/phone/list-messages', {method:'POST',headers:{'Content-Type':'application/json'},body:'{"limit":"10"}'}).then(r=>r.json()).catch(()=>({})),
      fetch('/v1/connectors').then(r=>r.json()).catch(()=>({})),
    ]);
    const phoneData = numRes.data || {};
    numbers = phoneData.incoming_phone_numbers || phoneData.phone_numbers || [];
    if (Array.isArray(phoneData)) numbers = phoneData;
    const callData = callRes.data || {};
    calls = callData.calls || [];
    if (Array.isArray(callData)) calls = callData;
    const msgData = msgRes.data || {};
    messages = msgData.messages || [];
    connectorName = numRes.connector || callRes.connector || 'none';
    if (numRes.error) connErr = numRes.error;
  } catch(e) { connErr = e.message; }

  const numCount = Array.isArray(numbers) ? numbers.length : 0;
  const callCount = Array.isArray(calls) ? calls.length : 0;
  const msgCount = Array.isArray(messages) ? messages.length : 0;

  container.innerHTML = `
    <div class="page-section">
      ${connectorName !== 'none' && !connErr ? `
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:1rem;padding:8px 14px;background:rgba(34,197,94,.08);border:1px solid rgba(34,197,94,.2);border-radius:var(--radius-sm);font-size:.85rem;">
          <span style="color:var(--green);font-weight:600;">Connected:</span> ${escapeHtml(connectorName)}
          <button class="btn" style="margin-left:auto;font-size:.75rem;padding:3px 10px;" onclick="showConnectorSwitch('phone')">Switch</button>
        </div>` : `
        <div style="display:flex;align-items:center;gap:8px;margin-bottom:1rem;padding:10px 14px;background:rgba(239,68,68,.08);border:1px solid rgba(239,68,68,.2);border-radius:var(--radius-sm);font-size:.85rem;">
          <span style="color:var(--red);font-weight:600;">Not Connected</span>
          <span style="color:var(--text-muted);">${connErr ? escapeHtml(connErr) : 'Add phone credentials to connect'}</span>
          <button class="btn-primary" style="margin-left:auto;font-size:.75rem;padding:5px 14px;" onclick="showConnectorSetup('phone','twilio')">Connect Twilio</button>
        </div>`}

      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);">Phone System${helpIcon('Manage phone numbers, view call logs, send SMS, and configure IVR routing.')}</h3>
        <div style="display:flex;gap:.5rem;">
          <button class="btn-primary" onclick="showPhoneSearchModal()">+ Buy Number</button>
          <button class="btn" onclick="showSendSmsModal()">+ Send SMS</button>
        </div>
      </div>

      <!-- Stats -->
      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:1rem;margin-bottom:1.5rem;">
        <div class="stat-card" style="border-left:4px solid var(--blue);">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Active Numbers</div>
          <div style="font-size:1.8rem;font-weight:700;color:var(--text-primary);">${numCount}</div>
        </div>
        <div class="stat-card" style="border-left:4px solid var(--green);">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Recent Calls</div>
          <div style="font-size:1.8rem;font-weight:700;color:var(--text-primary);">${callCount}</div>
        </div>
        <div class="stat-card" style="border-left:4px solid var(--accent);">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">SMS Sent</div>
          <div style="font-size:1.8rem;font-weight:700;color:var(--text-primary);">${msgCount}</div>
        </div>
      </div>

      <!-- Phone Numbers -->
      <h4 style="color:var(--text-primary);margin-bottom:.75rem;">Phone Numbers</h4>
      <table class="data-table" style="width:100%;border-collapse:collapse;margin-bottom:1.5rem;">
        <thead>
          <tr style="border-bottom:1px solid var(--border);">
            <th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Number</th>
            <th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Label</th>
            <th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Type</th>
            <th style="text-align:left;padding:.5rem;color:var(--text-secondary);">Status</th>
          </tr>
        </thead>
        <tbody>
          ${numCount === 0 ? '<tr><td colspan="4" class="empty-state" style="padding:2rem;text-align:center;">No phone numbers. Connect a provider and purchase a number to get started.</td></tr>' :
            numbers.map(n => `
              <tr style="border-bottom:1px solid var(--border-light);">
                <td style="padding:.5rem;color:var(--text-primary);font-family:var(--mono);">${escapeHtml(n.phone_number || n.friendly_name || n.number || '')}</td>
                <td style="padding:.5rem;color:var(--text-secondary);">${escapeHtml(n.friendly_name || n.label || '-')}</td>
                <td style="padding:.5rem;"><span class="badge">${n.capabilities ? 'Voice+SMS' : 'Phone'}</span></td>
                <td style="padding:.5rem;"><span style="color:var(--green);font-weight:500;">Active</span></td>
              </tr>
            `).join('')}
        </tbody>
      </table>

      <!-- Call Log -->
      <h4 style="color:var(--text-primary);margin-bottom:.75rem;">Recent Call Log</h4>
      <div class="stat-card" style="max-height:280px;overflow-y:auto;">
        ${callCount === 0 ? '<div class="empty-state" style="padding:2rem;text-align:center;">No call history yet.</div>' :
          calls.slice(0,15).map(c => `
            <div style="display:flex;align-items:center;gap:10px;padding:8px 0;border-bottom:1px solid var(--border-light);font-size:.85rem;">
              <span style="font-size:1rem;">${c.direction === 'inbound' ? '📥' : '📤'}</span>
              <div style="flex:1;">
                <span style="color:var(--text-primary);font-family:var(--mono);">${escapeHtml(c.from || '')} → ${escapeHtml(c.to || '')}</span>
              </div>
              <span style="color:var(--text-muted);font-size:.75rem;">${c.duration || '0'}s</span>
              <span class="badge" style="font-size:.7rem;">${escapeHtml(c.status || 'unknown')}</span>
            </div>
          `).join('')}
      </div>

      <!-- SMS Templates -->
      <h4 style="color:var(--text-primary);margin:1.5rem 0 .75rem;">SMS Templates</h4>
      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(250px,1fr));gap:1rem;">
        <div class="stat-card" style="cursor:pointer;" onclick="showSendSmsModal('Hi {{name}}, this is a reminder about your appointment on {{date}} at {{time}}. Reply YES to confirm.')">
          <div style="font-weight:600;color:var(--text-primary);margin-bottom:4px;">Appointment Reminder</div>
          <div style="font-size:.85rem;color:var(--text-muted);">Hi {{name}}, this is a reminder about your appointment on {{date}} at {{time}}. Reply YES to confirm.</div>
        </div>
        <div class="stat-card" style="cursor:pointer;" onclick="showSendSmsModal('Hi {{name}}, thanks for your interest! When would be a good time to connect?')">
          <div style="font-weight:600;color:var(--text-primary);margin-bottom:4px;">Follow-Up</div>
          <div style="font-size:.85rem;color:var(--text-muted);">Hi {{name}}, thanks for your interest! When would be a good time to connect?</div>
        </div>
        <div class="stat-card" style="cursor:pointer;" onclick="showSendSmsModal('Hi {{name}}, we\\'d love your feedback! Please leave us a review: {{link}}')">
          <div style="font-weight:600;color:var(--text-primary);margin-bottom:4px;">Review Request</div>
          <div style="font-size:.85rem;color:var(--text-muted);">Hi {{name}}, we'd love your feedback! Please leave us a review.</div>
        </div>
      </div>
    </div>
    <div id="phone-modal" class="modal-overlay" style="display:none;"></div>
  `;
}

// Send SMS modal
function showSendSmsModal(prefill) {
  let m = document.getElementById('phone-modal');
  if (!m) { const d = document.createElement('div'); d.id='phone-modal'; d.className='modal-overlay'; document.body.appendChild(d); m = d; }
  m.style.display = 'flex';
  m.innerHTML = `<div class="modal-card" style="max-width:450px;width:90%;"><h3 style="margin:0 0 1rem;color:var(--text-primary);">Send SMS</h3>
    <div class="form-group"><label>To *</label><input class="input" id="sms-to" placeholder="+1234567890"></div>
    <div class="form-group"><label>Message *</label><textarea class="input" id="sms-body" rows="4" placeholder="Type your message...">${prefill||''}</textarea></div>
    <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
      <button class="btn" onclick="document.getElementById('phone-modal').style.display='none'">Cancel</button>
      <button class="btn-primary" onclick="doSendSms()">Send</button>
    </div></div>`;
}
async function doSendSms() {
  const to = document.getElementById('sms-to').value;
  const body = document.getElementById('sms-body').value;
  if (!to || !body) return alert('To and Message are required');
  try {
    const r = await fetch('/v1/connector/phone/send-sms', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({to,body})});
    const d = await r.json();
    if (d.error) { alert('Error: ' + d.error); return; }
    document.getElementById('phone-modal').style.display='none';
    showToast('SMS sent successfully!', 'success');
    renderAgencyPhonePage(document.getElementById('main-content') || document.querySelector('.content'));
  } catch(e) { alert('Send failed: ' + e.message); }
}

// Phone number search modal
function showPhoneSearchModal() {
  let m = document.getElementById('phone-modal');
  if (!m) { const d = document.createElement('div'); d.id='phone-modal'; d.className='modal-overlay'; document.body.appendChild(d); m = d; }
  m.style.display = 'flex';
  m.innerHTML = `<div class="modal-card" style="max-width:500px;width:90%;"><h3 style="margin:0 0 1rem;color:var(--text-primary);">Search Available Numbers</h3>
    <div class="form-group"><label>Country</label><select class="input" id="num-country"><option value="US">United States</option><option value="CA">Canada</option><option value="GB">United Kingdom</option><option value="AU">Australia</option></select></div>
    <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
      <button class="btn" onclick="document.getElementById('phone-modal').style.display='none'">Cancel</button>
      <button class="btn-primary" onclick="doSearchNumbers()">Search</button>
    </div>
    <div id="num-results" style="margin-top:1rem;max-height:300px;overflow-y:auto;"></div></div>`;
}
async function doSearchNumbers() {
  const country = document.getElementById('num-country').value;
  const results = document.getElementById('num-results');
  results.innerHTML = '<div style="text-align:center;color:var(--text-muted);padding:1rem;">Searching...</div>';
  try {
    const r = await fetch('/v1/connector/phone/search-numbers', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({country})});
    const d = await r.json();
    if (d.error) { results.innerHTML = '<div style="color:var(--red);padding:1rem;">' + escapeHtml(d.error) + '</div>'; return; }
    const nums = d.data?.available_phone_numbers || d.data?.items || [];
    if (nums.length === 0) { results.innerHTML = '<div style="padding:1rem;color:var(--text-muted);">No numbers found for this region.</div>'; return; }
    results.innerHTML = nums.slice(0,10).map(n => `
      <div style="display:flex;align-items:center;justify-content:space-between;padding:8px 0;border-bottom:1px solid var(--border-light);">
        <span style="font-family:var(--mono);color:var(--text-primary);">${escapeHtml(n.phone_number || n.friendly_name || '')}</span>
        <button class="btn-primary" style="font-size:.75rem;padding:3px 10px;">Buy</button>
      </div>`).join('');
  } catch(e) { results.innerHTML = '<div style="color:var(--red);padding:1rem;">' + e.message + '</div>'; }
}

// Connector setup modal
function showConnectorSetup(category, provider) {
  let m = document.getElementById('phone-modal') || document.getElementById('inv-modal');
  if (!m) { const d = document.createElement('div'); d.id='phone-modal'; d.className='modal-overlay'; document.body.appendChild(d); m = d; }
  m.style.display = 'flex';
  const fields = {
    twilio: [{id:'account_sid',label:'Account SID',ph:'AC...'},{id:'auth_token',label:'Auth Token',ph:'Your auth token'},{id:'from_number',label:'From Number',ph:'+1234567890'}],
    stripe: [{id:'secret_key',label:'Secret Key',ph:'sk_test_...'}],
    ghl: [{id:'api_key',label:'API Key',ph:'Your GHL API key'},{id:'location_id',label:'Location ID',ph:'Location ID'}],
    vonage: [{id:'api_key',label:'API Key',ph:'Your Vonage key'},{id:'api_secret',label:'API Secret',ph:'Secret'}],
    plivo: [{id:'auth_id',label:'Auth ID',ph:'Your Plivo ID'},{id:'auth_token',label:'Auth Token',ph:'Token'}],
    paypal: [{id:'access_token',label:'Access Token',ph:'Your PayPal access token'},{id:'sandbox',label:'Sandbox Mode',ph:'true or false'}],
    square: [{id:'access_token',label:'Access Token',ph:'Your Square access token'},{id:'location_id',label:'Location ID',ph:'Location ID'},{id:'sandbox',label:'Sandbox Mode',ph:'true or false'}],
    authorizenet: [{id:'api_login_id',label:'API Login ID',ph:'Your login ID'},{id:'transaction_key',label:'Transaction Key',ph:'Your transaction key'},{id:'sandbox',label:'Sandbox Mode',ph:'true or false'}],
    gocardless: [{id:'access_token',label:'Access Token',ph:'Your GoCardless token'},{id:'sandbox',label:'Sandbox Mode',ph:'true or false'}],
    zelle: [{id:'access_token',label:'JPMC Access Token',ph:'Your J.P. Morgan API token'},{id:'debtor_account',label:'Debtor Account ID',ph:'Your bank account ID'},{id:'sandbox',label:'Sandbox Mode',ph:'true or false'}],
  };
  const f = fields[provider] || [{id:'api_key',label:'API Key',ph:'Enter key'}];
  m.innerHTML = `<div class="modal-card" style="max-width:450px;width:90%;"><h3 style="margin:0 0 1rem;color:var(--text-primary);">Connect ${escapeHtml(provider.charAt(0).toUpperCase()+provider.slice(1))}</h3>
    ${f.map(fi => `<div class="form-group"><label>${fi.label}</label><input class="input" id="setup-${fi.id}" placeholder="${fi.ph}"></div>`).join('')}
    <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
      <button class="btn" onclick="document.getElementById('phone-modal').style.display='none'">Cancel</button>
      <button class="btn-primary" onclick="doSaveConnector('${provider}', [${f.map(fi=>"'"+fi.id+"'").join(',')}])">Save & Connect</button>
    </div></div>`;
}
async function doSaveConnector(provider, fieldIds) {
  const fields = {};
  fieldIds.forEach(id => { fields[id] = document.getElementById('setup-' + id).value; });
  try {
    await fetch('/v1/credentials', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:provider,provider:provider,type:'api_key',value:JSON.stringify(fields),scope:'global'})});
    const m = document.getElementById('phone-modal');
    if (m) m.style.display = 'none';
    showToast(provider + ' credentials saved!', 'success');
    // Reload the current page to pick up the new connector
    const mc = document.getElementById('main-content') || document.querySelector('.content');
    if (mc) { const h = window.location.hash.replace('#',''); if (typeof window.renderPage === 'function') window.renderPage(h); }
  } catch(e) { alert('Save failed: ' + e.message); }
}

// Connector switch modal
function showConnectorSwitch(category) {
  fetch('/v1/connectors').then(r=>r.json()).then(d => {
    const conns = (d.connectors || {})[category] || [];
    let m = document.getElementById('phone-modal');
    if (!m) { const el = document.createElement('div'); el.id='phone-modal'; el.className='modal-overlay'; document.body.appendChild(el); m = el; }
    m.style.display = 'flex';
    m.innerHTML = `<div class="modal-card" style="max-width:400px;width:90%;"><h3 style="margin:0 0 1rem;color:var(--text-primary);">Switch ${category} Provider</h3>
      ${conns.map(c => `
        <div style="display:flex;align-items:center;gap:10px;padding:10px;border:1px solid ${c.active?'var(--blue)':'var(--border)'};border-radius:var(--radius-sm);margin-bottom:8px;cursor:pointer;background:${c.active?'rgba(59,130,246,.05)':'transparent'};" onclick="doSwitchConnector('${category}','${c.name}')">
          <div style="width:10px;height:10px;border-radius:50%;background:${c.active?'var(--blue)':'var(--border)'}"></div>
          <div><div style="font-weight:600;color:var(--text-primary);">${c.name.charAt(0).toUpperCase()+c.name.slice(1)}</div>
          <div style="font-size:.75rem;color:var(--text-muted);">${c.actions.length} actions${c.active?' (active)':''}</div></div>
        </div>`).join('')}
      <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
        <button class="btn" onclick="document.getElementById('phone-modal').style.display='none'">Close</button>
      </div></div>`;
  });
}
async function doSwitchConnector(category, name) {
  await fetch('/v1/connectors/activate', {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({category,name})});
  document.getElementById('phone-modal').style.display='none';
  showToast(name + ' activated for ' + category, 'success');
  const mc = document.getElementById('main-content') || document.querySelector('.content');
  if (mc) { const h = window.location.hash.replace('#',''); if (typeof window.renderPage === 'function') window.renderPage(h); }
}

// ── Agency Brand Boards Page ───────────────────────────────────────
async function renderAgencyBrandsPage(container) {
  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);">Brand Boards${helpIcon('Create and manage brand identity boards with colors, logos, fonts, and style guidelines.')}</h3>
        <button class="btn-primary">+ New Brand Board</button>
      </div>

      <!-- Default Brand Board -->
      <div class="stat-card" style="margin-bottom:1.5rem;">
        <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
          <h4 style="margin:0;color:var(--text-primary);">Default Brand</h4>
          <button class="btn" style="font-size:.75rem;padding:4px 10px;">Edit</button>
        </div>

        <!-- Color Palette -->
        <div style="margin-bottom:1rem;">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;margin-bottom:8px;">Color Palette</div>
          <div style="display:flex;gap:8px;flex-wrap:wrap;">
            <div style="text-align:center;">
              <div style="width:60px;height:60px;border-radius:var(--radius-sm);background:#e85d04;border:2px solid var(--border);cursor:pointer;" title="#e85d04"></div>
              <div style="font-size:.7rem;color:var(--text-muted);margin-top:4px;font-family:var(--mono);">#e85d04</div>
            </div>
            <div style="text-align:center;">
              <div style="width:60px;height:60px;border-radius:var(--radius-sm);background:#f48c06;border:2px solid var(--border);cursor:pointer;" title="#f48c06"></div>
              <div style="font-size:.7rem;color:var(--text-muted);margin-top:4px;font-family:var(--mono);">#f48c06</div>
            </div>
            <div style="text-align:center;">
              <div style="width:60px;height:60px;border-radius:var(--radius-sm);background:#1a1a2e;border:2px solid var(--border);cursor:pointer;" title="#1a1a2e"></div>
              <div style="font-size:.7rem;color:var(--text-muted);margin-top:4px;font-family:var(--mono);">#1a1a2e</div>
            </div>
            <div style="text-align:center;">
              <div style="width:60px;height:60px;border-radius:var(--radius-sm);background:#16213e;border:2px solid var(--border);cursor:pointer;" title="#16213e"></div>
              <div style="font-size:.7rem;color:var(--text-muted);margin-top:4px;font-family:var(--mono);">#16213e</div>
            </div>
            <div style="text-align:center;">
              <div style="width:60px;height:60px;border-radius:var(--radius-sm);background:#e2e8f0;border:2px solid var(--border);cursor:pointer;" title="#e2e8f0"></div>
              <div style="font-size:.7rem;color:var(--text-muted);margin-top:4px;font-family:var(--mono);">#e2e8f0</div>
            </div>
            <div style="text-align:center;cursor:pointer;" onclick="alert('Color picker coming soon')">
              <div style="width:60px;height:60px;border-radius:var(--radius-sm);border:2px dashed var(--border);display:flex;align-items:center;justify-content:center;font-size:1.5rem;color:var(--text-muted);">+</div>
              <div style="font-size:.7rem;color:var(--text-muted);margin-top:4px;">Add</div>
            </div>
          </div>
        </div>

        <!-- Typography -->
        <div style="margin-bottom:1rem;">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;margin-bottom:8px;">Typography</div>
          <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;">
            <div style="padding:16px;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--bg-main);">
              <div style="font-size:.75rem;color:var(--text-muted);text-transform:uppercase;margin-bottom:4px;">Heading Font</div>
              <div style="font-size:1.2rem;font-weight:700;color:var(--text-primary);">Inter</div>
              <div style="font-size:.85rem;color:var(--text-secondary);margin-top:4px;">The quick brown fox jumps over the lazy dog</div>
            </div>
            <div style="padding:16px;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--bg-main);">
              <div style="font-size:.75rem;color:var(--text-muted);text-transform:uppercase;margin-bottom:4px;">Body Font</div>
              <div style="font-size:1.2rem;font-weight:400;color:var(--text-primary);">Inter</div>
              <div style="font-size:.85rem;color:var(--text-secondary);margin-top:4px;">The quick brown fox jumps over the lazy dog</div>
            </div>
          </div>
        </div>

        <!-- Logos -->
        <div style="margin-bottom:1rem;">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;margin-bottom:8px;">Logos</div>
          <div style="display:flex;gap:1rem;flex-wrap:wrap;">
            <div style="width:120px;height:80px;border:2px dashed var(--border);border-radius:var(--radius-sm);display:flex;flex-direction:column;align-items:center;justify-content:center;cursor:pointer;transition:border-color .15s;" onmouseover="this.style.borderColor='var(--blue)'" onmouseout="this.style.borderColor='var(--border)'" onclick="alert('Logo upload coming soon')">
              <div style="font-size:1.2rem;color:var(--text-muted);">📷</div>
              <div style="font-size:.7rem;color:var(--text-muted);margin-top:2px;">Primary Logo</div>
            </div>
            <div style="width:120px;height:80px;border:2px dashed var(--border);border-radius:var(--radius-sm);display:flex;flex-direction:column;align-items:center;justify-content:center;cursor:pointer;transition:border-color .15s;" onmouseover="this.style.borderColor='var(--blue)'" onmouseout="this.style.borderColor='var(--border)'" onclick="alert('Logo upload coming soon')">
              <div style="font-size:1.2rem;color:var(--text-muted);">📷</div>
              <div style="font-size:.7rem;color:var(--text-muted);margin-top:2px;">Dark Logo</div>
            </div>
            <div style="width:120px;height:80px;border:2px dashed var(--border);border-radius:var(--radius-sm);display:flex;flex-direction:column;align-items:center;justify-content:center;cursor:pointer;transition:border-color .15s;" onmouseover="this.style.borderColor='var(--blue)'" onmouseout="this.style.borderColor='var(--border)'" onclick="alert('Logo upload coming soon')">
              <div style="font-size:1.2rem;color:var(--text-muted);">📷</div>
              <div style="font-size:.7rem;color:var(--text-muted);margin-top:2px;">Favicon</div>
            </div>
          </div>
        </div>

        <!-- Brand Voice -->
        <div>
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;margin-bottom:8px;">Brand Voice</div>
          <div style="display:flex;gap:.5rem;flex-wrap:wrap;">
            <span class="badge" style="padding:6px 12px;background:rgba(232,93,4,.15);color:var(--accent);">Professional</span>
            <span class="badge" style="padding:6px 12px;background:rgba(59,130,246,.15);color:var(--blue);">Innovative</span>
            <span class="badge" style="padding:6px 12px;background:rgba(16,185,129,.15);color:var(--green);">Trustworthy</span>
            <button class="btn" style="font-size:.7rem;padding:3px 8px;">+ Add Trait</button>
          </div>
        </div>
      </div>
    </div>
  `;
}

// ── Voice AI Page ──────────────────────────────────────────────────
async function renderVoiceAIPage(container) {
  let agents = [];
  try { const r = await fetch('/v1/agents'); const d = await r.json(); agents = d.agents || []; } catch(e) {}

  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);">Voice AI${helpIcon('Configure voice agents for inbound/outbound calls with AI-powered conversation handling.')}</h3>
        <button class="btn-primary">+ New Voice Agent</button>
      </div>

      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:1rem;margin-bottom:1.5rem;">
        <div class="stat-card" style="border-left:4px solid var(--blue);">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Voice Agents</div>
          <div style="font-size:1.8rem;font-weight:700;color:var(--text-primary);">0</div>
        </div>
        <div class="stat-card" style="border-left:4px solid var(--green);">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Calls Handled</div>
          <div style="font-size:1.8rem;font-weight:700;color:var(--text-primary);">0</div>
        </div>
        <div class="stat-card" style="border-left:4px solid var(--accent);">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Avg Duration</div>
          <div style="font-size:1.8rem;font-weight:700;color:var(--text-primary);">0s</div>
        </div>
        <div class="stat-card" style="border-left:4px solid #a855f7;">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Success Rate</div>
          <div style="font-size:1.8rem;font-weight:700;color:var(--text-primary);">-%</div>
        </div>
      </div>

      <!-- Voice Agent Config Card -->
      <div class="stat-card" style="margin-bottom:1.5rem;">
        <h4 style="margin:0 0 1rem;color:var(--text-primary);">Create Voice Agent</h4>
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;">
          <div class="form-group">
            <label style="font-size:.8rem;color:var(--text-secondary);">Agent Name</label>
            <input class="input" placeholder="e.g., Sales Assistant">
          </div>
          <div class="form-group">
            <label style="font-size:.8rem;color:var(--text-secondary);">Base Agent</label>
            <select class="input">
              <option>Select an agent...</option>
              ${agents.map(a => '<option value="'+escapeHtml(a.name || a.id)+'">'+escapeHtml(a.name || a.id)+'</option>').join('')}
            </select>
          </div>
          <div class="form-group">
            <label style="font-size:.8rem;color:var(--text-secondary);">Voice Model</label>
            <select class="input">
              <option>Default (System TTS)</option>
              <option>ElevenLabs</option>
              <option>OpenAI TTS</option>
              <option>Qwen TTS</option>
            </select>
          </div>
          <div class="form-group">
            <label style="font-size:.8rem;color:var(--text-secondary);">Voice Style</label>
            <select class="input">
              <option>Professional</option>
              <option>Friendly</option>
              <option>Calm</option>
              <option>Energetic</option>
            </select>
          </div>
        </div>
        <div class="form-group" style="margin-top:.5rem;">
          <label style="font-size:.8rem;color:var(--text-secondary);">Greeting Message</label>
          <textarea class="input" rows="2" placeholder="Hi, thanks for calling! How can I help you today?"></textarea>
        </div>
        <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
          <button class="btn">Test Voice</button>
          <button class="btn-primary">Save Agent</button>
        </div>
      </div>

      <!-- Active Voice Agents -->
      <h4 style="color:var(--text-primary);margin-bottom:.75rem;">Active Voice Agents</h4>
      <div class="stat-card">
        <div class="empty-state" style="padding:2rem;text-align:center;">No voice agents configured. Create one above to get started.</div>
      </div>
    </div>
  `;
}

// ── Conversation AI Page ───────────────────────────────────────────
async function renderConversationAIPage(container) {
  let agents = [];
  try { const r = await fetch('/v1/agents'); const d = await r.json(); agents = d.agents || []; } catch(e) {}

  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);">Conversation AI${helpIcon('Set up auto-reply chatbots and AI conversation flows across channels.')}</h3>
        <button class="btn-primary">+ New Chatbot</button>
      </div>

      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:1rem;margin-bottom:1.5rem;">
        <div class="stat-card" style="border-left:4px solid var(--blue);">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Active Bots</div>
          <div style="font-size:1.8rem;font-weight:700;color:var(--text-primary);">0</div>
        </div>
        <div class="stat-card" style="border-left:4px solid var(--green);">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Messages Today</div>
          <div style="font-size:1.8rem;font-weight:700;color:var(--text-primary);">0</div>
        </div>
        <div class="stat-card" style="border-left:4px solid var(--accent);">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;">Resolution Rate</div>
          <div style="font-size:1.8rem;font-weight:700;color:var(--text-primary);">-%</div>
        </div>
      </div>

      <!-- Chatbot Config -->
      <div class="stat-card" style="margin-bottom:1.5rem;">
        <h4 style="margin:0 0 1rem;color:var(--text-primary);">Chatbot Configuration</h4>
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:1rem;">
          <div class="form-group">
            <label style="font-size:.8rem;color:var(--text-secondary);">Bot Name</label>
            <input class="input" placeholder="e.g., Support Bot">
          </div>
          <div class="form-group">
            <label style="font-size:.8rem;color:var(--text-secondary);">AI Agent</label>
            <select class="input">
              <option>Select an agent...</option>
              ${agents.map(a => '<option value="'+escapeHtml(a.name || a.id)+'">'+escapeHtml(a.name || a.id)+'</option>').join('')}
            </select>
          </div>
        </div>
        <div class="form-group" style="margin-top:.5rem;">
          <label style="font-size:.8rem;color:var(--text-secondary);">Channels</label>
          <div style="display:flex;gap:.5rem;flex-wrap:wrap;">
            <label style="display:flex;align-items:center;gap:4px;padding:6px 12px;border:1px solid var(--border);border-radius:var(--radius-sm);cursor:pointer;font-size:.85rem;color:var(--text-primary);"><input type="checkbox" checked> Website Chat</label>
            <label style="display:flex;align-items:center;gap:4px;padding:6px 12px;border:1px solid var(--border);border-radius:var(--radius-sm);cursor:pointer;font-size:.85rem;color:var(--text-primary);"><input type="checkbox"> SMS</label>
            <label style="display:flex;align-items:center;gap:4px;padding:6px 12px;border:1px solid var(--border);border-radius:var(--radius-sm);cursor:pointer;font-size:.85rem;color:var(--text-primary);"><input type="checkbox"> Email</label>
            <label style="display:flex;align-items:center;gap:4px;padding:6px 12px;border:1px solid var(--border);border-radius:var(--radius-sm);cursor:pointer;font-size:.85rem;color:var(--text-primary);"><input type="checkbox"> Facebook Messenger</label>
            <label style="display:flex;align-items:center;gap:4px;padding:6px 12px;border:1px solid var(--border);border-radius:var(--radius-sm);cursor:pointer;font-size:.85rem;color:var(--text-primary);"><input type="checkbox"> Instagram DM</label>
          </div>
        </div>
        <div class="form-group" style="margin-top:.5rem;">
          <label style="font-size:.8rem;color:var(--text-secondary);">Response Mode</label>
          <div style="display:flex;gap:1rem;">
            <label style="display:flex;align-items:center;gap:4px;font-size:.85rem;color:var(--text-primary);"><input type="radio" name="ai-mode" checked> Full Auto</label>
            <label style="display:flex;align-items:center;gap:4px;font-size:.85rem;color:var(--text-primary);"><input type="radio" name="ai-mode"> Suggest Only</label>
            <label style="display:flex;align-items:center;gap:4px;font-size:.85rem;color:var(--text-primary);"><input type="radio" name="ai-mode"> After-Hours Only</label>
          </div>
        </div>
        <div class="form-group" style="margin-top:.5rem;">
          <label style="font-size:.8rem;color:var(--text-secondary);">System Prompt</label>
          <textarea class="input" rows="3" placeholder="You are a helpful customer support assistant. Be concise and professional..."></textarea>
        </div>
        <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
          <button class="btn-primary">Save Bot</button>
        </div>
      </div>
    </div>
  `;
}

// ── Content AI Page ────────────────────────────────────────────────
async function renderContentAIPage(container) {
  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);">Content AI${helpIcon('Generate blog posts, emails, social content, and ad copy using AI models.')}</h3>
      </div>

      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:1rem;margin-bottom:1.5rem;">
        <div class="stat-card" style="cursor:pointer;transition:transform .15s;" onmouseover="this.style.transform='translateY(-2px)'" onmouseout="this.style.transform=''">
          <div style="font-size:1.5rem;margin-bottom:4px;">📝</div>
          <div style="font-weight:600;color:var(--text-primary);">Blog Post</div>
          <div style="font-size:.8rem;color:var(--text-muted);margin-top:2px;">SEO-optimized articles</div>
        </div>
        <div class="stat-card" style="cursor:pointer;transition:transform .15s;" onmouseover="this.style.transform='translateY(-2px)'" onmouseout="this.style.transform=''">
          <div style="font-size:1.5rem;margin-bottom:4px;">📧</div>
          <div style="font-weight:600;color:var(--text-primary);">Email</div>
          <div style="font-size:.8rem;color:var(--text-muted);margin-top:2px;">Newsletters, sequences</div>
        </div>
        <div class="stat-card" style="cursor:pointer;transition:transform .15s;" onmouseover="this.style.transform='translateY(-2px)'" onmouseout="this.style.transform=''">
          <div style="font-size:1.5rem;margin-bottom:4px;">📱</div>
          <div style="font-weight:600;color:var(--text-primary);">Social Post</div>
          <div style="font-size:.8rem;color:var(--text-muted);margin-top:2px;">Platform-specific content</div>
        </div>
        <div class="stat-card" style="cursor:pointer;transition:transform .15s;" onmouseover="this.style.transform='translateY(-2px)'" onmouseout="this.style.transform=''">
          <div style="font-size:1.5rem;margin-bottom:4px;">📢</div>
          <div style="font-weight:600;color:var(--text-primary);">Ad Copy</div>
          <div style="font-size:.8rem;color:var(--text-muted);margin-top:2px;">Google, Meta, LinkedIn</div>
        </div>
      </div>

      <!-- Content Generator -->
      <div class="stat-card">
        <h4 style="margin:0 0 1rem;color:var(--text-primary);">AI Content Generator</h4>
        <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:1rem;margin-bottom:1rem;">
          <div class="form-group">
            <label style="font-size:.8rem;color:var(--text-secondary);">Content Type</label>
            <select class="input" id="content-type">
              <option>Blog Post</option>
              <option>Email</option>
              <option>Social Post</option>
              <option>Ad Copy</option>
              <option>Product Description</option>
              <option>Landing Page</option>
            </select>
          </div>
          <div class="form-group">
            <label style="font-size:.8rem;color:var(--text-secondary);">Tone</label>
            <select class="input">
              <option>Professional</option>
              <option>Casual</option>
              <option>Persuasive</option>
              <option>Informative</option>
              <option>Humorous</option>
            </select>
          </div>
          <div class="form-group">
            <label style="font-size:.8rem;color:var(--text-secondary);">Model</label>
            <select class="input">
              <option>Default Agent</option>
              <option>GPT-4o</option>
              <option>Claude Sonnet</option>
              <option>Gemini Pro</option>
              <option>Qwen 3.5</option>
            </select>
          </div>
        </div>
        <div class="form-group">
          <label style="font-size:.8rem;color:var(--text-secondary);">Topic / Brief</label>
          <textarea class="input" rows="3" id="content-brief" placeholder="Describe what you want to write about..."></textarea>
        </div>
        <div class="form-group">
          <label style="font-size:.8rem;color:var(--text-secondary);">Keywords (comma separated)</label>
          <input class="input" placeholder="e.g., AI automation, business growth, ROI">
        </div>
        <div style="display:flex;gap:.5rem;justify-content:flex-end;margin-top:1rem;">
          <button class="btn">Save Draft</button>
          <button class="btn-primary" onclick="alert('Content generation will use the Chat API. Coming soon!')">Generate Content</button>
        </div>
        <div id="content-output" style="margin-top:1rem;display:none;">
          <div style="font-size:.8rem;color:var(--text-muted);text-transform:uppercase;margin-bottom:8px;">Generated Content</div>
          <div style="padding:16px;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--bg-main);white-space:pre-wrap;font-size:.9rem;color:var(--text-primary);max-height:400px;overflow-y:auto;"></div>
        </div>
      </div>
    </div>
  `;
}

// ── Agent Templates Page ───────────────────────────────────────────
async function renderAgentTemplatesPage(container) {
  const templates = [
    { icon: '🤝', name: 'Sales Qualifier', category: 'Sales', desc: 'Qualifies inbound leads, books appointments, and logs deal stage.' },
    { icon: '🎧', name: 'Customer Support', category: 'Support', desc: 'Handles FAQs, routes complex issues, and tracks resolution time.' },
    { icon: '📋', name: 'Onboarding Agent', category: 'Operations', desc: 'Guides new clients through setup, collects info, and sends welcome materials.' },
    { icon: '📊', name: 'Reporting Agent', category: 'Analytics', desc: 'Generates daily/weekly reports from connected data sources.' },
    { icon: '📱', name: 'Social Media Manager', category: 'Marketing', desc: 'Drafts posts, schedules content, and responds to comments.' },
    { icon: '📧', name: 'Email Nurture', category: 'Marketing', desc: 'Sends automated email sequences based on lead behavior and triggers.' },
    { icon: '🔍', name: 'Research Assistant', category: 'Research', desc: 'Searches web, summarizes findings, and compiles research reports.' },
    { icon: '📝', name: 'Content Writer', category: 'Content', desc: 'Creates blog posts, ad copy, and product descriptions from briefs.' },
    { icon: '💼', name: 'HR Assistant', category: 'Operations', desc: 'Screens resumes, schedules interviews, and answers policy questions.' },
    { icon: '🏗️', name: 'Project Manager', category: 'Operations', desc: 'Creates tasks, tracks deadlines, sends status updates, and flags risks.' },
    { icon: '🧮', name: 'Invoice Agent', category: 'Finance', desc: 'Generates invoices, sends reminders, and reconciles payments.' },
    { icon: '🔗', name: 'Integration Bot', category: 'Development', desc: 'Connects APIs, syncs data between platforms, and monitors webhooks.' },
  ];
  const categories = [...new Set(templates.map(t => t.category))];

  container.innerHTML = `
    <div class="page-section">
      <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1rem;">
        <h3 style="margin:0;color:var(--text-primary);">Agent Templates${helpIcon('Browse and deploy pre-built agent templates for common business workflows.')}</h3>
        <button class="btn-primary">+ Create Custom</button>
      </div>

      <div style="display:flex;gap:.5rem;margin-bottom:1rem;flex-wrap:wrap;align-items:center;">
        <button class="btn-primary" style="font-size:.8rem;" onclick="filterTemplates('all')">All</button>
        ${categories.map(c => '<button class="btn" style="font-size:.8rem;" onclick="filterTemplates(\''+c+'\')">'+c+'</button>').join('')}
        <input type="text" placeholder="Search templates..." style="margin-left:auto;padding:8px 12px;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--bg-input);color:var(--text-primary);font-size:.85rem;min-width:200px;" oninput="searchTemplates(this.value)">
      </div>

      <div id="templates-grid" style="display:grid;grid-template-columns:repeat(auto-fill,minmax(280px,1fr));gap:1rem;">
        ${templates.map(t => `
          <div class="stat-card template-card" data-category="${t.category}" style="cursor:pointer;transition:transform .15s,box-shadow .15s;" onmouseover="this.style.transform='translateY(-2px)';this.style.boxShadow='0 8px 24px rgba(0,0,0,.15)'" onmouseout="this.style.transform='';this.style.boxShadow=''">
            <div style="display:flex;align-items:center;gap:10px;margin-bottom:8px;">
              <div style="font-size:1.5rem;">${t.icon}</div>
              <div>
                <div style="font-weight:600;color:var(--text-primary);">${t.name}</div>
                <span class="badge" style="font-size:.7rem;padding:2px 8px;background:rgba(59,130,246,.1);color:var(--blue);">${t.category}</span>
              </div>
            </div>
            <div style="font-size:.85rem;color:var(--text-muted);margin-bottom:12px;">${t.desc}</div>
            <div style="display:flex;gap:.5rem;">
              <button class="btn-primary" style="font-size:.8rem;flex:1;">Deploy</button>
              <button class="btn" style="font-size:.8rem;">Preview</button>
            </div>
          </div>
        `).join('')}
      </div>
    </div>
  `;
}
function filterTemplates(cat) { document.querySelectorAll('.template-card').forEach(el => { el.style.display = (cat === 'all' || el.dataset.category === cat) ? '' : 'none'; }); }
function searchTemplates(q) { document.querySelectorAll('.template-card').forEach(el => { el.style.display = el.textContent.toLowerCase().includes(q.toLowerCase()) ? '' : 'none'; }); }
