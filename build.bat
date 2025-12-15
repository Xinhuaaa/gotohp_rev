@echo off
setlocal

echo [1/3] Attempting to generate bindings...
where wails3 >nul 2>nul
if %errorlevel% equ 0 (
    wails3 generate bindings
    if %errorlevel% neq 0 (
        echo Warning: Binding generation failed. Continuing...
    ) else (
        echo Bindings generated.
    )
) else (
    echo "wails3" not found in PATH. Skipping binding generation.
)

echo [2/3] Building Frontend...
cd frontend
if not exist node_modules (
    echo Installing dependencies...
    call npm install
)
echo Compiling frontend assets...
call npm run build
if %errorlevel% neq 0 (
    echo Frontend build failed!
    exit /b %errorlevel%
)
cd ..

echo [3/3] Building Backend...
go build -o gotohp.exe
if %errorlevel% neq 0 (
    echo Backend build failed!
    exit /b %errorlevel%
)

echo Build SUCCESS! Output: gotohp.exe
endlocal
pause
