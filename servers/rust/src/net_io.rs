//! The TCP listener behind the HTTP server. Its streams rewrite the status
//! line of 413 answers to the reason phrase OpenSearch (Netty) sends,
//! "Request Entity Too Large", which hyper cannot be told to use.

use std::io;
use std::net::SocketAddr;
use std::pin::Pin;
use std::task::{Context, Poll};

use tokio::io::{AsyncRead, AsyncWrite, ReadBuf};
use tokio::net::{TcpListener, TcpStream};

const HYPER_413: &[u8] = b"HTTP/1.1 413 Payload Too Large\r\n";
const NETTY_413: &[u8] = b"HTTP/1.1 413 Request Entity Too Large\r\n";

pub struct OsListener(pub TcpListener);

impl axum::serve::Listener for OsListener {
    type Io = OsStream;
    type Addr = SocketAddr;

    async fn accept(&mut self) -> (OsStream, SocketAddr) {
        loop {
            match self.0.accept().await {
                Ok((inner, addr)) => {
                    return (
                        OsStream {
                            inner,
                            pending: Vec::new(),
                        },
                        addr,
                    )
                }
                Err(_) => tokio::task::yield_now().await,
            }
        }
    }

    fn local_addr(&self) -> io::Result<SocketAddr> {
        self.0.local_addr()
    }
}

pub struct OsStream {
    inner: TcpStream,
    /// Rewritten bytes already reported as written but not yet sent.
    pending: Vec<u8>,
}

impl OsStream {
    fn drain(&mut self, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        while !self.pending.is_empty() {
            match Pin::new(&mut self.inner).poll_write(cx, &self.pending) {
                Poll::Ready(Ok(0)) => return Poll::Ready(Err(io::ErrorKind::WriteZero.into())),
                Poll::Ready(Ok(n)) => {
                    self.pending.drain(..n);
                }
                Poll::Ready(Err(e)) => return Poll::Ready(Err(e)),
                Poll::Pending => return Poll::Pending,
            }
        }
        Poll::Ready(Ok(()))
    }
}

impl AsyncRead for OsStream {
    fn poll_read(
        self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &mut ReadBuf<'_>,
    ) -> Poll<io::Result<()>> {
        Pin::new(&mut self.get_mut().inner).poll_read(cx, buf)
    }
}

impl AsyncWrite for OsStream {
    fn poll_write(self: Pin<&mut Self>, cx: &mut Context<'_>, buf: &[u8]) -> Poll<io::Result<usize>> {
        let this = self.get_mut();
        match this.drain(cx) {
            Poll::Ready(Ok(())) => {}
            Poll::Ready(Err(e)) => return Poll::Ready(Err(e)),
            Poll::Pending => return Poll::Pending,
        }
        if let Some(pos) = buf.windows(HYPER_413.len()).position(|w| w == HYPER_413) {
            let mut out = Vec::with_capacity(buf.len() + NETTY_413.len());
            out.extend_from_slice(&buf[..pos]);
            out.extend_from_slice(NETTY_413);
            out.extend_from_slice(&buf[pos + HYPER_413.len()..]);
            this.pending = out;
            if let Poll::Ready(Err(e)) = this.drain(cx) {
                return Poll::Ready(Err(e));
            }
            return Poll::Ready(Ok(buf.len()));
        }
        Pin::new(&mut this.inner).poll_write(cx, buf)
    }

    fn poll_flush(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        let this = self.get_mut();
        match this.drain(cx) {
            Poll::Ready(Ok(())) => Pin::new(&mut this.inner).poll_flush(cx),
            other => other,
        }
    }

    fn poll_shutdown(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<io::Result<()>> {
        let this = self.get_mut();
        match this.drain(cx) {
            Poll::Ready(Ok(())) => Pin::new(&mut this.inner).poll_shutdown(cx),
            other => other,
        }
    }
}
