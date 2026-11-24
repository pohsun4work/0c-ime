// 「模型」頁（#180）：安裝包內建的選字與補標點模型是哪一份（來源、版本與雜湊）。只讀，資料來自
// 0c-update models status -json。
//
// 上半是不碰畫面的判斷（ZcModelsLogic，node 測試直接載入：test/models_test.js），下半是畫面。
(function () {
  'use strict';

  const ROLE_FALLBACK = { select: '統計引擎', punct: '原文提交' };
  const HF = /^https:\/\/huggingface\.co\/[A-Za-z0-9._\/-]+$/;

  const shortRev = (rev) => (rev || '').slice(0, 8);

  // 頁首的狀態一句話。沒有內建模型是安裝損毀。
  function summary(status) {
    if (!status) return { text: '讀取中…', tone: 'muted' };
    if (status.source === 'bundled') return { text: '隨安裝包內附。', tone: 'ok' };
    return { text: '找不到內建的模型，目前使用統計選字。重新安裝可以修復。', tone: 'warn' };
  }

  function shortSha(sha) {
    return sha && sha.length > 20 ? sha.slice(0, 12) + '…' + sha.slice(-8) : (sha || '');
  }

  // 0.2.2 以前官網套件內建的模型：不是 Hugging Face 那一份檔案，但權重相同、只差匯出資訊（status 的 equivalent）。
  function equivalentNote(model) {
    return model && model.equivalent && model.url ? '權重與公開版相同，僅匯出資訊不同。' : null;
  }

  const safeUrl = (url) => typeof url === 'string' && HF.test(url) && !url.includes('..');

  // 引擎回報的狀態（同總覽的 modelLabel）；引擎沒在跑時是空字串。
  function stateText(model, modelLabel) {
    if (!model.state) return '輸入法未執行';
    return modelLabel(model.state, ROLE_FALLBACK[model.role] || '統計引擎');
  }

  const logic = { summary, equivalentNote, shortSha, safeUrl, stateText };
  window.ZcModelsLogic = logic;

  // ---- 畫面 ----
  // ctx：h、invoke、toast、copyText、modelLabel、render()（重畫整個畫面）。
  window.ZcModels = function (ctx) {
    const { h, invoke, toast, copyText, modelLabel } = ctx;
    const m = { status: null, error: null };

    async function refresh() {
      try {
        const result = await invoke('models_command');
        if (result && result.ok === false) m.error = (result.error && result.error.message) || '無法讀取模型狀態。';
        else {
          m.status = result;
          m.error = null;
        }
      } catch (error) {
        m.error = String(error);
      }
    }

    function openLink(url) {
      invoke('open_url', { url }).catch((error) => toast('無法開啟瀏覽器：' + error + '。網址：' + url, 'error'));
    }

    function shaRow(sha) {
      return h('span', { class: 'sha' },
        h('code', { class: 'mono', title: sha }, shortSha(sha)),
        h('button', { type: 'button', class: 'link', 'data-focus': 'sha:' + sha.slice(0, 12), 'aria-label': '複製 SHA-256', onclick: () => copyText(sha) }, '複製'));
    }

    function modelCard(model) {
      const facts = [];
      if (model.version) facts.push(['版本', model.version]);
      if (model.path) facts.push(['路徑', h('span', { class: 'mono path' }, model.path)]);
      if (model.sha256) facts.push(['SHA-256', shaRow(model.sha256)]);
      if (model.url && safeUrl(model.url)) {
        facts.push(['來源', h('button', {
          type: 'button', class: 'link ext', title: model.url, 'data-focus': 'hf:' + model.role, onclick: () => openLink(model.url),
        }, 'Hugging Face ' + (model.repository || '') + (model.revision ? ' @ ' + shortRev(model.revision) : ''), h('span', { 'aria-hidden': 'true' }, ' ↗'))]);
      }
      if (model.license) facts.push(['授權', model.license]);
      const note = equivalentNote(model);
      return h('div', { class: 'card model-card' },
        h('div', { class: 'row' },
          h('div', { class: 'label' }, model.label || model.role),
          h('div', { class: 'value' }, h('span', { class: model.state === 'ready' ? 'status-ok' : 'muted' }, stateText(model, modelLabel)))),
        facts.length ? h('dl', { class: 'facts' }, ...facts.flatMap(([k, v]) => [h('dt', {}, k), h('dd', {}, v)])) : null,
        note ? h('p', { class: 'muted small model-note' }, note) : null);
    }

    function render(root) {
      root.append(h('h1', {}, '模型'), h('p', { class: 'lead' }, '神經選字與補標點使用的模型。'));
      if (!m.status) {
        if (m.error) {
          root.append(h('div', { class: 'notice error' }, h('p', {}, '無法讀取模型狀態：' + m.error)),
            h('button', { type: 'button', class: 'secondary', onclick: async () => { await refresh(); ctx.render(); } }, '重試'));
        } else root.append(h('p', { class: 'muted' }, '讀取中…'));
        return;
      }
      const sum = summary(m.status);
      root.append(h('div', { class: 'notice' + (sum.tone === 'warn' ? '' : ' info'), role: 'status' }, h('p', {}, sum.text)));
      for (const model of m.status.models || []) root.append(modelCard(model));
    }

    return { render, refresh };
  };
})();
