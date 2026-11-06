// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//! The encrypted copy (ADR-0033 §4, offline-sync.md §9 rule 6): one SQLite database per workspace
//! and account, encrypted at rest by SQLCipher under a key that lives only in the platform's
//! keystore. It is the shells' half of the sync engine's `Storage` port (`SqliteStorage` in
//! `packages/sync-engine`): records go in and out as the text the engine wrote, never read here.
//!
//! A copy whose key is gone is unreadable by anybody, so it is deleted and started again: the
//! server holds everything it held but what waited to be sent, and that is lost with the key.

use std::collections::HashMap;
use std::io;
use std::path::{Path, PathBuf};
use std::sync::{Mutex, MutexGuard};

use rusqlite::{params, Connection, OptionalExtension};

/// The key's length: SQLCipher's raw key, 256 bits, taken as it is rather than derived.
const KEY_LEN: usize = 32;

/// How long a name's segment may be: a file name is at most 255 bytes on every system shipped.
const SEGMENT_MAX: usize = 200;

/// Why a copy could not answer. The web app learns only the code: the copy is then absent, and
/// the app works online as the browser does.
#[derive(Debug)]
pub enum CopyError {
    /// The name is not one `copyNameFor` composes.
    Name,
    /// The keystore refused or is not there.
    Keystore(String),
    /// The system gave no randomness for a new key.
    Random(io::Error),
    /// The database refused.
    Store(rusqlite::Error),
    /// The file could not be made or removed.
    File(io::Error),
}

impl CopyError {
    /// The code the web app receives (rule 8: no sentence leaves here).
    pub fn code(&self) -> &'static str {
        match self {
            CopyError::Name => "copy.name_invalid",
            CopyError::Keystore(_) => "copy.keystore_unavailable",
            CopyError::Random(_) => "copy.random_unavailable",
            CopyError::Store(_) => "copy.store_failed",
            CopyError::File(_) => "copy.file_failed",
        }
    }
}

// What a developer reads in a debugger or a test failure; it never reaches the web app.
impl std::fmt::Display for CopyError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            CopyError::Name => f.write_str(self.code()),
            CopyError::Keystore(cause) => write!(f, "{}: {cause}", self.code()),
            CopyError::Random(cause) | CopyError::File(cause) => {
                write!(f, "{}: {cause}", self.code())
            }
            CopyError::Store(cause) => write!(f, "{}: {cause}", self.code()),
        }
    }
}

impl std::error::Error for CopyError {}

impl From<rusqlite::Error> for CopyError {
    fn from(error: rusqlite::Error) -> Self {
        CopyError::Store(error)
    }
}

/// Where a copy's key is held: the platform's keystore in the app, memory in the tests.
pub trait Keystore: Send + Sync {
    fn read(&self, name: &str) -> Result<Option<Vec<u8>>, CopyError>;
    fn write(&self, name: &str, key: &[u8]) -> Result<(), CopyError>;
    fn remove(&self, name: &str) -> Result<(), CopyError>;
}

/// The copies of this device, opened on first use and held open.
pub struct Copies {
    root: PathBuf,
    keystore: Box<dyn Keystore>,
    open: Mutex<HashMap<String, Connection>>,
}

impl Copies {
    pub fn new(root: PathBuf, keystore: Box<dyn Keystore>) -> Self {
        Copies {
            root,
            keystore,
            open: Mutex::new(HashMap::new()),
        }
    }

    pub fn get(&self, name: &str, collection: &str, id: &str) -> Result<Option<String>, CopyError> {
        self.with(name, |db| {
            db.query_row(
                "SELECT value FROM record WHERE collection = ?1 AND id = ?2",
                params![collection, id],
                |row| row.get(0),
            )
            .optional()
        })
    }

    pub fn put(
        &self,
        name: &str,
        collection: &str,
        id: &str,
        value: &str,
    ) -> Result<(), CopyError> {
        self.with(name, |db| {
            db.execute(
                "INSERT INTO record (collection, id, value) VALUES (?1, ?2, ?3)
                 ON CONFLICT (collection, id) DO UPDATE SET value = excluded.value",
                params![collection, id, value],
            )
            .map(drop)
        })
    }

