#requires -Version 7.3
[CmdletBinding()]
param([Parameter(Mandatory)][string]$ToolsFile)
$ErrorActionPreference='Stop'
if(-not $IsWindows){throw 'AEGIS_LOCAL_WINDOWS_REQUIRED: Use PowerShell 7 on Windows.'}
try{$launchConfig=Get-Content -LiteralPath $ToolsFile -Raw | ConvertFrom-Json -AsHashtable}
catch{throw 'AEGIS_LOCAL_CONFIG_INVALID: Select the documented external tools JSON. Its contents are never echoed.'}
$launchKeys=@('nodeExe','pnpmCjs','goRoot','goArchive','moduleCacheSeed','runtimeRoot')
if($launchConfig.Count -ne $launchKeys.Count -or @($launchConfig.Keys | Where-Object {$_ -notin $launchKeys}).Count){
    throw 'AEGIS_LOCAL_CONFIG_INVALID: Tools JSON must contain only the six documented tool/runtime paths.'
}
if(-not $launchConfig.nodeExe -or -not [IO.Path]::IsPathFullyQualified($launchConfig.nodeExe) -or -not [IO.File]::Exists($launchConfig.nodeExe)){
    throw 'AEGIS_LOCAL_NODE_MISSING: Set nodeExe to an existing absolute node.exe path in the external tools JSON.'
}
if([IO.Path]::GetFileName($launchConfig.nodeExe) -ine 'node.exe'){throw 'AEGIS_LOCAL_NODE_MISSING: nodeExe must select node.exe.'}
# The child waits for bootstrap on stdin. It cannot spawn tools/services before Job assignment.
if(-not ('AegisLocal.WindowsJob' -as [type])){
    Add-Type -Path (Join-Path $PSScriptRoot 'launcher/windows-job.cs')
}
$launchBootstrap=@{vueRoot=$PSScriptRoot;tools=$launchConfig} | ConvertTo-Json -Depth 4 -Compress
$launchCode=[AegisLocal.WindowsJob]::Run($launchConfig.nodeExe,(Join-Path $PSScriptRoot 'launcher/workspace.mjs'),$launchBootstrap)
$global:LASTEXITCODE=$launchCode
if($launchCode -ne 0){throw "AEGIS_LOCAL_FAILED: This session stopped with code $launchCode. See the specific message above."}
