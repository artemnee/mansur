APP := dist/Mansur.app

.PHONY: build run app dmg clean

build:
	CGO_ENABLED=0 go build -o bin/mansur .

run: build
	./bin/mansur -fg

# Universal .app (Apple Silicon + Intel), ad-hoc signed.
app:
	rm -rf $(APP) dist/Mansur.iconset
	mkdir -p $(APP)/Contents/MacOS $(APP)/Contents/Resources
	CGO_ENABLED=0 GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o dist/mansur-arm64 .
	CGO_ENABLED=0 GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/mansur-amd64 .
	lipo -create -output $(APP)/Contents/MacOS/mansur dist/mansur-arm64 dist/mansur-amd64
	cp packaging/Info.plist $(APP)/Contents/Info.plist
	dist/mansur-arm64 icon dist/Mansur.iconset
	iconutil -c icns -o $(APP)/Contents/Resources/Mansur.icns dist/Mansur.iconset
	codesign --force --deep -s - $(APP)
	rm -rf dist/mansur-arm64 dist/mansur-amd64 dist/Mansur.iconset

dmg: app
	rm -rf dist/dmg dist/Mansur.dmg
	mkdir -p dist/dmg
	cp -R $(APP) dist/dmg/
	ln -s /Applications dist/dmg/Applications
	hdiutil create -volname Mansur -srcfolder dist/dmg -ov -format UDZO dist/Mansur.dmg
	rm -rf dist/dmg

clean:
	rm -rf bin dist