    pub fn delete(&self, name: &str, collection: &str, id: &str) -> Result<(), CopyError> {
        self.with(name, |db| {
            db.execute(
                "DELETE FROM record WHERE collection = ?1 AND id = ?2",
                params![collection, id],
            )
            .map(drop)
        })
    }

    pub fn all(&self, name: &str, collection: &str) -> Result<Vec<String>, CopyError> {
        self.with(name, |db| {
            let mut rows =
                db.prepare_cached("SELECT value FROM record WHERE collection = ?1 ORDER BY id")?;
            let values = rows.query_map(params![collection], |row| row.get(0))?;
            values.collect()
        })
    }

    pub fn collections(&self, name: &str) -> Result<Vec<String>, CopyError> {
        self.with(name, |db| {
            let mut rows =
                db.prepare_cached("SELECT DISTINCT collection FROM record ORDER BY collection")?;
            let names = rows.query_map([], |row| row.get(0))?;
            names.collect()
        })
    }

    /// The copy deleted rather than emptied (offline-sync.md §9.6), its key kept: what the engine
    /// asks of the store on `sync.cursor_too_old`, a forgotten device and its reset. The next use
    /// opens a new, empty one under the same key.
    pub fn clear(&self, name: &str) -> Result<(), CopyError> {
        let segments = segments(name)?;
        self.lock().remove(name);
        remove_files(&self.path_of(segments))
    }

    /// Sign-out: the copy and its key, so nothing on this device can read what was there.
    pub fn forget(&self, name: &str) -> Result<(), CopyError> {
        self.clear(name)?;
        self.keystore.remove(name)
    }

    fn with<T>(
        &self,
        name: &str,
        act: impl FnOnce(&Connection) -> rusqlite::Result<T>,
    ) -> Result<T, CopyError> {
        let segments = segments(name)?;
        let mut open = self.lock();
        if !open.contains_key(name) {
            let db = self.open_copy(name, &self.path_of(segments))?;
            open.insert(name.to_string(), db);
        }
        let db = open.get(name).ok_or(CopyError::Name)?;
        Ok(act(db)?)
    }

    fn open_copy(&self, name: &str, path: &Path) -> Result<Connection, CopyError> {
        let key = match self.keystore.read(name)? {
            Some(key) if key.len() == KEY_LEN => key,
            _ => {
                remove_files(path)?;
                let key = new_key()?;
                self.keystore.write(name, &key)?;
                key
            }
        };
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent).map_err(CopyError::File)?;
        }
        match open_with(path, &key) {
            Ok(db) => Ok(db),
            // A file the key does not open was written under another one; nobody can read it.
            Err(_) => {
                remove_files(path)?;
                Ok(open_with(path, &key)?)
            }
        }
    }

    fn path_of(&self, (workspace, account): (&str, &str)) -> PathBuf {
        self.root.join(workspace).join(format!("{account}.db"))
    }

    fn lock(&self) -> MutexGuard<'_, HashMap<String, Connection>> {
        // A panic while the map was held leaves connections that are still sound.
        self.open
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
    }
}

/// The workspace and the account a name holds, `<workspace>/<account>`, each as `copyNameFor`
/// escapes it. Anything else is refused, so a name never reaches outside the copies' directory.
fn segments(name: &str) -> Result<(&str, &str), CopyError> {
    let (workspace, account) = name.split_once('/').ok_or(CopyError::Name)?;
    if is_segment(workspace) && is_segment(account) {
        Ok((workspace, account))
    } else {
        Err(CopyError::Name)
    }
}

fn is_segment(segment: &str) -> bool {
    !segment.is_empty()
        && segment.len() <= SEGMENT_MAX
        && segment != "."
        && segment != ".."
        && segment
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'.' || b == b'-' || b == b'_')
}

