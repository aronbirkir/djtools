APP_NAME := djtools
APP_ID   := is.ankeri.djtools
APP_PKG  := ./cmd/djtools-app
DIST     := dist

GOGIO := go run gioui.org/cmd/gogio@v0.10.0

MAC_APP := $(DIST)/macos/$(APP_NAME).app

.PHONY: help run build test icon macos macos-arm64 macos-amd64 clean

help:
	@echo "Targets:"
	@echo "  run            Run the desktop app from source"
	@echo "  build          Build dj and djtools-app for this platform into $(DIST)/"
	@echo "  test           Run all tests"
	@echo "  icon           Regenerate cmd/djtools-app/icon.png"
	@echo "  macos          Universal (Apple Silicon + Intel) .app bundle"
	@echo "  macos-arm64    Apple Silicon .app bundle"
	@echo "  macos-amd64    Intel .app bundle"
	@echo "  clean          Remove $(DIST)/"

run:
	go run $(APP_PKG)

build:
	go build -o $(DIST)/dj ./cmd/dj
	go build -o $(DIST)/djtools-app $(APP_PKG)

test:
	go test -race ./...

icon:
	go generate $(APP_PKG)

# gogio marks the bundle as a generic bundle (BNDL); patch it to an
# application, then re-sign ad hoc since editing Info.plist breaks the signature.
define finish_mac_app
	plutil -replace CFBundlePackageType -string APPL "$(1)/Contents/Info.plist"
	plutil -replace CFBundleName -string "$(APP_NAME)" "$(1)/Contents/Info.plist"
	codesign --force --deep --sign - "$(1)"
endef

macos-arm64 macos-amd64: macos-%:
	rm -rf "$(DIST)/macos-$*"
	mkdir -p "$(DIST)/macos-$*"
	$(GOGIO) -target macos -arch $* -appid $(APP_ID) -icon $(APP_PKG)/icon.png -o "$(DIST)/macos-$*/$(APP_NAME).app" $(APP_PKG)
	$(call finish_mac_app,$(DIST)/macos-$*/$(APP_NAME).app)

# Build both architectures and merge the executables with lipo.
macos: macos-arm64 macos-amd64
	rm -rf "$(DIST)/macos"
	mkdir -p "$(DIST)/macos"
	cp -R "$(DIST)/macos-arm64/$(APP_NAME).app" "$(MAC_APP)"
	lipo -create \
		"$(DIST)/macos-arm64/$(APP_NAME).app/Contents/MacOS/$(APP_NAME)" \
		"$(DIST)/macos-amd64/$(APP_NAME).app/Contents/MacOS/$(APP_NAME)" \
		-output "$(MAC_APP)/Contents/MacOS/$(APP_NAME)"
	$(call finish_mac_app,$(MAC_APP))
	@echo "Built $(MAC_APP)"

clean:
	rm -rf $(DIST)
