//! 組字區四種顯示屬性：一般細實線、使用者選字粗實線、自動修正點線、加詞標示（#123）的反白粗線。
//! 前三種的底線先沿用宿主文字色，背景深淺判斷好後換成白線或黑線版（見 Line、tone.rs）；
//! 標示用系統的選取色（COLOR_HIGHLIGHT／COLOR_HIGHLIGHTTEXT），適應深色與高對比主題。
use std::cell::Cell;

use windows::Win32::Foundation::COLORREF;
use windows::Win32::Graphics::Gdi::{GetSysColor, COLOR_HIGHLIGHT, COLOR_HIGHLIGHTTEXT};
use windows::Win32::UI::TextServices::{
    IEnumTfDisplayAttributeInfo, IEnumTfDisplayAttributeInfo_Impl, ITfDisplayAttributeInfo,
    ITfDisplayAttributeInfo_Impl, TF_ATTR_CONVERTED, TF_ATTR_INPUT, TF_ATTR_TARGET_CONVERTED,
    TF_CT_COLORREF, TF_DA_COLOR, TF_DA_COLOR_0, TF_DISPLAYATTRIBUTE, TF_LS_DOT, TF_LS_SOLID,
};
use windows_core::{implement, ComObject, BSTR, GUID};

use crate::guids::{
    GUID_DISPLAY_ATTR_CORRECTED, GUID_DISPLAY_ATTR_CORRECTED_BLACK, GUID_DISPLAY_ATTR_CORRECTED_WHITE,
    GUID_DISPLAY_ATTR_INPUT, GUID_DISPLAY_ATTR_INPUT_BLACK, GUID_DISPLAY_ATTR_INPUT_WHITE, GUID_DISPLAY_ATTR_LOCKED,
    GUID_DISPLAY_ATTR_LOCKED_BLACK, GUID_DISPLAY_ATTR_LOCKED_WHITE, GUID_DISPLAY_ATTR_MARKED,
};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Kind {
    Input,
    Locked,
    Corrected,
    /// 組字中用 Shift＋←／→ 標示、要加入詞庫的範圍（核心樣式 marked，#123）。
    Marked,
}
impl Kind {
    fn attributes(self) -> TF_DISPLAYATTRIBUTE {
        let color = |index| TF_DA_COLOR {
            r#type: TF_CT_COLORREF,
            Anonymous: TF_DA_COLOR_0 { cr: COLORREF(unsafe { GetSysColor(index) }) },
        };
        if self == Self::Marked {
            // 反白（系統選取色）加上選取色的粗底線：會畫背景的宿主（記事本、Word）反白，只畫底線的宿主
            // （WPF、Chromium）也看得出和已選字（黑色粗線）不同。顏色在查詢時才讀系統設定，跟著主題與高對比；
            // 用 COLORREF 而不是 TF_CT_SYSCOLOR：實測 Edge 與 WPF 對 TF_CT_SYSCOLOR 什麼都不畫，
            // 換成 COLORREF 後底線才變成選取色（兩者都不畫背景色）。底線用點線：WPF、Chromium 只看底線，
            // 實心粗線跟平常組字的底線只差粗細，點線一眼分得出來（2026-10-18 使用者在 A/B 截圖中選定；
            // 記事本、Word 的反白蓋過底線，看起來不變）。自動修正也是點線，但那是黑色細線。
            return TF_DISPLAYATTRIBUTE {
                crText: color(COLOR_HIGHLIGHTTEXT),
                crBk: color(COLOR_HIGHLIGHT),
                crLine: color(COLOR_HIGHLIGHT),
                lsStyle: TF_LS_DOT,
                fBoldLine: true.into(),
                bAttr: TF_ATTR_TARGET_CONVERTED,
                ..Default::default()
            };
        }
        TF_DISPLAYATTRIBUTE {
            lsStyle: if self == Self::Corrected {
                TF_LS_DOT
            } else {
                TF_LS_SOLID
            },
            fBoldLine: (self == Self::Locked).into(),
            bAttr: if self == Self::Locked {
                TF_ATTR_CONVERTED
            } else {
                TF_ATTR_INPUT
            },
            ..Default::default()
        }
    }
}

/// 組字、已選字、自動修正的底線顏色（P6.5）。沒指定顏色時宿主沿用文字色，文字畫成透明、另一層才畫字的
/// 輸入框（VS Code 的 Claude Code）就看不到底線；判斷出背景深淺後，深底用白線、淺底用黑線。
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum Line {
    /// 還沒判斷好（或判斷不了）：沿用宿主文字色。
    #[default]
    Host,
    White,
    Black,
}

