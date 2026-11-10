$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$projectRoot = Split-Path $PSScriptRoot
$cli = Join-Path $projectRoot '.local\shipcheck.exe'
$testRoot = Join-Path $projectRoot ('.local\extended-' + [Guid]::NewGuid().ToString('N'))
$resultsRoot = Join-Path $testRoot 'results'
New-Item -ItemType Directory -Force -Path $testRoot,$resultsRoot | Out-Null

function Write-JSON([string]$Path, $Value) {
    $Value | ConvertTo-Json -Depth 30 | Set-Content -LiteralPath $Path -Encoding utf8
}

function Invoke-CLI([string[]]$Arguments, [int]$Expected) {
    $lines = @(& $cli @Arguments)
    $code = $LASTEXITCODE
    if ($code -ne $Expected) { throw "命令 $($Arguments -join ' ') 返回 $code，预期 $Expected。$($lines -join [Environment]::NewLine)" }
    $lines
}

function Invoke-Recorded([string[]]$Arguments, [int]$Expected) {
    $before = @(Get-ChildItem -LiteralPath $resultsRoot -Directory | Select-Object -ExpandProperty FullName)
    $lines = @(Invoke-CLI $Arguments $Expected)
    $created = @(Get-ChildItem -LiteralPath $resultsRoot -Directory | Where-Object { $_.FullName -notin $before })
    if ($created.Count -ne 1) { throw '每次操作应生成一份独立记录' }
    $directory = $created[0].FullName
    foreach ($name in 'order.json','verification.json','receipt.json','report.html') {
        if (!(Test-Path -LiteralPath (Join-Path $directory $name) -PathType Leaf)) { throw "缺少记录 $name" }
    }
    [pscustomobject]@{
        Directory = $directory
        Receipt = Get-Content -Raw -LiteralPath (Join-Path $directory 'receipt.json') | ConvertFrom-Json
        Verification = Get-Content -Raw -LiteralPath (Join-Path $directory 'verification.json') | ConvertFrom-Json
        Lines = $lines
    }
}

$staging = Join-Path $projectRoot 'examples\staging'
$ticket = Get-Content -Raw -LiteralPath (Join-Path $projectRoot 'examples\order.json') | ConvertFrom-Json -AsHashtable
$ticket.order_id = 'EXTENDED-ACCEPTANCE'
$ticket.files = @($ticket.required_files | ForEach-Object {
    $sourcePath = Join-Path $staging $_
    @{ path = $_; sha256 = (Get-FileHash -LiteralPath $sourcePath -Algorithm SHA256).Hash.ToLowerInvariant(); size = (Get-Item -LiteralPath $sourcePath).Length; allow_empty = $false }
})
$ticket.allow_extra_files = $false
$ticket.allowed_directories = @('app','dependencies','migrations')
$ticket.forbidden_patterns = @('*.bak','app/*.bak')
$orderPath = Join-Path $testRoot 'order.json'
Write-JSON $orderPath $ticket
$null = Invoke-CLI @('order','validate','--order',$orderPath) 0

$complete = Invoke-Recorded @('run','--order',$orderPath,'--staging',$staging,'--output','upgrade.zip','--results',$resultsRoot,'--','tar.exe','-a','-cf','upgrade.zip','release.json','app','dependencies','migrations') 0
$zip = $complete.Verification.zip_path
if ((Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash.ToLowerInvariant() -cne $complete.Receipt.zip_sha256) { throw '报告应绑定验收 ZIP 的 SHA-256' }
$verified = Invoke-Recorded @('verify','--order',$orderPath,'--zip',$zip,'--results',$resultsRoot) 0
if ($verified.Verification.status -cne 'passed') { throw '独立验收应通过' }

$wrong = Get-Content -Raw -LiteralPath $orderPath | ConvertFrom-Json -AsHashtable
$wrong.files[0].sha256 = '0' * 64
$wrongPath = Join-Path $testRoot 'wrong-hash.json'
Write-JSON $wrongPath $wrong
$rejected = Invoke-Recorded @('verify','--order',$wrongPath,'--zip',$zip,'--results',$resultsRoot) 1
if ($rejected.Verification.issues.Count -lt 1) { throw '错误摘要必须说明拒收原因' }

$failed = Invoke-Recorded @('run','--order',$orderPath,'--staging',$staging,'--output','upgrade.zip','--results',$resultsRoot,'--','tar.exe','-a','-cf','upgrade.zip','release.json','app','dependencies') 1
$beforeHashes = @('order.json','execution.json','verification.json','input\migrations\1.6_to_1.7.sql') | ForEach-Object {
    Get-FileHash -LiteralPath (Join-Path $failed.Directory $_) -Algorithm SHA256
}
$corrected = Invoke-Recorded @('revise','--evidence',$failed.Directory,'--results',$resultsRoot,'--','tar.exe','-a','-cf','upgrade.zip','release.json','app','dependencies','migrations') 0
if ($corrected.Receipt.source_directory -cne $failed.Directory) { throw '修正记录必须关联原失败记录' }
foreach ($original in $beforeHashes) {
    if ((Get-FileHash -LiteralPath $original.Path -Algorithm SHA256).Hash -cne $original.Hash) { throw "原记录内容发生变化 $($original.Path)" }
}
if ($corrected.Receipt.comparison.resolved.Count -lt 1) { throw '修正报告应列出已解决的问题' }
$replayed = Invoke-Recorded @('replay','--evidence',$failed.Directory,'--results',$resultsRoot) 1
if ($replayed.Verification.issues.path -notcontains 'migrations/1.6_to_1.7.sql') { throw '原命令复跑应报告缺少迁移脚本' }

$templatePath = Join-Path $testRoot 'template.json'
$newOrderPath = Join-Path $testRoot 'new-order.json'
$null = Invoke-CLI @('template','save','--name','示例交付','--version','1','--order',$orderPath,'--output',$templatePath,'--','tar.exe','-a','-cf','upgrade.zip','release.json','app','dependencies','migrations') 0
$null = Invoke-CLI @('order','create','--template',$templatePath,'--id','TEMPLATE-ACCEPTANCE','--output',$newOrderPath) 0
$newOrder = Get-Content -Raw -LiteralPath $newOrderPath | ConvertFrom-Json
if ($newOrder.template.name -cne '示例交付' -or $newOrder.template.version -cne '1' -or $newOrder.files.Count -ne 3) { throw '新工单应保留模板来源和完整规则' }
$newOrderHash = (Get-FileHash -LiteralPath $newOrderPath).Hash
$templateData = Get-Content -Raw -LiteralPath $templatePath | ConvertFrom-Json -AsHashtable
$templateData.version = '2'
Write-JSON $templatePath $templateData
if ((Get-FileHash -LiteralPath $newOrderPath).Hash -cne $newOrderHash) { throw '模板更新不能影响现有工单' }

$history = @(Invoke-CLI @('history','--results',$resultsRoot,'--order-id','EXTENDED-ACCEPTANCE','--status','rejected','--json') 0) -join [Environment]::NewLine | ConvertFrom-Json
if (@($history).Count -ne 3) { throw "拒收历史数量不符，当前 $(@($history).Count)" }
if (@($history | Where-Object { $_.status -cne 'rejected' }).Count -ne 0) { throw '历史状态筛选结果不符' }
Write-Output "EXTENDED_ACCEPTANCE_PASSED $testRoot"
