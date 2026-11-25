//! 組字區的背景深淺（P6.5）：組字開始、第一次拿到組字位置時，在背景讀那一帶的螢幕，取最多的顏色判斷深淺，
//! 讀完通知打字的執行緒，把組字、已選字、自動修正的底線換成白線（深底）或黑線（淺底）。
//! 讀一次螢幕要 2～20 ms（等下一個畫面），所以不在打字的執行緒上讀。每次組字都重讀：Chromium 系宿主
//! （Chrome、VS Code）同一種輸入框共用一個 ITfContext，記在 context 上會把淺色頁的結果帶到深色區塊。
use std::sync::atomic::{AtomicU8, Ordering};
use std::sync::Arc;
use std::time::Instant;

use windows::Win32::Foundation::{HWND, LPARAM, RECT, WPARAM};
use windows::Win32::Graphics::Gdi::{
    BitBlt, CreateCompatibleDC, CreateDIBSection, DeleteDC, DeleteObject, GdiFlush, GetDC, GetMonitorInfoW,
    IntersectRect, MonitorFromRect, ReleaseDC, SelectObject, BITMAPINFO, BITMAPINFOHEADER, BI_RGB, DIB_RGB_COLORS,
    MONITORINFO, MONITOR_DEFAULTTONEAREST, SRCCOPY,
};
use windows::Win32::UI::HiDpi::{GetThreadDpiAwarenessContext, SetThreadDpiAwarenessContext, DPI_AWARENESS_CONTEXT};
use windows::Win32::UI::TextServices::{
    ITfComposition, ITfContext, ITfEditSession, ITfEditSession_Impl, ITfKeyEventSink, TF_ES_ASYNCDONTCARE,
    TF_ES_READWRITE,
};
use windows::Win32::UI::WindowsAndMessaging::{PostMessageW, WM_APP};
use windows_core::{implement, ComObject, IUnknownImpl};

use crate::displayattr::Line;
use crate::log::zdebug;
use crate::service::TextService_Impl;

/// 背景讀完：通知 DLL 的訊息視窗（candidate.rs）。
pub const WM_TONE_READY: u32 = WM_APP + 65;

/// 讀的範圍：組字開頭左右各 80 px、組字那一列的高度（夾在 16～48 px 之間）。
const HALF_WIDTH: i32 = 80;
const MIN_HEIGHT: i32 = 16;
const MAX_HEIGHT: i32 = 48;

const PENDING: u8 = 0;
const WHITE: u8 = 1;
const BLACK: u8 = 2;
const UNKNOWN: u8 = 3;

/// 一次組字的背景判斷。
pub struct Probe {
    /// 讀的是哪一次組字：組字換了就重讀，舊的結果不套到新組字上。
    pub composition: ITfComposition,
    result: Arc<AtomicU8>,
    /// 組字區已經用判斷結果畫過底線（讀完的通知不必再重畫）。
    pub painted: bool,
}

impl Probe {
    /// 在背景讀 `caret` 一帶的螢幕；讀完對 `notify` 送 WM_TONE_READY。`caret` 是 GetTextExt 的螢幕座標，
    /// 背景執行緒換成打字執行緒的 DPI 感知，座標系才一致。
    pub fn start(composition: ITfComposition, caret: RECT, notify: HWND) -> Probe {
        let result = Arc::new(AtomicU8::new(PENDING));
        let out = result.clone();
        let notify = notify.0 as isize;
        let dpi = unsafe { GetThreadDpiAwarenessContext() }.0 as isize;
        crate::background(move || {
            let started = Instant::now();
            unsafe {
                let _ = SetThreadDpiAwarenessContext(DPI_AWARENESS_CONTEXT(dpi as _));
            }
            let line = unsafe { grab(area(caret)) }.as_deref().and_then(dark).map(|dark| if dark { WHITE } else { BLACK });
            out.store(line.unwrap_or(UNKNOWN), Ordering::Release);
            zdebug!("tone {:?} {} ms", line, started.elapsed().as_millis());
            if notify != 0 && line.is_some() {
                unsafe {
                    let _ = PostMessageW(Some(HWND(notify as _)), WM_TONE_READY, WPARAM(0), LPARAM(0));
                }
            }
        });
        Probe { composition, result, painted: false }
    }

    /// 判斷好就是白線或黑線，還沒讀完或讀不到沿用宿主色。
    pub fn line(&self) -> Line {
        match self.result.load(Ordering::Acquire) {
            WHITE => Line::White,
            BLACK => Line::Black,
            _ => Line::Host,
        }
    }
}

/// 要讀的螢幕範圍（還沒裁到螢幕內）。
fn area(caret: RECT) -> RECT {
    let middle = (caret.top + caret.bottom) / 2;
    let half = ((caret.bottom - caret.top).clamp(MIN_HEIGHT, MAX_HEIGHT) + 1) / 2;
    RECT { left: caret.left - HALF_WIDTH, top: middle - half, right: caret.left + HALF_WIDTH, bottom: middle + half }
}

