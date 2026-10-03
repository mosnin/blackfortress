// swift-tools-version:5.9
// Black Fortress — native macOS shell for the local compliance runtime.
// Build with `swift build -c release` or `scripts/build-app.sh` (assembles the .app).

import PackageDescription

let package = Package(
    name: "BlackFortress",
    platforms: [
        .macOS(.v14)
    ],
    products: [
        .executable(name: "BlackFortress", targets: ["BlackFortress"])
    ],
    targets: [
        .executableTarget(
            name: "BlackFortress",
            path: "Sources/BlackFortress"
        )
    ]
)
