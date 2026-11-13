// Package serverinfo reports where the middleware is listening: a console
// banner for interactive runs and a plain-text file an admin can open and copy
// URLs from when it runs as a Windows service.
package serverinfo

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileName is written next to config.yaml and edge.db.
const FileName = "server-info.txt"

type Info struct {
	Addr    string // bind address as configured, e.g. "127.0.0.1:3000"
	Config  string // directory holding config.yaml / edge.db
	LogFile string
	Running bool
}

// hostPort splits Addr and picks a host an admin can actually open in a browser.
// A wildcard bind (0.0.0.0, ::, empty) is reachable via localhost.
func (i Info) hostPort() (host, port string, local bool) {
	host, port, err := net.SplitHostPort(i.Addr)
	if err != nil {
		return i.Addr, "", true
	}
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return "localhost", port, false
	case "127.0.0.1", "::1", "localhost":
		return host, port, true
	}
	return host, port, false
}

func (i Info) baseURL() string {
	host, port, _ := i.hostPort()
	if strings.Contains(host, ":") { // bare IPv6 literal
		host = "[" + host + "]"
	}
	return "http://" + host + ":" + port
}

// DashboardURL is the page operators open in a browser.
func (i Info) DashboardURL() string { return i.baseURL() + "/" }

// Text renders the info as plain lines, shared by the banner and the file.
func (i Info) Text() string {
	base := i.baseURL()
	_, port, local := i.hostPort()

	state := "RUNNING"
	if !i.Running {
		state = "STOPPED"
	}

	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }

	line("RFID Middleware - server info")
	line("Updated   : %s", time.Now().Format("2006-01-02 15:04:05"))
	line("Status    : %s (pid %d)", state, os.Getpid())
	line("")
	line("Port      : %s", port)
	line("Bind      : %s", i.Addr)
	line("Dashboard : %s/", base)
	line("Health    : %s/health", base)
	line("Gate API  : POST %s/rfid/check", base)
	line("Live logs : %s/rfid/logs/stream", base)
	line("")
	line("Library   : koha")
	line("Config    : %s", filepath.Join(i.Config, "config.yaml"))
	line("Log file  : %s", i.LogFile)
	if local {
		line("")
		line("NOTE: bound to localhost, so only this PC can open the dashboard.")
		line("      To allow other PCs, set server.address to 0.0.0.0 in config.yaml and restart.")
	}
	return b.String()
}

// PrintBanner writes a boxed, easy-to-spot summary to w (the console).
func PrintBanner(w io.Writer, i Info) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, strings.Repeat("=", 64))
	fmt.Fprint(w, i.Text())
	fmt.Fprintln(w, strings.Repeat("=", 64))
	fmt.Fprintln(w)
}

// WriteFile (over)writes dir/server-info.txt. Failure is non-fatal for the
// caller; the info is a convenience, not part of serving traffic.
func WriteFile(dir string, i Info) error {
	// CRLF so it opens cleanly in Notepad.
	text := strings.ReplaceAll(i.Text(), "\n", "\r\n")
	return os.WriteFile(filepath.Join(dir, FileName), []byte(text), 0o644)
}
