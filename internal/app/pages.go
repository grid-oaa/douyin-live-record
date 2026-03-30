package app

import "net/http"

const baseTemplate = `
{{define "base"}}
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Data.Title}} - 抖音录制服务</title>
  <link rel="stylesheet" href="/assets/app.css">
</head>
<body data-page="{{.Page}}">
  <div class="shell">
    {{if ne .Page "login"}}
    <aside class="sidebar">
      <h1>抖音录制服务</h1>
      <nav>
        <a class="{{if eq .Data.CurrentPath "/admin/config"}}active{{end}}" href="/admin/config">录制配置</a>
        <a class="{{if eq .Data.CurrentPath "/admin/status"}}active{{end}}" href="/admin/status">当前状态</a>
        <a class="{{if eq .Data.CurrentPath "/admin/history"}}active{{end}}" href="/admin/history">录制历史</a>
      </nav>
      <button id="logoutBtn" class="ghost">退出登录</button>
    </aside>
    {{end}}
    <main class="content">
      {{if eq .Page "login"}}
      {{template "login" .Data}}
      {{else if eq .Page "config"}}
      {{template "config" .Data}}
      {{else if eq .Page "status"}}
      {{template "status" .Data}}
      {{else if eq .Page "history"}}
      {{template "history" .Data}}
      {{end}}
    </main>
  </div>
  <script src="/assets/app.js"></script>
</body>
</html>
{{end}}
`

const loginTemplate = `
{{define "login"}}
<section class="panel narrow">
  <h2>登录</h2>
  <p class="muted">使用管理员账号进入录制配置界面。</p>
  <form id="loginForm" class="form">
    <label><span>账号</span><input name="username" type="text" required></label>
    <label><span>密码</span><input name="password" type="password" required></label>
    <button type="submit">登录</button>
    <p id="loginError" class="error"></p>
  </form>
</section>
{{end}}
`

const configTemplate = `
{{define "config"}}
<section class="header-row">
  <div>
    <h2>录制配置</h2>
    <p class="muted">“仅保存配置”只更新配置，不会立刻发起检测；“保存并立即检测”会启用自动录制并马上检查主播是否开播。当前正在录制时，直播地址、清晰度、分段时长和保存目录会在下一场直播生效。</p>
  </div>
  <div class="status-chip {{.Status.State}}">{{statusCategory .Status.State}} / {{.Status.Message}}</div>
</section>
<section class="panel">
  <form id="configForm" class="form grid">
    <label><span>主播名称</span><input name="streamer_name" type="text" value="{{.Config.StreamerName}}" placeholder="例如：某主播"></label>
    <label><span>抖音直播间 URL</span><input name="room_url" type="url" value="{{.Config.RoomURL}}" placeholder="https://live.douyin.com/..."></label>
    <label class="checkbox"><input name="auto_record_enabled" type="checkbox" {{if .Config.AutoRecordEnabled}}checked{{end}}><span>开启自动检查录制</span></label>
    <label><span>轮询间隔（秒）</span><input name="poll_interval_seconds" type="number" min="10" value="{{.Config.PollIntervalSeconds}}"></label>
    <label><span>录制清晰度</span><input name="stream_quality" type="text" value="{{.Config.StreamQuality}}"></label>
    <label><span>分段时长（分钟）</span><input name="segment_minutes" type="number" min="1" value="{{.Config.SegmentMinutes}}"></label>
    <label><span>保存子目录</span><input name="save_subdir" type="text" value="{{.Config.SaveSubdir}}" placeholder="/主播A"></label>
    <label><span>保留天数</span><input name="keep_days" type="number" min="1" value="{{.Config.KeepDays}}"></label>
    <label><span>最低剩余空间（GB）</span><input name="min_free_gb" type="number" min="1" value="{{.Config.MinFreeGB}}"></label>
    <label><span>清理后目标空间（GB）</span><input name="cleanup_to_gb" type="number" min="1" value="{{.Config.CleanupToGB}}"></label>
    <label><span>Cookies 文件（相对 cookies 根目录）</span><input name="cookies_file" type="text" value="{{.Config.CookiesFile}}" placeholder="douyin/cookies.txt"></label>
    <div class="actions">
      <button type="submit">仅保存配置</button>
      <button type="button" class="ghost" id="manualStartBtn">保存并立即检测</button>
      <button type="button" class="ghost danger" id="manualStopBtn">停止录制</button>
    </div>
    <p id="configMessage" class="success"></p>
  </form>
</section>
<section class="panel">
  <h3>字段生效说明</h3>
  <table>
    <thead><tr><th>字段</th><th>生效方式</th></tr></thead>
    <tbody>
      {{range .ApplyInfo}}
      <tr><td>{{.Field}}</td><td>{{.Mode}}</td></tr>
      {{end}}
    </tbody>
  </table>
</section>
{{end}}
`

