// Edited by Frendy start
// Gate decision with layered fallback. No WinForms dependency, so it can be tested
// on its own. Written for .NET 2.0 / C# 3 (no HttpClient, no LINQ, no Task).
//
// Order for every scan:
//   1. the local Go middleware (POST <MiddlewareUrl>)           -> Source = Middleware
//   2. Koha RfidGateApi directly, only if configured (POST      -> Source = KohaDirect
//      <KohaBaseUrl>/rfid/check with X-API-Key) when 1 failed
//   3. both failed: FAIL OPEN (Decision = 0, no alarm), because  -> Source = None
//      alarming on every wallet and jacket would make the gate unusable
//
// "Failed" means a transport error, a timeout, a non-200 status or a body that does not
// contain a valid decision. A decision of 0 is NEVER inferred from a failure inside step 1
// or 2, so an unreachable middleware can no longer look like "allow" by accident.
//
// Circuit breaker (one per source): after the middleware fails it is skipped for BreakerSeconds,
// so the following scans go straight to Koha instead of each waiting for the middleware again;
// the same for Koha direct. When both are skipped a scan returns immediately (fail open) instead of
// every person waiting for two timeouts. After the time passes the source is tried once more
// (half-open) and, if it answers, used.

using System;
using System.Collections.Generic;
using System.IO;
using System.Net;
using System.Text;
using System.Text.RegularExpressions;

namespace RFD1861Demo
{
    public enum GateSource
    {
        Middleware,
        KohaDirect,
        None
    }

    public sealed class GateResult
    {
        public int Decision;        // 0 = allow, 1 = alarm
        public GateSource Source;
        public string Detail;       // short human-readable note for the log

        public bool FailOpen
        {
            get { return Source == GateSource.None; }
        }
    }

    public sealed class GateSettings
    {
        public string MiddlewareUrl = "http://127.0.0.1:3000/rfid/check";
        public int MiddlewareTimeoutMs = 1000;
        // Koha fallback is OFF unless both are set. Example base url:
        //   https://catalogue.uiii.ac.id/api/v1/contrib/rfid_gate
        public string KohaBaseUrl = "";
        public string KohaApiKey = "";
        public int KohaTimeoutMs = 1500;
        public int BreakerSeconds = 10;

        public bool KohaConfigured
        {
            get { return KohaBaseUrl.Length > 0 && KohaApiKey.Length > 0; }
        }

        // Reads simple "Key=Value" lines (blank lines and lines starting with # or ; are
        // ignored). A missing file means defaults. Unknown keys and bad numbers are ignored
        // rather than stopping the gate from starting.
        public static GateSettings Load(string path)
        {
            GateSettings s = new GateSettings();
            if (path == null || !File.Exists(path)) return s;

            foreach (string raw in File.ReadAllLines(path))
            {
                string line = raw.Trim();
                if (line.Length == 0 || line[0] == '#' || line[0] == ';') continue;
                int eq = line.IndexOf('=');
                if (eq <= 0) continue;
                string key = line.Substring(0, eq).Trim();
                string val = line.Substring(eq + 1).Trim();

                if (Eq(key, "MiddlewareUrl")) { if (val.Length > 0) s.MiddlewareUrl = val; }
                else if (Eq(key, "KohaBaseUrl")) s.KohaBaseUrl = val.TrimEnd('/');
                else if (Eq(key, "KohaApiKey")) s.KohaApiKey = val;
                else if (Eq(key, "MiddlewareTimeoutMs")) s.MiddlewareTimeoutMs = Num(val, s.MiddlewareTimeoutMs);
                else if (Eq(key, "KohaTimeoutMs")) s.KohaTimeoutMs = Num(val, s.KohaTimeoutMs);
                else if (Eq(key, "BreakerSeconds")) s.BreakerSeconds = Num(val, s.BreakerSeconds);
            }
            return s;
        }

        private static bool Eq(string a, string b)
        {
            return string.Equals(a, b, StringComparison.OrdinalIgnoreCase);
        }

        private static int Num(string v, int fallback)
        {
            int n;
            return (int.TryParse(v, out n) && n > 0) ? n : fallback;
        }
    }

    public delegate DateTime NowFunc();

    public sealed class GateDecisionClient
    {
        private static readonly Regex DecisionRx = new Regex("\"decision\"\\s*:\\s*(-?\\d+)", RegexOptions.Compiled);
        private static readonly Regex StatusRx = new Regex("\"status\"\\s*:\\s*\"([A-Za-z_]+)\"", RegexOptions.Compiled);

        private readonly GateSettings settings;
        private readonly object gate = new object();
        private DateTime middlewareSkipUntil = DateTime.MinValue;
        private DateTime kohaSkipUntil = DateTime.MinValue;

