(function () {
  'use strict';

  var params = new URLSearchParams(location.search);
  var uuid = params.get('uuid') || '';
  var nodeName = uuid;
  var ws = null;
  var fileReady = false;
  var fileInitialDirectoryLoaded = false;
  var fitAddon = null;
  var term = null;
  var metricTimer = null;
  var connectionHeartbeatTimer = null;
  var requestSequence = 0;
  var pendingRequests = new Map();
  var toastTimer = null;
  var currentTransfer = null;
  var fileClipboard = null;
  var fileBusy = 0;
  var fileState = { path: '', parent: '', offset: 0, total: 0, hasMore: false, items: [], selected: null, selectedPaths: new Set(), home: '', showHidden: false, sort: 'name', order: 'asc' };

  function byId(id) { return document.getElementById(id); }
  var nodeNameEl = byId('nodeName');
  var statusEl = byId('status');
  var statusText = byId('statusText');
  var terminalEl = byId('terminal');
  var fallbackEl = byId('fallback');
  var workspace = byId('workspace');
  var filePanel = byId('filePanel');
  var fileStatus = byId('fileStatus');
  var pathInput = byId('pathInput');
  var fileRows = byId('fileRows');
  var contextMenu = byId('contextMenu');
  var fileListWrap = document.querySelector('.file-list-wrap');
  var pasteButton = byId('pasteButton');
  var reconnectButton = byId('reconnectButton');

  function isDirectoryItem(item) { return !!(item && (item.is_dir || item.link_is_dir)); }
  function selectedItems() { return fileState.items.filter(function (item) { return fileState.selectedPaths.has(item.path); }); }
  function setFileBusy(busy) {
    fileBusy += busy ? 1 : -1;
    if (fileBusy < 0) fileBusy = 0;
    var disabled = fileBusy > 0;
    if (filePanel) filePanel.querySelectorAll('button').forEach(function (button) {
      if (button.id !== 'cancelTransfer') button.disabled = disabled || (button.dataset.selectionAction && !selectedItems().length);
    });
    updateClipboardUI();
    updateSelectionUI();
  }
  async function runFileOperation(action) {
    setFileBusy(true);
    try { return await action(); } finally { setFileBusy(false); }
  }

  function setStatus(text, state) {
    statusText.textContent = text;
    statusEl.classList.remove('ok', 'bad');
    if (state) statusEl.classList.add(state);
  }

  function setFileStatus(text, state) {
    fileStatus.textContent = text;
    fileStatus.classList.remove('ok', 'bad');
    if (state) fileStatus.classList.add(state);
  }

  function showToast(message, bad) {
    var toast = byId('toast');
    toast.textContent = message;
    toast.classList.toggle('bad', !!bad);
    toast.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { toast.hidden = true; }, bad ? 5000 : 2600);
  }

  function websocketURL(path) {
    var protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    return protocol + '//' + location.host + path;
  }

  function terminalURL() {
    return websocketURL('/api/admin/client/' + encodeURIComponent(uuid) + '/terminal');
  }

  function isWSOpen(socket) {
    return !!socket && socket.readyState === WebSocket.OPEN;
  }

  function isWSConnecting(socket) {
    return !!socket && socket.readyState === WebSocket.CONNECTING;
  }

  async function rpc(method, rpcParams) {
    var response = await fetch('/api/rpc2', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({ jsonrpc: '2.0', method: method, params: rpcParams || {}, id: Date.now() + Math.random() })
    });
    if (!response.ok) throw new Error('请求失败 (' + response.status + ')');
    var payload = await response.json();
    if (payload && payload.error) throw new Error(payload.error.message || '请求失败');
    return payload ? payload.result : null;
  }

  async function getTerminalSettings() {
    try { return await rpc('admin:getXtermjsSettings', {}) || {}; } catch (_) { return {}; }
  }

  async function getClientName() {
    if (!uuid) return '';
    try {
      var client = await rpc('admin:getClient', { uuid: uuid });
      return client && client.name ? String(client.name) : '';
    } catch (_) { return ''; }
  }

  function formatBytes(value) {
    var bytes = Number(value || 0);
    if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
    var units = ['B', 'KB', 'MB', 'GB', 'TB'];
    var index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
    var number = bytes / Math.pow(1024, index);
    return (number >= 100 || index === 0 ? number.toFixed(0) : number.toFixed(1)) + ' ' + units[index];
  }

  function formatRate(value) { return formatBytes(value) + '/s'; }

  function percent(used, total) {
    used = Number(used || 0);
    total = Number(total || 0);
    if (!total) return '--';
    return Math.max(0, Math.min(100, used / total * 100)).toFixed(1) + '%';
  }

  async function updateMetrics() {
    if (!uuid || document.hidden) return;
    try {
      var data = await rpc('common:getNodesLatestStatus', { uuid: uuid });
      if (!data) return;
      byId('metricCpu').textContent = Number(data.cpu || 0).toFixed(1) + '%';
      byId('metricRam').textContent = percent(data.ram, data.ram_total);
      byId('metricDisk').textContent = percent(data.disk, data.disk_total);
      byId('metricUp').textContent = formatRate(data.net_out);
      byId('metricDown').textContent = formatRate(data.net_in);
    } catch (_) {}
  }

  function fit() {
    if (!fitAddon || !term || !terminalEl.offsetParent) return;
    try {
      fitAddon.fit();
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
      }
    } catch (_) {}
  }

  function writeTerminalMessage(data) {
    if (!term) return;
    if (typeof data === 'string') { term.write(data); return; }
    if (data instanceof ArrayBuffer) { term.write(new TextDecoder().decode(new Uint8Array(data))); return; }
    if (data instanceof Blob) data.arrayBuffer().then(writeTerminalMessage);
  }

  function handleWorkspaceMessage(connection, data) {
    if (typeof data !== 'string') {
      writeTerminalMessage(data);
      return;
    }
    var message;
    try { message = JSON.parse(data); } catch (_) {
      writeTerminalMessage(data);
      return;
    }
    if (!message || (message.type !== 'system' && message.type !== 'response')) {
      writeTerminalMessage(data);
      return;
    }
    if (message.type === 'system') {
      if (!message.ok) {
        fileReady = false;
        setFileStatus(message.error || '连接失败', 'bad');
        renderFileError(message.error || '文件管理连接失败');
        return;
      }
      fileReady = true;
      setFileStatus('已连接', 'ok');
      if (message.data && message.data.home) {
        fileState.home = message.data.home;
        if (!fileInitialDirectoryLoaded) {
          fileInitialDirectoryLoaded = true;
          var initialPath = fileState.path || fileState.home;
          loadDirectory(initialPath, 0, { silentError: initialPath !== fileState.home }).then(function (loaded) {
            if (!loaded && initialPath !== fileState.home && ws === connection) loadDirectory(fileState.home, 0);
          });
        }
      }
      return;
    }
    if (message.type === 'response' && message.id) {
      var pending = pendingRequests.get(message.id);
      if (!pending) return;
      pendingRequests.delete(message.id);
      clearTimeout(pending.timer);
      if (message.ok) pending.resolve(message.data);
      else {
        var error = new Error(message.error || '文件操作失败');
        error.code = message.code || '';
        error.details = message.details || null;
        pending.reject(error);
      }
    }
  }

  function connectTerminal(displayName) {
    if (!uuid) return null;
    if (isWSOpen(ws) || isWSConnecting(ws)) return ws;
    setStatus('连接中');
    var connection = new WebSocket(terminalURL());
    ws = connection;
    fileReady = false;
    fileInitialDirectoryLoaded = false;
    connection.binaryType = 'arraybuffer';
    connection.onopen = function () {
      if (ws !== connection) return;
      setStatus('已连接', 'ok');
      setFileStatus('等待 Agent');
      term.writeln('\r\nConnected. Waiting for agent...');
      term.focus();
      fit();
    };
    connection.onmessage = function (event) {
      if (ws === connection) handleWorkspaceMessage(connection, event.data);
    };
    connection.onerror = function () { if (ws === connection) setStatus('连接错误', 'bad'); };
    connection.onclose = function () {
      if (ws !== connection) return;
      ws = null;
      fileReady = false;
      rejectPendingFileRequests();
      setStatus('已断开', 'bad');
      setFileStatus('已断开', 'bad');
      if (term) term.writeln('\r\n\r\n[connection closed]');
    };
    return connection;
  }

  function reconnectDisconnectedChannels() {
    if (!term || !uuid) return;
    if (!isWSOpen(ws) && !isWSConnecting(ws)) connectTerminal(nodeName);
  }

  function rejectPendingFileRequests() {
    pendingRequests.forEach(function (pending) {
      clearTimeout(pending.timer);
      pending.reject(new Error('文件管理连接已断开'));
    });
    pendingRequests.clear();
  }

  function connectFiles(options) {
    options = options || {};
    if (!uuid) return null;
    if (!isWSOpen(ws) && !isWSConnecting(ws)) connectTerminal(nodeName);
    return ws;
  }

  function startConnectionHeartbeat() {
    stopConnectionHeartbeat();
    connectionHeartbeatTimer = setInterval(function () {
      if (ws && ws.readyState === WebSocket.OPEN && term) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
      }
    }, 25000);
  }

  function stopConnectionHeartbeat() {
    if (!connectionHeartbeatTimer) return;
    clearInterval(connectionHeartbeatTimer);
    connectionHeartbeatTimer = null;
  }

  function fileRequest(type, payload, timeout) {
    return new Promise(function (resolve, reject) {
      if (!ws || ws.readyState !== WebSocket.OPEN || !fileReady) {
        reject(new Error('文件管理尚未连接'));
        return;
      }
      var id = 'file-' + Date.now() + '-' + (++requestSequence);
      var request = Object.assign({ id: id, type: type }, payload || {});
      var timer = setTimeout(function () {
        pendingRequests.delete(id);
        reject(new Error('文件操作超时'));
      }, timeout || 30000);
      pendingRequests.set(id, { resolve: resolve, reject: reject, timer: timer });
      ws.send(JSON.stringify(request));
    });
  }

  function renderFileError(message) {
    fileRows.replaceChildren();
    var row = document.createElement('tr');
    var cell = document.createElement('td');
    cell.colSpan = 7;
    cell.className = 'empty';
    cell.textContent = message;
    row.appendChild(cell);
    fileRows.appendChild(row);
  }

  async function loadDirectory(path, offset, viewOptions) {
    viewOptions = viewOptions || {};
    hideContextMenu();
    renderFileError('正在读取目录...');
    try {
      var data = await fileRequest('list', { path: path || fileState.home, offset: 0, limit: 1000, show_hidden: fileState.showHidden, sort: fileState.sort, order: fileState.order });
      fileState.path = data.path;
      fileState.parent = data.parent;
      fileState.offset = 0;
      fileState.total = Number(data.total || 0);
      fileState.hasMore = !!data.has_more;
      fileState.items = Array.isArray(data.items) ? data.items : [];
      fileState.selected = null;
      fileState.selectedPaths = new Set();
      pathInput.value = data.path;
      renderFiles();
      requestAnimationFrame(function () {
        if (!fileListWrap) return;
        var requestedScroll = Number(viewOptions.scrollTop || 0);
        var maximumScroll = Math.max(0, fileListWrap.scrollHeight - fileListWrap.clientHeight);
        fileListWrap.scrollTop = Math.min(Math.max(0, requestedScroll), maximumScroll);
      });
      if (data.truncated) showToast('目录项目超过 1000 个，仅显示前 1000 个', true);
      return true;
    } catch (err) {
      if (!viewOptions.silentError) {
        renderFileError(err.message);
        showToast(err.message, true);
      }
      return false;
    }
  }

  function formatModified(value) {
    var date = new Date(value);
    if (Number.isNaN(date.getTime())) return '--';
    return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
  }

  function renderFiles() {
    fileRows.replaceChildren();
    if (!fileState.items.length) {
      renderFileError('此目录为空');
    } else {
      fileState.items.forEach(function (item) {
        var row = document.createElement('tr');
        row.className = 'file-row';
        if (fileState.selectedPaths.has(item.path)) row.classList.add('selected');
        if (fileClipboard && fileClipboard.mode === 'move' && fileClipboard.paths.indexOf(item.path) >= 0) row.classList.add('clipboard-cut');
        row.dataset.path = item.path;
        row.title = [item.mode, item.link_target ? '→ ' + item.link_target : ''].filter(Boolean).join('\n');

        var selectedCell = document.createElement('td');
        selectedCell.className = 'select-column';
        var checkbox = document.createElement('input');
        checkbox.type = 'checkbox';
        checkbox.checked = fileState.selectedPaths.has(item.path);
        checkbox.setAttribute('aria-label', '选择 ' + item.name);
        checkbox.addEventListener('click', function (event) { event.stopPropagation(); });
        checkbox.addEventListener('change', function () { selectFile(item, row, true, checkbox.checked); });
        selectedCell.appendChild(checkbox);

        var nameCell = document.createElement('td');
        var nameWrap = document.createElement('div');
        nameWrap.className = 'file-name';
        var icon = document.createElement('span');
        icon.className = 'file-icon';
        icon.textContent = isDirectoryItem(item) ? '📁' : (item.is_link ? '🔗' : '📄');
        var name = document.createElement('span');
        name.textContent = item.name + (item.is_link && item.link_target ? ' → ' + item.link_target : '');
        nameWrap.append(icon, name);
        nameCell.appendChild(nameWrap);

        var typeCell = document.createElement('td');
        typeCell.className = 'file-meta file-type';
        typeCell.textContent = item.type || (isDirectoryItem(item) ? '目录' : '文件');
        var sizeCell = document.createElement('td');
        sizeCell.className = 'file-meta';
        sizeCell.textContent = isDirectoryItem(item) ? '--' : formatBytes(item.size);
        var permissionsCell = document.createElement('td');
        permissionsCell.className = 'file-meta';
        permissionsCell.textContent = item.permissions || item.mode || '--';
        var ownerCell = document.createElement('td');
        ownerCell.className = 'file-meta file-owner';
        ownerCell.textContent = (item.owner || '--') + '/' + (item.group || '--');
        var modifiedCell = document.createElement('td');
        modifiedCell.className = 'file-meta';
        modifiedCell.textContent = formatModified(item.modified);
        row.append(selectedCell, nameCell, typeCell, sizeCell, permissionsCell, ownerCell, modifiedCell);
        row.addEventListener('click', function (event) { selectFile(item, row, event.metaKey || event.ctrlKey || event.shiftKey); });
        row.addEventListener('dblclick', function () { openItem(item); });
        row.addEventListener('contextmenu', function (event) {
          event.preventDefault();
          if (!fileState.selectedPaths.has(item.path)) selectFile(item, row, false);
          showContextMenu(event.clientX, event.clientY, item);
        });
        fileRows.appendChild(row);
      });
    }
    updateClipboardUI();
    updateSelectionUI();
    updateSortUI();
  }

  function selectFile(item, row, toggle, checked) {
    fileState.selected = item;
    if (!toggle) fileState.selectedPaths = new Set([item.path]);
    else if (checked === false || (checked === undefined && fileState.selectedPaths.has(item.path))) fileState.selectedPaths.delete(item.path);
    else fileState.selectedPaths.add(item.path);
    fileRows.querySelectorAll('.file-row').forEach(function (node) { node.classList.toggle('selected', fileState.selectedPaths.has(node.dataset.path)); });
    var selectAll = byId('selectAllFiles');
    if (selectAll) selectAll.checked = fileState.items.length > 0 && fileState.selectedPaths.size === fileState.items.length;
    updateSelectionUI();
  }

  function showContextMenu(x, y, item) {
    fileState.selected = item;
    contextMenu.hidden = false;
    var openButton = contextMenu.querySelector('[data-action="open"]');
    var editButton = contextMenu.querySelector('[data-action="edit"]');
    var downloadButton = contextMenu.querySelector('[data-action="download"]');
    var extractButton = contextMenu.querySelector('[data-action="extract"]');
    openButton.textContent = '打开';
    editButton.hidden = !item.text_candidate;
    downloadButton.hidden = isDirectoryItem(item);
    extractButton.hidden = !isArchiveName(item.name);
    var width = contextMenu.offsetWidth;
    var height = contextMenu.offsetHeight;
    contextMenu.style.left = Math.min(x, window.innerWidth - width - 8) + 'px';
    contextMenu.style.top = Math.min(y, window.innerHeight - height - 8) + 'px';
  }

  function hideContextMenu() { contextMenu.hidden = true; }

  function currentFileScroll() {
    return fileListWrap ? fileListWrap.scrollTop : 0;
  }

  function reloadCurrentDirectory() {
    return loadDirectory(fileState.path, fileState.offset, { scrollTop: currentFileScroll() });
  }

  function refreshFiles() {
    if (fileReady) reloadCurrentDirectory();
  }

  function updateClipboardUI() {
    pasteButton.disabled = fileBusy > 0 || !fileClipboard;
    pasteButton.textContent = fileClipboard ? (fileClipboard.mode === 'move' ? '粘贴（剪切）' : '粘贴（复制）') : '粘贴';
    pasteButton.title = fileClipboard ? fileClipboard.items.length + ' 项 → ' + fileState.path : '请先复制或剪切文件';
    fileRows.querySelectorAll('.clipboard-cut').forEach(function (row) { row.classList.remove('clipboard-cut'); });
    if (fileClipboard && fileClipboard.mode === 'move') {
      Array.from(fileRows.querySelectorAll('.file-row')).forEach(function (row) {
        if (fileClipboard.paths.indexOf(row.dataset.path) >= 0) row.classList.add('clipboard-cut');
      });
    }
  }

  function setFileClipboard(items, mode) {
    items = Array.isArray(items) ? items : [items];
    if (!items.length) return;
    fileClipboard = { paths: items.map(function (item) { return item.path; }), items: items, mode: mode };
    updateClipboardUI();
    showToast((mode === 'move' ? '已剪切 ' : '已复制 ') + items.length + ' 项');
  }

  async function pasteFileClipboard() {
    if (!fileClipboard) return;
    var operation = fileClipboard.mode === 'move' ? 'move' : 'copy';
    var scrollTop = currentFileScroll();
    await runFileOperation(async function () {
      try {
        var completed = fileClipboard;
        var result = await requestWithConflict(operation + '_many', { paths: completed.paths, destination: fileState.path }, operation === 'move' ? '移动' : '复制', 1800000);
        if (operation === 'move') fileClipboard = null;
        showBatchResult(result, operation === 'move' ? '移动' : '复制');
        await loadDirectory(fileState.path, fileState.offset, { scrollTop: scrollTop });
      } catch (err) { showToast(err.message, true); }
      finally { updateClipboardUI(); }
    });
  }

  function updateSelectionUI() {
    var count = selectedItems().length;
    ['batchCopyButton', 'batchCutButton', 'batchArchiveButton', 'batchDeleteButton'].forEach(function (id) {
      var button = byId(id);
      if (button) button.disabled = fileBusy > 0 || !count;
    });
    var selectAll = byId('selectAllFiles');
    if (selectAll) {
      selectAll.indeterminate = count > 0 && count < fileState.items.length;
      selectAll.checked = fileState.items.length > 0 && count === fileState.items.length;
    }
  }

  function updateSortUI() {
    document.querySelectorAll('.sort-button').forEach(function (button) {
      var active = button.dataset.sort === fileState.sort;
      button.classList.toggle('active', active);
      button.classList.toggle('desc', active && fileState.order === 'desc');
    });
  }

  async function openItem(item) {
    hideContextMenu();
    if (isDirectoryItem(item)) { loadDirectory(item.path, 0); return; }
    if (item.text_candidate) { await editFile(item); return; }
    showToast('该文件不是已识别的文本文件，可使用“下载”或在属性中查看详情', true);
  }

  function modal(options) {
    options = options || {};
    return new Promise(function (resolve) {
      var overlay = byId('modal');
      var title = byId('modalTitle');
      var message = byId('modalMessage');
      var input = byId('modalInput');
      var editor = byId('modalEditor');
      var confirm = byId('modalConfirm');
      var cancel = byId('modalCancel');
      var close = byId('modalClose');
      var choices = byId('modalChoices');
      var mode = options.editor ? 'editor' : (options.input ? 'input' : 'confirm');
      title.textContent = options.title || '提示';
      message.textContent = options.message || '';
      message.hidden = !options.message;
      input.hidden = mode !== 'input';
      editor.hidden = mode !== 'editor';
      input.value = mode === 'input' ? (options.value || '') : '';
      editor.value = mode === 'editor' ? (options.value || '') : '';
      confirm.textContent = options.confirmLabel || '确定';
      confirm.style.background = options.danger ? '#dc2626' : '';
      confirm.style.borderColor = options.danger ? 'rgba(239,68,68,.6)' : '';
      choices.replaceChildren();
      var optionChoices = Array.isArray(options.choices) ? options.choices : [];
      choices.hidden = !optionChoices.length;
      confirm.hidden = !!optionChoices.length;
      overlay.hidden = false;

      var settled = false;
      function finish(value) {
        if (settled) return;
        settled = true;
        overlay.hidden = true;
        confirm.removeEventListener('click', accept);
        cancel.removeEventListener('click', reject);
        close.removeEventListener('click', reject);
        overlay.removeEventListener('click', backdrop);
        document.removeEventListener('keydown', keydown);
        choices.replaceChildren();
        choices.hidden = true;
        confirm.hidden = false;
        resolve(value);
      }
      function accept() { finish(mode === 'input' ? input.value : (mode === 'editor' ? editor.value : true)); }
      function reject() { finish(null); }
      function backdrop(event) { if (event.target === overlay) reject(); }
      function keydown(event) {
        if (event.key === 'Escape') reject();
        if (!optionChoices.length && event.key === 'Enter' && mode !== 'editor' && (event.metaKey || event.ctrlKey || mode === 'confirm')) accept();
      }
      confirm.addEventListener('click', accept);
      cancel.addEventListener('click', reject);
      close.addEventListener('click', reject);
      overlay.addEventListener('click', backdrop);
      document.addEventListener('keydown', keydown);
      optionChoices.forEach(function (choice) {
        var button = document.createElement('button');
        button.type = 'button';
        button.textContent = choice.label;
        if (choice.danger) button.classList.add('danger-button');
        button.addEventListener('click', function () { finish(choice.value); });
        choices.appendChild(button);
      });
      setTimeout(function () { if (mode === 'input') input.select(); else if (mode === 'editor') editor.focus(); }, 0);
    });
  }

  async function editFile(item) {
    try {
      var data;
      try { data = await fileRequest('read', { path: item.path }); }
      catch (err) {
        if (err.code !== 'text_confirmation_required') throw err;
        var accepted = await modal({ title: '打开大文件', message: err.message, confirmLabel: '继续打开', danger: true });
        if (!accepted) return;
        data = await fileRequest('read', { path: item.path, force: true }, 90000);
      }
      var original = data.content_base64 ? new TextDecoder().decode(base64ToBytes(data.content_base64)) : (data.content || '');
      var content = await modal({ title: '编辑 ' + item.name, message: '最大支持 2 MiB 的 UTF-8 文本文件', editor: true, value: original, confirmLabel: '保存' });
      if (content === null) return;
      await runFileOperation(function () { return fileRequest('write', { path: item.path, content_base64: bytesToBase64(new TextEncoder().encode(content)) }, 45000); });
      showToast('文件已保存');
      reloadCurrentDirectory();
    } catch (err) { showToast(err.message, true); }
  }

  async function renameItem(item) {
    var name = await modal({ title: '重命名', message: item.path, input: true, value: item.name, confirmLabel: '保存' });
    if (name === null || !name.trim() || name === item.name) return;
    try {
      await runFileOperation(function () { return fileRequest('rename', { path: item.path, new_name: name.trim() }); });
      showToast('重命名完成');
      if (fileClipboard && fileClipboard.paths.indexOf(item.path) >= 0) fileClipboard = null;
      reloadCurrentDirectory();
    } catch (err) { showToast(err.message, true); }
  }

  async function deleteItems(items) {
    items = Array.isArray(items) ? items : [items];
    if (!items.length) return;
    var protectedNames = items.filter(function (item) { return item.protected; }).map(function (item) { return item.path; });
    var accepted = await modal({
      title: '确认删除',
      message: '确定删除 ' + items.length + ' 项吗？' + (protectedNames.length ? '\n包含系统目录：' + protectedNames.join('、') + '，请特别确认。' : ''),
      confirmLabel: '删除',
      danger: true
    });
    if (!accepted) return;
    var scrollTop = currentFileScroll();
    await runFileOperation(async function () {
      try {
        var paths = items.map(function (item) { return item.path; });
        var result = await fileRequest('delete_many', { paths: paths, recursive: false }, 1800000);
        var retry = (result.results || []).filter(function (entry) { return !entry.ok && entry.code === 'directory_not_empty'; }).map(function (entry) { return entry.path; });
        if (retry.length) {
          var recursiveAccepted = await modal({ title: '目录包含文件', message: '该目录包含文件，删除后无法恢复，是否继续？', confirmLabel: '递归删除', danger: true });
          if (recursiveAccepted) {
            var recursiveResult = await fileRequest('delete_many', { paths: retry, recursive: true }, 1800000);
            result.results = (result.results || []).filter(function (entry) { return retry.indexOf(entry.path) < 0; }).concat(recursiveResult.results || []);
          }
        }
        if (fileClipboard && paths.some(function (path) { return fileClipboard.paths.indexOf(path) >= 0; })) fileClipboard = null;
        showBatchResult(result, '删除');
        await loadDirectory(fileState.path, fileState.offset, { scrollTop: scrollTop });
      } catch (err) { showToast(err.message, true); }
    });
  }

  function bytesToBase64(bytes) {
    var binary = '';
    var block = 0x8000;
    for (var i = 0; i < bytes.length; i += block) {
      binary += String.fromCharCode.apply(null, bytes.subarray(i, Math.min(i + block, bytes.length)));
    }
    return btoa(binary);
  }

  function base64ToBytes(encoded) {
    var binary = atob(encoded);
    var bytes = new Uint8Array(binary.length);
    for (var i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    return bytes;
  }

  function isArchiveName(name) {
    var lower = String(name || '').toLowerCase();
    return lower.endsWith('.zip') || lower.endsWith('.tar.gz') || lower.endsWith('.tgz');
  }

  async function chooseConflict(label) {
    return modal({
      title: label + '遇到同名文件',
      message: '目标目录中已有同名文件，请选择处理方式。',
      choices: [
        { label: '覆盖', value: 'overwrite', danger: true },
        { label: '跳过', value: 'skip' },
        { label: '重命名', value: 'rename' }
      ]
    });
  }

  async function requestWithConflict(type, payload, label, timeout) {
    var conflict = 'ask';
    while (true) {
      try { return await fileRequest(type, Object.assign({}, payload, { conflict: conflict }), timeout); }
      catch (err) {
        if (err.code !== 'conflict') throw err;
        conflict = await chooseConflict(label);
        if (!conflict) throw new Error('已取消' + label);
      }
    }
  }

  function showBatchResult(result, label) {
    var rows = result && Array.isArray(result.results) ? result.results : [];
    if (!rows.length) { showToast(label + '完成'); return; }
    var failures = rows.filter(function (row) { return !row.ok; });
    var skipped = rows.filter(function (row) { return row.data && row.data.skipped; });
    if (failures.length) {
      showToast(label + '完成，但有 ' + failures.length + ' 项失败：' + (failures[0].error || '请查看日志'), true);
    } else if (skipped.length) {
      showToast(label + '完成，已跳过 ' + skipped.length + ' 项');
    } else showToast(label + '完成');
  }

  function beginTransfer(name, size, cancel) {
    var transfer = byId('transfer');
    currentTransfer = { canceled: false, cancel: cancel || null };
    byId('transferName').textContent = name;
    byId('transferProgress').value = 0;
    byId('transferProgress').max = Math.max(1, Number(size || 1));
    transfer.hidden = false;
    return currentTransfer;
  }

  function updateTransfer(value) { byId('transferProgress').value = Number(value || 0); }
  function endTransfer() { byId('transfer').hidden = true; currentTransfer = null; }

  async function uploadOne(file) {
    if (file.size > 1073741824) throw new Error(file.name + ' 超过 1 GiB 限制');
    var started = await requestWithConflict('upload_start', { path: fileState.path, name: file.name, size: file.size }, '上传');
    if (started && started.skipped) { showToast(file.name + ' 已跳过'); return; }
    var uploadID = started.upload_id;
    var chunkSize = Number(started.chunk_size || 262144);
    var transferState = beginTransfer('上传 ' + file.name, file.size, function () {
      return fileRequest('upload_cancel', { upload_id: uploadID }).catch(function () {});
    });
    try {
      var offset = 0;
      while (offset < file.size) {
        if (transferState.canceled) throw new Error('上传已取消');
        var buffer = await file.slice(offset, Math.min(offset + chunkSize, file.size)).arrayBuffer();
        var bytes = new Uint8Array(buffer);
        var result = await fileRequest('upload_chunk', { upload_id: uploadID, offset: offset, data: bytesToBase64(bytes) }, 60000);
        offset = Number(result.written);
        updateTransfer(offset);
      }
      await fileRequest('upload_finish', { upload_id: uploadID }, 60000);
      showToast(file.name + ' 上传完成');
    } catch (err) {
      await fileRequest('upload_cancel', { upload_id: uploadID }).catch(function () {});
      throw err;
    } finally { endTransfer(); }
  }

  async function uploadFiles(files) {
    await runFileOperation(async function () {
      for (var i = 0; i < files.length; i++) {
        try { await uploadOne(files[i]); } catch (err) { showToast(err.message, true); break; }
      }
      await reloadCurrentDirectory();
    });
  }

  async function downloadItem(item) {
    var downloadID = '';
    var fileHandle = null;
    var writable = null;
    var chunks = null;
    if (typeof window.showSaveFilePicker === 'function') {
      try {
        fileHandle = await window.showSaveFilePicker({ suggestedName: item.name || 'download' });
      } catch (err) {
        if (err && err.name === 'AbortError') return;
        showToast('无法打开保存窗口：' + err.message, true);
        return;
      }
    }
    try {
      var started = await fileRequest('download_start', { path: item.path });
      downloadID = started.download_id;
      var size = Number(started.size || 0);
      if (fileHandle) writable = await fileHandle.createWritable();
      else chunks = [];
      var transferState = beginTransfer('下载 ' + started.name, size, function () {
        return fileRequest('download_cancel', { download_id: downloadID }).catch(function () {});
      });
      var offset = 0;
      while (offset < size || (size === 0 && offset === 0)) {
        if (transferState.canceled) throw new Error('下载已取消');
        var data = await fileRequest('download_chunk', { download_id: downloadID, offset: offset }, 60000);
        var bytes = base64ToBytes(data.data || '');
        if (writable) await writable.write(bytes);
        else chunks.push(bytes);
        offset = Number(data.next_offset || 0);
        updateTransfer(offset);
        if (data.eof) break;
      }
      if (writable) {
        await writable.close();
        writable = null;
      } else {
        var blob = new Blob(chunks, { type: 'application/octet-stream' });
        var url = URL.createObjectURL(blob);
        var link = document.createElement('a');
        link.href = url;
        link.download = started.name || item.name;
        document.body.appendChild(link);
        link.click();
        link.remove();
        setTimeout(function () { URL.revokeObjectURL(url); }, 30000);
      }
      showToast(item.name + ' 下载完成');
    } catch (err) {
      if (writable) {
        try { await writable.abort(); } catch (_) {}
        writable = null;
      }
      if (downloadID) await fileRequest('download_cancel', { download_id: downloadID }).catch(function () {});
      showToast(err.message, true);
    } finally { endTransfer(); }
  }

  async function createEntry(type) {
    var folder = type === 'mkdir';
    var name = await modal({ title: folder ? '新建文件夹' : '新建文件', input: true, value: '', confirmLabel: '创建' });
    if (name === null || !name.trim()) return;
    try {
      await runFileOperation(function () { return fileRequest(type, { path: fileState.path, name: name.trim() }); });
      showToast('创建完成');
      reloadCurrentDirectory();
    } catch (err) { showToast(err.message, true); }
  }

  async function moveItems(items) {
    items = Array.isArray(items) ? items : [items];
    if (!items.length) return;
    var destination = await modal({ title: '移动到目录', message: '请输入目标目录的绝对路径', input: true, value: fileState.path, confirmLabel: '移动' });
    if (destination === null || !destination.trim()) return;
    await runFileOperation(async function () {
      try {
        var result = await requestWithConflict('move_many', { paths: items.map(function (item) { return item.path; }), destination: destination.trim() }, '移动', 1800000);
        showBatchResult(result, '移动');
        await reloadCurrentDirectory();
      } catch (err) { showToast(err.message, true); }
    });
  }

  async function archiveItems(items) {
    items = Array.isArray(items) ? items : [items];
    if (!items.length) return;
    var format = await modal({ title: '创建压缩包', message: '选择压缩格式', choices: [{ label: '.zip', value: 'zip' }, { label: '.tar.gz', value: 'tar.gz' }] });
    if (!format) return;
    var defaultName = (items.length === 1 ? items[0].name : 'archive') + (format === 'zip' ? '.zip' : '.tar.gz');
    var name = await modal({ title: '压缩包名称', message: '压缩包将保存到当前目录', input: true, value: defaultName, confirmLabel: '创建' });
    if (name === null || !name.trim()) return;
    await runFileOperation(async function () {
      try {
        await fileRequest('archive', { paths: items.map(function (item) { return item.path; }), path: fileState.path, name: name.trim(), format: format }, 1800000);
        showToast('压缩包已创建');
        await reloadCurrentDirectory();
      } catch (err) { showToast(err.message, true); }
    });
  }

  async function extractItem(item) {
    var destination = await modal({ title: '解压到目录', message: '请输入解压目标目录；不会覆盖已存在文件。', input: true, value: fileState.path, confirmLabel: '解压' });
    if (destination === null || !destination.trim()) return;
    await runFileOperation(async function () {
      try {
        await fileRequest('extract', { path: item.path, destination: destination.trim() }, 1800000);
        showToast('解压完成');
        await reloadCurrentDirectory();
      } catch (err) { showToast(err.message, true); }
    });
  }

  function propertyText(data) {
    return [
      '路径：' + (data.path || '--'),
      '类型：' + (data.type || '--'),
      '大小：' + (isDirectoryItem(data) ? '--' : formatBytes(data.size)),
      '权限：' + (data.permissions || data.mode || '--'),
      '所有者/组：' + (data.owner || '--') + '/' + (data.group || '--'),
      '修改时间：' + formatModified(data.modified),
      data.is_link ? '软链接目标：' + (data.link_target || '目标不存在或无法读取') : '',
      data.protected ? '警告：这是系统关键目录，修改前请确认。' : ''
    ].filter(Boolean).join('\n');
  }

  async function propertiesItem(item) {
    try {
      var data = await fileRequest('properties', { path: item.path });
      var action = await modal({ title: '文件属性', message: propertyText(data), choices: [
        { label: '修改权限', value: 'chmod', danger: true },
        { label: '修改所有者', value: 'chown', danger: true },
        { label: '修改用户组', value: 'chgrp', danger: true }
      ] });
      if (!action) return;
      var labels = { chmod: '权限（如 755）', chown: '所有者（用户名或 UID）', chgrp: '用户组（组名或 GID）' };
      var value = await modal({ title: '确认修改' + labels[action], message: (data.protected ? '系统关键目录：' + data.path + '\n' : '') + '此操作可能影响服务运行，请确认输入。', input: true, value: action === 'chmod' ? (data.permissions || '644').replace(/^0/, '') : (action === 'chown' ? (data.owner || '') : (data.group || '')), confirmLabel: '保存', danger: true });
      if (value === null || !value.trim()) return;
      await runFileOperation(async function () {
        try {
          var payload = { path: item.path };
          payload[action === 'chmod' ? 'mode' : (action === 'chown' ? 'owner' : 'group')] = value.trim();
          await fileRequest(action, payload);
          showToast('属性已更新');
          await reloadCurrentDirectory();
        } catch (err) { showToast(err.message, true); }
      });
    } catch (err) { showToast(err.message, true); }
  }

  function bindFileUI() {
    byId('homeButton').addEventListener('click', function () { loadDirectory(fileState.home || '', 0); });
    byId('upButton').addEventListener('click', function () { loadDirectory(fileState.parent || fileState.path, 0); });
    byId('refreshButton').addEventListener('click', refreshFiles);
    pathInput.addEventListener('keydown', function (event) { if (event.key === 'Enter') loadDirectory(pathInput.value, 0); });
    byId('newFileButton').addEventListener('click', function () { createEntry('create'); });
    byId('newFolderButton').addEventListener('click', function () { createEntry('mkdir'); });
    pasteButton.addEventListener('click', pasteFileClipboard);
    byId('showHiddenInput').addEventListener('change', function () { fileState.showHidden = this.checked; reloadCurrentDirectory(); });
    byId('selectAllFiles').addEventListener('change', function () {
      fileState.selectedPaths = this.checked ? new Set(fileState.items.map(function (item) { return item.path; })) : new Set();
      if (!this.checked) fileState.selected = null;
      renderFiles();
    });
    document.querySelectorAll('.sort-button').forEach(function (button) {
      button.addEventListener('click', function () {
        var sort = button.dataset.sort;
        fileState.order = fileState.sort === sort && fileState.order === 'asc' ? 'desc' : 'asc';
        fileState.sort = sort;
        reloadCurrentDirectory();
      });
    });
    byId('batchCopyButton').addEventListener('click', function () { setFileClipboard(selectedItems(), 'copy'); });
    byId('batchCutButton').addEventListener('click', function () { setFileClipboard(selectedItems(), 'move'); });
    byId('batchArchiveButton').addEventListener('click', function () { archiveItems(selectedItems()); });
    byId('batchDeleteButton').addEventListener('click', function () { deleteItems(selectedItems()); });
    byId('uploadButton').addEventListener('click', function () { byId('uploadInput').click(); });
    byId('uploadInput').addEventListener('change', function () {
      var files = Array.from(this.files || []);
      this.value = '';
      if (files.length) uploadFiles(files);
    });
    byId('cancelTransfer').addEventListener('click', function () {
      if (!currentTransfer) return;
      currentTransfer.canceled = true;
      if (currentTransfer.cancel) currentTransfer.cancel();
    });
    contextMenu.addEventListener('click', function (event) {
      var button = event.target.closest('[data-action]');
      var item = fileState.selected;
      if (!button || !item) return;
      hideContextMenu();
      var action = button.dataset.action;
      if (action === 'open') openItem(item);
      if (action === 'edit') editFile(item);
      if (action === 'download') downloadItem(item);
      if (action === 'copy') setFileClipboard(selectedItems().length ? selectedItems() : [item], 'copy');
      if (action === 'cut') setFileClipboard(selectedItems().length ? selectedItems() : [item], 'move');
      if (action === 'move') moveItems(selectedItems().length ? selectedItems() : [item]);
      if (action === 'rename') renameItem(item);
      if (action === 'archive') archiveItems(selectedItems().length ? selectedItems() : [item]);
      if (action === 'extract') extractItem(item);
      if (action === 'permissions' || action === 'properties') propertiesItem(item);
      if (action === 'delete') deleteItems(selectedItems().length ? selectedItems() : [item]);
    });
    document.addEventListener('click', function (event) { if (!contextMenu.contains(event.target)) hideContextMenu(); });
  }

  function setFilesVisible(visible) {
    if (window.matchMedia('(max-width: 980px)').matches) {
      document.body.classList.toggle('mobile-files', visible);
    } else {
      workspace.classList.toggle('files-collapsed', !visible);
    }
    setTimeout(fit, 180);
  }

  function bindLayout() {
    reconnectButton.addEventListener('click', reconnectDisconnectedChannels);
    byId('filesToggle').addEventListener('click', function () {
      if (window.matchMedia('(max-width: 980px)').matches) {
        setFilesVisible(!document.body.classList.contains('mobile-files'));
      } else {
        setFilesVisible(workspace.classList.contains('files-collapsed'));
      }
    });
    byId('closeFiles').addEventListener('click', function () { setFilesVisible(false); });
    var resizer = byId('resizer');
    resizer.addEventListener('pointerdown', function (event) {
      event.preventDefault();
      resizer.classList.add('dragging');
      resizer.setPointerCapture(event.pointerId);
    });
    resizer.addEventListener('pointermove', function (event) {
      if (!resizer.classList.contains('dragging')) return;
      var width = Math.max(320, Math.min(window.innerWidth * .6, window.innerWidth - event.clientX - 18));
      document.documentElement.style.setProperty('--file-width', width + 'px');
      fit();
    });
    function stopDrag(event) {
      if (!resizer.classList.contains('dragging')) return;
      resizer.classList.remove('dragging');
      try { resizer.releasePointerCapture(event.pointerId); } catch (_) {}
      fit();
    }
    resizer.addEventListener('pointerup', stopDrag);
    resizer.addEventListener('pointercancel', stopDrag);
    window.addEventListener('resize', fit);
    if ('ResizeObserver' in window) new ResizeObserver(fit).observe(workspace);
  }

  async function start() {
    bindFileUI();
    bindLayout();
    nodeNameEl.textContent = uuid ? '节点：加载中...' : '缺少节点';
    if (!uuid) {
      setStatus('缺少节点', 'bad');
      setFileStatus('缺少节点', 'bad');
      return;
    }
    var TerminalCtor = globalThis.Terminal || self.Terminal || window.Terminal;
    var FitAddonCtor = (globalThis.FitAddon && globalThis.FitAddon.FitAddon) ||
      (self.FitAddon && self.FitAddon.FitAddon) || (window.FitAddon && window.FitAddon.FitAddon);
    if (!TerminalCtor || !FitAddonCtor) {
      terminalEl.style.display = 'none';
      fallbackEl.style.display = 'block';
      setStatus('组件加载失败', 'bad');
      return;
    }

    nodeName = (await getClientName()) || uuid;
    nodeNameEl.textContent = '节点：' + nodeName;
    var settings = await getTerminalSettings();
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
    options.theme = Object.assign({
      foreground: '#e2e8f0', background: '#020617', cursor: '#22c55e', selectionBackground: '#334155'
    }, incomingTheme || {});
    term = new TerminalCtor(options);
    fitAddon = new FitAddonCtor();
    term.loadAddon(fitAddon);
    term.open(terminalEl);
    term.writeln('Connecting to ' + nodeName + ' ...');
    term.onData(function (data) {
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'input', input: data }));
    });
    setTimeout(fit, 0);

    connectTerminal(nodeName);
    connectFiles();
    startConnectionHeartbeat();
    updateMetrics();
    metricTimer = setInterval(updateMetrics, 3000);
  }

  document.addEventListener('visibilitychange', function () { if (!document.hidden) updateMetrics(); });
  window.addEventListener('beforeunload', function () {
    clearInterval(metricTimer);
    stopConnectionHeartbeat();
    if (ws) ws.close();
  });

  start().catch(function (err) {
    console.error(err);
    setStatus('启动失败', 'bad');
    showToast(err && err.message ? err.message : '启动失败', true);
  });
})();
