// 更新方式的名稱與說明（#244，ui/update-mode.js）：node --test platforms/settings/test/
// 由 scripts/test_release.py 在有 node 時執行。
'use strict';
const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');

globalThis.window = globalThis;
require('../ui/update-mode.js');
const mode = window.ZcUpdateMode;

test('三種方式的名稱', () => {
  assert.strictEqual(mode('auto').label, '自動（建議）');
  assert.strictEqual(mode('ask').label, '下載後詢問');
  assert.strictEqual(mode('manual').label, '手動檢查');
});

test('「自動」照實說換輸入法本身要確認權限', () => {
  const note = mode('auto').note;
  assert.match(note, /背景完成/);
  assert.match(note, /系統管理員權限/);
  assert.match(note, /確認/);
  // #253：裝好不必登出，只有已經開著的程式要重開。
  assert.match(note, /不必登出/);
  assert.match(note, /重新開啟/);
  assert.doesNotMatch(note, /登出再登入|重新登入/);
});

test('手動檢查：排程不查詢也不下載', () => {
  const manual = mode('manual');
  assert.strictEqual(manual.scheduled, false);
  assert.strictEqual(manual.background(), false);
  assert.match(manual.note, /不檢查也不下載/);
});

test('自動與下載後詢問會在背景下載，暫停中不會', () => {
  for (const value of ['auto', 'ask']) {
    assert.strictEqual(mode(value).scheduled, true);
    assert.strictEqual(mode(value).background(), true);
    assert.strictEqual(mode(value).background('2026-11-07T00:00:00Z'), false);
  }
});

test('不認得的值（新版寫的、或沒有回報）照實顯示，不猜說明', () => {
  assert.deepStrictEqual([mode('future').label, mode('future').note], ['future', '']);
  assert.deepStrictEqual([mode(undefined).label, mode(undefined).note], ['未知', '']);
});

test('index.html 在 app.js 之前載入 update-mode.js', () => {
  const html = fs.readFileSync(path.join(__dirname, '../ui/index.html'), 'utf8');
  assert.ok(html.indexOf('src="update-mode.js"') > 0 && html.indexOf('src="update-mode.js"') < html.indexOf('src="app.js"'));
});