fn open_with(path: &Path, key: &[u8]) -> rusqlite::Result<Connection> {
    let db = Connection::open(path)?;
    // A raw key in SQLCipher's `x'…'` form: hex only, so nothing of it is SQL.
    db.execute_batch(&format!("PRAGMA key = \"x'{}'\";", hex(key)))?;
    // The first read: a wrong key fails here, as `SQLITE_NOTADB`.
    db.execute_batch(
        "PRAGMA journal_mode = WAL;
         PRAGMA synchronous = NORMAL;
         CREATE TABLE IF NOT EXISTS record (
           collection TEXT NOT NULL,
           id TEXT NOT NULL,
           value TEXT NOT NULL,
           PRIMARY KEY (collection, id)
         ) WITHOUT ROWID;",
    )?;
    Ok(db)
}

/// The database and what SQLite keeps beside it.
fn remove_files(path: &Path) -> Result<(), CopyError> {
    let mut files = vec![path.to_path_buf()];
    for suffix in ["-wal", "-shm", "-journal"] {
        let mut beside = path.as_os_str().to_owned();
        beside.push(suffix);
        files.push(PathBuf::from(beside));
    }
    for file in files {
        match std::fs::remove_file(&file) {
            Err(error) if error.kind() != io::ErrorKind::NotFound => {
                return Err(CopyError::File(error))
            }
            _ => {}
        }
    }
    Ok(())
}

pub(crate) fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

fn new_key() -> Result<Vec<u8>, CopyError> {
    let mut key = vec![0u8; KEY_LEN];
    fill_random(&mut key).map_err(CopyError::Random)?;
    Ok(key)
}

// The system's own randomness, through the standard library and the system's interface alone:
// no crate beyond F7 Decision 4 is admitted for it.
#[cfg(unix)]
fn fill_random(buffer: &mut [u8]) -> io::Result<()> {
    use std::io::Read;
    std::fs::File::open("/dev/urandom")?.read_exact(buffer)
}

#[cfg(windows)]
fn fill_random(buffer: &mut [u8]) -> io::Result<()> {
    #[link(name = "bcrypt")]
    extern "system" {
        fn BCryptGenRandom(
            algorithm: *mut std::ffi::c_void,
            buffer: *mut u8,
            size: u32,
            flags: u32,
        ) -> i32;
    }
    const BCRYPT_USE_SYSTEM_PREFERRED_RNG: u32 = 0x0000_0002;
    let size = u32::try_from(buffer.len()).map_err(io::Error::other)?;
    // SAFETY: the buffer is valid for `size` bytes, and the system's generator takes no handle.
    let status = unsafe {
        BCryptGenRandom(
            std::ptr::null_mut(),
            buffer.as_mut_ptr(),
            size,
            BCRYPT_USE_SYSTEM_PREFERRED_RNG,
        )
    };
    if status == 0 {
        Ok(())
    } else {
        Err(io::Error::other(format!(
            "BCryptGenRandom answered {status:#x}"
        )))
    }
}

/// The platform's keystore (ADR-0033 §4): the Windows Credential Manager, the Secret Service on
/// Linux, the Keychain where the shell is built for development on a Mac. Its entries are filed
/// under a service of the app's identifier and what it keeps there.
pub struct SystemKeystore {
    #[cfg_attr(target_os = "ios", allow(dead_code))]
    pub(crate) service: &'static str,
}

impl SystemKeystore {
    /// The copies' keys.
    pub const COPIES: SystemKeystore = SystemKeystore {
        service: "eu.hubtask.app.copy",
    };
    /// The sessions the app signed in with, one per workspace (UC-SYN-07 checks 3 and 4).
    pub const SESSIONS: SystemKeystore = SystemKeystore {
        service: "eu.hubtask.app.session",
    };
}

#[cfg(not(any(target_os = "android", target_os = "ios")))]
impl SystemKeystore {
    fn entry(&self, name: &str) -> Result<keyring::Entry, CopyError> {
        keyring::Entry::new(self.service, name).map_err(refused)
    }
}

#[cfg(not(any(target_os = "android", target_os = "ios")))]
fn refused(error: keyring::Error) -> CopyError {
    CopyError::Keystore(error.to_string())
}

