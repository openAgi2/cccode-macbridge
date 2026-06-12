import XCTest

final class AppDependenciesTests: XCTestCase {
    func testReadsOpenCodeCredentialsFromLaunchAgent() throws {
        let url = try writeLaunchAgent(environment: [
            "OPENCODE_SERVER_USERNAME": "opencode",
            "OPENCODE_SERVER_PASSWORD": "local-test-password",
        ])
        defer { try? FileManager.default.removeItem(at: url.deletingLastPathComponent()) }

        let credentials = OpenCodeLaunchAgentCredentials.read(from: url)

        XCTAssertEqual(credentials?.user, "opencode")
        XCTAssertEqual(credentials?.password, "local-test-password")
    }

    func testRejectsIncompleteOpenCodeLaunchAgentCredentials() throws {
        let url = try writeLaunchAgent(environment: [
            "OPENCODE_SERVER_USERNAME": "opencode",
        ])
        defer { try? FileManager.default.removeItem(at: url.deletingLastPathComponent()) }

        XCTAssertNil(OpenCodeLaunchAgentCredentials.read(from: url))
    }

    private func writeLaunchAgent(environment: [String: String]) throws -> URL {
        let directory = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: true)
        try FileManager.default.createDirectory(
            at: directory,
            withIntermediateDirectories: true
        )

        let url = directory.appendingPathComponent("com.opencode.server.plist")
        let data = try PropertyListSerialization.data(
            fromPropertyList: ["EnvironmentVariables": environment],
            format: .xml,
            options: 0
        )
        try data.write(to: url)
        return url
    }
}
