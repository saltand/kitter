set shell := ["zsh", "-cu"]

root := justfile_directory()

default:
    @just --list

# vet + race tests + mygo vet
check:
    cd native && go vet ./... && go test -race -count=3 ./... && go tool mygo vet .

# Standalone CLI → native/build/kitter
cli:
    cd native && go build -o build/kitter ./cmd/kitter

# .app bundle + dmg (code signing, dmg and notarization require macOS)
app:
    cd native && go tool mygo build -platform darwin/arm64

# Universal .app (both architectures; macOS only for signing/dmg)
app-universal:
    cd native && go tool mygo build -platform darwin/universal

# Dev server
run:
    cd native && go tool mygo dev

# Regenerate the committed macOS icon with Apple's standalone Icon Composer.
macos-icon:
    "{{root}}/scripts/export-macos-icon.sh"