        /// <summary>Optional log sink (the gate app wires this to its Log class). Never throws into the caller.</summary>
        public Action<string> Logger;

        /// <summary>Clock, replaceable in tests.</summary>
        public NowFunc Now = delegate { return DateTime.Now; };

        static GateDecisionClient()
        {
            // .NET 2.0 negotiates at most TLS 1.0 by default, and the Koha/catalogue host only accepts
            // TLS 1.2+ (verified: default => SendFailure, with this flag => HTTP 200). The enum member
            // Tls12 does not exist in .NET 2.0, so the numeric value is used. If this machine's
            // runtime/OS cannot do it, the plain-HTTP middleware path is unaffected.
            try
            {
                ServicePointManager.SecurityProtocol =
                    ServicePointManager.SecurityProtocol | (SecurityProtocolType)3072;
            }
            catch (Exception) { }
            ServicePointManager.Expect100Continue = false;
        }

        public GateDecisionClient(GateSettings settings)
        {
            this.settings = settings ?? new GateSettings();
        }

        public GateSettings Settings
        {
            get { return settings; }
        }

        public GateResult Check(string rfid)
        {
            string note1 = null;

            if (MiddlewareAllowed())
            {
                int decision;
                string err = CallMiddleware(rfid, out decision);
                if (err == null)
                {
                    CloseBreaker();
                    return new GateResult { Decision = decision, Source = GateSource.Middleware, Detail = "middleware" };
                }
                OpenBreaker();
                note1 = "middleware: " + err;
                Log("middleware failed (" + err + "); skipping it for " + settings.BreakerSeconds + "s");
            }
            else
            {
                note1 = "middleware skipped (breaker open)";
            }

            if (settings.KohaConfigured && KohaAllowed())
            {
                int decision;
                string err = CallKoha(rfid, out decision);
                if (err == null)
                {
                    CloseKohaBreaker();
                    return new GateResult { Decision = decision, Source = GateSource.KohaDirect, Detail = note1 + "; koha direct ok" };
                }
                OpenKohaBreaker();
                Log("koha direct failed (" + err + "); skipping it for " + settings.BreakerSeconds + "s");
                note1 = note1 + "; koha: " + err;
            }
            else if (settings.KohaConfigured)
            {
                note1 = note1 + "; koha skipped (breaker open)";
            }
            else
            {
                note1 = note1 + "; koha fallback not configured";
            }

            return new GateResult { Decision = 0, Source = GateSource.None, Detail = note1 };
        }

        // ---- circuit breaker ------------------------------------------------

        private bool MiddlewareAllowed()
        {
            lock (gate) { return Now() >= middlewareSkipUntil; }
        }

        private void OpenBreaker()
        {
            lock (gate) { middlewareSkipUntil = Now().AddSeconds(settings.BreakerSeconds); }
        }

        private void CloseBreaker()
        {
            lock (gate)
            {
                if (middlewareSkipUntil != DateTime.MinValue) Log("middleware is back, using it again");
                middlewareSkipUntil = DateTime.MinValue;
            }
        }

        private bool KohaAllowed()
        {
            lock (gate) { return Now() >= kohaSkipUntil; }
        }

        private void OpenKohaBreaker()
        {
            lock (gate) { kohaSkipUntil = Now().AddSeconds(settings.BreakerSeconds); }
        }

        private void CloseKohaBreaker()
        {
            lock (gate)
            {
                if (kohaSkipUntil != DateTime.MinValue) Log("koha direct is back, using it again");
                kohaSkipUntil = DateTime.MinValue;
            }
        }

        // ---- step 1: middleware --------------------------------------------

        // Returns null on success (decision filled in), otherwise a short error text.
        private string CallMiddleware(string rfid, out int decision)
        {
            decision = 0;
            string body = "{\"rfid\":\"" + JsonEscape(rfid) + "\"}";
            string response;
            string err = Post(settings.MiddlewareUrl, body, null, settings.MiddlewareTimeoutMs, true, out response);
            if (err != null) return err;

            Match m = DecisionRx.Match(response);
            if (!m.Success) return "no decision in response";
            int d = int.Parse(m.Groups[1].Value);
            if (d != 0 && d != 1) return "unexpected decision " + d;
            decision = d;
            return null;
        }

        // ---- step 2: Koha direct -------------------------------------------

        private string CallKoha(string rfid, out int decision)
        {
            decision = 0;
            string body = "{\"rfid_tag\":\"" + JsonEscape(rfid) + "\"}";
            string response;
            string err = Post(settings.KohaBaseUrl + "/rfid/check", body, settings.KohaApiKey,
                settings.KohaTimeoutMs, false, out response);
            if (err != null) return err;

            Match m = StatusRx.Match(response);
            if (!m.Success) return "no status in response";
            switch (m.Groups[1].Value)
            {
                case "BORROWED": decision = 0; return null;       // on loan: allowed to leave
                case "NOT_BORROWED": decision = 1; return null;   // not on loan: alarm
                case "ITEM_NOT_FOUND": decision = 0; return null; // not a library item (wallet, clothes)
                default: return "unknown status " + m.Groups[1].Value;
            }
        }

