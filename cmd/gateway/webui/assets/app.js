/* ==========================================================================
   NeteaseBedrockGateway Web 控制台 —— 前端逻辑（对接后端 HTTP API）
   --------------------------------------------------------------------------
   · 会话：浏览器会话 Cookie（后端不设 Max-Age）→ 关闭标签页即退出登录
   · 数据：全部来自 /api/*，不再使用 localStorage
   · 日志：EventSource 订阅 SSE（/api/instances/{id}/logs?stream=1）实时刷新
   ========================================================================== */

async function api(method, path, body) {
  const opt = { method, headers: {}, credentials: 'same-origin' };
  if (body !== undefined) {
    opt.headers['Content-Type'] = 'application/json; charset=utf-8';
    opt.body = JSON.stringify(body);
  }
  const r = await fetch(path, opt);
  if (r.status === 401 && !path.endsWith('/login')) {
    location.replace('login.html');
    throw new Error('未登录');
  }
  const txt = await r.text();
  let data = null;
  try { data = txt ? JSON.parse(txt) : null; } catch (e) { data = txt; }
  if (!r.ok) throw new Error((data && data.msg) ? data.msg : ('HTTP ' + r.status));
  return data;
}
const GET = p => api('GET', p);
const POST = (p, b) => api('POST', p, b === undefined ? {} : b);
const PUT = (p, b) => api('PUT', p, b);
const DEL = p => api('DELETE', p);

const TRIGGER_LABEL = {
  'instance.start': '网关启动',
  'instance.stop': '网关停止',
  'account.login': '登录成功',
  'room.created': '房间创建成功',
  'room.changed': '房间号变化',
  'room.recreated': '房间重建',
  'player.join': '玩家加入',
  'player.leave': '玩家离开',
  'keepalive.fail': '存活检查失败',
};

const PLACEHOLDERS = [
  { ph: '{&roomid}', desc: '当前房间号' },
  { ph: '{&timestamp}', desc: 'Unix 时间戳（秒）' },
  { ph: '{&datetime}', desc: '本地时间，如 2026-10-10 03:20:00' },
  { ph: '{&account}', desc: '实例使用的 4399 账号' },
  { ph: '{&instancename}', desc: '实例名称' },
  { ph: '{&server}', desc: '转发目标 IP:端口' },
  { ph: '{&capacity}', desc: '房间最大人数' },
  { ph: '{&players}', desc: '当前在线玩家数' },
  { ph: '{&event}', desc: '触发事件，如 room.changed' },
];

const $ = sel => document.querySelector(sel);
function esc(s) {
  return String(s === undefined || s === null ? '' : s)
    .replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}
function toast(msg, kind = 'ok') {
  const el = document.createElement('div');
  el.className = 'toast ' + kind;
  el.textContent = msg;
  $('#toast-area').appendChild(el);
  setTimeout(() => el.remove(), 3200);
}
function openModal(id) { $('#' + id).classList.add('open'); }
function closeModal(id) { $('#' + id).classList.remove('open'); }
function onModalClosed(id) { if (id === 'logModal') stopLogStream(); }

document.querySelectorAll('.modal-mask').forEach(mask => {
  mask.addEventListener('click', e => { if (e.target === mask) { mask.classList.remove('open'); onModalClosed(mask.id); } });
  mask.querySelectorAll('[data-close]').forEach(b =>
    b.addEventListener('click', () => { closeModal(b.dataset.close); onModalClosed(b.dataset.close); }));
});
document.addEventListener('keydown', e => {
  if (e.key === 'Escape') document.querySelectorAll('.modal-mask.open').forEach(m => { m.classList.remove('open'); onModalClosed(m.id); });
});

let state = { accounts: [], instances: [], settings: { user: 'user', publicAccess: false } };
let editingAccountId = null;
let editingInstanceId = null;
let cmdCtx = { instanceId: null, cmdId: null };

