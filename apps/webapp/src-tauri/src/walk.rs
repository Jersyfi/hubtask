// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//! The smoke walk's stand-in for the system browser (UC-SYN-07 check 9), built only with the
//! `walk` feature, which no release turns on. WebDriver drives the app's webview, never the
//! system's browser, so the walk build hands every address the app would open there to the walk
//! on a fixed loopback port, and the walk answers the workspace's consent as a person would there.
//! Nothing else differs from the app a person installs.

use std::io::{self, Read, Write};
use std::net::{SocketAddr, TcpStream};
use std::time::Duration;

/// Where the walk listens: this machine, or the emulator's host through `adb reverse`.
pub const STAND_IN: ([u8; 4], u16) = ([127, 0, 0, 1], 4780);

/// Rule 7: a stand-in that does not answer fails the step rather than holding the window.
const LIMIT: Duration = Duration::from_secs(5);

/// Hands `url` to the walk, as the system would hand it to the browser.
pub fn open(url: &str) -> io::Result<()> {
    open_at(SocketAddr::from(STAND_IN), url)
}

fn open_at(at: SocketAddr, url: &str) -> io::Result<()> {
    let mut stream = TcpStream::connect_timeout(&at, LIMIT)?;
    stream.set_read_timeout(Some(LIMIT))?;
    stream.set_write_timeout(Some(LIMIT))?;
    write!(
        stream,
        "POST /open HTTP/1.1\r\nHost: 127.0.0.1\r\nContent-Type: text/plain\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{url}",
        url.len()
    )?;
    let mut answer = String::new();
    stream.take(1024).read_to_string(&mut answer)?;
    if answer.starts_with("HTTP/1.1 204") || answer.starts_with("HTTP/1.0 204") {
        Ok(())
    } else {
        Err(io::Error::other("the walk refused the address"))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::BufRead;
    use std::net::{Ipv4Addr, TcpListener};
    use std::thread;

    #[test]
    fn the_address_reaches_the_walk_whole() {
        let listener = TcpListener::bind((Ipv4Addr::LOCALHOST, 0)).unwrap();
        let at = listener.local_addr().unwrap();
        let walk = thread::spawn(move || {
            let (stream, _) = listener.accept().unwrap();
            let mut reader = io::BufReader::new(stream);
            let mut length = 0;
            loop {
                let mut line = String::new();
                reader.read_line(&mut line).unwrap();
                if let Some(value) = line.strip_prefix("Content-Length: ") {
                    length = value.trim().parse().unwrap();
                }
                if line == "\r\n" {
                    break;
                }
            }
            let mut body = vec![0; length];
            reader.read_exact(&mut body).unwrap();
            reader
                .get_mut()
                .write_all(b"HTTP/1.1 204 No Content\r\n\r\n")
                .unwrap();
            String::from_utf8(body).unwrap()
        });
        let url = "http://localhost:18080/oauth/consent?client_id=a&state=b%20c";
        open_at(at, url).unwrap();
        assert_eq!(walk.join().unwrap(), url);
    }

    #[test]
    fn no_walk_listening_is_an_error_not_a_wait() {
        let free = TcpListener::bind((Ipv4Addr::LOCALHOST, 0)).unwrap();
        let at = free.local_addr().unwrap();
        drop(free);
        assert!(open_at(at, "http://localhost/").is_err());
    }
}
