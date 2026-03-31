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
  <link rel="stylesheet" href="{{appPath "/assets/app.css"}}">
</head>
<body data-page="{{.Page}}" data-base-path="{{basePath}}">
  <div class="site-bg"></div>
  <div class="site-glow site-glow-a"></div>
  <div class="site-glow site-glow-b"></div>
  {{if eq .Page "login"}}
  <main class="login-shell">
    {{template "login" .Data}}
  </main>
  {{else}}
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand-block">
        <span class="brand-kicker">Recorder Console</span>
        <h1>抖音直播录制台</h1>
        <p>围绕单主播录制场景设计，重点是稳定检测、清晰状态和低维护成本。</p>
      </div>
      <nav class="nav-links">
        <a class="{{if eq .Data.CurrentPath (appPath "/admin/config")}}active{{end}}" href="{{appPath "/admin/config"}}">
          <span>录制配置</span>
          <small>管理主播、轮询和保存策略</small>
        </a>
        <a class="{{if eq .Data.CurrentPath (appPath "/admin/status")}}active{{end}}" href="{{appPath "/admin/status"}}">
          <span>当前状态</span>
          <small>查看检测结果、事件与磁盘信息</small>
        </a>
        <a class="{{if eq .Data.CurrentPath (appPath "/admin/history")}}active{{end}}" href="{{appPath "/admin/history"}}">
          <span>录制历史</span>
          <small>确认录制产物、失败原因和完成状态</small>
        </a>
      </nav>
      <div class="sidebar-footer">
        <div class="sidebar-note">
          <strong>运行原则</strong>
          <p>未开播保持等待，开播后分段落盘，下播后再合并为 MP4。</p>
        </div>
        <button id="logoutBtn" class="secondary wide">退出登录</button>
      </div>
    </aside>
    <main class="content-shell">
      <header class="page-topbar">
        <div>
          <span class="page-eyebrow">Control Panel</span>
          <h2>{{.Data.Title}}</h2>
        </div>
        <div class="topbar-meta">
          <span>单机部署</span>
          <span>SQLite</span>
          <span>Streamlink + FFmpeg</span>
        </div>
      </header>
      <div class="page-body">
        {{if eq .Page "config"}}
        {{template "config" .Data}}
        {{else if eq .Page "status"}}
        {{template "status" .Data}}
        {{else if eq .Page "history"}}
        {{template "history" .Data}}
        {{end}}
      </div>
    </main>
  </div>
  {{end}}
  <script src="{{appPath "/assets/app.js"}}"></script>
</body>
</html>
{{end}}
`

const loginTemplate = `
{{define "login"}}
<section class="login-panel">
  <div class="login-copy">
    <span class="page-eyebrow">Private Recorder</span>
    <h2>登录录制控制台</h2>
    <p>这里用于管理单主播自动录制任务。配置会实时写入数据库，保存后无需重启服务。</p>
    <div class="login-points">
      <article>
        <strong>自动检查</strong>
        <span>按轮询策略探测主播是否开播</span>
      </article>
      <article>
        <strong>分段录制</strong>
        <span>直播中持续写入 TS 分段，降低单文件损坏风险</span>
      </article>
      <article>
        <strong>结束合并</strong>
        <span>下播后自动合并为 MP4，便于后续回看</span>
      </article>
    </div>
  </div>
  <div class="panel login-card">
    <div class="panel-heading compact">
      <div>
        <span class="panel-kicker">Admin Access</span>
        <h3>管理员登录</h3>
      </div>
    </div>
    <p class="muted">使用初始化管理员账号进入配置页面。</p>
    <form id="loginForm" class="form">
      <label><span>账号</span><input name="username" type="text" required></label>
      <label><span>密码</span><input name="password" type="password" required></label>
      <button type="submit" class="primary wide">登录</button>
      <p id="loginError" class="feedback error"></p>
    </form>
  </div>