/* ------------------------------ 账号 ------------------------------ */
function renderAccounts() {
  const body = $('#accountBody');
  if (!state.accounts.length) {
    body.innerHTML = `<tr><td colspan="5" class="empty">还没有账号，点下方「添加账号」新增一个</td></tr>`;
    return;
  }
  body.innerHTML = state.accounts.map(a => {
    const owner = state.instances.find(i => i.accountId === a.id);
    return `<tr>
      <td>${esc(a.note || '—')}</td>
      <td class="mono">${esc(a.account)}</td>
      <td class="mono muted">${esc(a.passwordMask || '••••••')}</td>
      <td>${owner ? esc(owner.name) : '<span class="muted">未使用</span>'}</td>
      <td class="actions">
        <button class="btn-link" data-acc-edit="${a.id}">编辑</button>
        <button class="btn-link danger" data-acc-del="${a.id}">删除</button>
      </td>
    </tr>`;
  }).join('');
}

/* ------------------------------ 实例 ------------------------------ */
function statusBadge(inst) {
  if (inst.status === 'running') return '<span class="badge badge-running">运行中</span>';
  if (inst.status === 'error') return '<span class="badge badge-error">异常退出</span>';
  return '<span class="badge badge-stopped">已停止</span>';
}

function renderInstances() {
  const box = $('#instanceList');
  $('#instanceCount').textContent = state.instances.length;
  if (!state.instances.length) {
    box.innerHTML = `<div class="card empty">还没有实例，点右上角「创建实例」新建一个</div>`;
    return;
  }
  box.innerHTML = state.instances.map(inst => {
    const running = inst.status === 'running';
    const cmds = inst.commands || [];
    const cmdRows = cmds.length ? cmds.map(c => `
      <div class="cmdrow">
        <select class="trigger" data-cmd-trigger="${inst.id}:${c.id}">
          ${Object.entries(TRIGGER_LABEL).map(([v, t]) =>
            `<option value="${v}" ${v === c.trigger ? 'selected' : ''}>${t}</option>`).join('')}
        </select>
        <span class="cmd ${c.enabled ? '' : 'off'}" title="${esc(c.line)}">${esc(c.line)}</span>
        <button class="btn-link" data-cmd-toggle="${inst.id}:${c.id}">${c.enabled ? '停用' : '启用'}</button>
        <button class="btn-link" data-cmd-edit="${inst.id}:${c.id}">编辑</button>
        <button class="btn-link danger" data-cmd-del="${inst.id}:${c.id}">删除</button>
      </div>`).join('') : `<div class="cmd-empty">暂无命令；点「创建命令行」添加，例如房间号变化时发通知</div>`;

    return `<div class="instance">
      <div class="instance-head">
        <span class="name">${esc(inst.name || '未命名实例')}</span>
        ${statusBadge(inst)}
        <div class="spacer"></div>
        <button class="btn btn-sm ${running ? '' : 'btn-primary'}" data-inst-toggle="${inst.id}">${running ? '停止' : '启动'}</button>
        <button class="btn btn-sm" data-inst-log="${inst.id}">打开网关日志</button>
        <button class="btn btn-sm" data-inst-edit="${inst.id}">编辑</button>
        <button class="btn btn-sm btn-danger" data-inst-del="${inst.id}">删除</button>
      </div>
      <div class="instance-body">
        <div class="kv">
          <div class="item"><span class="k">使用账号</span><span class="v mono">${esc(inst.account || '（账号已删除）')}</span></div>
          <div class="item"><span class="k">房间号</span><span class="v mono">${running && inst.roomId ? esc(inst.roomId) : '—'}</span></div>
          <div class="item"><span class="k">服务器</span><span class="v mono">${esc(inst.target)}</span></div>
          <div class="item"><span class="k">在线玩家</span><span class="v">${running ? inst.players : 0} / ${esc(inst.capacity)}</span></div>
          <div class="item"><span class="k">房间密码</span><span class="v">${inst.roomPassword ? esc(inst.roomPassword) : '<span class="muted">无</span>'}</span></div>
          <div class="item"><span class="k">命令数</span><span class="v">${cmds.length} 条（${cmds.filter(c => c.enabled).length} 条启用）</span></div>
        </div>
        ${inst.status === 'error' && inst.msg ? `<div class="alert alert-warn" style="margin:10px 0 0">${esc(inst.msg)}</div>` : ''}
        <div class="subhead">
          <span>命令设置</span>
          <div class="spacer"></div>
          <button class="btn btn-sm" data-cmd-add="${inst.id}">创建命令行</button>
        </div>
        <div class="cmdlist">${cmdRows}</div>
      </div>
    </div>`;
  }).join('');
}