#[cfg(not(any(target_os = "android", target_os = "ios")))]
impl Keystore for SystemKeystore {
    fn read(&self, name: &str) -> Result<Option<Vec<u8>>, CopyError> {
        match self.entry(name)?.get_secret() {
            Ok(key) => Ok(Some(key)),
            Err(keyring::Error::NoEntry) => Ok(None),
            Err(error) => Err(refused(error)),
        }
    }

    fn write(&self, name: &str, key: &[u8]) -> Result<(), CopyError> {
        self.entry(name)?.set_secret(key).map_err(refused)
    }

    fn remove(&self, name: &str) -> Result<(), CopyError> {
        match self.entry(name)?.delete_credential() {
            Ok(()) | Err(keyring::Error::NoEntry) => Ok(()),
            Err(error) => Err(refused(error)),
        }
    }
}

// The Android app's keystore is its own bridge (`android.rs`); iOS waits for `2.0` (ADR-0084), so
// there no copy opens and the app works online.
#[cfg(target_os = "ios")]
impl Keystore for SystemKeystore {
    fn read(&self, _name: &str) -> Result<Option<Vec<u8>>, CopyError> {
        Err(CopyError::Keystore(
            "no keystore on this target".to_string(),
        ))
    }
    fn write(&self, _name: &str, _key: &[u8]) -> Result<(), CopyError> {
        Err(CopyError::Keystore(
            "no keystore on this target".to_string(),
        ))
    }
    fn remove(&self, _name: &str) -> Result<(), CopyError> {
        Ok(())
    }
}

