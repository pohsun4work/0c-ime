// 「模型」頁（#180，ui/models.js）的判斷：node --test platforms/settings/test/
// 狀態依 0c-update models status -json（schema 1）的欄位。
'use strict';
const test = require('node:test');
const assert = require('node:assert');

globalThis.window = globalThis;
require('../ui/models.js');
const L = window.ZcModelsLogic;

const SELECT = { role: 'select', label: '選字', state: 'ready', source: 'bundled', version: 'd512-cv2-20261111', path: 'C:\\Zuin\\neural\\select',
  sha256: 'c'.repeat(64), size: 130723137, repository: 'Funmula/0c-sequence-d512', revision: '3533e34ef25896afeacc928d136ba7f35783bba7',
  url: 'https://huggingface.co/Funmula/0c-sequence-d512/tree/3533e34ef25896afeacc928d136ba7f35783bba7', license: 'MIT', equivalent: false };

test('內建模型：說明隨安裝包內附', () => {
  assert.deepStrictEqual(L.summary({ schema: 1, source: 'bundled', models: [SELECT] }), { text: '隨安裝包內附。', tone: 'ok' });
  assert.deepStrictEqual(L.summary(null), { text: '讀取中…', tone: 'muted' });
});

test('找不到內建模型：統計選字，提示重新安裝', () => {
  assert.deepStrictEqual(L.summary({ schema: 1, source: 'none', models: [] }),
    { text: '找不到內建的模型，目前使用統計選字。重新安裝可以修復。', tone: 'warn' });
});

test('SHA-256 截斷顯示，網址只放行 Hugging Face', () => {
  const sha = '0123456789abcdef'.repeat(4);
  assert.strictEqual(L.shortSha(sha), '0123456789ab…89abcdef');
  assert.ok(L.safeUrl(SELECT.url));
  for (const bad of ['http://huggingface.co/x', 'https://huggingface.co.evil.example/x', 'https://huggingface.co/x?a=1', 'javascript:alert(1)', 'https://huggingface.co/../x', null]) {
    assert.ok(!L.safeUrl(bad), String(bad));
  }
});

test('引擎狀態沿用總覽的文字；引擎沒在跑時說明', () => {
  const label = (v, fallback) => v + '/' + fallback;
  assert.strictEqual(L.stateText({ role: 'select', state: 'ready' }, label), 'ready/統計引擎');
  assert.strictEqual(L.stateText({ role: 'punct', state: 'not_installed' }, label), 'not_installed/原文提交');
  assert.strictEqual(L.stateText({ role: 'select', state: '' }, label), '輸入法未執行');
});

test('0.2.2 以前內建的模型（equivalent）：照樣顯示 Hugging Face 來源，另加一行小字', () => {
  const old = Object.assign({}, SELECT, { equivalent: true });
  assert.strictEqual(L.equivalentNote(old), '權重與公開版相同，僅匯出資訊不同。');
  assert.strictEqual(L.equivalentNote(SELECT), null);
  assert.strictEqual(L.equivalentNote(Object.assign({}, old, { url: '' })), null); // 沒有上游資訊時不說
});
