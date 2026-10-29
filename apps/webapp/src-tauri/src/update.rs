// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//! Staying current on Windows and Linux (UC-SYN-07 check 5, ADR-0097): Tauri's updater, with the
//! release's manifest checked before anything is downloaded.
//!
//! The updater checks a package's signature, not the manifest's. So the endpoint is `update.json`,
//! a pointer in the updater's own single-download format whose "package" is `latest.json` and
//! whose signature is `latest.json.sig` (versioning-release.md §8): fetching it through the
//! updater checks the manifest under the update key, and the version the key signed into it,
//! before the manifest names a package at all. The package is then downloaded and checked
//! before the person is offered it, and installed only once they agree.

use std::collections::HashMap;
use std::sync::Mutex;
use std::time::Duration;

use tauri::utils::config::BundleType;
use tauri::{AppHandle, Runtime};
use tauri_plugin_updater::{Error, Update, UpdaterExt};

/// How long the pointer and the manifest may take, and how long the package.
const CHECKING: Duration = Duration::from_secs(30);
const FETCHING: Duration = Duration::from_secs(15 * 60);

/// What does not carry the project's signature: the manifest, or the package it names.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Refused {
    Manifest,
    Package,
}

/// What a check found.
#[derive(Debug, PartialEq, Eq)]
pub enum Found {
    Current,
    /// Downloaded, checked and waiting for the person's agreement.
    Ready {
        version: String,
    },
    /// The `.deb`, which the system's package manager updates: a notice with a download.
    Available {
        version: String,
    },
    Refused {
        what: Refused,
        version: String,
    },
}

impl Found {
    /// The answer as the page reads it (`platform/shell.ts`).
    pub fn fields(&self) -> HashMap<&'static str, String> {
        let (state, version, refused) = match self {
            Found::Current => ("current", "", ""),
            Found::Ready { version } => ("ready", version.as_str(), ""),
            Found::Available { version } => ("available", version.as_str(), ""),
            Found::Refused { what, version } => (
                "refused",
                version.as_str(),
                match what {
                    Refused::Manifest => "manifest",
                    Refused::Package => "package",
                },
            ),
        };
        HashMap::from([
            ("state", state.to_string()),
            ("version", version.to_string()),
            ("refused", refused.to_string()),
        ])
    }
}

/// Why a step stopped: a refusal the person is told about, or a failure that is not theirs to see
/// (no network, no release yet), after which the app checks again at its next start.
#[derive(Debug, PartialEq, Eq)]
pub enum Stop {
    Refused(Refused),
    Quiet(String),
}

/// A signature that does not hold under the update key, or holds for another version, is a
/// refusal; anything else is quiet.
pub fn stop(error: Error, what: Refused) -> Stop {
    match error {
        Error::Minisign(_)
        | Error::Base64(_)
        | Error::SignatureUtf8(_)
        | Error::SignedVersionMismatch { .. }
        | Error::MissingSignedVersion => Stop::Refused(what),
        other => Stop::Quiet(other.to_string()),
    }
}

/// The manifest, once its signature held for `announced`: it must say that version itself.
pub fn read_manifest(bytes: &[u8], announced: &str) -> Result<serde_json::Value, Stop> {
    let manifest: serde_json::Value =
        serde_json::from_slice(bytes).map_err(|_| Stop::Refused(Refused::Manifest))?;
    if manifest.get("version").and_then(serde_json::Value::as_str) != Some(announced) {
        return Err(Stop::Refused(Refused::Manifest));
    }
    Ok(manifest)
}

/// The package checked and kept until the person agrees.
#[derive(Default)]
pub struct Held(Mutex<Option<(Update, Vec<u8>)>>);

impl Held {
    fn keep(&self, update: Update, bytes: Vec<u8>) {
        *self
            .0
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner()) = Some((update, bytes));
    }

    pub fn take(&self) -> Option<(Update, Vec<u8>)> {
        self.0
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .take()
    }
}

