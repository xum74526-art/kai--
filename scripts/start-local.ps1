[CmdletBinding()]
param(
    [string]$DataDir = $env:CANVAS_BACKEND_DATA_DIR,
    [switch]$InitializeData
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "windows-proxy.ps1")
Import-CanvasWindowsProxy

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$backendDir = Join-Path $repoRoot "backend"
$webDir = Join-Path $repoRoot "web"
$agentRuntimeDir = Join-Path $backendDir 'agent-runtime\pi'
$useEmbeddedAgent = [string]::IsNullOrWhiteSpace($env:YINGCE_AGENT_URL)
$localDataDir = Join-Path $repoRoot ".local\project-workbench-debug"
# 工作树升级必须继续使用既有账号库；不能因工作树目录不同而自动创建空库。
if (-not $DataDir) {
    $candidates = @($localDataDir)
    if (Get-Command git -ErrorAction SilentlyContinue) {
        $commonDir = & git -C $repoRoot rev-parse --path-format=absolute --git-common-dir 2>$null
        if ($LASTEXITCODE -eq 0 -and (Split-Path -Leaf $commonDir) -eq '.git') {
            $candidates += Join-Path (Split-Path -Parent $commonDir) '.local\project-workbench-debug'
        }
    }
    $existingDataDirs = @($candidates | Select-Object -Unique | Where-Object { Test-Path -LiteralPath (Join-Path $_ 'open_ai_canvas.db') })
    if ($existingDataDirs.Count -gt 1) {
        throw '检测到多个本地账号数据库，请使用 -DataDir 明确指定既有数据目录。'
    }
    if ($existingDataDirs.Count -eq 1) {
        $DataDir = $existingDataDirs[0]
    } elseif ($InitializeData) {
        $DataDir = $localDataDir
    } else {
        throw '未找到既有账号数据库。请使用 -DataDir 指向 .local/project-workbench-debug；首次使用才传 -InitializeData。'
    }
}
if (-not [IO.Path]::IsPathRooted($DataDir)) {
    $DataDir = Join-Path $repoRoot $DataDir
}
$DataDir = [IO.Path]::GetFullPath($DataDir)
$hasDatabase = Test-Path -LiteralPath (Join-Path $DataDir 'open_ai_canvas.db')
if (-not $hasDatabase -and -not $InitializeData) {
    throw '指定目录没有账号数据库，已停止以避免误建空库。首次使用请传 -InitializeData。'
}
if ($hasDatabase -and -not (Test-Path -LiteralPath (Join-Path $DataDir '.settings-key'))) {
    throw '既有数据库缺少 .settings-key，请先找回原加密密钥，不要重新生成。'
}
$autoMigrate = $env:CANVAS_AUTO_MIGRATE
if (-not $autoMigrate) {
    # 旧库应先备份并用 migrate-schema up/verify 升级，再启动新版服务。
    $autoMigrate = if ($hasDatabase) { 'false' } else { 'true' }
}
$goBuildCache = Join-Path $repoRoot ".local\cache\go-build"
$goModuleCache = Join-Path $repoRoot ".local\cache\go-mod"
$logDir = Join-Path $repoRoot ".local\cache\logs"

$requiredCommands = @('go', 'bun')
if ($useEmbeddedAgent) {
    $requiredCommands += @('node', 'npm')
}
foreach ($commandName in $requiredCommands) {
    if (-not (Get-Command $commandName -ErrorAction SilentlyContinue)) {
        throw "未找到 $commandName，请先安装项目要求的运行时。"
    }
}

if ($useEmbeddedAgent) {
    $nodeVersion = (& node --version).Trim()
    if ($LASTEXITCODE -ne 0 -or $nodeVersion -notmatch '^v?(\d+\.\d+\.\d+)$' -or [version]$Matches[1] -lt [version]'22.19.0') {
        throw '内嵌 Agent 需要 Node.js 22.19.0 或更高稳定版本，请先更新 Node.js。'
    }
    foreach ($runtimeFile in @('agent-runtime.mjs', 'package.json', 'package-lock.json')) {
        if (-not (Test-Path -LiteralPath (Join-Path $agentRuntimeDir $runtimeFile) -PathType Leaf)) {
            throw "Agent runtime 缺少 $runtimeFile，请先完整同步 backend/agent-runtime/pi。"
        }
    }
}

function Test-ListeningPort([int]$Port) {
    try {
        return $null -ne (Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction Stop | Select-Object -First 1)
    } catch {
        return $false
    }
}

