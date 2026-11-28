//! 螢幕座標可為負數；候選窗在縮放後也必須留在工作區。
use windows::Win32::Foundation::RECT;

/// 放在游標上方（above）或下方時視窗的頂端。
fn side_top(y: i32, caret_height: i32, gap: i32, height: i32, above: bool) -> i32 {
    if above {
        y.saturating_sub(gap).saturating_sub(height)
    } else {
        y.saturating_add(caret_height).saturating_add(gap)
    }
}

/// 游標上方（above）或下方放不放得下 height 高的視窗。
pub fn fits(y: i32, caret_height: i32, gap: i32, height: i32, above: bool, work: RECT) -> bool {
    let top = side_top(y, caret_height, gap, height, above);
    if above {
        top >= work.top
    } else {
        top.saturating_add(height) <= work.bottom
    }
}

/// above：放游標上方，否則放下方；放不下的部分夾回工作區。
pub fn place(x: i32, y: i32, caret_height: i32, gap: i32, width: i32, height: i32, above: bool, work: RECT) -> RECT {
    let width = width
        .max(1)
        .min(work.right.saturating_sub(work.left).max(1));
    let height = height
        .max(1)
        .min(work.bottom.saturating_sub(work.top).max(1));
    let left = x.clamp(work.left, work.right.saturating_sub(width).max(work.left));
    let top = side_top(y, caret_height, gap, height, above).clamp(work.top, work.bottom.saturating_sub(height).max(work.top));
    RECT {
        left,
        top,
        right: left.saturating_add(width),
        bottom: top.saturating_add(height),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn scaled_windows_fit_negative_monitors_and_extreme_carets() {
        for work in [
            RECT {
                left: 0,
                top: 0,
                right: 1920,
                bottom: 1040,
            },
            RECT {
                left: -1920,
                top: -200,
                right: 0,
                bottom: 840,
            },
        ] {
            for (x, y) in [
                (work.left, work.top),
                (work.right - 1, work.bottom - 1),
                (i32::MIN, i32::MIN),
                (i32::MAX, i32::MAX),
            ] {
                for dpi in [96, 120, 144, 192, 288, 384] {
                    for above in [false, true] {
                        let r = place(x, y, 20 * dpi / 96, dpi as i32 / 96, 400 * dpi / 96, 600 * dpi / 96, above, work);
                        assert!(
                            r.left >= work.left
                                && r.top >= work.top
                                && r.right <= work.right
                                && r.bottom <= work.bottom
                        );
                        assert!(r.right > r.left && r.bottom > r.top);
                    }
                }
            }
        }
    }

    #[test]
    fn caller_picks_the_side() {
        let work = RECT {
            left: 0,
            top: 0,
            right: 1920,
            bottom: 1040,
        };
        assert!(fits(900, 20, 2, 60, false, work));
        assert!(!fits(900, 20, 2, 320, false, work));
        assert!(fits(900, 20, 2, 320, true, work));
        assert!(!fits(100, 20, 2, 320, true, work));
        assert_eq!(place(100, 900, 20, 2, 200, 60, true, work).bottom, 898);
        assert_eq!(place(100, 900, 20, 2, 200, 60, false, work).top, 922);
    }
}
