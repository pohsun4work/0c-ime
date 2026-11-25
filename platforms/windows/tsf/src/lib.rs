//! Zuin 注音的 Windows TSF text service（docs/19）。
//!
//! 這個 DLL 會被載入每一個有文字輸入的應用程式，是全專案出錯代價最高的程式。它只做三件事：
//! 實作 TSF 要求的 COM 介面、把按鍵送進 named pipe、依快照更新組字區與提交文字。
//! 不含非同步執行環境；少數不能卡住宿主執行緒的工作（語言列狀態、組字區背景深淺）丟給 `background`。每個 COM 入口都以 catch_unwind 攔 panic：攔到就回傳錯誤碼
//! 並退回放行按鍵，未攔截的 panic 會終止宿主應用程式。
#![allow(non_snake_case)]

use std::ffi::c_void;
use std::sync::atomic::{AtomicIsize, AtomicUsize, Ordering};

use windows::Win32::Foundation::{
    CloseHandle, FreeLibrary, CLASS_E_CLASSNOTAVAILABLE, E_FAIL, E_POINTER, HMODULE, S_FALSE, S_OK,
};
use windows::Win32::System::LibraryLoader::{
    DisableThreadLibraryCalls, FreeLibraryAndExitThread, GetModuleHandleExW, GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS,
};
use windows::Win32::System::Threading::{CreateThread, THREAD_CREATION_FLAGS};
use windows::Win32::System::SystemServices::DLL_PROCESS_ATTACH;
use windows_core::{ComObject, Interface, BOOL, GUID, HRESULT, PCWSTR};

mod candidate;
mod caret;
mod click;
mod context;
mod cuas;
mod displayattr;
mod edit;
mod factory;
mod guids;
mod ipc;
mod keys;
mod langbar;
mod layout;
mod log;
mod msctf;
mod privacy;
mod register;
mod service;
mod session;
mod tone;
mod ui;
mod uielement;

static DLL_INSTANCE: AtomicIsize = AtomicIsize::new(0);
/// 活著的 COM 物件（TextService、ClassFactory、edit session、顯示屬性物件）與 LockServer 的數量；
/// 都是零時 DLL 才可卸載。每個 `#[implement]` 的型別都要在建構時加、Drop 時減。
static LIVE_OBJECTS: AtomicUsize = AtomicUsize::new(0);

pub(crate) fn dll_instance() -> HMODULE {
    HMODULE(DLL_INSTANCE.load(Ordering::Relaxed) as *mut c_void)
}

pub(crate) fn object_created() {
    LIVE_OBJECTS.fetch_add(1, Ordering::Relaxed);
}

pub(crate) fn object_destroyed() {
    LIVE_OBJECTS.fetch_sub(1, Ordering::Relaxed);
}

/// 背景執行緒的工作與它持有的 DLL 模組參考。
struct Job {
    module: HMODULE,
    work: Box<dyn FnOnce() + Send>,
}

/// 在背景執行。期間計入 DLL 物件數，另外由呼叫端先取得一份模組參考交給執行緒，
/// 執行緒以 FreeLibraryAndExitThread 放開（#89）：減完物件數到執行緒真正結束之間仍在跑本 DLL 的程式碼，
/// 只靠物件數時宿主可能在這段時間卸載 DLL（CoFreeUnusedLibraries），執行緒就跑進已卸載的位址。
pub(crate) fn background(f: impl FnOnce() + Send + 'static) {
    let mut module = HMODULE::default();
    let locked = unsafe {
        GetModuleHandleExW(
            GET_MODULE_HANDLE_EX_FLAG_FROM_ADDRESS,
            PCWSTR::from_raw(background_main as *const () as *const u16), // FROM_ADDRESS：以本 DLL 內的位址找模組
            &mut module,
        )
    };
    if locked.is_err() {
        log::log("取得模組參考失敗，略過背景工作");
        return;
    }
    crate::object_created();
    let job = Box::into_raw(Box::new(Job { module, work: Box::new(f) }));
    let started =
        unsafe { CreateThread(None, 0, Some(background_main), Some(job as *const c_void), THREAD_CREATION_FLAGS(0), None) };
    match started {
        Ok(thread) => unsafe {
            let _ = CloseHandle(thread);
        },
        Err(e) => unsafe {
            log::log(&format!("建立背景執行緒失敗 {}", e));
            drop(Box::from_raw(job));
            crate::object_destroyed();
            let _ = FreeLibrary(module);
        },
    }
}

