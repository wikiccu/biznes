#Requires -Version 7.0

[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('check', 'run', 'test', 'lint', 'fmt', 'fmt-check', 'db-up', 'db-down', 'migrate-up', 'migrate-down', 'migrate-status')]
    [string]$Command = 'check'
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false

Push-Location $PSScriptRoot
try {
    if ($Command -in 'check', 'fmt-check') {
        $unformatted = & gofmt -l cmd internal
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        if ($unformatted) {
            $unformatted
            Write-Output 'Run pwsh -File ./dev.ps1 fmt to format these files.'
            exit 1
        }
    }

    switch ($Command) {
        'check' {
            & go test ./...
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
            & go vet ./...
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
            & go mod verify
        }
        'run' { & go run ./cmd/api }
        'test' { & go test ./... }
        'lint' { & go vet ./... }
        'fmt' { & gofmt -w cmd internal }
        'db-up' { & docker compose up -d --wait --wait-timeout 60 postgres }
        'db-down' { & docker compose down }
        'migrate-up' { & go run ./cmd/migrate up }
        'migrate-down' { & go run ./cmd/migrate down }
        'migrate-status' { & go run ./cmd/migrate status }
    }

    exit $LASTEXITCODE
} finally {
    Pop-Location
}
