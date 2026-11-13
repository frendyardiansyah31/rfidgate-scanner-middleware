# Gate app (C#) changes for Fase 5

The gate app lives outside this repo (`C:\RFID_Gate\rfid_31072026_RFIDGateDecompile-v1`, not under git).
This folder keeps the new/changed pieces here so they can be reviewed and tracked.

| File | What |
|---|---|
| `GateDecisionClient.cs` | New class file for `RFD1861Demo/`. Layered decision (middleware, Koha direct, fail-open), circuit breakers, `gate.ini` loader, `gate-status.txt` writer. No WinForms dependency. .NET 2.0 / C# 3 compatible. |
| `Form1.cs.diff` | The only change to `Form1.cs`: `checklabel` now calls `GateDecisionClient`; `CallGoMiddleware` and `ParseDecisionFromJson` are removed. |
| `gate.ini.example` | Copy to `gate.ini` next to `RFD1861Demo.exe` and edit. |
| `GateTests.cs` | 42 tests with fake HTTP servers. |

Original sources were backed up to `C:\frendy-project\backups\rfid-gate-csharp\2026-11-11\`.

## Run the tests (CLR 2.0, same runtime as the gate)

```powershell
& "C:\Windows\Microsoft.NET\Framework\v3.5\csc.exe" -nologo -target:exe -out:gatetests.exe GateTests.cs GateDecisionClient.cs
.\gatetests.exe        # prints PASS/FAIL per case and "passed=N failed=0"
```

## Build the gate app

Add `GateDecisionClient.cs` to the `RFD1861Demo` project (the SDK-style `.csproj` includes it automatically
when it sits in the project folder) and build the way you normally do (Visual Studio: the form resources need
the .NET Framework MSBuild; `dotnet build` stops at ResGen). Compile-only verification was done with the
`.resx` files excluded: the patched project builds with the same 7 warnings as the original.
