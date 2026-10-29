use super::{measure, Theme};
use std::cell::RefCell;
use windows::Win32::Foundation::{COLORREF, HWND, RECT};
use windows::Win32::Graphics::Gdi::{
    DrawTextW, FillRect, FrameRect, GetDC, ReleaseDC, SelectObject, SetTextColor, TextOutW, DT_CENTER, DT_NOPREFIX,
    DT_SINGLELINE, DT_VCENTER, HDC, HFONT, HGDIOBJ,
};
use windows::Win32::UI::WindowsAndMessaging::GetClientRect;
use zuin_win_common::{
    candidate_key, header_height, header_lines, large_heading, notice_row, CandidateCell, UiMessage,
};

const HINT: &str = "Enter 選取 · 方向鍵移動 · Esc 關閉";

/// 整合風格的符號表（kind "keyboard"）：鍵盤形狀的格子，每格左上角小字是鍵名、下面是符號，反白那一格用選取色。
pub(super) fn is_keyboard(m: &UiMessage) -> bool {
    m.kind == "keyboard" && !m.rows.is_empty()
}

fn utf16(s: &str) -> Vec<u16> {
    s.encode_utf16().collect()
}

/// 各格的位置：照 rows 每列的鍵數由左而右排、照起點錯開。key 是鍵寬、row 是列高、top 是第一列的上緣。
fn keyboard_cells(m: &UiMessage, key: i32, row: i32, pad: i32, top: i32) -> Vec<CandidateCell> {
    let mut items = m.candidates.iter();
    let mut cells = Vec::new();
    for (r, keys) in m.rows.iter().enumerate() {
        let left = pad + (keys.offset * key as f32).round() as i32;
        for (c, candidate) in items.by_ref().take(keys.keys).enumerate() {
            cells.push(CandidateCell { id: candidate.id, left: left + c as i32 * key, top: top + r as i32 * row, width: key, height: row });
        }
    }
    cells
}

/// 鍵盤的寬度（以鍵寬計）：最長那一列的右緣。
fn keyboard_width(m: &UiMessage) -> f32 {
    m.rows.iter().map(|r| r.offset + r.keys as f32).fold(0.0, f32::max)
}

thread_local! {
    /// 最寬的符號：一頁的符號不變，只移反白時不必每次重量（字型、這一頁的符號都一樣才沿用）。
    static WIDEST: RefCell<Option<(isize, String, i32)>> = const { RefCell::new(None) };
}

fn widest(m: &UiMessage, hdc: HDC, font: HFONT) -> i32 {
    let page: String = m.candidates.iter().map(|c| c.text.as_str()).collect();
    WIDEST.with(|cell| {
        let mut cached = cell.borrow_mut();
        if let Some((f, p, w)) = cached.as_ref() {
            if *f == font.0 as isize && *p == page {
                return *w;
            }
        }
        let w = m.candidates.iter().map(|c| measure(hdc, font, &utf16(&c.text))).max().unwrap_or(0);
        *cached = Some((font.0 as isize, page, w));
        w
    })
}

/// 鍵寬與列高：鍵寬放得下最寬的符號（至少一個半列高），整個鍵盤超過 width 時縮小；列高是鍵名的小字加符號的一般字。
fn key_size(m: &UiMessage, hdc: HDC, t: &Theme, width: i32) -> (i32, i32) {
    let fit = ((width - t.pad * 2) as f32 / keyboard_width(m).max(1.0)) as i32;
    ((widest(m, hdc, t.font) + t.pad * 2).max(t.row * 3 / 2).min(fit).max(1), t.small_row + t.row)
}

/// 格子的位置，繪製與滑鼠命中共用；width 是候選窗的寬度。
fn cells(m: &UiMessage, hdc: HDC, t: &Theme, width: i32) -> Vec<CandidateCell> {
    let (key, row) = key_size(m, hdc, t, width);
    keyboard_cells(m, key, row, t.pad, t.pad + header_height(m, t.small_row))
}

/// 候選窗的大小，寬度不超過 max_width（螢幕的工作區）。
pub(super) fn size(m: &UiMessage, hdc: HDC, t: &Theme, max_width: i32) -> (i32, i32) {
    let (key, row) = key_size(m, hdc, t, max_width);
    let texts = header_lines(m)
        .iter()
        .map(|h| measure(hdc, if large_heading(m, h) { t.font } else { t.small }, &utf16(h)) + t.pad * 3)
        .chain([measure(hdc, t.small, &utf16(HINT)) + t.pad * 2])
        .max()
        .unwrap_or(0);
    let width = ((keyboard_width(m) * key as f32).ceil() as i32 + t.pad * 2).max(texts).min(max_width);
    (width, 2 * t.pad + header_height(m, t.small_row) + row * m.rows.len() as i32 + t.small_row)
}

