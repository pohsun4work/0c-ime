use windows::Win32::UI::Input::KeyboardAndMouse::{
    GetKeyState, MapVirtualKeyW, ToUnicode, MAPVK_VK_TO_VSC, VK_CONTROL, VK_LWIN, VK_MENU, VK_OEM_COMMA, VK_RWIN,
    VK_SHIFT,
};
use windows::Win32::UI::TextServices::{ITfKeystrokeMgr, TF_MOD_ALT, TF_MOD_CONTROL, TF_PRESERVEDKEY};
use windows_core::GUID;

/// 開關整合風格符號表的鍵名（操作規則 symbol_board）。
pub const KEY_SYMBOL_BOARD: &str = "Ctrl+Alt+,";

/// 符號表熱鍵的虛擬鍵（逗號）。
pub const VK_HOTKEY: u32 = VK_OEM_COMMA.0 as u32;

/// Ctrl＋Alt＋, 在 TSF 的保留鍵識別碼。
pub const GUID_SYMBOL_BOARD: GUID = GUID::from_u128(0x958fda44_c373_4ec0_b086_2c47ab8e64a2);

const HOTKEY: TF_PRESERVEDKEY = TF_PRESERVEDKEY { uVKey: VK_HOTKEY, uModifiers: TF_MOD_CONTROL | TF_MOD_ALT };

/// 把 Ctrl＋Alt＋, 註冊成 TSF 的保留鍵（同微軟注音）：直接用 TSF 的程式（VS Code 等 Chromium、UWP 的便籤）不把帶 Alt 的
/// 組合鍵交給 OnTestKeyDown，只有保留鍵收得到。何時註冊見 Service::sync_board_hotkey。
pub fn preserve(keystrokes: &ITfKeystrokeMgr, tid: u32) -> bool {
    let desc: Vec<u16> = "符號表".encode_utf16().collect();
    match unsafe { keystrokes.PreserveKey(tid, &GUID_SYMBOL_BOARD, &HOTKEY, &desc) } {
        Ok(()) => true,
        Err(e) => {
            crate::log::log(&format!("符號表熱鍵註冊失敗：{e}"));
            false
        }
    }
}

pub fn unpreserve(keystrokes: &ITfKeystrokeMgr) {
    let _ = unsafe { keystrokes.UnpreserveKey(&GUID_SYMBOL_BOARD, &HOTKEY) };
}

/// 這個執行緒的鍵盤排列裡 AltGr＋,（Ctrl＋Alt＋,）會不會打出字元或 dead key：會的話不註冊熱鍵，照常交給程式。
pub fn altgr_types_comma() -> bool {
    let mut state = [0u8; 256];
    state[VK_CONTROL.0 as usize] = 0x80;
    state[VK_MENU.0 as usize] = 0x80;
    let mut buf = [0u16; 8];
    unsafe { ToUnicode(VK_HOTKEY, MapVirtualKeyW(VK_HOTKEY, MAPVK_VK_TO_VSC), Some(&state), &mut buf, 0x4) != 0 }
}

/// 熱鍵之外還按著 Shift 或 Win（保留鍵比不到 Win）。
pub fn extra_modifiers() -> bool {
    [VK_SHIFT, VK_LWIN, VK_RWIN].iter().any(|k| unsafe { GetKeyState(k.0 as i32) < 0 })
}