/* ------------------------------ 设置 / 占位符 ------------------------------ */
function renderSettings() {
  $('#gsUser').value = state.settings.user || 'user';
  $('#gsPass').value = '';
  $('#gsPublic').checked = !!state.settings.publicAccess;
  // 端口跟随实际访问地址，避免换端口后显示错的
  const webPort = location.port || (location.protocol === 'https:' ? '443' : '80');
  $('#topMeta').textContent = state.settings.publicAccess
    ? `Web 控制台 0.0.0.0:${webPort} / [::]:${webPort}（局域网 / 公网可访问）`
    : `Web 控制台 127.0.0.1:${webPort}（仅本机）`;
}
function renderPlaceholders() {
  $('#phList').innerHTML = PLACEHOLDERS
    .map(p => `<span class="ph" data-ph="${esc(p.ph)}" title="${esc(p.desc)}">${esc(p.ph)}</span>`).join('');
}
function renderAccountOptions(selectedId, currentInstanceId) {
  const sel = $('#inAccount');
  if (!state.accounts.length) { sel.innerHTML = `<option value="">（请先在「账号管理」里添加账号）</option>`; return; }
  sel.innerHTML = state.accounts.map(a => {
    const owner = state.instances.find(i => i.accountId === a.id && i.id !== currentInstanceId);
    const mine = a.id === selectedId;
    const label = a.account + (a.note ? '（' + a.note + '）' : '') + (owner && !mine ? ` — 已被「${owner.name}」使用` : '');
    return `<option value="${a.id}" ${mine ? 'selected' : ''} ${owner && !mine ? 'disabled' : ''}>${esc(label)}</option>`;
  }).join('');
}
function firstFreeAccountId(excludeInstanceId) {
  const used = new Set(state.instances.filter(i => i.id !== excludeInstanceId).map(i => i.accountId));
  const free = state.accounts.find(a => !used.has(a.id));
  return free ? free.id : (state.accounts[0] ? state.accounts[0].id : '');
}

/* ------------------------------ 拉取 ------------------------------ */
async function refreshAll() {
  const [accounts, instances] = await Promise.all([GET('/api/accounts'), GET('/api/instances')]);
  state.accounts = accounts || [];
  state.instances = instances || [];
  renderAccounts();
  renderInstances();
}
async function refreshStatuses() {
  try { state.instances = (await GET('/api/instances')) || []; renderInstances(); } catch (e) {}
}

/* ------------------------------ 账号操作 ------------------------------ */
$('#btnAddAccount').addEventListener('click', () => {
  editingAccountId = null;
  $('#accountModalTitle').textContent = '添加账号';
  $('#acNote').value = ''; $('#acAccount').value = ''; $('#acPassword').value = '';
  openModal('accountModal');
});
$('#accountSave').addEventListener('click', async () => {
  const payload = { note: $('#acNote').value.trim(), account: $('#acAccount').value.trim(), password: $('#acPassword').value };
  if (!payload.account || !payload.password) { toast('账号和密码都要填', 'err'); return; }
  try {
    if (editingAccountId) await PUT('/api/accounts/' + editingAccountId, payload);
    else await POST('/api/accounts', payload);
    closeModal('accountModal');
    await refreshAll();
    toast(editingAccountId ? '账号已更新' : '账号已添加');
  } catch (e) { toast(e.message, 'err'); }
});
function editAccount(id) {
  const a = state.accounts.find(x => x.id === id);
  if (!a) return;
  editingAccountId = id;
  $('#accountModalTitle').textContent = '编辑账号';
  $('#acNote').value = a.note || '';
  $('#acAccount').value = a.account;
  $('#acPassword').value = ''; // 后端只回掩码，绝不回填明文；留空=不更改
  openModal('accountModal');
}
async function delAccount(id) {
  if (!confirm('确定删除这个账号？')) return;
  try { await DEL('/api/accounts/' + id); await refreshAll(); toast('账号已删除'); }
  catch (e) { toast(e.message, 'err'); }
}

