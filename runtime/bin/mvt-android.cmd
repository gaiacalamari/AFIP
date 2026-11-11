@echo off
rem Comando di AFIP Portable: avvia mvt-android con il Python portatile, ovunque sia installato.
setlocal
set "PYTHONDONTWRITEBYTECODE=1"
rem 1) percorso passato da AFIP Portable  2) runtime\python  3) tools\python\<versione>\python
set "PY=%AFIP_PYTHON%"
if not exist "%PY%" set "PY=%~dp0..\python\python.exe"
if not exist "%PY%" for /f "delims=" %%D in ('dir /b /ad /on "%~dp0..\..\tools\python" 2^>nul') do if exist "%~dp0..\..\tools\python\%%D\python\python.exe" set "PY=%~dp0..\..\tools\python\%%D\python\python.exe"
if not exist "%PY%" (
  echo Python portatile non trovato: installarlo da AFIP Portable, menu - Aggiornamenti software.
  exit /b 9009
)
"%PY%" -c "import sys; sys.argv[0]='mvt-android'; from mvt.android import main; sys.exit(main())" %*