/// Whether this app was installed from the `.deb`, which only the package manager replaces.
fn from_deb() -> bool {
    tauri::utils::platform::bundle_type() == Some(BundleType::Deb)
}

/// Looks for a newer release: the pointer, the manifest under the update key, then the package.
pub async fn check<R: Runtime>(app: &AppHandle<R>, held: &Held) -> Result<Found, Stop> {
    let quiet = |error: Error| Stop::Quiet(error.to_string());
    let checking = app
        .updater_builder()
        .timeout(CHECKING)
        .build()
        .map_err(quiet)?;
    let Some(pointer) = checking.check().await.map_err(quiet)? else {
        return Ok(Found::Current);
    };
    let version = pointer.version.clone();
    let refused = |what| Found::Refused {
        what,
        version: version.clone(),
    };
    let answered = |stop: Stop| match stop {
        Stop::Refused(what) => Ok(refused(what)),
        quiet => Err(quiet),
    };

    let bytes = match pointer.download(|_, _| {}, || {}).await {
        Ok(bytes) => bytes,
        Err(error) => return answered(stop(error, Refused::Manifest)),
    };
    let manifest = match read_manifest(&bytes, &pointer.version) {
        Ok(manifest) => manifest,
        Err(stop) => return answered(stop),
    };
    if from_deb() {
        return Ok(Found::Available { version });
    }

    let fetching = app
        .updater_builder()
        .timeout(FETCHING)
        .build()
        .map_err(quiet)?;
    let Some(package) = fetching.restore_update(manifest).map_err(quiet)? else {
        return Ok(Found::Current);
    };
    match package.download(|_, _| {}, || {}).await {
        Ok(bytes) => {
            held.keep(package, bytes);
            Ok(Found::Ready { version })
        }
        Err(error) => answered(stop(error, Refused::Package)),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    use serde_json::json;
    use tauri::test::{mock_builder, mock_context, noop_assets, MockRuntime};
    use tauri_plugin_updater::Updater;

    // Signed with a throwaway key made for these tests (`fixtures/update/`); the release's own key
    // is versioning-release.md §8's.
    const TRUSTED: &str = include_str!("../fixtures/update/trusted.pub");
    const MANIFEST: &[u8] = include_bytes!("../fixtures/update/latest.json");
    const MANIFEST_SIGNED: &str = include_str!("../fixtures/update/latest.json.sig");
    const MANIFEST_OTHER_KEY: &str = include_str!("../fixtures/update/latest.json.other.sig");
    const MANIFEST_OLDER: &str = include_str!("../fixtures/update/latest.json.older.sig");
    const OTHER_PACKAGE: &[u8] = include_bytes!("../fixtures/update/latest-other-package.json");
    const OTHER_PACKAGE_SIGNED: &str =
        include_str!("../fixtures/update/latest-other-package.json.sig");
    const PACKAGE: &[u8] = include_bytes!("../fixtures/update/package.bin");
    const VERSION: &str = "1.3.0+57";

    fn updater() -> Updater {
        let mut context = mock_context::<MockRuntime, _>(noop_assets());
        context.config_mut().plugins.0.insert(
            "updater".to_string(),
            json!({
                "pubkey": TRUSTED.trim(),
                "endpoints": ["https://example.invalid/update.json"],
                "requireSignedVersion": true
            }),
        );
        let app = mock_builder()
            .plugin(tauri_plugin_updater::Builder::new().build())
            .build(context)
            .expect("the mock app builds");
        app.handle()
            .updater_builder()
            .target("linux-x86_64")
            .build()
            .expect("the updater builds")
    }

    /// The pointer as `update.json` carries it: the manifest as the download, with its signature.
    fn pointer(updater: &Updater, signature: &str) -> Update {
        updater
            .restore_update(json!({
                "version": VERSION,
                "url": "https://example.invalid/latest.json",
                "signature": signature.trim()
            }))
            .expect("the pointer reads")
            .expect("the pointer names a newer release")
    }

    fn manifest(updater: &Updater, signature: &str, bytes: &[u8]) -> Result<Update, Stop> {
        let pointer = pointer(updater, signature);
        pointer
            .verify(bytes)
            .map_err(|error| stop(error, Refused::Manifest))?;
        let manifest = read_manifest(bytes, &pointer.version)?;
        Ok(updater
            .restore_update(manifest)
            .expect("the manifest reads")
            .expect("the manifest names a newer release"))
    }

    #[test]
    fn a_manifest_and_package_signed_with_the_update_key_are_offered() {
        let updater = updater();
        let package = manifest(&updater, MANIFEST_SIGNED, MANIFEST).expect("the manifest holds");
        assert_eq!(package.version, VERSION);
        assert!(package.verify(PACKAGE).is_ok());
    }

    #[test]
    fn a_manifest_signed_with_another_key_is_refused() {
        let updater = updater();
        assert_eq!(
            manifest(&updater, MANIFEST_OTHER_KEY, MANIFEST).err(),
            Some(Stop::Refused(Refused::Manifest))
        );
    }

    #[test]
    fn a_manifest_signed_for_another_version_is_refused() {
        let updater = updater();
        assert_eq!(
            manifest(&updater, MANIFEST_OLDER, MANIFEST).err(),
            Some(Stop::Refused(Refused::Manifest))
        );
    }

    #[test]
    fn a_changed_manifest_is_refused() {
        let updater = updater();
        let changed = String::from_utf8_lossy(MANIFEST).replace("example.invalid", "example.test");
        assert_eq!(
            manifest(&updater, MANIFEST_SIGNED, changed.as_bytes()).err(),
            Some(Stop::Refused(Refused::Manifest))
        );
    }

    #[test]
    fn a_package_signed_with_another_key_is_refused() {
        let updater = updater();
        let package = manifest(&updater, OTHER_PACKAGE_SIGNED, OTHER_PACKAGE)
            .expect("the manifest itself holds");
        let refused = package
            .verify(PACKAGE)
            .map_err(|error| stop(error, Refused::Package));
        assert_eq!(refused, Err(Stop::Refused(Refused::Package)));
    }

    #[test]
    fn a_changed_package_is_refused() {
        let updater = updater();
        let package = manifest(&updater, MANIFEST_SIGNED, MANIFEST).expect("the manifest holds");
        let refused = package
            .verify(b"something else")
            .map_err(|error| stop(error, Refused::Package));
        assert_eq!(refused, Err(Stop::Refused(Refused::Package)));
    }

    #[test]
    fn a_manifest_that_names_another_version_is_refused() {
        let named = String::from_utf8_lossy(MANIFEST).replace(VERSION, "9.9.9+1");
        assert_eq!(
            read_manifest(named.as_bytes(), VERSION).err(),
            Some(Stop::Refused(Refused::Manifest))
        );
        assert_eq!(
            read_manifest(b"not json", VERSION).err(),
            Some(Stop::Refused(Refused::Manifest))
        );
    }

    #[test]
    fn a_failure_that_is_no_signature_is_quiet() {
        assert!(matches!(
            stop(Error::Network("offline".into()), Refused::Package),
            Stop::Quiet(_)
        ));
        assert!(matches!(
            stop(Error::ReleaseNotFound, Refused::Manifest),
            Stop::Quiet(_)
        ));
    }

    #[test]
    fn the_page_reads_what_was_found() {
        let found = Found::Refused {
            what: Refused::Package,
            version: VERSION.to_string(),
        };
        let fields = found.fields();
        assert_eq!(fields["state"], "refused");
        assert_eq!(fields["refused"], "package");
        assert_eq!(fields["version"], VERSION);
        assert_eq!(Found::Current.fields()["state"], "current");
    }
}
