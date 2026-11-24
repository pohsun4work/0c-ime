// 總覽要不要提示「輸入法仍未全部切換」：只看 0c-update status -json，不碰畫面，node 測試直接載入
// （platforms/settings/test/engine_state_test.js）。
//
// 應該在執行的版本與 0c-update 算 update.installed 的方式相同：做過相容核心更新時是引擎指標
// （status.engine，只換引擎、外殼不變），沒有指標時是完整安裝的收據（status.installed）。
// 只拿收據比，核心更新後執行中的新引擎會被當成「舊版」，總覽一直誤報仍未切換。
window.ZcEngineState = function (status) {
  const runtime = (status && status.runtime) || {};
  const update = (status && status.update) || {};
  const expected = (status && (status.engine || status.installed)) || '';
  const shellPending = update.stage === 'pending' && !!update.target && update.target.kind === 'shell';
  const runningOld = runtime.state === 'running' && !!runtime.version && !!expected && runtime.version !== expected;
  // 核心更新只換引擎：舊引擎在組字結束、安全時機交接，不用登出。
  // 完整安裝（外殼）也不等重新登入（#253）：舊引擎閒置時由排程換新，已經開著的應用程式留在舊 DLL 照常打字。
  const coreSwitch = !!(status && status.engine);
  return {
    expected,
    running: runtime.version || '',
    runningOld,
    // 總覽要不要說明「閒置時換新，不必登出」。
    engineWaiting: shellPending || (runningOld && !coreSwitch),
  };
};