unsafe extern "system" fn background_main(param: *mut c_void) -> u32 {
    let job = Box::from_raw(param as *mut Job);
    let Job { module, work } = *job;
    if std::panic::catch_unwind(std::panic::AssertUnwindSafe(work)).is_err() {
        log::log("panic 已攔截：背景工作");
    }
    crate::object_destroyed();
    FreeLibraryAndExitThread(module, 0)
}

/// 攔截 panic：COM 邊界不得 unwind。給 `#[implement]` 的介面方法用（回傳 Result）。
pub(crate) fn guarded<T, F: FnOnce() -> windows_core::Result<T>>(what: &str, f: F) -> windows_core::Result<T> {
    match std::panic::catch_unwind(std::panic::AssertUnwindSafe(f)) {
        Ok(r) => r,
        Err(_) => {
            log::log(&format!("panic 已攔截：{}", what));
            Err(E_FAIL.into())
        }
    }
}

/// 攔截 panic：DLL 匯出用（回傳 HRESULT）。
pub(crate) fn guard<F: FnOnce() -> HRESULT>(what: &str, f: F) -> HRESULT {
    match std::panic::catch_unwind(std::panic::AssertUnwindSafe(f)) {
        Ok(hr) => hr,
        Err(_) => {
            log::log(&format!("panic 已攔截：{}", what));
            E_FAIL
        }
    }
}

#[no_mangle]
pub extern "system" fn DllMain(instance: HMODULE, reason: u32, _reserved: *mut c_void) -> BOOL {
    if reason == DLL_PROCESS_ATTACH {
        DLL_INSTANCE.store(instance.0 as isize, Ordering::Relaxed);
        unsafe {
            let _ = DisableThreadLibraryCalls(instance);
        }
    }
    BOOL(1)
}

#[no_mangle]
pub extern "system" fn DllGetClassObject(rclsid: *const GUID, riid: *const GUID, ppv: *mut *mut c_void) -> HRESULT {
    log::install_panic_hook();
    guard("DllGetClassObject", || unsafe {
        if ppv.is_null() || rclsid.is_null() || riid.is_null() {
            return E_POINTER;
        }
        *ppv = std::ptr::null_mut();
        if *rclsid != guids::CLSID_ZUIN {
            return CLASS_E_CLASSNOTAVAILABLE;
        }
        let factory: windows_core::IUnknown = ComObject::new(factory::ClassFactory::new()).into_interface();
        factory.query(riid, ppv)
    })
}

#[no_mangle]
pub extern "system" fn DllCanUnloadNow() -> HRESULT {
    if LIVE_OBJECTS.load(Ordering::Relaxed) == 0 {
        S_OK
    } else {
        S_FALSE
    }
}

#[no_mangle]
pub extern "system" fn DllRegisterServer() -> HRESULT {
    log::install_panic_hook();
    guard("DllRegisterServer", || match register::register() {
        Ok(()) => S_OK,
        Err(e) => {
            log::log(&format!("註冊失敗：{}", e));
            e.code()
        }
    })
}

#[no_mangle]
pub extern "system" fn DllUnregisterServer() -> HRESULT {
    guard("DllUnregisterServer", || match register::unregister() {
        Ok(()) => S_OK,
        Err(e) => {
            log::log(&format!("取消註冊失敗：{}", e));
            e.code()
        }
    })
}
