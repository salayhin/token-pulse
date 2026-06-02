'use strict';

const fmtUSD = n => '$' + (n || 0).toFixed(2);
const fmtInt = n => (n || 0).toLocaleString();
const fmtPct = n => ((n || 0) * 100).toFixed(1) + '%';
const shortTs = s => (s || '').slice(0, 16);

// Convert UTC ISO timestamp to user's timezone (extracted HH:MM:SS)
function tsInTz(isoStr, tzName) {
  if (!isoStr) return '';
  try {
    const date = new Date(isoStr);
    const formatter = new Intl.DateTimeFormat('en-US', {
      timeZone: tzName || 'UTC',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false
    });
    const parts = formatter.formatToParts(date);
    const h = parts.find(p => p.type === 'hour')?.value || '00';
    const m = parts.find(p => p.type === 'minute')?.value || '00';
    const s = parts.find(p => p.type === 'second')?.value || '00';
    return `${h}:${m}:${s}`;
  } catch {
    return (isoStr || '').slice(11, 19);
  }
}

// fmtMtoks: render as Millions. 2 dp for ≥1M, 4 dp for <1M so small counts
// (e.g. 5,000 → 0.0050M) stay legible instead of rounding to 0.00M.
function fmtMtoks(n) {
  const v = (n || 0) / 1_000_000;
  return v.toFixed(v >= 1 ? 2 : 4) + 'M';
}

// fmtTokens: compact per-message — raw int below 1k, k for thousands, M for millions.
function fmtTokens(n) {
  const v = n || 0;
  if (v < 1000) return String(v);
  if (v < 1_000_000) return (v / 1000).toFixed(v < 10_000 ? 1 : 0) + 'k';
  return (v / 1_000_000).toFixed(2) + 'M';
}

