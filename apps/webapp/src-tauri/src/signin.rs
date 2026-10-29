// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//! The sign-in through the system browser (ADR-0096, identity.md §15.3): the loopback address the
//! workspace's consent sends its code back to, and the session the app keeps in the system's
//! keystore. The web app runs the flow; only what a page may not do is here.

use std::io::{self, BufRead, BufReader, Write};
use std::net::{Ipv4Addr, TcpListener, TcpStream};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use crate::copy::Keystore;

/// The one path the listener answers; the workspace's consent appends `code` and `state`.
pub const REDIRECT_PATH: &str = "/oauth";

/// A request line longer than this is nobody's redirect.
const REQUEST_MAX: usize = 8 * 1024;

/// A connection that sends nothing is dropped after this long, so one silent client cannot hold
/// the listener while the browser's redirect waits.
const READ_TIMEOUT: Duration = Duration::from_secs(10);

/// What came back to the loopback address: the code and the state, or the consent's refusal.
#[derive(Debug, Default, PartialEq, Eq, Clone)]
pub struct Returned {
    pub code: Option<String>,
    pub state: Option<String>,
    pub error: Option<String>,
}

impl Returned {
    /// What the web app receives: only the fields that came back.
    pub fn fields(self) -> std::collections::HashMap<&'static str, String> {
        [
            ("code", self.code),
            ("state", self.state),
            ("error", self.error),
        ]
        .into_iter()
        .filter_map(|(key, value)| value.map(|value| (key, value)))
        .collect()
    }
}

/// The page the browser shows once the code is taken: the app's own words, handed in by the web
/// app in the reader's language, so it holds nothing of the workspace - and the card's look, the
/// web app's resolved tokens as a stylesheet and its mark, since the page loads nothing.
#[derive(Debug, Clone)]
pub struct Page {
    pub title: String,
    pub body: String,
    pub lang: String,
    pub style: String,
    pub mark: String,
}

/// The listeners of this run. A new sign-in retires the one before, so only the latest browser
/// tab can hand a code back.
#[derive(Default)]
pub struct Listeners {
    generation: Arc<AtomicU64>,
    current: Mutex<Option<u16>>,
}

impl Listeners {
    /// Opens a listener on a loopback port the system picks (RFC 8252 §7.3) and answers it; the
    /// redirect, when it comes, goes to `deliver`, once.
    pub fn listen(
        &self,
        page: Page,
        deliver: impl FnOnce(Returned) + Send + 'static,
    ) -> io::Result<u16> {
        self.retire();
        let listener = TcpListener::bind((Ipv4Addr::LOCALHOST, 0))?;
        let port = listener.local_addr()?.port();
        let mine = self.generation.fetch_add(1, Ordering::SeqCst) + 1;
        let generation = Arc::clone(&self.generation);
        *self
            .current
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner()) = Some(port);

        std::thread::spawn(move || {
            for stream in listener.incoming() {
                if generation.load(Ordering::SeqCst) != mine {
                    return;
                }
                let Ok(stream) = stream else { continue };
                if let Some(returned) = answer(stream, &page) {
                    deliver(returned);
                    return;
                }
            }
        });
        Ok(port)
    }

    /// Ends the waiting listener, if there is one: *Use another address*, or the next sign-in.
    pub fn retire(&self) {
        self.generation.fetch_add(1, Ordering::SeqCst);
        let held = self
            .current
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .take();
        if let Some(port) = held {
            // Wakes the blocked accept, which then sees it is no longer the current one.
            let _ = TcpStream::connect((Ipv4Addr::LOCALHOST, port));
        }
    }
}

/// Reads one request and answers it. `Some` only for the redirect itself; anything else - the
/// browser's icon request, a probe - is refused and the listener keeps waiting.
fn answer(mut stream: TcpStream, page: &Page) -> Option<Returned> {
    let _ = stream.set_read_timeout(Some(READ_TIMEOUT));
    let mut line = String::new();
    let read = BufReader::new((&stream).take_bounded())
        .read_line(&mut line)
        .ok()?;
    if read == 0 {
        return None;
    }
    match redirect_of(&line) {
        Some(returned) => {
            let _ = respond(&mut stream, "200 OK", &page_html(page));
            Some(returned)
        }
        None => {
            let _ = respond(&mut stream, "404 Not Found", "");
            None
        }
    }
}

trait TakeBounded: io::Read + Sized {
    fn take_bounded(self) -> io::Take<Self> {
        self.take(REQUEST_MAX as u64)
    }
}
impl<R: io::Read> TakeBounded for R {}

