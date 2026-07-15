(function () {
  'use strict';

  var params = new URLSearchParams(location.search);
  var uuid = params.get('uuid') || '';
  var nodeName = uuid;
  var ws = null;
  var fileWS = null;
  var fitAddon = null;
  var term = null;
  var metricTimer = null;
  var connectionHeartbeatTimer = null;
  var requestSequence = 0;
  var pendingRequests = new Map();
  var toastTimer = null;
  var currentTransfer = null;
  var fileClipboard = null;
  var fileState = { path: '', parent: '', offset: 0, total: 0, hasMore: false, items: [], selected: null, home: '' };

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

  function filesURL() {
    return websocketURL('/api/admin/client/' + encodeURIComponent(uuid) + '/files');
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

  function connectTerminal(displayName) {
    if (ws) {
      try { ws.close(); } catch (_) {}
    }
    setStatus('连接中');
    var connection = new WebSocket(terminalURL());
    ws = connection;
    connection.binaryType = 'arraybuffer';
    connection.onopen = function () {
      if (ws !== connection) return;
      setStatus('已连接', 'ok');
      term.writeln('\r\nConnected. Waiting for agent...');
      term.focus();
      fit();
    };
    connection.onmessage = function (event) { writeTerminalMessage(event.data); };
    connection.onerror = function () { if (ws === connection) setStatus('连接错误', 'bad'); };
    connection.onclose = function () {
      if (ws !== connection) return;
      ws = null;
      setStatus('已断开', 'bad');
      if (term) term.writeln('\r\n\r\n[connection closed]');
    };
  }

  function connectFiles() {
    if (!uuid) return;
    if (fileWS) {
      try { fileWS.close(); } catch (_) {}
    }
    setFileStatus('连接中');
    var connection = new WebSocket(filesURL());
    fileWS = connection;
    connection.onopen = function () { if (fileWS === connection) setFileStatus('等待 Agent'); };
    connection.onmessage = function (event) {
      if (fileWS !== connection) return;
      var parse = function (text) {
        var message;
        try { message = JSON.parse(text); } catch (_) { return; }
        if (message.type === 'system') {
          if (!message.ok) {
            setFileStatus(message.error || '连接失败', 'bad');
            renderFileError(message.error || '文件管理连接失败');
            return;
          }
          if (message.status === 'connected') setFileStatus('已连接', 'ok');
          if (message.data && message.data.home) {
            fileState.home = message.data.home;
            setFileStatus('已连接', 'ok');
            loadDirectory(message.data.home, 0);
          }
          return;
        }
        if (message.type === 'response' && message.id) {
          var pending = pendingRequests.get(message.id);
          if (!pending) return;
          pendingRequests.delete(message.id);
          clearTimeout(pending.timer);
          if (message.ok) pending.resolve(message.data);
          else pending.reject(new Error(message.error || '文件操作失败'));
        }
      };
      if (typeof event.data === 'string') parse(event.data);
      else if (event.data instanceof Blob) event.data.text().then(parse);
    };
    connection.onerror = function () { if (fileWS === connection) setFileStatus('连接错误', 'bad'); };
    connection.onclose = function () {
      if (fileWS !== connection) return;
      fileWS = null;
      setFileStatus('已断开', 'bad');
      pendingRequests.forEach(function (pending) {
        clearTimeout(pending.timer);
        pending.reject(new Error('文件管理连接已断开'));
      });
      pendingRequests.clear();
    };
  }

  function startConnectionHeartbeat() {
    stopConnectionHeartbeat();
    connectionHeartbeatTimer = setInterval(function () {
      if (ws && ws.readyState === WebSocket.OPEN && term) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
      }
      if (fileWS && fileWS.readyState === WebSocket.OPEN) {
        fileRequest('ping', {}, 10000).catch(function () {});
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
      if (!fileWS || fileWS.readyState !== WebSocket.OPEN) {
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
      fileWS.send(JSON.stringify(request));
    });
  }

  function renderFileError(message) {
    fileRows.replaceChildren();
    var row = document.createElement('tr');
    var cell = document.createElement('td');
    cell.colSpan = 3;
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
      var data = await fileRequest('list', { path: path || '', offset: 0, limit: 1000 });
      fileState.path = data.path;
      fileState.parent = data.parent;
      fileState.offset = 0;
      fileState.total = Number(data.total || 0);
      fileState.hasMore = !!data.has_more;
      fileState.items = Array.isArray(data.items) ? data.items : [];
      fileState.selected = null;
      pathInput.value = data.path;
      renderFiles();
      requestAnimationFrame(function () {
        if (!fileListWrap) return;
        var requestedScroll = Number(viewOptions.scrollTop || 0);
        var maximumScroll = Math.max(0, fileListWrap.scrollHeight - fileListWrap.clientHeight);
        fileListWrap.scrollTop = Math.min(Math.max(0, requestedScroll), maximumScroll);
      });
      if (data.truncated) showToast('目录项目超过 1000 个，仅显示前 1000 个', true);
    } catch (err) {
      renderFileError(err.message);
      showToast(err.message, true);
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
        if (fileClipboard && fileClipboard.mode === 'move' && fileClipboard.path === item.path) row.classList.add('clipboard-cut');
        row.dataset.path = item.path;
        row.title = item.mode || '';

        var nameCell = document.createElement('td');
        var nameWrap = document.createElement('div');
        nameWrap.className = 'file-name';
        var icon = document.createElement('span');
        icon.className = 'file-icon';
        icon.textContent = item.is_dir ? '📁' : (item.is_link ? '🔗' : '📄');
        var name = document.createElement('span');
        name.textContent = item.name;
        nameWrap.append(icon, name);
        nameCell.appendChild(nameWrap);

        var sizeCell = document.createElement('td');
        sizeCell.className = 'file-meta';
        sizeCell.textContent = item.is_dir ? '--' : formatBytes(item.size);
        var modifiedCell = document.createElement('td');
        modifiedCell.className = 'file-meta';
        modifiedCell.textContent = formatModified(item.modified);
        row.append(nameCell, sizeCell, modifiedCell);
        row.addEventListener('click', function () { selectFile(item, row); });
        row.addEventListener('dblclick', function () { openItem(item); });
        row.addEventListener('contextmenu', function (event) {
          event.preventDefault();
          selectFile(item, row);
          showContextMenu(event.clientX, event.clientY, item);
        });
        fileRows.appendChild(row);
      });
    }
    updateClipboardUI();
  }

  function selectFile(item, row) {
    fileState.selected = item;
    fileRows.querySelectorAll('.selected').forEach(function (node) { node.classList.remove('selected'); });
    if (row) row.classList.add('selected');
  }

  function showContextMenu(x, y, item) {
    fileState.selected = item;
    contextMenu.hidden = false;
    var openButton = contextMenu.querySelector('[data-action="open"]');
    var downloadButton = contextMenu.querySelector('[data-action="download"]');
    openButton.textContent = item.is_dir ? '打开' : '编辑';
    downloadButton.hidden = !!item.is_dir;
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

  function updateClipboardUI() {
    pasteButton.disabled = !fileClipboard;
    pasteButton.textContent = fileClipboard ? (fileClipboard.mode === 'move' ? '粘贴（剪切）' : '粘贴（复制）') : '粘贴';
    pasteButton.title = fileClipboard ? fileClipboard.name + ' → ' + fileState.path : '请先右键复制或剪切文件';
    fileRows.querySelectorAll('.clipboard-cut').forEach(function (row) { row.classList.remove('clipboard-cut'); });
    if (fileClipboard && fileClipboard.mode === 'move') {
      Array.from(fileRows.querySelectorAll('.file-row')).forEach(function (row) {
        if (row.dataset.path === fileClipboard.path) row.classList.add('clipboard-cut');
      });
    }
  }

  function setFileClipboard(item, mode) {
    fileClipboard = { path: item.path, name: item.name, isDir: !!item.is_dir, mode: mode };
    updateClipboardUI();
    showToast((mode === 'move' ? '已剪切：' : '已复制：') + item.name);
  }

  async function pasteFileClipboard() {
    if (!fileClipboard) return;
    var operation = fileClipboard.mode === 'move' ? 'move' : 'copy';
    var scrollTop = currentFileScroll();
    pasteButton.disabled = true;
    try {
      await fileRequest(operation, { path: fileClipboard.path, destination: fileState.path }, 1800000);
      var completed = fileClipboard;
      if (operation === 'move') fileClipboard = null;
      showToast((operation === 'move' ? '移动完成：' : '复制完成：') + completed.name);
      await loadDirectory(fileState.path, fileState.offset, { scrollTop: scrollTop });
    } catch (err) {
      showToast(err.message, true);
    } finally {
      updateClipboardUI();
    }
  }

  async function openItem(item) {
    hideContextMenu();
    if (item.is_dir) { loadDirectory(item.path, 0); return; }
    await editFile(item);
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
        resolve(value);
      }
      function accept() { finish(mode === 'input' ? input.value : (mode === 'editor' ? editor.value : true)); }
      function reject() { finish(null); }
      function backdrop(event) { if (event.target === overlay) reject(); }
      function keydown(event) {
        if (event.key === 'Escape') reject();
        if (event.key === 'Enter' && mode !== 'editor' && (event.metaKey || event.ctrlKey || mode === 'confirm')) accept();
      }
      confirm.addEventListener('click', accept);
      cancel.addEventListener('click', reject);
      close.addEventListener('click', reject);
      overlay.addEventListener('click', backdrop);
      document.addEventListener('keydown', keydown);
      setTimeout(function () { if (mode === 'input') input.select(); else if (mode === 'editor') editor.focus(); }, 0);
    });
  }

  async function editFile(item) {
    try {
      var data = await fileRequest('read', { path: item.path });
      var original = data.content_base64 ? new TextDecoder().decode(base64ToBytes(data.content_base64)) : (data.content || '');
      var content = await modal({ title: '编辑 ' + item.name, message: '最大支持 2 MiB 的 UTF-8 文本文件', editor: true, value: original, confirmLabel: '保存' });
      if (content === null) return;
      await fileRequest('write', { path: item.path, content_base64: bytesToBase64(new TextEncoder().encode(content)) }, 45000);
      showToast('文件已保存');
      reloadCurrentDirectory();
    } catch (err) { showToast(err.message, true); }
  }

  async function renameItem(item) {
    var name = await modal({ title: '重命名', message: item.path, input: true, value: item.name, confirmLabel: '保存' });
    if (name === null || !name.trim() || name === item.name) return;
    try {
      await fileRequest('rename', { path: item.path, new_name: name.trim() });
      showToast('重命名完成');
      if (fileClipboard && fileClipboard.path === item.path) fileClipboard = null;
      reloadCurrentDirectory();
    } catch (err) { showToast(err.message, true); }
  }

  async function deleteItem(item) {
    var accepted = await modal({
      title: '确认删除',
      message: '确定删除“' + item.name + '”吗？非空文件夹不会被删除。',
      confirmLabel: '删除',
      danger: true
    });
    if (!accepted) return;
    var scrollTop = currentFileScroll();
    try {
      await fileRequest('delete', { path: item.path });
      if (fileClipboard && fileClipboard.path === item.path) fileClipboard = null;
      showToast('已删除');
      loadDirectory(fileState.path, fileState.offset, { scrollTop: scrollTop });
    } catch (err) { showToast(err.message, true); }
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
    var overwrite = false;
    var started;
    try {
      started = await fileRequest('upload_start', { path: fileState.path, name: file.name, size: file.size, overwrite: false });
    } catch (err) {
      if (String(err.message).indexOf('已存在') < 0) throw err;
      overwrite = await modal({ title: '覆盖文件', message: '“' + file.name + '”已经存在，是否覆盖？', confirmLabel: '覆盖', danger: true });
      if (!overwrite) return;
      started = await fileRequest('upload_start', { path: fileState.path, name: file.name, size: file.size, overwrite: true });
    }
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
    for (var i = 0; i < files.length; i++) {
      try { await uploadOne(files[i]); } catch (err) { showToast(err.message, true); break; }
    }
    reloadCurrentDirectory();
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
      await fileRequest(type, { path: fileState.path, name: name.trim() });
      showToast('创建完成');
      reloadCurrentDirectory();
    } catch (err) { showToast(err.message, true); }
  }

  function bindFileUI() {
    byId('homeButton').addEventListener('click', function () { loadDirectory(fileState.home || '', 0); });
    byId('upButton').addEventListener('click', function () { loadDirectory(fileState.parent || fileState.path, 0); });
    byId('refreshButton').addEventListener('click', reloadCurrentDirectory);
    pathInput.addEventListener('keydown', function (event) { if (event.key === 'Enter') loadDirectory(pathInput.value, 0); });
    byId('newFileButton').addEventListener('click', function () { createEntry('create'); });
    byId('newFolderButton').addEventListener('click', function () { createEntry('mkdir'); });
    pasteButton.addEventListener('click', pasteFileClipboard);
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
      if (action === 'download') downloadItem(item);
      if (action === 'copy') setFileClipboard(item, 'copy');
      if (action === 'cut') setFileClipboard(item, 'move');
      if (action === 'rename') renameItem(item);
      if (action === 'delete') deleteItem(item);
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
    if (fileWS) fileWS.close();
  });

  start().catch(function (err) {
    console.error(err);
    setStatus('启动失败', 'bad');
    showToast(err && err.message ? err.message : '启动失败', true);
  });
})();
