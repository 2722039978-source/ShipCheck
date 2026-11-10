param([Parameter(Mandatory)][string]$Evidence)

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$projectRoot = Split-Path $PSScriptRoot
$testRoot = Join-Path $projectRoot ('.local\gui-extended-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $testRoot | Out-Null
Add-Type -Path (Join-Path $PSScriptRoot 'NativeUI.cs')

function Wait-Value([scriptblock]$Probe, [string]$Message) {
    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    do {
        if ($process.HasExited) { throw "窗口已退出，退出码 $($process.ExitCode)" }
        $value = & $Probe
        if ($value) { return $value }
        Start-Sleep -Milliseconds 60
    } while ([DateTime]::UtcNow -lt $deadline)
    throw $Message
}

function Find-Control([IntPtr]$Parent, [scriptblock]$Condition, [string]$Name) {
    $found = @([NativeUI]::Children($Parent) | Where-Object $Condition)
    if ($found.Count -ne 1) { throw "控件 $Name 数量为 $($found.Count)，预期 1" }
    $found[0].Handle
}

function Button([string]$Name) {
    Find-Control $mainWindow { $_.ClassName -eq 'Button' -and $_.Text -ceq $Name } $Name
}

function Field([string]$Label) {
    $caption = Find-Control $mainWindow { $_.ClassName -like '*Walk_Static_Class*' -and $_.Text -ceq $Label } $Label
    Find-Control ([NativeUI]::GetParent($caption)) { $_.ClassName -eq 'Edit' } $Label
}

function Wait-Report([string]$Pattern) {
    Wait-Value {
        $text = [NativeUI]::Text($report)
        if ($text -clike $Pattern) { $text }
    } "报告没有出现 $Pattern，当前内容为 $([NativeUI]::Text($report))"
}

function Save-Dialog([string]$Title, [string]$Path) {
    $dialog = Wait-Value { $handle = [NativeUI]::FindWindow($process.Id, $Title); if ($handle -ne [IntPtr]::Zero) { $handle } } "没有出现对话框 $Title"
    $fileName = Wait-Value {
        $fields = @([NativeUI]::Children($dialog) | Where-Object { $_.ClassName -eq 'Edit' -and $_.Id -in @(1148,1001) })
        if ($fields.Count -eq 1) { $fields[0].Handle }
    } '文件名输入控件未创建'
    [NativeUI]::SetText($fileName, $Path)
    $save = Find-Control $dialog { $_.ClassName -eq 'Button' -and $_.Id -eq 1 } '确认保存'
    [NativeUI]::Click($save)
    $null = Wait-Value { Test-Path -LiteralPath $Path -PathType Leaf } "文件未保存 $Path"
}

$process = Start-Process -FilePath (Join-Path $projectRoot 'ShipCheck.exe') -WorkingDirectory $testRoot -WindowStyle Hidden -PassThru
try {
    $mainWindow = Wait-Value { $handle = [NativeUI]::FindWindow($process.Id, 'ShipCheck'); if ($handle -ne [IntPtr]::Zero) { $handle } } '窗口未启动'
    $report = Wait-Value {
        $found = @([NativeUI]::Children($mainWindow) | Where-Object { $_.ClassName -eq 'Edit' -and ($_.Style -band 4) -and ($_.Style -band 0x800) })
        if ($found.Count -eq 1) { $found[0].Handle }
    } '报告控件未创建'
    $newOrderButton = Button '新建工单'
    $tabs = Find-Control $mainWindow { $_.ClassName -eq 'SysTabControl32' } '页面选择'
    1..3 | ForEach-Object { [NativeUI]::Right($tabs) }
    if ([NativeUI]::SelectedTab($tabs) -ne 3) { throw '工单页面未打开' }
    [NativeUI]::Click($newOrderButton)
    $null = Wait-Report '*填写工单号*'
    [NativeUI]::SetText((Field '编辑工单号'), 'GUI-EXTENDED')
    [NativeUI]::SetText((Field '起始版本'), '1.6')
    [NativeUI]::SetText((Field '目标版本'), '1.7')
    $editor = Find-Control $mainWindow { $_.ClassName -eq 'Edit' -and ($_.Style -band 4) -and !($_.Style -band 0x800) } '工单 JSON'
    [NativeUI]::SetText($editor, (Get-Content -LiteralPath (Join-Path $projectRoot 'examples\order.json') -Raw))
    [NativeUI]::Click((Button '校验工单'))
    $null = Wait-Report '*工单 GUI-EXTENDED*可以保存工单*'
    $orderPath = Join-Path $testRoot 'gui-order.json'
    [NativeUI]::Click((Button '保存工单并使用'))
    Save-Dialog '将工单保存到新文件' $orderPath
    $null = Wait-Report '*验收和打包页面已使用此工单*'
    $saved = Get-Content -LiteralPath $orderPath -Raw | ConvertFrom-Json
    if ($saved.order_id -cne 'GUI-EXTENDED' -or $saved.required_files.Count -ne 3) { throw '窗口保存的工单内容不符' }
    [NativeUI]::SetText((Field '模板名称'), '窗口验收模板')
    [NativeUI]::SetText((Field '模板版本'), '1')
    $templatePath = Join-Path $testRoot 'gui-template.json'
    [NativeUI]::Click((Button '保存为模板'))
    Save-Dialog '将模板保存到新文件' $templatePath
    $null = Wait-Report '*模板包含完整工单规则*'
    [NativeUI]::Click((Button '新建工单'))
    $null = Wait-Report '*填写工单号*'
    [NativeUI]::Click((Button '读取模板'))
    Save-Dialog '读取工单模板' $templatePath
    $null = Wait-Report '*打包页面已载入模板默认值*'
    $templateOrder = [NativeUI]::Text($editor) | ConvertFrom-Json
    if ($templateOrder.template.name -cne '窗口验收模板' -or $templateOrder.template.sha256.Length -ne 64) { throw '读取模板应保留规则来源' }
    Write-Output 'GUI_ORDER_AND_TEMPLATE_PASSED'

    [NativeUI]::Left($tabs)
    [NativeUI]::SetText((Field '证据目录'), (Resolve-Path -LiteralPath $Evidence).ProviderPath)
    [NativeUI]::SetText((Field '修正打包命令'), 'tar.exe -a -cf upgrade.zip release.json app dependencies migrations')
    [NativeUI]::Click((Button '创建修正尝试'))
    $revised = Wait-Report '*修正结果目录 *'
    if ($revised -cnotlike '*验收通过*') { throw "窗口修正尝试未通过 $revised" }
    if (![NativeUI]::IsWindowEnabled((Button '打开验收报告'))) { throw '修正报告应可打开' }
    Write-Output 'GUI_REVISION_PASSED'

    [NativeUI]::Right($tabs)
    [NativeUI]::Right($tabs)
    $evidenceOrder = Get-Content -LiteralPath (Join-Path $Evidence 'order.json') -Raw | ConvertFrom-Json
    [NativeUI]::SetText((Field '查询工单号'), $evidenceOrder.order_id)
    [NativeUI]::Click((Button '查询历史记录'))
    $null = Wait-Report '*选择记录后可以打开报告*'
    $historyList = Find-Control $mainWindow { $_.ClassName -eq 'ListBox' } '历史列表'
    [NativeUI]::SelectList($historyList, 0)
    $null = Wait-Report '*结果目录 *'
    if (![NativeUI]::IsWindowEnabled((Button '打开验收报告'))) { throw '历史记录的报告应可打开' }
    Write-Output 'GUI_HISTORY_QUERY_PASSED'

    [NativeUI]::Close($mainWindow)
    if (!$process.WaitForExit(5000) -or $process.ExitCode -ne 0) { throw '窗口没有正常退出' }
    Write-Output "GUI_EXTENDED_ACCEPTANCE_PASSED $testRoot"
} finally {
    if (!$process.HasExited) { Stop-Process -Id $process.Id }
}