const statusTemplate = `
{{define "status"}}
<section class="header-row">
  <div>
    <h2>当前状态</h2>
    <p class="muted">每 10 秒自动刷新。</p>
  </div>
  <button id="rebuildBtn" class="ghost">重试最近一次合并</button>
</section>
<section class="stats">
  <div class="panel stat"><h3>服务状态</h3><strong id="statusState">{{statusCategory .Status.State}}</strong><span id="statusMessage">{{.Status.Message}}</span></div>
  <div class="panel stat"><h3>最近检查</h3><strong id="lastCheck">{{fmtTime .Status.LastCheckAt}}</strong><span>轮询结果时间</span></div>
  <div class="panel stat"><h3>磁盘剩余</h3><strong id="diskFree">{{fmtBytesU .Status.DiskFreeBytes}}</strong><span>总容量 {{fmtBytesU .Status.DiskTotalBytes}}</span></div>
</section>
<section class="panel">
  <h3>当前配置摘要</h3>
  <dl class="summary">
    <dt>主播</dt><dd id="cfgStreamer">{{.Status.CurrentConfig.StreamerName}}</dd>
    <dt>直播间</dt><dd id="cfgRoom">{{.Status.CurrentConfig.RoomURL}}</dd>
    <dt>保存子目录</dt><dd id="cfgSubdir">{{.Status.CurrentConfig.SaveSubdir}}</dd>
    <dt>录制根目录</dt><dd>{{.Status.RecordingRoot}}</dd>
  </dl>
</section>
<section class="panel">
  <h3>运行事件</h3>
  <div id="eventList">
    {{range .Events}}
    <article class="event"><strong>{{.EventType}}</strong><span>{{fmtTimeValue .CreatedAt}}</span><p>{{.Message}}</p></article>
    {{else}}
    <p class="muted">暂无事件</p>
    {{end}}
  </div>
</section>
{{end}}
`

const historyTemplate = `
{{define "history"}}
<section class="header-row">
  <div>
    <h2>录制历史</h2>
    <p class="muted">展示最近 50 条录制会话。</p>
  </div>
</section>
<section class="panel">
  <table>
    <thead>
      <tr>
        <th>ID</th>
        <th>主播</th>
        <th>开始时间</th>
        <th>结束时间</th>
        <th>状态</th>
        <th>文件大小</th>
        <th>MP4 路径</th>
        <th>失败原因</th>
      </tr>
    </thead>
    <tbody>
      {{range .History}}
      <tr>
        <td>{{.ID}}</td>
        <td>{{.StreamerName}}</td>
        <td>{{fmtTimeValue .StartedAt}}</td>
        <td>{{fmtTime .EndedAt}}</td>
        <td>{{.Status}}</td>
        <td>{{fmtBytes .FileSizeBytes}}</td>
        <td class="path">{{.FinalFilePath}}</td>
        <td>{{.ErrorMessage}}</td>
      </tr>
      {{else}}
      <tr><td colspan="8" class="muted">暂无录制历史</td></tr>
      {{end}}
    </tbody>
  </table>
</section>
{{end}}
`