pub(super) unsafe fn draw(m: &UiMessage, hdc: HDC, width: i32, t: &Theme) {
    let mut y = t.pad;
    for heading in header_lines(m) {
        let notice = !m.notice.is_empty() && heading == m.notice;
        let (color, font, height) = if notice {
            (t.colors.notice, t.font, notice_row(t.small_row))
        } else if large_heading(m, heading) {
            (t.colors.text, t.font, notice_row(t.small_row))
        } else {
            (t.colors.hint, t.small, t.small_row)
        };
        SetTextColor(hdc, COLORREF(color));
        let old = SelectObject(hdc, HGDIOBJ(font.0));
        let _ = TextOutW(hdc, t.pad, y, &utf16(heading));
        SelectObject(hdc, old);
        y += height;
    }
    let cells = cells(m, hdc, t, width);
    for (i, cell) in cells.iter().enumerate() {
        let selected = i as i64 == m.highlight;
        let rect = RECT { left: cell.left + 1, top: cell.top + 1, right: cell.left + cell.width - 1, bottom: cell.top + cell.height - 1 };
        if selected {
            FillRect(hdc, &rect, t.highlight);
        }
        FrameRect(hdc, &rect, t.border);
        let old = SelectObject(hdc, HGDIOBJ(t.small.0));
        SetTextColor(hdc, COLORREF(if selected { t.colors.selected_text } else { t.colors.hint }));
        let _ = TextOutW(hdc, rect.left + t.pad / 2, rect.top + 1, &utf16(&candidate_key(i, &m.candidates[i])));
        SelectObject(hdc, HGDIOBJ(t.font.0));
        SetTextColor(hdc, COLORREF(if selected { t.colors.selected_text } else { t.colors.text }));
        let mut symbol = RECT { top: rect.top + t.small_row, ..rect };
        DrawTextW(hdc, &mut utf16(&m.candidates[i].text), &mut symbol, DT_CENTER | DT_VCENTER | DT_SINGLELINE | DT_NOPREFIX);
        SelectObject(hdc, old);
    }
    let bottom = cells.last().map(|c| c.top + c.height).unwrap_or(y);
    let old = SelectObject(hdc, HGDIOBJ(t.small.0));
    SetTextColor(hdc, COLORREF(t.colors.hint));
    let _ = TextOutW(hdc, t.pad, bottom, &utf16(HINT));
    SelectObject(hdc, old);
}

/// 滑鼠點在 (x, y) 的那一格。
pub(super) unsafe fn hit(hwnd: HWND, m: &UiMessage, t: &Theme, x: i32, y: i32) -> Option<i64> {
    let mut rc = RECT::default();
    let _ = GetClientRect(hwnd, &mut rc);
    let hdc = GetDC(Some(hwnd));
    let id = cells(m, hdc, t, rc.right - rc.left).iter().find(|cell| cell.contains(x, y)).map(|cell| cell.id);
    ReleaseDC(Some(hwnd), hdc);
    id
}

#[cfg(test)]
mod tests {
    use super::*;
    use zuin_win_common::{KeyRow, UiCandidate};

    #[test]
    fn keyboard_cells_follow_the_staggered_rows() {
        // 整合風格的符號表：13／13／11／10 鍵，各列照美式鍵盤錯開 0、1.5、1.75、2.25 鍵寬。
        let rows = [(13, 0.0), (13, 1.5), (11, 1.75), (10, 2.25)];
        let m = UiMessage {
            candidates: (0..47).map(|id| UiCandidate { id, text: "○".into(), ..Default::default() }).collect(),
            rows: rows.iter().map(|&(keys, offset)| KeyRow { keys, offset }).collect(),
            kind: "keyboard".into(),
            ..Default::default()
        };
        assert!(is_keyboard(&m));
        let cells = keyboard_cells(&m, 40, 50, 8, 20);
        assert_eq!(cells.len(), 47);
        for (index, left, top) in [(0, 8, 20), (12, 488, 20), (13, 68, 70), (26, 78, 120), (37, 98, 170), (46, 458, 170)] {
            assert_eq!((cells[index].left, cells[index].top, cells[index].id), (left, top, index as i64), "{index}");
        }
        assert!(cells[13].contains(70, 75) && !cells[13].contains(60, 75));
        assert_eq!(keyboard_width(&m), 14.5);
        // 少於列數的項目不會越界。
        let short = UiMessage { candidates: m.candidates[..20].to_vec(), ..m.clone() };
        assert_eq!(keyboard_cells(&short, 40, 50, 8, 20).len(), 20);
    }
}
