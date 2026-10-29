use windows::Win32::UI::Input::KeyboardAndMouse::{
    GetKeyState, GetKeyboardState, ToUnicode, VIRTUAL_KEY, VK_CONTROL, VK_LWIN, VK_MENU, VK_OEM_COMMA, VK_RWIN,
};

/// 開關整合風格符號表的鍵名（操作規則 symbol_board）。
pub const KEY_SYMBOL_BOARD: &str = "Ctrl+Alt+,";

/// 這一鍵是不是開關符號表的 Ctrl＋Alt＋,：只按著 Ctrl 與 Alt（左右都算），沒有 Shift、Win。這個組合在目前的
/// 鍵盤排列打得出字元時（AltGr 排列）不算，照舊交給應用程式。要不要攔由呼叫端看引擎回報的 symbol_board。
/// 先看最常不成立的條件：大多數鍵不是逗號，打中文逗號的 Ctrl＋, 沒按 Alt。
pub fn symbol_board_hotkey(vk: u32, lparam: isize) -> bool {
    let down = |k: VIRTUAL_KEY| unsafe { GetKeyState(k.0 as i32) < 0 };
    vk == VK_OEM_COMMA.0 as u32
        && down(VK_MENU)
        && is_symbol_board_hotkey(vk, down(VK_CONTROL), true, crate::keys::shift_down() || down(VK_LWIN) || down(VK_RWIN))
        && !types_char(vk, lparam)
}

/// [`symbol_board_hotkey`] 的按鍵判斷本身（測試用）。
pub fn is_symbol_board_hotkey(vk: u32, ctrl: bool, alt: bool, other: bool) -> bool {
    vk == VK_OEM_COMMA.0 as u32 && ctrl && alt && !other
}

/// 照目前按著的修飾鍵，這一鍵會不會打出字元（AltGr＋, 在某些排列是字元）。
fn types_char(vk: u32, lparam: isize) -> bool {
    let mut state = [0u8; 256];
    unsafe {
        if GetKeyboardState(&mut state).is_err() {
            return false;
        }
        let mut buf = [0u16; 8];
        ToUnicode(vk, ((lparam >> 16) & 0xff) as u32, Some(&state), &mut buf, 0x4) > 0
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use windows::Win32::UI::Input::KeyboardAndMouse::VK_OEM_PERIOD;

    #[test]
    fn symbol_board_hotkey_is_only_ctrl_alt_comma() {
        let comma = VK_OEM_COMMA.0 as u32;
        assert!(is_symbol_board_hotkey(comma, true, true, false));
        assert!(!is_symbol_board_hotkey(comma, true, false, false), "Ctrl＋,");
        assert!(!is_symbol_board_hotkey(comma, false, true, false), "Alt＋,");
        assert!(!is_symbol_board_hotkey(comma, true, true, true), "多按 Shift／Win");
        assert!(!is_symbol_board_hotkey(VK_OEM_PERIOD.0 as u32, true, true, false), "Ctrl＋Alt＋.");
    }
}