const cssContent = `
:root {
  --bg: #f4efe6;
  --panel: #fffaf3;
  --line: #d8ccb9;
  --text: #2d241b;
  --muted: #6f6459;
  --accent: #af4b2a;
  --accent-dark: #7e3118;
  --danger: #b42318;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  font-family: "Segoe UI", "PingFang SC", sans-serif;
  color: var(--text);
  background:
    radial-gradient(circle at top right, rgba(175,75,42,.18), transparent 30%),
    linear-gradient(180deg, #f7f1e8 0%, #f0e5d7 100%);
}
.shell { display: flex; min-height: 100vh; }
.sidebar {
  width: 260px;
  padding: 32px 24px;
  border-right: 1px solid var(--line);
  background: rgba(255,250,243,.86);
  backdrop-filter: blur(10px);
}
.sidebar h1 { margin-top: 0; font-size: 24px; }
.sidebar nav { display: grid; gap: 10px; margin: 24px 0; }
.sidebar a {
  text-decoration: none;
  color: var(--text);
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid transparent;
}
.sidebar a.active, .sidebar a:hover {
  border-color: var(--line);
  background: white;
}
.content { flex: 1; padding: 32px; }
.panel {
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: 20px;
  padding: 24px;
  box-shadow: 0 16px 42px rgba(59, 39, 25, 0.07);
  margin-bottom: 20px;
}
.panel.narrow { max-width: 420px; margin: 80px auto; }
.form { display: grid; gap: 16px; }
.form.grid { grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); align-items: start; }
.form label { display: grid; gap: 8px; font-size: 14px; color: var(--muted); }
.form input {
  width: 100%;
  padding: 12px 14px;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: white;
  color: var(--text);
}
.checkbox { display: flex !important; align-items: center; gap: 10px; margin-top: 28px; }
.checkbox input { width: auto; }
button {
  appearance: none;
  border: 0;
  border-radius: 999px;
  padding: 12px 18px;
  background: var(--accent);
  color: white;
  cursor: pointer;
}
button:hover { background: var(--accent-dark); }
button.ghost {
  background: transparent;
  color: var(--text);
  border: 1px solid var(--line);
}
button.danger { color: var(--danger); }
.header-row { display: flex; justify-content: space-between; align-items: start; gap: 16px; margin-bottom: 20px; }
.muted { color: var(--muted); }
.error { color: var(--danger); min-height: 20px; }
.success { color: var(--accent-dark); min-height: 20px; }
.status-chip {
  border-radius: 999px;
  padding: 10px 14px;
  background: white;
  border: 1px solid var(--line);
}
.status-chip.recording { color: #0b7a3b; }
.status-chip.error { color: var(--danger); }
.status-chip.disabled { color: var(--muted); }
.actions { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; }
.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
  margin-bottom: 20px;
}
.stat strong { display: block; font-size: 28px; margin-bottom: 8px; }
.summary {
  display: grid;
  grid-template-columns: 140px 1fr;
  gap: 12px 16px;
}
.summary dt { color: var(--muted); }
table { width: 100%; border-collapse: collapse; }
th, td { text-align: left; padding: 12px; border-bottom: 1px solid var(--line); vertical-align: top; }
.path { word-break: break-all; }
.event { padding: 12px 0; border-bottom: 1px solid var(--line); }
.event strong { display: block; margin-bottom: 4px; }
@media (max-width: 900px) {
  .shell { flex-direction: column; }
  .sidebar { width: auto; border-right: 0; border-bottom: 1px solid var(--line); }
  .content { padding: 20px; }
}
`

