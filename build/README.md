# Build Directory

The build directory houses build files and assets for the macOS app bundle.

Structure:

* `bin` - Output directory
* `darwin` - macOS specific files
* `appicon.svg` - Source icon (macOS squircle, 1024x1024 canvas)
* `appicon.png` - Rasterized icon consumed by `wails build`

## macOS

The `darwin` directory holds files specific to Mac builds. These may be customised and used as part of the build.
To return these files to the default state, delete them and build with `wails build`.

- `Info.plist` - main plist used when building with `wails build`.
- `Info.dev.plist` - same as the main plist but used when building with `wails dev`.

Only arm64 macOS is supported; `scripts/bundle.sh` enforces this and produces the publishable
artifacts under `dist/`.

Regenerate `appicon.png` after editing `appicon.svg` (Quick Look renders the SVG with alpha and
filters intact):

```sh
qlmanage -t -s 1024 -o build build/appicon.svg
mv build/appicon.svg.png build/appicon.png
```

`wails build` converts `appicon.png` into the bundle's `iconfile.icns`.