/// 一種顯示屬性：樣式加上底線顏色。加詞標示已有自己的顏色，一律是 Host。
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Attr {
    kind: Kind,
    line: Line,
}
pub const ATTRS: [Attr; 10] = {
    use Kind::*;
    use Line::*;
    const fn a(kind: Kind, line: Line) -> Attr {
        Attr { kind, line }
    }
    [
        a(Input, Host),
        a(Locked, Host),
        a(Corrected, Host),
        a(Marked, Host),
        a(Input, White),
        a(Locked, White),
        a(Corrected, White),
        a(Input, Black),
        a(Locked, Black),
        a(Corrected, Black),
    ]
};
impl Attr {
    pub fn new(kind: Kind, line: Line) -> Attr {
        let line = if kind == Kind::Marked { Line::Host } else { line };
        Attr { kind, line }
    }
    /// 在 ATTRS 的位置（atom 陣列依此存取）。
    pub fn index(self) -> usize {
        self.kind as usize
            + match self.line {
                Line::Host => 0,
                Line::White => 4,
                Line::Black => 7,
            }
    }
    pub fn guid(self) -> GUID {
        use Kind::*;
        use Line::*;
        match (self.kind, self.line) {
            (Input, Host) => GUID_DISPLAY_ATTR_INPUT,
            (Locked, Host) => GUID_DISPLAY_ATTR_LOCKED,
            (Corrected, Host) => GUID_DISPLAY_ATTR_CORRECTED,
            (Input, White) => GUID_DISPLAY_ATTR_INPUT_WHITE,
            (Locked, White) => GUID_DISPLAY_ATTR_LOCKED_WHITE,
            (Corrected, White) => GUID_DISPLAY_ATTR_CORRECTED_WHITE,
            (Input, Black) => GUID_DISPLAY_ATTR_INPUT_BLACK,
            (Locked, Black) => GUID_DISPLAY_ATTR_LOCKED_BLACK,
            (Corrected, Black) => GUID_DISPLAY_ATTR_CORRECTED_BLACK,
            (Marked, _) => GUID_DISPLAY_ATTR_MARKED,
        }
    }
    fn description(self) -> &'static str {
        use Kind::*;
        use Line::*;
        match (self.kind, self.line) {
            (Input, Host) => "靈犀注音：組字",
            (Locked, Host) => "靈犀注音：已選字",
            (Corrected, Host) => "靈犀注音：自動修正",
            (Input, White) => "靈犀注音：組字（深色背景）",
            (Locked, White) => "靈犀注音：已選字（深色背景）",
            (Corrected, White) => "靈犀注音：自動修正（深色背景）",
            (Input, Black) => "靈犀注音：組字（淺色背景）",
            (Locked, Black) => "靈犀注音：已選字（淺色背景）",
            (Corrected, Black) => "靈犀注音：自動修正（淺色背景）",
            (Marked, _) => "靈犀注音：加入詞庫的範圍",
        }
    }
    fn attributes(self) -> TF_DISPLAYATTRIBUTE {
        let mut da = self.kind.attributes();
        let rgb = match self.line {
            Line::Host => return da,
            Line::White => 0x00FF_FFFF,
            Line::Black => 0,
        };
        da.crLine = TF_DA_COLOR { r#type: TF_CT_COLORREF, Anonymous: TF_DA_COLOR_0 { cr: COLORREF(rgb) } };
        da
    }
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct StyledRange {
    pub start: i32,
    pub end: i32,
    pub kind: Kind,
}
/// TSF 位移是 UTF-16 單位，不是 Unicode scalar 或 UTF-8 bytes。
pub fn style_ranges(segments: &[crate::session::Segment]) -> Option<Vec<StyledRange>> {
    let mut ranges = Vec::new();
    let mut start = 0i32;
    for segment in segments {
        let length = i32::try_from(segment.text.encode_utf16().count()).ok()?;
        let end = start.checked_add(length)?;
        if length > 0 {
            let kind = match segment.style.as_str() {
                "locked" => Kind::Locked,
                "corrected" => Kind::Corrected,
                "marked" => Kind::Marked,
                _ => Kind::Input,
            };
            ranges.push(StyledRange { start, end, kind });
        }
        start = end;
    }
    Some(ranges)
}

#[implement(ITfDisplayAttributeInfo)]
pub struct InputAttribute {
    attr: Attr,
}

impl InputAttribute {
    pub fn new(attr: Attr) -> InputAttribute {
        crate::object_created();
        InputAttribute { attr }
    }
}

impl Drop for InputAttribute {
    fn drop(&mut self) {
        crate::object_destroyed();
    }
}

impl ITfDisplayAttributeInfo_Impl for InputAttribute_Impl {
    fn GetGUID(&self) -> windows_core::Result<GUID> {
        crate::guarded("GetGUID", || Ok(self.attr.guid()))
    }
    fn GetDescription(&self) -> windows_core::Result<BSTR> {
        crate::guarded("GetDescription", || Ok(BSTR::from(self.attr.description())))
    }
    fn GetAttributeInfo(&self, pda: *mut TF_DISPLAYATTRIBUTE) -> windows_core::Result<()> {
        crate::guarded("GetAttributeInfo", || {
            if pda.is_null() {
                return Err(windows::Win32::Foundation::E_POINTER.into());
            }
            let da = self.attr.attributes();
            unsafe { pda.write(da) };
            Ok(())
        })
    }
    fn SetAttributeInfo(&self, _pda: *const TF_DISPLAYATTRIBUTE) -> windows_core::Result<()> {
        crate::guarded("SetAttributeInfo", || Ok(()))
    }
    fn Reset(&self) -> windows_core::Result<()> {
        crate::guarded("Reset", || Ok(()))
    }
}

#[implement(IEnumTfDisplayAttributeInfo)]
pub struct AttributeEnum {
    index: Cell<u32>,
}

impl AttributeEnum {
    pub fn new() -> AttributeEnum {
        AttributeEnum::at(0)
    }

    fn at(index: u32) -> AttributeEnum {
        crate::object_created();
        AttributeEnum {
            index: Cell::new(index),
        }
    }
}

impl Drop for AttributeEnum {
    fn drop(&mut self) {
        crate::object_destroyed();
    }
}

impl IEnumTfDisplayAttributeInfo_Impl for AttributeEnum_Impl {
    fn Clone(&self) -> windows_core::Result<IEnumTfDisplayAttributeInfo> {
        crate::guarded("Clone", || {
            Ok(ComObject::new(AttributeEnum::at(self.index.get())).into_interface())
        })
    }
    fn Next(
        &self,
        count: u32,
        rginfo: *mut Option<ITfDisplayAttributeInfo>,
        fetched: *mut u32,
    ) -> windows_core::Result<()> {
        crate::guarded("Next", || {
            if (count > 0 && rginfo.is_null()) || (count != 1 && fetched.is_null()) {
                return Err(windows::Win32::Foundation::E_POINTER.into());
            }
            let mut n = 0u32;
            while n < count && (self.index.get() as usize) < ATTRS.len() {
                let attr = ATTRS[self.index.get() as usize];
                let info: ITfDisplayAttributeInfo =
                    ComObject::new(InputAttribute::new(attr)).into_interface();
                // [out] 陣列在呼叫端通常沒有初始化（#89）：只能 write，不能用 `*p = …`，
                // 後者會先把垃圾值當成舊的介面 Release，宿主（Outlook）當場崩潰。
                unsafe { rginfo.add(n as usize).write(Some(info)) };
                self.index.set(self.index.get() + 1);
                n += 1;
            }
            if !fetched.is_null() {
                unsafe { fetched.write(n) };
            }
            if n == count {
                Ok(())
            } else {
                Err(windows::Win32::Foundation::S_FALSE.into())
            }
        })
    }
    fn Reset(&self) -> windows_core::Result<()> {
        crate::guarded("Reset", || {
            self.index.set(0);
            Ok(())
        })
    }
    fn Skip(&self, count: u32) -> windows_core::Result<()> {
        crate::guarded("Skip", || {
            let remaining = ATTRS.len() as u32 - self.index.get();
            self.index.set(
                self.index
                    .get()
                    .saturating_add(count)
                    .min(ATTRS.len() as u32),
            );
            if count <= remaining {
                Ok(())
            } else {
                Err(windows::Win32::Foundation::S_FALSE.into())
            }
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::session::Segment;
    use std::ffi::c_void;
    use std::sync::atomic::{AtomicU32, Ordering};
    use windows::Win32::UI::TextServices::TF_CT_NONE;
    use windows_core::Interface;
    #[test]
    fn utf16_styles_cover_surrogates_empty_and_unknown_segments() {
        let segments = [
            ("你", "normal"),
            ("𠮷", "locked"),
            ("", "locked"),
            ("好", "corrected"),
            ("ㄋ", "pending"),
            ("a", "future"),
            ("世傑", "marked"),
        ]
        .into_iter()
        .map(|(text, style)| Segment {
            text: text.into(),
            style: style.into(),
        })
        .collect::<Vec<_>>();
        let ranges = style_ranges(&segments).unwrap();
        assert_eq!(
            ranges
                .iter()
                .map(|r| (r.start, r.end, r.kind))
                .collect::<Vec<_>>(),
            vec![
                (0, 1, Kind::Input),
                (1, 3, Kind::Locked),
                (3, 4, Kind::Corrected),
                (4, 5, Kind::Input),
                (5, 6, Kind::Input),
                (6, 8, Kind::Marked)
            ]
        );
        let marked = Kind::Marked.attributes();
        assert!(marked.fBoldLine.as_bool());
        assert_eq!(marked.lsStyle, TF_LS_DOT);
        assert_eq!(marked.bAttr, TF_ATTR_TARGET_CONVERTED);
        for c in [marked.crBk, marked.crLine, marked.crText] {
            assert_eq!(c.r#type, TF_CT_COLORREF);
        }
        assert_eq!(unsafe { marked.crBk.Anonymous.cr.0 }, unsafe { GetSysColor(COLOR_HIGHLIGHT) });
        assert_eq!(unsafe { marked.crLine.Anonymous.cr.0 }, unsafe { GetSysColor(COLOR_HIGHLIGHT) });
        assert_eq!(unsafe { marked.crText.Anonymous.cr.0 }, unsafe { GetSysColor(COLOR_HIGHLIGHTTEXT) });
        // 各種屬性的 GUID 各不相同，索引就是在 ATTRS 的位置（atom 陣列依此存取）。
        for (i, attr) in ATTRS.into_iter().enumerate() {
            assert_eq!(attr.index(), i);
            assert_eq!(Attr::new(attr.kind, attr.line), attr);
            assert!(ATTRS.iter().filter(|a| a.guid() == attr.guid()).count() == 1);
        }
        assert!(Kind::Locked.attributes().fBoldLine.as_bool());
        assert_eq!(Kind::Corrected.attributes().lsStyle, TF_LS_DOT);
        assert_eq!(Kind::Input.attributes().lsStyle, TF_LS_SOLID);
    }
    #[test]
    fn white_and_black_lines_only_change_the_line_color() {
        // P6.5：組字、已選字、自動修正的白線／黑線版只換底線顏色，樣式不變；沿用宿主色的版本不指定底線顏色。
        for kind in [Kind::Input, Kind::Locked, Kind::Corrected] {
            let host = Attr::new(kind, Line::Host).attributes();
            assert_eq!(host.crLine.r#type, TF_CT_NONE);
            for (line, rgb) in [(Line::White, 0x00FF_FFFF), (Line::Black, 0)] {
                let da = Attr::new(kind, line).attributes();
                assert_eq!(da.crLine.r#type, TF_CT_COLORREF);
                assert_eq!(unsafe { da.crLine.Anonymous.cr.0 }, rgb);
                assert_eq!((da.lsStyle, da.fBoldLine, da.bAttr), (host.lsStyle, host.fBoldLine, host.bAttr));
                assert_eq!(da.crText.r#type, TF_CT_NONE);
            }
        }
        // 加詞標示已有自己的顏色：不分深淺。
        assert_eq!(Attr::new(Kind::Marked, Line::White), Attr::new(Kind::Marked, Line::Host));
        assert_eq!(Attr::new(Kind::Marked, Line::Black), Attr::new(Kind::Marked, Line::Host));
    }
    #[test]
    fn attribute_enumerator_batches_clones_and_reports_end() {
        let list: IEnumTfDisplayAttributeInfo =
            ComObject::new(AttributeEnum::new()).into_interface();
        unsafe {
            let mut first = [None];
            let mut fetched = 0;
            list.Next(&mut first, &mut fetched).unwrap();
            assert_eq!(fetched, 1);
            assert_eq!(
                first[0].as_ref().unwrap().GetGUID().unwrap(),
                Attr::new(Kind::Input, Line::Host).guid()
            );
            let clone = list.Clone().unwrap();
            let mut rest = [None, None, None, None, None];
            clone.Next(&mut rest, &mut fetched).unwrap();
            assert_eq!(fetched, 5);
            assert_eq!(
                rest[0].as_ref().unwrap().GetGUID().unwrap(),
                Attr::new(Kind::Locked, Line::Host).guid()
            );
            assert_eq!(
                rest[1].as_ref().unwrap().GetGUID().unwrap(),
                Attr::new(Kind::Corrected, Line::Host).guid()
            );
            assert_eq!(
                rest[2].as_ref().unwrap().GetGUID().unwrap(),
                Attr::new(Kind::Marked, Line::Host).guid()
            );
            assert_eq!(
                rest[3].as_ref().unwrap().GetGUID().unwrap(),
                Attr::new(Kind::Input, Line::White).guid()
            );
            assert_eq!(
                rest[4].as_ref().unwrap().GetGUID().unwrap(),
                Attr::new(Kind::Locked, Line::White).guid()
            );
            list.Next(&mut first, &mut fetched).unwrap();
            assert_eq!(
                first[0].as_ref().unwrap().GetGUID().unwrap(),
                Attr::new(Kind::Locked, Line::Host).guid()
            );
            list.Reset().unwrap();
            let _ = list.Skip(ATTRS.len() as u32 + 1);
            let _ = list.Next(&mut first, &mut fetched);
            assert_eq!(fetched, 0);
        }
    }

    /// 只有 IUnknown vtable 的計數物件：記錄被 Release 幾次，當作呼叫端陣列裡的「舊值」。
    #[repr(C)]
    struct Sentinel {
        vtbl: *const windows_core::IUnknown_Vtbl,
        releases: AtomicU32,
    }
    unsafe extern "system" fn sentinel_query(
        _: *mut c_void,
        _: *const GUID,
        out: *mut *mut c_void,
    ) -> windows_core::HRESULT {
        if !out.is_null() {
            out.write(std::ptr::null_mut());
        }
        windows::Win32::Foundation::E_NOINTERFACE
    }
    unsafe extern "system" fn sentinel_add_ref(_: *mut c_void) -> u32 {
        1
    }
    unsafe extern "system" fn sentinel_release(this: *mut c_void) -> u32 {
        (*(this as *const Sentinel)).releases.fetch_add(1, Ordering::SeqCst);
        1
    }
    static SENTINEL_VTBL: windows_core::IUnknown_Vtbl = windows_core::IUnknown_Vtbl {
        QueryInterface: sentinel_query,
        AddRef: sentinel_add_ref,
        Release: sentinel_release,
    };

    #[test]
    fn next_overwrites_uninitialized_outputs_without_releasing_them() {
        // msctf（Outlook 啟動時）傳進來的 [out] 陣列沒有初始化（#89）。前五格放計數物件的指標
        // 當垃圾值：Next 必須直接覆寫、不能 Release；超出 count 的第六格填 0xDD，必須原封不動。
        let sentinel = Sentinel {
            vtbl: &SENTINEL_VTBL,
            releases: AtomicU32::new(0),
        };
        let garbage = &sentinel as *const Sentinel as *mut c_void;
        let poison = usize::from_ne_bytes([0xDD; std::mem::size_of::<usize>()]) as *mut c_void;
        let mut out = [garbage, garbage, garbage, garbage, garbage, poison];
        let mut fetched = 0xDDDD_DDDDu32;
        let list: IEnumTfDisplayAttributeInfo =
            ComObject::new(AttributeEnum::new()).into_interface();
        let hr = unsafe {
            (Interface::vtable(&list).Next)(Interface::as_raw(&list), 5, out.as_mut_ptr(), &mut fetched)
        };
        assert_eq!(hr, windows_core::HRESULT(0));
        assert_eq!(fetched, 5);
        assert_eq!(
            sentinel.releases.load(Ordering::SeqCst),
            0,
            "Next 把呼叫端的垃圾值當成介面 Release 了"
        );
        assert_eq!(out[5], poison, "count 以外的格子不能動");
        for (slot, attr) in out[..5].iter().zip(ATTRS) {
            assert_ne!(*slot, garbage);
            let info = unsafe { ITfDisplayAttributeInfo::from_raw(*slot) };
            assert_eq!(unsafe { info.GetGUID() }.unwrap(), attr.guid());
        }
        // 已到結尾：再要一個時回 S_FALSE、fetched 為 0，輸出格同樣不讀不放。
        unsafe { list.Skip(ATTRS.len() as u32 - 5) }.unwrap();
        let mut tail = [garbage];
        let hr = unsafe {
            (Interface::vtable(&list).Next)(Interface::as_raw(&list), 1, tail.as_mut_ptr(), &mut fetched)
        };
        assert_eq!(hr, windows::Win32::Foundation::S_FALSE);
        assert_eq!(fetched, 0);
        assert_eq!(tail[0], garbage);
        assert_eq!(sentinel.releases.load(Ordering::SeqCst), 0);
    }
}
