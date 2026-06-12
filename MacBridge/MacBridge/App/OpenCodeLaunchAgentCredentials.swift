import Foundation

enum OpenCodeLaunchAgentCredentials {
    static func read(from url: URL) -> (user: String, password: String)? {
        guard let data = try? Data(contentsOf: url),
              let plist = try? PropertyListSerialization.propertyList(
                  from: data,
                  options: [],
                  format: nil
              ) as? [String: Any],
              let environment = plist["EnvironmentVariables"] as? [String: Any],
              let user = environment["OPENCODE_SERVER_USERNAME"] as? String,
              let password = environment["OPENCODE_SERVER_PASSWORD"] as? String,
              !user.isEmpty,
              !password.isEmpty else {
            return nil
        }
        return (user, password)
    }
}