</section>
{{end}}
`

const configTemplate = `
{{define "config"}}
<section class="hero-card hero-card-config">
  <div>
    <span class="page-eyebrow">Recording Profile</span>
    <h3>录制规则配置</h3>
    <p class="muted strong-copy">“仅保存配置”只更新数据库和运行时配置，不会立刻发起检测；“保存并立即检测”会启用自动录制并马上触发一次开播检查。当前正在录制时，直播地址、清晰度、分段时长和保存目录会在下一场直播生效。</p>
  </div>
  <div class="hero-side">
    <span class="status-chip {{.Status.State}}">{{statusCategory .Status.State}}</span>
    <p>{{.Status.Message}}</p>
  </div>
</section>

<section class="content-grid two-columns">
  <div class="panel">
    <div class="panel-heading">
      <div>
        <span class="panel-kicker">Core Settings</span>
        <h3>基础配置</h3>
      </div>
    </div>
    <form id="configForm" class="form grid-form">
      <label>
        <span>主播名称</span>
        <input name="streamer_name" type="text" value="{{.Config.StreamerName}}" placeholder="例如：某主播">
        <small class="field-help">仅用于页面展示和录制历史标识，不参与抖音直播间探测。</small>
      </label>
      <label>
        <span>抖音直播间 URL</span>
        <input name="room_url" type="url" value="{{.Config.RoomURL}}" placeholder="https://live.douyin.com/...">
        <small class="field-help">建议填写精简后的直播间地址，例如 https://live.douyin.com/123456，不必保留分享链接中的 query 参数。</small>
      </label>
      <label class="checkbox card-checkbox">
        <input name="auto_record_enabled" type="checkbox" {{if .Config.AutoRecordEnabled}}checked{{end}}>
        <span>开启自动检查录制</span>
        <small class="field-help">勾选后会按轮询间隔持续检查主播是否开播；不勾选时只保存配置，不会持续自动检查。</small>
      </label>
      <label>
        <span>轮询间隔（秒）</span>
        <input name="poll_interval_seconds" type="number" min="10" value="{{.Config.PollIntervalSeconds}}">
        <small class="field-help">表示服务每隔多少秒检查一次直播间是否开播。间隔越短，发现开播越快，但探测请求也会更多。</small>
      </label>
      <label>
        <span>录制清晰度</span>
        <input name="stream_quality" type="text" value="{{.Config.StreamQuality}}">
        <small class="field-help">该值会直接传给 Streamlink。建议填写 best；常见可填值有 best、worst，部分直播间还可能支持 1080p、720p 等具体流名称，实际以可探测到的流为准。</small>
      </label>
      <label>
        <span>分段时长（分钟）</span>
        <input name="segment_minutes" type="number" min="1" value="{{.Config.SegmentMinutes}}">
        <small class="field-help">表示录制过程中每个 TS 分段文件的时长。比如填 15，直播中会每 15 分钟切出一个分段，直播结束后再把这些分段合并成最终 MP4。</small>
      </label>
      <label>
        <span>保存子目录</span>
        <input name="save_subdir" type="text" value="{{.Config.SaveSubdir}}" placeholder="/主播A">
        <small class="field-help">这是录制根目录下的业务子目录，不是宿主机任意绝对路径。最终文件会保存在“录制根目录 / 保存子目录 / 日期 / session-x”下。</small>
      </label>
      <label>
        <span>保留天数</span>
        <input name="keep_days" type="number" min="1" value="{{.Config.KeepDays}}">
        <small class="field-help">指“已完成录制会话的最终 MP4 文件”最多保留多少天。超过这个天数的旧 MP4 会被自动删除；正在录制的分段文件不按这个规则清理。</small>
      </label>
      <label>
        <span>最低剩余空间（GB）</span>
        <input name="min_free_gb" type="number" min="1" value="{{.Config.MinFreeGB}}">
        <small class="field-help">表示录制根目录所在磁盘允许的最低剩余空间阈值。剩余空间低于这个值时，系统会开始删除旧的最终 MP4 文件。</small>
      </label>
      <label>
        <span>清理后目标空间（GB）</span>
        <input name="cleanup_to_gb" type="number" min="1" value="{{.Config.CleanupToGB}}">
        <small class="field-help">当剩余空间低于“最低剩余空间”后，系统不会只删一个文件就停，而是持续删除旧 MP4，直到磁盘剩余空间回升到这个目标值或更高。</small>
      </label>
      <label class="full">
        <span>Cookies 文件（相对 cookies 根目录）</span>
        <input name="cookies_file" type="text" value="{{.Config.CookiesFile}}" placeholder="douyin/cookies.txt">
        <small class="field-help">填写 cookies 根目录下的相对路径，例如 douyin/cookies.txt。当匿名访问拿不到可播放流时，可通过这里挂接浏览器导出的登录态。</small>
      </label>
      <div class="actions full">
        <button type="submit" class="primary">仅保存配置</button>
        <button type="button" class="secondary" id="manualStartBtn">保存并立即检测</button>
        <button type="button" class="secondary danger" id="manualStopBtn">停止录制</button>
      </div>
      <p id="configMessage" class="feedback success full"></p>
    </form>
  </div>

  <div class="stack-panel">
    <section class="panel info-panel dark-panel">
      <div class="panel-heading compact">
        <div>
          <span class="panel-kicker">Operational Notes</span>
          <h3>使用提醒</h3>
        </div>
      </div>
      <ul class="note-list">
        <li>推荐使用去掉 query 参数的直播间地址，例如 https://live.douyin.com/123456。</li>
        <li>保存目录只填录制根目录下的业务子目录，避免跨目录挂载问题。</li>
        <li>如果匿名访问拿不到流，可以在 cookies 根目录放入 cookies.txt 后再配置相对路径。</li>
      </ul>
    </section>
  </div>