        // ---- HTTP -----------------------------------------------------------

        // Returns null on HTTP 200 (response filled), otherwise a short error text. Never throws.
        private static string Post(string url, string json, string apiKey, int timeoutMs, bool noProxy, out string response)
        {
            response = null;
            try
            {
                HttpWebRequest req = (HttpWebRequest)WebRequest.Create(url);
                req.Method = "POST";
                req.ContentType = "application/json";
                req.Timeout = timeoutMs;
                req.ReadWriteTimeout = timeoutMs;
                // localhost must not wait on proxy auto-detection (can add seconds in .NET 2.0)
                if (noProxy) req.Proxy = null;
                if (apiKey != null) req.Headers.Add("X-API-Key", apiKey);

                byte[] data = Encoding.UTF8.GetBytes(json);
                req.ContentLength = data.Length;
                using (Stream s = req.GetRequestStream())
                {
                    s.Write(data, 0, data.Length);
                }

                using (HttpWebResponse resp = (HttpWebResponse)req.GetResponse())
                using (StreamReader r = new StreamReader(resp.GetResponseStream(), Encoding.UTF8))
                {
                    if (resp.StatusCode != HttpStatusCode.OK) return "http " + (int)resp.StatusCode;
                    response = r.ReadToEnd();
                    return null;
                }
            }
            catch (WebException ex)
            {
                HttpWebResponse hr = ex.Response as HttpWebResponse;
                if (hr != null) return "http " + (int)hr.StatusCode;
                return ex.Status.ToString();
            }
            catch (Exception ex)
            {
                return ex.GetType().Name;
            }
        }

        private static string JsonEscape(string s)
        {
            if (s == null) return "";
            StringBuilder sb = new StringBuilder(s.Length + 8);
            foreach (char c in s)
            {
                if (c == '\\' || c == '"') { sb.Append('\\').Append(c); }
                else if (c < ' ') { sb.Append(' '); }
                else sb.Append(c);
            }
            return sb.ToString();
        }

        private void Log(string text)
        {
            try
            {
                Action<string> l = Logger;
                if (l != null) l(text);
            }
            catch (Exception) { }
        }
    }

    // Tells the watchdog what the gate is doing, via a small file in a folder both the gate app and
    // the watchdog task can read: %ProgramData%\RFIDGate\gate-status.txt
    //
    //   mode=normal | koha-direct | fail-open
    //   since=<local time the mode started>
    //   updated=<local time of the last scan in that mode>
    //
    // Written on a mode change and, while degraded, at most every RefreshSeconds. watchdog.ps1 raises
    // an alert when mode=fail-open and "updated" is recent, i.e. scans are passing UNCHECKED right now.
    public sealed class GateStatusFile
    {
        private readonly string path;
        private readonly object gate = new object();
        private string mode = "";
        private DateTime since = DateTime.MinValue;
        private DateTime lastWrite = DateTime.MinValue;
        public int RefreshSeconds = 20;
        public NowFunc Now = delegate { return DateTime.Now; };

        public GateStatusFile(string path)
        {
            this.path = path;
        }

        public static string DefaultPath()
        {
            return Path.Combine(
                Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData), "RFIDGate"),
                "gate-status.txt");
        }

        public static string ModeOf(GateSource s)
        {
            switch (s)
            {
                case GateSource.Middleware: return "normal";
                case GateSource.KohaDirect: return "koha-direct";
                default: return "fail-open";
            }
        }

        public void Report(GateSource source)
        {
            string m = ModeOf(source);
            try
            {
                lock (gate)
                {
                    DateTime now = Now();
                    bool changed = m != mode;
                    if (changed) { mode = m; since = now; }
                    bool degraded = m != "normal";
                    if (!changed && !(degraded && (now - lastWrite).TotalSeconds >= RefreshSeconds)) return;

                    string dir = Path.GetDirectoryName(path);
                    if (!Directory.Exists(dir)) Directory.CreateDirectory(dir);
                    File.WriteAllText(path,
                        "mode=" + m + "\r\n" +
                        "since=" + since.ToString("yyyy-MM-dd HH:mm:ss") + "\r\n" +
                        "updated=" + now.ToString("yyyy-MM-dd HH:mm:ss") + "\r\n");
                    lastWrite = now;
                }
            }
            catch (Exception) { /* a status file problem must never affect a gate decision */ }
        }
    }
}
// Edited by Frendy end