/* ------------------------------ 实例操作 ------------------------------ */
$('#btnCreateInstance').addEventListener('click', () => {
  editingInstanceId = null;
  $('#instanceModalTitle').textContent = '创建实例';
  $('#inName').value = ''; $('#inTarget').value = ''; $('#inRoomPassword').value = ''; $('#inCapacity').value = 8;
  renderAccountOptions(firstFreeAccountId(null), null);
  openModal('instanceModal');
});
$('#instanceSave').addEventListener('click', async () => {
  const payload = {
    name: $('#inName').value.trim(),
    accountId: $('#inAccount').value,
    target: $('#inTarget').value.trim(),
    roomPassword: $('#inRoomPassword').value,
    capacity: Math.max(1, Math.min(100, parseInt($('#inCapacity').value, 10) || 8)),
  };
  if (!payload.accountId) { toast('请先在「账号管理」里添加一个账号', 'err'); return; }
  if (!/^.+:\d{1,5}$/.test(payload.target)) { toast('服务器地址要写成 IP:端口，例如 服务器IP:49780', 'err'); return; }
  try {
    if (editingInstanceId) await PUT('/api/instances/' + editingInstanceId, payload);
    else await POST('/api/instances', payload);
    closeModal('instanceModal');
    await refreshAll();
    toast(editingInstanceId ? '实例已更新' : '实例已创建，点「启动」开始开房');
  } catch (e) { toast(e.message, 'err'); }
});
function editInstance(id) {
  const inst = state.instances.find(i => i.id === id);
  if (!inst) return;
  editingInstanceId = id;
  $('#instanceModalTitle').textContent = '编辑实例';
  $('#inName').value = inst.name || '';
  $('#inTarget').value = inst.target || '';
  $('#inRoomPassword').value = inst.roomPassword || '';
  $('#inCapacity').value = inst.capacity || 8;
  renderAccountOptions(inst.accountId, id);
  openModal('instanceModal');
}
async function delInstance(id) {
  const inst = state.instances.find(i => i.id === id);
  if (!confirm(`确定删除实例「${inst ? inst.name : id}」？`)) return;
  try { await DEL('/api/instances/' + id); await refreshAll(); toast('实例已删除'); }
  catch (e) { toast(e.message, 'err'); }
}
async function toggleInstance(id) {
  const inst = state.instances.find(i => i.id === id);
  if (!inst) return;
  const start = inst.status !== 'running';
  try {
    await POST(`/api/instances/${id}/${start ? 'start' : 'stop'}`);
    await refreshAll();
    toast(start ? `实例「${inst.name}」已启动` : `实例「${inst.name}」已停止`);
  } catch (e) { toast(e.message, 'err'); await refreshAll(); }
}

