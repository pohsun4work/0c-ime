//! Zuin 的 COM 識別碼。改了 CLSID 等於換一個輸入法，安裝程式與註冊都要一起改。
use windows_core::GUID;

/// text service 的 CLSID（InprocServer32）。
pub const CLSID_ZUIN: GUID = GUID::from_u128(0x138c35d0_7f08_4cef_ba90_c34c16301528);
/// 語言 profile：zh-TW 底下的「靈犀注音」。
pub const GUID_PROFILE: GUID = GUID::from_u128(0xef62f743_9855_4c94_a658_1f0f98b7b840);
/// 組字區的顯示屬性（底線）。
pub const GUID_DISPLAY_ATTR_INPUT: GUID = GUID::from_u128(0xa6ca2572_a961_4492_975f_29a4ebb7c312);

pub const GUID_DISPLAY_ATTR_LOCKED: GUID = GUID::from_u128(0x7a4b5f38_7c42_4315_95b8_6b29f72c5b3b);
pub const GUID_DISPLAY_ATTR_CORRECTED: GUID =
    GUID::from_u128(0x923cdd42_5c7e_4781_8fbe_12232f607742);
/// 加詞標示（#123）。
pub const GUID_DISPLAY_ATTR_MARKED: GUID = GUID::from_u128(0x7f850af6_4e7a_4b41_a24f_baeca02ce683);
/// 中英混打時按空白會組成中文的數字（#162）。
pub const GUID_DISPLAY_ATTR_READING: GUID = GUID::from_u128(0xcbedd798_5d14_4f4d_9917_26b934dfa003);
/// 組字、已選字、自動修正在深色背景的白線版與淺色背景的黑線版（tone.rs）。
pub const GUID_DISPLAY_ATTR_INPUT_WHITE: GUID = GUID::from_u128(0x4f9db93a_373d_4a1c_be43_4a59ea6697fa);
pub const GUID_DISPLAY_ATTR_LOCKED_WHITE: GUID = GUID::from_u128(0xf285fdde_9513_4648_94ad_9ad16396d2d5);
pub const GUID_DISPLAY_ATTR_CORRECTED_WHITE: GUID = GUID::from_u128(0x1e280916_e250_41fa_b957_43cb432f77d3);
pub const GUID_DISPLAY_ATTR_INPUT_BLACK: GUID = GUID::from_u128(0x5a642554_36f5_4153_8c3d_dc35625cd1ad);
pub const GUID_DISPLAY_ATTR_LOCKED_BLACK: GUID = GUID::from_u128(0x056b3a61_6c57_4d57_aeaa_d66925021c89);
pub const GUID_DISPLAY_ATTR_CORRECTED_BLACK: GUID = GUID::from_u128(0x6ecddd9e_a3ab_4c84_9989_98be01768ac9);

/// zh-TW。
pub const LANGID: u16 = 0x0404;
pub const DESCRIPTION: &str = "靈犀注音";