</section>
{{end}}
`

const statusTemplate = `
{{define "status"}}
<section class="hero-card hero-card-status">
  <div>
    <span class="page-eyebrow">Live Runtime</span>
    <h3>当前运行状态</h3>
    <p class="muted strong-copy">页面每 10 秒自动刷新一次，用于确认是否在等待开播、正在录制或出现探测异常。</p>
  </div>
  <div class="hero-side actions-right">
    <span id="statusState" class="status-chip {{.Status.State}}">{{statusCategory .Status.State}}</span>
    <button id="rebuildBtn" class="secondary">重试最近一次合并</button>
  </div>
</section>

<section class="stats-grid">
  <article class="stat-card emphasis">
    <span>运行消息</span>
    <strong id="statusMessage">{{.Status.Message}}</strong>
    <small>当前录制状态说明</small>
  </article>
  <article class="stat-card">
    <span>最近检查</span>
    <strong id="lastCheck">{{fmtTime .Status.LastCheckAt}}</strong>
    <small>轮询结果时间</small>
  </article>
  <article class="stat-card">
    <span>磁盘剩余</span>
    <strong id="diskFree">{{fmtBytesU .Status.DiskFreeBytes}}</strong>
    <small>总容量 {{fmtBytesU .Status.DiskTotalBytes}}</small>
  </article>
</section>

<section class="content-grid two-columns status-layout">
  <div class="panel">
    <div class="panel-heading compact">
      <div>
        <span class="panel-kicker">Config Snapshot</span>
        <h3>当前配置摘要</h3>
      </div>
    </div>
    <dl class="summary-grid">
      <div><dt>主播</dt><dd id="cfgStreamer">{{.Status.CurrentConfig.StreamerName}}</dd></div>
      <div><dt>直播间</dt><dd id="cfgRoom">{{.Status.CurrentConfig.RoomURL}}</dd></div>
      <div><dt>保存子目录</dt><dd id="cfgSubdir">{{.Status.CurrentConfig.SaveSubdir}}</dd></div>
      <div><dt>录制根目录</dt><dd>{{.Status.RecordingRoot}}</dd></div>
    </dl>
  </div>

  <div class="panel">
    <div class="panel-heading compact">
      <div>
        <span class="panel-kicker">Event Feed</span>
        <h3>运行事件</h3>
      </div>
    </div>
    <div id="eventList" class="event-list">
      {{range .Events}}
      <article class="event">
        <div class="event-head">
          <strong>{{.EventType}}</strong>
          <span>{{fmtTimeValue .CreatedAt}}</span>
        </div>
        <p>{{.Message}}</p>
      </article>
      {{else}}
      <p class="muted">暂无事件</p>
      {{end}}
    </div>
  </div>
