package public

import (
	"html"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/pkg/config"
)

func serveTerminalPage(c *gin.Context) {
	cfg, _ := config.GetMany(map[string]any{
		config.SitenameKey: "VPS Monitor",
	})
	siteName, _ := cfg[config.SitenameKey].(string)
	if siteName == "" {
		siteName = "VPS Monitor"
	}

	escapedSiteName := html.EscapeString(siteName)
	favTimestamp := getFaviconTimestamp()

	page := `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width,initial-scale=1" />
  <title>终端 - ` + escapedSiteName + `</title>
  <link rel="icon" href="/favicon.ico?t=` + favTimestamp + `" />
  <link rel="stylesheet" href="/terminal-assets/xterm.css" />
  <style>
    :root {
      color-scheme: dark;
      --bg: #07111f;
      --panel: rgba(15, 23, 42, .92);
      --line: rgba(148, 163, 184, .24);
      --text: #e2e8f0;
      --muted: #94a3b8;
      --green: #22c55e;
      --red: #ef4444;
      --blue: #3b82f6;
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    * { box-sizing: border-box; }
    html, body { height: 100%; }
    body {
      margin: 0;
      min-height: 100%;
      background:
        linear-gradient(135deg, rgba(34, 197, 94, .09), transparent 35%),
        radial-gradient(circle at 78% 14%, rgba(59, 130, 246, .12), transparent 34%),
        var(--bg);
      color: var(--text);
      overflow: hidden;
    }
    .shell {
      height: 100%;
      padding: 18px;
      display: grid;
      grid-template-rows: auto 1fr;
      gap: 14px;
    }
    .topbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      min-height: 50px;
      padding: 10px 14px;
      border: 1px solid var(--line);
      border-radius: 16px;
      background: rgba(15, 23, 42, .74);
      backdrop-filter: blur(18px);
    }
    .title {
      min-width: 0;
      display: flex;
      flex-direction: column;
      gap: 3px;
    }
    h1 {
      margin: 0;
      font-size: 18px;
      line-height: 1.2;
    }
    .uuid {
      color: var(--muted);
      font: 12px/1.3 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: min(640px, 58vw);
    }
    .status {
      flex-shrink: 0;
      display: inline-flex;
      align-items: center;
      gap: 8px;
      color: var(--muted);
      font-size: 13px;
    }
    .dot {
      width: 9px;
      height: 9px;
      border-radius: 999px;
      background: var(--muted);
      box-shadow: 0 0 0 4px rgba(148, 163, 184, .14);
    }
    .status.ok .dot {
      background: var(--green);
      box-shadow: 0 0 0 4px rgba(34, 197, 94, .14);
    }
    .status.bad .dot {
      background: var(--red);
      box-shadow: 0 0 0 4px rgba(239, 68, 68, .14);
    }
    .terminal-wrap {
      min-height: 0;
      overflow: hidden;
      border: 1px solid var(--line);
      border-radius: 18px;
      background: rgba(2, 6, 23, .92);
      box-shadow: 0 18px 60px rgba(0, 0, 0, .26);
    }
    #terminal {
      width: 100%;
      height: 100%;
      padding: 14px;
    }
    .fallback {
      height: 100%;
      padding: 22px;
      color: var(--muted);
      font-size: 14px;
      display: none;
    }
    .fallback strong { color: var(--text); }
    .fallback a { color: var(--blue); }
    @media (max-width: 720px) {
      .shell { padding: 10px; gap: 10px; }
      .topbar { align-items: flex-start; flex-direction: column; }
      .uuid { max-width: calc(100vw - 54px); }
      .status { font-size: 12px; }
      #terminal { padding: 10px; }
    }
  </style>
</head>
<body>
  <main class="shell">
    <header class="topbar">
      <div class="title">
        <h1>` + escapedSiteName + ` 终端</h1>
        <div class="uuid" id="uuid">准备连接...</div>
      </div>
      <div class="status" id="status"><span class="dot"></span><span id="statusText">初始化</span></div>
    </header>
    <section class="terminal-wrap">
      <div id="terminal"></div>
      <div class="fallback" id="fallback">
        <strong>终端组件加载失败。</strong>
        <p>请刷新页面，或返回后台重新打开终端。</p>
        <p><a href="/admin">返回后台</a></p>
      </div>
    </section>
  </main>
  <script src="/terminal-assets/xterm.js"></script>
  <script src="/terminal-assets/xterm-addon-fit.js"></script>
  <script>
    (function () {
      var params = new URLSearchParams(location.search);
      var uuid = params.get('uuid') || '';
      var uuidEl = document.getElementById('uuid');
      var statusEl = document.getElementById('status');
      var statusText = document.getElementById('statusText');
      var terminalEl = document.getElementById('terminal');
      var fallbackEl = document.getElementById('fallback');
      var ws = null;
      var fitAddon = null;
      var term = null;

      function setStatus(text, state) {
        statusText.textContent = text;
        statusEl.classList.remove('ok', 'bad');
        if (state) statusEl.classList.add(state);
      }

      function wsURL() {
        var protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        return protocol + '//' + location.host + '/api/admin/client/' + encodeURIComponent(uuid) + '/terminal';
      }

      async function getTerminalSettings() {
        try {
          var response = await fetch('/api/rpc2', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({
              jsonrpc: '2.0',
              method: 'admin:getXtermjsSettings',
              params: {},
              id: Date.now()
            })
          });
          if (!response.ok) return {};
          var payload = await response.json();
          return payload && payload.result ? payload.result : {};
        } catch (err) {
          return {};
        }
      }

      async function getClientName() {
        if (!uuid) return '';
        try {
          var response = await fetch('/api/rpc2', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({
              jsonrpc: '2.0',
              method: 'admin:getClient',
              params: { uuid: uuid },
              id: Date.now()
            })
          });
          if (!response.ok) return '';
          var payload = await response.json();
          var client = payload && payload.result ? payload.result : null;
          return client && client.name ? String(client.name) : '';
        } catch (err) {
          return '';
        }
      }

      function fit() {
        if (!fitAddon || !term) return;
        try {
          fitAddon.fit();
          if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
          }
        } catch (err) {}
      }

      function writeMessage(data) {
        if (!term) return;
        if (typeof data === 'string') {
          term.write(data);
          return;
        }
        if (data instanceof ArrayBuffer) {
          term.write(new TextDecoder().decode(new Uint8Array(data)));
          return;
        }
        if (data instanceof Blob) {
          data.arrayBuffer().then(writeMessage);
        }
      }

      async function start() {
        try {
          uuidEl.textContent = uuid ? '节点：加载中...' : '缺少节点';
          if (!uuid) {
            setStatus('缺少节点', 'bad');
            return;
          }
          var clientNamePromise = getClientName();
          var TerminalCtor = globalThis.Terminal || self.Terminal || window.Terminal;
          var FitAddonCtor = (globalThis.FitAddon && globalThis.FitAddon.FitAddon) ||
            (self.FitAddon && self.FitAddon.FitAddon) ||
            (window.FitAddon && window.FitAddon.FitAddon);
          if (!TerminalCtor || !FitAddonCtor) {
            terminalEl.style.display = 'none';
            fallbackEl.style.display = 'block';
            setStatus('组件加载失败', 'bad');
            return;
          }

          var clientName = await clientNamePromise;
          var displayName = clientName || uuid;
          uuidEl.textContent = '节点：' + displayName;

          var settings = await getTerminalSettings();
          var defaultTheme = {
            foreground: '#e2e8f0',
            background: '#020617',
            cursor: '#22c55e',
            selectionBackground: '#334155'
          };
          var incomingOptions = Object.assign({}, settings.terminalOptions || {});
          var incomingTheme = incomingOptions.theme;
          delete incomingOptions.theme;
          var options = Object.assign({
            cursorBlink: true,
            convertEol: true,
            fontFamily: "'Cascadia Mono', 'Noto Sans SC', Menlo, Monaco, Consolas, monospace",
            fontSize: 15,
            scrollback: 5000,
            macOptionIsMeta: true
          }, incomingOptions);
          options.theme = Object.assign({}, defaultTheme, incomingTheme || {});

          term = new TerminalCtor(options);
          fitAddon = new FitAddonCtor();
          term.loadAddon(fitAddon);
          term.open(terminalEl);
          term.writeln('Connecting to ' + displayName + ' ...');
          window.addEventListener('resize', fit);
          setTimeout(fit, 0);

          setStatus('连接中');
          ws = new WebSocket(wsURL());
          ws.binaryType = 'arraybuffer';
          ws.onopen = function () {
            setStatus('已连接', 'ok');
            term.writeln('\r\nConnected. Waiting for agent...');
            term.focus();
            fit();
          };
          ws.onmessage = function (event) {
            writeMessage(event.data);
          };
          ws.onerror = function () {
            setStatus('连接错误', 'bad');
          };
          ws.onclose = function () {
            setStatus('已断开', 'bad');
            if (term) term.writeln('\r\n\r\n[connection closed]');
          };
          term.onData(function (data) {
            if (ws && ws.readyState === WebSocket.OPEN) {
              ws.send(JSON.stringify({ type: 'input', input: data }));
            }
          });
        } catch (err) {
          console.error(err);
          setStatus('启动失败', 'bad');
          if (term) term.writeln('\r\nTerminal startup failed: ' + (err && err.message ? err.message : err));
        }
      }

      start();
    })();
  </script>
</body>
</html>`

	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}
