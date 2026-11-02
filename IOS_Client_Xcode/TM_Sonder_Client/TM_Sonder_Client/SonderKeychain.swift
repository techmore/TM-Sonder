import Foundation
import Security

/// Minimal Keychain helper for server credentials (account session or legacy
/// pairing token). Tokens must not live in UserDefaults (backups / device sync).
/// Each server identity has its own entry; older builds kept one global entry,
/// which is adopted by the first server that asks for it.
enum SonderKeychain {
    private static let service = "com.tmsonder.client"
    private static let legacyAccount = "serverAccessToken"

    static func account(for server: URL?) -> String {
        guard let server else { return legacyAccount }
        return "\(legacyAccount)|\(server.absoluteString)"
    }

    static func loadAccessToken(for server: URL?) -> String? {
        if let token = load(account: account(for: server)), token.isEmpty == false { return token }
        guard server != nil, let legacy = load(account: legacyAccount), legacy.isEmpty == false else { return nil }
        saveAccessToken(legacy, for: server)
        delete(account: legacyAccount)
        return legacy
    }

    static func saveAccessToken(_ token: String, for server: URL?) {
        let trimmed = token.trimmingCharacters(in: .whitespacesAndNewlines)
        let account = account(for: server)
        delete(account: account)
        guard trimmed.isEmpty == false else { return }
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecValueData as String: Data(trimmed.utf8),
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        ]
        SecItemAdd(query as CFDictionary, nil)
    }

    static func deleteAccessToken(for server: URL?) {
        delete(account: account(for: server))
    }

    private static func load(account: String) -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]
        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        guard status == errSecSuccess, let data = item as? Data else { return nil }
        return String(data: data, encoding: .utf8)
    }

    private static func delete(account: String) {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account
        ]
        SecItemDelete(query as CFDictionary)
    }
}
