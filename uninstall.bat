@echo off
setlocal EnableDelayedExpansion

set "AGENT_DIR=%~dp0"
set "AGENT_EXE=%AGENT_DIR%rfid-middleware.exe"
set "SERVICE_NAME=RFIDMiddleware"
set "LOG=%AGENT_DIR%uninstall_log.txt"

call :log INFO "===== uninstall.bat dimulai: %date% %time% ====="

:: Require Administrator
net session >nul 2>&1
if %errorlevel% neq 0 (
    call :log ERROR "Harus dijalankan sebagai Administrator."
    pause
    exit /b 1
)

:: Stop service
call :log INFO "Menghentikan service %SERVICE_NAME%..."
net stop "%SERVICE_NAME%" >nul 2>&1
sc stop "%SERVICE_NAME%" >nul 2>&1

:: Uninstall service via binary
if exist "%AGENT_EXE%" (
    "%AGENT_EXE%" uninstall >nul 2>&1
    call :log INFO "rfid-middleware.exe uninstall selesai (errorlevel=%errorlevel%)"
) else (
    call :log WARNING "rfid-middleware.exe tidak ditemukan di %AGENT_DIR%, skip unregister service"
)

:: Tunggu proses benar-benar mati, force kill kalau perlu
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
    call :log WARNING "Menghentikan paksa (kill)..."
    taskkill /F /IM rfid-middleware.exe >nul 2>&1
    goto :afterwait
)
timeout /t 1 /nobreak >nul
goto :waitloop

:afterwait

call :log INFO "Cek status service %SERVICE_NAME% setelah uninstall..."
sc query "%SERVICE_NAME%" >nul 2>&1
if %errorlevel% equ 0 (
    call :log WARNING "Service %SERVICE_NAME% masih terdaftar di SCM."
) else (
    call :log SUCCESS "Service %SERVICE_NAME% sudah tidak terdaftar."
)

echo.
echo Service %SERVICE_NAME% uninstalled.
echo rfid-middleware.exe, config.yaml, edge.db TIDAK dihapus (tetap ada di %AGENT_DIR%).

call :log INFO "===== uninstall.bat selesai: %date% %time% ====="
endlocal
exit /b 0

:log
echo [%date% %time%] [%~1] %~2
echo [%date% %time%] [%~1] %~2 >> "%LOG%"
exit /b
