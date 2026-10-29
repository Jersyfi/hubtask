// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// No console window beside the app on Windows, in a release build.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

fn main() {
    hubtask_lib::run()
}
