// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//! The Android app's own bridge (F7 Decision 4): the Android Keystore, the system's share sheet,
//! the system bars and the release's update manifest, in Kotlin (`gen/android/app/src/main/java/eu/hubtask/app/HubtaskPlugin.kt`),
//! called through Tauri's mobile plugin interface. No crate beyond Decision 4's is needed for it.

use std::collections::HashMap;

use tauri::plugin::{Builder, PluginHandle, TauriPlugin};
use tauri::{Manager, Wry};

use crate::copy::{CopyError, Keystore};

/// The Kotlin half, once the plugin is set up.
pub struct Bridge(pub PluginHandle<Wry>);

/// Registers the Kotlin half under the app's own identifier.
pub fn plugin() -> TauriPlugin<Wry> {
    Builder::<Wry>::new("hubtask")
        .setup(|app, api| {
            let handle = api.register_android_plugin("eu.hubtask.app", "HubtaskPlugin")?;
            app.manage(Bridge(handle));
            Ok(())
        })
        .build()
}

type Answer = HashMap<String, String>;

/// A keystore entry, kept by the Kotlin half sealed under a key the Android Keystore holds and
/// never hands out (ADR-0033 §4): the copies' keys and the sessions, filed by service and name as
/// on the desktop.
pub struct AndroidKeystore {
    handle: PluginHandle<Wry>,
    service: &'static str,
}

impl AndroidKeystore {
    pub fn new(handle: PluginHandle<Wry>, service: &'static str) -> Self {
        Self { handle, service }
    }

    fn call(&self, command: &str, args: HashMap<&str, String>) -> Result<Answer, CopyError> {
        self.handle
            .run_mobile_plugin::<Answer>(command, args)
            .map_err(|error| CopyError::Keystore(error.to_string()))
    }

    fn args(&self, name: &str) -> HashMap<&'static str, String> {
        HashMap::from([
            ("service", self.service.to_string()),
            ("name", name.to_string()),
        ])
    }
}

impl Keystore for AndroidKeystore {
    fn read(&self, name: &str) -> Result<Option<Vec<u8>>, CopyError> {
        match self.call("secretRead", self.args(name))?.remove("value") {
            None => Ok(None),
            Some(hex) => unhex(&hex)
                .map(Some)
                .ok_or_else(|| CopyError::Keystore("the keystore answered no value".to_string())),
        }
    }

    fn write(&self, name: &str, key: &[u8]) -> Result<(), CopyError> {
        let mut args = self.args(name);
        args.insert("value", crate::copy::hex(key));
        self.call("secretWrite", args).map(|_| ())
    }

    fn remove(&self, name: &str) -> Result<(), CopyError> {
        self.call("secretRemove", self.args(name)).map(|_| ())
    }
}

/// An entry's title and link to the system's share sheet (UC-SYN-07 check 8).
pub fn send_link(handle: &PluginHandle<Wry>, title: String, url: String) -> Result<(), String> {
    handle
        .run_mobile_plugin::<Answer>("share", HashMap::from([("title", title), ("url", url)]))
        .map(|_| ())
        .map_err(|_| "shell.share_unavailable".to_string())
}

/// The system bars in the page's background, with icons that read on it.
pub fn paint_bars(
    handle: &PluginHandle<Wry>,
    background: String,
    is_dark: bool,
) -> Result<(), String> {
    handle
        .run_mobile_plugin::<Answer>(
            "bars",
            HashMap::from([("background", background), ("isDark", is_dark.to_string())]),
        )
        .map(|_| ())
        .map_err(|_| "shell.bars_unavailable".to_string())
}

/// Where every release's update manifest is published; `latest` is the newest that is no
/// pre-release (versioning-release.md §8).
const MANIFEST: &str = "https://github.com/Jersyfi/hubtask/releases/latest/download/latest.json";

/// The release's update manifest, fetched by the Kotlin half (the page's request would be refused
/// by the download's missing cross-origin headers). What it says is read in the page
/// (`platform/update.ts`); the app installs nothing (ADR-0097).
pub fn release_manifest(handle: &PluginHandle<Wry>) -> Result<String, String> {
    handle
        .run_mobile_plugin::<Answer>("fetchText", HashMap::from([("url", MANIFEST.to_string())]))
        .ok()
        .and_then(|mut answer| answer.remove("text"))
        .ok_or_else(|| "update.unavailable".to_string())
}

/// The keystore's hex back into bytes; `None` for anything that is not hex.
fn unhex(text: &str) -> Option<Vec<u8>> {
    if !text.len().is_multiple_of(2) {
        return None;
    }
    (0..text.len())
        .step_by(2)
        .map(|at| u8::from_str_radix(text.get(at..at + 2)?, 16).ok())
        .collect()
}
