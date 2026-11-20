// swift-tools-version: 6.2
// What the two iOS apps are made of (DEC-112). The app targets (service/ios/project.yml) only start a feature:
// - ACKit: the Core API contract types (generated), the REST client and the sign-in;
// - ACShared: the display language, the refusal texts, the session and the sign-in screen;
// - CustomerFeature, TechnicianFeature: each app's screens and their models.
// The models are tested here with `swift test`; the screens by the apps' UI tests.
import PackageDescription

let package = Package(
    name: "ACKit",
    platforms: [.iOS("17.4"), .macOS("14.4")],  // 17.4: the web authentication session with a callback scheme
    products: [
        .library(name: "ACKit", targets: ["ACKit"]),
        .library(name: "ACShared", targets: ["ACShared"]),
        .library(name: "CustomerFeature", targets: ["CustomerFeature"]),
        .library(name: "TechnicianFeature", targets: ["TechnicianFeature"]),
    ],
    targets: [
        .target(name: "ACKit"),
        .target(name: "ACShared", dependencies: ["ACKit"]),
        .target(name: "CustomerFeature", dependencies: ["ACKit", "ACShared"]),
        .target(name: "TechnicianFeature", dependencies: ["ACKit", "ACShared"]),
        // the tests' stand-ins (a transport, a session store): no product names it, so no app links it
        .target(name: "ACTestSupport", dependencies: ["ACKit"], path: "Tests/Support"),
        .testTarget(name: "ACKitTests", dependencies: ["ACKit", "ACTestSupport"], resources: [.copy("Fixtures")]),
        .testTarget(name: "ACSharedTests", dependencies: ["ACShared", "ACTestSupport"]),
        .testTarget(name: "CustomerFeatureTests", dependencies: ["CustomerFeature", "ACTestSupport"], resources: [.copy("Fixtures")]),
        .testTarget(name: "TechnicianFeatureTests", dependencies: ["TechnicianFeature", "ACTestSupport"], resources: [.copy("Fixtures")]),
    ]
)
