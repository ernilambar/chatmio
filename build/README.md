# Build Directory

The build directory houses build files and assets for the macOS app bundle.

Structure:

* `bin` - Output directory
* `darwin` - macOS specific files
* `appicon.png` - Source icon

## macOS

The `darwin` directory holds files specific to Mac builds. These may be customised and used as part of the build.
To return these files to the default state, delete them and build with `wails build`.

- `Info.plist` - main plist used when building with `wails build`.
- `Info.dev.plist` - same as the main plist but used when building with `wails dev`.

Only arm64 macOS is supported; `scripts/build-app.sh` enforces this.
