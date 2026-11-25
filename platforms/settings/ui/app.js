// 靈犀注音設定的畫面。資料全部來自 0c-update 的 JSON 契約（docs/settings-contract.md）：
// 設定項目依契約動態產生，所以日後新增設定不必改這裡；所有文字都以 textContent 組成，不使用 innerHTML。
(function () {
  'use strict';

  const tauri = window.__TAURI__;
  const invoke = (command, args) => tauri.core.invoke(command, args);
  const PAGES = ['overview', 'mode', 'candidates', 'keyboard', 'vocab', 'models', 'update', 'troubleshoot', 'about'];
  // 0.2.5 以前的「輸入設定」（#207 拆成三頁）：舊的 0c-update、捷徑與說明寫的 --page input 導到側欄的第一頁「中英切換」。
  const PAGE_ALIAS = { input: 'mode' };
  const pageName = (name) => PAGE_ALIAS[name] || (PAGES.includes(name) ? name : 'overview');

  const state = {
    info: null, status: null, settings: null, error: null,
    busy: new Set(), checking: false, updating: null, repairAsk: false, exporting: false, summary: null,
  };
  let page = 'overview';
  let toastTimer = null;
  // 「我的詞彙」只在 features.vocab 為 true 時出現（0.2.0 起恆為 true；舊版 0c-update 沒有這個欄位時藏起來）；
  // null 表示還不知道（狀態還沒讀到）。
  const vocabEnabled = () => (state.settings || state.status
    ? !!((state.settings && state.settings.features && state.settings.features.vocab) || (state.status && state.status.features && state.status.features.vocab))
    : null);
  let vocab = null;
  let models = null; // 「模型」頁（#180，models.js）

  // ---- DOM ----
  function h(tag, attrs, ...kids) {
    const node = document.createElement(tag);
    for (const [key, value] of Object.entries(attrs || {})) {
      if (value === false || value == null) continue;
      if (key === 'class') node.className = value;
      else if (key.startsWith('on')) node.addEventListener(key.slice(2), value);
      else if (value === true) node.setAttribute(key, '');
      else node.setAttribute(key, value);
    }
    for (const kid of kids.flat()) {
      if (kid == null || kid === false) continue;
      node.append(kid.nodeType ? kid : document.createTextNode(String(kid)));
    }
    return node;
  }
  const clear = (node) => { while (node.firstChild) node.removeChild(node.firstChild); return node; };
  const row = (left, right) => h('div', { class: 'row' }, left, right);
  const labelBlock = (label, desc, id) => h('div', {}, h('div', { class: 'label', id }, label), desc ? h('div', { class: 'desc' }, desc) : null);

  async function copyText(text) {
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(text);
        toast('已複製。', 'ok');
        return;
      }
    } catch (_) { /* 退回 execCommand */ }
    const area = h('textarea', { class: 'offscreen', 'aria-hidden': 'true' });
    area.value = text;
    document.body.append(area);
    area.select();
    let ok = false;
    try { ok = document.execCommand('copy'); } catch (_) { ok = false; }
    area.remove();
    toast(ok ? '已複製。' : '無法複製，請手動選取文字。', ok ? 'ok' : 'error');
  }

  function toast(message, kind) {
    const node = document.getElementById('toast');
    node.textContent = message;
    node.className = 'toast' + (kind === 'error' ? ' error' : kind === 'warn' ? ' warn' : '');
    node.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { node.hidden = true; }, kind === 'ok' ? 5000 : 12000);
  }

  // ---- 文字 ----
  const fmtTime = (value) => (value ? new Date(value).toLocaleString('zh-TW', { hour12: false }) : '無紀錄');
  const ENGINE = {
    running: '執行中', not_running: '未執行（開始輸入時會自動啟動）', unavailable: '暫時無法查詢',
  };
  function modelLabel(value, fallback) {
    switch (value) {
      case 'ready': return '已就緒';
      case 'loading': return '載入中，暫用' + fallback;
      case 'fallback': return '模型無法使用，已退回' + fallback;
      case 'disabled_by_setting': return '依設定關閉';
      case 'not_installed': return '未附模型，使用' + fallback;
      case 'runtime_missing': return '缺少推論元件，使用' + fallback;
      case 'closed': return '已停止，使用' + fallback;
      default: return '未知（引擎未提供即時狀態）';
    }
  }
  // 引擎的 *_reason：模型退回的原因（與 0c-update status 的文字相同）。
  const MODEL_REASON = {
    missing_runtime: '（缺少系統元件：Visual C++ 執行階段）', timeout: '（模型載入逾時）', failed: '（模型程式無法啟動）',
  };
  const updateMode = () => window.ZcUpdateMode(state.status?.mode);
  const REASON = {
    rollout: '分批發布還沒輪到這台電腦', no_asset: '沒有這個平台的安裝包',
    withdrawn: '該版本已撤回', failed: '該版本在這台電腦上檢查失敗過，已暫停',
  };
  const STAGE = {
    found: '可立即下載，也可等候背景下載', downloaded: '已下載，等待準備', prepared: '已下載並核對完成，可立即安裝',
    failed: '該版本檢查失敗，已暫停',
  };
  const BLOCKED = {
    needs_consent: '等待你確認安裝', engine_busy: '正在組字，閒置時才會更新', needs_relogin: '重新登入後生效',
    install_failed: '安裝沒有完成', health_failed: '新版檢查失敗，已回復原版',
  };
  const VALUE = { neural: '神經選字', statistical: '統計引擎', off: '關閉', select: '僅供選字', auto: '自動修正（實驗）', vertical: '直排', grid: '多排', before: '游標前方', after: '游標後方' };
  const ERROR = {
    read_only: '設定檔來自較新版本的靈犀注音，這個版本只能查看，不能修改。',
    corrupt: '設定檔無法解析。可到「總覽」修復。',
    busy: '另一個程序正在修改設定，請稍後再試。',
    in_progress: '更新正在進行，完成後可再檢查。',
  };
  const errorText = (error) => (error && (ERROR[error.code] || error.message)) || '發生未知的錯誤。';

  const bytesText = (bytes) => (Math.max(0, bytes || 0) / (1024 * 1024)).toLocaleString('zh-TW', { maximumFractionDigits: 1 }) + ' MB';
  const percentOf = (op) => op.total > 0 ? Math.min(100, Math.max(0, Math.floor(op.received * 100 / op.total))) : null;
  function operationText(op) {
    switch (op.phase) {
      case 'checking': return '正在檢查更新…';
      case 'downloading': return '正在下載' + (percentOf(op) == null ? '…' : ' ' + percentOf(op) + '%');
      case 'retrying': return '連線中斷，正在重試下載…';
      case 'verifying': return '下載完成，正在核對檔案…';
      case 'preparing': return '正在準備安裝…';
      case 'installing': return '正在安裝，請稍候…';
      default: return '更新正在進行…';
    }
  }

  // 外殼裝好不等重新登入（#253）：待完成只會是新引擎還沒接手，說明由 0c-update 照實給（ShellEngine*Detail）。
  const REOPEN_NOTE = '之後開啟的應用程式直接使用新版；已經開著的要重新開啟才會換成新版。';

  function updateSummary(update) {
    if (!update) return { text: '尚無更新資訊。', tone: 'muted' };
    if (update.operation) return { text: operationText(update.operation), tone: 'warn' };
    if (update.stage === 'pending' && update.target) {
      if (update.target.kind !== 'shell') return { text: '正在確認新版引擎是否可用。', tone: 'warn' };
      return { text: update.target.version + '：' + (update.blocked?.detail || '已安裝，正在確認新版引擎') + '。', tone: 'warn' };
    }
    switch (update.state) {
      case 'never_checked': return { text: '尚未檢查更新。', tone: 'muted' };
      case 'update_available': return { text: '有新版 ' + update.target.version + '。', tone: 'warn' };
      case 'check_failed': return { text: '目前無法連線更新伺服器，目前版本仍可使用。', tone: 'warn' };
      default: {
        let text = '已是目前可用的最新版本。';
        const latest = update.latest;
        if (latest && !latest.eligible && latest.reason) {
          text += '索引上有 ' + latest.version + '，但' + (REASON[latest.reason] || '暫時不可更新') + '。';
        }
        return { text, tone: 'ok' };
      }
    }
  }

  function applyText(apply) {
    switch (apply && apply.state) {
      case 'applied': return ['已儲存，下一個按鍵起生效。', 'ok'];
      // 引擎沒在執行：外殼在下一個按鍵啟動引擎（由 0c-update 先叫起常駐引擎），啟動時讀新設定。
      case 'engine_not_running': return ['已儲存，下一個按鍵起生效。', 'ok'];
      case 'deferred_composing': return ['已儲存。你正在組字，完成後會在輸入法下次重新啟動時生效。', 'warn'];
      case 'timeout': return ['已儲存，但等候套用逾時，尚未生效。', 'warn'];
      case 'failed': return ['已儲存，但尚未套用' + (apply.detail ? '（' + apply.detail + '）' : '') + '。', 'warn'];
      default: return ['已儲存，輸入法下次啟動時生效。', 'ok'];
    }
  }

  // ---- 資料 ----
  async function refresh() {
    try {
      const [status, settings] = await Promise.all([invoke('get_status'), invoke('get_settings')]);
      state.status = status;
      state.settings = settings;
      state.error = null;
    } catch (error) {
      state.error = String(error);
    }
    if (vocabEnabled() && page === 'overview') await vocab.refreshOverview();
    if (page === 'models') await models.refresh();
    render();
  }

  async function change(key, value) {
    return changePairs([[key, value]]);
  }

  async function changePairs(pairs) {
    const focused = document.activeElement?.dataset?.focus;
    for (const [key] of pairs) state.busy.add(key);
    render();
    try {
      const result = await invoke('set_settings', { pairs, apply: true, repair: false });
      if (!result.ok) toast(errorText(result.error), 'error');
      else toast(...applyText(result.apply));
    } catch (error) {
      toast(String(error), 'error');
    }
    for (const [key] of pairs) state.busy.delete(key);
    await refresh();
    if (focused && document.activeElement === document.body) {
      const control = Array.from(document.querySelectorAll('[data-focus]')).find((node) => node.dataset.focus === focused);
      if (control && !control.disabled) control.focus();
    }
  }

  async function repair() {
    state.repairAsk = false;
    try {
      const result = await invoke('set_settings', { pairs: [['selection', 'neural']], apply: true, repair: true });
      if (!result.ok) toast(errorText(result.error), 'error');
      else toast('設定檔已修復，原檔已改名保留。', 'ok');
    } catch (error) {
      toast(String(error), 'error');
    }
    await refresh();
  }

  async function checkNow() {
    state.checking = true;
    render();
    try {
      const result = await invoke('check_update');
      if (!result.ok) toast(errorText(result.error), result.error?.code === 'in_progress' ? 'warn' : 'error');
      // 結果由頁面上的狀態框顯示，toast 只確認檢查做完，不重複同一句（#330）。
      else toast(result.update.state === 'check_failed' ? '檢查沒有完成，原因見下方。' : '檢查完成。', result.update.state === 'check_failed' ? 'warn' : 'ok');
    } catch (error) {
      toast(String(error), 'error');
    }
    state.checking = false;
    await refresh();
  }

  async function updateNow(kind) {
    const target = state.status?.update?.target;
    if (!target || state.updating) return;
    state.updating = kind;
    render();
    try {
      const result = await invoke(kind === 'download' ? 'download_update' : 'install_update', { version: target.version, sha256: target.sha256 });
      if (!result.ok) toast(errorText(result.error), result.error?.code === 'in_progress' ? 'warn' : 'error');
      else if (result.update?.blocked && result.update.stage !== 'pending' && !(kind === 'download' && result.update.stage === 'prepared')) toast(result.update.blocked.detail || '更新尚未完成，請再試一次。', 'warn');
      else if (kind === 'download') toast(result.update?.operation ? '已開始下載，進度見下方。' : result.update?.stage === 'pending' ? updateSummary(result.update).text : '下載完成，可按「立即安裝」。', 'ok');
      else toast(result.update?.stage === 'pending' ? updateSummary(result.update).text + REOPEN_NOTE : '更新作業已完成。', 'ok');
    } catch (error) {
      toast(String(error), 'error');
    } finally {
      state.updating = null;
      await refresh();
    }
  }

  let pollingStatus = false;
  async function pollUpdate() {
    if (document.hidden || pollingStatus || (page !== 'update' && !state.updating)) return;
    pollingStatus = true;
    try {
      state.status = await invoke('get_status');
      render();
    } catch (_) { /* refresh reports persistent read failures; retain the last known progress */ }
    finally { pollingStatus = false; }
  }

  async function exportSummary() {
    state.exporting = true;
    render();
    try {
      state.summary = (await invoke('export_summary')).path;
    } catch (error) {
      toast(String(error), 'error');
    }
    state.exporting = false;
    render();
  }

  // ---- 頁面 ----
  // 資料還沒到、或讀取失敗時的畫面；回傳 true 表示這一頁沒有資料可畫。
  function problem(root) {
    if (state.error) {
      root.append(
        h('div', { class: 'notice error' }, h('p', {}, '無法讀取靈犀注音的狀態：' + state.error)),
        h('button', { type: 'button', class: 'secondary', onclick: refresh }, '重試'));
    }
    if (state.status && state.settings) return false;
    if (!state.error) root.append(h('p', { class: 'lead' }, '讀取中…'));
    return true;
  }

  function fileNotices(settings) {
    const out = [];
    if (!settings) return out;
    const file = settings.file;
    if (file.state === 'corrupt') {
      out.push(h('div', { class: 'notice error' },
        h('p', {}, '設定檔無法解析，目前顯示的是預設值。'),
        state.repairAsk
          ? [h('p', {}, '修復會把原檔改名保留（settings.json.corrupt-…），再從預設值重新開始；你的個人詞庫不受影響。'),
            h('div', { class: 'actions' },
              h('button', { type: 'button', class: 'primary', onclick: repair }, '確認修復'),
              h('button', { type: 'button', class: 'secondary', onclick: () => { state.repairAsk = false; render(); } }, '取消'))]
          : h('div', { class: 'actions' },
            h('button', { type: 'button', class: 'primary', 'data-focus': 'repair', onclick: () => { state.repairAsk = true; render(); } }, '修復設定檔'))));
    }
    if (file.read_only) {
      out.push(h('div', { class: 'notice' }, h('p', {}, '設定檔來自較新版本的靈犀注音，這個版本只能查看，不能修改。')));
    }
    return out;
  }

  function overview(root) {
    root.append(h('h1', {}, '總覽'));
    if (problem(root)) return;
    const status = state.status;
    const settings = state.settings;
    // 版本、是否仍在跑舊版的判斷在 engine-state.js：核心更新後以引擎指標為準，不只看外殼的收據。
    const engine = window.ZcEngineState(status);
    root.append(h('p', { class: 'lead' }, '已安裝版本 ' + (engine.expected || '（尚無版本收據）')));
    root.append(...fileNotices(settings));
    if (settings.pending_restart) {
      // 帶到第一個尚未套用的項目所在的那一頁（#207）。
      const pending = (settings.items || []).find((item) => item.pending);
      const target = pending ? pageOfKey(pending.key) : 'mode';
      root.append(h('div', { class: 'notice' },
        h('p', {}, '有設定已儲存、尚未套用；完成組字後，輸入法重新啟動時會生效。'),
        h('div', { class: 'actions' }, h('button', { type: 'button', class: 'secondary', onclick: () => show(target) }, '查看「' + pageTitle(target) + '」'))));
    }
    const runtime = status.runtime || {};
    if (engine.engineWaiting) {
      root.append(h('div', { class: 'notice' },
        h('p', {}, engine.runningOld ? '已安裝 ' + engine.expected + '，目前輸入引擎使用 ' + engine.running + '。' : '已安裝 ' + engine.expected + '，正在確認輸入引擎。'),
        engine.runningOld ? h('p', {}, '完成組字後，閒置時會自動換上這一版的引擎，不必登出；這段時間照常可以打字。') : null,
        h('p', {}, '已經開著的應用程式要重新開啟才會換成這一版。')));
    } else if (engine.runningOld) {
      root.append(h('div', { class: 'notice' },
        h('p', {}, '新版已安裝；目前執行的輸入法仍未全部切換。'),
        h('p', {}, '已安裝 ' + engine.expected + '，目前輸入引擎使用 ' + engine.running + '。'),
        h('p', {}, '請先完成組字，等待輸入引擎重新啟動以載入新版。')));
    }
    const rows = [
      row(labelBlock('目前執行的輸入引擎'), h('div', { class: 'value' }, (ENGINE[runtime.state] || '未知') + (runtime.version ? '（' + runtime.version + '）' : ''))),
    ];
    if (runtime.state === 'running') {
      rows.push(
        row(labelBlock('神經選字'), h('div', { class: 'value' }, modelLabel(runtime.selection_state, '統計引擎') + (MODEL_REASON[runtime.selection_reason] || ''), ' ',
          h('button', { type: 'button', class: 'secondary', 'data-focus': 'models-card', onclick: () => show('models') }, '查看'))),
        row(labelBlock('補標點（Beta）'), h('div', { class: 'value' }, modelLabel(runtime.punctuation_state, '原文提交') + (MODEL_REASON[runtime.punctuation_reason] || ''))));
    }
    const summary = updateSummary(status.update);
    rows.push(
      row(labelBlock('更新'), h('div', { class: 'value' },
        h('span', { class: summary.tone === 'ok' ? 'status-ok' : summary.tone === 'warn' ? 'status-warn' : 'muted' }, summary.text), ' ',
        h('button', { type: 'button', class: 'secondary', onclick: () => show('update') }, '查看'))),
      row(labelBlock('更新方式'), h('div', { class: 'value' }, updateMode().label)));
    root.append(h('div', { class: 'card' }, ...rows));
    if (vocabEnabled()) {
      const text = vocab.overviewText();
      root.append(h('div', { class: 'card vocab-card' }, row(
        labelBlock('我的詞彙', text || '讀取中…'),
        h('button', { type: 'button', class: 'secondary', 'data-focus': 'vocab-card', onclick: () => show('vocab') }, text && text.startsWith('把') ? '開始加詞' : '管理'))));
    }
  }

  function control(item, busy) {
    const values = item.values || [];
    const toggle = values.length === 2 && values.includes('off');
    // item.disabled：另一個設定讓這一項不起作用（不用 Shift 切換時的單按時限，#319），畫成不能改；設定檔中的值不動，條件解除後照舊。
    const value = item.disabled && toggle ? 'off' : item.value;
    const disabled = !!(busy || item.disabled);
    const labelId = 'label-' + item.key;
    if (toggle) {
      const on = values.find((v) => v !== 'off');
      const checked = value === on;
      return h('button', {
        type: 'button', class: 'switch', role: 'switch', 'aria-checked': String(checked), 'aria-labelledby': labelId,
        'data-focus': item.key, disabled, onclick: () => change(item.key, checked ? 'off' : on),
      });
    }
    return h('fieldset', { class: 'choices', 'aria-labelledby': labelId, disabled },
      ...values.map((v) => h('label', {},
        h('input', {
          type: 'radio', name: item.key, value: v, checked: value === v, 'data-focus': item.key + ':' + v,
          onchange: () => change(item.key, v),
        }), (item.value_labels && item.value_labels[v]) || VALUE[v] || v)));
  }

  function itemRow(item, locked) {
    const name = h('span', {}, item.label || item.key);
    if (item.disabled && item.disabled_reason) name.append(h('span', { class: 'badge muted-badge' }, item.disabled_reason));
    else if (item.pending) name.append(h('span', { class: 'badge' }, '已儲存，尚未套用'));
    return row(
      h('div', {}, h('div', { class: 'label', id: 'label-' + item.key }, name), item.description ? h('div', { class: 'desc' }, item.description) : null,
        causeLink(item)),
      control(item, locked || state.busy.has(item.key)));
  }

  // ---- 中英切換、選字、鍵盤與符號（#207） ----
  // 各頁依這裡的順序畫（也是 Tab 順序）。不在任何一頁的新項目（較新的 0c-update 才有的設定）放在「選字」最後，
  // 所以契約新增設定時畫面不必先改；要放別頁再加進 keys。
  const SAVE_NOTE = '改動會立即儲存，並在沒有組字時讓輸入法套用。';
  const INPUT_PAGES = {
    mode: {
      title: '中英切換', lead: '用哪個鍵切換注音和英文、換程式後回到哪個模式，以及英文模式的補全。',
      // shift-tap-ms（#319，單按 Shift 的時限）是中英切換鍵的細節，接在它下面。
      keys: ['toggle-key', 'shift-tap-ms', 'start-mode', 'mode-memory', 'english-autocomplete'],
    },
    candidates: {
      title: '選字', lead: '候選字怎麼排、怎麼選、重複按的注音要不要合併，以及要不要列出近音的字（模糊音）。',
      // personal-learning（#297，記住我選的字詞）是個人偏好，排在選字項目最後、按鍵規則之前。
      keys: ['selection', 'document-context', 'redundant-keys', 'candidate-cursor', 'advance-after-select', 'candidate-layout', 'page-size', 'selection-keys', 'esc-clear', 'personal-learning', 'candidate-tab', 'candidate-space', 'candidate-enter', 'candidate-navigation'],
    },
    keyboard: {
      title: '鍵盤與符號', lead: '提交時補標點，以及 Shift＋空白、Shift＋Delete 的作用。',
      keys: ['punctuation', 'shift-space', 'shift-delete'],
    },
  };
  const pageTitle = (name) => (INPUT_PAGES[name] ? INPUT_PAGES[name].title : '輸入設定');
  function pageOfKey(key) {
    if (key.startsWith('fuzzy-')) return 'candidates';
    for (const [name, spec] of Object.entries(INPUT_PAGES)) if (spec.keys.includes(key)) return name;
    return 'candidates';
  }

  // 停用的原因在另一頁時，說明要去哪裡改，按了就過去；原因在同一頁（單按時限就在中英切換鍵下面）不必另外寫。
  function causeLink(item) {
    if (!item.disabled || !item.disabled_by) return null;
    const target = pageOfKey(item.disabled_by);
    if (target === page) return null;
    const cause = (state.settings?.items || []).find((other) => other.key === item.disabled_by);
    const what = cause ? '「' + (cause.label || cause.key) + '」' : '';
    return h('div', { class: 'desc' }, h('button', {
      type: 'button', class: 'link goto', 'data-focus': 'goto:' + item.key, onclick: () => goTo(item.disabled_by),
    }, '到「' + pageTitle(target) + '」修改' + what));
  }

  // 切到某一項所在的頁面，並把焦點放在它的開關或選中的選項上。
  function goTo(key) {
    show(pageOfKey(key));
    const section = document.getElementById('page-' + page);
    const target = section && (section.querySelector('input[name="' + key + '"]:checked') ||
      Array.from(section.querySelectorAll('[data-focus]')).find((node) => node.dataset.focus === key || node.dataset.focus.startsWith(key + ':')));
    if (target && !target.disabled) target.focus();
  }

  function settingsPage(name, root) {
    const spec = INPUT_PAGES[name];
    root.append(h('h1', {}, spec.title), h('p', { class: 'lead' }, spec.lead + SAVE_NOTE));
    if (problem(root)) return;
    const settings = state.settings;
    root.append(...fileNotices(settings));
    const locked = settings.file.read_only;
    const items = settings.items || [];
    const order = (item) => (spec.keys.includes(item.key) ? spec.keys.indexOf(item.key) : spec.keys.length);
    const rows = items.filter((item) => pageOfKey(item.key) === name && !item.key.startsWith('fuzzy-'))
      .map((item, i) => [order(item), i, item]).sort((a, b) => a[0] - b[0] || a[1] - b[1]).map(([, , item]) => itemRow(item, locked));
    if (rows.length) root.append(h('div', { class: 'card' }, ...rows));
    const fuzzy = name === 'candidates' ? items.filter((item) => item.key.startsWith('fuzzy-')) : [];
    if (fuzzy.length) {
      const enabled = fuzzy.some((item) => item.value !== 'off');
      const label = h('span', {}, '模糊音');
      if (fuzzy.some((item) => item.pending)) label.append(h('span', { class: 'badge' }, '已儲存，尚未套用'));
      root.append(h('div', { class: 'card' }, row(
        h('div', {}, h('div', { class: 'label', id: 'label-fuzzy' }, label),
          h('div', { class: 'desc' }, '開啟時一起加入 ㄗ／ㄓ、ㄘ／ㄔ、ㄙ／ㄕ、ㄣ／ㄥ 的近音候選，原音仍排在前面')),
        h('button', {
          type: 'button', class: 'switch', role: 'switch', 'aria-checked': String(enabled), 'aria-labelledby': 'label-fuzzy',
          'data-focus': 'fuzzy', disabled: locked || fuzzy.some((item) => state.busy.has(item.key)),
          onclick: () => changePairs(fuzzy.map((item) => [item.key, enabled ? 'off' : 'select'])),
        }))));
    }
    if ((settings.file.notes || []).length) {
      root.append(h('h2', {}, '注意'), h('div', { class: 'notice' }, ...settings.file.notes.map((note) => h('p', {}, note))));
    }
  }

  function update(root) {
    root.append(h('h1', {}, '更新'));
    if (problem(root)) return;
    const status = state.status;
    const view = status.update || {};
    const op = view.operation;
    const blocked = view.blocked || status.blocked;
    const summary = updateSummary(view);
    root.append(h('p', { class: 'lead' }, '目前版本 ' + (window.ZcEngineState(status).expected || '未知')));
    // 進行中的作業由下面的進度列顯示同一句話，狀態框就不再出現（#330）。
    if (!op) root.append(h('div', { class: 'notice' + (summary.tone === 'warn' ? '' : ' info') },
      h('p', {}, summary.text),
      view.target && !op ? h('p', {}, view.stage === 'pending' && view.target.kind === 'shell' ? REOPEN_NOTE
        : view.stage === 'pending' ? '正在確認新版引擎是否可用。' : (view.stage === 'found' && !updateMode().background(status.paused_until) ? '可立即下載' : STAGE[view.stage] || ('進度：' + view.stage)) + '。') : null,
      blocked && !op && view.stage !== 'pending' && !(view.stage === 'found' && ['needs_relogin', 'needs_consent'].includes(blocked.reason))
        ? h('p', {}, blocked.detail || BLOCKED[blocked.reason] || '更新暫時受阻。') : null));
    if (op) {
      const downloading = ['downloading', 'retrying'].includes(op.phase);
      const percent = downloading ? percentOf(op) : null;
      root.append(h('div', { class: 'download-progress', role: 'status' },
        h('div', { class: 'label' }, operationText(op)),
        h('progress', { max: 100, value: percent, 'aria-label': operationText(op) }),
        downloading ? h('div', { class: 'muted' }, bytesText(op.received) + ' / ' + bytesText(op.total), op.interactive ? ' · 立即下載' : ' · 背景下載') : null));
    }
    const canDownload = view.target && ['found', 'downloaded'].includes(view.stage) && (!op || (['downloading', 'retrying'].includes(op.phase) && !op.interactive));
    const canInstall = view.target && view.stage === 'prepared' && !op;
    root.append(h('div', { class: 'actions' },
      h('button', { type: 'button', class: 'secondary', disabled: state.checking || !!state.updating || !!op, 'data-focus': 'check', onclick: checkNow }, state.checking ? '檢查中…' : '檢查更新'),
      canDownload ? h('button', { type: 'button', class: 'primary', disabled: !!state.updating, 'data-focus': 'download', onclick: () => updateNow('download') }, state.updating === 'download' ? '下載中…' : op ? '立即加速下載' : '立即下載') : null,
      canInstall ? h('button', { type: 'button', class: 'primary', disabled: !!state.updating, 'data-focus': 'install', onclick: () => updateNow('install') }, state.updating === 'install' ? '安裝中…' : '立即安裝') : null));
    const facts = [
      ['上次檢查', fmtTime(view.last_check)],
      ['上次成功', fmtTime(view.last_success)],
    ];
    // 手動檢查時排程不查詢（狀態檔留著的時間不算數）。
    if (updateMode().scheduled) facts.push(['下次排定檢查', fmtTime(view.next_check)]);
    if (view.check_failures) facts.push(['連續失敗', view.check_failures + ' 次（' + (view.error || '原因不明') + '）']);
    if (view.latest) facts.push(['索引最新版本', view.latest.version]);
    if (status.paused_until) facts.push(['暫停到', fmtTime(status.paused_until)]);
    const mode = updateMode();
    facts.push(['更新方式', mode.label]);
    root.append(h('h2', {}, '詳細資訊'), h('div', { class: 'card' }, h('dl', { class: 'facts' }, ...facts.flatMap(([k, v]) => [h('dt', {}, k), h('dd', {}, v)]))));
    if (mode.note) root.append(h('p', { class: 'muted', id: 'update-mode-note' }, mode.note));
    // 「自動」的說明已經寫了裝好不必登出，其他方式才在這裡補。
    const elevation = '，可能要求系統管理員確認' + (/重新開啟/.test(mode.note) ? '' : '；裝好不必登出，已經開著的應用程式要重新開啟才會換成新版');
    root.append(h('p', { class: 'muted' }, '「檢查更新」查詢新版；「立即下載」現在下載並核對套件。完成後可按「立即安裝」' + elevation + '。'));
  }

  // 設定視窗不開外部網址（main.rs 的 local_origin），所以信箱只寫成文字，點一下整段選取方便複製（#152）。
  const SUPPORT_EMAIL = 'code@biggo.vip';
  const email = () => h('strong', { class: 'email' }, SUPPORT_EMAIL);

  function troubleshoot(root) {
    root.append(h('h1', {}, '問題排解'),
      h('p', { class: 'lead' }, '遇到問題，寫信到 ', email(), '，描述發生了什麼（在哪個程式、打了什麼、結果怎樣），並附上這裡匯出的檔案。'));
    root.append(h('div', { class: 'card' }, h('div', { class: 'row' },
      labelBlock('匯出狀態摘要', '包含版本、平台、引擎與設定的狀態、更新狀態。不含你輸入的文字、個人詞庫與使用者名稱。'),
      h('button', { type: 'button', class: 'primary', disabled: state.exporting, 'data-focus': 'export', onclick: exportSummary }, state.exporting ? '匯出中…' : '匯出'))));
    if (state.summary) {
      root.append(h('div', { class: 'notice info' },
        h('p', {}, '已匯出到：'), h('p', {}, h('code', {}, state.summary)),
        h('p', {}, '寫信到 ', email(), '，附上這個檔案，並描述發生了什麼。')));
    }
    root.append(h('h2', {}, '命令列'),
      h('p', { class: 'muted' }, '設定程式打不開、或你偏好終端機時，同樣的設定可以這樣查看與修改：'),
      h('pre', { class: 'cmd' }, '0c-update settings\n0c-update set punctuation off -apply'));
  }

  function about(root) {
    root.append(h('h1', {}, '關於'));
    const info = state.info || {};
    const status = state.status || {};
    const settings = state.settings || {};
    const facts = [
      ['這個設定視窗的版本', info.version || '未知'],
      ['已安裝版本', status.installed || '（尚無版本收據）'],
      ['目前執行的引擎', status.runtime?.version || '尚未執行'],
      ['已選用的核心版本', status.engine || status.installed || '未知'],
      ['平台', info.platform || '未知'],
      ['設定檔', settings.file ? settings.file.path : '未知'],
      ['契約版本', String(settings.schema || status.schema || '未知')],
    ];
    root.append(h('p', { class: 'lead' }, '靈犀注音設定'),
      h('div', { class: 'card' }, h('dl', { class: 'facts' }, ...facts.flatMap(([k, v]) => [h('dt', {}, k), h('dd', {}, v)]))));
  }

  // 「我的詞彙」的 DOM 只建一次（vocab.js），不在每次重畫時清掉：輸入框裡正在組字時重建會打斷輸入法。
  function vocabPage(root) {
    if (vocabEnabled() === null) { clear(root).append(h('h1', {}, '我的詞彙')); problem(root); return; }
    vocab.render(root);
  }

  const RENDER = {
    overview, vocab: vocabPage, models: (root) => models.render(root), update, troubleshoot, about,
    mode: (root) => settingsPage('mode', root), candidates: (root) => settingsPage('candidates', root), keyboard: (root) => settingsPage('keyboard', root),
  };

  function render() {
    const main = document.querySelector('main');
    const scroll = { top: main.scrollTop, left: main.scrollLeft };
    const focused = document.activeElement && document.activeElement.dataset ? document.activeElement.dataset.focus : null;
    const vocabOn = vocabEnabled();
    if (page === 'vocab' && vocabOn === false) { page = 'overview'; vocab.setActive(false); }
    document.querySelectorAll('.nav').forEach((button) => {
      if (button.dataset.page === 'vocab') button.hidden = !vocabOn;
      if (button.dataset.page === page) button.setAttribute('aria-current', 'page');
      else button.removeAttribute('aria-current');
    });
    for (const name of PAGES) {
      const section = document.getElementById('page-' + name);
      section.hidden = name !== page;
      if (name === page) RENDER[name](name === 'vocab' ? section : clear(section));
    }
    if (page === 'vocab' && vocabOn && !vocabActive) { vocabActive = true; vocab.setActive(true); }
    else if (page !== 'vocab' && vocabActive) { vocabActive = false; vocab.setActive(false); }
    // 背景更新（每 15 秒、視窗重新取得焦點）會重建控制項。區段暫時清空時被歸零的捲動位置要還原，
    // 還原焦點也不能把畫面拉回舊控制項（#268）。
    main.scrollTop = scroll.top;
    main.scrollLeft = scroll.left;
    if (focused) {
      const again = Array.from(document.querySelectorAll('[data-focus]')).find((node) => node.dataset.focus === focused);
      if (again && !again.disabled) again.focus({ preventScroll: true });
    }
  }

  let vocabActive = false;
  function show(name) {
    const next = pageName(name);
    if (next !== page) document.querySelector('main').scrollTop = 0;
    page = next;
    if (page === 'overview' && vocabEnabled()) vocab.refreshOverview().then(() => { if (page === 'overview') render(); });
    if (page === 'models') models.refresh().then(() => { if (page === 'models') render(); });
    render();
  }

  async function init() {
    // 「清除學習紀錄」（#301）在「我的詞彙」的「靈犀學到的」清單底下，由 vocab.js 決定何時顯示：
    // 0.2.12 起的 0c-update 才有（settings -json 的 features.personal_reset）。「我的詞彙」不顯示時（features.vocab 為假的舊版）就不出現。
    const personalReset = window.ZcPersonalReset({ h, clear, invoke, toast, onDone: () => vocab.afterReset() });
    vocab = window.ZcVocab({
      h, clear, invoke, toast,
      mcpPath: () => state.info && state.info.mcp_path,
      mcpPresent: () => !!(state.info && state.info.mcp_present),
      // 引擎是否在執行（status 的 runtime）；還不知道時 null。
      engineRunning: () => (state.status && state.status.runtime ? state.status.runtime.state === 'running' : null),
      personalReset,
      personalResetEnabled: () => !!(state.settings && state.settings.features && state.settings.features.personal_reset),
    });
    models = window.ZcModels({ h, invoke, toast, copyText, modelLabel, render });
    document.querySelectorAll('.nav').forEach((button) => button.addEventListener('click', () => show(button.dataset.page)));
    try { state.info = await invoke('app_info'); } catch (_) { /* 關於頁顯示未知 */ }
    // 要開的頁面由網址參數帶進來（?page=update）；舊的 input 導到中英切換，不認得的開總覽。
    page = pageName(new URLSearchParams(window.location.search).get('page'));
    if (tauri.event) {
      await tauri.event.listen('navigate', (event) => show(event.payload));
      // 詞彙版本改變（AI 助手、組字中加詞、另一個視窗）：Rust 端的 0c-update vocab watch 轉來的事件。
      await tauri.event.listen('vocab-changed', (event) => vocab.onChanged(event.payload));
    }
    render();
    refresh();
    setInterval(() => { if (!document.hidden) refresh(); }, 15000);
    setInterval(pollUpdate, 1000);
    window.addEventListener('focus', () => { refresh(); vocab.focus(); });
    // 視窗隱藏時結束 watch，再出現時重新抓並恢復。
    document.addEventListener('visibilitychange', () => vocab.visibility());
  }

  // 任何沒被處理的錯誤都顯示在畫面上，而不是留下一片空白。
  window.addEventListener('error', (event) => toast('畫面發生錯誤：' + event.message, 'error'));
  window.addEventListener('unhandledrejection', (event) => toast('發生錯誤：' + ((event.reason && event.reason.message) || event.reason), 'error'));

  init();
})();
