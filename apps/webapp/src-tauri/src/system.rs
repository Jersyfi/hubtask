// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//! What the operating system says about this device, read with the standard library alone: no
//! crate beyond F7 Decision 4 is admitted for it.

/// The name the system gives this device, or the app's own name where it gives none. A label a
/// person reads in a device list, so it is trimmed and bounded rather than validated.
pub fn device_name() -> String {
    named(system_name())
}

/// The system, as the device list's platform column names it.
pub fn platform() -> &'static str {
    std::env::consts::OS
}

/// At most this many characters reach the device list (the sync engine bounds it the same way).
const NAME_MAX: usize = 120;

fn named(raw: Option<String>) -> String {
    let name: String = raw
        .as_deref()
        .map(str::trim)
        .unwrap_or_default()
        .chars()
        .filter(|c| !c.is_control())
        .take(NAME_MAX)
        .collect();
    if name.is_empty() {
        "Hubtask".to_string()
    } else {
        name
    }
}

#[cfg(windows)]
fn system_name() -> Option<String> {
    std::env::var("COMPUTERNAME").ok()
}

#[cfg(target_os = "linux")]
fn system_name() -> Option<String> {
    // The kernel's own answer first; /etc/hostname is what it was set from at boot.
    ["/proc/sys/kernel/hostname", "/etc/hostname"]
        .iter()
        .find_map(|path| std::fs::read_to_string(path).ok())
}

#[cfg(not(any(windows, target_os = "linux")))]
fn system_name() -> Option<String> {
    std::env::var("HOSTNAME").ok()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_name_is_trimmed_and_bounded() {
        let cases = [
            (Some("workstation\n".to_string()), "workstation".to_string()),
            (Some("  laptop  ".to_string()), "laptop".to_string()),
            (Some("a\u{7}b".to_string()), "ab".to_string()),
            (Some("x".repeat(200)), "x".repeat(NAME_MAX)),
        ];
        for (raw, want) in cases {
            assert_eq!(named(raw), want);
        }
    }

    #[test]
    fn no_name_is_the_apps_own() {
        assert_eq!(named(None), "Hubtask");
        assert_eq!(named(Some(" \n".to_string())), "Hubtask");
    }

    #[test]
    fn the_platform_is_the_systems_name() {
        assert!(!platform().is_empty());
    }
}