fn respond(stream: &mut TcpStream, status: &str, body: &str) -> io::Result<()> {
    // The page loads nothing and runs nothing; it is closed to everything else too.
    write!(
        stream,
        "HTTP/1.1 {status}\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: {}\r\n\
         Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'\r\n\
         Referrer-Policy: no-referrer\r\nCache-Control: no-store\r\nConnection: close\r\n\r\n{body}",
        body.len()
    )?;
    stream.flush()
}

/// The redirect's request line, `GET /oauth?code=…&state=… HTTP/1.1`, read into what came back.
fn redirect_of(line: &str) -> Option<Returned> {
    let mut parts = line.split_whitespace();
    if parts.next()? != "GET" {
        return None;
    }
    returned_from(parts.next()?)
}

/// The Android app's own scheme, which the consent sends the code to there (identity.md §15.3).
pub const APP_SCHEME: &str = "eu.hubtask.app:";

/// The address Android handed the app, `eu.hubtask.app:/oauth?code=…&state=…`, read into what
/// came back; `None` for any other address that reached the app.
#[cfg_attr(not(target_os = "android"), allow(dead_code))]
pub fn app_return(url: &str) -> Option<Returned> {
    returned_from(url.strip_prefix(APP_SCHEME)?)
}

/// `/oauth?…`, the redirect's path and query, read into what came back.
fn returned_from(target: &str) -> Option<Returned> {
    let (path, query) = target.split_once('?').unwrap_or((target, ""));
    if path != REDIRECT_PATH {
        return None;
    }
    let mut returned = Returned::default();
    for pair in query.split('&') {
        let (key, value) = pair.split_once('=').unwrap_or((pair, ""));
        let value = decoded(value)?;
        match key {
            "code" => returned.code = Some(value),
            "state" => returned.state = Some(value),
            "error" => returned.error = Some(value),
            _ => {}
        }
    }
    if returned.code.is_none() && returned.error.is_none() {
        return None;
    }
    Some(returned)
}

/// Percent-decoding of a query value; `None` for one that is not well formed.
fn decoded(raw: &str) -> Option<String> {
    let bytes = raw.as_bytes();
    let mut out = Vec::with_capacity(bytes.len());
    let mut index = 0;
    while index < bytes.len() {
        match bytes[index] {
            b'%' => {
                let hex = raw.get(index + 1..index + 3)?;
                out.push(u8::from_str_radix(hex, 16).ok()?);
                index += 3;
            }
            b'+' => {
                out.push(b' ');
                index += 1;
            }
            byte => {
                out.push(byte);
                index += 1;
            }
        }
    }
    String::from_utf8(out).ok()
}

fn escaped(text: &str) -> String {
    text.chars()
        .map(|c| match c {
            '&' => "&amp;".to_string(),
            '<' => "&lt;".to_string(),
            '>' => "&gt;".to_string(),
            '"' => "&quot;".to_string(),
            '\'' => "&#39;".to_string(),
            other => other.to_string(),
        })
        .collect()
}

/// The stylesheet as handed in, where nothing in it can end the element; the plain one otherwise.
fn style_of(page: &Page) -> &str {
    if page.style.is_empty() || page.style.contains('<') {
        "body{font-family:system-ui,sans-serif}"
    } else {
        &page.style
    }
}

/// The mark as handed in, where it is one drawing and nothing else; none otherwise. The page's
/// policy runs no script whatever it holds - this keeps it to what it is for.
fn mark_of(page: &Page) -> &str {
    let mark = page.mark.trim();
    let lower = mark.to_ascii_lowercase();
    let one_drawing =
        mark.starts_with("<svg") && mark.ends_with("</svg>") && lower.matches("<svg").count() == 1;
    if one_drawing
        && !lower.contains("script")
        && !lower.contains("foreignobject")
        && !lower.contains(" on")
    {
        mark
    } else {
        ""
    }
}

fn page_html(page: &Page) -> String {
    let lang: String = page
        .lang
        .chars()
        .filter(|c| c.is_ascii_alphanumeric() || *c == '-')
        .take(35)
        .collect();
    format!(
        "<!doctype html><html lang=\"{lang}\"><meta charset=\"utf-8\">\
         <meta name=\"viewport\" content=\"width=device-width\"><title>{title}</title>\
         <style>{style}</style>\
         <main><span class=\"logo\">{mark}<span class=\"wordmark\" translate=\"no\">Hubtask</span></span>\
         <div class=\"text\"><h1>{title}</h1><p>{body}</p></div></main></html>",
        title = escaped(&page.title),
        body = escaped(&page.body),
        style = style_of(page),
        mark = mark_of(page),
    )
}