</section>
{{end}}
`

const historyTemplate = `
{{define "history"}}
<section class="hero-card hero-card-history">
  <div>
    <span class="page-eyebrow">Session Archive</span>
    <h3>录制历史</h3>
    <p class="muted strong-copy">展示最近 50 条录制会话，重点关注完成状态、最终 MP4 路径和失败原因。</p>
  </div>
  <div class="hero-side history-side">
    <span class="metric-label">最近会话数</span>
    <strong>{{len .History}}</strong>
  </div>
</section>

<section class="panel table-panel">
  <div class="table-wrap">
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
          <td data-label="ID">{{.ID}}</td>
          <td data-label="主播">{{.StreamerName}}</td>
          <td data-label="开始时间">{{fmtTimeValue .StartedAt}}</td>
          <td data-label="结束时间">{{fmtTime .EndedAt}}</td>
          <td data-label="状态"><span class="table-status {{.Status}}">{{.Status}}</span></td>
          <td data-label="文件大小">{{fmtBytes .FileSizeBytes}}</td>
          <td data-label="MP4 路径" class="path">{{.FinalFilePath}}</td>
          <td data-label="失败原因">{{.ErrorMessage}}</td>
        </tr>
        {{else}}
        <tr><td colspan="8" class="muted empty-cell">暂无录制历史</td></tr>
        {{end}}
      </tbody>
    </table>
  </div>
</section>
{{end}}
`

const cssContent = `
:root {
  --bg: #07111f;
  --bg-soft: #0d1827;
  --surface: rgba(9, 18, 31, 0.78);
  --surface-strong: rgba(10, 20, 35, 0.94);
  --surface-alt: rgba(255, 255, 255, 0.06);
  --line: rgba(255, 255, 255, 0.11);
  --line-strong: rgba(255, 255, 255, 0.18);
  --text: #f5f7fb;
  --muted: #98a8be;
  --accent: #ff7a18;
  --accent-strong: #ff9b3d;
  --accent-cool: #49c6e5;
  --success: #39d98a;
  --danger: #ff6b6b;
  --shadow: 0 24px 80px rgba(0, 0, 0, 0.35);
}

* {
  box-sizing: border-box;
}

html {
  min-height: 100%;
}

body {
  margin: 0;
  min-height: 100vh;
  position: relative;
  color: var(--text);
  font-family: "IBM Plex Sans", "PingFang SC", "Microsoft YaHei", sans-serif;
  background:
    radial-gradient(circle at 12% 18%, rgba(73, 198, 229, 0.12), transparent 24%),
    radial-gradient(circle at 86% 14%, rgba(255, 122, 24, 0.18), transparent 22%),
    linear-gradient(180deg, #08111e 0%, #050b14 100%);
}

body::before {
  content: "";
  position: fixed;
  inset: 0;
  pointer-events: none;
  background-image: linear-gradient(rgba(255,255,255,0.02) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.02) 1px, transparent 1px);
  background-size: 28px 28px;
  mask-image: linear-gradient(180deg, rgba(0,0,0,0.9), rgba(0,0,0,0.2));
}

.site-bg,
.site-glow {
  position: fixed;
  pointer-events: none;
  z-index: 0;
}

.site-bg {
  inset: 24px;
  border: 1px solid rgba(255,255,255,0.04);
  border-radius: 32px;
}

.site-glow {
  width: 32vw;
  height: 32vw;
  filter: blur(40px);
  opacity: 0.55;
}

.site-glow-a {
  top: 10vh;
  left: -8vw;
  background: radial-gradient(circle, rgba(255, 122, 24, 0.22), transparent 70%);
}

.site-glow-b {
  right: -10vw;
  bottom: 6vh;
  background: radial-gradient(circle, rgba(73, 198, 229, 0.2), transparent 70%);
}

body > * {
  position: relative;
  z-index: 1;
}

.login-shell {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 40px 24px;
}

.login-panel {
  width: min(1120px, 100%);
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(340px, 420px);
  gap: 28px;
  align-items: stretch;
}