/* ------------------------------ 命令行 ------------------------------ */
function openCmdModal(instanceId, cmdId) {
  cmdCtx = { instanceId, cmdId: cmdId || null };
  const inst = state.instances.find(i => i.id === instanceId);
  const c = cmdId && inst ? (inst.commands || []).find(x => x.id === cmdId) : null;
  $('#cmdModalTitle').textContent = c ? '编辑命令行' : '创建命令行';
  $('#cmdTrigger').value = c ? c.trigger : 'room.changed';
  $('#cmdLine').value = c ? c.line : '';
  $('#cmdNote').value = c ? (c.note || '') : '';
  $('#cmdEnabled').checked = c ? c.enabled !== false : true;
  openModal('cmdModal');
  setTimeout(() => $('#cmdLine').focus(), 60);
}
$('#cmdSave').addEventListener('click', async () => {
  const payload = {
    trigger: $('#cmdTrigger').value,
    line: $('#cmdLine').value.trim(),
    note: $('#cmdNote').value.trim(),
    enabled: $('#cmdEnabled').checked,
  };
  if (!payload.line) { toast('命令行不能为空', 'err'); return; }
  const { instanceId, cmdId } = cmdCtx;
  try {
    if (cmdId) await PUT(`/api/instances/${instanceId}/commands/${cmdId}`, payload);
    else await POST(`/api/instances/${instanceId}/commands`, payload);
    closeModal('cmdModal');
    await refreshAll();
    toast(cmdId ? '命令行已更新' : '命令行已创建');
  } catch (e) { toast(e.message, 'err'); }
});
async function toggleCmd(instanceId, cmdId) {
  const inst = state.instances.find(i => i.id === instanceId);
  const c = inst && (inst.commands || []).find(x => x.id === cmdId);
  if (!c) return;
  try {
    await PUT(`/api/instances/${instanceId}/commands/${cmdId}`, { trigger: c.trigger, line: c.line, note: c.note, enabled: !c.enabled });
    await refreshAll();
  } catch (e) { toast(e.message, 'err'); }
}
async function setCmdTrigger(instanceId, cmdId, trigger) {
  const inst = state.instances.find(i => i.id === instanceId);
  const c = inst && (inst.commands || []).find(x => x.id === cmdId);
  if (!c) return;
  try {
    await PUT(`/api/instances/${instanceId}/commands/${cmdId}`, { trigger, line: c.line, note: c.note, enabled: c.enabled });
    toast('触发条件已改为「' + TRIGGER_LABEL[trigger] + '」');
  } catch (e) { toast(e.message, 'err'); await refreshAll(); }
}
async function delCmd(instanceId, cmdId) {
  if (!confirm('确定删除这条命令？')) return;
  try { await DEL(`/api/instances/${instanceId}/commands/${cmdId}`); await refreshAll(); toast('命令行已删除'); }
  catch (e) { toast(e.message, 'err'); }
}
$('#phList').addEventListener('click', e => {
  const ph = e.target.closest('.ph');
  if (!ph) return;
  const ta = $('#cmdLine');
  const s = ta.selectionStart === undefined ? ta.value.length : ta.selectionStart;
  const en = ta.selectionEnd === undefined ? s : ta.selectionEnd;
  ta.value = ta.value.slice(0, s) + ph.dataset.ph + ta.value.slice(en);
  ta.focus();
  ta.selectionStart = ta.selectionEnd = s + ph.dataset.ph.length;
});

/* ------------------------------ 日志（SSE） ------------------------------ */
let logSource = null;
let logInstanceId = null;
let logAutoScroll = true;

function appendLogLine(l) {
  const box = $('#logView');
  const div = document.createElement('div');
  const lv = (l.level || 'INFO').toLowerCase();
  div.innerHTML = `<span class="l-time">${esc(l.time)}</span> <span class="l-${lv}">[${esc(l.level || 'INFO')}]</span> ${esc(l.msg)}`;
  box.appendChild(div);
  if (logAutoScroll) box.scrollTop = box.scrollHeight;
}
function stopLogStream() {
  if (logSource) { try { logSource.close(); } catch (e) {} logSource = null; }
}
async function openLog(id) {
  const inst = state.instances.find(i => i.id === id);
  $('#logTitle').textContent = `网关日志 · ${inst ? inst.name : id}`;
  $('#logView').innerHTML = '';
  openModal('logModal');
  stopLogStream();
  logInstanceId = id;
  try {
    const lines = await GET(`/api/instances/${id}/logs?tail=200`);
    (lines || []).forEach(appendLogLine);
  } catch (e) { toast(e.message, 'err'); }
  try {
    logSource = new EventSource(`/api/instances/${id}/logs?stream=1`);
    logSource.onmessage = ev => { try { appendLogLine(JSON.parse(ev.data)); } catch (e) {} };
  } catch (e) { toast('实时日志连接失败，可点「刷新」', 'err'); }
}
$('#logAutoscroll').addEventListener('change', e => { logAutoScroll = e.target.checked; });
$('#logRefresh').addEventListener('click', async () => {
  if (!logInstanceId) return;
  try {
    const lines = await GET(`/api/instances/${logInstanceId}/logs?tail=200`);
    $('#logView').innerHTML = '';
    (lines || []).forEach(appendLogLine);
  } catch (e) { toast(e.message, 'err'); }
});
$('#logClear').addEventListener('click', () => { $('#logView').innerHTML = ''; toast('已清空显示（不影响后端缓冲）'); });
$('#logCopy').addEventListener('click', async () => {
  try { await navigator.clipboard.writeText($('#logView').innerText); toast('日志已复制'); }
  catch (e) { toast('浏览器拒绝了剪贴板访问，请手动选择复制', 'err'); }
});
$('#logDownload').addEventListener('click', () => {
  const blob = new Blob([$('#logView').innerText], { type: 'text/plain;charset=utf-8' });
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = `gateway-${logInstanceId || 'instance'}.log`;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
});