// The engine's storage contract (packages/sync-engine/test/storage.test.ts) against this store,
// case by case, and what only the shells promise: the file unreadable without its key, sign-out
// taking file and key, the copy surviving a restart.
#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;

    const NAME: &str = "https_3A_2F_2Fhubtask.example/account-1";

    #[derive(Default, Clone)]
    struct MemoryKeystore(Arc<Mutex<HashMap<String, Vec<u8>>>>);

    impl MemoryKeystore {
        fn held(&self, name: &str) -> Option<Vec<u8>> {
            self.0.lock().unwrap().get(name).cloned()
        }
    }

    impl Keystore for MemoryKeystore {
        fn read(&self, name: &str) -> Result<Option<Vec<u8>>, CopyError> {
            Ok(self.held(name))
        }
        fn write(&self, name: &str, key: &[u8]) -> Result<(), CopyError> {
            self.0
                .lock()
                .unwrap()
                .insert(name.to_string(), key.to_vec());
            Ok(())
        }
        fn remove(&self, name: &str) -> Result<(), CopyError> {
            self.0.lock().unwrap().remove(name);
            Ok(())
        }
    }

    /// A directory of its own per test, removed when the test ends.
    struct Scratch(PathBuf);

    impl Scratch {
        fn new(test: &str) -> Self {
            let dir =
                std::env::temp_dir().join(format!("hubtask-copy-{}-{test}", std::process::id()));
            let _ = std::fs::remove_dir_all(&dir);
            Scratch(dir)
        }
        fn copies(&self, keystore: &MemoryKeystore) -> Copies {
            Copies::new(self.0.clone(), Box::new(keystore.clone()))
        }
        fn file(&self) -> PathBuf {
            self.0
                .join("https_3A_2F_2Fhubtask.example")
                .join("account-1.db")
        }
    }

    impl Drop for Scratch {
        fn drop(&mut self) {
            let _ = std::fs::remove_dir_all(&self.0);
        }
    }

    #[test]
    fn a_record_is_put_read_back_listed_and_deleted() {
        let scratch = Scratch::new("crud");
        let copies = scratch.copies(&MemoryKeystore::default());
        assert_eq!(copies.get(NAME, "items", "a").unwrap(), None);

        copies
            .put(NAME, "items", "a", r#"{"id":"a","title":"one"}"#)
            .unwrap();
        copies
            .put(NAME, "items", "b", r#"{"id":"b","title":"two"}"#)
            .unwrap();
        copies
            .put(NAME, "containers", "c", r#"{"id":"c"}"#)
            .unwrap();
        assert_eq!(
            copies.get(NAME, "items", "a").unwrap().as_deref(),
            Some(r#"{"id":"a","title":"one"}"#)
        );
        assert_eq!(copies.all(NAME, "items").unwrap().len(), 2);
        assert_eq!(copies.collections(NAME).unwrap(), ["containers", "items"]);

        copies
            .put(NAME, "items", "a", r#"{"id":"a","title":"one, again"}"#)
            .unwrap();
        assert_eq!(
            copies.get(NAME, "items", "a").unwrap().as_deref(),
            Some(r#"{"id":"a","title":"one, again"}"#),
            "a put replaces"
        );

        copies.delete(NAME, "items", "a").unwrap();
        assert_eq!(copies.get(NAME, "items", "a").unwrap(), None);
        assert_eq!(
            copies.all(NAME, "items").unwrap(),
            [r#"{"id":"b","title":"two"}"#]
        );
        copies.delete(NAME, "items", "never-there").unwrap();
    }

    #[test]
    fn a_collection_this_build_never_named_is_a_collection_like_any_other() {
        let scratch = Scratch::new("unknown");
        let copies = scratch.copies(&MemoryKeystore::default());
        let record = r#"{"id":"r","extra":{"nested":[1,2]}}"#;
        copies.put(NAME, "something_newer", "r", record).unwrap();
        assert_eq!(
            copies.get(NAME, "something_newer", "r").unwrap().as_deref(),
            Some(record)
        );
        assert!(copies
            .collections(NAME)
            .unwrap()
            .contains(&"something_newer".to_string()));
    }

    #[test]
    fn clear_deletes_the_file_keeps_the_key_and_the_copy_works_again() {
        let scratch = Scratch::new("clear");
        let keystore = MemoryKeystore::default();
        let copies = scratch.copies(&keystore);
        copies.put(NAME, "items", "a", "{}").unwrap();
        copies.put(NAME, "meta", "device", "{}").unwrap();
        let key = keystore.held(NAME).expect("a key was created on first use");

        copies.clear(NAME).unwrap();
        assert!(
            !scratch.file().exists(),
            "the database is deleted, not emptied"
        );
        assert_eq!(keystore.held(NAME), Some(key.clone()), "the key stays");

        assert_eq!(copies.get(NAME, "items", "a").unwrap(), None);
        assert_eq!(copies.get(NAME, "meta", "device").unwrap(), None);
        assert!(copies.collections(NAME).unwrap().is_empty());
        copies.put(NAME, "items", "b", "{}").unwrap();
        assert_eq!(
            copies.get(NAME, "items", "b").unwrap().as_deref(),
            Some("{}")
        );
        assert_eq!(keystore.held(NAME), Some(key));
    }

    #[test]
    fn the_copy_survives_a_restart_under_the_same_key() {
        let scratch = Scratch::new("restart");
        let keystore = MemoryKeystore::default();
        scratch
            .copies(&keystore)
            .put(NAME, "items", "a", r#"{"title":"kept"}"#)
            .unwrap();
        let key = keystore.held(NAME);

        // A new start of the app: nothing held open, the key read from the keystore again.
        let again = scratch.copies(&keystore);
        assert_eq!(
            again.get(NAME, "items", "a").unwrap().as_deref(),
            Some(r#"{"title":"kept"}"#)
        );
        assert_eq!(
            keystore.held(NAME),
            key,
            "the key is created once, not per start"
        );
    }

    #[test]
    fn the_file_is_unreadable_without_its_key() {
        let scratch = Scratch::new("unreadable");
        let keystore = MemoryKeystore::default();
        let copies = scratch.copies(&keystore);
        copies
            .put(NAME, "items", "a", r#"{"title":"Quarterly salaries"}"#)
            .unwrap();
        drop(copies);

        let bytes = std::fs::read(scratch.file()).unwrap();
        assert!(
            !bytes.starts_with(b"SQLite format 3"),
            "the header is encrypted too"
        );
        assert!(
            !bytes.windows(b"Quarterly".len()).any(|w| w == b"Quarterly"),
            "no record's text is in the file"
        );

        // Opened without a key, as any SQLite would: not a database.
        let bare = Connection::open(scratch.file()).unwrap();
        assert!(bare
            .query_row("SELECT count(*) FROM sqlite_master", [], |row| row
                .get::<_, i64>(0))
            .is_err());
        // Nor with another key.
        assert!(open_with(&scratch.file(), &[7u8; KEY_LEN]).is_err());
        // With its own, it reads.
        let own = open_with(&scratch.file(), &keystore.held(NAME).unwrap()).unwrap();
        let title: String = own
            .query_row("SELECT value FROM record WHERE id = 'a'", [], |row| {
                row.get(0)
            })
            .unwrap();
        assert!(title.contains("Quarterly"));
    }

    #[test]
    fn sign_out_removes_the_file_and_the_key() {
        let scratch = Scratch::new("forget");
        let keystore = MemoryKeystore::default();
        let copies = scratch.copies(&keystore);
        copies.put(NAME, "items", "a", "{}").unwrap();
        assert!(scratch.file().exists());

        copies.forget(NAME).unwrap();
        assert!(!scratch.file().exists(), "the file is gone");
        for suffix in ["-wal", "-shm"] {
            let mut beside = scratch.file().into_os_string();
            beside.push(suffix);
            assert!(!PathBuf::from(beside).exists(), "nothing beside it stays");
        }
        assert_eq!(keystore.held(NAME), None, "the key is gone");
        // Forgetting what is not there is not an error: a sign-out without a copy.
        copies.forget(NAME).unwrap();
    }

    #[test]
    fn a_copy_whose_key_is_lost_starts_again_empty() {
        let scratch = Scratch::new("lost");
        let keystore = MemoryKeystore::default();
        scratch
            .copies(&keystore)
            .put(NAME, "items", "a", "{}")
            .unwrap();
        keystore.remove(NAME).unwrap();

        let again = scratch.copies(&keystore);
        assert_eq!(again.get(NAME, "items", "a").unwrap(), None);
        assert!(keystore.held(NAME).is_some(), "under a new key");
        again.put(NAME, "items", "b", "{}").unwrap();
    }

    #[test]
    fn two_accounts_and_two_workspaces_hold_separate_copies_and_keys() {
        let scratch = Scratch::new("separate");
        let keystore = MemoryKeystore::default();
        let copies = scratch.copies(&keystore);
        let names = [
            NAME,
            "https_3A_2F_2Fhubtask.example/account-2",
            "https_3A_2F_2Fother.example/account-1",
        ];
        for (i, name) in names.iter().enumerate() {
            copies.put(name, "items", "a", &i.to_string()).unwrap();
        }
        for (i, name) in names.iter().enumerate() {
            assert_eq!(copies.get(name, "items", "a").unwrap(), Some(i.to_string()));
        }
        let keys: Vec<_> = names
            .iter()
            .map(|name| keystore.held(name).unwrap())
            .collect();
        assert!(
            keys[0] != keys[1] && keys[1] != keys[2] && keys[0] != keys[2],
            "a key per copy"
        );

        copies.forget(names[0]).unwrap();
        assert_eq!(
            copies.get(names[1], "items", "a").unwrap().as_deref(),
            Some("1"),
            "nothing else goes"
        );
        assert!(keystore.held(names[2]).is_some());
    }

    #[test]
    fn a_name_never_reaches_outside_the_copies() {
        for name in [
            "",
            "account",
            "/account",
            "workspace/",
            "../account",
            "workspace/..",
            "a/b/c",
            "a b/c",
            "a/c:d",
        ] {
            assert!(
                matches!(segments(name), Err(CopyError::Name)),
                "{name:?} was accepted"
            );
        }
        assert_eq!(
            segments(NAME).unwrap(),
            ("https_3A_2F_2Fhubtask.example", "account-1")
        );
        let scratch = Scratch::new("names");
        let copies = scratch.copies(&MemoryKeystore::default());
        let refused = copies.put("../escape", "items", "a", "{}").unwrap_err();
        assert_eq!(refused.code(), "copy.name_invalid");
    }

    #[test]
    fn a_new_key_is_random() {
        let (one, two) = (new_key().unwrap(), new_key().unwrap());
        assert_eq!(one.len(), KEY_LEN);
        assert_ne!(one, two);
    }
}