.login-copy {
  padding: 40px 12px 40px 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.login-copy h2,
.page-topbar h2,
.hero-card h3,
.panel-heading h3,
.login-card h3 {
  margin: 10px 0 12px;
  font-family: "Avenir Next", "IBM Plex Sans", "PingFang SC", sans-serif;
  letter-spacing: 0.01em;
}

.login-copy h2 {
  font-size: clamp(2.1rem, 4vw, 4.4rem);
  line-height: 1;
  max-width: 12ch;
}

.login-copy p,
.muted {
  color: var(--muted);
}

.strong-copy {
  max-width: 70ch;
  line-height: 1.7;
}

.login-points {
  display: grid;
  gap: 14px;
  margin-top: 28px;
  max-width: 560px;
}

.login-points article,
.sidebar-note,
.stat-card,
.apply-list article,
.note-list li,
.event,
.table-status,
.status-chip,
.card-checkbox {
  border: 1px solid var(--line);
}

.login-points article {
  display: grid;
  gap: 6px;
  padding: 18px 20px;
  border-radius: 20px;
  background: rgba(255,255,255,0.04);
  box-shadow: inset 0 1px 0 rgba(255,255,255,0.04);
}

.login-points strong {
  font-size: 1rem;
}

.app-shell {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 310px minmax(0, 1fr);
}

.sidebar {
  padding: 28px;
  border-right: 1px solid rgba(255,255,255,0.08);
  background: linear-gradient(180deg, rgba(8, 17, 30, 0.92), rgba(7, 14, 25, 0.78));
  backdrop-filter: blur(18px);
  display: flex;
  flex-direction: column;
  gap: 28px;
}

.brand-block h1 {
  margin: 10px 0 12px;
  font-size: 2rem;
  line-height: 1.02;
}

.brand-block p,
.sidebar-note p,
.nav-links small,
.stat-card small,
.metric-label {
  color: var(--muted);
}

.brand-kicker,
.page-eyebrow,
.panel-kicker {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 0.74rem;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--accent-cool);
}

.nav-links {
  display: grid;
  gap: 12px;
}

.nav-links a {
  color: inherit;
  text-decoration: none;
  padding: 18px 18px 16px;
  border-radius: 22px;
  background: rgba(255,255,255,0.03);
  border: 1px solid transparent;
  transition: transform 0.2s ease, border-color 0.2s ease, background 0.2s ease;
}

.nav-links a span {
  display: block;
  margin-bottom: 6px;
  font-size: 1rem;
  font-weight: 700;
}

.nav-links a:hover,
.nav-links a.active {
  transform: translateY(-2px);
  background: linear-gradient(180deg, rgba(255,255,255,0.08), rgba(255,255,255,0.04));
  border-color: rgba(255, 122, 24, 0.36);
}

.sidebar-footer {
  margin-top: auto;
  display: grid;
  gap: 14px;
}

.sidebar-note {
  padding: 18px;
  border-radius: 20px;
  background: rgba(255,255,255,0.04);
}

.content-shell {
  padding: 32px;
}

.page-topbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 18px;
  margin-bottom: 26px;
}

.page-topbar h2 {
  font-size: clamp(1.8rem, 2.4vw, 2.7rem);
}

.topbar-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: flex-end;
}

.topbar-meta span {
  padding: 10px 14px;
  border-radius: 999px;
  border: 1px solid var(--line);
  background: rgba(255,255,255,0.04);
  color: var(--muted);
}

.page-body,
.stack-panel {
  display: grid;
  gap: 20px;
}

.hero-card,
.panel,
.login-card {
  border-radius: 28px;
  border: 1px solid var(--line);
  background: linear-gradient(180deg, rgba(12, 22, 37, 0.92), rgba(8, 15, 26, 0.9));
  box-shadow: var(--shadow);
}

.hero-card,
.panel {
  padding: 26px;
}

.login-card {
  padding: 30px;
}

.hero-card {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 20px;
  overflow: hidden;
}

.hero-card::after {
  content: "";
  position: absolute;
  inset: auto -60px -80px auto;
  width: 220px;
  height: 220px;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(255,122,24,0.22), transparent 68%);
  pointer-events: none;
}

.hero-card-config::before,
.hero-card-status::before,
.hero-card-history::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(120deg, rgba(73,198,229,0.08), transparent 35%, rgba(255,122,24,0.06) 80%);
}

.hero-side {
  min-width: 220px;
  display: grid;
  gap: 14px;
  justify-items: end;
  text-align: right;
}

.actions-right {
  align-items: end;
}

