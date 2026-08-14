package public

import (
	"html"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/monitor-monitor/monitor/pkg/config"
)

func serveTerminalPage(c *gin.Context) {
	cfg, _ := config.GetMany(map[string]any{config.SitenameKey: "VPS Monitor"})
	siteName, _ := cfg[config.SitenameKey].(string)
	if siteName == "" {
		siteName = "VPS Monitor"
	}
	escapedSiteName := html.EscapeString(siteName)
	page := `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width,initial-scale=1" />
  <title>节点控制台 - ` + escapedSiteName + `</title>
  <link rel="icon" href="/favicon.ico?t=` + getFaviconTimestamp() + `" />
  <link rel="stylesheet" href="/terminal-assets/xterm.css" />
  <link rel="stylesheet" href="/terminal-assets/terminal-page.css?v=20260723-1" />
</head>
<body data-site-name="` + escapedSiteName + `">
  <main class="shell">
    <header class="topbar">
      <div class="identity">
        <h1>` + escapedSiteName + ` 终端</h1>
        <div class="node-name" id="nodeName">准备连接...</div>
      </div>
      <div class="metrics" aria-label="节点实时状态">
        <div class="metric"><span>CPU</span><strong id="metricCpu">--</strong></div>
        <div class="metric"><span>RAM</span><strong id="metricRam">--</strong></div>
        <div class="metric"><span>磁盘</span><strong id="metricDisk">--</strong></div>
        <div class="metric upload"><span>实时上行</span><strong id="metricUp">--</strong></div>
        <div class="metric download"><span>实时下行</span><strong id="metricDown">--</strong></div>
      </div>
      <div class="top-actions">
        <button class="icon-button" id="reconnectButton" type="button" title="重新连接断开的通道" aria-label="重新连接断开的通道">↻</button>
        <button class="icon-button files-toggle" id="filesToggle" type="button" title="显示或隐藏文件管理">文件</button>
        <div class="status" id="status"><span class="dot"></span><span id="statusText">初始化</span></div>
      </div>
    </header>

    <section class="workspace" id="workspace">
      <section class="terminal-wrap">
        <div id="terminal"></div>
        <div class="fallback" id="fallback">
          <strong>终端组件加载失败。</strong>
          <p>请刷新页面，或返回后台重新打开终端。</p>
          <p><a href="/admin">返回后台</a></p>
        </div>
      </section>
      <div class="resizer" id="resizer" title="拖动调整宽度"></div>
      <aside class="file-panel" id="filePanel">
        <div class="file-header">
          <div>
            <h2>文件管理</h2>
            <span class="file-status" id="fileStatus">连接中</span>
          </div>
          <button class="icon-button close-files" id="closeFiles" type="button" title="收起文件管理">×</button>
        </div>
        <div class="path-row">
          <button class="icon-button" id="homeButton" type="button" title="主目录">⌂</button>
          <button class="icon-button" id="upButton" type="button" title="上级目录">↑</button>
          <input id="pathInput" aria-label="当前路径" spellcheck="false" />
          <button class="icon-button" id="refreshButton" type="button" title="刷新">↻</button>
        </div>
        <div class="file-toolbar">
          <button type="button" id="uploadButton">上传</button>
          <button type="button" id="newFileButton">新建文件</button>
          <button type="button" id="newFolderButton">新建文件夹</button>
          <button type="button" id="pasteButton" disabled>粘贴</button>
          <button type="button" id="batchCopyButton" disabled>复制所选</button>
          <button type="button" id="batchCutButton" disabled>剪切所选</button>
          <button type="button" id="batchArchiveButton" disabled>压缩所选</button>
          <button type="button" id="batchDeleteButton" class="danger-button" disabled>删除所选</button>
          <label class="check-control"><input type="checkbox" id="showHiddenInput" /> 显示隐藏文件</label>
          <input type="file" id="uploadInput" hidden multiple />
        </div>
        <div class="transfer" id="transfer" hidden>
          <div><span id="transferName">传输中</span><button id="cancelTransfer" type="button">取消</button></div>
          <progress id="transferProgress" max="100" value="0"></progress>
        </div>
        <div class="file-list-wrap">
          <table class="file-list">
            <thead><tr>
              <th class="select-column"><input id="selectAllFiles" type="checkbox" aria-label="全选当前目录" /></th>
              <th><button class="sort-button" data-sort="name" type="button">名称</button></th>
              <th>类型</th>
              <th><button class="sort-button" data-sort="size" type="button">大小</button></th>
              <th>权限</th>
              <th>所有者/组</th>
              <th><button class="sort-button" data-sort="modified" type="button">修改时间</button></th>
            </tr></thead>
            <tbody id="fileRows"><tr><td colspan="7" class="empty">正在连接 Agent...</td></tr></tbody>
          </table>
        </div>
      </aside>
    </section>
  </main>

  <div class="context-menu" id="contextMenu" hidden>
    <button data-action="open">打开</button>
    <button data-action="edit">编辑</button>
    <button data-action="download">下载</button>
    <button data-action="copy">复制</button>
    <button data-action="cut">剪切</button>
    <button data-action="move">移动</button>
    <button data-action="rename">重命名</button>
    <button data-action="archive">压缩</button>
    <button data-action="extract">解压</button>
    <button data-action="permissions">权限</button>
    <button data-action="properties">属性</button>
    <button data-action="delete" class="danger">删除</button>
  </div>

  <div class="modal-backdrop" id="modal" hidden>
    <section class="modal-card" role="dialog" aria-modal="true">
      <div class="modal-head"><h3 id="modalTitle">提示</h3><button id="modalClose" type="button">×</button></div>
      <div id="modalMessage" class="modal-message"></div>
      <input id="modalInput" class="modal-input" hidden />
      <textarea id="modalEditor" class="modal-editor" hidden spellcheck="false"></textarea>
      <div class="modal-actions"><button id="modalCancel" type="button">取消</button><span id="modalChoices"></span><button id="modalConfirm" type="button" class="primary">确定</button></div>
    </section>
  </div>

  <div class="toast" id="toast" hidden></div>
  <script src="/terminal-assets/xterm.js"></script>
  <script src="/terminal-assets/xterm-addon-fit.js"></script>
  <script src="/terminal-assets/terminal-page.js?v=20260814-1"></script>
</body>
</html>`
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}