/* ------------------------------ 设置 ------------------------------ */
$('#gsSave').addEventListener('click', async () => {
  const user = $('#gsUser').value.trim();
  const pass = $('#gsPass').value;
  if (!user) { toast('用户名不能为空', 'err'); return; }
  try {
    const r = await POST('/api/settings', { user, password: pass, publicAccess: $('#gsPublic').checked });
    state.settings.user = user;
    state.settings.publicAccess = $('#gsPublic').checked;
    renderSettings();
    toast(pass ? '已保存（用户名与密码均已更新）' : '已保存（密码保持不变）');
  } catch (e) { toast(e.message, 'err'); }
});
$('#gsReset').addEventListener('click', async () => {
  try { state.settings = await GET('/api/settings'); renderSettings(); toast('已还原为当前设置'); }
  catch (e) { toast(e.message, 'err'); }
});

/* ------------------------------ 事件委托 ------------------------------ */
$('#instanceList').addEventListener('click', e => {
  const el = e.target.closest('[data-inst-toggle],[data-inst-log],[data-inst-edit],[data-inst-del],' +
                               '[data-cmd-add],[data-cmd-edit],[data-cmd-del],[data-cmd-toggle]');
  if (!el) return;
  const d = el.dataset;
  if (d.instToggle) return toggleInstance(d.instToggle);
  if (d.instLog) return openLog(d.instLog);
  if (d.instEdit) return editInstance(d.instEdit);
  if (d.instDel) return delInstance(d.instDel);
  if (d.cmdAdd) return openCmdModal(d.cmdAdd, null);
  if (d.cmdEdit) { const p = d.cmdEdit.split(':'); return openCmdModal(p[0], p[1]); }
  if (d.cmdDel) { const p = d.cmdDel.split(':'); return delCmd(p[0], p[1]); }
  if (d.cmdToggle) { const p = d.cmdToggle.split(':'); return toggleCmd(p[0], p[1]); }
});
$('#instanceList').addEventListener('change', e => {
  const sel = e.target.closest('[data-cmd-trigger]');
  if (!sel) return;
  const p = sel.dataset.cmdTrigger.split(':');
  setCmdTrigger(p[0], p[1], sel.value);
});
$('#accountBody').addEventListener('click', e => {
  const el = e.target.closest('[data-acc-edit],[data-acc-del]');
  if (!el) return;
  if (el.dataset.accEdit) return editAccount(el.dataset.accEdit);
  if (el.dataset.accDel) return delAccount(el.dataset.accDel);
});
$('#btnLogout').addEventListener('click', async () => {
  try { await POST('/api/logout'); } catch (e) {}
  location.replace('login.html');
});

/* ------------------------------ 启动 ------------------------------ */
(async function boot() {
  try {
    const s = await GET('/api/session');
    if (!s || !s.loggedIn) { location.replace('login.html'); return; }
  } catch (e) { location.replace('login.html'); return; }

  renderPlaceholders();
  try {
    state.settings = await GET('/api/settings');
    await refreshAll();
    renderSettings();
  } catch (e) { toast('加载失败: ' + e.message, 'err'); }

  setInterval(refreshStatuses, 3000);
})();