.history-side strong {
  font-size: 3rem;
  line-height: 1;
}

.content-grid.two-columns {
  display: grid;
  grid-template-columns: minmax(0, 1.45fr) minmax(300px, 0.9fr);
  gap: 20px;
}

.panel-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 14px;
  margin-bottom: 18px;
}

.panel-heading.compact {
  margin-bottom: 14px;
}

.panel-heading h3,
.hero-card h3,
.login-card h3 {
  font-size: 1.45rem;
}

.form {
  display: grid;
  gap: 16px;
}

.grid-form {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  align-items: start;
}

label {
  display: grid;
  gap: 10px;
}

label span {
  font-size: 0.92rem;
  color: #c6d3e2;
}

.field-help {
  display: block;
  margin-top: -2px;
  font-size: 0.82rem;
  line-height: 1.6;
  color: var(--muted);
}

input {
  width: 100%;
  appearance: none;
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 18px;
  padding: 14px 16px;
  background: rgba(255,255,255,0.05);
  color: var(--text);
  outline: none;
  transition: border-color 0.2s ease, transform 0.2s ease, background 0.2s ease;
}

input::placeholder {
  color: rgba(152, 168, 190, 0.72);
}

input:focus {
  border-color: rgba(73, 198, 229, 0.65);
  background: rgba(255,255,255,0.08);
  transform: translateY(-1px);
}

.full {
  grid-column: 1 / -1;
}

.card-checkbox {
  grid-column: span 1;
  min-height: 100%;
  padding: 18px;
  border-radius: 22px;
  background: rgba(255,255,255,0.04);
}

.checkbox {
  display: flex;
  align-items: center;
  gap: 12px;
}

.checkbox input {
  appearance: auto;
  -webkit-appearance: checkbox;
  width: 18px;
  height: 18px;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  accent-color: var(--accent);
  cursor: pointer;
  flex: 0 0 auto;
}

.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 8px;
}

button {
  appearance: none;
  border: 0;
  border-radius: 999px;
  padding: 13px 20px;
  font: inherit;
  font-weight: 700;
  cursor: pointer;
  transition: transform 0.2s ease, box-shadow 0.2s ease, background 0.2s ease, border-color 0.2s ease, color 0.2s ease;
}

button:hover {
  transform: translateY(-1px);
}

button.primary {
  color: #111;
  background: linear-gradient(135deg, var(--accent), var(--accent-strong));
  box-shadow: 0 14px 30px rgba(255,122,24,0.22);
}

button.secondary {
  color: var(--text);
  background: rgba(255,255,255,0.04);
  border: 1px solid rgba(255,255,255,0.12);
}

button.secondary:hover {
  border-color: rgba(73, 198, 229, 0.42);
  background: rgba(255,255,255,0.08);
}

button.danger {
  color: #ffb4b4;
}

button.wide {
  width: 100%;
}

.feedback {
  min-height: 24px;
  margin: 0;
}

.feedback.error {
  color: #ffb4b4;
}

.feedback.success {
  color: #9ef0c7;
}

.status-chip {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 44px;
  padding: 10px 18px;
  border-radius: 999px;
  background: rgba(255,255,255,0.05);
  font-weight: 700;
  color: var(--text);
}

.status-chip.recording,
.status-chip.stopping,
.status-chip.merging {
  color: #91f3bf;
  border-color: rgba(57, 217, 138, 0.28);
  background: rgba(57, 217, 138, 0.12);
}

.status-chip.error {
  color: #ffb6b6;
  border-color: rgba(255, 107, 107, 0.3);
  background: rgba(255, 107, 107, 0.12);
}

.status-chip.disabled,
.status-chip.idle {
  color: #c8d5e5;
  border-color: rgba(73, 198, 229, 0.24);
  background: rgba(73, 198, 229, 0.1);
}

.apply-list {
  display: grid;
  gap: 12px;
}

.apply-list article {
  padding: 16px;
  border-radius: 18px;
  background: rgba(255,255,255,0.04);
  display: grid;
  gap: 6px;
}

.apply-list span {
  color: var(--muted);
  line-height: 1.6;
}

.info-panel {
  min-height: 100%;
}

.accent-panel {
  background: linear-gradient(180deg, rgba(10, 22, 37, 0.92), rgba(16, 26, 42, 0.92));
}