const jsContent = `
async function api(path, options = {}) {
  const response = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    credentials: 'same-origin',
    ...options
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(data.error || '请求失败');
  }
  return data;
}

function statusCategory(state) {
  if (state === 'recording' || state === 'stopping' || state === 'merging') {
    return '正在录制';
  }
  if (state === 'error') {
    return '探测异常';
  }
  return '未开播';
}

function bindLogout() {
  const btn = document.getElementById('logoutBtn');
  if (!btn) return;
  btn.addEventListener('click', async () => {
    await api('/api/logout', { method: 'POST' });
    window.location.href = '/login';
  });
}

function bindLoginPage() {
  const form = document.getElementById('loginForm');
  if (!form) return;
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    const errorNode = document.getElementById('loginError');
    errorNode.textContent = '';
    const payload = Object.fromEntries(new FormData(form).entries());
    try {
      await api('/api/login', { method: 'POST', body: JSON.stringify(payload) });
      window.location.href = '/admin/status';
    } catch (error) {
      errorNode.textContent = error.message;
    }
  });
}

function bindConfigPage() {
  const form = document.getElementById('configForm');
  if (!form) return;
  const messageNode = document.getElementById('configMessage');
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    messageNode.textContent = '';
    const raw = Object.fromEntries(new FormData(form).entries());
    const payload = {
      ...raw,
      auto_record_enabled: form.querySelector('[name=auto_record_enabled]').checked,
      poll_interval_seconds: Number(raw.poll_interval_seconds),
      segment_minutes: Number(raw.segment_minutes),
      keep_days: Number(raw.keep_days),
      min_free_gb: Number(raw.min_free_gb),
      cleanup_to_gb: Number(raw.cleanup_to_gb)
    };
    try {
      await api('/api/config', { method: 'PUT', body: JSON.stringify(payload) });
      messageNode.textContent = '配置已保存，系统会按轮询间隔执行下一次检查';
    } catch (error) {
      messageNode.textContent = error.message;
    }
  });

  document.getElementById('manualStartBtn')?.addEventListener('click', async () => {
    try {
      const raw = Object.fromEntries(new FormData(form).entries());
      const payload = {
        ...raw,
        auto_record_enabled: true,
        poll_interval_seconds: Number(raw.poll_interval_seconds),
        segment_minutes: Number(raw.segment_minutes),
        keep_days: Number(raw.keep_days),
        min_free_gb: Number(raw.min_free_gb),
        cleanup_to_gb: Number(raw.cleanup_to_gb)
      };
      await api('/api/config', { method: 'PUT', body: JSON.stringify(payload) });
      await api('/api/recording/start', { method: 'POST' });
      form.querySelector('[name=auto_record_enabled]').checked = true;
      messageNode.textContent = '配置已保存，已启用自动录制，并立即触发一次开播检测';
    } catch (error) {
      messageNode.textContent = error.message;
    }
  });
  document.getElementById('manualStopBtn')?.addEventListener('click', async () => {
    try {
      await api('/api/recording/stop', { method: 'POST' });
      messageNode.textContent = '已停止自动录制';
    } catch (error) {
      messageNode.textContent = error.message;
    }
  });
}

function bindStatusPage() {
  const rebuildBtn = document.getElementById('rebuildBtn');
  if (rebuildBtn) {
    rebuildBtn.addEventListener('click', async () => {
      try {
        await api('/api/recording/rebuild-latest', { method: 'POST' });
        alert('已触发最近一次录制重试合并');
      } catch (error) {
        alert(error.message);
      }
    });
  }
  if (!document.getElementById('statusState')) return;
  const refresh = async () => {
    try {
      const data = await api('/api/status');
      const status = data.status;
      document.getElementById('statusState').textContent = statusCategory(status.state);
      document.getElementById('statusMessage').textContent = status.message;
      document.getElementById('lastCheck').textContent = status.last_check_at ? new Date(status.last_check_at).toLocaleString() : '-';
      document.getElementById('diskFree').textContent = (status.disk_free_bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB';
      document.getElementById('cfgStreamer').textContent = status.current_config.streamer_name || '-';
      document.getElementById('cfgRoom').textContent = status.current_config.room_url || '-';
      document.getElementById('cfgSubdir').textContent = status.current_config.save_subdir || '-';
      const eventList = document.getElementById('eventList');
      eventList.innerHTML = (data.events || []).map(event => '<article class="event"><strong>' + event.event_type + '</strong><span>' + new Date(event.created_at).toLocaleString() + '</span><p>' + event.message + '</p></article>').join('') || '<p class="muted">暂无事件</p>';
    } catch (error) {
      console.error(error);
    }
  };
  refresh();
  setInterval(refresh, 10000);
}

bindLogout();
bindLoginPage();
bindConfigPage();
bindStatusPage();
`

func (a *App) handleCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write([]byte(cssContent))
}

func (a *App) handleJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(jsContent))
}
