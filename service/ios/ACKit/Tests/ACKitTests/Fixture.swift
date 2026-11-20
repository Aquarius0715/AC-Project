import Foundation

/// The text of a fixture: a real answer of the Core API on the dev stack (Tests/ACKitTests/Fixtures).
func fixture(_ name: String) throws -> Data {
    guard let url = Bundle.module.url(forResource: name, withExtension: "json", subdirectory: "Fixtures") else {
        throw CocoaError(.fileNoSuchFile)
    }
    return try Data(contentsOf: url)
}
