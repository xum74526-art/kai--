# KAI影本地启动兼容入口。账号库选择、端口检查及后台进程统一由 start-local.ps1 管理。
[CmdletBinding()]
param(
    [string]$DataDir = $env:CANVAS_BACKEND_DATA_DIR,
    [switch]$InitializeData
)

$ErrorActionPreference = "Stop"

# 保留旧快捷方式的重复启动行为，不重启或关闭已运行的服务。
$listeningPorts = @(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty LocalPort)
if ($listeningPorts -contains 3000 -and $listeningPorts -contains 8080) {
    Write-Host '本地端口 3000 / 8080 已在使用，未重复启动。访问 http://localhost:3000' -ForegroundColor Green
    return
}

# 不覆盖公网地址，也不创建、重配或自动启动公网隧道。
& (Join-Path $PSScriptRoot 'start-local.ps1') @PSBoundParameters
