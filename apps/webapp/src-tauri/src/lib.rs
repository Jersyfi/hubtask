// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//! The shell of the installed apps (ADR-0031): a window that renders the web app, and the few
//! commands a page may not carry out itself. Business logic never lands here; the web app's
//! `platform/` seam is the only caller (ADR-0033).

mod system;

use std::collections::HashMap;

/// How this device introduces itself in a workspace's device list: the name the system gives it
/// and the system's own name. Asked once, when the web app starts.
#[tauri::command]
fn system_identity() -> HashMap<&'static str, String> {
    HashMap::from([
        ("name", system::device_name()),
        ("platform", system::platform().to_string()),
    ])
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![system_identity])
        .run(tauri::generate_context!())
        .expect("the shell could not start");
}