/// The sessions the app keeps, one per workspace it holds (UC-SYN-07 checks 3 and 4, UC-ID-25
/// check 7): the pair and the account, as the web app hands them over, in the system's keystore and
/// nowhere else. Each is filed under its workspace's origin, so two workspaces - of one installation
/// or of two - never share an entry, and forgetting one leaves the others as they are.
pub struct Sessions {
    keystore: Box<dyn Keystore>,
}

/// More than any pair and its address; a larger value is not one.
const SESSION_MAX: usize = 16 * 1024;

impl Sessions {
    pub fn new(keystore: Box<dyn Keystore>) -> Self {
        Self { keystore }
    }

    pub fn read(&self, server: &str) -> Result<Option<String>, &'static str> {
        let held = self
            .keystore
            .read(entry_of(server)?)
            .map_err(|_| UNAVAILABLE)?;
        Ok(held.and_then(|bytes| String::from_utf8(bytes).ok()))
    }

    pub fn keep(&self, server: &str, value: &str) -> Result<(), &'static str> {
        let entry = entry_of(server)?;
        if value.is_empty() || value.len() > SESSION_MAX {
            return Err("session.invalid");
        }
        self.keystore
            .write(entry, value.as_bytes())
            .map_err(|_| UNAVAILABLE)
    }

    pub fn forget(&self, server: &str) -> Result<(), &'static str> {
        self.keystore
            .remove(entry_of(server)?)
            .map_err(|_| UNAVAILABLE)
    }
}

/// The entry a workspace's session is filed under: its origin, as the app speaks to it - an address
/// the browser may open, with nothing after the host.
fn entry_of(server: &str) -> Result<&str, &'static str> {
    let rest = server.split_once("://").map_or("", |(_, rest)| rest);
    if server.len() > 300 || rest.contains(['/', '?', '#']) || !openable(server) {
        return Err("session.invalid");
    }
    Ok(server)
}

const UNAVAILABLE: &str = "session.keystore_unavailable";