// Use whenever interpolating user-controlled text into an innerHTML template.
function escapeHtml(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function humanDuration(startISO, endISO) {
  if (!startISO || !endISO) return '—';
  const a = Date.parse(startISO.replace(' ', 'T') + 'Z');
  const b = Date.parse(endISO.replace(' ', 'T') + 'Z');
  if (!a || !b || b < a) return '—';
  let s = Math.round((b - a) / 1000);
  const d = Math.floor(s / 86400); s -= d * 86400;
  const h = Math.floor(s / 3600);  s -= h * 3600;
  const m = Math.floor(s / 60);
  if (d) return `${d}d ${h}h`;
  if (h) return `${h}h ${m}m`;
  return `${m}m`;
}

async function getJSON(url) {
  const r = await fetch(url);
  if (!r.ok) throw new Error(url + ' → ' + r.status);
  return r.json();
}

function openSessionInNewTab(id) {
  // ?focus is read on boot to render the session-only "focus" view.
  const url = window.location.origin + window.location.pathname +
              '?focus#sessions/' + encodeURIComponent(id);
  window.open(url, '_blank', 'noopener');
}

function renderSkillsList(items, containerSelector, allSelector, buttonSelector) {
  const container = document.querySelector(containerSelector);
  const allContainer = document.querySelector(allSelector);
  const toggleBtn = document.querySelector(buttonSelector);

  if (!container || !items || items.length === 0) {
    if (container) container.innerHTML = '<div style="color:#666;">—</div>';
    if (toggleBtn) toggleBtn.classList.add('hidden');
    return;
  }

  const top10 = items.slice(0, 10);
  const hasMore = items.length > 10;

  // Render top 10
  container.innerHTML = top10.map(item => `
    <div class="skills-plugins-item">
      <span class="name">${escapeHtml(item.name)}</span>
      <span class="percentage">${item.percentage.toFixed(1)}%</span>
    </div>
  `).join('');

  // Render all (hidden)
  if (hasMore && allContainer) {
    allContainer.innerHTML = items.map(item => `
      <div class="skills-plugins-item">
        <span class="name">${escapeHtml(item.name)}</span>
        <span class="percentage">${item.percentage.toFixed(1)}%</span>
      </div>
    `).join('');

    if (toggleBtn) {
      toggleBtn.classList.remove('hidden');
      toggleBtn.textContent = 'Show all';
      toggleBtn.addEventListener('click', function() {
        const isHidden = allContainer.classList.contains('hidden');
        allContainer.classList.toggle('hidden');
        this.textContent = isHidden ? 'Hide' : 'Show all';
      });
    }
  }
}

async function loadOverviewSkills() {
  try {
    const data = await getJSON('/api/v1/skills');
    renderSkillsList(data.skills, '#skills-list', '#skills-all', '#skills-show-all');
    renderSkillsList(data.plugins, '#plugins-list', '#plugins-all', '#plugins-show-all');
  } catch (err) {
    console.error('Failed to load skills:', err);
    document.querySelector('#skills-list').innerHTML = '<div style="color:#f55;">Error loading skills</div>';
  }
}

async function loadSessionSkills(sessionId) {
  try {
    const data = await getJSON(`/api/v1/sessions/${encodeURIComponent(sessionId)}/skills`);
    renderSkillsList(data.skills, '#session-skills-list', '#session-skills-all', '#session-skills-show-all');
    renderSkillsList(data.plugins, '#session-plugins-list', '#session-plugins-all', '#session-plugins-show-all');
  } catch (err) {
    console.error('Failed to load session skills:', err);
  }
}

// ---------- Theme ----------
function applyTheme(t) {
  document.documentElement.setAttribute('data-theme', t);
  localStorage.setItem('tp-theme', t);
}
(function initTheme() {
  const saved = localStorage.getItem('tp-theme');
  if (saved) applyTheme(saved);
  else if (matchMedia && matchMedia('(prefers-color-scheme: light)').matches) applyTheme('light');
  else applyTheme('dark');
})();
document.getElementById('theme-toggle').addEventListener('click', () => {
  const cur = document.documentElement.getAttribute('data-theme') || 'dark';
  applyTheme(cur === 'dark' ? 'light' : 'dark');
});

// ---------- Hash routing (saved views) ----------
// #overview · #projects · #sessions · #sessions/<id> · #project/<slug>
function parseHash() {
  const h = (location.hash || '#overview').slice(1);
  const [head, ...rest] = h.split('/');
  return { head, arg: rest.join('/') };
}

function setHash(h) {
  if (location.hash !== '#' + h) location.hash = h;
}

// ---------- Tabs ----------
function activateTab(name, opts = {}) {
  for (const b of document.querySelectorAll('.tab')) {
    b.classList.toggle('active', b.dataset.tab === name);
  }
  for (const v of document.querySelectorAll('.view')) {
    v.classList.toggle('hidden', v.id !== 'view-' + name);
  }
  if (name === 'overview') loadOverview();
  if (name === 'projects') loadProjects();
  if (name === 'sessions' && !opts.suppressLoad) loadSessions(true);
  if (name === 'budget') loadBudget();
  if (name === 'settings') loadSettings();
  if (!opts.suppressHash) setHash(name);
}
document.querySelectorAll('.tab').forEach(b =>
  b.addEventListener('click', () => activateTab(b.dataset.tab)));

// Logo click to go home
document.getElementById('logo')?.addEventListener('click', (e) => {
  e.preventDefault();
  activateTab('overview');
});

// ---------- Overview ----------
function fillTotals(prefix, t) {
  document.getElementById(prefix + '-cost').textContent = fmtUSD(t.cost_usd);
  document.getElementById(prefix + '-sessions').textContent = fmtInt(t.sessions);
  document.getElementById(prefix + '-messages').textContent = fmtInt(t.messages);
  document.getElementById(prefix + '-tools').textContent = fmtInt(t.tool_calls);
}
function fillCache(c) {
  document.getElementById('cache-rate').textContent = fmtPct(c.hit_rate);
  document.getElementById('cache-reads').textContent = fmtMtoks(c.cache_read_tokens);
  document.getElementById('cache-creates').textContent = fmtMtoks(c.cache_create_tokens);
}
function fillProjection(p) {
  document.getElementById('proj-mtd').textContent = fmtUSD(p.month_to_date_usd);
  document.getElementById('proj-avg').textContent = fmtUSD(p.basis_daily_avg_usd);
  document.getElementById('proj-month').textContent = fmtUSD(p.projected_month_usd);
  document.getElementById('proj-rem').textContent = (p.days_in_month - p.days_elapsed) + ' / ' + p.days_in_month;
}
function fillDailyTable(rows) {
  const tbody = document.querySelector('#daily-table tbody');
  tbody.innerHTML = '';
  for (const r of rows) {
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td>${r.date}</td><td>${fmtInt(r.sessions)}</td><td>${fmtInt(r.messages)}</td>
      <td>${fmtMtoks(r.input_tokens)}</td><td>${fmtMtoks(r.output_tokens)}</td>
      <td>${fmtMtoks(r.cache_read_tokens)}</td><td>${fmtMtoks(r.cache_create_tokens)}</td>
      <td>${fmtUSD(r.cost_usd)}</td>`;
    tbody.appendChild(tr);
  }
}

let trendsChart, toolsChart;
function trendsChartRender(points) {
  const labels = points.map(p => p.date);
  if (trendsChart) trendsChart.destroy();
  trendsChart = new Chart(document.getElementById('trends-chart'), {
    type: 'bar',
    data: {
      labels,
      datasets: [
        { label: 'Cost', data: points.map(p => p.cost_usd), backgroundColor: '#7c9cff', order: 2 },
        { label: '7d MA', data: points.map(p => p.ma7), type: 'line', borderColor: '#5fcfa6', tension: 0.25, pointRadius: 0, order: 1 },
      ],
    },
    options: {
      plugins: { legend: { labels: { color: '#9aa0aa' } } },
      scales: {
        x: { ticks: { color: '#9aa0aa' }, grid: { color: '#1d1f25' } },
        y: { ticks: { color: '#9aa0aa' }, grid: { color: '#1d1f25' } },
      },
    },
  });
}
function toolsChartRender(tools) {
  if (toolsChart) toolsChart.destroy();
  toolsChart = new Chart(document.getElementById('tools-chart'), {
    type: 'bar',
    data: { labels: tools.map(t => t.name), datasets: [{ label: 'Calls', data: tools.map(t => t.count), backgroundColor: '#5fcfa6' }] },
    options: {
      indexAxis: 'y',
      plugins: { legend: { display: false } },
      scales: {
        x: { ticks: { color: '#9aa0aa' }, grid: { color: '#1d1f25' } },
        y: { ticks: { color: '#9aa0aa' }, grid: { color: '#1d1f25' } },
      },
    },
  });
}

function modelsTableRender(models) {
  const tbody = document.querySelector('#models-table tbody');
  if (!tbody) return;
  tbody.innerHTML = '';
  if (!models || !models.length) {
    tbody.innerHTML = `<tr><td colspan="6" class="muted">No assistant messages yet.</td></tr>`;
    return;
  }
  for (const m of models) {
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td style="text-align:left"><code>${escapeHtml(m.model)}</code></td>
      <td>${fmtInt(m.assistant_messages)}</td>
      <td>${fmtMtoks(m.input_tokens)}</td>
      <td>${fmtMtoks(m.output_tokens)}</td>
      <td>${fmtUSD(m.cost_usd)}</td>
      <td>${fmtUSD(m.avg_cost_per_msg_usd)}</td>`;
    tbody.appendChild(tr);
  }
}

function renderCostBreakdown(lineItems) {
  const tbody = document.querySelector('#sd-cost-breakdown-table tbody');
  if (!tbody) return;
  tbody.innerHTML = '';

  if (!lineItems || !lineItems.length) {
    tbody.innerHTML = `<tr><td colspan="4" class="muted">No costs.</td></tr>`;
    return;
  }

  let totalCost = 0;
  for (const item of lineItems) {
    const tr = document.createElement('tr');
    const tokenTypeLabel = item.token_type.replace(/_/g, ' ');
    const lineItemLabel = `${escapeHtml(item.model)} ${tokenTypeLabel}`;
    tr.innerHTML = `
      <td style="text-align:left">${lineItemLabel}</td>
      <td>${fmtMtoks(item.token_count)}</td>
      <td>$${item.rate.toFixed(2)}/1M</td>
      <td>${fmtUSD(item.cost_usd)}`;
    tbody.appendChild(tr);
    totalCost += item.cost_usd;
  }

  // Add total row
  const totalRow = document.createElement('tr');
  totalRow.style.fontWeight = 'bold';
  totalRow.style.borderTop = '1px solid var(--border)';
  totalRow.innerHTML = `
    <td style="text-align:left">Total</td>
    <td>—</td>
    <td>—</td>
    <td>${fmtUSD(totalCost)}`;
  tbody.appendChild(totalRow);
}

let currentTimezone = 'UTC'; // Store current user timezone

async function loadOverview() {
  const [stats, daily, cache, trends, proj, models] = await Promise.all([
    getJSON('/api/v1/stats'),
    getJSON('/api/v1/stats/daily?days=30'),
    getJSON('/api/v1/cache'),
    getJSON('/api/v1/stats/trends?days=30'),
    getJSON('/api/v1/stats/projections'),
    getJSON('/api/v1/models'),
  ]);
  currentTimezone = stats.timezone || 'UTC';
  document.getElementById('tz').textContent = currentTimezone;
  fillTotals('today', stats.today);
  fillTotals('all', stats.all_time);
  fillCache(cache);
  fillProjection(proj);
  fillDailyTable(daily.daily || []);
  trendsChartRender(trends.trends || []);
  modelsTableRender(models.models || []);
  loadOverviewSkills();
}

// ---------- Budget ----------
// Renders today/this-week spend against the configured budgets. The bar
// turns yellow at ≥75% and red when over budget; if no budget is set,
// the bar is muted and the metric label says "not set".
async function loadBudget() {
  try {
    const data = await getJSON('/api/v1/budget');
    document.getElementById('budget-tz').textContent = data.timezone ? `(${data.timezone})` : '';

    // Replace the generic derivation hint with the concrete numbers when set.
    const dHint = document.getElementById('budget-derivation');
    if (dHint) {
      if (data.monthly_budget_usd > 0 && data.days_in_month > 0) {
        dHint.textContent =
          `daily = $${data.derived_daily_usd.toFixed(2)} ` +
          `($${data.monthly_budget_usd.toFixed(2)} ÷ ${data.days_in_month}d), ` +
          `weekly = $${data.derived_weekly_usd.toFixed(2)}`;
      } else {
        dHint.textContent = 'daily = monthly ÷ days in month, weekly = daily × 7';
      }
    }

    renderBudgetCard('day', data.day);
    renderBudgetCard('week', data.week);
    renderBudgetCard('month', data.month);

    const hint = document.getElementById('budget-empty-hint');
    if (hint) {
      hint.textContent = data.monthly_budget_usd > 0
        ? ''
        : 'No monthly budget configured yet — set one in Settings → Budget.';
    }
  } catch (err) {
    console.error('Failed to load budget:', err);
  }
}

function renderBudgetCard(prefix, p) {
  const set = (id, v) => { const el = document.getElementById(id); if (el) el.textContent = v; };
  set(`budget-${prefix}-cost`, fmtUSD(p.cost_usd));
  set(`budget-${prefix}-limit`, p.budget_usd > 0 ? fmtUSD(p.budget_usd) : 'not set');
  if (p.budget_usd > 0) {
    set(`budget-${prefix}-remaining`,
        (p.remaining_usd >= 0 ? '' : '−') + fmtUSD(Math.abs(p.remaining_usd)));
    set(`budget-${prefix}-pct`, p.used_percent.toFixed(1) + '%');
  } else {
    set(`budget-${prefix}-remaining`, '—');
    set(`budget-${prefix}-pct`, '—');
  }

  // Window label, e.g. "Mon Jun 02 – Mon Jun 09 (excl.)"
  const win = document.getElementById(`budget-${prefix}-window`);
  if (win) {
    const fmt = (iso) => {
      try { return new Date(iso).toLocaleString(undefined, { weekday: 'short', month: 'short', day: '2-digit' }); }
      catch { return iso; }
    };
    win.textContent = `${fmt(p.start)} → ${fmt(p.end)}`;
  }

  // Progress bar
  const wrap = document.getElementById(`budget-${prefix}-bar-wrap`);
  const fill = document.getElementById(`budget-${prefix}-bar`);
  if (!wrap || !fill) return;
  if (p.budget_usd <= 0) {
    wrap.classList.add('disabled');
    fill.style.width = '0%';
    fill.classList.remove('warn', 'over');
    return;
  }
  wrap.classList.remove('disabled');
  const pct = Math.max(0, Math.min(100, p.used_percent));
  fill.style.width = pct + '%';
  fill.classList.toggle('over', p.used_percent >= 100);
  fill.classList.toggle('warn', p.used_percent >= 75 && p.used_percent < 100);
}

// ---------- Projects ----------
async function loadProjects() {
  const data = await getJSON('/api/v1/projects');
  const tbody = document.querySelector('#projects-table tbody');
  tbody.innerHTML = '';
  for (const p of data.projects || []) {
    const tr = document.createElement('tr');
    tr.innerHTML = `
      <td>${p.slug}</td><td>${fmtInt(p.sessions)}</td><td>${fmtInt(p.messages)}</td>
      <td>${fmtInt(p.tool_calls)}</td><td>${fmtUSD(p.cost_usd)}</td><td>${shortTs(p.last_active)}</td>`;
    tr.addEventListener('click', () => setHash('project/' + p.slug));
    tbody.appendChild(tr);
  }
}

// ---------- Sessions ----------
const sessionsState = { project: '', cursor: '', loaded: 0, from: '', to: '' };

async function loadSessions(reset) {
  if (reset) {
    sessionsState.cursor = '';
    sessionsState.loaded = 0;
    document.querySelector('#sessions-table tbody').innerHTML = '';
    document.getElementById('session-detail').classList.add('hidden');
    document.getElementById('sessions-table').classList.remove('hidden');
    document.getElementById('sessions-filter').classList.remove('hidden');
    document.querySelector('.pager').classList.remove('hidden');
  }
  const params = new URLSearchParams({ limit: '50' });
  if (sessionsState.project) params.set('project', sessionsState.project);
  if (sessionsState.cursor) params.set('cursor', sessionsState.cursor);
  if (sessionsState.from) params.set('from', sessionsState.from);
  if (sessionsState.to) params.set('to', sessionsState.to);
  const data = await getJSON('/api/v1/sessions?' + params.toString());
  const tbody = document.querySelector('#sessions-table tbody');
  for (const s of data.sessions || []) {
    const tr = document.createElement('tr');
    const titleCell = s.display_title
      ? `<span class="session-title">${escapeHtml(s.display_title)}</span>`
      : `<span class="session-id">${s.id.slice(0, 8)}</span>`;
    const modelCell = s.dominant_model
      ? `<code>${escapeHtml(s.dominant_model)}</code>`
      : '<span class="muted">—</span>';
    tr.innerHTML = `
      <td>${shortTs(s.ended_at)}</td><td>${titleCell}</td>
      <td>${s.project_slug}</td>
      <td>${modelCell}</td>
      <td>${fmtInt(s.message_count)}</td><td>${fmtInt(s.tool_calls)}</td>
      <td>${fmtUSD(s.cost_usd)}</td><td>${escapeHtml((s.first_prompt || '').slice(0, 80))}</td>`;
    tr.title = 'Open session';
    tr.addEventListener('click', () => showSession(s.id));
    tbody.appendChild(tr);
  }
  sessionsState.loaded += (data.sessions || []).length;
  sessionsState.cursor = data.next_cursor || '';
  const parts = [];
  if (sessionsState.project) parts.push(sessionsState.project);
  if (sessionsState.from || sessionsState.to) {
    parts.push(`${sessionsState.from || '…'} → ${sessionsState.to || sessionsState.from || '…'}`);
  }
  const meta = parts.length ? '· ' + parts.join(' · ') : '';
  document.getElementById('sessions-meta').textContent = `(${sessionsState.loaded} loaded ${meta})`;
  document.getElementById('sessions-more').style.display = sessionsState.cursor ? '' : 'none';
}
document.getElementById('sessions-more').addEventListener('click', () => loadSessions(false));

document.getElementById('sessions-filter').addEventListener('submit', e => {
  e.preventDefault();
  const fromEl = document.getElementById('sessions-from');
  const toEl = document.getElementById('sessions-to');
  let from = fromEl.value;
  let to = toEl.value;
  // If only one bound is set, mirror it onto the other so the user gets a single-day filter.
  if (from && !to) { to = from; toEl.value = from; }
  if (to && !from) { from = to; fromEl.value = to; }
  if (from && to && from > to) { [from, to] = [to, from]; fromEl.value = from; toEl.value = to; }
  sessionsState.from = from;
  sessionsState.to = to;
  loadSessions(true);
});

document.getElementById('sessions-clear').addEventListener('click', () => {
  document.getElementById('sessions-from').value = '';
  document.getElementById('sessions-to').value = '';
  sessionsState.from = '';
  sessionsState.to = '';
  loadSessions(true);
});

let sdTimelineChart, sdToolsChart;
async function showSession(id, opts = {}) {
  if (!opts.suppressHash) setHash('sessions/' + id);
  const d = await getJSON('/api/v1/sessions/' + encodeURIComponent(id));
  if (!d) return;
  document.getElementById('sessions-table').classList.add('hidden');
  document.getElementById('sessions-filter').classList.add('hidden');
  document.querySelector('.pager').classList.add('hidden');
  document.getElementById('session-detail').classList.remove('hidden');
  const title = d.session.display_title || id;
  document.getElementById('sd-title').textContent = title;
  const idSuffix = d.session.display_title ? ` · ${id.slice(0, 8)}` : '';
  document.getElementById('sd-meta').textContent =
    `${d.session.project_slug} · branch ${d.session.git_branch || '—'} · ` +
    `${d.session.message_count} messages · ${fmtUSD(d.session.cost_usd)} · ` +
    `${shortTs(d.session.started_at)} → ${shortTs(d.session.ended_at)}${idSuffix}`;

  // Cards
  document.getElementById('sd-cost').textContent = fmtUSD(d.session.cost_usd);
  document.getElementById('sd-messages').textContent = fmtInt(d.session.message_count);
  document.getElementById('sd-tools-count').textContent = fmtInt(d.session.tool_calls);
  document.getElementById('sd-started').textContent = shortTs(d.session.started_at) || '—';
  document.getElementById('sd-ended').textContent = shortTs(d.session.ended_at) || '—';
  document.getElementById('sd-duration').textContent = humanDuration(d.session.started_at, d.session.ended_at);
  if (d.cache) {
    document.getElementById('sd-cache-rate').textContent = fmtPct(d.cache.hit_rate);
    document.getElementById('sd-cache-reads').textContent = fmtMtoks(d.cache.cache_read_tokens);
    document.getElementById('sd-cache-creates').textContent = fmtMtoks(d.cache.cache_create_tokens);
  }

  // Models card: count assistant turns per real model (skip <synthetic> stubs).
  const modelCounts = {};
  for (const m of d.messages || []) {
    if (m.role !== 'assistant' || !m.model || m.model.startsWith('<')) continue;
    modelCounts[m.model] = (modelCounts[m.model] || 0) + 1;
  }
  const sortedModels = Object.entries(modelCounts).sort((a, b) => b[1] - a[1]);
  const sdModels = document.getElementById('sd-models');
  if (sdModels) {
    sdModels.innerHTML = sortedModels.length
      ? sortedModels.map(([model, count]) => `
          <div class="metric">
            <span class="label" style="font-family:ui-monospace,monospace">${escapeHtml(model)}</span>
            <span>${fmtInt(count)} msg${count === 1 ? '' : 's'}</span>
          </div>`).join('')
      : '<span class="muted">—</span>';
  }

  // Render detailed cost line-item breakdown
  renderCostBreakdown(d.cost_line_items || []);

  // Timeline chart: assistant turns over time, cost on Y.
  const turns = (d.messages || []).filter(m => m.role === 'assistant');
  if (sdTimelineChart) sdTimelineChart.destroy();
  sdTimelineChart = new Chart(document.getElementById('sd-timeline-chart'), {
    type: 'bar',
    data: {
      labels: turns.map(t => tsInTz(t.ts, currentTimezone)),
      datasets: [{ label: 'Cost', data: turns.map(t => t.cost_usd || 0), backgroundColor: '#7c9cff' }],
    },
    options: {
      plugins: { legend: { display: false } },
      scales: {
        x: { ticks: { color: '#9aa0aa', maxRotation: 0, autoSkip: true, maxTicksLimit: 12 }, grid: { color: '#1d1f25' } },
        y: { ticks: { color: '#9aa0aa', callback: v => '$' + v }, grid: { color: '#1d1f25' } },
      },
    },
  });

  // Top tools (this session): aggregate from message tool_calls.
  const counts = {};
  for (const m of d.messages || []) {
    for (const t of m.tool_calls || []) counts[t.name] = (counts[t.name] || 0) + 1;
  }
  const top = Object.entries(counts).sort((a, b) => b[1] - a[1]).slice(0, 12);
  if (sdToolsChart) sdToolsChart.destroy();
  sdToolsChart = new Chart(document.getElementById('sd-tools-chart'), {
    type: 'bar',
    data: { labels: top.map(t => t[0]), datasets: [{ data: top.map(t => t[1]), backgroundColor: '#5fcfa6' }] },
    options: {
      indexAxis: 'y',
      plugins: { legend: { display: false } },
      scales: {
        x: { ticks: { color: '#9aa0aa' }, grid: { color: '#1d1f25' } },
        y: { ticks: { color: '#9aa0aa' }, grid: { color: '#1d1f25' } },
      },
    },
  });

  const thread = document.getElementById('sd-thread');
  thread.innerHTML = '';
  // Render newest-first; the timeline chart still uses ascending order.
  for (const m of [...(d.messages || [])].reverse()) {
    if (m.role === 'user-tool-result') continue;
    const div = document.createElement('div');
    div.className = 'turn ' + m.role;
    const cost = m.cost_usd ? `<span class="cost">${fmtUSD(m.cost_usd)}</span>` : '';
    const meta = m.role === 'assistant' ? `${m.role} · ${m.model || ''} · ${shortTs(m.ts)}` : `${m.role} · ${shortTs(m.ts)}`;
    const tools = (m.tool_calls || []).map(t => t.name).join(', ');
    let tokenLine = '';
    if (m.role === 'assistant') {
      const tok = { in: m.input_tokens || 0, out: m.output_tokens || 0, cc: m.cache_create_tokens || 0, cr: m.cache_read_tokens || 0 };
      if (tok.in || tok.out || tok.cc || tok.cr) {
        tokenLine = `<div class="tokens">
          <span title="Input tokens">in <b>${fmtTokens(tok.in)}</b></span>
          <span title="Output tokens">out <b>${fmtTokens(tok.out)}</b></span>
          <span title="Cache create tokens">cache cr <b>${fmtTokens(tok.cc)}</b></span>
          <span title="Cache read tokens">cache rd <b>${fmtTokens(tok.cr)}</b></span>
        </div>`;
      }
    }
    div.innerHTML = `
      <div class="role">${meta}${cost}</div>
      ${tokenLine}
      <div class="body"></div>
      ${tools ? `<div class="tools">↳ ${tools}</div>` : ''}`;
    div.querySelector('.body').textContent = m.text || m.preview || '';
    thread.appendChild(div);
  }
}
document.getElementById('session-back').addEventListener('click', () => loadSessions(true));

document.getElementById('sd-theme-toggle').addEventListener('click', () => {
  const cur = document.documentElement.getAttribute('data-theme') || 'dark';
  applyTheme(cur === 'dark' ? 'light' : 'dark');
});

document.getElementById('sd-open-in-app').addEventListener('click', () => {
  const { arg } = parseHash();
  if (arg) window.location.href = '/#sessions/' + encodeURIComponent(arg);
});

document.getElementById('sd-copy-link').addEventListener('click', async () => {
  const btn = document.getElementById('sd-copy-link');
  try {
    await navigator.clipboard.writeText(window.location.href);
    const orig = btn.textContent;
    btn.textContent = 'Copied!';
    setTimeout(() => { btn.textContent = orig; }, 1200);
  } catch {
    btn.textContent = 'Copy failed';
  }
});

// ---------- SSE live updates ----------
function connectEvents() {
  const live = document.getElementById('live');
  const es = new EventSource('/api/v1/events');
  es.addEventListener('ready', () => live.classList.remove('stale'));
  es.addEventListener('updated', () => {
    live.style.color = '#ffd96a';
    setTimeout(() => { live.style.color = ''; }, 600);
    // Refresh whatever tab is active.
    const active = document.querySelector('.tab.active').dataset.tab;
    if (active === 'overview') loadOverview();
    if (active === 'projects') loadProjects();
    if (active === 'sessions') loadSessions(true);
    if (active === 'budget') loadBudget();
  });
  es.onerror = () => live.classList.add('stale');
}

// ---------- Hash → view dispatch ----------
async function dispatchHash() {
  const { head, arg } = parseHash();
  if (head === 'sessions' && arg) {
    activateTab('sessions', { suppressLoad: true, suppressHash: true });
    await showSession(arg, { suppressHash: true });
    return;
  }
  if (head === 'project' && arg) {
    sessionsState.project = arg;
    sessionsState.cursor = '';
    activateTab('sessions', { suppressLoad: true, suppressHash: true });
    setHash('project/' + arg);
    await loadSessions(true);
    return;
  }
  if (head === 'search' && arg) {
    activateTab('search', { suppressHash: true });
    await runSearch(decodeURIComponent(arg), { suppressHash: true });
    return;
  }
  if (['overview', 'projects', 'sessions', 'budget', 'settings', 'tools', 'search'].includes(head)) {
    activateTab(head, { suppressHash: true });
    return;
  }
  activateTab('overview', { suppressHash: true });
}
window.addEventListener('hashchange', dispatchHash);

// ---------- boot ----------
const focusMode = new URLSearchParams(window.location.search).has('focus');

(async function main() {
  try {
    if (focusMode) {
      document.body.classList.add('focus-mode');
      const { head, arg } = parseHash();
      if (head === 'sessions' && arg) {
        // Render only the session detail; no chrome, no chart fetches.
        document.getElementById('view-sessions').classList.remove('hidden');
        document.getElementById('sessions-table').classList.add('hidden');
        document.querySelector('.pager').classList.add('hidden');
        await showSession(arg, { suppressHash: true });
        connectEvents();
        return;
      }
    }
    const { head } = parseHash();
    // For deep-linked session/search/project views, don't pay for the overview fetch up front.
    if (['sessions', 'project', 'search'].includes(head)) {
      await dispatchHash();
      loadOverview().catch(() => {});
    } else {
      await loadOverview();
      await dispatchHash();
    }
    connectEvents();
  } catch (e) {
    document.body.insertAdjacentHTML('afterbegin',
      '<div style="background:#5b1f1f;color:#fff;padding:12px;border-radius:6px;margin-bottom:16px">' +
      'Failed to load: ' + e.message + '</div>');
  }
})();

// ---------- Settings ----------
const FIRST_RUN_MODELS = [
  'claude-opus-4', 'claude-sonnet-4', 'claude-haiku-4',
  'claude-opus-4-5', 'claude-sonnet-4-5', 'claude-haiku-4-5',
];

let settingsData = null;

async function loadSettings() {
  try {
    const data = await getJSON('/api/v1/settings');
    settingsData = data;
    renderSettings(data);
  } catch (err) {
    console.error('Failed to load settings:', err);
  }
}

function renderSettings(data) {
  // Config path
  const pathEl = document.getElementById('settings-path');
  if (pathEl) pathEl.textContent = data.config_path ? `(${data.config_path})` : '';

  // Timezone selector
  const tzSel = document.getElementById('settings-tz');
  const tzCur = document.getElementById('settings-tz-current');
  if (tzSel) {
    const zones = Intl.supportedValuesOf ? Intl.supportedValuesOf('timeZone') : [];
    const current = data.timezone || 'UTC';
    tzSel.innerHTML = zones.map(z =>
      `<option value="${z}"${z === current ? ' selected' : ''}>${z}</option>`
    ).join('');
    if (tzCur) tzCur.textContent = `Current: ${current}`;
  }

  // Budget (single monthly value; daily/weekly are derived on the Budget tab)
  const monthlyEl = document.getElementById('settings-monthly-budget');
  if (monthlyEl) monthlyEl.value = data.monthly_budget != null ? data.monthly_budget : 0;

  // Pricing table
  renderPricingTable(data);

  // Populate datalist for model autocomplete
  const dl = document.getElementById('observed-models-list');
  if (dl) {
    const observed = data.observed_models && data.observed_models.length
      ? data.observed_models : FIRST_RUN_MODELS;
    dl.innerHTML = observed.map(m => `<option value="${escapeHtml(m)}">`).join('');
  }
}

function renderPricingTable(data) {
  const tbody = document.querySelector('#settings-pricing tbody');
  if (!tbody) return;
  const pricing = data.pricing || {};
  const models = pricing.models || {};
  const observed = data.observed_models && data.observed_models.length
    ? data.observed_models : FIRST_RUN_MODELS;

  tbody.innerHTML = observed.map(model => {
    const rates = models[model] || resolveRatesForModel(model, models, pricing.fallback || {});
    return pricingRow(model, rates, true);
  }).join('');

  // Wire delete buttons
  tbody.querySelectorAll('.settings-delete').forEach(btn => {
    btn.addEventListener('click', () => deleteModel(btn.dataset.model));
  });
}

function pricingRow(model, rates, observed) {
  const tag = observed
    ? `<span class="settings-row-tag" title="observed in your usage data">●</span>`
    : `<span class="settings-row-tag muted" title="not yet seen in usage data">○</span>`;
  return `<tr>
    <td style="text-align:left">${tag}<code>${escapeHtml(model)}</code></td>
    <td>${(rates.input || 0).toFixed(3)}</td>
    <td>${(rates.output || 0).toFixed(3)}</td>
    <td>${(rates.cache_read || 0).toFixed(3)}</td>
    <td>${(rates.cache_create || 0).toFixed(3)}</td>
    <td>${(rates.cache_create_1h || 0).toFixed(3)}</td>
    <td><button class="btn settings-delete" data-model="${escapeHtml(model)}" title="Edit">✎</button></td>
  </tr>`;
}

function resolveRatesForModel(model, models, fallback) {
  // Longest-prefix match
  let best = '', bestRates = fallback;
  for (const [k, v] of Object.entries(models)) {
    if (model.startsWith(k) && k.length > best.length) { best = k; bestRates = v; }
  }
  return bestRates;
}

function deleteModel(model) {
  // Open edit form pre-filled for this model
  openEditForm(model);
}

function openEditForm(model) {
  const form = document.getElementById('settings-edit-form');
  if (!form) return;
  form.classList.remove('hidden');

  const pricing = (settingsData && settingsData.pricing) || {};
  const models = pricing.models || {};
  const rates = models[model] || resolveRatesForModel(model, models, pricing.fallback || {});

  document.getElementById('edit-model-name').value = model || '';
  document.getElementById('edit-input').value = (rates.input || 0).toFixed(3);
  document.getElementById('edit-output').value = (rates.output || 0).toFixed(3);
  document.getElementById('edit-cache-read').value = (rates.cache_read || 0).toFixed(3);
  document.getElementById('edit-cache-create').value = (rates.cache_create || 0).toFixed(3);
  document.getElementById('edit-cache-create-1h').value = (rates.cache_create_1h || 0).toFixed(3);
  document.getElementById('edit-model-name').dataset.original = model || '';
}

document.getElementById('settings-add-btn').addEventListener('click', () => openEditForm(''));
document.getElementById('edit-cancel-btn').addEventListener('click', () => {
  document.getElementById('settings-edit-form').classList.add('hidden');
});

document.getElementById('edit-save-btn').addEventListener('click', async () => {
  const model = document.getElementById('edit-model-name').value.trim();
  if (!model) { alert('Model name is required'); return; }
  const patch = collectSettings();
  patch.pricing.models = patch.pricing.models || {};
  patch.pricing.models[model] = {
    input: parseFloat(document.getElementById('edit-input').value) || 0,
    output: parseFloat(document.getElementById('edit-output').value) || 0,
    cache_read: parseFloat(document.getElementById('edit-cache-read').value) || 0,
    cache_create: parseFloat(document.getElementById('edit-cache-create').value) || 0,
    cache_create_1h: parseFloat(document.getElementById('edit-cache-create-1h').value) || 0,
  };
  await saveSettings(patch, 'settings-status');
  document.getElementById('settings-edit-form').classList.add('hidden');
});

document.getElementById('settings-tz').addEventListener('change', async () => {
  await saveSettings(collectSettings(), 'settings-tz-status');
});

// Monthly budget: save on blur (so users can finish typing) rather than per-keystroke.
(() => {
  const el = document.getElementById('settings-monthly-budget');
  if (!el) return;
  el.addEventListener('change', async () => {
    await saveSettings(collectSettings(), 'settings-monthly-budget-status');
    // The Budget tab depends on this value — refresh in the background so
    // users see the derived daily/weekly numbers update without clicking.
    loadBudget().catch(() => {});
  });
})();

function collectSettings() {
  const tz = document.getElementById('settings-tz').value;
  const pricing = (settingsData && settingsData.pricing) ? JSON.parse(JSON.stringify(settingsData.pricing)) : {};
  const modelBudgets = (settingsData && settingsData.model_budgets)
    ? JSON.parse(JSON.stringify(settingsData.model_budgets)) : {};
  const monthlyEl = document.getElementById('settings-monthly-budget');
  const monthly = monthlyEl ? parseFloat(monthlyEl.value) : 0;
  return {
    timezone: tz,
    pricing,
    model_budgets: modelBudgets,
    monthly_budget: Number.isFinite(monthly) ? monthly : 0,
  };
}

async function saveSettings(patch, statusId) {
  const statusEl = document.getElementById(statusId);
  try {
    const resp = await fetch('/api/v1/settings', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    });
    if (!resp.ok) throw new Error(await resp.text());
    if (statusEl) { statusEl.textContent = 'Saved ✓'; setTimeout(() => { statusEl.textContent = ''; }, 2000); }
    await loadSettings();
  } catch (err) {
    if (statusEl) statusEl.textContent = 'Error: ' + err.message;
  }
}
