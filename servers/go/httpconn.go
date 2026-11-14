package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// Go's HTTP server rejects a request line whose path holds an invalid
// percent escape ("%", "%zz") with a bare 400 before any handler runs.
// OpenSearch instead answers 405 listing the methods of the matching
// route. escListener rewrites such paths on the wire (each stray '%'
// becomes "%25") and flags the request with badEscapeHeader so ServeHTTP
// can answer like OpenSearch.

const badEscapeHeader = "X-Slopensearch-Bad-Escape"

type escListener struct{ net.Listener }

func (l escListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &escConn{Conn: c, in: bufio.NewReaderSize(c, 64<<10)}, nil
}

type escConn struct {
	net.Conn
	in          *bufio.Reader
	out         []byte // rewritten head bytes not yet handed out
	bodyLeft    int64  // body bytes of the current request still to pass
	passthrough bool   // framing unknown (chunked): stop rewriting

	// head being read (kept across reads that fail, e.g. on deadlines)
	line    []byte
	head    bytes.Buffer
	started bool // request line seen
	length  int64
}

func (c *escConn) Read(p []byte) (int, error) {
	if len(c.out) > 0 {
		n := copy(p, c.out)
		c.out = c.out[n:]
		return n, nil
	}
	if c.passthrough {
		return c.in.Read(p)
	}
	if c.bodyLeft > 0 {
		if int64(len(p)) > c.bodyLeft {
			p = p[:c.bodyLeft]
		}
		n, err := c.in.Read(p)
		c.bodyLeft -= int64(n)
		return n, err
	}
	if err := c.readHead(); err != nil {
		if c.passthrough || len(c.out) > 0 {
			n := copy(p, c.out)
			c.out = c.out[n:]
			return n, nil
		}
		return 0, err
	}
	n := copy(p, c.out)
	c.out = c.out[n:]
	return n, nil
}

// readHead reads one request head (request line and headers) and queues
// it, rewritten when needed.
func (c *escConn) readHead() error {
	for {
		chunk, err := c.in.ReadSlice('\n')
		c.line = append(c.line, chunk...)
		if err == bufio.ErrBufferFull {
			if len(c.line) > 1<<20 {
				// absurd line: hand everything through untouched
				c.head.Write(c.line)
				c.out = append([]byte(nil), c.head.Bytes()...)
				c.passthrough = true
				return err
			}
			continue
		}
		if err != nil {
			if len(c.line) > 0 && errors.Is(err, io.EOF) {
				// incomplete head at EOF: pass it on for the server to judge
				c.head.Write(c.line)
				c.line = nil
				c.out = append([]byte(nil), c.head.Bytes()...)
				c.head.Reset()
				c.passthrough = true
			}
			return err
		}
		line := c.line
		c.line = nil
		trimmed := bytes.TrimRight(line, "\r\n")
		if !c.started {
			if len(trimmed) == 0 {
				c.head.Write(line) // stray empty line between requests
				continue
			}
			c.started = true
			if fixed, ok := fixRequestLine(string(trimmed)); ok {
				c.head.WriteString(fixed + "\r\n")
				c.head.WriteString(badEscapeHeader + ": 1\r\n")
			} else {
				c.head.Write(line)
			}
			continue
		}
		c.head.Write(line)
		if len(trimmed) == 0 {
			break
		}
		if i := bytes.IndexByte(trimmed, ':'); i > 0 {
			name := strings.ToLower(strings.TrimSpace(string(trimmed[:i])))
			val := strings.TrimSpace(string(trimmed[i+1:]))
			switch name {
			case "content-length":
				if n, err := strconv.ParseInt(val, 10, 64); err == nil && n > 0 {
					c.length = n
				}
			case "transfer-encoding":
				c.passthrough = true
			}
		}
	}
	c.bodyLeft = c.length
	c.out = append([]byte(nil), c.head.Bytes()...)
	c.head.Reset()
	c.started = false
	c.length = 0
	return nil
}

// fixRequestLine escapes stray '%' signs in the path of a request line.
func fixRequestLine(line string) (string, bool) {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 {
		return "", false
	}
	target := parts[1]
	path, query := target, ""
	if i := strings.IndexByte(target, '?'); i >= 0 {
		path, query = target[:i], target[i:]
	}
	var b strings.Builder
	bad := false
	for i := 0; i < len(path); i++ {
		if path[i] == '%' && (i+2 >= len(path) || !isHex(path[i+1]) || !isHex(path[i+2])) {
			b.WriteString("%25")
			bad = true
			continue
		}
		b.WriteByte(path[i])
	}
	if !bad {
		return "", false
	}
	return parts[0] + " " + b.String() + query + " " + parts[2], true
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// writeBadEscape answers a request whose path held an invalid percent
// escape: 405 with the methods the matching route accepts, found by
// routing the request with a method no handler accepts.
func (s *Server) writeBadEscape(w http.ResponseWriter, r *http.Request) {
	probe := r.Clone(r.Context())
	probe.Method = "SLOPPROBE"
	probe.Body = http.NoBody
	probe.ContentLength = 0
	pw := &captureWriter{h: http.Header{}}
	s.route(pw, probe)
	allowed := map[string]bool{}
	for _, m := range strings.Split(pw.h.Get("Allow"), ",") {
		if m = strings.TrimSpace(m); m != "" {
			allowed[m] = true
		}
	}
	var list []string
	for _, m := range []string{"HEAD", "PUT", "GET", "POST", "DELETE", "OPTIONS", "PATCH"} {
		if allowed[m] {
			list = append(list, m)
		}
	}
	w.Header().Set("Allow", strings.Join(list, ","))
	writeJSON(w, 405, orderedJSON{{"error", "Unexpected HTTP method, allowed: [" + strings.Join(list, ", ") + "]"}, {"status", 405}})
}
