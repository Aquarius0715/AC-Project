/// The tests' wait between polls: none, the stand-in answers at once.
@Sendable func noWaitBetweenPolls(_: Duration) async {
    await Task.yield()
}

extension UnitControlFailureTests {
    static let noWait: @Sendable (Duration) async -> Void = noWaitBetweenPolls
}

extension UnitControlModelTests {
    static let noWait: @Sendable (Duration) async -> Void = noWaitBetweenPolls
}