.dark-panel {
  background: linear-gradient(180deg, rgba(9, 15, 25, 0.96), rgba(7, 12, 20, 0.96));
}

.note-list {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 12px;
}

.note-list li {
  padding: 15px 16px;
  border-radius: 18px;
  background: rgba(255,255,255,0.04);
  line-height: 1.65;
  color: #d6e0eb;
}

.stats-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
}

.stat-card {
  padding: 20px 22px;
  border-radius: 24px;
  background: rgba(255,255,255,0.04);
}

.stat-card span {
  display: block;
  font-size: 0.86rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--muted);
}

.stat-card strong {
  display: block;
  margin: 16px 0 10px;
  font-size: clamp(1.3rem, 2.6vw, 2rem);
  line-height: 1.25;
}

.stat-card.emphasis {
  background: linear-gradient(135deg, rgba(255,122,24,0.16), rgba(73,198,229,0.11));
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  margin: 0;
}

.summary-grid div {
  padding: 18px;
  border-radius: 20px;
  background: rgba(255,255,255,0.04);
  border: 1px solid rgba(255,255,255,0.06);
}

.summary-grid dt {
  margin-bottom: 8px;
  color: var(--muted);
}

.summary-grid dd {
  margin: 0;
  line-height: 1.65;
  word-break: break-word;
}

.event-list {
  display: grid;
  gap: 12px;
}

.event {
  padding: 16px 18px;
  border-radius: 20px;
  background: rgba(255,255,255,0.04);
}

.event-head {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: baseline;
  margin-bottom: 8px;
}

.event-head span,
.event p {
  color: var(--muted);
}

.event p {
  margin: 0;
  line-height: 1.6;
}

.table-panel {
  padding: 10px;
}

.table-wrap {
  overflow: auto;
  border-radius: 22px;
}

table {
  width: 100%;
  min-width: 920px;
  border-collapse: collapse;
}

th,
 td {
  padding: 16px 18px;
  text-align: left;
  border-bottom: 1px solid rgba(255,255,255,0.08);
  vertical-align: top;
}

