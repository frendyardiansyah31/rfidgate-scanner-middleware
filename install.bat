@echo off
setlocal EnableDelayedExpansion

set "AGENT_DIR=%~dp0"
set "AGENT_EXE=%AGENT_DIR%rfid-middleware.exe"
set "SERVICE_NAME=RFIDMiddleware"
set "LOG=%AGENT_DIR%install_log.txt"

call :log INFO "===== install.bat dimulai: %date% %time% ====="

:: Require Administrator
net session >nul 2>&1
if %errorlevel% neq 0 (
    call :log ERROR "Harus dijalankan sebagai Administrator."
    pause
    exit /b 1
)

if not exist "%AGENT_EXE%" (
    call :log ERROR "rfid-middleware.exe tidak ditemukan di %AGENT_DIR%"
    pause
    exit /b 1
)

:: Stop service dan pastikan proses benar-benar mati sebelum lanjut
call :log INFO "Mencoba menghentikan service %SERVICE_NAME%..."
net stop "%SERVICE_NAME%" >nul 2>&1
call :log INFO "net stop %SERVICE_NAME% selesai (errorlevel=%errorlevel%)"

call :log INFO "Menunggu proses rfid-middleware.exe berhenti..."
set "MAXTRY=15"
set "TRY=0"

:waitloop
tasklist /FI "IMAGENAME eq rfid-middleware.exe" 2>NUL | find /I "rfid-middleware.exe" >NUL
if errorlevel 1 (
    call :log SUCCESS "rfid-middleware.exe sudah tidak berjalan."
    goto :afterwait
)

set /a TRY+=1
call :log INFO "rfid-middleware.exe masih berjalan, percobaan ke-!TRY!/%MAXTRY%..."
if !TRY! GEQ %MAXTRY% (
    call :log WARNING "rfid-middleware.exe masih berjalan setelah %MAXTRY% percobaan. Menghentikan paksa (kill)..."
    taskkill /F /IM rfid-middleware.exe >nul 2>&1
    call :log INFO "taskkill /F /IM rfid-middleware.exe selesai (errorlevel=%errorlevel%)"

    set "KILLTRY=0"
    :killwait
    tasklist /FI "IMAGENAME eq rfid-middleware.exe" 2>NUL | find /I "rfid-middleware.exe" >NUL
    if errorlevel 1 (
        call :log SUCCESS "Proses rfid-middleware.exe berhasil dihentikan paksa."
        goto :afterwait
    )
    set /a KILLTRY+=1
    call :log INFO "Masih ada setelah taskkill, cek ulang ke-!KILLTRY!/10..."
    if !KILLTRY! GEQ 10 (
        call :log ERROR "rfid-middleware.exe tetap berjalan meski sudah di-taskkill. Membatalkan instalasi."
        pause
        exit /b 1
    )
    timeout /t 1 /nobreak >nul
    goto :killwait
)

timeout /t 1 /nobreak >nul
goto :waitloop

:afterwait

:: Hapus service lama jika ada (registrasi ulang bersih)
"%AGENT_EXE%" uninstall >nul 2>&1

:: Install sebagai Windows Service (auto-start)
call :log INFO "Menjalankan rfid-middleware.exe install..."
"%AGENT_EXE%" install
if %errorlevel% neq 0 (
    call :log ERROR "Gagal install Windows Service."
    pause
    exit /b 1
)
call :log SUCCESS "Service %SERVICE_NAME% berhasil diregister"

:: Auto-start saat boot dan pulih otomatis bila proses mati:
::   kegagalan ke-1 dan ke-2: restart setelah 5 detik; ke-3 dan seterusnya: setelah 30 detik
::   (jeda lebih panjang supaya middleware yang tidak bisa start tidak di-restart terus-menerus).
::   Penghitung kegagalan di-reset setelah 24 jam tanpa kegagalan.
::   failureflag 1: pulihkan juga bila proses keluar dengan kode error, bukan hanya saat crash.
sc config "%SERVICE_NAME%" start= auto >nul
call :log INFO "sc config start= auto (errorlevel=%errorlevel%)"
sc failure "%SERVICE_NAME%" reset= 86400 actions= restart/5000/restart/5000/restart/30000 >nul
if %errorlevel% neq 0 (
    call :log WARNING "Gagal mengatur pemulihan otomatis (sc failure). Atur manual lewat services.msc, tab Recovery."
) else (
    call :log SUCCESS "Pemulihan otomatis diatur: restart 5s, 5s, lalu 30s"
)
sc failureflag "%SERVICE_NAME%" 1 >nul
call :log INFO "sc failureflag 1 (errorlevel=%errorlevel%)"

:: Start service
call :log INFO "Menjalankan net start %SERVICE_NAME%..."
net start "%SERVICE_NAME%"
if %errorlevel% neq 0 (
    call :log WARNING "Service terinstall tapi gagal start sekarang. Akan start otomatis saat boot."
) else (
    call :log SUCCESS "Service %SERVICE_NAME% berhasil di-start"
)

call :log INFO "Cek status service %SERVICE_NAME%..."
sc query "%SERVICE_NAME%" | find "RUNNING" >nul
if errorlevel 1 (
    call :log WARNING "Service %SERVICE_NAME% tidak terdeteksi RUNNING."
) else (
    call :log SUCCESS "Service %SERVICE_NAME% RUNNING"
)

echo.
echo Agent installed as Windows Service.
echo Directory : %AGENT_DIR%
echo Service   : %SERVICE_NAME% (auto-start)

call :log INFO "===== install.bat selesai: %date% %time% ====="
endlocal
exit /b 0

:log
echo [%date% %time%] [%~1] %~2
echo [%date% %time%] [%~1] %~2 >> "%LOG%"
exit /b