/// 讀螢幕上的一塊（裁到所在的那個螢幕內：螢幕之間的空隙讀出來是黑的），像素是 0x00RRGGBB。
unsafe fn grab(want: RECT) -> Option<Vec<u32>> {
    let mut monitor = MONITORINFO { cbSize: std::mem::size_of::<MONITORINFO>() as u32, ..Default::default() };
    if !GetMonitorInfoW(MonitorFromRect(&want, MONITOR_DEFAULTTONEAREST), &mut monitor).as_bool() {
        return None;
    }
    let mut r = RECT::default();
    if !IntersectRect(&mut r, &want, &monitor.rcMonitor).as_bool() {
        return None;
    }
    let (w, h) = (r.right - r.left, r.bottom - r.top);
    let screen = GetDC(None);
    if screen.is_invalid() {
        return None;
    }
    let dc = CreateCompatibleDC(Some(screen));
    let info = BITMAPINFO {
        bmiHeader: BITMAPINFOHEADER {
            biSize: std::mem::size_of::<BITMAPINFOHEADER>() as u32,
            biWidth: w,
            biHeight: -h,
            biPlanes: 1,
            biBitCount: 32,
            biCompression: BI_RGB.0,
            ..Default::default()
        },
        ..Default::default()
    };
    let mut bits = std::ptr::null_mut();
    let mut pixels = None;
    if let Ok(bitmap) = CreateDIBSection(Some(dc), &info, DIB_RGB_COLORS, &mut bits, None, 0) {
        let old = SelectObject(dc, bitmap.into());
        if BitBlt(dc, 0, 0, w, h, Some(screen), r.left, r.top, SRCCOPY).is_ok() {
            let _ = GdiFlush();
            pixels = Some(std::slice::from_raw_parts(bits as *const u32, (w * h) as usize).to_vec());
        }
        SelectObject(dc, old);
        let _ = DeleteObject(bitmap.into());
    }
    let _ = DeleteDC(dc);
    ReleaseDC(None, screen);
    pixels
}

/// 最多的顏色（每通道取高 4 bit 分桶，取桶的中間值）亮度低於一半就是深底。字只佔少數，不影響結果。
fn dark(pixels: &[u32]) -> Option<bool> {
    let mut counts = vec![0u32; 1 << 12];
    for p in pixels {
        counts[(((p >> 12) & 0xF00) | ((p >> 8) & 0xF0) | ((p >> 4) & 0xF)) as usize] += 1;
    }
    let (bucket, &n) = counts.iter().enumerate().max_by_key(|(_, n)| **n)?;
    if n == 0 {
        return None;
    }
    let channel = |shift: usize| ((bucket >> shift) & 0xF) as u32 * 16 + 8;
    let (r, g, b) = (channel(8), channel(4), channel(0));
    Some(299 * r + 587 * g + 114 * b < 128_000)
}

/// 讀完時在打字的執行緒上要一次 edit session，用當下的組字重設底線（不動文字與游標）。
/// 持有 service 的 COM 參考直到 edit session 完成。
#[implement(ITfEditSession)]
struct Restyle {
    _keep_alive: ITfKeyEventSink,
    service: *const TextService_Impl,
}
impl Drop for Restyle {
    fn drop(&mut self) {
        crate::object_destroyed();
    }
}
impl ITfEditSession_Impl for Restyle_Impl {
    fn DoEditSession(&self, ec: u32) -> windows_core::Result<()> {
        crate::guarded("Restyle", || {
            unsafe { (*self.service).restyle_in_edit(ec) };
            Ok(())
        })
    }
}
pub fn restyle(service: &TextService_Impl, ctx: &ITfContext, client: u32) {
    crate::object_created();
    let edit: ITfEditSession =
        ComObject::new(Restyle { _keep_alive: service.to_interface(), service }).into_interface();
    unsafe {
        let _ = ctx.RequestEditSession(client, &edit, TF_ES_ASYNCDONTCARE | TF_ES_READWRITE);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_most_common_color_decides_and_text_does_not() {
        // 11-25 實測：VS Code #101010～#303030 → 白線；記事本、Chrome 淺色頁 #F0F0F0、選取反白 #90C0E0 → 黑線。
        let with_text = |bg: u32, fg: u32| {
            let mut v = vec![bg; 60];
            v.extend(vec![fg; 40]);
            v
        };
        for bg in [0x101010, 0x1E1E1E, 0x303030, 0x202020] {
            assert_eq!(dark(&with_text(bg, 0xD4D4D4)), Some(true), "{bg:06X}");
        }
        for bg in [0xF0F0F0, 0xFFFFFF, 0x90C0E0] {
            assert_eq!(dark(&with_text(bg, 0x000000)), Some(false), "{bg:06X}");
        }
        // 分桶：差一點點的顏色（反鋸齒、漸層）算同一種，合起來比字多。
        let mut v = vec![0x1E1E1E; 30];
        v.extend(vec![0x1F1F1F; 30]);
        v.extend(vec![0xFFFFFF; 40]);
        assert_eq!(dark(&v), Some(true));
        assert_eq!(dark(&[]), None);
    }

    #[test]
    fn the_area_is_centred_on_the_composition_start() {
        let a = area(RECT { left: 500, top: 100, right: 501, bottom: 120 });
        assert_eq!((a.left, a.right), (420, 580));
        assert_eq!((a.top, a.bottom), (100, 120));
        // 太矮（只有一條線的游標）或太高（整個輸入框）都夾住。
        let a = area(RECT { left: 0, top: 100, right: 1, bottom: 102 });
        assert_eq!(a.bottom - a.top, MIN_HEIGHT);
        let a = area(RECT { left: 0, top: 0, right: 1, bottom: 400 });
        assert_eq!((a.top, a.bottom), (176, 224));
    }
}