th {
  font-size: 0.84rem;
  color: var(--muted);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

tbody tr {
  background: rgba(255,255,255,0.02);
}

tbody tr:nth-child(even) {
  background: rgba(255,255,255,0.035);
}

.path,
.empty-cell {
  word-break: break-all;
}

.table-status {
  display: inline-flex;
  padding: 6px 10px;
  border-radius: 999px;
  background: rgba(255,255,255,0.04);
  color: #d9e4ef;
}

.table-status.completed {
  color: #9ef0c7;
  border-color: rgba(57, 217, 138, 0.28);
}

.table-status.failed,
.table-status.error {
  color: #ffb4b4;
  border-color: rgba(255, 107, 107, 0.28);
}

@media (max-width: 1180px) {
  .content-grid.two-columns,
  .login-panel,
  .stats-grid {
    grid-template-columns: 1fr;
  }

  .hero-side,
  .actions-right {
    justify-items: start;
    text-align: left;
  }
}

@media (max-width: 960px) {
  .app-shell {
    grid-template-columns: 1fr;
  }

  .sidebar {
    border-right: 0;
    border-bottom: 1px solid rgba(255,255,255,0.08);
  }

  .content-shell {
    padding: 22px;
  }

  .page-topbar,
  .hero-card {
    flex-direction: column;
  }

  .topbar-meta {
    justify-content: flex-start;
  }
}

@media (max-width: 720px) {
  .login-shell,
  .content-shell,
  .sidebar {
    padding: 18px;
  }

  .hero-card,
  .panel,
  .login-card {
    border-radius: 24px;
    padding: 20px;
  }

  .grid-form,
  .summary-grid {
    grid-template-columns: 1fr;
  }

  .actions {
    flex-direction: column;
  }

  .actions button {
    width: 100%;
  }

  .site-bg {
    inset: 10px;
    border-radius: 22px;
  }
}

@media (max-width: 820px) {
  .table-panel {
    padding: 0;
    background: transparent;
    border: 0;
    box-shadow: none;
  }

  .table-wrap {
    overflow: visible;
  }

  table {
    min-width: 0;
  }

  thead {
    display: none;
  }

  tbody,
  tbody tr,
  tbody td {
    display: block;
    width: 100%;
  }

  tbody tr {
    margin-bottom: 14px;
    padding: 18px;
    border: 1px solid var(--line);
    border-radius: 22px;
    background: linear-gradient(180deg, rgba(12, 22, 37, 0.92), rgba(8, 15, 26, 0.9));
    box-shadow: var(--shadow);
  }

  tbody tr:nth-child(even) {
    background: linear-gradient(180deg, rgba(12, 22, 37, 0.92), rgba(8, 15, 26, 0.9));
  }

  tbody td {
    padding: 0;
    border-bottom: 0;
  }

  tbody td + td {
    margin-top: 12px;
  }

  tbody td::before {
    content: attr(data-label);
    display: block;
    margin-bottom: 6px;
    font-size: 0.78rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--muted);
  }

  .empty-cell {
    padding: 20px;
    text-align: center;
    border: 1px solid var(--line);
    border-radius: 22px;
    background: linear-gradient(180deg, rgba(12, 22, 37, 0.92), rgba(8, 15, 26, 0.9));
    box-shadow: var(--shadow);
  }

  .empty-cell::before {
    display: none;
  }
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

function appPath(path = '') {
  const base = document.body?.dataset?.basePath || '';
  if (!path || path === '/') {
    return base + '/';
  }
  return base + (path.startsWith('/') ? path : '/' + path);
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

function applyStatusAppearance(node, state) {
  if (!node) return;
  node.classList.remove('recording', 'stopping', 'merging', 'error', 'disabled', 'idle');
  if (state) {
    node.classList.add(state);
  }
}

function bindLogout() {
  const btn = document.getElementById('logoutBtn');
  if (!btn) return;
  btn.addEventListener('click', async () => {
    await api(appPath('/api/logout'), { method: 'POST' });
    window.location.href = appPath('/login');
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
      await api(appPath('/api/login'), { method: 'POST', body: JSON.stringify(payload) });
      window.location.href = appPath('/admin/status');
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
      await api(appPath('/api/config'), { method: 'PUT', body: JSON.stringify(payload) });
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
      await api(appPath('/api/config'), { method: 'PUT', body: JSON.stringify(payload) });
      await api(appPath('/api/recording/start'), { method: 'POST' });
      form.querySelector('[name=auto_record_enabled]').checked = true;
      messageNode.textContent = '配置已保存，已启用自动录制，并立即触发一次开播检测';
    } catch (error) {
      messageNode.textContent = error.message;
    }
  });
  document.getElementById('manualStopBtn')?.addEventListener('click', async () => {
    try {
      await api(appPath('/api/recording/stop'), { method: 'POST' });
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
        await api(appPath('/api/recording/rebuild-latest'), { method: 'POST' });
        alert('已触发最近一次录制重试合并');
      } catch (error) {
        alert(error.message);
      }
    });
  }
  if (!document.getElementById('statusState')) return;
  const refresh = async () => {
    try {
      const data = await api(appPath('/api/status'));
      const status = data.status;
      const stateNode = document.getElementById('statusState');
      stateNode.textContent = statusCategory(status.state);
      applyStatusAppearance(stateNode, status.state);
      document.getElementById('statusMessage').textContent = status.message;
      document.getElementById('lastCheck').textContent = status.last_check_at ? new Date(status.last_check_at).toLocaleString() : '-';
      document.getElementById('diskFree').textContent = (status.disk_free_bytes / 1024 / 1024 / 1024).toFixed(2) + ' GB';
      document.getElementById('cfgStreamer').textContent = status.current_config.streamer_name || '-';
      document.getElementById('cfgRoom').textContent = status.current_config.room_url || '-';
      document.getElementById('cfgSubdir').textContent = status.current_config.save_subdir || '-';
      const eventList = document.getElementById('eventList');
      eventList.innerHTML = (data.events || []).map(event => '<article class="event"><div class="event-head"><strong>' + event.event_type + '</strong><span>' + new Date(event.created_at).toLocaleString() + '</span></div><p>' + event.message + '</p></article>').join('') || '<p class="muted">暂无事件</p>';
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
