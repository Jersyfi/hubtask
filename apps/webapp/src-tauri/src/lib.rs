// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//! The shell of the installed apps (ADR-0031): a window that renders the web app, and the few
//! commands a page may not carry out itself. Business logic never lands here; the web app's
//! `platform/` seam is the only caller (ADR-0033).

#[cfg(target_os = "android")]
mod android;
mod copy;
mod signin;
mod system;
#[cfg(not(target_os = "android"))]
mod update;
#[cfg(feature = "walk")]
mod walk;

use std::collections::HashMap;

use tauri::{AppHandle, Emitter, Manager, State};

use copy::{Copies, CopyError, SystemKeystore};
use signin::{Listeners, Page, Sessions};

/// How this device introduces itself in a workspace's device list: the name the system gives it
/// and the system's own name. Asked once, when the web app starts.
#[tauri::command]
fn system_identity() -> HashMap<&'static str, String> {
    HashMap::from([
        ("name", system::device_name()),
        ("platform", system::platform().to_string()),
    ])
}

// The encrypted copy behind the engine's `Storage` port (`SqliteStorage`), one command per
// method. Off the main thread (`async`), because a write waits for the disk.

fn answered<T>(result: Result<T, CopyError>) -> Result<T, String> {
    result.map_err(|error| error.code().to_string())
}

#[tauri::command(async)]
fn copy_get(
    copies: State<'_, Copies>,
    name: String,
    collection: String,
    id: String,
) -> Result<Option<String>, String> {
    answered(copies.get(&name, &collection, &id))
}

#[tauri::command(async)]
fn copy_put(
    copies: State<'_, Copies>,
    name: String,
    collection: String,
    id: String,
    value: String,
) -> Result<(), String> {
    answered(copies.put(&name, &collection, &id, &value))
}

#[tauri::command(async)]
fn copy_delete(
    copies: State<'_, Copies>,
    name: String,
    collection: String,
    id: String,
) -> Result<(), String> {
    answered(copies.delete(&name, &collection, &id))
}

#[tauri::command(async)]
fn copy_all(
    copies: State<'_, Copies>,
    name: String,
    collection: String,
) -> Result<Vec<String>, String> {
    answered(copies.all(&name, &collection))
}

#[tauri::command(async)]
fn copy_collections(copies: State<'_, Copies>, name: String) -> Result<Vec<String>, String> {
    answered(copies.collections(&name))
}

#[tauri::command(async)]
fn copy_clear(copies: State<'_, Copies>, name: String) -> Result<(), String> {
    answered(copies.clear(&name))
}

#[tauri::command(async)]
fn copy_forget(copies: State<'_, Copies>, name: String) -> Result<(), String> {
    answered(copies.forget(&name))
}

// The sign-in through the system browser (ADR-0096): a session per workspace in the keystore, the loopback
// listener the consent's code comes back to, and the browser it is opened in.

#[tauri::command(async)]
fn session_read(sessions: State<'_, Sessions>, server: String) -> Result<Option<String>, String> {
    sessions.read(&server).map_err(str::to_string)
}

#[tauri::command(async)]
fn session_keep(
    sessions: State<'_, Sessions>,
    server: String,
    value: String,
) -> Result<(), String> {
    sessions.keep(&server, &value).map_err(str::to_string)
}

#[tauri::command(async)]
fn session_forget(sessions: State<'_, Sessions>, server: String) -> Result<(), String> {
    sessions.forget(&server).map_err(str::to_string)
}

/// Opens the listener and answers its port; the redirect arrives as the `signin-returned` event.
#[tauri::command]
fn signin_listen(
    app: AppHandle,
    listeners: State<'_, Listeners>,
    title: String,
    body: String,
    lang: String,
    style: String,
    mark: String,
) -> Result<u16, String> {
    listeners
        .listen(
            Page {
                title,
                body,
                lang,
                style,
                mark,
            },
            move |returned| {
                let _ = app.emit("signin-returned", returned.fields());
            },
        )
        .map_err(|_| "signin.listen_failed".to_string())
}

#[tauri::command]
fn signin_retire(listeners: State<'_, Listeners>) {
    listeners.retire();
}

/// The workspace's sign-in or step-up in the system's browser, never in this window.
#[tauri::command]
fn open_in_browser(url: String) -> Result<(), String> {
    if !signin::openable(&url) {
        return Err("signin.address_refused".to_string());
    }
    #[cfg(feature = "walk")]
    let opened = walk::open(&url).map_err(|_| ());
    #[cfg(not(feature = "walk"))]
    let opened = tauri_plugin_opener::open_url(url, None::<&str>).map_err(|_| ());
    opened.map_err(|()| "signin.browser_unavailable".to_string())
}

// What only the Android app does: an entry's link to the share sheet, and the system bars in the
// page's colour (UC-SYN-07 check 8). The desktop has neither, and its web app never asks.

#[cfg(target_os = "android")]
#[tauri::command(async)]
fn send_link(bridge: State<'_, android::Bridge>, title: String, url: String) -> Result<(), String> {
    android::send_link(&bridge.0, title, url)
}

#[cfg(not(target_os = "android"))]
#[tauri::command]
fn send_link(title: String, url: String) -> Result<(), String> {
    let _ = (title, url);
    Err("shell.not_on_this_system".to_string())
}