# npm ci 会替换依赖目录；必须先确认没有现有服务使用它。
foreach ($port in @(3000, 8080)) {
    if (Test-ListeningPort $port) {
        throw "端口 $port 已被占用，请先关闭占用进程后重试。"
    }
}

foreach ($directory in @($DataDir, $goBuildCache, $goModuleCache, $logDir)) {
    New-Item -ItemType Directory -Force -Path $directory | Out-Null
}

Write-Host "正在按锁文件同步前端依赖..." -ForegroundColor Yellow
Push-Location $webDir
try {
    # Vite 存在不代表新版本依赖齐全；已同步时 Bun 不会重复安装。
    & bun install --frozen-lockfile
    if ($LASTEXITCODE -ne 0) {
        throw "bun install 失败，无法启动前端。"
    }
} finally {
    Pop-Location
}

if ($useEmbeddedAgent) {
    Write-Host '正在按锁文件同步内嵌 Agent 依赖...' -ForegroundColor Yellow
    Push-Location $agentRuntimeDir
    try {
        & npm ci --omit=dev
        if ($LASTEXITCODE -ne 0) {
            throw 'npm ci 失败，无法启动内嵌 Agent。'
        }
    } finally {
        Pop-Location
    }
}

$powerShellPath = (Get-Command pwsh -ErrorAction SilentlyContinue).Source
if (-not $powerShellPath) {
    $powerShellPath = (Get-Command powershell.exe -ErrorAction SilentlyContinue).Source
}
if (-not $powerShellPath) {
    throw "未找到 PowerShell，无法启动前后端后台进程。"
}

function ConvertTo-PowerShellLiteral([string]$Value) {
    return "'" + $Value.Replace("'", "''") + "'"
}

$backendDirLiteral = ConvertTo-PowerShellLiteral $backendDir
$webDirLiteral = ConvertTo-PowerShellLiteral $webDir
$dataDirLiteral = ConvertTo-PowerShellLiteral $DataDir
$agentRuntimeCommand = if ($useEmbeddedAgent) {
    "`$env:CANVAS_PI_RUNTIME_DIR = $(ConvertTo-PowerShellLiteral $agentRuntimeDir)"
} else {
    ''
}

$backendCommand = @"
`$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $backendDirLiteral
`$env:CANVAS_BACKEND_ADDR = '127.0.0.1:8080'
`$env:CANVAS_BACKEND_DATA_DIR = $dataDirLiteral
`$env:CANVAS_AUTO_MIGRATE = $(ConvertTo-PowerShellLiteral $autoMigrate)
`$env:GOCACHE = $(ConvertTo-PowerShellLiteral $goBuildCache)
`$env:GOMODCACHE = $(ConvertTo-PowerShellLiteral $goModuleCache)
$agentRuntimeCommand
Write-Host 'KAI影后端：http://127.0.0.1:8080' -ForegroundColor Cyan
go run ./cmd/server
"@

$webCommand = @"
`$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $webDirLiteral
`$env:VITE_API_PROXY_TARGET = 'http://127.0.0.1:8080'
Write-Host 'KAI影前端：http://localhost:3000' -ForegroundColor Cyan
bun run dev
"@

$launchId = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
$backendLog = Join-Path $logDir "backend-$launchId.log"
$backendErrorLog = Join-Path $logDir "backend-$launchId.error.log"
$webLog = Join-Path $logDir "web-$launchId.log"
$webErrorLog = Join-Path $logDir "web-$launchId.error.log"
$backendProcess = Start-Process -FilePath $powerShellPath -WindowStyle Hidden -WorkingDirectory $backendDir -RedirectStandardOutput $backendLog -RedirectStandardError $backendErrorLog -PassThru -ArgumentList @("-NoLogo", "-NoProfile", "-Command", $backendCommand)
$webProcess = Start-Process -FilePath $powerShellPath -WindowStyle Hidden -WorkingDirectory $webDir -RedirectStandardOutput $webLog -RedirectStandardError $webErrorLog -PassThru -ArgumentList @("-NoLogo", "-NoProfile", "-Command", $webCommand)

Write-Host "已提交前后端后台启动请求；请确认后端日志与健康接口。" -ForegroundColor Green
Write-Host "后端 PID: $($backendProcess.Id)；前端 PID: $($webProcess.Id)"
Write-Host "后端日志: $backendLog；错误日志: $backendErrorLog"
Write-Host "前端日志: $webLog；错误日志: $webErrorLog"
Write-Host "访问 http://localhost:3000；关闭当前终端不会主动停止这些服务。"
