// Tests for RFD1861Demo.GateDecisionClient. Compiled with the .NET 3.5 compiler, run on CLR 2.0.
using System;
using System.IO;
using System.Net;
using System.Text;
using System.Threading;
using RFD1861Demo;

class FakeServer : IDisposable
{
    public delegate void Handler(HttpListenerContext c);
    private readonly HttpListener l = new HttpListener();
    private readonly Thread t;
    public int Hits;
    public string LastBody = "";
    public string LastApiKey = "";
    public int Status = 200;
    public string Reply = "";
    public int DelayMs = 0;

    public FakeServer(int port)
    {
        l.Prefixes.Add("http://127.0.0.1:" + port + "/");
        l.Start();
        t = new Thread(Loop); t.IsBackground = true; t.Start();
    }
    private void Loop()
    {
        while (true)
        {
            HttpListenerContext c;
            try { c = l.GetContext(); } catch (Exception) { return; }
            Interlocked.Increment(ref Hits);
            try
            {
                LastBody = new StreamReader(c.Request.InputStream, Encoding.UTF8).ReadToEnd();
                LastApiKey = c.Request.Headers["X-API-Key"] ?? "";
                if (DelayMs > 0) Thread.Sleep(DelayMs);
                byte[] b = Encoding.UTF8.GetBytes(Reply);
                c.Response.StatusCode = Status;
                c.Response.ContentType = "application/json";
                c.Response.OutputStream.Write(b, 0, b.Length);
                c.Response.Close();
            }
            catch (Exception) { }
        }
    }
    public void Dispose() { try { l.Stop(); l.Close(); } catch (Exception) { } }
}

static class T
{
    static int pass, fail;
    public static void Ok(bool cond, string name)
    {
        if (cond) { pass++; Console.WriteLine("  PASS  " + name); }
        else { fail++; Console.WriteLine("  FAIL  " + name); }
    }
    public static int Summary() { Console.WriteLine(); Console.WriteLine("passed={0} failed={1}", pass, fail); return fail == 0 ? 0 : 1; }
}

class Program
{
    const int MwPort = 3401, KohaPort = 3402, DeadPort = 3499;
    static string MwUrl { get { return "http://127.0.0.1:" + MwPort + "/rfid/check"; } }
    static string KohaBase { get { return "http://127.0.0.1:" + KohaPort; } }

    static string Mw(int d) { return "{\"success\":true,\"code\":\"OK\",\"data\":{\"decision\":" + d + ",\"reason\":\"X\",\"signal\":false},\"meta\":{}}"; }
    static string Kh(string s) { return "{\"status\":\"" + s + "\",\"rfid_tag\":\"T\"}"; }

    static GateSettings Cfg(bool koha, int mwPort, int mwTimeout)
    {
        GateSettings s = new GateSettings();
        s.MiddlewareUrl = "http://127.0.0.1:" + mwPort + "/rfid/check";
        s.MiddlewareTimeoutMs = mwTimeout;
        if (koha) { s.KohaBaseUrl = KohaBase; s.KohaApiKey = "gate-key"; }
        s.KohaTimeoutMs = 800;
        s.BreakerSeconds = 30;
        return s;
    }