#[cfg(target_os = "android")]
#[tauri::command(async)]
fn paint_bars(
    bridge: State<'_, android::Bridge>,
    background: String,
    is_dark: bool,
) -> Result<(), String> {
    android::paint_bars(&bridge.0, background, is_dark)
}

#[cfg(not(target_os = "android"))]
#[tauri::command]
fn paint_bars(background: String, is_dark: bool) -> Result<(), String> {
    let _ = (background, is_dark);
    Err("shell.not_on_this_system".to_string())
}

// Staying current (UC-SYN-07 checks 5 and 6, ADR-0097): the updater on Windows and Linux, the
// release's manifest for the Android app's notice.

/// Looks for a newer release, and downloads and checks it before the page offers it.
#[cfg(not(target_os = "android"))]
#[tauri::command]
async fn update_check(
    app: AppHandle,
    held: State<'_, update::Held>,
) -> Result<HashMap<&'static str, String>, String> {
    match update::check(&app, &held).await {
        Ok(found) => Ok(found.fields()),
        Err(update::Stop::Refused(_)) => Err("update.refused".to_string()),
        Err(update::Stop::Quiet(_)) => Err("update.unavailable".to_string()),
    }
}

#[cfg(target_os = "android")]
#[tauri::command]
fn update_check() -> Result<HashMap<&'static str, String>, String> {
    Err("shell.not_on_this_system".to_string())
}

/// Installs the package the check held, after the person agreed, and starts the new version.
#[cfg(not(target_os = "android"))]
#[tauri::command(async)]
fn update_install(app: AppHandle, held: State<'_, update::Held>) -> Result<(), String> {
    let (package, bytes) = held.take().ok_or("update.nothing_ready")?;
    package
        .install(bytes)
        .map_err(|_| "update.install_failed".to_string())?;
    app.restart()
}

#[cfg(target_os = "android")]
#[tauri::command]
fn update_install() -> Result<(), String> {
    Err("shell.not_on_this_system".to_string())
}

/// The release's update manifest as published, for the Android app's notice: the page may not
/// fetch it, because the releases' downloads answer no cross-origin request.
#[cfg(target_os = "android")]
#[tauri::command(async)]
fn release_manifest(bridge: State<'_, android::Bridge>) -> Result<String, String> {
    android::release_manifest(&bridge.0)
}

#[cfg(not(target_os = "android"))]
#[tauri::command]
fn release_manifest() -> Result<String, String> {
    Err("shell.not_on_this_system".to_string())
}

/// The keystores: the system's own on the desktop, the Android Keystore through the app's bridge.
#[cfg(not(target_os = "android"))]
fn keystores(_app: &tauri::App) -> (Box<dyn copy::Keystore>, Box<dyn copy::Keystore>) {
    (
        Box::new(SystemKeystore::COPIES),
        Box::new(SystemKeystore::SESSIONS),
    )
}

#[cfg(target_os = "android")]
fn keystores(app: &tauri::App) -> (Box<dyn copy::Keystore>, Box<dyn copy::Keystore>) {
    let handle = app.state::<android::Bridge>().0.clone();
    (
        Box::new(android::AndroidKeystore::new(
            handle.clone(),
            SystemKeystore::COPIES.service,
        )),
        Box::new(android::AndroidKeystore::new(
            handle,
            SystemKeystore::SESSIONS.service,
        )),
    )
}

/// The sign-in's code, as Android hands the app its own address: sent on as the desktop's listener
/// sends it (`signin-returned`).
#[cfg(target_os = "android")]
fn hear_returns(app: &tauri::App) {
    use tauri_plugin_deep_link::DeepLinkExt;
    let handle = app.handle().clone();
    app.deep_link().on_open_url(move |event| {
        for url in event.urls() {
            if let Some(returned) = signin::app_return(url.as_str()) {
                let _ = handle.emit("signin-returned", returned.fields());
            }
        }
    });
}

#[cfg(not(target_os = "android"))]
fn hear_returns(_app: &tauri::App) {}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let builder = tauri::Builder::default();
    #[cfg(target_os = "android")]
    let builder = builder
        .plugin(tauri_plugin_deep_link::init())
        .plugin(android::plugin());
    #[cfg(not(target_os = "android"))]
    let builder = builder
        .plugin(tauri_plugin_updater::Builder::new().build())
        .manage(update::Held::default());
    builder
        .setup(|app| {
            // The app's own data directory, never a folder a person browses to.
            let root = app.path().app_local_data_dir()?.join("copies");
            let (copies, sessions) = keystores(app);
            app.manage(Copies::new(root, copies));
            app.manage(Sessions::new(sessions));
            app.manage(Listeners::default());
            hear_returns(app);
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            system_identity,
            copy_get,
            copy_put,
            copy_delete,
            copy_all,
            copy_collections,
            copy_clear,
            copy_forget,
            session_read,
            session_keep,
            session_forget,
            signin_listen,
            signin_retire,
            open_in_browser,
            send_link,
            paint_bars,
            update_check,
            update_install,
            release_manifest
        ])
        .run(tauri::generate_context!())
        .expect("the shell could not start");
}