/// Whether the app may hand an address to the system's browser: a workspace's, over HTTPS - or
/// this machine's over HTTP, for development - and never anything a page could make run.
pub fn openable(url: &str) -> bool {
    let Some((scheme, rest)) = url.split_once("://") else {
        return false;
    };
    let authority = rest.split(['/', '?', '#']).next().unwrap_or_default();
    if authority.is_empty() || authority.contains('@') || url.contains(char::is_whitespace) {
        return false;
    }
    let name = authority
        .rsplit_once(':')
        .map_or(authority, |(name, _)| name);
    match scheme {
        "https" => true,
        "http" => matches!(name, "localhost" | "127.0.0.1" | "[::1]"),
        _ => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::copy::{Copies, CopyError};
    use std::collections::HashMap;
    use std::io::Read;
    use std::sync::mpsc;

    #[derive(Default, Clone)]
    struct MemoryKeystore(Arc<Mutex<HashMap<String, Vec<u8>>>>);

    impl Keystore for MemoryKeystore {
        fn read(&self, name: &str) -> Result<Option<Vec<u8>>, CopyError> {
            Ok(self.0.lock().unwrap().get(name).cloned())
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

    fn page() -> Page {
        Page {
            title: "Back to the app".into(),
            body: "You can close <this> tab.".into(),
            lang: "en\"><script>".into(),
            style: "main{display:grid}".into(),
            mark: "<svg viewBox=\"0 0 64 64\"><circle cx=\"54\" cy=\"12\" r=\"5\" fill=\"rgb(242, 161, 0)\"></circle></svg>".into(),
        }
    }

    #[test]
    fn the_android_return_is_read_and_anything_else_is_not() {
        let cases = [
            (
                "eu.hubtask.app:/oauth?code=hbt_oac_x&state=s%2D1",
                Some(Returned {
                    code: Some("hbt_oac_x".to_string()),
                    state: Some("s-1".to_string()),
                    error: None,
                }),
            ),
            (
                "eu.hubtask.app:/oauth?error=access_denied&state=s",
                Some(Returned {
                    code: None,
                    state: Some("s".to_string()),
                    error: Some("access_denied".to_string()),
                }),
            ),
            ("eu.hubtask.app:/oauth", None),
            ("eu.hubtask.app:/other?code=x", None),
            ("eu.hubtask.app://oauth?code=x", None),
            ("https://tasks.example.com/oauth?code=x", None),
            ("eu.hubtask.app:/oauth?code=%zz", None),
        ];
        for (url, want) in cases {
            assert_eq!(app_return(url), want, "{url}");
        }
    }

    #[test]
    fn the_redirect_is_read_and_anything_else_is_not() {
        let cases = [
            (
                "GET /oauth?code=hbt_oac_x&state=s%2D1 HTTP/1.1\r\n",
                Some(Returned {
                    code: Some("hbt_oac_x".into()),
                    state: Some("s-1".into()),
                    error: None,
                }),
            ),
            (
                "GET /oauth?error=access_denied&state=s HTTP/1.1\r\n",
                Some(Returned {
                    code: None,
                    state: Some("s".into()),
                    error: Some("access_denied".into()),
                }),
            ),
            ("GET /favicon.ico HTTP/1.1\r\n", None),
            ("GET /oauth HTTP/1.1\r\n", None),
            ("POST /oauth?code=x HTTP/1.1\r\n", None),
            ("GET /oauth/?code=x HTTP/1.1\r\n", None),
            ("GET /oauth?code=%zz HTTP/1.1\r\n", None),
        ];
        for (line, want) in cases {
            assert_eq!(redirect_of(line), want, "{line}");
        }
    }

    #[test]
    fn the_page_escapes_what_it_is_handed() {
        let html = page_html(&page());
        assert!(html.contains("You can close &lt;this&gt; tab."));
        assert!(html.contains("lang=\"enscript\""));
        assert!(!html.contains("<script>"));
        assert!(html.contains("<style>main{display:grid}</style>"));
        assert!(html.contains("<svg viewBox"));
    }

    #[test]
    fn the_look_is_taken_only_where_it_cannot_be_more_than_a_look() {
        let mut handed = page();
        handed.style = "main{}</style><script>alert(1)</script>".into();
        handed.mark = "<svg onload=\"alert(1)\"></svg>".into();
        let html = page_html(&handed);
        assert!(!html.contains("<script") && !html.contains("alert"));
        assert!(!html.contains("onload"));
        assert!(html.contains("<style>body{font-family:system-ui,sans-serif}</style>"));
        handed.mark = "<svg></svg><svg><foreignObject></foreignObject></svg>".into();
        assert!(!page_html(&handed).contains("foreignObject"));
    }

    fn get(port: u16, target: &str) -> String {
        let mut stream = TcpStream::connect((Ipv4Addr::LOCALHOST, port)).unwrap();
        write!(stream, "GET {target} HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n").unwrap();
        let mut answer = String::new();
        stream.read_to_string(&mut answer).unwrap();
        answer
    }

    #[test]
    fn the_listener_hands_back_one_redirect_on_loopback() {
        let listeners = Listeners::default();
        let (sent, received) = mpsc::channel();
        let port = listeners
            .listen(page(), move |returned| sent.send(returned).unwrap())
            .unwrap();

        assert!(get(port, "/favicon.ico").starts_with("HTTP/1.1 404"));
        let answer = get(port, "/oauth?code=c1&state=s1");
        assert!(answer.starts_with("HTTP/1.1 200"), "{answer}");
        assert!(answer.contains("Back to the app"));
        assert!(answer.contains("Content-Security-Policy: default-src 'none'"));

        let returned = received.recv_timeout(Duration::from_secs(5)).unwrap();
        assert_eq!(returned.code.as_deref(), Some("c1"));
        assert_eq!(returned.state.as_deref(), Some("s1"));
        // One redirect, then the port is closed.
        std::thread::sleep(Duration::from_millis(50));
        assert!(TcpStream::connect((Ipv4Addr::LOCALHOST, port)).is_err());
    }

    #[test]
    fn a_new_sign_in_retires_the_waiting_one() {
        let listeners = Listeners::default();
        let (sent, received) = mpsc::channel::<Returned>();
        let stale = sent.clone();
        let first = listeners
            .listen(page(), move |r| stale.send(r).unwrap())
            .unwrap();
        let _second = listeners
            .listen(page(), move |r| sent.send(r).unwrap())
            .unwrap();
        std::thread::sleep(Duration::from_millis(50));
        let _ = TcpStream::connect((Ipv4Addr::LOCALHOST, first)).map(|mut stream| {
            let _ = write!(stream, "GET /oauth?code=old HTTP/1.1\r\n\r\n");
        });
        assert!(received.recv_timeout(Duration::from_millis(300)).is_err());
    }

    const ACME: &str = "https://tasks.acme.com";
    const VOGT: &str = "https://hubtask.vogt.example";

    #[test]
    fn the_session_lives_in_the_keystore() {
        let keystore = MemoryKeystore::default();
        let sessions = Sessions::new(Box::new(keystore.clone()));
        assert_eq!(sessions.read(ACME), Ok(None));
        sessions
            .keep(ACME, "{\"server\":\"https://tasks.acme.com\"}")
            .unwrap();
        assert!(keystore.0.lock().unwrap().contains_key(ACME));

        // A restart: a new holder over the same keystore reads the same session.
        let again = Sessions::new(Box::new(keystore.clone()));
        assert_eq!(
            again.read(ACME).unwrap().as_deref(),
            Some("{\"server\":\"https://tasks.acme.com\"}")
        );

        again.forget(ACME).unwrap();
        assert!(keystore.0.lock().unwrap().is_empty());
        assert_eq!(sessions.keep(ACME, ""), Err("session.invalid"));
        assert_eq!(
            sessions.keep(ACME, &"x".repeat(SESSION_MAX + 1)),
            Err("session.invalid")
        );
    }

    #[test]
    fn a_session_is_filed_only_under_a_workspaces_origin() {
        let sessions = Sessions::new(Box::new(MemoryKeystore::default()));
        for server in [
            "",
            "session",
            "https://tasks.acme.com/",
            "https://tasks.acme.com/api/v1",
            "https://tasks.acme.com?x",
            "http://tasks.acme.com",
            "file:///etc/passwd",
        ] {
            assert_eq!(
                sessions.keep(server, "{}"),
                Err("session.invalid"),
                "{server}"
            );
            assert_eq!(sessions.read(server), Err("session.invalid"), "{server}");
        }
        sessions.keep("http://localhost:8080", "{}").unwrap();
    }

    // UC-SYN-07 check 4, UC-ID-25 check 7: two workspaces of two installations
    // hold separate sessions and separate copies, and forgetting one takes its session, its copy and
    // the copy's key - and nothing of the other.
    #[test]
    fn two_workspaces_hold_separate_sessions_and_copies_and_forgetting_one_keeps_the_other() {
        let dir = std::env::temp_dir().join(format!("hubtask-workspaces-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        let held = MemoryKeystore::default();
        let keys = MemoryKeystore::default();
        let sessions = Sessions::new(Box::new(held.clone()));
        let copies = Copies::new(dir.clone(), Box::new(keys.clone()));
        let acme_copy = "https_3A_2F_2Ftasks.acme.com/account-1";
        let vogt_copy = "https_3A_2F_2Fhubtask.vogt.example/account-1";

        sessions.keep(ACME, "{\"refresh\":\"acme\"}").unwrap();
        sessions.keep(VOGT, "{\"refresh\":\"vogt\"}").unwrap();
        copies.put(acme_copy, "items", "a", "\"acme\"").unwrap();
        copies.put(vogt_copy, "items", "a", "\"vogt\"").unwrap();
        assert_ne!(sessions.read(ACME).unwrap(), sessions.read(VOGT).unwrap());
        let held_keys = keys.0.lock().unwrap().clone();
        assert_ne!(
            held_keys.get(acme_copy),
            held_keys.get(vogt_copy),
            "a key per copy"
        );

        // Forgetting Acme: its session, then its copy with the key.
        sessions.forget(ACME).unwrap();
        copies.forget(acme_copy).unwrap();
        assert_eq!(sessions.read(ACME).unwrap(), None);
        assert!(!keys.0.lock().unwrap().contains_key(acme_copy));
        assert!(!dir
            .join("https_3A_2F_2Ftasks.acme.com")
            .join("account-1.db")
            .exists());

        assert_eq!(
            sessions.read(VOGT).unwrap().as_deref(),
            Some("{\"refresh\":\"vogt\"}")
        );
        assert!(keys.0.lock().unwrap().contains_key(vogt_copy));
        assert_eq!(
            copies.get(vogt_copy, "items", "a").unwrap().as_deref(),
            Some("\"vogt\"")
        );
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn only_a_workspace_address_opens_in_the_browser() {
        for (url, want) in [
            ("https://tasks.example.com/oauth/consent?client_id=x", true),
            ("http://localhost:8080/oauth/consent", true),
            ("http://127.0.0.1:8080/x", true),
            ("http://tasks.example.com/", false),
            ("file:///etc/passwd", false),
            ("javascript:alert(1)", false),
            ("https://user@evil.test/", false),
            ("https:///nothing", false),
            ("https://a b/", false),
        ] {
            assert_eq!(openable(url), want, "{url}");
        }
    }
}