    static int Main()
    {
        Console.WriteLine("CLR " + Environment.Version);
        using (FakeServer mw = new FakeServer(MwPort))
        using (FakeServer koha = new FakeServer(KohaPort))
        {
            Console.WriteLine("1. middleware is the normal path");
            mw.Reply = Mw(1);
            GateResult r = new GateDecisionClient(Cfg(true, MwPort, 1000)).Check("SN0038AA");
            T.Ok(r.Decision == 1 && r.Source == GateSource.Middleware, "decision 1 (alarm) from middleware");
            T.Ok(mw.LastBody == "{\"rfid\":\"SN0038AA\"}", "middleware gets {\"rfid\":...} exactly: " + mw.LastBody);
            T.Ok(koha.Hits == 0, "Koha is not touched while the middleware answers");
            mw.Reply = Mw(0);
            r = new GateDecisionClient(Cfg(true, MwPort, 1000)).Check("X");
            T.Ok(r.Decision == 0 && r.Source == GateSource.Middleware, "decision 0 (allow) from middleware");
            mw.Reply = "{\"success\":true,\"data\":{ \"decision\" : 1 }}";
            r = new GateDecisionClient(Cfg(true, MwPort, 1000)).Check("X");
            T.Ok(r.Decision == 1 && r.Source == GateSource.Middleware, "parser tolerates whitespace around the colon");

            Console.WriteLine("2. a bad middleware answer is a FAILURE, never an implicit allow");
            mw.Status = 500; mw.Reply = "boom";
            koha.Reply = Kh("NOT_BORROWED");
            r = new GateDecisionClient(Cfg(true, MwPort, 1000)).Check("T1");
            T.Ok(r.Source == GateSource.KohaDirect && r.Decision == 1, "HTTP 500 from middleware -> Koha fallback (alarm for NOT_BORROWED)");
            mw.Status = 200; mw.Reply = "{\"data\":{}}";
            r = new GateDecisionClient(Cfg(true, MwPort, 1000)).Check("T1");
            T.Ok(r.Source == GateSource.KohaDirect, "200 without a decision -> Koha fallback");
            mw.Reply = Mw(7);
            r = new GateDecisionClient(Cfg(true, MwPort, 1000)).Check("T1");
            T.Ok(r.Source == GateSource.KohaDirect, "decision value other than 0/1 -> Koha fallback");
            mw.Status = 200; mw.Reply = Mw(0);

            Console.WriteLine("3. middleware DOWN (nothing listening): Koha direct, mapped like the legacy gate");
            koha.Reply = Kh("BORROWED");
            r = new GateDecisionClient(Cfg(true, DeadPort, 1000)).Check("SN0038BB");
            T.Ok(r.Decision == 0 && r.Source == GateSource.KohaDirect, "BORROWED -> allow");
            T.Ok(koha.LastBody == "{\"rfid_tag\":\"SN0038BB\"}", "Koha gets {\"rfid_tag\":...}: " + koha.LastBody);
            T.Ok(koha.LastApiKey == "gate-key", "X-API-Key header is sent");
            koha.Reply = Kh("NOT_BORROWED");
            r = new GateDecisionClient(Cfg(true, DeadPort, 1000)).Check("T");
            T.Ok(r.Decision == 1 && r.Source == GateSource.KohaDirect, "NOT_BORROWED -> ALARM");
            koha.Reply = Kh("ITEM_NOT_FOUND");
            r = new GateDecisionClient(Cfg(true, DeadPort, 1000)).Check("T");
            T.Ok(r.Decision == 0 && r.Source == GateSource.KohaDirect, "ITEM_NOT_FOUND -> allow (wallet, clothes)");

            Console.WriteLine("4. both down -> FAIL OPEN, clearly marked");
            koha.Reply = Kh("SOMETHING_ELSE");
            r = new GateDecisionClient(Cfg(true, DeadPort, 1000)).Check("T");
            T.Ok(r.Decision == 0 && r.FailOpen, "unknown Koha status -> fail open");
            koha.Status = 401; koha.Reply = "{\"status\":\"UNAUTHORIZED\"}";
            r = new GateDecisionClient(Cfg(true, DeadPort, 1000)).Check("T");
            T.Ok(r.Decision == 0 && r.FailOpen, "Koha 401 (bad key) -> fail open, not a crash");
            koha.Status = 200;
            r = new GateDecisionClient(Cfg(false, DeadPort, 1000)).Check("T");
            T.Ok(r.Decision == 0 && r.FailOpen, "Koha fallback not configured + middleware down -> fail open");
            GateSettings dead = Cfg(true, DeadPort, 1000); dead.KohaBaseUrl = "http://127.0.0.1:" + DeadPort;
            r = new GateDecisionClient(dead).Check("T");
            T.Ok(r.Decision == 0 && r.FailOpen, "both unreachable -> fail open");

            Console.WriteLine("5. timeouts: a HANGING middleware must not stall the gate");
            mw.DelayMs = 3000; mw.Reply = Mw(1);
            koha.Reply = Kh("BORROWED");
            DateTime t0 = DateTime.Now;
            r = new GateDecisionClient(Cfg(true, MwPort, 300)).Check("T");
            double ms = (DateTime.Now - t0).TotalMilliseconds;
            T.Ok(r.Source == GateSource.KohaDirect && ms < 1500, "hung middleware (300ms timeout) -> Koha in " + (int)ms + " ms");
            mw.DelayMs = 0;
            Thread.Sleep(3500); // let the fake server finish the delayed request before the next section

            Console.WriteLine("6. circuit breaker");
            GateDecisionClient c = new GateDecisionClient(Cfg(true, MwPort, 1000));
            DateTime clock = new DateTime(2026, 11, 11, 10, 0, 0);
            c.Now = delegate { return clock; };
            mw.Status = 500; mw.Reply = "down"; mw.Hits = 0;
            koha.Reply = Kh("BORROWED");
            c.Check("T");
            T.Ok(mw.Hits == 1, "first scan tries the middleware once");
            c.Check("T"); c.Check("T"); c.Check("T");
            T.Ok(mw.Hits == 1, "next scans within 30s skip the middleware (still 1 hit)");
            clock = clock.AddSeconds(31);
            c.Check("T");
            T.Ok(mw.Hits == 2, "after 30s the middleware is tried again (half-open)");
            clock = clock.AddSeconds(5);
            c.Check("T");
            T.Ok(mw.Hits == 2, "still failing -> breaker re-opened, skipped again");
            clock = clock.AddSeconds(31);
            mw.Status = 200; mw.Reply = Mw(1);
            r = c.Check("T");
            T.Ok(r.Source == GateSource.Middleware && r.Decision == 1, "middleware recovered -> used again");
            mw.Hits = 0; c.Check("T");
            T.Ok(mw.Hits == 1, "and it stays in use (breaker closed)");

            Console.WriteLine("6b. Koha has its own breaker: with both sources down, scans must not each wait for timeouts");
            GateSettings both = Cfg(true, DeadPort, 1000); both.BreakerSeconds = 10; both.KohaTimeoutMs = 600;
            GateDecisionClient c2 = new GateDecisionClient(both);
            DateTime clock2 = new DateTime(2026, 11, 11, 11, 0, 0);
            c2.Now = delegate { return clock2; };
            koha.Hits = 0; koha.DelayMs = 2000; koha.Status = 200; koha.Reply = Kh("BORROWED");
            DateTime a0 = DateTime.Now;
            r = c2.Check("T");
            double first = (DateTime.Now - a0).TotalMilliseconds;
            T.Ok(r.FailOpen && first >= 1000 && first < 2500, "first scan pays the two timeouts once (middleware 1000 + Koha 600 = " + (int)first + " ms) then fails open");
            a0 = DateTime.Now;
            r = c2.Check("T"); c2.Check("T"); c2.Check("T");
            double next3 = (DateTime.Now - a0).TotalMilliseconds;
            T.Ok(r.FailOpen && next3 < 100, "next scans fail open immediately (3 scans in " + (int)next3 + " ms), no timeouts");
            Thread.Sleep(2500); // let the fake Koha finish the delayed request
            T.Ok(koha.Hits == 1, "Koha was contacted once, not once per scan (hits=" + koha.Hits + ")");
            koha.DelayMs = 0;
            clock2 = clock2.AddSeconds(11);
            r = c2.Check("T");
            T.Ok(r.Source == GateSource.KohaDirect && r.Decision == 0, "after the breaker time Koha is tried again and used when it answers");
            koha.Hits = 0; c2.Check("T");
            T.Ok(koha.Hits == 1, "and it stays in use");

            Console.WriteLine("7. input safety");
            mw.Status = 200; mw.Reply = Mw(0);
            new GateDecisionClient(Cfg(true, MwPort, 1000)).Check("A\"B\\C");
            T.Ok(mw.LastBody == "{\"rfid\":\"A\\\"B\\\\C\"}", "quotes and backslashes are escaped: " + mw.LastBody);
        }

        Console.WriteLine("8. settings file");
        string f = Path.Combine(Path.GetTempPath(), "gate-test.ini");
        File.WriteAllText(f, "# comment\r\n; another\r\n\r\nMiddlewareUrl = http://x:1/rfid/check\r\nKohaBaseUrl=https://k.example/api/v1/contrib/rfid_gate/\r\nKohaApiKey = secret\r\nBreakerSeconds=abc\r\nKohaTimeoutMs=2500\r\nUnknown=1\r\n");
        GateSettings s = GateSettings.Load(f);
        T.Ok(s.MiddlewareUrl == "http://x:1/rfid/check" && s.KohaApiKey == "secret", "keys are read, spaces around = are fine");
        T.Ok(s.KohaBaseUrl == "https://k.example/api/v1/contrib/rfid_gate", "trailing slash is trimmed");
        T.Ok(s.BreakerSeconds == 10 && s.KohaTimeoutMs == 2500, "bad number keeps the default, good number is applied");
        T.Ok(s.KohaConfigured, "Koha counts as configured when url and key are set");
        T.Ok(!GateSettings.Load(Path.Combine(Path.GetTempPath(), "does-not-exist.ini")).KohaConfigured, "missing file -> defaults, Koha fallback off");
        File.Delete(f);

        Console.WriteLine("9. status file for the watchdog");
        string sf = Path.Combine(Path.GetTempPath(), "gate-status-test.txt");
        if (File.Exists(sf)) File.Delete(sf);
        GateStatusFile st = new GateStatusFile(sf);
        DateTime now = new DateTime(2026, 11, 11, 10, 0, 0);
        st.Now = delegate { return now; };
        st.Report(GateSource.Middleware);
        T.Ok(File.Exists(sf) && File.ReadAllText(sf).Contains("mode=normal"), "normal mode is written once on start");
        DateTime mtime = File.GetLastWriteTime(sf);
        now = now.AddSeconds(5); st.Report(GateSource.Middleware);
        T.Ok(File.GetLastWriteTime(sf) == mtime, "normal mode does not rewrite the file on every scan");
        now = now.AddSeconds(5); st.Report(GateSource.None);
        string txt = File.ReadAllText(sf);
        T.Ok(txt.Contains("mode=fail-open") && txt.Contains("since=2026-11-11 10:00:10"), "fail-open is written immediately with its start time");
        now = now.AddSeconds(5); st.Report(GateSource.None);
        T.Ok(File.ReadAllText(sf).Contains("updated=2026-11-11 10:00:10"), "refresh is rate limited (20s)");
        now = now.AddSeconds(30); st.Report(GateSource.None);
        txt = File.ReadAllText(sf);
        T.Ok(txt.Contains("updated=2026-11-11 10:00:45") && txt.Contains("since=2026-11-11 10:00:10"), "refreshed after 20s, 'since' stays the original start");
        now = now.AddSeconds(2); st.Report(GateSource.KohaDirect);
        T.Ok(File.ReadAllText(sf).Contains("mode=koha-direct"), "mode change to koha-direct is written");
        GateStatusFile bad = new GateStatusFile("Z:\\no\\such\\drive\\x.txt");
        bad.Report(GateSource.None);
        T.Ok(true, "an unwritable status path does not throw");
        File.Delete(sf);

        return T.Summary();
    }
}
